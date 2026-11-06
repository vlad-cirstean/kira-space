package postgres

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// White-box (package postgres) coverage for NewConnSet's own wiring onto adapters.ConnSet — the
// LRU/single-flight behaviour itself is covered once, generically, by
// internal/adapters's own connset_test.go; what's left here is postgres-specific: the primaryKey
// normalization between Acquire's key and dial's own database argument still lines up, and Acquire
// (through the shared pool) still serializes concurrent dials for the same database with no real
// network needed.
func TestAcquire_SingleFlightPerDatabase(t *testing.T) {
	old := pgxConnect
	t.Cleanup(func() { pgxConnect = old })

	var inFlight, maxInFlight, totalDials int32
	var gotDatabase atomic.Value
	pgxConnect = func(ctx context.Context, cfg *pgx.ConnConfig) (*pgx.Conn, error) {
		gotDatabase.Store(cfg.Database)
		n := atomic.AddInt32(&inFlight, 1)
		atomic.AddInt32(&totalDials, 1)
		for {
			m := atomic.LoadInt32(&maxInFlight)
			if n <= m || atomic.CompareAndSwapInt32(&maxInFlight, m, n) {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
		atomic.AddInt32(&inFlight, -1)
		return nil, context.DeadlineExceeded
	}

	s := NewConnSet(model.ResolvedConnectionConfig{}, func(string, string) {})
	const callers = 12
	var wg sync.WaitGroup
	wg.Add(callers)
	for i := 0; i < callers; i++ {
		go func() {
			defer wg.Done()
			_, _, _ = s.Acquire(context.Background(), "")
		}()
	}
	wg.Wait()

	if got := atomic.LoadInt32(&maxInFlight); got > 1 {
		t.Errorf("max concurrent dials for the primary database = %d, want at most 1", got)
	}
	if atomic.LoadInt32(&totalDials) == 0 {
		t.Fatal("pgxConnect was never called at all")
	}
	// buildConfig falls back to "postgres" for an entirely blank config (P24) — the point here is
	// only that dial saw the real database argument ("" -> its own fallback), never the internal
	// primaryKey sentinel string leaking through as a literal database name.
	if got := gotDatabase.Load(); got != "postgres" {
		t.Errorf("dial's own database argument = %q, want the unmapped default (\"postgres\"), not the primaryKey sentinel", got)
	}
}
