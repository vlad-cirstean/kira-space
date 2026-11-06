package appflow_test

import (
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// Guards the one settings surface: database sections save through SettingsService, persist, and
// broadcast one settings:changed.
func TestSettingsCarryDatabaseSections(t *testing.T) {
	app := flowharness.New(t)
	mark := app.Events.Mark()
	budget, off := 128, false
	got, err := app.W.Settings.Set(bridge.SettingsSetArgs{Patch: model.SettingsPatch{
		Cache: &model.CachePatch{L2BudgetMb: &budget},
		DbMcp: &model.DbMcpPatch{FocusOnApproval: &off},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if got.Cache.L2BudgetMb != 128 || got.DbMcp.FocusOnApproval {
		t.Fatalf("merged = %+v %+v", got.Cache, got.DbMcp)
	}
	if n := len(app.Events.Since(mark, bridge.ChannelSettingsChanged)); n != 1 {
		t.Fatalf("settings:changed events = %d, want 1", n)
	}
	app.Restart()
	again, err := app.W.Settings.GetAll()
	if err != nil {
		t.Fatal(err)
	}
	if again.Cache.L2BudgetMb != 128 || again.DbMcp.FocusOnApproval {
		t.Fatalf("after restart = %+v %+v", again.Cache, again.DbMcp)
	}
}
