// Package tosu reads osu!'s live game state from tosu's local WebSocket API
// (https://github.com/tosuapp/tosu). Auto play mode uses it to see when the
// requested map has actually been played; there is no timer fallback.
package tosu

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
)

const (
	ProcessName = "tosu.exe"
	DownloadURL = "https://github.com/tosuapp/tosu/releases/latest"
	DefaultURL  = "ws://127.0.0.1:24050/websocket/v2"
)

type Level string

const (
	Red    Level = "red"
	Yellow Level = "yellow"
	Green  Level = "green"
)

// Game is the part of tosu's v2 payload the helper uses.
type Game struct {
	Client    string `json:"client"` // stable | lazer
	State     string `json:"state"`  // menu, play, resultScreen, selectPlay, … (tosu GameState names)
	SetID     int    `json:"setId"`
	BeatmapID int    `json:"beatmapId"`
	Checksum  string `json:"checksum"`
	Failed    bool   `json:"failed"`
	Play      Score  `json:"play"`    // live numbers during gameplay
	Results   Score  `json:"results"` // the results screen (only meaningful in state resultScreen)
}

// Score is one play's numbers, from tosu's play or resultsScreen block.
type Score struct {
	Accuracy float64 `json:"accuracy"`
	Rank     string  `json:"rank"`
	MaxCombo int     `json:"maxCombo"`
	Score    int64   `json:"score"`
	PP       float64 `json:"pp"`
	Mods     string  `json:"mods"` // e.g. "HDHR", "" for no mods
	Misses   int     `json:"misses"`
}

// Brief is what the status pill needs; it changes far less often than Game.
type Brief struct {
	Client    string `json:"client"`
	State     string `json:"state"`
	SetID     int    `json:"setId"`
	BeatmapID int    `json:"beatmapId"`
}

func (g Game) Brief() Brief {
	return Brief{Client: g.Client, State: g.State, SetID: g.SetID, BeatmapID: g.BeatmapID}
}

type Status struct {
	Level     Level  `json:"level"`
	Code      string `json:"code"` // not_running | connecting | no_game | ok
	Running   bool   `json:"running"`
	Path      string `json:"path"`
	Connected bool   `json:"connected"`
	Game      Brief  `json:"game"`
	Error     string `json:"error,omitempty"`
}

type Env struct {
	Running func(exe string) bool
	Path    func(exe string) (string, bool) // path of the running process
	Launch  func(path string) error
}

type Settings struct {
	URL        string
	Path       string
	Reconnect  time.Duration
	AutoLaunch bool
}

type Client struct {
	Env      Env
	Settings func() Settings
	OnGame   func(Game)   // every payload, already de-duplicated
	OnStatus func(Status) // when the status or the visible game state changes
	OnPath   func(string) // tosu.exe found running somewhere new

	mu         sync.Mutex
	st         Status
	lastLaunch time.Time
	kick       chan struct{}
}

func New(env Env, settings func() Settings) *Client {
	return &Client{Env: env, Settings: settings, kick: make(chan struct{}, 1)}
}

func (c *Client) Status() Status {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.st
}

// Connected reports whether game state is flowing from tosu.
func (c *Client) Connected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.st.Connected
}

func (c *Client) Reconnect() {
	select {
	case c.kick <- struct{}{}:
	default:
	}
}

type strErr string

func (e strErr) Error() string { return string(e) }

const ErrNoPath = strErr("tosu.exe location unknown: set it in Settings → Connection, or start tosu once")

func (c *Client) LaunchNow() error {
	p := c.path()
	if p == "" {
		return ErrNoPath
	}
	c.mu.Lock()
	c.lastLaunch = time.Now()
	c.mu.Unlock()
	err := c.Env.Launch(p)
	c.Reconnect()
	return err
}

func (c *Client) path() string {
	if p := c.Settings().Path; p != "" {
		return p
	}
	return c.Status().Path
}

func classify(s Status) (Level, string) {
	switch {
	case !s.Running && !s.Connected:
		return Red, "not_running"
	case !s.Connected:
		return Yellow, "connecting"
	case s.Game.State == "" || s.Game.State == "unknown":
		return Yellow, "no_game"
	}
	return Green, "ok"
}

