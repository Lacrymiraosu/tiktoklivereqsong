// Package config loads, validates and saves config.json next to the executable.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"

	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/mods"
)

type Connection struct {
	TikFinityURL   string `json:"tikfinityUrl"`
	DashboardPort  int    `json:"dashboardPort"`
	AutoLaunch     bool   `json:"autoLaunchTikfinity"`
	TikFinityPath  string `json:"tikfinityPath"`
	ReconnectSec   int    `json:"reconnectSec"`
	OpenDashboard  bool   `json:"openDashboardOnStart"`
	LogRawEvents   bool   `json:"logRawEvents"`
	TosuURL        string `json:"tosuUrl"`
	TosuPath       string `json:"tosuPath"` // filled in automatically once tosu is seen running
	AutoLaunchTosu bool   `json:"autoLaunchTosu"`
	CheckUpdates   bool   `json:"checkUpdates"` // ask GitHub for a newer release
}

type Osu struct {
	Game         string `json:"game"`           // stable | lazer
	DetectGame   bool   `json:"detectGameTosu"` // switch Game to the client tosu reports
	Folder       string `json:"folder"`
	CacheFolder  string `json:"cacheFolder"`
	DownloadsDir string `json:"downloadsFolder"`
	UseOsuDB     bool   `json:"useOsuDb"`  // stable: skip download when osu!.db already has the map
	ViewerURL    string `json:"viewerUrl"` // beatmap viewer opened with ?s={set}&b={map}; empty hides the button
}

type Command struct {
	Name         string   `json:"name"`
	Aliases      []string `json:"aliases"`
	Enabled      bool     `json:"enabled"`
	BareNumberAs string   `json:"bareNumberAs"` // beatmap | set
	Search       bool     `json:"searchByName"` // "!req <song name>" searches catboy.best

	// Mod commands. An empty name turns that command off.
	Mods            []string `json:"mods"`            // TikTok users who may use mod commands
	TrustTikTokMods bool     `json:"trustTikTokMods"` // also accept TikTok's own moderator flag
	SkipCmd         string   `json:"skipCmd"`         // mod: skip the song being played
	RemoveCmd       string   `json:"removeCmd"`       // anyone: remove own latest; mod: "!remove @user"
	OpenCmd         string   `json:"openCmd"`         // mod: requests on
	CloseCmd        string   `json:"closeCmd"`        // mod: requests off

	// Viewer commands, answered on the overlay. An empty name turns it off.
	PosCmd      string `json:"posCmd"`      // anyone: where are my requests in the queue
	NowCmd      string `json:"nowCmd"`      // anyone: what is playing
	VoteSkipCmd string `json:"voteSkipCmd"` // anyone: vote to skip the song being played
	// VoteSkipVotes is how many different viewers must vote to skip; 0 = off.
	VoteSkipVotes int `json:"voteSkipVotes"`
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
	// BlacklistSongs are song rules (see ParseSongRule): artist="a_hisa",
	// title="Ghost", both, "Artist - Title" or a bare title. They match every
	// set of the song, see validator.SongBanned.
	BlacklistSongs []string `json:"blacklistSongs"`
	ExemptUsers    []string `json:"exemptUsers"` // no per-user limit or cooldown (streamer, mods)
	// ModeStars overrides MinStars/MaxStars per mode (osu, taiko, fruits,
	// mania); a 0 keeps the global value. Stars of different modes are not
	// comparable, so a mania streamer can allow 4.5★ while osu! stops at 6★.
	ModeStars map[string]StarRange `json:"modeStars,omitempty"`
	// Mods in requests ("!req 123 HDHR"). Stars are always the no-mod rating;
	// HardModMaxStars (0 = none) caps maps requested with DT, NC or HR.
	AllowMods       bool     `json:"allowMods"`
	BannedMods      []string `json:"bannedMods"`
	HardModMaxStars float64  `json:"hardModMaxStars"`
	FollowersOnly   bool     `json:"followersOnly"`          // reject viewers TikTok says do not follow
	SubscriberLimit int      `json:"subscriberPerUserLimit"` // per-user limit for subscribers; 0 = same as everyone
}

// StarRange is a min/max star limit; 0 means "no override".
type StarRange struct {
	Min float64 `json:"min"`
	Max float64 `json:"max"`
}

