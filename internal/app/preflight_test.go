package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/config"
	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/downloader"
	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/queue"
)

func byID(cs []Check) map[string]Check {
	m := map[string]Check{}
	for _, c := range cs {
		m[c.ID] = c
	}
	return m
}

func TestPreflightLocalChecks(t *testing.T) {
	osu := t.TempDir()
	a := newTestApp(t, func(c *config.Config) {
		c.Osu.Folder = osu
		c.Test.DryRun = true
		c.Hotkeys.Enabled = true
		c.Command.Enabled = false
	})
	a.Queue.Add(&queue.Item{ID: "t", User: "dashboard", Test: true, AddedAt: time.Now()})

	got := byID(a.Preflight(context.Background(), PreflightEnv{HotkeyErrors: map[string]string{"skip": "taken", "next": "taken"}}))
	want := map[string]string{"game": "fail", "hotkeys": "fail", "overlay": "warn", "requests": "warn", "dryrun": "warn", "testdata": "warn"}
	for id, st := range want {
		if got[id].Status != st {
			t.Errorf("%s = %+v, want %s", id, got[id], st)
		}
	}
	if got["hotkeys"].Vars["keys"] != "next, skip" {
		t.Errorf("hotkey names: %+v", got["hotkeys"])
	}

	// fix everything that can be fixed locally
	os.Mkdir(filepath.Join(osu, "Songs"), 0o755)
	c := a.Cfg.Get()
	c.Test.DryRun, c.Command.Enabled = false, true
	a.Cfg.Set(c)
	a.ClearTest()
	got = byID(a.Preflight(context.Background(), PreflightEnv{OverlayClients: 1}))
	for _, id := range []string{"game", "hotkeys", "overlay"} {
		if got[id].Status != "ok" {
			t.Errorf("%s = %+v, want ok", id, got[id])
		}
	}
	for _, id := range []string{"requests", "dryrun", "testdata", "update"} {
		if _, ok := got[id]; ok {
			t.Errorf("%s should not be listed when fine: %+v", id, got[id])
		}
	}

	c.Osu.Game = "lazer"
	a.Cfg.Set(c)
	if g := byID(a.Preflight(context.Background(), PreflightEnv{}))["game"]; g.Status != "ok" || g.Key != "game_lazer" {
		t.Errorf("lazer needs no Songs folder: %+v", g)
	}
}

func TestMirrorChecks(t *testing.T) {
	if cs := mirrorChecks(nil); len(cs) != 1 || cs[0].Status != "fail" || cs[0].Key != "mirror_none" {
		t.Errorf("no mirrors: %+v", cs)
	}
	cs := mirrorChecks([]downloader.ProbeResult{{Source: "catboy", OK: true, Ms: 120}, {Source: "osu.direct", Error: "HTTP 503"}})
	if cs[0].Status != "ok" || cs[1].Status != "warn" || cs[1].Detail != "HTTP 503" {
		t.Errorf("one working mirror is enough, the other only warns: %+v", cs)
	}
	cs = mirrorChecks([]downloader.ProbeResult{{Source: "catboy", Error: "timeout"}})
	if cs[0].Status != "fail" {
		t.Errorf("no working mirror is a failure: %+v", cs)
	}
}
