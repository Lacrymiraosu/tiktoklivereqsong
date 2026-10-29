// Package server serves the dashboard, the OBS overlay and a small JSON API
// on 127.0.0.1 only, and pushes state over WebSocket.
package server

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"io/fs"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/app"
	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/config"
	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/queue"
	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/tikfinity"
	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/tosu"
	"github.com/coder/websocket"
)

//go:embed web
var webFS embed.FS

type Server struct {
	App     *app.App
	Version string
	Extra   func() map[string]any // platform info (hotkey errors, paths)
	Quit    func()                // stops the helper (dashboard's Quit button)

	mu      sync.Mutex
	clients map[chan []byte]bool
	pushT   *time.Timer
}

func New(a *app.App, version string) *Server {
	return &Server{App: a, Version: version, clients: map[chan []byte]bool{}}
}

type State struct {
	Version        string                        `json:"version"`
	TikFinity      tikfinity.Status              `json:"tikfinity"`
	Tosu           tosu.Status                   `json:"tosu"`
	AutoGap        int                           `json:"autoGap"`
	Queue          queue.Snapshot                `json:"queue"`
	Requests       bool                          `json:"requests"`
	Resolving      int                           `json:"resolving"`
	Feed           []app.FeedEvent               `json:"feed"`
	Notices        []app.Notice                  `json:"notices"`
	CacheMB        float64                       `json:"cacheMb"`
	Overlay        config.Overlay                `json:"overlay"`
	Command        string                        `json:"command"`
	Lang           string                        `json:"lang"`
	PlayMode       string                        `json:"playMode"`
	ViewerURL      string                        `json:"viewerUrl"`
	Volume         int                           `json:"masterVolume"`
	BlacklistUsers []string                      `json:"blacklistUsers"`
	DryRun         bool                          `json:"dryRun"`
	Raw            []tikfinity.RawEvent          `json:"raw,omitempty"`
	FirstRaw       map[string]tikfinity.RawEvent `json:"firstRaw,omitempty"`
	Extra          map[string]any                `json:"extra,omitempty"`
}

func (s *Server) state() State {
	cfg := s.App.Cfg.Get()
	raw, first := s.App.TF.Raw()
	st := State{
		Version: s.Version, TikFinity: s.App.TF.Status(), Tosu: s.App.Tosu.Status(), AutoGap: cfg.Queue.AutoGapSec, Queue: s.App.Queue.Snapshot(),
		Requests: cfg.Command.Enabled, Resolving: s.App.Resolving(), Notices: s.App.Notices(), Feed: s.App.Feed(),
		CacheMB: float64(s.App.DL.CacheSize()) / (1 << 20), Overlay: cfg.Overlay, Command: cfg.Command.Name,
		Lang: cfg.Language, PlayMode: cfg.Queue.PlayMode, ViewerURL: cfg.Osu.ViewerURL, Volume: cfg.MasterVolume, BlacklistUsers: cfg.Rules.BlacklistUsers, DryRun: cfg.Test.DryRun, Raw: raw, FirstRaw: first,
	}
	if s.Extra != nil {
		st.Extra = s.Extra()
	}
	return st
}

// Push schedules a state broadcast (coalesced).
func (s *Server) Push() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pushT != nil {
		return
	}
	s.pushT = time.AfterFunc(120*time.Millisecond, func() {
		s.mu.Lock()
		s.pushT = nil
		s.mu.Unlock()
		b, _ := json.Marshal(s.state())
		s.mu.Lock()
		for c := range s.clients {
			select {
			case c <- b:
			default: // slow client; it gets the next one
			}
		}
		s.mu.Unlock()
	})
}

func (s *Server) Handler(port int) http.Handler {
	mux := http.NewServeMux()
	sub, _ := fs.Sub(webFS, "web")
	files := http.FileServerFS(sub)
	mux.Handle("GET /assets/", http.StripPrefix("/assets/", files))
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) { serveFile(w, sub, "index.html") })
	mux.HandleFunc("GET /overlay", func(w http.ResponseWriter, r *http.Request) { serveFile(w, sub, "overlay.html") })
	mux.HandleFunc("GET /api/state", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, s.state()) })
	mux.HandleFunc("GET /ws", s.ws)
	mux.HandleFunc("GET /api/config", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"config": s.App.Cfg.Get(), "defaults": config.Default()})
	})
	mux.HandleFunc("PUT /api/config", s.putConfig)
	mux.HandleFunc("POST /api/action/{name}", s.action)
	mux.HandleFunc("POST /api/queue/{id}/{op}", s.queueOp)
	mux.HandleFunc("POST /api/test", s.test)
	mux.HandleFunc("POST /api/simulate", s.simulate)
	mux.HandleFunc("POST /api/simulateGift", s.simulateGift)
	mux.HandleFunc("POST /api/ban", s.ban)
	mux.HandleFunc("POST /api/volume", s.volume)
	return guard(port, mux)
}