// StarLimits returns the star range for mode and whether a per-mode
// override was used.
func (r Rules) StarLimits(mode string) (min, max float64, perMode bool) {
	min, max = r.MinStars, r.MaxStars
	if o, ok := r.ModeStars[mode]; ok {
		if o.Min > 0 {
			min, perMode = o.Min, true
		}
		if o.Max > 0 {
			max, perMode = o.Max, true
		}
	}
	return min, max, perMode
}

type Queue struct {
	MaxSize      int  `json:"maxSize"`
	PerUserLimit int  `json:"perUserLimit"`
	CooldownSec  int  `json:"cooldownSec"`
	NoRepeatMin  int  `json:"noRepeatMin"`
	Fair         bool `json:"fair"` // new requests take turns between viewers (queue.AddFair)
	// Requests close by themselves when AutoCloseAt requests are waiting (0 =
	// off) and open again at AutoReopenAt or fewer; CloseAt ("HH:MM", local
	// time, empty = off) closes them once a day, e.g. before the stream ends.
	AutoCloseAt  int    `json:"autoCloseAt"`
	AutoReopenAt int    `json:"autoReopenAt"`
	CloseAt      string `json:"closeAt"`
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
	Visible     int    `json:"visible"`
	Requester   bool   `json:"showRequester"`
	Theme       string `json:"theme"`       // pink | sky | mono
	Scale       int    `json:"scale"`       // percent
	Position    string `json:"position"`    // top-left | top-right | bottom-left | bottom-right
	Feedback    bool   `json:"feedback"`    // pop-ups for added/rejected requests, mod commands, gifts, results
	FeedbackSec int    `json:"feedbackSec"` // how long each pop-up stays
	Lang        string `json:"lang"`        // auto | en | th
	Avatars     bool   `json:"avatars"`     // viewers' TikTok pictures next to their names
}

type Hotkeys struct {
	Enabled bool   `json:"enabled"`
	Next    string `json:"next"`
	Skip    string `json:"skip"`
	Remove  string `json:"removeLast"`
	Toggle  string `json:"toggleRequests"`
}

// Gifts turns TikTok gifts into queue priority.
type Gifts struct {
	Enabled          bool `json:"enabled"`
	CoinsPerPriority int  `json:"coinsPerPriority"` // coins (added up per viewer) for one priority request
	CreditMinutes    int  `json:"creditMinutes"`    // how long an unused priority (or paid request) waits
	// RequestCoins > 0 makes requests gift-only: each request costs this many
	// gift coins (added up per viewer). Works with or without priority.
	RequestCoins int `json:"requestCoins"`
}

// Remote is the mod panel for a phone on the same network (package remote).
type Remote struct {
	Enabled bool   `json:"enabled"`
	Port    int    `json:"port"`
	PIN     string `json:"pin"` // 6-12 digits
}

// TTS reads chat aloud in the dashboard, without commands ("!req …"),
// which TikFinity's own TTS cannot skip.
type TTS struct {
	Enabled bool   `json:"enabled"`
	Lang    string `json:"lang"`  // auto (by the message's letters) | th | en
	Voice   string `json:"voice"` // a voice name from the browser; empty = best for the language
	// Engine is who speaks: auto (the browser's voice when it has one for the
	// language, Google's otherwise), browser, or google (always Google's
	// voice; the text is sent to Google).
	Engine      string  `json:"engine"`
	Rate        float64 `json:"rate"` // 0.5-2
	Volume      int     `json:"volume"`
	ReadName    bool    `json:"readName"`    // "name says …"
	MaxLength   int     `json:"maxLength"`   // longer messages are cut
	CooldownSec int     `json:"cooldownSec"` // per viewer
	// Allow limits whose chat is read: subscriber (ใจส้ม, TikTok LIVE
	// subscription), follower, gifter (sent a gift during this stream), mod.
	// A viewer needs any one; empty = everyone. No-limit users always pass.
	Allow         []string `json:"allow"`
	FollowersOnly bool     `json:"followersOnly"` // older setting, read as allow: [follower]
	BlockedWords  []string `json:"blockedWords"`  // messages containing one are not read
	// Announce speaks what happens to requests from Templates (event ->
	// text with {placeholders}; see app.Announcements). Empty or "-" = silent.
	Announce  bool              `json:"announce"`
	Templates map[string]string `json:"templates"`
}

