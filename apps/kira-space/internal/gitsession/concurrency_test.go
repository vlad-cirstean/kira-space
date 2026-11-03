package gitsession

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitclient"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitclient/porcelain"
)

// This file is the concurrency tier: every test here is designed to fail only for a concurrency
// reason (ordering, a lost release, a double-close, a race on shared state) and is run at
// `-race -count=10` by name (every test here begins TestConcurrent, the filter). Each of the first
// several reproduces one of the findings on the pre-fix tree — the exact pre-fix output is quoted
// in its own comment so a future reader can tell a regression test from a speculative one.

// TestConcurrent_OpenAfterCloseIsARelease is the deterministic ordering: Close() then Open() for
// the same connection must release rather than store the ref Open takes, and the caller still gets
// a correct summary back. On the pre-fix tree this leaves refs=1 forever (the probe: "after
// Close-then-Open: entry present=true refs=1 heldLen=1").
func TestConcurrent_OpenAfterCloseIsARelease(t *testing.T) {
	reg := newTestRegistry()
	reg.LingerFor = time.Hour
	c := NewConn("c1", "client-1", "label-1", func(string, any) {})

	c.Close()
	summary, err := c.Open(context.Background(), reg, "/usr/bin/git", "/repo")
	if err != nil {
		t.Fatalf("Open after Close: %v", err)
	}
	if summary.RepoID != "/repo" {
		t.Fatalf("Open after Close returned repoID %q, want /repo — the caller must still get a correct answer", summary.RepoID)
	}

	reg.mu.Lock()
	sl, ok := reg.entries["/repo"]
	var refs int
	if ok {
		refs = sl.refs
	}
	reg.mu.Unlock()
	if ok && refs != 0 {
		t.Fatalf("refs = %d after Open on an already-closed connection, want 0 or the entry gone (F1)", refs)
	}

	c.mu.Lock()
	held := len(c.held)
	c.mu.Unlock()
	if held != 0 {
		t.Fatalf("Conn.held has %d entries after Open on an already-closed connection, want 0", held)
	}
}

// TestConcurrent_OpenRacingCloseNeverLeaksAHold is the probe, promoted to a permanent
// regression test: 200 iterations of a real Open racing a real Close on the same connection. On
// the pre-fix tree this reproduced "leaked holds in 200 Open/Close races: 160" — an Open that wins
// the race against Close's map swap stores its hold into the map Close already replaced, and
// nothing ever releases it. After the fix every iteration ends at refcount zero.
func TestConcurrent_OpenRacingCloseNeverLeaksAHold(t *testing.T) {
	const n = 200
	reg := newTestRegistry()
	reg.LingerFor = time.Hour

	leaks := 0
	for i := 0; i < n; i++ {
		path := "/repo"
		c := NewConn(ConnID("c"), "client-1", "label-1", func(string, any) {})

		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			_, _ = c.Open(context.Background(), reg, "/usr/bin/git", path)
		}()
		go func() {
			defer wg.Done()
			c.Close()
		}()
		wg.Wait()

		// Whichever order they ran in, the connection ends up closed and must hold nothing.
		c.mu.Lock()
		held := len(c.held)
		c.mu.Unlock()
		if held != 0 {
			leaks++
		}

		reg.mu.Lock()
		sl, ok := reg.entries[path]
		refs := 0
		if ok {
			refs = sl.refs
		}
		reg.mu.Unlock()
		if refs != 0 {
			leaks++
			// Clean this iteration's leaked ref so it cannot pollute the next iteration's count.
			reg.mu.Lock()
			delete(reg.entries, path)
			reg.mu.Unlock()
		}
	}
	if leaks != 0 {
		t.Fatalf("leaked holds/refs in %d Open/Close races: %d (want 0) — F1", n, leaks)
	}
}

// TestConcurrent_SubscribeRacingTeardown is the probe: 200 iterations of a real Subscribe
// racing a real Registry.Close (which tears entries down regardless of refcount) — reachable
// whenever a repo.open/repo.close is still in flight when the app quits. On the pre-fix tree this
// panics with "assignment to entry in nil map" the moment Subscribe's own critical section runs
// after teardown has already nilled e.subs.
func TestConcurrent_SubscribeRacingTeardown(t *testing.T) {
	const n = 200
	for i := 0; i < n; i++ {
		reg := newTestRegistry()
		entry, _, err := reg.Acquire(context.Background(), "/usr/bin/git", "/repo")
		if err != nil {
			t.Fatalf("Acquire: %v", err)
		}

		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			unsub := entry.Subscribe(ConnID("c"), func(Event) {})
			unsub()
		}()
		go func() {
			defer wg.Done()
			reg.Close()
		}()
		wg.Wait()
	}
}

