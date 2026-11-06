package enginecache

import (
	"sync"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/page"
)

// CacheStats is data-ops.ts's CacheStats, six ints. The router merges this with the Node engine's
// own stats during coexistence — that merge lives in the router, not here.
// JSON tags matter here, not just style: this struct crosses the wire directly as a data-plane
// response/event payload, and the renderer's CacheStats expects these exact lowercase-first
// key names — without the tags, encoding/json would emit "L2Bytes" and the renderer's zod parse
// would fail (its own decode is more forgiving, matching case-insensitively, which is exactly the
// kind of one-way compatibility that hides this bug until the marshal side is exercised).
type CacheStats struct {
	L2Bytes       int `json:"l2Bytes"`
	L2BudgetBytes int `json:"l2BudgetBytes"`
	L2Entries     int `json:"l2Entries"`
	L2Hits        int `json:"l2Hits"`
	L2Misses      int `json:"l2Misses"`
	L3Entries     int `json:"l3Entries"`
}

func (a CacheStats) equal(b CacheStats) bool { return a == b }

// Cache is the Go analogue of cache/index.ts: L2 (pages) + L3 (counts) plus the throttled
// stats-changed notification. Unlike the TypeScript module-level singleton, this is a value
// constructed by main.go and held by the router — one mutex guards pages, counts and the
// listener/throttle state together, since ByteLru itself has none.
type Cache struct {
	log func(level, message string)

	mu     sync.Mutex
	pages  *pageStore
	counts *countStore
	gen    *generationTracker

	listeners      map[int]func(CacheStats)
	nextListenerID int
	lastEmitted    *CacheStats
	throttled      bool
}

// NewCache constructs a Cache with an initial L2 budget. log receives "warn" for the half-budget
// refusal (lru.ts:56-63); pass nil to discard it.
func NewCache(l2BudgetBytes int, log func(level, message string)) *Cache {
	if log == nil {
		log = func(string, string) {}
	}
	c := &Cache{log: log, listeners: make(map[int]func(CacheStats)), gen: newGenerationTracker()}
	warn := func(msg string) { c.log("warn", msg) }
	c.pages = newPageStore(warn)
	c.counts = newCountStore(warn)
	if l2BudgetBytes > 0 {
		c.pages.configureBudget(l2BudgetBytes)
	}
	return c
}

func (c *Cache) currentStatsLocked() CacheStats {
	ps := c.pages.stats()
	return CacheStats{
		L2Bytes: ps.bytes, L2BudgetBytes: ps.budgetBytes, L2Entries: ps.entries,
		L2Hits: ps.hits, L2Misses: ps.misses, L3Entries: c.counts.entryCount(),
	}
}

// scheduleEmitLocked mirrors index.ts's scheduleEmit: throttled to at most 1 Hz (D16), via
// time.AfterFunc rather than a ticker — a ticker fires while idle, and "an idle app posts nothing"
// is a property the status bar depends on. Must be called with c.mu held.
func (c *Cache) scheduleEmitLocked() {
	if c.throttled {
		return
	}
	c.throttled = true
	time.AfterFunc(time.Second, func() {
		c.mu.Lock()
		c.throttled = false
		stats := c.currentStatsLocked()
		changed := c.lastEmitted == nil || !stats.equal(*c.lastEmitted)
		var toNotify []func(CacheStats)
		if changed {
			c.lastEmitted = &stats
			for _, cb := range c.listeners {
				toNotify = append(toNotify, cb)
			}
		}
		c.mu.Unlock()
		for _, cb := range toNotify {
			cb(stats)
		}
	})
}

// Configure mirrors cache.configure.
func (c *Cache) Configure(l2BudgetBytes int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pages.configureBudget(l2BudgetBytes)
	c.scheduleEmitLocked()
}

// ReadPage mirrors cache.readPage. The caller computes key via PageCacheKey(req) once and reuses
// it for the paired StorePage call on a miss — the same split data.ts's handleRead makes, so a
// cache hit costs one map lookup, not one hash plus a lookup.
func (c *Cache) ReadPage(key string) (page.Page, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	p, ok := c.pages.get(key)
	c.scheduleEmitLocked()
	return p, ok
}

// StorePage mirrors cache.storePage. Unconditional — kept for callers (tests) that construct and
// store a page directly with no invalidation race to guard against; Dispatcher.Read goes through
// StorePageIfCurrent instead (F4).
func (c *Cache) StorePage(key, label string, req ReadRequest, p page.Page) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pages.put(key, req, label, p)
	c.scheduleEmitLocked()
}

