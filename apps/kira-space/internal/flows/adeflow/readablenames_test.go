package adeflow_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
)

// P584 readable file and step names. Contract ade-readable-names: tests/ui/ade-readable-names.spec.ts
// reads these.
const rnScenario = "ade-readable-names"

const rnWorkflow = "id: wf\nname: Flow\nsteps:\n" +
	"  - step: new-step\n" +
	"  - id: check\n    name: Check\n    kind: agent\n    runs_on: each repo\n    timeout: 1m\n    prompt: check\n" +
	"    results:\n      - id: done\n        ok: true\n      - id: failed\n        ok: false\n        next: new-step\n"

func readText(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func absent(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("%s still exists (%v)", path, err)
	}
}

func TestStepRenameFollowsName(t *testing.T) {
	app := flowharness.New(t)
	steps, flows := app.ConfigDir("steps"), app.ConfigDir("workflows")

	first, err := app.W.AdeTask.NewStep(ctx, adewire.NewStepArgs{Name: "New step", Kind: adewire.KindAgent})
	if err != nil || first.FileName != "new-step.yaml" {
		t.Fatalf("NewStep = %+v, %v", first, err)
	}
	second, err := app.W.AdeTask.NewStep(ctx, adewire.NewStepArgs{Name: "New step", Kind: adewire.KindAgent})
	if err != nil || second.FileName != "new-step-2.yaml" {
		t.Fatalf("second NewStep = %+v, %v", second, err)
	}
	saveWorkflow(t, app, "wf", rnWorkflow)
	before, err := app.W.AdeTask.Workflows(ctx)
	if err != nil {
		t.Fatal(err)
	}
	app.Contract(t, rnScenario, "AdeTaskService.Workflows#before", before)

	st := *first.Step
	st.Name = "Lint repo"
	renamed, err := app.W.AdeTask.SaveStep(ctx, adewire.SaveStepArgs{FileName: first.FileName, Step: st, BaseHash: first.Hash})
	if err != nil || renamed.FileName != "lint-repo.yaml" || renamed.Error != nil {
		t.Fatalf("SaveStep = %+v, %v", renamed, err)
	}
	if renamed.Step == nil || renamed.Step.ID != "lint-repo" || renamed.Step.Name != "Lint repo" {
		t.Fatalf("renamed step = %+v", renamed.Step)
	}
	if len(renamed.UsedBy) != 1 || renamed.UsedBy[0] != "wf" {
		t.Fatalf("usedBy = %v, want [wf]", renamed.UsedBy)
	}
	app.Contract(t, rnScenario, "AdeTaskService.SaveStep#renamed", renamed)
	absent(t, filepath.Join(steps, "new-step.yaml"))
	wy := readText(t, filepath.Join(flows, "wf.yaml"))
	mustContain(t, "workflow yaml", wy, "step: lint-repo", "next: lint-repo")
	if strings.Contains(wy, "new-step") {
		t.Fatalf("workflow still names the old step:\n%s", wy)
	}
	entry, list := workflowEntry(t, app, "wf.yaml")
	if entry.Error != nil || entry.Workflow.Steps[0].ID != "lint-repo" || entry.Workflow.Steps[0].Use != "lint-repo" {
		t.Fatalf("workflow after rename = %+v", entry)
	}
	app.Contract(t, rnScenario, "AdeTaskService.Workflows#renamed", list)

	// A second step with the same name takes the next free stem.
	st2 := *second.Step
	st2.Name = "Lint repo"
	other, err := app.W.AdeTask.SaveStep(ctx, adewire.SaveStepArgs{FileName: second.FileName, Step: st2, BaseHash: second.Hash})
	if err != nil || other.FileName != "lint-repo-2.yaml" {
		t.Fatalf("second rename = %+v, %v", other, err)
	}

	// Same name again: the stem still matches, so the file stays.
	again := *other.Step
	again.Timeout = "5m"
	kept, err := app.W.AdeTask.SaveStep(ctx, adewire.SaveStepArgs{FileName: other.FileName, Step: again, BaseHash: other.Hash})
	if err != nil || kept.FileName != "lint-repo-2.yaml" {
		t.Fatalf("matching-stem save = %+v, %v", kept, err)
	}
}

