package adeflow_test

import (
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/appsettings"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeprovider"
)

// P431 stage settings. Contract ade-stage-settings: tests/ui/ade-v2-stage-settings.spec.ts reads these.

const ssScenario = "ade-stage-settings"

const ssBlock = "    settings:\n      claude:\n        auto_compact_window: 200000\n"

func TestStageSettingsHeadless(t *testing.T) {
	f := newRunFixture(t, []fakeagent.Action{{Name: "done"}}, agentStep("tuned", ssBlock)+agentStep("plain", ""))
	f.start(t, "feat/api-stage")
	waitRun(t, f.app, f.taskID, "tuned", "done")
	waitRun(t, f.app, f.taskID, "plain", "done")
	mustContain(t, "tuned argv", fakeFile(t, f.app, "api-1.args"), "--autocompact\n200000\n")
	mustLack(t, "plain argv", fakeFile(t, f.app, "api-2.args"), "--autocompact")
}

func TestStageSettingsManualSession(t *testing.T) {
	f := newRunFixture(t, []fakeagent.Action{{Name: "sleep"}}, manualStep+ssBlock)
	launch, err := f.app.W.AdeTask.LaunchStep(ctx, adewire.LaunchStepArgs{TaskID: f.taskID})
	if err != nil {
		t.Fatalf("LaunchStep: %v", err)
	}
	mustContain(t, "launch command", launch.Command, " '--autocompact' '200000'")
	f.app.Contract(t, ssScenario, "AdeTaskService.LaunchStep#stage-settings", launch, flowharness.Mask("terminalId", "sessionId", "command", "cwd"))
}

func TestStageSettingsEditor(t *testing.T) {
	app := flowharness.New(t)
	saveWorkflow(t, app, "flow", flowYAML("flow", agentStep("tuned", ssBlock)+agentStep("plain", "")))
	entry, list := workflowEntry(t, app, "flow.yaml")
	app.Contract(t, ssScenario, "AdeTaskService.Workflows#stage-settings", list)
	tuned := entry.Workflow.Steps[stepIndex(t, *entry.Workflow, "tuned")]
	if got := tuned.Settings["claude"]["auto_compact_window"]; got != "200000" {
		t.Fatalf("tuned settings = %+v, want 200000", tuned.Settings)
	}
	if got := entry.Workflow.Steps[stepIndex(t, *entry.Workflow, "plain")].Settings; len(got) != 0 {
		t.Fatalf("plain settings = %+v, want none", got)
	}

	save := func(wf adewire.Workflow, base string) (adewire.WorkflowEntry, adewire.SaveWorkflowArgs, error) {
		args := adewire.SaveWorkflowArgs{FileName: "flow.yaml", Workflow: wf, BaseHash: base}
		saved, err := app.W.AdeTask.SaveWorkflow(ctx, args)
		return saved, args, err
	}
	withValue := func(wf adewire.Workflow, id, v string) adewire.Workflow {
		wf.Steps = append([]adewire.WorkflowStep(nil), wf.Steps...)
		i := stepIndex(t, wf, id)
		wf.Steps[i].Settings = map[string]map[string]string{"claude": {"auto_compact_window": v}}
		if v == "" {
			wf.Steps[i].Settings = map[string]map[string]string{}
		}
		return wf
	}

	saved, args, err := save(withValue(*entry.Workflow, "plain", "300000"), entry.Hash)
	if err != nil || saved.Error != nil {
		t.Fatalf("SaveWorkflow = %+v, %v", saved, err)
	}
	app.Contract(t, ssScenario, "args:AdeTaskService.SaveWorkflow#stage-settings", args, flowharness.Mask("baseHash"))
	y, err := app.W.AdeTask.WorkflowYaml(ctx, adewire.FileNameArgs{FileName: "flow.yaml"})
	if err != nil {
		t.Fatal(err)
	}
	mustContain(t, "saved yaml", y.Yaml, "auto_compact_window: 300000")

	_, _, err = save(withValue(*saved.Workflow, "plain", "50000"), saved.Hash)
	msg := badRequest(t, err)
	mustContain(t, "range message", msg, "settings.claude.auto_compact_window must be a whole number from 100000 to 1000000")
	app.Contract(t, ssScenario, "AdeTaskService.SaveWorkflow#stage-settings-range", err.Error())

	cleared := withValue(withValue(*saved.Workflow, "plain", ""), "tuned", "")
	if _, _, err := save(cleared, saved.Hash); err != nil {
		t.Fatalf("clearing save: %v", err)
	}
	y, err = app.W.AdeTask.WorkflowYaml(ctx, adewire.FileNameArgs{FileName: "flow.yaml"})
	if err != nil {
		t.Fatal(err)
	}
	mustLack(t, "cleared yaml", y.Yaml, "settings:", "auto_compact_window")
}

func TestStageSettingsOtherProvider(t *testing.T) {
	fake := fakeprovider.New()
	f := newRunFixtureWith(t, nil, agentStep("tuned", ssBlock), flowharness.WithProvider(fake))
	id := fakeprovider.ID
	patch := model.SettingsPatch{Agent: &model.AgentPatch{AgentPatch: appsettings.AgentPatch{Provider: &id}}}
	if _, err := f.app.W.Settings.Set(bridge.SettingsSetArgs{Patch: patch}); err != nil {
		t.Fatalf("set agent.provider: %v", err)
	}
	f.start(t, "feat/api-stage")
	waitRun(t, f.app, f.taskID, "tuned", "done")
	calls := fake.Headless()
	if len(calls) != 1 || len(calls[0].Stage) != 0 {
		t.Fatalf("fake headless calls = %+v, want one without stage values", calls)
	}
}
