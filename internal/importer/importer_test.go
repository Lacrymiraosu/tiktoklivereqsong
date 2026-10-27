package importer

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func TestSafeJoin(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Songs", "1 A - B")
	for _, bad := range []string{"../evil.txt", "a/../../evil.txt", "/abs.txt", "C:/Windows/evil.txt", `..\evil.txt`, "a\\..\\..\\x", ""} {
		if _, err := SafeJoin(root, bad); err == nil {
			t.Errorf("SafeJoin(%q) should fail", bad)
		}
	}
	for _, ok := range []string{"audio.mp3", "sb/bg.png", "Artist - Title (Mapper) [Hard].osu"} {
		if _, err := SafeJoin(root, ok); err != nil {
			t.Errorf("SafeJoin(%q): %v", ok, err)
		}
	}
}

func writeZip(t *testing.T, path string, files map[string]string) {
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for n, c := range files {
		w, _ := zw.Create(n)
		w.Write([]byte(c))
	}
	zw.Close()
	f.Close()
}

func TestExtractRejectsTraversal(t *testing.T) {
	dir := t.TempDir()
	songs := filepath.Join(dir, "Songs")
	os.MkdirAll(songs, 0o755)
	evil := filepath.Join(dir, "evil.osz")
	writeZip(t, evil, map[string]string{"ok.osu": "x", "../../pwned.txt": "x"})
	if _, err := ExtractToSongs(evil, songs, Target{SetID: 1, Artist: "A", Title: "B"}); err == nil {
		t.Fatal("expected traversal error")
	}
	if _, err := os.Stat(filepath.Join(dir, "pwned.txt")); err == nil {
		t.Fatal("file escaped the Songs folder")
	}
}

func TestExtractAndCopy(t *testing.T) {
	dir := t.TempDir()
	songs := filepath.Join(dir, "Songs")
	os.MkdirAll(songs, 0o755)
	src := filepath.Join(dir, "1.osz")
	writeZip(t, src, map[string]string{"a.osu": "x", "sb/bg.png": "y"})
	tg := Target{SetID: 1, Artist: `AC/DC`, Title: `What? <Live>`}
	root, err := ExtractToSongs(src, songs, tg)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(root) != "1 ACDC - What Live" {
		t.Errorf("folder name %q", filepath.Base(root))
	}
	if _, err := os.Stat(filepath.Join(root, "sb", "bg.png")); err != nil {
		t.Error(err)
	}
	dst, err := CopyToSongs(src, songs, tg)
	if err != nil || filepath.Ext(dst) != ".osz" {
		t.Fatalf("copy: %v %s", err, dst)
	}
}

func TestImportRefusesUntrusted(t *testing.T) {
	im := &Importer{Trusted: func(string) bool { return false }, ShellOpen: func(string) error { t.Fatal("opened"); return nil }}
	if err := im.Import("C:/anything.osz", Open, "stable", "", Target{}); err != ErrUntrusted {
		t.Fatalf("got %v", err)
	}
}

func TestScanHashes(t *testing.T) {
	db := []byte{0x14, 0, 0, 0, 0x0b, 0x05, 'h', 'e', 'l', 'l', 'o', 0x0b, 0x20}
	db = append(db, []byte("0123456789abcdef0123456789abcdef")...)
	db = append(db, 0x0b, 0x20)
	db = append(db, []byte("NOT-A-HASH-NOT-A-HASH-NOT-A-HASH")...)
	h := ScanHashes(db)
	if len(h) != 1 || !h["0123456789abcdef0123456789abcdef"] {
		t.Fatalf("got %v", h)
	}
}

func TestSongsDirReadsBeatmapDirectory(t *testing.T) {
	osu := t.TempDir()
	if _, err := SongsDir(osu); err == nil {
		t.Fatal("no Songs folder should be an error")
	}
	os.MkdirAll(filepath.Join(osu, "Songs"), 0o755)
	if d, err := SongsDir(osu); err != nil || d != filepath.Join(osu, "Songs") {
		t.Fatalf("default Songs: %s %v", d, err)
	}
	moved := filepath.Join(t.TempDir(), "Beatmaps")
	os.MkdirAll(moved, 0o755)
	os.WriteFile(filepath.Join(osu, "osu!.someone.cfg"), []byte("VolumeUniversal = 100\r\nBeatmapDirectory = "+moved+"\r\n"), 0o644)
	if d, err := SongsDir(osu); err != nil || d != moved {
		t.Fatalf("moved Songs: got %s %v, want %s", d, err, moved)
	}
}
