package app

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/queue"
	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/validator"
)

// Ban adds a user, beatmapset, mapper or song ("Artist - Title") to the blacklist and drops the
// matching waiting requests (never the one being played). It returns how
// many requests were removed.
func (a *App) Ban(kind, value string) (int, error) {
	value = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(value), "@"))
	if value == "" {
		return 0, uerr("ban_empty", "nothing to ban", nil)
	}
	cfg := a.Cfg.Get()
	var match func(queue.Item) bool
	switch kind {
	case "user":
		if !containsFold(cfg.Rules.BlacklistUsers, value) {
			cfg.Rules.BlacklistUsers = append(cfg.Rules.BlacklistUsers, value)
		}
		match = func(it queue.Item) bool { return strings.EqualFold(it.User, value) }
	case "set":
		id, err := strconv.Atoi(value)
		if err != nil || id <= 0 {
			return 0, uerr("ban_set_id", fmt.Sprintf("not a beatmapset id: %q", value), map[string]string{"v": value})
		}
		if !slices.Contains(cfg.Rules.BlacklistSets, id) {
			cfg.Rules.BlacklistSets = append(cfg.Rules.BlacklistSets, id)
		}
		match = func(it queue.Item) bool { return it.SetID == id }
	case "song":
		artist, title, _ := strings.Cut(value, " - ")
		key := validator.SongKey(artist, title)
		if _, dup := validator.SongBanned(cfg.Rules.BlacklistSongs, artist, title); !dup {
			cfg.Rules.BlacklistSongs = append(cfg.Rules.BlacklistSongs, value)
		}
		match = func(it queue.Item) bool { return validator.SongKey(it.Artist, it.Title) == key }
	case "mapper":
		if !containsFold(cfg.Rules.BlacklistMapper, value) {
			cfg.Rules.BlacklistMapper = append(cfg.Rules.BlacklistMapper, value)
		}
		match = func(it queue.Item) bool { return strings.EqualFold(it.Creator, value) }
	default:
		return 0, uerr("ban_kind", fmt.Sprintf("unknown ban kind %q", kind), nil)
	}
	if err := a.Cfg.Set(cfg); err != nil {
		return 0, err
	}
	removed := 0
	for _, it := range a.Queue.First(1 << 30) {
		if match(it) {
			a.Cancel(it.ID)
			if a.Queue.Remove(it.ID) {
				removed++
			}
		}
	}
	a.Say("info", "banned_"+kind, map[string]string{"v": value, "n": strconv.Itoa(removed)}, "banned %s %s (removed %d waiting request(s))", kind, value, removed)
	return removed, nil
}

func containsFold(list []string, v string) bool {
	for _, x := range list {
		if strings.EqualFold(strings.TrimPrefix(strings.TrimSpace(x), "@"), v) {
			return true
		}
	}
	return false
}
