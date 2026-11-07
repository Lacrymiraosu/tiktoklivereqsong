package app

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/config"
)

// multiApp answers sets 1-5 (easy) and 9 (too hard); bare numbers are set IDs.
func multiApp(t *testing.T, mutate func(*config.Config)) *App {
	web := &fakeWeb{sets: map[int]string{1: set(1, "One", 2), 2: set(2, "Two", 2), 3: set(3, "Three", 2), 4: set(4, "Four", 2),
		5: set(5, "Five", 2), 9: set(9, "Too Hard", 9)}}
	return searchApp(t, web, func(c *config.Config) {
		c.Command.BareNumberAs = "set"
		c.Command.MaxPerMessage = 3
		c.Rules.MaxStars = 5
		c.Queue.CooldownSec, c.Queue.PerUserLimit = 0, 0
		if mutate != nil {
			mutate(c)
		}
	})
}

// outcome lists what happened to each map: "ok <set> <mods>" or the code.
func outcome(r SimResult) string {
	var out []string
	for _, m := range r.Maps {
		if m.Item != nil {
			out = append(out, strings.TrimSpace(fmt.Sprintf("ok %d %s", m.Item.SetID, m.Item.Mods)))
		} else {
			out = append(out, m.Rej.Code)
		}
	}
	return strings.Join(out, ", ")
}

func rejectedCodes(a *App) []string {
	var out []string
	for _, r := range a.Queue.Snapshot().Rejected {
		out = append(out, r.Code)
	}
	return out
}

func TestSeveralMapsInOneMessage(t *testing.T) {
	a := multiApp(t, nil)
	r := a.SimulateMessage("fan", "!req 1 2 HDHR 3")
	if got := outcome(r); r.Kind != SimRequest || r.Asked != 3 || got != "ok 1, ok 2 HDHR, ok 3" {
		t.Fatalf("got %q %+v", got, r)
	}
	if a.Queue.Len() != 3 {
		t.Errorf("want 3 queue items, got %d", a.Queue.Len())
	}
	// SimulateChat answers with the first map
	if it, rej, _ := a.SimulateChat("fan2", "!req 4 5"); rej != nil || it == nil || it.SetID != 4 {
		t.Errorf("SimulateChat: %+v %+v", it, rej)
	}

	// map rules apply to each map; one rejected map does not stop the others
	r = a.SimulateMessage("fan3", "!req 9 1 +EZHR")
	if got := outcome(r); got != "stars_high, bad_mods" {
		t.Errorf("got %q", got)
	}
	r = a.SimulateMessage("fan4", "!req 1 3")
	if got := outcome(r); got != "duplicate, duplicate" {
		t.Errorf("maps already queued: %q", got)
	}
}

func TestOneMapPerMessageByDefault(t *testing.T) {
	a := multiApp(t, func(c *config.Config) { c.Command.MaxPerMessage = 1 })
	r := a.SimulateMessage("fan", "!req 1 2 3")
	if got := outcome(r); r.Asked != 1 || got != "ok 1" || a.Queue.Len() != 1 {
		t.Errorf("one map per message: %q, %d in queue", got, a.Queue.Len())
	}
	if c := config.Default(); c.Command.MaxPerMessage != 1 {
		t.Errorf("default maps per message = %d", c.Command.MaxPerMessage)
	}
}

func TestSeveralMapsPerUserLimit(t *testing.T) {
	a := multiApp(t, func(c *config.Config) { c.Queue.PerUserLimit = 2 })
	r := a.SimulateMessage("fan", "!req 1 2 3")
	if got := outcome(r); got != "ok 1, ok 2, user_limit" || a.Queue.Len() != 2 {
		t.Errorf("each map counts toward the limit: %q, %d in queue", got, a.Queue.Len())
	}
	// one waiting already: only one more fits
	a.SimulateMessage("fan2", "!req 4")
	if got := outcome(a.SimulateMessage("fan2", "!req 5 9")); got != "ok 5, user_limit" {
		t.Errorf("got %q", got)
	}
	// a viewer rejection ends the message: the map after it is not tried
	if got := outcome(a.SimulateMessage("fan2", "!req 9 3")); got != "user_limit" {
		t.Errorf("got %q", got)
	}
}

func TestSeveralMapsCooldown(t *testing.T) {
	a := multiApp(t, func(c *config.Config) { c.Queue.CooldownSec = 600 })
	r := a.SimulateMessage("fan", "!req 1 2 3")
	if got := outcome(r); got != "ok 1, ok 2, ok 3" {
		t.Fatalf("the maps of one message must not wait for each other: %q", got)
	}
	n := len(a.Queue.Snapshot().Rejected)
	r = a.SimulateMessage("fan", "!req 4 5")
	if got := outcome(r); got != "cooldown" || a.Queue.Len() != 3 {
		t.Errorf("the next message waits: %q, %d in queue", got, a.Queue.Len())
	}
	if got := len(a.Queue.Snapshot().Rejected) - n; got != 1 || a.Queue.Snapshot().Rejected[0].Input != "!req 4 5" {
		t.Errorf("a message in its cooldown is one rejection for all its maps, got %d: %+v", got, a.Queue.Snapshot().Rejected[0])
	}
	if _, rej, _ := a.SimulateChat("fan", "!req 4"); rej == nil || rej.Code != "cooldown" {
		t.Errorf("a single map inside the cooldown, as before: %+v", rej)
	}
	// a first map that is rejected (not for the viewer) does not start the cooldown
	if got := outcome(a.SimulateMessage("fan2", "!req 9 4")); got != "stars_high, ok 4" {
		t.Errorf("got %q", got)
	}
	if got := outcome(a.SimulateMessage("fan2", "!req 5")); got != "cooldown" {
		t.Errorf("got %q", got)
	}

	// the cooldown runs from the last map added
	st := a.Queue.ValidationState("fan")
	items := a.Queue.First(10)
	if last := items[2].AddedAt; !st.LastRequest.Equal(last) {
		t.Errorf("cooldown starts at %v, want the last map's %v", st.LastRequest, last)
	}
}

