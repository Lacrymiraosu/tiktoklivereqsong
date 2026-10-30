// Package remote is the mod panel: a small page for a phone on the same
// network, so a mod (or the streamer away from the PC) can run the queue.
// It is a second listener, off by default; the dashboard itself stays on
// 127.0.0.1. Access needs the PIN from Settings, comes only from private
// network addresses, and offers only queue actions (no settings, bans or quit).
package remote

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/app"
	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/config"
	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/queue"
)

//go:embed web/mod.html
var page []byte

const (
	cookieName  = "osureq_mod"
	sessionTTL  = 12 * time.Hour
	maxFailsIP  = 5  // wrong PINs from one address before it is locked out
	maxFailsAll = 20 // wrong PINs from everyone before the panel locks
	lockFor     = 10 * time.Minute
)

type session struct {
	expires time.Time
	pin     string // hash of the PIN it was opened with; a new PIN ends it
}

type fails struct {
	n     int
	since time.Time
}

type Server struct {
	App      *app.App
	Settings func() config.Remote

	mu       sync.Mutex
	sessions map[string]session
	failsIP  map[string]*fails
	failsAll fails
	srv      *http.Server
	running  config.Remote // what the listener was started with
	err      string        // last listen error, for the dashboard
	now      func() time.Time
}

func New(a *app.App, settings func() config.Remote) *Server {
	return &Server{App: a, Settings: settings, sessions: map[string]session{}, failsIP: map[string]*fails{}, now: time.Now}
}

// Status is what the dashboard shows about the mod panel.
type Status struct {
	Enabled  bool     `json:"enabled"`
	URLs     []string `json:"urls"` // one per LAN address of this PC
	Error    string   `json:"error,omitempty"`
	Sessions int      `json:"sessions"`
}

func (s *Server) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := Status{Enabled: s.srv != nil, Error: s.err}
	now := s.now()
	for _, x := range s.sessions {
		if now.Before(x.expires) {
			st.Sessions++
		}
	}
	if s.srv != nil {
		for _, ip := range lanIPs() {
			st.URLs = append(st.URLs, fmt.Sprintf("http://%s/m", net.JoinHostPort(ip, strconv.Itoa(s.running.Port))))
		}
	}
	return st
}

// Apply starts, restarts or stops the listener to match c.
func (s *Server) Apply(c config.Remote) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.srv != nil && c.Enabled && c.Port == s.running.Port {
		s.running = c
		return // PIN changes need no restart; old sessions end by themselves
	}
	if s.srv != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		s.srv.Shutdown(ctx)
		cancel()
		s.srv = nil
		log.Printf("mod panel stopped")
	}
	s.err, s.running = "", c
	if !c.Enabled {
		return
	}
	ln, err := net.Listen("tcp", net.JoinHostPort("", strconv.Itoa(c.Port)))
	if err != nil {
		s.err = err.Error()
		log.Printf("mod panel: %v", err)
		return
	}
	srv := &http.Server{Handler: s.Handler(), ReadHeaderTimeout: 10 * time.Second}
	s.srv = srv
	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("mod panel: %v", err)
		}
	}()
	log.Printf("mod panel on port %d (PIN required)", c.Port)
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/m", http.StatusFound) })
	mux.HandleFunc("GET /m", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.Write(page)
	})
	mux.HandleFunc("POST /m/login", s.login)
	mux.HandleFunc("POST /m/logout", s.logout)
	mux.HandleFunc("GET /m/api/state", s.auth(s.state))
	mux.HandleFunc("POST /m/api/action/{name}", s.auth(s.action))
	mux.HandleFunc("POST /m/api/queue/{id}/{op}", s.auth(s.queueOp))
	return s.guard(mux)
}

// guard admits only private-network clients that address this PC by IP,
// localhost, a .local name or its own name (a DNS name pointing here from
// another site, i.e. DNS rebinding, is refused), and requires the custom
// header on every change.
func (s *Server) guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !privateAddr(r.RemoteAddr) {
			http.Error(w, "only devices on your own network may use the mod panel", http.StatusForbidden)
			return
		}
		if !localHost(r.Host) {
			http.Error(w, "forbidden host", http.StatusForbidden)
			return
		}
		if r.Method != http.MethodGet && r.Header.Get("X-Osureq") != "1" {
			http.Error(w, "missing X-Osureq header", http.StatusForbidden)
			return
		}
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Content-Security-Policy", "default-src 'self'; script-src 'unsafe-inline'; style-src 'unsafe-inline' https://fonts.googleapis.com; font-src https://fonts.gstatic.com; img-src https: data:; connect-src 'self'; frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}

