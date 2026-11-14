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

const authoredFlow = "id: authored\nname: Authored\nsteps:\n" +
	"  - id: plan\n    name: plan\n    kind: agent\n    runs_on: each repo\n    timeout: 1m\n    prompt: PLANMARK {branch} in {repo}\n" +
	"  - id: build\n    name: build\n    kind: agent\n    runs_on: each repo\n    timeout: 1m\n    prompt: BUILDMARK for {branch}\n" +
	"  - id: lint\n    name: lint\n    kind: script\n    runs_on: each repo\n    timeout: 1m\n    command: 'echo lint-ran > lint.out'\n" +
	"  - id: verify\n    name: verify\n    kind: manual\n    status: in-review\n    session: true\n"

// J-WF-010, J-WF-001. What a workflow author chose is what runs: prompts and script.
func TestWorkflowAuthoredChoicesReachRuns(t *testing.T) {
	app := flowharness.New(t)
	claude(app, map[string][]fakeagent.Action{"*": {{Name: "done"}, {Name: "done"}}})
	repo, _ := cloneWithMain(app, filepath.Join(app.Work, "api"))
	rec := importRepo(t, app, repo.Dir)
	saveWorkflow(t, app, "authored", authoredFlow)
	task := createTask(t, app, "Authored", "authored", rec.ID)
	br := branchOf(t, board(t, app), task.ID, rec.ID)
	start := func() {
		if _, err := app.W.AdeTask.StartRun(ctx, adewire.StartRunArgs{TaskID: task.ID, BranchNames: map[string]string{br.ID: "feat/authored"}}); err != nil {
			t.Fatalf("StartRun: %v", err)
		}
	}
	start()
	waitRun(t, app, task.ID, "plan", "done")
	if p := fakeFile(t, app, "api-1.prompt"); !strings.Contains(p, "PLANMARK feat/authored in api") {
		t.Fatalf("plan prompt = %q, want the authored text with variables filled", p)
	}
	waitRun(t, app, task.ID, "build", "done")
	p := fakeFile(t, app, "api-2.prompt")
	if !strings.Contains(p, "BUILDMARK for feat/authored") {
		t.Fatalf("build prompt = %q, want the authored prompt", p)
	}
	if strings.Contains(p, "PLANMARK") {
		t.Fatalf("build prompt carries the plan step's text: %q", p)
	}
	waitRun(t, app, task.ID, "lint", "done")
	br = branchOf(t, board(t, app), task.ID, rec.ID)
	if out, err := os.ReadFile(filepath.Join(br.Worktree, "lint.out")); err != nil || strings.TrimSpace(string(out)) != "lint-ran" {
		t.Fatalf("script step output = %q, %v", out, err)
	}
}

// J-WF-010. Agreed part of the switch: same id is a no-op that keeps runs, a live run refuses the
// switch, and a switch never touches another task's runs.
func TestWorkflowSwitchKeepsOtherTasksRuns(t *testing.T) {
	app := flowharness.New(t)
	claude(app, map[string][]fakeagent.Action{"*": {{Name: "done"}}})
	repo, _ := cloneWithMain(app, filepath.Join(app.Work, "api"))
	rec := importRepo(t, app, repo.Dir)
	saveWorkflow(t, app, "one", flowYAML("one", agentStep("a", "")))
	saveWorkflow(t, app, "two", flowYAML("two", agentStep("b", "")))
	mine := createTask(t, app, "Mine", "one", rec.ID)
	other := createTask(t, app, "Other", "one", rec.ID)
	for _, id := range []string{mine.ID, other.ID} {
		br := branchOf(t, board(t, app), id, rec.ID)
		if _, err := app.W.AdeTask.StartRun(ctx, adewire.StartRunArgs{TaskID: id, BranchNames: map[string]string{br.ID: "feat/" + id[len(id)-6:]}}); err != nil {
			t.Fatal(err)
		}
		waitRun(t, app, id, "a", "done")
	}
	otherRuns := len(taskOf(t, board(t, app), other.ID).Runs)

	same, err := app.W.AdeTask.SetTaskWorkflow(ctx, adewire.SetTaskWorkflowArgs{TaskID: mine.ID, WorkflowID: "one"})
	if err != nil || len(same.Runs) != 1 {
		t.Fatalf("same-workflow switch = %+v, %v, want a no-op keeping the run", same.Runs, err)
	}
	if _, err := app.W.AdeTask.SetTaskWorkflow(ctx, adewire.SetTaskWorkflowArgs{TaskID: mine.ID, WorkflowID: "nope"}); err == nil {
		t.Fatal("switch to a missing workflow accepted")
	}
	if _, err := app.W.AdeTask.SetTaskWorkflow(ctx, adewire.SetTaskWorkflowArgs{TaskID: mine.ID, WorkflowID: "two"}); err != nil {
		t.Fatalf("switch: %v", err)
	}
	got := taskOf(t, board(t, app), mine.ID)
	if got.WorkflowID != "two" || got.EntryStepID != "b" {
		t.Fatalf("task after switch = %+v, want workflow two at step b", got)
	}
	if n := len(taskOf(t, board(t, app), other.ID).Runs); n != otherRuns || n == 0 {
		t.Fatalf("other task runs = %d, want the %d it had", n, otherRuns)
	}
}

