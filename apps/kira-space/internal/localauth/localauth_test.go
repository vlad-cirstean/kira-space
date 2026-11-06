package localauth_test

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/localauth"
)

// fakeClock gives a test full control over "now" without a real sleep.
type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time          { return c.t }
func (c *fakeClock) advance(d time.Duration) { c.t = c.t.Add(d) }

// TestAuthorizeDecisionTable pins the whole reveal gate: a small state machine over (OS
// availability, the grace deadline, the clock, the renderer's confirmed flag) whose wrong answers
// are silent and security-relevant — a grace window that never expires, a cancelled prompt that
// still records a grant, or a confirmed flag honoured while real OS authentication is available
// would each quietly weaken this gate without ever failing a build.
func TestAuthorizeDecisionTable(t *testing.T) {
	t.Run("first call prompts and grants", func(t *testing.T) {
		clock := &fakeClock{t: time.Unix(0, 0)}
		calls := 0
		evaluate := func(string) (localauth.Outcome, error) {
			calls++
			return localauth.Granted, nil
		}
		a := localauth.New(clock.now, evaluate, alwaysAvailable)

		out, err := a.Authorize("reveal", false)
		if err != nil || out != localauth.Granted {
			t.Fatalf("Authorize = (%v, %v), want (Granted, nil)", out, err)
		}
		if calls != 1 {
			t.Fatalf("evaluate called %d times, want 1", calls)
		}
	})

	t.Run("a second call inside the grace window does not prompt again", func(t *testing.T) {
		clock := &fakeClock{t: time.Unix(0, 0)}
		calls := 0
		evaluate := func(string) (localauth.Outcome, error) {
			calls++
			return localauth.Granted, nil
		}
		a := localauth.New(clock.now, evaluate, alwaysAvailable)

		mustAuthorize(t, a, false, localauth.Granted)
		clock.advance(1 * time.Minute) // still inside the 5-minute window
		mustAuthorize(t, a, false, localauth.Granted)
		if calls != 1 {
			t.Fatalf("evaluate called %d times inside the grace window, want 1", calls)
		}
	})

	t.Run("a call past the grace deadline prompts again", func(t *testing.T) {
		clock := &fakeClock{t: time.Unix(0, 0)}
		calls := 0
		evaluate := func(string) (localauth.Outcome, error) {
			calls++
			return localauth.Granted, nil
		}
		a := localauth.New(clock.now, evaluate, alwaysAvailable)

		mustAuthorize(t, a, false, localauth.Granted)
		clock.advance(localauth.GraceWindow + time.Second)
		mustAuthorize(t, a, false, localauth.Granted)
		if calls != 2 {
			t.Fatalf("evaluate called %d times across the deadline, want 2", calls)
		}
	})

	t.Run("a cancelled prompt records no grant, so the next call prompts again", func(t *testing.T) {
		clock := &fakeClock{t: time.Unix(0, 0)}
		calls := 0
		evaluate := func(string) (localauth.Outcome, error) {
			calls++
			return localauth.Cancelled, nil
		}
		a := localauth.New(clock.now, evaluate, alwaysAvailable)

		mustAuthorize(t, a, false, localauth.Cancelled)
		mustAuthorize(t, a, false, localauth.Cancelled)
		if calls != 2 {
			t.Fatalf("evaluate called %d times after a cancel, want 2 (no grant recorded)", calls)
		}
	})

	t.Run("an evaluate error surfaces through the returned error", func(t *testing.T) {
		clock := &fakeClock{t: time.Unix(0, 0)}
		wantErr := errors.New("boom")
		evaluate := func(string) (localauth.Outcome, error) { return localauth.Cancelled, wantErr }
		a := localauth.New(clock.now, evaluate, alwaysAvailable)

		_, err := a.Authorize("reveal", false)
		if !errors.Is(err, wantErr) {
			t.Fatalf("Authorize error = %v, want %v", err, wantErr)
		}
	})

	t.Run("confirmed is ignored while OS authentication is available", func(t *testing.T) {
		clock := &fakeClock{t: time.Unix(0, 0)}
		calls := 0
		evaluate := func(string) (localauth.Outcome, error) {
			calls++
			return localauth.Granted, nil
		}
		a := localauth.New(clock.now, evaluate, alwaysAvailable)

		// confirmed:true must not skip the real prompt just because OS auth can run.
		mustAuthorize(t, a, true, localauth.Granted)
		if calls != 1 {
			t.Fatalf("evaluate called %d times, want 1 — confirmed must not bypass a real prompt", calls)
		}
	})

	t.Run("confirmed grants once and records no grace while OS auth is unavailable", func(t *testing.T) {
		clock := &fakeClock{t: time.Unix(0, 0)}
		evaluate := func(string) (localauth.Outcome, error) {
			t.Fatal("evaluate must never be called while available() is false")
			return localauth.Granted, nil
		}
		a := localauth.New(clock.now, evaluate, neverAvailable)

		mustAuthorize(t, a, true, localauth.Granted)
		// No grace recorded: an unconfirmed call right afterward must ask again, not ride the
		// previous confirm's coattails.
		mustAuthorize(t, a, false, localauth.Unavailable)
	})

	t.Run("an unconfirmed call reports unavailable without ever prompting", func(t *testing.T) {
		clock := &fakeClock{t: time.Unix(0, 0)}
		evaluate := func(string) (localauth.Outcome, error) {
			t.Fatal("evaluate must never be called while available() is false")
			return localauth.Granted, nil
		}
		a := localauth.New(clock.now, evaluate, neverAvailable)

		mustAuthorize(t, a, false, localauth.Unavailable)
	})
}

func alwaysAvailable() bool { return true }
func neverAvailable() bool  { return false }

func mustAuthorize(t *testing.T, a *localauth.Authorizer, confirmed bool, want localauth.Outcome) {
	t.Helper()
	out, err := a.Authorize("reveal a saved connection password.", confirmed)
	if err != nil {
		t.Fatalf("Authorize(confirmed=%t) error = %v, want nil", confirmed, err)
	}
	if out != want {
		t.Fatalf("Authorize(confirmed=%t) = %v, want %v", confirmed, out, want)
	}
}

// TestConcurrentAuthorizePromptsOnce is P168 Part 2 F5: reveals racing outside the grace window
// share one OS prompt.
func TestConcurrentAuthorizePromptsOnce(t *testing.T) {
	var calls atomic.Int32
	evaluate := func(string) (localauth.Outcome, error) {
		calls.Add(1)
		time.Sleep(50 * time.Millisecond)
		return localauth.Granted, nil
	}
	a := localauth.New(func() time.Time { return time.Unix(0, 0) }, evaluate, alwaysAvailable)

	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			mustAuthorize(t, a, false, localauth.Granted)
		}()
	}
	wg.Wait()
	if got := calls.Load(); got != 1 {
		t.Fatalf("evaluate called %d times, want 1", got)
	}
}
