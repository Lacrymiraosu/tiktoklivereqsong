package server

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/app"
	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/config"
	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/downloader"
	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/queue"
	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/tikfinity"
	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/tosu"
	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/update"
)

const port = 21299

// newTestServer builds a Server over real config and queue stores in a temp
// folder. Nothing touches the network, TikFinity, tosu or osu!.
func newTestServer(t *testing.T) (*Server, http.Handler) {
	t.Helper()
	dir := t.TempDir()
	store, err := config.Load(filepath.Join(dir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	q, err := queue.Open(filepath.Join(dir, "queue.json"))
	if err != nil {
		t.Fatal(err)
	}
	noOpen := func(string) error { return errors.New("not in tests") }
	tf := tikfinity.New(tikfinity.Env{
		Find:    func(string) (string, bool) { return "", false },
		Running: func(string) bool { return false },
		Launch:  func(string) error { return nil },
		Close:   func(string) (int, error) { return 0, nil },
	}, func() tikfinity.Settings { return tikfinity.Settings{} })
	a := app.New(store, q, nil, downloader.New(store.Get, filepath.Join(dir, "cache"), noOpen), nil, tf, noOpen)
	a.Tosu = tosu.New(tosu.Env{}, func() tosu.Settings { return tosu.Settings{} })
	s := New(a, "test")
	return s, s.Handler(port)
}

// do sends a request as the dashboard would (right Host, X-Osureq header).
func do(h http.Handler, method, path, body string, edit ...func(*http.Request)) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Host = fmt.Sprintf("127.0.0.1:%d", port)
	if method != http.MethodGet {
		r.Header.Set("X-Osureq", "1")
	}
	for _, f := range edit {
		f(r)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func decode(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatalf("not JSON (%d): %s", w.Code, w.Body)
	}
	return m
}

func TestGuardRejectsForeignHost(t *testing.T) {
	_, h := newTestServer(t)
	for _, host := range []string{"evil.example:21299", "127.0.0.1:1", "192.168.1.5:21299", ""} {
		w := do(h, http.MethodGet, "/api/state", "", func(r *http.Request) { r.Host = host })
		if w.Code != http.StatusForbidden {
			t.Errorf("Host %q: got %d, want 403", host, w.Code)
		}
	}
	for _, host := range []string{"127.0.0.1:21299", "localhost:21299", "LOCALHOST:21299"} {
		w := do(h, http.MethodGet, "/api/state", "", func(r *http.Request) { r.Host = host })
		if w.Code != http.StatusOK {
			t.Errorf("Host %q: got %d, want 200", host, w.Code)
		}
	}
}

func TestGuardNeedsHeaderForChanges(t *testing.T) {
	s, h := newTestServer(t)
	before := s.App.Cfg.Get().Command.Enabled
	w := do(h, http.MethodPost, "/api/action/toggleRequests", "", func(r *http.Request) { r.Header.Del("X-Osureq") })
	if w.Code != http.StatusForbidden {
		t.Fatalf("POST without X-Osureq: got %d, want 403", w.Code)
	}
	if s.App.Cfg.Get().Command.Enabled != before {
		t.Fatal("a refused request must not change anything")
	}
	w = do(h, http.MethodPut, "/api/config", "{}", func(r *http.Request) { r.Header.Del("X-Osureq") })
	if w.Code != http.StatusForbidden {
		t.Fatalf("PUT without X-Osureq: got %d, want 403", w.Code)
	}
}

func TestStateAndHeaders(t *testing.T) {
	_, h := newTestServer(t)
	w := do(h, http.MethodGet, "/api/state", "")
	if w.Code != http.StatusOK {
		t.Fatalf("got %d", w.Code)
	}
	if w.Header().Get("X-Content-Type-Options") != "nosniff" || w.Header().Get("Referrer-Policy") != "no-referrer" {
		t.Errorf("missing security headers: %v", w.Header())
	}
	m := decode(t, w)
	if m["version"] != "test" || m["command"] != "!req" {
		t.Errorf("unexpected state: version=%v command=%v", m["version"], m["command"])
	}
}

func TestPagesServed(t *testing.T) {
	_, h := newTestServer(t)
	for _, p := range []string{"/", "/overlay", "/assets/app.js", "/assets/app.css"} {
		if w := do(h, http.MethodGet, p, ""); w.Code != http.StatusOK || w.Body.Len() == 0 {
			t.Errorf("%s: got %d (%d bytes)", p, w.Code, w.Body.Len())
		}
	}
}

func TestActions(t *testing.T) {
	s, h := newTestServer(t)
	if w := do(h, http.MethodPost, "/api/action/nope", ""); w.Code != http.StatusNotFound {
		t.Errorf("unknown action: got %d, want 404", w.Code)
	}

	on := s.App.Cfg.Get().Command.Enabled
	if m := decode(t, do(h, http.MethodPost, "/api/action/toggleRequests", "")); m["ok"] != true {
		t.Fatalf("toggleRequests: %v", m)
	}
	if s.App.Cfg.Get().Command.Enabled == on {
		t.Error("toggleRequests did not toggle")
	}

	// An app error comes back with its code so the dashboard can translate it.
	m := decode(t, do(h, http.MethodPost, "/api/action/next", ""))
	if m["ok"] != false || m["code"] != "queue_empty" {
		t.Errorf("next on an empty queue: %v", m)
	}
	m = decode(t, do(h, http.MethodPost, "/api/action/launchTikfinity", ""))
	if m["code"] != "tikfinity_missing" {
		t.Errorf("launching a missing TikFinity: %v", m)
	}
}

func TestQueueOps(t *testing.T) {
	s, h := newTestServer(t)
	now := time.Now()
	for i, id := range []string{"a", "b", "c"} {
		s.App.Queue.Add(&queue.Item{ID: id, User: "u" + id, SetID: i + 1, AddedAt: now.Add(time.Duration(i) * time.Second)})
	}
	if m := decode(t, do(h, http.MethodPost, "/api/queue/c/move", `{"to":0}`)); m["ok"] != true {
		t.Fatalf("move: %v", m)
	}
	if m := decode(t, do(h, http.MethodPost, "/api/queue/a/remove", "")); m["ok"] != true {
		t.Fatalf("remove: %v", m)
	}
	var got []string
	for _, it := range s.App.Queue.First(10) {
		got = append(got, it.ID)
	}
	if strings.Join(got, ",") != "c,b" {
		t.Errorf("queue after move+remove: %v", got)
	}
	if m := decode(t, do(h, http.MethodPost, "/api/queue/zzz/remove", "")); m["code"] != "not_found" {
		t.Errorf("removing a missing item: %v", m)
	}
	if w := do(h, http.MethodPost, "/api/queue/c/move", `not json`); w.Code != http.StatusBadRequest {
		t.Errorf("bad move body: got %d", w.Code)
	}
	if w := do(h, http.MethodPost, "/api/queue/c/explode", ""); w.Code != http.StatusNotFound {
		t.Errorf("unknown op: got %d", w.Code)
	}
}

func TestPutConfigValidates(t *testing.T) {
	s, h := newTestServer(t)
	c := s.App.Cfg.Get()
	c.Command.Name = "req" // no "!"
	b, _ := json.Marshal(c)
	w := do(h, http.MethodPut, "/api/config", string(b))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("invalid config: got %d", w.Code)
	}
	if m := decode(t, w); m["code"] != "command_name" {
		t.Errorf("want code command_name: %v", m)
	}
	if s.App.Cfg.Get().Command.Name != "!req" {
		t.Error("an invalid config must not be stored")
	}

	c.Command.Name = "!song"
	b, _ = json.Marshal(c)
	if m := decode(t, do(h, http.MethodPut, "/api/config", string(b))); m["ok"] != true || m["restartNeeded"] != false {
		t.Errorf("valid config: %v", m)
	}
	if s.App.Cfg.Get().Command.Name != "!song" {
		t.Error("valid config was not stored")
	}
}

func TestSimulateAndBan(t *testing.T) {
	s, h := newTestServer(t)
	if m := decode(t, do(h, http.MethodPost, "/api/simulate", `{"user":"fan","message":"hello there"}`)); m["code"] != "ignored" {
		t.Errorf("plain chat should be ignored: %v", m)
	}
	s.App.Queue.Add(&queue.Item{ID: "x", User: "troll", SetID: 1, AddedAt: time.Now()})
	m := decode(t, do(h, http.MethodPost, "/api/ban", `{"kind":"user","value":"@troll"}`))
	if m["ok"] != true || m["removed"] != float64(1) {
		t.Errorf("ban: %v", m)
	}
	if m := decode(t, do(h, http.MethodPost, "/api/ban", `{"kind":"set","value":"abc"}`)); m["code"] != "ban_set_id" {
		t.Errorf("bad set id: %v", m)
	}
	if w := do(h, http.MethodPost, "/api/simulateGift", `{"user":"fan","coins":0}`); w.Code != http.StatusBadRequest {
		t.Errorf("gift without coins: got %d", w.Code)
	}
}

func TestErrBody(t *testing.T) {
	cases := []struct {
		err  error
		code any
	}{
		{errors.New("plain"), nil},
		{fmt.Errorf("wrapped: %w", tikfinity.ErrNotInstalled), "tikfinity_missing"},
		{tosu.ErrNoPath, "tosu_no_path"},
		{&config.ValidationError{Problems: []config.Problem{{Code: "game", Msg: "bad game"}}}, "game"},
	}
	for _, c := range cases {
		b := errBody(c.err)
		if b["ok"] != false || b["error"] != c.err.Error() || b["code"] != c.code {
			t.Errorf("%v: %v", c.err, b)
		}
	}
}

func TestPreflightEndpoint(t *testing.T) {
	s, h := newTestServer(t)
	s.App.Meta = nil // no network in tests
	c := s.App.Cfg.Get()
	c.Download.Sources = nil
	s.App.Cfg.Set(c)
	s.Extra = func() map[string]any { return map[string]any{"hotkeyErrors": map[string]string{}} }
	m := decode(t, do(h, http.MethodPost, "/api/preflight", ""))
	checks, _ := m["checks"].([]any)
	if m["ok"] != true || len(checks) == 0 {
		t.Fatalf("preflight: %v", m)
	}
	ids := map[string]string{}
	for _, c := range checks {
		c := c.(map[string]any)
		ids[c["id"].(string)] = c["status"].(string)
	}
	if ids["mirror"] != "fail" || ids["overlay"] != "warn" || ids["tikfinity"] != "fail" {
		t.Errorf("checks: %v", ids)
	}
}

func TestSupportFile(t *testing.T) {
	s, h := newTestServer(t)
	s.LogPath = filepath.Join(t.TempDir(), "osureq.log")
	c := s.App.Cfg.Get()
	c.Remote = config.Remote{Enabled: true, Port: 21281, PIN: "482915"}
	if err := s.App.Cfg.Set(c); err != nil {
		t.Fatal(err)
	}
	if w := do(h, http.MethodPost, "/api/support", "", func(r *http.Request) { r.Header.Del("X-Osureq") }); w.Code != http.StatusForbidden {
		t.Errorf("without X-Osureq: %d", w.Code)
	}
	w := do(h, http.MethodPost, "/api/support", "")
	if w.Code != http.StatusOK || w.Header().Get("Content-Type") != "application/zip" || !strings.HasPrefix(w.Body.String(), "PK") {
		t.Fatalf("got %d %s", w.Code, w.Header())
	}
	zr, err := zip.NewReader(bytes.NewReader(w.Body.Bytes()), int64(w.Body.Len()))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range zr.File {
		rc, _ := f.Open()
		b, _ := io.ReadAll(rc)
		rc.Close()
		if strings.Contains(string(b), "482915") {
			t.Errorf("%s contains the mod panel PIN", f.Name)
		}
	}
}

func TestTTSProxy(t *testing.T) {
	voice := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/mpeg")
		w.Write([]byte("mp3:" + r.URL.Query().Get("q")))
	}))
	defer voice.Close()
	s, h := newTestServer(t)
	s.Voice.URL = voice.URL

	w := do(h, http.MethodPost, "/api/tts", `{"lang":"th","text":"สวัสดีครับ"}`)
	if w.Code != http.StatusOK || w.Header().Get("Content-Type") != "audio/mpeg" || w.Body.String() != "mp3:สวัสดีครับ" {
		t.Fatalf("got %d %q %q", w.Code, w.Header().Get("Content-Type"), w.Body)
	}
	if w := do(h, http.MethodPost, "/api/tts", `{"lang":"xx","text":"hi"}`); w.Code != http.StatusBadGateway {
		t.Errorf("unknown language: got %d", w.Code)
	}
	w = do(h, http.MethodPost, "/api/tts", `{"lang":"th","text":"hi"}`, func(r *http.Request) { r.Header.Del("X-Osureq") })
	if w.Code != http.StatusForbidden {
		t.Errorf("without X-Osureq another site could use it: got %d", w.Code)
	}
}