func (c *Client) set(f func(*Status)) {
	c.mu.Lock()
	old := c.st
	f(&c.st)
	c.st.Level, c.st.Code = classify(c.st)
	st := c.st
	c.mu.Unlock()
	if c.OnStatus != nil && (old.Level != st.Level || old.Code != st.Code || old.Error != st.Error ||
		old.Running != st.Running || old.Path != st.Path || old.Game != st.Game) {
		c.OnStatus(st)
	}
}

// Run keeps the WebSocket connected until ctx ends.
func (c *Client) Run(ctx context.Context) {
	for ctx.Err() == nil {
		s := c.Settings()
		running := c.Env.Running(ProcessName)
		path := ""
		if running {
			if p, ok := c.Env.Path(ProcessName); ok {
				path = p
				if c.OnPath != nil && !strings.EqualFold(p, s.Path) {
					c.OnPath(p)
				}
			}
		}
		c.set(func(st *Status) {
			st.Running = running
			if path != "" {
				st.Path = path
			} else if st.Path == "" {
				st.Path = s.Path
			}
		})
		if !running && s.AutoLaunch {
			c.mu.Lock()
			due := time.Since(c.lastLaunch) > 60*time.Second
			if due {
				c.lastLaunch = time.Now()
			}
			c.mu.Unlock()
			if p := c.path(); due && p != "" {
				if err := c.Env.Launch(p); err != nil {
					c.set(func(st *Status) { st.Error = "auto-launch failed: " + err.Error() })
				}
			}
		}
		c.connect(ctx, s)
		select {
		case <-ctx.Done():
			return
		case <-c.kick:
		case <-time.After(s.Reconnect):
		}
	}
}

func (c *Client) connect(ctx context.Context, s Settings) {
	dctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	conn, _, err := websocket.Dial(dctx, s.URL, nil)
	cancel()
	if err != nil {
		c.set(func(st *Status) {
			st.Connected, st.Game = false, Brief{}
			if st.Running {
				st.Error = "WebSocket: " + err.Error()
			} else {
				st.Error = ""
			}
		})
		return
	}
	defer conn.CloseNow()
	conn.SetReadLimit(32 << 20) // the v2 payload includes leaderboards
	c.set(func(st *Status) { st.Connected, st.Error = true, "" })

	rctx, rcancel := context.WithCancel(ctx)
	defer rcancel()
	go func() {
		select {
		case <-rctx.Done():
		case <-c.kick:
			conn.Close(websocket.StatusNormalClosure, "reconnect")
		}
	}()
	var last Game
	for {
		_, data, err := conn.Read(rctx)
		if err != nil {
			c.set(func(st *Status) {
				st.Connected, st.Game = false, Brief{}
				if ctx.Err() == nil && websocket.CloseStatus(err) != websocket.StatusNormalClosure {
					st.Error = "disconnected: " + err.Error()
				}
			})
			return
		}
		g, ok := Parse(data)
		if !ok || g == last {
			continue
		}
		last = g
		if b := g.Brief(); b != c.Status().Game {
			c.set(func(st *Status) { st.Game = b })
		}
		if c.OnGame != nil {
			c.OnGame(g)
		}
	}
}

type v2 struct {
	Client string `json:"client"`
	State  struct {
		Number int    `json:"number"`
		Name   string `json:"name"`
	} `json:"state"`
	Beatmap struct {
		ID       int    `json:"id"`
		Set      int    `json:"set"`
		Checksum string `json:"checksum"`
	} `json:"beatmap"`
	Play struct {
		Failed   bool    `json:"failed"`
		Accuracy float64 `json:"accuracy"`
		Score    int64   `json:"score"`
		Combo    struct {
			Max int `json:"max"`
		} `json:"combo"`
		Rank struct {
			Current string `json:"current"`
		} `json:"rank"`
		PP struct {
			Current float64 `json:"current"`
		} `json:"pp"`
		Mods v2mods `json:"mods"`
		Hits v2hits `json:"hits"`
	} `json:"play"`
	Results struct {
		Accuracy float64 `json:"accuracy"`
		Score    int64   `json:"score"`
		MaxCombo int     `json:"maxCombo"`
		Rank     string  `json:"rank"`
		PP       struct {
			Current float64 `json:"current"`
		} `json:"pp"`
		Mods v2mods `json:"mods"`
		Hits v2hits `json:"hits"`
	} `json:"resultsScreen"`
	Error string `json:"error"`
}

