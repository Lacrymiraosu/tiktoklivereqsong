package tosu

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestParse(t *testing.T) {
	g, ok := Parse([]byte(`{"client":"stable","state":{"number":2,"name":"play"},
		"beatmap":{"id":2161977,"set":1001507,"checksum":"0E408D471D32017A53D9F8F22D574874","time":{"live":1234}},
		"play":{"failed":false,"playerName":"x"},"leaderboard":[]}`))
	want := Game{Client: "stable", State: "play", SetID: 1001507, BeatmapID: 2161977, Checksum: "0e408d471d32017a53d9f8f22d574874"}
	if !ok || g != want {
		t.Fatalf("got %+v %v", g, ok)
	}
	// only the number present -> name from tosu's GameState enum
	if g, _ := Parse([]byte(`{"state":{"number":7}}`)); g.State != "resultScreen" {
		t.Errorf("number 7 = %q, want resultScreen", g.State)
	}
	if _, ok := Parse([]byte(`{"error":"osu! is not running"}`)); ok {
		t.Error("an error payload must not count as game state")
	}
	if _, ok := Parse([]byte(`not json`)); ok {
		t.Error("garbage must not parse")
	}
}

func run(tr *Tracker, steps ...Game) []Phase {
	var out []Phase
	now := time.Unix(0, 0)
	for _, g := range steps {
		now = now.Add(time.Second)
		tr.Update(g, now)
		out = append(out, tr.Phase)
	}
	return out
}

func g(state string, set int) Game { return Game{State: state, SetID: set} }

func TestTracker(t *testing.T) {
	const req = 100
	cases := []struct {
		name  string
		steps []Game
		want  Phase
	}{
		{"played, results, back to select", []Game{g("selectPlay", req), g("play", req), g("resultScreen", req), g("selectPlay", req)}, Ended},
		{"sitting on results is not the end", []Game{g("play", req), g("resultScreen", req)}, Results},
		{"quit mid-song", []Game{g("play", req), g("selectPlay", req)}, Ended},
		{"retries stay playing", []Game{g("play", req), g("play", req), g("play", req)}, Playing},
		{"retry from results", []Game{g("play", req), g("resultScreen", req), g("play", req)}, Playing},
		{"never started: other map played", []Game{g("selectPlay", 7), g("play", 7), g("selectPlay", 7)}, Waiting},
		{"song select alone does not count", []Game{g("selectPlay", req), g("menu", req)}, Waiting},
		{"straight into another map", []Game{g("play", req), g("play", 7)}, Ended},
		{"loading glitches are ignored", []Game{g("play", req), g("busy", 0), g("unknown", 0), g("", 0), g("play", req)}, Playing},
		{"results then menu", []Game{g("play", req), g("resultScreen", req), g("menu", 0)}, Ended},
	}
	for _, c := range cases {
		var tr Tracker
		tr.Reset(req, time.Unix(0, 0))
		got := run(&tr, c.steps...)
		if got[len(got)-1] != c.want {
			t.Errorf("%s: phases %v, want final %s", c.name, got, c.want)
		}
	}
	var idle Tracker
	if idle.Update(g("play", req), time.Now()) {
		t.Error("a tracker with no map must not change")
	}
}

// fakeTosu serves /websocket/v2 and pushes whatever state the test sets.
type fakeTosu struct {
	mu     sync.Mutex
	state  string
	set    int
	closed bool // drop connections, as if tosu quit
}

func (f *fakeTosu) put(state string, set int) {
	f.mu.Lock()
	f.state, f.set = state, set
	f.mu.Unlock()
}

func (f *fakeTosu) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer c.CloseNow()
	for {
		f.mu.Lock()
		if f.closed {
			f.mu.Unlock()
			return
		}
		msg := fmt.Sprintf(`{"client":"stable","state":{"number":0,"name":%q},"beatmap":{"id":1,"set":%d,"checksum":"ab"},"play":{"failed":false}}`, f.state, f.set)
		f.mu.Unlock()
		if c.Write(r.Context(), websocket.MessageText, []byte(msg)) != nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestClientFollowsGameState(t *testing.T) {
	fake := &fakeTosu{state: "menu", set: 5}
	srv := httptest.NewServer(fake)
	defer srv.Close()
	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/websocket/v2"

	c := New(Env{Running: func(string) bool { return true }, Path: func(string) (string, bool) { return `C:\tosu\tosu.exe`, true }, Launch: func(string) error { return nil }},
		func() Settings { return Settings{URL: url, Reconnect: 50 * time.Millisecond} })
	var mu sync.Mutex
	var seen []string
	c.OnGame = func(g Game) { mu.Lock(); seen = append(seen, g.State); mu.Unlock() }
	var path string
	c.OnPath = func(p string) { path = p }
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go c.Run(ctx)

	wait := func(cond func() bool, what string) {
		t.Helper()
		for i := 0; i < 200; i++ {
			if cond() {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatalf("timed out waiting for %s (status %+v)", what, c.Status())
	}
	wait(func() bool { return c.Connected() && c.Status().Level == Green }, "green")
	fake.put("play", 5)
	wait(func() bool { return c.Status().Game.State == "play" }, "play")
	mu.Lock()
	n := 0
	for _, s := range seen {
		if s == "menu" {
			n++
		}
	}
	mu.Unlock()
	if n != 1 {
		t.Errorf("repeated identical payloads should be reported once, saw menu %d times", n)
	}
	if path != `C:\tosu\tosu.exe` {
		t.Errorf("running tosu path not reported: %q", path)
	}
	fake.mu.Lock()
	fake.closed = true
	fake.mu.Unlock()
	wait(func() bool { return !c.Connected() }, "disconnect")
}