// One dashboard page reads TTS: the last one to start it. Closing it frees
// the voice; switching TTS off keeps it, so the other pages stay quiet.
func TestTTSSpeaker(t *testing.T) {
	s, h := newTestServer(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	p := ln.Addr().(*net.TCPAddr).Port
	srv := httptest.NewUnstartedServer(s.Handler(p))
	srv.Listener.Close()
	srv.Listener = ln
	srv.Start()
	defer srv.Close()
	state := func() State {
		var st State
		json.Unmarshal(do(h, http.MethodGet, "/api/state", "").Body.Bytes(), &st)
		return st
	}
	open := func(page string) *websocket.Conn {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		c, _, err := websocket.Dial(ctx, fmt.Sprintf("ws://127.0.0.1:%d/ws?page=%s", p, page), nil)
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	claim := func(page string, on bool) {
		t.Helper()
		if w := do(h, http.MethodPost, "/api/tts/speaker", fmt.Sprintf(`{"page":%q,"on":%v}`, page, on)); w.Code != http.StatusOK {
			t.Fatalf("claim: %d %s", w.Code, w.Body)
		}
	}
	a, b := open("a"), open("b")
	defer b.CloseNow()
	waitPages := func(n int) {
		t.Helper()
		for i := 0; i < 300; i++ {
			s.mu.Lock()
			got := len(s.pages)
			s.mu.Unlock()
			if got == n {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatalf("pages never reached %d", n)
	}
	waitPages(2)
	if st := state(); st.TTSSpeaker != "" {
		t.Fatalf("speaker before anyone started TTS: %q", st.TTSSpeaker)
	}
	claim("a", true)
	claim("b", true)
	if st := state(); st.TTSSpeaker != "b" || !st.TTSSpeakerOn {
		t.Fatalf("the last page to start should speak: %+v", st.TTSSpeaker)
	}
	claim("a", false) // a page that does not speak switching off changes nothing
	if st := state(); st.TTSSpeaker != "b" || !st.TTSSpeakerOn {
		t.Fatalf("after a quiet page switched off: %q %v", st.TTSSpeaker, st.TTSSpeakerOn)
	}
	claim("a", true)
	b.Close(websocket.StatusNormalClosure, "") // closing a page that does not speak changes nothing
	waitPages(1)
	if st := state(); st.TTSSpeaker != "a" {
		t.Fatalf("after closing the quiet page: %q", st.TTSSpeaker)
	}
	claim("a", false)
	if st := state(); st.TTSSpeaker != "a" || st.TTSSpeakerOn {
		t.Fatalf("switched off: %q %v", st.TTSSpeaker, st.TTSSpeakerOn)
	}
	a.Close(websocket.StatusNormalClosure, "")
	waitPages(0)
	if st := state(); st.TTSSpeaker != "" || st.TTSSpeakerOn {
		t.Fatalf("closing the speaking page should free the voice: %q", st.TTSSpeaker)
	}
	for _, body := range []string{`{"on":true}`, `{"page":"` + strings.Repeat("x", 65) + `","on":true}`, `nope`} {
		if w := do(h, http.MethodPost, "/api/tts/speaker", body); w.Code != http.StatusBadRequest {
			t.Errorf("%s: got %d", body, w.Code)
		}
	}
}

func TestInstallUpdate(t *testing.T) {
	s, h := newTestServer(t)
	restarted := make(chan bool, 1)
	s.Restart = func() { restarted <- true }

	if m := decode(t, do(h, http.MethodPost, "/api/action/installUpdate", "")); m["ok"] != false {
		t.Errorf("no update yet: %v", m)
	}
	s.App.SetUpdate(update.Release{Version: "9.0.0", URL: update.ReleasesURL + "tag/v9.0.0"})
	if m := decode(t, do(h, http.MethodPost, "/api/action/installUpdate", "")); m["ok"] != false {
		t.Errorf("found but not downloaded: %v", m)
	}
	s.App.SetUpdate(update.Release{Version: "9.0.0", URL: update.ReleasesURL + "tag/v9.0.0", Ready: true})
	s.App.SetUpdate(update.Release{Version: "9.0.0", URL: update.ReleasesURL + "tag/v9.0.0"}) // the next check
	st := decode(t, do(h, http.MethodGet, "/api/state", ""))
	if u, _ := st["update"].(map[string]any); u["ready"] != true {
		t.Fatalf("state should say the update is ready: %v", st["update"])
	}
	if m := decode(t, do(h, http.MethodPost, "/api/action/installUpdate", "")); m["ok"] != true {
		t.Fatalf("ready: %v", m)
	}
	select {
	case <-restarted:
	case <-time.After(3 * time.Second):
		t.Error("Restart was not called")
	}
}

func TestPreviewAndMirrorsInState(t *testing.T) {
	s, h := newTestServer(t)
	s.App.AskPreview(39804)
	st := decode(t, do(h, http.MethodGet, "/api/state", ""))
	if ask, _ := st["previewAsk"].(map[string]any); ask["setId"] != float64(39804) || ask["n"] != float64(1) {
		t.Errorf("the dashboard should see the mod panel's preview press: %v", st["previewAsk"])
	}
	if ms, _ := st["mirrors"].([]any); len(ms) != 6 || ms[0].(map[string]any)["name"] != "hinai" || ms[0].(map[string]any)["state"] != "unknown" {
		t.Errorf("mirror panel, before any check: %v", st["mirrors"])
	}

	if m := decode(t, do(h, http.MethodPut, "/api/preview", `{"setId":39804}`)); m["ok"] != true || s.App.Previewing() != 39804 {
		t.Errorf("preview report: %v, previewing %d", m, s.App.Previewing())
	}
	if w := do(h, http.MethodPut, "/api/preview", `{"setId":-1}`); w.Code != http.StatusBadRequest {
		t.Errorf("a negative set: %d", w.Code)
	}
	if w := do(h, http.MethodPut, "/api/preview", `{"setId":1}`, func(r *http.Request) { r.Header.Del("X-Osureq") }); w.Code != http.StatusForbidden {
		t.Errorf("without X-Osureq: %d", w.Code)
	}
}
