// Package leases is the in-memory book of named resource leases: per queue one holder and ranked
// waiters. It knows nothing of sessions or the app; callers pass keys and callbacks.
package leases

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"
)

// DefaultTTL is how long a holder may stay silent before Sweep frees its lease.
const DefaultTTL = 10 * time.Minute

const maxNameLen = 64

var (
	ErrUnknownQueue = errors.New("unknown lease queue")
	ErrNoHolder     = errors.New("lease queue has no holder")

	namePattern = regexp.MustCompile(`^[A-Za-z0-9 ._-]+$`)
)

// Key identifies a lease owner: the session record id, or "run:<runID>" for a headless run.
type Key string

const (
	StateHeld   = "held"
	StateQueued = "queued"
)

// Reason says why a Grant fired.
type Reason string

const (
	ReasonGranted Reason = "granted" // a waiter became holder
	ReasonRevoked Reason = "revoked" // the user took the lease away
	ReasonExpired Reason = "expired" // the holder went silent past the TTL
	ReasonRemoved Reason = "removed" // the user deleted the queue a waiter was waiting in
)

// Grant is a lease change a key must hear about.
type Grant struct {
	Queue  string
	Key    Key
	TaskID string
	Reason Reason
}

// Entry is one holder or waiter.
type Entry struct {
	Key          Key
	TaskID       string
	Seq          uint64
	Since, Seen  time.Time
	ReleaseAsked bool
}

// Queue is a snapshot of one named queue, waiters in grant order.
type Queue struct {
	Name    string
	Holder  *Entry
	Waiters []Entry
}

// Status answers Acquire. Position is 0 when held, else the 1-based place in line.
type Status struct {
	Queue      string
	State      string
	Position   int
	HolderTask string
	TTLSeconds int
}

type queue struct {
	name    string
	holder  *Entry
	waiters []Entry
}

// Book holds every queue. All methods are non-blocking and safe for concurrent use. Callbacks run
// outside the lock. order is called outside the lock, at most once per mutating call.
type Book struct {
	mu       sync.Mutex
	queues   map[string]*queue
	names    []string
	seq      uint64
	now      func() time.Time
	ttl      time.Duration
	order    func() []string
	onGrant  func(Grant)
	onChange func()
}

// Options configures a Book. Zero values give the wall clock, DefaultTTL and no callbacks.
type Options struct {
	Now      func() time.Time
	TTL      time.Duration
	Order    func() []string // task ids, highest priority first
	OnGrant  func(Grant)
	OnChange func()
}

func New(o Options) *Book {
	b := &Book{queues: map[string]*queue{}, now: o.Now, ttl: o.TTL, order: o.Order, onGrant: o.OnGrant, onChange: o.OnChange}
	if b.now == nil {
		b.now = time.Now
	}
	if b.ttl <= 0 {
		b.ttl = DefaultTTL
	}
	return b
}

// TTL is the silence limit Sweep applies.
func (b *Book) TTL() time.Duration { return b.ttl }

// ranker maps a task id to its priority index; unlisted tasks rank after all listed ones.
type ranker map[string]int

func (b *Book) ranker() ranker {
	if b.order == nil {
		return ranker{}
	}
	ids := b.order()
	r := make(ranker, len(ids))
	for i, id := range ids {
		if _, ok := r[id]; !ok {
			r[id] = i
		}
	}
	return r
}

// lockRanked locks b.mu and returns a ranker for the queues as they stand. Ranking asks the board
// for its task order, which is costly and must run outside the lock, so it is read only when some
// queue holds at least minWaiters waiters (fewer cannot be out of order) and is re-checked once the
// lock is back.
func (b *Book) lockRanked(minWaiters int) ranker {
	var rk ranker
	loaded := false
	for {
		b.mu.Lock()
		if loaded || b.maxWaiters() < minWaiters {
			return rk
		}
		b.mu.Unlock()
		rk, loaded = b.ranker(), true
	}
}

// maxWaiters is the longest wait line. b.mu is held.
func (b *Book) maxWaiters() int {
	most := 0
	for _, q := range b.queues {
		most = max(most, len(q.waiters))
	}
	return most
}

func (r ranker) of(taskID string) int {
	if i, ok := r[taskID]; ok {
		return i
	}
	return len(r)
}

// sorted returns the waiters by (task rank, arrival).
func (r ranker) sorted(ws []Entry) []Entry {
	out := slices.Clone(ws)
	sort.SliceStable(out, func(i, j int) bool {
		ri, rj := r.of(out[i].TaskID), r.of(out[j].TaskID)
		if ri != rj {
			return ri < rj
		}
		return out[i].Seq < out[j].Seq
	})
	return out
}

