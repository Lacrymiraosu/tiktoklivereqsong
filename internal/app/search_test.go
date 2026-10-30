package app

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/config"
	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/osumeta"
	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/queue"
	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/search"
	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/tosu"
)

// fakeWeb answers a mirror's search and osu!'s beatmapset pages.
type fakeWeb struct {
	results string         // JSON array returned by the search
	sets    map[int]string // set id -> beatmapset JSON
	pages   []int          // osu! pages asked for
}

func (f *fakeWeb) RoundTrip(r *http.Request) (*http.Response, error) {
	body, code := "", http.StatusOK
	switch {
	case r.URL.Host == "search.test":
		body = f.results
	case r.URL.Host == "osu.ppy.sh" && strings.HasPrefix(r.URL.Path, "/beatmapsets/"):
		var id int
		fmt.Sscanf(r.URL.Path, "/beatmapsets/%d", &id)
		f.pages = append(f.pages, id)
		if js, ok := f.sets[id]; ok {
			body = `<html><script id="json-beatmapset" type="application/json">` + js + `</script></html>`
		} else {
			code = http.StatusNotFound
		}
	default:
		code = http.StatusNotFound
	}
	return &http.Response{StatusCode: code, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}, Request: r}, nil
}

func set(id int, title string, stars float64) string {
	return fmt.Sprintf(`{"id":%d,"title":%q,"artist":"xi","creator":"m","status":"ranked",
		"beatmaps":[{"id":%d,"version":"Hard","mode":"osu","difficulty_rating":%v,"total_length":200,"checksum":"aa","status":"ranked"}]}`, id, title, id*10, stars)
}

func searchApp(t *testing.T, web *fakeWeb, mutate func(*config.Config)) *App {
	a := newTestApp(t, mutate)
	a.Meta = osumeta.New()
	a.Meta.MinGap = 0
	a.Meta.HTTP = &http.Client{Transport: web}
	a.Search = search.New()
	a.Search.MinGap = 0
	a.Search.Mirrors = []search.Mirror{search.Catboy("https://search.test/api/v2/search")}
	a.Search.HTTP = &http.Client{Transport: web}
	return a
}

func TestRequestBySongName(t *testing.T) {
	web := &fakeWeb{
		results: `[{"id":2,"artist":"V","title":"Stream Practice","status":"ranked","play_count":9000000,"beatmaps":[{"mode":"osu"}]},
		           {"id":1,"artist":"xi","title":"FREEDOM DiVE","status":"ranked","play_count":20000000,"beatmaps":[{"mode":"osu"}]},
		           {"id":3,"artist":"xi","title":"FREEDOM DiVE (easy)","status":"ranked","play_count":5000,"beatmaps":[{"mode":"osu"}]}]`,
		sets: map[int]string{1: set(1, "FREEDOM DiVE", 7.4), 2: set(2, "Stream Practice", 5), 3: set(3, "FREEDOM DiVE (easy)", 3.1)},
	}
	a := searchApp(t, web, nil)
	it, rej, kind := a.SimulateChat("fan", "!req freedom dive")
	if kind != SimRequest || rej != nil || it == nil || it.SetID != 1 || !it.BySet {
		t.Fatalf("got %+v %+v %v", it, rej, kind)
	}

	// too hard: the next title match that passes the rules is taken
	c := a.Cfg.Get()
	c.Rules.MaxStars = 6
	c.Queue.CooldownSec, c.Queue.PerUserLimit = 0, 0
	a.Cfg.Set(c)
	web.pages = nil
	it, rej, _ = a.SimulateChat("fan2", "!req FREEDOM DiVE")
	if rej != nil || it.SetID != 3 {
		t.Fatalf("should fall back to the easier set: %+v %+v", it, rej)
	}
	if fmt.Sprint(web.pages) != "[3]" {
		t.Errorf("set 1 is cached and too hard; only set 3 should be fetched, got %v", web.pages)
	}
}

func TestSongNameRejections(t *testing.T) {
	web := &fakeWeb{results: `[]`, sets: map[int]string{1: set(1, "FREEDOM DiVE", 7.4)}}
	a := searchApp(t, web, func(c *config.Config) { c.Rules.MaxStars = 5 })
	if _, rej, _ := a.SimulateChat("fan", "!req nothing like this"); rej == nil || rej.Code != "search_none" {
		t.Errorf("no result: %+v", rej)
	}
	web.results = `[{"id":1,"artist":"xi","title":"FREEDOM DiVE","status":"ranked","play_count":1,"beatmaps":[{"mode":"osu"}]}]`
	_, rej, _ := a.SimulateChat("fan2", "!req freedom dive")
	if rej == nil || rej.Code != "stars_high" || !strings.HasPrefix(rej.Short, "FREEDOM DiVE · ") {
		t.Errorf("rejection should name the map that was found: %+v", rej)
	}
	web.results = `<html>`
	if _, rej, _ := a.SimulateChat("fan3", "!req something else"); rej == nil || rej.Code != "search_error" {
		t.Errorf("search failure: %+v", rej)
	}

	c := a.Cfg.Get()
	c.Command.Search = false
	a.Cfg.Set(c)
	if _, _, kind := a.SimulateChat("fan4", "!req freedom dive"); kind != SimIgnored {
		t.Errorf("with search off a name is not a request, got %v", kind)
	}
}

