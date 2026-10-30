package update

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func makeZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var b bytes.Buffer
	zw := zip.NewWriter(&b)
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(body))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// fakeRelease serves a zip and its .sha256 under /releases/download/v<ver>/
// and points DownloadPrefix at it for the test.
func fakeRelease(t *testing.T, ver string, zipData []byte, sum string) Release {
	t.Helper()
	name := ZipName(ver)
	mux := http.NewServeMux()
	mux.HandleFunc("/releases/download/v"+ver+"/"+name, func(w http.ResponseWriter, r *http.Request) { w.Write(zipData) })
	mux.HandleFunc("/releases/download/v"+ver+"/"+name+".sha256", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(sum + "  " + name + "\r\n"))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	old := DownloadPrefix
	DownloadPrefix = srv.URL + "/releases/download/"
	t.Cleanup(func() { DownloadPrefix = old })
	base := DownloadPrefix + "v" + ver + "/" + name
	return Release{Version: ver, Zip: base, Sum: base + ".sha256"}
}

func hashOf(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

func TestLatestAssets(t *testing.T) {
	old := DownloadPrefix
	t.Cleanup(func() { DownloadPrefix = old })
	DownloadPrefix = ReleasesURL + "download/"
	dl := DownloadPrefix + "v1.5.0/"
	url := serve(t, `{"tag_name":"v1.5.0","html_url":"`+ReleasesURL+`tag/v1.5.0","assets":[
		{"name":"osureq-1.5.0-windows-x64.zip","browser_download_url":"`+dl+`osureq-1.5.0-windows-x64.zip"},
		{"name":"osureq-1.5.0-windows-x64.zip.sha256","browser_download_url":"https://evil.example/x.sha256"}]}`, 200)
	rel, err := Latest(context.Background(), http.DefaultClient, url, "1.4.1")
	if err != nil || rel == nil {
		t.Fatal(rel, err)
	}
	if rel.Zip != dl+"osureq-1.5.0-windows-x64.zip" || rel.Sum != "" {
		t.Errorf("zip from the release, hash file from elsewhere refused: %+v", rel)
	}
}

func TestDownload(t *testing.T) {
	data := makeZip(t, map[string]string{"osureq.exe": "NEW EXE", "LICENSE": "MIT"})
	rel := fakeRelease(t, "1.5.0", data, strings.ToUpper(hashOf(data)))
	dir := t.TempDir()
	path, err := Download(context.Background(), http.DefaultClient, rel, dir)
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(path); string(b) != "NEW EXE" || filepath.Base(path) != "osureq.update-1.5.0.exe" {
		t.Errorf("staged %s = %q", path, b)
	}
	if v, p := Staged(dir); v != "1.5.0" || p != path {
		t.Errorf("Staged = %s %s", v, p)
	}
}

func TestDownloadRefuses(t *testing.T) {
	good := makeZip(t, map[string]string{"osureq.exe": "NEW EXE"})
	noExe := makeZip(t, map[string]string{"LICENSE": "MIT"})
	nested := makeZip(t, map[string]string{"dist/osureq.exe": "NEW EXE"})
	for name, c := range map[string]struct {
		data []byte
		sum  string
		edit func(*Release)
	}{
		"wrong hash":     {good, hashOf([]byte("something else")), nil},
		"no hash":        {good, "not-a-hash", nil},
		"no exe":         {noExe, hashOf(noExe), nil},
		"exe in folder":  {nested, hashOf(nested), nil},
		"foreign host":   {good, hashOf(good), func(r *Release) { r.Zip = "https://evil.example/osureq.zip" }},
		"no zip in list": {good, hashOf(good), func(r *Release) { r.Zip = "" }},
	} {
		rel := fakeRelease(t, "1.5.0", c.data, c.sum)
		if c.edit != nil {
			c.edit(&rel)
		}
		dir := t.TempDir()
		if _, err := Download(context.Background(), http.DefaultClient, rel, dir); err == nil {
			t.Errorf("%s: want an error", name)
		}
		if left, _ := os.ReadDir(dir); len(left) != 0 {
			t.Errorf("%s: left %d file(s) behind", name, len(left))
		}
	}
}

func TestInstall(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "osureq.exe")
	staged := filepath.Join(dir, StagedName("1.5.0"))
	os.WriteFile(exe, []byte("OLD"), 0o755)
	os.WriteFile(staged, []byte("NEW"), 0o755)
	os.WriteFile(exe+".old", []byte("OLDER"), 0o755)
	if err := Install(exe, staged); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(exe); string(b) != "NEW" {
		t.Errorf("osureq.exe = %q", b)
	}
	if b, _ := os.ReadFile(exe + ".old"); string(b) != "OLD" {
		t.Errorf("osureq.exe.old = %q", b)
	}
	if _, err := os.Stat(staged); !os.IsNotExist(err) {
		t.Error("the staged file should be gone")
	}

	// a missing update puts the old exe back
	if err := Install(exe, filepath.Join(dir, StagedName("9.9.9"))); err == nil {
		t.Error("want an error for a missing update")
	}
	if b, _ := os.ReadFile(exe); string(b) != "NEW" {
		t.Errorf("rolled back osureq.exe = %q", b)
	}
}

func TestCleanup(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "osureq.exe")
	for _, f := range []string{"osureq.exe", "osureq.exe.old", StagedName("1.4.0"), StagedName("1.4.1"), StagedName("1.5.0"), StagedName("1.6.0") + ".part"} {
		os.WriteFile(filepath.Join(dir, f), nil, 0o644)
	}
	Cleanup(exe, "1.4.1")
	var left []string
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		left = append(left, e.Name())
	}
	if strings.Join(left, ",") != "osureq.exe,"+StagedName("1.5.0") {
		t.Errorf("left %v; want osureq.exe and the newer update", left)
	}
}
