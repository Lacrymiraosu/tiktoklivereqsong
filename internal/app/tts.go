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
func (a *App) speakChat(ch tikfinity.Chat, test bool) {
	cfg := a.Cfg.Get()
	t := cfg.TTS
	if !t.Enabled || strings.HasPrefix(strings.TrimSpace(ch.Comment), "!") {
		return
	}
	if containsFold(cfg.Rules.BlacklistUsers, ch.User) || (t.FollowersOnly && ch.Follower == tikfinity.No) {
		return
	}
	lower := strings.ToLower(ch.Comment)
	for _, w := range t.BlockedWords {
		if w = strings.ToLower(strings.TrimSpace(w)); w != "" && strings.Contains(lower, w) {
			return
		}
	}
	text := SpeakableText(ch.Comment, t.MaxLength)
	if text == "" {
		return
	}
	now := time.Now()
	a.mu.Lock()
	defer a.mu.Unlock()
	key := strings.ToLower(ch.User)
	if a.ttsAt == nil || len(a.ttsAt) > 500 {
		a.ttsAt = map[string]time.Time{}
	}
	if now.Sub(a.ttsAt[key]) < time.Duration(t.CooldownSec)*time.Second {
		return
	}
	a.ttsAt[key] = now
	a.addSpeechLocked(Speech{User: ch.User, Name: speakName(ch.Nick, ch.User), Text: text, Test: test, Kind: "chat"})
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
