package server

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/app"
	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/config"
	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/downloader"
	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/queue"
	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/tikfinity"
	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/tosu"
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
