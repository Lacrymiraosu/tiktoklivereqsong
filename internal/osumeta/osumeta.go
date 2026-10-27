// Package osumeta fetches beatmap metadata and reference MD5s from osu.ppy.sh
// without an API key. It scrapes unofficial endpoints, so all of that logic
// lives here and nowhere else.
package osumeta

import (
	"bufio"
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const base = "https://osu.ppy.sh"

type Diff struct {
	ID       int     `json:"id"`
	Version  string  `json:"version"`
	Mode     string  `json:"mode"`
	Stars    float64 `json:"stars"`
	Length   int     `json:"length"` // seconds
	Checksum string  `json:"checksum"`
	Status   string  `json:"status"`
}

type Set struct {
	ID      int       `json:"id"`
	Title   string    `json:"title"`
	Artist  string    `json:"artist"`
	Creator string    `json:"creator"`
	Status  string    `json:"status"`
	Video   bool      `json:"video"`
	Cover   string    `json:"cover"`
	Diffs   []Diff    `json:"diffs"`
	Partial bool      `json:"partial"` // built from a raw .osu fallback; stars/length/status unknown
	Fetched time.Time `json:"-"`
}

func (s *Set) Diff(id int) *Diff {
	for i := range s.Diffs {
		if s.Diffs[i].ID == id {
			return &s.Diffs[i]
		}
	}
	return nil
}

var ErrNotFound = errors.New("beatmap not found on osu!")

type Client struct {
	HTTP   *http.Client
	TTL    time.Duration
	MinGap time.Duration // between requests to osu.ppy.sh

	mu     sync.Mutex
	sets   map[int]*Set
	mapSet map[int]int
	last   time.Time
	gate   sync.Mutex
}

func New() *Client {
	return &Client{
		HTTP:   &http.Client{Timeout: 20 * time.Second},
		TTL:    10 * time.Minute,
		MinGap: 700 * time.Millisecond,
		sets:   map[int]*Set{},
		mapSet: map[int]int{},
	}
}

// ResolveSet returns the set id for a beatmap id.
func (c *Client) ResolveSet(ctx context.Context, beatmapID int) (int, error) {
	c.mu.Lock()
	if s, ok := c.mapSet[beatmapID]; ok {
		c.mu.Unlock()
		return s, nil
	}
	c.mu.Unlock()
	set, err := c.fetchPage(ctx, fmt.Sprintf("%s/beatmaps/%d", base, beatmapID))
	if err == nil {
		return set.ID, nil
	}
	if !errors.Is(err, errNoJSON) {
		return 0, err
	}
	fb, ferr := c.fromRawOsu(ctx, beatmapID)
	if ferr != nil {
		return 0, err
	}
	return fb.ID, nil
}

// Get returns the beatmapset. beatmapID (optional) is used for the raw .osu fallback.
func (c *Client) Get(ctx context.Context, setID, beatmapID int) (*Set, error) {
	c.mu.Lock()
	if s, ok := c.sets[setID]; ok && time.Since(s.Fetched) < c.TTL && !s.Partial {
		c.mu.Unlock()
		return s, nil
	}
	c.mu.Unlock()
	s, err := c.fetchPage(ctx, fmt.Sprintf("%s/beatmapsets/%d", base, setID))
	if err == nil {
		return s, nil
	}
	if errors.Is(err, ErrNotFound) || beatmapID == 0 {
		return nil, err
	}
	fb, ferr := c.fromRawOsu(ctx, beatmapID)
	if ferr != nil {
		return nil, fmt.Errorf("%v (fallback: %v)", err, ferr)
	}
	return fb, nil
}

var errNoJSON = errors.New("osu! page did not contain beatmapset data")

var reJSON = regexp.MustCompile(`(?s)<script id="json-beatmapset" type="application/json">(.*?)</script>`)

func (c *Client) fetchPage(ctx context.Context, url string) (*Set, error) {
	body, _, err := c.get(ctx, url)
	if err != nil {
		return nil, err
	}
	m := reJSON.FindSubmatch(body)
	if m == nil {
		return nil, errNoJSON
	}
	s, err := ParsePageJSON(bytes.TrimSpace(m[1]))
	if err != nil {
		return nil, err
	}
	c.store(s)
	return s, nil
}

type rawSet struct {
	ID      int    `json:"id"`
	Title   string `json:"title"`
	Artist  string `json:"artist"`
	Creator string `json:"creator"`
	Status  string `json:"status"`
	Video   bool   `json:"video"`
	Covers  struct {
		Cover string `json:"cover"`
	} `json:"covers"`
	Beatmaps []struct {
		ID       int     `json:"id"`
		Version  string  `json:"version"`
		Mode     string  `json:"mode"`
		Stars    float64 `json:"difficulty_rating"`
		Length   int     `json:"total_length"`
		Checksum string  `json:"checksum"`
		Status   string  `json:"status"`
	} `json:"beatmaps"`
}

// ParsePageJSON parses the beatmapset JSON embedded in the osu! website.
func ParsePageJSON(b []byte) (*Set, error) {
	var r rawSet
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, fmt.Errorf("osu! beatmapset JSON: %w", err)
	}
	if r.ID == 0 || len(r.Beatmaps) == 0 {
		return nil, errNoJSON
	}
	s := &Set{ID: r.ID, Title: r.Title, Artist: r.Artist, Creator: r.Creator, Status: r.Status, Video: r.Video, Cover: r.Covers.Cover, Fetched: time.Now()}
	for _, b := range r.Beatmaps {
		s.Diffs = append(s.Diffs, Diff{ID: b.ID, Version: b.Version, Mode: b.Mode, Stars: b.Stars, Length: b.Length, Checksum: strings.ToLower(b.Checksum), Status: b.Status})
	}
	return s, nil
}