// TestConcurrent_UnsubscribeAfterTeardown is the probe: a Subscribe that completes BEFORE
// teardown, whose unsubscribe is then called AFTER teardown already closed every subscriber it
// found. On the pre-fix tree this panics with "close of closed channel" — teardown's own close and
// this unsubscribe's close both fire on the same subscriber.
func TestConcurrent_UnsubscribeAfterTeardown(t *testing.T) {
	const n = 200
	for i := 0; i < n; i++ {
		reg := newTestRegistry()
		entry, _, err := reg.Acquire(context.Background(), "/usr/bin/git", "/repo")
		if err != nil {
			t.Fatalf("Acquire: %v", err)
		}
		unsub := entry.Subscribe(ConnID("c"), func(Event) {})
		reg.Close() // tears the entry down immediately, closing the subscriber above itself.
		unsub()     // must be a safe no-op, not a double close.
	}
}

// TestConcurrent_SubscribeSameConnNeverOrphansTheWinner is the regression proof (probe: 8
// concurrent Opens x 200 trials, 27/200 ended with zero subscribers).
// Two (or more) concurrent Subscribe calls for the SAME conn id — conn.Open's own real race, two
// repo.open requests for one repo dispatched on their own goroutines, each calling
// entry.Subscribe(c.ID, ...) before either learns which one "wins" c.held — used to collide on
// subs' old ConnID key: the second call's subscriber silently overwrote the first's (orphaning its
// goroutine), and whichever call's own unsubscribe fired first then deleted-and-closed whatever
// currently occupied that slot — the OTHER call's subscriber — leaving the repo/conn pair with
// zero subscribers registered (no repo.changed delivery) until the next repo.open. On the pre-fix
// tree this fails intermittently: some trials end with 0 remaining subscribers instead of 1, or
// the surviving subscriber never receives the signal this test sends afterward.
func TestConcurrent_SubscribeSameConnNeverOrphansTheWinner(t *testing.T) {
	const trials = 200
	const concurrent = 8

	for trial := 0; trial < trials; trial++ {
		reg := newTestRegistry()
		entry, _, err := reg.Acquire(context.Background(), "/usr/bin/git", "/repo")
		if err != nil {
			t.Fatalf("trial %d: Acquire: %v", trial, err)
		}

		var delivered int32
		unsubs := make([]func(), concurrent)
		var wg sync.WaitGroup
		wg.Add(concurrent)
		for i := 0; i < concurrent; i++ {
			go func() {
				defer wg.Done()
				unsubs[i] = entry.Subscribe(ConnID("c"), func(Event) {
					atomic.AddInt32(&delivered, 1)
				})
			}()
		}
		wg.Wait()

		// conn.Open's own shape: exactly one caller "wins" the c.held race and keeps its
		// unsubscribe (stored on the connection, called only at conn teardown); every other
		// caller unsubscribes immediately, right here, on losing.
		for i := 1; i < concurrent; i++ {
			unsubs[i]()
		}

		entry.mu.Lock()
		remaining := len(entry.subs)
		entry.mu.Unlock()
		if remaining != 1 {
			t.Fatalf("trial %d: %d subscribers remain, want exactly 1 (the winner's)", trial, remaining)
		}

		entry.note(gitclient.SignalRefsChanged)
		deadline := time.Now().Add(2 * time.Second)
		for atomic.LoadInt32(&delivered) == 0 && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}
		if atomic.LoadInt32(&delivered) == 0 {
			t.Fatalf("trial %d: the winner's subscriber never received a signal", trial)
		}

		unsubs[0]()
	}
}

