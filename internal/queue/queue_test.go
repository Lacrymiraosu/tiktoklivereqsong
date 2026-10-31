package queue

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNowPlayingDoesNotCountTowardUserLimit(t *testing.T) {
	q, err := Open(filepath.Join(t.TempDir(), "queue.json"))
	if err != nil {
		t.Fatal(err)
	}
	a := &Item{ID: "a", User: "Viewer", SetID: 1, AddedAt: time.Now()}
	b := &Item{ID: "b", User: "viewer", SetID: 2, AddedAt: time.Now()}
	q.Add(a)
	q.Add(b)
	if st := q.ValidationState("VIEWER"); st.UserActive != 2 {
		t.Fatalf("two waiting requests, got %d", st.UserActive)
	}
	q.Promote("a", "played")
	st := q.ValidationState("viewer")
	if st.UserActive != 1 {
		t.Errorf("now playing should not count, got %d", st.UserActive)
	}
	if !st.QueuedSets[1] || !st.QueuedSets[2] {
		t.Errorf("both sets (playing and waiting) must still block duplicates: %v", st.QueuedSets)
	}
}

func TestClearTestKeepsRealData(t *testing.T) {
	q, err := Open(filepath.Join(t.TempDir(), "queue.json"))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	q.Add(&Item{ID: "real", User: "fan", SetID: 1, AddedAt: now})
	q.Add(&Item{ID: "sim", User: "tester", SetID: 2, AddedAt: now, Test: true, Simulated: true})
	q.Add(&Item{ID: "quick", User: "dashboard", SetID: 3, AddedAt: now, Test: true})
	q.Promote("sim", "played")
	q.Promote("real", "played") // sim goes to history as test data
	q.Reject(Rejected{User: "tester", Code: "cooldown", Test: true})
	q.Reject(Rejected{User: "fan", Code: "cooldown"})

	if st := q.ValidationState("tester"); st.LastRequest.IsZero() {
		t.Fatal("simulated chat should set a cooldown like real chat")
	}
	if st := q.ValidationState("dashboard"); !st.LastRequest.IsZero() {
		t.Fatal("quick add must not set a cooldown")
	}

	ids := q.ClearTest()
	if len(ids) != 1 || ids[0] != "quick" {
		t.Errorf("removed %v, want [quick]", ids)
	}
	s := q.Snapshot()
	if s.Now == nil || s.Now.ID != "real" || len(s.Items) != 0 {
		t.Errorf("real now-playing must stay: %+v", s)
	}
	if len(s.History) != 0 {
		t.Errorf("test history should be gone: %+v", s.History)
	}
	if len(s.Rejected) != 1 || s.Rejected[0].User != "fan" {
		t.Errorf("only the real rejection should remain: %+v", s.Rejected)
	}
	if !q.ValidationState("tester").LastRequest.IsZero() {
		t.Error("simulated cooldown should be reset")
	}
	if q.ValidationState("fan").LastRequest.IsZero() {
		t.Error("real cooldown must be kept")
	}
}

func TestAddFair(t *testing.T) {
	q, err := Open(filepath.Join(t.TempDir(), "queue.json"))
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	add := func(user string) {
		n++
		q.AddFair(&Item{ID: fmt.Sprintf("%s%d", user, n), User: user, AddedAt: time.Now()})
	}
	order := func() string {
		var s []string
		for _, it := range q.First(100) {
			s = append(s, it.User)
		}
		return strings.Join(s, " ")
	}
	add("A")
	add("A")
	add("A")
	add("B")
	add("C")
	add("b")
	if got := order(); got != "A B C A b A" {
		t.Errorf("order = %q, want rounds A B C | A b | A", got)
	}
	// a priority request stays on top; a newcomer still lands in round 1, after it
	q.Update(q.First(100)[5].ID, func(it *Item) { it.Priority = true })
	q.Move(q.First(100)[5].ID, 0)
	add("D")
	if got := order(); got != "A A B C D A b" {
		t.Errorf("after priority: %q", got)
	}
}

func TestBackToPrevious(t *testing.T) {
	q, err := Open(filepath.Join(t.TempDir(), "queue.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"a", "b", "c"} {
		q.Add(&Item{ID: id, User: "u-" + id, SetID: int(id[0]), Artist: "x", Title: id, State: Ready, File: id + ".osz"})
	}
	if _, ok := q.Previous(); ok {
		t.Fatal("nothing was played yet")
	}
	q.Promote("a", "played")
	q.Promote("b", "played")
	if s := q.Snapshot(); s.Prev != "x - a" || len(s.History) != 1 {
		t.Fatalf("prev %q history %d", s.Prev, len(s.History))
	}
	prev, ok := q.Previous()
	if !ok || prev.ID != "a" {
		t.Fatalf("previous = %+v", prev)
	}
	if _, ok := q.Back("zzz"); ok {
		t.Fatal("Back must refuse a map that is not the previous one")
	}
	if _, ok := q.Back("a"); !ok {
		t.Fatal("Back failed")
	}
	s := q.Snapshot()
	if s.Now == nil || s.Now.ID != "a" || s.Now.PlayPhase != "waiting" {
		t.Errorf("now = %+v", s.Now)
	}
	if len(s.Items) != 2 || s.Items[0].ID != "b" || s.Items[0].State != Ready || s.Items[0].PlayPhase != "" {
		t.Errorf("current map should be back at the front, ready: %+v", s.Items)
	}
	if len(s.History) != 0 || s.Prev != "" {
		t.Errorf("a is playing again: history %+v prev %q", s.History, s.Prev)
	}
	if !q.Files()["b.osz"] {
		t.Error("b's file must be kept")
	}
	q.Promote("b", "played")
	if !q.Files()["a.osz"] {
		t.Error("the previous map's file is kept for Back")
	}
}
