package app

import (
	"fmt"
	"testing"
	"time"

	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/config"
	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/tikfinity"
)

func TestAutoCloseAndReopen(t *testing.T) {
	a := newTestApp(t, func(c *config.Config) { c.Queue.AutoCloseAt, c.Queue.AutoReopenAt = 3, 1 })
	now := time.Now()
	on := func() bool { return a.Cfg.Get().Command.Enabled }
	for i := 0; i < 3; i++ {
		a.autoRequests(now)
		add(a, fmt.Sprint(i), "u", i+1, now)
	}
	if !on() {
		t.Fatal("still below the limit before this check")
	}
	a.autoRequests(now)
	if on() {
		t.Fatal("3 waiting should close requests")
	}
	if f := a.Feed()[0]; f.Kind != "status" || f.Code != "auto_off" || f.Short != "3/3" {
		t.Errorf("overlay event: %+v", f)
	}
	a.Remove("0")
	a.autoRequests(now)
	if on() {
		t.Fatal("2 waiting is above the reopen size 1")
	}
	a.Remove("1")
	a.autoRequests(now)
	if !on() || a.Feed()[0].Code != "auto_on" {
		t.Fatal("1 waiting should reopen")
	}

	// the streamer closes by hand: never reopened automatically
	add(a, "x", "u", 9, now)
	a.autoRequests(now)
	a.ToggleRequests()
	a.Remove("x")
	a.autoRequests(now)
	if on() {
		t.Error("requests closed by hand must stay closed")
	}
	// opened by hand while full: respected until the queue crosses the limit again
	add(a, "w", "u", 10, now)
	add(a, "y", "u", 11, now)
	add(a, "z", "u", 12, now)
	a.autoRequests(now)
	if on() {
		t.Fatal("setup: should still be closed by hand")
	}
	a.ToggleRequests()
	a.autoRequests(now)
	if !on() || a.Queue.Len() < 3 {
		t.Error("opening by hand while full is the streamer's choice")
	}
}

func TestReqoffAfterAutoCloseStaysClosed(t *testing.T) {
	a := newTestApp(t, func(c *config.Config) {
		c.Queue.AutoCloseAt, c.Queue.AutoReopenAt = 2, 0
		c.Command.Mods = []string{"mod"}
	})
	now := time.Now()
	a.autoRequests(now)
	add(a, "1", "u", 1, now)
	add(a, "2", "u", 2, now)
	a.autoRequests(now)
	if a.Cfg.Get().Command.Enabled {
		t.Fatal("setup: should be closed by auto-close")
	}
	a.command(tikfinity.Chat{User: "mod", Comment: "!reqoff"}, false) // "stop for tonight"
	a.Remove("1")
	a.Remove("2")
	a.autoRequests(now)
	if a.Cfg.Get().Command.Enabled {
		t.Error("!reqoff after an auto-close must keep requests closed")
	}
}

func TestCloseAtTime(t *testing.T) {
	a := newTestApp(t, func(c *config.Config) { c.Queue.CloseAt = "23:30" })
	day := time.Date(2026, 10, 30, 0, 0, 0, 0, time.Local)
	a.autoRequests(day.Add(23*time.Hour + 29*time.Minute))
	a.autoRequests(day.Add(23*time.Hour + 29*time.Minute + 30*time.Second))
	if !a.Cfg.Get().Command.Enabled {
		t.Fatal("too early")
	}
	a.autoRequests(day.Add(23*time.Hour + 30*time.Minute + 1*time.Second))
	if a.Cfg.Get().Command.Enabled || a.Feed()[0].Code != "time_off" {
		t.Fatal("passing 23:30 should close requests")
	}
	a.ToggleRequests()
	a.autoRequests(day.Add(23*time.Hour + 45*time.Minute))
	if !a.Cfg.Get().Command.Enabled {
		t.Error("reopened by hand after the time: it fires once")
	}

	b := newTestApp(t, func(c *config.Config) { c.Queue.CloseAt = "20:00" })
	b.autoRequests(day.Add(21 * time.Hour)) // started after the time
	b.autoRequests(day.Add(21*time.Hour + time.Minute))
	if !b.Cfg.Get().Command.Enabled {
		t.Error("starting osureq after the closing time must not close at once")
	}
}

func TestAutoCloseValidation(t *testing.T) {
	c := config.Default()
	c.Queue.AutoCloseAt, c.Queue.AutoReopenAt = 5, 5
	if c.Validate() == nil {
		t.Error("reopen must be below close")
	}
	c.Queue.AutoReopenAt = 2
	for _, bad := range []string{"7:30", "24:00", "12:60", "ab:cd"} {
		c.Queue.CloseAt = bad
		if c.Validate() == nil {
			t.Errorf("close time %q should be invalid", bad)
		}
	}
	c.Queue.CloseAt = "07:30"
	if err := c.Validate(); err != nil {
		t.Error(err)
	}
}
