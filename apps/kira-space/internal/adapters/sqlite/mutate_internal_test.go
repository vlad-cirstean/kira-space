package sqlite

import (
	"context"
	"database/sql"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters/testsupport"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// flippingCtx simulates a real cancellation landing after exactly `after` deliberate ctx.Err()
// checks — adapters.CheckNotStarted's own calls (query.go's runRows/runCommand), the only places in
// this package that explicitly ask "has this op been cancelled". Done() is a pure accessor: it
// never advances the counter itself, only Err() does. This distinction matters because
// modernc.org/sqlite's own driver calls ctx.Done() on every single statement execution as part of
// its interruptOnDone machinery (a background goroutine watching for the channel to close, to fire
// sqlite3_interrupt) — if Done() itself also tripped the counter, that housekeeping call would
// silently advance the count on every statement regardless of any real cancellation, making `after`
// mean nothing. With Done() inert, the channel only closes once cumulative Err() checks cross
// `after`, and from that instant modernc.org/sqlite's own Done()-driven interrupt machinery reacts
// exactly the way a genuine context.WithCancel's cancel() would — including for BEGIN
// IMMEDIATE/COMMIT, which (via RunSQLMutation's shared execCommand) carry their own CheckNotStarted
// like every other statement; only the cleanup ROLLBACK runs on a detached cleanupCtx and so never
// trips this counter at all. This lets a test force a cancellation to land at an exact,
// deterministic point in mutate()'s own sequence of ctx-checked calls (the catalog lookup inside
// getReadTarget, then each compiled row op) without any sleep or goroutine race — which of those
// calls lands where isn't part of this package's exported surface, so a test sweeps every plausible
// index instead of hardcoding one.
type flippingCtx struct {
	context.Context
	calls *int32
	after int32
	done  chan struct{}
	once  *sync.Once
}

func newFlippingCtx(after int32) flippingCtx {
	var calls int32
	return flippingCtx{
		Context: context.Background(), calls: &calls, after: after,
		done: make(chan struct{}), once: &sync.Once{},
	}
}

func (c flippingCtx) Err() error {
	if atomic.AddInt32(c.calls, 1) > c.after {
		c.once.Do(func() { close(c.done) })
		return context.Canceled
	}
	return nil
}
func (c flippingCtx) Done() <-chan struct{} {
	return c.done
}

func orderItemsQuantityPlan(id, quantity string) model.MutationPlan {
	return model.MutationPlan{
		Path: model.NodePath{ConnectionID: "test", Segments: []model.PathSegment{
			{Kind: "database", Name: "main"}, {Kind: "table", Name: "order_items"},
		}},
		Ops: []model.MutationRowOp{{
			Kind:    "update",
			Key:     model.RowValues{{Name: "id", Value: &id}},
			Changes: model.RowValues{{Name: "quantity", Value: &quantity}},
		}},
	}
}

// TestMutateCancelledMidTransactionDoesNotLeakOpenTransaction is P2 R2's own regression test for
// mutate()'s cleanup fix — deliberately a direct, same-package call rather than a black-box
// a.Mutate(ctx, ...) call through the exported Adapter (sqlite_test.go's own suite has a comment
// explaining why): runOnConn wraps every op in adapters.RunWithAbortRace (B8's own "opposite
// polarity" design, see abort.go), which runs the real work on an adapter-owned driverCtx derived
// from context.Background(), never from the caller's own ctx — so a caller-side cancellation can
// only ever race "give up waiting", not reach mutate()'s own BEGIN IMMEDIATE/COMMIT sequence at
// all. Calling mutate() directly, the same way postgres_test.go and mysqlfamily_test.go's own
// equivalents call their (undetached) mutate() functions, is the only way to actually land a
// cancellation inside the code this fix touches.
func TestMutateCancelledMidTransactionDoesNotLeakOpenTransaction(t *testing.T) {
	fixture := testsupport.StartSqlite(t)
	db, err := sql.Open("sqlite", "file:"+fixture.Path)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer db.Close()
	// Mirrors the adapter's own Connect (adapter.go): a single physical connection, so a *sql.Conn
	// returned to the pool mid-transaction (this bug) is the very next *sql.Conn any later op gets.
	db.SetMaxOpenConns(1)

	readQuantity := func(id string) string {
		t.Helper()
		var quantity string
		if err := db.QueryRow("SELECT quantity FROM order_items WHERE id = ?", id).Scan(&quantity); err != nil {
			t.Fatalf("readQuantity(%s): %v", id, err)
		}
		return quantity
	}
	baseline := readQuantity("1")

	runMutate := func(ctx context.Context, opID string, plan model.MutationPlan) error {
		t.Helper()
		conn, err := db.Conn(context.Background())
		if err != nil {
			t.Fatalf("Conn: %v", err)
		}
		defer conn.Close()
		_, err = mutate(ctx, conn, adapters.NewOpCtx(opID), false, plan)
		return err
	}

	for after := int32(1); after <= 20; after++ {
		ctx := newFlippingCtx(after)
		err := runMutate(ctx, "op-cancel", orderItemsQuantityPlan("1", "77"))
		if err == nil {
			// This `after` fell past every ctx check mutate() makes (COMMIT included) — the mutate
			// ran to completion normally. Reset row 1 and move on.
			if got := readQuantity("1"); got != "77" {
				t.Fatalf("after=%d: mutate reported success but quantity = %s, want 77", after, got)
			}
			if err := runMutate(context.Background(), "op-reset", orderItemsQuantityPlan("1", baseline)); err != nil {
				t.Fatalf("after=%d: reset mutate: %v", after, err)
			}
			continue
		}

		// A cancellation landed somewhere at or before COMMIT. What that leaves row 1 holding is
		// not fully determined for sqlite specifically: modernc.org/sqlite's own cancellation model
		// fires sqlite3_interrupt() from a background goroutine racing the statement's own
		// completion (interruptOnDone) — the same ambiguity Go's database/sql docs call out for a
		// context-cancelled Tx.Commit generally ("the operation may have succeeded"), and
		// empirically reproducible here under heavy CPU contention. The one guarantee this fix
		// makes, and the one the original bug broke, is narrower and fully deterministic regardless
		// of that race: the transaction is always definitively ended (rolled back or committed),
		// never left open — so a completely unrelated follow-up mutate, on the same pooled
		// connection (SetMaxOpenConns(1) means the next db.Conn() call hands back the very
		// connection the cancelled mutate left behind) and a real bounded ctx, must always complete
		// quickly rather than hang or fail behind a stale open transaction.
		followUpCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err = runMutate(followUpCtx, "op-followup", orderItemsQuantityPlan("2", "55"))
		cancel()
		if err != nil {
			t.Fatalf("after=%d: follow-up mutate on a different row: %v (the earlier cancellation left a stale open transaction)", after, err)
		}
		// Reset both rows for the next sweep value, regardless of whether row 1's cancelled write
		// happened to land (see comment above).
		if err := runMutate(context.Background(), "op-reset", orderItemsQuantityPlan("1", baseline)); err != nil {
			t.Fatalf("after=%d: reset row 1: %v", after, err)
		}
		if err := runMutate(context.Background(), "op-reset-2", orderItemsQuantityPlan("2", "1")); err != nil {
			t.Fatalf("after=%d: reset row 2: %v", after, err)
		}
	}
}