func TestRenameRefusedOnBrokenWorkflow(t *testing.T) {
	app := flowharness.New(t)
	created, err := app.W.AdeTask.NewStep(ctx, adewire.NewStepArgs{Name: "New step", Kind: adewire.KindAgent})
	if err != nil {
		t.Fatal(err)
	}
	saveWorkflow(t, app, "wf", rnWorkflow)
	if _, err := app.W.AdeTask.SaveWorkflowYaml(ctx, adewire.SaveWorkflowYamlArgs{FileName: "wf.yaml", Yaml: strings.Replace(rnWorkflow, "steps:", "steps: [", 1)}); err != nil {
		t.Fatal(err)
	}
	st := *created.Step
	st.Name = "Lint repo"
	_, err = app.W.AdeTask.SaveStep(ctx, adewire.SaveStepArgs{FileName: created.FileName, Step: st, BaseHash: created.Hash})
	msg := badRequest(t, err)
	if !strings.Contains(msg, "workflow wf has a YAML error on line") || !strings.Contains(msg, `fix it before renaming step "new-step"`) {
		t.Fatalf("SaveStep = %q", msg)
	}
	steps := app.ConfigDir("steps")
	absent(t, filepath.Join(steps, "lint-repo.yaml"))
	if !strings.Contains(readText(t, filepath.Join(steps, "new-step.yaml")), "name: New step") {
		t.Fatal("old step file changed")
	}
}

const rnAgentFlow = "id: new-workflow\nname: New workflow\nsteps:\n" +
	"  - id: implement\n    name: Implement\n    kind: agent\n    runs_on: each repo\n    timeout: 1m\n    prompt: work on {branch} in {repo}\n"

func TestWorkflowRenameFollowsName(t *testing.T) {
	app := flowharness.New(t)
	claude(app, map[string][]fakeagent.Action{"*": {{Name: "done"}, {Name: "done"}}})
	repo, _ := cloneWithMain(app, filepath.Join(app.Work, "api"))
	rec := importRepo(t, app, repo.Dir)

	created, err := app.W.AdeTask.NewWorkflow(ctx, adewire.NewWorkflowArgs{Name: "New workflow"})
	if err != nil || created.FileName != "new-workflow.yaml" {
		t.Fatalf("NewWorkflow = %+v, %v", created, err)
	}
	saveWorkflow(t, app, "new-workflow", rnAgentFlow)
	started := createTask(t, app, "Started", "new-workflow", rec.ID)
	idle := createTask(t, app, "Idle", "new-workflow", rec.ID)
	br := branchOf(t, board(t, app), started.ID, rec.ID)
	if _, err := app.W.AdeTask.StartRun(ctx, adewire.StartRunArgs{TaskID: started.ID, BranchNames: map[string]string{br.ID: "feat/started"}}); err != nil {
		t.Fatal(err)
	}
	waitRun(t, app, started.ID, "implement", "done")

	entry, _ := workflowEntry(t, app, "new-workflow.yaml")
	wf := *entry.Workflow
	wf.Name = "Release"
	renamed, err := app.W.AdeTask.SaveWorkflow(ctx, adewire.SaveWorkflowArgs{FileName: entry.FileName, Workflow: wf, BaseHash: entry.Hash})
	if err != nil || renamed.FileName != "release.yaml" || renamed.Error != nil {
		t.Fatalf("SaveWorkflow = %+v, %v", renamed, err)
	}
	if renamed.Workflow == nil || renamed.Workflow.ID != "release" || renamed.Workflow.Name != "Release" {
		t.Fatalf("renamed workflow = %+v", renamed.Workflow)
	}
	app.Contract(t, rnScenario, "AdeTaskService.SaveWorkflow#renamed", renamed)
	absent(t, filepath.Join(app.ConfigDir("workflows"), "new-workflow.yaml"))

	b := board(t, app)
	for _, id := range []string{started.ID, idle.ID} {
		if got := taskOf(t, b, id).WorkflowID; got != "release" {
			t.Fatalf("task %s workflowId = %q, want release", id, got)
		}
	}
	if snap := taskOf(t, b, started.ID).Workflow; snap == nil || snap.ID != "new-workflow" || snap.Name != "New workflow" {
		t.Fatalf("started task's snapshot = %+v, want the workflow as it started", snap)
	}
}

