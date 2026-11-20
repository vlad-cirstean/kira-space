package fswatch

import (
	"sync/atomic"
	"testing"
	"time"
)

// A steady Schedule stream never goes quiet: maxWait must still fire fn.
func TestDebouncerMaxWaitCapsSteadyStream(t *testing.T) {
	fired := make(chan struct{}, 1)
	d := NewDebouncer(80*time.Millisecond, 250*time.Millisecond, func() { fired <- struct{}{} })
	defer d.Stop()
	tick := time.NewTicker(20 * time.Millisecond)
	defer tick.Stop()
	start := time.Now()
	for {
		d.Schedule()
		select {
		case <-fired:
			if el := time.Since(start); el > 2*time.Second {
				t.Fatalf("fired after %v, max wait 250ms", el)
			}
			return
		case <-tick.C:
			if time.Since(start) > 5*time.Second {
				t.Fatal("never fired under a steady stream")
			}
		}
	}
}

// Stop waits for an in-flight fn, and nothing fires after it returns.
func TestDebouncerStopWaitsAndSilences(t *testing.T) {
	var calls atomic.Int32
	started, release := make(chan struct{}), make(chan struct{})
	d := NewDebouncer(time.Millisecond, 0, func() {
		if calls.Add(1) == 1 {
			close(started)
			<-release
		}
	})
	d.Schedule()
	<-started
	stopped := make(chan struct{})
	go func() { d.Stop(); close(stopped) }()
	select {
	case <-stopped:
		t.Fatal("Stop returned while fn was running")
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	<-stopped
	d.Schedule()
	time.Sleep(50 * time.Millisecond)
	if n := calls.Load(); n != 1 {
		t.Fatalf("fn ran %d times, want 1 (none after Stop)", n)
	}
}
