package app

import (
	"log"
	"strings"

	"github.com/draft/osureq/internal/tikfinity"
)

// IsMod reports whether a chat user may use mod commands: listed in
// command.mods, or flagged as a moderator by TikTok (when trusted and sent).
func (a *App) IsMod(ch tikfinity.Chat) bool {
	c := a.Cfg.Get().Command
	for _, m := range c.Mods {
		if strings.EqualFold(strings.TrimPrefix(strings.TrimSpace(m), "@"), ch.User) {
			return true
		}
	}
	return c.TrustTikTokMods && ch.Moderator == tikfinity.Yes
}

// command handles the chat commands other than requests. It reports whether
// the message was one of them (even when the sender was not allowed), so
// it is never also parsed as a request. test marks results as test data.
func (a *App) command(ch tikfinity.Chat, test bool) bool {
	c := a.Cfg.Get().Command
	f := strings.Fields(ch.Comment)
	if len(f) == 0 {
		return false
	}
	is := func(name string) bool { return name != "" && strings.EqualFold(f[0], name) }
	ev := FeedEvent{Kind: "command", User: ch.User, Nick: ch.Nick, Test: test}

	switch {
	case is(c.RemoveCmd):
		// Anyone may take back their own latest request; mods may name a user.
		owner := ch.User
		if len(f) >= 2 && a.IsMod(ch) {
			owner = strings.TrimPrefix(f[1], "@")
		}
		it, ok := a.Queue.RemoveLatestBy(owner)
		if !ok {
			return true
		}
		a.Cancel(it.ID)
		ev.Code, ev.Title, ev.Artist, ev.Version = "remove", it.Title, it.Artist, it.Version
		if !strings.EqualFold(owner, ch.User) {
			ev.Target = it.User
		}
		log.Printf("chat: @%s removed %s - %s (@%s)", ch.User, it.Artist, it.Title, it.User)

	case is(c.SkipCmd):
		if !a.IsMod(ch) {
			return true
		}
		now, playing := a.Queue.Now()
		if !playing {
			return true
		}
		go func() {
			if err := a.Skip(""); err != nil {
				a.Say("warn", "skipErr", map[string]string{"u": ch.User, "msg": err.Error()}, "!skip by @%s: %v", ch.User, err)
			}
		}()
		ev.Code, ev.Title, ev.Artist, ev.Version = "skip", now.Title, now.Artist, now.Version
		log.Printf("chat: @%s skipped %s - %s", ch.User, now.Artist, now.Title)

	case is(c.OpenCmd), is(c.CloseCmd):
		if !a.IsMod(ch) {
			return true
		}
		on := is(c.OpenCmd)
		cfg := a.Cfg.Get()
		if cfg.Command.Enabled != on {
			cfg.Command.Enabled = on
			a.Cfg.Set(cfg)
		}
		ev.Code = "off"
		if on {
			ev.Code = "on"
		}
		log.Printf("chat: @%s turned requests %s", ch.User, ev.Code)

	default:
		return false
	}
	a.emit(ev)
	a.Say("info", "usedCmd", map[string]string{"u": ch.User, "cmd": f[0]}, "@%s used %s", ch.User, f[0])
	return true
}
