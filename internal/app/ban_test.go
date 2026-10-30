package app

import (
	"testing"
	"time"

	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/queue"
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

func TestBanSong(t *testing.T) {
	a := newTestApp(t, nil)
	now := time.Now()
	a.Queue.Add(&queue.Item{ID: "a", User: "u1", SetID: 1, Artist: "xi", Title: "FREEDOM DiVE (Cut Ver.)", AddedAt: now})
	a.Queue.Add(&queue.Item{ID: "b", User: "u2", SetID: 2, Artist: "xi", Title: "Blue Zenith", AddedAt: now})
	n, err := a.Ban("song", "xi - FREEDOM DiVE")
	if err != nil || n != 1 {
		t.Fatalf("ban song: %d %v", n, err)
	}
	if got := ids(a); len(got) != 1 || got[0] != "b" {
		t.Errorf("only the banned song's request goes: %v", got)
	}
	a.Ban("song", "Xi - Freedom Dive")
	if l := a.Cfg.Get().Rules.BlacklistSongs; len(l) != 1 {
		t.Errorf("the same song is listed once: %v", l)
	}
}

func TestBanArtist(t *testing.T) {
	a := newTestApp(t, nil)
	now := time.Now()
	a.Queue.Add(&queue.Item{ID: "a", User: "u1", SetID: 1, Artist: "a_hisa", Title: "One", AddedAt: now})
	a.Queue.Add(&queue.Item{ID: "b", User: "u2", SetID: 2, Artist: "a_hisa feat. x", Title: "Two", AddedAt: now})
	a.Queue.Add(&queue.Item{ID: "c", User: "u3", SetID: 3, Artist: "xi", Title: "Three", AddedAt: now})
	if n, err := a.Ban("artist", "a_hisa"); err != nil || n != 2 {
		t.Fatalf("ban artist: %d %v", n, err)
	}
	if got := ids(a); len(got) != 1 || got[0] != "c" {
		t.Errorf("queue: %v", got)
	}
	if l := a.Cfg.Get().Rules.BlacklistSongs; len(l) != 1 || l[0] != `artist="a_hisa"` {
		t.Errorf("stored rule: %v", l)
	}
	a.Queue.Add(&queue.Item{ID: "j", User: "u4", SetID: 4, Artist: "Nanahira", ArtistUnicode: "ななひら", Title: "Four", AddedAt: now})
	if n, _ := a.Ban("song", `artist="ななひら"`); n != 1 {
		t.Errorf("a rule in the original script should also drop waiting requests, removed %d", n)
	}
	if _, err := a.Ban("song", `artist=""`); err == nil {
		t.Error("an invalid rule should be refused")
	}
}
