package search

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// testdata/freedom_dive.json is a real catboy response for "freedom dive"
// (ranked, approved, qualified, loved), trimmed to the fields used.
func fixture(t *testing.T) []Result {
	t.Helper()
	f, err := os.Open("testdata/freedom_dive.json")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	rs, err := Parse(f)
	if err != nil {
		t.Fatal(err)
	}
	return rs
}

func TestRankRealResponse(t *testing.T) {
	rs := fixture(t)
	if len(rs) < 10 {
		t.Fatalf("fixture has %d sets", len(rs))
	}
	got := Rank("freedom dive", rs)
	if got[0].SetID != 39804 {
		t.Errorf("the most played FREEDOM DiVE (39804) should come first, got %s", got[0].Label())
	}
	for _, r := range got[:5] {
		if !strings.Contains(strings.ToLower(r.Title), "freedom dive") {
			t.Errorf("a set whose title does not match ranks too high: %s", r.Label())
		}
	}
	// mania only: the first result must have a mania difficulty or convert
	mania := Rank("freedom dive", Playable(rs, []string{"mania"}))
	if len(mania) == 0 {
		t.Fatal("no mania result")
	}
	for _, r := range Playable(rs, []string{"mania"}) {
		ok := false
		for _, m := range r.Modes {
			ok = ok || m == "mania" || m == "osu"
		}
		if !ok {
			t.Errorf("%s has no mania difficulty and cannot convert", r.Label())
		}
	}
}

func TestRankRules(t *testing.T) {
	rs := []Result{
		{SetID: 1, Artist: "Various", Title: "Compilation", Plays: 9e6},         // matches on tags only
		{SetID: 2, Artist: "xi", Title: "FREEDOM DiVE", Plays: 100},             // title match, few plays
		{SetID: 3, Artist: "xi", Title: "FREEDOM DiVE (Cut Ver.)", Plays: 5000}, // title match, more plays
		{SetID: 4, Artist: "Freedom", Title: "Dive In", Plays: 1e6},             // artist + title match
		{SetID: 5, Artist: "Natsukawa", Title: "Kani*Do-Luck!", Plays: 10},
	}
	got := Rank("Freedom Dive", rs)
	var order []int
	for _, r := range got {
		order = append(order, r.SetID)
	}
	if fmt.Sprint(order) != "[3 2 4]" {
		t.Errorf("order = %v, want title matches by plays, then artist+title; tag-only matches dropped", order)
	}
	if got := Rank("night of nights", rs); len(got) != 0 {
		t.Errorf("nothing has those words in artist or title: %v", got)
	}
	if got := Rank("短笛", []Result{{SetID: 1, Title: "Duan Di", TitleUnicode: "短笛・大王"}}); len(got) != 1 {
		t.Errorf("original-script titles should match: %v", got)
	}
	if got := Rank("kanidoluck", rs); got[0].SetID != 5 {
		t.Errorf("punctuation in a title should not stop a match: %v", got[0])
	}
	if got := Rank("รัก", []Result{{SetID: 1, Title: "Other"}, {SetID: 2, Title: "รักแรกพบ"}}); got[0].SetID != 2 {
		t.Errorf("Thai titles (no spaces) should match by substring: %v", got[0])
	}
}

func TestFindSendsFiltersAndCaches(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if r.Header.Get("User-Agent") == "" {
			t.Error("catboy refuses requests without a User-Agent")
		}
		q := r.URL.Query()
		if q.Get("q") != "freedom dive" || strings.Join(q["status"], ",") != "1,4" {
			t.Errorf("query = %v", q)
		}
		http.ServeFile(w, r, "testdata/freedom_dive.json")
	}))
	defer srv.Close()
	c := New()
	c.Mirrors = []Mirror{Catboy(srv.URL)}
	for i := 0; i < 2; i++ {
		rs, err := c.Find(context.Background(), "freedom dive", []string{"ranked", "loved", "bogus"}, nil)
		if err != nil || len(rs) == 0 || rs[0].SetID != 39804 {
			t.Fatalf("find: %v %v", err, rs)
		}
	}
	if hits != 1 {
		t.Errorf("the same search should be cached, got %d requests", hits)
	}
}

func TestFindErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.RawQuery, "html") {
			w.Write([]byte("<html>"))
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	c := New()
	c.MinGap = 0
	c.Mirrors = []Mirror{Catboy(srv.URL)}
	if _, err := c.Find(context.Background(), "x y z", nil, nil); err == nil {
		t.Error("HTTP 503 from every mirror should be an error")
	}
	if _, err := c.Find(context.Background(), "html", nil, nil); err == nil {
		t.Error("a non-JSON answer should be an error")
	}
}

// mirrorServer answers each path with its handler and records the order.
func mirrorServer(t *testing.T, paths map[string]http.HandlerFunc) (*httptest.Server, *[]string) {
	var order []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		order = append(order, r.URL.Path)
		paths[r.URL.Path](w, r)
	}))
	t.Cleanup(srv.Close)
	return srv, &order
}

