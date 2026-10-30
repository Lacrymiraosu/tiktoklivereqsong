package app

import (
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/tikfinity"
)

// Speech is a chat message for the dashboard to read aloud (config tts).
type Speech struct {
	ID   int64     `json:"id"`
	At   time.Time `json:"at"`
	User string    `json:"user"`
	Name string    `json:"name"` // display name, cleaned for speaking
	Text string    `json:"text"`
	Test bool      `json:"test,omitempty"`
	Kind string    `json:"kind"` // chat (a viewer's message) | say (an announcement)
}

const maxSpeech = 20

var (
	reLink   = regexp.MustCompile(`(?i)\b(?:https?://|www\.)\S+|\b\S+\.(?:com|net|org|io|gg|me|tv|th|ly|be|sh)(?:/\S*)?\b`)
	reSpaces = regexp.MustCompile(`\s+`)
)

// speakChat queues a chat message for TTS unless it is a command: anything
// starting with "!" is never read, which is the point of this TTS. Links and
// emoji are left out, long runs ("5555555") are shortened, and the settings'
// filters apply (blacklisted users, followers only, cooldown, blocked words).
// It reports whether the message was queued.
func (a *App) speakChat(ch tikfinity.Chat, test bool) bool {
	cfg := a.Cfg.Get()
	t := cfg.TTS
	if !t.Enabled || strings.HasPrefix(strings.TrimSpace(ch.Comment), "!") {
		return false
	}
	if containsFold(cfg.Rules.BlacklistUsers, ch.User) || !a.ttsAllowed(ch, t.Allow, cfg.Rules.ExemptUsers) {
		return false
	}
	lower := strings.ToLower(ch.Comment)
	for _, w := range t.BlockedWords {
		if w = strings.ToLower(strings.TrimSpace(w)); w != "" && strings.Contains(lower, w) {
			return false
		}
	}
	text := SpeakableText(ch.Comment, t.MaxLength)
	if text == "" {
		return false
	}
	now := time.Now()
	a.mu.Lock()
	defer a.mu.Unlock()
	key := strings.ToLower(ch.User)
	if a.ttsAt == nil || len(a.ttsAt) > 500 {
		a.ttsAt = map[string]time.Time{}
	}
	if now.Sub(a.ttsAt[key]) < time.Duration(t.CooldownSec)*time.Second {
		return false
	}
	a.ttsAt[key] = now
	a.addSpeechLocked(Speech{User: ch.User, Name: speakName(ch.Nick, ch.User), Text: text, Test: test, Kind: "chat"})
	return true
}

// queueSpeech adds something for the dashboard to say.
func (a *App) queueSpeech(s Speech) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.addSpeechLocked(s)
}

func (a *App) addSpeechLocked(s Speech) {
	a.speechSeq++
	s.ID, s.At = a.speechSeq, time.Now()
	a.speech = append([]Speech{s}, a.speech...)
	if len(a.speech) > maxSpeech {
		a.speech = a.speech[:maxSpeech]
	}
	go a.Changed()
}

// ttsAllowed reports whether a viewer is in one of the groups whose chat is
// read. TikTok's subscriber and follower flags come through TikFinity and
// may be missing; missing counts as no (or the filter would let everyone
// through), and Activity says so once.
func (a *App) ttsAllowed(ch tikfinity.Chat, allow, exempt []string) bool {
	if len(allow) == 0 || containsFold(exempt, ch.User) {
		return true
	}
	for _, g := range allow {
		switch g {
		case "subscriber":
			if ch.Subscriber == tikfinity.Unknown {
				a.warnOnce("ttsSubUnknown", nil, "TTS: TikFinity did not say whether viewers are subscribers, so only other groups are read")
			}
			if ch.Subscriber == tikfinity.Yes {
				return true
			}
		case "follower":
			if ch.Follower == tikfinity.Unknown {
				a.warnOnce("ttsFollowUnknown", nil, "TTS: TikFinity did not say whether viewers follow, so only other groups are read")
			}
			if ch.Follower == tikfinity.Yes {
				return true
			}
		case "gifter":
			if a.gaveGift(ch.User) {
				return true
			}
		case "mod":
			if a.IsMod(ch) {
				return true
			}
		}
	}
	return false
}

// gaveGift reports whether user sent a gift during this stream session.
func (a *App) gaveGift(user string) bool {
	start := a.Queue.Summary().Start
	a.mu.Lock()
	defer a.mu.Unlock()
	at, ok := a.gifters[strings.ToLower(user)]
	return ok && !at.Before(start)
}

// Speech returns the recent messages to read, newest first.
func (a *App) Speech() []Speech {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]Speech(nil), a.speech...)
}

// SpeakableText cleans a chat message for a TTS voice: no links, no emoji,
// no runs longer than four of the same character, at most max letters.
func SpeakableText(s string, max int) string {
	s = reLink.ReplaceAllString(s, " ")
	var b strings.Builder
	var last rune
	run := 0
	for _, r := range s {
		if unicode.Is(unicode.So, r) || unicode.Is(unicode.Sk, r) || unicode.Is(unicode.Cs, r) || unicode.Is(unicode.Co, r) ||
			r == '‍' || (r >= '︀' && r <= '️') || (r >= 0x1f3fb && r <= 0x1f3ff) {
			continue // emoji, their joiners, skin tones and variation selectors
		}
		if r == last {
			if run++; run >= 4 {
				continue
			}
		} else {
			last, run = r, 0
		}
		b.WriteRune(r)
	}
	out := strings.TrimSpace(reSpaces.ReplaceAllString(b.String(), " "))
	if max > 0 {
		if r := []rune(out); len(r) > max {
			out = strings.TrimSpace(string(r[:max])) + "…"
		}
	}
	if strings.IndexFunc(out, func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }) < 0 {
		return "" // only punctuation left
	}
	return out
}
