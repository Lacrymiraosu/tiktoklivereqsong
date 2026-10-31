package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/config"
	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/queue"
	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/tikfinity"
	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/tosu"
	"github.com/coder/websocket"
)

// fakeTosu sends one tosu v2 payload over and over until closed.
type fakeTosu struct {
	mu  sync.Mutex
	msg string
}

func (f *fakeTosu) put(msg string) { f.mu.Lock(); f.msg = msg; f.mu.Unlock() }

func (f *fakeTosu) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer c.CloseNow()
	for {
		f.mu.Lock()
		msg := f.msg
		f.mu.Unlock()
		if c.Write(r.Context(), websocket.MessageText, []byte(msg)) != nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// connectTosu points a.Tosu at a fake tosu sending msg and waits until it is connected.
func connectTosu(t *testing.T, a *App, msg string) *fakeTosu {
	t.Helper()
	fake := &fakeTosu{msg: msg}
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/websocket/v2"
	a.Tosu = tosu.New(tosu.Env{Running: func(string) bool { return true }, Path: func(string) (string, bool) { return "", false },
		Launch: func(string) error { return nil }}, func() tosu.Settings {
		return tosu.Settings{URL: url, Reconnect: 50 * time.Millisecond}
	})
	a.Tosu.OnGame = a.OnGame
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go a.Tosu.Run(ctx)
	waitFor(t, a.Tosu.Connected, "tosu")
	return fake
}

func waitFor(t *testing.T, cond func() bool, what string) {
	t.Helper()
	for i := 0; i < 300; i++ {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

const lotusPayload = `{"client":"stable","state":{"number":2,"name":"play"},
	"beatmap":{"id":1,"set":292301,"artist":"Susumu Hirasawa","title":"SWITCHED-ON LOTUS","mapper":"Dr Manhattan","version":"Nattahnam","stats":{"stars":{"total":7.64}}},
	"play":{"mods":{"name":"HD"}}}`

func TestNowPlayingFollowsTosu(t *testing.T) {
	a := newTestApp(t, nil)
	add(a, "req-1", "fan", 39804, time.Now())
	a.Queue.Promote("req-1", "played") // the queue says set 39804 is playing

	np := func(user string) FeedEvent {
		t.Helper()
		a.command(tikfinity.Chat{User: user, Comment: "!np"}, false)
		return a.Feed()[0]
	}
	// tosu not connected: the queue's now-playing map
	if f := np("u1"); f.Code != "np" || f.SetID != 39804 || f.Target != "fan" {
		t.Errorf("without tosu: %+v", f)
	}

	fake := connectTosu(t, a, lotusPayload)
	waitFor(t, func() bool { g, ok := a.tosuMap(); return ok && g.SetID == 292301 }, "tosu map")

	// osu! plays something nobody requested: tosu wins, no requester
	f := np("u2")
	if f.Code != "np" || f.SetID != 292301 || f.Title != "SWITCHED-ON LOTUS" || f.Artist != "Susumu Hirasawa" ||
		f.Version != "Nattahnam" || f.Mapper != "Dr Manhattan" || f.Stars != 7.64 || f.Mods != "HD" || f.Target != "" {
		t.Errorf("from tosu: %+v", f)
	}

	// the open map is the request: its requester is named
	fake.put(strings.Replace(lotusPayload, `"set":292301`, `"set":39804`, 1))
	waitFor(t, func() bool { g, _ := a.tosuMap(); return g.SetID == 39804 }, "tosu on the request")
	if f := np("u3"); f.SetID != 39804 || f.Target != "fan" || f.Title != "SWITCHED-ON LOTUS" {
		t.Errorf("tosu on the requested map: %+v", f)
	}
}

func TestAutoSkipsFailedFirstItem(t *testing.T) {
	a := newTestApp(t, func(c *config.Config) { c.Queue.PlayMode, c.Test.DryRun = "auto", true })
	t0 := time.Now()
	add(a, "bad", "u1", 1, t0)
	add(a, "good", "u2", 2, t0.Add(time.Second))
	a.Queue.Update("bad", func(x *queue.Item) { x.State = queue.Failed })
	a.Queue.Update("good", func(x *queue.Item) { x.State = queue.Ready })
	connectTosu(t, a, `{"client":"stable","state":{"number":5,"name":"selectPlay"},"beatmap":{"id":9,"set":9}}`)

	a.tick(context.Background())
	waitFor(t, func() bool { now, ok := a.Queue.Now(); return ok && now.ID == "good" }, "auto to open the map after the failed one")
}

func TestWarnRefusedOncePerMirror(t *testing.T) {
	a := newTestApp(t, nil)
	banned := []queue.Attempt{{Source: "catboy", Status: "failed", Message: "HTTP 403: you have been banned"}, {Source: "sayobot", Status: "ok"}}
	count := func() int {
		n := 0
		for _, x := range a.Notices() {
			if x.Key == "mirrorRefused" {
				n++
			}
		}
		return n
	}
	a.warnRefused(banned)
	a.warnRefused(banned)
	if n := count(); n != 1 {
		t.Fatalf("a refusing mirror is reported once, got %d", n)
	}
	if v := a.Notices()[0].Vars; v["src"] != "catboy" || !strings.Contains(v["msg"], "banned") {
		t.Errorf("notice vars: %v", v)
	}
	a.warnRefused([]queue.Attempt{{Source: "catboy", Status: "failed", Message: "HTTP 500"}})
	a.warnRefused([]queue.Attempt{{Source: "catboy", Status: "ok"}})
	a.warnRefused(banned)
	if n := count(); n != 2 {
		t.Errorf("refusing again after working should be reported again, got %d", n)
	}
}

func TestPrevious(t *testing.T) {
	a := newTestApp(t, func(c *config.Config) { c.Test.DryRun = true })
	if err := a.Previous(); err == nil {
		t.Fatal("no previous map yet")
	}
	t0 := time.Now()
	add(a, "one", "u1", 1, t0)
	add(a, "two", "u2", 2, t0.Add(time.Second))
	a.Queue.Promote("one", "played")
	a.Queue.Promote("two", "played")
	if err := a.Previous(); err != nil {
		t.Fatal(err)
	}
	if now, _ := a.Queue.Now(); now.ID != "one" {
		t.Errorf("now = %s", now.ID)
	}
	if ids := ids(a); len(ids) != 1 || ids[0] != "two" {
		t.Errorf("queue = %v", ids)
	}
}

func TestJumpOpensHistoryMap(t *testing.T) {
	a := newTestApp(t, nil)
	var opened []string
	a.OpenURL = func(u string) error { opened = append(opened, u); return nil }
	a.Queue.Add(&queue.Item{ID: "x", User: "u", SetID: 39804, BeatmapID: 86324, Title: "FREEDOM DiVE", AddedAt: time.Now()})
	a.Queue.Promote("x", "played")
	a.Queue.Finish("played")
	h := a.Queue.Snapshot().History
	if len(h) != 1 || h[0].BeatmapID != 86324 {
		t.Fatalf("history should keep the difficulty: %+v", h)
	}
	if err := a.Jump(h[0].SetID, h[0].BeatmapID); err != nil {
		t.Fatal(err)
	}
	if len(opened) != 1 || opened[0] != "osu://b/86324" {
		t.Errorf("opened %v", opened)
	}
	if a.Queue.Len() != 0 {
		t.Error("jumping must not touch the queue")
	}
	if err := a.Jump(39804, 0); err == nil {
		t.Error("an old entry without a difficulty and no osu! lookup should fail, not open a wrong link")
	}
}

func TestAutoNeedsPass(t *testing.T) {
	a := newTestApp(t, func(c *config.Config) {
		c.Queue.PlayMode, c.Queue.AutoGapSec, c.Queue.AutoNeedsPass, c.Test.DryRun = "auto", 0, true, true
	})
	t0 := time.Now()
	add(a, "cur", "u1", 1, t0)
	add(a, "next", "u2", 2, t0.Add(time.Second))
	a.Queue.Update("next", func(x *queue.Item) { x.State = queue.Ready })
	a.Queue.Promote("cur", "played")
	end := func(outcome string) {
		a.Queue.Update("cur", func(x *queue.Item) {
			x.PlayPhase, x.PhaseAt, x.Result = string(tosu.Ended), t0.Add(-time.Minute), &queue.Result{Outcome: outcome}
		})
	}
	connectTosu(t, a, `{"client":"stable","state":{"number":5,"name":"selectPlay"},"beatmap":{"id":9,"set":9}}`)

	end("failed")
	a.tick(context.Background())
	time.Sleep(100 * time.Millisecond)
	if now, _ := a.Queue.Now(); now.ID != "cur" {
		t.Fatalf("a failed play must not open the next map, now %s", now.ID)
	}
	end("passed")
	a.tick(context.Background())
	waitFor(t, func() bool { now, _ := a.Queue.Now(); return now.ID == "next" }, "the next map after a clear")
}
