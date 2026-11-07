package adeflow_test

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
)

func TestWorkflowsYaml(t *testing.T) {
	app := flowharness.New(t)
	svc := app.W.AdeTask

	fresh, err := svc.NewWorkflow(ctx, adewire.NewWorkflowArgs{Name: "Release flow"})
	if err != nil {
		t.Fatal(err)
	}
	if fresh.FileName != "release-flow.yaml" || fresh.Workflow == nil || len(fresh.Workflow.Steps) != 1 {
		t.Fatalf("NewWorkflow = %+v, want release-flow.yaml with one step", fresh)
	}

	broken := "id: broken\nname: Broken\nsteps:\n  - id: build\n    name: Build\n    kind: nonsense\n"
	val, err := svc.ValidateWorkflowYaml(ctx, adewire.ValidateWorkflowYamlArgs{Yaml: broken})
	if err != nil {
		t.Fatal(err)
	}
	if val.Error == nil || val.Error.Line != 6 || val.Workflow != nil {
		t.Fatalf("validation = %+v, want an error on line 6", val)
	}
	saved, err := svc.SaveWorkflowYaml(ctx, adewire.SaveWorkflowYamlArgs{FileName: "broken.yaml", Yaml: broken})
	if err != nil {
		t.Fatal(err)
	}
	if saved.Error == nil || saved.Workflow != nil {
		t.Fatalf("saving invalid yaml = %+v, want the file kept with its error", saved)
	}

	two := flowYAML("two-step", agentStep("one", "")+"  - id: review\n    name: Review\n    kind: manual\n    status: in-review\n")
	saveWorkflow(t, app, "two-step", two)

	imported, err := svc.ImportWorkflow(ctx, adewire.ImportWorkflowArgs{Path: mustAbs(t, filepath.Join("testdata", "imported.yaml"))})
	if err != nil {
		t.Fatal(err)
	}
	if imported.FileName != "imported.yaml" || imported.Workflow == nil || len(imported.Workflow.Steps) != 2 {
		t.Fatalf("ImportWorkflow = %+v", imported)
	}
	if _, err := svc.ImportWorkflow(ctx, adewire.ImportWorkflowArgs{Path: mustAbs(t, filepath.Join("testdata", "imported.yaml"))}); err == nil {
		t.Fatal("importing the same workflow twice succeeded")
	}

	list, err := svc.Workflows(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range list.Workflows {
		names = append(names, e.FileName)
	}
	if !slices.Equal(names, []string{"broken.yaml", "imported.yaml", "release-flow.yaml", "two-step.yaml"}) {
		t.Fatalf("workflows = %v", names)
	}

	repo := app.NewRepo("proj")
	repo.Commit("base", map[string]string{"a.txt": "a\n"})
	rec := importRepo(t, app, repo.Dir)
	task := createTask(t, app, "switch", "release-flow", rec.ID)
	if task.WorkflowID != "release-flow" || task.EntryStepID != "work" {
		t.Fatalf("task = %+v, want release-flow at step work", task)
	}
	switched, err := svc.SetTaskWorkflow(ctx, adewire.SetTaskWorkflowArgs{TaskID: task.ID, WorkflowID: "two-step"})
	if err != nil {
		t.Fatal(err)
	}
	if switched.WorkflowID != "two-step" || switched.EntryStepID != "one" {
		t.Fatalf("switched task = %+v, want two-step at its first step one", switched)
	}
	list, _ = svc.Workflows(ctx)
	used := map[string]int{}
	for _, e := range list.Workflows {
		used[e.FileName] = e.UsedBy
	}
	if used["two-step.yaml"] != 1 || used["release-flow.yaml"] != 0 {
		t.Fatalf("workflow usage = %v, want two-step 1 and release-flow 0", used)
	}
	cleared, err := svc.SetTaskWorkflow(ctx, adewire.SetTaskWorkflowArgs{TaskID: task.ID, WorkflowID: ""})
	if err != nil || cleared.WorkflowID != "" || cleared.EntryStepID != "" {
		t.Fatalf("cleared task = %+v, %v", cleared, err)
	}
	if _, err := svc.SetTaskWorkflow(ctx, adewire.SetTaskWorkflowArgs{TaskID: task.ID, WorkflowID: "missing"}); err == nil {
		t.Fatal("unknown workflow accepted")
	}
}
