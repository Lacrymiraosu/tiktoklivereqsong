// Package app wires the modules together: chat -> parser -> osuMeta ->
// validator -> queue -> downloader/verifier -> importer.
package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/config"
	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/downloader"
	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/importer"
	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/mods"
	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/osumeta"
	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/parser"
	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/queue"
	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/search"
	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/tikfinity"
	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/tosu"
	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/update"
	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/validator"
	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/verifier"
)

type App struct {
	Cfg     *config.Store
	Queue   *queue.Queue
	Meta    *osumeta.Client
	DL      *downloader.Downloader
	Import  *importer.Importer
	TF      *tikfinity.Client
	Tosu    *tosu.Client // song-end detection for auto mode (required)
	OsuDB   *importer.OsuDB
	Search  *search.Client // "!req <song name>"; nil turns names off
	OpenURL func(string) error

	changed    func()
	wake       chan struct{}
	mu         sync.Mutex
	inflight   map[string]context.CancelFunc // item id -> download cancel
	notices    []Notice
	advance    sync.Mutex
	pending    int // requests being resolved
	track      tosu.Tracker
	trackID    string // queue item the tracker follows
	feed       []FeedEvent
	feedSeq    int64
	lastPlay   tosu.Score // latest live numbers of the map being tracked
	lastFailed bool
	warned     map[string]bool
	infoAt     map[string]time.Time // user|command -> last !pos / !np answered
	voteID     string               // now-playing item the skip votes are for
	voters     map[string]bool
	votePassed bool              // the vote on voteID reached the count; later votes do nothing
	latest     *update.Release   // a newer osureq release, if GitHub has one
	summaryN   int64             // bumped each time the overlay should show the session summary
	avatars    map[string]string // user -> profile picture, from their last command or gift
	autoClosed bool              // requests were closed by autoCloseAt, so they may reopen by themselves
	lastLen    int               // queue length at the previous auto-close check (-1 before the first)
	lastTick   time.Time         // previous closeAt check
	speech     []Speech          // chat for the dashboard to read aloud, newest first
	speechSeq  int64
	ttsAt      map[string]time.Time // user -> last message read (TTS cooldown)
	gifters    map[string]time.Time // user -> last gift, for TTS "gifters only"
}

type Notice struct {
	At   time.Time `json:"at"`
	Kind string    `json:"kind"` // info | warn | error
	Text string    `json:"text"` // English, also written to the log
	// Key and Vars let the dashboard show the notice in its own language
	// (T["n_"+key] with {var} placeholders); Text is the fallback.
	Key  string            `json:"key,omitempty"`
	Vars map[string]string `json:"vars,omitempty"`
}

func New(cfg *config.Store, q *queue.Queue, meta *osumeta.Client, dl *downloader.Downloader, im *importer.Importer, tf *tikfinity.Client, openURL func(string) error) *App {
	a := &App{Cfg: cfg, Queue: q, Meta: meta, DL: dl, Import: im, TF: tf, OsuDB: &importer.OsuDB{}, OpenURL: openURL,
		wake: make(chan struct{}, 1), inflight: map[string]context.CancelFunc{}, lastLen: -1}
	q.OnChange(func() { a.Changed(); a.Wake() })
	return a
}

// OnChange registers the UI push callback.
func (a *App) OnChange(f func()) { a.changed = f }

func (a *App) Changed() {
	if a.changed != nil {
		a.changed()
	}
}

func (a *App) Wake() {
	select {
	case a.wake <- struct{}{}:
	default:
	}
}

func (a *App) Notify(kind, f string, args ...any) { a.Say(kind, "", nil, f, args...) }

// Say is Notify with a translation key and its variables for the dashboard.
func (a *App) Say(kind, key string, vars map[string]string, f string, args ...any) {
	n := Notice{At: time.Now(), Kind: kind, Text: fmt.Sprintf(f, args...), Key: key, Vars: vars}
	log.Printf("[%s] %s", kind, n.Text)
	a.mu.Lock()
	a.notices = append([]Notice{n}, a.notices...)
	if len(a.notices) > 50 {
		a.notices = a.notices[:50]
	}
	a.mu.Unlock()
	a.Changed()
}

// warnOnce adds an activity warning once per run for each key.
func (a *App) warnOnce(key string, vars map[string]string, f string, args ...any) {
	a.mu.Lock()
	if a.warned == nil {
		a.warned = map[string]bool{}
	}
	seen := a.warned[key]
	a.warned[key] = true
	a.mu.Unlock()
	if !seen {
		a.Say("warn", key, vars, f, args...)
	}
}

func (a *App) Notices() []Notice {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]Notice(nil), a.notices...)
}

