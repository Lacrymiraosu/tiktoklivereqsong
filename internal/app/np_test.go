package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

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

const lotusPayload = `{"client":"stable","state":{"number":2,"name":"play"},
	"beatmap":{"id":1,"set":292301,"artist":"Susumu Hirasawa","title":"SWITCHED-ON LOTUS","mapper":"Dr Manhattan","version":"Nattahnam","stats":{"stars":{"total":7.64}}},
	"play":{"mods":{"name":"HD"}}}`

func TestNowPlayingFollowsTosu(t *testing.T) {
	fake := &fakeTosu{msg: lotusPayload}
	srv := httptest.NewServer(fake)
	defer srv.Close()
	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/websocket/v2"

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

	a.Tosu = tosu.New(tosu.Env{Running: func(string) bool { return true }, Path: func(string) (string, bool) { return "", false },
		Launch: func(string) error { return nil }}, func() tosu.Settings {
		return tosu.Settings{URL: url, Reconnect: 50 * time.Millisecond}
	})
	a.Tosu.OnGame = a.OnGame
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go a.Tosu.Run(ctx)
	wait := func(cond func() bool, what string) {
		t.Helper()
		for i := 0; i < 300; i++ {
			if cond() {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatalf("timed out waiting for %s", what)
	}
	wait(func() bool { g, ok := a.tosuMap(); return ok && g.SetID == 292301 }, "tosu map")

	// osu! plays something nobody requested: tosu wins, no requester
	f := np("u2")
	if f.Code != "np" || f.SetID != 292301 || f.Title != "SWITCHED-ON LOTUS" || f.Artist != "Susumu Hirasawa" ||
		f.Version != "Nattahnam" || f.Mapper != "Dr Manhattan" || f.Stars != 7.64 || f.Mods != "HD" || f.Target != "" {
		t.Errorf("from tosu: %+v", f)
	}

	// the open map is the request: its requester is named
	fake.put(strings.Replace(lotusPayload, `"set":292301`, `"set":39804`, 1))
	wait(func() bool { g, _ := a.tosuMap(); return g.SetID == 39804 }, "tosu on the request")
	if f := np("u3"); f.SetID != 39804 || f.Target != "fan" || f.Title != "SWITCHED-ON LOTUS" {
		t.Errorf("tosu on the requested map: %+v", f)
	}
}
