package adapters

import (
	"context"
	"errors"
	"sync"
)

// ErrConnSetClosed is Get's own error once CloseAll has run (finding F3): a "not connected" state
// for whatever key was asked for, distinct from a real dial failure.
var ErrConnSetClosed = errors.New("adapters: connection set is closed")

// This file hoists the LRU pool with single-flight dial postgres/mysqlfamily/redis's own client.go
// each implemented verbatim (ported adapter to adapter): one entry per
// key, capped at Max, evicting the least-recently-used non-Primary entry to make room, with
// concurrent Get calls for the same not-yet-open key waiting on one shared dial rather than each
// starting (and leaking) their own.

// connSetDialInFlight is one in-progress Get dial for a key — done closes once the attempt
// finishes, success or failure. A waiter never reads this dial's own outcome directly: it
// re-runs Get's own loop once done closes, which re-checks the conns map (present if this dial
// succeeded) and otherwise falls through to dialing the key itself. That keeps a failed dial's
// error from needing to be threaded through every waiter; each one gets an equal chance to retry.
type connSetDialInFlight struct {
	done chan struct{}
}

// ConnSetOptions configures a ConnSet. Dial and Close are the only dialect-specific pieces —
// everything else (the LRU pool, the single-flight dial) is shared.
type ConnSetOptions[K comparable, C any] struct {
	// Dial opens a fresh entry for key. Called with no lock held — a real network round trip.
	Dial func(ctx context.Context, key K) (C, error)
	// Close releases an entry evicted by Get or returned by CloseAll. Called with no lock held.
	Close func(ctx context.Context, entry C)
	// Max is the most entries ConnSet keeps open at once (counting in-flight dials too — P21 round
	// 2 performance finding 4's own "related, minor" fix).
	Max int
	// Primary is the key Get's own eviction never selects as a victim.
	Primary K
}

// ConnSet is client.ts's ClientSet generalized over the key type (a database name, a db index, …)
// and the entry type each adapter's own dial produces — misleadingly-named "Pool" avoided on
// purpose: a real pool does not reliably tell a caller which physical connection ran its own
// query, which postgres's pg_cancel_backend (and friends) need to know.
type ConnSet[K comparable, C any] struct {
	opts ConnSetOptions[K, C]

	mu sync.Mutex
	// closed is set once by CloseAll (F3): Get refuses a new dial once true, and a dial already in
	// flight when CloseAll ran closes its own freshly-opened entry instead of storing it, rather
	// than leaking a connection CloseAll could not have known about.
	closed  bool
	conns   map[K]C
	lru     []K
	dialing map[K]*connSetDialInFlight
}

// NewConnSet constructs a ConnSet from opts.
func NewConnSet[K comparable, C any](opts ConnSetOptions[K, C]) *ConnSet[K, C] {
	return &ConnSet[K, C]{
		opts:    opts,
		conns:   make(map[K]C),
		dialing: make(map[K]*connSetDialInFlight),
	}
}