func serveFile(w http.ResponseWriter, sub fs.FS, name string) {
	b, err := fs.ReadFile(sub, name)
	if err != nil {
		http.NotFound(w, nil)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write(b)
}

// guard rejects requests whose Host is not this machine (DNS rebinding) and
// state-changing requests without the custom header, which a cross-site page
// cannot send without a CORS preflight we never answer.
func guard(port int, next http.Handler) http.Handler {
	p := strconv.Itoa(port)
	ok := map[string]bool{"127.0.0.1:" + p: true, "localhost:" + p: true}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !ok[strings.ToLower(r.Host)] {
			http.Error(w, "forbidden host", http.StatusForbidden)
			return
		}
		if r.Method != http.MethodGet && r.Header.Get("X-Osureq") != "1" {
			http.Error(w, "missing X-Osureq header", http.StatusForbidden)
			return
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) ws(w http.ResponseWriter, r *http.Request) {
	// Default AcceptOptions enforce Origin == Host, so other sites cannot connect.
	c, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer c.CloseNow()
	ch := make(chan []byte, 4)
	s.mu.Lock()
	s.clients[ch] = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.clients, ch)
		s.mu.Unlock()
	}()
	ctx := c.CloseRead(r.Context())
	first, _ := json.Marshal(s.state())
	if c.Write(ctx, websocket.MessageText, first) != nil {
		return
	}
	ping := time.NewTicker(20 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case b := <-ch:
			wctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			err := c.Write(wctx, websocket.MessageText, b)
			cancel()
			if err != nil {
				return
			}
		case <-ping.C:
			b, _ := json.Marshal(s.state())
			if c.Write(ctx, websocket.MessageText, b) != nil {
				return
			}
		}
	}
}

func (s *Server) putConfig(w http.ResponseWriter, r *http.Request) {
	var c config.Config
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	if err := dec.Decode(&c); err != nil {
		httpErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	old := s.App.Cfg.Get()
	if err := s.App.Cfg.Set(c); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(errBody(err))
		return
	}
	s.App.Wake()
	s.Push()
	restart := c.Connection.DashboardPort != old.Connection.DashboardPort
	writeJSON(w, map[string]any{"ok": true, "restartNeeded": restart})
}

func (s *Server) action(w http.ResponseWriter, r *http.Request) {
	a := s.App
	var err error
	switch r.PathValue("name") {
	case "next":
		err = a.Next("played")
	case "skip":
		err = a.Skip("")
	case "removeLast":
		err = a.RemoveLast()
	case "toggleRequests":
		a.ToggleRequests()
	case "clear":
		a.Queue.Clear()
	case "launchTosu":
		err = a.Tosu.LaunchNow()
	case "reconnectTosu":
		a.Tosu.Reconnect()
	case "openTosuDownload":
		err = a.OpenURL(tosu.DownloadURL)
	case "launchTikfinity":
		err = a.TF.LaunchNow()
	case "reconnect":
		a.TF.Reconnect()
	case "cleanup":
		cfg := a.Cfg.Get()
		n := a.DL.Cleanup(cfg.Download.CacheLimitMB, a.Queue.Files())
		a.Say("info", "cacheCleanup", map[string]string{"n": strconv.Itoa(n)}, "cache cleanup removed %d file(s)", n)
	case "stopTosu":
		err = s.stopApp("tosu", a.Tosu.StopNow)
	case "stopTikfinity":
		err = s.stopApp("TikFinity", a.TF.StopNow)
	case "quitAll":
		// close tosu and TikFinity, then osureq itself (even if one would not close)
		err = errors.Join(s.stopApp("tosu", a.Tosu.StopNow), s.stopApp("TikFinity", a.TF.StopNow))
		if s.Quit != nil {
			a.Say("info", "quitAll", nil, "quit everything from the dashboard")
			go func() { time.Sleep(300 * time.Millisecond); s.Quit() }()
		}
	case "quit":
		if s.Quit != nil {
			a.Say("info", "quit", nil, "quit from the dashboard")
			go func() { time.Sleep(300 * time.Millisecond); s.Quit() }()
		}
	case "toggleDryRun":
		a.ToggleDryRun()
	case "clearTest":
		a.ClearTest()
	case "openTikfinityDownload":
		err = a.OpenURL(tikfinity.DownloadURL)
	default:
		httpErr(w, http.StatusNotFound, "unknown action")
		return
	}
	s.Push()
	result(w, err)
}

// stopApp closes a helper app (tosu or TikFinity) and notes it in Activity.
func (s *Server) stopApp(name string, stop func() (int, error)) error {
	n, err := stop()
	switch {
	case err != nil:
		s.App.Say("error", "closeFailed", map[string]string{"app": name, "err": err.Error()}, "could not close %s: %v", name, err)
	case n == 0:
		s.App.Say("info", "closeNone", map[string]string{"app": name}, "%s was not running", name)
	default:
		s.App.Say("info", "closed", map[string]string{"app": name}, "closed %s", name)
	}
	return err
}

