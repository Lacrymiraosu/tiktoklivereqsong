// Package config loads, validates and saves config.json next to the executable.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type Connection struct {
	TikFinityURL  string `json:"tikfinityUrl"`
	DashboardPort int    `json:"dashboardPort"`
	AutoLaunch    bool   `json:"autoLaunchTikfinity"`
	TikFinityPath string `json:"tikfinityPath"`
	ReconnectSec  int    `json:"reconnectSec"`
	OpenDashboard bool   `json:"openDashboardOnStart"`
	LogRawEvents  bool   `json:"logRawEvents"`
}

type Osu struct {
	Game         string `json:"game"` // stable | lazer
	Folder       string `json:"folder"`
	CacheFolder  string `json:"cacheFolder"`
	DownloadsDir string `json:"downloadsFolder"`
	UseOsuDB     bool   `json:"useOsuDb"` // stable: skip download when osu!.db already has the map
}

type Command struct {
	Name         string   `json:"name"`
	Aliases      []string `json:"aliases"`
	Enabled      bool     `json:"enabled"`
	BareNumberAs string   `json:"bareNumberAs"` // beatmap | set
}

type Rules struct {
	MinStars        float64  `json:"minStars"`
	MaxStars        float64  `json:"maxStars"`     // 0 = no limit
	MaxLengthSec    int      `json:"maxLengthSec"` // 0 = no limit
	Modes           []string `json:"modes"`        // osu taiko fruits mania
	Statuses        []string `json:"statuses"`     // ranked approved qualified loved pending wip graveyard
	BlacklistSets   []int    `json:"blacklistSets"`
	BlacklistMapper []string `json:"blacklistMappers"`
	BlacklistUsers  []string `json:"blacklistUsers"`
}

type Queue struct {
	MaxSize      int    `json:"maxSize"`
	PerUserLimit int    `json:"perUserLimit"`
	CooldownSec  int    `json:"cooldownSec"`
	NoRepeatMin  int    `json:"noRepeatMin"`
	PlayMode     string `json:"playMode"` // manual | auto
	AutoGapSec   int    `json:"autoGapSec"`
}

type Source struct {
	Name       string `json:"name"`
	Enabled    bool   `json:"enabled"`
	URL        string `json:"url"`        // {id} is replaced with the set id
	NoVideoURL string `json:"noVideoUrl"` // empty = mirror has no no-video variant
	Referer    string `json:"referer"`
	TimeoutSec int    `json:"timeoutSec"`
	Official   bool   `json:"official"` // browser + Downloads folder watch
}

type Download struct {
	Sources      []Source `json:"sources"`
	NoVideo      bool     `json:"noVideo"`
	CacheLimitMB int      `json:"cacheLimitMb"`
	AutoCleanup  bool     `json:"autoCleanup"`
}

type MD5 struct {
	Enabled        bool   `json:"enabled"`
	Level          string `json:"level"` // strict | requested
	VerifyCache    bool   `json:"verifyCache"`
	OnAllFail      string `json:"onAllFail"` // skip | warn
	MetaTimeoutSec int    `json:"metaTimeoutSec"`
}

type Import struct {
	Mode         string `json:"mode"` // open | copy-to-songs | extract-to-songs
	Preload      bool   `json:"preload"`
	PreloadCount int    `json:"preloadCount"`
}

type Overlay struct {
	Visible   int    `json:"visible"`
	Requester bool   `json:"showRequester"`
	Theme     string `json:"theme"`    // pink | sky | mono
	Scale     int    `json:"scale"`    // percent
	Position  string `json:"position"` // top-left | top-right | bottom-left | bottom-right
}

type Hotkeys struct {
	Enabled bool   `json:"enabled"`
	Next    string `json:"next"`
	Skip    string `json:"skip"`
	Remove  string `json:"removeLast"`
	Toggle  string `json:"toggleRequests"`
}

type Config struct {
	Language   string     `json:"language"` // en | th
	Connection Connection `json:"connection"`
	Osu        Osu        `json:"osu"`
	Command    Command    `json:"command"`
	Rules      Rules      `json:"rules"`
	Queue      Queue      `json:"queue"`
	Download   Download   `json:"download"`
	MD5        MD5        `json:"md5"`
	Import     Import     `json:"import"`
	Overlay    Overlay    `json:"overlay"`
	Hotkeys    Hotkeys    `json:"hotkeys"`
}

