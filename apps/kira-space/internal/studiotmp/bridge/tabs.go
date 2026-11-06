package bridge

import (
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiotmp/appcore"
	"github.com/kirathecat/kira-studio/internal/appstorage"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
)

type TabsService struct {
	Deps appcore.Deps
}

type TabsListArgs struct {
	WindowKey string `json:"windowKey"`
}

// List returns args.WindowKey's own tab set (P8 F6/C4 — tabs are per-window, not app-wide).
func (s *TabsService) List(args TabsListArgs) ([]model.TabRecord, error) {
	if args.WindowKey == "" {
		return nil, ipcerr.Required("windowKey")
	}
	return appstorage.ListWindowTabs(s.Deps.Repos.Windows, s.Deps.Repos.Tabs, args.WindowKey)
}

type TabsSaveArgs struct {
	WindowKey string            `json:"windowKey"`
	Tabs      []model.TabRecord `json:"tabs"`
}

func (s *TabsService) Save(args TabsSaveArgs) error {
	if args.WindowKey == "" {
		return ipcerr.Required("windowKey")
	}
	return appstorage.SaveWindowTabs(s.Deps.Repos.Windows, s.Deps.Repos.Tabs, args.WindowKey, args.Tabs)
}
