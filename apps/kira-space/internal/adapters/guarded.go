package adapters

import "sync"

// Guarded holds a value behind a mutex — the mutex-plus-field-set shape every adapter hand-wrote a
// getX/setConnected/clearConnected trio around for its own connection state. Load takes
// the lock, copies the value out, and releases it; Update takes the lock, applies fn to the value
// in place, and releases it. Neither ever holds the lock across a network call — fn must not do
// I/O, and neither should whatever a caller does with a value Load returned.
type Guarded[T any] struct {
	mu sync.Mutex
	v  T
}

// Load returns a copy of the current value.
func (g *Guarded[T]) Load() T {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.v
}

// Update applies fn to the guarded value under the lock.
func (g *Guarded[T]) Update(fn func(*T)) {
	g.mu.Lock()
	fn(&g.v)
	g.mu.Unlock()
}

// View runs fn with the current value while holding the lock. Load's own copy is enough whenever a
// caller only needs T's scalar/pointer fields, but a field that is itself a map or slice mutated in
// place by a later Update (never reassigned wholesale) needs the read to stay inside the same
// critical section as that mutation, or it races the map/slice's own backing storage even though
// the struct copy itself was safe. fn must not do I/O.
func (g *Guarded[T]) View(fn func(T)) {
	g.mu.Lock()
	fn(g.v)
	g.mu.Unlock()
}
