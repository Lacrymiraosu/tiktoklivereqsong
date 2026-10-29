// Package app wires the modules together: chat -> parser -> osuMeta ->
// validator -> queue -> downloader/verifier -> importer.
package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/draft/osureq/internal/config"
	"github.com/draft/osureq/internal/downloader"
	"github.com/draft/osureq/internal/importer"
	"github.com/draft/osureq/internal/osumeta"
	"github.com/draft/osureq/internal/parser"
	"github.com/draft/osureq/internal/queue"
	"github.com/draft/osureq/internal/tikfinity"
	"github.com/draft/osureq/internal/tosu"
	"github.com/draft/osureq/internal/validator"
	"github.com/draft/osureq/internal/verifier"
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
	OpenURL func(string) error

	changed  func()
	wake     chan struct{}
	mu       sync.Mutex
	inflight map[string]context.CancelFunc // item id -> download cancel
	notices  []Notice
	advance  sync.Mutex
	pending  int // requests being resolved
	track    tosu.Tracker
	trackID  string // queue item the tracker follows
	feed     []FeedEvent
	feedSeq  int64
}

type Notice struct {
	At   time.Time `json:"at"`
	Kind string    `json:"kind"` // info | warn | error
	Text string    `json:"text"`
}

func New(cfg *config.Store, q *queue.Queue, meta *osumeta.Client, dl *downloader.Downloader, im *importer.Importer, tf *tikfinity.Client, openURL func(string) error) *App {
	a := &App{Cfg: cfg, Queue: q, Meta: meta, DL: dl, Import: im, TF: tf, OsuDB: &importer.OsuDB{}, OpenURL: openURL,
		wake: make(chan struct{}, 1), inflight: map[string]context.CancelFunc{}}
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

func (a *App) Notify(kind, f string, args ...any) {
	n := Notice{At: time.Now(), Kind: kind, Text: fmt.Sprintf(f, args...)}
	log.Printf("[%s] %s", kind, n.Text)
	a.mu.Lock()
	a.notices = append([]Notice{n}, a.notices...)
	if len(a.notices) > 50 {
		a.notices = a.notices[:50]
	}
	a.mu.Unlock()
	a.Changed()
}

func (a *App) Notices() []Notice {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]Notice(nil), a.notices...)
}

// HandleChat is called for every TikFinity chat event.
func (a *App) HandleChat(ch tikfinity.Chat) {
	cfg := a.Cfg.Get()
	req, ok := parser.Parse(ch.Comment, parser.Options{Command: cfg.Command.Name, Aliases: cfg.Command.Aliases, BareNumberAs: cfg.Command.BareNumberAs})
	if !ok {
		return
	}
	if !cfg.Command.Enabled {
		a.Queue.Reject(queue.Rejected{At: time.Now(), User: ch.User, Input: ch.Comment, Code: "requests_off", Detail: "requests are turned off"})
		a.emit(FeedEvent{Kind: "rejected", User: ch.User, Nick: ch.Nick, Code: "requests_off"})
		return
	}
	if !a.TF.Accepting() {
		return
	}
	go a.submit(ch.User, ch.Nick, ch.Comment, req, fromChat)
}

// origin says where a request came from.
type origin int

const (
	fromChat  origin = iota // TikTok chat via TikFinity
	fromSim                 // test panel: simulated chat, every rule applies
	fromQuick               // test panel: quick add, map rules only
)

func testUser(user, def string) string {
	if user = strings.TrimPrefix(strings.TrimSpace(user), "@"); user == "" {
		return def
	}
	return user
}

