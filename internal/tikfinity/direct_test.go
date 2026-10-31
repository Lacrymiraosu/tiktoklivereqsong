package tikfinity

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

func waitStatus(t *testing.T, c *Client, ok func(Status) bool) Status {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if st := c.Status(); ok(st) {
			return st
		}
		time.Sleep(5 * time.Millisecond)
	}
	st := c.Status()
	t.Fatalf("status never matched: %+v", st)
	return st
}

func TestDirectMode(t *testing.T) {
	var mu sync.Mutex
	set := Settings{Direct: true, Reconnect: time.Millisecond}
	found := make(chan bool, 1)
	find := func(string) (string, bool) {
		mu.Lock()
		direct := set.Direct
		mu.Unlock()
		if direct {
			t.Error("direct mode looked for TikFinity")
		}
		select {
		case found <- true:
		default:
		}
		return "", false
	}
	c := New(Env{Find: find, Running: func(string) bool { return false }},
		func() Settings { mu.Lock(); defer mu.Unlock(); return set })
	var chats []Chat
	var gifts []Gift
	c.OnChat = func(ch Chat) { mu.Lock(); chats = append(chats, ch); mu.Unlock() }
	c.OnGift = func(g Gift) { mu.Lock(); gifts = append(gifts, g); mu.Unlock() }
	calls := make(chan string, 10)
	live := make(chan struct{})
	c.Direct = func(ctx context.Context, user, apiKey string, ev DirectEvents) error {
		calls <- user + "|" + apiKey
		switch user {
		case "nobody":
			return ErrNoAccount
		case "later":
			return fmt.Errorf("the LIVE ended: %w", ErrOffline)
		}
		ev.Connected()
		ev.Chat(Chat{User: "v", Nick: "v", Comment: "!req 1"})
		ev.Gift(Gift{User: "v", Coins: 5})
		close(live)
		<-ctx.Done()
		return ctx.Err()
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go c.Run(ctx)

	st := waitStatus(t, c, func(s Status) bool { return s.Code == "no_user" })
	if st.Level != Red || st.Source != "tiktok" || c.Accepting() {
		t.Errorf("no user: %+v", st)
	}

	mu.Lock()
	set.User = "nobody"
	mu.Unlock()
	c.Reconnect()
	waitStatus(t, c, func(s Status) bool { return s.Code == "no_account" && s.Level == Red })

	mu.Lock()
	set.User = "later"
	mu.Unlock()
	c.Reconnect()
	st = waitStatus(t, c, func(s Status) bool { return s.Code == "waiting_stream" })
	if st.Level != Yellow || st.Error != "" {
		t.Errorf("offline: %+v", st)
	}

	mu.Lock()
	set.User, set.APIKey = "streamer", "k"
	mu.Unlock()
	c.Reconnect()
	<-live
	st = waitStatus(t, c, func(s Status) bool { return s.Code == "live" })
	if st.Level != Green || !c.Accepting() || st.User != "streamer" {
		t.Errorf("live: %+v", st)
	}
	mu.Lock()
	if len(chats) != 1 || chats[0].Comment != "!req 1" || len(gifts) != 1 || gifts[0].Coins != 5 {
		t.Errorf("events: %+v %+v", chats, gifts)
	}
	mu.Unlock()
	_, first := c.Raw()
	if first["chat (command)"].Raw == "" || first["gift (counted)"].Raw == "" {
		t.Errorf("raw: %+v", first)
	}
	for len(calls) > 0 {
		if s := <-calls; s == "streamer|k" {
			goto keyed
		}
	}
	t.Error("the API key was not passed")
keyed:

	// Back to TikFinity: the direct stream stops and TikFinity is looked for.
	mu.Lock()
	set.Direct = false
	mu.Unlock()
	c.Reconnect()
	select {
	case <-found:
	case <-time.After(3 * time.Second):
		t.Fatal("did not switch back to TikFinity")
	}
	st = waitStatus(t, c, func(s Status) bool { return s.Source == "tikfinity" })
	if st.Code != "not_installed" || st.User != "" {
		t.Errorf("back to TikFinity: %+v", st)
	}
}

func TestDirectBackOff(t *testing.T) {
	c := New(Env{}, func() Settings { return Settings{} })
	c.Direct = func(context.Context, string, string, DirectEvents) error { return fmt.Errorf("dial: refused") }
	s := Settings{Direct: true, User: "x", Reconnect: 5 * time.Second}
	var waits []time.Duration
	for range 8 {
		waits = append(waits, c.direct(context.Background(), s))
	}
	want := []time.Duration{10, 20, 40, 80, 160, 300, 300, 300}
	for i, w := range want {
		if waits[i] != w*time.Second {
			t.Fatalf("waits = %v", waits)
		}
	}
	if st := c.Status(); st.Code != "connecting" || st.Error == "" {
		t.Errorf("status: %+v", st)
	}
	c.Direct = func(context.Context, string, string, DirectEvents) error { return ErrRateLimited }
	if w := c.direct(context.Background(), s); w != 5*time.Minute {
		t.Errorf("rate limited wait %v", w)
	}
}
