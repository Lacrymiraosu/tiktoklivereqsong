package queue

import (
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
		q.d.SessionStart, q.d.SessionRejected = now, 0
	}
}

// NewSession starts counting a new stream session now.
func (q *Queue) NewSession(now time.Time) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.d.SessionStart, q.d.SessionRejected = now, 0
	q.changed()
}

// Summary covers the real (not test) requests finished since the session
// started. The map being played counts once it is finished.
func (q *Queue) Summary() Summary {
	q.mu.Lock()
	defer q.mu.Unlock()
	s := Summary{Start: q.d.SessionStart, Rejected: q.d.SessionRejected}
	counts := map[string]*UserCount{}
	for i := range q.d.History {
		h := q.d.History[i]
		if h.Test || h.At.Before(s.Start) {
			continue
		}
		key := strings.ToLower(h.User)
		if counts[key] == nil {
			counts[key] = &UserCount{User: h.User}
		}
		counts[key].Count++
		if h.Result == "skipped" {
			s.Skipped++
			continue
		}
		s.Played++
		if h.Play != nil && h.Play.Outcome == "passed" {
			s.Passed++
			if s.Best == nil || better(h.Play, s.Best.Play) {
				best := h
				s.Best = &best
			}
		}
	}
	s.Viewers = len(counts)
	for _, c := range counts {
		s.Top = append(s.Top, *c)
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
func better(a, b *Result) bool {
	if a.PP != b.PP {
		return a.PP > b.PP
	}
	return a.Accuracy > b.Accuracy
}
