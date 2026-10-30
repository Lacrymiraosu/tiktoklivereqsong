// Package search finds beatmapsets by song name through the mirrors'
// search APIs (hinai, Nekoha, osu.direct, then catboy, the order kiki-bm.app
// uses), so viewers can request "!req freedom dive". A mirror that fails or
// finds nothing hands over to the next. It only returns candidates: the app
// still checks the chosen set against osu! (osumeta) and the rules like any
// other request.
package search

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
)

const (
	HinaiURL     = "https://mirror.hinamizawa.ai/api/v2/search"
	NekohaURL    = "https://mirror.nekoha.moe/api/search"
	OsuDirectURL = "https://osu.direct/api/v2/search"
	CatboyURL    = "https://catboy.best/api/v2/search"
)

// cacheFor is how long a search answer is reused.
const cacheFor = 10 * time.Minute

// Result is one beatmapset the search found.
type Result struct {
	SetID  int    `json:"setId"`
	Artist string `json:"artist"`
	Title  string `json:"title"`
	// The names in their own script, when osu! has them (日本語, ไทย…).
	ArtistUnicode string   `json:"artistUnicode,omitempty"`
	TitleUnicode  string   `json:"titleUnicode,omitempty"`
	Creator       string   `json:"creator"`
	Status        string   `json:"status"`
	Plays         int64    `json:"plays"`
	Modes         []string `json:"modes"` // osu taiko fruits mania
}

// statusCodes are osu!'s numeric ranked states, which hinai, osu.direct and
// catboy filter by.
var statusCodes = map[string]string{
	"graveyard": "-2", "wip": "-1", "pending": "0",
	"ranked": "1", "approved": "2", "qualified": "3", "loved": "4",
}

// Mirror is one search API: where it is and how it wants the query.
type Mirror struct {
	Name string
	URL  string
	// Query builds the parameters; local says the statuses could not be
	// sent and the results must be filtered here.
	Query func(q string, statuses []string) (v url.Values, local bool)
}

// Hinai searches mirror.hinamizawa.ai. Only its first status is searched,
// so several statuses are left out and filtered here; most played first,
// since it sorts by ranked date otherwise.
func Hinai(u string) Mirror {
	return Mirror{Name: "hinai", URL: u, Query: func(q string, statuses []string) (url.Values, bool) {
		v := url.Values{"q": {q}, "amount": {"100"}, "sort": {"play_count:desc"}}
		codes := codesOf(statuses)
		if len(codes) == 1 {
			v.Set("status", codes[0])
		}
		return v, len(codes) > 1
	}}
}

// Nekoha searches mirror.nekoha.moe, which takes status names and answers
// {"beatmapsets": [...]}, 100 a page.
func Nekoha(u string) Mirror {
	return Mirror{Name: "nekoha", URL: u, Query: func(q string, statuses []string) (url.Values, bool) {
		v := url.Values{"q": {q}}
		var names []string
		for _, s := range statuses {
			if _, ok := statusCodes[s]; ok {
				names = append(names, s)
			}
		}
		if len(names) > 0 {
			v.Set("status", strings.Join(names, ","))
		}
		return v, false
	}}
}

// OsuDirect searches osu.direct, which takes every status in one request.
func OsuDirect(u string) Mirror {
	return Mirror{Name: "osu.direct", URL: u, Query: func(q string, statuses []string) (url.Values, bool) {
		v := url.Values{"q": {q}, "amount": {"100"}, "sort": {"play_count:desc"}}
		if codes := codesOf(statuses); len(codes) > 0 {
			v.Set("status", strings.Join(codes, ","))
		}
		return v, false
	}}
}

// Catboy searches catboy.best, one status parameter per status.
func Catboy(u string) Mirror {
	return Mirror{Name: "catboy", URL: u, Query: func(q string, statuses []string) (url.Values, bool) {
		return url.Values{"q": {q}, "limit": {"50"}, "status": codesOf(statuses)}, false
	}}
}

// DefaultMirrors are tried in this order.
func DefaultMirrors() []Mirror {
	return []Mirror{Hinai(HinaiURL), Nekoha(NekohaURL), OsuDirect(OsuDirectURL), Catboy(CatboyURL)}
}

func codesOf(statuses []string) []string {
	var out []string
	for _, s := range statuses {
		if code, ok := statusCodes[s]; ok {
			out = append(out, code)
		}
	}
	return out
}

