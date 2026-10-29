// Package queue holds requests, now playing, play history and the rejection
// log, and persists them so the queue survives restarts.
package queue

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/draft/osureq/internal/validator"
)

type State string

const (
	Queued      State = "queued"
	Downloading State = "downloading"
	Ready       State = "ready"
	Failed      State = "failed"
	Playing     State = "playing"
)

type Attempt struct {
	Source   string `json:"source"`
	Status   string `json:"status"` // running | ok | failed | mismatch | skipped
	Received int64  `json:"received"`
	Total    int64  `json:"total"`
	Message  string `json:"message,omitempty"`
}

type Item struct {
	ID        string    `json:"id"`
	User      string    `json:"user"`
	Nick      string    `json:"nick"`
	SetID     int       `json:"setId"`
	BeatmapID int       `json:"beatmapId"` // diff shown as requested
	BySet     bool      `json:"bySet"`     // viewer requested the whole set
	Title     string    `json:"title"`
	Artist    string    `json:"artist"`
	Creator   string    `json:"creator"`
	Version   string    `json:"version"`
	Mode      string    `json:"mode"`
	Stars     float64   `json:"stars"`
	Length    int       `json:"length"`
	Status    string    `json:"status"`
	Cover     string    `json:"cover"`
	AddedAt   time.Time `json:"addedAt"`
	Test      bool      `json:"test,omitempty"`      // added from the dashboard's test panel
	Simulated bool      `json:"simulated,omitempty"` // simulated chat: counts for cooldown/limits like a real one

	State     State     `json:"state"`
	File      string    `json:"file,omitempty"`
	Source    string    `json:"source,omitempty"`
	MD5       string    `json:"md5,omitempty"` // ok | cached | installed | unverified | mismatch
	Warning   string    `json:"warning,omitempty"`
	Error     string    `json:"error,omitempty"`
	Attempts  []Attempt `json:"attempts,omitempty"`
	Imported  bool      `json:"imported,omitempty"`
	StartedAt time.Time `json:"startedAt,omitempty"`
	// what tosu has seen of the now-playing map: waiting | playing | results | ended
	PlayPhase string    `json:"playPhase,omitempty"`
	PhaseAt   time.Time `json:"phaseAt,omitempty"`
	Result    *Result   `json:"result,omitempty"` // how the streamer did, from tosu
}

// Result is how a requested map went, as tosu saw it.
type Result struct {
	Outcome  string  `json:"outcome"` // passed | failed | quit
	Accuracy float64 `json:"accuracy"`
	Rank     string  `json:"rank,omitempty"`
	MaxCombo int     `json:"maxCombo"`
	Score    int64   `json:"score"`
	PP       float64 `json:"pp,omitempty"`
	Mods     string  `json:"mods,omitempty"`
	Misses   int     `json:"misses"`
}

type Played struct {
	SetID  int       `json:"setId"`
	Title  string    `json:"title"`
	User   string    `json:"user"`
	At     time.Time `json:"at"`
	Result string    `json:"result"` // played | skipped
	Test   bool      `json:"test,omitempty"`
	Play   *Result   `json:"play,omitempty"` // tosu's numbers, when the map was played
}

type Rejected struct {
	At     time.Time `json:"at"`
	User   string    `json:"user"`
	Input  string    `json:"input"`
	Code   string    `json:"code"`
	Detail string    `json:"detail"`
	Short  string    `json:"short,omitempty"`
	Test   bool      `json:"test,omitempty"`
}

type data struct {
	Items    []*Item              `json:"items"`
	Now      *Item                `json:"now"`
	History  []Played             `json:"history"`
	LastReq  map[string]time.Time `json:"lastRequest"`
	Rejected []Rejected           `json:"rejected"`
	// users whose cooldown was set by a simulated chat, reset by ClearTest
	TestUsers map[string]bool `json:"testUsers,omitempty"`
}

// Snapshot is a deep enough copy for rendering.
type Snapshot struct {
	Items    []Item     `json:"items"`
	Now      *Item      `json:"now"`
	History  []Played   `json:"history"`
	Rejected []Rejected `json:"rejected"`
}

type Queue struct {
	mu       sync.Mutex
	d        data
	path     string
	onChange func()
	saveT    *time.Timer
}

const (
	maxHistory  = 200
	maxRejected = 100
)

