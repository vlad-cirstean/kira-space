package fswatch

import (
	"sync"
	"time"
)

// Debouncer runs fn once a burst of Schedule calls goes quiet, optionally capped so a steady stream
// cannot defer it forever.
type Debouncer struct {
	quiet, maxWait time.Duration
	fn             func()

	mu      sync.Mutex
	timer   *time.Timer
	first   time.Time // first Schedule since fn last started
	pending bool
	stopped bool
	running sync.WaitGroup // in-flight fn; Stop waits for it
}

// NewDebouncer returns a Debouncer; maxWait 0 means no cap.
func NewDebouncer(quiet, maxWait time.Duration, fn func()) *Debouncer {
	return &Debouncer{quiet: quiet, maxWait: maxWait, fn: fn}
}

// Schedule (re)arms the timer: wait is quiet, capped at first+maxWait-now, floor 0. No-op after Stop.
func (d *Debouncer) Schedule() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.stopped {
		return
	}
	now := time.Now()
	if !d.pending {
		d.pending, d.first = true, now
	}
	wait := d.quiet
	if d.maxWait > 0 {
		wait = min(wait, max(d.first.Add(d.maxWait).Sub(now), 0))
	}
	if d.timer == nil {
		d.timer = time.AfterFunc(wait, d.fire)
		return
	}
	d.timer.Reset(wait)
}

func (d *Debouncer) fire() {
	d.mu.Lock()
	if d.stopped {
		d.mu.Unlock()
		return
	}
	d.pending = false
	d.running.Add(1)
	d.mu.Unlock()
	defer d.running.Done()
	d.fn()
}

// Stop prevents any further fire and waits for an in-flight fn.
func (d *Debouncer) Stop() {
	d.mu.Lock()
	d.stopped = true
	if d.timer != nil {
		d.timer.Stop()
	}
	d.mu.Unlock()
	d.running.Wait()
}
