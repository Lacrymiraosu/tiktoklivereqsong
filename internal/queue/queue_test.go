package queue

import (
	"path/filepath"
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