// HandleChat is called for every TikFinity chat event.
func (a *App) HandleChat(ch tikfinity.Chat) {
	cfg := a.Cfg.Get()
	if !a.TF.Accepting() {
		return
	}
	if strings.HasPrefix(ch.Comment, "!") {
		a.rememberAvatar(ch.User, ch.Avatar)
	}
	a.speakChat(ch, false)
	if a.command(ch, false) != cmdNone {
		return
	}
	req, ok := parser.Parse(ch.Comment, a.parseOptions(cfg))
	if !ok {
		return
	}
	if !cfg.Command.Enabled {
		a.Queue.Reject(queue.Rejected{At: time.Now(), User: ch.User, Input: ch.Comment, Code: "requests_off", Detail: "requests are turned off"})
		a.emit(FeedEvent{Kind: "rejected", User: ch.User, Nick: ch.Nick, Code: "requests_off"})
		return
	}
	if cfg.Rules.FollowersOnly && ch.Follower == tikfinity.Unknown {
		a.warnOnce("followUnknown", map[string]string{"u": ch.User}, "followers-only is on, but TikFinity did not say whether @%s follows; letting the request through", ch.User)
	}
	go a.submit(ch.User, ch.Nick, ch.Comment, req, fromChat, validator.Roles{Follower: int8(ch.Follower), Subscriber: int8(ch.Subscriber)})
}

// rememberAvatar keeps a viewer's picture for their requests and pop-ups.
// Only viewers who use a command or send a gift are kept, and the map starts
// over if a very busy stream fills it.
func (a *App) rememberAvatar(user, url string) {
	if url == "" {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.avatars == nil || len(a.avatars) >= 5000 {
		a.avatars = map[string]string{}
	}
	a.avatars[strings.ToLower(user)] = url
}

func (a *App) avatar(user string) string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.avatars[strings.ToLower(user)]
}

// origin says where a request came from.
type origin int

const (
	fromChat  origin = iota // TikTok chat via TikFinity
	fromSim                 // test panel: simulated chat, every rule applies
	fromQuick               // test panel: quick add, map rules only
)

func (a *App) parseOptions(cfg config.Config) parser.Options {
	return parser.Options{Command: cfg.Command.Name, Aliases: cfg.Command.Aliases, BareNumberAs: cfg.Command.BareNumberAs,
		Search: cfg.Command.Search && a.Search != nil}
}

func testUser(user, def string) string {
	if user = strings.TrimPrefix(strings.TrimSpace(user), "@"); user == "" {
		return def
	}
	return user
}

// SimKind says what a simulated chat message turned out to be.
type SimKind string

const (
	SimIgnored  SimKind = "ignored" // neither a request nor a command: real chat ignores it
	SimRequest  SimKind = "request" // a request (see item / rejection)
	SimCmdDone  SimKind = "cmd_done"
	SimCmdNoMod SimKind = "cmd_not_mod"
	SimCmdNoop  SimKind = "cmd_nothing"
)

// SimulateChat feeds a chat message through the same path as TikTok chat
// (commands, requests on/off, per-user limit, cooldown, map rules), but works
// while offline and marks the result as test data.
func (a *App) SimulateChat(user, comment string) (it *queue.Item, rej *validator.Rejection, kind SimKind) {
	cfg := a.Cfg.Get()
	user = testUser(user, "viewer")
	a.speakChat(tikfinity.Chat{User: user, Nick: user, Comment: comment}, true)
	switch a.command(tikfinity.Chat{User: user, Nick: user, Comment: comment}, true) {
	case cmdDone:
		return nil, nil, SimCmdDone
	case cmdNotMod:
		return nil, nil, SimCmdNoMod
	case cmdNothing:
		return nil, nil, SimCmdNoop
	}
	req, ok := parser.Parse(comment, a.parseOptions(cfg))
	if !ok {
		return nil, nil, SimIgnored
	}
	if !cfg.Command.Enabled {
		r := &validator.Rejection{Code: "requests_off", Detail: "requests are turned off"}
		rr := queue.RejectedFrom(user, comment, r)
		rr.Test = true
		a.Queue.Reject(rr)
		return nil, r, SimRequest
	}
	it, rej = a.submit(user, user, comment, req, fromSim, validator.Roles{})
	return it, rej, SimRequest
}

// TestRequest adds a map from the dashboard. It runs the map rules but not
// the per-user limits, and works while offline.
func (a *App) TestRequest(user, input string) (*queue.Item, *validator.Rejection) {
	cfg := a.Cfg.Get()
	user = testUser(user, "dashboard")
	input = strings.TrimSpace(input)
	words := strings.Fields(input)
	if len(words) > 0 && strings.HasPrefix(words[0], "!") {
		words = words[1:] // "!req" alone leaves nothing, not a search for "!req"
	}
	o := a.parseOptions(cfg)
	req, ok := parser.ParseInput(words, o.BareNumberAs, o.Search)
	if !ok {
		r := &validator.Rejection{Code: "bad_input", Detail: "not a beatmap ID, osu! link or song name"}
		rr := queue.RejectedFrom(user, input, r)
		rr.Test = true
		a.Queue.Reject(rr)
		return nil, r
	}
	return a.submit(user, user, input, req, fromQuick, validator.Roles{})
}

