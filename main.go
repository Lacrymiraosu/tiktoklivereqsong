// osureq: TikTok LIVE (via TikFinity) -> osu! beatmap request helper.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/app"
	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/config"
	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/downloader"
	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/importer"
	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/osumeta"
	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/platform"
	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/queue"
	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/remote"
	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/search"
	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/server"
	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/tikfinity"
	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/tosu"
	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/update"
)

var version = "dev"

func main() {
	platform.SetConsoleTitle("osureq " + version)
	dirFlag := flag.String("dir", "", "folder for config.json, queue.json, cache and log (default: next to the exe)")
	noBrowser := flag.Bool("no-browser", false, "do not open the dashboard on start")
	restarted := flag.Bool("restarted", false, "started again after installing an update: wait for the port and do not open the dashboard")
	flag.Parse()

	base := *dirFlag
	if base == "" {
		base = exeDir()
	}
	if err := os.MkdirAll(base, 0o755); err != nil {
		fatal(err)
	}
	setupLog(filepath.Join(base, "osureq.log"))
	log.Printf("osureq %s starting in %s", version, base)

	store, err := config.Load(filepath.Join(base, "config.json"))
	if err != nil {
		fatal(err)
	}
	cfg := store.Get()

	// An update downloaded by the last run is installed before anything starts.
	exe, _ := os.Executable()
	if exe != "" && cfg.Connection.CheckUpdates && cfg.Connection.AutoUpdate && installUpdate(exe, os.Args[1:]) {
		return
	}
	if exe != "" {
		update.Cleanup(exe, version)
		// the copy that installed this one may still be closing and hold the old exe
		time.AfterFunc(15*time.Second, func() { os.Remove(exe + ".old") })
	}

	if cfg.Osu.Game == "stable" {
		if _, err := importer.SongsDir(cfg.Osu.Folder); err != nil {
			if dir, ok := platform.FindOsuStable(); ok && !strings.EqualFold(dir, cfg.Osu.Folder) {
				if _, err := importer.SongsDir(dir); err == nil {
					log.Printf("osu! folder %s has no Songs folder; using %s (from the .osz file association)", cfg.Osu.Folder, dir)
					cfg.Osu.Folder = dir
					if err := store.Set(cfg); err != nil {
						log.Printf("saving detected osu! folder: %v", err)
					}
				}
			}
		}
	}

	q, err := queue.Open(filepath.Join(base, "queue.json"))
	if err != nil {
		fatal(fmt.Errorf("queue.json: %w", err))
	}

	cacheDir := func() string {
		d := store.Get().Osu.CacheFolder
		if !filepath.IsAbs(d) {
			d = filepath.Join(base, d)
		}
		return d
	}
	dl := downloader.New(store.Get, cacheDir(), platform.ShellOpen)
	im := &importer.Importer{Trusted: dl.Trusted, ShellOpen: platform.ShellOpen}

	tf := tikfinity.New(tikfinity.Env{Find: platform.FindTikFinity, Running: platform.ProcessRunning, Launch: platform.Launch, Close: platform.Close},
		func() tikfinity.Settings {
			c := store.Get().Connection
			return tikfinity.Settings{URL: c.TikFinityURL, Path: c.TikFinityPath, Reconnect: time.Duration(c.ReconnectSec) * time.Second, AutoLaunch: c.AutoLaunch, LogRaw: c.LogRawEvents}
		})

	a := app.New(store, q, osumeta.New(), dl, im, tf, platform.ShellOpen)
	a.Search = search.New()
	tf.OnChat = a.HandleChat
	tf.OnGift = a.HandleGift
	srv := server.New(a, version)
	srv.LogPath = filepath.Join(base, "osureq.log")
	a.OnChange(srv.Push)
	ts := tosu.New(tosu.Env{Running: platform.ProcessRunning, Path: platform.ProcessPath, Launch: platform.Launch, Close: platform.Close},
		func() tosu.Settings {
			c := store.Get().Connection
			return tosu.Settings{URL: c.TosuURL, Path: c.TosuPath, Reconnect: time.Duration(c.ReconnectSec) * time.Second, AutoLaunch: c.AutoLaunchTosu}
		})
	a.Tosu = ts
	ts.OnGame = a.OnGame
	ts.OnStatus = func(st tosu.Status) {
		log.Printf("tosu: %s (%s) state=%s set=%d %s", st.Level, st.Code, st.Game.State, st.Game.SetID, st.Error)
		srv.Push()
	}
	ts.OnPath = func(p string) { // remember where tosu.exe lives so it can be launched later
		c := store.Get()
		c.Connection.TosuPath = p
		if err := store.Set(c); err != nil {
			log.Printf("saving tosu path: %v", err)
		}
	}
	tf.OnStatus = func(st tikfinity.Status) {
		log.Printf("TikFinity: %s (%s) %s", st.Level, st.Code, st.Error)
		srv.Push()
	}

	var hk platform.Hotkeys
	var hkMu sync.Mutex
	hkErrs := map[string]string{}
	applyHotkeys := func(c config.Config) {
		hkMu.Lock()
		defer hkMu.Unlock()
		hkErrs = map[string]string{}
		if !c.Hotkeys.Enabled {
			hk.Stop()
			return
		}
		failed := hk.Start(map[string]string{"next": c.Hotkeys.Next, "skip": c.Hotkeys.Skip, "removeLast": c.Hotkeys.Remove, "toggleRequests": c.Hotkeys.Toggle}, a.Hotkey)
		for k, v := range failed {
			hkErrs[k] = v.Error()
			log.Printf("hotkey %s: %v", k, v)
		}
	}
	applyHotkeys(cfg)

	mods := remote.New(a, func() config.Remote { return store.Get().Remote })

	prev := cfg
	store.OnChange(func(c config.Config) {
		if c.Remote != prev.Remote {
			mods.Apply(c.Remote)
		}
		if c.Hotkeys != prev.Hotkeys {
			applyHotkeys(c)
		}
		if c.Connection.TikFinityURL != prev.Connection.TikFinityURL || c.Connection.TikFinityPath != prev.Connection.TikFinityPath {
			tf.Reconnect()
		}
		if c.Connection.TosuURL != prev.Connection.TosuURL {
			ts.Reconnect()
		}
		dl.CacheDir = cacheDir()
		prev = c
		srv.Push()
	})
	srv.Extra = func() map[string]any {
		hkMu.Lock()
		defer hkMu.Unlock()
		errs := map[string]string{}
		for k, v := range hkErrs {
			errs[k] = v
		}
		return map[string]any{"hotkeyErrors": errs, "baseDir": base, "cacheDir": dl.CacheDir, "remote": mods.Status()}
	}

	// Ctrl+C, or closing the CMD window (Go reports that as SIGTERM), stops cleanly.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	port := cfg.Connection.DashboardPort
	url := fmt.Sprintf("http://127.0.0.1:%d/", port)
	err = srv.Listen(ctx, port)
	for i := 0; err != nil && *restarted && i < 30; i++ { // the copy that installed this update is still closing
		time.Sleep(500 * time.Millisecond)
		err = srv.Listen(ctx, port)
	}
	if err != nil {
		if alreadyRunning(url) {
			// Started twice: show the running copy instead of an error.
			log.Printf("another osureq is already running on %s; opening its dashboard", url)
			fmt.Printf("\n  osureq is already running (see its other window).\n  Opening its dashboard: %s\n\n", url)
			if !*noBrowser {
				platform.ShellOpen(url)
			}
			time.Sleep(4 * time.Second) // long enough to read the message
			return
		}
		fatal(fmt.Errorf("cannot listen on 127.0.0.1:%d (another program uses this port; change dashboardPort in config.json): %w", port, err))
	}
	srv.Quit = stop
	var restart atomic.Bool
	srv.Restart = func() { restart.Store(true); stop() }
	log.Printf("dashboard: %s   overlay: %soverlay", url, url)
	fmt.Printf("\n  osureq %s\n  Dashboard:     %s\n  OBS overlay:   %soverlay\n\n  Keep this window open while you stream. Close it (or press Ctrl+C) to stop.\n\n", version, url, url)
	if cfg.Connection.OpenDashboard && !*noBrowser && !*restarted {
		platform.ShellOpen(url)
	}

	mods.Apply(cfg.Remote)
	go tf.Run(ctx)
	go ts.Run(ctx)
	go a.Run(ctx)
	go a.RunMirrorChecks(ctx)
	var staging atomic.Bool
	go update.Run(ctx, version, func() bool { return store.Get().Connection.CheckUpdates }, func(r update.Release) {
		a.SetUpdate(r)
		if exe != "" && store.Get().Connection.AutoUpdate && staging.CompareAndSwap(false, true) {
			go func() {
				defer staging.Store(false)
				stageUpdate(ctx, exe, r, a)
			}()
		}
	})
	<-ctx.Done()
	mods.Apply(config.Remote{}) // stop the mod panel
	hk.Stop()
	q.Flush()
	if restart.Load() {
		args := os.Args[1:]
		if !slices.Contains(args, "-restarted") {
			args = append(args, "-restarted")
		}
		if installUpdate(exe, args) {
			return
		}
	}
	log.Printf("bye")
}

