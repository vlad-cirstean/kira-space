package harness

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestEventQueuePerSenderOrder(t *testing.T) {
	var mu sync.Mutex
	got := map[string][]int{}
	q := newEventQueue(func(ev AgentEvent) {
		var n int
		_, _ = fmt.Sscan(ev.Message, &n)
		mu.Lock()
		got[ev.TerminalID] = append(got[ev.TerminalID], n)
		mu.Unlock()
	})
	var wg sync.WaitGroup
	for g := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range 100 {
				q.enqueue(AgentEvent{TerminalID: fmt.Sprint(g), Message: fmt.Sprint(i)})
			}
		}()
	}
	wg.Wait()
	q.close()
	for g := range 8 {
		seq := got[fmt.Sprint(g)]
		if len(seq) != 100 {
			t.Fatalf("sender %d: %d events, want 100", g, len(seq))
		}
		for i, n := range seq {
			if n != i {
				t.Fatalf("sender %d: event %d is %d", g, i, n)
			}
		}
	}
}

func TestEventQueueBackpressure(t *testing.T) {
	release := make(chan struct{})
	started := make(chan struct{}, 1)
	q := newEventQueue(func(AgentEvent) {
		select {
		case started <- struct{}{}:
		default:
		}
		<-release
	})
	q.enqueue(AgentEvent{})
	<-started
	for range eventQueueCap {
		q.enqueue(AgentEvent{})
	}
	returned := make(chan struct{})
	go func() {
		q.enqueue(AgentEvent{})
		close(returned)
	}()
	select {
	case <-returned:
		t.Fatal("enqueue returned while the queue was full")
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	select {
	case <-returned:
	case <-time.After(5 * time.Second):
		t.Fatal("enqueue still blocked after release")
	}
	q.close()
}

func TestEventQueueCloseDrains(t *testing.T) {
	var n, afterClose atomic.Int32
	var closed atomic.Bool
	q := newEventQueue(func(AgentEvent) {
		time.Sleep(time.Millisecond)
		if closed.Load() {
			afterClose.Add(1)
		}
		n.Add(1)
	})
	for range 50 {
		q.enqueue(AgentEvent{})
	}
	q.close()
	closed.Store(true)
	if n.Load() != 50 {
		t.Fatalf("handled %d, want 50", n.Load())
	}
	q.enqueue(AgentEvent{})
	time.Sleep(10 * time.Millisecond)
	if n.Load() != 50 || afterClose.Load() != 0 {
		t.Fatalf("event handled after close: n=%d", n.Load())
	}
}

func TestEventQueueBarrierWaitsForEarlier(t *testing.T) {
	var n atomic.Int32
	q := newEventQueue(func(AgentEvent) {
		time.Sleep(2 * time.Millisecond)
		n.Add(1)
	})
	defer q.close()
	for range 20 {
		q.enqueue(AgentEvent{})
	}
	q.barrier("")
	if n.Load() != 20 {
		t.Fatalf("barrier returned after %d events, want 20", n.Load())
	}
}

func TestEventQueuePanicKeepsWorker(t *testing.T) {
	var n atomic.Int32
	q := newEventQueue(func(ev AgentEvent) {
		if ev.Message == "boom" {
			panic("boom")
		}
		n.Add(1)
	})
	q.enqueue(AgentEvent{Message: "boom"})
	q.enqueue(AgentEvent{})
	q.close()
	if n.Load() != 1 {
		t.Fatalf("handled %d after panic, want 1", n.Load())
	}
}

// A handler stuck on one terminal delays that terminal's later events and barrier, not another
// terminal's.
func TestEventQueueTerminalsDoNotBlockEachOther(t *testing.T) {
	release := make(chan struct{})
	var handled sync.Map
	q := newEventQueue(func(ev AgentEvent) {
		if ev.TerminalID == "slow" {
			<-release
		}
		handled.Store(ev.TerminalID+ev.Message, true)
	})
	defer q.close()
	q.enqueue(AgentEvent{TerminalID: "slow", Message: "1"})
	q.enqueue(AgentEvent{TerminalID: "slow", Message: "2"})
	q.enqueue(AgentEvent{TerminalID: "fast", Message: "1"})
	done := make(chan struct{})
	go func() { q.barrier("fast"); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("fast terminal's barrier waited on the stuck terminal")
	}
	if _, ok := handled.Load("fast1"); !ok {
		t.Fatal("fast event not handled before its barrier returned")
	}
	slowDone := make(chan struct{})
	go func() { q.barrier("slow"); close(slowDone) }()
	select {
	case <-slowDone:
		t.Fatal("slow terminal's barrier returned while its handler was stuck")
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	<-slowDone
	if _, ok := handled.Load("slow2"); !ok {
		t.Fatal("slow terminal's second event lost")
	}
}
