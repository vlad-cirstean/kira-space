package enginecache

import "testing"

// Read/Count store their results unconditionally once the underlying op
// returns — a cache miss that races InvalidateAfterMutation/DropTarget/DropConnection/Clear (or a
// reconnect's own DropConnection) while still in flight could still complete and cache its own
// now-stale, pre-invalidation result as fresh. These exercise the guard directly at the Cache
// level (CurrentGeneration + StorePageIfCurrent/StoreCountIfCurrent); the real, genuinely
// concurrent production race lives in adapterhost's own
// TestDispatcher_Read_DoesNotCacheStaleResultRacingConcurrentInvalidate.

func TestCache_StorePageIfCurrent_StoresWhenNothingInvalidatedInBetween(t *testing.T) {
	c := NewCache(DefaultPageBudgetBytes, nil)
	const connID, path = "conn", "database:app/table:t"
	req := ReadRequest{ConnectionID: connID, Path: path, Cursor: pageOffsetCursor(0)}
	key, label := PageCacheKey(req)

	gen := c.CurrentGeneration(connID, path)
	c.StorePageIfCurrent(key, label, req, fakePage(10), gen)

	if _, ok := c.ReadPage(key); !ok {
		t.Error("a miss with no concurrent invalidation must still cache its result")
	}
}

func TestCache_StorePageIfCurrent_SkipsAfterConcurrentDropTarget(t *testing.T) {
	c := NewCache(DefaultPageBudgetBytes, nil)
	const connID, path = "conn", "database:app/table:t"
	req := ReadRequest{ConnectionID: connID, Path: path, Cursor: pageOffsetCursor(0)}
	key, label := PageCacheKey(req)

	gen := c.CurrentGeneration(connID, path)
	// Simulates DropTarget landing while this miss's own op is still in flight.
	c.DropTarget(connID, path)
	c.StorePageIfCurrent(key, label, req, fakePage(10), gen)

	if _, ok := c.ReadPage(key); ok {
		t.Error("a miss racing a concurrent DropTarget must not cache its own stale result")
	}
}

func TestCache_StorePageIfCurrent_SkipsAfterConcurrentDropConnection(t *testing.T) {
	c := NewCache(DefaultPageBudgetBytes, nil)
	const connID, path = "conn", "database:app/table:t"
	req := ReadRequest{ConnectionID: connID, Path: path, Cursor: pageOffsetCursor(0)}
	key, label := PageCacheKey(req)

	gen := c.CurrentGeneration(connID, path)
	// Simulates a disconnect/reconnect's own DropConnection landing mid-flight.
	c.DropConnection(connID)
	c.StorePageIfCurrent(key, label, req, fakePage(10), gen)

	if _, ok := c.ReadPage(key); ok {
		t.Error("a miss racing a concurrent DropConnection must not cache its own stale result")
	}
}

func TestCache_StorePageIfCurrent_SkipsAfterConcurrentClear(t *testing.T) {
	c := NewCache(DefaultPageBudgetBytes, nil)
	const connID, path = "conn", "database:app/table:t"
	req := ReadRequest{ConnectionID: connID, Path: path, Cursor: pageOffsetCursor(0)}
	key, label := PageCacheKey(req)

	gen := c.CurrentGeneration(connID, path)
	c.Clear()
	c.StorePageIfCurrent(key, label, req, fakePage(10), gen)

	if _, ok := c.ReadPage(key); ok {
		t.Error("a miss racing a concurrent Clear must not cache its own stale result")
	}
}

// A Count in flight when a local mutation runs is the widest realistic window (count queries on
// large tables are slow) — InvalidateAfterMutation only marks counts stale, it does not drop them,
// so without this guard the in-flight Count's own eventual store would silently overwrite that
// stale mark with its pre-mutation value presented as fresh.
func TestCache_StoreCountIfCurrent_SkipsAfterConcurrentInvalidateAfterMutation(t *testing.T) {
	c := NewCache(DefaultPageBudgetBytes, nil)
	const connID, path = "conn", "database:app/table:t"
	filter := "status = 'open'"

	gen := c.CurrentGeneration(connID, path)
	c.InvalidateAfterMutation(connID, path)
	c.StoreCountIfCurrent(connID, path, &filter, 42, true, gen)

	if _, ok := c.Count(connID, path, &filter); ok {
		t.Error("a Count racing a concurrent InvalidateAfterMutation must not overwrite the stale mark with a fresh-looking result")
	}
}

func TestCache_StoreCountIfCurrent_StoresWhenNothingInvalidatedInBetween(t *testing.T) {
	c := NewCache(DefaultPageBudgetBytes, nil)
	const connID, path = "conn", "database:app/table:t"
	filter := "status = 'open'"

	gen := c.CurrentGeneration(connID, path)
	c.StoreCountIfCurrent(connID, path, &filter, 42, true, gen)

	entry, ok := c.Count(connID, path, &filter)
	if !ok || entry.Value != 42 {
		t.Fatalf("Count() = (%+v, %v), want (42, true)", entry, ok)
	}
}

// A different (connectionID, path) target's own invalidation must never block an unrelated
// target's store — the generation is per-target (plus per-connection and global), not one shared
// counter that would over-invalidate every concurrent read across the whole cache.
func TestCache_StorePageIfCurrent_UnrelatedTargetInvalidationDoesNotBlockStore(t *testing.T) {
	c := NewCache(DefaultPageBudgetBytes, nil)
	const connID, path, otherPath = "conn", "database:app/table:t", "database:app/table:other"
	req := ReadRequest{ConnectionID: connID, Path: path, Cursor: pageOffsetCursor(0)}
	key, label := PageCacheKey(req)

	gen := c.CurrentGeneration(connID, path)
	c.DropTarget(connID, otherPath)
	c.StorePageIfCurrent(key, label, req, fakePage(10), gen)

	if _, ok := c.ReadPage(key); !ok {
		t.Error("an unrelated target's DropTarget must not block this target's own store")
	}
}
