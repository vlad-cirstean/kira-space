package keptvars

import (
	"container/heap"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"sort"
	"sync"
	"time"
)

// Reason is why an entry left the store.
type Reason string

const (
	ReasonExpired  Reason = "expired"
	ReasonCleared  Reason = "cleared"
	ReasonReplaced Reason = "replaced"
	ReasonEnded    Reason = "ended"
	ReasonQuit     Reason = "quit"
)

// Removal is what a watcher learns.
type Removal struct {
	ID, Name string
	Reason   Reason
}

// Options configures New.
type Options struct {
	// Now is the clock; nil means time.Now.
	Now func() time.Time
	// Retention is read per Put; nil or a non-positive result means DefaultTTL.
	Retention func() time.Duration
	// OnChange runs after every mutation batch, outside the lock, with a monotonic revision.
	OnChange func(revision uint64)
}

// Put is one entry to store. Replacing a name frees the old entry first.
type Put struct {
	Scope, Name string
	Value       json.RawMessage
	Secret      bool
	Source      Source
	// Origin binds a SourceUser value to the origin it was supplied for.
	Origin string
	// TTL of zero means Options.Retention; it never exceeds MaxTTL.
	TTL time.Duration
	// NotAfter, when set, caps the expiry: a value derived from another never outlives it.
	NotAfter time.Time
}

type item struct {
	meta  Meta
	value []byte
	index int // position in the expiry heap
}

// goneMemory bounds the removal reasons kept for ids that watchers ask about late.
const goneMemory = 1024

// Store is the in-memory kept variable store. Safe for concurrent use.
type Store struct {
	opt Options

	mu       sync.Mutex
	byID     map[string]*item
	byScope  map[string]map[string]*item
	bytes    int
	rev      uint64
	expiry   expiryHeap
	watchers map[string][]*watcher
	gone     map[string]Reason
	goneOrd  []string
	closed   bool

	wake chan struct{}
	stop chan struct{}
	done chan struct{}
	once sync.Once
}

// New returns a store and starts its sweeper.
func New(o Options) *Store {
	if o.Now == nil {
		o.Now = time.Now
	}
	s := &Store{
		opt: o, byID: map[string]*item{}, byScope: map[string]map[string]*item{},
		watchers: map[string][]*watcher{}, gone: map[string]Reason{},
		wake: make(chan struct{}, 1), stop: make(chan struct{}), done: make(chan struct{}),
	}
	go s.sweeper()
	return s
}

func (s *Store) now() time.Time { return s.opt.Now() }

func newID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return "kv_" + hex.EncodeToString(b[:])
}

// batch collects what a mutation removed, to report once the lock is released.
type batch struct {
	fires []fire
	dirty bool
}

type fire struct {
	w *watcher
	r Removal
}

// removeLocked drops it and queues its watchers.
func (s *Store) removeLocked(it *item, why Reason, b *batch) {
	delete(s.byID, it.meta.ID)
	if scope := s.byScope[it.meta.Scope]; scope != nil {
		delete(scope, it.meta.Name)
		if len(scope) == 0 {
			delete(s.byScope, it.meta.Scope)
		}
	}
	s.bytes -= it.meta.Bytes
	if it.index >= 0 {
		heap.Remove(&s.expiry, it.index)
	}
	clear(it.value)
	s.remember(it.meta.ID, why)
	rm := Removal{ID: it.meta.ID, Name: it.meta.Name, Reason: why}
	for _, w := range s.watchers[it.meta.ID] {
		b.fires = append(b.fires, fire{w: w, r: rm})
	}
	delete(s.watchers, it.meta.ID)
	b.dirty = true
}

func (s *Store) remember(id string, why Reason) {
	s.gone[id] = why
	s.goneOrd = append(s.goneOrd, id)
	if len(s.goneOrd) > goneMemory {
		delete(s.gone, s.goneOrd[0])
		s.goneOrd = s.goneOrd[1:]
	}
}

// dueLocked removes every entry whose expiry has passed.
func (s *Store) dueLocked(b *batch) {
	now := s.now()
	for len(s.expiry) > 0 && !now.Before(s.expiry[0].meta.ExpiresAt) {
		s.removeLocked(s.expiry[0], ReasonExpired, b)
	}
}

// finish bumps the revision once for a dirty batch, then runs watchers and OnChange outside the lock.
// It must be called after Unlock.
func (s *Store) finish(b *batch, rev uint64) {
	for _, f := range b.fires {
		f.w.fire(f.r)
	}
	if b.dirty && s.opt.OnChange != nil {
		s.opt.OnChange(rev)
	}
}

// commitLocked bumps the revision for a dirty batch and returns it.
func (s *Store) commitLocked(b *batch) uint64 {
	if b.dirty {
		s.rev++
	}
	return s.rev
}

func (s *Store) ttlFor(p Put) time.Duration {
	ttl := p.TTL
	if ttl <= 0 && s.opt.Retention != nil {
		ttl = s.opt.Retention()
	}
	if ttl <= 0 {
		ttl = DefaultTTL
	}
	return min(ttl, MaxTTL)
}

// Retention is how long a value Put now would be kept: the setting, capped at MaxTTL.
func (s *Store) Retention() time.Duration { return s.ttlFor(Put{}) }

