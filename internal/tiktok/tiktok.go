// Package tiktok reads a TikTok LIVE straight from TikTok, without TikFinity.
//
// TikTok's LIVE WebSocket only accepts a signed address. Euler Stream
// (eulerstream.com, the sign server TikTok-Live-Connector uses) signs it:
// without an API key it allows 5 connections a minute, 30 an hour and 100 a
// day, a free key allows more. Only connecting uses the sign server; whether
// the account is live comes from TikTok, and chat and gifts come straight from
// TikTok's WebSocket. Message types come from the gotiktoklive protobufs.
package tiktok

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/coder/websocket"
	pb "github.com/steampoweredtaco/gotiktoklive/proto"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"

	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/tikfinity"
)

const (
	SignServer = "https://tiktok.eulerstream.com"
	RoomAPI    = "https://www.tiktok.com/api-live/user/room/"
	userAgent  = "osureq/1.0 (TikTok LIVE request helper)"
)

type Client struct {
	HTTP    *http.Client
	SignURL string // SignServer, or a test server
	RoomURL string // RoomAPI, or a test server
	Ping    time.Duration
	Idle    time.Duration // no frame for this long: the connection is dead
}

func New() *Client {
	return &Client{HTTP: &http.Client{Timeout: 15 * time.Second}, SignURL: SignServer, RoomURL: RoomAPI, Ping: 10 * time.Second, Idle: time.Minute}
}

// CleanUser turns "@Name", a profile link or " name " into "name".
func CleanUser(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, "tiktok.com/"); i >= 0 {
		s = s[i+len("tiktok.com/"):]
	}
	s = strings.TrimPrefix(s, "@")
	if i := strings.IndexAny(s, "/?#"); i >= 0 {
		s = s[:i]
	}
	return strings.ToLower(strings.TrimSpace(s))
}

// RoomID returns the room of the account's LIVE: tikfinity.ErrOffline when
// it is not live, tikfinity.ErrNoAccount when there is no such account.
func (c *Client) RoomID(ctx context.Context, user string) (string, error) {
	q := url.Values{"aid": {"1988"}, "sourceType": {"54"}, "uniqueId": {user}}
	b, _, err := c.get(ctx, c.RoomURL+"?"+q.Encode(), nil)
	if err != nil {
		return "", fmt.Errorf("TikTok: %w", err)
	}
	var r struct {
		StatusCode int `json:"statusCode"`
		Data       struct {
			User struct {
				RoomID string `json:"roomId"`
				Status int    `json:"status"`
			} `json:"user"`
			LiveRoom *struct {
				Status int `json:"status"`
			} `json:"liveRoom"`
		} `json:"data"`
	}
	if err := json.Unmarshal(b, &r); err != nil {
		return "", fmt.Errorf("TikTok: unexpected answer: %w", err)
	}
	switch r.StatusCode {
	case 0:
	case 19881007: // user does not exist
		return "", tikfinity.ErrNoAccount
	default:
		return "", fmt.Errorf("TikTok: statusCode %d", r.StatusCode)
	}
	status := r.Data.User.Status
	if r.Data.LiveRoom != nil {
		status = r.Data.LiveRoom.Status
	}
	if r.Data.User.RoomID == "" || status != 2 { // 2 live, 4 ended
		return "", tikfinity.ErrOffline
	}
	return r.Data.User.RoomID, nil
}