// AnnounceEvents are the events that can be spoken, in the order the
// settings show them. "rejected" is used for any rejection without its own
// "rejected_<code>" template.
var AnnounceEvents = []string{"added", "rejected", "rejected_stars_high", "rejected_stars_low", "rejected_mod_stars", "rejected_too_long",
	"rejected_duplicate", "rejected_recently_played", "rejected_cooldown", "rejected_user_limit", "rejected_search_none",
	"nowplaying", "np", "np_none", "pos", "result_passed", "result_failed", "gift", "voteskip", "auto_off", "auto_on"}

// ThaiAnnouncements are the default templates.
func ThaiAnnouncements() map[string]string {
	return map[string]string{
		"added":                    "{user} ขอเพลง {title} ของ {artist} แมพโดย {mapper} ระดับ {diff} {mods}",
		"rejected":                 "{user} ขอเพลงไม่ได้ {reason}",
		"rejected_stars_high":      "เพลง {title} ยากเกินไป {stars} ดาว",
		"rejected_stars_low":       "เพลง {title} ง่ายเกินไป {stars} ดาว",
		"rejected_mod_stars":       "เพลง {title} ยากเกินไปสำหรับ {mods}",
		"rejected_too_long":        "เพลง {title} ยาวเกินไป",
		"rejected_duplicate":       "เพลง {title} อยู่ในคิวแล้ว",
		"rejected_recently_played": "เพลง {title} เพิ่งเล่นไป",
		"rejected_cooldown":        "-",
		"rejected_user_limit":      "-",
		"rejected_search_none":     "ไม่เจอเพลงที่ {user} ขอ",
		"nowplaying":               "ต่อไปเพลง {title} ที่ {user} ขอ",
		"np":                       "กำลังเล่น {artist} - {title} {setid}",
		"np_none":                  "ยังไม่มีเพลงที่เล่นอยู่",
		"pos":                      "-",
		"result_passed":            "ผ่านเพลง {title} ความแม่นยำ {acc} เปอร์เซ็นต์",
		"result_failed":            "-",
		"gift":                     "ขอบคุณ {user} สำหรับของขวัญ",
		"voteskip":                 "ผู้ชมโหวตข้ามเพลง {title}",
		"auto_off":                 "คิวเต็มแล้ว พักรับคำขอก่อนนะ",
		"auto_on":                  "เปิดรับคำขอแล้ว",
	}
}

// EnglishAnnouncements are the same templates in English.
func EnglishAnnouncements() map[string]string {
	return map[string]string{
		"added":                    "{user} requested {title} by {artist}, mapped by {mapper}, {diff} {mods}",
		"rejected":                 "{user}, that request did not work: {reason}",
		"rejected_stars_high":      "{title} is too hard, {stars} stars",
		"rejected_stars_low":       "{title} is too easy, {stars} stars",
		"rejected_mod_stars":       "{title} is too hard with {mods}",
		"rejected_too_long":        "{title} is too long",
		"rejected_duplicate":       "{title} is already in the queue",
		"rejected_recently_played": "{title} was played recently",
		"rejected_cooldown":        "-",
		"rejected_user_limit":      "-",
		"rejected_search_none":     "No map found for {user}",
		"nowplaying":               "Next up, {title}, requested by {user}",
		"np":                       "Now playing {artist} - {title}, set {setid}",
		"np_none":                  "Nothing is playing yet",
		"pos":                      "-",
		"result_passed":            "Cleared {title} with {acc} percent",
		"result_failed":            "-",
		"gift":                     "Thank you {user} for the gift",
		"voteskip":                 "Chat voted to skip {title}",
		"auto_off":                 "The queue is full, requests are paused",
		"auto_on":                  "Requests are open again",
	}
}

// Test holds the dashboard's test-panel switches.
type Test struct {
	DryRun bool `json:"dryRun"` // Next/Play move the queue but never open or copy anything into osu!
}