// TestConcurrent_CatFileAfterTeardownIsRefused is the probe: CatFile() after teardown must
// return nil (never a live, unmemoised session nothing will ever close) so its callers can refuse
// with ErrRepoTornDown instead of leaking a `cat-file --batch` pair. On the pre-fix tree this
// returns a fresh, live session every time.
func TestConcurrent_CatFileAfterTeardownIsRefused(t *testing.T) {
	reg := newTestRegistry()
	entry, _, err := reg.Acquire(context.Background(), "/usr/bin/git", "/repo")
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	reg.Close()

	if s := entry.CatFile(); s != nil {
		t.Fatal("CatFile() after teardown returned a live session, want nil (F3)")
	}
	// A second call must not construct anything either.
	if s := entry.CatFile(); s != nil {
		t.Fatal("a second CatFile() after teardown returned a live session, want nil")
	}
}

// TestConcurrent_AutoFetchArmsOnOpen is the probe: a repository opened while the auto-fetch
// interval reads as 0 gets no timer at construction (correct); the off→on direction is what the
// startAutoFetch-only-from-newRepoEntry wiring never handled — on the pre-fix tree no LATER Open,
// however many windows, ever arms one. A second Open (already armed) must arm no second timer, and
// an entry `disabled` by a real fetch failure must never be resurrected by an Open.
func TestConcurrent_AutoFetchArmsOnOpen(t *testing.T) {
	reg := newTestRegistry()
	reg.LingerFor = time.Hour
	minutes := 0
	reg.Settings = func() ([]string, int, string) { return nil, minutes, "" }

	c1 := NewConn("c1", "client-1", "label-1", func(string, any) {})
	if _, err := c1.Open(context.Background(), reg, "/usr/bin/git", "/repo"); err != nil {
		t.Fatalf("Open: %v", err)
	}

	reg.mu.Lock()
	entry := reg.entries["/repo"].entry
	reg.mu.Unlock()
	defer reg.Close()

	entry.autoFetch.mu.Lock()
	armedAtConstruction := entry.autoFetch.timer != nil
	entry.autoFetch.mu.Unlock()
	if armedAtConstruction {
		t.Fatal("timer armed at construction despite a 0 interval")
	}

	// The setting flips on; a SECOND connection's Open must arm the timer this repository never
	// got (F4 — the off->on direction newRepoEntry-only arming never handles).
	minutes = 5
	c2 := NewConn("c2", "client-2", "label-2", func(string, any) {})
	if _, err := c2.Open(context.Background(), reg, "/usr/bin/git", "/repo"); err != nil {
		t.Fatalf("Open (2nd conn): %v", err)
	}
	entry.autoFetch.mu.Lock()
	timer1 := entry.autoFetch.timer
	entry.autoFetch.mu.Unlock()
	if timer1 == nil {
		t.Fatal("no timer armed after Open with a non-zero interval (F4)")
	}

	// A THIRD Open must not replace the already-armed timer with a second one.
	c3 := NewConn("c3", "client-3", "label-3", func(string, any) {})
	if _, err := c3.Open(context.Background(), reg, "/usr/bin/git", "/repo"); err != nil {
		t.Fatalf("Open (3rd conn): %v", err)
	}
	entry.autoFetch.mu.Lock()
	timer2 := entry.autoFetch.timer
	entry.autoFetch.mu.Unlock()
	if timer1 != timer2 {
		t.Fatal("a second Open on an already-armed entry armed a NEW timer")
	}

	// A `disabled` entry (a real fetch failure already killed its timer, G7 D23) must never be
	// resurrected by a later Open.
	entry.disableAutoFetch()
	c4 := NewConn("c4", "client-4", "label-4", func(string, any) {})
	if _, err := c4.Open(context.Background(), reg, "/usr/bin/git", "/repo"); err != nil {
		t.Fatalf("Open (4th conn): %v", err)
	}
	entry.autoFetch.mu.Lock()
	timer3, disabled := entry.autoFetch.timer, entry.autoFetch.disabled
	entry.autoFetch.mu.Unlock()
	if timer3 != nil || !disabled {
		t.Fatal("Open resurrected a disabled auto-fetch entry")
	}
}

