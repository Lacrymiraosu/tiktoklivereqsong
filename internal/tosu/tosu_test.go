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
		"beatmap":{"id":2161977,"set":1001507,"checksum":"0E408D471D32017A53D9F8F22D574874","time":{"live":1234},
			"artist":"Susumu Hirasawa","title":"SWITCHED-ON LOTUS","mapper":"Dr Manhattan","version":"Nattahnam","stats":{"stars":{"live":1.2,"total":7.64}}},
		"play":{"failed":false,"playerName":"x"},"leaderboard":[]}`))
	want := Game{Client: "stable", State: "play", SetID: 1001507, BeatmapID: 2161977, Checksum: "0e408d471d32017a53d9f8f22d574874",
		Title: "SWITCHED-ON LOTUS", Artist: "Susumu Hirasawa", Mapper: "Dr Manhattan", Version: "Nattahnam", Stars: 7.64}
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

func TestParseScores(t *testing.T) {
	g, ok := Parse([]byte(`{"state":{"name":"resultScreen"},"beatmap":{"set":1},
		"play":{"failed":true,"accuracy":97.5,"score":123,"combo":{"current":5,"max":300},"rank":{"current":"A","maxThisPlay":"S"},"pp":{"current":88.8},"mods":{"name":"HD","number":8},"hits":{"0":3,"300":10}},
		"resultsScreen":{"accuracy":98.52,"score":9000000,"maxCombo":812,"rank":"S","pp":{"current":301.4,"fc":310},"mods":{"name":"HDHR"},"hits":{"0":2}}}`))
	if !ok {
		t.Fatal("parse failed")
	}
	if g.Play != (Score{Accuracy: 97.5, Rank: "A", MaxCombo: 300, Score: 123, PP: 88.8, Mods: "HD", Misses: 3}) || !g.Failed {
		t.Errorf("play = %+v failed=%v", g.Play, g.Failed)
	}
	if g.Results != (Score{Accuracy: 98.52, Rank: "S", MaxCombo: 812, Score: 9000000, PP: 301.4, Mods: "HDHR", Misses: 2}) {
		t.Errorf("results = %+v", g.Results)
	}
}

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
		{"retry after a quit", []Game{g("play", req), g("selectPlay", req), g("play", req)}, Playing},
		{"retry after a quit, then cleared", []Game{g("play", req), g("selectPlay", req), g("play", req), g("resultScreen", req)}, Results},
		{"other map after the end", []Game{g("play", req), g("selectPlay", req), g("play", 7)}, Ended},
	}
	for _, c := range cases {
		var tr Tracker
		tr.Reset(req, time.Unix(0, 0))
		got := run(&tr, c.steps...)
		if got[len(got)-1] != c.want {
			t.Errorf("%s: phases %v, want final %s", c.name, got, c.want)
		}
	}
	// osu! reports set -1 for a map whose .osu has no set ID: the song still matches
	tr := Tracker{Song: SongKey("Susumu Hirasawa", "SWITCHED-ON LOTUS")}
	tr.Reset(req, time.Unix(0, 0))
	song := func(state string) Game {
		return Game{State: state, SetID: -1, Artist: "Susumu Hirasawa", Title: "Switched-On Lotus"}
	}
	if got := run(&tr, song("selectPlay"), song("play"), song("resultScreen"), song("selectPlay")); got[len(got)-1] != Ended {
		t.Errorf("same song, other set id: phases %v", got)
	}
	tr.Reset(req, time.Unix(0, 0))
	if got := run(&tr, Game{State: "play", SetID: -1, Artist: "x", Title: "other song"}); got[0] != Waiting {
		t.Errorf("another song must not count: %v", got)
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

func TestParseSelectedMode(t *testing.T) {
	for raw, want := range map[string]string{
		`{"state":{"name":"selectPlay"},"settings":{"mode":{"number":3,"name":"mania"}}}`: "mania",
		`{"state":{"name":"selectPlay"},"settings":{"mode":{"number":0,"name":"osu"}}}`:   "osu",
		`{"state":{"name":"selectPlay"},"settings":{"mode":{"name":"fruits"}}}`:           "fruits",
		`{"state":{"name":"selectPlay"}}`:                                                 "",
	} {
		if g, _ := Parse([]byte(raw)); g.Mode != want {
			t.Errorf("%s -> %q, want %q", raw, g.Mode, want)
		}
	}
}

func TestStopHoldsOffAutoLaunch(t *testing.T) {
	var mu sync.Mutex
	launches, closed := 0, ""
	c := New(Env{
		Running: func(string) bool { return false },
		Path:    func(string) (string, bool) { return "", false },
		Launch:  func(string) error { mu.Lock(); launches++; mu.Unlock(); return nil },
		Close:   func(exe string) (int, error) { mu.Lock(); closed = exe; mu.Unlock(); return 1, nil },
	}, func() Settings {
		return Settings{URL: "ws://127.0.0.1:1/websocket/v2", Path: `C:\tosu\tosu.exe`, Reconnect: 20 * time.Millisecond, AutoLaunch: true}
	})
	if n, err := c.StopNow(); n != 1 || err != nil {
		t.Fatalf("StopNow = %d, %v", n, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go c.Run(ctx)
	time.Sleep(200 * time.Millisecond)
	mu.Lock()
	if closed != ProcessName || launches != 0 {
		t.Errorf("after StopNow: closed %q, %d auto-launches (want %q, 0)", closed, launches, ProcessName)
	}
	mu.Unlock()
	if err := c.LaunchNow(); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if launches != 1 {
		t.Errorf("LaunchNow after StopNow: %d launches, want 1", launches)
	}
}
