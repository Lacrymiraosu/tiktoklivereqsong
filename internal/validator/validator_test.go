package validator

import (
	"testing"
	"time"

	"github.com/draft/osureq/internal/config"
	"github.com/draft/osureq/internal/osumeta"
)

var now = time.Date(2026, 10, 27, 20, 0, 0, 0, time.UTC)

func set() *osumeta.Set {
	return &osumeta.Set{
		ID: 100, Title: "Song", Artist: "Artist", Creator: "Mapper", Status: "ranked",
		Diffs: []osumeta.Diff{
			{ID: 1, Version: "Easy", Mode: "osu", Stars: 1.8, Length: 120},
			{ID: 2, Version: "Hard", Mode: "osu", Stars: 3.9, Length: 120},
			{ID: 3, Version: "Insane", Mode: "osu", Stars: 5.2, Length: 120},
			{ID: 4, Version: "4K", Mode: "mania", Stars: 2.5, Length: 120},
		},
	}
}

func rules() config.Rules {
	return config.Rules{Modes: []string{"osu", "taiko", "fruits", "mania"}, Statuses: []string{"ranked", "loved"}}
}

func empty() State { return State{QueuedSets: map[int]bool{}, PlayedAt: map[int]time.Time{}} }

func code(r *Rejection) string {
	if r == nil {
		return ""
	}
	return r.Code
}

func TestCheckUser(t *testing.T) {
	q := config.Queue{MaxSize: 3, PerUserLimit: 2, CooldownSec: 60}
	r := rules()
	r.BlacklistUsers = []string{"@Troll"}
	cases := []struct {
		name string
		user string
		st   State
		want string
	}{
		{"ok", "viewer", State{QueueLen: 1}, ""},
		{"blacklisted case-insensitive", "troll", State{}, "user_blacklisted"},
		{"queue full", "viewer", State{QueueLen: 3}, "queue_full"},
		{"per-user limit", "viewer", State{QueueLen: 2, UserActive: 2}, "user_limit"},
		{"cooldown", "viewer", State{LastRequest: now.Add(-30 * time.Second)}, "cooldown"},
		{"cooldown over", "viewer", State{LastRequest: now.Add(-61 * time.Second)}, ""},
	}
	for _, c := range cases {
		if got := code(CheckUser(c.user, c.st, r, q, now)); got != c.want {
			t.Errorf("%s: got %q want %q", c.name, got, c.want)
		}
	}
	// zero limits mean "no limit"
	if r := CheckUser("viewer", State{QueueLen: 999, UserActive: 999, LastRequest: now}, rules(), config.Queue{}, now); r != nil {
		t.Errorf("zero limits should allow everything, got %v", r)
	}
}

func TestCheckMapBeatmap(t *testing.T) {
	q := config.Queue{NoRepeatMin: 30}
	cases := []struct {
		name string
		mod  func(*config.Rules, *State, *osumeta.Set)
		id   int
		mode string
		want string
		diff int
	}{
		{"ok", nil, 2, "", "", 2},
		{"unknown diff", nil, 99, "", "not_found", 0},
		{"stars low", func(r *config.Rules, _ *State, _ *osumeta.Set) { r.MinStars = 2 }, 1, "", "stars_low", 0},
		{"stars high", func(r *config.Rules, _ *State, _ *osumeta.Set) { r.MaxStars = 5 }, 3, "", "stars_high", 0},
		{"stars at max ok", func(r *config.Rules, _ *State, _ *osumeta.Set) { r.MaxStars = 5.2 }, 3, "", "", 3},
		{"too long", func(r *config.Rules, _ *State, _ *osumeta.Set) { r.MaxLengthSec = 90 }, 2, "", "too_long", 0},
		{"mode", func(r *config.Rules, _ *State, _ *osumeta.Set) { r.Modes = []string{"osu"} }, 4, "", "mode", 0},
		{"convert mode from link", func(r *config.Rules, _ *State, _ *osumeta.Set) { r.Modes = []string{"osu"} }, 2, "taiko", "mode", 0},
		{"status", func(_ *config.Rules, _ *State, s *osumeta.Set) { s.Status = "graveyard" }, 2, "", "status", 0},
		{"set blacklist", func(r *config.Rules, _ *State, _ *osumeta.Set) { r.BlacklistSets = []int{100} }, 2, "", "set_blacklisted", 0},
		{"mapper blacklist", func(r *config.Rules, _ *State, _ *osumeta.Set) { r.BlacklistMapper = []string{"mapper"} }, 2, "", "mapper_blacklisted", 0},
		{"duplicate", func(_ *config.Rules, st *State, _ *osumeta.Set) { st.QueuedSets[100] = true }, 2, "", "duplicate", 0},
		{"recently played", func(_ *config.Rules, st *State, _ *osumeta.Set) { st.PlayedAt[100] = now.Add(-10 * time.Minute) }, 2, "", "recently_played", 0},
		{"played long ago", func(_ *config.Rules, st *State, _ *osumeta.Set) { st.PlayedAt[100] = now.Add(-31 * time.Minute) }, 2, "", "", 2},
	}
	for _, c := range cases {
		r, st, s := rules(), empty(), set()
		if c.mod != nil {
			c.mod(&r, &st, s)
		}
		d, rej := CheckMap(Input{Set: s, BeatmapID: c.id, Mode: c.mode}, st, r, q, now)
		if code(rej) != c.want {
			t.Errorf("%s: got %q want %q (%v)", c.name, code(rej), c.want, rej)
			continue
		}
		if c.want == "" && d.ID != c.diff {
			t.Errorf("%s: diff %d want %d", c.name, d.ID, c.diff)
		}
	}
}

