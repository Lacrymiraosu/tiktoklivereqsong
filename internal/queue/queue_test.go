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