func (a *App) submit(user, nick, input string, req parser.Request, o origin, roles validator.Roles) (*queue.Item, *validator.Rejection) {
	a.mu.Lock()
	a.pending++
	a.mu.Unlock()
	defer func() {
		a.mu.Lock()
		a.pending--
		a.mu.Unlock()
		a.Changed()
	}()
	it, rej := a.resolve(user, nick, input, req, o, roles)
	if rej != nil {
		rr := queue.RejectedFrom(user, input, rej)
		rr.Test = o != fromChat
		a.Queue.Reject(rr)
		a.emit(FeedEvent{Kind: "rejected", User: user, Nick: nick, Code: rej.Code, Short: rej.Short, Test: o != fromChat,
			Title: rej.Vars["title"], Artist: rej.Vars["artist"], Mapper: rej.Vars["mapper"], Version: rej.Vars["diff"], Mods: req.Mods, Vars: rej.Vars})
		log.Printf("rejected @%s %q: %s", user, input, rej.Detail)
		return nil, rej
	}
	if a.Cfg.Get().Queue.Fair {
		a.Queue.AddFair(it)
	} else {
		a.Queue.Add(it)
	}
	a.applyCredit(user, nick, o != fromChat)
	a.emit(FeedEvent{Kind: "added", User: user, Nick: nick, Title: it.Title, Artist: it.Artist, Version: it.Version,
		Mapper: it.Creator, SetID: it.SetID, Stars: it.Stars, Mods: it.Mods, Pos: a.Queue.Position(it.ID), Test: o != fromChat})
	log.Printf("queued @%s: %s - %s [%s] +%s (%d)", user, it.Artist, it.Title, it.Version, it.Mods, it.SetID)
	return it, nil
}

func (a *App) resolve(user, nick, input string, req parser.Request, o origin, roles validator.Roles) (_ *queue.Item, rej *validator.Rejection) {
	cfg := a.Cfg.Get()
	now := time.Now()
	st := a.Queue.ValidationState(user)
	if o != fromQuick {
		st.Roles = roles
		if r := validator.CheckUser(user, st, cfg.Rules, cfg.Queue, now); r != nil {
			return nil, r
		}
	} else if cfg.Queue.MaxSize > 0 && st.QueueLen >= cfg.Queue.MaxSize {
		return nil, &validator.Rejection{Code: "queue_full", Detail: fmt.Sprintf("queue is full (%d)", cfg.Queue.MaxSize)}
	}
	// Gift-only requests: a paid request is taken now and given back if the
	// map is then rejected, so two quick requests cannot share one payment.
	if need := cfg.Gifts.RequestCoins; need > 0 && o != fromQuick && !validator.Exempt(cfg.Rules, user) {
		if !a.Queue.TakeRequest(user, now) {
			_, coins := a.Queue.Requests(user, now)
			return nil, &validator.Rejection{Code: "needs_gift", Detail: fmt.Sprintf("requests cost %d gift coins (@%s has %d)", need, user, coins),
				Short: fmt.Sprintf("%d/%d coins", coins, need)}
		}
		defer func() {
			if rej != nil {
				a.Queue.RefundRequest(user)
			}
		}()
	}

	if e := req.ModsErr; e != nil {
		return nil, &validator.Rejection{Code: "bad_mods", Detail: e.Error(), Short: e.Short}
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(max(cfg.MD5.MetaTimeoutSec, 5))*time.Second)
	defer cancel()
	var set *osumeta.Set
	if req.Kind == parser.KindSearch {
		var rej *validator.Rejection
		if set, rej = a.find(ctx, req.Query, req.Mods, st, cfg, now); rej != nil {
			return nil, rej
		}
		req = parser.Request{Kind: parser.KindSet, SetID: set.ID, Mods: req.Mods}
	} else {
		var err error
		set, err = a.lookup(ctx, req)
		if errors.Is(err, osumeta.ErrNotFound) && req.Bare {
			// Viewers often paste a set ID where a beatmap ID is expected (or the
			// reverse); try the other meaning before giving up.
			if s, ferr := a.lookup(ctx, req.Flip()); ferr == nil {
				set, err, req = s, nil, req.Flip()
			}
		}
		if err != nil {
			return nil, metaErr(err)
		}
	}
	beatmapID := 0
	if req.Kind == parser.KindBeatmap {
		beatmapID = req.BeatmapID
	}
	diff, rej := validator.CheckMap(validator.Input{Set: set, BeatmapID: beatmapID, Mode: req.Mode, Mods: req.Mods}, st, cfg.Rules, cfg.Queue, now)
	if rej != nil {
		return nil, rej.WithMap(set)
	}
	mode := diff.Mode
	if req.Mode != "" {
		mode = req.Mode
	}
	it := &queue.Item{
		ID: queue.NewID(), User: user, Nick: nick, SetID: set.ID, BeatmapID: diff.ID, BySet: beatmapID == 0,
		Title: set.Title, Artist: set.Artist, Creator: set.Creator, Version: diff.Version, Mode: mode, Keys: diff.Keys, Convert: diff.Convert,
		Stars: diff.Stars, Length: diff.Length, Status: set.Status, Cover: set.Cover, AddedAt: now, Test: o != fromChat, Simulated: o == fromSim,
		Avatar: a.avatar(user), Mods: req.Mods, ArtistUnicode: set.ArtistUnicode, TitleUnicode: set.TitleUnicode,
	}
	if set.Partial {
		it.Warning = "osu! page unavailable; star rating, length and status were not checked"
	}
	return it, nil
}