func TestCheckMapWholeSetPicksHardestFitting(t *testing.T) {
	r := rules()
	r.MaxStars = 4
	d, rej := CheckMap(Input{Set: set()}, empty(), r, config.Queue{}, now)
	if rej != nil || d.ID != 2 {
		t.Fatalf("want Hard (id 2), got %+v %v", d, rej)
	}
	r.MinStars, r.MaxStars = 6, 7
	if _, rej := CheckMap(Input{Set: set()}, empty(), r, config.Queue{}, now); rej == nil {
		t.Fatal("no diff in 6-7★ should reject")
	}
}

func TestPartialMetadataSkipsUnknownChecks(t *testing.T) {
	s := &osumeta.Set{ID: 7, Partial: true, Diffs: []osumeta.Diff{{ID: 70, Mode: "osu", Stars: -1, Length: -1}}}
	r := rules()
	r.MinStars, r.MaxStars, r.MaxLengthSec = 3, 5, 60
	if _, rej := CheckMap(Input{Set: s, BeatmapID: 70}, empty(), r, config.Queue{}, now); rej != nil {
		t.Errorf("partial metadata should not be rejected on unknown values: %v", rej)
	}
}

func TestExemptUsersSkipLimitsButNotQueueSize(t *testing.T) {
	q := config.Queue{MaxSize: 3, PerUserLimit: 1, CooldownSec: 60}
	r := rules()
	r.ExemptUsers = []string{"@Streamer"}
	busy := State{QueueLen: 2, UserActive: 5, LastRequest: now}
	if rej := CheckUser("streamer", busy, r, q, now); rej != nil {
		t.Errorf("exempt user was rejected: %v", rej)
	}
	if got := code(CheckUser("viewer", busy, r, q, now)); got != "user_limit" {
		t.Errorf("normal user: got %q want user_limit", got)
	}
	if got := code(CheckUser("streamer", State{QueueLen: 3}, r, q, now)); got != "queue_full" {
		t.Errorf("exempt user still respects queue size: got %q", got)
	}
}

func TestShortReasonsAreLanguageNeutral(t *testing.T) {
	r := rules()
	r.MaxStars, r.MaxLengthSec = 5, 90
	_, rej := CheckMap(Input{Set: set(), BeatmapID: 3}, empty(), r, config.Queue{}, now)
	if rej == nil || rej.Short != "5.20★ > 5.00★" {
		t.Errorf("stars_high short = %+v", rej)
	}
	r.MaxStars = 0
	_, rej = CheckMap(Input{Set: set(), BeatmapID: 2}, empty(), r, config.Queue{}, now)
	if rej == nil || rej.Short != "2:00 > 1:30" {
		t.Errorf("too_long short = %+v", rej)
	}
	got := CheckUser("viewer", State{LastRequest: now.Add(-50 * time.Second)}, rules(), config.Queue{CooldownSec: 60}, now)
	if got == nil || got.Short != "10s" {
		t.Errorf("cooldown short = %+v", got)
	}
}

