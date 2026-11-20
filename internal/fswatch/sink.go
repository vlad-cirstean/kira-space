package fswatch

// sink is the bounded output. The watcher goroutine never blocks on it: a full buffer drops path
// events and leaves one pending Rescan, which the goroutine sends (select on pendingC) as soon as
// the reader drains. Owned by the watcher goroutine; only out is shared.
type sink struct {
	out     chan Event
	pending bool
}

func newSink(n int) *sink { return &sink{out: make(chan Event, n)} }

func (s *sink) offer(ev Event) {
	if s.pending {
		return // a Rescan is already owed; it covers this event
	}
	select {
	case s.out <- ev:
	default:
		s.pending = true
	}
}

// pendingC is out while a Rescan is owed, else nil (a select case on nil never fires).
func (s *sink) pendingC() chan<- Event {
	if s.pending {
		return s.out
	}
	return nil
}

func (s *sink) flushed() { s.pending = false }
