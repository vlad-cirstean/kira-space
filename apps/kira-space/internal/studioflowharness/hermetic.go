package studioflowharness

import (
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
	"github.com/kirathecat/kira-studio/internal/appsettings"
)

// SetHermetic sets agent.hermetic through SettingsService.Set, the way the UI does.
func (a *App) SetHermetic(t testing.TB, on bool) {
	t.Helper()
	patch := model.SettingsPatch{Agent: &appsettings.AgentPatch{Hermetic: &on}}
	if _, err := a.W.StudioSettings.Set(bridge.StudioSettingsSetArgs{Patch: patch}); err != nil {
		t.Fatalf("set agent.hermetic: %v", err)
	}
}