type Client struct {
	HTTP    *http.Client
	Mirrors []Mirror
	MinGap  time.Duration // between search requests

	mu      sync.Mutex
	last    time.Time
	cache   map[string]cached
	resting map[string]time.Time // mirror -> when it may be asked again after HTTP 429
}

type cached struct {
	at  time.Time
	res []Result
}

func New() *Client {
	return &Client{HTTP: &http.Client{Timeout: 10 * time.Second}, Mirrors: DefaultMirrors(), MinGap: time.Second,
		cache: map[string]cached{}, resting: map[string]time.Time{}}
}

// Find returns sets matching query with one of the statuses (all when
// empty) and a difficulty playable in one of the modes (all when empty),
// best match first (see Rank).
func (c *Client) Find(ctx context.Context, query string, statuses, modes []string) ([]Result, error) {
	// "[" starts a filter block on hinai and osu.direct
	query = strings.Join(strings.Fields(strings.NewReplacer("[", " ", "]", " ").Replace(query)), " ")
	key := query + "\x00" + strings.Join(statuses, ",")
	c.mu.Lock()
	if e, ok := c.cache[key]; ok && time.Since(e.at) < cacheFor {
		c.mu.Unlock()
		return Rank(query, Playable(e.res, modes)), nil
	}
	c.mu.Unlock()

	var errs []error
	var res []Result
	asked := 0
	for _, m := range c.Mirrors {
		if c.restingFor(m.Name) > 0 {
			continue
		}
		asked++
		if err := c.wait(ctx); err != nil {
			return nil, err
		}
		v, local := m.Query(query, statuses)
		rs, err := c.fetch(ctx, m, m.URL+"?"+v.Encode())
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", m.Name, err))
			continue
		}
		if local {
			rs = withStatus(rs, statuses)
		}
		res = rs
		if len(rs) > 0 {
			break // nothing found may also be a mirror whose sources are down: ask the next
		}
	}
	if asked == 0 {
		return nil, errors.New("every search mirror is rate limited; try again in a minute")
	}
	if res == nil && len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	c.mu.Lock()
	for k, e := range c.cache { // every chat search lands here: drop the stale ones
		if time.Since(e.at) >= cacheFor {
			delete(c.cache, k)
		}
	}
	c.cache[key] = cached{at: time.Now(), res: res}
	c.mu.Unlock()
	return Rank(query, Playable(res, modes)), nil
}

// wait keeps MinGap between requests.
func (c *Client) wait(ctx context.Context) error {
	c.mu.Lock()
	wait := time.Until(c.last.Add(c.MinGap))
	c.last = time.Now().Add(max(wait, 0))
	c.mu.Unlock()
	if wait <= 0 {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(wait):
		return nil
	}
}

func (c *Client) restingFor(name string) time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	return time.Until(c.resting[name])
}

func (c *Client) fetch(ctx context.Context, m Mirror, u string) ([]Result, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	// catboy refuses requests that do not say who is asking
	req.Header.Set("User-Agent", "osureq/1.0 (TikTok LIVE request helper; +https://github.com/Lacrymiraosu/tiktoklivereqsong)")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusTooManyRequests {
		wait := 60 * time.Second
		if n, err := strconv.Atoi(strings.TrimSpace(resp.Header.Get("Retry-After"))); err == nil && n > 0 {
			wait = min(time.Duration(n)*time.Second, time.Hour)
		}
		c.mu.Lock()
		c.resting[m.Name] = time.Now().Add(wait)
		c.mu.Unlock()
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("search answered HTTP %d", resp.StatusCode)
	}
	return Parse(http.MaxBytesReader(nil, resp.Body, 8<<20))
}

// withStatus keeps results in one of statuses (all when empty). osu!
// searches ranked and approved together, and so does this.
func withStatus(rs []Result, statuses []string) []Result {
	if len(statuses) == 0 {
		return rs
	}
	ok := map[string]bool{}
	for _, s := range statuses {
		ok[s] = true
		if s == "ranked" || s == "approved" {
			ok["ranked"], ok["approved"] = true, true
		}
	}
	var out []Result
	for _, r := range rs {
		if ok[r.Status] {
			out = append(out, r)
		}
	}
	return out
}

