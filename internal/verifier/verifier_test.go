package verifier

import (
	"archive/zip"
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"flag"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// Run `go test ./internal/verifier -update` to regenerate testdata/*.osz.
var update = flag.Bool("update", false, "rewrite the .osz fixtures in testdata")

const (
	easyOsu = "osu file format v14\r\n\r\n[General]\r\nAudioFilename: audio.mp3\r\nMode: 0\r\n\r\n[Metadata]\r\nTitle:Fixture\r\nArtist:Tester\r\nCreator:osureq\r\nVersion:Easy\r\nBeatmapID:1001\r\nBeatmapSetID:500\r\n\r\n[HitObjects]\r\n256,192,1000,1,0,0:0:0:0:\r\n"
	hardOsu = "osu file format v14\r\n\r\n[General]\r\nAudioFilename: audio.mp3\r\nMode: 0\r\n\r\n[Metadata]\r\nTitle:Fixture\r\nArtist:Tester\r\nCreator:osureq\r\nVersion:Hard\r\nBeatmapID:1002\r\nBeatmapSetID:500\r\n\r\n[HitObjects]\r\n256,192,1000,1,0,0:0:0:0:\r\n100,100,1500,1,0,0:0:0:0:\r\n"
	// An older revision of Hard, as an outdated mirror would serve it.
	hardOld  = "osu file format v14\r\n\r\n[General]\r\nAudioFilename: audio.mp3\r\nMode: 0\r\n\r\n[Metadata]\r\nTitle:Fixture\r\nArtist:Tester\r\nCreator:osureq\r\nVersion:Hard\r\nBeatmapID:1002\r\nBeatmapSetID:500\r\n\r\n[HitObjects]\r\n256,192,1000,1,0,0:0:0:0:\r\n"
	extraOsu = "osu file format v14\r\n\r\n[Metadata]\r\nVersion:Guest diff not on osu!\r\nBeatmapSetID:500\r\n"
)

func sum(s string) string { h := md5.Sum([]byte(s)); return hex.EncodeToString(h[:]) }

var ref = Reference{Checksums: map[int]string{1001: sum(easyOsu), 1002: sum(hardOsu)}}

func mkzip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, name := range sortedKeys(files) {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(files[name]))
	}
	zw.Close()
	return buf.Bytes()
}

func sortedKeys(m map[string]string) []string { return slices.Sorted(maps.Keys(m)) }

var fixtures = map[string]map[string]string{
	"set_good.osz":     {"Tester - Fixture (osureq) [Easy].osu": easyOsu, "Tester - Fixture (osureq) [Hard].osu": hardOsu, "audio.mp3": "ID3 not really audio"},
	"set_outdated.osz": {"Tester - Fixture (osureq) [Easy].osu": easyOsu, "Tester - Fixture (osureq) [Hard].osu": hardOld, "audio.mp3": "ID3 not really audio"},
	"set_extra.osz":    {"Tester - Fixture (osureq) [Easy].osu": easyOsu, "Tester - Fixture (osureq) [Hard].osu": hardOsu, "extra.osu": extraOsu},
	"set_no_osu.osz":   {"audio.mp3": "ID3", "bg.jpg": "jpeg"},
}

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	p := filepath.Join("testdata", name)
	if *update {
		var b []byte
		if name == "not_zip.osz" {
			b = []byte("<!DOCTYPE html><html><body>Beatmap not found</body></html>")
		} else {
			b = mkzip(t, fixtures[name])
		}
		os.MkdirAll("testdata", 0o755)
		if err := os.WriteFile(p, b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("missing fixture %s (run with -update): %v", p, err)
	}
	return b
}

func TestVerify(t *testing.T) {
	cases := []struct {
		file  string
		ref   Reference
		level Level
		ok    bool
		err   error
	}{
		{"set_good.osz", ref, Strict, true, nil},
		{"set_good.osz", withReq(1002), Requested, true, nil},
		{"set_outdated.osz", ref, Strict, false, nil},
		{"set_outdated.osz", withReq(1002), Requested, false, nil},
		// Easy is unchanged, so a request for Easy passes at "requested" level
		{"set_outdated.osz", withReq(1001), Requested, true, nil},
		// requested by set id -> whole set must match even at "requested" level
		{"set_outdated.osz", ref, Requested, false, nil},
		{"set_extra.osz", ref, Strict, false, nil},
		{"set_extra.osz", ref, Requested, true, nil},
		{"set_no_osu.osz", ref, Strict, false, ErrNoOsu},
		{"not_zip.osz", ref, Strict, false, ErrNotZip},
	}
	for _, c := range cases {
		b := fixture(t, c.file)
		res, err := Verify(b, c.ref, c.level)
		if c.err != nil {
			if err == nil || !errors.Is(err, c.err) {
				t.Errorf("%s/%s: want error %v, got %v", c.file, c.level, c.err, err)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s/%s: unexpected error %v", c.file, c.level, err)
			continue
		}
		if res.OK != c.ok {
			t.Errorf("%s/%s req=%d: OK=%v want %v (%s)", c.file, c.level, c.ref.RequestedID, res.OK, c.ok, res)
		}
	}
}

func withReq(id int) Reference { r := ref; r.RequestedID = id; return r }

func TestMissingAndUnknownReported(t *testing.T) {
	res, err := Verify(fixture(t, "set_outdated.osz"), ref, Strict)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Missing) != 1 || res.Missing[0] != 1002 {
		t.Errorf("missing = %v, want [1002]", res.Missing)
	}
	if len(res.Unknown) != 1 {
		t.Errorf("unknown = %v, want the outdated Hard", res.Unknown)
	}
	if res.Matched != 1 || res.Expected != 2 {
		t.Errorf("matched %d/%d, want 1/2", res.Matched, res.Expected)
	}
}

func TestChecksumCaseInsensitive(t *testing.T) {
	up := Reference{Checksums: map[int]string{1001: "", 1002: ""}}
	for id, s := range ref.Checksums {
		up.Checksums[id] = string(bytes.ToUpper([]byte(s)))
	}
	res, _ := Verify(fixture(t, "set_good.osz"), up, Strict)
	if !res.OK {
		t.Errorf("upper-case reference should match: %s", res)
	}
}

func TestEmptyReferenceNeverPasses(t *testing.T) {
	res, _ := Verify(fixture(t, "set_good.osz"), Reference{Checksums: map[int]string{}}, Requested)
	if res.OK {
		t.Error("an empty reference must not verify")
	}
}

func TestLooksLikeZip(t *testing.T) {
	if LooksLikeZip([]byte(`{"error":"not found"}`)) || LooksLikeZip(nil) {
		t.Error("JSON/empty must not look like a zip")
	}
	if !LooksLikeZip(fixture(t, "set_good.osz")) {
		t.Error("fixture should look like a zip")
	}
}