// Stream connects to the account's LIVE and calls ev until ctx ends or the
// connection drops (see tikfinity.Direct). It is a tikfinity.Direct.
func (c *Client) Stream(ctx context.Context, user, apiKey string, ev tikfinity.DirectEvents) error {
	room, err := c.RoomID(ctx, user)
	if err != nil {
		return err
	}
	first, cookie, err := c.sign(ctx, room, apiKey)
	if err != nil {
		return err
	}
	q := url.Values{"version_code": {"180800"}, "aid": {"1988"}, "app_name": {"tiktok_web"}, "device_platform": {"web"},
		"browser_online": {"true"}, "cookie_enabled": {"true"}, "identity": {"audience"}, "live_id": {"12"}, "ws_direct": {"0"},
		"sup_ws_ds_opt": {"1"}, "update_version_code": {"2.0.0"}, "did_rule": {"3"}, "heartbeat_duration": {"0"},
		"resp_content_type": {"protobuf"}, "history_comment_count": {"0"}, "client_enter": {"1"}, "compress": {"gzip"},
		"room_id": {room}, "internal_ext": {string(first.InternalExt)}, "cursor": {first.Cursor}, "User-Agent": {userAgent}}
	for k, v := range first.RouteParamsMap {
		if v != "" {
			q.Set(k, v)
		}
	}
	h := http.Header{"User-Agent": {userAgent}}
	if cookie != "" {
		h.Set("Cookie", cookie)
	}
	dctx, dcancel := context.WithTimeout(ctx, 15*time.Second)
	conn, _, err := websocket.Dial(dctx, first.PushServer+"?"+q.Encode()+"&version_code=270000", &websocket.DialOptions{HTTPHeader: h, HTTPClient: c.HTTP})
	dcancel()
	if err != nil {
		return fmt.Errorf("TikTok WebSocket: %w", plain(err))
	}
	defer conn.CloseNow()
	conn.SetReadLimit(16 << 20)

	rid, _ := strconv.ParseInt(room, 10, 64)
	if err := send(ctx, conn, "im_enter_room", enterRoom(rid)); err != nil {
		return fmt.Errorf("TikTok WebSocket: %w", err)
	}
	wctx, wcancel := context.WithCancel(ctx)
	defer wcancel()
	go func() {
		t := time.NewTicker(c.Ping)
		defer t.Stop()
		for seq := uint64(1); ; seq++ {
			select {
			case <-wctx.Done():
				return
			case <-t.C:
				var hb []byte
				hb = protowire.AppendTag(hb, 1, protowire.VarintType)
				hb = protowire.AppendVarint(hb, uint64(rid))
				hb = protowire.AppendTag(hb, 2, protowire.VarintType)
				hb = protowire.AppendVarint(hb, seq)
				send(wctx, conn, "hb", hb)
			}
		}
	}()

	since := time.Now().Add(-10 * time.Second).UnixMilli() // older chat is history, not new requests
	entered := false
	for {
		rctx, rcancel := context.WithTimeout(wctx, c.Idle)
		_, data, err := conn.Read(rctx)
		rcancel()
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if reason := closeReason(err); reason != "" {
				return fmt.Errorf("TikTok closed the connection (%s)", reason)
			}
			return fmt.Errorf("TikTok WebSocket: %w", plain(err))
		}
		var f pb.WebcastPushFrame
		if proto.Unmarshal(data, &f) != nil {
			continue
		}
		payload := unzip(f.Payload)
		switch f.PayloadType {
		case "im_enter_room_resp":
			if !entered && ev.Connected != nil {
				ev.Connected()
			}
			entered = true
			continue
		case "msg":
		default:
			continue
		}
		var r pb.WebcastResponse
		if proto.Unmarshal(payload, &r) != nil {
			continue
		}
		if r.NeedsAck {
			ack, _ := proto.Marshal(&pb.WebcastPushFrame{LogId: f.LogId, PayloadEncoding: "pb", PayloadType: "ack", Payload: r.InternalExt})
			conn.Write(wctx, websocket.MessageBinary, ack)
		}
		if !entered { // some rooms send messages before the enter answer
			entered = true
			if ev.Connected != nil {
				ev.Connected()
			}
		}
		for _, m := range r.Messages {
			if m.GetIsHistory() {
				continue
			}
			if err := handle(m, since, ev); err != nil {
				return err
			}
		}
	}
}