// SimulateChat feeds a chat message through the same path as TikTok chat
// (command, requests on/off, per-user limit, cooldown, map rules), but works
// while offline and marks the result as test data. matched is false when the
// message is not a request at all, which real chat would silently ignore.
func (a *App) SimulateChat(user, comment string) (it *queue.Item, rej *validator.Rejection, matched bool) {
	cfg := a.Cfg.Get()
	user = testUser(user, "viewer")
	req, ok := parser.Parse(comment, parser.Options{Command: cfg.Command.Name, Aliases: cfg.Command.Aliases, BareNumberAs: cfg.Command.BareNumberAs})
	if !ok {
		return nil, nil, false
	}
	if !cfg.Command.Enabled {
		r := &validator.Rejection{Code: "requests_off", Detail: "requests are turned off"}
		rr := queue.RejectedFrom(user, comment, r)
		rr.Test = true
		a.Queue.Reject(rr)
		return nil, r, true
	}
	it, rej = a.submit(user, user, comment, req, fromSim)
	return it, rej, true
}

// TestRequest adds a map from the dashboard. It runs the map rules but not
// the per-user limits, and works while offline.
func (a *App) TestRequest(user, input string) (*queue.Item, *validator.Rejection) {
	cfg := a.Cfg.Get()
	user = testUser(user, "dashboard")
	input = strings.TrimSpace(input)
	arg := input
	if f := strings.Fields(input); len(f) >= 2 && strings.HasPrefix(f[0], "!") {
		arg = f[1]
	}
	req, ok := parser.ParseArg(arg, cfg.Command.BareNumberAs)
	if !ok {
		r := &validator.Rejection{Code: "bad_input", Detail: "not a beatmap ID or osu! link"}
		rr := queue.RejectedFrom(user, input, r)
		rr.Test = true
		a.Queue.Reject(rr)
		return nil, r
	}
	return a.submit(user, user, input, req, fromQuick)
}

func (a *App) submit(user, nick, input string, req parser.Request, o origin) (*queue.Item, *validator.Rejection) {
	a.mu.Lock()
	a.pending++
	a.mu.Unlock()
	defer func() {
		a.mu.Lock()
		a.pending--
		a.mu.Unlock()
		a.Changed()
	}()
	it, rej := a.resolve(user, nick, input, req, o)
	if rej != nil {
		rr := queue.RejectedFrom(user, input, rej)
		rr.Test = o != fromChat
		a.Queue.Reject(rr)
		a.emit(FeedEvent{Kind: "rejected", User: user, Nick: nick, Code: rej.Code, Short: rej.Short, Test: o != fromChat})
		log.Printf("rejected @%s %q: %s", user, input, rej.Detail)
		return nil, rej
	}
	a.Queue.Add(it)
	a.emit(FeedEvent{Kind: "added", User: user, Nick: nick, Title: it.Title, Artist: it.Artist, Version: it.Version,
		Stars: it.Stars, Pos: a.Queue.Position(it.ID), Test: o != fromChat})
	log.Printf("queued @%s: %s - %s [%s] (%d)", user, it.Artist, it.Title, it.Version, it.SetID)
	return it, nil
}