// effects collects callbacks while the lock is held; run them after unlocking.
type effects struct {
	grants  []Grant
	changed bool
}

func (b *Book) run(e effects) {
	if b.onGrant != nil {
		for _, g := range e.grants {
			b.onGrant(g)
		}
	}
	if e.changed && b.onChange != nil {
		b.onChange()
	}
}

// promote makes the best-ranked waiter the holder. It never touches a present holder.
func (b *Book) promote(q *queue, rk ranker, e *effects) {
	if q.holder != nil || len(q.waiters) == 0 {
		return
	}
	next := rk.sorted(q.waiters)[0]
	q.waiters = slices.DeleteFunc(q.waiters, func(w Entry) bool { return w.Seq == next.Seq })
	now := b.now()
	next.Since, next.Seen, next.ReleaseAsked = now, now, false
	q.holder = &next
	e.grants = append(e.grants, Grant{Queue: q.name, Key: next.Key, TaskID: next.TaskID, Reason: ReasonGranted})
	e.changed = true
}

func (b *Book) status(q *queue, key Key, rk ranker) Status {
	st := Status{Queue: q.name, State: StateQueued, TTLSeconds: int(b.ttl / time.Second)}
	if q.holder != nil {
		st.HolderTask = q.holder.TaskID
		if q.holder.Key == key {
			st.State = StateHeld
			return st
		}
	}
	for i, w := range rk.sorted(q.waiters) {
		if w.Key == key {
			st.Position = i + 1
			break
		}
	}
	return st
}

// ValidateNames checks queue names the way SetQueues does and returns them trimmed.
func ValidateNames(names []string) ([]string, error) {
	out := make([]string, 0, len(names))
	seen := map[string]bool{}
	for _, n := range names {
		n = strings.TrimSpace(n)
		switch {
		case n == "":
			return nil, errors.New("queue name is empty")
		case len(n) > maxNameLen:
			return nil, fmt.Errorf("queue name %q is over %d characters", n, maxNameLen)
		case !namePattern.MatchString(n):
			return nil, fmt.Errorf("queue name %q may only hold letters, digits, space, dot, underscore and hyphen", n)
		case seen[n]:
			return nil, fmt.Errorf("queue name %q is listed twice", n)
		}
		seen[n] = true
		out = append(out, n)
	}
	return out, nil
}

// SetQueues replaces the queue names in the given order. A removed queue drops its holder and
// waiters; the holder hears a revoked grant, each waiter a removed one.
func (b *Book) SetQueues(names []string) error {
	names, err := ValidateNames(names)
	if err != nil {
		return err
	}
	var e effects
	b.mu.Lock()
	keep := map[string]bool{}
	for _, n := range names {
		keep[n] = true
		if b.queues[n] == nil {
			b.queues[n] = &queue{name: n}
		}
	}
	for n, q := range b.queues {
		if keep[n] {
			continue
		}
		if q.holder != nil {
			e.grants = append(e.grants, Grant{Queue: n, Key: q.holder.Key, TaskID: q.holder.TaskID, Reason: ReasonRevoked})
		}
		for _, w := range q.waiters {
			e.grants = append(e.grants, Grant{Queue: n, Key: w.Key, TaskID: w.TaskID, Reason: ReasonRemoved})
		}
		delete(b.queues, n)
	}
	changed := !slices.Equal(b.names, names)
	b.names = names
	e.changed = changed
	b.mu.Unlock()
	b.run(e)
	return nil
}

// Names lists the queue names in order.
func (b *Book) Names() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return slices.Clone(b.names)
}

// Acquire takes the lease or queues for it, never blocking. A repeat call keeps the place.
func (b *Book) Acquire(key Key, taskID, name string) (Status, error) {
	// One more waiter may join, so a line of one is the first that needs ordering.
	rk := b.lockRanked(1)
	var e effects
	q := b.queues[name]
	if q == nil {
		names := slices.Clone(b.names)
		b.mu.Unlock()
		return Status{}, fmt.Errorf("%w %q; queues: %s", ErrUnknownQueue, name, listNames(names))
	}
	switch {
	case q.holder != nil && q.holder.Key == key:
		q.holder.Seen = b.now()
	case slices.ContainsFunc(q.waiters, func(w Entry) bool { return w.Key == key }):
	case q.holder == nil:
		b.seq++
		q.waiters = append(q.waiters, Entry{Key: key, TaskID: taskID, Seq: b.seq})
		b.promote(q, rk, &e)
	default:
		b.seq++
		q.waiters = append(q.waiters, Entry{Key: key, TaskID: taskID, Seq: b.seq})
		e.changed = true
	}
	st := b.status(q, key, rk)
	b.mu.Unlock()
	// The caller learns of its own grant from st, not from a callback.
	e.grants = slices.DeleteFunc(e.grants, func(g Grant) bool { return g.Key == key && g.Reason == ReasonGranted })
	b.run(e)
	return st, nil
}