type v2mods struct {
	Name string `json:"name"`
}

type v2hits struct {
	Miss int `json:"0"`
}

// Parse extracts the game state from a tosu /websocket/v2 message.
func Parse(b []byte) (Game, bool) {
	var m v2
	if json.Unmarshal(b, &m) != nil || m.Error != "" {
		return Game{}, false
	}
	name := m.State.Name
	if name == "" {
		name = stateName(m.State.Number)
	}
	p, r := m.Play, m.Results
	return Game{
		Client: m.Client, State: name, SetID: m.Beatmap.Set, BeatmapID: m.Beatmap.ID,
		Checksum: strings.ToLower(m.Beatmap.Checksum), Failed: p.Failed,
		Play: Score{Accuracy: p.Accuracy, Rank: p.Rank.Current, MaxCombo: p.Combo.Max, Score: p.Score,
			PP: p.PP.Current, Mods: p.Mods.Name, Misses: p.Hits.Miss},
		Results: Score{Accuracy: r.Accuracy, Rank: r.Rank, MaxCombo: r.MaxCombo, Score: r.Score,
			PP: r.PP.Current, Mods: r.Mods.Name, Misses: r.Hits.Miss},
	}, true
}

// tosu's GameState enum, used when a payload carries only the number.
var states = []string{"menu", "edit", "play", "exit", "selectEdit", "selectPlay", "selectDrawings", "resultScreen",
	"update", "busy", "unknown", "lobby", "matchSetup", "selectMulti", "rankingVs", "onlineSelection",
	"optionsOffsetWizard", "rankingTagCoop", "rankingTeam", "beatmapImport", "packageUpdater", "benchmark", "tourney", "charts"}

func stateName(n int) string {
	if n >= 0 && n < len(states) {
		return states[n]
	}
	return "unknown"
}

// Phase of the map being played, as seen through tosu.
type Phase string

const (
	Waiting Phase = "waiting" // opened, not started yet
	Playing Phase = "playing" // in gameplay (retries stay here)
	Results Phase = "results" // on the results screen
	Ended   Phase = "ended"   // back in song select / menu after playing
)

// Tracker decides when the requested map has been played. A map counts as
// played once tosu shows it in gameplay and the game then leaves gameplay and
// the results screen. Picking another map in song select does not count.
type Tracker struct {
	SetID int
	Phase Phase
	Since time.Time // when Phase was entered
}

// Reset starts tracking a new now-playing map.
func (t *Tracker) Reset(setID int, now time.Time) {
	*t = Tracker{SetID: setID, Phase: Waiting, Since: now}
}

// Update feeds a game state and reports whether the phase changed.
func (t *Tracker) Update(g Game, now time.Time) bool {
	if t.SetID == 0 {
		return false
	}
	switch g.State {
	case "", "busy", "unknown", "update":
		return false // transient readings while osu! loads; never end a map on them
	}
	next := t.Phase
	switch t.Phase {
	case Waiting:
		if g.State == "play" && g.SetID == t.SetID {
			next = Playing
		}
	case Playing:
		switch {
		case g.State == "play" && g.SetID == t.SetID:
		case g.State == "resultScreen":
			next = Results
		default: // song select, menu, or straight into another map
			next = Ended
		}
	case Results:
		switch g.State {
		case "resultScreen":
		case "play":
			if g.SetID == t.SetID {
				next = Playing // retry from the results screen
			} else {
				next = Ended
			}
		default:
			next = Ended
		}
	}
	if next == t.Phase {
		return false
	}
	t.Phase, t.Since = next, now
	return true
}
