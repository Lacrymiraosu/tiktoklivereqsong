package validator

import (
	"testing"
	"time"

	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/config"
	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/osumeta"
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

func convertSet() *osumeta.Set {
	s := set()
	s.Converts = []osumeta.Diff{{ID: 3, Version: "Insane", Mode: "mania", Stars: 3.4, Length: 120, Keys: 4, Convert: true}}
	return s
}

func TestConvertUsesItsOwnStars(t *testing.T) {
	r := rules()
	r.MaxStars = 4 // the osu! Insane is 5.2★, its mania convert is 3.4★
	d, rej := CheckMap(Input{Set: convertSet(), BeatmapID: 3, Mode: "mania"}, empty(), r, config.Queue{}, now)
	if rej != nil || d.Stars != 3.4 || d.Mode != "mania" || !d.Convert || d.Keys != 4 {
		t.Fatalf("mania convert: %+v %v", d, rej)
	}
	// no rating from osu!: stars unknown, so star limits cannot block it
	d, rej = CheckMap(Input{Set: convertSet(), BeatmapID: 3, Mode: "taiko"}, empty(), r, config.Queue{}, now)
	if rej != nil || d.Stars != -1 || d.Mode != "taiko" || !d.Convert {
		t.Fatalf("taiko convert without rating: %+v %v", d, rej)
	}
	// only osu! maps convert
	if _, rej := CheckMap(Input{Set: convertSet(), BeatmapID: 4, Mode: "osu"}, empty(), r, config.Queue{}, now); rej == nil || rej.Code != "no_convert" {
		t.Fatalf("mania map as osu!: want no_convert, got %v", rej)
	}
}

func TestPerModeStarLimits(t *testing.T) {
	r := rules()
	r.MaxStars = 6
	r.ModeStars = map[string]config.StarRange{"mania": {Max: 2}}
	if _, rej := CheckMap(Input{Set: set(), BeatmapID: 3}, empty(), r, config.Queue{}, now); rej != nil {
		t.Errorf("osu! 5.2★ under the global 6★: %v", rej)
	}
	_, rej := CheckMap(Input{Set: set(), BeatmapID: 4}, empty(), r, config.Queue{}, now)
	if rej == nil || rej.Code != "stars_high" || rej.Short != "2.50★ > 2.00★ (mania)" {
		t.Errorf("mania 2.5★ over its own 2★: %+v", rej)
	}
}

func TestSongKey(t *testing.T) {
	same := [][2][2]string{
		{{"xi", "FREEDOM DiVE"}, {"Xi", "Freedom Dive (Cut Ver.)"}},
		{{"Natsukawa", "Kani*Do-Luck!"}, {"natsukawa", "Kani*Do-Luck! (TV Size)"}},
		{{"Camellia", "Ghost"}, {"Camellia", "Ghost [Extended Mix] (Sped Up Ver.)"}},
		{{"Hanatan", "Harumachi Clover"}, {"Hanatan", "Harumachi Clover ～Short ver.～"}},
	}
	for _, c := range same {
		if a, b := SongKey(c[0][0], c[0][1]), SongKey(c[1][0], c[1][1]); a != b {
			t.Errorf("%v and %v should be one song: %q vs %q", c[0], c[1], a, b)
		}
	}
	if SongKey("xi", "FREEDOM DiVE") == SongKey("xi", "Blue Zenith") || SongKey("xi", "Ghost") == SongKey("Camellia", "Ghost") {
		t.Error("different songs must not match")
	}
}

func TestSongBlacklist(t *testing.T) {
	list := []string{"xi - FREEDOM DiVE", "Ghost", "  "}
	cases := []struct {
		artist, title string
		banned        bool
	}{
		{"xi", "FREEDOM DiVE (Cut Ver.)", true},
		{"xi", "Blue Zenith", false},
		{"Camellia", "Ghost (TV Size)", true}, // title-only entry: any artist
		{"Someone", "Ghost", true},
		{"Someone", "Ghostly", false},
		{"other", "FREEDOM DiVE", false}, // artist given in the entry must match
	}
	for _, c := range cases {
		if _, got := SongBanned(list, Song{Artist: c.artist, Title: c.title}); got != c.banned {
			t.Errorf("%s - %s: banned=%v, want %v", c.artist, c.title, got, c.banned)
		}
	}
	rules := config.Default().Rules
	rules.BlacklistSongs = list
	set := &osumeta.Set{ID: 9, Artist: "xi", Title: "FREEDOM DiVE (Long Ver.)", Status: "ranked", Diffs: []osumeta.Diff{{ID: 1, Mode: "osu", Stars: 5}}}
	if _, r := CheckMap(Input{Set: set}, State{}, rules, config.Default().Queue, time.Now()); r == nil || r.Code != "song_blacklisted" {
		t.Errorf("CheckMap: %+v", r)
	}
}

func TestModRules(t *testing.T) {
	set := &osumeta.Set{ID: 1, Artist: "a", Title: "t", Status: "ranked", Diffs: []osumeta.Diff{
		{ID: 10, Mode: "osu", Stars: 4.5, Length: 240, Version: "Insane"},
		{ID: 11, Mode: "osu", Stars: 6.2, Length: 240, Version: "Extra"},
	}}
	rules := config.Default().Rules
	q := config.Default().Queue
	check := func(r config.Rules, beatmap int, m string) (*osumeta.Diff, *Rejection) {
		return CheckMap(Input{Set: set, BeatmapID: beatmap, Mods: m}, State{}, r, q, time.Now())
	}
	if _, rej := check(rules, 11, "HDHR"); rej != nil {
		t.Fatalf("mods allowed by default: %v", rej)
	}

	r := rules
	r.AllowMods = false
	if _, rej := check(r, 11, "HD"); rej == nil || rej.Code != "mods_off" {
		t.Errorf("mods off: %v", rej)
	}
	if _, rej := check(r, 11, ""); rej != nil {
		t.Errorf("no mods is always fine: %v", rej)
	}

	r = rules
	r.BannedMods = []string{"fl"}
	if _, rej := check(r, 11, "HDFL"); rej == nil || rej.Code != "mod_not_allowed" || rej.Short != "FL" {
		t.Errorf("banned FL: %+v", rej)
	}

	r = rules
	r.HardModMaxStars = 5
	if _, rej := check(r, 11, "DT"); rej == nil || rej.Code != "mod_stars" || rej.Short != "6.20★ > 5.00★ +DT" {
		t.Errorf("DT on 6.2★ with a 5★ cap: %+v", rej)
	}
	if _, rej := check(r, 11, "HDEZ"); rej != nil {
		t.Errorf("EZ does not make it harder: %v", rej)
	}
	if d, rej := check(r, 0, "NC"); rej != nil || d.ID != 10 {
		t.Errorf("whole set with NC should pick the diff under the cap: %v %v", d, rej)
	}

	r = rules
	r.MaxLengthSec = 200
	if _, rej := check(r, 10, ""); rej == nil || rej.Code != "too_long" {
		t.Errorf("4:00 is over 3:20: %v", rej)
	}
	if _, rej := check(r, 10, "DT"); rej != nil {
		t.Errorf("4:00 with DT is 2:40: %v", rej)
	}
	if _, rej := check(r, 10, "HT"); rej == nil || rej.Short != "5:20 > 3:20" {
		t.Errorf("HT makes it 5:20: %+v", rej)
	}

	c := config.Default()
	c.Rules.BannedMods = []string{"XX"}
	if c.Validate() == nil {
		t.Error("unknown banned mod should not validate")
	}
}

func TestArtistRejectionCode(t *testing.T) {
	rules := config.Default().Rules
	rules.BlacklistSongs = []string{`artist="xi"`}
	set := &osumeta.Set{ID: 9, Artist: "xi", Title: "FREEDOM DiVE", Status: "ranked", Diffs: []osumeta.Diff{{ID: 1, Mode: "osu", Stars: 5}}}
	if _, r := CheckMap(Input{Set: set}, State{}, rules, config.Default().Queue, time.Now()); r == nil || r.Code != "artist_blacklisted" || r.Short != "xi" {
		t.Errorf("artist rule: %+v", r)
	}
}

func TestSongRulesKeyValue(t *testing.T) {
	list := []string{`Artist=“a_hisa”`, `title="Ghost"`, `artist='xi' title="Blue Zenith"`, `artist=ナナヒラ`}
	cases := []struct {
		s      Song
		banned bool
	}{
		{Song{Artist: "a_hisa", Title: "Anything"}, true},
		{Song{Artist: "A_HISA", Title: "Other (TV Size)"}, true},
		{Song{Artist: "a_hisa feat. someone", Title: "x"}, true}, // whole words in a row
		{Song{Artist: "a_hisashi", Title: "x"}, false},
		{Song{Artist: "Camellia", Title: "Ghost (Extended Mix)"}, true},
		{Song{Artist: "xi", Title: "Blue Zenith (Cut Ver.)"}, true},
		{Song{Artist: "xi", Title: "FREEDOM DiVE"}, false}, // artist alone is not enough for that rule
		{Song{Artist: "Nanahira", ArtistUnicode: "ナナヒラ", Title: "x"}, true},
		{Song{Artist: "xinxin", Title: "Blue Zenith"}, false},
	}
	for _, c := range cases {
		if _, got := SongBanned(list, c.s); got != c.banned {
			t.Errorf("%+v: banned=%v, want %v", c.s, got, c.banned)
		}
	}
	if RuleKey("xi - FREEDOM DiVE") != RuleKey(`artist="Xi" title="Freedom Dive (TV Size)"`) || RuleKey("") != "" {
		t.Error("RuleKey should treat the two forms alike")
	}
}
