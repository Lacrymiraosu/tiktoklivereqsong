package app

import (
	"testing"

	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/config"
	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/tikfinity"
)

func TestSpeakableText(t *testing.T) {
	cases := map[string]string{
		"hello there": "hello there",
		"สวัสดีครับ 😀🎉": "สวัสดีครับ",
		"555555555555":                        "5555",
		"look https://evil.example/x at this": "look at this",
		"go to www.example.com now":           "go to now",
		"👍👍👍":                                 "",
		"!!!???":                              "",
		"woooooooow":                          "woooow",
		"family 👨‍👩‍👧 emoji":                  "family emoji",
	}
	for in, want := range cases {
		if got := SpeakableText(in, 0); got != want {
			t.Errorf("SpeakableText(%q) = %q, want %q", in, got, want)
		}
	}
	if got := SpeakableText("abcdefghij", 5); got != "abcde…" {
		t.Errorf("cut: %q", got)
	}
}

func TestSpeakChatSkipsCommands(t *testing.T) {
	a := newTestApp(t, func(c *config.Config) {
		c.TTS.Enabled, c.TTS.CooldownSec = true, 0
		c.TTS.BlockedWords = []string{"spoiler"}
		c.Rules.BlacklistUsers = []string{"troll"}
	})
	say := func(user, msg string) { a.speakChat(tikfinity.Chat{User: user, Nick: user, Comment: msg}, false) }
	say("fan", "!req 2096297 HDHR")
	say("fan", "  !np")
	say("fan", "!anything at all")
	say("troll", "hello")
	say("fan", "big SPOILER ahead")
	say("fan", "🎉🎉")
	if n := len(a.Speech()); n != 0 {
		t.Fatalf("commands, blacklisted users, blocked words and emoji-only must not be read: %+v", a.Speech())
	}
	say("fan", "สวัสดีครับ อยากฟังเพลงนี้")
	if s := a.Speech(); len(s) != 1 || s[0].Text != "สวัสดีครับ อยากฟังเพลงนี้" || s[0].Name != "fan" {
		t.Errorf("a normal message should be read: %+v", s)
	}

	c := a.Cfg.Get()
	c.TTS.CooldownSec = 60
	a.Cfg.Set(c)
	say("kid", "one")
	say("kid", "two")
	if s := a.Speech(); s[0].Text != "one" {
		t.Errorf("cooldown: the second message within 60 s is skipped: %+v", s[0])
	}

	c.TTS.Enabled = false
	a.Cfg.Set(c)
	say("other", "hello")
	if s := a.Speech(); s[0].User == "other" {
		t.Error("TTS off reads nothing")
	}
}

func TestSimulatedChatIsReadToo(t *testing.T) {
	a := newTestApp(t, func(c *config.Config) { c.TTS.Enabled = true })
	if _, _, kind := a.SimulateChat("viewer", "hi from the test panel"); kind != SimIgnored {
		t.Fatalf("plain chat is still not a request: %v", kind)
	}
	if s := a.Speech(); len(s) != 1 || !s[0].Test {
		t.Errorf("simulated chat should be read, marked as test: %+v", s)
	}
}

func TestTTSAllowGroups(t *testing.T) {
	a := newTestApp(t, func(c *config.Config) {
		c.TTS.Enabled, c.TTS.CooldownSec = true, 0
		c.TTS.Allow = []string{"subscriber", "gifter"}
		c.Rules.ExemptUsers = []string{"streamer"}
	})
	read := func(ch tikfinity.Chat) bool {
		before := len(a.Speech())
		ch.Nick = ch.User
		a.speakChat(ch, false)
		return len(a.Speech()) > before
	}
	if read(tikfinity.Chat{User: "nobody", Comment: "hi", Subscriber: tikfinity.No}) {
		t.Error("not a subscriber and no gift: not read")
	}
	if !read(tikfinity.Chat{User: "sub", Comment: "hi", Subscriber: tikfinity.Yes}) {
		t.Error("ใจส้ม subscribers are read")
	}
	a.HandleGift(tikfinity.Gift{User: "Giver", Coins: 1})
	if !read(tikfinity.Chat{User: "giver", Comment: "thanks!"}) {
		t.Error("a viewer who sent a gift this stream is read")
	}
	if !read(tikfinity.Chat{User: "streamer", Comment: "hello chat"}) {
		t.Error("no-limit users are always read")
	}
	if read(tikfinity.Chat{User: "unknown", Comment: "hi"}) {
		t.Error("an unknown subscriber flag counts as no")
	}
	if n := a.Notices(); len(n) == 0 || n[0].Key != "ttsSubUnknown" {
		t.Errorf("Activity should say the flag is missing: %+v", n)
	}

	a.NewSession()
	if read(tikfinity.Chat{User: "giver", Comment: "again"}) {
		t.Error("a gift from before the session started does not count")
	}

	c := a.Cfg.Get()
	c.TTS.Allow = []string{"follower", "mod"}
	c.Command.Mods = []string{"modman"}
	a.Cfg.Set(c)
	if !read(tikfinity.Chat{User: "f", Comment: "hi", Follower: tikfinity.Yes}) || !read(tikfinity.Chat{User: "modman", Comment: "hi"}) {
		t.Error("followers and mods are read")
	}
}
