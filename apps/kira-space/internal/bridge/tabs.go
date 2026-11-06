package bridge

import (
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/appcore"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/appstorage"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
)

// TabsService persists a window's tabs in the one `tabs` table.
type TabsService struct {
	Deps appcore.Deps
}

type TabsListArgs struct {
	WindowKey string `json:"windowKey"`
}

// List returns the window's tabs ordered by their order field.
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

// Save replaces the window's tab set.
func (s *TabsService) Save(args TabsSaveArgs) error {
	if args.WindowKey == "" {
		return ipcerr.Required("windowKey")
	}
	return appstorage.SaveWindowTabs(s.Deps.Repos.Windows, s.Deps.Repos.Tabs, args.WindowKey, args.Tabs)
}
