package remote

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/app"
	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/config"
	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/queue"
	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/tikfinity"
	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/tosu"
)

func newPanel(t *testing.T) (*Server, http.Handler, *config.Store) {
	t.Helper()
	dir := t.TempDir()
	store, err := config.Load(filepath.Join(dir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	c := store.Get()
	c.Remote = config.Remote{Enabled: true, Port: 21281, PIN: "482915"}
	if err := store.Set(c); err != nil {
		t.Fatal(err)
	}
	q, _ := queue.Open(filepath.Join(dir, "queue.json"))
	no := func(string) error { return errors.New("no") }
	tf := tikfinity.New(tikfinity.Env{Find: func(string) (string, bool) { return "", false }, Running: func(string) bool { return false }, Launch: no,
		Close: func(string) (int, error) { return 0, nil }}, func() tikfinity.Settings { return tikfinity.Settings{} })
	a := app.New(store, q, nil, nil, nil, tf, no)
	a.Tosu = tosu.New(tosu.Env{}, func() tosu.Settings { return tosu.Settings{} })
	s := New(a, func() config.Remote { return store.Get().Remote })
	return s, s.Handler(), store
}

type client struct {
	t      *testing.T
	h      http.Handler
	ip     string
	host   string
	cookie *http.Cookie
}

func (c *client) do(method, path, body string, header bool) (*httptest.ResponseRecorder, map[string]any) {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.RemoteAddr = net.JoinHostPort(c.ip, "50000")
	r.Host = c.host
	if header {
		r.Header.Set("X-Osureq", "1")
	}
	if c.cookie != nil {
		r.AddCookie(c.cookie)
	}
	w := httptest.NewRecorder()
	c.h.ServeHTTP(w, r)
	for _, ck := range w.Result().Cookies() {
		if ck.Name == cookieName {
			c.cookie = ck
		}
	}
	var m map[string]any
	json.Unmarshal(w.Body.Bytes(), &m)
	return w, m
}

func phone(t *testing.T, h http.Handler) *client {
	return &client{t: t, h: h, ip: "192.168.1.23", host: "192.168.1.10:21281"}
}

func TestOnlyPrivateNetworksAndLocalHostNames(t *testing.T) {
	_, h, _ := newPanel(t)
	for _, ip := range []string{"8.8.8.8", "203.0.113.5", "2001:4860::1"} {
		c := phone(t, h)
		c.ip = ip
		if w, _ := c.do("GET", "/m", "", false); w.Code != http.StatusForbidden {
			t.Errorf("internet address %s: %d", ip, w.Code)
		}
	}
	for _, ip := range []string{"192.168.1.23", "10.0.0.7", "172.20.1.1", "100.101.102.103", "127.0.0.1", "fd00::5"} {
		c := phone(t, h)
		c.ip = ip
		if w, _ := c.do("GET", "/m", "", false); w.Code != http.StatusOK {
			t.Errorf("network address %s: %d", ip, w.Code)
		}
	}
	for host, ok := range map[string]bool{"192.168.1.10:21281": true, "localhost:21281": true, "streampc.local:21281": true, "[fd00::1]:21281": true,
		"evil.example:21281": false, "192.168.1.10.nip.io:21281": false} {
		c := phone(t, h)
		c.host = host
		if w, _ := c.do("GET", "/m", "", false); (w.Code == http.StatusOK) != ok {
			t.Errorf("Host %s: %d", host, w.Code)
		}
	}
}

func TestLoginAndActions(t *testing.T) {
	s, h, store := newPanel(t)
	s.App.Queue.Add(&queue.Item{ID: "a", User: "fan", Title: "Song A", AddedAt: time.Now()})
	s.App.Queue.Add(&queue.Item{ID: "b", User: "kid", Title: "Song B", AddedAt: time.Now()})
	c := phone(t, h)

	if w, m := c.do("GET", "/m/api/state", "", false); w.Code != http.StatusUnauthorized || m["code"] != "login" {
		t.Fatalf("state without login: %d %v", w.Code, m)
	}
	if w, _ := c.do("POST", "/m/login", `{"pin":"482915"}`, false); w.Code != http.StatusForbidden {
		t.Fatalf("login without the header: %d", w.Code)
	}
	if _, m := c.do("POST", "/m/login", `{"pin":"482915"}`, true); m["ok"] != true || c.cookie == nil {
		t.Fatalf("login: %v", m)
	}
	if !c.cookie.HttpOnly || c.cookie.SameSite != http.SameSiteStrictMode {
		t.Errorf("cookie must be HttpOnly and SameSite=Strict: %+v", c.cookie)
	}
	_, m := c.do("GET", "/m/api/state", "", false)
	if items, _ := m["items"].([]any); len(items) != 2 || m["requests"] != true {
		t.Fatalf("state: %v", m)
	}

	if w, _ := c.do("POST", "/m/api/queue/b/move", `{"to":0}`, false); w.Code != http.StatusForbidden {
		t.Errorf("action without the header: %d", w.Code)
	}
	if _, m := c.do("POST", "/m/api/queue/b/move", `{"to":0}`, true); m["ok"] != true || s.App.Queue.First(1)[0].ID != "b" {
		t.Errorf("move: %v", m)
	}
	if _, m := c.do("POST", "/m/api/queue/a/remove", "", true); m["ok"] != true || s.App.Queue.Len() != 1 {
		t.Errorf("remove: %v", m)
	}
	c.do("POST", "/m/api/action/toggleRequests", "", true)
	if store.Get().Command.Enabled {
		t.Error("toggleRequests did not close requests")
	}
	for _, path := range []string{"/m/api/action/quit", "/m/api/action/clear", "/m/api/queue/b/play"} {
		if w, _ := c.do("POST", path, "", true); w.Code != http.StatusNotFound {
			t.Errorf("%s must not exist on the mod panel: %d", path, w.Code)
		}
	}
	if n := s.App.Notices(); len(n) == 0 || !strings.Contains(n[0].Text, "mod panel (192.168.1.23)") {
		t.Errorf("actions should be in Activity: %+v", n)
	}

	// a new PIN ends every session
	cfg := store.Get()
	cfg.Remote.PIN = "730261"
	store.Set(cfg)
	if w, _ := c.do("GET", "/m/api/state", "", false); w.Code != http.StatusUnauthorized {
		t.Errorf("old session after a PIN change: %d", w.Code)
	}
	c.do("POST", "/m/login", `{"pin":"730261"}`, true)
	c.do("POST", "/m/logout", "", true)
	if w, _ := c.do("GET", "/m/api/state", "", false); w.Code != http.StatusUnauthorized {
		t.Errorf("after logout: %d", w.Code)
	}
}

func TestLockouts(t *testing.T) {
	s, h, _ := newPanel(t)
	now := time.Now()
	s.now = func() time.Time { return now }
	c := phone(t, h)
	for i := 0; i < maxFailsIP; i++ {
		if w, m := c.do("POST", "/m/login", `{"pin":"000001"}`, true); w.Code != http.StatusUnauthorized || m["code"] != "wrong_pin" {
			t.Fatalf("wrong PIN %d: %d %v", i, w.Code, m)
		}
	}
	if w, m := c.do("POST", "/m/login", `{"pin":"482915"}`, true); w.Code != http.StatusTooManyRequests || m["code"] != "locked" {
		t.Fatalf("even the right PIN is refused while locked: %d %v", w.Code, m)
	}
	other := phone(t, h)
	other.ip = "192.168.1.99"
	if _, m := other.do("POST", "/m/login", `{"pin":"482915"}`, true); m["ok"] != true {
		t.Errorf("another device is not locked out by one address: %v", m)
	}
	now = now.Add(lockFor + time.Second)
	if _, m := c.do("POST", "/m/login", `{"pin":"482915"}`, true); m["ok"] != true {
		t.Errorf("the lock ends after %v: %v", lockFor, m)
	}

	// many devices guessing together lock the panel for everyone
	_, h2, _ := newPanel(t)
	for i := 0; i < maxFailsAll; i++ {
		c := phone(t, h2)
		c.ip = "10.0.0." + strconv.Itoa(i+1) // a different device each time
		c.do("POST", "/m/login", `{"pin":"000001"}`, true)
	}
	if w, _ := phone(t, h2).do("POST", "/m/login", `{"pin":"482915"}`, true); w.Code != http.StatusTooManyRequests {
		t.Errorf("global lock: %d", w.Code)
	}
}

func TestGoodPIN(t *testing.T) {
	for pin, ok := range map[string]bool{"482915": true, "73026194": true, "12345": false, "1234567890123": false, "123456": false,
		"987654": false, "111111": false, "12a456": false, "": false} {
		if config.GoodPIN(pin) != ok {
			t.Errorf("GoodPIN(%q) = %v", pin, !ok)
		}
	}
}

func TestPreviewOnStreamPC(t *testing.T) {
	s, h, _ := newPanel(t)
	s.App.Queue.Add(&queue.Item{ID: "a", User: "fan", SetID: 39804, Title: "FREEDOM DiVE", AddedAt: time.Now()})
	c := phone(t, h)
	c.do("POST", "/m/login", `{"pin":"482915"}`, true)

	if _, m := c.do("POST", "/m/api/queue/a/preview", "", true); m["ok"] != true {
		t.Fatalf("preview: %v", m)
	}
	if ask := s.App.PreviewAsked(); ask.SetID != 39804 || ask.N != 1 {
		t.Errorf("the dashboard should be asked to play set 39804 once: %+v", ask)
	}
	c.do("POST", "/m/api/queue/a/preview", "", true)
	if ask := s.App.PreviewAsked(); ask.N != 2 {
		t.Errorf("every press is a new ask (the dashboard toggles): %+v", ask)
	}
	if _, m := c.do("POST", "/m/api/queue/gone/preview", "", true); m["ok"] != false {
		t.Errorf("a request that is gone: %v", m)
	}

	s.App.SetPreviewing(39804) // the dashboard reports what it plays
	if _, m := c.do("GET", "/m/api/state", "", false); m["previewing"] != float64(39804) {
		t.Errorf("the mod panel should see which preview plays: %v", m["previewing"])
	}
}
