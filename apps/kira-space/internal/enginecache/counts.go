package enginecache

import "time"

// Nominal, not measured: an entry is four scalars plus a key, the variance between
// entries is noise, and a fixed cost keeps L3 the same ByteLru shape as L2 instead of a separate
// accounting scheme. ~2048 entries before eviction — well past any realistic browsing session,
// while still a real bound.
const (
	countTTL        = 5 * time.Minute
	countDropAfter  = 30 * time.Minute
	countEntryBytes = 128
	L3BudgetBytes   = 256 * 1024
)

// CountEntry is counts.ts's CountEntry.
type CountEntry struct {
	Value int64
	Exact bool
	At    time.Time
	Stale bool
}

type storedCount struct {
	value int64
	exact bool
	at    time.Time
	stale bool
}

// countKeySep is '\0': it can't appear in a connectionId (uuid) or an encoded path, so it's a safe
// separator between the three key parts.
const countKeySep = "\x00"

func countKey(connectionID, path string, filter *string) string {
	f := ""
	if filter != nil {
		f = *filter
	}
	return connectionID + countKeySep + path + countKeySep + f
}

// countStore is the Go analogue of counts.ts's module-level state, held as a Cache field (§4.7's
// Go-specific note — no package-level singleton).
type countStore struct {
	lru *ByteLru[storedCount]
}

func newCountStore(warn func(string)) *countStore {
	return &countStore{lru: NewByteLru[storedCount](L3BudgetBytes, warn)}
}

// get mirrors counts.ts's getCount: an entry older than countDropAfter is evicted outright; one
// past countTTL, or explicitly marked stale by a local mutation (§7, D18), is returned stale —
// kept, not blanked, so the pager can grey the total and offer a refresh instead of losing the
// number.
func (s *countStore) get(connectionID, path string, filter *string) (CountEntry, bool) {
	key := countKey(connectionID, path, filter)
	entry, ok := s.lru.Get(key)
	if !ok {
		return CountEntry{}, false
	}
	age := time.Since(entry.at)
	if age > countDropAfter {
		s.lru.Delete(key)
		return CountEntry{}, false
	}
	return CountEntry{
		Value: entry.value, Exact: entry.exact, At: entry.at,
		Stale: entry.stale || age > countTTL,
	}, true
}

func (s *countStore) put(connectionID, path string, filter *string, value int64, exact bool) {
	meta := EntryMeta{ConnectionID: connectionID, Path: path, Label: filterLabel(filter)}
	s.lru.Set(countKey(connectionID, path, filter), storedCount{value: value, exact: exact, at: time.Now()}, countEntryBytes, meta)
}

func filterLabel(filter *string) string {
	if filter == nil {
		return ""
	}
	return *filter
}

// markTargetStale mirrors counts.ts's markCountTargetStale (§7): a local mutation marks a target's
// counts stale instead of dropping them, so the pager keeps showing the last known total.
func (s *countStore) markTargetStale(connectionID, path string) int {
	marked := 0
	for _, e := range s.lru.Entries() {
		if e.Meta.ConnectionID != connectionID || e.Meta.Path != path || e.Value.stale {
			continue
		}
		next := e.Value
		next.stale = true
		s.lru.Update(e.Key, next, countEntryBytes)
		marked++
	}
	return marked
}

func (s *countStore) dropTarget(connectionID, path string) int {
	return s.lru.DeleteWhere(func(m EntryMeta) bool { return m.ConnectionID == connectionID && m.Path == path })
}

func (s *countStore) dropConnection(connectionID string) int {
	return s.lru.DeleteWhere(func(m EntryMeta) bool { return m.ConnectionID == connectionID })
}

func (s *countStore) clear() { s.lru.Clear() }

func (s *countStore) entryCount() int { return s.lru.Size() }
