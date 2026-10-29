package osumeta

import "testing"

// Trimmed copy of the JSON osu.ppy.sh embeds in <script id="json-beatmapset">.
const page = `{"id":962870,"title":"Song","artist":"Artist","creator":"Mapper","status":"ranked","video":true,
"covers":{"cover":"https://assets.ppy.sh/beatmaps/962870/covers/cover.jpg"},
"beatmaps":[{"id":2096297,"version":"4K Hard","mode":"mania","difficulty_rating":3.21,"total_length":95,"checksum":"BC693B5C8EF5A4832AD9A7AD12AF7C80","status":"ranked"}]}`

func TestParsePageJSON(t *testing.T) {
	s, err := ParsePageJSON([]byte(page))
	if err != nil {
		t.Fatal(err)
	}
	d := s.Diff(2096297)
	if s.ID != 962870 || !s.Video || d == nil || d.Mode != "mania" || d.Stars != 3.21 || d.Length != 95 {
		t.Fatalf("bad parse: %+v %+v", s, d)
	}
	if d.Checksum != "bc693b5c8ef5a4832ad9a7ad12af7c80" {
		t.Errorf("checksum should be lower-cased, got %s", d.Checksum)
	}
	if _, err := ParsePageJSON([]byte(`{"id":1,"beatmaps":[]}`)); err == nil {
		t.Error("a set without beatmaps should fail")
	}
}

func TestParseOsuHeader(t *testing.T) {
	h := ParseOsuHeader([]byte("osu file format v14\r\n\r\n[General]\r\nMode: 3\r\n\r\n[Metadata]\r\nTitle:A: B\r\nBeatmapSetID:42\r\n\r\n[Events]\r\nTitle:nope\r\n"))
	if h["Mode"] != "3" || h["Title"] != "A: B" || h["BeatmapSetID"] != "42" {
		t.Fatalf("got %v", h)
	}
}

func TestParseConvertsAndManiaKeys(t *testing.T) {
	s, err := ParsePageJSON([]byte(`{"id":1,"title":"T","artist":"A","creator":"C","status":"ranked",
	"beatmaps":[{"id":10,"version":"Insane","mode":"osu","difficulty_rating":5.12,"total_length":233,"checksum":"AA","cs":4},
	            {"id":11,"version":"7K Hard","mode":"mania","difficulty_rating":3.9,"total_length":233,"checksum":"BB","cs":7}],
	"converts":[{"id":10,"mode":"mania","difficulty_rating":3.38,"total_length":233,"cs":4,"convert":true},
	            {"id":10,"mode":"taiko","difficulty_rating":3.95,"total_length":233,"convert":true}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if d := s.Diff(11); d.Keys != 7 {
		t.Errorf("mania keys = %d, want 7", d.Keys)
	}
	c := s.Convert(10, "mania")
	if c == nil || c.Stars != 3.38 || !c.Convert || c.Keys != 4 || c.Checksum != "aa" || c.Version != "Insane" {
		t.Fatalf("mania convert = %+v", c)
	}
	if s.Convert(10, "fruits") != nil || s.Convert(11, "taiko") != nil {
		t.Error("only conversions osu! sent should exist")
	}
}