func Open(path string) (*Queue, error) {
	q := &Queue{path: path, d: data{LastReq: map[string]time.Time{}}}
	b, err := os.ReadFile(path)
	if err == nil {
		if err := json.Unmarshal(b, &q.d); err != nil {
			return nil, err
		}
		if q.d.LastReq == nil {
			q.d.LastReq = map[string]time.Time{}
		}
		// Downloads did not survive the restart, and only files verified in
		// this session may be opened: send everything back through the cache check.
		for _, it := range q.d.Items {
			if it.State == Downloading || it.State == Ready {
				it.State, it.File, it.Attempts = Queued, "", nil
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	return q, nil
}

func (q *Queue) OnChange(f func()) { q.onChange = f }

// changed must be called with q.mu held.
func (q *Queue) changed() {
	if q.saveT == nil {
		q.saveT = time.AfterFunc(300*time.Millisecond, q.Flush)
	} else {
		q.saveT.Reset(300 * time.Millisecond)
	}
	if q.onChange != nil {
		go q.onChange()
	}
}

func (q *Queue) Flush() {
	q.mu.Lock()
	b, err := json.MarshalIndent(q.d, "", " ")
	q.mu.Unlock()
	if err != nil {
		return
	}
	tmp := q.path + ".tmp"
	if os.WriteFile(tmp, b, 0o644) == nil {
		os.Rename(tmp, q.path)
	}
}

func NewID() string {
	b := make([]byte, 6)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// ValidationState builds the validator snapshot for a user.
func (q *Queue) ValidationState(user string) validator.State {
	q.mu.Lock()
	defer q.mu.Unlock()
	st := validator.State{
		QueueLen:    len(q.d.Items),
		LastRequest: q.d.LastReq[strings.ToLower(user)],
		QueuedSets:  map[int]bool{},
		PlayedAt:    map[int]time.Time{},
	}
	// The map being played no longer counts toward the user's limit, so a
	// viewer can queue their next request while their current one plays.
	if q.d.Now != nil {
		st.QueuedSets[q.d.Now.SetID] = true
	}
	for _, it := range q.d.Items {
		st.QueuedSets[it.SetID] = true
		if strings.EqualFold(it.User, user) {
			st.UserActive++
		}
	}
	for _, h := range q.d.History {
		if h.Result == "played" && h.At.After(st.PlayedAt[h.SetID]) {
			st.PlayedAt[h.SetID] = h.At
		}
	}
	return st
}

func (q *Queue) Add(it *Item) {
	q.mu.Lock()
	defer q.mu.Unlock()
	it.State = Queued
	q.d.Items = append(q.d.Items, it)
	if !it.Test || it.Simulated {
		u := strings.ToLower(it.User)
		q.d.LastReq[u] = it.AddedAt
		if it.Simulated {
			if q.d.TestUsers == nil {
				q.d.TestUsers = map[string]bool{}
			}
			q.d.TestUsers[u] = true
		}
	}
	q.changed()
}

// ClearTest removes everything the test panel created: test requests (also
// when playing), their history and rejections, and simulated cooldowns. It
// returns the ids of removed items so running downloads can be cancelled.
func (q *Queue) ClearTest() []string {
	q.mu.Lock()
	defer q.mu.Unlock()
	var ids []string
	items := q.d.Items[:0]
	for _, it := range q.d.Items {
		if it.Test {
			ids = append(ids, it.ID)
		} else {
			items = append(items, it)
		}
	}
	q.d.Items = items
	if q.d.Now != nil && q.d.Now.Test {
		ids = append(ids, q.d.Now.ID)
		q.d.Now = nil
	}
	hist := q.d.History[:0]
	for _, h := range q.d.History {
		if !h.Test {
			hist = append(hist, h)
		}
	}
	q.d.History = hist
	rej := q.d.Rejected[:0]
	for _, r := range q.d.Rejected {
		if !r.Test {
			rej = append(rej, r)
		}
	}
	q.d.Rejected = rej
	for u := range q.d.TestUsers {
		delete(q.d.LastReq, u)
	}
	q.d.TestUsers = nil
	q.changed()
	return ids
}

func (q *Queue) Reject(r Rejected) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.d.Rejected = append([]Rejected{r}, q.d.Rejected...)
	if len(q.d.Rejected) > maxRejected {
		q.d.Rejected = q.d.Rejected[:maxRejected]
	}
	q.changed()
}

func RejectedFrom(user, input string, r *validator.Rejection) Rejected {
	return Rejected{At: time.Now(), User: user, Input: input, Code: r.Code, Detail: r.Detail, Short: r.Short}
}

// Update runs f on the item (queued or now playing) with the lock held.
func (q *Queue) Update(id string, f func(*Item)) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	it := q.find(id)
	if it == nil {
		return false
	}
	f(it)
	q.changed()
	return true
}

// Get returns a copy of an item.
func (q *Queue) Get(id string) (Item, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if it := q.find(id); it != nil {
		return copyItem(it), true
	}
	return Item{}, false
}

func (q *Queue) find(id string) *Item {
	if q.d.Now != nil && q.d.Now.ID == id {
		return q.d.Now
	}
	for _, it := range q.d.Items {
		if it.ID == id {
			return it
		}
	}
	return nil
}

func (q *Queue) Remove(id string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	for i, it := range q.d.Items {
		if it.ID == id {
			q.d.Items = append(q.d.Items[:i], q.d.Items[i+1:]...)
			q.changed()
			return true
		}
	}
	return false
}

// Drop removes a queued item and records it in the history with result.
func (q *Queue) Drop(id, result string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	for i, it := range q.d.Items {
		if it.ID == id {
			q.d.Items = append(q.d.Items[:i], q.d.Items[i+1:]...)
			q.record(it, result)
			q.changed()
			return true
		}
	}
	return false
}

// RemoveLatestBy removes the most recently added waiting request of user.
func (q *Queue) RemoveLatestBy(user string) (Item, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	idx := -1
	for i, it := range q.d.Items {
		if strings.EqualFold(it.User, user) && (idx < 0 || it.AddedAt.After(q.d.Items[idx].AddedAt)) {
			idx = i
		}
	}
	if idx < 0 {
		return Item{}, false
	}
	it := q.d.Items[idx]
	q.d.Items = append(q.d.Items[:idx], q.d.Items[idx+1:]...)
	q.changed()
	return copyItem(it), true
}

// RemoveLast removes the most recently added request.
func (q *Queue) RemoveLast() (Item, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	idx := -1
	for i, it := range q.d.Items {
		if idx < 0 || it.AddedAt.After(q.d.Items[idx].AddedAt) {
			idx = i
		}
	}
	if idx < 0 {
		return Item{}, false
	}
	it := q.d.Items[idx]
	q.d.Items = append(q.d.Items[:idx], q.d.Items[idx+1:]...)
	q.changed()
	return copyItem(it), true
}

// Move puts item id at position to (0 = top).
func (q *Queue) Move(id string, to int) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	from := -1
	for i, it := range q.d.Items {
		if it.ID == id {
			from = i
		}
	}
	if from < 0 {
		return false
	}
	it := q.d.Items[from]
	rest := append(append([]*Item{}, q.d.Items[:from]...), q.d.Items[from+1:]...)
	to = max(0, min(to, len(rest)))
	q.d.Items = append(rest[:to], append([]*Item{it}, rest[to:]...)...)
	q.changed()
	return true
}

func (q *Queue) Clear() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.d.Items = nil
	q.changed()
}

