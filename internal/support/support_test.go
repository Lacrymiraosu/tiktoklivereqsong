package support

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func files(t *testing.T, b []byte) map[string]string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, f := range zr.File {
		rc, _ := f.Open()
		data, _ := io.ReadAll(rc)
		rc.Close()
		out[f.Name] = string(data)
	}
	return out
}

func TestWrite(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "osureq.log")
	os.WriteFile(logPath, []byte("2026/10/30 line one C:\\Users\\Kiki\\osureq\n2026/10/30 line two\n"), 0o644)
	os.WriteFile(logPath+".old", []byte("old line\n"), 0o644)

	var buf bytes.Buffer
	err := Write(&buf, Input{
		Version: "1.4.0", LogPath: logPath, Home: `C:\Users\Kiki`, Now: time.Now(),
		Config: map[string]any{"osu": map[string]string{"folder": `C:\Users\Kiki\AppData\Local\osu!`, "downloads": "c:/users/kiki/Downloads"}},
		State:  map[string]any{"version": "1.4.0"},
		Raw:    []string{`{"event":"chat","data":{"uniqueId":"fan"}}`},
	})
	if err != nil {
		t.Fatal(err)
	}
	f := files(t, buf.Bytes())
	for _, name := range []string{"README.txt", "config.json", "state.json", "tikfinity-events.json", "osureq.log", "osureq.log.old"} {
		if _, ok := f[name]; !ok {
			t.Errorf("missing %s", name)
		}
	}
	for name, body := range f {
		if strings.Contains(strings.ToLower(body), "kiki") {
			t.Errorf("%s still has the Windows user name:\n%s", name, body)
		}
	}
	if !strings.Contains(f["config.json"], `~\\AppData\\Local\\osu!`) || !strings.Contains(f["config.json"], "~/Downloads") {
		t.Errorf("home folder should become ~:\n%s", f["config.json"])
	}
	if !strings.Contains(f["tikfinity-events.json"], "uniqueId") {
		t.Errorf("raw events missing: %s", f["tikfinity-events.json"])
	}
}

func TestMissingLogIsNoted(t *testing.T) {
	var buf bytes.Buffer
	if err := Write(&buf, Input{LogPath: filepath.Join(t.TempDir(), "nope.log"), Now: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if f := files(t, buf.Bytes()); !strings.Contains(f["README.txt"], "Not included") {
		t.Errorf("README should say the log was missing:\n%s", f["README.txt"])
	}
}

func TestTailStartsAtALine(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x.log")
	os.WriteFile(p, []byte("aaaa\nbbbb\ncccc\n"), 0o644)
	b, err := tail(p, 8)
	if err != nil || string(b) != "cccc\n" {
		t.Errorf("tail = %q, %v", b, err)
	}
}
