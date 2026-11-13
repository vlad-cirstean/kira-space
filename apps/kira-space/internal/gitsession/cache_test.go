package gitsession

import (
	"errors"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitclient/porcelain"
)

// diffCache's own `order` used to be a plain `[]diffCacheKey`, scanned linearly
// (removeFromOrderLocked) on every touch-on-get and every re-set of an already-cached key — O(n)
// per access against a cache with no entry-count cap, only a byte budget a repository with many
// small patches can hold thousands of entries under. Rewired onto a container/list.List (O(1)
// MoveToBack/Remove via the list's own element pointers) plus an index from key to *list.Element.
// These tests are diffCache's first-ever coverage — they exist to prove the list-based rewrite
// preserves the exact LRU behaviour the old slice-based one had, not merely that it doesn't crash.

func diffBody(tag string) porcelain.FileDiffBody {
	return porcelain.FileDiffBody{Kind: porcelain.BodyText, Hunks: []porcelain.DiffHunk{{Heading: tag}}}
}

func TestDiffCache_GetSetRoundTrips(t *testing.T) {
	t.Parallel()
	c := newDiffCache(1 << 20)
	c.set("base", "sha", "a.txt", diffBody("a"), 10)

	body, bytes, ok := c.get("base", "sha", "a.txt")
	if !ok {
		t.Fatal("get: not found, want a hit")
	}
	if body.Hunks[0].Heading != "a" || bytes != 10 {
		t.Fatalf("get = (%+v, %d), want (a, 10)", body, bytes)
	}

	if _, _, ok := c.get("base", "sha", "missing.txt"); ok {
		t.Fatal("get for an unset key: want a miss")
	}
}

// TestDiffCache_EvictsLeastRecentlyUsedFirst: a byte budget too small for all three entries must
// evict the one that was neither set most recently NOR touched by a get() since — proving
// `order`'s own front/back semantics (least- to most-recently-used).
func TestDiffCache_EvictsLeastRecentlyUsedFirst(t *testing.T) {
	t.Parallel()
	c := newDiffCache(25) // room for exactly two 10-byte-ish entries plus slack, never three.
	c.set("base", "sha", "a.txt", diffBody("a"), 10)
	c.set("base", "sha", "b.txt", diffBody("b"), 10)
	// Touching a.txt moves it to the most-recently-used end — b.txt is now the oldest untouched
	// entry, even though it was set AFTER a.txt.
	if _, _, ok := c.get("base", "sha", "a.txt"); !ok {
		t.Fatal("get a.txt: want a hit before the touch even matters")
	}
	// Pushes total past capacity — b.txt (now the least-recently-used) must be evicted, not a.txt.
	c.set("base", "sha", "c.txt", diffBody("c"), 10)

	if _, _, ok := c.get("base", "sha", "a.txt"); !ok {
		t.Fatal("a.txt was touched most recently — it must survive eviction")
	}
	if _, _, ok := c.get("base", "sha", "b.txt"); ok {
		t.Fatal("b.txt was the least-recently-used entry — it must have been evicted")
	}
	if _, _, ok := c.get("base", "sha", "c.txt"); !ok {
		t.Fatal("c.txt was just set — it must still be present")
	}
}

// TestDiffCache_ReSettingAnExistingKeyMovesItToMostRecentlyUsed proves set() on an ALREADY-cached
// key re-touches it too (not only get()) — the same removeFromOrderLocked + re-append/re-push
// path both old and new implementations share.
func TestDiffCache_ReSettingAnExistingKeyMovesItToMostRecentlyUsed(t *testing.T) {
	t.Parallel()
	c := newDiffCache(25)
	c.set("base", "sha", "a.txt", diffBody("a1"), 10)
	c.set("base", "sha", "b.txt", diffBody("b"), 10)
	// Re-setting a.txt (a fresh patch for the same key, the real-world "content changed" case)
	// must move it to most-recently-used, exactly like a get() would.
	c.set("base", "sha", "a.txt", diffBody("a2"), 10)
	c.set("base", "sha", "c.txt", diffBody("c"), 10) // evicts the least-recently-used: b.txt.

	if _, _, ok := c.get("base", "sha", "b.txt"); ok {
		t.Fatal("b.txt must have been evicted — a.txt's re-set should have outranked it")
	}
	body, _, ok := c.get("base", "sha", "a.txt")
	if !ok {
		t.Fatal("a.txt must still be present")
	}
	if body.Hunks[0].Heading != "a2" {
		t.Fatalf("a.txt body = %q, want the re-set value a2 (not the stale a1)", body.Hunks[0].Heading)
	}
}

func TestDiffCache_ClearDropsEverything(t *testing.T) {
	t.Parallel()
	c := newDiffCache(1 << 20)
	c.set("base", "sha", "a.txt", diffBody("a"), 10)
	c.set("base", "sha", "b.txt", diffBody("b"), 10)

	c.clear()

	if _, _, ok := c.get("base", "sha", "a.txt"); ok {
		t.Fatal("a.txt: want a miss after clear()")
	}
	if _, _, ok := c.get("base", "sha", "b.txt"); ok {
		t.Fatal("b.txt: want a miss after clear()")
	}
	// The cache must still be usable after clear() — not left in some half-torn-down state.
	c.set("base", "sha", "c.txt", diffBody("c"), 10)
	if _, _, ok := c.get("base", "sha", "c.txt"); !ok {
		t.Fatal("c.txt: want a hit — the cache must work after clear()")
	}
}

// entryMeta: a value lives entryMetaTTL, a write drops it, a failed read is never stored.
func TestEntryMeta_TTLDropAndErrors(t *testing.T) {
	t.Parallel()
	m := newEntryMeta()
	now := time.Unix(1000, 0)
	m.now = func() time.Time { return now }
	loads := 0
	load := func() (string, error) { loads++; return "v", nil }

	for range 3 {
		if v, err := metaGet(m, "k", load); err != nil || v != "v" {
			t.Fatalf("metaGet = %q, %v", v, err)
		}
	}
	if loads != 1 {
		t.Fatalf("loads within TTL = %d, want 1", loads)
	}
	now = now.Add(entryMetaTTL)
	metaGet(m, "k", load) //nolint:errcheck
	if loads != 2 {
		t.Fatalf("loads after TTL = %d, want 2", loads)
	}
	m.drop()
	metaGet(m, "k", load) //nolint:errcheck
	if loads != 3 {
		t.Fatalf("loads after drop = %d, want 3", loads)
	}

	fails := 0
	failing := func() (string, error) { fails++; return "", errors.New("boom") }
	for range 2 {
		if _, err := metaGet(m, "bad", failing); err == nil {
			t.Fatal("want the load error")
		}
	}
	if fails != 2 {
		t.Fatalf("failed loads = %d, want 2 (errors never cache)", fails)
	}
}
