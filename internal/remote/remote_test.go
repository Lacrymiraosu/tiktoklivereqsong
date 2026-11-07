package remote

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/app"
	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/config"
	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/queue"
	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/tikfinity"
	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/tosu"
	"github.com/coder/websocket"
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

func TestStateShowsViewers(t *testing.T) {
	s, h, _ := newPanel(t)
	s.App.TF.Settings = func() tikfinity.Settings {
		return tikfinity.Settings{Direct: true, User: "streamer", Reconnect: time.Millisecond}
	}
	s.App.TF.Direct = func(ctx context.Context, user, apiKey string, ev tikfinity.DirectEvents) error {
		ev.Connected()
		ev.Viewers(1234)
		<-ctx.Done()
		return ctx.Err()
	}
	c := phone(t, h)
	c.do("POST", "/m/login", `{"pin":"482915"}`, true)
	if _, m := c.do("GET", "/m/api/state", "", false); m["viewers"] != float64(0) {
		t.Errorf("before the LIVE: viewers = %v, want 0 (unknown)", m["viewers"])
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.App.TF.Run(ctx)
	deadline := time.Now().Add(3 * time.Second)
	for s.App.TF.Status().Viewers != 1234 {
		if time.Now().After(deadline) {
			t.Fatalf("viewers never arrived: %+v", s.App.TF.Status())
		}
		time.Sleep(5 * time.Millisecond)
	}
	if _, m := c.do("GET", "/m/api/state", "", false); m["viewers"] != float64(1234) || m["chatSource"] != "tiktok" {
		t.Errorf("state: viewers = %v, chatSource = %v", m["viewers"], m["chatSource"])
	}
}

func TestKikiPlayerOnModPanel(t *testing.T) {
	s, h, store := newPanel(t)
	s.App.Queue.Add(&queue.Item{ID: "a", User: "fan", SetID: 39804, BeatmapID: 129891, Mode: "osu", Mods: "HDDT", Title: "FREEDOM DiVE", AddedAt: time.Now()})
	c := phone(t, h)
	w, _ := c.do("GET", "/m", "", false)
	csp := w.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "; frame-src https://player.kiki-bm.app;") {
		t.Errorf("the page may frame KIKI's player and nothing else: %q", csp)
	}
	for _, keep := range []string{"default-src 'self'", "connect-src 'self'", "frame-ancestors 'none'", "script-src 'unsafe-inline';"} {
		if !strings.Contains(csp, keep) {
			t.Errorf("CSP lost %q: %q", keep, csp)
		}
	}
	if w.Header().Get("X-Frame-Options") != "DENY" {
		t.Errorf("the panel itself must not be framed: %v", w.Header())
	}
	c.do("POST", "/m/login", `{"pin":"482915"}`, true)
	_, m := c.do("GET", "/m/api/state", "", false)
	if m["kikiPlayer"] != true {
		t.Errorf("the map player is on by default: %v", m["kikiPlayer"])
	}
	items, _ := m["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("items: %v", m["items"])
	}
	it := items[0].(map[string]any)
	if it["mode"] != "osu" || it["mods"] != "HDDT" || it["beatmapId"] != float64(129891) || it["setId"] != float64(39804) {
		t.Errorf("a row needs mode, mods and both ids for the player: %v", it)
	}
	cfg := store.Get()
	cfg.Osu.KikiPlayerMod = false
	if err := store.Set(cfg); err != nil {
		t.Fatal(err)
	}
	if _, m := c.do("GET", "/m/api/state", "", false); m["kikiPlayer"] != false {
		t.Errorf("turned off in Settings: %v", m["kikiPlayer"])
	}
}

// overlayPanel is newPanel with the overlay link on. It returns the link's secret.
func overlayPanel(t *testing.T) (*Server, http.Handler, *config.Store, string) {
	t.Helper()
	s, h, store := newPanel(t)
	c := store.Get()
	c.Overlay.Remote = true
	c.Overlay.RemoteToken = config.NewOverlayToken()
	if err := store.Set(c); err != nil {
		t.Fatal(err)
	}
	s.OverlayPage = []byte("<!DOCTYPE html><title>overlay</title>")
	s.Version = "test"
	return s, h, store, c.Overlay.RemoteToken
}

// logTo sends the log to a buffer for the rest of the test.
func logTo(t *testing.T) *bytes.Buffer {
	var buf bytes.Buffer
	old := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(old) })
	return &buf
}

