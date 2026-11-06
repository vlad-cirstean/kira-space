package bridge

import (
	"slices"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/appcore"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
)

type LayoutService struct {
	Deps appcore.StudioDeps
}

func (s *LayoutService) GetAll() (model.Layout, error) {
	return ipcerr.InternalResult(s.Deps.Repos.Layout.GetAll())
}

type LayoutSetArgs struct {
	Patch model.LayoutPatch `json:"patch"`
}

// Set merges and persists the patch, then broadcasts the merged layout unconditionally —
// mirroring SettingsService.Set (settings.go, events.go's ChannelSettingsChanged). Panel layout
// is app-wide by design (a preference like font size, not a per-window fact), and
// "app-wide" is only true if every window agrees — before this, window A resizing the
// project panel left window B showing the old width until relaunch.
func (s *LayoutService) Set(args LayoutSetArgs) (model.Layout, error) {
	for mode := range args.Patch.Modes {
		if !slices.Contains(model.WindowModes.Valid, mode) {
			return model.Layout{}, ipcerr.Contract("layout: modes: unknown mode %q", mode)
		}
	}
	merged, err := s.Deps.Repos.Layout.Set(args.Patch)
	if err != nil {
		return model.Layout{}, ipcerr.InternalErr(err)
	}
	s.Deps.Events.Emit(ChannelLayoutChanged, merged)
	return merged, nil
}
