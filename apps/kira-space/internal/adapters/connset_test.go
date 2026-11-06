package adapters

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// White-box coverage for ConnSet.Get/detachLRULocked — the concurrency behaviour under test now
// lives here, not per adapter; each adapter's own Dial/Close wiring keeps whatever adapter-specific
// locking test it still needs (e.g. mysqlfamily's own per-entry Acquire serialization).

type fakeEntry struct{ id int }

func connSetForTest(dial func(ctx context.Context, key string) (*fakeEntry, error), max int) *ConnSet[string, *fakeEntry] {
	return NewConnSet(ConnSetOptions[string, *fakeEntry]{
		Dial:    dial,
		Close:   func(context.Context, *fakeEntry) {},
		Max:     max,
		Primary: "primary",
	})
}

// TestConnSetDetachLRULocked_NeverEvictsPrimary confirms the primary key is skipped even when it is
// the least-recently-used entry.
func TestConnSetDetachLRULocked_NeverEvictsPrimary(t *testing.T) {
	s := connSetForTest(nil, 8)
	primary := &fakeEntry{id: 0}
	s.conns["primary"] = primary
	s.lru = append(s.lru, "primary")
	sidedb := &fakeEntry{id: 1}
	s.conns["sidedb"] = sidedb
	s.lru = append(s.lru, "sidedb")

	got, ok := s.detachLRULocked()
	if !ok || got != sidedb {
		t.Fatalf("detachLRULocked() = %+v, %v, want the non-primary entry %+v, true", got, ok, sidedb)
	}
	if _, stillPresent := s.conns["primary"]; !stillPresent {
		t.Error("the primary entry was evicted — it must never be")
	}
}

// TestConnSetDetachLRULocked_NoOpWhenOnlyPrimaryIsOpen confirms the "everything open is primary"
// no-op path returns hasVictim=false rather than picking the primary anyway.
func TestConnSetDetachLRULocked_NoOpWhenOnlyPrimaryIsOpen(t *testing.T) {
	s := connSetForTest(nil, 8)
	s.conns["primary"] = &fakeEntry{}
	s.lru = append(s.lru, "primary")

	if _, ok := s.detachLRULocked(); ok {
		t.Fatal("detachLRULocked() reported a victim with only the primary entry open")
	}
}

// TestConnSetDetachLRULocked_NeverTouchesVictimClose is a regression: eviction must
// only ever touch the map/lru under mu, never call Close (a potential network round trip) while mu
// is held — this locks a channel Close would need to reach before calling detachLRULocked directly
// and asserts it still returns immediately.
func TestConnSetDetachLRULocked_NeverTouchesVictimClose(t *testing.T) {
	s := connSetForTest(nil, 8)
	for i := 0; i < 8; i++ {
		key := string(rune('a' + i))
		s.conns[key] = &fakeEntry{id: i}
		s.lru = append(s.lru, key)
	}
	victimKey := s.lru[0]
	victim := s.conns[victimKey]

	done := make(chan *fakeEntry, 1)
	go func() {
		got, _ := s.detachLRULocked()
		done <- got
	}()

	select {
	case got := <-done:
		if got != victim {
			t.Fatalf("detachLRULocked returned %+v, want the LRU victim %+v", got, victim)
		}
		if _, stillPresent := s.conns[victimKey]; stillPresent {
			t.Errorf("victim key %q is still in conns after detachLRULocked", victimKey)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("detachLRULocked blocked — it must only touch the map/lru, never call Close")
	}
}

// TestConnSetGet_SingleFlightPerKey is a regression test: Get used to dial with no lock
// held at all, so two concurrent Get calls for the same not-yet-open key both dialed, the second
// overwriting the first in conns — a leaked entry (and whatever live resource it held) for the life
// of the app. This fires many concurrent Get calls for the *same* key against a fake Dial that
// counts how many dials are ever in flight at once (and sleeps briefly so a real race would have a
// real window to manifest) — the max observed concurrency for one key must be exactly 1.
func TestConnSetGet_SingleFlightPerKey(t *testing.T) {
	var inFlight, maxInFlight, totalDials int32
	dial := func(context.Context, string) (*fakeEntry, error) {
		n := atomic.AddInt32(&inFlight, 1)
		atomic.AddInt32(&totalDials, 1)
		for {
			m := atomic.LoadInt32(&maxInFlight)
			if n <= m || atomic.CompareAndSwapInt32(&maxInFlight, m, n) {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
		atomic.AddInt32(&inFlight, -1)
		return nil, context.DeadlineExceeded
	}

	s := connSetForTest(dial, 8)
	const callers = 12
	var wg sync.WaitGroup
	wg.Add(callers)
	for i := 0; i < callers; i++ {
		go func() {
			defer wg.Done()
			_, _ = s.Get(context.Background(), "samekey")
		}()
	}
	wg.Wait()

	if got := atomic.LoadInt32(&maxInFlight); got > 1 {
		t.Errorf("max concurrent dials for the same key = %d, want at most 1 — a real dial race would have leaked one entry per overlap", got)
	}
	if atomic.LoadInt32(&totalDials) == 0 {
		t.Fatal("Dial was never called at all")
	}
}

// TestConnSetGet_DifferentKeysDialConcurrently confirms the single-flight above is per-key, not an
// accidental whole-ConnSet serialization: two different not-yet-open keys must still be able to
// dial at the same time.
func TestConnSetGet_DifferentKeysDialConcurrently(t *testing.T) {
	release := make(chan struct{})
	started := make(chan struct{}, 2)
	dial := func(context.Context, string) (*fakeEntry, error) {
		started <- struct{}{}
		<-release
		return nil, context.DeadlineExceeded
	}

	s := connSetForTest(dial, 8)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _ = s.Get(context.Background(), "keyone")
	}()
	go func() {
		defer wg.Done()
		_, _ = s.Get(context.Background(), "keytwo")
	}()

	for i := 0; i < 2; i++ {
		select {
		case <-started:
		case <-time.After(1 * time.Second):
			t.Fatal("both different-key dials never started concurrently — one key's dial appears to block the other's")
		}
	}
	close(release)
	wg.Wait()
}