func (c *Client) fromRawOsu(ctx context.Context, beatmapID int) (*Set, error) {
	body, _, err := c.get(ctx, fmt.Sprintf("%s/osu/%d", base, beatmapID))
	if err != nil {
		return nil, err
	}
	if len(body) == 0 || !bytes.HasPrefix(bytes.TrimLeft(bytes.TrimPrefix(body, []byte{0xEF, 0xBB, 0xBF}), " \r\n"), []byte("osu file format")) {
		return nil, ErrNotFound
	}
	sum := md5.Sum(body)
	h := ParseOsuHeader(body)
	setID, _ := strconv.Atoi(h["BeatmapSetID"])
	if setID <= 0 {
		return nil, errors.New("raw .osu has no BeatmapSetID")
	}
	s := &Set{
		ID: setID, Title: h["Title"], Artist: h["Artist"], Creator: h["Creator"], Partial: true, Fetched: time.Now(),
		Cover: fmt.Sprintf("https://assets.ppy.sh/beatmaps/%d/covers/cover.jpg", setID),
		Diffs: []Diff{{ID: beatmapID, Version: h["Version"], Mode: modeName(h["Mode"]), Stars: -1, Length: -1, Checksum: hex.EncodeToString(sum[:])}},
	}
	c.store(s)
	return s, nil
}

// ParseOsuHeader reads key:value pairs from the [General] and [Metadata] sections.
func ParseOsuHeader(b []byte) map[string]string {
	out := map[string]string{}
	sc := bufio.NewScanner(bytes.NewReader(b))
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	sec := ""
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "[") {
			sec = line
			if sec != "[General]" && sec != "[Metadata]" && len(out) > 0 && out["BeatmapSetID"] != "" {
				break
			}
			continue
		}
		if sec != "[General]" && sec != "[Metadata]" {
			continue
		}
		if k, v, ok := strings.Cut(line, ":"); ok {
			out[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	return out
}

func modeName(m string) string {
	switch m {
	case "1":
		return "taiko"
	case "2":
		return "fruits"
	case "3":
		return "mania"
	}
	return "osu"
}

func (c *Client) store(s *Set) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if old, ok := c.sets[s.ID]; ok && s.Partial && !old.Partial {
		return
	}
	c.sets[s.ID] = s
	for _, d := range s.Diffs {
		c.mapSet[d.ID] = s.ID
	}
}

func (c *Client) get(ctx context.Context, url string) ([]byte, *http.Response, error) {
	c.gate.Lock()
	if wait := c.MinGap - time.Since(c.last); wait > 0 {
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			c.gate.Unlock()
			return nil, nil, ctx.Err()
		}
	}
	c.last = time.Now()
	c.gate.Unlock()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("User-Agent", "osureq/1.0 (TikTok LIVE request helper)")
	req.Header.Set("Accept", "text/html,application/json;q=0.9,*/*;q=0.8")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return nil, resp, ErrNotFound
	case resp.StatusCode == http.StatusTooManyRequests:
		return nil, resp, errors.New("osu! rate limited the helper, try again shortly")
	case resp.StatusCode != http.StatusOK:
		return nil, resp, fmt.Errorf("osu! returned HTTP %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	return b, resp, err
}
