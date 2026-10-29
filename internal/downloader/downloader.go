// Package downloader fetches .osz files from the configured sources in order,
// verifies each one, and keeps verified archives in the cache folder.
package downloader

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/config"
	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/queue"
	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/verifier"
)

const maxOsz = 400 << 20

type Job struct {
	SetID    int
	Ref      verifier.Reference
	HasVideo bool
}

type Outcome struct {
	File    string
	Source  string
	MD5     string // ok | cached | unverified | mismatch
	Warning string
}

type Progress func([]queue.Attempt)

type Downloader struct {
	Cfg      func() config.Config
	CacheDir string
	OpenURL  func(string) error // opens the default browser
	HTTP     *http.Client
	Allow    func(url string) bool // host whitelist; tests swap it out

	mu       sync.Mutex
	verified map[string]bool
}

func New(cfg func() config.Config, cacheDir string, openURL func(string) error) *Downloader {
	return &Downloader{
		Cfg: cfg, CacheDir: cacheDir, OpenURL: openURL,
		HTTP: &http.Client{
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) > 5 {
					return errors.New("too many redirects")
				}
				return nil
			},
		},
		Allow:    config.HostAllowed,
		verified: map[string]bool{},
	}
}

// Trusted reports whether path is an archive this helper downloaded and checked.
func (d *Downloader) Trusted(path string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.verified[filepath.Clean(path)]
}

func (d *Downloader) trust(path string) {
	d.mu.Lock()
	d.verified[filepath.Clean(path)] = true
	d.mu.Unlock()
}

func (d *Downloader) cachePath(setID int, noVideo bool) string {
	n := ""
	if noVideo {
		n = "n"
	}
	return filepath.Join(d.CacheDir, strconv.Itoa(setID)+n+".osz")
}

// Get returns a verified archive for the job.
func (d *Downloader) Get(ctx context.Context, job Job, progress Progress) (Outcome, error) {
	cfg := d.Cfg()
	if err := os.MkdirAll(d.CacheDir, 0o755); err != nil {
		return Outcome{}, err
	}
	level := verifier.Level(cfg.MD5.Level)
	check := cfg.MD5.Enabled && len(job.Ref.Checksums) > 0
	noVideo := cfg.Download.NoVideo && job.HasVideo

	var attempts []queue.Attempt
	report := func() {
		if progress != nil {
			progress(append([]queue.Attempt(nil), attempts...))
		}
	}

	// 1. cache
	for _, p := range []string{d.cachePath(job.SetID, noVideo), d.cachePath(job.SetID, !noVideo)} {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		if check && cfg.MD5.VerifyCache {
			res, err := verifier.Verify(b, job.Ref, level)
			if err != nil || !res.OK {
				os.Remove(p)
				continue
			}
		} else if !verifier.LooksLikeZip(b) {
			os.Remove(p)
			continue
		}
		now := time.Now()
		os.Chtimes(p, now, now)
		d.trust(p)
		attempts = append(attempts, queue.Attempt{Source: "cache", Status: "ok", Received: int64(len(b)), Total: int64(len(b))})
		report()
		md5 := "cached"
		if !check {
			md5 = "unverified"
		}
		return Outcome{File: p, Source: "cache", MD5: md5}, nil
	}

	// 2. sources
	var fallback []byte
	var fallbackSrc string
	for _, src := range cfg.Download.Sources {
		if !src.Enabled {
			continue
		}
		if src.Official && !check {
			continue // without MD5 checks the official site is never used
		}
		attempts = append(attempts, queue.Attempt{Source: src.Name, Status: "running"})
		i := len(attempts) - 1
		report()

		var b []byte
		var err error
		if src.Official {
			attempts[i].Message = "waiting for browser download"
			report()
			b, err = d.official(ctx, src, job.SetID, noVideo)
		} else {
			b, err = d.fetch(ctx, src, job.SetID, noVideo, func(got, total int64) {
				attempts[i].Received, attempts[i].Total = got, total
				report()
			})
		}
		if ctx.Err() != nil {
			return Outcome{}, ctx.Err()
		}
		if err != nil {
			attempts[i].Status, attempts[i].Message = "failed", err.Error()
			report()
			continue
		}
		if !check {
			if _, err := verifier.OsuHashes(b); err != nil {
				attempts[i].Status, attempts[i].Message = "failed", err.Error()
				report()
				continue
			}
			p, err := d.store(job.SetID, noVideo, b)
			if err != nil {
				return Outcome{}, err
			}
			attempts[i].Status, attempts[i].Message = "ok", "MD5 check off"
			report()
			return Outcome{File: p, Source: src.Name, MD5: "unverified"}, nil
		}
		res, err := verifier.Verify(b, job.Ref, level)
		if err != nil {
			attempts[i].Status, attempts[i].Message = "failed", err.Error()
			report()
			continue
		}
		if !res.OK {
			attempts[i].Status, attempts[i].Message = "mismatch", res.String()
			report()
			fallback, fallbackSrc = b, src.Name
			continue
		}
		p, err := d.store(job.SetID, noVideo, b)
		if err != nil {
			return Outcome{}, err
		}
		attempts[i].Status, attempts[i].Message = "ok", res.String()
		report()
		return Outcome{File: p, Source: src.Name, MD5: "ok"}, nil
	}

	if fallback != nil && cfg.MD5.OnAllFail == "warn" {
		p, err := d.store(job.SetID, noVideo, fallback)
		if err != nil {
			return Outcome{}, err
		}
		return Outcome{File: p, Source: fallbackSrc, MD5: "mismatch", Warning: "no source matched osu!'s MD5; using the latest download from " + fallbackSrc}, nil
	}
	if len(attempts) == 0 {
		return Outcome{}, errors.New("no download source is enabled")
	}
	return Outcome{}, errors.New("all download sources failed")
}

