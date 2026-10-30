package app

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/config"
	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/queue"
	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/tikfinity"
)

func TestRenderAnnouncement(t *testing.T) {
	got := RenderAnnouncement("@{user} ขอเพลง {mapper} {artist} {title} {diff} {mods}", map[string]string{
		"user": "test1", "mapper": "Sotarks", "artist": "xi", "title": "Blue Zenith", "diff": "FOUR DIMENSIONS", "mods": ""})
	if got != "test1 ขอเพลง Sotarks xi Blue Zenith FOUR DIMENSIONS" {
		t.Errorf("got %q", got)
	}
	if got := RenderAnnouncement("{nope} hi {user}", map[string]string{"user": "a"}); got != "hi a" {
		t.Errorf("unknown placeholders vanish: %q", got)
	}
}

func spoken(a *App) []string {
	var out []string
	for _, s := range a.Speech() {
		if s.Kind == "say" {
			out = append(out, s.Text)
		}
	}
	return out
}

func TestAnnouncements(t *testing.T) {
	web := &fakeWeb{sets: map[int]string{1: set(1, "Blue Zenith", 4.5), 2: set(2, "Hard Song", 7.25), 3: set(3, "Third", 3)}}
	a := searchApp(t, web, func(c *config.Config) {
		c.TTS.Announce = true
		c.Rules.MaxStars = 6
		c.Queue.CooldownSec, c.Queue.PerUserLimit = 0, 0
		c.TTS.Templates["added"] = "@{user} ขอเพลง {mapper} {artist} {title} {diff} {mods}"
	})
	a.SimulateChat("test1", "!req https://osu.ppy.sh/beatmapsets/1 HDHR")
	a.SimulateChat("test2", "!req https://osu.ppy.sh/beatmapsets/2")
	a.SimulateChat("test3", "!req https://osu.ppy.sh/beatmapsets/1") // duplicate
	got := spoken(a)
	want := []string{"เพลง Blue Zenith อยู่ในคิวแล้ว", "เพลง Hard Song ยากเกินไป 7.25 ดาว", "test1 ขอเพลง m xi Blue Zenith Hard HD HR"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("spoken (newest first):\n got %q\nwant %q", got, want)
	}

	// a code without its own template uses "rejected" with a {reason}
	c := a.Cfg.Get()
	c.Command.Enabled = true
	c.Rules.BlacklistUsers = []string{"troll"}
	a.Cfg.Set(c)
	a.SimulateChat("troll", "!req https://osu.ppy.sh/beatmapsets/1")
	if s := spoken(a)[0]; s != "troll ขอเพลงไม่ได้ ตอนนี้ขอเพลงไม่ได้" {
		t.Errorf("generic rejection: %q", s)
	}
	// "-" silences: cooldown is silent by default
	n := len(spoken(a))
	c.Queue.CooldownSec = 600
	a.Cfg.Set(c)
	a.SimulateChat("fan", "!req https://osu.ppy.sh/beatmapsets/3") // accepted: announced
	if _, rej, _ := a.SimulateChat("fan", "!req https://osu.ppy.sh/beatmapsets/2"); rej == nil || rej.Code != "cooldown" {
		t.Fatalf("setup: want a cooldown, got %+v", rej)
	}
	if len(spoken(a)) != n+1 {
		t.Errorf("cooldown rejections are silent by default: %q", spoken(a))
	}

	// announcements off: nothing new
	c.TTS.Announce = false
	a.Cfg.Set(c)
	n = len(a.Speech())
	a.SimulateChat("x", "!req https://osu.ppy.sh/beatmapsets/2")
	if len(a.Speech()) != n {
		t.Error("announce off must say nothing")
	}
}

func TestAnnounceNowPlayingAndGift(t *testing.T) {
	a := newTestApp(t, func(c *config.Config) { c.TTS.Announce = true; c.Test.DryRun = true })
	a.Queue.Add(&queue.Item{ID: "x", User: "fan", Nick: "Fan", Title: "Blue Zenith", Artist: "xi", AddedAt: time.Now()})
	a.Queue.Update("x", func(it *queue.Item) { it.State = queue.Ready })
	if err := a.Next("played"); err != nil {
		t.Fatal(err)
	}
	if s := spoken(a); len(s) == 0 || s[0] != "ต่อไปเพลง Blue Zenith ที่ Fan ขอ" {
		t.Errorf("now playing: %q", s)
	}
	a.HandleGift(tikfinity.Gift{User: "kid", Nick: "Kid", Name: "Rose", Coins: 1})
	if s := spoken(a)[0]; !strings.Contains(s, "ขอบคุณ Kid") {
		t.Errorf("gift thanks: %q", s)
	}
}
