package bridge

import (
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/appcore"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
	"github.com/kirathecat/kira-studio/internal/logging"
)

// SettingsService owns every setting, the database module's sections included.
type SettingsService struct {
	Deps appcore.Deps
	// OnChanged runs after every successful Set with the merged settings.
	OnChanged func(model.Settings)
}

func (s *SettingsService) GetAll() (model.Settings, error) {
	return ipcerr.InternalResult(s.Deps.Repos.Settings.GetAll())
}

type SettingsSetArgs struct {
	Patch model.SettingsPatch `json:"patch"`
}

// Set merges the patch, applies advanced.gitLogLevel immediately, re-pushes the engine's cache
// budget when cache.l2BudgetMb was patched, then broadcasts the merged settings unconditionally.
func (s *SettingsService) Set(args SettingsSetArgs) (model.Settings, error) {
	merged, err := s.Deps.Repos.Settings.Set(args.Patch)
	if err != nil {
		return model.Settings{}, ipcerr.InternalErr(err)
	}
	if args.Patch.Advanced != nil && args.Patch.Advanced.GitLogLevel != nil {
		logging.SetLevel(*args.Patch.Advanced.GitLogLevel)
	}
	if args.Patch.Cache != nil && args.Patch.Cache.L2BudgetMb != nil && s.Deps.Router != nil {
		s.Deps.Router.PushCacheConfig(merged)
	}
	s.Deps.Events.Emit(ChannelSettingsChanged, merged)
	if s.OnChanged != nil {
		s.OnChanged(merged)
	}
	return merged, nil
}