func (a *App) resolve(user, nick, input string, req parser.Request, o origin) (*queue.Item, *validator.Rejection) {
	cfg := a.Cfg.Get()
	now := time.Now()
	st := a.Queue.ValidationState(user)
	if o != fromQuick {
		if r := validator.CheckUser(user, st, cfg.Rules, cfg.Queue, now); r != nil {
			return nil, r
		}
	} else if cfg.Queue.MaxSize > 0 && st.QueueLen >= cfg.Queue.MaxSize {
		return nil, &validator.Rejection{Code: "queue_full", Detail: fmt.Sprintf("queue is full (%d)", cfg.Queue.MaxSize)}
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(max(cfg.MD5.MetaTimeoutSec, 5))*time.Second)
	defer cancel()
	set, err := a.lookup(ctx, req)
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
	beatmapID := 0
	if req.Kind == parser.KindBeatmap {
		beatmapID = req.BeatmapID
	}
	diff, rej := validator.CheckMap(validator.Input{Set: set, BeatmapID: beatmapID, Mode: req.Mode}, st, cfg.Rules, cfg.Queue, now)
	if rej != nil {
		return nil, rej
	}
	mode := diff.Mode
	if req.Mode != "" {
		mode = req.Mode
	}
	it := &queue.Item{
		ID: queue.NewID(), User: user, Nick: nick, SetID: set.ID, BeatmapID: diff.ID, BySet: beatmapID == 0,
		Title: set.Title, Artist: set.Artist, Creator: set.Creator, Version: diff.Version, Mode: mode,
		Stars: diff.Stars, Length: diff.Length, Status: set.Status, Cover: set.Cover, AddedAt: now, Test: o != fromChat, Simulated: o == fromSim,
	}
	if set.Partial {
		it.Warning = "osu! page unavailable; star rating, length and status were not checked"
	}
	return it, nil
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

// OnGame follows tosu's game state to see when the now-playing map has been
// played through (see tosu.Tracker), and records it on the item.
func (a *App) OnGame(g tosu.Game) {
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
	}
	changed := a.track.Update(g, time.Now())
	phase, since := a.track.Phase, a.track.Since
	a.mu.Unlock()
	if !changed {
		return
	}
	a.Queue.Update(now.ID, func(x *queue.Item) { x.PlayPhase, x.PhaseAt = string(phase), since })
	log.Printf("tosu: %s - %s is now %s", now.Artist, now.Title, phase)
	if phase == tosu.Ended {
		a.Notify("info", "finished: %s - %s (@%s)", now.Artist, now.Title, now.User)
	}
	a.Wake()
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
		a.Notify("warn", "%s - %s: %s", it.Artist, it.Title, out.Warning)
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
	a.Notify("error", "%s - %s: %s", it.Artist, it.Title, msg)
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
		return errors.New("already opening the next map")
	}
	defer a.advance.Unlock()
	for _, it := range a.Queue.First(1 << 30) {
		if it.State == queue.Failed {
			continue
		}
		return a.play(it.ID, result)
	}
	a.Queue.Finish(result)
	return errors.New("the queue is empty")
}

// Play opens a specific queued item now.
func (a *App) Play(id string) error {
	if !a.advance.TryLock() {
		return errors.New("already opening a map")
	}
	defer a.advance.Unlock()
	return a.play(id, "played")
}

func (a *App) play(id, result string) error {
	it, ok := a.Queue.Get(id)
	if !ok {
		return errors.New("request not found")
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
			return errors.New("request was removed")
		}
		if it.State != queue.Ready {
			return fmt.Errorf("could not get %s - %s: %s", it.Artist, it.Title, it.Error)
		}
	}
	cfg := a.Cfg.Get()
	var err error
	switch {
	case cfg.Test.DryRun:
		a.Notify("info", "dry run: would open %s - %s [%s] (%s)", it.Artist, it.Title, it.Version, cfg.Import.Mode)
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
		return errors.New("request not found")
	}
	return nil
}

func (a *App) Remove(id string) error {
	a.Cancel(id)
	if !a.Queue.Remove(id) {
		return errors.New("request not found")
	}
	return nil
}

func (a *App) RemoveLast() error {
	it, ok := a.Queue.RemoveLast()
	if !ok {
		return errors.New("the queue is empty")
	}
	a.Cancel(it.ID)
	a.Notify("info", "removed %s - %s (@%s)", it.Artist, it.Title, it.User)
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
	c := a.Cfg.Get()
	c.Command.Enabled = !c.Command.Enabled
	a.Cfg.Set(c)
	if c.Command.Enabled {
		a.Notify("info", "requests ON")
	} else {
		a.Notify("info", "requests OFF")
	}
	return c.Command.Enabled
}

func (a *App) Resolving() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.pending
}

// ClearTest removes all test-panel data and cancels its downloads.
func (a *App) ClearTest() int {
	ids := a.Queue.ClearTest()
	for _, id := range ids {
		a.Cancel(id)
	}
	a.Notify("info", "cleared %d test request(s)", len(ids))
	return len(ids)
}

func (a *App) ToggleDryRun() bool {
	c := a.Cfg.Get()
	c.Test.DryRun = !c.Test.DryRun
	a.Cfg.Set(c)
	if c.Test.DryRun {
		a.Notify("info", "dry run ON: Next/Play will not open osu!")
	} else {
		a.Notify("info", "dry run OFF")
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
		a.Notify("warn", "%s: %v", name, err)
	}
}
