package ade

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"runtime/debug"
)

// joinOp runs fn once per key at a time. A caller that arrives while an identical call runs waits for
// it and gets its result and error, so a double-click adds no second worktree or run row.
//
// fn runs tracked (Close waits for it) with a ctx that keeps the first caller's values but not its
// cancel, so one window closing never fails the joined callers; the board's own ctx still stops it.
// A caller's cancel only ends its own wait, and the work may still commit. A panic in fn comes back
// as an error: singleflight would re-panic it on a goroutine nothing recovers.
func joinOp[T any](ctx context.Context, b *TaskBoard, key string, fn func(ctx context.Context) (T, error)) (T, error) {
	ch := b.ops.DoChan(key, func() (any, error) {
		if !b.track() {
			return nil, errBoardClosed
		}
		defer b.wg.Done()
		work, cancel := context.WithCancel(context.WithoutCancel(ctx))
		defer cancel()
		stop := context.AfterFunc(b.ctx, cancel)
		defer stop()
		return runRecovered(key, func() (any, error) { return fn(work) })
	})
	select {
	case r := <-ch:
		v, _ := r.Val.(T)
		return v, r.Err
	case <-ctx.Done():
		var zero T
		return zero, ctx.Err()
	}
}

// runRecovered runs fn and turns a panic into an error carrying the op name.
func runRecovered(op string, fn func() (any, error)) (v any, err error) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("ade: operation panicked", "scope", "ade", "op", op, "panic", r, "stack", string(debug.Stack()))
			v, err = nil, fmt.Errorf("ade: %s failed unexpectedly", op)
		}
	}()
	return fn()
}

// opKey names a user operation by kind and every input that changes its outcome: different inputs
// never join.
func opKey(kind string, in any) string {
	raw, err := json.Marshal(in)
	if err != nil {
		return kind + ":" + err.Error() // unmarshalable input never matches another call
	}
	return kind + ":" + string(raw)
}
