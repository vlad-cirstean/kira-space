package journeyflow_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
	"github.com/kirathecat/kira-studio/internal/memory"
	"github.com/kirathecat/kira-studio/internal/scripts"
	"github.com/kirathecat/kira-studio/internal/scripts/scriptstest"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// A run in flight when the app quits reads as no longer running after the relaunch, leaves no
// worktree lock behind, and retries into the same worktree.
func TestRestartMidRun(t *testing.T) {
	hold := filepath.Join(t.TempDir(), "hold")
	app := flowharness.New(t)
	claude(app, fakeagent.Action{WaitFile: hold}, fakeagent.Action{Name: "done"})
	repo, _ := cloneWithMain(app, filepath.Join(app.Work, "api"))
	rec := importRepo(t, app, repo.Dir)
	saveWorkflow(t, app, workflowYAML)
	task, err := app.W.AdeTask.CreateTask(ctx, adewire.CreateTaskArgs{Title: "Fix login", CodeRepoIDs: []string{rec.ID}, WorkflowID: "flow"})
	if err != nil {
		t.Fatal(err)
	}
	br := branchOf(t, board(t, app), task.ID, rec.ID)
	if _, err := app.W.AdeTask.StartRun(ctx, adewire.StartRunArgs{TaskID: task.ID, BranchNames: map[string]string{br.ID: "feat/api-restart"}}); err != nil {
		t.Fatal(err)
	}
	waitRun(t, app, task.ID, "one", "running")
	br = branchOf(t, board(t, app), task.ID, rec.ID)
	testx.WaitUntil(t, waitFor, func() bool { return exists(filepath.Join(app.FakeDir, "api-1.args")) })

	app.Restart()
	if err := os.WriteFile(hold, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	var after adewire.Run
	for _, tk := range board(t, app).Tasks {
		for _, r := range tk.Runs {
			after = r
		}
	}
	if after.ID == "" || after.State == "running" || after.State == "done" {
		t.Fatalf("run after restart = %+v, want failed or stopped", after)
	}
	sessions, err := app.W.AdeTask.Sessions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range sessions.Sessions {
		if s.State == "running" {
			t.Fatalf("session %+v still running after restart", s)
		}
	}
	if list := gitOut(t, repo.Dir, "worktree", "list", "--porcelain"); !strings.Contains(list, br.Worktree) || strings.Contains(list, "locked") {
		t.Fatalf("worktree list after restart:\n%s\nwant %s present and unlocked", list, br.Worktree)
	}
	if err := app.W.AdeTask.RetryRun(ctx, adewire.RunArgs{RunID: after.ID}); err != nil {
		t.Fatalf("RetryRun after restart: %v", err)
	}
	if retried := waitRun(t, app, task.ID, "one", "done"); retried.Attempt <= after.Attempt {
		t.Fatalf("retry reused attempt %d, was %d", retried.Attempt, after.Attempt)
	}
	if got := gitOut(t, br.Worktree, "rev-parse", "--abbrev-ref", "HEAD"); got != "feat/api-restart" {
		t.Fatalf("worktree branch = %q", got)
	}
}

// Everything a user keeps outside a task survives a relaunch: command collections, memories and
// the workflow file (saved as structure, read back as text).
func TestRestartKeepsUserData(t *testing.T) {
	app := flowharness.New(t)
	gate, err := filepath.Abs(filepath.Join("testdata", "gate-accept.json"))
	if err != nil {
		t.Fatal(err)
	}
	claude(app, fakeagent.Action{Emit: gate})

	cs := app.W.CustomScripts
	script, err := cs.Create(bridge.CustomScriptsCreateArgs{Fields: scripts.CustomScriptFields{Name: "build", Command: "make", Color: "blue", Collection: "Build"}})
	if err != nil {
		t.Fatal(err)
	}
	scriptstest.Update(t, cs, script.ID, scripts.CustomScriptFields{
		Name: "build all", Command: "make all", Color: "green", Collection: "Deploy",
	})
	extra, err := cs.Create(bridge.CustomScriptsCreateArgs{Fields: scripts.CustomScriptFields{Name: "temp", Command: "true", Color: "blue"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := cs.Delete(bridge.CustomScriptsDeleteArgs{ID: extra.ID}); err != nil {
		t.Fatal(err)
	}

	items := []bridge.MemoryStoreItem{
		{Fact: "billing runs postgres", Reason: "migration notes"},
		{Fact: "staging deploys at 09:00", Reason: "release calendar"},
	}
	if res, err := app.W.Memory.Store(ctx, bridge.MemoryStoreArgs{Items: items, Scope: memory.ScopePersonal}); err != nil || res.Status != "stored" {
		t.Fatalf("Store = %+v, %v", res, err)
	}

	saveWorkflow(t, app, workflowYAML)
	list, err := app.W.AdeTask.Workflows(ctx)
	if err != nil || len(list.Workflows) == 0 {
		t.Fatalf("Workflows = %+v, %v", list, err)
	}
	var wf adewire.Workflow
	for _, e := range list.Workflows {
		if e.Workflow != nil && e.Workflow.ID == "flow" {
			wf = *e.Workflow
		}
	}
	wf.Name = "Renamed flow"
	wf.Steps[1].Name = "Peer review"
	if entry, err := app.W.AdeTask.SaveWorkflow(ctx, adewire.SaveWorkflowArgs{FileName: "flow.yaml", Workflow: wf}); err != nil || entry.Error != nil {
		t.Fatalf("SaveWorkflow = %+v, %v", entry, err)
	}
	yaml, err := app.W.AdeTask.WorkflowYaml(ctx, adewire.FileNameArgs{FileName: "flow.yaml"})
	if err != nil || !strings.Contains(yaml.Yaml, "Renamed flow") || !strings.Contains(yaml.Yaml, "Peer review") {
		t.Fatalf("WorkflowYaml = %+v, %v, want the saved names", yaml, err)
	}
	if _, err := app.W.AdeTask.WorkflowYaml(ctx, adewire.FileNameArgs{FileName: "missing.yaml"}); err == nil {
		t.Fatal("WorkflowYaml of a missing file succeeded")
	}

	scripts1, _ := cs.List()
	recent1, _ := app.W.Memory.Recent(ctx)
	app.Restart()
	scripts2, err := app.W.CustomScripts.List()
	if err != nil || !reflect.DeepEqual(scripts1, scripts2) {
		t.Fatalf("scripts after restart = %+v, %v, want %+v", scripts2, err, scripts1)
	}
	if len(scripts2.Scripts) != 1 || scripts2.Scripts[0].Name != "build all" || scripts2.Scripts[0].Collection != "Deploy" {
		t.Fatalf("script after restart = %+v", scripts2.Scripts)
	}
	recent2, err := app.W.Memory.Recent(ctx)
	if err != nil || len(recent2) != 2 || !reflect.DeepEqual(recent1, recent2) {
		t.Fatalf("memories after restart = %d (%v), want %d equal", len(recent2), err, len(recent1))
	}
	app.Contract(t, "restart", "MemoryService.Recent#after-restart", recent2, flowharness.Mask("createdAt"))
	yaml2, err := app.W.AdeTask.WorkflowYaml(ctx, adewire.FileNameArgs{FileName: "flow.yaml"})
	if err != nil || yaml2.Yaml != yaml.Yaml {
		t.Fatalf("workflow text changed across restart: %v", err)
	}
}
