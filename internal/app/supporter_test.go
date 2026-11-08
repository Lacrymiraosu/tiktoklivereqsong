package app

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/config"
	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/downloader"
	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/importer"
	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/osumeta"
	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/queue"
)

// The map every test here uses: set 9, difficulty 90.
const (
	supSet  = 9
	supDiff = 90
	supOsu  = "osu file format v14\n[Metadata]\nTitle:Song\nArtist:xi\nVersion:Hard\nBeatmapSetID:9\nBeatmapID:90\n"
)

func supSum() string {
	s := md5.Sum([]byte(supOsu))
	return hex.EncodeToString(s[:])
}

// opens records every way osureq could hand something to Windows: the app's
// OpenURL, the importer's ShellOpen and the downloader's browser.
type opens struct {
	mu  sync.Mutex
	all []string
}

func (o *opens) open(s string) error {
	o.mu.Lock()
	o.all = append(o.all, s)
	o.mu.Unlock()
	return nil
}

func (o *opens) list() []string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]string(nil), o.all...)
}

func (o *opens) links() (n int) {
	for _, s := range o.list() {
		if strings.HasPrefix(s, "osu://") || strings.HasPrefix(s, "http") {
			n++
		}
	}
	return n
}

// supWeb answers osu!'s pages for set 9 and a mirror serving its .osz.
type supWeb struct {
	mu     sync.Mutex
	fail   bool // the mirror answers HTTP 500
	mirror int  // downloads asked for
	pages  []string
}

func (f *supWeb) RoundTrip(r *http.Request) (*http.Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	body, code := "", http.StatusOK
	switch {
	case r.URL.Host == "osu.ppy.sh" && (r.URL.Path == fmt.Sprintf("/beatmapsets/%d", supSet) || r.URL.Path == fmt.Sprintf("/beatmaps/%d", supDiff)):
		f.pages = append(f.pages, r.URL.Path)
		body = fmt.Sprintf(`<html><script id="json-beatmapset" type="application/json">{"id":%d,"title":"Song","artist":"xi","creator":"m","status":"ranked",
			"beatmaps":[{"id":%d,"version":"Hard","mode":"osu","difficulty_rating":4,"total_length":200,"checksum":%q,"status":"ranked"}]}</script></html>`, supSet, supDiff, supSum())
	case r.URL.Host == "mirror.test":
		f.mirror++
		if f.fail {
			code = http.StatusInternalServerError
			break
		}
		var b bytes.Buffer
		zw := zip.NewWriter(&b)
		w, _ := zw.Create("xi - Song (m) [Hard].osu")
		w.Write([]byte(supOsu))
		zw.Close()
		body = b.String()
	default:
		code = http.StatusNotFound
	}
	return &http.Response{StatusCode: code, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}, Request: r}, nil
}

func (f *supWeb) downloads() int { f.mu.Lock(); defer f.mu.Unlock(); return f.mirror }

type supCase struct {
	supporter, useOsuDb, installed, lazer, dry bool
}

func (c supCase) String() string {
	return fmt.Sprintf("supporter=%v useOsuDb=%v inOsuDb=%v lazer=%v dry=%v", c.supporter, c.useOsuDb, c.installed, c.lazer, c.dry)
}

