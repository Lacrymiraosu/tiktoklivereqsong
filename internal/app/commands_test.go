package app

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/config"
	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/queue"
	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/tikfinity"
)

// newTestApp builds an App with real config and queue stores in a temp
// folder and no network-facing parts.
func newTestApp(t *testing.T, mutate func(*config.Config)) *App {
	t.Helper()
	dir := t.TempDir()
	store, err := config.Load(filepath.Join(dir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if mutate != nil {
		c := store.Get()
		mutate(&c)
		if err := store.Set(c); err != nil {
			t.Fatal(err)
		}
	}
	q, err := queue.Open(filepath.Join(dir, "queue.json"))
	if err != nil {
		t.Fatal(err)
	}
	return New(store, q, nil, nil, nil, nil, nil)
}

func add(a *App, id, user string, setID int, at time.Time) {
	a.Queue.Add(&queue.Item{ID: id, User: user, SetID: setID, Title: id, AddedAt: at})
}

func ids(a *App) []string {
	var out []string
	for _, it := range a.Queue.First(100) {
		out = append(out, it.ID)
	}
	return out
}

func TestRemoveCommand(t *testing.T) {
	a := newTestApp(t, func(c *config.Config) { c.Command.Mods = []string{"@ModMan"} })
	t0 := time.Now()
	add(a, "fan-1", "fan", 1, t0)
	add(a, "fan-2", "fan", 2, t0.Add(time.Second))
	add(a, "other-1", "other", 3, t0.Add(2*time.Second))

	if a.command(tikfinity.Chat{User: "fan", Comment: "!remove"}, false) != cmdDone {
		t.Fatal("!remove should be handled")
	}
	if got := ids(a); len(got) != 2 || got[0] != "fan-1" {
		t.Errorf("own latest request should go: %v", got)
	}
	// a viewer cannot remove someone else's request
	a.command(tikfinity.Chat{User: "fan", Comment: "!remove @other"}, false)
	if got := ids(a); len(got) != 1 || got[0] != "other-1" {
		t.Errorf("viewer's @name must be ignored (removes own instead): %v", got)
	}
	// a mod can
	a.command(tikfinity.Chat{User: "modman", Comment: "!REMOVE @Other"}, false)
	if got := ids(a); len(got) != 0 {
		t.Errorf("mod should remove @other's request: %v", got)
	}
	if f := a.Feed(); len(f) == 0 || f[0].Kind != "command" || f[0].Code != "remove" || f[0].Target != "other" {
		t.Errorf("feed should show the mod removal: %+v", f)
	}
}

func TestModOnlyCommands(t *testing.T) {
	a := newTestApp(t, func(c *config.Config) { c.Command.Mods = []string{"modman"} })

	// not a mod: handled (never parsed as a request) but nothing changes
	if got := a.command(tikfinity.Chat{User: "fan", Comment: "!reqoff"}, false); got != cmdNotMod {
		t.Fatalf("!reqoff from a viewer = %q, want not_mod", got)
	}
	if !a.Cfg.Get().Command.Enabled {
		t.Fatal("a viewer must not close requests")
	}
	a.command(tikfinity.Chat{User: "modman", Comment: "!reqoff"}, false)
	if a.Cfg.Get().Command.Enabled {
		t.Fatal("a mod should close requests")
	}
	a.command(tikfinity.Chat{User: "modman", Comment: "!reqon"}, false)
	if !a.Cfg.Get().Command.Enabled {
		t.Fatal("a mod should open requests")
	}

	// TikTok's moderator flag counts when trusted, and not when it isn't
	tiktokMod := tikfinity.Chat{User: "someone", Comment: "!reqoff", Moderator: tikfinity.Yes}
	if !a.IsMod(tiktokMod) {
		t.Error("TikTok moderator should be a mod by default")
	}
	c := a.Cfg.Get()
	c.Command.TrustTikTokMods = false
	a.Cfg.Set(c)
	if a.IsMod(tiktokMod) {
		t.Error("TikTok moderator flag must be ignored when not trusted")
	}

	// renamed or disabled commands
	c.Command.SkipCmd = ""
	a.Cfg.Set(c)
	if a.command(tikfinity.Chat{User: "modman", Comment: "!skip"}, false) != cmdNone {
		t.Error("a disabled command must fall through")
	}
	if a.command(tikfinity.Chat{User: "fan", Comment: "!req 123"}, false) != cmdNone {
		t.Error("requests are not commands")
	}
	if got := a.command(tikfinity.Chat{User: "nobody", Comment: "!remove"}, false); got != cmdNothing {
		t.Errorf("!remove with nothing queued = %q, want nothing", got)
	}
}

func TestUserFacingErrorsHaveCodes(t *testing.T) {
	a := newTestApp(t, nil)
	for name, err := range map[string]error{"queue_empty": a.Next("played"), "not_found": a.Remove("nope")} {
		var ue *UserError
		if !errors.As(err, &ue) || ue.Code != name {
			t.Errorf("want code %s, got %v", name, err)
		}
	}
}