// Put stores a value under a name in a scope, replacing an entry of the same name from the same kind
// of source. It refuses at a cap; it never evicts.
func (s *Store) Put(p Put) (Meta, error) {
	if p.Scope == "" {
		return Meta{}, errBadScope
	}
	if err := ValidName(p.Name); err != nil {
		return Meta{}, err
	}
	value, typ, items, err := analyze(p.Value)
	if err != nil {
		return Meta{}, err
	}
	ttl := s.ttlFor(p)
	var b batch
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return Meta{}, ErrClosed
	}
	s.dueLocked(&b)
	old := s.byScope[p.Scope][p.Name]
	if old != nil && old.meta.Source.Kind != p.Source.Kind {
		rev := s.commitLocked(&b)
		s.mu.Unlock()
		s.finish(&b, rev)
		return Meta{}, ErrNameTaken
	}
	if err := s.checkCapsLocked(p.Scope, old, len(value)); err != nil {
		rev := s.commitLocked(&b)
		s.mu.Unlock()
		s.finish(&b, rev)
		return Meta{}, err
	}
	if old != nil {
		s.removeLocked(old, ReasonReplaced, &b)
	}
	now := s.now()
	expires := now.Add(ttl)
	if !p.NotAfter.IsZero() && p.NotAfter.Before(expires) {
		expires = p.NotAfter
	}
	it := &item{value: value, index: -1, meta: Meta{
		ID: newID(), Scope: p.Scope, Name: p.Name, Type: typ, Bytes: len(value), Items: items,
		Secret: p.Secret, Source: p.Source, Origin: p.Origin, CreatedAt: now, ExpiresAt: expires,
	}}
	s.byID[it.meta.ID] = it
	if s.byScope[p.Scope] == nil {
		s.byScope[p.Scope] = map[string]*item{}
	}
	s.byScope[p.Scope][p.Name] = it
	s.bytes += it.meta.Bytes
	heap.Push(&s.expiry, it)
	b.dirty = true
	rev := s.commitLocked(&b)
	meta := it.meta
	s.mu.Unlock()
	s.poke()
	s.finish(&b, rev)
	return meta, nil
}

// checkCapsLocked applies the caps as if old (the entry being replaced) were already gone.
func (s *Store) checkCapsLocked(scope string, old *item, size int) error {
	count, scopeCount, bytes := len(s.byID), len(s.byScope[scope]), s.bytes
	if old != nil {
		count, scopeCount, bytes = count-1, scopeCount-1, bytes-old.meta.Bytes
	}
	switch {
	case scopeCount >= MaxScopeEntries:
		return ErrScopeFull
	case count >= MaxStoreEntries, bytes+size > MaxStoreBytes:
		return ErrStoreFull
	}
	return nil
}

func (it *item) entry() Entry {
	return Entry{Meta: it.meta, Value: append(json.RawMessage(nil), it.value...)}
}

func (s *Store) live(it *item) bool { return it != nil && s.now().Before(it.meta.ExpiresAt) }

// Get returns the live entry of a name in a scope. An entry at or past its expiry is absent even before
// the sweeper removes it.
func (s *Store) Get(scope, name string) (Entry, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	it := s.byScope[scope][name]
	if !s.live(it) {
		return Entry{}, false
	}
	return it.entry(), true
}

// ByID returns the live entry with that id.
func (s *Store) ByID(id string) (Entry, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	it := s.byID[id]
	if !s.live(it) {
		return Entry{}, false
	}
	return it.entry(), true
}

// Secrets returns copies of the live secret entries of every scope, for building response masks.
func (s *Store) Secrets() []Entry {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []Entry
	for _, it := range s.byID {
		if it.meta.Secret && s.live(it) {
			out = append(out, it.entry())
		}
	}
	return out
}

// List returns the live entries of a scope, newest first; scope "" lists every scope.
func (s *Store) List(scope string) []Meta {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []Meta{}
	for _, it := range s.byID {
		if s.live(it) && (scope == "" || it.meta.Scope == scope) {
			out = append(out, it.meta)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.After(out[j].CreatedAt)
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// clearWhere removes every entry matching pick, with one revision for the batch.
func (s *Store) clearWhere(why Reason, pick func(*item) bool) int {
	var b batch
	n := 0
	s.mu.Lock()
	s.dueLocked(&b)
	var hit []*item
	for _, it := range s.byID {
		if pick(it) {
			hit = append(hit, it)
		}
	}
	for _, it := range hit {
		s.removeLocked(it, why, &b)
		n++
	}
	rev := s.commitLocked(&b)
	s.mu.Unlock()
	s.finish(&b, rev)
	return n
}

// Clear removes the entries with those ids and reports how many it removed.
func (s *Store) Clear(ids ...string) int {
	want := make(map[string]bool, len(ids))
	for _, id := range ids {
		want[id] = true
	}
	return s.clearWhere(ReasonCleared, func(it *item) bool { return want[it.meta.ID] })
}

// ClearScope removes every entry of a scope: its session ended.
func (s *Store) ClearScope(scope string) int {
	return s.clearWhere(ReasonEnded, func(it *item) bool { return it.meta.Scope == scope })
}

// ClearAll removes every entry.
func (s *Store) ClearAll() int {
	return s.clearWhere(ReasonCleared, func(*item) bool { return true })
}

// Close stops the sweeper and removes every entry (reason quit). Later Puts fail. Idempotent.
func (s *Store) Close() {
	s.once.Do(func() {
		close(s.stop)
		<-s.done
		s.mu.Lock()
		s.closed = true
		s.mu.Unlock()
		s.clearWhere(ReasonQuit, func(*item) bool { return true })
	})
}
