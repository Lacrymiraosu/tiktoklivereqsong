package app

import "time"

// FeedEvent is something viewers should see on the overlay, since the helper
// cannot answer in TikTok chat: a request added or rejected, a mod command,
// a gift that bought priority, or the result of a played map.
//
// Text fields are language-neutral (names, numbers); the overlay translates
// Kind and Code itself.
type FeedEvent struct {
	ID      int64     `json:"id"`
	At      time.Time `json:"at"`
	Kind    string    `json:"kind"` // added | rejected | command | gift | result
	User    string    `json:"user"`
	Nick    string    `json:"nick,omitempty"`
	Code    string    `json:"code,omitempty"`  // rejection code, command name, or result outcome
	Short   string    `json:"short,omitempty"` // e.g. "5.88★ > 5.00★", "98.52% S"
	Title   string    `json:"title,omitempty"`
	Artist  string    `json:"artist,omitempty"`
	Version string    `json:"version,omitempty"`
	Stars   float64   `json:"stars,omitempty"`
	Pos     int       `json:"pos,omitempty"` // queue position (1 = next)
	Target  string    `json:"target,omitempty"`
	Test    bool      `json:"test,omitempty"`
	Avatar  string    `json:"avatar,omitempty"` // User's TikTok picture, when TikFinity sent one
}

const maxFeed = 30

func (a *App) emit(e FeedEvent) {
	if e.Avatar == "" && e.Kind != "result" {
		e.Avatar = a.avatar(e.User)
	}
	a.mu.Lock()
	a.feedSeq++
	e.ID = a.feedSeq
	if e.At.IsZero() {
		e.At = time.Now()
	}
	a.feed = append([]FeedEvent{e}, a.feed...)
	if len(a.feed) > maxFeed {
		a.feed = a.feed[:maxFeed]
	}
	a.mu.Unlock()
	a.Changed()
}

// Feed returns recent events, newest first.
func (a *App) Feed() []FeedEvent {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]FeedEvent(nil), a.feed...)
}
