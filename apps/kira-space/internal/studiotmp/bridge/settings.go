package bridge

import (
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/appcore"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
	"github.com/kirathecat/kira-studio/internal/logging"
)

type SettingsService struct {
	Deps appcore.StudioDeps
}

func (s *SettingsService) GetAll() (model.Settings, error) {
	return ipcerr.InternalResult(s.Deps.Repos.Settings.GetAll())
}

type SettingsSetArgs struct {
	Patch model.SettingsPatch `json:"patch"`
}

// Set ports src/main/ipc/settings.ts verbatim: merge, conditionally re-push the engine's cache
// budget when cache.l2BudgetMb was in the patch, then broadcast the merged settings
// unconditionally — the broadcast closes the gap settings.ts:15-18 names (a settings change made
// through any path other than the renderer's own patchSettings() wrapper would otherwise never
// reach the renderer's local settingsState).
func (s *SettingsService) Set(args SettingsSetArgs) (model.Settings, error) {
	merged, err := s.Deps.Repos.Settings.Set(args.Patch)
	if err != nil {
		return model.Settings{}, ipcerr.InternalErr(err)
	}
	if args.Patch.Cache != nil && args.Patch.Cache.L2BudgetMb != nil {
		s.Deps.Router.PushCacheConfig(merged)
	}
	// P72 §9.2: advanced.logLevel's own actual mechanism — apply the new verbosity immediately
	// rather than only on next launch.
	if args.Patch.Advanced != nil && args.Patch.Advanced.LogLevel != nil {
		logging.SetLevel(*args.Patch.Advanced.LogLevel)
	}
	s.Deps.Events.Emit(ChannelSettingsChanged, merged)
	return merged, nil
}
