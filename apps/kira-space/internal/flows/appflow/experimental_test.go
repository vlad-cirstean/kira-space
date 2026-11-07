package appflow_test

import (
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

const experimentalScenario = "settings-experimental"

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
