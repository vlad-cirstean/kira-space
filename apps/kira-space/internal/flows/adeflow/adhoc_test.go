package adeflow_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
)

// P410 ad hoc session. Contract ade-adhoc: tests/ui/ade-v2-task-launch.spec.ts reads these.
func TestAdHocSession(t *testing.T) {
	dir := t.TempDir()
	hold := filepath.Join(dir, "hold")
	app := flowharness.New(t, flowharness.WithTrackerGrace(time.Second))
	claude(app, map[string][]fakeagent.Action{"*": {{WaitFile: hold}, {Name: "sleep"}}})
	t.Cleanup(func() { _ = os.WriteFile(hold, nil, 0o644) })
	repo, _ := cloneWithMain(app, filepath.Join(app.Work, "api"))
	rec := importRepo(t, app, repo.Dir)
	saveWorkflow(t, app, "flow", flowYAML("flow", agentStep("one", "")+manualStep))
	task := createTask(t, app, "Fix login", "flow", rec.ID)
	row := branchOf(t, board(t, app), task.ID, rec.ID)

	// An unnamed branch has nothing to open a session on.
	args := adewire.StartAdHocArgs{BranchID: row.ID}
	app.Contract(t, "ade-adhoc", "args:AdeTaskService.StartAdHoc", args)
	_, err := app.W.AdeTask.StartAdHoc(ctx, args)
	if errCode(err) != "E_BAD_REQUEST" {
		t.Fatalf("StartAdHoc on an unnamed branch = %v, want E_BAD_REQUEST", err)
	}
	app.Contract(t, "ade-adhoc", "error:AdeTaskService.StartAdHoc#not-created", err.Error())

	br, err := app.W.AdeTask.SetTaskBranch(ctx, adewire.SetTaskBranchArgs{BranchID: row.ID, Name: "feat/api-adhoc"})
	if err != nil || br.Worktree == "" {
		t.Fatalf("SetTaskBranch = %+v, %v", br, err)
	}
	args = adewire.StartAdHocArgs{BranchID: br.ID}
	launch, err := app.W.AdeTask.StartAdHoc(ctx, args)
	if err != nil {
		t.Fatal(err)
	}
	app.Contract(t, "ade-adhoc", "AdeTaskService.StartAdHoc", launch,
		flowharness.Mask("sessionId", "terminalId", "command", "cwd"))
	if launch.Cwd != br.Worktree {
		t.Fatalf("launch cwd = %q, want %q", launch.Cwd, br.Worktree)
	}
	openLaunch(t, app, launch)
	s := waitSession(t, app, launch.SessionID, "running")
	if s.Mode != "tui" || s.TaskID != task.ID || s.BranchID != br.ID || s.StepID != "" {
		t.Fatalf("session = %+v, want a tui on the task branch with no step", s)
	}
	if raw, _ := os.ReadFile(filepath.Join(app.FakeDir, "api-1.prompt")); strings.TrimSpace(string(raw)) != "" {
		t.Fatalf("ad hoc session got a first prompt:\n%s", raw)
	}

	// One session per branch.
	_, err = app.W.AdeTask.StartAdHoc(ctx, args)
	if errCode(err) != "E_BAD_REQUEST" {
		t.Fatalf("second StartAdHoc = %v, want E_BAD_REQUEST", err)
	}
	app.Contract(t, "ade-adhoc", "error:AdeTaskService.StartAdHoc#open", err.Error())
	if _, err := app.W.AdeTask.StartBranch(ctx, adewire.StartBranchArgs{BranchID: br.ID}); err == nil {
		t.Fatal("StartBranch beside an ad hoc session succeeded")
	}

	// After it stops, Take over resumes the conversation in the same worktree.
	if err := os.WriteFile(hold, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	waitSession(t, app, launch.SessionID, "stopped")
	back, err := app.W.AdeTask.TakeOver(ctx, adewire.TakeOverArgs{SessionID: launch.SessionID})
	if err != nil || back.Cwd != br.Worktree || !strings.Contains(back.Command, "--resume") {
		t.Fatalf("TakeOver = %+v, %v, want a resume in %s", back, err, br.Worktree)
	}

	// An archived task refuses.
	openLaunch(t, app, back)
	waitSession(t, app, back.SessionID, "running")
	if err := app.W.AdeTask.ArchiveTask(ctx, adewire.TaskArgs{TaskID: task.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.W.AdeTask.StartAdHoc(ctx, args); err == nil {
		t.Fatal("StartAdHoc on an archived task succeeded")
	}
}
