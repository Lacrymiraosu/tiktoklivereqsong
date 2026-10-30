package downloader

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/config"
	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/queue"
	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/verifier"
)

const good = "osu file format v14\n[Metadata]\nVersion:Hard\nBeatmapSetID:9\n"
const stale = "osu file format v14\n[Metadata]\nVersion:Hard (old)\nBeatmapSetID:9\n"

func osz(content string) []byte {
	var b bytes.Buffer
	zw := zip.NewWriter(&b)
	w, _ := zw.Create("a.osu")
	w.Write([]byte(content))
	zw.Close()
	return b.Bytes()
}

func ref() verifier.Reference {
	s := md5.Sum([]byte(good))
	return verifier.Reference{Checksums: map[int]string{90: hex.EncodeToString(s[:])}, RequestedID: 90}
}

// Two fake mirrors: the first serves an outdated copy, the second the right one.
func setup(t *testing.T, mutate func(*config.Config)) (*Downloader, *atomic.Int32) {
	var hits atomic.Int32 // mirrors are probed side by side
	mux := http.NewServeMux()
	mux.HandleFunc("/stale/", func(w http.ResponseWriter, r *http.Request) { hits.Add(1); w.Write(osz(stale)) })
	mux.HandleFunc("/good/", func(w http.ResponseWriter, r *http.Request) { hits.Add(1); w.Write(osz(good)) })
	mux.HandleFunc("/html/", func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Write([]byte("<html>rate limited</html>"))
	})
	mux.HandleFunc("/404/", func(w http.ResponseWriter, r *http.Request) { hits.Add(1); http.NotFound(w, r) })
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	cfg := config.Default()
	cfg.Download.Sources = []config.Source{
		{Name: "html", Enabled: true, URL: srv.URL + "/html/{id}", TimeoutSec: 5},
		{Name: "404", Enabled: true, URL: srv.URL + "/404/{id}", TimeoutSec: 5},
		{Name: "stale", Enabled: true, URL: srv.URL + "/stale/{id}", TimeoutSec: 5},
		{Name: "good", Enabled: true, URL: srv.URL + "/good/{id}", TimeoutSec: 5},
	}
	if mutate != nil {
		mutate(&cfg)
	}
	d := New(func() config.Config { return cfg }, t.TempDir(), func(string) error { return nil })
	d.Allow = func(u string) bool { return strings.HasPrefix(u, srv.URL) }
	return d, &hits
}

func TestFallsThroughToMatchingMirror(t *testing.T) {
	d, _ := setup(t, nil)
	var last []queue.Attempt
	out, err := d.Get(context.Background(), Job{SetID: 9, Ref: ref()}, func(a []queue.Attempt) { last = a })
	if err != nil {
		t.Fatal(err)
	}
	if out.Source != "good" || out.MD5 != "ok" || !d.Trusted(out.File) {
		t.Fatalf("got %+v trusted=%v", out, d.Trusted(out.File))
	}
	want := []string{"failed", "failed", "mismatch", "ok"}
	for i, a := range last {
		if a.Status != want[i] {
			t.Errorf("attempt %d (%s) = %s, want %s (%s)", i, a.Source, a.Status, want[i], a.Message)
		}
	}
}

func TestCacheHitSkipsNetwork(t *testing.T) {
	d, hits := setup(t, nil)
	if _, err := d.Get(context.Background(), Job{SetID: 9, Ref: ref()}, nil); err != nil {
		t.Fatal(err)
	}
	before := hits.Load()
	out, err := d.Get(context.Background(), Job{SetID: 9, Ref: ref()}, nil)
	if err != nil || out.Source != "cache" || hits.Load() != before {
		t.Fatalf("expected cache hit, got %+v err=%v hits %d->%d", out, err, before, hits.Load())
	}
}

func TestAllFailSkipOrWarn(t *testing.T) {
	only := func(c *config.Config) { c.Download.Sources = c.Download.Sources[:3] } // no good mirror
	d, _ := setup(t, only)
	if _, err := d.Get(context.Background(), Job{SetID: 9, Ref: ref()}, nil); err == nil {
		t.Fatal("skip mode should fail when nothing matches")
	}
	d, _ = setup(t, func(c *config.Config) { only(c); c.MD5.OnAllFail = "warn" })
	out, err := d.Get(context.Background(), Job{SetID: 9, Ref: ref()}, nil)
	if err != nil || out.MD5 != "mismatch" || out.Warning == "" || out.Source != "stale" {
		t.Fatalf("warn mode should return the latest download with a warning: %+v %v", out, err)
	}
}

