package ade

import (
	"context"
	"encoding/json"

	"golang.org/x/sync/singleflight"
)

// joinOp runs fn once per key at a time. A caller that arrives while an identical call runs waits for
// it and gets its result and error, so a double-click adds no second worktree or run row. The first
// caller's ctx drives the work; a joined caller's cancel only stops its own wait.
func joinOp[T any](ctx context.Context, g *singleflight.Group, key string, fn func() (T, error)) (T, error) {
	ch := g.DoChan(key, func() (any, error) { return fn() })
	select {
	case r := <-ch:
		v, _ := r.Val.(T)
		return v, r.Err
	case <-ctx.Done():
		var zero T
		return zero, ctx.Err()
	}
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
