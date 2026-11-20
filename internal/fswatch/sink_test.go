package fswatch

import "testing"

// A full buffer drops path events and owes exactly one Rescan, sent once the reader drains.
func TestSinkOverflowDeliversOneRescan(t *testing.T) {
	s := newSink(2)
	for _, p := range []string{"a", "b", "c", "d"} {
		s.offer(Event{Path: p})
	}
	if s.pendingC() == nil {
		t.Fatal("overflow left no Rescan pending")
	}
	<-s.out
	<-s.out
	select {
	case s.pendingC() <- Event{Rescan: true}:
		s.flushed()
	default:
		t.Fatal("pending Rescan not sendable after drain")
	}
	s.offer(Event{Path: "e"}) // pending cleared: normal delivery resumes
	var got []Event
	for len(s.out) > 0 {
		got = append(got, <-s.out)
	}
	if len(got) != 2 || !got[0].Rescan || got[1].Path != "e" {
		t.Fatalf("got %+v, want one Rescan then e", got)
	}
	if s.pendingC() != nil {
		t.Fatal("pending left set")
	}
}

// While a Rescan is owed, further events (Rescans included) are absorbed, not queued twice.
func TestSinkPendingAbsorbs(t *testing.T) {
	s := newSink(1)
	s.offer(Event{Path: "a"})
	s.offer(Event{Path: "b"})
	s.offer(Event{Rescan: true})
	s.offer(Event{Path: "c"})
	<-s.out
	select {
	case s.pendingC() <- Event{Rescan: true}:
		s.flushed()
	default:
		t.Fatal("Rescan not sendable")
	}
	if n := len(s.out); n != 1 {
		t.Fatalf("%d queued after one flush, want 1", n)
	}
}
