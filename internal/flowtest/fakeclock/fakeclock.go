// Package fakeclock is a manual clock for tests of timer-driven code: time moves only on Advance.
package fakeclock

import (
	"sync"
	"time"
)

type timer struct {
	at time.Time
	ch chan time.Time
}

// Clock implements Now and After over a time set by the test.
type Clock struct {
	mu      sync.Mutex
	now     time.Time
	ran     time.Duration
	timers  []timer
	changed chan struct{}
}

// New returns a clock standing at start.
func New(start time.Time) *Clock {
	return &Clock{now: start, changed: make(chan struct{})}
}

// Now is the current fake time.
func (c *Clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// After returns a channel that receives once the fake time has moved d forward.
func (c *Clock) After(d time.Duration) <-chan time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	ch := make(chan time.Time, 1)
	if d <= 0 {
		ch <- c.now
		return ch
	}
	c.timers = append(c.timers, timer{at: c.now.Add(d), ch: ch})
	close(c.changed)
	c.changed = make(chan struct{})
	return ch
}

// Advance moves the time forward and fires every timer that came due.
func (c *Clock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
	c.ran += d
	kept := c.timers[:0]
	for _, t := range c.timers {
		if t.at.After(c.now) {
			kept = append(kept, t)
			continue
		}
		t.ch <- c.now
	}
	c.timers = kept
}

// Suspend moves the wall time forward the way a sleeping laptop does: no timer fires and no
// running time passes, so Elapsed stays put.
func (c *Clock) Suspend(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// Elapsed is the time the clock has run: every Advance counts, a Suspend does not.
func (c *Clock) Elapsed() time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ran
}

// Waiters is how many timers are pending.
func (c *Clock) Waiters() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.timers)
}

// BlockUntil waits until at least n timers are pending or the deadline passes; it reports which.
func (c *Clock) BlockUntil(n int, within time.Duration) bool {
	deadline := time.After(within)
	for {
		c.mu.Lock()
		have, changed := len(c.timers), c.changed
		c.mu.Unlock()
		if have >= n {
			return true
		}
		select {
		case <-changed:
		case <-deadline:
			return false
		}
	}
}