func listNames(names []string) string {
	if len(names) == 0 {
		return "none configured"
	}
	return strings.Join(names, ", ")
}

// Release frees key's holds and waits in one queue, or in every queue when queue is empty. It
// returns the queue names touched and grants each freed queue's next waiter.
func (b *Book) Release(key Key, name string) []string {
	rk := b.lockRanked(2)
	var e effects
	var released []string
	for _, n := range b.names {
		if name != "" && n != name {
			continue
		}
		q := b.queues[n]
		touched := false
		if q.holder != nil && q.holder.Key == key {
			q.holder, touched = nil, true
		}
		before := len(q.waiters)
		q.waiters = slices.DeleteFunc(q.waiters, func(w Entry) bool { return w.Key == key })
		if len(q.waiters) != before {
			touched = true
		}
		if touched {
			released = append(released, n)
			e.changed = true
			b.promote(q, rk, &e)
		}
	}
	b.mu.Unlock()
	b.run(e)
	return released
}

// ReleaseSession frees everything key holds or waits for; used when its session ends.
func (b *Book) ReleaseSession(key Key) { b.Release(key, "") }

// ForceRelease drops the holder, grants the next waiter and tells the old holder it was revoked.
func (b *Book) ForceRelease(name string) (Key, error) {
	rk := b.lockRanked(2)
	var e effects
	q := b.queues[name]
	if q == nil || q.holder == nil {
		b.mu.Unlock()
		if q == nil {
			return "", fmt.Errorf("%w %q", ErrUnknownQueue, name)
		}
		return "", fmt.Errorf("%w %q", ErrNoHolder, name)
	}
	old := *q.holder
	q.holder = nil
	e.grants = append(e.grants, Grant{Queue: name, Key: old.Key, TaskID: old.TaskID, Reason: ReasonRevoked})
	e.changed = true
	b.promote(q, rk, &e)
	b.mu.Unlock()
	b.run(e)
	return old.Key, nil
}

// RequestRelease flags the holder as asked to finish; it changes nothing else.
func (b *Book) RequestRelease(name string) (Key, error) {
	var e effects
	b.mu.Lock()
	q := b.queues[name]
	if q == nil || q.holder == nil {
		b.mu.Unlock()
		if q == nil {
			return "", fmt.Errorf("%w %q", ErrUnknownQueue, name)
		}
		return "", fmt.Errorf("%w %q", ErrNoHolder, name)
	}
	q.holder.ReleaseAsked = true
	key := q.holder.Key
	e.changed = true
	b.mu.Unlock()
	b.run(e)
	return key, nil
}

// Touch extends every hold of key: any sign of life counts as a heartbeat.
func (b *Book) Touch(key Key) {
	now := b.now()
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, q := range b.queues {
		if q.holder != nil && q.holder.Key == key {
			q.holder.Seen = now
		}
	}
}

// Sweep frees each holder silent for more than the TTL unless alive(key), which is true while the
// holder is mid-turn, so a long tool run never expires. alive runs outside the lock.
func (b *Book) Sweep(alive func(Key) bool) {
	dead := map[Key]bool{}
	b.mu.Lock()
	now := b.now()
	var cands []Key
	for _, q := range b.queues {
		if q.holder != nil && now.Sub(q.holder.Seen) > b.ttl {
			cands = append(cands, q.holder.Key)
		}
	}
	b.mu.Unlock()
	for _, k := range cands {
		if alive == nil || !alive(k) {
			dead[k] = true
		}
	}
	if len(dead) == 0 {
		return
	}
	rk := b.lockRanked(2)
	var e effects
	now = b.now()
	for _, n := range b.names {
		q := b.queues[n]
		if q.holder == nil || !dead[q.holder.Key] || now.Sub(q.holder.Seen) <= b.ttl {
			continue
		}
		e.grants = append(e.grants, Grant{Queue: n, Key: q.holder.Key, TaskID: q.holder.TaskID, Reason: ReasonExpired})
		q.holder = nil
		e.changed = true
		b.promote(q, rk, &e)
	}
	b.mu.Unlock()
	b.run(e)
}

// Snapshot lists the queues in name order with waiters in grant order.
func (b *Book) Snapshot() []Queue {
	rk := b.lockRanked(2)
	defer b.mu.Unlock()
	out := make([]Queue, 0, len(b.names))
	for _, n := range b.names {
		q := b.queues[n]
		s := Queue{Name: n, Waiters: rk.sorted(q.waiters)}
		if q.holder != nil {
			h := *q.holder
			s.Holder = &h
		}
		out = append(out, s)
	}
	return out
}