func Default() Config {
	home, _ := os.UserHomeDir()
	local := os.Getenv("LOCALAPPDATA")
	return Config{
		Language: "en",
		Connection: Connection{
			TikFinityURL:  "ws://127.0.0.1:21213/",
			DashboardPort: 21280,
			TikFinityPath: filepath.Join(local, "Programs", "tikfinity", "TikFinity.exe"),
			ReconnectSec:  5,
			OpenDashboard: true,
		},
		Osu: Osu{
			Game:         "stable",
			Folder:       filepath.Join(local, "osu!"),
			CacheFolder:  "cache",
			DownloadsDir: filepath.Join(home, "Downloads"),
			UseOsuDB:     false,
		},
		Command: Command{Name: "!req", Aliases: []string{"!request", "!r"}, Enabled: true, BareNumberAs: "beatmap"},
		Rules: Rules{
			MinStars: 0, MaxStars: 0, MaxLengthSec: 0,
			Modes:    []string{"osu", "taiko", "fruits", "mania"},
			Statuses: []string{"ranked", "approved", "qualified", "loved"},
		},
		Queue: Queue{MaxSize: 30, PerUserLimit: 2, CooldownSec: 60, NoRepeatMin: 60, PlayMode: "manual", AutoGapSec: 15},
		Download: Download{
			Sources: []Source{
				{Name: "catboy", Enabled: true, URL: "https://catboy.best/d/{id}", NoVideoURL: "https://catboy.best/d/{id}n", Referer: "https://catboy.best/", TimeoutSec: 60},
				{Name: "osu.direct", Enabled: true, URL: "https://osu.direct/api/d/{id}", NoVideoURL: "https://osu.direct/api/d/{id}?noVideo=1", TimeoutSec: 60},
				{Name: "official", Enabled: true, URL: "https://osu.ppy.sh/beatmapsets/{id}/download", NoVideoURL: "https://osu.ppy.sh/beatmapsets/{id}/download?noVideo=1", TimeoutSec: 180, Official: true},
			},
			NoVideo: true, CacheLimitMB: 2048, AutoCleanup: true,
		},
		MD5:     MD5{Enabled: true, Level: "requested", VerifyCache: true, OnAllFail: "skip", MetaTimeoutSec: 15},
		Import:  Import{Mode: "open", Preload: true, PreloadCount: 2},
		Overlay: Overlay{Visible: 5, Requester: true, Theme: "pink", Scale: 100, Position: "top-left"},
		Hotkeys: Hotkeys{Enabled: false, Next: "Ctrl+Alt+N", Skip: "Ctrl+Alt+S", Remove: "Ctrl+Alt+Backspace", Toggle: "Ctrl+Alt+R"},
	}
}

// AllowedHosts are the only hosts the downloader may fetch from.
var AllowedHosts = []string{"catboy.best", "osu.direct", "osu.ppy.sh"}

func (c *Config) Validate() error {
	var errs []string
	add := func(f string, a ...any) { errs = append(errs, fmt.Sprintf(f, a...)) }
	if p := c.Connection.DashboardPort; p < 1024 || p > 65535 {
		add("dashboard port must be 1024-65535")
	}
	if !strings.HasPrefix(c.Connection.TikFinityURL, "ws://127.0.0.1") && !strings.HasPrefix(c.Connection.TikFinityURL, "ws://localhost") {
		add("TikFinity URL must be ws://127.0.0.1:<port> or ws://localhost:<port>")
	}
	if c.Connection.ReconnectSec < 1 {
		c.Connection.ReconnectSec = 1
	}
	if c.Osu.Game != "stable" && c.Osu.Game != "lazer" {
		add("game must be stable or lazer")
	}
	if !strings.HasPrefix(c.Command.Name, "!") || len(c.Command.Name) < 2 || strings.ContainsAny(c.Command.Name, " \t") {
		add("command must start with ! and contain no spaces")
	}
	if c.Command.BareNumberAs != "beatmap" && c.Command.BareNumberAs != "set" {
		add("bare number must be beatmap or set")
	}
	if c.Rules.MaxStars > 0 && c.Rules.MinStars > c.Rules.MaxStars {
		add("min stars is above max stars")
	}
	if c.Queue.PlayMode != "manual" && c.Queue.PlayMode != "auto" {
		add("play mode must be manual or auto")
	}
	if c.MD5.Level != "strict" && c.MD5.Level != "requested" {
		add("MD5 level must be strict or requested")
	}
	if c.MD5.OnAllFail != "skip" && c.MD5.OnAllFail != "warn" {
		add("all-fail behaviour must be skip or warn")
	}
	switch c.Import.Mode {
	case "open", "copy-to-songs", "extract-to-songs":
	default:
		add("import mode must be open, copy-to-songs or extract-to-songs")
	}
	if c.Import.PreloadCount < 0 || c.Import.PreloadCount > 10 {
		add("preload count must be 0-10")
	}
	for _, s := range c.Download.Sources {
		for _, u := range []string{s.URL, s.NoVideoURL} {
			if u != "" && !HostAllowed(u) {
				add("source %q uses a host that is not whitelisted", s.Name)
			}
		}
		if s.TimeoutSec < 5 {
			add("source %q timeout must be at least 5 seconds", s.Name)
		}
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

// HostAllowed reports whether an https URL points at a whitelisted host.
func HostAllowed(raw string) bool {
	rest, ok := strings.CutPrefix(raw, "https://")
	if !ok {
		return false
	}
	host, _, _ := strings.Cut(rest, "/")
	host, _, _ = strings.Cut(host, "?")
	for _, h := range AllowedHosts {
		if host == h {
			return true
		}
	}
	return false
}

// Store guards the live config and persists it.
type Store struct {
	mu   sync.RWMutex
	cfg  Config
	path string
	subs []func(Config)
}

func Load(path string) (*Store, error) {
	s := &Store{path: path, cfg: Default()}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, s.save()
	}
	if err != nil {
		return nil, err
	}
	// Unmarshal over the defaults so new fields keep sane values.
	if err := json.Unmarshal(b, &s.cfg); err != nil {
		return nil, fmt.Errorf("config.json: %w", err)
	}
	if err := s.cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config.json: %w", err)
	}
	return s, nil
}

func (s *Store) Get() Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg
}

func (s *Store) Set(c Config) error {
	if err := c.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	s.cfg = c
	err := s.save()
	subs := s.subs
	s.mu.Unlock()
	for _, f := range subs {
		f(c)
	}
	return err
}

func (s *Store) OnChange(f func(Config)) {
	s.mu.Lock()
	s.subs = append(s.subs, f)
	s.mu.Unlock()
}

func (s *Store) save() error {
	b, _ := json.MarshalIndent(s.cfg, "", "  ")
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}
