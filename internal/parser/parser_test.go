package parser

import (
	"fmt"
	"strings"
	"testing"
)

var opts = Options{Command: "!req", Aliases: []string{"!r", "!request"}, BareNumberAs: "beatmap"}

func TestParse(t *testing.T) {
	cases := []struct {
		in   string
		ok   bool
		want Request
	}{
		{"!req 2096297", true, Request{Kind: KindBeatmap, BeatmapID: 2096297, Bare: true}},
		{"!REQ 2096297", true, Request{Kind: KindBeatmap, BeatmapID: 2096297, Bare: true}},
		{"  !r   123  please", true, Request{Kind: KindBeatmap, BeatmapID: 123, Bare: true}},
		{"!request 123", true, Request{Kind: KindBeatmap, BeatmapID: 123, Bare: true}},
		{"!req https://osu.ppy.sh/beatmapsets/962870#mania/2096297", true, Request{Kind: KindBeatmap, SetID: 962870, BeatmapID: 2096297, Mode: "mania"}},
		{"!req osu.ppy.sh/beatmapsets/962870#osu/2096297", true, Request{Kind: KindBeatmap, SetID: 962870, BeatmapID: 2096297, Mode: "osu"}},
		{"!req https://osu.ppy.sh/beatmapsets/962870", true, Request{Kind: KindSet, SetID: 962870}},
		{"!req https://osu.ppy.sh/beatmapsets/962870/", true, Request{Kind: KindSet, SetID: 962870}},
		{"!req https://osu.ppy.sh/beatmapsets/962870#taiko", true, Request{Kind: KindSet, SetID: 962870}},
		{"!req https://osu.ppy.sh/b/2096297", true, Request{Kind: KindBeatmap, BeatmapID: 2096297}},
		{"!req https://osu.ppy.sh/b/2096297?m=0", true, Request{Kind: KindBeatmap, BeatmapID: 2096297}},
		{"!req https://osu.ppy.sh/beatmaps/2096297", true, Request{Kind: KindBeatmap, BeatmapID: 2096297}},
		{"!req https://osu.ppy.sh/s/962870", true, Request{Kind: KindSet, SetID: 962870}},
		{"!req http://old.ppy.sh/s/962870", true, Request{Kind: KindSet, SetID: 962870}},
		{"!req <https://osu.ppy.sh/s/962870>", true, Request{Kind: KindSet, SetID: 962870}},

		// rejected
		{"!req", false, Request{}},
		{"req 123", false, Request{}},
		{"hello !req 123", false, Request{}},
		{"!reqq 123", false, Request{}},
		{"!req abc", false, Request{}},
		{"!req 0", false, Request{}},
		{"!req -5", false, Request{}},
		{"!req 12.5", false, Request{}},
		{"!req 99999999999", false, Request{}},
		{"!req https://evil.example/beatmapsets/1", false, Request{}},
		{"!req https://osu.ppy.sh.evil.example/s/1", false, Request{}},
		{"!req https://osu.ppy.sh/users/2", false, Request{}},
		{"!req https://osu.ppy.sh/beatmapsets/abc", false, Request{}},
		{"!req https://osu.ppy.sh/beatmapsets/1#osu/abc", false, Request{}},
		{"!req https://osu.ppy.sh/beatmapsets/1#foo/2", false, Request{}},
		{"!req https://osu.ppy.sh/beatmapsets/1/../../users/2", false, Request{}},
		{"!req javascript:alert(1)", false, Request{}},
		{"!req １２３", false, Request{}}, // full-width digits
	}
	for _, c := range cases {
		got, ok := Parse(c.in, opts)
		if ok != c.ok || got != c.want {
			t.Errorf("Parse(%q) = %+v, %v; want %+v, %v", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestBareNumberAsSet(t *testing.T) {
	o := opts
	o.BareNumberAs = "set"
	got, ok := Parse("!req 962870", o)
	if !ok || got != (Request{Kind: KindSet, SetID: 962870, Bare: true}) {
		t.Errorf("got %+v %v", got, ok)
	}
	// links keep their own meaning
	got, _ = Parse("!req https://osu.ppy.sh/b/5", o)
	if got.Kind != KindBeatmap || got.BeatmapID != 5 {
		t.Errorf("link should stay a beatmap: %+v", got)
	}
}

func TestFlip(t *testing.T) {
	r, _ := Parse("!req 2618766", opts)
	if f := r.Flip(); f != (Request{Kind: KindSet, SetID: 2618766, Bare: true}) {
		t.Errorf("flip beatmap -> set: %+v", f)
	}
	if f := r.Flip().Flip(); f != r {
		t.Errorf("double flip: %+v", f)
	}
}

func TestCustomCommand(t *testing.T) {
	o := Options{Command: "!osu", BareNumberAs: "beatmap"}
	if _, ok := Parse("!req 1", o); ok {
		t.Error("old command must not match")
	}
	if _, ok := Parse("!osu 1", o); !ok {
		t.Error("custom command should match")
	}
}

func TestSearchQueries(t *testing.T) {
	o := Options{Command: "!req", BareNumberAs: "beatmap", Search: true}
	cases := []struct {
		in    string
		kind  Kind
		query string
	}{
		{"!req freedom dive", KindSearch, "freedom dive"},
		{"!req   Blue  Zenith  ", KindSearch, "Blue Zenith"},
		{"!req u's Snow Halation", KindSearch, "u's Snow Halation"},
		{"!req รักแรกพบ", KindSearch, "รักแรกพบ"},
		{"!req 2096297 please", KindBeatmap, ""}, // an ID first still wins
		{"!req osu.ppy.sh/b/123 thanks", KindBeatmap, ""},
	}
	for _, c := range cases {
		r, ok := Parse(c.in, o)
		if !ok || r.Kind != c.kind || r.Query != c.query {
			t.Errorf("%q: got %+v ok=%v, want %v %q", c.in, r, ok, c.kind, c.query)
		}
	}
	for _, in := range []string{
		"!req ab",                         // too short
		"!req https://youtube.com/watch",  // other site
		"!req youtube.com/watch?v=x",      // other site without scheme
		"!req www.example.com",            // a web address
		"!req osu.ppy.sh/users/2 my page", // osu! link that is not a map
		"!req",
	} {
		if r, ok := Parse(in, o); ok {
			t.Errorf("%q should not be a request: %+v", in, r)
		}
	}
	if _, ok := Parse("!req freedom dive", Options{Command: "!req"}); ok {
		t.Error("with search off, names are ignored as before")
	}
	long := "!req " + strings.Repeat("a", 200)
	if r, _ := Parse(long, o); len(r.Query) != MaxQuery {
		t.Errorf("long query should be cut to %d, got %d", MaxQuery, len(r.Query))
	}
}

func TestMods(t *testing.T) {
	o := Options{Command: "!req", BareNumberAs: "beatmap", Search: true}
	cases := []struct {
		in, mods, err, query string
	}{
		{"!req 2096297 HDHR", "HDHR", "", ""},
		{"!req 2096297 +hdhr", "HDHR", "", ""},
		{"!req 2096297 HD DT please", "HDDT", "", ""},
		{"!req 2096297 hd dt", "HDDT", "", ""}, // lower case: only when nothing else follows
		{"!req 2096297 hd dt please", "", "", ""},
		{"!req 2096297 so good", "", "", ""}, // "so" is chat here, not SpunOut
		{"!req 2096297 OK", "", "", ""},
		{"!req 2096297 GG thanks", "", "", ""},
		{"!req 2096297 +HD pls", "HD", "", ""},
		{"!req 2096297 HDDT pls", "HDDT", "", ""},
		{"!req freedom dive OK", "", "", "freedom dive OK"},
		{"!req https://osu.ppy.sh/b/1 DTNC", "NC", "", ""},
		{"!req 2096297 please", "", "", ""}, // not mods: ignored as before
		{"!req 2096297 +HDXX", "", "unknown", ""},
		{"!req 2096297 EZHR", "", "conflict", ""},
		{"!req freedom dive +HDHR", "HDHR", "", "freedom dive"},
		{"!req freedom dive DT", "DT", "", "freedom dive"},
		{"!req blue zenith hd", "", "", "blue zenith hd"}, // lower-case: part of the name
		{"!req freedom dive +EZHR", "", "conflict", "freedom dive"},
	}
	for _, c := range cases {
		r, ok := Parse(c.in, o)
		if !ok {
			t.Errorf("%q: not parsed", c.in)
			continue
		}
		code := ""
		if r.ModsErr != nil {
			code = r.ModsErr.Code
		}
		if r.Mods != c.mods || code != c.err || r.Query != c.query {
			t.Errorf("%q: mods=%q err=%q query=%q, want %q %q %q", c.in, r.Mods, code, r.Query, c.mods, c.err, c.query)
		}
	}
}

func TestParseAll(t *testing.T) {
	o := Options{Command: "!req", BareNumberAs: "beatmap", Search: true, MaxMaps: 3}
	type want struct {
		id   int
		mods string
		err  string
		text string
	}
	read := func(in string, o Options) ([]want, int, bool) {
		parts, extra, ok := ParseAll(in, o)
		var got []want
		for _, p := range parts {
			id := p.BeatmapID
			if p.Kind == KindSet {
				id = -p.SetID
			}
			code := ""
			if p.ModsErr != nil {
				code = p.ModsErr.Code
			}
			got = append(got, want{id, p.Mods, code, p.Text})
		}
		return got, extra, ok
	}
	cases := []struct {
		in    string
		want  []want
		extra int
	}{
		{"!req 123 456 HDHR 789", []want{{123, "", "", "123"}, {456, "HDHR", "", "456 HDHR"}, {789, "", "", "789"}}, 0},
		{"!req 123 +hd dt 456", []want{{123, "HDDT", "", "123 +hd dt"}, {456, "", "", "456"}}, 0},
		{"!req https://osu.ppy.sh/b/5 6 osu.ppy.sh/s/7 NC", []want{{5, "", "", "https://osu.ppy.sh/b/5"}, {6, "", "", "6"}, {-7, "NC", "", "osu.ppy.sh/s/7 NC"}}, 0},
		{"!req 123 456 123 HD", []want{{123, "", "", "123"}, {456, "", "", "456"}}, 0},             // named twice: counts once
		{"!req 5 osu.ppy.sh/b/5 6", []want{{5, "", "", "5"}, {6, "", "", "6"}}, 0},                 // the same map as an ID and a link
		{"!req 1 2 3 4 5 HD", []want{{1, "", "", "1"}, {2, "", "", "2"}, {3, "", "", "3"}}, 2},     // past the limit: left out
		{"!req 1 2 2 3 3 4", []want{{1, "", "", "1"}, {2, "", "", "2"}, {3, "", "", "3"}}, 1},      // repeats do not use up the limit
		{"!req 123 +HDXX 456", []want{{123, "", "unknown", "123 +HDXX"}, {456, "", "", "456"}}, 0}, // only that map has wrong mods
		{"!req 123 EZHR 456 HD", []want{{123, "", "conflict", "123 EZHR"}, {456, "HD", "", "456 HD"}}, 0},
		{"!req 123 HD DT HT 456", []want{{123, "", "conflict", "123 HD DT HT"}, {456, "", "", "456"}}, 0},
		// not every word is a map or mods: one request, read as before
		{"!req 123 456 please", []want{{123, "", "", "123 456 please"}}, 0},
		{"!req 123 HDHR", []want{{123, "HDHR", "", "123 HDHR"}}, 0},
		{"!req 123 123", []want{{123, "", "", "123 123"}}, 0},
		{"!req 123 hdxx 456", []want{{123, "", "", "123 hdxx 456"}}, 0},
		{"!req HDHR 123 456", nil, 0}, // mods first: a song name ("HDHR 123 456") is searched instead
	}
	for _, c := range cases {
		got, extra, ok := read(c.in, o)
		if c.want == nil {
			if !ok || len(got) != 1 || got[0].id != 0 {
				t.Errorf("%q: want a song name search, got %+v %v", c.in, got, ok)
			}
			continue
		}
		if !ok || fmt.Sprint(got) != fmt.Sprint(c.want) || extra != c.extra {
			t.Errorf("%q:\n got %+v extra=%d ok=%v\nwant %+v extra=%d", c.in, got, extra, ok, c.want, c.extra)
		}
	}

	// a song name is always one search, also when it has numbers or mods in it
	if parts, _, ok := ParseAll("!req freedom dive 2 HDHR", o); !ok || len(parts) != 1 || parts[0].Kind != KindSearch || parts[0].Query != "freedom dive 2" || parts[0].Mods != "HDHR" {
		t.Errorf("song name: %+v %v", parts, ok)
	}
	if _, _, ok := ParseAll("hello 1 2", o); ok {
		t.Error("no command, no request")
	}
}

// With one map per message (the default), ParseAll is exactly Parse.
func TestParseAllDefaultIsParse(t *testing.T) {
	for _, max := range []int{0, 1} {
		o := Options{Command: "!req", Aliases: []string{"!r"}, BareNumberAs: "beatmap", Search: true, MaxMaps: max}
		for _, in := range []string{"!req 123 456 HDHR 789", "!req 1 2", "!r 2096297 HD DT please", "!req freedom dive +HDHR",
			"!req https://osu.ppy.sh/b/1 https://osu.ppy.sh/b/2", "!req 123 +HDXX", "!req", "hello", "!req ab"} {
			want, wok := Parse(in, o)
			parts, extra, ok := ParseAll(in, o)
			if ok != wok || extra != 0 || (ok && (len(parts) != 1 || fmt.Sprint(parts[0].Request) != fmt.Sprint(want))) {
				t.Errorf("max %d, %q: ParseAll = %+v %d %v; Parse = %+v %v", max, in, parts, extra, ok, want, wok)
			}
		}
	}
}