func TestSeveralMapsGiftOnly(t *testing.T) {
	a := multiApp(t, func(c *config.Config) { c.Gifts.RequestCoins = 30 })
	if _, n, err := a.SimulateGift("fan", 60); err != nil || n != 2 {
		t.Fatalf("60 coins should pay for 2 requests: %d %v", n, err)
	}
	r := a.SimulateMessage("fan", "!req 1 2 3")
	if got := outcome(r); got != "ok 1, ok 2, needs_gift" || a.Queue.Snapshot().Rejected[0].Input != "!req 3" {
		t.Errorf("2 paid requests, 3 maps: %q %+v", got, a.Queue.Snapshot().Rejected)
	}
	if left, _ := a.Queue.Requests("fan", time.Now()); left != 0 {
		t.Errorf("paid requests left = %d, want 0", left)
	}

	// a rejected map gives its paid request back to the next map
	if _, n, _ := a.SimulateGift("fan2", 60); n != 2 {
		t.Fatal("gift")
	}
	if got := outcome(a.SimulateMessage("fan2", "!req 9 4 5")); got != "stars_high, ok 4, ok 5" {
		t.Errorf("got %q", got)
	}
	if left, _ := a.Queue.Requests("fan2", time.Now()); left != 0 {
		t.Errorf("paid requests left = %d, want 0", left)
	}
}

func TestSeveralMapsPastTheLimit(t *testing.T) {
	a := multiApp(t, func(c *config.Config) { c.Command.MaxPerMessage = 2 })
	r := a.SimulateMessage("fan", "!req 1 1 2 3 4 HD")
	if got := outcome(r); got != "ok 1, ok 2" || r.Asked != 2 || r.Extra != 2 {
		t.Errorf("got %q asked=%d extra=%d", got, r.Asked, r.Extra)
	}
	if got := rejectedCodes(a); len(got) != 1 || got[0] != "too_many_maps" {
		t.Errorf("maps past the limit: one rejection, got %v", got)
	}
	if rr := a.Queue.Snapshot().Rejected[0]; rr.Short != "4 > 2" || !rr.Test {
		t.Errorf("rejection: %+v", rr)
	}
}

func TestSeveralMapsSpeakEachOutcomeOnce(t *testing.T) {
	a := multiApp(t, func(c *config.Config) {
		c.Command.MaxPerMessage = 5
		c.TTS.Announce = true
		c.TTS.Templates["added"] = "{user} added {title}"
		c.TTS.Templates["rejected"] = "{user} rejected {reason}"
		c.TTS.Templates["rejected_stars_high"] = "{title} too hard"
	})
	a.SimulateMessage("fan", "!req 1 2 9 3 4")
	got := spoken(a)
	want := []string{"Too Hard too hard", "fan added One"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("spoken (newest first):\n got %q\nwant %q", got, want)
	}
	if n := len(a.Feed()); n != 5 {
		t.Errorf("the overlay still shows every map: %d events", n)
	}
	// the next message speaks again
	a.SimulateMessage("fan2", "!req 5")
	if s := spoken(a); len(s) != 3 || s[0] != "fan2 added Five" {
		t.Errorf("got %q", s)
	}

	// different outcomes: at most two lines per message, the overlay shows all
	n, events := len(spoken(a)), len(a.Feed())
	r := a.SimulateMessage("fan3", "!req 9 1 +EZHR 2 1 2 3 4")
	if got := outcome(r); got != "stars_high, bad_mods, duplicate, duplicate, duplicate" {
		t.Fatalf("setup: %q", got)
	}
	if got := len(spoken(a)) - n; got != spokenPerMessage {
		t.Errorf("one message said %d lines: %q", got, spoken(a)[:got])
	}
	if got := len(a.Feed()) - events; got != 5 {
		t.Errorf("overlay events: %d, want 5", got)
	}

	// maps past the limit: on the overlay, never spoken
	c := a.Cfg.Get()
	c.Command.MaxPerMessage = 2
	a.Cfg.Set(c)
	n = len(spoken(a))
	a.SimulateMessage("fan4", "!req 9 1 2")
	if f := a.Feed()[0]; f.Kind != "rejected" || f.Code != "too_many_maps" {
		t.Errorf("last event: %+v", f)
	}
	if s := spoken(a); len(s)-n != 2 || strings.Contains(fmt.Sprint(s[:len(s)-n]), "rejected") {
		t.Errorf("spoken: %q", s[:len(s)-n])
	}
}