// Get returns key's entry, opening one via Dial if none exists yet and evicting the
// least-recently-used non-Primary entry first if the set is full.
//
// Two independent fixes over the single-dial version this
// replaced in every adapter that used it. (a) eviction used to close the victim — a network round
// trip, and (for postgres/mysqlfamily) the victim's own per-connection lock held for its entire
// in-flight op — while the set's own mu was still held, so opening one more entry while a long op
// ran on the LRU victim blocked *every* other entry (including a bare map lookup for the
// already-open primary) behind that op. detachLocked below only touches the map/lru under mu; the
// actual Close happens after mu is released. (b) dialing itself used to run with no lock held at
// all, so two concurrent Get calls for the same not-yet-open key both dialed, the second
// overwriting the first in conns — a leaked entry (and whatever live server-side resource it held)
// for the life of the app. The dialing map turns this into a single-flight: the first caller for a
// key dials while holding a placeholder; everyone else waits on that placeholder's own outcome
// instead.
func (s *ConnSet[K, C]) Get(ctx context.Context, key K) (C, error) {
	for {
		s.mu.Lock()
		if s.closed {
			s.mu.Unlock()
			var zero C
			return zero, ErrConnSetClosed
		}
		if existing, ok := s.conns[key]; ok {
			s.touchLocked(key)
			s.mu.Unlock()
			return existing, nil
		}
		if inFlight, ok := s.dialing[key]; ok {
			s.mu.Unlock()
			select {
			case <-inFlight.done:
				// Re-check from the top: the dial that just finished may have been this key's
				// (conns now has it, or it failed and this caller should try dialing itself) or —
				// in principle — a still-different one if keys were reused mid-wait, which cannot
				// happen here since a key is only ever removed from dialing once, by its own
				// dialer.
				continue
			case <-ctx.Done():
				var zero C
				return zero, ctx.Err()
			}
		}
		// This goroutine is now the one dialing key — every concurrent caller for the same key takes
		// the branch above instead, until waiter.done closes.
		waiter := &connSetDialInFlight{done: make(chan struct{})}
		s.dialing[key] = waiter
		var victim C
		var hasVictim bool
		// Counting in-flight dials against Max too closes the "related, minor" gap the doc comment
		// above names: without it, N concurrent first-time opens could all pass this check before any
		// of them finished dialing, transiently exceeding Max.
		if len(s.conns)+len(s.dialing) > s.opts.Max {
			victim, hasVictim = s.detachLRULocked()
		}
		s.mu.Unlock()

		if hasVictim {
			s.opts.Close(ctx, victim)
		}

		entry, err := s.opts.Dial(ctx, key)

		s.mu.Lock()
		delete(s.dialing, key)
		closedNow := s.closed
		if err == nil && !closedNow {
			s.conns[key] = entry
			s.touchLocked(key)
		}
		s.mu.Unlock()

		close(waiter.done)

		if err == nil && closedNow {
			// CloseAll ran while this dial was in flight — CloseAll's own snapshot could not have
			// included this entry (it was still in s.dialing, not s.conns, at that instant), so
			// nothing else will ever close it. Close it now rather than leak it, and report the
			// same "not connected" state Get itself would have returned had this dial not raced in.
			s.opts.Close(ctx, entry)
			var zero C
			return zero, ErrConnSetClosed
		}
		return entry, err
	}
}

// Current returns key's currently-live entry without dialing — ok is false when key has no open
// entry right now (never opened, evicted, or the set is closed). Acquire's own caller re-checks its
// already-locked entry against this after locking (F3): an LRU eviction's Close can run
// concurrently with that lock attempt (both contend for the same entry-level lock), so the entry Get
// handed back may no longer be the set's own live one for key by the time the lock actually succeeds
// — retrying from Get instead of trusting a possibly-already-closed entry is the caller's own job,
// this only supplies the up-to-date fact to check against.
func (s *ConnSet[K, C]) Current(key K) (entry C, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok = s.conns[key]
	return entry, ok
}

func (s *ConnSet[K, C]) touchLocked(key K) {
	for i, k := range s.lru {
		if k == key {
			s.lru = append(s.lru[:i], s.lru[i+1:]...)
			break
		}
	}
	s.lru = append(s.lru, key)
}

// detachLRULocked picks the least-recently-used non-Primary entry to make room, removes it from the
// map/lru under mu (the only part of eviction that needs the shared lock), and returns it for the
// caller to Close *after* releasing mu. hasVictim is false when every open entry is Primary (never
// evicted).
func (s *ConnSet[K, C]) detachLRULocked() (victim C, hasVictim bool) {
	var victimKey K
	for _, k := range s.lru {
		if k != s.opts.Primary {
			victimKey = k
			hasVictim = true
			break
		}
	}
	if !hasVictim {
		return victim, false
	}
	victim = s.conns[victimKey]
	delete(s.conns, victimKey)
	for i, k := range s.lru {
		if k == victimKey {
			s.lru = append(s.lru[:i], s.lru[i+1:]...)
			break
		}
	}
	return victim, true
}

// Drop removes key's entry when it is still entry and closes it — for an entry found unusable (a
// dead connection) rather than evicted. A no-op when key already maps to something else.
func (s *ConnSet[K, C]) Drop(ctx context.Context, key K, entry C) {
	s.mu.Lock()
	current, ok := s.conns[key]
	if !ok || any(current) != any(entry) {
		s.mu.Unlock()
		return
	}
	delete(s.conns, key)
	for i, k := range s.lru {
		if k == key {
			s.lru = append(s.lru[:i], s.lru[i+1:]...)
			break
		}
	}
	s.mu.Unlock()
	s.opts.Close(ctx, current)
}

// CloseAll closes every open entry, releasing mu before any of them (P2 R2 — see Get's own eviction
// comment: the same reasoning applies to shutdown, not just eviction).
func (s *ConnSet[K, C]) CloseAll(ctx context.Context) {
	s.mu.Lock()
	s.closed = true
	all := make([]C, 0, len(s.conns))
	for _, e := range s.conns {
		all = append(all, e)
	}
	s.conns = make(map[K]C)
	s.lru = nil
	s.mu.Unlock()

	for _, e := range all {
		s.opts.Close(ctx, e)
	}
}
