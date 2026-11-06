package enginecache

import (
	"strconv"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/page"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// Ported from tests/unit/engine-cache.spec.ts's "L2 page cache" and "L3 count cache" describe
// blocks. Each test builds its own Cache, unlike the TypeScript spec's process-global singleton
// that has to call clearPages()/clearCounts() between cases — the exact reason for making Cache a
// constructed value here instead of a package-level one.

func fakePage(byteSize int) page.Page {
	return page.TabularPage{ByteSize: byteSize}
}

func TestCache_L2NeverExceedsBudgetAfterEviction(t *testing.T) {
	c := NewCache(1000, nil)
	req := ReadRequest{ConnectionID: "conn", Path: "database:kira_test/schema:app/table:big_rows"}
	for i := 0; i < 20; i++ {
		key, label := PageCacheKey(ReadRequest{ConnectionID: req.ConnectionID, Path: req.Path, Cursor: pageOffsetCursor(i)})
		c.StorePage(key, label, req, fakePage(200))
	}
	stats := c.Stats()
	if stats.L2Bytes > 1000 {
		t.Errorf("L2Bytes = %d, want <= 1000", stats.L2Bytes)
	}
	if stats.L2Entries >= 20 {
		t.Errorf("L2Entries = %d, want < 20", stats.L2Entries)
	}
}

func TestCache_L3BoundedAtExactly2048Entries(t *testing.T) {
	c := NewCache(DefaultPageBudgetBytes, nil)
	const combos = 2500
	for i := 0; i < combos; i++ {
		filter := filterFor(i)
		c.StoreCount("conn", "database:kira_test/schema:app/table:order_items", &filter, 3, true)
	}
	if got := c.Stats().L3Entries; got != 2048 {
		t.Errorf("L3Entries = %d, want 2048", got)
	}
}

// InvalidateAfterMutation's asymmetry is not exercised by the ported TypeScript unit spec (it is
// presumably covered indirectly by a higher-level packages/db-fixtures/ mutate case), but it is
// one of the four behaviours flagged as looking like a bug without its reason attached, so it gets
// its own direct Go coverage: a local mutation must drop the target's pages outright but only mark
// its counts stale, never drop them — DropTarget is the one that drops both.
func TestCache_InvalidateAfterMutation_DropsPagesButOnlyStalesCounts(t *testing.T) {
	const connID, path = "conn", "database:kira_test/schema:app/table:order_items"
	filter := "status = 'open'"

	c := NewCache(DefaultPageBudgetBytes, nil)
	req := ReadRequest{ConnectionID: connID, Path: path, Cursor: pageOffsetCursor(0)}
	key, label := PageCacheKey(req)
	c.StorePage(key, label, req, fakePage(10))
	c.StoreCount(connID, path, &filter, 42, true)

	c.InvalidateAfterMutation(connID, path)

	if _, ok := c.ReadPage(key); ok {
		t.Error("InvalidateAfterMutation must drop the target's pages")
	}
	entry, ok := c.Count(connID, path, &filter)
	if !ok {
		t.Fatal("InvalidateAfterMutation must not drop the target's counts")
	}
	if entry.Value != 42 || !entry.Stale {
		t.Errorf("Count() = %+v, want value=42 stale=true", entry)
	}
}

// P21 round 3 functional finding 16: InvalidateAfterMutation -> markTargetStale used to flip the
// stale bool by round-tripping the entry through ByteLru.Set, which always moves the touched entry
// to the newest end of the LRU eviction order — a stale-flip carries no new recency information of
// its own, so this silently promoted whatever was merely being marked stale ahead of every other
// L3 entry actually competing for the 2048-entry/256 KiB budget on real activity. (The finding as
// originally written also described this as resetting the stored count's own 30-minute
// countDropAfter clock; verified against this codebase that claim does not hold — that clock reads
// storedCount's own `at` field, set once in put() and left untouched by the stale-flip either way,
// a separate field from ByteLru's internal recency timestamp. The LRU-promotion effect below is
// the real, verified part of the finding.) Direct coverage of the underlying mechanism —
// ByteLru.Update leaving both position and its own internal timestamp untouched — lives in
// lru_test.go's TestByteLru_UpdateDoesNotTouchPositionOrTimestamp.
func TestCache_InvalidateAfterMutation_DoesNotPromoteAStaleFlipInEvictionOrder(t *testing.T) {
	const connID, staleTarget = "conn", "database:kira_test/schema:app/table:stale_me"
	c := NewCache(DefaultPageBudgetBytes, nil)

	// Fill L3 to exactly its 2048-entry budget: staleTarget first (so it starts as the oldest —
	// the first one due for eviction), then 2047 more to fill the rest.
	c.StoreCount(connID, staleTarget, nil, 1, true)
	for i := 0; i < 2047; i++ {
		filter := filterFor(i)
		c.StoreCount("other-conn", "database:kira_test/schema:app/table:filler", &filter, 2, true)
	}
	if got := c.Stats().L3Entries; got != 2048 {
		t.Fatalf("L3Entries = %d, want 2048 before invalidate", got)
	}

	// A local mutation against staleTarget marks it stale — no genuine new activity on it.
	c.InvalidateAfterMutation(connID, staleTarget)

	// One more entry pushes L3 one over budget: the pre-fix Set-based flip would have promoted
	// staleTarget to the newest end, evicting one of the "other-conn" fillers instead — leaving
	// staleTarget's own now-stale total sitting in cache indefinitely at every other target's
	// expense. Fixed, staleTarget is still the oldest and is what gets evicted.
	filter := filterFor(9999)
	c.StoreCount("other-conn", "database:kira_test/schema:app/table:filler", &filter, 2, true)

	if _, ok := c.Count(connID, staleTarget, nil); ok {
		t.Error("staleTarget should have been evicted as the oldest entry — marking it stale must not have promoted it")
	}
}

func filterFor(i int) string {
	return "(1=1) OR (0=" + strconv.Itoa(i) + ")"
}

func pageOffsetCursor(offset int) model.PageCursor {
	return model.PageCursor{Mode: "offset", Offset: offset}
}
