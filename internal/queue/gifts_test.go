package queue

import (
	"path/filepath"
	"testing"
	"time"
)

func openQ(t *testing.T) *Queue {
	q, err := Open(filepath.Join(t.TempDir(), "queue.json"))
	if err != nil {
		t.Fatal(err)
	}
	return q
}

func order(q *Queue) string {
	s := ""
	for _, it := range q.First(100) {
		if it.Priority {
			s += "*"
		}
		s += it.ID + " "
	}
	return s
}

func TestGiftCoinsAddUpAndPrioritise(t *testing.T) {
	q := openQ(t)
	now := time.Now()
	for _, x := range []struct{ id, user string }{{"a", "u1"}, {"b", "rich"}, {"c", "u2"}, {"d", "rich"}, {"e", "fan"}} {
		q.Add(&Item{ID: x.id, User: x.user, AddedAt: now})
	}
	if n := q.AddCoins("Rich", 20, 30, time.Hour, false, now); n != 0 {
		t.Fatalf("20 of 30 coins should earn nothing, got %d", n)
	}
	if n := q.AddCoins("rich", 45, 30, time.Hour, false, now); n != 2 {
		t.Fatalf("20+45 = 65 coins should earn 2, got %d", n)
	}
	if _, coins := q.Tokens("rich", now); coins != 5 {
		t.Errorf("5 coins should carry over, got %d", coins)
	}
	q.UseToken("rich", now)
	q.UseToken("rich", now)
	if got := order(q); got != "*b *d a c e " {
		t.Errorf("rich's requests should lead in their own order: %q", got)
	}
	// a later priority goes behind the earlier ones
	q.AddCoins("fan", 30, 30, time.Hour, false, now)
	q.UseToken("fan", now)
	if got := order(q); got != "*b *d *e a c " {
		t.Errorf("new priority should queue behind existing priority: %q", got)
	}
}

func TestGiftCreditWaitsAndExpires(t *testing.T) {
	q := openQ(t)
	now := time.Now()
	q.AddCoins("early", 30, 30, 10*time.Minute, false, now)
	if _, ok := q.UseToken("early", now); ok {
		t.Fatal("no request yet: the token must wait")
	}
	q.Add(&Item{ID: "x", User: "other", AddedAt: now})
	q.Add(&Item{ID: "y", User: "early", AddedAt: now})
	if _, ok := q.UseToken("early", now.Add(5*time.Minute)); !ok || order(q) != "*y x " {
		t.Fatalf("stored credit should apply to the next request: %q", order(q))
	}
	q.AddCoins("late", 30, 30, 10*time.Minute, false, now)
	q.Add(&Item{ID: "z", User: "late", AddedAt: now})
	if _, ok := q.UseToken("late", now.Add(11*time.Minute)); ok {
		t.Error("an expired credit must not apply")
	}
}

func TestClearTestRemovesTestCredits(t *testing.T) {
	q := openQ(t)
	now := time.Now()
	q.AddCoins("real", 10, 30, time.Hour, false, now)
	q.AddCoins("tester", 60, 30, time.Hour, true, now)
	q.ClearTest()
	if tok, _ := q.Tokens("tester", now); tok != 0 {
		t.Error("test credits should be cleared")
	}
	if _, coins := q.Tokens("real", now); coins != 10 {
		t.Error("real credits must stay")
	}
}

func TestRequestCoins(t *testing.T) {
	q := openQ(t)
	now := time.Now()
	if q.TakeRequest("fan", now) {
		t.Fatal("no gift, no request")
	}
	if n := q.AddRequestCoins("fan", 20, 30, time.Hour, false, now); n != 0 {
		t.Fatalf("20 of 30 coins earned %d", n)
	}
	if n := q.AddRequestCoins("Fan", 70, 30, time.Hour, false, now); n != 3 {
		t.Fatalf("90 coins in total should be 3 requests, got %d", n)
	}
	if r, c := q.Requests("fan", now); r != 3 || c != 0 {
		t.Errorf("requests=%d coins=%d", r, c)
	}
	if !q.TakeRequest("FAN", now) {
		t.Fatal("a paid request should be usable")
	}
	q.RefundRequest("fan")
	if r, _ := q.Requests("fan", now); r != 3 {
		t.Errorf("refund should give it back: %d", r)
	}
	if q.TakeRequest("fan", now.Add(2*time.Hour)) {
		t.Error("paid requests expire with creditMinutes")
	}

	// priority expiring must not wipe paid requests
	q.AddCoins("fan", 30, 30, time.Minute, false, now)
	q.AddRequestCoins("fan", 30, 30, 3*time.Hour, false, now.Add(2*time.Hour))
	q.AddCoins("fan", 5, 30, time.Minute, false, now.Add(2*time.Hour))
	if r, _ := q.Requests("fan", now.Add(2*time.Hour)); r != 1 {
		t.Errorf("paid request lost when priority expired: %d", r)
	}
}
