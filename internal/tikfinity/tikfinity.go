// Package tikfinity watches TikFinity Desktop and reads TikTok LIVE events
// from its local WebSocket API. TikFinity is the only event source.
package tikfinity

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
)

const ProcessName = "TikFinity.exe"
const DownloadURL = "https://tikfinity.zerody.one/"

type Level string

const (
	Red    Level = "red"
	Yellow Level = "yellow"
	Green  Level = "green"
)

type Status struct {
	Level     Level     `json:"level"`
	Code      string    `json:"code"` // not_installed | not_running | connecting | waiting_stream | live
	Installed bool      `json:"installed"`
	Path      string    `json:"path"`
	Running   bool      `json:"running"`
	Connected bool      `json:"connected"`
	Handshake bool      `json:"handshake"`
	Live      bool      `json:"live"`
	LastEvent time.Time `json:"lastEvent"`
	Error     string    `json:"error,omitempty"`
}

type Chat struct {
	User    string // TikTok @uniqueId
	Nick    string
	Comment string
	// Viewer roles, when TikFinity forwards them. Unknown is the zero value,
	// so rules never block a viewer because a field is missing.
	Moderator  Tri
	Follower   Tri
	Subscriber Tri
}

// Tri is a yes/no answer that may be unknown.
type Tri int8

const (
	Unknown Tri = 0
	Yes     Tri = 1
	No      Tri = -1
)

func tri(b bool) Tri {
	if b {
		return Yes
	}
	return No
}

type RawEvent struct {
	At    time.Time `json:"at"`
	Event string    `json:"event"`
	Raw   string    `json:"raw"`
}

// Platform hooks, injected so the package stays testable.
type Env struct {
	Find    func(configured string) (string, bool)
	Running func(exe string) bool
	Launch  func(path string) error
}

type Settings struct {
	URL        string
	Path       string
	Reconnect  time.Duration
	AutoLaunch bool
	LogRaw     bool
}

type Client struct {
	Env      Env
	Settings func() Settings
	OnChat   func(Chat)
	OnStatus func(Status)

	mu          sync.Mutex
	st          Status
	raw         []RawEvent
	firstOfKind map[string]RawEvent
	lastLaunch  time.Time
	kick        chan struct{}
}

const liveTimeout = 5 * time.Minute

func New(env Env, settings func() Settings) *Client {
	return &Client{Env: env, Settings: settings, firstOfKind: map[string]RawEvent{}, kick: make(chan struct{}, 1)}
}

func (c *Client) Status() Status {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.st
}

// Raw returns recent raw events (newest first) and the first event of each kind.
func (c *Client) Raw() ([]RawEvent, map[string]RawEvent) {
	c.mu.Lock()
	defer c.mu.Unlock()
	first := map[string]RawEvent{}
	for k, v := range c.firstOfKind {
		first[k] = v
	}
	return append([]RawEvent(nil), c.raw...), first
}

// Reconnect makes the loop re-check immediately (e.g. after settings change).
func (c *Client) Reconnect() {
	select {
	case c.kick <- struct{}{}:
	default:
	}
}

// LaunchNow starts TikFinity if it is installed.
func (c *Client) LaunchNow() error {
	s := c.Settings()
	path, ok := c.Env.Find(s.Path)
	if !ok {
		return errNotInstalled
	}
	c.mu.Lock()
	c.lastLaunch = time.Now()
	c.mu.Unlock()
	err := c.Env.Launch(path)
	c.Reconnect()
	return err
}

type strErr string

func (e strErr) Error() string { return string(e) }

const errNotInstalled = strErr("TikFinity is not installed")

func (c *Client) set(f func(*Status)) {
	c.mu.Lock()
	old := c.st
	f(&c.st)
	c.st.Level, c.st.Code = classify(c.st)
	st := c.st
	c.mu.Unlock()
	if c.OnStatus != nil && (old.Level != st.Level || old.Code != st.Code || old.Connected != st.Connected || old.Error != st.Error || old.Installed != st.Installed || old.Running != st.Running) {
		c.OnStatus(st)
	}
}

func classify(s Status) (Level, string) {
	switch {
	case !s.Installed:
		return Red, "not_installed"
	case !s.Running && !s.Connected:
		return Red, "not_running"
	case !s.Connected:
		return Yellow, "connecting"
	case !s.Live:
		return Yellow, "waiting_stream"
	}
	return Green, "live"
}

// Accepting reports whether chat requests may be taken.
func (c *Client) Accepting() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.st.Connected
}

// Run checks TikFinity and keeps the WebSocket connected until ctx ends.
func (c *Client) Run(ctx context.Context) {
	for ctx.Err() == nil {
		s := c.Settings()
		path, installed := c.Env.Find(s.Path)
		running := c.Env.Running(ProcessName)
		c.set(func(st *Status) { st.Installed, st.Path, st.Running = installed, path, running })

		if installed && !running && s.AutoLaunch {
			c.mu.Lock()
			due := time.Since(c.lastLaunch) > 60*time.Second
			if due {
				c.lastLaunch = time.Now()
			}
			c.mu.Unlock()
			if due {
				if err := c.Env.Launch(path); err != nil {
					c.set(func(st *Status) { st.Error = "auto-launch failed: " + err.Error() })
				}
			}
		}
		// Try to connect even if the process check failed: the user may run a
		// portable build under another name.
		if installed || running {
			c.connect(ctx, s)
		}
		select {
		case <-ctx.Done():
			return
		case <-c.kick:
		case <-time.After(s.Reconnect):
		}
	}
}