// searchTries is how many search results are checked against the rules
// before giving up; each one is an osu! page lookup.
const searchTries = 3

// find turns a song name into the best matching set that passes the map
// rules. If none of the first results passes, the first one's rejection is
// returned with the map's title in front, so the viewer sees what was found.
func (a *App) find(ctx context.Context, query, mods string, st validator.State, cfg config.Config, now time.Time) (*osumeta.Set, *validator.Rejection) {
	rs, err := a.Search.Find(ctx, query, cfg.Rules.Statuses, cfg.Rules.Modes)
	if err != nil {
		return nil, &validator.Rejection{Code: "search_error", Detail: "song search failed: " + err.Error()}
	}
	if len(rs) == 0 {
		return nil, &validator.Rejection{Code: "search_none", Detail: fmt.Sprintf("no map found for %q", query)}
	}
	var first *validator.Rejection
	for i, r := range rs {
		if i == searchTries {
			break
		}
		set, err := a.Meta.Get(ctx, r.SetID, 0)
		if err != nil {
			if first == nil {
				first = metaErr(err)
			}
			continue
		}
		if _, rej := validator.CheckMap(validator.Input{Set: set, Mods: mods}, st, cfg.Rules, cfg.Queue, now); rej != nil {
			if first == nil {
				short := rej.Short
				if !strings.Contains(short, set.Title) { // song bans already name it
					short = strings.TrimSuffix(set.Title+" · "+short, " · ")
				}
				first = (&validator.Rejection{Code: rej.Code, Detail: set.Title + ": " + rej.Detail, Short: short, Vars: rej.Vars}).WithMap(set)
			}
			continue
		}
		log.Printf("search %q -> %s", query, r.Label())
		return set, nil
	}
	return nil, first
}

func (a *App) lookup(ctx context.Context, req parser.Request) (*osumeta.Set, error) {
	setID := req.SetID
	if setID == 0 {
		id, err := a.Meta.ResolveSet(ctx, req.BeatmapID)
		if err != nil {
			return nil, err
		}
		setID = id
	}
	return a.Meta.Get(ctx, setID, req.BeatmapID)
}

func metaErr(err error) *validator.Rejection {
	if errors.Is(err, osumeta.ErrNotFound) {
		return &validator.Rejection{Code: "not_found", Detail: "beatmap not found on osu!"}
	}
	return &validator.Rejection{Code: "meta_error", Detail: err.Error()}
}

// Run drives preloading and auto play until ctx ends.
func (a *App) Run(ctx context.Context) {
	t := time.NewTicker(2 * time.Second)
	defer t.Stop()
	for {
		a.tick(ctx)
		select {
		case <-ctx.Done():
			return
		case <-a.wake:
		case <-t.C:
		}
	}
}

func (a *App) tick(ctx context.Context) {
	a.autoRequests(time.Now())
	cfg := a.Cfg.Get()
	n := 0
	if cfg.Import.Preload {
		n = cfg.Import.PreloadCount
	}
	if cfg.Queue.PlayMode == "auto" {
		n = max(n, 1)
	}
	for _, it := range a.Queue.First(n) {
		if it.State == queue.Queued {
			a.startDownload(ctx, it.ID, cfg.Import.Preload && cfg.Osu.Game == "stable")
			break // one download at a time
		}
		if it.State == queue.Downloading {
			break
		}
	}
	if cfg.Queue.PlayMode == "auto" {
		head, ok := a.Queue.Head()
		if !ok || head.State != queue.Ready {
			return
		}
		// Auto mode only trusts tosu: without it there is no way to know the
		// song has ended, so it waits instead of guessing.
		if a.Tosu == nil || !a.Tosu.Connected() {
			return
		}
		if now, playing := a.Queue.Now(); playing {
			if now.PlayPhase != string(tosu.Ended) || time.Since(now.PhaseAt) < time.Duration(cfg.Queue.AutoGapSec)*time.Second {
				return
			}
		}
		go a.Next("played")
	}
}

// byHand records that a person opened or closed requests, so auto-close
// will not reopen them (even "!reqoff" while they were already closed by it).
func (a *App) byHand() {
	a.mu.Lock()
	a.autoClosed = false
	a.mu.Unlock()
}