func TestMD5OffTakesFirstZipAndNeverOfficial(t *testing.T) {
	d, _ := setup(t, func(c *config.Config) {
		c.MD5.Enabled = false
		c.Download.Sources = append([]config.Source{{Name: "official", Enabled: true, Official: true, URL: "https://osu.ppy.sh/beatmapsets/{id}/download", TimeoutSec: 5}}, c.Download.Sources...)
	})
	opened := false
	d.OpenURL = func(string) error { opened = true; return nil }
	out, err := d.Get(context.Background(), Job{SetID: 9, Ref: ref()}, nil)
	if err != nil || out.Source != "stale" || out.MD5 != "unverified" {
		t.Fatalf("got %+v %v", out, err)
	}
	if opened {
		t.Error("official source must not be used with MD5 off")
	}
}

func TestCleanupKeepsReferencedFiles(t *testing.T) {
	d, _ := setup(t, nil)
	big := bytes.Repeat([]byte("x"), 600<<10)
	keep := d.cachePath(1, false)
	os.WriteFile(keep, big, 0o644)
	os.WriteFile(d.cachePath(2, false), big, 0o644)
	if n := d.Cleanup(1, map[string]bool{keep: true}); n != 1 {
		t.Fatalf("removed %d, want 1", n)
	}
	if _, err := os.Stat(keep); err != nil {
		t.Error("referenced file was deleted")
	}
}

func TestProbe(t *testing.T) {
	d, _ := setup(t, func(c *config.Config) {
		c.Download.Sources = append(c.Download.Sources,
			config.Source{Name: "off", Enabled: false, URL: c.Download.Sources[0].URL, TimeoutSec: 5},
			config.Source{Name: "official", Enabled: true, Official: true, URL: c.Download.Sources[0].URL, TimeoutSec: 5})
	})
	got := d.Probe(context.Background(), 9)
	want := map[string]bool{"html": false, "404": false, "stale": true, "good": true}
	if len(got) != len(want) {
		t.Fatalf("disabled and official sources must be skipped: %+v", got)
	}
	for _, r := range got {
		if r.OK != want[r.Source] {
			t.Errorf("%s: ok=%v (%s)", r.Source, r.OK, r.Error)
		}
	}
	if entries, _ := os.ReadDir(d.CacheDir); len(entries) != 0 {
		t.Errorf("a probe must not store anything: %v", entries)
	}
}

func TestRedirects(t *testing.T) {
	foreign := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("a non-whitelisted host must never be requested")
	}))
	t.Cleanup(foreign.Close)
	mux := http.NewServeMux()
	mux.HandleFunc("/api/d/", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/storage/9.osz", http.StatusFound) })
	mux.HandleFunc("/storage/", func(w http.ResponseWriter, r *http.Request) { w.Write(osz(good)) })
	mux.HandleFunc("/evil/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, foreign.URL+"/x.osz", http.StatusFound)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	cfg := config.Default()
	cfg.Download.Sources = []config.Source{
		{Name: "evil", Enabled: true, URL: srv.URL + "/evil/{id}", TimeoutSec: 5},
		{Name: "mirror", Enabled: true, URL: srv.URL + "/api/d/{id}", TimeoutSec: 5},
	}
	d := New(func() config.Config { return cfg }, t.TempDir(), func(string) error { return nil })
	d.Allow = func(u string) bool { return strings.HasPrefix(u, srv.URL) }
	var last []queue.Attempt
	out, err := d.Get(context.Background(), Job{SetID: 9, Ref: ref()}, func(a []queue.Attempt) { last = a })
	if err != nil || out.Source != "mirror" {
		t.Fatalf("a redirect on the whitelist should work: %+v %v", out, err)
	}
	if last[0].Status != "failed" || !strings.Contains(last[0].Message, "not whitelisted") {
		t.Errorf("redirect off the whitelist: %+v", last[0])
	}
}

func TestOsuDirectStorageAllowed(t *testing.T) {
	if !config.HostAllowed("https://storage.osu.direct/osudirect/osz/1.osz?X-Amz-Expires=15") {
		t.Error("osu.direct serves its downloads from storage.osu.direct")
	}
	if config.HostAllowed("https://storage.osu.direct.evil.example/x") {
		t.Error("only the exact host is allowed")
	}
}
