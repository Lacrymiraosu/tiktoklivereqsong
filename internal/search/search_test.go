package search

import (
	"context"
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
	order := []int{got[0].SetID, got[1].SetID, got[2].SetID, got[3].SetID}
	if order[0] != 3 || order[1] != 2 || order[2] != 4 {
		t.Errorf("order = %v, want title matches by plays, then artist+title, then the rest", order)
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
	c.URL = srv.URL
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
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()
	c := New()
	c.URL = srv.URL
	if _, err := c.Find(context.Background(), "x y z", nil, nil); err == nil {
		t.Error("HTTP 429 should be an error")
	}
	c.MinGap = 0
	if _, err := c.Find(context.Background(), "html", nil, nil); err == nil {
		t.Error("a non-JSON answer should be an error")
	}
}