// autoRequests closes requests when the queue reaches queue.autoCloseAt,
// opens them again at queue.autoReopenAt (only if it closed them), and
// closes them when the clock passes queue.closeAt. It acts on changes only:
// a streamer who opens requests by hand while the queue is still full, or
// after the closing time, is not overruled.
func (a *App) autoRequests(now time.Time) {
	cfg := a.Cfg.Get()
	q := cfg.Queue
	n := a.Queue.Len()
	a.mu.Lock()
	prev, auto, last := a.lastLen, a.autoClosed, a.lastTick
	a.lastLen, a.lastTick = n, now
	if auto && cfg.Command.Enabled {
		a.autoClosed, auto = false, false // opened by hand meanwhile
	}
	a.mu.Unlock()

	setRequests := func(on bool, code, short string) {
		c := a.Cfg.Get()
		c.Command.Enabled = on
		if err := a.Cfg.Set(c); err != nil {
			log.Printf("auto requests: %v", err)
			return
		}
		a.mu.Lock()
		a.autoClosed = code == "auto_off"
		a.mu.Unlock()
		a.emit(FeedEvent{Kind: "status", Code: code, Short: short})
	}
	switch {
	case q.AutoCloseAt > 0 && cfg.Command.Enabled && n >= q.AutoCloseAt && prev < q.AutoCloseAt:
		setRequests(false, "auto_off", fmt.Sprintf("%d/%d", n, q.AutoCloseAt))
		a.Say("info", "autoClosed", map[string]string{"n": strconv.Itoa(n)}, "requests closed: %d waiting (auto-close)", n)
		return
	case auto && !cfg.Command.Enabled && n <= q.AutoReopenAt:
		setRequests(true, "auto_on", "")
		a.Say("info", "autoReopened", map[string]string{"n": strconv.Itoa(n)}, "requests open again: %d waiting", n)
		return
	}
	if at, ok := config.ClockTime(q.CloseAt); ok && cfg.Command.Enabled && !last.IsZero() {
		y, m, d := now.Date()
		t := time.Date(y, m, d, at/60, at%60, 0, 0, now.Location())
		if last.Before(t) && !now.Before(t) {
			setRequests(false, "time_off", q.CloseAt)
			a.Say("info", "timeClosed", map[string]string{"t": q.CloseAt}, "requests closed: it is %s", q.CloseAt)
		}
	}
}

// OnGame follows tosu's game state to see when the now-playing map has been
// played through (see tosu.Tracker), and records it on the item.
func (a *App) OnGame(g tosu.Game) {
	a.detectGame(g.Client)
	now, ok := a.Queue.Now()
	a.mu.Lock()
	if !ok {
		a.track, a.trackID = tosu.Tracker{}, ""
		a.mu.Unlock()
		return
	}
	if a.trackID != now.ID {
		a.trackID = now.ID
		a.track = tosu.Tracker{SetID: now.SetID, Phase: tosu.Phase(now.PlayPhase), Since: now.PhaseAt}
		if a.track.Phase == "" {
			a.track.Reset(now.SetID, time.Now())
		}
		a.lastPlay, a.lastFailed = tosu.Score{}, false
	}
	before := a.track.Phase
	changed := a.track.Update(g, time.Now())
	phase, since := a.track.Phase, a.track.Since
	if phase == tosu.Playing && g.State == "play" {
		a.lastPlay, a.lastFailed = g.Play, g.Failed // keep the latest live numbers
	}
	var res *queue.Result
	switch {
	case changed && phase == tosu.Results:
		sc := g.Results
		if sc.Score == 0 && sc.Accuracy == 0 {
			sc = a.lastPlay // results block not filled yet: use the last live numbers
		}
		res = toResult("passed", sc)
	case changed && phase == tosu.Ended && before == tosu.Playing && a.lastPlay.Score > 0:
		outcome := "quit"
		if a.lastFailed {
			outcome = "failed"
		}
		res = toResult(outcome, a.lastPlay)
	}
	a.mu.Unlock()
	if !changed {
		return
	}
	if res != nil && now.Mods != "" {
		// osu! cannot be told which mods to use; say when asked-for ones were not played
		if miss := mods.Missing(now.Mods, res.Mods); len(miss) > 0 {
			res.Missing = strings.Join(miss, "")
			a.Say("warn", "modsMissing", map[string]string{"title": now.Artist + " - " + now.Title, "mods": now.Mods, "missing": res.Missing},
				"%s - %s was played without %s (requested +%s)", now.Artist, now.Title, res.Missing, now.Mods)
		}
	}
	a.Queue.Update(now.ID, func(x *queue.Item) {
		x.PlayPhase, x.PhaseAt = string(phase), since
		if res != nil {
			x.Result = res
		}
	})
	log.Printf("tosu: %s - %s is now %s", now.Artist, now.Title, phase)
	if res != nil {
		log.Printf("result for @%s: %s %s", now.User, res.Outcome, ResultShort(res))
		a.emit(FeedEvent{Kind: "result", User: now.User, Nick: now.Nick, Code: res.Outcome, Short: ResultShort(res),
			Title: now.Title, Artist: now.Artist, Version: now.Version, Mapper: now.Creator, Test: now.Test, Vars: map[string]string{
				"acc": strconv.FormatFloat(res.Accuracy, 'f', 2, 64), "rank": res.Rank, "combo": strconv.Itoa(res.MaxCombo),
				"misses": strconv.Itoa(res.Misses), "pp": strconv.Itoa(int(res.PP + 0.5)), "mods": res.Mods}})
	}
	if phase == tosu.Ended {
		a.Say("info", "finished", map[string]string{"title": now.Artist + " - " + now.Title, "u": now.User}, "finished: %s - %s (@%s)", now.Artist, now.Title, now.User)
	}
	a.Wake()
}

