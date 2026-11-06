// Package relational holds the shape postgres and mysqlfamily's own adapter.go files are otherwise
// byte-identical over: their connection state and the Disconnect body built around it.
// Each engine's own ConnSet type differs (postgres.ConnSet vs mysqlfamily.ConnSet), so ConnState
// and Disconnect stay generic over it rather than this package importing either engine.
package relational

import (
	"context"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// ConnState is every field Connect/Disconnect write concurrently with an in-flight op reading them
// (F3) — guarded together via adapters.Guarded in each engine's own Adapter (P113 G1). S is that
// engine's own *ConnSet wrapper type.
type ConnState[S any] struct {
	ConnSet         S
	Cfg             *model.ResolvedConnectionConfig
	PrimaryDatabase string
	ReadOnly        bool
}

// Disconnect is postgres and mysqlfamily's shared Disconnect body: cancel every query the adapter
// still tracks as running, server-side, before drain — nothing else stops a still-in-flight query's
// own background goroutine from touching the connection, and drain's own wait is bounded by ctx
// rather than able to block this call for as long as the longest still-running query — then close
// the connection set if one is open, then clear the adapter's connection state.
//
// S must be comparable so a not-yet-connected (zero) connSet can be told apart from a real one
// without each engine passing its own nil check; both engines' own ConnSet is a pointer, so this
// holds for both instantiations.
func Disconnect[S comparable](
	ctx context.Context,
	runningOpIDs []string,
	cancel func(context.Context, string) (bool, error),
	drain func(context.Context),
	connSet S,
	closeAll func(S, context.Context),
	clearConnected func(),
) error {
	for _, opID := range runningOpIDs {
		_, _ = cancel(ctx, opID)
	}
	drain(ctx)
	var zero S
	if connSet != zero {
		closeAll(connSet, ctx)
	}
	clearConnected()
	return nil
}
