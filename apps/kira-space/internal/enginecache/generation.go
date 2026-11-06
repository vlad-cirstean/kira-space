package enginecache

// generationTracker tracks a monotonically increasing generation per (connectionID, path) target,
// one per connection, and one global. Guarded by Cache.mu, not a lock of its
// own: every method here is only ever called from a Cache method that already holds it.
//
// Read/Count store their results unconditionally once the underlying op returns, with nothing
// guarding against InvalidateAfterMutation, DropTarget, DropConnection (including mid-reconnect)
// or Clear having run while that op was still in flight — a cache miss that races one of those
// could still complete and cache its own now-stale, pre-invalidation result as if it were fresh
// (Source: "cache" on the next read). Cache.CurrentGeneration captures a snapshot right after the
// miss decision, before the underlying op is even issued; StorePageIfCurrent/StoreCountIfCurrent
// only store the result if nothing has bumped the generation by the time it returns.
type generationTracker struct {
	global int64
	conn   map[string]int64
	target map[string]int64
}

func newGenerationTracker() *generationTracker {
	return &generationTracker{conn: make(map[string]int64), target: make(map[string]int64)}
}

// GenerationSnapshot opaquely captures one (connectionID, path) target's invalidation generation
// at a point in time — comparable with ==, otherwise meaningless outside this package.
type GenerationSnapshot struct {
	global, conn, target int64
}

func targetGenKey(connectionID, path string) string { return connectionID + "\x00" + path }

func (g *generationTracker) snapshot(connectionID, path string) GenerationSnapshot {
	return GenerationSnapshot{
		global: g.global,
		conn:   g.conn[connectionID],
		target: g.target[targetGenKey(connectionID, path)],
	}
}

// bumpTarget invalidates one (connectionID, path) target — DropTarget, DropPagesOnly,
// InvalidateAfterMutation (the mutation's own pages-drop and, since it also marks counts stale
// rather than dropping them, the reason a stale-mark must bump this too: a Count already in flight
// when the mutation ran must not silently overwrite that stale mark with its own pre-mutation
// result presented as fresh).
func (g *generationTracker) bumpTarget(connectionID, path string) {
	g.target[targetGenKey(connectionID, path)]++
}

// bumpConnection invalidates every target under connectionID at once (DropConnection, including a
// reconnect's own call) — a snapshot taken before this call never matches one taken after it,
// regardless of which specific target it was for; nothing needs to enumerate individual targets.
func (g *generationTracker) bumpConnection(connectionID string) {
	g.conn[connectionID]++
}

// bumpGlobal invalidates every target across every connection (Clear).
func (g *generationTracker) bumpGlobal() {
	g.global++
}
