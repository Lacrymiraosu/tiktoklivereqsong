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
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/draft/osureq/internal/app"
	"github.com/draft/osureq/internal/config"
	"github.com/draft/osureq/internal/downloader"
	"github.com/draft/osureq/internal/importer"
	"github.com/draft/osureq/internal/osumeta"
	"github.com/draft/osureq/internal/platform"
	"github.com/draft/osureq/internal/queue"
	"github.com/draft/osureq/internal/server"
	"github.com/draft/osureq/internal/tikfinity"
	"github.com/draft/osureq/internal/tosu"
)

var version = "dev"

func main() {
	platform.SetConsoleTitle("osureq " + version)
	dirFlag := flag.String("dir", "", "folder for config.json, queue.json, cache and log (default: next to the exe)")
	noBrowser := flag.Bool("no-browser", false, "do not open the dashboard on start")
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
	tf.OnChat = a.HandleChat
	tf.OnGift = a.HandleGift
	srv := server.New(a, version)
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

	prev := cfg
	store.OnChange(func(c config.Config) {
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
		return map[string]any{"hotkeyErrors": errs, "baseDir": base, "cacheDir": dl.CacheDir}
	}

	// Ctrl+C, or closing the CMD window (Go reports that as SIGTERM), stops cleanly.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	port := cfg.Connection.DashboardPort
	url := fmt.Sprintf("http://127.0.0.1:%d/", port)
	if err := srv.Listen(ctx, port); err != nil {
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
	log.Printf("dashboard: %s   overlay: %soverlay", url, url)
	fmt.Printf("\n  osureq %s\n  Dashboard:     %s\n  OBS overlay:   %soverlay\n\n  Keep this window open while you stream. Close it (or press Ctrl+C) to stop.\n\n", version, url, url)
	if cfg.Connection.OpenDashboard && !*noBrowser {
		platform.ShellOpen(url)
	}

	go tf.Run(ctx)
	go ts.Run(ctx)
	go a.Run(ctx)
	<-ctx.Done()
	hk.Stop()
	q.Flush()
	log.Printf("bye")
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