// TestConcurrent_RemoteOpSlotAdmitsExactlyOne is the pin: opSlot.claim is a
// mutex-guarded CAS, and the existing gitflow coverage already drives it with a
// genuinely in-flight op — what nothing exercises is N goroutines calling claim at the SAME
// instant. Already correct today; this locks it in under real concurrency rather than a sequential
// fake.
func TestConcurrent_RemoteOpSlotAdmitsExactlyOne(t *testing.T) {
	var slot opSlot
	const n = 16

	claimAll := func(kind string) int32 {
		var winners int32
		var wg sync.WaitGroup
		wg.Add(n)
		for i := 0; i < n; i++ {
			go func() {
				defer wg.Done()
				_, cancel := context.WithCancel(context.Background())
				if slot.claim(kind, cancel, false) {
					atomic.AddInt32(&winners, 1)
				} else {
					cancel()
				}
			}()
		}
		wg.Wait()
		return winners
	}

	if got := claimAll("fetch"); got != 1 {
		t.Fatalf("winners among %d simultaneous claims = %d, want exactly 1", n, got)
	}

	// The losers' own view: tryCancel is honest about killable state, never an error.
	if slot.tryCancel() {
		t.Fatal("tryCancel succeeded on a non-killable claimed slot")
	}
	slot.setKillable(true)
	if !slot.tryCancel() {
		t.Fatal("tryCancel failed on a killable, claimed slot")
	}
	slot.release()

	if got := claimAll("push"); got != 1 {
		t.Fatalf("winners among %d simultaneous claims after release = %d, want exactly 1", n, got)
	}
	slot.release()
}

// TestConcurrent_WalkPairIsolationUnderLoad is the pin under real concurrency, which its
// single-threaded coverage cannot give: paging the graph walk and the review walk on the same Conn
// simultaneously, 100 iterations each, must never let one reset the other's store or run its
// nextSeq backwards.
func TestConcurrent_WalkPairIsolationUnderLoad(t *testing.T) {
	skipWithoutGitWalk(t)
	repoDir := initWalkRepo(t, 8)
	shas := revListShas(t, repoDir)
	conn, _, repoID := newWalkTestConn(t, repoDir)
	defer conn.Close()

	graphWalk, err := conn.Walk(repoID, "git", porcelain.WalkSpec{Scope: "all"}, 0, nil)
	if err != nil {
		t.Fatalf("graph Walk: %v", err)
	}
	reviewSpec := porcelain.WalkSpec{Range: &porcelain.RangeSpec{Base: shas[0], Branch: "main"}}
	reviewWalk, err := conn.Walk(repoID, "git", reviewSpec, 0, nil)
	if err != nil {
		t.Fatalf("review Walk: %v", err)
	}

	const iterations = 100
	page := func(w *Walk) error {
		prevSeq := -1
		for i := 0; i < iterations; i++ {
			if _, err := w.ReadPage(context.Background(), 1); err != nil {
				return fmt.Errorf("ReadPage: %w", err)
			}
			w.mu.Lock()
			seq := w.nextSeq
			w.mu.Unlock()
			if seq < prevSeq {
				return fmt.Errorf("nextSeq went backwards: %d -> %d", prevSeq, seq)
			}
			prevSeq = seq
		}
		return nil
	}

	errCh := make(chan error, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); errCh <- page(graphWalk) }()
	go func() { defer wg.Done(); errCh <- page(reviewWalk) }()
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Error(err)
		}
	}

	gl, _, ge, err := graphWalk.Status(context.Background())
	if err != nil {
		t.Fatalf("graph Status: %v", err)
	}
	if gl != 8 || !ge {
		t.Fatalf("graph walk = {loaded:%d exhausted:%v}, want {8 true} -- disturbed by the review walk", gl, ge)
	}
	rl, _, re, err := reviewWalk.Status(context.Background())
	if err != nil {
		t.Fatalf("review Status: %v", err)
	}
	if rl != 7 || !re {
		t.Fatalf("review walk = {loaded:%d exhausted:%v}, want {7 true} -- disturbed by the graph walk", rl, re)
	}
}

// TestConcurrent_RestackAfterTeardownIsRefused: a slot claimed after teardown's
// forceCancel is uncancellable, so the claimer re-checks tornDown (as RunRemote does).
func TestConcurrent_RestackAfterTeardownIsRefused(t *testing.T) {
	reg := newTestRegistry()
	entry, _, err := reg.Acquire(context.Background(), "/usr/bin/git", "/repo")
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	reg.Close()

	if _, err := entry.RunRestack(context.Background(), nil, "main"); !errors.Is(err, ErrRepoTornDown) {
		t.Fatalf("RunRestack after teardown = %v, want ErrRepoTornDown", err)
	}
}
