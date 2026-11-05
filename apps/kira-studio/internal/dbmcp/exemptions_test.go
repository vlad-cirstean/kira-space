package dbmcp

import (
	"testing"
	"time"
)

func TestExemptionStore(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	clock := func() time.Time { return now }
	rec := exemptionRecord{connectionID: "c", sql: "select 1"}

	t.Run("single use", func(t *testing.T) {
		s := newExemptionStore(clock)
		id := s.mint(rec)
		if got, ok := s.claim(id); !ok || got.sql != "select 1" {
			t.Fatalf("first claim = %+v, %v", got, ok)
		}
		if _, ok := s.claim(id); ok {
			t.Fatal("second claim succeeded")
		}
	})

	t.Run("expires at TTL and a failed claim spends the id", func(t *testing.T) {
		s := newExemptionStore(clock)
		early, late := s.mint(rec), s.mint(rec)
		now = now.Add(ApprovalTimeout - time.Second)
		if _, ok := s.claim(early); !ok {
			t.Fatal("claim just before TTL failed")
		}
		now = now.Add(time.Second)
		if _, ok := s.claim(late); ok {
			t.Fatal("claim at TTL succeeded")
		}
		if len(s.byID) != 0 {
			t.Fatalf("expired record kept: %d", len(s.byID))
		}
	})

	t.Run("mint purges expired", func(t *testing.T) {
		s := newExemptionStore(clock)
		s.mint(rec)
		now = now.Add(ApprovalTimeout)
		s.mint(rec)
		if len(s.byID) != 1 {
			t.Fatalf("store holds %d, want 1", len(s.byID))
		}
	})

	t.Run("evicts the oldest past the cap", func(t *testing.T) {
		s := newExemptionStore(clock)
		first := s.mint(rec)
		for i := 0; i < maxExemptions; i++ {
			now = now.Add(time.Millisecond)
			s.mint(rec)
		}
		if len(s.byID) != maxExemptions {
			t.Fatalf("store holds %d, want %d", len(s.byID), maxExemptions)
		}
		if _, ok := s.claim(first); ok {
			t.Fatal("oldest record survived eviction")
		}
	})
}