func TestOverlayLink(t *testing.T) {
	s, h, store, tok := overlayPanel(t)
	logs := logTo(t)
	obs := phone(t, h) // OBS on the other PC
	w, _ := obs.do("GET", "/o/"+tok, "", false)
	if w.Code != http.StatusOK || w.Body.String() != string(s.OverlayPage) {
		t.Fatalf("right link: %d %q", w.Code, w.Body)
	}
	hd := w.Header()
	for k, want := range map[string]string{"Cache-Control": "no-store", "X-Content-Type-Options": "nosniff", "Referrer-Policy": "no-referrer", "X-Frame-Options": "DENY",
		"Content-Type": "text/html; charset=utf-8"} {
		if hd.Get(k) != want {
			t.Errorf("%s = %q, want %q", k, hd.Get(k), want)
		}
	}
	csp := hd.Get("Content-Security-Policy")
	for _, keep := range []string{"default-src 'none'", "script-src 'unsafe-inline'", "img-src https: data:", "connect-src 'self'", "frame-ancestors 'none'", "font-src https://fonts.gstatic.com"} {
		if !strings.Contains(csp, keep) {
			t.Errorf("overlay CSP lacks %q: %q", keep, csp)
		}
	}
	if strings.Contains(csp, "player.kiki-bm.app") {
		t.Errorf("the overlay frames nothing: %q", csp)
	}

	// a wrong link looks like any missing page, and counts like a wrong PIN
	for _, p := range []string{"/o/" + config.NewOverlayToken(), "/o/" + tok[:len(tok)-1], "/o/x"} {
		if w, _ := obs.do("GET", p, "", false); w.Code != http.StatusNotFound {
			t.Errorf("wrong link %s: %d", p, w.Code)
		}
	}
	if f := s.failsIP[obs.ip]; f == nil || f.n != 3 {
		t.Errorf("wrong links must count toward the lockout: %+v", f)
	}
	if s.failsAll.n != 0 {
		t.Errorf("wrong links must not count toward the panel-wide PIN lock: %d", s.failsAll.n)
	}
	if w, _ := obs.do("GET", "/o/"+tok+"/state", "", false); w.Code != http.StatusNotFound {
		t.Errorf("nothing else under the link: %d", w.Code)
	}

	// read-only: no other method exists under /o/
	for _, m := range []string{"POST", "PUT", "DELETE", "PATCH"} {
		if w, _ := obs.do(m, "/o/"+tok, "{}", true); w.Code != http.StatusNotFound {
			t.Errorf("%s /o/<link>: %d", m, w.Code)
		}
		if w, _ := obs.do(m, "/o/"+tok, "{}", false); w.Code != http.StatusForbidden {
			t.Errorf("%s /o/<link> without the header: %d", m, w.Code)
		}
	}

	// only from your own network, exactly like the mod panel
	far := phone(t, h)
	far.ip = "203.0.113.5"
	if w, _ := far.do("GET", "/o/"+tok, "", false); w.Code != http.StatusForbidden {
		t.Errorf("internet address: %d", w.Code)
	}
	evil := phone(t, h)
	evil.host = "evil.example:21281"
	if w, _ := evil.do("GET", "/o/"+tok, "", false); w.Code != http.StatusForbidden {
		t.Errorf("foreign Host: %d", w.Code)
	}

	// the other routes and the status never carry the secret
	pc := phone(t, h)
	pc.ip = "192.168.1.50"
	for _, p := range []string{"/", "/m", "/m/api/state"} {
		if w, _ := pc.do("GET", p, "", false); strings.Contains(w.Body.String()+fmt.Sprint(w.Header()), tok) {
			t.Errorf("%s shows the link", p)
		}
	}
	pc.do("POST", "/m/login", `{"pin":"482915"}`, true)
	if w, _ := pc.do("GET", "/m/api/state", "", false); strings.Contains(w.Body.String(), tok) {
		t.Error("the mod panel state shows the link")
	}
	if b, _ := json.Marshal(s.Status()); strings.Contains(string(b), tok) {
		t.Errorf("status shows the link: %s", b)
	}

	// switched off: a 404 even with the right link, and nothing is counted
	c := store.Get()
	c.Overlay.Remote = false
	store.Set(c)
	before := s.failsIP[pc.ip]
	if w, _ := pc.do("GET", "/o/"+tok, "", false); w.Code != http.StatusNotFound {
		t.Errorf("link off: %d", w.Code)
	}
	pc.do("GET", "/o/wrong", "", false)
	if s.failsIP[pc.ip] != before {
		t.Errorf("nothing to guess while the link is off: %+v", s.failsIP[pc.ip])
	}

	// mod panel off: no link either (and no listener at all)
	c.Overlay.Remote = true
	c.Remote.Enabled = false
	store.Set(c)
	if w, _ := pc.do("GET", "/o/"+tok, "", false); w.Code != http.StatusNotFound {
		t.Errorf("mod panel off: %d", w.Code)
	}
	s.Apply(c.Remote)
	if st := s.Status(); st.Enabled || s.srv != nil {
		t.Errorf("no listener while the mod panel is off: %+v", st)
	}
	if strings.Contains(logs.String(), tok) || strings.Contains(logs.String(), tok[:len(tok)-1]) {
		t.Errorf("the log shows the link: %s", logs)
	}
	if !strings.Contains(logs.String(), "overlay link: wrong link from 192.168.1.23") {
		t.Errorf("wrong links should be logged (without the link): %s", logs)
	}
}