type Config struct {
	Language string `json:"language"` // en | th
	// MasterVolume (0-100) scales every sound the dashboard plays (map previews).
	MasterVolume int        `json:"masterVolume"`
	Connection   Connection `json:"connection"`
	Osu          Osu        `json:"osu"`
	Command      Command    `json:"command"`
	Rules        Rules      `json:"rules"`
	Queue        Queue      `json:"queue"`
	Download     Download   `json:"download"`
	MD5          MD5        `json:"md5"`
	Import       Import     `json:"import"`
	Overlay      Overlay    `json:"overlay"`
	Hotkeys      Hotkeys    `json:"hotkeys"`
	Test         Test       `json:"test"`
	Gifts        Gifts      `json:"gifts"`
	Remote       Remote     `json:"remote"`
	TTS          TTS        `json:"tts"`
}

func Default() Config {
	home, _ := os.UserHomeDir()
	local := os.Getenv("LOCALAPPDATA")
	return Config{
		Language:     "en",
		MasterVolume: 10,
		Connection: Connection{
			TikFinityURL:  "ws://127.0.0.1:21213/",
			DashboardPort: 21280,
			TikFinityPath: filepath.Join(local, "Programs", "tikfinity", "TikFinity.exe"),
			ReconnectSec:  5,
			OpenDashboard: true,
			TosuURL:       "ws://127.0.0.1:24050/websocket/v2",
			CheckUpdates:  true,
		},
		Osu: Osu{
			Game:         "stable",
			DetectGame:   true,
			Folder:       filepath.Join(local, "osu!"),
			CacheFolder:  "cache",
			DownloadsDir: filepath.Join(home, "Downloads"),
			UseOsuDB:     false,
			ViewerURL:    "https://osu-beatmap-viewer.vercel.app/",
		},
		Command: Command{Name: "!req", Aliases: []string{"!request", "!r"}, Enabled: true, BareNumberAs: "beatmap", Search: true,
			TrustTikTokMods: true, SkipCmd: "!skip", RemoveCmd: "!remove", OpenCmd: "!reqon", CloseCmd: "!reqoff",
			PosCmd: "!pos", NowCmd: "!np", VoteSkipCmd: "!voteskip"},
		Rules: Rules{
			MinStars: 0, MaxStars: 0, MaxLengthSec: 0,
			Modes:     []string{"osu", "taiko", "fruits", "mania"},
			Statuses:  []string{"ranked", "approved", "qualified", "loved"},
			AllowMods: true,
		},
		Queue: Queue{MaxSize: 30, PerUserLimit: 2, CooldownSec: 60, NoRepeatMin: 60, PlayMode: "manual", AutoGapSec: 5},
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
		Overlay: Overlay{Visible: 5, Requester: true, Theme: "pink", Scale: 100, Position: "top-left", Feedback: true, FeedbackSec: 7, Lang: "auto", Avatars: true},
		Gifts:   Gifts{Enabled: false, CoinsPerPriority: 30, CreditMinutes: 60},
		Remote:  Remote{Port: 21281},
		TTS:     TTS{Lang: "auto", Engine: "auto", Rate: 1, Volume: 80, ReadName: true, MaxLength: 120, CooldownSec: 3, Announce: true, Templates: ThaiAnnouncements()},
		Hotkeys: Hotkeys{Enabled: false, Next: "Ctrl+Alt+N", Skip: "Ctrl+Alt+S", Remove: "Ctrl+Alt+Backspace", Toggle: "Ctrl+Alt+R"},
	}
}

// AllowedHosts are the only hosts the downloader may fetch from.
// storage.osu.direct is where osu.direct redirects every download.
var AllowedHosts = []string{"catboy.best", "osu.direct", "storage.osu.direct", "osu.ppy.sh"}

// Problem is one invalid setting. Code and Vars let the dashboard say it in
// the viewer's language (T["e_"+code]); Msg is the English text.
type Problem struct {
	Code string            `json:"code"`
	Vars map[string]string `json:"vars,omitempty"`
	Msg  string            `json:"msg"`
}

// ValidationError lists every invalid setting.
type ValidationError struct{ Problems []Problem }

func (e *ValidationError) Error() string {
	msgs := make([]string, len(e.Problems))
	for i, p := range e.Problems {
		msgs[i] = p.Msg
	}
	return strings.Join(msgs, "; ")
}

// ErrCode returns the first problem's code and variables for translation.
func (e *ValidationError) ErrCode() (string, map[string]string) {
	return e.Problems[0].Code, e.Problems[0].Vars
}