// CurrentGeneration captures connectionID/path's current invalidation generation (F4, P108 Part
// 6) — Read/Count call this right after their own cache-miss check, before issuing the underlying
// op, and pass the result to StorePageIfCurrent/StoreCountIfCurrent once that op returns.
func (c *Cache) CurrentGeneration(connectionID, path string) GenerationSnapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.gen.snapshot(connectionID, path)
}

// StorePageIfCurrent is StorePage, but only stores when gen (from a CurrentGeneration
// call made before the now-finished op was issued) still matches this target's generation —
// a cache miss that raced InvalidateAfterMutation/DropTarget/DropConnection/DropPagesOnly/Clear
// (including mid-reconnect) while the op was in flight must never cache its own now-stale,
// pre-invalidation result as fresh.
func (c *Cache) StorePageIfCurrent(key, label string, req ReadRequest, p page.Page, gen GenerationSnapshot) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.gen.snapshot(req.ConnectionID, req.Path) != gen {
		return
	}
	c.pages.put(key, req, label, p)
	c.scheduleEmitLocked()
}

// Count mirrors cache.count.
func (c *Cache) Count(connectionID, path string, filter *string) (CountEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.counts.get(connectionID, path, filter)
}

// StoreCount mirrors cache.storeCount. Unconditional — see StorePage's own doc comment;
// Dispatcher.Count goes through StoreCountIfCurrent instead (F4).
func (c *Cache) StoreCount(connectionID, path string, filter *string, value int64, exact bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.counts.put(connectionID, path, filter, value, exact)
	c.scheduleEmitLocked()
}

// StoreCountIfCurrent is StoreCount, guarded the same way StorePageIfCurrent guards StorePage
// (F4) — count queries on large tables are slow, widening the exact race window this guards
// against.
func (c *Cache) StoreCountIfCurrent(connectionID, path string, filter *string, value int64, exact bool, gen GenerationSnapshot) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.gen.snapshot(connectionID, path) != gen {
		return
	}
	c.counts.put(connectionID, path, filter, value, exact)
	c.scheduleEmitLocked()
}

// DropTarget mirrors cache.dropTarget: an explicit Refresh (DATA_OP.invalidate, scope 'all') drops
// both its pages and its counts hard.
func (c *Cache) DropTarget(connectionID, path string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pages.dropTarget(connectionID, path)
	c.counts.dropTarget(connectionID, path)
	c.gen.bumpTarget(connectionID, path)
	c.scheduleEmitLocked()
}

// InvalidateAfterMutation mirrors cache.invalidateAfterMutation (§7, P43 F12/D17): a local mutation
// drops the target's pages (they may now be wrong) but only marks its counts stale — the pager
// keeps the last known total, greyed, until the user asks to refresh.
func (c *Cache) InvalidateAfterMutation(connectionID, path string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pages.dropTarget(connectionID, path)
	c.counts.markTargetStale(connectionID, path)
	c.gen.bumpTarget(connectionID, path)
	c.scheduleEmitLocked()
}

// DropPagesOnly mirrors cache.dropPagesOnly (DATA_OP.invalidate scope 'pages' — the post-mutation
// reload; leaves the stale count intact).
func (c *Cache) DropPagesOnly(connectionID, path string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pages.dropTarget(connectionID, path)
	c.gen.bumpTarget(connectionID, path)
	c.scheduleEmitLocked()
}

// DropConnection mirrors cache.dropConnection (§2.2: disconnecting releases all its cached pages).
func (c *Cache) DropConnection(connectionID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pages.dropConnection(connectionID)
	c.counts.dropConnection(connectionID)
	c.gen.bumpConnection(connectionID)
	c.scheduleEmitLocked()
}

// Clear mirrors cache.clear.
func (c *Cache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pages.clear()
	c.counts.clear()
	c.gen.bumpGlobal()
	c.scheduleEmitLocked()
}

// Stats mirrors cache.stats.
func (c *Cache) Stats() CacheStats {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.currentStatsLocked()
}

// OnStatsChanged mirrors cache.onStatsChanged: registers cb and returns an unsubscribe func.
func (c *Cache) OnStatsChanged(cb func(CacheStats)) func() {
	c.mu.Lock()
	id := c.nextListenerID
	c.nextListenerID++
	c.listeners[id] = cb
	c.mu.Unlock()
	return func() {
		c.mu.Lock()
		delete(c.listeners, id)
		c.mu.Unlock()
	}
}
