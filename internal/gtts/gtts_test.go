package gtts

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestSplit(t *testing.T) {
	if got := Split("  สวัสดีครับ  ", 180); len(got) != 1 || got[0] != "สวัสดีครับ" {
		t.Errorf("short text: %q", got)
	}
	words := strings.Repeat("hello ", 50) // 300 letters
	for _, p := range Split(words, 180) {
		if len([]rune(p)) > 180 || strings.HasPrefix(p, " ") || strings.HasSuffix(p, "ell") {
			t.Errorf("piece %q: too long or cut inside a word", p)
		}
	}
	thai := strings.Repeat("ก", 400) // no spaces at all
	got := Split(thai, 180)
	if len(got) != 3 || strings.Join(got, "") != thai {
		t.Errorf("Thai run: %d pieces, joined back = %v", len(got), strings.Join(got, "") == thai)
	}
}

func TestSpeak(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		q := r.URL.Query()
		if q.Get("tl") != "th" || q.Get("client") != "tw-ob" || len([]rune(q.Get("q"))) > 180 {
			http.Error(w, "bad query", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "audio/mpeg")
		w.Write([]byte("mp3[" + q.Get("idx") + "]"))
	}))
	defer srv.Close()
	c := New()
	c.URL = srv.URL

	text := strings.Repeat("สวัสดี ", 40) // two pieces
	b, err := c.Speak(context.Background(), "th", text)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "mp3[0]mp3[1]" {
		t.Errorf("pieces should be joined in order: %q", b)
	}
	if _, err := c.Speak(context.Background(), "th", text); err != nil || hits.Load() != 2 {
		t.Errorf("the same line again should come from the cache: %d requests, %v", hits.Load(), err)
	}
}

func TestSpeakRefuses(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("<html>captcha</html>"))
	}))
	defer srv.Close()
	c := New()
	c.URL = srv.URL
	for name, call := range map[string]func() error{
		"language": func() error { _, err := c.Speak(context.Background(), "jp", "hi"); return err },
		"empty":    func() error { _, err := c.Speak(context.Background(), "th", "  "); return err },
		"too long": func() error {
			_, err := c.Speak(context.Background(), "th", strings.Repeat("ก", MaxText+1))
			return err
		},
		"not audio": func() error { _, err := c.Speak(context.Background(), "en", "hello"); return err },
	} {
		if call() == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}
