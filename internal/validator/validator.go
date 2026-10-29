// Package validator decides whether a request may enter the queue.
package validator

import (
	"fmt"
	"strings"
	"time"

	"github.com/draft/osureq/internal/config"
	"github.com/draft/osureq/internal/osumeta"
)

// Rejection carries a stable code (translated by the dashboard and overlay),
// an English detail for the log, and Short: the numbers behind it without
// words (e.g. "5.88★ > 5.00★"), so viewers can be told in any language.
type Rejection struct {
	Code   string `json:"code"`
	Detail string `json:"detail"`
	Short  string `json:"short,omitempty"`
}

func (r *Rejection) Error() string { return r.Code + ": " + r.Detail }

func reject(code, f string, a ...any) *Rejection {
	return &Rejection{Code: code, Detail: fmt.Sprintf(f, a...)}
}

func (r *Rejection) with(short string, a ...any) *Rejection {
	r.Short = fmt.Sprintf(short, a...)
	return r
}

// Roles is what TikTok says about the viewer. Each value is 1 (yes),
// -1 (no) or 0 (unknown, e.g. TikFinity did not send it); unknown never blocks.
type Roles struct {
	Follower   int8
	Subscriber int8
}

// State is a snapshot of the queue used for the checks.
type State struct {
	Roles       Roles
	QueueLen    int
	UserActive  int               // requests by this user still waiting (not the one playing)
	LastRequest time.Time         // last accepted request by this user
	QueuedSets  map[int]bool      // sets waiting or playing
	PlayedAt    map[int]time.Time // set id -> last time it was played
}

// CheckUser runs the checks that need no beatmap metadata, so a spammer
// costs no osu! requests.
func CheckUser(user string, st State, rules config.Rules, q config.Queue, now time.Time) *Rejection {
	if listed(rules.BlacklistUsers, user) {
		return reject("user_blacklisted", "@%s is blacklisted", user)
	}
	if q.MaxSize > 0 && st.QueueLen >= q.MaxSize {
		return reject("queue_full", "queue is full (%d)", q.MaxSize).with("%d/%d", st.QueueLen, q.MaxSize)
	}
	if listed(rules.ExemptUsers, user) {
		return nil
	}
	if rules.FollowersOnly && st.Roles.Follower < 0 {
		return reject("followers_only", "@%s does not follow the stream", user)
	}
	limit := q.PerUserLimit
	if st.Roles.Subscriber > 0 && rules.SubscriberLimit > 0 && (limit == 0 || rules.SubscriberLimit > limit) {
		limit = rules.SubscriberLimit
	}
	if limit > 0 && st.UserActive >= limit {
		return reject("user_limit", "@%s already has %d request(s) waiting", user, st.UserActive).with("%d/%d", st.UserActive, limit)
	}
	if q.CooldownSec > 0 && !st.LastRequest.IsZero() {
		if left := time.Duration(q.CooldownSec)*time.Second - now.Sub(st.LastRequest); left > 0 {
			return reject("cooldown", "@%s must wait %ds", user, int(left.Seconds()+0.999)).with("%ds", int(left.Seconds()+0.999))
		}
	}
	return nil
}

// listed reports whether a TikTok user (@ optional) is in the list.
func listed(list []string, user string) bool {
	for _, b := range list {
		if strings.EqualFold(strings.TrimPrefix(strings.TrimSpace(b), "@"), user) {
			return true
		}
	}
	return false
}

// Input is a resolved request.
type Input struct {
	Set       *osumeta.Set
	BeatmapID int    // 0 = whole set requested
	Mode      string // mode from a #mode/id link, may be empty
}

