// Package validator decides whether a request may enter the queue.
package validator

import (
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/config"
	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/mods"
	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/osumeta"
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
	PlayingSet  int               // set being played right now (0 = none)
	PlayedAt    map[int]time.Time // set id -> last time it was played
}

// Exempt reports whether user is on the no-limit list.
func Exempt(rules config.Rules, user string) bool { return listed(rules.ExemptUsers, user) }

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
	Mods      string // requested mods, e.g. "HDHR" (see package mods)
}

// CheckMap validates the beatmap and returns the difficulty that will be shown
// as "requested". For set requests it picks the hardest diff that passes.
func CheckMap(in Input, st State, rules config.Rules, q config.Queue, now time.Time) (*osumeta.Diff, *Rejection) {
	s := in.Set
	if in.Mods != "" {
		if !rules.AllowMods {
			return nil, reject("mods_off", "requests with mods are off").with("+%s", in.Mods)
		}
		for _, b := range rules.BannedMods {
			if m := strings.ToUpper(strings.TrimSpace(b)); m != "" && mods.Has(in.Mods, m) {
				return nil, reject("mod_not_allowed", "%s is not allowed", m).with("%s", m)
			}
		}
	}
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
	if entry, ok := SongBanned(rules.BlacklistSongs, s.Artist, s.Title); ok {
		return nil, reject("song_blacklisted", "%s - %s is blacklisted (%s)", s.Artist, s.Title, entry).with("%s", s.Title)
	}
	if !s.Partial && len(rules.Statuses) > 0 && !contains(rules.Statuses, s.Status) {
		return nil, reject("status", "status %q is not allowed", s.Status).with("%s", s.Status)
	}
	if st.PlayingSet == s.ID {
		return nil, reject("now_playing", "%s - %s is being played right now", s.Artist, s.Title)
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
		if in.Mode != "" && in.Mode != d.Mode {
			// A #taiko/#fruits/#mania link to an osu! map: a convert. Only
			// osu! maps convert; use osu!'s rating for that mode when it has one.
			if d.Mode != "osu" {
				return nil, reject("no_convert", "%s maps cannot be played as %s", d.Mode, in.Mode).with("%s → %s", d.Mode, in.Mode)
			}
			if c := s.Convert(d.ID, in.Mode); c != nil {
				d = c
			} else {
				c := *d
				c.Mode, c.Convert, c.Stars, c.Keys = in.Mode, true, -1, 0
				d = &c
			}
			mode = in.Mode
		}
		if r := checkDiff(d, mode, in.Mods, rules); r != nil {
			return nil, r
		}
		return d, nil
	}

	// Whole set: try the modes in the order of the allowed-modes setting
	// (osu! first by default) and take the hardest fitting diff of the first
	// mode that has one. Counting diffs per mode is not enough: FREEDOM DiVE
	// has more taiko guest diffs than osu! ones.
	order := rules.Modes
	if len(order) == 0 {
		order = []string{"osu", "taiko", "fruits", "mania"}
	}
	var best *osumeta.Diff
	var first *Rejection
	for _, mode := range order {
		for i := range s.Diffs {
			d := &s.Diffs[i]
			if !strings.EqualFold(d.Mode, mode) {
				continue
			}
			if r := checkDiff(d, d.Mode, in.Mods, rules); r != nil {
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
	if first == nil && best == nil {
		for i := range s.Diffs { // only modes that are not allowed: report why
			if r := checkDiff(&s.Diffs[i], s.Diffs[i].Mode, in.Mods, rules); r != nil {
				first = r
				break
			}
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

// reSuffix matches the version tags mappers add to a title: "(TV Size)",
// "[Extended Mix]", "(Cut Ver.)", "～Short ver.～" and the like, at the end.
var reSuffix = regexp.MustCompile(`\s*(\([^()]*\)|\[[^\[\]]*\]|～[^～]*～|-[^-]*(ver\.?|version|size|edit|mix|cut)\s*-)\s*$`)

// SongKey makes artist and title comparable across the sets of one song:
// lower case, letters and digits only, and the title without version tags.
// "xi", "FREEDOM DiVE (Cut Ver.)" and "Xi", "Freedom Dive" give the same key.
func SongKey(artist, title string) string {
	for t := ""; t != title; {
		t, title = title, reSuffix.ReplaceAllString(title, "")
	}
	return squash(artist) + "|" + squash(title)
}

func squash(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// SongBanned reports which blacklist entry matches the song. An entry
// "Artist - Title" needs both to match; a bare "Title" matches any artist.
func SongBanned(list []string, artist, title string) (string, bool) {
	key := SongKey(artist, title)
	_, titleKey, _ := strings.Cut(key, "|")
	for _, e := range list {
		a, t, both := strings.Cut(strings.TrimSpace(e), " - ")
		if !both {
			if k := SongKey("", a); k != "|" && strings.TrimPrefix(k, "|") == titleKey {
				return e, true
			}
			continue
		}
		if k := SongKey(a, t); k != "|" && k == key {
			return e, true
		}
	}
	return "", false
}

func checkDiff(d *osumeta.Diff, mode, modStr string, rules config.Rules) *Rejection {
	if len(rules.Modes) > 0 && !contains(rules.Modes, mode) {
		return reject("mode", "mode %s is not allowed", mode).with("%s", mode)
	}
	if d.Stars >= 0 {
		min, max, perMode := rules.StarLimits(mode)
		tag := ""
		if perMode {
			tag = " (" + modeLabel(mode) + ")"
		}
		if min > 0 && d.Stars < min {
			return reject("stars_low", "%.2f★ is below %.2f★%s", d.Stars, min, tag).with("%.2f★ < %.2f★%s", d.Stars, min, tag)
		}
		if max > 0 && d.Stars > max {
			return reject("stars_high", "%.2f★ is above %.2f★%s", d.Stars, max, tag).with("%.2f★ > %.2f★%s", d.Stars, max, tag)
		}
		if cap := rules.HardModMaxStars; cap > 0 && mods.Harder(modStr) && d.Stars > cap {
			return reject("mod_stars", "%.2f★ is above %.2f★ for +%s", d.Stars, cap, modStr).with("%.2f★ > %.2f★ +%s", d.Stars, cap, modStr)
		}
	}
	// DT/NC play 1.5× as fast and HT 0.75×, so the song is that much shorter or longer.
	length := int(float64(d.Length)/mods.Rate(modStr) + 0.5)
	if d.Length >= 0 && rules.MaxLengthSec > 0 && length > rules.MaxLengthSec {
		return reject("too_long", "%s is longer than %s", clock(length), clock(rules.MaxLengthSec)).with("%s > %s", clock(length), clock(rules.MaxLengthSec))
	}
	return nil
}

func modeLabel(m string) string {
	switch m {
	case "osu":
		return "osu!"
	case "fruits":
		return "catch"
	}
	return m
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
