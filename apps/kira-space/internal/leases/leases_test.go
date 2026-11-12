package leases

import (
	"fmt"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type clock struct{ t atomic.Int64 }

func (c *clock) now() time.Time          { return time.Unix(0, c.t.Load()) }
func (c *clock) advance(d time.Duration) { c.t.Add(int64(d)) }

type rec struct {
	mu     sync.Mutex
	grants []Grant
}

func (r *rec) add(g Grant) { r.mu.Lock(); r.grants = append(r.grants, g); r.mu.Unlock() }
func (r *rec) take() []Grant {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := r.grants
	r.grants = nil
	return out
}

func newBook(t *testing.T, order *[]string, ck *clock, r *rec) *Book {
	t.Helper()
	b := New(Options{
		Now:     ck.now,
		TTL:     time.Minute,
		Order:   func() []string { return slices.Clone(*order) },
		OnGrant: r.add,
	})
	if err := b.SetQueues([]string{"push", "port"}); err != nil {
		t.Fatal(err)
	}
	return b
}

func waiterKeys(b *Book, queue string) []Key {
	for _, q := range b.Snapshot() {
		if q.Name == queue {
			var out []Key
			for _, w := range q.Waiters {
				out = append(out, w.Key)
			}
			return out
		}
	}
	return nil
}

func holderKey(b *Book, queue string) Key {
	for _, q := range b.Snapshot() {
		if q.Name == queue && q.Holder != nil {
			return q.Holder.Key
		}
	}
	return ""
}

func mustAcquire(t *testing.T, b *Book, key Key, task, queue string) Status {
	t.Helper()
	st, err := b.Acquire(key, task, queue)
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func TestRankByListIndexThenArrival(t *testing.T) {
	order := []string{"A", "B", "C"}
	b := newBook(t, &order, &clock{}, &rec{})
	mustAcquire(t, b, "hold", "X", "push")
	// Arrival C, unlisted U1, B, A, unlisted U2: rank A, B, C, then unlisted by arrival.
	for _, w := range [][2]string{{"c", "C"}, {"u1", "U1"}, {"b", "B"}, {"a", "A"}, {"u2", "U2"}} {
		mustAcquire(t, b, Key(w[0]), w[1], "push")
	}
	if got, want := waiterKeys(b, "push"), []Key{"a", "b", "c", "u1", "u2"}; !slices.Equal(got, want) {
		t.Fatalf("waiters = %v, want %v", got, want)
	}
	if st := mustAcquire(t, b, "c", "C", "push"); st.State != StateQueued || st.Position != 3 {
		t.Fatalf("re-call = %+v, want queued at 3", st)
	}
}

func TestReRankBetweenGrantsAndNoPreemption(t *testing.T) {
	order := []string{"A", "B", "C"}
	r := &rec{}
	b := newBook(t, &order, &clock{}, r)
	mustAcquire(t, b, "h", "C", "push")
	mustAcquire(t, b, "a", "A", "push")
	mustAcquire(t, b, "b", "B", "push")
	// A higher-ranked arrival never preempts the holder.
	if got := holderKey(b, "push"); got != "h" {
		t.Fatalf("holder = %q", got)
	}
	// The user reorders the list before the lease frees: B now outranks A.
	order = []string{"B", "A", "C"}
	b.Release("h", "push")
	if got := holderKey(b, "push"); got != "b" {
		t.Fatalf("holder = %q, want b after re-rank", got)
	}
	gs := r.take()
	if len(gs) != 1 || gs[0].Key != "b" || gs[0].Reason != ReasonGranted {
		t.Fatalf("grants = %+v", gs)
	}
}

func TestSessionReleaseFreesSeveralQueues(t *testing.T) {
	order := []string{"A", "B"}
	r := &rec{}
	b := newBook(t, &order, &clock{}, r)
	mustAcquire(t, b, "s1", "A", "push")
	mustAcquire(t, b, "s1", "A", "port")
	mustAcquire(t, b, "s2", "B", "push")
	mustAcquire(t, b, "s3", "B", "port")
	mustAcquire(t, b, "s1", "A", "port") // holder re-call
	r.take()
	b.ReleaseSession("s1")
	if holderKey(b, "push") != "s2" || holderKey(b, "port") != "s3" {
		t.Fatalf("holders = %q %q", holderKey(b, "push"), holderKey(b, "port"))
	}
	if gs := r.take(); len(gs) != 2 {
		t.Fatalf("grants = %+v, want one per queue", gs)
	}
	// A waiter that ends leaves the line without a grant.
	mustAcquire(t, b, "s4", "A", "push")
	b.ReleaseSession("s4")
	if got := waiterKeys(b, "push"); len(got) != 0 {
		t.Fatalf("waiters = %v", got)
	}
	if gs := r.take(); len(gs) != 0 {
		t.Fatalf("grants = %+v", gs)
	}
}

func TestSweepHonoursTTLAndAlive(t *testing.T) {
	order := []string{"A", "B"}
	ck := &clock{}
	r := &rec{}
	b := newBook(t, &order, ck, r)
	mustAcquire(t, b, "s1", "A", "push")
	mustAcquire(t, b, "s2", "B", "push")
	r.take()

	ck.advance(30 * time.Second)
	b.Touch("s1")
	ck.advance(45 * time.Second) // 75s since grant, 45s since the touch
	b.Sweep(func(Key) bool { return false })
	if holderKey(b, "push") != "s1" {
		t.Fatal("a touched holder expired")
	}

	ck.advance(30 * time.Second) // 75s silent
	b.Sweep(func(k Key) bool { return k == "s1" })
	if holderKey(b, "push") != "s1" {
		t.Fatal("a busy holder expired")
	}

	b.Sweep(func(Key) bool { return false })
	if holderKey(b, "push") != "s2" {
		t.Fatalf("holder = %q, want s2 after expiry", holderKey(b, "push"))
	}
	gs := r.take()
	if len(gs) != 2 || gs[0].Reason != ReasonExpired || gs[0].Key != "s1" || gs[1].Reason != ReasonGranted || gs[1].Key != "s2" {
		t.Fatalf("grants = %+v", gs)
	}
}

func TestForceReleaseRevokesAndPromotes(t *testing.T) {
	order := []string{"A", "B"}
	r := &rec{}
	b := newBook(t, &order, &clock{}, r)
	mustAcquire(t, b, "s1", "A", "push")
	mustAcquire(t, b, "s2", "B", "push")
	r.take()
	if _, err := b.RequestRelease("push"); err != nil {
		t.Fatal(err)
	}
	if q := b.Snapshot()[0]; !q.Holder.ReleaseAsked {
		t.Fatal("ReleaseAsked not set")
	}
	old, err := b.ForceRelease("push")
	if err != nil || old != "s1" {
		t.Fatalf("ForceRelease = %q, %v", old, err)
	}
	if q := b.Snapshot()[0]; q.Holder == nil || q.Holder.Key != "s2" || q.Holder.ReleaseAsked {
		t.Fatalf("holder = %+v, want fresh s2", q.Holder)
	}
	gs := r.take()
	if len(gs) != 2 || gs[0].Reason != ReasonRevoked || gs[1].Reason != ReasonGranted {
		t.Fatalf("grants = %+v", gs)
	}
}

func TestSetQueuesDropsRemovedQueue(t *testing.T) {
	order := []string{"A"}
	r := &rec{}
	b := newBook(t, &order, &clock{}, r)
	mustAcquire(t, b, "s1", "A", "port")
	r.take()
	if err := b.SetQueues([]string{"push"}); err != nil {
		t.Fatal(err)
	}
	gs := r.take()
	if len(gs) != 1 || gs[0].Reason != ReasonRevoked || gs[0].Queue != "port" {
		t.Fatalf("grants = %+v", gs)
	}
	if _, err := b.Acquire("s1", "A", "port"); err == nil {
		t.Fatal("acquire on a removed queue succeeded")
	}
	for _, bad := range [][]string{{""}, {"a", "a"}, {"bad/name"}, {string(make([]byte, 65))}} {
		if err := b.SetQueues(bad); err == nil {
			t.Fatalf("SetQueues(%q) accepted", bad)
		}
	}
}

// Many sessions race Acquire, Release and ForceRelease. A checker watches every snapshot: a key
// appears once per queue, and the holder is never also a waiter. Once every session ends, no
// queue keeps a holder or waiter.
func TestConcurrentRace(t *testing.T) {
	const sessions, rounds = 24, 200
	tasks := make([]string, sessions)
	for i := range tasks {
		tasks[i] = fmt.Sprintf("t%d", i)
	}
	var live atomic.Int64
	b := New(Options{Order: func() []string { return tasks }})
	if err := b.SetQueues([]string{"q1", "q2"}); err != nil {
		t.Fatal(err)
	}
	stop := make(chan struct{})
	var cw sync.WaitGroup
	cw.Add(1)
	go func() {
		defer cw.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			for _, q := range b.Snapshot() {
				seen := map[Key]bool{}
				if q.Holder != nil {
					seen[q.Holder.Key] = true
				}
				for _, w := range q.Waiters {
					if seen[w.Key] {
						t.Errorf("queue %s lists %s twice", q.Name, w.Key)
					}
					seen[w.Key] = true
				}
			}
		}
	}()
	var wg sync.WaitGroup
	for i := range sessions {
		wg.Add(1)
		go func() {
			defer wg.Done()
			key := Key(fmt.Sprintf("s%d", i))
			for r := range rounds {
				q := []string{"q1", "q2"}[(i+r)%2]
				if _, err := b.Acquire(key, tasks[i], q); err != nil {
					t.Error(err)
					return
				}
				live.Add(1)
				switch r % 4 {
				case 0:
					b.Release(key, q)
				case 1:
					_, _ = b.ForceRelease(q)
				case 2:
					b.Release(key, "")
				default:
					_, _ = b.RequestRelease(q)
				}
			}
			b.ReleaseSession(key)
		}()
	}
	wg.Wait()
	close(stop)
	cw.Wait()
	for _, q := range b.Snapshot() {
		if q.Holder != nil || len(q.Waiters) != 0 {
			t.Fatalf("queue %s not empty after every session ended: %+v", q.Name, q)
		}
	}
	if live.Load() != sessions*rounds {
		t.Fatalf("acquires = %d", live.Load())
	}
}
