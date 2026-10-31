package tiktok

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	pb "github.com/steampoweredtaco/gotiktoklive/proto"
	"google.golang.org/protobuf/proto"

	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/tikfinity"
)

func TestCleanUser(t *testing.T) {
	for in, want := range map[string]string{"@Kiki_Nomae ": "kiki_nomae", "https://www.tiktok.com/@kiki.nomae/live?x=1": "kiki.nomae", "kiki": "kiki"} {
		if got := CleanUser(in); got != want {
			t.Errorf("CleanUser(%q) = %q, want %q", in, got, want)
		}
	}
}

func roomServer(t *testing.T, body string) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("uniqueId") != "streamer" {
			t.Errorf("uniqueId = %q", r.URL.Query().Get("uniqueId"))
		}
		w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestRoomID(t *testing.T) {
	cases := []struct {
		body string
		room string
		err  error
	}{
		{`{"statusCode":0,"data":{"user":{"roomId":"42","status":2},"liveRoom":{"status":2}}}`, "42", nil},
		{`{"statusCode":0,"data":{"user":{"roomId":"42","status":4},"liveRoom":{"status":4}}}`, "", tikfinity.ErrOffline},
		{`{"statusCode":19881007,"data":{}}`, "", tikfinity.ErrNoAccount},
	}
	for _, tc := range cases {
		c := New()
		c.RoomURL = roomServer(t, tc.body).URL
		room, err := c.RoomID(context.Background(), "streamer")
		if room != tc.room || !errors.Is(err, tc.err) {
			t.Errorf("%s: got %q, %v", tc.body, room, err)
		}
	}
}