func srcURL(src config.Source, setID int, noVideo bool) string {
	u := src.URL
	if noVideo && src.NoVideoURL != "" {
		u = src.NoVideoURL
	}
	return strings.ReplaceAll(u, "{id}", strconv.Itoa(setID))
}

func (d *Downloader) fetch(ctx context.Context, src config.Source, setID int, noVideo bool, prog func(got, total int64)) ([]byte, error) {
	u := srcURL(src, setID, noVideo)
	if !d.Allow(u) {
		return nil, fmt.Errorf("host not whitelisted: %s", u)
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(src.TimeoutSec)*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "osureq/1.0 (TikTok LIVE request helper)")
	if src.Referer != "" {
		req.Header.Set("Referer", src.Referer)
	}
	resp, err := d.HTTP.Do(req)
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, fmt.Errorf("timed out after %ds", src.TimeoutSec)
		}
		return nil, err
	}
	defer resp.Body.Close()
	if !d.Allow(resp.Request.URL.String()) {
		return nil, fmt.Errorf("redirected to a host that is not whitelisted: %s", resp.Request.URL.Host)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength > maxOsz {
		return nil, errors.New("file is too large")
	}
	var buf bytes.Buffer
	if resp.ContentLength > 0 {
		buf.Grow(int(resp.ContentLength))
	}
	chunk := make([]byte, 64<<10)
	var got int64
	last := time.Time{}
	for {
		n, err := resp.Body.Read(chunk)
		if n > 0 {
			buf.Write(chunk[:n])
			got += int64(n)
			if got > maxOsz {
				return nil, errors.New("file is too large")
			}
			if got <= int64(n) && !verifier.LooksLikeZip(buf.Bytes()) && got >= 4 {
				return nil, notZip(buf.Bytes())
			}
			if time.Since(last) > 250*time.Millisecond {
				prog(got, resp.ContentLength)
				last = time.Now()
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return nil, fmt.Errorf("timed out after %ds", src.TimeoutSec)
			}
			return nil, err
		}
	}
	prog(got, resp.ContentLength)
	if !verifier.LooksLikeZip(buf.Bytes()) {
		return nil, notZip(buf.Bytes())
	}
	return buf.Bytes(), nil
}

func notZip(b []byte) error {
	s := strings.TrimSpace(string(b[:min(len(b), 120)]))
	s = strings.Map(func(r rune) rune {
		if r < 32 {
			return ' '
		}
		return r
	}, s)
	return fmt.Errorf("not a ZIP (got %q)", s)
}

