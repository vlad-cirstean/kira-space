package adapterhost

import (
	"context"

	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/tableref"
)

// TablesResult is what a statement touches: Refs, or Refusal when the statement cannot be vouched for.
type TablesResult struct {
	Refs    []tableref.Ref
	Refusal string
}

// StatementTables names the tables statement touches on connectionID's live adapter, for the DB MCP
// table allow list (P532). Parsing is pure; the catalog probe runs as a throttled op only when the
// parse needs one. An adapter that cannot name tables refuses.
func (r *Router) StatementTables(ctx context.Context, connectionID, encodedPath, statement string) (TablesResult, error) {
	adapter, err := requireLiveAdapter(connectionID)
	if err != nil {
		return TablesResult{}, err
	}
	path, err := decodePath(connectionID, encodedPath)
	if err != nil {
		return TablesResult{}, err
	}
	ref, ok := adapter.(adapters.TableReferencer)
	if !ok {
		return TablesResult{Refusal: "this engine cannot name the tables of a statement"}, nil
	}
	parsed := ref.ParseTables(statement, path)
	if parsed.Refusal != "" {
		return TablesResult{Refusal: parsed.Refusal}, nil
	}
	if !parsed.NeedsProbe() {
		return TablesResult{Refs: parsed.Refs}, nil
	}
	id := connectionID
	_, value, err := r.host.RunOp(ctx, OpSpec{ConnectionID: &id, Kind: "tables"},
		func(ctx context.Context, op *adapters.OpCtx) (any, error) {
			refs, refusal, err := ref.ResolveTables(ctx, parsed, path, op)
			if err != nil {
				return nil, err
			}
			return TablesResult{Refs: append(append([]tableref.Ref{}, parsed.Refs...), refs...), Refusal: refusal}, nil
		})
	if err != nil {
		return TablesResult{}, err
	}
	return value.(TablesResult), nil
}