// TestOverlayLinkNoRedirect: an unclean path is not redirected to the clean
// one, which would put the link in a Location header.
func TestOverlayLinkNoRedirect(t *testing.T) {
	_, h, _, tok := overlayPanel(t)
	c := phone(t, h)
	for _, p := range []string{"//o/" + tok, "/o/./" + tok, "/o/x/../" + tok, "/o//" + tok, "/./o/" + tok, "/o/" + tok + "/./ws", "/o/" + tok + "/"} {
		w, _ := c.do("GET", p, "", false)
		if w.Code != http.StatusNotFound || w.Header().Get("Location") != "" {
			t.Errorf("%s: %d, Location %q", strings.ReplaceAll(p, tok, "<link>"), w.Code, strings.ReplaceAll(w.Header().Get("Location"), tok, "<link>"))
		}
	}
	if w, _ := c.do("GET", "//m", "", false); w.Code != http.StatusMovedPermanently && w.Code != http.StatusTemporaryRedirect {
		t.Errorf("other paths keep the usual clean-up redirect: %d", w.Code)
	}
}

func TestOverlayLinkLockout(t *testing.T) {
	s, h, _, tok := overlayPanel(t)
	now := time.Now()
	s.now = func() time.Time { return now }
	obs := phone(t, h)
	for i := 0; i < maxFailsIP; i++ {
		obs.do("GET", "/o/"+config.NewOverlayToken(), "", false)
	}
	if w, _ := obs.do("GET", "/o/"+tok, "", false); w.Code != http.StatusNotFound {
		t.Errorf("even the right link is refused while locked: %d", w.Code)
	}
	if w, _ := obs.do("GET", "/o/"+tok+"/ws", "", false); w.Code != http.StatusNotFound {
		t.Errorf("and its data too: %d", w.Code)
	}
	if w, m := obs.do("POST", "/m/login", `{"pin":"482915"}`, true); w.Code != http.StatusTooManyRequests || m["code"] != "locked" {
		t.Errorf("it is the same lockout as for wrong PINs: %d %v", w.Code, m)
	}
	other := phone(t, h)
	other.ip = "192.168.1.99"
	if w, _ := other.do("GET", "/o/"+tok, "", false); w.Code != http.StatusOK {
		t.Errorf("another PC is not locked out: %d", w.Code)
	}
	if w, m := other.do("POST", "/m/login", `{"pin":"482915"}`, true); m["ok"] != true {
		t.Errorf("the PIN login is not locked for everyone: %d %v", w.Code, m)
	}
	now = now.Add(lockFor + time.Second)
	if w, _ := obs.do("GET", "/o/"+tok, "", false); w.Code != http.StatusOK {
		t.Errorf("the lock ends after %v: %d", lockFor, w.Code)
	}

	// the right link does not wipe wrong PINs from the same address
	pc := phone(t, h)
	pc.ip = "192.168.1.77"
	for i := 0; i < maxFailsIP-1; i++ {
		pc.do("POST", "/m/login", `{"pin":"000001"}`, true)
	}
	pc.do("GET", "/o/"+tok, "", false)
	pc.do("POST", "/m/login", `{"pin":"000001"}`, true)
	if w, _ := pc.do("POST", "/m/login", `{"pin":"482915"}`, true); w.Code != http.StatusTooManyRequests {
		t.Errorf("loading the overlay must not reset the PIN count: %d", w.Code)
	}
}