func (c *Config) Validate() error {
	var probs []Problem
	add := func(code string, vars map[string]string, f string, a ...any) {
		probs = append(probs, Problem{Code: code, Vars: vars, Msg: fmt.Sprintf(f, a...)})
	}
	if c.MasterVolume < 0 || c.MasterVolume > 100 {
		add("vol_range", nil, "master volume must be 0-100")
	}
	if p := c.Connection.DashboardPort; p < 1024 || p > 65535 {
		add("port_range", nil, "dashboard port must be 1024-65535")
	}
	if !strings.HasPrefix(c.Connection.TikFinityURL, "ws://127.0.0.1") && !strings.HasPrefix(c.Connection.TikFinityURL, "ws://localhost") {
		add("tikfinity_url", nil, "TikFinity URL must be ws://127.0.0.1:<port> or ws://localhost:<port>")
	}
	if !strings.HasPrefix(c.Connection.TosuURL, "ws://127.0.0.1") && !strings.HasPrefix(c.Connection.TosuURL, "ws://localhost") {
		add("tosu_url", nil, "tosu URL must be ws://127.0.0.1:<port>/websocket/v2 or ws://localhost:<port>/websocket/v2")
	}
	if c.Connection.ReconnectSec < 1 {
		c.Connection.ReconnectSec = 1
	}
	if v := c.Osu.ViewerURL; v != "" && !strings.HasPrefix(v, "https://") && !strings.HasPrefix(v, "http://localhost") && !strings.HasPrefix(v, "http://127.0.0.1") {
		add("viewer_url", nil, "viewer URL must start with https:// (or http://localhost for a local copy)")
	}
	if c.Osu.Game != "stable" && c.Osu.Game != "lazer" {
		add("game", nil, "game must be stable or lazer")
	}
	if !strings.HasPrefix(c.Command.Name, "!") || len(c.Command.Name) < 2 || strings.ContainsAny(c.Command.Name, " \t") {
		add("command_name", nil, "command must start with ! and contain no spaces")
	}
	for _, mc := range []string{c.Command.SkipCmd, c.Command.RemoveCmd, c.Command.OpenCmd, c.Command.CloseCmd, c.Command.PosCmd, c.Command.NowCmd, c.Command.VoteSkipCmd} {
		if mc != "" && (!strings.HasPrefix(mc, "!") || len(mc) < 2 || strings.ContainsAny(mc, " \t")) {
			add("mod_cmd", map[string]string{"cmd": mc}, "mod command %q must start with ! and contain no spaces", mc)
		}
	}
	if c.Gifts.RequestCoins < 0 {
		add("request_coins", nil, "coins per request cannot be negative")
	}
	if c.Command.VoteSkipVotes < 0 {
		add("vote_skip", nil, "votes to skip cannot be negative")
	}
	if p := c.Remote.Port; p < 1024 || p > 65535 || p == c.Connection.DashboardPort {
		add("remote_port", nil, "mod panel port must be 1024-65535 and not the dashboard port")
	}
	if c.Remote.Enabled && !GoodPIN(c.Remote.PIN) {
		add("remote_pin", nil, "mod panel PIN must be 6-12 digits, not all the same and not a simple run like 123456")
	}
	if t := c.TTS; t.Lang != "auto" && t.Lang != "th" && t.Lang != "en" {
		add("tts_lang", nil, "TTS language must be auto, th or en")
	}
	if t := c.TTS; !slices.Contains([]string{"auto", "browser", "google"}, t.Engine) {
		add("tts_engine", nil, "TTS voice must be auto, browser or google")
	}
	if t := c.TTS; t.Rate < 0.5 || t.Rate > 2 || t.Volume < 0 || t.Volume > 100 || t.MaxLength < 10 || t.MaxLength > 500 || t.CooldownSec < 0 {
		add("tts_range", nil, "TTS: speed 0.5-2, volume 0-100, max length 10-500, cooldown 0 or more")
	}
	for _, w := range c.TTS.Allow {
		if !slices.Contains([]string{"subscriber", "follower", "gifter", "mod"}, w) {
			add("tts_allow", map[string]string{"who": w}, "unknown TTS group %q", w)
		}
	}
	for k, v := range c.TTS.Templates {
		if !slices.Contains(AnnounceEvents, k) {
			add("tts_template", map[string]string{"event": k}, "unknown announcement %q", k)
		} else if len([]rune(v)) > 300 {
			add("tts_template_long", map[string]string{"event": k}, "announcement %q is longer than 300 letters", k)
		}
	}
	if c.Command.BareNumberAs != "beatmap" && c.Command.BareNumberAs != "set" {
		add("bare_number", nil, "bare number must be beatmap or set")
	}
	for _, e := range c.Rules.BlacklistSongs {
		if _, ok := ParseSongRule(e); !ok && strings.TrimSpace(e) != "" {
			add("song_rule", map[string]string{"rule": e}, "song rule %q: use artist=\"…\", title=\"…\", \"Artist - Title\" or a title", e)
		}
	}
	if c.Rules.HardModMaxStars < 0 {
		add("hard_mod_stars", nil, "star limit for DT/HR requests cannot be negative")
	}
	for _, m := range c.Rules.BannedMods {
		if got, err := mods.Parse(m); err != nil || len(got) != 2 {
			add("banned_mod", map[string]string{"mod": m}, "%q is not a mod (use two letters, e.g. FL)", m)
		}
	}
	if c.Rules.MaxStars > 0 && c.Rules.MinStars > c.Rules.MaxStars {
		add("stars_order", nil, "min stars is above max stars")
	}
	for mode, r := range c.Rules.ModeStars {
		switch mode {
		case "osu", "taiko", "fruits", "mania":
		default:
			add("mode_unknown", map[string]string{"mode": mode}, "unknown mode %q in per-mode stars", mode)
			continue
		}
		if r.Min < 0 || r.Max < 0 {
			add("stars_negative", map[string]string{"mode": mode}, "%s stars cannot be negative", mode)
		}
		if min, max, _ := c.Rules.StarLimits(mode); max > 0 && min > max {
			add("stars_order_mode", map[string]string{"mode": mode}, "%s: min stars is above max stars", mode)
		}
	}
	if c.Queue.AutoCloseAt < 0 || c.Queue.AutoReopenAt < 0 || (c.Queue.AutoCloseAt > 0 && c.Queue.AutoReopenAt >= c.Queue.AutoCloseAt) {
		add("auto_close", nil, "auto-close: reopen must be below the close size (both 0 or more)")
	}
	if _, ok := ClockTime(c.Queue.CloseAt); !ok && c.Queue.CloseAt != "" {
		add("close_at", nil, "close time must be HH:MM, e.g. 23:30")
	}
	if c.Queue.PlayMode != "manual" && c.Queue.PlayMode != "auto" {
		add("play_mode", nil, "play mode must be manual or auto")
	}
	if c.MD5.Level != "strict" && c.MD5.Level != "requested" {
		add("md5_level", nil, "MD5 level must be strict or requested")
	}
	if c.MD5.OnAllFail != "skip" && c.MD5.OnAllFail != "warn" {
		add("all_fail", nil, "all-fail behaviour must be skip or warn")
	}
	switch c.Import.Mode {
	case "open", "copy-to-songs", "extract-to-songs":
	default:
		add("import_mode", nil, "import mode must be open, copy-to-songs or extract-to-songs")
	}
	if c.Import.PreloadCount < 0 || c.Import.PreloadCount > 10 {
		add("preload_count", nil, "preload count must be 0-10")
	}
	for _, s := range c.Download.Sources {
		for _, u := range []string{s.URL, s.NoVideoURL} {
			if u != "" && !HostAllowed(u) {
				add("source_host", map[string]string{"name": s.Name}, "source %q uses a host that is not whitelisted", s.Name)
			}
		}
		if s.TimeoutSec < 5 {
			add("source_timeout", map[string]string{"name": s.Name}, "source %q timeout must be at least 5 seconds", s.Name)
		}
	}
	if len(probs) > 0 {
		return &ValidationError{Problems: probs}
	}
	return nil
}