// CheckMap validates the beatmap and returns the difficulty that will be shown
// as "requested". For set requests it picks the hardest diff that passes.
func CheckMap(in Input, st State, rules config.Rules, q config.Queue, now time.Time) (*osumeta.Diff, *Rejection) {
	s := in.Set
	for _, id := range rules.BlacklistSets {
		if id == s.ID {
			return nil, reject("set_blacklisted", "set %d is blacklisted", s.ID)
		}
	}
	for _, m := range rules.BlacklistMapper {
		if m = strings.TrimSpace(m); m != "" && strings.EqualFold(m, s.Creator) {
			return nil, reject("mapper_blacklisted", "maps by %s are blacklisted", s.Creator).with("%s", s.Creator)
		}
	}
	if !s.Partial && len(rules.Statuses) > 0 && !contains(rules.Statuses, s.Status) {
		return nil, reject("status", "status %q is not allowed", s.Status).with("%s", s.Status)
	}
	if st.QueuedSets[s.ID] {
		return nil, reject("duplicate", "%s - %s is already in the queue", s.Artist, s.Title)
	}
	if q.NoRepeatMin > 0 {
		if at, ok := st.PlayedAt[s.ID]; ok && now.Sub(at) < time.Duration(q.NoRepeatMin)*time.Minute {
			return nil, reject("recently_played", "played %d min ago", int(now.Sub(at).Minutes())).with("%d min", int(now.Sub(at).Minutes()))
		}
	}

	if in.BeatmapID != 0 {
		d := s.Diff(in.BeatmapID)
		if d == nil {
			return nil, reject("not_found", "difficulty %d is not in set %d", in.BeatmapID, s.ID)
		}
		mode := d.Mode
		if in.Mode != "" {
			mode = in.Mode
		}
		if r := checkDiff(d, mode, rules); r != nil {
			return nil, r
		}
		return d, nil
	}

	// Whole set: prefer the set's main mode (the one most of its diffs use),
	// so an osu! set with one guest taiko diff does not pick the taiko one.
	count := map[string]int{}
	for _, d := range s.Diffs {
		count[d.Mode]++
	}
	mainMode := ""
	for m, n := range count {
		if n > count[mainMode] || n == count[mainMode] && m == "osu" {
			mainMode = m
		}
	}
	var best *osumeta.Diff
	var first *Rejection
	for _, onlyMain := range []bool{true, false} {
		for i := range s.Diffs {
			d := &s.Diffs[i]
			if onlyMain && d.Mode != mainMode {
				continue
			}
			if r := checkDiff(d, d.Mode, rules); r != nil {
				if first == nil {
					first = r
				}
				continue
			}
			if best == nil || d.Stars > best.Stars {
				best = d
			}
		}
		if best != nil {
			break
		}
	}
	if best == nil {
		if first == nil {
			first = reject("not_found", "set %d has no difficulties", s.ID)
		}
		first.Detail = "no difficulty fits: " + first.Detail
		return nil, first
	}
	return best, nil
}

func checkDiff(d *osumeta.Diff, mode string, rules config.Rules) *Rejection {
	if len(rules.Modes) > 0 && !contains(rules.Modes, mode) {
		return reject("mode", "mode %s is not allowed", mode).with("%s", mode)
	}
	if d.Stars >= 0 {
		if rules.MinStars > 0 && d.Stars < rules.MinStars {
			return reject("stars_low", "%.2f★ is below %.2f★", d.Stars, rules.MinStars).with("%.2f★ < %.2f★", d.Stars, rules.MinStars)
		}
		if rules.MaxStars > 0 && d.Stars > rules.MaxStars {
			return reject("stars_high", "%.2f★ is above %.2f★", d.Stars, rules.MaxStars).with("%.2f★ > %.2f★", d.Stars, rules.MaxStars)
		}
	}
	if d.Length >= 0 && rules.MaxLengthSec > 0 && d.Length > rules.MaxLengthSec {
		return reject("too_long", "%s is longer than %s", clock(d.Length), clock(rules.MaxLengthSec)).with("%s > %s", clock(d.Length), clock(rules.MaxLengthSec))
	}
	return nil
}

func clock(s int) string { return fmt.Sprintf("%d:%02d", s/60, s%60) }

func contains(list []string, v string) bool {
	for _, x := range list {
		if strings.EqualFold(x, v) {
			return true
		}
	}
	return false
}
