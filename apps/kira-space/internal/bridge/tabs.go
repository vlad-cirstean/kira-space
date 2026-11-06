package bridge

import (
	"sort"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/appcore"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	studiomodel "github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
	"github.com/kirathecat/kira-studio/internal/appstorage"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
)

// TabsService persists a window's tabs. Studio tab kinds (studiomodel.IsStudioTabKind) go to the
// Studio database, which keeps connection_id and its foreign key; every other kind goes to Space's.
type TabsService struct {
	Deps   appcore.Deps
	Studio appcore.StudioDeps
}

type TabsListArgs struct {
	WindowKey string `json:"windowKey"`
}

// List returns both stores' tabs of one window, ordered by their order field.
func (s *TabsService) List(args TabsListArgs) ([]model.TabRecord, error) {
	if args.WindowKey == "" {
		return nil, ipcerr.Required("windowKey")
	}
	out, err := appstorage.ListWindowTabs(s.Deps.Repos.Windows, s.Deps.Repos.Tabs, args.WindowKey)
	if err != nil {
		return nil, err
	}
	if err := s.Studio.Repos.Windows.EnsureExists(args.WindowKey); err != nil {
		return nil, ipcerr.InternalErr(err)
	}
	studio, err := appstorage.ListWindowTabs(s.Studio.Repos.Windows, s.Studio.Repos.Tabs, args.WindowKey)
	if err != nil {
		return nil, err
	}
	for _, t := range studio {
		out = append(out, model.TabRecord{
			ID: t.ID, ConnectionID: t.ConnectionID, Path: t.Path, Kind: t.Kind, State: t.State,
			Order: t.Order, Active: t.Active, WorkspaceID: t.WorkspaceID,
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Order < out[j].Order })
	return out, nil
}

type TabsSaveArgs struct {
	WindowKey string            `json:"windowKey"`
	Tabs      []model.TabRecord `json:"tabs"`
}

// Save replaces the window's tab set in both stores, each with its own kinds.
func (s *TabsService) Save(args TabsSaveArgs) error {
	if args.WindowKey == "" {
		return ipcerr.Required("windowKey")
	}
	space := make([]model.TabRecord, 0, len(args.Tabs))
	studio := make([]studiomodel.TabRecord, 0, len(args.Tabs))
	for _, t := range args.Tabs {
		if !studiomodel.IsStudioTabKind(t.Kind) {
			space = append(space, t)
			continue
		}
		studio = append(studio, studiomodel.TabRecord{
			ID: t.ID, ConnectionID: t.ConnectionID, Path: t.Path, Kind: t.Kind, State: t.State,
			Order: t.Order, Active: t.Active, WorkspaceID: t.WorkspaceID,
		})
	}
	if err := appstorage.SaveWindowTabs(s.Deps.Repos.Windows, s.Deps.Repos.Tabs, args.WindowKey, space); err != nil {
		return err
	}
	if err := s.Studio.Repos.Windows.EnsureExists(args.WindowKey); err != nil {
		return ipcerr.InternalErr(err)
	}
	return appstorage.SaveWindowTabs(s.Studio.Repos.Windows, s.Studio.Repos.Tabs, args.WindowKey, studio)
}
