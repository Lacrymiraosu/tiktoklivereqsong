// Package remote is the mod panel: a small page for a phone on the same
// network, so a mod (or the streamer away from the PC) can run the queue.
// It is a second listener, off by default; the dashboard itself stays on
// 127.0.0.1. Access needs the PIN from Settings, comes only from private
// network addresses, and offers only queue actions (no settings, bans or quit).
package remote

import (
	"bytes"
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
	"path"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/app"
	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/config"
	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/queue"
	"github.com/coder/websocket"
)

//go:embed web/mod.html
var page []byte

const (
	cookieName  = "osureq_mod"
	sessionTTL  = 12 * time.Hour
	maxFailsIP  = 5  // wrong PINs from one address before it is locked out
	maxFailsAll = 20 // wrong PINs from everyone before the panel locks
	lockFor     = 10 * time.Minute

	overlayTick = 200 * time.Millisecond // how often the overlay link looks for changes
	keepAlive   = 20 * time.Second       // the state is sent at least this often
	maxRetired  = 4
	// overlayCSP is the overlay page's policy: its own inline script and
	// styles, Google Fonts, covers and avatars over https, and its WebSocket.
	overlayCSP = "default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline' https://fonts.googleapis.com; font-src https://fonts.gstatic.com; img-src https: data:; connect-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'"
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

	// The overlay link (overlay.remote): OBS on another PC loads the overlay
	// from this listener with a secret link. main.go hands over the page and
	// the version (the overlay reloads itself when osureq was updated).
	OverlayPage []byte
	Version     string
	overlays    int      // overlays connected through the link
	linkHash    string   // hash of the current link
	retired     []string // hashes of links replaced while running (see overlayOK)
}

