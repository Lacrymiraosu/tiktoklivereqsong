// Package search finds beatmapsets by song name through catboy.best's
// search API (https://catboy.best/api/v2/search), so viewers can request
// "!req freedom dive". It only returns candidates: the app still checks the
// chosen set against osu! (osumeta) and the rules like any other request.
package search

import (
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

const DefaultURL = "https://catboy.best/api/v2/search"

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

// statusCodes are osu!'s numeric ranked states, which catboy filters by.
var statusCodes = map[string]string{
	"graveyard": "-2", "wip": "-1", "pending": "0",
	"ranked": "1", "approved": "2", "qualified": "3", "loved": "4",
}

type Client struct {
	HTTP   *http.Client
	URL    string
	MinGap time.Duration // between requests to catboy

	mu    sync.Mutex
	last  time.Time
	cache map[string]cached
}

type cached struct {
	at  time.Time
	res []Result
}

func New() *Client {
	return &Client{HTTP: &http.Client{Timeout: 10 * time.Second}, URL: DefaultURL, MinGap: time.Second, cache: map[string]cached{}}
}

// Find returns sets matching query with one of the statuses (all when
// empty) and a difficulty playable in one of the modes (all when empty),
// best match first (see Rank).
func (c *Client) Find(ctx context.Context, query string, statuses, modes []string) ([]Result, error) {
	q := url.Values{"q": {query}, "limit": {"50"}}
	for _, s := range statuses {
		if code, ok := statusCodes[s]; ok {
			q.Add("status", code)
		}
	}
	key := q.Encode()
	c.mu.Lock()
	if e, ok := c.cache[key]; ok && time.Since(e.at) < 10*time.Minute {
		c.mu.Unlock()
		return Rank(query, Playable(e.res, modes)), nil
	}
	wait := time.Until(c.last.Add(c.MinGap))
	c.last = time.Now().Add(max(wait, 0))
	c.mu.Unlock()
	if wait > 0 {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(wait):
		}
	}

	res, err := c.fetch(ctx, c.URL+"?"+key)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	c.cache[key] = cached{at: time.Now(), res: res}
	c.mu.Unlock()
	return Rank(query, Playable(res, modes)), nil
}

func (c *Client) fetch(ctx context.Context, u string) ([]Result, error) {
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
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("search answered HTTP %d", resp.StatusCode)
	}
	return Parse(http.MaxBytesReader(nil, resp.Body, 8<<20))
}

// Parse reads catboy's search response (a JSON array of beatmapsets).
func Parse(r io.Reader) ([]Result, error) {
	var raw []struct {
		ID            int    `json:"id"`
		Artist        string `json:"artist"`
		Title         string `json:"title"`
		ArtistUnicode string `json:"artist_unicode"`
		TitleUnicode  string `json:"title_unicode"`
		Creator       string `json:"creator"`
		Status        string `json:"status"`
		PlayCount     int64  `json:"play_count"`
		Beatmaps      []struct {
			Mode string `json:"mode"`
		} `json:"beatmaps"`
	}
	if err := json.NewDecoder(r).Decode(&raw); err != nil {
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
// matches first, each by play count. catboy also matches on tags and puts
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