func TestGiftOnlyRequests(t *testing.T) {
	web := &fakeWeb{sets: map[int]string{1: set(1, "Easy Song", 2), 2: set(2, "Hard Song", 9)}}
	a := searchApp(t, web, func(c *config.Config) {
		c.Gifts.RequestCoins = 30
		c.Queue.CooldownSec, c.Queue.PerUserLimit = 0, 0
		c.Rules.MaxStars = 5
		c.Rules.ExemptUsers = []string{"streamer"}
	})
	req := func(user string, set int) string {
		_, rej, _ := a.SimulateChat(user, fmt.Sprintf("!req https://osu.ppy.sh/beatmapsets/%d", set))
		if rej != nil {
			return rej.Code + " " + rej.Short
		}
		return "ok"
	}
	if got := req("fan", 1); got != "needs_gift 0/30 coins" {
		t.Fatalf("without a gift: %s", got)
	}
	if _, n, err := a.SimulateGift("fan", 40); err != nil || n != 1 {
		t.Fatalf("40 coins should pay for 1 request: %d %v", n, err)
	}
	if f := a.Feed()[0]; f.Kind != "gift" || f.Code != "requests" || f.Pos != 1 {
		t.Errorf("gift event: %+v", f)
	}
	if got := req("fan", 2); got != "stars_high 9.00★ > 5.00★" {
		t.Fatalf("too hard: %s", got)
	}
	if got := req("fan", 1); got != "ok" {
		t.Fatalf("the rejected map must not use up the paid request: %s", got)
	}
	if got := req("fan", 1); got != "needs_gift 10/30 coins" {
		t.Errorf("paid request used up, 10 coins left over: %s", got)
	}
	if got := req("streamer", 1); strings.HasPrefix(got, "needs_gift") {
		t.Errorf("exempt users need no gift: %s", got)
	}
	if it, rej := a.TestRequest("dashboard", "https://osu.ppy.sh/beatmapsets/1"); rej != nil && rej.Code == "needs_gift" {
		t.Errorf("quick add from the dashboard needs no gift: %+v %+v", it, rej)
	}
}

func TestRequestWithMods(t *testing.T) {
	web := &fakeWeb{
		results: `[{"id":1,"artist":"xi","title":"FREEDOM DiVE","status":"ranked","play_count":5,"beatmaps":[{"mode":"osu"}]}]`,
		sets:    map[int]string{1: set(1, "FREEDOM DiVE", 4.5), 2: set(2, "Hard Song", 6.5)},
	}
	a := searchApp(t, web, func(c *config.Config) {
		c.Queue.CooldownSec, c.Queue.PerUserLimit = 0, 0
		c.Rules.HardModMaxStars = 5
	})
	it, rej, _ := a.SimulateChat("fan", "!req https://osu.ppy.sh/beatmapsets/1 hdhr")
	if rej != nil || it.Mods != "HDHR" {
		t.Fatalf("mods on a link: %+v %+v", it, rej)
	}
	if f := a.Feed()[0]; f.Kind != "added" || f.Mods != "HDHR" {
		t.Errorf("added pop-up should show the mods: %+v", f)
	}
	if _, rej, _ := a.SimulateChat("fan2", "!req https://osu.ppy.sh/beatmapsets/2 +DT"); rej == nil || rej.Code != "mod_stars" {
		t.Errorf("6.5★ with DT over the 5★ cap: %+v", rej)
	}
	if _, rej, _ := a.SimulateChat("fan3", "!req https://osu.ppy.sh/beatmapsets/2 +EZHR"); rej == nil || rej.Code != "bad_mods" || rej.Short != "EZ+HR" {
		t.Errorf("conflicting mods: %+v", rej)
	}
	a.Remove(it.ID)
	if it, rej, _ := a.SimulateChat("fan4", "!req freedom dive +HD"); rej != nil || it.Mods != "HD" || it.SetID != 1 {
		t.Errorf("mods on a song name: %+v %+v", it, rej)
	}
}

func TestPlayedWithoutRequestedMods(t *testing.T) {
	a := newTestApp(t, nil)
	a.Queue.Add(&queue.Item{ID: "x", User: "fan", SetID: 7, Title: "T", Artist: "A", Mods: "HDHR", AddedAt: time.Now()})
	a.Queue.Promote("x", "played")
	sc := tosu.Score{Accuracy: 97, Score: 1000, Mods: "HD", MaxCombo: 100}
	a.OnGame(tosu.Game{State: "play", SetID: 7, Play: sc})
	a.OnGame(tosu.Game{State: "resultScreen", SetID: 7, Results: sc})
	now, _ := a.Queue.Now()
	if now.Result == nil || now.Result.Missing != "HR" {
		t.Fatalf("result should note the missing HR: %+v", now.Result)
	}
	if n := a.Notices(); len(n) == 0 || n[0].Key != "modsMissing" {
		t.Errorf("Activity should warn: %+v", n)
	}
}

func TestQuickAddBareCommand(t *testing.T) {
	web := &fakeWeb{results: `[]`}
	a := searchApp(t, web, nil)
	if _, rej := a.TestRequest("dashboard", "!req"); rej == nil || rej.Code != "bad_input" {
		t.Errorf("a bare command is bad input, not a search: %+v", rej)
	}
}
