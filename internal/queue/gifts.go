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
	// Gift-only requests (gifts.requestCoins): coins toward the next request
	// and requests paid for but not made yet. Counted apart from priority.
	ReqCoins   int       `json:"reqCoins,omitempty"`
	Requests   int       `json:"requests,omitempty"`
	ReqExpires time.Time `json:"reqExpires,omitempty"`
}

// AddCoins adds gift coins for user and turns every full `need` coins into a
// priority token. It returns how many new tokens were earned.
func (q *Queue) AddCoins(user string, coins, need int, ttl time.Duration, test bool, now time.Time) int {
	if need <= 0 || coins <= 0 {
		return 0
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	c := q.credit(user)
	if c.Tokens > 0 && now.After(c.Expires) {
		c.Coins, c.Tokens = 0, 0 // expired priority starts over (paid requests are kept)
	}
	c.markTest(test)
	c.Coins += coins
	earned := c.Coins / need
	c.Coins %= need
	c.Tokens += earned
	if earned > 0 {
		c.Expires = now.Add(ttl)
	}
	q.changed()
	return earned
}

// AddRequestCoins adds gift coins toward paid requests: every full `need`
// coins is one request. It returns how many requests were earned.
func (q *Queue) AddRequestCoins(user string, coins, need int, ttl time.Duration, test bool, now time.Time) int {
	if need <= 0 || coins <= 0 {
		return 0
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	c := q.credit(user)
	if c.Requests > 0 && now.After(c.ReqExpires) {
		c.Requests = 0
	}
	c.markTest(test)
	c.ReqCoins += coins
	earned := c.ReqCoins / need
	c.ReqCoins %= need
	c.Requests += earned
	if earned > 0 {
		c.ReqExpires = now.Add(ttl)
	}
	q.changed()
	return earned
}

// TakeRequest spends one paid request of user, if they have one.
func (q *Queue) TakeRequest(user string, now time.Time) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	c := q.d.Credits[strings.ToLower(user)]
	if c == nil || c.Requests <= 0 || now.After(c.ReqExpires) {
		return false
	}
	c.Requests--
	q.changed()
	return true
}

// RefundRequest gives back a paid request whose map was then rejected.
func (q *Queue) RefundRequest(user string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.credit(user).Requests++
	q.changed()
}

// Requests reports user's paid requests left and coins toward the next one.
func (q *Queue) Requests(user string, now time.Time) (requests, coins int) {
	q.mu.Lock()
	defer q.mu.Unlock()
	c := q.d.Credits[strings.ToLower(user)]
	if c == nil {
		return 0, 0
	}
	if now.After(c.ReqExpires) {
		return 0, c.ReqCoins
	}
	return c.Requests, c.ReqCoins
}

// markTest keeps Test true only while everything in the credit came from
// the test panel, so "Clear test data" never takes a viewer's real coins.
func (c *Credit) markTest(test bool) {
	empty := c.Coins == 0 && c.Tokens == 0 && c.ReqCoins == 0 && c.Requests == 0
	switch {
	case !test:
		c.Test = false
	case empty:
		c.Test = true
	}
}

// credit returns user's credit, creating it; q.mu must be held.
func (q *Queue) credit(user string) *Credit {
	if q.d.Credits == nil {
		q.d.Credits = map[string]*Credit{}
	}
	u := strings.ToLower(user)
	if q.d.Credits[u] == nil {
		q.d.Credits[u] = &Credit{}
	}
	return q.d.Credits[u]
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