func (c *Client) connect(ctx context.Context, s Settings) {
	dctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	conn, _, err := websocket.Dial(dctx, s.URL, nil)
	cancel()
	if err != nil {
		c.set(func(st *Status) {
			st.Connected, st.Handshake, st.Live = false, false, false
			st.Error = "WebSocket: " + shortErr(err)
		})
		return
	}
	defer conn.CloseNow()
	conn.SetReadLimit(16 << 20)
	c.set(func(st *Status) { st.Connected, st.Error = true, "" })

	// Watch for the live timeout and for settings changes that need a reconnect.
	wctx, wcancel := context.WithCancel(ctx)
	defer wcancel()
	go func() {
		t := time.NewTicker(10 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-wctx.Done():
				return
			case <-c.kick:
				conn.Close(websocket.StatusNormalClosure, "reconnect")
				return
			case <-t.C:
				c.set(func(st *Status) {
					if st.Live && time.Since(st.LastEvent) > liveTimeout {
						st.Live = false
					}
				})
			}
		}
	}()

	for {
		_, data, err := conn.Read(wctx)
		if err != nil {
			c.set(func(st *Status) {
				st.Connected, st.Handshake, st.Live = false, false, false
				if ctx.Err() == nil && websocket.CloseStatus(err) != websocket.StatusNormalClosure {
					st.Error = "disconnected: " + shortErr(err)
				}
			})
			return
		}
		c.handle(data, s.LogRaw)
	}
}

type envelope struct {
	Event string          `json:"event"`
	Data  json.RawMessage `json:"data"`
}

func (c *Client) handle(b []byte, logRaw bool) {
	var e envelope
	if err := json.Unmarshal(b, &e); err != nil || e.Event == "" {
		return
	}
	raw := string(b)
	if len(raw) > 2000 {
		raw = raw[:2000] + "…"
	}
	ev := RawEvent{At: time.Now(), Event: e.Event, Raw: raw}
	c.mu.Lock()
	if _, ok := c.firstOfKind[e.Event]; !ok && e.Event != "config" {
		c.firstOfKind[e.Event] = ev
	}
	if logRaw {
		c.raw = append([]RawEvent{ev}, c.raw...)
		if len(c.raw) > 100 {
			c.raw = c.raw[:100]
		}
	}
	c.mu.Unlock()

	if e.Event == "config" {
		c.set(func(st *Status) { st.Handshake = true })
		return
	}
	c.set(func(st *Status) { st.Handshake, st.Live, st.LastEvent = true, true, time.Now() })
	if e.Event == "chat" && c.OnChat != nil {
		if ch, ok := ParseChat(e.Data); ok {
			c.OnChat(ch)
		}
	}
}

// ParseChat extracts the comment and author from a TikFinity chat event.
// Field names follow TikFinity's forwarded TikTok payload (data.comment,
// data.uniqueId, data.nickname); a few alternates are accepted in case a
// TikFinity update nests them under data.user.
func ParseChat(data json.RawMessage) (Chat, bool) {
	var m map[string]any
	if json.Unmarshal(data, &m) != nil {
		return Chat{}, false
	}
	user, _ := m["user"].(map[string]any)
	pick := func(keys ...string) string {
		for _, k := range keys {
			if v, ok := m[k].(string); ok && v != "" {
				return v
			}
			if user != nil {
				if v, ok := user[k].(string); ok && v != "" {
					return v
				}
			}
		}
		return ""
	}
	ch := Chat{
		Comment: pick("comment", "text", "message"),
		User:    pick("uniqueId", "username", "userId"),
		Nick:    pick("nickname", "displayName"),
	}
	if ch.Nick == "" {
		ch.Nick = ch.User
	}
	if ch.Comment == "" || ch.User == "" {
		return Chat{}, false
	}
	// Role fields as TikTok sends them through TikFinity: isModerator,
	// isSubscriber, followRole (0 none, 1 follower, 2 friend), or the
	// userIdentity block. Not verified on a live stream yet.
	ident, _ := m["userIdentity"].(map[string]any)
	flag := func(keys ...string) Tri {
		for _, src := range []map[string]any{m, user, ident} {
			for _, k := range keys {
				if v, ok := src[k].(bool); ok {
					return tri(v)
				}
			}
		}
		return Unknown
	}
	ch.Moderator = flag("isModerator", "isModeratorOfAnchor")
	ch.Subscriber = flag("isSubscriber", "isSubscriberOfAnchor")
	ch.Follower = flag("isFollower", "isFollowerOfAnchor")
	for _, src := range []map[string]any{m, user} {
		if v, ok := src["followRole"].(float64); ok && ch.Follower == Unknown {
			ch.Follower = tri(v >= 1)
		}
	}
	return ch, true
}

func shortErr(err error) string {
	s := err.Error()
	if i := strings.LastIndex(s, ": "); i > 0 && len(s) > 120 {
		s = s[i+2:]
	}
	return s
}
