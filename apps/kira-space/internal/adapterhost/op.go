package adapterhost

import (
	"context"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
)

// runOp folds host.RunOp + the value.(T) type assertion, repeated across Router's Children/
// Describe/Definition/SchemaColumns/KeyTypes and Dispatcher's Read/Count/Mutate/ObjectDownload.
// adapter is already resolved by the caller (requireLiveAdapter stays a separate,
// explicit call there — Dispatcher's four methods run it before a decodePath this function knows
// nothing about, and folding it in here would silently reorder the two checks). call gets the
// live adapter and *adapters.OpCtx; its own SetRows call, nil-slice guard and any cache
// bookkeeping stay with the caller since each op differs there.
func runOp[T any](ctx context.Context, host *Host, connectionID, kind, opID string, tabID *string, adapter adapters.Adapter,
	call func(ctx context.Context, adapter adapters.Adapter, op *adapters.OpCtx) (T, error),
) (T, error) {
	var zero T
	id := connectionID
	_, value, err := host.RunOp(ctx, OpSpec{ConnectionID: &id, Kind: kind, OpID: opID, TabID: tabID},
		func(ctx context.Context, op *adapters.OpCtx) (any, error) {
			return call(ctx, adapter, op)
		})
	if err != nil {
		return zero, err
	}
	return value.(T), nil
}
