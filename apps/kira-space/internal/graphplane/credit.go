package graphplane

import (
	"context"
	"sync"
)

// creditGate is rpc.ts's own CreditGate class, transcribed: a small counting semaphore an open
// stream's emit loop waits on before pushing its next chunk. The one stream handler
// (graph.stream) never calls acquire — it has no commits to walk yet — but the mechanism is
// written now, correctly, rather than stubbed, since this is the one place its backpressure has
// to be exactly right.
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
