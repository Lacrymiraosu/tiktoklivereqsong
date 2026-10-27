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