// stageUpdate downloads r next to the exe, where installUpdate finds it on
// the next start, and marks it ready in the dashboard.
func stageUpdate(ctx context.Context, exe string, r update.Release, a *app.App) {
	if v, _ := update.Staged(filepath.Dir(exe)); v != r.Version {
		log.Printf("update: downloading osureq %s", r.Version)
		hc := &http.Client{Timeout: 5 * time.Minute}
		if _, err := update.Download(ctx, hc, r, filepath.Dir(exe)); err != nil {
			a.Say("warn", "updateFailed", map[string]string{"v": r.Version, "err": err.Error()}, "could not download osureq %s: %v", r.Version, err)
			return
		}
	}
	r.Ready = true
	a.SetUpdate(r)
}

// installUpdate puts a downloaded update in place of exe and starts it in
// this console with args. It reports whether the new copy took over.
func installUpdate(exe string, args []string) bool {
	v, staged := update.Staged(filepath.Dir(exe))
	if staged == "" || !update.Newer(v, version) {
		return false
	}
	log.Printf("installing osureq %s", v)
	fmt.Printf("\n  Installing osureq %s ...\n", v)
	if err := update.Install(exe, staged); err != nil {
		log.Printf("update: %v", err)
		return false
	}
	if err := update.Relaunch(exe, args); err != nil {
		log.Printf("starting osureq %s: %v", v, err)
		return false
	}
	log.Printf("bye (osureq %s takes over)", v)
	return true
}