func TestWholeSetFollowsModeOrder(t *testing.T) {
	// Like FREEDOM DiVE: more taiko guest diffs than osu! diffs.
	s := &osumeta.Set{ID: 39804, Status: "approved", Diffs: []osumeta.Diff{
		{ID: 1, Mode: "osu", Stars: 7.8, Length: 258}, {ID: 2, Mode: "osu", Stars: 4.2, Length: 258},
		{ID: 3, Mode: "taiko", Stars: 6.06, Length: 258}, {ID: 4, Mode: "taiko", Stars: 5.1, Length: 258}, {ID: 5, Mode: "taiko", Stars: 3.4, Length: 258},
	}}
	r := rules()
	r.Statuses = append(r.Statuses, "approved")
	r.MaxStars = 8
	if d, rej := CheckMap(Input{Set: s}, empty(), r, config.Queue{}, now); rej != nil || d.ID != 1 {
		t.Fatalf("want the 7.8★ osu! diff, got %+v %v", d, rej)
	}
	r.Modes = []string{"taiko", "osu"} // the setting decides
	if d, rej := CheckMap(Input{Set: s}, empty(), r, config.Queue{}, now); rej != nil || d.ID != 3 {
		t.Fatalf("taiko listed first should pick the taiko diff, got %+v %v", d, rej)
	}
	r.Modes = []string{"osu", "taiko"}
	r.MaxStars = 3 // no osu! diff fits: the reason should be about osu!, not about modes
	if _, rej := CheckMap(Input{Set: s}, empty(), r, config.Queue{}, now); rej == nil || rej.Code != "stars_high" {
		t.Fatalf("want stars_high, got %v", rej)
	}
}

func TestNowPlayingIsNotADuplicate(t *testing.T) {
	st := empty()
	st.QueuedSets[100], st.PlayingSet = true, 100
	if _, rej := CheckMap(Input{Set: set(), BeatmapID: 2}, st, rules(), config.Queue{}, now); rej == nil || rej.Code != "now_playing" {
		t.Fatalf("want now_playing, got %v", rej)
	}
}

func TestWholeSetPrefersMainMode(t *testing.T) {
	s := set() // 3 osu! diffs (max 5.2★) and one 2.5★ mania diff
	s.Diffs = append(s.Diffs, osumeta.Diff{ID: 5, Version: "Taiko Oni", Mode: "taiko", Stars: 6.0, Length: 120})
	d, rej := CheckMap(Input{Set: s}, empty(), rules(), config.Queue{}, now)
	if rej != nil || d.ID != 3 {
		t.Fatalf("want the hardest osu! diff (3), got %+v %v", d, rej)
	}
	r := rules()
	r.Modes = []string{"taiko"} // main mode not allowed: fall back to the other modes
	if d, rej := CheckMap(Input{Set: s}, empty(), r, config.Queue{}, now); rej != nil || d.ID != 5 {
		t.Fatalf("want the taiko diff when only taiko is allowed, got %+v %v", d, rej)
	}
}

func TestFollowerAndSubscriberRules(t *testing.T) {
	r := rules()
	r.FollowersOnly = true
	r.SubscriberLimit = 4
	q := config.Queue{PerUserLimit: 2}
	cases := []struct {
		name  string
		roles Roles
		busy  int
		want  string
	}{
		{"follower", Roles{Follower: 1}, 0, ""},
		{"not following", Roles{Follower: -1}, 0, "followers_only"},
		{"unknown follow status is let through", Roles{}, 0, ""},
		{"normal limit", Roles{Follower: 1}, 2, "user_limit"},
		{"subscriber gets more", Roles{Follower: 1, Subscriber: 1}, 3, ""},
		{"subscriber limit reached", Roles{Follower: 1, Subscriber: 1}, 4, "user_limit"},
	}
	for _, c := range cases {
		st := State{Roles: c.roles, UserActive: c.busy}
		if got := code(CheckUser("viewer", st, r, q, now)); got != c.want {
			t.Errorf("%s: got %q want %q", c.name, got, c.want)
		}
	}
	r.ExemptUsers = []string{"viewer"}
	if rej := CheckUser("viewer", State{Roles: Roles{Follower: -1}}, r, q, now); rej != nil {
		t.Errorf("exempt users skip the follower rule: %v", rej)
	}
}
