package enginecache

import (
	"testing"
	"time"
)

// Ported 1:1 from tests/unit/engine-cache.spec.ts's "ByteLru" describe block.

func TestByteLru_EvictsOldestOnceBudgetExceeded(t *testing.T) {
	lru := NewByteLru[string](100, nil)
	meta := EntryMeta{ConnectionID: "c", Path: "p"}
	lru.Set("a", "a", 40, meta)
	lru.Set("b", "b", 40, meta)
	lru.Set("c", "c", 40, meta) // 120 > 100 — evicts 'a' (oldest)
	if lru.Bytes() > 100 {
		t.Errorf("bytes = %d, want <= 100", lru.Bytes())
	}
	if _, ok := lru.Get("a"); ok {
		t.Error("'a' should have been evicted")
	}
	if v, ok := lru.Get("b"); !ok || v != "b" {
		t.Errorf("'b' = %q, %v, want \"b\", true", v, ok)
	}
	if v, ok := lru.Get("c"); !ok || v != "c" {
		t.Errorf("'c' = %q, %v, want \"c\", true", v, ok)
	}
}

func TestByteLru_GetTouchesEntry(t *testing.T) {
	lru := NewByteLru[string](100, nil)
	meta := EntryMeta{ConnectionID: "c", Path: "p"}
	lru.Set("a", "a", 40, meta)
	lru.Set("b", "b", 40, meta)
	lru.Get("a")                // touch — 'a' is now newer than 'b'
	lru.Set("c", "c", 40, meta) // 120 > 100 — evicts the now-oldest, 'b', not 'a'
	if v, ok := lru.Get("a"); !ok || v != "a" {
		t.Errorf("'a' = %q, %v, want \"a\", true", v, ok)
	}
	if _, ok := lru.Get("b"); ok {
		t.Error("'b' should have been evicted, not 'a'")
	}
}

func TestByteLru_RefusesEntryOverHalfBudget(t *testing.T) {
	var warned string
	lru := NewByteLru[string](100, func(msg string) { warned = msg })
	meta := EntryMeta{ConnectionID: "c", Path: "p", Label: "huge"}
	lru.Set("huge", "huge", 51, meta) // > 100/2
	if _, ok := lru.Get("huge"); ok {
		t.Error("an entry over half the budget must not be stored")
	}
	if lru.Bytes() != 0 || lru.Size() != 0 {
		t.Errorf("bytes=%d size=%d, want 0, 0", lru.Bytes(), lru.Size())
	}
	if warned == "" {
		t.Error("expected a warning for the refused entry")
	}
}

func TestByteLru_DeleteWhere(t *testing.T) {
	lru := NewByteLru[string](1000, nil)
	lru.Set("a", "a", 10, EntryMeta{ConnectionID: "conn1", Path: "p1"})
	lru.Set("b", "b", 10, EntryMeta{ConnectionID: "conn2", Path: "p2"})
	removed := lru.DeleteWhere(func(m EntryMeta) bool { return m.ConnectionID == "conn1" })
	if removed != 1 {
		t.Errorf("removed = %d, want 1", removed)
	}
	if lru.Size() != 1 || lru.Bytes() != 10 {
		t.Errorf("size=%d bytes=%d, want 1, 10", lru.Size(), lru.Bytes())
	}
	if _, ok := lru.Get("a"); ok {
		t.Error("'a' should have been removed")
	}
	if v, ok := lru.Get("b"); !ok || v != "b" {
		t.Errorf("'b' = %q, %v, want \"b\", true", v, ok)
	}
}

// countStore.markTargetStale used to round-trip a stale flip
// through Set — Set's own "touch" semantics (stamp `at: time.Now()`, move to the newest end) are
// right for a fetch that genuinely refreshes an entry, but wrong for this purely local field flip,
// which carries no new recency information at all. Update exists precisely to mutate a value in
// place without either side effect.
func TestByteLru_UpdateDoesNotTouchPositionOrTimestamp(t *testing.T) {
	lru := NewByteLru[string](100, nil)
	meta := EntryMeta{ConnectionID: "c", Path: "p"}
	lru.Set("a", "a", 40, meta)
	time.Sleep(2 * time.Millisecond) // a real, observable clock difference from 'a' to 'b'
	lru.Set("b", "b", 40, meta)

	before := lru.Entries() // oldest first
	if before[0].Key != "a" {
		t.Fatalf("expected 'a' to be oldest before Update, entries = %+v", before)
	}
	originalAt := before[0].At

	if !lru.Update("a", "a-updated", 40) {
		t.Fatal("Update: want true for an existing key")
	}

	after := lru.Entries()
	if after[0].Key != "a" {
		t.Errorf("'a' moved in LRU order after Update — want it to stay oldest, got order %+v", after)
	}
	if after[0].Value != "a-updated" {
		t.Errorf("value = %q, want %q", after[0].Value, "a-updated")
	}
	if !after[0].At.Equal(originalAt) {
		t.Errorf("At = %v, want unchanged %v — Update must not stamp a new timestamp", after[0].At, originalAt)
	}

	// Confirm the position claim behaviorally, not just via Entries(): overflowing the budget
	// must still evict 'a' first (still the oldest) — Set's own touch semantics would have
	// promoted 'a' to newest, and 'b' would be evicted instead.
	lru.Set("c", "c", 40, meta)
	if _, ok := lru.Get("a"); ok {
		t.Error("'a' should have been evicted as the oldest entry — Update must not have promoted it")
	}
	if v, ok := lru.Get("b"); !ok || v != "b" {
		t.Errorf("'b' = %q, %v, want \"b\", true", v, ok)
	}
}

func TestByteLru_UpdateOnMissingKeyIsNoop(t *testing.T) {
	lru := NewByteLru[string](100, nil)
	if lru.Update("missing", "x", 10) {
		t.Error("Update on a missing key: want false")
	}
	if lru.Size() != 0 || lru.Bytes() != 0 {
		t.Errorf("size=%d bytes=%d, want 0, 0", lru.Size(), lru.Bytes())
	}
}

func TestByteLru_SetBudgetShrinksImmediately(t *testing.T) {
	lru := NewByteLru[string](1000, nil)
	meta := EntryMeta{ConnectionID: "c", Path: "p"}
	lru.Set("a", "a", 400, meta)
	lru.Set("b", "b", 400, meta)
	lru.SetBudget(500) // 800 > 500 — evicts 'a' (oldest) until under budget
	if lru.Bytes() > 500 {
		t.Errorf("bytes = %d, want <= 500", lru.Bytes())
	}
	if _, ok := lru.Get("a"); ok {
		t.Error("'a' should have been evicted")
	}
	if v, ok := lru.Get("b"); !ok || v != "b" {
		t.Errorf("'b' = %q, %v, want \"b\", true", v, ok)
	}
}
