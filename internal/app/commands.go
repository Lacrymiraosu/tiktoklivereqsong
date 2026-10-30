package app

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/tikfinity"
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

// Outcomes of a chat command.
const (
	cmdNone    = ""        // not a command: parse it as a request
	cmdDone    = "done"    // carried out
	cmdNotMod  = "not_mod" // a mod command from someone who is not a mod
	cmdNothing = "nothing" // nothing to do (no request to remove, nothing playing)
)

// command handles the chat commands other than requests and says what
// happened. Anything but cmdNone means the message was a command (even one
// the sender may not use), so it is never also parsed as a request. Real chat
// ignores the refusals silently; the test panel shows them. test marks
// results as test data.
func (a *App) command(ch tikfinity.Chat, test bool) string {
	c := a.Cfg.Get().Command
	f := strings.Fields(ch.Comment)
	if len(f) == 0 {
		return cmdNone
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
			return cmdNothing
		}
		a.Cancel(it.ID)
		ev.Code, ev.Title, ev.Artist, ev.Version = "remove", it.Title, it.Artist, it.Version
		if !strings.EqualFold(owner, ch.User) {
			ev.Target = it.User
		}
		log.Printf("chat: @%s removed %s - %s (@%s)", ch.User, it.Artist, it.Title, it.User)

	case is(c.SkipCmd):
		if !a.IsMod(ch) {
			return cmdNotMod
		}
		now, playing := a.Queue.Now()
		if !playing {
			return cmdNothing
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
			return cmdNotMod
		}
		on := is(c.OpenCmd)
		a.byHand()
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

	case is(c.PosCmd), is(c.NowCmd):
		// Answers only the viewer who asked, so it stays off the Activity log.
		code := "pos"
		if is(c.NowCmd) {
			code = "np"
		}
		if !a.infoAllowed(ch.User, code) {
			return cmdNothing
		}
		a.emit(a.infoEvent(ev, code, ch.User))
		return cmdDone

	case is(c.VoteSkipCmd) && c.VoteSkipVotes > 0:
		return a.voteSkip(ch, ev, c.VoteSkipVotes)

	default:
		return cmdNone
	}
	a.emit(ev)
	a.Say("info", "usedCmd", map[string]string{"u": ch.User, "cmd": f[0]}, "@%s used %s", ch.User, f[0])
	return cmdDone
}

// infoEvery is how often one viewer may ask the same info command; more
// often is ignored, so a few viewers cannot flood the overlay.
const infoEvery = 20 * time.Second

func (a *App) infoAllowed(user, code string) bool {
	key := strings.ToLower(user) + "|" + code
	now := time.Now()
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.infoAt == nil {
		a.infoAt = map[string]time.Time{}
	}
	if now.Sub(a.infoAt[key]) < infoEvery {
		return false
	}
	if len(a.infoAt) > 500 { // a busy chat: forget answers that no longer limit anyone
		for k, at := range a.infoAt {
			if now.Sub(at) >= infoEvery {
				delete(a.infoAt, k)
			}
		}
	}
	a.infoAt[key] = now
	return true
}

// infoEvent fills ev with the answer to !pos ("pos"/"pos_none") or !np
// ("np"/"np_none") for user.
func (a *App) infoEvent(ev FeedEvent, code, user string) FeedEvent {
	if code == "np" {
		now, ok := a.Queue.Now()
		if !ok {
			ev.Code = "np_none"
			return ev
		}
		ev.Code, ev.Title, ev.Artist, ev.Version, ev.Stars, ev.Target = "np", now.Title, now.Artist, now.Version, now.Stars, now.User
		ev.Mapper, ev.Mods, ev.SetID = now.Creator, now.Mods, now.SetID
		return ev
	}
	items, pos := a.Queue.ByUser(user)
	if len(items) == 0 {
		ev.Code = "pos_none"
		return ev
	}
	ev.Code, ev.Pos, ev.Title, ev.Artist, ev.Version, ev.Stars = "pos", pos[0], items[0].Title, items[0].Artist, items[0].Version, items[0].Stars
	ev.Mapper, ev.Mods, ev.SetID = items[0].Creator, items[0].Mods, items[0].SetID
	if len(pos) > 1 {
		nums := make([]string, len(pos))
		for i, p := range pos {
			nums[i] = "#" + strconv.Itoa(p)
		}
		ev.Short = strings.Join(nums, ", ")
	}
	return ev
}

// VoteSkip is the vote on the song being played.
type VoteSkip struct {
	Votes  int `json:"votes"`
	Needed int `json:"needed"` // 0 = vote skip is off
}

// voteSkip counts one vote per viewer on the song being played and skips it
// once enough different viewers voted.
func (a *App) voteSkip(ch tikfinity.Chat, ev FeedEvent, needed int) string {
	now, playing := a.Queue.Now()
	if !playing {
		return cmdNothing
	}
	user := strings.ToLower(ch.User)
	a.mu.Lock()
	if a.voteID != now.ID {
		a.voteID, a.voters, a.votePassed = now.ID, map[string]bool{}, false
	}
	if a.voters[user] || a.votePassed {
		a.mu.Unlock()
		return cmdNothing // already voted, or the vote already passed
	}
	a.voters[user] = true
	votes := len(a.voters)
	a.votePassed = votes >= needed // decided here, so only one vote skips
	a.mu.Unlock()

	ev.Title, ev.Artist, ev.Version = now.Title, now.Artist, now.Version
	ev.Code, ev.Short = "vote", fmt.Sprintf("%d/%d", votes, needed)
	if votes < needed {
		a.emit(ev)
		return cmdDone
	}
	ev.Code = "voteskip"
	a.emit(ev)
	log.Printf("chat: vote skip (%d/%d) on %s - %s", votes, needed, now.Artist, now.Title)
	a.Say("info", "voteSkipped", map[string]string{"title": now.Artist + " - " + now.Title, "n": strconv.Itoa(votes)}, "%d viewers voted to skip %s - %s", votes, now.Artist, now.Title)
	go func() {
		if cur, ok := a.Queue.Now(); !ok || cur.ID != now.ID {
			return // the streamer moved on already: never skip the next song
		}
		if err := a.Skip(""); err != nil {
			a.Say("warn", "skipErr", map[string]string{"u": ch.User, "msg": err.Error()}, "vote skip: %v", err)
		}
	}()
	return cmdDone
}

// Votes returns the skip vote on the song being played.
func (a *App) Votes() VoteSkip {
	needed := a.Cfg.Get().Command.VoteSkipVotes
	now, playing := a.Queue.Now()
	a.mu.Lock()
	defer a.mu.Unlock()
	if !playing || needed == 0 || a.voteID != now.ID {
		return VoteSkip{Needed: needed}
	}
	return VoteSkip{Votes: len(a.voters), Needed: needed}
}
