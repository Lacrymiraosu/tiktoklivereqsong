package queue

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSummary(t *testing.T) {
	q, err := Open(filepath.Join(t.TempDir(), "queue.json"))
	if err != nil {
		t.Fatal(err)
	}
	start := q.Summary().Start
	if start.IsZero() || time.Since(start) > time.Minute {
		t.Fatalf("a fresh queue should start a session now, got %v", start)
	}
	play := func(id, user string, res *Result, result string, test bool) {
		q.Add(&Item{ID: id, User: user, Title: id, AddedAt: time.Now(), Test: test})
		q.Promote(id, "played")
		if res != nil {
			q.Update(id, func(it *Item) { it.Result = res })
		}
		q.Finish(result)
	}
	play("a", "Fan", &Result{Outcome: "passed", Accuracy: 99, PP: 120}, "played", false)
	play("b", "fan", &Result{Outcome: "passed", Accuracy: 95, PP: 300}, "played", false)
	play("c", "other", &Result{Outcome: "failed", Accuracy: 80}, "played", false)
	play("d", "third", nil, "skipped", false)
	play("e", "tester", &Result{Outcome: "passed", Accuracy: 100, PP: 999}, "played", true)
	q.Reject(Rejected{User: "x", Code: "cooldown"})
	q.Reject(Rejected{User: "y", Code: "cooldown", Test: true})

	s := q.Summary()
	if s.Played != 3 || s.Skipped != 1 || s.Passed != 2 || s.Rejected != 1 || s.Viewers != 3 {
		t.Errorf("counts: %+v", s)
	}
	if s.Best == nil || s.Best.Title != " - b" || s.Best.Play.PP != 300 {
		t.Errorf("best should be the 300pp play, not the test one: %+v", s.Best)
	}
	if len(s.Top) != 3 || !strings.EqualFold(s.Top[0].User, "fan") || s.Top[0].Count != 2 {
		t.Errorf("top requesters: %+v", s.Top)
	}

	q.NewSession(time.Now().Add(time.Second))
	if s := q.Summary(); s.Played != 0 || s.Rejected != 0 || s.Best != nil {
		t.Errorf("a new session starts empty: %+v", s)
	}
}

func TestSessionSurvivesRestartButNotADayOff(t *testing.T) {
	path := filepath.Join(t.TempDir(), "queue.json")
	write := func(start, lastPlay time.Time) {
		d := data{SessionStart: start, SessionRejected: 4, History: []Played{{Title: "x", User: "u", At: lastPlay, Result: "played"}}}
		b, _ := json.Marshal(d)
		if err := os.WriteFile(path, b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now()
	write(now.Add(-3*time.Hour), now.Add(-10*time.Minute))
	q, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if s := q.Summary(); !s.Start.Equal(now.Add(-3*time.Hour)) || s.Rejected != 4 || s.Played != 1 {
		t.Errorf("a restart mid-stream keeps the session: %+v", s)
	}

	write(now.Add(-30*time.Hour), now.Add(-20*time.Hour))
	if q, err = Open(path); err != nil {
		t.Fatal(err)
	}
	if s := q.Summary(); time.Since(s.Start) > time.Minute || s.Rejected != 0 || s.Played != 0 {
		t.Errorf("starting the next day begins a new session: %+v", s)
	}
}
