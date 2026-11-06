package adapterhost

import (
	"context"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
)

// kibanaOp runs call against connectionID's live adapter as an op of kind. Op kinds reuse the
// existing set so the Operations panel needs no new filter: data views list as children, one view
// as describe, a histogram as count, a value suggestion as read.
func kibanaOp[T any](ctx context.Context, r *Router, connectionID, kind, opID, tabID string,
	call func(context.Context, adapters.Discoverer, *adapters.OpCtx) (T, error),
) (T, error) {
	var zero T
	adapter, err := requireLiveAdapter(connectionID)
	if err != nil {
		return zero, err
	}
	d, ok := adapter.(adapters.Discoverer)
	if !ok {
		return zero, adapters.Unsupported(adapter.Kind(), "discover")
	}
	var tab *string
	if tabID != "" {
		tab = &tabID
	}
	return runOp(ctx, r.host, connectionID, kind, opID, tab, adapter,
		func(ctx context.Context, _ adapters.Adapter, op *adapters.OpCtx) (T, error) {
			return call(ctx, d, op)
		})
}

func (r *Router) KibanaDataViews(ctx context.Context, req model.KibanaDataViewsRequest) ([]model.KibanaDataView, error) {
	views, err := kibanaOp(ctx, r, req.ConnectionID, "children", req.OpID, "",
		func(ctx context.Context, d adapters.Discoverer, op *adapters.OpCtx) ([]model.KibanaDataView, error) {
			views, err := d.DataViews(ctx, op)
			op.SetRows(len(views))
			return views, err
		})
	if views == nil {
		views = []model.KibanaDataView{}
	}
	return views, err
}

func (r *Router) KibanaDataView(ctx context.Context, req model.KibanaDataViewRequest) (model.KibanaDataView, error) {
	dv, err := kibanaOp(ctx, r, req.ConnectionID, "describe", req.OpID, "",
		func(ctx context.Context, d adapters.Discoverer, op *adapters.OpCtx) (model.KibanaDataView, error) {
			dv, err := d.DataView(ctx, req.ID, op)
			op.SetRows(len(dv.Fields))
			return dv, err
		})
	if dv.Fields == nil {
		dv.Fields = []model.KibanaField{}
	}
	return dv, err
}

func (r *Router) KibanaHistogram(ctx context.Context, req model.KibanaHistogramRequest) (model.KibanaHistogram, error) {
	h, err := kibanaOp(ctx, r, req.ConnectionID, "count", req.OpID, req.TabID,
		func(ctx context.Context, d adapters.Discoverer, op *adapters.OpCtx) (model.KibanaHistogram, error) {
			h, err := d.Histogram(ctx, req, op)
			op.SetRows(len(h.Buckets))
			return h, err
		})
	if h.Buckets == nil {
		h.Buckets = []model.KibanaHistogramBucket{}
	}
	return h, err
}

func (r *Router) KibanaSuggest(ctx context.Context, req model.KibanaSuggestRequest) (model.KibanaSuggestions, error) {
	s, err := kibanaOp(ctx, r, req.ConnectionID, "read", req.OpID, req.TabID,
		func(ctx context.Context, d adapters.Discoverer, op *adapters.OpCtx) (model.KibanaSuggestions, error) {
			s, err := d.ValueSuggestions(ctx, req, op)
			op.SetRows(len(s.Values))
			return s, err
		})
	if s.Values == nil {
		s.Values = []string{}
	}
	return s, err
}
