package app

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/tikfinity"
)

// HandleGift turns gift coins into queue priority (config gifts). Coins add
// up per viewer; every full gifts.coinsPerPriority moves that viewer's oldest
// waiting request to the top, or is kept for their next request.
func (a *App) HandleGift(g tikfinity.Gift) { a.gift(g, false) }

// SimulateGift is the test panel's version of HandleGift. It returns the
// priorities and paid requests the gift earned.
func (a *App) SimulateGift(user string, coins int) (tokens, requests int, err error) {
	if c := a.Cfg.Get().Gifts; !c.Enabled && c.RequestCoins == 0 {
		return 0, 0, uerr("gifts_off", "gift priority and gift-only requests are off (Settings → Queue)", nil)
	}
	user = testUser(user, "viewer")
	tokens, requests = a.gift(tikfinity.Gift{User: user, Nick: user, Name: "test gift", Coins: coins}, true)
	return tokens, requests, nil
}

// gift counts the coins toward paid requests (gifts.requestCoins) and toward
// priority (gifts.enabled), each on its own.
func (a *App) gift(g tikfinity.Gift, test bool) (earned, requests int) {
	a.rememberAvatar(g.User, g.Avatar)
	a.mu.Lock()
	if a.gifters == nil {
		a.gifters = map[string]time.Time{}
	}
	a.gifters[strings.ToLower(g.User)] = time.Now() // for TTS "gifters only"
	a.mu.Unlock()
	a.announce(FeedEvent{Kind: "giftthanks", User: g.User, Nick: g.Nick, Test: test, Vars: map[string]string{"coins": strconv.Itoa(g.Coins), "gift": g.Name}})
	cfg := a.Cfg.Get().Gifts
	now := time.Now()
	ttl := time.Duration(cfg.CreditMinutes) * time.Minute
	if cfg.RequestCoins > 0 {
		if requests = a.Queue.AddRequestCoins(g.User, g.Coins, cfg.RequestCoins, ttl, test, now); requests > 0 {
			left, _ := a.Queue.Requests(g.User, now)
			log.Printf("gift: @%s sent %s (%d coins), %d request(s) earned", g.User, g.Name, g.Coins, requests)
			a.emit(FeedEvent{Kind: "gift", Code: "requests", User: g.User, Nick: g.Nick, Short: fmt.Sprintf("%d coins", g.Coins), Pos: left, Test: test,
				Vars: map[string]string{"coins": strconv.Itoa(g.Coins)}})
		}
	}
	if !cfg.Enabled {
		return 0, requests
	}
	earned = a.Queue.AddCoins(g.User, g.Coins, cfg.CoinsPerPriority, ttl, test, now)
	log.Printf("gift: @%s sent %s (%d coins), %d priority earned", g.User, g.Name, g.Coins, earned)
	for i := 0; i < earned; i++ {
		ev := FeedEvent{Kind: "gift", User: g.User, Nick: g.Nick, Short: fmt.Sprintf("%d coins", g.Coins), Test: test, Vars: map[string]string{"coins": strconv.Itoa(g.Coins)}}
		if it, ok := a.Queue.UseToken(g.User, now); ok {
			ev.Code, ev.Title, ev.Artist, ev.Version = "priority", it.Title, it.Artist, it.Version
			a.Say("info", "giftMoved", map[string]string{"u": g.User, "title": it.Artist + " - " + it.Title}, "@%s's gift moved %s - %s up", g.User, it.Artist, it.Title)
		} else {
			ev.Code = "credit" // no request waiting: applied to their next one
		}
		a.emit(ev)
	}
	return earned, requests
}

// applyCredit gives a just-added request the priority its owner already paid for.
func (a *App) applyCredit(user, nick string, test bool) {
	if !a.Cfg.Get().Gifts.Enabled {
		return
	}
	if it, ok := a.Queue.UseToken(user, time.Now()); ok {
		a.emit(FeedEvent{Kind: "gift", Code: "priority", User: user, Nick: nick, Title: it.Title, Artist: it.Artist, Version: it.Version, Test: test})
	}
}
