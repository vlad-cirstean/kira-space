package gitsession

import (
	"sync"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitclient"
)

// subscriber is the coalescing per-connection delivery: the watcher goroutine's note() never
// blocks and never allocates per event — it ORs a flag under a small mutex and does a
// non-blocking wake — and one dedicated goroutine per subscriber turns that into at most one
// deliver call per signal kind, however many events arrived in between. This is what "fanned
// out ... over its own coalescing buffered channel so one slow client can't stall the watcher
// for others" means: coalescing lives in the flags, not in a channel of events, so a burst never
// queues N deliveries and a client stuck for a second only ever gets one repo.changed once it's
// unstuck.
type subscriber struct {
	repoID string
	// deliver ends in a connection's emit, which can itself block on a full send
	// buffer behind a wedged socket — precisely why it only ever runs on this subscriber's own
	// goroutine, never on the watcher's.
	deliver func(Event)

	mu       sync.Mutex
	refs     bool
	worktree bool
	crs      bool

	wake chan struct{} // capacity 1: a pending wake, never more.
	done chan struct{}
}

func newSubscriber(repoID string, deliver func(Event)) *subscriber {
	s := &subscriber{
		repoID:  repoID,
		deliver: deliver,
		wake:    make(chan struct{}, 1),
		done:    make(chan struct{}),
	}
	go s.run()
	return s
}

// note is called from the watcher's own goroutine (RepoEntry.note) — it must never block.
func (s *subscriber) note(sig gitclient.Signal) {
	s.mu.Lock()
	switch sig {
	case gitclient.SignalRefsChanged:
		s.refs = true
	case gitclient.SignalWorktreeChanged:
		s.worktree = true
	case SignalChangeRequestsChanged:
		s.crs = true
	}
	s.mu.Unlock()

	select {
	case s.wake <- struct{}{}:
	default: // already woken; the pending flags just set above will be seen on that wake.
	}
}

func (s *subscriber) run() {
	for {
		select {
		case <-s.wake:
			s.mu.Lock()
			refs, worktree, crs := s.refs, s.worktree, s.crs
			s.refs, s.worktree, s.crs = false, false, false
			s.mu.Unlock()

			// Refs first, matching upstream's own ordering when both fire together.
			if refs {
				s.deliver(Event{RepoID: s.repoID, Kind: string(gitclient.SignalRefsChanged)})
			}
			if worktree {
				s.deliver(Event{RepoID: s.repoID, Kind: string(gitclient.SignalWorktreeChanged)})
			}
			if crs {
				s.deliver(Event{RepoID: s.repoID, Kind: string(SignalChangeRequestsChanged)})
			}
		case <-s.done:
			return
		}
	}
}

func (s *subscriber) close() {
	close(s.done)
}
