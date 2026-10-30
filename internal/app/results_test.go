package app

import (
	"testing"
	"time"

	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/tosu"
)

func playing(set int, s tosu.Score, failed bool) tosu.Game {
	return tosu.Game{State: "play", SetID: set, Play: s, Failed: failed}
}

func TestResultFromResultsScreen(t *testing.T) {
	a := newTestApp(t, nil)
	add(a, "x", "kiki", 77, time.Now())
	a.Queue.Promote("x", "played")

	a.OnGame(tosu.Game{State: "selectPlay", SetID: 77})
	a.OnGame(playing(77, tosu.Score{Accuracy: 91, MaxCombo: 100, Score: 1000}, false))
	a.OnGame(playing(77, tosu.Score{Accuracy: 97.1, MaxCombo: 400, Score: 5000}, false))
	a.OnGame(tosu.Game{State: "resultScreen", SetID: 77, Results: tosu.Score{Accuracy: 98.52, Rank: "S", MaxCombo: 812, Score: 9000000, PP: 301.4, Mods: "HDHR", Misses: 2}})

	now, _ := a.Queue.Now()
	r := now.Result
	if r == nil || r.Outcome != "passed" || r.Rank != "S" || r.MaxCombo != 812 {
		t.Fatalf("result = %+v", r)
	}
	if got := ResultShort(r); got != "98.52% S · 812x · 2 miss · 301pp · HDHR" {
		t.Errorf("short = %q", got)
	}
	f := a.Feed()
	if len(f) == 0 || f[0].Kind != "result" || f[0].User != "kiki" || f[0].Code != "passed" {
		t.Errorf("feed = %+v", f)
	}
	// the result follows the map into history
	a.OnGame(tosu.Game{State: "selectPlay", SetID: 77})
	a.Queue.Finish("played")
	if h := a.Queue.Snapshot().History; len(h) == 0 || h[0].Play == nil || h[0].Play.Accuracy != 98.52 {
		t.Errorf("history = %+v", h)
	}
}

func TestResultFailedAndQuit(t *testing.T) {
	for _, c := range []struct {
		failed bool
		want   string
	}{{true, "failed"}, {false, "quit"}} {
		a := newTestApp(t, nil)
		add(a, "x", "kiki", 77, time.Now())
		a.Queue.Promote("x", "played")
		a.OnGame(playing(77, tosu.Score{Accuracy: 88.2, MaxCombo: 150, Score: 300000}, c.failed))
		a.OnGame(tosu.Game{State: "selectPlay", SetID: 77})
		now, _ := a.Queue.Now()
		if now.Result == nil || now.Result.Outcome != c.want || now.Result.Accuracy != 88.2 {
			t.Errorf("failed=%v: result %+v, want %s", c.failed, now.Result, c.want)
		}
		if got := ResultShort(now.Result); got != "88.20% · 150x" {
			t.Errorf("no rank or pp for a %s play: %q", c.want, got)
		}
	}
}

func TestQuitBeforeScoringLeavesNoResult(t *testing.T) {
	a := newTestApp(t, nil)
	add(a, "x", "kiki", 77, time.Now())
	a.Queue.Promote("x", "played")
	a.OnGame(playing(77, tosu.Score{}, false))
	a.OnGame(tosu.Game{State: "selectPlay", SetID: 77})
	if now, _ := a.Queue.Now(); now.Result != nil || now.PlayPhase != "ended" {
		t.Errorf("an instant quit should end the map without a result: %+v", now)
	}
}

func TestDetectGameFromTosu(t *testing.T) {
	a := newTestApp(t, nil)
	a.OnGame(tosu.Game{Client: "lazer", State: "menu"})
	if g := a.Cfg.Get().Osu.Game; g != "lazer" {
		t.Fatalf("game = %s, want lazer", g)
	}
	if n := a.Notices(); len(n) != 1 || n[0].Key != "gameDetected" {
		t.Errorf("the switch should be noted once: %+v", n)
	}
	a.OnGame(tosu.Game{Client: "lazer", State: "play"})
	a.OnGame(tosu.Game{Client: "", State: "menu"})
	if len(a.Notices()) != 1 || a.Cfg.Get().Osu.Game != "lazer" {
		t.Error("same or unknown client must not switch again")
	}

	c := a.Cfg.Get()
	c.Osu.DetectGame = false
	a.Cfg.Set(c)
	a.OnGame(tosu.Game{Client: "stable"})
	if a.Cfg.Get().Osu.Game != "lazer" {
		t.Error("detection turned off must leave the setting alone")
	}
}