// privateAddr reports whether a client address is on a home or VPN network:
// loopback, RFC 1918, link-local, IPv6 ULA, or 100.64.0.0/10 (Tailscale).
func privateAddr(remote string) bool {
	host, _, err := net.SplitHostPort(remote)
	if err != nil {
		host = remote
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() {
		return true
	}
	_, cgnat, _ := net.ParseCIDR("100.64.0.0/10")
	return cgnat.Contains(ip)
}

func localHost(hostport string) bool {
	host := hostport
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		host = h
	}
	host = strings.ToLower(strings.Trim(host, "[]"))
	if host == "" {
		return false
	}
	if net.ParseIP(host) != nil || host == "localhost" || strings.HasSuffix(host, ".local") {
		return true
	}
	name, _ := os.Hostname()
	return name != "" && host == strings.ToLower(name)
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func hashPIN(pin string) string {
	sum := sha256.Sum256([]byte("osureq-mod-panel:" + pin))
	return hex.EncodeToString(sum[:])
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		PIN string `json:"pin"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 256)).Decode(&body) != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "code": "bad_request"})
		return
	}
	ip, now := clientIP(r), s.now()
	pin := s.Settings().PIN
	s.mu.Lock()
	f := s.failsIP[ip]
	if f != nil && now.Sub(f.since) > lockFor {
		delete(s.failsIP, ip)
		f = nil
	}
	if now.Sub(s.failsAll.since) > lockFor {
		s.failsAll = fails{}
	}
	if (f != nil && f.n >= maxFailsIP) || s.failsAll.n >= maxFailsAll {
		s.mu.Unlock()
		writeJSON(w, http.StatusTooManyRequests, map[string]any{"ok": false, "code": "locked", "minutes": int(lockFor.Minutes())})
		return
	}
	if pin == "" || subtle.ConstantTimeCompare([]byte(hashPIN(body.PIN)), []byte(hashPIN(pin))) != 1 {
		if f == nil {
			f = &fails{since: now}
			s.failsIP[ip] = f
		}
		f.n++
		if s.failsAll.n == 0 {
			s.failsAll.since = now
		}
		s.failsAll.n++
		s.mu.Unlock()
		log.Printf("mod panel: wrong PIN from %s", ip)
		writeJSON(w, http.StatusUnauthorized, map[string]any{"ok": false, "code": "wrong_pin"})
		return
	}
	delete(s.failsIP, ip)
	tok := make([]byte, 32)
	rand.Read(tok)
	token := hex.EncodeToString(tok)
	for k, x := range s.sessions { // forget expired sessions
		if now.After(x.expires) {
			delete(s.sessions, k)
		}
	}
	s.sessions[token] = session{expires: now.Add(sessionTTL), pin: hashPIN(pin)}
	s.mu.Unlock()
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: token, Path: "/m", MaxAge: int(sessionTTL.Seconds()), HttpOnly: true, SameSite: http.SameSiteStrictMode})
	s.App.Say("info", "modLogin", map[string]string{"ip": ip}, "mod panel: signed in from %s", ip)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(cookieName); err == nil {
		s.mu.Lock()
		delete(s.sessions, c.Value)
		s.mu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/m", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteStrictMode})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// auth lets a request through with a live session made with the current PIN.
func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(cookieName)
		ok := false
		if err == nil {
			s.mu.Lock()
			x, found := s.sessions[c.Value]
			ok = found && s.now().Before(x.expires) && x.pin == hashPIN(s.Settings().PIN)
			if found && !ok {
				delete(s.sessions, c.Value)
			}
			s.mu.Unlock()
		}
		if !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"ok": false, "code": "login"})
			return
		}
		next(w, r)
	}
}

// State is the part of the dashboard state a mod needs.
type State struct {
	Lang      string       `json:"lang"`
	Requests  bool         `json:"requests"`
	PlayMode  string       `json:"playMode"`
	TikFinity string       `json:"tikfinity"` // red | yellow | green
	Tosu      string       `json:"tosu"`
	Now       *Item        `json:"now"`
	Items     []Item       `json:"items"`
	VoteSkip  app.VoteSkip `json:"voteSkip"`
}

type Item struct {
	ID       string  `json:"id"`
	User     string  `json:"user"`
	Title    string  `json:"title"`
	Artist   string  `json:"artist"`
	Version  string  `json:"version"`
	Stars    float64 `json:"stars"`
	Mode     string  `json:"mode"`
	State    string  `json:"state"`
	Priority bool    `json:"priority,omitempty"`
	Phase    string  `json:"phase,omitempty"`
	SetID    int     `json:"setId"`
}

func (s *Server) state(w http.ResponseWriter, r *http.Request) {
	a, cfg := s.App, s.App.Cfg.Get()
	snap := a.Queue.Snapshot()
	st := State{Lang: cfg.Language, Requests: cfg.Command.Enabled, PlayMode: cfg.Queue.PlayMode,
		TikFinity: string(a.TF.Status().Level), Tosu: string(a.Tosu.Status().Level), VoteSkip: a.Votes(), Items: []Item{}}
	conv := func(it queue.Item) Item {
		return Item{ID: it.ID, User: it.User, Title: it.Title, Artist: it.Artist, Version: it.Version, Stars: it.Stars, Mode: it.Mode,
			State: string(it.State), Priority: it.Priority, Phase: it.PlayPhase, SetID: it.SetID}
	}
	if snap.Now != nil {
		n := conv(*snap.Now)
		st.Now = &n
	}
	for _, it := range snap.Items {
		st.Items = append(st.Items, conv(it))
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) action(w http.ResponseWriter, r *http.Request) {
	a, name := s.App, r.PathValue("name")
	var err error
	switch name {
	case "next":
		err = a.Next("played")
	case "skip":
		err = a.Skip("")
	case "toggleRequests":
		a.ToggleRequests()
	default:
		writeJSON(w, http.StatusNotFound, map[string]any{"ok": false, "code": "unknown"})
		return
	}
	s.note(r, name)
	reply(w, err)
}

func (s *Server) queueOp(w http.ResponseWriter, r *http.Request) {
	a, id, op := s.App, r.PathValue("id"), r.PathValue("op")
	var err error
	switch op {
	case "remove":
		err = a.Remove(id)
	case "skip":
		err = a.Skip(id)
	case "move":
		var body struct {
			To int `json:"to"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 256)).Decode(&body) != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "code": "bad_request"})
			return
		}
		if !a.Queue.Move(id, body.To) {
			err = errors.New("request not found")
		}
	default:
		writeJSON(w, http.StatusNotFound, map[string]any{"ok": false, "code": "unknown"})
		return
	}
	s.note(r, op)
	reply(w, err)
}

// note puts mod panel actions in Activity, so the streamer sees who did what.
func (s *Server) note(r *http.Request, what string) {
	ip := clientIP(r)
	s.App.Say("info", "modAction", map[string]string{"ip": ip, "action": what}, "mod panel (%s): %s", ip, what)
}

func reply(w http.ResponseWriter, err error) {
	if err == nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}
	body := map[string]any{"ok": false, "error": err.Error()}
	var coded interface {
		ErrCode() (string, map[string]string)
	}
	if errors.As(err, &coded) {
		body["code"], body["vars"] = coded.ErrCode()
	}
	writeJSON(w, http.StatusOK, body)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

// lanIPs lists this PC's IPv4 addresses on private networks.
func lanIPs() []string {
	var out []string
	ifs, _ := net.Interfaces()
	for _, ifc := range ifs {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := ifc.Addrs()
		for _, a := range addrs {
			if n, ok := a.(*net.IPNet); ok && n.IP.To4() != nil && privateAddr(n.IP.String()) && !n.IP.IsLinkLocalUnicast() {
				out = append(out, n.IP.String())
			}
		}
	}
	return out
}