// detectGame switches osu.game to the client tosu sees running (tosu's
// ClientType names: stable | lazer), so the streamer does not have to.
func (a *App) detectGame(client string) {
	cfg := a.Cfg.Get()
	if !cfg.Osu.DetectGame || (client != "stable" && client != "lazer") || client == cfg.Osu.Game {
		return
	}
	cfg.Osu.Game = client
	if err := a.Cfg.Set(cfg); err != nil {
		log.Printf("switching osu! game to %s: %v", client, err)
		return
	}
	a.Say("info", "gameDetected", map[string]string{"game": client}, "tosu sees osu! %s running; switched Settings → osu! → Game to %s", client, client)
}

func toResult(outcome string, s tosu.Score) *queue.Result {
	return &queue.Result{Outcome: outcome, Accuracy: s.Accuracy, Rank: s.Rank, MaxCombo: s.MaxCombo,
		Score: s.Score, PP: s.PP, Mods: s.Mods, Misses: s.Misses}
}

// ResultShort formats a result without words, e.g. "98.52% S · 812x · 2 miss · HDHR".
func ResultShort(r *queue.Result) string {
	parts := []string{fmt.Sprintf("%.2f%%", r.Accuracy)}
	if r.Rank != "" && r.Outcome == "passed" {
		parts[0] += " " + r.Rank
	}
	parts = append(parts, fmt.Sprintf("%dx", r.MaxCombo))
	if r.Misses > 0 {
		parts = append(parts, fmt.Sprintf("%d miss", r.Misses))
	}
	if r.PP >= 1 && r.Outcome == "passed" {
		parts = append(parts, fmt.Sprintf("%.0fpp", r.PP))
	}
	if r.Mods != "" && r.Mods != "NM" {
		parts = append(parts, r.Mods)
	}
	return strings.Join(parts, " · ")
}

// startDownload fetches the item in the background. silentImport copies it
// into Songs (stable) so the later "open" is instant.
func (a *App) startDownload(ctx context.Context, id string, silentImport bool) {
	a.mu.Lock()
	if _, busy := a.inflight[id]; busy {
		a.mu.Unlock()
		return
	}
	dctx, cancel := context.WithCancel(ctx)
	a.inflight[id] = cancel
	a.mu.Unlock()
	a.Queue.Update(id, func(it *queue.Item) { it.State, it.Error, it.Attempts = queue.Downloading, "", nil })
	go func() {
		defer func() {
			a.mu.Lock()
			delete(a.inflight, id)
			a.mu.Unlock()
			cancel()
			a.Wake()
		}()
		err := a.download(dctx, id)
		if err == nil && silentImport {
			a.preImport(id)
		}
	}()
}

func (a *App) download(ctx context.Context, id string) error {
	it, ok := a.Queue.Get(id)
	if !ok {
		return errors.New("item removed")
	}
	cfg := a.Cfg.Get()
	mctx, cancel := context.WithTimeout(ctx, time.Duration(max(cfg.MD5.MetaTimeoutSec, 5))*time.Second)
	set, err := a.Meta.Get(mctx, it.SetID, it.BeatmapID)
	cancel()
	ref := verifier.Reference{Checksums: map[int]string{}}
	if !it.BySet {
		ref.RequestedID = it.BeatmapID
	}
	if err == nil {
		for _, d := range set.Diffs {
			ref.Checksums[d.ID] = d.Checksum
		}
	} else if cfg.MD5.Enabled {
		a.fail(id, "could not fetch reference MD5 from osu!: "+err.Error())
		return err
	}

	// Already installed in osu! stable?
	if cfg.MD5.Enabled && cfg.Osu.Game == "stable" && cfg.Osu.UseOsuDB && !it.BySet {
		if sum := ref.Checksums[it.BeatmapID]; sum != "" && a.OsuDB.Has(cfg.Osu.Folder, []string{sum}) {
			a.Queue.Update(id, func(x *queue.Item) {
				x.State, x.Source, x.MD5, x.File = queue.Ready, "osu!.db", "installed", ""
				x.Attempts = []queue.Attempt{{Source: "osu!.db", Status: "ok", Message: "already in osu!"}}
			})
			return nil
		}
	}

	job := downloader.Job{SetID: it.SetID, Ref: ref, HasVideo: set == nil || set.Video}
	out, err := a.DL.Get(ctx, job, func(at []queue.Attempt) {
		a.Queue.Update(id, func(x *queue.Item) { x.Attempts = at })
	})
	if err != nil {
		if ctx.Err() != nil {
			a.Queue.Update(id, func(x *queue.Item) {
				if x.State == queue.Downloading {
					x.State = queue.Queued
				}
			})
			return err
		}
		a.fail(id, err.Error())
		return err
	}
	a.Queue.Update(id, func(x *queue.Item) {
		x.State, x.File, x.Source, x.MD5 = queue.Ready, out.File, out.Source, out.MD5
		if out.Warning != "" {
			x.Warning = out.Warning
		}
	})
	if out.Warning != "" {
		a.Say("warn", "mismatchUsed", map[string]string{"title": it.Artist + " - " + it.Title, "src": out.Source}, "%s - %s: %s", it.Artist, it.Title, out.Warning)
	}
	if cfg.Download.AutoCleanup {
		if n := a.DL.Cleanup(cfg.Download.CacheLimitMB, a.Queue.Files()); n > 0 {
			log.Printf("cache cleanup removed %d file(s)", n)
		}
	}
	return nil
}

