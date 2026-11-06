package adeflow_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
)

// P526 step library. Contract ade-step-library: tests/ui/ade-v2-workflow-steps.spec.ts reads these.
const slScenario = "ade-step-library"

const slWorkflow = "id: lib-flow\nname: Library flow\nsteps:\n" +
	"  - step: implement\n    prompt_extra: Also update CHANGELOG.md.\n    timeout: 2h\n    routes: { failed: end }\n"

func stepEntryOf(t *testing.T, app *flowharness.App, file string) adewire.StepEntry {
	t.Helper()
	list, err := app.W.AdeTask.Workflows(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range list.Steps {
		if e.FileName == file {
			if e.Step == nil {
				t.Fatalf("step %s invalid: %+v", file, e.Error)
			}
			return e
		}
	}
	t.Fatalf("no step %s in %+v", file, list.Steps)
	return adewire.StepEntry{}
}

func TestStepLibrary(t *testing.T) {
	app := flowharness.New(t)
	claude(app, map[string][]fakeagent.Action{"*": {{Name: "done"}, {Name: "done"}}})
	repo, _ := cloneWithMain(app, filepath.Join(app.Work, "api"))
	rec := importRepo(t, app, repo.Dir)

	created, err := app.W.AdeTask.NewStep(ctx, adewire.NewStepArgs{Name: "Implement", Kind: adewire.KindAgent})
	if err != nil || created.FileName != "implement.yaml" {
		t.Fatalf("NewStep = %+v, %v", created, err)
	}
	step := *created.Step
	step.Prompt = "Implement the task in {repo}."
	step.Timeout = "1m"
	saved, err := app.W.AdeTask.SaveStep(ctx, adewire.SaveStepArgs{FileName: "implement.yaml", Step: step, BaseHash: created.Hash})
	if err != nil || saved.Error != nil {
		t.Fatalf("SaveStep = %+v, %v", saved, err)
	}
	app.Contract(t, slScenario, "AdeTaskService.SaveStep#ok", saved)

	saveWorkflow(t, app, "lib-flow", slWorkflow)
	entry, list := workflowEntry(t, app, "lib-flow.yaml")
	got := entry.Workflow.Steps[0]
	if got.Use != "implement" || got.Timeout != "2h" || got.Prompt != step.Prompt || got.PromptExtra != "Also update CHANGELOG.md." {
		t.Fatalf("resolved step = %+v", got)
	}
	if next := got.Results[stepResultIndex(t, got, "failed")].Next; next != "end" {
		t.Fatalf("failed route = %q, want end", next)
	}
	app.Contract(t, slScenario, "AdeTaskService.Workflows#steps", list)
	steps := stepEntryOf(t, app, "implement.yaml")
	if len(steps.UsedBy) != 1 || steps.UsedBy[0] != "lib-flow" {
		t.Fatalf("usedBy = %v, want [lib-flow]", steps.UsedBy)
	}
	y, err := app.W.AdeTask.StepYaml(ctx, adewire.FileNameArgs{FileName: "implement.yaml"})
	if err != nil {
		t.Fatal(err)
	}
	app.Contract(t, slScenario, "AdeTaskService.StepYaml#implement", y)

	// A structured save writes only what differs from the library step.
	wf := *entry.Workflow
	wf.Steps = append([]adewire.WorkflowStep(nil), wf.Steps...)
	wf.Steps[0].Timeout = "3h"
	if _, err := app.W.AdeTask.SaveWorkflow(ctx, adewire.SaveWorkflowArgs{FileName: "lib-flow.yaml", Workflow: wf, BaseHash: entry.Hash}); err != nil {
		t.Fatal(err)
	}
	wy, err := app.W.AdeTask.WorkflowYaml(ctx, adewire.FileNameArgs{FileName: "lib-flow.yaml"})
	if err != nil {
		t.Fatal(err)
	}
	mustContain(t, "workflow yaml", wy.Yaml, "step: implement", "timeout: 3h", "prompt_extra: Also update CHANGELOG.md.", "failed: end")
	for _, not := range []string{"prompt: ", "runs_on", "kind:"} {
		if strings.Contains(wy.Yaml, not) {
			t.Fatalf("workflow yaml repeats the library value %q:\n%s", not, wy.Yaml)
		}
	}

	// A task started now runs the library prompt plus the workflow's extra.
	first := createTask(t, app, "First", "lib-flow", rec.ID)
	second := createTask(t, app, "Second", "lib-flow", rec.ID)
	br := branchOf(t, board(t, app), first.ID, rec.ID)
	if _, err := app.W.AdeTask.StartRun(ctx, adewire.StartRunArgs{TaskID: first.ID, BranchNames: map[string]string{br.ID: "feat/first"}}); err != nil {
		t.Fatal(err)
	}
	waitRun(t, app, first.ID, "implement", "done")
	mustContain(t, "first prompt", fakeFile(t, app, "api-1.prompt"), "Implement the task in api.", "Also update CHANGELOG.md.")

	// A library edit: the started task keeps its snapshot and reads outdated, the idle one follows.
	edited := stepEntryOf(t, app, "implement.yaml")
	step = *edited.Step
	step.Prompt = "Rewrite the task in {repo}."
	if _, err := app.W.AdeTask.SaveStep(ctx, adewire.SaveStepArgs{FileName: "implement.yaml", Step: step, BaseHash: edited.Hash}); err != nil {
		t.Fatal(err)
	}
	b := board(t, app)
	if t1 := taskOf(t, b, first.ID); !t1.WorkflowOutdated || t1.Workflow == nil || t1.Workflow.Steps[0].Prompt != "Implement the task in {repo}." {
		t.Fatalf("started task = outdated %v, workflow %+v; want outdated on its snapshot", t1.WorkflowOutdated, t1.Workflow)
	}
	if t2 := taskOf(t, b, second.ID); t2.WorkflowOutdated || t2.Workflow != nil {
		t.Fatalf("idle task = %+v, want it to follow the live workflow", t2)
	}
	br2 := branchOf(t, board(t, app), second.ID, rec.ID)
	if _, err := app.W.AdeTask.StartRun(ctx, adewire.StartRunArgs{TaskID: second.ID, BranchNames: map[string]string{br2.ID: "feat/second"}}); err != nil {
		t.Fatal(err)
	}
	waitRun(t, app, second.ID, "implement", "done")
	mustContain(t, "second prompt", fakeFile(t, app, "api-2.prompt"), "Rewrite the task in api.", "Also update CHANGELOG.md.")

	// Delete: refused while a workflow references the step, allowed after.
	_, derr := app.W.AdeTask.StepYaml(ctx, adewire.FileNameArgs{FileName: "nope.yaml"})
	if msg := badRequest(t, derr); !strings.Contains(msg, "does not exist") {
		t.Fatalf("StepYaml of a missing step = %q", msg)
	}
	used := app.W.AdeTask.DeleteStep(ctx, adewire.DeleteStepArgs{FileName: "implement.yaml"})
	msg := badRequest(t, used)
	if !strings.Contains(msg, "used by workflows: lib-flow") {
		t.Fatalf("DeleteStep = %q, want the using workflows", msg)
	}
	app.Contract(t, slScenario, "AdeTaskService.DeleteStep#used", msg)
	saveWorkflow(t, app, "lib-flow", flowYAML("lib-flow", agentStep("implement", "")))
	if err := app.W.AdeTask.DeleteStep(ctx, adewire.DeleteStepArgs{FileName: "implement.yaml"}); err != nil {
		t.Fatalf("DeleteStep after the reference is gone: %v", err)
	}
	if _, err := os.Stat(filepath.Join(app.ConfigDir("steps"), "implement.yaml")); !os.IsNotExist(err) {
		t.Fatalf("step file still there: %v", err)
	}
}

func stepResultIndex(t *testing.T, s adewire.WorkflowStep, id string) int {
	t.Helper()
	for i, r := range s.Results {
		if r.ID == id {
			return i
		}
	}
	t.Fatalf("step has no result %s", id)
	return -1
}

func TestStepLibraryYaml(t *testing.T) {
	app := flowharness.New(t)
	bad, err := app.W.AdeTask.ValidateStepYaml(ctx, adewire.ValidateStepYamlArgs{Yaml: "id: x\nname: X\nkind: agent\nruns_on: once\ntimeout: 1m\nprompt: p\nskip: true\n"})
	if err != nil || bad.Error == nil || !strings.Contains(bad.Error.Message, "skip is set in a workflow") {
		t.Fatalf("ValidateStepYaml = %+v, %v", bad, err)
	}
	if _, err := app.W.AdeTask.SaveStepYaml(ctx, adewire.SaveStepYamlArgs{FileName: "broken.yaml", Yaml: "id: broken\n"}); err != nil {
		t.Fatal(err)
	}
	list, err := app.W.AdeTask.Workflows(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, e := range list.Steps {
		if e.FileName == "broken.yaml" {
			found = true
			if e.Error == nil {
				t.Fatal("broken step has no error")
			}
		}
	}
	if !found {
		t.Fatal("broken step not listed")
	}
	app.Contract(t, slScenario, "AdeTaskService.ValidateStepYaml#skip", bad)
	app.Contract(t, slScenario, "AdeTaskService.Workflows#broken-step", list)
}

const slMigrateOne = "id: m-one\nname: M one\nsteps:\n" +
	"  - id: implement\n    name: implement\n    kind: agent\n    runs_on: each repo\n    timeout: 1m\n    prompt: first body\n" +
	"  - id: gate\n    name: Gate\n    kind: manual\n    skip: true\n"

const slMigrateTwo = "id: m-two\nname: M two\nsteps:\n" +
	"  - id: implement\n    name: implement\n    kind: agent\n    runs_on: each repo\n    timeout: 1m\n    prompt: second body\n"

func TestStepLibraryMigration(t *testing.T) {
	app := flowharness.New(t)
	claude(app, map[string][]fakeagent.Action{"*": {{Name: "done"}}})
	repo, _ := cloneWithMain(app, filepath.Join(app.Work, "api"))
	rec := importRepo(t, app, repo.Dir)

	// Plant pre-library files, as an older release left them, and start a task on one.
	wfDir := app.ConfigDir("workflows")
	files := map[string]string{"m-one.yaml": slMigrateOne, "m-two.yaml": slMigrateTwo, "m-broken.yaml": "id: m-broken\n  oops: [\n"}
	for name, src := range files {
		if err := os.WriteFile(filepath.Join(wfDir, name), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	task := createTask(t, app, "Started before", "m-one", rec.ID)
	br := branchOf(t, board(t, app), task.ID, rec.ID)
	if _, err := app.W.AdeTask.StartRun(ctx, adewire.StartRunArgs{TaskID: task.ID, BranchNames: map[string]string{br.ID: "feat/old"}}); err != nil {
		t.Fatal(err)
	}
	waitRun(t, app, task.ID, "implement", "done")
	before := map[string]adewire.Workflow{}
	for _, f := range []string{"m-one.yaml", "m-two.yaml"} {
		e, _ := workflowEntry(t, app, f)
		before[f] = *e.Workflow
	}

	// The first boot already ran the migration on an empty home; drop its marker to run it for these files.
	if err := os.RemoveAll(filepath.Join(wfDir, ".p526-backup")); err != nil {
		t.Fatal(err)
	}
	app.Restart()

	stepsDir := app.ConfigDir("steps")
	for _, f := range []string{"implement.yaml", "implement-2.yaml", "gate.yaml"} {
		if _, err := os.Stat(filepath.Join(stepsDir, f)); err != nil {
			t.Errorf("library file %s: %v", f, err)
		}
	}
	for name, src := range files {
		if bak, err := os.ReadFile(filepath.Join(wfDir, ".p526-backup", name)); err != nil || string(bak) != src {
			t.Errorf("backup of %s: %v", name, err)
		}
	}
	if raw, _ := os.ReadFile(filepath.Join(wfDir, "m-broken.yaml")); string(raw) != files["m-broken.yaml"] {
		t.Errorf("broken workflow touched: %q", raw)
	}
	one, _ := workflowEntry(t, app, "m-one.yaml")
	two, _ := workflowEntry(t, app, "m-two.yaml")
	if one.Workflow.Steps[0].Use != "implement" || two.Workflow.Steps[0].Use != "implement-2" {
		t.Fatalf("uses = %q / %q, want implement / implement-2", one.Workflow.Steps[0].Use, two.Workflow.Steps[0].Use)
	}
	for name, got := range map[string]adewire.Workflow{"m-one.yaml": *one.Workflow, "m-two.yaml": *two.Workflow} {
		want := before[name]
		for i := range got.Steps {
			got.Steps[i].Use = ""
		}
		if !sameWorkflow(want, got) {
			t.Errorf("%s resolves differently after the move:\n%+v\n%+v", name, got, want)
		}
	}
	if t1 := taskOf(t, board(t, app), task.ID); t1.WorkflowOutdated {
		t.Error("the move flagged a started task as outdated")
	}

	// A second start does not extract again.
	y, err := app.W.AdeTask.WorkflowYaml(ctx, adewire.FileNameArgs{FileName: "m-one.yaml"})
	if err != nil {
		t.Fatal(err)
	}
	app.Restart()
	if again, _ := app.W.AdeTask.WorkflowYaml(ctx, adewire.FileNameArgs{FileName: "m-one.yaml"}); again.Hash != y.Hash {
		t.Error("the migration ran twice")
	}
}

func sameWorkflow(a, b adewire.Workflow) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return bytes.Equal(x, y)
}
