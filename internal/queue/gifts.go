package queue

import (
	"strings"
	"time"
)

// Credit is what a viewer's gifts have earned so far: coins that have not yet
// added up to a priority, and unused priorities waiting for their next request.
type Credit struct {
	Coins   int       `json:"coins"`
	Tokens  int       `json:"tokens"`
	Expires time.Time `json:"expires"`
	Test    bool      `json:"test,omitempty"`
}

// AddCoins adds gift coins for user and turns every full `need` coins into a
// priority token. It returns how many new tokens were earned.
func (q *Queue) AddCoins(user string, coins, need int, ttl time.Duration, test bool, now time.Time) int {
	if need <= 0 || coins <= 0 {
		return 0
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.d.Credits == nil {
		q.d.Credits = map[string]*Credit{}
	}
	u := strings.ToLower(user)
	c := q.d.Credits[u]
	if c == nil || (c.Tokens > 0 && now.After(c.Expires)) {
		c = &Credit{}
		q.d.Credits[u] = c
	}
	c.Coins += coins
	earned := c.Coins / need
	c.Coins %= need
	c.Tokens += earned
	c.Test = c.Test || test
	if earned > 0 {
		c.Expires = now.Add(ttl)
	}
	q.changed()
	return earned
}

// UseToken spends one of user's priority tokens on their oldest waiting
// request that is not already prioritised. It returns that request.
func (q *Queue) UseToken(user string, now time.Time) (Item, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	c := q.d.Credits[strings.ToLower(user)]
	if c == nil || c.Tokens <= 0 || now.After(c.Expires) {
		return Item{}, false
	}
	for _, it := range q.d.Items {
		if strings.EqualFold(it.User, user) && !it.Priority {
			c.Tokens--
			q.prioritize(it)
			q.changed()
			return copyItem(it), true
		}
	}
	return Item{}, false
}

// prioritize marks it as priority and moves it behind the other priority
// requests at the top. q.mu must be held.
func (q *Queue) prioritize(it *Item) {
	it.Priority = true
	rest := make([]*Item, 0, len(q.d.Items))
	for _, x := range q.d.Items {
		if x != it {
			rest = append(rest, x)
		}
	}
	at := 0
	for at < len(rest) && rest[at].Priority {
		at++
	}
	q.d.Items = append(rest[:at], append([]*Item{it}, rest[at:]...)...)
}

// Tokens reports user's unused, unexpired priority tokens and coins so far.
func (q *Queue) Tokens(user string, now time.Time) (tokens, coins int) {
	q.mu.Lock()
	defer q.mu.Unlock()
	c := q.d.Credits[strings.ToLower(user)]
	if c == nil {
		return 0, 0
	}
	if now.After(c.Expires) {
		return 0, c.Coins
	}
	return c.Tokens, c.Coins
}