func (a *App) fail(id, msg string) {
	a.Queue.Update(id, func(x *queue.Item) { x.State, x.Error = queue.Failed, msg })
	it, _ := a.Queue.Get(id)
	a.Say("error", "failed", map[string]string{"title": it.Artist + " - " + it.Title, "msg": msg}, "%s - %s: %s", it.Artist, it.Title, msg)
}

func (a *App) preImport(id string) {
	it, ok := a.Queue.Get(id)
	if !ok || it.File == "" || it.Imported {
		return
	}
	cfg := a.Cfg.Get()
	if cfg.Test.DryRun {
		return // dry run never touches the Songs folder
	}
	if err := a.Import.Import(it.File, importer.Copy, cfg.Osu.Game, cfg.Osu.Folder, target(it)); err != nil {
		log.Printf("preload import of %d failed: %v", it.SetID, err)
		return
	}
	a.Queue.Update(id, func(x *queue.Item) { x.Imported = true })
}

func target(it queue.Item) importer.Target {
	return importer.Target{SetID: it.SetID, Artist: it.Artist, Title: it.Title}
}

// Next finishes now playing with result ("played" or "skipped") and opens the
// first playable item.
func (a *App) Next(result string) error {
	if !a.advance.TryLock() {
		return uerr("busy", "already opening the next map", nil)
	}
	defer a.advance.Unlock()
	for _, it := range a.Queue.First(1 << 30) {
		if it.State == queue.Failed {
			continue
		}
		return a.play(it.ID, result)
	}
	a.Queue.Finish(result)
	return uerr("queue_empty", "the queue is empty", nil)
}

// Play opens a specific queued item now.
func (a *App) Play(id string) error {
	if !a.advance.TryLock() {
		return uerr("busy", "already opening a map", nil)
	}
	defer a.advance.Unlock()
	return a.play(id, "played")
}

func (a *App) play(id, result string) error {
	it, ok := a.Queue.Get(id)
	if !ok {
		return uerr("not_found", "request not found", nil)
	}
	if it.State != queue.Ready {
		a.mu.Lock()
		cancel := a.inflight[id]
		a.mu.Unlock()
		if cancel != nil {
			// a preload is running: wait for it rather than starting a second download
			for i := 0; i < 1200; i++ {
				time.Sleep(250 * time.Millisecond)
				a.mu.Lock()
				_, busy := a.inflight[id]
				a.mu.Unlock()
				if !busy {
					break
				}
			}
		} else {
			a.Queue.Update(id, func(x *queue.Item) { x.State, x.Error, x.Attempts = queue.Downloading, "", nil })
			a.mu.Lock()
			ctx, c := context.WithCancel(context.Background())
			a.inflight[id] = c
			a.mu.Unlock()
			a.download(ctx, id)
			a.mu.Lock()
			delete(a.inflight, id)
			a.mu.Unlock()
			c()
		}
		if it, ok = a.Queue.Get(id); !ok {
			return uerr("not_found", "request was removed", nil)
		}
		if it.State != queue.Ready {
			return uerr("get_failed", fmt.Sprintf("could not get %s - %s: %s", it.Artist, it.Title, it.Error), map[string]string{"title": it.Artist + " - " + it.Title, "msg": it.Error})
		}
	}
	cfg := a.Cfg.Get()
	var err error
	switch {
	case cfg.Test.DryRun:
		a.Say("info", "dryOpen", map[string]string{"title": it.Artist + " - " + it.Title + " [" + it.Version + "]"}, "dry run: would open %s - %s [%s] (%s)", it.Artist, it.Title, it.Version, cfg.Import.Mode)
	case it.MD5 == "installed":
		err = a.OpenURL(fmt.Sprintf("osu://b/%d", it.BeatmapID))
	case it.Imported || cfg.Osu.Game == "lazer":
		// preloaded maps are already in Songs; opening the .osz jumps straight to them
		err = a.Import.Import(it.File, importer.Open, cfg.Osu.Game, cfg.Osu.Folder, target(it))
	default:
		err = a.Import.Import(it.File, importer.Mode(cfg.Import.Mode), cfg.Osu.Game, cfg.Osu.Folder, target(it))
	}
	if err != nil {
		a.fail(id, "import failed: "+err.Error())
		return err
	}
	a.Queue.Promote(id, result)
	a.announcePlaying(it)
	log.Printf("now playing: %s - %s [%s] for @%s", it.Artist, it.Title, it.Version, it.User)
	return nil
}

