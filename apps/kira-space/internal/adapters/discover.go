package adapters

import (
	"context"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// Discoverer is optional adapter surface for a Kibana-style kind: data views, hit histograms and
// value suggestions beyond what Adapter covers. Like StatementClassifier it is not a method on
// Adapter, so the other kinds need no stub.
type Discoverer interface {
	// DataViews lists the views with empty Fields.
	DataViews(ctx context.Context, op *OpCtx) ([]model.KibanaDataView, error)
	// DataView returns one view with its fields, sorted by name.
	DataView(ctx context.Context, id string, op *OpCtx) (model.KibanaDataView, error)
	Histogram(ctx context.Context, req model.KibanaHistogramRequest, op *OpCtx) (model.KibanaHistogram, error)
	ValueSuggestions(ctx context.Context, req model.KibanaSuggestRequest, op *OpCtx) (model.KibanaSuggestions, error)
}