// handle passes one LIVE message on. It returns tikfinity.ErrOffline when the
// LIVE ended.
func handle(m *pb.WebcastResponse_Message, since int64, ev tikfinity.DirectEvents) error {
	switch m.GetMethod() {
	case "WebcastChatMessage":
		var x pb.WebcastChatMessage
		if proto.Unmarshal(m.GetPayload(), &x) != nil {
			return nil
		}
		if t := x.GetCommon().GetCreateTime(); t > 0 && t < since {
			return nil
		}
		if ch, ok := chat(&x); ok && ev.Chat != nil {
			ev.Chat(ch)
		}
		return nil
	case "WebcastGiftMessage":
		var x pb.WebcastGiftMessage
		if proto.Unmarshal(m.GetPayload(), &x) != nil {
			return nil
		}
		if g, ok := gift(&x); ok && ev.Gift != nil {
			ev.Gift(g)
		}
		return nil
	case "WebcastControlMessage":
		var x pb.WebcastControlMessage
		if proto.Unmarshal(m.GetPayload(), &x) == nil {
			if a := x.GetAction(); a == pb.ControlAction_STREAM_ENDED || a == pb.ControlAction_STREAM_ENDED_BAN {
				return fmt.Errorf("the LIVE ended: %w", tikfinity.ErrOffline)
			}
		}
	}
	if ev.Other != nil {
		ev.Other()
	}
	return nil
}

func chat(x *pb.WebcastChatMessage) (tikfinity.Chat, bool) {
	u := x.GetUser()
	ch := tikfinity.Chat{User: u.GetDisplayId(), Nick: u.GetNickname(), Comment: x.GetContent(), Avatar: avatar(u)}
	if ch.Nick == "" {
		ch.Nick = ch.User
	}
	if id := x.GetUserIdentity(); id != nil {
		ch.Moderator = tri(id.GetIsModeratorOfAnchor() || id.GetIsAnchor())
		ch.Follower = tri(id.GetIsFollowerOfAnchor())
		ch.Subscriber = tri(id.GetIsSubscriberOfAnchor())
	}
	return ch, ch.User != "" && ch.Comment != ""
}

// gift counts a streakable gift (type 1) once, on its last message, like
// tikfinity.ParseGift.
func gift(x *pb.WebcastGiftMessage) (tikfinity.Gift, bool) {
	gs := x.GetGift()
	if gs == nil || (gs.GetType() == 1 && x.GetRepeatEnd() == 0) {
		return tikfinity.Gift{}, false
	}
	count := int(x.GetRepeatCount())
	if count < 1 {
		count = 1
	}
	u := x.GetUser()
	g := tikfinity.Gift{User: u.GetDisplayId(), Nick: u.GetNickname(), Name: gs.GetName(), Coins: int(gs.GetDiamondCount()) * count, Avatar: avatar(u)}
	if g.Nick == "" {
		g.Nick = g.User
	}
	return g, g.User != "" && g.Coins > 0
}

func avatar(u *pb.User) string {
	for _, img := range []*pb.Image{u.GetAvatarThumb(), u.GetAvatarMedium(), u.GetAvatarLarge(), u.GetAvatarJpg()} {
		for _, s := range img.GetUrlList() {
			if v := tikfinity.SafeAvatar(s); v != "" {
				return v
			}
		}
	}
	return ""
}

func tri(b bool) tikfinity.Tri {
	if b {
		return tikfinity.Yes
	}
	return tikfinity.No
}

// sign asks the sign server for the room's signed WebSocket address and the
// cookie that goes with it.
func (c *Client) sign(ctx context.Context, room, apiKey string) (*pb.WebcastResponse, string, error) {
	q := url.Values{"client": {"osureq"}, "room_id": {room}, "uuc": {"1"}, "client_enter": {"true"}}
	h := http.Header{"Accept": {"application/protobuf"}}
	if apiKey != "" {
		q.Set("apiKey", apiKey)
		h.Set("x-api-key", apiKey)
	}
	b, rh, err := c.get(ctx, c.SignURL+"/webcast/fetch/?"+q.Encode(), h)
	if err != nil {
		var se *statusErr
		if errors.As(err, &se) {
			return nil, "", se.signErr()
		}
		return nil, "", fmt.Errorf("sign server: %w", err)
	}
	var r pb.WebcastResponse
	if err := proto.Unmarshal(b, &r); err != nil || r.PushServer == "" || !strings.HasPrefix(r.PushServer, "wss://") && !strings.HasPrefix(r.PushServer, "ws://127.0.0.1") {
		return nil, "", errors.New("sign server: no WebSocket address in the answer")
	}
	return &r, cookie(rh.Get("X-Set-TT-Cookie")), nil
}

