package redis

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// White-box (package redis) coverage for newDbConnectionSet's own wiring onto adapters.ConnSet:
// the LRU/single-flight behaviour itself is covered once, generically, by
// internal/adapters's own connset_test.go — this confirms get (via the real Dial/Close
// closures newDbConnectionSet builds) still serializes concurrent dials for the same db index with
// no real Redis server needed.
func TestGet_SingleFlightPerDbIndex(t *testing.T) {
	old := redisPing
	t.Cleanup(func() { redisPing = old })

	var inFlight, maxInFlight, totalDials int32
	redisPing = func(_ context.Context, _ *goredis.Client) error {
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
		return context.DeadlineExceeded
	}

	s := newDbConnectionSet(connectFields{host: "127.0.0.1", port: 6379}, 0, func(string, string) {})
	const callers = 12
	var wg sync.WaitGroup
	wg.Add(callers)
	for i := 0; i < callers; i++ {
		go func() {
			defer wg.Done()
			_, _, _ = s.get(context.Background(), 3)
		}()
	}
	wg.Wait()

	if got := atomic.LoadInt32(&maxInFlight); got > 1 {
		t.Errorf("max concurrent dials for the same db index = %d, want at most 1", got)
	}
	if atomic.LoadInt32(&totalDials) == 0 {
		t.Fatal("redisPing was never called at all")
	}
}

// An LRU eviction must not close a client another op still holds (F11): the close waits for the
// last release, and a Disconnect (closing) forces it.
func TestClientEntry_EvictionWaitsForLastRelease(t *testing.T) {
	closed := func(e *clientEntry) bool {
		return errors.Is(e.client.Ping(context.Background()).Err(), goredis.ErrClosed)
	}
	newEntry := func() *clientEntry {
		return &clientEntry{client: goredis.NewClient(&goredis.Options{Addr: "127.0.0.1:1"})}
	}

	e := newEntry()
	if !e.acquire() || !e.acquire() {
		t.Fatal("acquire on a live entry failed")
	}
	e.retire(false)
	if closed(e) {
		t.Fatal("closed while two ops still hold it")
	}
	e.release()
	if closed(e) {
		t.Fatal("closed with one op still holding it")
	}
	e.release()
	if !closed(e) {
		t.Fatal("not closed after the last release")
	}
	if e.acquire() {
		t.Fatal("acquire succeeded on a retired entry")
	}

	f := newEntry()
	f.acquire()
	f.retire(true)
	if !closed(f) {
		t.Fatal("forced retire (Disconnect) left the client open")
	}
}
