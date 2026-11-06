package adapters

import "context"

// RunWithAbortRace is abort.ts's withAbortRace: issue runs the real driver call on a context
// detached from ctx's own cancellation, on its own goroutine, while this function returns to the
// caller as soon as either the query settles or ctx is done — whichever first. A local
// cancellation (adapterhost.Host.CancelOp's own ctx.cancel(), the first of its two steps) must
// unblock the caller immediately without itself touching the still-running server-side work
// (query.ts:77-80's "do not fix it by trying to make the query itself abort"): every
// context-native Go driver this package's adapters use (pgx, go-sql-driver/mysql, an HTTP
// request's own ctx, modernc.org/sqlite) honours ctx natively and would otherwise race its own
// cancellation against — and often win before — the adapter's own explicit server-side kill, the
// one place a real cancellation is meant to happen.
//
// release is called exactly once, whenever issue settles — not merely when ctx fires — so
// a caller's own Cancel(opID) still finds whatever it tracks (a backend pid, a thread id, a
// query_id) if it runs shortly after a local abort. sqlite's own adapter uses this same helper with
// the opposite polarity: there, the *op's* context must never reach the driver at all, since
// modernc.org/sqlite's own sqlite3_interrupt firing on ctx.Done() is the only cancellation
// mechanism sqlite has — an adapter-owned cancellable context takes the op's place, and
// RunWithAbortRace races the caller's own ctx.Done() against that context's completion the same
// way it races a network driver's.
func RunWithAbortRace[T any](ctx context.Context, release func(), issue func(context.Context) (T, error)) (T, error) {
	type result struct {
		value T
		err   error
	}
	done := make(chan result, 1)
	go func() {
		v, err := issue(context.WithoutCancel(ctx))
		done <- result{value: v, err: err}
		release()
	}()

	select {
	case r := <-done:
		return r.value, r.err
	case <-ctx.Done():
		var zero T
		return zero, CheckCancelled(ctx)
	}
}