// Finish moves now playing into history.
func (q *Queue) Finish(result string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.finishLocked(result)
}

func (q *Queue) finishLocked(result string) {
	if q.d.Now == nil {
		return
	}
	q.record(q.d.Now, result)
	q.d.Now = nil
	q.changed()
}

// record adds it to the history; q.mu must be held.
func (q *Queue) record(it *Item, result string) {
	q.d.History = append([]Played{{SetID: it.SetID, Title: it.Artist + " - " + it.Title, User: it.User, At: time.Now(), Result: result, Test: it.Test, Play: it.Result}}, q.d.History...)
	if len(q.d.History) > maxHistory {
		q.d.History = q.d.History[:maxHistory]
	}
}

// Promote makes item id now playing (finishing the current one as result).
func (q *Queue) Promote(id, result string) (Item, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for i, it := range q.d.Items {
		if it.ID == id {
			q.finishLocked(result)
			q.d.Items = append(q.d.Items[:i], q.d.Items[i+1:]...)
			it.State = Playing
			it.StartedAt = time.Now()
			it.PlayPhase, it.PhaseAt = "waiting", it.StartedAt
			q.d.Now = it
			q.changed()
			return copyItem(it), true
		}
	}
	return Item{}, false
}

// Position returns the 1-based place of a waiting item (0 if not queued).
func (q *Queue) Position(id string) int {
	q.mu.Lock()
	defer q.mu.Unlock()
	for i, it := range q.d.Items {
		if it.ID == id {
			return i + 1
		}
	}
	return 0
}

// Head returns the first item, if any.
func (q *Queue) Head() (Item, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.d.Items) == 0 {
		return Item{}, false
	}
	return copyItem(q.d.Items[0]), true
}

// Now returns the item being played.
func (q *Queue) Now() (Item, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.d.Now == nil {
		return Item{}, false
	}
	return copyItem(q.d.Now), true
}

// First returns copies of the first n queued items.
func (q *Queue) First(n int) []Item {
	q.mu.Lock()
	defer q.mu.Unlock()
	var out []Item
	for i := 0; i < len(q.d.Items) && i < n; i++ {
		out = append(out, copyItem(q.d.Items[i]))
	}
	return out
}

// Files returns every .osz path still referenced by the queue.
func (q *Queue) Files() map[string]bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	out := map[string]bool{}
	for _, it := range q.d.Items {
		if it.File != "" {
			out[it.File] = true
		}
	}
	if q.d.Now != nil && q.d.Now.File != "" {
		out[q.d.Now.File] = true
	}
	return out
}

func (q *Queue) Snapshot() Snapshot {
	q.mu.Lock()
	defer q.mu.Unlock()
	s := Snapshot{History: append([]Played{}, q.d.History...), Rejected: append([]Rejected{}, q.d.Rejected...)}
	if len(s.History) > 30 {
		s.History = s.History[:30]
	}
	for _, it := range q.d.Items {
		s.Items = append(s.Items, copyItem(it))
	}
	if q.d.Now != nil {
		n := copyItem(q.d.Now)
		s.Now = &n
	}
	return s
}

func copyItem(it *Item) Item {
	c := *it
	c.Attempts = append([]Attempt(nil), it.Attempts...)
	return c
}