func workflowFile(t *testing.T, app *flowharness.App, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(app.ConfigDir("workflows"), name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(raw)
}

// J-WF-011, J-WF-001. The YAML editor writes the user's text even when broken (documented), but the
// last valid workflow keeps serving and starting tasks; the structured save refuses a bad route and
// leaves the file's bytes.
func TestWorkflowSaveRefusalsKeepLastValid(t *testing.T) {
	app := flowharness.New(t)
	good := flowYAML("wf", agentStep("a", "")+agentStep("b", ""))
	saveWorkflow(t, app, "wf", good)
	if onDisk := workflowFile(t, app, "wf.yaml"); onDisk != good {
		t.Fatalf("file = %q, want the saved yaml byte for byte", onDisk)
	}
	for name, bad := range map[string]string{
		"not yaml":           "id: wf\nsteps: [unclosed\n",
		"route to missing":   flowYAML("wf", agentStep("a", "    results:\n      done: nowhere\n")),
		"duplicate step ids": flowYAML("wf", agentStep("a", "")+agentStep("a", "")),
		"empty workflow":     "id: wf\nname: Flow\nsteps:\n",
	} {
		entry, err := app.W.AdeTask.SaveWorkflowYaml(ctx, adewire.SaveWorkflowYamlArgs{FileName: "wf.yaml", Yaml: bad})
		if err != nil || entry.Error == nil {
			t.Errorf("%s: save = %+v, %v, want the error reported", name, entry, err)
		}
		served, _ := workflowEntry(t, app, "wf.yaml")
		if served.Workflow == nil || len(served.Workflow.Steps) != 2 {
			t.Errorf("%s: served workflow = %+v, want the last valid two steps", name, served.Workflow)
		}
		if _, err := app.W.AdeTask.CreateTask(ctx, adewire.CreateTaskArgs{Title: name, WorkflowID: "wf"}); err != nil {
			t.Errorf("%s: task from the last valid workflow: %v", name, err)
		}
	}
	// Restore a valid file, then try the structured save with a route to a missing step.
	saveWorkflow(t, app, "wf", good)
	entry, _ := workflowEntry(t, app, "wf.yaml")
	wf := *entry.Workflow
	wf.Steps = append([]adewire.WorkflowStep(nil), wf.Steps...)
	wf.Steps[0].Results = []adewire.StepResult{{ID: "done", OK: true, Next: "nowhere"}}
	if saved, err := app.W.AdeTask.SaveWorkflow(ctx, adewire.SaveWorkflowArgs{FileName: "wf.yaml", Workflow: wf, BaseHash: entry.Hash}); err == nil && saved.Error == nil {
		t.Fatal("structured save with a route to a missing step accepted")
	}
	if got := workflowFile(t, app, "wf.yaml"); got != good {
		t.Fatalf("file after refused structured save = %q", got)
	}
}

// J-WF-011 step 1-2. A save built on an older file hash is refused and leaves the newer file; a save
// that names the fresh hash (the confirmed overwrite) lands whole.
func TestWorkflowSaveOnStaleBaseRefused(t *testing.T) {
	app := flowharness.New(t)
	saveWorkflow(t, app, "wf", flowYAML("wf", agentStep("a", "")))
	opened, err := app.W.AdeTask.WorkflowYaml(ctx, adewire.FileNameArgs{FileName: "wf.yaml"})
	if err != nil {
		t.Fatal(err)
	}
	// Window two (or an external editor) saves first.
	second := flowYAML("wf", agentStep("a", "")+agentStep("b", ""))
	if e, err := app.W.AdeTask.SaveWorkflowYaml(ctx, adewire.SaveWorkflowYamlArgs{FileName: "wf.yaml", Yaml: second, BaseHash: opened.Hash}); err != nil || e.Error != nil {
		t.Fatalf("first save: %+v, %v", e, err)
	}
	mine := strings.Replace(flowYAML("wf", agentStep("a", "")), "work on", "do", 1)
	_, err = app.W.AdeTask.SaveWorkflowYaml(ctx, adewire.SaveWorkflowYamlArgs{FileName: "wf.yaml", Yaml: mine, BaseHash: opened.Hash})
	if errCode(err) != "E_CONFLICT" {
		t.Fatalf("stale save = %v, want E_CONFLICT", err)
	}
	if got := workflowFile(t, app, "wf.yaml"); got != second {
		t.Fatalf("file after refused stale save = %q, want the other window's save", got)
	}
	fresh, err := app.W.AdeTask.WorkflowYaml(ctx, adewire.FileNameArgs{FileName: "wf.yaml"})
	if err != nil {
		t.Fatal(err)
	}
	if e, err := app.W.AdeTask.SaveWorkflowYaml(ctx, adewire.SaveWorkflowYamlArgs{FileName: "wf.yaml", Yaml: mine, BaseHash: fresh.Hash}); err != nil || e.Error != nil {
		t.Fatalf("confirmed overwrite: %+v, %v", e, err)
	}
	if got := workflowFile(t, app, "wf.yaml"); got != mine {
		t.Fatalf("file after confirmed overwrite = %q, want mine", got)
	}
}

// J-WF-002. A task keeps the workflow it began with: the running step's snapshot survives an edit, a
// task started afterwards sees the new version.
func TestWorkflowEditWhileTaskRuns(t *testing.T) {
	app := flowharness.New(t)
	claude(app, map[string][]fakeagent.Action{"*": {{Name: "done"}}})
	repo, _ := cloneWithMain(app, filepath.Join(app.Work, "api"))
	rec := importRepo(t, app, repo.Dir)
	saveWorkflow(t, app, "wf", flowYAML("wf", agentStep("one", "")))
	old := createTask(t, app, "Old", "wf", rec.ID)
	br := branchOf(t, board(t, app), old.ID, rec.ID)
	if _, err := app.W.AdeTask.StartRun(ctx, adewire.StartRunArgs{TaskID: old.ID, BranchNames: map[string]string{br.ID: "feat/old"}}); err != nil {
		t.Fatal(err)
	}
	waitRun(t, app, old.ID, "one", "done")

	saveWorkflow(t, app, "wf", flowYAML("wf", agentStep("one", "")+agentStep("two", "")))
	fresh := createTask(t, app, "Fresh", "wf", rec.ID)

	var snapshot string
	if err := app.DB().QueryRow(`SELECT workflow_json FROM ade_tasks WHERE id = ?`, old.ID).Scan(&snapshot); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(snapshot, `"one"`) || strings.Contains(snapshot, `"two"`) {
		t.Fatalf("running task snapshot = %s, want v1 only", snapshot)
	}
	var freshSnap string
	if err := app.DB().QueryRow(`SELECT COALESCE(workflow_json, '') FROM ade_tasks WHERE id = ?`, fresh.ID).Scan(&freshSnap); err != nil {
		t.Fatal(err)
	}
	if freshSnap != "" && !strings.Contains(freshSnap, `"two"`) {
		t.Fatalf("new task snapshot = %s, want v2", freshSnap)
	}
}