func TestOverlayNewLinkDoesNotLockOutOBS(t *testing.T) {
	s, h, store, old := overlayPanel(t)
	c := store.Get()
	c.Overlay.RemoteToken = config.NewOverlayToken() // "New link"
	store.Set(c)
	obs := phone(t, h)
	for i := 0; i < 3*maxFailsIP; i++ { // OBS still has the old link and keeps retrying
		if w, _ := obs.do("GET", "/o/"+old+"/ws", "", false); w.Code != http.StatusNotFound {
			t.Fatalf("old link: %d", w.Code)
		}
	}
	if f := s.failsIP[obs.ip]; f != nil {
		t.Errorf("a replaced link is not a guess: %+v", f)
	}
	if w, _ := obs.do("GET", "/o/"+c.Overlay.RemoteToken, "", false); w.Code != http.StatusOK {
		t.Errorf("the new link works on the same PC: %d", w.Code)
	}
}

func TestOverlayWebSocket(t *testing.T) {
	s, h, store, tok := overlayPanel(t)
	c := store.Get()
	c.Gifts.RequestCoins = 10 // a gift then makes a pop-up
	store.Set(c)
	s.App.Queue.Add(&queue.Item{ID: "a", User: "fan", Title: "Song A", Artist: "Artist", Stars: 5.2, Mode: "osu", AddedAt: time.Now()})
	srv := httptest.NewServer(h) // 127.0.0.1: loopback counts as your own network
	defer srv.Close()
	base := "ws" + strings.TrimPrefix(srv.URL, "http")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if _, resp, err := websocket.Dial(ctx, base+"/o/"+config.NewOverlayToken()+"/ws", nil); err == nil || resp == nil || resp.StatusCode != http.StatusNotFound {
		t.Fatalf("wrong link: %v %v", err, resp)
	}
	conn, _, err := websocket.Dial(ctx, base+"/o/"+tok+"/ws", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()
	read := func() map[string]any {
		_, b, err := conn.Read(ctx)
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		var m map[string]any
		json.Unmarshal(b, &m)
		return m
	}
	m := read()
	items, _ := m["queue"].(map[string]any)["items"].([]any)
	if len(items) != 1 || m["version"] != "test" {
		t.Fatalf("first state: %v", m)
	}
	if s.Overlays() != 1 {
		t.Errorf("connected overlays: %d", s.Overlays())
	}
	if _, _, err := s.App.SimulateGift("giver", 10); err != nil {
		t.Fatal(err)
	}
	for {
		m = read()
		if feed, _ := m["feed"].([]any); len(feed) > 0 {
			e := feed[0].(map[string]any)
			if e["kind"] != "gift" || e["user"] == nil || e["pos"] != float64(1) || e["vars"] != nil || e["nick"] != nil {
				t.Errorf("pop-up: %v", e)
			}
			break
		}
	}

	// "New link": the overlay that is already open stops at once
	c = store.Get()
	c.Overlay.RemoteToken = config.NewOverlayToken()
	store.Set(c)
	for {
		if _, _, err := conn.Read(ctx); err != nil {
			if websocket.CloseStatus(err) != websocket.StatusPolicyViolation {
				t.Errorf("closed with %v", err)
			}
			break
		}
	}
	deadline := time.Now().Add(3 * time.Second)
	for s.Overlays() != 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if s.Overlays() != 0 {
		t.Errorf("connected overlays after closing: %d", s.Overlays())
	}
}

// TestOverlayStateWhitelist: the overlay link sends what overlay.html reads
// and nothing private.
func TestOverlayStateWhitelist(t *testing.T) {
	s, _, store, tok := overlayPanel(t)
	c := store.Get()
	c.Connection.EulerAPIKey = "euler-SECRET-key"
	c.Connection.TikFinityPath = `C:\Users\streamer\TikFinity.exe`
	c.Osu.ViewerURL = "https://viewer.example/"
	c.Rules.BlacklistUsers = []string{"banned-user"}
	c.Gifts.RequestCoins = 10
	if err := store.Set(c); err != nil {
		t.Fatal(err)
	}
	now := &queue.Item{ID: "n", User: "fan", Nick: "Fan Nick", SetID: 39804, Title: "FREEDOM DiVE", Artist: "xi", Creator: "Nakagawa-Kanon", Version: "FOUR DIMENSIONS",
		Stars: 7.1, Mode: "mania", Keys: 7, Convert: true, Cover: "https://assets.ppy.sh/c.jpg", Avatar: "https://p16.tiktokcdn.com/a.jpg", Mods: "HD",
		File: `C:\Users\streamer\cache\39804.osz`, Error: "secret error", Source: "catboy", AddedAt: time.Now()}
	s.App.Queue.Add(now)
	s.App.Queue.Promote("n", "played")
	s.App.Queue.Add(&queue.Item{ID: "b", User: "kid", Title: "B", Artist: "y", Stars: 3, Mode: "taiko", Keys: 4, Convert: true, Priority: true, Mods: "DT",
		File: `C:\Users\streamer\cache\2.osz`, AddedAt: time.Now()})
	s.App.SimulateGift("giver", 10)
	b, _ := json.Marshal(s.overlayState())
	out := string(b)
	for _, bad := range []string{tok, "482915", "euler-SECRET-key", `streamer`, "viewer.example", "banned-user", "secret error", "Fan Nick", "catboy",
		`"raw"`, `"firstRaw"`, `"extra"`, `"tts"`, `"speech"`, `"pin"`, `"remoteToken"`, `"remote"`, `"eulerApiKey"`, `"viewerUrl"`, `"blacklistUsers"`,
		`"file"`, `"attempts"`, `"nick"`, `"vars"`, `"notices"`, `"mirrors"`, `"update"`, `"history"`, `"rejected":`, `"at"`} {
		if strings.Contains(out, bad) {
			t.Errorf("the overlay link sends %s: %s", bad, out)
		}
	}
	var m map[string]any
	json.Unmarshal(b, &m)
	page, err := os.ReadFile(filepath.Join("..", "server", "web", "overlay.html"))
	if err != nil {
		t.Fatal(err)
	}
	// every S.<field> and st.version the page reads, and every overlay setting (o.<field>)
	for _, k := range regexp.MustCompile(`\bS\.(\w+)`).FindAllStringSubmatch(string(page), -1) {
		if _, ok := m[k[1]]; !ok {
			t.Errorf("overlay.html reads S.%s, which is not sent", k[1])
		}
	}
	if m["version"] != "test" {
		t.Errorf("version: %v", m["version"])
	}
	o, _ := m["overlay"].(map[string]any)
	for _, k := range regexp.MustCompile(`\bo\.(\w+)`).FindAllStringSubmatch(string(page), -1) {
		if _, ok := o[k[1]]; !ok {
			t.Errorf("overlay.html reads o.%s, which is not sent", k[1])
		}
	}
	has := func(what string, obj any, keys ...string) {
		mm, _ := obj.(map[string]any)
		for _, k := range keys {
			if _, ok := mm[k]; !ok {
				t.Errorf("%s lacks %s: %v", what, k, mm)
			}
		}
	}
	q := m["queue"].(map[string]any)
	has("now playing", q["now"], "id", "user", "title", "artist", "version", "creator", "stars", "mode", "keys", "convert", "cover", "setId", "avatar", "mods")
	items := q["items"].([]any)
	has("upcoming", items[0], "id", "user", "title", "artist", "stars", "mode", "keys", "convert", "priority", "mods")
	has("tikfinity", m["tikfinity"], "level")
	has("summary", m["summary"], "played", "passed", "viewers")
	feed := m["feed"].([]any)
	has("pop-up", feed[0], "id", "kind", "user", "code", "short", "pos")
	// the rest of a pop-up's fields, when set
	e, _ := json.Marshal(OverlayEvent{ID: 1, Kind: "added", User: "u", Code: "c", Short: "s", Title: "t", Artist: "a", Version: "v", Stars: 1, Mods: "HD", Pos: 2, Target: "x", Avatar: "https://p"})
	var em map[string]any
	json.Unmarshal(e, &em)
	has("pop-up", em, "title", "artist", "version", "stars", "mods", "target", "avatar")
	sb, _ := json.Marshal(OverlaySummary{Best: &OverlayBest{Title: "t", User: "u", Play: &OverlayPlay{Accuracy: 99, Rank: "S", PP: 300}}, Top: []queue.UserCount{{User: "u", Count: 2}}})
	var sm map[string]any
	json.Unmarshal(sb, &sm)
	has("summary", sm, "best", "top")
	has("best play", sm["best"], "title", "user", "play")
	has("best play", sm["best"].(map[string]any)["play"], "accuracy", "rank", "pp")

	// only the upcoming maps the overlay shows
	c.Overlay.Visible = 0
	store.Set(c)
	if st := s.overlayState(); len(st.Queue.Items) != 0 {
		t.Errorf("overlay.visible = 0 still sends %d maps", len(st.Queue.Items))
	}
}