// Parse reads a search response: a JSON array of beatmapsets in osu!'s
// format, or Nekoha's {"beatmapsets": [...]}.
func Parse(r io.Reader) ([]Result, error) {
	type set struct {
		ID            int    `json:"id"`
		Artist        string `json:"artist"`
		Title         string `json:"title"`
		ArtistUnicode string `json:"artist_unicode"`
		TitleUnicode  string `json:"title_unicode"`
		Creator       string `json:"creator"`
		Status        string `json:"status"`
		PlayCount     int64  `json:"play_count"`
		Beatmaps      []struct {
			Mode      string `json:"mode"`
			PlayCount int64  `json:"playcount"`
		} `json:"beatmaps"`
	}
	var msg json.RawMessage
	if err := json.NewDecoder(r).Decode(&msg); err != nil {
		return nil, errors.New("unexpected search response: " + err.Error())
	}
	var raw []set
	if b := bytes.TrimSpace(msg); len(b) > 0 && b[0] == '{' {
		var wrapped struct {
			Beatmapsets []set `json:"beatmapsets"`
		}
		if err := json.Unmarshal(b, &wrapped); err != nil {
			return nil, errors.New("unexpected search response: " + err.Error())
		}
		raw = wrapped.Beatmapsets
	} else if err := json.Unmarshal(b, &raw); err != nil {
		return nil, errors.New("unexpected search response: " + err.Error())
	}
	out := make([]Result, 0, len(raw))
	for _, s := range raw {
		if s.ID <= 0 {
			continue
		}
		r := Result{SetID: s.ID, Artist: s.Artist, Title: s.Title, ArtistUnicode: s.ArtistUnicode, TitleUnicode: s.TitleUnicode,
			Creator: s.Creator, Status: s.Status, Plays: s.PlayCount}
		seen := map[string]bool{}
		for _, b := range s.Beatmaps {
			if s.PlayCount == 0 { // Nekoha has no set play count for older sets
				r.Plays += b.PlayCount
			}
			if b.Mode != "" && !seen[b.Mode] {
				seen[b.Mode] = true
				r.Modes = append(r.Modes, b.Mode)
			}
		}
		out = append(out, r)
	}
	return out, nil
}

// Playable keeps sets with a difficulty in one of modes. osu! maps convert
// to the other modes, so a set with an osu! difficulty stays when any mode
// is allowed; the rules decide later whether the converted map fits.
func Playable(rs []Result, modes []string) []Result {
	if len(modes) == 0 {
		return rs
	}
	allowed := map[string]bool{}
	for _, m := range modes {
		allowed[m] = true
	}
	var out []Result
	for _, r := range rs {
		for _, m := range r.Modes {
			if allowed[m] || m == "osu" {
				out = append(out, r)
				break
			}
		}
	}
	return out
}

// Rank keeps the results whose title, or artist and title, contain every
// word of the query (in Latin letters or the original script), title
// matches first, each by play count. The mirrors also match on tags and put
// obscure uploads first, so "freedom dive" would otherwise find a graveyard
// copy or a compilation, and "night of nights" a map that only has those
// words in its tags.
func Rank(query string, rs []Result) []Result {
	words := tokens(query)
	tier := func(r Result) int {
		switch {
		case containsAll(tokens(r.Title), words) || containsAll(tokens(r.TitleUnicode), words):
			return 0
		case containsAll(tokens(r.Artist+" "+r.Title), words) || containsAll(tokens(r.ArtistUnicode+" "+r.TitleUnicode), words):
			return 1
		}
		return 2
	}
	var out []Result
	for _, r := range rs {
		if tier(r) < 2 {
			out = append(out, r)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		ti, tj := tier(out[i]), tier(out[j])
		if ti != tj {
			return ti < tj
		}
		return out[i].Plays > out[j].Plays
	})
	return out
}

// tokens lowercases s and splits it into words of letters and digits.
func tokens(s string) []string {
	return strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && !unicode.Is(unicode.Mn, r) && !unicode.Is(unicode.Mc, r)
	})
}

func containsAll(have, want []string) bool {
	if len(want) == 0 {
		return false
	}
	set := map[string]bool{}
	for _, h := range have {
		set[h] = true
	}
	joined := strings.Join(have, "")
	for _, w := range want {
		// "Kani*Do-Luck" is one word for a viewer typing "kanidoluck"
		if !set[w] && !strings.Contains(joined, w) {
			return false
		}
	}
	return true
}

// Label is how a result reads in logs: "xi - FREEDOM DiVE (39804)".
func (r Result) Label() string {
	return r.Artist + " - " + r.Title + " (" + strconv.Itoa(r.SetID) + ")"
}