// Skip removes a queued item (recording it as skipped) or, with id "", skips now playing.
func (a *App) Skip(id string) error {
	if id == "" {
		return a.Next("skipped")
	}
	a.Cancel(id)
	if !a.Queue.Drop(id, "skipped") {
		return uerr("not_found", "request not found", nil)
	}
	return nil
}

func (a *App) Remove(id string) error {
	a.Cancel(id)
	if !a.Queue.Remove(id) {
		return uerr("not_found", "request not found", nil)
	}
	return nil
}

func (a *App) RemoveLast() error {
	it, ok := a.Queue.RemoveLast()
	if !ok {
		return uerr("queue_empty", "the queue is empty", nil)
	}
	a.Cancel(it.ID)
	a.Say("info", "removedLast", map[string]string{"title": it.Artist + " - " + it.Title, "u": it.User}, "removed %s - %s (@%s)", it.Artist, it.Title, it.User)
	return nil
}

func (a *App) Retry(id string) {
	a.Queue.Update(id, func(x *queue.Item) {
		if x.State == queue.Failed {
			x.State, x.Error, x.Attempts = queue.Queued, "", nil
		}
	})
	a.Wake()
}

func (a *App) Cancel(id string) {
	a.mu.Lock()
	c := a.inflight[id]
	a.mu.Unlock()
	if c != nil {
		c()
	}
}

func (a *App) ToggleRequests() bool {
	a.byHand()
	c := a.Cfg.Get()
	c.Command.Enabled = !c.Command.Enabled
	a.Cfg.Set(c)
	if c.Command.Enabled {
		a.Say("info", "reqOn", nil, "requests ON")
	} else {
		a.Say("info", "reqOff", nil, "requests OFF")
	}
	return c.Command.Enabled
}

func (a *App) Resolving() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.pending
}

// SetUpdate records a newer osureq release; Activity mentions each version once.
func (a *App) SetUpdate(r update.Release) {
	a.mu.Lock()
	seen := a.latest != nil && a.latest.Version == r.Version
	a.latest = &r
	a.mu.Unlock()
	if !seen {
		a.Say("info", "update", map[string]string{"v": r.Version}, "osureq %s is available: %s", r.Version, r.URL)
	}
}

// Update returns the newer release, or nil.
func (a *App) Update() *update.Release {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.latest
}

// ShowSummary asks the overlay to show the session summary for a while.
func (a *App) ShowSummary() {
	a.mu.Lock()
	a.summaryN++
	a.mu.Unlock()
	a.Changed()
}

// SummaryShown counts ShowSummary calls; the overlay shows the card when it changes.
func (a *App) SummaryShown() int64 {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.summaryN
}

// NewSession starts a new stream session for the summary.
func (a *App) NewSession() {
	a.Queue.NewSession(time.Now())
	a.Say("info", "newSession", nil, "started a new stream session")
}

// ClearTest removes all test-panel data and cancels its downloads.
func (a *App) ClearTest() int {
	ids := a.Queue.ClearTest()
	for _, id := range ids {
		a.Cancel(id)
	}
	a.Say("info", "clearedTest", map[string]string{"n": strconv.Itoa(len(ids))}, "cleared test data (%d request(s))", len(ids))
	return len(ids)
}

// SetVolume stores the dashboard's master volume (0-100).
func (a *App) SetVolume(v int) error {
	c := a.Cfg.Get()
	c.MasterVolume = max(0, min(100, v))
	return a.Cfg.Set(c)
}

func (a *App) ToggleDryRun() bool {
	c := a.Cfg.Get()
	c.Test.DryRun = !c.Test.DryRun
	a.Cfg.Set(c)
	if c.Test.DryRun {
		a.Say("info", "dryOn", nil, "dry run ON: Next/Play will not open osu!")
	} else {
		a.Say("info", "dryOff", nil, "dry run OFF")
	}
	return c.Test.DryRun
}

// Hotkey dispatches a global hotkey.
func (a *App) Hotkey(name string) {
	var err error
	switch name {
	case "next":
		err = a.Next("played")
	case "skip":
		err = a.Skip("")
	case "removeLast":
		err = a.RemoveLast()
	case "toggleRequests":
		a.ToggleRequests()
	}
	if err != nil {
		a.Say("warn", "hotkeyErr", map[string]string{"key": name, "msg": err.Error()}, "%s: %v", name, err)
	}
}
