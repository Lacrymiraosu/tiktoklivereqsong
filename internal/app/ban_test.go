package app

import (
	"testing"
	"time"

	"github.com/draft/osureq/internal/queue"
)

func TestBan(t *testing.T) {
	a := newTestApp(t, nil)
	now := time.Now()
	for _, it := range []queue.Item{
		{ID: "p", User: "spam", SetID: 9, Creator: "Mapper"}, // will be playing
		{ID: "a", User: "spam", SetID: 1, Creator: "X"},
		{ID: "b", User: "Spam", SetID: 2, Creator: "Mapper"},
		{ID: "c", User: "fan", SetID: 2, Creator: "Mapper"},
		{ID: "d", User: "fan", SetID: 3, Creator: "Y"},
	} {
		it := it
		it.AddedAt = now
		a.Queue.Add(&it)
	}
	a.Queue.Promote("p", "played")

	if n, err := a.Ban("user", "@SPAM"); err != nil || n != 2 {
		t.Fatalf("ban user: removed %d, %v", n, err)
	}
	if n, _ := a.Ban("user", "spam"); n != 0 || len(a.Cfg.Get().Rules.BlacklistUsers) != 1 {
		t.Errorf("banning twice must not duplicate: %v", a.Cfg.Get().Rules.BlacklistUsers)
	}
	if now, ok := a.Queue.Now(); !ok || now.ID != "p" {
		t.Error("the map being played must stay")
	}
	if n, _ := a.Ban("mapper", "mapper"); n != 1 || ids(a)[0] != "d" {
		t.Errorf("ban mapper: removed %d, left %v", n, ids(a))
	}
	if n, _ := a.Ban("set", "3"); n != 1 || len(ids(a)) != 0 {
		t.Errorf("ban set: removed %d, left %v", n, ids(a))
	}
	r := a.Cfg.Get().Rules
	if len(r.BlacklistSets) != 1 || r.BlacklistSets[0] != 3 || len(r.BlacklistMapper) != 1 {
		t.Errorf("blacklists = %+v", r)
	}
	if _, err := a.Ban("set", "abc"); err == nil {
		t.Error("a non-numeric set id must fail")
	}
}
