package adapterhost

import (
	"context"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/page"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/storage/model"
)

// MaxBrowseRows caps a browse page.
const MaxBrowseRows = 100

// BrowseRequest is browse_table's read: one page through adapter.Read on the read lane, never cached.
type BrowseRequest struct {
	OpID             string
	ConnectionID     string
	Path             string
	Projection       []string
	Conditions       []model.Condition
	Sort             *model.SortSpec // structured only
	PageSize         int
	Offset           int
	StatementTimeout time.Duration
	TimeoutByEngine  bool
}

// Browse reads one page of a relation on connectionID's read lane. The page cache is bypassed in both
// directions: conditions are not in its key, and an MCP read must never be served from a grid page.
func (r *Router) Browse(ctx context.Context, req BrowseRequest) (page.Page, error) {
	if req.PageSize < 1 || req.PageSize > MaxBrowseRows || req.Offset < 0 {
		return nil, adapters.Contract("browse page size or offset out of range", nil)
	}
	if req.Sort != nil && req.Sort.Kind != "structured" {
		return nil, adapters.Contract("browse sorts by structured terms only", nil)
	}
	primary, err := requireLiveAdapter(req.ConnectionID)
	if err != nil {
		return nil, err
	}
	path, err := decodePath(req.ConnectionID, req.Path)
	if err != nil {
		return nil, err
	}
	cfg, ok := r.lanes.config(req.ConnectionID)
	if !ok {
		return nil, adapters.New(adapters.CodeEngineDown, "connection "+req.ConnectionID+" is not connected", nil)
	}
	if !laneKinds[cfg.Kind] {
		return nil, adapters.Contract("browse needs a read lane; "+cfg.Kind+" has none", nil)
	}
	lane, err := r.acquireLane(ctx, req.ConnectionID, primary, cfg)
	if err != nil {
		return nil, err
	}
	defer r.lanes.release(req.ConnectionID, lane, r.closeIdleLane)

	id := req.ConnectionID
	spec := OpSpec{ConnectionID: &id, Kind: "read", OpID: req.OpID, Adapter: lane.adapter, Timeout: req.StatementTimeout}
	if req.TimeoutByEngine {
		spec.TimeoutGrace = statementTimeoutGrace
	}
	_, value, err := r.host.RunOp(ctx, spec, func(ctx context.Context, op *adapters.OpCtx) (any, error) {
		op.SetPath(req.Path)
		p, err := lane.adapter.Read(ctx, adapters.ReadRequest{
			Path: path, Projection: req.Projection, Sort: req.Sort, PageSize: req.PageSize,
			Cursor: model.PageCursor{Mode: "offset", Offset: req.Offset}, Conditions: req.Conditions,
			StatementTimeout: req.StatementTimeout,
		}, op)
		if err != nil {
			return nil, err
		}
		op.SetRows(p.Rows())
		return p, nil
	})
	if err != nil {
		return nil, err
	}
	return value.(page.Page), nil
}
