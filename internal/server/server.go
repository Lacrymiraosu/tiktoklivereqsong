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

	"github.com/coder/websocket"
	"github.com/draft/osureq/internal/app"
	"github.com/draft/osureq/internal/config"
	"github.com/draft/osureq/internal/queue"
	"github.com/draft/osureq/internal/tikfinity"
	"github.com/draft/osureq/internal/tosu"
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
	Version   string                        `json:"version"`
	TikFinity tikfinity.Status              `json:"tikfinity"`
	Tosu      tosu.Status                   `json:"tosu"`
	AutoGap   int                           `json:"autoGap"`
	Queue     queue.Snapshot                `json:"queue"`
	Requests  bool                          `json:"requests"`
	Resolving int                           `json:"resolving"`
	Feed      []app.FeedEvent               `json:"feed"`
	Notices   []app.Notice                  `json:"notices"`
	CacheMB   float64                       `json:"cacheMb"`
	Overlay   config.Overlay                `json:"overlay"`
	Command   string                        `json:"command"`
	Lang      string                        `json:"lang"`
	PlayMode  string                        `json:"playMode"`
	DryRun    bool                          `json:"dryRun"`
	Raw       []tikfinity.RawEvent          `json:"raw,omitempty"`
	FirstRaw  map[string]tikfinity.RawEvent `json:"firstRaw,omitempty"`
	Extra     map[string]any                `json:"extra,omitempty"`
}

func (s *Server) state() State {
	cfg := s.App.Cfg.Get()
	raw, first := s.App.TF.Raw()
	st := State{
		Version: s.Version, TikFinity: s.App.TF.Status(), Tosu: s.App.Tosu.Status(), AutoGap: cfg.Queue.AutoGapSec, Queue: s.App.Queue.Snapshot(),
		Requests: cfg.Command.Enabled, Resolving: s.App.Resolving(), Notices: s.App.Notices(), Feed: s.App.Feed(),
		CacheMB: float64(s.App.DL.CacheSize()) / (1 << 20), Overlay: cfg.Overlay, Command: cfg.Command.Name,
		Lang: cfg.Language, PlayMode: cfg.Queue.PlayMode, DryRun: cfg.Test.DryRun, Raw: raw, FirstRaw: first,
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
		httpErr(w, http.StatusBadRequest, err.Error())
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
		a.Notify("info", "cache cleanup removed %d file(s)", n)
	case "quit":
		if s.Quit != nil {
			a.Notify("info", "quit from the dashboard")
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
		writeJSON(w, map[string]any{"ok": false, "code": rej.Code, "error": rej.Detail})
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
	it, rej, matched := s.App.SimulateChat(body.User, body.Message)
	s.Push()
	switch {
	case !matched:
		writeJSON(w, map[string]any{"ok": false, "code": "ignored", "error": "not a request: real chat would ignore this message"})
	case rej != nil:
		writeJSON(w, map[string]any{"ok": false, "code": rej.Code, "error": rej.Detail})
	case it == nil:
		writeJSON(w, map[string]any{"ok": true, "command": true}) // a chat command, not a request
	default:
		writeJSON(w, map[string]any{"ok": true, "item": it})
	}
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
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true})
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