func (s *Server) queueOp(w http.ResponseWriter, r *http.Request) {
	a := s.App
	id := r.PathValue("id")
	var err error
	switch r.PathValue("op") {
	case "remove":
		err = a.Remove(id)
	case "skip":
		err = a.Skip(id)
	case "play":
		err = a.Play(id)
	case "retry":
		a.Retry(id)
	case "move":
		var body struct {
			To int `json:"to"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&body) != nil {
			httpErr(w, http.StatusBadRequest, "invalid body")
			return
		}
		if !a.Queue.Move(id, body.To) {
			err = errors.New("request not found")
		}
	default:
		httpErr(w, http.StatusNotFound, "unknown operation")
		return
	}
	s.Push()
	result(w, err)
}

func (s *Server) test(w http.ResponseWriter, r *http.Request) {
	var body struct {
		User  string `json:"user"`
		Input string `json:"input"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body) != nil {
		httpErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	it, rej := s.App.TestRequest(body.User, body.Input)
	s.Push()
	if rej != nil {
		writeJSON(w, map[string]any{"ok": false, "code": rej.Code, "short": rej.Short, "error": rej.Detail})
		return
	}
	writeJSON(w, map[string]any{"ok": true, "item": it})
}

// simulate runs a chat message through the real chat path.
func (s *Server) simulate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		User    string `json:"user"`
		Message string `json:"message"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body) != nil {
		httpErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	it, rej, kind := s.App.SimulateChat(body.User, body.Message)
	s.Push()
	switch {
	case kind == app.SimIgnored:
		writeJSON(w, map[string]any{"ok": false, "code": "ignored", "error": "not a request: real chat would ignore this message"})
	case kind != app.SimRequest: // a chat command: say whether it ran
		writeJSON(w, map[string]any{"ok": kind == app.SimCmdDone, "command": string(kind)})
	case rej != nil:
		writeJSON(w, map[string]any{"ok": false, "code": rej.Code, "short": rej.Short, "error": rej.Detail})
	default:
		writeJSON(w, map[string]any{"ok": true, "item": it})
	}
}

// volume sets the master volume: {"volume": 0-100}.
func (s *Server) volume(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Volume int `json:"volume"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 256)).Decode(&body) != nil {
		httpErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	err := s.App.SetVolume(body.Volume)
	s.Push()
	result(w, err)
}

// ban blacklists a user, beatmapset or mapper and drops matching requests.
func (s *Server) ban(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Kind  string `json:"kind"` // user | set | mapper
		Value string `json:"value"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&body) != nil {
		httpErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	n, err := s.App.Ban(body.Kind, body.Value)
	s.Push()
	if err != nil {
		result(w, err)
		return
	}
	writeJSON(w, map[string]any{"ok": true, "removed": n})
}

// simulateGift pretends a viewer sent a gift worth `coins`.
func (s *Server) simulateGift(w http.ResponseWriter, r *http.Request) {
	var body struct {
		User  string `json:"user"`
		Coins int    `json:"coins"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&body) != nil || body.Coins <= 0 {
		httpErr(w, http.StatusBadRequest, "send {user, coins > 0}")
		return
	}
	n, err := s.App.SimulateGift(body.User, body.Coins)
	s.Push()
	if err != nil {
		result(w, err)
		return
	}
	writeJSON(w, map[string]any{"ok": true, "earned": n})
}

func result(w http.ResponseWriter, err error) {
	if err != nil {
		writeJSON(w, errBody(err))
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

// errBody is an error response; errors with a code (app.UserError,
// config.ValidationError and a few known ones) can be translated by the
// dashboard as T["e_"+code] with vars, falling back to the English error.
func errBody(err error) map[string]any {
	body := map[string]any{"ok": false, "error": err.Error()}
	var coded interface {
		ErrCode() (string, map[string]string)
	}
	switch {
	case errors.As(err, &coded):
		body["code"], body["vars"] = coded.ErrCode()
	case errors.Is(err, tikfinity.ErrNotInstalled):
		body["code"] = "tikfinity_missing"
	case errors.Is(err, tosu.ErrNoPath):
		body["code"] = "tosu_no_path"
	}
	return body
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(v)
}

func httpErr(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": msg})
}

// Listen binds to 127.0.0.1 only.
func (s *Server) Listen(ctx context.Context, port int) error {
	ln, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		return err
	}
	srv := &http.Server{Handler: s.Handler(port), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		srv.Shutdown(sctx)
	}()
	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("dashboard server: %v", err)
		}
	}()
	return nil
}
