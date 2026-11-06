package bridge

import (
	"context"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiotmp/appcore"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
)

// KibanaService is the Discover surface beyond execute(): data views, the hit histogram and value
// suggestions. Hits themselves run through the ordinary console execute.
type KibanaService struct {
	Deps appcore.Deps
}

func (s *KibanaService) DataViews(ctx context.Context, req model.KibanaDataViewsRequest) ([]model.KibanaDataView, error) {
	if req.ConnectionID == "" {
		return nil, ipcerr.Required("connectionId")
	}
	return s.Deps.Router.KibanaDataViews(ctx, req)
}

func (s *KibanaService) DataView(ctx context.Context, req model.KibanaDataViewRequest) (model.KibanaDataView, error) {
	if req.ConnectionID == "" {
		return model.KibanaDataView{}, ipcerr.Required("connectionId")
	}
	if req.ID == "" {
		return model.KibanaDataView{}, ipcerr.Required("id")
	}
	return s.Deps.Router.KibanaDataView(ctx, req)
}

func (s *KibanaService) Histogram(ctx context.Context, req model.KibanaHistogramRequest) (model.KibanaHistogram, error) {
	if req.ConnectionID == "" {
		return model.KibanaHistogram{}, ipcerr.Required("connectionId")
	}
	return s.Deps.Router.KibanaHistogram(ctx, req)
}

func (s *KibanaService) Suggest(ctx context.Context, req model.KibanaSuggestRequest) (model.KibanaSuggestions, error) {
	if req.ConnectionID == "" {
		return model.KibanaSuggestions{}, ipcerr.Required("connectionId")
	}
	return s.Deps.Router.KibanaSuggest(ctx, req)
}