// SongRule is one rules.blacklistSongs entry; an empty field matches anything.
type SongRule struct {
	Artist string
	Title  string
}

var (
	quotes    = strings.NewReplacer("“", `"`, "”", `"`, "„", `"`, "‟", `"`, "″", `"`, "＂", `"`, "‘", "'", "’", "'", "＝", "=")
	reRuleKey = regexp.MustCompile(`(?i)^\s*(artist|title)\s*=`)
	reRuleKV  = regexp.MustCompile(`(?i)^\s*(artist|title)\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s"']+))`)
)

// ParseSongRule reads a song rule in one of these forms:
//
//	artist="a_hisa"                  every song by that artist
//	title="Ghost"                    that song by anyone
//	artist="xi" title="Blue Zenith"  that song by that artist
//	xi - Blue Zenith                 the same, short form
//	Blue Zenith                      a title by anyone
//
// Keys are case-insensitive, quotes may be straight or curly (as phones type
// them) or left out for one word.
func ParseSongRule(s string) (SongRule, bool) {
	s = strings.TrimSpace(quotes.Replace(s))
	if s == "" {
		return SongRule{}, false
	}
	if reRuleKey.MatchString(s) {
		var r SongRule
		for rest := s; strings.TrimSpace(rest) != ""; {
			m := reRuleKV.FindStringSubmatch(rest)
			if m == nil {
				return SongRule{}, false // something that is not key=value
			}
			v := strings.TrimSpace(m[2] + m[3] + m[4])
			if v == "" {
				return SongRule{}, false
			}
			if strings.EqualFold(m[1], "artist") {
				r.Artist = v
			} else {
				r.Title = v
			}
			rest = rest[len(m[0]):]
		}
		return r, true
	}
	if a, t, ok := strings.Cut(s, " - "); ok && strings.TrimSpace(a) != "" && strings.TrimSpace(t) != "" {
		return SongRule{Artist: strings.TrimSpace(a), Title: strings.TrimSpace(t)}, true
	}
	return SongRule{Title: s}, true
}

