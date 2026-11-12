package appevent

import (
	"sync"
	"testing"
	"time"
)

type flushed struct {
	batch []int
	done  bool
	final string
}

type sink struct {
	mu  sync.Mutex
	got []flushed
}

func (s *sink) add(batch []int, done bool, final string) {
	s.mu.Lock()
	s.got = append(s.got, flushed{batch, done, final})
	s.mu.Unlock()
}

func (s *sink) all() []flushed {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]flushed(nil), s.got...)
}

func TestCoalescerPushDoesNotWaitOnFlush(t *testing.T) {
	block, entered := make(chan struct{}), make(chan struct{}, 1)
	c := NewCoalescer(time.Millisecond, 100, func(int) int { return 1 }, func([]int, bool, string) {
		select {
		case entered <- struct{}{}:
		default:
		}
		<-block
	})
	c.Push(1)
	<-entered
	returned := make(chan struct{})
	go func() {
		c.Push(2)
		close(returned)
	}()
	select {
	case <-returned:
	case <-time.After(2 * time.Second):
		t.Fatal("Push blocked while a flush was in progress")
	}
	close(block)
}

func TestCoalescerKeepsOrder(t *testing.T) {
	var s sink
	c := NewCoalescer(time.Millisecond, 7, func(int) int { return 1 }, s.add)
	for i := range 1000 {
		c.Push(i)
		if i%50 == 0 {
			time.Sleep(2 * time.Millisecond)
		}
	}
	c.Finish("end")
	var seq []int
	for _, f := range s.all() {
		seq = append(seq, f.batch...)
	}
	if len(seq) != 1000 {
		t.Fatalf("flushed %d items, want 1000", len(seq))
	}
	for i, v := range seq {
		if v != i {
			t.Fatalf("item %d is %d: order lost", i, v)
		}
	}
}

func TestCoalescerFinishIsLastAndOnce(t *testing.T) {
	block, entered := make(chan struct{}), make(chan struct{}, 1)
	var s sink
	c := NewCoalescer(time.Millisecond, 100, func(int) int { return 1 }, func(b []int, d bool, f string) {
		if !d {
			entered <- struct{}{}
			<-block
		}
		s.add(b, d, f)
	})
	c.Push(1)
	<-entered
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		c.Finish("end")
		c.Finish("again")
	}()
	go func() {
		defer wg.Done()
		time.Sleep(20 * time.Millisecond)
		c.Push(2)
	}()
	time.Sleep(50 * time.Millisecond)
	close(block)
	wg.Wait()
	got := s.all()
	if len(got) != 2 || got[0].done || !got[1].done || got[1].final != "end" {
		t.Fatalf("flushes = %+v, want the push flush then one final", got)
	}
}
