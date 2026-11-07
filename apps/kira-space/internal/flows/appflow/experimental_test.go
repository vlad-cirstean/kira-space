package appflow_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

const experimentalScenario = "settings-experimental"

func memoryFlag(t *testing.T, app *flowharness.App) bool {
	t.Helper()
	got, err := app.W.Settings.GetAll()
	if err != nil {
		t.Fatalf("GetAll: %v", err)
	}
	return got.Experimental.Memory
}

func setMemoryFlag(t *testing.T, app *flowharness.App, on bool) {
	t.Helper()
	if _, err := app.W.Settings.Set(bridge.SettingsSetArgs{Patch: model.SettingsPatch{Experimental: &model.ExperimentalPatch{Memory: &on}}}); err != nil {
		t.Fatalf("Set: %v", err)
	}
}

func TestExperimentalMemory(t *testing.T) {
	app := flowharness.New(t)
	got, err := app.W.Settings.GetAll()
	if err != nil {
		t.Fatal(err)
	}
	app.Contract(t, experimentalScenario, "SettingsService.GetAll#experimental-default", got.Experimental)
	if got.Experimental.Memory {
		t.Fatal("fresh home reads experimental.memory on")
	}

	on := true
	patch := model.SettingsPatch{Experimental: &model.ExperimentalPatch{Memory: &on}}
	app.Contract(t, experimentalScenario, "SettingsService.Set#experimental-memory", patch)
	setMemoryFlag(t, app, true)
	got, err = app.W.Settings.GetAll()
	if err != nil {
		t.Fatal(err)
	}
	app.Contract(t, experimentalScenario, "SettingsService.GetAll#experimental-on", got.Experimental)
	if !got.Experimental.Memory {
		t.Fatal("experimental.memory reads off after Set(true)")
	}
}

func TestExperimentalMemoryUpgradeSeed(t *testing.T) {
	app := flowharness.New(t)
	if err := os.MkdirAll(app.MemoryHome, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(app.MemoryHome, "memory.db"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	app.Restart()
	if !memoryFlag(t, app) {
		t.Fatal("existing memory.db does not seed experimental.memory on")
	}
	setMemoryFlag(t, app, false)
	app.Restart()
	if memoryFlag(t, app) {
		t.Fatal("explicit off does not survive restart with memory.db present")
	}
}