func New(a *app.App, settings func() config.Remote) *Server {
	s := &Server{App: a, Settings: settings, sessions: map[string]session{}, failsIP: map[string]*fails{}, now: time.Now}
	if a != nil && a.Cfg != nil {
		s.linkHash = hashLink(a.Cfg.Get().Overlay.RemoteToken)
		a.Cfg.OnChange(func(c config.Config) {
			h := hashLink(c.Overlay.RemoteToken)
			s.mu.Lock()
			if h != s.linkHash {
				if s.linkHash != hashLink("") {
					s.retired = append(s.retired, s.linkHash)
					if len(s.retired) > maxRetired {
						s.retired = s.retired[1:]
					}
				}
				s.linkHash = h
			}
			s.mu.Unlock()
		})
	}
	return s
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
	// overlay link: read-only, nothing else (no other method) exists under /o/
	mux.HandleFunc("GET /o/{token}", s.overlayPage)
	mux.HandleFunc("GET /o/{token}/ws", s.overlayWS)
	mux.HandleFunc("/o/", func(w http.ResponseWriter, r *http.Request) { overlayHeaders(w); http.NotFound(w, r) })
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
		// The mux would redirect "//o/<link>" or "/o/./<link>" to the clean
		// path, putting the overlay link in a Location header: a plain 404.
		if c := path.Clean(r.URL.Path); c != r.URL.Path && (c == "/o" || strings.HasPrefix(c, "/o/")) {
			overlayHeaders(w)
			http.NotFound(w, r)
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
		h.Set("Content-Security-Policy", "default-src 'self'; script-src 'unsafe-inline'; style-src 'unsafe-inline' https://fonts.googleapis.com; font-src https://fonts.gstatic.com; img-src https: data:; connect-src 'self'; frame-src https://player.kiki-bm.app; frame-ancestors 'none'")
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
	if s.lockedIP(ip, now) || s.lockedAll(now) {
		s.mu.Unlock()
		writeJSON(w, http.StatusTooManyRequests, map[string]any{"ok": false, "code": "locked", "minutes": int(lockFor.Minutes())})
		return
	}
	if pin == "" || subtle.ConstantTimeCompare([]byte(hashPIN(body.PIN)), []byte(hashPIN(pin))) != 1 {
		s.failIP(ip, now)
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

// lockedIP reports whether ip has had too many wrong PINs or overlay links;
// the count is forgotten lockFor after its first failure. s.mu must be held.
func (s *Server) lockedIP(ip string, now time.Time) bool {
	f := s.failsIP[ip]
	if f != nil && now.Sub(f.since) > lockFor {
		delete(s.failsIP, ip)
		f = nil
	}
	return f != nil && f.n >= maxFailsIP
}

// lockedAll reports whether everyone together has had too many wrong PINs.
// s.mu must be held.
func (s *Server) lockedAll(now time.Time) bool {
	if now.Sub(s.failsAll.since) > lockFor {
		s.failsAll = fails{}
	}
	return s.failsAll.n >= maxFailsAll
}

// failIP counts a wrong PIN or overlay link from ip. s.mu must be held.
func (s *Server) failIP(ip string, now time.Time) {
	f := s.failsIP[ip]
	if f == nil {
		f = &fails{since: now}
		s.failsIP[ip] = f
	}
	f.n++
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
	TikFinity string       `json:"tikfinity"`  // red | yellow | green
	ChatFrom  string       `json:"chatSource"` // tikfinity | tiktok
	Viewers   int          `json:"viewers"`    // people watching the LIVE, 0 = unknown
	Tosu      string       `json:"tosu"`
	Now       *Item        `json:"now"`
	Items     []Item       `json:"items"`
	VoteSkip  app.VoteSkip `json:"voteSkip"`
	// Previewing is the set whose preview the dashboard on the stream PC
	// is playing (0 = none).
	Previewing int    `json:"previewing"`
	Prev       string `json:"prev,omitempty"` // "artist - title" the previous button returns to
	// KikiPlayer is osu.kikiPlayerMod: "▶ view map" opens KIKI's beatmap
	// player (straight from player.kiki-bm.app and its mirrors) under a row.
	KikiPlayer bool `json:"kikiPlayer"`
}

type Item struct {
	ID        string  `json:"id"`
	User      string  `json:"user"`
	Title     string  `json:"title"`
	Artist    string  `json:"artist"`
	Version   string  `json:"version"`
	Stars     float64 `json:"stars"`
	Mode      string  `json:"mode"`
	State     string  `json:"state"`
	Priority  bool    `json:"priority,omitempty"`
	Phase     string  `json:"phase,omitempty"`
	SetID     int     `json:"setId"`
	BeatmapID int     `json:"beatmapId,omitempty"` // the difficulty requested, for the map player
	Mods      string  `json:"mods,omitempty"`
}

func (s *Server) state(w http.ResponseWriter, r *http.Request) {
	a, cfg := s.App, s.App.Cfg.Get()
	snap := a.Queue.Snapshot()
	tf := a.TF.Status()
	st := State{Lang: cfg.Language, Requests: cfg.Command.Enabled, PlayMode: cfg.Queue.PlayMode,
		TikFinity: string(tf.Level), ChatFrom: tf.Source, Viewers: tf.Viewers, Tosu: string(a.Tosu.Status().Level), VoteSkip: a.Votes(), Previewing: a.Previewing(), KikiPlayer: cfg.Osu.KikiPlayerMod, Items: []Item{}}
	conv := func(it queue.Item) Item {
		return Item{ID: it.ID, User: it.User, Title: it.Title, Artist: it.Artist, Version: it.Version, Stars: it.Stars, Mode: it.Mode,
			State: string(it.State), Priority: it.Priority, Phase: it.PlayPhase, SetID: it.SetID, BeatmapID: it.BeatmapID, Mods: it.Mods}
	}
	if snap.Now != nil {
		n := conv(*snap.Now)
		st.Now = &n
	}
	st.Prev = snap.Prev
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
	case "previous":
		err = a.Previous()
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
	case "preview": // played (or stopped) by the dashboard on the stream PC
		it, ok := a.Queue.Get(id)
		if !ok {
			err = errors.New("request not found")
			break
		}
		a.AskPreview(it.SetID)
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

// Overlays counts overlays connected through the overlay link, for the
// dashboard's pre-stream check.
func (s *Server) Overlays() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.overlays
}

func hashLink(token string) string {
	sum := sha256.Sum256([]byte("osureq-overlay-link:" + token))
	return hex.EncodeToString(sum[:])
}

func overlayHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", overlayCSP)
}

// linkOn returns the overlay settings and whether the link is usable: on in
// Settings, with a secret, and the mod panel on too.
func (s *Server) linkOn() (config.Overlay, bool) {
	o := s.App.Cfg.Get().Overlay
	return o, o.Remote && o.RemoteToken != "" && s.Settings().Enabled
}

// rightLink compares a token with the secret in constant time (as hashes,
// so both sides have the same length).
func rightLink(token, secret string) bool {
	return subtle.ConstantTimeCompare([]byte(hashLink(token)), []byte(hashLink(secret))) == 1
}

// overlayOK checks a request for /o/{token}. Anything but the right token,
// while the link is on, gets the same 404 as a path that does not exist.
// A wrong token counts toward the same per-address lockout as a wrong PIN
// (not the panel-wide one, so guesses cannot black out the PIN login), and
// a locked address is refused even with the right token. A link replaced
// by "New link" while osureq runs is not counted: an OBS that still has it
// keeps retrying, and must not lock its PC out of the new link.
func (s *Server) overlayOK(r *http.Request) bool {
	o, on := s.linkOn()
	if !on {
		return false // nothing to guess at: not counted
	}
	ip, now, token := clientIP(r), s.now(), r.PathValue("token")
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lockedIP(ip, now) {
		return false
	}
	if rightLink(token, o.RemoteToken) {
		return true // the PIN count is kept: the link must not reset it
	}
	if slices.Contains(s.retired, hashLink(token)) {
		return false
	}
	s.failIP(ip, now)
	log.Printf("overlay link: wrong link from %s", ip)
	return false
}

func (s *Server) overlayPage(w http.ResponseWriter, r *http.Request) {
	overlayHeaders(w)
	if len(s.OverlayPage) == 0 || !s.overlayOK(r) {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(s.OverlayPage)
}

// overlayWS pushes the overlay's state like the dashboard's /ws: it looks
// for a change every overlayTick (the dashboard pushes 120 ms after one)
// and sends the state at least every keepAlive. It closes as soon as the
// link is switched off or replaced.
func (s *Server) overlayWS(w http.ResponseWriter, r *http.Request) {
	overlayHeaders(w)
	if !s.overlayOK(r) {
		http.NotFound(w, r)
		return
	}
	// Default AcceptOptions enforce Origin == Host, so other sites cannot connect.
	c, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer c.CloseNow()
	token := r.PathValue("token")
	s.mu.Lock()
	s.overlays++
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.overlays--
		s.mu.Unlock()
	}()
	log.Printf("overlay link: overlay connected from %s", clientIP(r))
	ctx := c.CloseRead(r.Context())
	tick := time.NewTicker(overlayTick)
	defer tick.Stop()
	var last []byte
	var sent time.Time
	for {
		if o, on := s.linkOn(); !on || !rightLink(token, o.RemoteToken) {
			c.Close(websocket.StatusPolicyViolation, "")
			return
		}
		b, _ := json.Marshal(s.overlayState())
		if !bytes.Equal(b, last) || time.Since(sent) >= keepAlive {
			wctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			err := c.Write(wctx, websocket.MessageText, b)
			cancel()
			if err != nil {
				return
			}
			last, sent = b, time.Now()
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

// OverlayState is everything the overlay link sends, and nothing more: the
// fields overlay.html reads (S.…), built field by field so nothing private
// (paths, raw events, settings, keys, the PIN or the link) can slip in.
type OverlayState struct {
	Version      string          `json:"version"` // the page reloads when osureq was updated
	Overlay      OverlaySettings `json:"overlay"`
	Queue        OverlayQueue    `json:"queue"`
	Feed         []OverlayEvent  `json:"feed"`
	Requests     bool            `json:"requests"`
	TikFinity    OverlayChat     `json:"tikfinity"`
	Command      string          `json:"command"`
	Search       bool            `json:"search"`
	RequestCoins int             `json:"requestCoins"`
	Summary      OverlaySummary  `json:"summary"`
	SummaryShown int64           `json:"summaryShown"`
}

// OverlaySettings is config.Overlay without the link.
type OverlaySettings struct {
	Visible     int    `json:"visible"`
	Requester   bool   `json:"showRequester"`
	Theme       string `json:"theme"`
	Scale       int    `json:"scale"`
	Position    string `json:"position"`
	Feedback    bool   `json:"feedback"`
	FeedbackSec int    `json:"feedbackSec"`
	Lang        string `json:"lang"`
	Avatars     bool   `json:"avatars"`
}

// OverlayChat says whether chat is connected (the "!req" line shows then).
type OverlayChat struct {
	Level string `json:"level"`
}

type OverlayQueue struct {
	Now   *OverlayNow   `json:"now"`
	Items []OverlayItem `json:"items"` // only the ones shown (overlay.visible)
}

type OverlayItem struct {
	ID       string  `json:"id"`
	User     string  `json:"user"`
	Title    string  `json:"title"`
	Artist   string  `json:"artist"`
	Stars    float64 `json:"stars"`
	Mode     string  `json:"mode"`
	Keys     int     `json:"keys,omitempty"`
	Convert  bool    `json:"convert,omitempty"`
	Priority bool    `json:"priority,omitempty"`
	Mods     string  `json:"mods,omitempty"`
}

// OverlayNow is the now-playing card.
type OverlayNow struct {
	OverlayItem
	Version string `json:"version"`
	Creator string `json:"creator"`
	Cover   string `json:"cover"`
	SetID   int    `json:"setId"`
	Avatar  string `json:"avatar,omitempty"`
}

// OverlayEvent is a pop-up (app.FeedEvent without Vars, nick, times, test).
type OverlayEvent struct {
	ID      int64   `json:"id"`
	Kind    string  `json:"kind"`
	User    string  `json:"user"`
	Code    string  `json:"code,omitempty"`
	Short   string  `json:"short,omitempty"`
	Title   string  `json:"title,omitempty"`
	Artist  string  `json:"artist,omitempty"`
	Version string  `json:"version,omitempty"`
	Stars   float64 `json:"stars,omitempty"`
	Mods    string  `json:"mods,omitempty"`
	Pos     int     `json:"pos,omitempty"`
	Target  string  `json:"target,omitempty"`
	Avatar  string  `json:"avatar,omitempty"`
}

// OverlaySummary is the end-of-stream card.
type OverlaySummary struct {
	Played  int               `json:"played"`
	Passed  int               `json:"passed"`
	Viewers int               `json:"viewers"`
	Best    *OverlayBest      `json:"best,omitempty"`
	Top     []queue.UserCount `json:"top,omitempty"`
}

type OverlayBest struct {
	Title string       `json:"title"`
	User  string       `json:"user"`
	Play  *OverlayPlay `json:"play,omitempty"`
}

type OverlayPlay struct {
	Accuracy float64 `json:"accuracy"`
	Rank     string  `json:"rank,omitempty"`
	PP       float64 `json:"pp,omitempty"`
}

func (s *Server) overlayState() OverlayState {
	a, cfg := s.App, s.App.Cfg.Get()
	o := cfg.Overlay
	st := OverlayState{Version: s.Version, Requests: cfg.Command.Enabled, Command: cfg.Command.Name, Search: cfg.Command.Search && a.Search != nil,
		RequestCoins: cfg.Gifts.RequestCoins, SummaryShown: a.SummaryShown(), Feed: []OverlayEvent{},
		Overlay: OverlaySettings{Visible: o.Visible, Requester: o.Requester, Theme: o.Theme, Scale: o.Scale, Position: o.Position,
			Feedback: o.Feedback, FeedbackSec: o.FeedbackSec, Lang: o.Lang, Avatars: o.Avatars},
		TikFinity: OverlayChat{Level: string(a.TF.Status().Level)}, Queue: OverlayQueue{Items: []OverlayItem{}}}
	item := func(it queue.Item) OverlayItem {
		return OverlayItem{ID: it.ID, User: it.User, Title: it.Title, Artist: it.Artist, Stars: it.Stars, Mode: it.Mode, Keys: it.Keys,
			Convert: it.Convert, Priority: it.Priority, Mods: it.Mods}
	}
	snap := a.Queue.Snapshot()
	if n := snap.Now; n != nil {
		st.Queue.Now = &OverlayNow{OverlayItem: item(*n), Version: n.Version, Creator: n.Creator, Cover: n.Cover, SetID: n.SetID, Avatar: n.Avatar}
	}
	for i, it := range snap.Items {
		if i >= o.Visible {
			break
		}
		st.Queue.Items = append(st.Queue.Items, item(it))
	}
	for _, e := range a.Feed() {
		st.Feed = append(st.Feed, OverlayEvent{ID: e.ID, Kind: e.Kind, User: e.User, Code: e.Code, Short: e.Short, Title: e.Title, Artist: e.Artist,
			Version: e.Version, Stars: e.Stars, Mods: e.Mods, Pos: e.Pos, Target: e.Target, Avatar: e.Avatar})
	}
	sum := a.Queue.Summary()
	st.Summary = OverlaySummary{Played: sum.Played, Passed: sum.Passed, Viewers: sum.Viewers, Top: sum.Top}
	if b := sum.Best; b != nil {
		st.Summary.Best = &OverlayBest{Title: b.Title, User: b.User}
		if p := b.Play; p != nil {
			st.Summary.Best.Play = &OverlayPlay{Accuracy: p.Accuracy, Rank: p.Rank, PP: p.PP}
		}
	}
	return st
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
