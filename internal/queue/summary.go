package queue

import (
	"maps"
	"sort"
	"strings"
	"time"
)

// sessionGap is how long the helper must have been idle before a start
// counts as a new stream session.
const sessionGap = 6 * time.Hour

// Summary is how the current stream session went, for the end of a stream.
type Summary struct {
	Start    time.Time   `json:"start"`
	Played   int         `json:"played"`
	Skipped  int         `json:"skipped"`
	Passed   int         `json:"passed"`
	Rejected int         `json:"rejected"`
	Viewers  int         `json:"viewers"` // different viewers with a played or skipped request
	Best     *Played     `json:"best,omitempty"`
	Top      []UserCount `json:"top,omitempty"` // most requests, up to 3
}

type UserCount struct {
	User  string `json:"user"`
	Count int    `json:"count"`
}

// maybeNewSession starts a session when there is none, or when nothing has
// happened for sessionGap (yesterday's stream). q.mu need not be held: it
// only runs from Open.
func (q *Queue) maybeNewSession(now time.Time) {
	last := q.d.SessionStart
	for _, h := range q.d.History {
		if !h.Test && h.At.After(last) {
			last = h.At
			break // newest first
		}
	}
	if q.d.Now != nil && q.d.Now.StartedAt.After(last) {
		last = q.d.Now.StartedAt
	}
	if q.d.SessionStart.IsZero() || now.Sub(last) > sessionGap {
		q.d.SessionStart, q.d.SessionRejected, q.d.SessionOverflow = now, 0, nil
	}
}

// NewSession starts counting a new stream session now.
func (q *Queue) NewSession(now time.Time) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.d.SessionStart, q.d.SessionRejected, q.d.SessionOverflow = now, 0, nil
	q.changed()
}

// sessionTotals also stores the contribution of plays that age out of History.
type sessionTotals struct {
	Played  int                  `json:"played"`
	Skipped int                  `json:"skipped"`
	Passed  int                  `json:"passed"`
	Best    *Played              `json:"best,omitempty"`
	Users   map[string]UserCount `json:"users,omitempty"`
}

func (t *sessionTotals) count(h Played) {
	if t.Users == nil {
		t.Users = make(map[string]UserCount)
	}
	key := strings.ToLower(h.User)
	user := t.Users[key]
	user.User = h.User
	user.Count++
	t.Users[key] = user
	if h.Result == "skipped" {
		t.Skipped++
		return
	}
	t.Played++
	if h.Play != nil && h.Play.Outcome == "passed" {
		t.Passed++
		if t.Best == nil || better(h, *t.Best) {
			t.Best = &h
		}
	}
}

// Summary covers the real (not test) requests finished since the session
// started. The map being played counts once it is finished.
func (q *Queue) Summary() Summary {
	q.mu.Lock()
	defer q.mu.Unlock()
	var totals sessionTotals
	if archived := q.d.SessionOverflow; archived != nil {
		totals = *archived
		totals.Users = maps.Clone(archived.Users)
	}
	// Oldest first so the latest spelling of each viewer's name is displayed.
	for i := len(q.d.History) - 1; i >= 0; i-- {
		h := q.d.History[i]
		if !h.Test && !h.At.Before(q.d.SessionStart) {
			totals.count(h)
		}
	}
	s := Summary{Start: q.d.SessionStart, Rejected: q.d.SessionRejected, Played: totals.Played,
		Skipped: totals.Skipped, Passed: totals.Passed, Best: totals.Best, Viewers: len(totals.Users)}
	for _, c := range totals.Users {
		s.Top = append(s.Top, c)
	}
	sort.Slice(s.Top, func(i, j int) bool {
		if s.Top[i].Count != s.Top[j].Count {
			return s.Top[i].Count > s.Top[j].Count
		}
		return strings.ToLower(s.Top[i].User) < strings.ToLower(s.Top[j].User)
	})
	if len(s.Top) > 3 {
		s.Top = s.Top[:3]
	}
	return s
}

// better ranks passed plays by pp, then accuracy (pp is 0 when tosu has none).
func better(a, b Played) bool {
	if a.Play.PP != b.Play.PP {
		return a.Play.PP > b.Play.PP
	}
	if a.Play.Accuracy != b.Play.Accuracy {
		return a.Play.Accuracy > b.Play.Accuracy
	}
	return a.At.After(b.At)
}
