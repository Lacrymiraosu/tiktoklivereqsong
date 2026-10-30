// Package gtts fetches speech from Google Translate's voice, for when the
// browser has no voice for a language: Windows ships no Thai voice by
// default, and Chrome-based browsers can only use the voices Windows has.
// The text being read is sent to Google.
package gtts

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
)

const DefaultURL = "https://translate.google.com/translate_tts"

// MaxText is the longest text Speak accepts (a 500-letter chat message plus
// "name says").
const MaxText = 600

// chunk is how many letters go in one request; Google refuses more than 200.
const chunk = 180

// Langs are the languages the dashboard reads in.
var Langs = map[string]bool{"th": true, "en": true}

type Client struct {
	HTTP *http.Client
	URL  string

	mu    sync.Mutex
	cache map[string][]byte // recent lines, e.g. the same announcement again
	order []string
}

func New() *Client {
	return &Client{HTTP: &http.Client{Timeout: 15 * time.Second}, URL: DefaultURL, cache: map[string][]byte{}}
}

// Speak returns MP3 audio of text read in lang. Long text is fetched in
// pieces and joined; MP3 frames play back to back.
func (c *Client) Speak(ctx context.Context, lang, text string) ([]byte, error) {
	text = strings.TrimSpace(text)
	switch {
	case !Langs[lang]:
		return nil, fmt.Errorf("no Google voice for %q", lang)
	case text == "":
		return nil, errors.New("nothing to say")
	case len([]rune(text)) > MaxText:
		return nil, fmt.Errorf("text is longer than %d letters", MaxText)
	}
	key := lang + "\x00" + text
	c.mu.Lock()
	if b, ok := c.cache[key]; ok {
		c.mu.Unlock()
		return b, nil
	}
	c.mu.Unlock()

	parts := Split(text, chunk)
	var out bytes.Buffer
	for i, p := range parts {
		b, err := c.fetch(ctx, lang, p, i, len(parts))
		if err != nil {
			return nil, err
		}
		out.Write(b)
	}
	c.mu.Lock()
	c.cache[key] = out.Bytes()
	c.order = append(c.order, key)
	if len(c.order) > 30 {
		delete(c.cache, c.order[0])
		c.order = c.order[1:]
	}
	c.mu.Unlock()
	return out.Bytes(), nil
}

func (c *Client) fetch(ctx context.Context, lang, text string, idx, total int) ([]byte, error) {
	q := url.Values{"ie": {"UTF-8"}, "client": {"tw-ob"}, "tl": {lang}, "q": {text},
		"idx": {strconv.Itoa(idx)}, "total": {strconv.Itoa(total)}, "textlen": {strconv.Itoa(len([]rune(text)))}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.URL+"?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	// Google answers 403 to clients that do not look like a browser
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0 Safari/537.36")
	req.Header.Set("Referer", "https://translate.google.com/")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Google voice answered HTTP %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "audio/") {
		return nil, fmt.Errorf("Google voice sent %q instead of audio", ct)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 4<<20))
}

// Split cuts text into pieces of at most n letters, at a space or after
// punctuation when there is one in the second half of the piece. Thai is
// written without spaces between words, so a long Thai run is cut where it
// reaches n.
func Split(text string, n int) []string {
	var out []string
	r := []rune(strings.TrimSpace(text))
	for len(r) > n {
		cut := n
		for i := n; i > n/2; i-- {
			if unicode.IsSpace(r[i]) || strings.ContainsRune(",.!?;:", r[i-1]) {
				cut = i
				break
			}
		}
		if p := strings.TrimSpace(string(r[:cut])); p != "" {
			out = append(out, p)
		}
		r = []rune(strings.TrimSpace(string(r[cut:])))
	}
	if len(r) > 0 {
		out = append(out, string(r))
	}
	return out
}