func TestFindFallsBack(t *testing.T) {
	srv, order := mirrorServer(t, map[string]http.HandlerFunc{
		"/hinai":  func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) },
		"/nekoha": func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{"beatmapsets":[]}`)) },
		"/direct": func(w http.ResponseWriter, r *http.Request) { http.ServeFile(w, r, "testdata/freedom_dive.json") },
		"/catboy": func(w http.ResponseWriter, r *http.Request) { t.Error("catboy should not be asked") },
	})
	c := New()
	c.MinGap = 0
	c.Mirrors = []Mirror{Hinai(srv.URL + "/hinai"), Nekoha(srv.URL + "/nekoha"), OsuDirect(srv.URL + "/direct"), Catboy(srv.URL + "/catboy")}
	rs, err := c.Find(context.Background(), "freedom dive", nil, nil)
	if err != nil || len(rs) == 0 || rs[0].SetID != 39804 {
		t.Fatalf("find: %v %v", err, rs)
	}
	if got := strings.Join(*order, " "); got != "/hinai /nekoha /direct" {
		t.Errorf("a failing mirror and one that finds nothing hand over: %s", got)
	}
}

func TestFindRestsAfter429(t *testing.T) {
	srv, order := mirrorServer(t, map[string]http.HandlerFunc{
		"/a": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Retry-After", "120")
			w.WriteHeader(http.StatusTooManyRequests)
		},
		"/b": func(w http.ResponseWriter, r *http.Request) { http.ServeFile(w, r, "testdata/freedom_dive.json") },
	})
	c := New()
	c.MinGap = 0
	c.Mirrors = []Mirror{OsuDirect(srv.URL + "/a"), Catboy(srv.URL + "/b")}
	for _, q := range []string{"freedom dive", "freedom dive extra"} {
		if _, err := c.Find(context.Background(), q, nil, nil); err != nil {
			t.Fatal(err)
		}
	}
	if got := strings.Join(*order, " "); got != "/a /b /b" {
		t.Errorf("a rate-limited mirror rests for its Retry-After: %s", got)
	}
	c.Mirrors = c.Mirrors[:1]
	if _, err := c.Find(context.Background(), "another song", nil, nil); err == nil {
		t.Error("no mirror left to ask should be an error")
	}
}

func TestMirrorQueries(t *testing.T) {
	all := []string{"ranked", "approved", "qualified", "loved"}
	v, local := Hinai("").Query("blue zenith", all)
	if v.Has("status") || !local || v.Get("sort") != "play_count:desc" {
		t.Errorf("hinai searches only its first status, so several are filtered here: %v %v", v, local)
	}
	if v, local := Hinai("").Query("x", []string{"loved"}); v.Get("status") != "4" || local {
		t.Errorf("hinai, one status: %v %v", v, local)
	}
	if v, _ := OsuDirect("").Query("x", all); v.Get("status") != "1,2,3,4" {
		t.Errorf("osu.direct takes every status at once: %v", v)
	}
	if v, _ := Nekoha("").Query("x", all); v.Get("status") != "ranked,approved,qualified,loved" {
		t.Errorf("Nekoha takes status names: %v", v)
	}
	if v, _ := Catboy("").Query("x", []string{"ranked", "loved"}); strings.Join(v["status"], ",") != "1,4" {
		t.Errorf("catboy, one parameter each: %v", v)
	}
	rs := withStatus([]Result{{SetID: 1, Status: "graveyard"}, {SetID: 2, Status: "approved"}, {SetID: 3, Status: "loved"}}, []string{"ranked", "loved"})
	if len(rs) != 2 || rs[0].SetID != 2 || rs[1].SetID != 3 {
		t.Errorf("local filter (ranked includes approved): %v", rs)
	}
}

func TestParseNekoha(t *testing.T) {
	rs, err := Parse(strings.NewReader(`{"beatmapsets":[{"id":39804,"artist":"xi","title":"FREEDOM DiVE","status":"approved","play_count":null,
		"beatmaps":[{"mode":"osu","playcount":10934869},{"mode":"osu","playcount":5}]}]}`))
	if err != nil || len(rs) != 1 || rs[0].SetID != 39804 || rs[0].Modes[0] != "osu" || rs[0].Plays != 10934874 {
		t.Errorf("Nekoha wraps its sets and may only count plays per difficulty: %v %v", rs, err)
	}
}

func TestFindDropsFilterBrackets(t *testing.T) {
	var got string
	srv, _ := mirrorServer(t, map[string]http.HandlerFunc{
		"/a": func(w http.ResponseWriter, r *http.Request) { got = r.URL.Query().Get("q"); w.Write([]byte("[]")) },
	})
	c := New()
	c.MinGap = 0
	c.Mirrors = []Mirror{Hinai(srv.URL + "/a")}
	c.Find(context.Background(), "blue [zenith]", nil, nil)
	if got != "blue zenith" {
		t.Errorf(`"[" starts a filter block on hinai and osu.direct: q = %q`, got)
	}
}