func TestSignErrors(t *testing.T) {
	sign := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") == "bad" {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"message":"Invalid API key"}`))
			return
		}
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"message":"Daily limit reached"}`))
	}))
	defer sign.Close()
	c := New()
	c.RoomURL = roomServer(t, `{"statusCode":0,"data":{"user":{"roomId":"42","status":2}}}`).URL
	c.SignURL = sign.URL
	err := c.Stream(context.Background(), "streamer", "", tikfinity.DirectEvents{})
	if !errors.Is(err, tikfinity.ErrRateLimited) || !strings.Contains(err.Error(), "Daily limit") {
		t.Errorf("keyless: %v", err)
	}
	err = c.Stream(context.Background(), "streamer", "bad", tikfinity.DirectEvents{})
	if err == nil || !strings.Contains(err.Error(), "API key") || strings.Contains(err.Error(), "apiKey=bad") {
		t.Errorf("bad key: %v", err)
	}
}

func frame(t *testing.T, kind string, payload []byte) []byte {
	b, err := proto.Marshal(&pb.WebcastPushFrame{PayloadType: kind, Payload: payload, LogId: 7})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func message(t *testing.T, method string, m proto.Message) *pb.WebcastResponse_Message {
	b, err := proto.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return &pb.WebcastResponse_Message{Method: method, Payload: b}
}

func gz(b []byte) []byte {
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	w.Write(b)
	w.Close()
	return buf.Bytes()
}

func TestStream(t *testing.T) {
	now := time.Now().UnixMilli()
	viewer := &pb.User{DisplayId: "viewer1", Nickname: "Viewer One", IdStr: "123456789",
		AvatarThumb: &pb.Image{UrlList: []string{"https://evil.example/a.jpg", "https://p16.tiktokcdn.com/a.jpg"}}}
	ws := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("room_id") != "42" || r.URL.Query().Get("X-Bogus") != "sig" || r.Header.Get("Cookie") != "ttwid=abc" {
			t.Errorf("ws request: %s cookie %q", r.URL.RawQuery, r.Header.Get("Cookie"))
		}
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		defer conn.CloseNow()
		ctx := r.Context()
		_, b, err := conn.Read(ctx)
		var f pb.WebcastPushFrame
		if err != nil || proto.Unmarshal(b, &f) != nil || f.PayloadType != "im_enter_room" {
			t.Errorf("first frame: %v %q", err, f.PayloadType)
			return
		}
		conn.Write(ctx, websocket.MessageBinary, frame(t, "im_enter_room_resp", nil))
		resp := &pb.WebcastResponse{NeedsAck: true, InternalExt: []byte("ext"), Messages: []*pb.WebcastResponse_Message{
			message(t, "WebcastChatMessage", &pb.WebcastChatMessage{Common: &pb.Common{CreateTime: now - 60_000}, User: viewer, Content: "!req old"}),
			message(t, "WebcastChatMessage", &pb.WebcastChatMessage{Common: &pb.Common{CreateTime: now}, User: viewer, Content: "!req 2096297",
				UserIdentity: &pb.UserIdentity{IsFollowerOfAnchor: true, IsModeratorOfAnchor: true}}),
			message(t, "WebcastChatMessage", &pb.WebcastChatMessage{User: nil, Content: "no user"}),
			message(t, "WebcastGiftMessage", &pb.WebcastGiftMessage{User: viewer, RepeatCount: 3, RepeatEnd: 0, Gift: &pb.GiftStruct{Type: 1, DiamondCount: 1, Name: "Rose"}}),
			message(t, "WebcastGiftMessage", &pb.WebcastGiftMessage{User: viewer, RepeatCount: 5, RepeatEnd: 1, Gift: &pb.GiftStruct{Type: 1, DiamondCount: 1, Name: "Rose"}}),
			message(t, "WebcastGiftMessage", &pb.WebcastGiftMessage{User: viewer, Gift: &pb.GiftStruct{Type: 2, DiamondCount: 30, Name: "Doughnut"}}),
			message(t, "WebcastGiftMessage", &pb.WebcastGiftMessage{User: viewer}),
			message(t, "WebcastRoomUserSeqMessage", &pb.WebcastChatMessage{}),
		}}
		b, _ = proto.Marshal(resp)
		conn.Write(ctx, websocket.MessageBinary, frame(t, "msg", gz(b)))
		if _, b, err = conn.Read(ctx); err != nil || proto.Unmarshal(b, &f) != nil || f.PayloadType != "ack" || string(f.Payload) != "ext" || f.LogId != 7 {
			t.Errorf("ack: %v %q %q", err, f.PayloadType, f.Payload)
		}
		end := &pb.WebcastResponse{Messages: []*pb.WebcastResponse_Message{
			message(t, "WebcastControlMessage", &pb.WebcastControlMessage{Action: pb.ControlAction_STREAM_ENDED})}}
		b, _ = proto.Marshal(end)
		conn.Write(ctx, websocket.MessageBinary, frame(t, "msg", b))
		conn.Read(ctx) // until the client goes
	}))
	defer ws.Close()
	sign := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if r.URL.Path != "/webcast/fetch/" || q.Get("room_id") != "42" || q.Get("apiKey") != "key1" || r.Header.Get("x-api-key") != "key1" {
			t.Errorf("sign request: %s %s", r.URL.Path, r.URL.RawQuery)
		}
		b, _ := proto.Marshal(&pb.WebcastResponse{PushServer: "ws" + strings.TrimPrefix(ws.URL, "http") + "/ws", Cursor: "c1",
			RouteParamsMap: map[string]string{"X-Bogus": "sig", "empty": ""}})
		w.Header().Set("X-Set-TT-Cookie", "ttwid=abc; Path=/; Domain=tiktok.com")
		w.Write(b)
	}))
	defer sign.Close()

	c := New()
	c.RoomURL = roomServer(t, `{"statusCode":0,"data":{"user":{"roomId":"42","status":2},"liveRoom":{"status":2}}}`).URL
	c.SignURL = sign.URL
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var chats []tikfinity.Chat
	var gifts []tikfinity.Gift
	connected, other := 0, 0
	err := c.Stream(ctx, "streamer", "key1", tikfinity.DirectEvents{
		Connected: func() { connected++ },
		Chat:      func(ch tikfinity.Chat) { chats = append(chats, ch) },
		Gift:      func(g tikfinity.Gift) { gifts = append(gifts, g) },
		Other:     func() { other++ },
	})
	if !errors.Is(err, tikfinity.ErrOffline) {
		t.Fatalf("Stream = %v, want ErrOffline when the LIVE ends", err)
	}
	if connected != 1 || other != 1 {
		t.Errorf("connected %d, other %d", connected, other)
	}
	want := tikfinity.Chat{User: "viewer1", Nick: "Viewer One", Comment: "!req 2096297", Avatar: "https://p16.tiktokcdn.com/a.jpg",
		Moderator: tikfinity.Yes, Follower: tikfinity.Yes, Subscriber: tikfinity.No}
	if len(chats) != 1 || chats[0] != want {
		t.Errorf("chats = %+v", chats)
	}
	if len(gifts) != 2 || gifts[0].Coins != 5 || gifts[0].Name != "Rose" || gifts[1].Coins != 30 || gifts[1].User != "viewer1" {
		t.Errorf("gifts = %+v", gifts)
	}
}
