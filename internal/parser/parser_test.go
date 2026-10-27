package parser

import "testing"

var opts = Options{Command: "!req", Aliases: []string{"!r", "!request"}, BareNumberAs: "beatmap"}

func TestParse(t *testing.T) {
	cases := []struct {
		in   string
		ok   bool
		want Request
	}{
		{"!req 2096297", true, Request{Kind: KindBeatmap, BeatmapID: 2096297}},
		{"!REQ 2096297", true, Request{Kind: KindBeatmap, BeatmapID: 2096297}},
		{"  !r   123  please", true, Request{Kind: KindBeatmap, BeatmapID: 123}},
		{"!request 123", true, Request{Kind: KindBeatmap, BeatmapID: 123}},
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
	if !ok || got != (Request{Kind: KindSet, SetID: 962870}) {
		t.Errorf("got %+v %v", got, ok)
	}
	// links keep their own meaning
	got, _ = Parse("!req https://osu.ppy.sh/b/5", o)
	if got.Kind != KindBeatmap || got.BeatmapID != 5 {
		t.Errorf("link should stay a beatmap: %+v", got)
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