func exeDir() string {
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	dir := filepath.Dir(exe)
	// `go run` builds into a temp folder; keep state in the working directory instead.
	if strings.HasPrefix(strings.ToLower(dir), strings.ToLower(os.TempDir())) || strings.Contains(strings.ToLower(dir), "go-build") {
		if wd, err := os.Getwd(); err == nil {
			return wd
		}
	}
	return dir
}

func setupLog(path string) {
	if st, err := os.Stat(path); err == nil && st.Size() > 5<<20 {
		os.Rename(path, path+".old")
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	log.SetOutput(io.MultiWriter(f, os.Stderr)) // file first: a failing console must not lose log lines
	log.SetFlags(log.LstdFlags)
}

func fatal(err error) {
	log.Printf("fatal: %v", err)
	fmt.Fprintf(os.Stderr, "\nError: %v\n\nPress Enter to exit.", err)
	fmt.Scanln()
	os.Exit(1)
}

// alreadyRunning reports whether the dashboard URL is served by an osureq.
func alreadyRunning(url string) bool {
	c := &http.Client{Timeout: 2 * time.Second}
	resp, err := c.Get(url + "api/state")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	var st struct {
		Version string `json:"version"`
	}
	return resp.StatusCode == http.StatusOK && json.NewDecoder(resp.Body).Decode(&st) == nil && st.Version != ""
}
