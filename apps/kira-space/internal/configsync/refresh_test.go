package configsync

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// A burst of RefreshAsync calls while a pass runs collapses into one follow-up pass.
func TestRefreshAsyncCoalesces(t *testing.T) {
	s := New(Options{Home: t.TempDir()})
	var passes atomic.Int32
	started, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	s.refreshRun = func() {
		passes.Add(1)
		once.Do(func() { close(started) })
		<-release
	}
	s.RefreshAsync()
	<-started
	for range 50 {
		s.RefreshAsync()
	}
	close(release)
	deadline := time.Now().Add(5 * time.Second)
	for {
		s.refreshMu.Lock()
		idle := !s.refreshing
		s.refreshMu.Unlock()
		if idle {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("refresh loop did not settle")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if got := passes.Load(); got != 2 {
		t.Fatalf("passes = %d, want 2 (running + one queued)", got)
	}
}
