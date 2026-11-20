package appflow_test

import (
	"reflect"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// Guards upgrades from v0.3.18: leaves of the removed memory module stay in the settings table and
// must neither fail the load, change a default nor be rewritten.
func TestSettingsLegacyMemoryKeys(t *testing.T) {
	app := flowharness.New(t)
	want, err := app.W.Settings.GetAll()
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range map[string]string{"memory.models": `{"gate":"sonnet"}`, "experimental.memory": "true"} {
		if _, err := app.DB().Exec(`INSERT INTO settings (key, value) VALUES (?, ?)`, k, v); err != nil {
			t.Fatal(err)
		}
	}
	got, err := app.W.Settings.GetAll()
	if err != nil {
		t.Fatalf("GetAll with legacy rows: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("legacy rows changed settings:\n got %+v\nwant %+v", got, want)
	}

	on := false
	if _, err := app.W.Settings.Set(bridge.SettingsSetArgs{Patch: model.SettingsPatch{Advanced: &model.AdvancedPatch{NotifyPrompts: &on}}}); err != nil {
		t.Fatalf("Set with legacy rows: %v", err)
	}
	var n int
	if err := app.DB().QueryRow(`SELECT COUNT(*) FROM settings WHERE key LIKE 'memory.%' OR key LIKE 'experimental.%'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("memory/experimental rows after Set = %d, want the 2 seeded", n)
	}
}
