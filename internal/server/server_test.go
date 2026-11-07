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
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/app"
	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/config"
	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/downloader"
	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/osumeta"
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
	for _, p := range []string{"/", "/overlay", "/assets/app.js", "/assets/app.css", "/assets/bootstrap.min.css", "/assets/bootstrap.bundle.min.js"} {
		if w := do(h, http.MethodGet, p, ""); w.Code != http.StatusOK || w.Body.Len() == 0 {
			t.Errorf("%s: got %d (%d bytes)", p, w.Code, w.Body.Len())
		}
	}
}

// The dashboard loads app.js as a module, which browsers only run with a
// JavaScript type; the files must not depend on what the PC registers for .js.
func TestAssetTypes(t *testing.T) {
	_, h := newTestServer(t)
	for p, want := range map[string]string{
		"/assets/app.js":                  "text/javascript",
		"/assets/bootstrap.bundle.min.js": "text/javascript",
		"/assets/app.css":                 "text/css",
		"/assets/bootstrap.min.css":       "text/css",
		"/assets/icon.svg":                "image/svg+xml",
	} {
		if got := do(h, http.MethodGet, p, "").Header().Get("Content-Type"); !strings.HasPrefix(got, want) {
			t.Errorf("%s: Content-Type %q, want %s", p, got, want)
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

	pre := s.App.Cfg.Get().Import.Preload
	if m := decode(t, do(h, http.MethodPost, "/api/action/toggleAutoDownload", "")); m["ok"] != true {
		t.Fatalf("toggleAutoDownload: %v", m)
	}
	if s.App.Cfg.Get().Import.Preload == pre {
		t.Error("toggleAutoDownload did not toggle")
	}
	if st := decode(t, do(h, http.MethodGet, "/api/state", "")); st["autoDownload"] != !pre {
		t.Errorf("state.autoDownload = %v, want %v", st["autoDownload"], !pre)
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

// oszBytes is a tiny .osz; the name tells the cache files apart.
func oszBytes(name string) []byte {
	var b bytes.Buffer
	zw := zip.NewWriter(&b)
	f, _ := zw.Create(name + ".osu")
	f.Write([]byte("osu file format v14\n"))
	zw.Close()
	return b.Bytes()
}

func TestOszFromCache(t *testing.T) {
	s, h := newTestServer(t)
	dl := s.App.DL
	os.MkdirAll(dl.CacheDir, 0o755)
	noVid, withVid := oszBytes("novideo"), oszBytes("video")
	os.WriteFile(filepath.Join(dl.CacheDir, "123n.osz"), noVid, 0o644)
	os.WriteFile(filepath.Join(dl.CacheDir, "123.osz"), withVid, 0o644)
	os.WriteFile(filepath.Join(dl.CacheDir, "456.osz"), withVid, 0o644) // never checked by this run

	if w := do(h, http.MethodGet, "/api/osz/123", ""); w.Code != http.StatusNotFound {
		t.Fatalf("a file not checked by this run (e.g. after a restart): %d, want 404", w.Code)
	}
	// a cache hit trusts the file; no-video is on by default, so 123n.osz
	if _, err := dl.Get(context.Background(), downloader.Job{SetID: 123, HasVideo: true}, nil); err != nil {
		t.Fatal(err)
	}
	check := func(path string, want []byte) {
		t.Helper()
		w := do(h, http.MethodGet, path, "")
		if w.Code != http.StatusOK || !bytes.Equal(w.Body.Bytes(), want) {
			t.Fatalf("%s: %d, %d bytes (want %d)", path, w.Code, w.Body.Len(), len(want))
		}
		hd := w.Header()
		if hd.Get("Content-Type") != "application/x-osu-beatmap-archive" || hd.Get("Cache-Control") != "no-store" || hd.Get("Content-Length") != fmt.Sprint(len(want)) {
			t.Errorf("%s: headers %v", path, hd)
		}
		if _, ok := hd["Access-Control-Allow-Origin"]; ok {
			t.Errorf("%s: the cache is for the dashboard only, no CORS: %v", path, hd)
		}
	}
	check("/api/osz/123?novideo=1", noVid)
	check("/api/osz/123", noVid) // the video file is not trusted: the other one is used
	if _, err := dl.Get(context.Background(), downloader.Job{SetID: 123}, nil); err != nil {
		t.Fatal(err) // a set without video: 123.osz is checked and trusted now
	}
	check("/api/osz/123", withVid)
	check("/api/osz/123?novideo=1", noVid)

	w := do(h, http.MethodHead, "/api/osz/123?novideo=1", "")
	if w.Code != http.StatusOK || w.Body.Len() != 0 || w.Header().Get("Content-Length") != fmt.Sprint(len(noVid)) {
		t.Errorf("HEAD: %d, %d bytes, headers %v", w.Code, w.Body.Len(), w.Header())
	}
	if w := do(h, http.MethodHead, "/api/osz/123", "", func(r *http.Request) { r.Header.Del("X-Osureq") }); w.Code != http.StatusForbidden {
		t.Errorf("HEAD without X-Osureq: %d (the guard stays as it is)", w.Code)
	}
	if w := do(h, http.MethodGet, "/api/osz/123", "", func(r *http.Request) { r.Host = "evil.example:21299" }); w.Code != http.StatusForbidden {
		t.Errorf("foreign host: %d", w.Code)
	}
	for _, path := range []string{"/api/osz/456", "/api/osz/789", "/api/osz/456?novideo=1"} {
		if w := do(h, http.MethodGet, path, ""); w.Code != http.StatusNotFound {
			t.Errorf("%s: %d, want 404", path, w.Code)
		}
	}
	for _, path := range []string{"/api/osz/0", "/api/osz/-1", "/api/osz/abc", "/api/osz/1.5", "/api/osz/99999999999999999999", "/api/osz/..%2F123", "/api/osz/123%2F..%2F..%2Fconfig.json"} {
		if w := do(h, http.MethodGet, path, ""); w.Code != http.StatusBadRequest {
			t.Errorf("%s: %d, want 400", path, w.Code)
		}
	}
}

func TestKikiFlagsInState(t *testing.T) {
	s, h := newTestServer(t)
	st := decode(t, do(h, http.MethodGet, "/api/state", ""))
	if st["kikiPlayer"] != true || st["noVideo"] != true {
		t.Errorf("defaults: kikiPlayer=%v noVideo=%v", st["kikiPlayer"], st["noVideo"])
	}
	c := s.App.Cfg.Get()
	c.Osu.KikiPlayer, c.Download.NoVideo = false, false
	if err := s.App.Cfg.Set(c); err != nil {
		t.Fatal(err)
	}
	st = decode(t, do(h, http.MethodGet, "/api/state", ""))
	if st["kikiPlayer"] != false || st["noVideo"] != false {
		t.Errorf("after turning them off: kikiPlayer=%v noVideo=%v", st["kikiPlayer"], st["noVideo"])
	}
}

// osuPages answers osu!'s beatmapset pages for the sets in it (no network).
type osuPages map[int]float64 // set id -> stars

func (p osuPages) RoundTrip(r *http.Request) (*http.Response, error) {
	var id int
	fmt.Sscanf(r.URL.Path, "/beatmapsets/%d", &id)
	body, code := "", http.StatusNotFound
	if stars, ok := p[id]; ok && r.URL.Host == "osu.ppy.sh" {
		body = fmt.Sprintf(`<html><script id="json-beatmapset" type="application/json">{"id":%d,"title":"Song %d","artist":"xi","creator":"m","status":"ranked",
			"beatmaps":[{"id":%d,"version":"Hard","mode":"osu","difficulty_rating":%v,"total_length":200,"checksum":"aa","status":"ranked"}]}</script></html>`, id, id, id*10, stars)
		code = http.StatusOK
	}
	return &http.Response{StatusCode: code, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}, Request: r}, nil
}

func TestSimulateSeveralMaps(t *testing.T) {
	s, h := newTestServer(t)
	s.App.Meta = osumeta.New()
	s.App.Meta.MinGap = 0
	s.App.Meta.HTTP = &http.Client{Transport: osuPages{1: 2, 2: 2, 3: 2, 9: 9}}
	c := s.App.Cfg.Get()
	c.Command.BareNumberAs, c.Command.MaxPerMessage = "set", 3
	c.Rules.MaxStars = 5
	c.Queue.CooldownSec, c.Queue.PerUserLimit = 600, 0
	if err := s.App.Cfg.Set(c); err != nil {
		t.Fatal(err)
	}
	sim := func(user, msg string) map[string]any {
		b, _ := json.Marshal(map[string]string{"user": user, "message": msg})
		return decode(t, do(h, http.MethodPost, "/api/simulate", string(b)))
	}

	m := sim("fan", "!req 1 9 2 HDHR 3 1")
	item, _ := m["item"].(map[string]any)
	if m["ok"] != true || m["maps"] != float64(3) || m["count"] != float64(2) || m["rejected"] != float64(1) || item["setId"] != float64(1) {
		t.Errorf("several maps: %v", m)
	}
	if m["code"] != "stars_high" || m["short"] != "9.00★ > 5.00★" || m["ignored"] != float64(1) {
		t.Errorf("first rejection and the map past the limit: %v", m)
	}
	if n := s.App.Queue.Len(); n != 2 {
		t.Errorf("queue has %d, want 2", n)
	}

	// none added: a rejection as for one map, with the counts
	m = sim("fan", "!req 1 2")
	if m["ok"] != false || m["code"] != "cooldown" || m["count"] != float64(0) || m["maps"] != float64(2) || m["item"] != nil {
		t.Errorf("message in its cooldown: %v", m)
	}

	// one map: the same answer as before, without the counts
	m = sim("fan2", "!req 3")
	if item, _ := m["item"].(map[string]any); m["ok"] != true || item["setId"] != float64(3) || m["maps"] != nil || m["count"] != nil {
		t.Errorf("one map: %v", m)
	}
	m = sim("fan2", "!req 1")
	if m["ok"] != false || m["code"] != "cooldown" || m["maps"] != nil {
		t.Errorf("one map rejected: %v", m)
	}
}

// TestPutConfigReplacesBans pins down why the dashboard merges before saving
// (mergeEdits in app.js): PUT /api/config stores the body as it is, so a
// config read before a ban brings the old blacklist back.
func TestPutConfigReplacesBans(t *testing.T) {
	s, h := newTestServer(t)
	stale := s.App.Cfg.Get() // a form opened before the ban
	if m := decode(t, do(h, http.MethodPost, "/api/ban", `{"kind":"user","value":"troll"}`)); m["ok"] != true {
		t.Fatalf("ban: %v", m)
	}
	stale.Rules.MinStars = 3
	b, _ := json.Marshal(stale)
	if m := decode(t, do(h, http.MethodPut, "/api/config", string(b))); m["ok"] != true {
		t.Fatalf("save: %v", m)
	}
	if got := s.App.Cfg.Get().Rules.BlacklistUsers; len(got) != 0 {
		t.Errorf("PUT /api/config should store the body as sent (the dashboard keeps bans itself), got %v", got)
	}
	// what the dashboard sends instead: the config read at save time with its own change on top
	if m := decode(t, do(h, http.MethodPost, "/api/ban", `{"kind":"user","value":"troll"}`)); m["ok"] != true {
		t.Fatalf("ban: %v", m)
	}
	fresh := s.App.Cfg.Get()
	fresh.Rules.MinStars = 4
	b, _ = json.Marshal(fresh)
	decode(t, do(h, http.MethodPut, "/api/config", string(b)))
	if c := s.App.Cfg.Get(); c.Rules.MinStars != 4 || len(c.Rules.BlacklistUsers) != 1 || c.Rules.BlacklistUsers[0] != "troll" {
		t.Errorf("a save built on the config of that moment keeps the ban: %+v", c.Rules)
	}
}

func TestOverlayLinkToken(t *testing.T) {
	s, h := newTestServer(t)
	if tok := s.App.Cfg.Get().Overlay.RemoteToken; tok != "" {
		t.Fatalf("no secret before the link is switched on: %q", tok)
	}
	// switched on for the first time: saving makes the secret
	c := s.App.Cfg.Get()
	c.Overlay.Remote = true
	body, _ := json.Marshal(c)
	if m := decode(t, do(h, http.MethodPut, "/api/config", string(body))); m["ok"] != true {
		t.Fatalf("save: %v", m)
	}
	first := s.App.Cfg.Get().Overlay.RemoteToken
	if !config.GoodOverlayToken(first) {
		t.Fatalf("first token: %q", first)
	}

	if w := do(h, http.MethodPost, "/api/overlay/token", "", func(r *http.Request) { r.Header.Del("X-Osureq") }); w.Code != http.StatusForbidden {
		t.Errorf("without X-Osureq: %d", w.Code)
	}
	if w := do(h, http.MethodPost, "/api/overlay/token", "", func(r *http.Request) { r.Host = "evil.example:21299" }); w.Code != http.StatusForbidden {
		t.Errorf("foreign Host: %d", w.Code)
	}
	m := decode(t, do(h, http.MethodPost, "/api/overlay/token", ""))
	second, _ := m["token"].(string)
	if m["ok"] != true || second == first || second != s.App.Cfg.Get().Overlay.RemoteToken || !config.GoodOverlayToken(second) {
		t.Fatalf("new link: %v (saved %q, old %q)", m, s.App.Cfg.Get().Overlay.RemoteToken, first)
	}

	// a settings dialog opened before "New link" saves the old token back: ignored
	c.Overlay.RemoteToken = first
	c.Overlay.Theme = "sky"
	body, _ = json.Marshal(c)
	if m := decode(t, do(h, http.MethodPut, "/api/config", string(body))); m["ok"] != true {
		t.Fatalf("save: %v", m)
	}
	if got := s.App.Cfg.Get().Overlay; got.RemoteToken != second || got.Theme != "sky" {
		t.Errorf("only the token endpoint changes the link: %q (theme %s)", got.RemoteToken, got.Theme)
	}

	// the dashboard's state (pushed, /api/state, the support file) never has it
	if w := do(h, http.MethodGet, "/api/state", ""); strings.Contains(w.Body.String(), second) || !strings.Contains(w.Body.String(), `"remote":true`) {
		t.Errorf("state: %s", w.Body)
	}
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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, fmt.Sprintf("ws://127.0.0.1:%d/ws", p), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()
	if _, b, err := conn.Read(ctx); err != nil || strings.Contains(string(b), second) {
		t.Errorf("pushed state: %v %s", err, b)
	}
}

func TestSupportFileHidesOverlayLink(t *testing.T) {
	s, h := newTestServer(t)
	s.LogPath = filepath.Join(t.TempDir(), "osureq.log")
	c := s.App.Cfg.Get()
	c.Overlay.Remote = true
	c.Overlay.RemoteToken = config.NewOverlayToken()
	if err := s.App.Cfg.Set(c); err != nil {
		t.Fatal(err)
	}
	w := do(h, http.MethodPost, "/api/support", "")
	zr, err := zip.NewReader(bytes.NewReader(w.Body.Bytes()), int64(w.Body.Len()))
	if err != nil {
		t.Fatal(err)
	}
	hidden := false
	for _, f := range zr.File {
		rc, _ := f.Open()
		b, _ := io.ReadAll(rc)
		rc.Close()
		if strings.Contains(string(b), c.Overlay.RemoteToken) {
			t.Errorf("%s contains the overlay link", f.Name)
		}
		hidden = hidden || strings.Contains(string(b), `"remoteToken": "(hidden)"`)
	}
	if !hidden {
		t.Error("the config in the support file should say the token is hidden")
	}
	if s.App.Cfg.Get().Overlay.RemoteToken != c.Overlay.RemoteToken {
		t.Error("the support file must not change the saved token")
	}
}

func TestPreflightCountsRemoteOverlay(t *testing.T) {
	s, h := newTestServer(t)
	s.App.Meta = nil // no network in tests
	c := s.App.Cfg.Get()
	c.Download.Sources = nil
	s.App.Cfg.Set(c)
	n := 0
	s.RemoteOverlays = func() int { return n } // remote.Server.Overlays in main.go
	check := func() map[string]any {
		m := decode(t, do(h, http.MethodPost, "/api/preflight", ""))
		for _, c := range m["checks"].([]any) {
			if c := c.(map[string]any); c["id"] == "overlay" {
				return c
			}
		}
		t.Fatalf("no overlay check: %v", m)
		return nil
	}
	if c := check(); c["status"] != "warn" {
		t.Errorf("no overlay anywhere: %v", c)
	}
	n = 1
	if c := check(); c["status"] != "ok" || c["vars"].(map[string]any)["n"] != "1" {
		t.Errorf("an overlay on the other PC counts: %v", c)
	}
}