func TestSharedFileKeepsStem(t *testing.T) {
	app := flowharness.New(t)
	created, err := app.W.AdeTask.NewStep(ctx, adewire.NewStepArgs{Name: "New step", Kind: adewire.KindAgent})
	if err != nil {
		t.Fatal(err)
	}
	wfCreated, err := app.W.AdeTask.NewWorkflow(ctx, adewire.NewWorkflowArgs{Name: "New workflow"})
	if err != nil {
		t.Fatal(err)
	}
	base := filepath.Join(app.ConfigDir(".sync"), "base")
	for _, f := range []string{filepath.Join("steps", created.FileName), filepath.Join("workflows", wfCreated.FileName)} {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(base, f)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(base, f), []byte("shared"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	st := *created.Step
	st.Name = "Lint repo"
	saved, err := app.W.AdeTask.SaveStep(ctx, adewire.SaveStepArgs{FileName: created.FileName, Step: st, BaseHash: created.Hash})
	if err != nil || saved.FileName != "new-step.yaml" || saved.Step == nil || saved.Step.Name != "Lint repo" {
		t.Fatalf("shared step save = %+v, %v", saved, err)
	}
	wf := *wfCreated.Workflow
	wf.Name = "Release"
	savedWf, err := app.W.AdeTask.SaveWorkflow(ctx, adewire.SaveWorkflowArgs{FileName: wfCreated.FileName, Workflow: wf, BaseHash: wfCreated.Hash})
	if err != nil || savedWf.FileName != "new-workflow.yaml" || savedWf.Workflow == nil || savedWf.Workflow.Name != "Release" {
		t.Fatalf("shared workflow save = %+v, %v", savedWf, err)
	}
}

func TestReadableNamesMigration(t *testing.T) {
	app := flowharness.New(t)
	claude(app, map[string][]fakeagent.Action{"*": {{Name: "done"}}})
	repo, _ := cloneWithMain(app, filepath.Join(app.Work, "api"))
	rec := importRepo(t, app, repo.Dir)
	steps, flows := app.ConfigDir("steps"), app.ConfigDir("workflows")
	for _, d := range []string{steps, flows} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	const (
		build  = "stp_8h3k2m9qz7ab"
		verify = "stp_7c4d5e6f8g9h"
	)
	legacy := "id: wf-x\nname: Deploy\nsteps:\n" +
		"  - step: new-step-2\n" +
		"  - id: " + build + "\n    name: Build\n    kind: agent\n    runs_on: each repo\n    timeout: 1m\n    prompt: build\n" +
		"  - id: " + verify + "\n    name: Verify\n    kind: agent\n    runs_on: each repo\n    timeout: 1m\n    prompt: verify\n" +
		"    results:\n      - id: done\n        ok: true\n      - id: failed\n        ok: false\n        next: " + build + "\n"
	libText := "id: new-step-2\nname: Lint\nkind: manual\n"
	if err := os.WriteFile(filepath.Join(steps, "new-step-2.yaml"), []byte(libText), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(flows, "wf-x.yaml"), []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}
	task := createTask(t, app, "Legacy", "wf-x", rec.ID)

	marker := filepath.Join(app.SpaceHome, "ade", "names-v1.done")
	if err := os.Remove(marker); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	app.Restart()

	deploy := readText(t, filepath.Join(flows, "deploy.yaml"))
	mustContain(t, "deploy.yaml", deploy, "id: deploy", "step: lint", "id: build", "id: verify", "next: build")
	if strings.Contains(deploy, "stp_") {
		t.Fatalf("legacy ids left:\n%s", deploy)
	}
	absent(t, filepath.Join(flows, "wf-x.yaml"))
	mustContain(t, "lint.yaml", readText(t, filepath.Join(steps, "lint.yaml")), "id: lint", "name: Lint")
	absent(t, filepath.Join(steps, "new-step-2.yaml"))
	if got := taskOf(t, board(t, app), task.ID).WorkflowID; got != "deploy" {
		t.Fatalf("task workflowId = %q, want deploy", got)
	}
	if e, ok := fileEntry(t, app, "deploy.yaml"); !ok || e.Error != nil {
		t.Fatalf("deploy.yaml entry = %+v, %v", e, ok)
	}
	backups, _ := filepath.Glob(filepath.Join(app.SpaceHome, "backups", "readable-names-*"))
	if len(backups) != 1 {
		t.Fatalf("backup dirs = %v, want one", backups)
	}
	if readText(t, filepath.Join(backups[0], "workflows", "wf-x.yaml")) != legacy ||
		readText(t, filepath.Join(backups[0], "steps", "new-step-2.yaml")) != libText {
		t.Fatal("backup does not hold the original files")
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("marker: %v", err)
	}

	app.Restart()
	if readText(t, filepath.Join(flows, "deploy.yaml")) != deploy {
		t.Fatal("second start changed deploy.yaml")
	}
	again, _ := filepath.Glob(filepath.Join(app.SpaceHome, "backups", "readable-names-*"))
	if len(again) != 1 {
		t.Fatalf("second start made backups: %v", again)
	}
}