var partial = []string{".crdownload", ".part", ".partial", ".download", ".tmp"}

// official opens the osu! download page in the browser (where the user is
// logged in) and waits for "{setId} *.osz" to appear in the Downloads folder.
func (d *Downloader) official(ctx context.Context, src config.Source, setID int, noVideo bool) ([]byte, error) {
	cfg := d.Cfg()
	dir := cfg.Osu.DownloadsDir
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		return nil, fmt.Errorf("downloads folder not found: %s", dir)
	}
	u := srcURL(src, setID, noVideo)
	if !d.Allow(u) {
		return nil, fmt.Errorf("host not whitelisted: %s", u)
	}
	start := time.Now().Add(-2 * time.Second)
	if err := d.OpenURL(u); err != nil {
		return nil, fmt.Errorf("could not open browser: %w", err)
	}
	re := regexp.MustCompile(`^` + strconv.Itoa(setID) + `( .*)?\.osz$`)
	ctx, cancel := context.WithTimeout(ctx, time.Duration(src.TimeoutSec)*time.Second)
	defer cancel()
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	sizes := map[string]int64{}
	for {
		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return nil, fmt.Errorf("no %d *.osz appeared in %s within %ds", setID, dir, src.TimeoutSec)
			}
			return nil, ctx.Err()
		case <-tick.C:
		}
		ents, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		busy := false
		for _, e := range ents {
			for _, p := range partial {
				if strings.HasSuffix(strings.ToLower(e.Name()), p) && strings.HasPrefix(e.Name(), strconv.Itoa(setID)) {
					busy = true
				}
			}
		}
		if busy {
			continue
		}
		for _, e := range ents {
			if e.IsDir() || !re.MatchString(e.Name()) {
				continue
			}
			info, err := e.Info()
			if err != nil || info.ModTime().Before(start) {
				continue
			}
			p := filepath.Join(dir, e.Name())
			if prev, ok := sizes[p]; !ok || prev != info.Size() || info.Size() == 0 {
				sizes[p] = info.Size() // wait one more tick for the size to settle
				continue
			}
			b, err := os.ReadFile(p)
			if err != nil {
				continue
			}
			if !verifier.LooksLikeZip(b) {
				return nil, notZip(b)
			}
			os.Remove(p) // it now lives in the cache
			return b, nil
		}
	}
}

func (d *Downloader) store(setID int, noVideo bool, b []byte) (string, error) {
	p := d.cachePath(setID, noVideo)
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, p); err != nil {
		return "", err
	}
	d.trust(p)
	return p, nil
}

// Cleanup deletes the oldest cached archives until the cache fits limitMB,
// never touching files in keep.
func (d *Downloader) Cleanup(limitMB int, keep map[string]bool) (removed int) {
	if limitMB <= 0 {
		return 0
	}
	ents, err := os.ReadDir(d.CacheDir)
	if err != nil {
		return 0
	}
	type f struct {
		path string
		size int64
		mod  time.Time
	}
	var files []f
	var total int64
	for _, e := range ents {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".osz") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, f{filepath.Join(d.CacheDir, e.Name()), info.Size(), info.ModTime()})
		total += info.Size()
	}
	sort.Slice(files, func(i, j int) bool { return files[i].mod.Before(files[j].mod) })
	limit := int64(limitMB) << 20
	for _, x := range files {
		if total <= limit {
			break
		}
		if keep[filepath.Clean(x.path)] || keep[x.path] {
			continue
		}
		if os.Remove(x.path) == nil {
			total -= x.size
			removed++
			d.mu.Lock()
			delete(d.verified, filepath.Clean(x.path))
			d.mu.Unlock()
		}
	}
	return removed
}

// CacheSize returns the cache size in bytes.
func (d *Downloader) CacheSize() int64 {
	var total int64
	ents, _ := os.ReadDir(d.CacheDir)
	for _, e := range ents {
		if info, err := e.Info(); err == nil && !e.IsDir() {
			total += info.Size()
		}
	}
	return total
}
