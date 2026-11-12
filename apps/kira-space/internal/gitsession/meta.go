package gitsession

import (
	"sync"
	"time"
)

// entryMetaTTL bounds how long repo-level facts that rarely change (remotes, origin HEAD, config
// values) are reused. A config edit made outside the app shows up after at most this long.
const entryMetaTTL = 30 * time.Second

type metaItem struct {
	val any
	at  time.Time
}

// entryMeta caches small per-entry git reads. Writes through the entry drop it whole; a failed
// read is never stored.
type entryMeta struct {
	mu    sync.Mutex
	items map[string]metaItem
	now   func() time.Time
}

func newEntryMeta() *entryMeta {
	return &entryMeta{items: map[string]metaItem{}, now: time.Now}
}

func (m *entryMeta) drop() {
	m.mu.Lock()
	clear(m.items)
	m.mu.Unlock()
}

// metaGet returns the fresh cached value for key, else load's result, which is cached only when
// load succeeds. load runs outside the lock; concurrent misses may each load once.
func metaGet[T any](m *entryMeta, key string, load func() (T, error)) (T, error) {
	m.mu.Lock()
	if it, ok := m.items[key]; ok && m.now().Sub(it.at) < entryMetaTTL {
		m.mu.Unlock()
		return it.val.(T), nil
	}
	m.mu.Unlock()
	v, err := load()
	if err != nil {
		var zero T
		return zero, err
	}
	m.mu.Lock()
	m.items[key] = metaItem{val: v, at: m.now()}
	m.mu.Unlock()
	return v, nil
}
