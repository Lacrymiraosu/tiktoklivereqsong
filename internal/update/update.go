// Package update asks GitHub whether a newer osureq release exists and, when
// automatic updates are on, downloads it, checks it against the release's
// SHA-256 file and swaps it in for osureq.exe (install.go).
package update

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const ReleasesURL = "https://github.com/Lacrymiraosu/tiktoklivereqsong/releases/"

// LatestURL and DownloadPrefix are variables only so a test build can point
// them at a local fake release (go build -ldflags "-X ...").
var LatestURL = "https://api.github.com/repos/Lacrymiraosu/tiktoklivereqsong/releases/latest"

// DownloadPrefix is the only place release files are downloaded from.
var DownloadPrefix = "https://github.com/Lacrymiraosu/tiktoklivereqsong/releases/download/"

// Release is a published version newer than the running one.
type Release struct {
	Version string `json:"version"`         // e.g. "1.4.0"
	URL     string `json:"url"`             // its page on GitHub
	Ready   bool   `json:"ready,omitempty"` // downloaded and checked; installed on the next start
	Zip     string `json:"-"`               // osureq-<version>-windows-x64.zip
	Sum     string `json:"-"`               // its .sha256 file
}

// Parse reads "1.2.3" or "v1.2.3". Pre-release and dev versions
// ("dev", "0.0.0-dev+abc", "1.4.0-rc1") are not comparable and return false.
func Parse(v string) ([3]int, bool) {
	var out [3]int
	parts := strings.Split(strings.TrimPrefix(strings.TrimSpace(v), "v"), ".")
	if len(parts) != 3 {
		return out, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return out, false
		}
		out[i] = n
	}
	return out, true
}

// Newer reports whether latest is a higher release than current. Builds
// whose version cannot be parsed (dev builds) never see an update.
func Newer(latest, current string) bool {
	l, ok1 := Parse(latest)
	c, ok2 := Parse(current)
	if !ok1 || !ok2 {
		return false
	}
	for i := range l {
		if l[i] != c[i] {
			return l[i] > c[i]
		}
	}
	return false
}

// Latest fetches the newest release. It returns nil when the running
// version is current (or a dev build).
func Latest(ctx context.Context, hc *http.Client, url, current string) (*Release, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "osureq/"+current+" (update check)")
	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub answered %s", resp.Status)
	}
	var body struct {
		Tag        string `json:"tag_name"`
		URL        string `json:"html_url"`
		Draft      bool   `json:"draft"`
		Prerelease bool   `json:"prerelease"`
		Assets     []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(nil, resp.Body, 1<<20)).Decode(&body); err != nil {
		return nil, err
	}
	if body.Draft || body.Prerelease || !Newer(body.Tag, current) {
		return nil, nil
	}
	ver := strings.TrimPrefix(body.Tag, "v")
	link := body.URL
	if !strings.HasPrefix(link, ReleasesURL) {
		link = ReleasesURL + "latest" // only ever point at this project's releases
	}
	rel := &Release{Version: ver, URL: link}
	zip := ZipName(ver)
	for _, a := range body.Assets {
		if !strings.HasPrefix(a.URL, DownloadPrefix) {
			continue
		}
		switch a.Name {
		case zip:
			rel.Zip = a.URL
		case zip + ".sha256":
			rel.Sum = a.URL
		}
	}
	return rel, nil
}

// Run checks shortly after start and then twice a day while enabled()
// is true, calling found with each newer release it sees.
func Run(ctx context.Context, current string, enabled func() bool, found func(Release)) {
	if _, ok := Parse(current); !ok {
		return // dev build
	}
	hc := &http.Client{Timeout: 15 * time.Second}
	wait := 10 * time.Second
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		wait = 12 * time.Hour
		if !enabled() {
			continue
		}
		rel, err := Latest(ctx, hc, LatestURL, current)
		switch {
		case err != nil:
			log.Printf("update check: %v", err)
		case rel != nil:
			found(*rel)
		}
	}
}
