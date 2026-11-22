package app

import (
	"fmt"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/config"
	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/downloader"
	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/parser"
	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/queue"
	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/validator"
)

type qaTransport func(*http.Request) (*http.Response, error)

func (f qaTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestQAOneQueueSlot(t *testing.T) {
	a := multiApp(t, func(c *config.Config) {
		c.Queue.MaxSize = 1
		c.Gifts.RequestCoins = 10
	})
	for _, viewer := range []string{"viewer1", "viewer2"} {
		a.Queue.AddRequestCoins(viewer, 10, 10, time.Hour, false, time.Now())
	}
	arrived := make(chan struct{}, 2)
	release := make(chan struct{})
	a.Meta.HTTP.Transport = qaTransport(func(r *http.Request) (*http.Response, error) {
		arrived <- struct{}{}
		select {
		case <-release:
		case <-r.Context().Done():
			return nil, r.Context().Err()
		}
		web := &fakeWeb{sets: map[int]string{1: set(1, "One", 2), 2: set(2, "Two", 2)}}
		return web.RoundTrip(r)
	})
	done := make(chan SimResult, 2)
	for i := 1; i <= 2; i++ {
		go func(i int) { done <- a.SimulateMessage(fmt.Sprintf("viewer%d", i), fmt.Sprintf("!req %d", i)) }(i)
	}
	// Allow both metadata lookups to overlap, as they can during a live stream.
	// A serialized implementation may have only one lookup in flight.
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
wait:
	for range 2 {
		select {
		case <-arrived:
		case <-timer.C:
			break wait
		}
	}
	close(release)
	first, second := <-done, <-done
	if n := a.Queue.Len(); n != 1 {
		t.Fatalf("queue max=1, got %d; replies %q and %q", n, outcome(first), outcome(second))
	}
	rejected := a.Queue.Snapshot().Rejected
	if len(rejected) != 1 || rejected[0].Code != "queue_full" {
		t.Fatalf("expected one queue_full rejection, got %+v", rejected)
	}
	if left, _ := a.Queue.Requests(rejected[0].User, time.Now()); left != 1 {
		t.Fatalf("rejected viewer has %d paid requests left, want 1", left)
	}

}

func TestQAPreviousAfterRestart(t *testing.T) {
	a, _, _ := supporterApp(t, supCase{})
	path := filepath.Join(t.TempDir(), "queue.json")
	q, err := queue.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	a.Queue = q
	q.Add(supItem("song"))
	if err := a.Next("played"); err != nil {
		t.Fatal(err)
	}
	q.Finish("played")
	q.Flush()
	// Restart with the saved queue, same cache folder and an empty in-memory trust list.
	a.Queue, err = queue.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	old := a.DL
	a.DL = downloader.New(old.Cfg, old.CacheDir, a.OpenURL)
	a.DL.HTTP, a.DL.Allow = old.HTTP, old.Allow
	a.Import.Trusted = a.DL.Trusted
	if err := a.Previous(); err != nil {
		t.Fatalf("previous failed after restart: %v", err)
	}
}

func TestQAClearTestPreservesLiveCooldown(t *testing.T) {
	a := multiApp(t, nil)
	if result := a.SimulateMessage("viewer", "!req 1"); outcome(result) != "ok 1" {
		t.Fatal(outcome(result))
	}
	request := func(id int) (*queue.Item, *validator.Rejection) {
		return a.submit("viewer", "Viewer", fmt.Sprintf("!req %d", id), parser.Request{Kind: parser.KindSet, SetID: id}, fromChat, validator.Roles{}, nil)
	}
	if _, reject := request(2); reject != nil {
		t.Fatal(reject)
	}
	cfg := a.Cfg.Get()
	cfg.Queue.CooldownSec = 60
	if err := a.Cfg.Set(cfg); err != nil {
		t.Fatal(err)
	}
	if _, reject := request(3); reject == nil || reject.Code != "cooldown" {
		t.Fatalf("expected cooldown before cleanup, got %v", reject)
	}
	a.ClearTest()
	if _, reject := request(3); reject == nil || reject.Code != "cooldown" {
		t.Fatalf("expected same real cooldown after cleanup, got %v", reject)
	}
}

func TestQASummaryCountsFullSession(t *testing.T) {
	a := newTestApp(t, nil)
	for i := 1; i <= 201; i++ {
		id := fmt.Sprint(i)
		a.Queue.Add(&queue.Item{ID: id, User: "viewer", SetID: i, AddedAt: time.Now()})
		a.Queue.Promote(id, "played")
		a.Queue.Finish("played")
	}
	if got := a.Queue.Summary().Played; got != 201 {
		t.Fatalf("201 plays in one session; summary reports %d", got)
	}
}
