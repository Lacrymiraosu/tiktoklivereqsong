package app

import (
	"fmt"
	"log"
	"time"

	"github.com/Lacrymiraosu/tiktoklivereqsong/internal/tikfinity"
)

// HandleGift turns gift coins into queue priority (config gifts). Coins add
// up per viewer; every full gifts.coinsPerPriority moves that viewer's oldest
// waiting request to the top, or is kept for their next request.
func (a *App) HandleGift(g tikfinity.Gift) { a.gift(g, false) }

// SimulateGift is the test panel's version of HandleGift.
func (a *App) SimulateGift(user string, coins int) (tokens int, err error) {
	if !a.Cfg.Get().Gifts.Enabled {
		return 0, uerr("gifts_off", "gift priority is off (Settings → Queue)", nil)
	}
	user = testUser(user, "viewer")
	return a.gift(tikfinity.Gift{User: user, Nick: user, Name: "test gift", Coins: coins}, true), nil
}

func (a *App) gift(g tikfinity.Gift, test bool) int {
	cfg := a.Cfg.Get().Gifts
	if !cfg.Enabled {
		return 0
	}
	now := time.Now()
	earned := a.Queue.AddCoins(g.User, g.Coins, cfg.CoinsPerPriority, time.Duration(cfg.CreditMinutes)*time.Minute, test, now)
	log.Printf("gift: @%s sent %s (%d coins), %d priority earned", g.User, g.Name, g.Coins, earned)
	for i := 0; i < earned; i++ {
		ev := FeedEvent{Kind: "gift", User: g.User, Nick: g.Nick, Short: fmt.Sprintf("%d coins", g.Coins), Test: test}
		if it, ok := a.Queue.UseToken(g.User, now); ok {
			ev.Code, ev.Title, ev.Artist, ev.Version = "priority", it.Title, it.Artist, it.Version
			a.Say("info", "giftMoved", map[string]string{"u": g.User, "title": it.Artist + " - " + it.Title}, "@%s's gift moved %s - %s up", g.User, it.Artist, it.Title)
		} else {
			ev.Code = "credit" // no request waiting: applied to their next one
		}
		a.emit(ev)
	}
	return earned
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
