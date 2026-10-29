package app

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/downloader"
	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/importer"
)

// probeSet is the beatmapset the pre-stream check looks up and asks the
// mirrors for: xi - FREEDOM DiVE, approved since 2012 and on every mirror.
const probeSet = 39804

// Check is one line of the pre-stream check. The dashboard shows it in its
// own language as T["pf_"+Key] with Vars; Detail is the raw error, if any.
type Check struct {
	ID     string            `json:"id"`
	Status string            `json:"status"` // ok | warn | fail | off
	Key    string            `json:"key"`
	Vars   map[string]string `json:"vars,omitempty"`
	Detail string            `json:"detail,omitempty"`
}

// PreflightEnv is what only the caller knows: hotkey registration errors
// (platform) and how many OBS overlays are connected (server).
type PreflightEnv struct {
	HotkeyErrors   map[string]string
	OverlayClients int
}

// Preflight runs every check a streamer would otherwise do by hand before
// going live. It reads only: nothing is downloaded to the cache, opened in
// osu! or changed.
func (a *App) Preflight(ctx context.Context, env PreflightEnv) []Check {
	cfg := a.Cfg.Get()
	var out []Check
	add := func(c Check) { out = append(out, c) }

	// network checks run side by side while the local ones are done
	var wg sync.WaitGroup
	var web Check
	var mirrors []Check
	if a.Meta != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			mctx, cancel := context.WithTimeout(ctx, 15*time.Second)
			defer cancel()
			web = Check{ID: "osuweb", Status: "ok", Key: "osuweb_ok"}
			if _, err := a.Meta.Get(mctx, probeSet, 0); err != nil {
				web = Check{ID: "osuweb", Status: "fail", Key: "osuweb_fail", Detail: err.Error()}
			}
		}()
	}
	if a.DL != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			mirrors = mirrorChecks(a.DL.Probe(ctx, probeSet))
		}()
	}

	if a.TF != nil {
		st := a.TF.Status()
		switch {
		case st.Level == "green":
			add(Check{ID: "tikfinity", Status: "ok", Key: "tf_live"})
		case st.Code == "waiting_stream":
			add(Check{ID: "tikfinity", Status: "ok", Key: "tf_ready"})
		default:
			add(Check{ID: "tikfinity", Status: "fail", Key: "tf_" + orConnecting(st.Code), Detail: st.Error})
		}
	}
	if a.Tosu != nil {
		st := a.Tosu.Status()
		switch {
		case st.Level == "green":
			add(Check{ID: "tosu", Status: "ok", Key: "tosu_ok", Vars: map[string]string{"client": st.Game.Client}})
		case st.Code == "no_game":
			add(Check{ID: "tosu", Status: "warn", Key: "tosu_no_game"})
		default:
			add(Check{ID: "tosu", Status: "fail", Key: "tosu_" + orConnecting(st.Code), Detail: st.Error})
		}
	}
	if cfg.Osu.Game == "lazer" {
		add(Check{ID: "game", Status: "ok", Key: "game_lazer"})
	} else if dir, err := importer.SongsDir(cfg.Osu.Folder); err != nil {
		add(Check{ID: "game", Status: "fail", Key: "songs_fail", Vars: map[string]string{"dir": cfg.Osu.Folder}, Detail: err.Error()})
	} else {
		add(Check{ID: "game", Status: "ok", Key: "songs_ok", Vars: map[string]string{"dir": dir}})
	}

	switch {
	case !cfg.Hotkeys.Enabled:
		add(Check{ID: "hotkeys", Status: "off", Key: "hotkeys_off"})
	case len(env.HotkeyErrors) > 0:
		keys := make([]string, 0, len(env.HotkeyErrors))
		for k := range env.HotkeyErrors {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		add(Check{ID: "hotkeys", Status: "fail", Key: "hotkeys_fail", Vars: map[string]string{"keys": strings.Join(keys, ", ")}, Detail: env.HotkeyErrors[keys[0]]})
	default:
		add(Check{ID: "hotkeys", Status: "ok", Key: "hotkeys_ok"})
	}
	if env.OverlayClients > 0 {
		add(Check{ID: "overlay", Status: "ok", Key: "overlay_ok", Vars: map[string]string{"n": strconv.Itoa(env.OverlayClients)}})
	} else {
		add(Check{ID: "overlay", Status: "warn", Key: "overlay_none"})
	}
	if !cfg.Command.Enabled {
		add(Check{ID: "requests", Status: "warn", Key: "requests_off"})
	}
	if cfg.Test.DryRun {
		add(Check{ID: "dryrun", Status: "warn", Key: "dryrun_on"})
	}
	if n := a.testItems(); n > 0 {
		add(Check{ID: "testdata", Status: "warn", Key: "testdata", Vars: map[string]string{"n": strconv.Itoa(n)}})
	}
	if u := a.Update(); u != nil {
		add(Check{ID: "update", Status: "warn", Key: "update", Vars: map[string]string{"v": u.Version}})
	}

	wg.Wait()
	if a.Meta != nil {
		add(web)
	}
	out = append(out, mirrors...)
	return out
}

// mirrorChecks turns probe results into checks. One working mirror is
// enough, so the others only warn then.
func mirrorChecks(rs []downloader.ProbeResult) []Check {
	if len(rs) == 0 {
		return []Check{{ID: "mirror", Status: "fail", Key: "mirror_none"}}
	}
	anyOK := false
	for _, r := range rs {
		anyOK = anyOK || r.OK
	}
	out := make([]Check, 0, len(rs))
	for _, r := range rs {
		c := Check{ID: "mirror:" + r.Source, Status: "ok", Key: "mirror_ok", Vars: map[string]string{"src": r.Source, "ms": strconv.FormatInt(r.Ms, 10)}}
		if !r.OK {
			c.Status, c.Key, c.Detail = "fail", "mirror_fail", r.Error
			if anyOK {
				c.Status = "warn"
			}
		}
		out = append(out, c)
	}
	return out
}

// orConnecting covers the moment before a client's first status.
func orConnecting(code string) string {
	if code == "" {
		return "connecting"
	}
	return code
}

// testItems counts test-panel requests still in the queue or playing.
func (a *App) testItems() int {
	s := a.Queue.Snapshot()
	n := 0
	for _, it := range s.Items {
		if it.Test {
			n++
		}
	}
	if s.Now != nil && s.Now.Test {
		n++
	}
	return n
}