// supporterApp builds an App whose osu! folder, cache, osu!.db, osu! pages
// and mirror are all fakes in a temp folder; nothing reaches Windows.
func supporterApp(t *testing.T, c supCase) (*App, *opens, *supWeb) {
	t.Helper()
	osuDir := t.TempDir()
	if c.installed {
		db := append([]byte("osu!db\x00\x0b\x20"), supSum()...)
		if err := os.WriteFile(filepath.Join(osuDir, "osu!.db"), db, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	a := newTestApp(t, func(cfg *config.Config) {
		cfg.Osu.Folder, cfg.Osu.Supporter, cfg.Osu.UseOsuDB, cfg.Osu.DetectGame = osuDir, c.supporter, c.useOsuDb, false
		cfg.Osu.Game = "stable"
		if c.lazer {
			cfg.Osu.Game = "lazer"
		}
		cfg.Test.DryRun = c.dry
		cfg.Import.Mode, cfg.Import.Preload = "open", false
	})
	rec, web := &opens{}, &supWeb{}
	a.OpenURL = rec.open
	a.Meta = osumeta.New()
	a.Meta.MinGap = 0
	a.Meta.HTTP = &http.Client{Transport: web}
	cfg := func() config.Config { // only the fake mirror: no official source and its browser
		x := a.Cfg.Get()
		x.Download.Sources = []config.Source{{Name: "fake", Enabled: true, URL: "https://mirror.test/d/{id}", TimeoutSec: 5}}
		return x
	}
	a.DL = downloader.New(cfg, filepath.Join(t.TempDir(), "cache"), rec.open)
	a.DL.HTTP = &http.Client{Transport: web}
	a.DL.Allow = func(u string) bool { return strings.HasPrefix(u, "https://mirror.test/") }
	a.Import = &importer.Importer{Trusted: a.DL.Trusted, ShellOpen: rec.open}
	return a, rec, web
}

func supItem(id string) *queue.Item {
	return &queue.Item{ID: id, User: "fan", SetID: supSet, BeatmapID: supDiff, Title: "Song", Artist: "xi", Version: "Hard", AddedAt: time.Now()}
}

// every combination of the switches, with fn checking one of them
func eachCase(t *testing.T, fn func(t *testing.T, c supCase)) {
	for _, sup := range []bool{false, true} {
		for _, db := range []bool{false, true} {
			for _, inst := range []bool{false, true} {
				for _, lazer := range []bool{false, true} {
					for _, dry := range []bool{false, true} {
						c := supCase{sup, db, inst, lazer, dry}
						t.Run(c.String(), func(t *testing.T) { fn(t, c) })
					}
				}
			}
		}
	}
}

// noBrowserOnStable is the bug: without supporter, stable must never get an
// osu:// link (osu! sends it to the browser) or any web page.
func noBrowserOnStable(t *testing.T, c supCase, rec *opens) {
	t.Helper()
	if !c.lazer && !c.supporter && rec.links() > 0 {
		t.Errorf("stable without supporter opened a link: %v", rec.list())
	}
}

func onlyOsz(t *testing.T, rec *opens) {
	t.Helper()
	if l := rec.list(); len(l) != 1 || !strings.HasSuffix(l[0], ".osz") {
		t.Errorf("want one .osz opened, got %v", l)
	}
}

func onlyLink(t *testing.T, rec *opens, want string) {
	t.Helper()
	if l := rec.list(); len(l) != 1 || l[0] != want {
		t.Errorf("want only %s, got %v", want, l)
	}
}

// Next on a queued map: the osu!.db shortcut (osu://b, no download) is taken
// only by supporters on stable with "Skip maps already in osu!" on.
func TestNextSupporterMatrix(t *testing.T) {
	eachCase(t, func(t *testing.T, c supCase) {
		a, rec, web := supporterApp(t, c)
		a.Queue.Add(supItem("r1"))
		if err := a.Next("played"); err != nil {
			t.Fatal(err)
		}
		if now, ok := a.Queue.Now(); !ok || now.ID != "r1" {
			t.Fatalf("r1 should be now playing: %+v", now)
		}
		noBrowserOnStable(t, c, rec)
		shortcut := c.supporter && c.useOsuDb && c.installed && !c.lazer
		switch {
		case shortcut && web.downloads() != 0:
			t.Errorf("a supporter's installed map must not be downloaded (%d)", web.downloads())
		case !shortcut && web.downloads() != 1:
			t.Errorf("want one download, got %d", web.downloads())
		}
		switch {
		case c.dry:
			if l := rec.list(); len(l) != 0 {
				t.Errorf("dry run opened %v", l)
			}
		case shortcut:
			onlyLink(t, rec, "osu://b/90")
		default:
			onlyOsz(t, rec)
		}
	})
}

// A queued map already marked "installed" (found in osu!.db while supporter
// was on, or from an old queue.json). useOsuDb and osu!.db no longer matter then,
// as they do not for Previous and Jump: every combination must still hold.
func TestNextStaleInstalledItem(t *testing.T) {
	eachCase(t, func(t *testing.T, c supCase) {
		a, rec, web := supporterApp(t, c)
		addInstalled(a, "old")
		if err := a.Next("played"); err != nil {
			t.Fatal(err)
		}
		noBrowserOnStable(t, c, rec)
		now, _ := a.Queue.Now()
		links := c.supporter || c.lazer // osu://b as before
		if links {
			if web.downloads() != 0 || now.MD5 != "installed" {
				t.Errorf("osu://b users keep the shortcut: %d downloads, MD5 %q", web.downloads(), now.MD5)
			}
		} else if web.downloads() != 1 || now.MD5 != "ok" || now.Source != "fake" || now.File == "" {
			t.Errorf("without supporter the map is downloaded: %d downloads, %+v", web.downloads(), now)
		}
		switch {
		case c.dry:
			if l := rec.list(); len(l) != 0 {
				t.Errorf("dry run opened %v", l)
			}
		case links:
			onlyLink(t, rec, "osu://b/90")
		default:
			onlyOsz(t, rec)
		}
	})
}

func TestNextStaleInstalledDownloadFails(t *testing.T) {
	a, rec, web := supporterApp(t, supCase{})
	web.fail = true
	addInstalled(a, "old")
	err := a.Next("played")
	var ue *UserError
	if err == nil || !asUserError(err, &ue) || ue.Code != "get_failed" {
		t.Fatalf("want get_failed, got %v", err)
	}
	if l := rec.list(); len(l) != 0 {
		t.Errorf("a failed download must open nothing, opened %v", l)
	}
	if got, _ := a.Queue.Get("old"); got.State != queue.Failed || got.MD5 == "installed" {
		t.Errorf("the item should be failed and no longer installed: %+v", got)
	}
	if _, playing := a.Queue.Now(); playing {
		t.Error("nothing should be playing")
	}
	// Next again: the failed item is passed over, no loop and still no link
	if err := a.Next("played"); err == nil {
		t.Error("only a failed item: the queue is empty")
	}
	if web.downloads() != 1 || rec.links() != 0 {
		t.Errorf("downloads %d, opened %v", web.downloads(), rec.list())
	}
}

// addInstalled queues the map as download() marks one found in osu!.db.
func addInstalled(a *App, id string) {
	a.Queue.Add(supItem(id))
	a.Queue.Update(id, func(x *queue.Item) {
		x.State, x.Source, x.MD5, x.File = queue.Ready, "osu!.db", "installed", ""
		x.Attempts = []queue.Attempt{{Source: "osu!.db", Status: "ok", Message: "already in osu!"}}
	})
}

func asUserError(err error, ue **UserError) bool {
	u, ok := err.(*UserError)
	*ue = u
	return ok
}

// played makes r1 the previous map (r2 now playing) with r1's file as given.
func played(t *testing.T, a *App, file string) {
	t.Helper()
	one, two := supItem("r1"), supItem("r2")
	two.SetID, two.BeatmapID = 10, 100
	one.State, one.File, one.MD5, two.State = queue.Ready, file, "ok", queue.Ready
	a.Queue.Add(one)
	a.Queue.Add(two)
	a.Queue.Promote("r1", "played")
	a.Queue.Promote("r2", "played")
}

func TestPreviousSupporterMatrix(t *testing.T) {
	for _, present := range []bool{false, true} {
		t.Run(fmt.Sprintf("file present=%v", present), func(t *testing.T) {
			eachCase(t, func(t *testing.T, c supCase) {
				a, rec, web := supporterApp(t, c)
				file := filepath.Join(t.TempDir(), "gone.osz")
				if present {
					out, err := a.DL.Get(context.Background(), downloader.Job{SetID: supSet}, nil) // a checked file, as after playing it
					if err != nil {
						t.Fatal(err)
					}
					file = out.File
				}
				before := web.downloads()
				played(t, a, file)
				if err := a.Previous(); err != nil {
					t.Fatal(err)
				}
				if now, _ := a.Queue.Now(); now.ID != "r1" {
					t.Fatalf("now = %s", now.ID)
				}
				noBrowserOnStable(t, c, rec)
				fetched := web.downloads() - before
				switch {
				case c.dry:
					if l := rec.list(); len(l) != 0 || fetched != 0 {
						t.Errorf("dry run opened %v, downloaded %d", l, fetched)
					}
				case present:
					onlyLink(t, rec, file)
				case c.supporter || c.lazer:
					onlyLink(t, rec, "osu://b/90")
				default:
					onlyOsz(t, rec)
					if fetched != 1 {
						t.Errorf("the gone file should be downloaded again, %d downloads", fetched)
					}
				}
			})
		})
	}
}

func TestPreviousDownloadFails(t *testing.T) {
	a, rec, web := supporterApp(t, supCase{})
	web.fail = true
	played(t, a, filepath.Join(t.TempDir(), "gone.osz"))
	err := a.Previous()
	var ue *UserError
	if err == nil || !asUserError(err, &ue) || ue.Code != "open_failed" {
		t.Fatalf("want open_failed, got %v", err)
	}
	if l := rec.list(); len(l) != 0 {
		t.Errorf("opened %v", l)
	}
	if now, _ := a.Queue.Now(); now.ID != "r2" {
		t.Errorf("a failed Previous must not move the queue, now %s", now.ID)
	}
}

// The history's "Open in osu!" for an entry with both IDs, only the set
// (old entries), or only the beatmap.
func TestJumpSupporterMatrix(t *testing.T) {
	entries := []struct {
		name          string
		set, beatmapp int
	}{{"both", supSet, supDiff}, {"set only", supSet, 0}, {"beatmap only", 0, supDiff}}
	for _, e := range entries {
		t.Run(e.name, func(t *testing.T) {
			eachCase(t, func(t *testing.T, c supCase) {
				a, rec, web := supporterApp(t, c)
				if err := a.Jump(e.set, e.beatmapp); err != nil {
					t.Fatal(err)
				}
				noBrowserOnStable(t, c, rec)
				switch {
				case c.dry:
					if l := rec.list(); len(l) != 0 || web.downloads() != 0 {
						t.Errorf("dry run opened %v, downloaded %d", l, web.downloads())
					}
					if n := a.Notices(); len(n) == 0 || n[0].Key != "dryOpen" {
						t.Errorf("dry run should say what it would open: %+v", n)
					}
				case c.supporter || c.lazer:
					onlyLink(t, rec, "osu://b/90")
					if web.downloads() != 0 {
						t.Errorf("osu://b needs no download, got %d", web.downloads())
					}
				default:
					onlyOsz(t, rec)
					if web.downloads() != 1 {
						t.Errorf("want one download, got %d", web.downloads())
					}
					// again: this run already checked the file, so no second download
					if err := a.Jump(e.set, e.beatmapp); err != nil {
						t.Fatal(err)
					}
					if l := rec.list(); web.downloads() != 1 || len(l) != 2 || l[1] != l[0] {
						t.Errorf("the second jump should reuse the file: %d downloads, opened %v", web.downloads(), l)
					}
				}
				if a.Queue.Len() != 0 {
					t.Error("jumping must not touch the queue")
				}
			})
		})
	}
}

func TestJumpDownloadFails(t *testing.T) {
	for _, e := range [][2]int{{supSet, supDiff}, {supSet, 0}, {0, supDiff}} {
		a, rec, web := supporterApp(t, supCase{})
		web.fail = true
		err := a.Jump(e[0], e[1])
		var ue *UserError
		if err == nil || !asUserError(err, &ue) || ue.Code != "open_failed" || !strings.Contains(err.Error(), "all download sources failed") {
			t.Errorf("%v: want open_failed after the failed download, got %v", e, err)
		}
		if l := rec.list(); len(l) != 0 {
			t.Errorf("%v: a failed jump must open nothing, opened %v", e, l)
		}
	}
	// no osu! lookup and no downloader (newTestApp): an error, never a link
	a := newTestApp(t, nil)
	rec := &opens{}
	a.OpenURL = rec.open
	if a.Jump(supSet, supDiff) == nil || a.Jump(0, supDiff) == nil || rec.links() != 0 {
		t.Errorf("without downloads set up Jump must fail without a link, opened %v", rec.list())
	}
}

// After a restart the cache folder still has the set: DL.Get checks it again
// and opens it without asking the mirror.
func TestJumpReusesCacheAfterRestart(t *testing.T) {
	a, rec, web := supporterApp(t, supCase{})
	if err := a.Jump(supSet, supDiff); err != nil {
		t.Fatal(err)
	}
	cfg := a.DL.Cfg
	a.DL = downloader.New(cfg, a.DL.CacheDir, rec.open) // a new run: nothing trusted yet
	a.DL.HTTP = &http.Client{Transport: web}
	a.DL.Allow = func(u string) bool { return strings.HasPrefix(u, "https://mirror.test/") }
	a.Import = &importer.Importer{Trusted: a.DL.Trusted, ShellOpen: rec.open}
	if _, ok := a.DL.CachedFile(supSet, false); ok {
		t.Fatal("a new run should not trust the cache yet")
	}
	if err := a.Jump(supSet, supDiff); err != nil {
		t.Fatal(err)
	}
	if l := rec.list(); web.downloads() != 1 || len(l) != 2 || l[1] != l[0] {
		t.Errorf("want the cached file reused: %d downloads, opened %v", web.downloads(), l)
	}
}

// The old behaviour of TestJumpOpensHistoryMap, now for supporters.
func TestJumpSupporterOpensLink(t *testing.T) {
	a := newTestApp(t, func(c *config.Config) { c.Osu.Supporter = true })
	rec := &opens{}
	a.OpenURL = rec.open
	if err := a.Jump(39804, 86324); err != nil {
		t.Fatal(err)
	}
	onlyLink(t, rec, "osu://b/86324")
	if err := a.Jump(39804, 0); err == nil {
		t.Error("an old entry without a difficulty and no osu! lookup should fail, not open a wrong link")
	}
}