// String writes the rule in the key="value" form.
func (r SongRule) String() string {
	var parts []string
	if r.Artist != "" {
		parts = append(parts, `artist="`+r.Artist+`"`)
	}
	if r.Title != "" {
		parts = append(parts, `title="`+r.Title+`"`)
	}
	return strings.Join(parts, " ")
}

// GoodPIN accepts 6-12 digits that are not all the same digit and not a
// plain run up or down (123456, 987654).
func GoodPIN(pin string) bool {
	if len(pin) < 6 || len(pin) > 12 {
		return false
	}
	same, up, down := true, true, true
	for i := 0; i < len(pin); i++ {
		if pin[i] < '0' || pin[i] > '9' {
			return false
		}
		if i > 0 {
			same = same && pin[i] == pin[0]
			up = up && pin[i] == pin[i-1]+1
			down = down && pin[i] == pin[i-1]-1
		}
	}
	return !same && !up && !down
}

// ClockTime parses "HH:MM" (24 h) into minutes after midnight.
func ClockTime(s string) (int, bool) {
	var h, m int
	if len(s) != 5 || s[2] != ':' {
		return 0, false
	}
	if _, err := fmt.Sscanf(s, "%02d:%02d", &h, &m); err != nil || h > 23 || m > 59 || h < 0 || m < 0 {
		return 0, false
	}
	return h*60 + m, true
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
	// Unmarshal over the defaults so new fields keep sane values. The
	// announcement lines are filled in afterwards instead (fillTemplates):
	// merged into the Thai defaults, an English set would get Thai lines.
	s.cfg.TTS.Templates = nil
	if err := json.Unmarshal(b, &s.cfg); err != nil {
		return nil, fmt.Errorf("config.json: %w", err)
	}
	s.cfg.TTS.fillTemplates()
	if s.cfg.TTS.FollowersOnly && len(s.cfg.TTS.Allow) == 0 {
		s.cfg.TTS.Allow, s.cfg.TTS.FollowersOnly = []string{"follower"}, false
	}
	if err := s.cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config.json: %w", err)
	}
	return s, nil
}

// fillTemplates gives announcements added in a newer version their default
// line, in the language the streamer's own lines are written in. The
// dashboard always saves every line, so a missing one was never seen.
func (t *TTS) fillTemplates() {
	if t.Templates == nil {
		t.Templates = map[string]string{}
	}
	defaults := ThaiAnnouncements()
	if added := t.Templates["added"]; added != "" && !strings.ContainsFunc(added, func(r rune) bool { return r >= 0x0E00 && r <= 0x0E7F }) {
		defaults = EnglishAnnouncements()
	}
	for k, v := range defaults {
		if _, ok := t.Templates[k]; !ok {
			t.Templates[k] = v
		}
	}
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