// cookie turns the sign server's Set-Cookie value into a Cookie header.
func cookie(set string) string {
	if set == "" {
		return ""
	}
	if c, err := http.ParseSetCookie(set); err == nil {
		return c.Name + "=" + c.Value
	}
	name, _, _ := strings.Cut(set, ";")
	return strings.TrimSpace(name)
}

type statusErr struct {
	code int
	body []byte
}

func (e *statusErr) Error() string { return "HTTP " + strconv.Itoa(e.code) }

func (e *statusErr) signErr() error {
	var j struct {
		Message string `json:"message"`
	}
	json.Unmarshal(e.body, &j)
	msg := strings.TrimSpace(j.Message)
	if len(msg) > 200 {
		msg = msg[:200] + "…"
	}
	switch {
	case e.code == http.StatusTooManyRequests:
		if msg == "" {
			return tikfinity.ErrRateLimited
		}
		return fmt.Errorf("%w: %s", tikfinity.ErrRateLimited, msg)
	case e.code == http.StatusUnauthorized || e.code == http.StatusForbidden:
		return fmt.Errorf("sign server refused the API key (HTTP %d) %s", e.code, msg)
	case msg != "":
		return fmt.Errorf("sign server: HTTP %d: %s", e.code, msg)
	}
	return fmt.Errorf("sign server: HTTP %d", e.code)
}

func (c *Client) get(ctx context.Context, u string, h http.Header) ([]byte, http.Header, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, nil, err
	}
	for k, v := range h {
		req.Header[k] = v
	}
	req.Header.Set("User-Agent", userAgent)
	rsp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, nil, plain(err)
	}
	defer rsp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(rsp.Body, 8<<20))
	if err != nil {
		return nil, nil, plain(err)
	}
	if rsp.StatusCode != http.StatusOK {
		return nil, nil, &statusErr{rsp.StatusCode, b}
	}
	return b, rsp.Header, nil
}

// plain drops the URL from an HTTP error, so an API key in it never reaches
// the log or the dashboard.
func plain(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		return ue.Err
	}
	return err
}

func enterRoom(room int64) []byte {
	var e []byte
	e = protowire.AppendTag(e, 1, protowire.VarintType) // room_id
	e = protowire.AppendVarint(e, uint64(room))
	e = protowire.AppendTag(e, 4, protowire.VarintType) // live_id
	e = protowire.AppendVarint(e, 12)
	e = protowire.AppendTag(e, 5, protowire.BytesType) // identity
	e = protowire.AppendString(e, "audience")
	e = protowire.AppendTag(e, 8, protowire.VarintType) // enter_unique_id
	e = protowire.AppendVarint(e, uint64(rand.Int64N(1<<62)+1))
	e = protowire.AppendTag(e, 9, protowire.BytesType) // filter_welcome_msg
	e = protowire.AppendString(e, "0")
	return e
}

func send(ctx context.Context, conn *websocket.Conn, kind string, payload []byte) error {
	b, err := proto.Marshal(&pb.WebcastPushFrame{PayloadEncoding: "pb", PayloadType: kind, Payload: payload})
	if err != nil {
		return err
	}
	return conn.Write(ctx, websocket.MessageBinary, b)
}

func unzip(b []byte) []byte {
	if len(b) < 2 || b[0] != 0x1f || b[1] != 0x8b {
		return b
	}
	zr, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		return b
	}
	out, err := io.ReadAll(io.LimitReader(zr, 16<<20))
	if err != nil {
		return b
	}
	return out
}

func closeReason(err error) string {
	var ce websocket.CloseError
	if !errors.As(err, &ce) {
		return ""
	}
	if ce.Reason != "" {
		return ce.Reason
	}
	return strconv.Itoa(int(ce.Code))
}
