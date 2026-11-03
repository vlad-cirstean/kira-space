package graphplane

import (
	"context"
	"sync"
)

// creditGate is a small counting semaphore: a stream's emit acquires one credit before pushing each
// chunk, and the client grants more as it consumes them.
type creditGate struct {
	mu        sync.Mutex
	available int
	waiters   []chan struct{}
}

func newCreditGate() *creditGate { return &creditGate{} }

func (g *creditGate) grant(n int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.available += n
	for g.available > 0 && len(g.waiters) > 0 {
		g.available--
		w := g.waiters[0]
		g.waiters = g.waiters[1:]
		close(w)
	}
}

// acquire blocks until a credit is available or ctx is done — the latter is how a cancelled
// stream's own emit loop unblocks rather than hanging forever on a consumer that stopped granting.
func (g *creditGate) acquire(ctx context.Context) error {
	g.mu.Lock()
	if g.available > 0 {
		g.available--
		g.mu.Unlock()
		return nil
	}
	ch := make(chan struct{})
	g.waiters = append(g.waiters, ch)
	g.mu.Unlock()
	select {
	case <-ch:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
