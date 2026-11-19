package keptvars

import (
	"container/heap"
	"time"
)

// expiryHeap orders items by ExpiresAt; each item tracks its index so removal is O(log n).
type expiryHeap []*item

func (h expiryHeap) Len() int           { return len(h) }
func (h expiryHeap) Less(i, j int) bool { return h[i].meta.ExpiresAt.Before(h[j].meta.ExpiresAt) }
func (h expiryHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].index, h[j].index = i, j
}
func (h *expiryHeap) Push(x any) {
	it := x.(*item)
	it.index = len(*h)
	*h = append(*h, it)
}
func (h *expiryHeap) Pop() any {
	old := *h
	n := len(old)
	it := old[n-1]
	old[n-1] = nil
	it.index = -1
	*h = old[:n-1]
	return it
}

// poke tells the sweeper the earliest expiry may have moved.
func (s *Store) poke() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// sweep removes every due entry as one batch: one revision, one push.
func (s *Store) sweep() {
	var b batch
	s.mu.Lock()
	s.dueLocked(&b)
	rev := s.commitLocked(&b)
	s.mu.Unlock()
	s.finish(&b, rev)
}

// nextDue is the wait until the earliest expiry; ok is false when the store is empty.
func (s *Store) nextDue() (time.Duration, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.expiry) == 0 {
		return 0, false
	}
	return max(s.expiry[0].meta.ExpiresAt.Sub(s.now()), 0), true
}

func (s *Store) sweeper() {
	defer close(s.done)
	timer := time.NewTimer(time.Hour)
	defer timer.Stop()
	for {
		if d, ok := s.nextDue(); ok {
			timer.Reset(d)
		} else {
			timer.Reset(time.Hour)
		}
		select {
		case <-timer.C:
			s.sweep()
		case <-s.wake:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
		case <-s.stop:
			return
		}
	}
}

var _ heap.Interface = (*expiryHeap)(nil)
