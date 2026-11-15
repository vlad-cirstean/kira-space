package adeflow_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
	"github.com/kirathecat/kira-studio/internal/shell"
	"github.com/kirathecat/kira-studio/internal/terminal"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// P469 stage move sessions. Contract ade-stage-move: tests/ui/ade-v2-stage-move.spec.ts reads these.

const smScenario = "ade-stage-move"

var smMask = flowharness.Mask("id", "taskId", "branchId", "runId", "sessionId", "cwd", "agentSessionId", "startedAt",
	"endedAt", "lastActiveAt", "finishedAt", "createdAt", "updatedAt", "lastActivityAt", "terminalId")

func smSteps() string {
	step := func(id, extra string) string {
		return "  - id: " + id + "\n    name: " + id + "\n    kind: manual\n" + extra
	}
	return step("spec", "    session: true\n") + step("build", "    session: true\n") + step("ship", "")
}

func TestStageMoveSessions(t *testing.T) {
	dir := t.TempDir()
	flag := func(n string) string { return filepath.Join(dir, n) }
	names := []string{"s1", "s2", "branch", "s3", "bare"}
	app := flowharness.New(t, flowharness.WithTrackerGrace(time.Minute))
	holds := make([]fakeagent.Action, len(names))
	for i, n := range names {
		holds[i] = fakeagent.Action{WaitFile: flag(n)}
	}
	claude(app, map[string][]fakeagent.Action{"*": holds})
	t.Cleanup(func() {
		for _, n := range names {
			_ = os.WriteFile(flag(n), nil, 0o644)
		}
	})
	repo, _ := cloneWithMain(app, filepath.Join(app.Work, "api"))
	rec := importRepo(t, app, repo.Dir)
	saveWorkflow(t, app, "flow", flowYAML("flow", smSteps()))
	app.WindowMgr.OpenWindow(shell.WindowRecord{Key: sessionWindow})

	launch := func(taskID string) string {
		l, err := app.W.AdeTask.LaunchStep(ctx, adewire.LaunchStepArgs{TaskID: taskID})
		if err != nil {
			t.Fatalf("LaunchStep: %v", err)
		}
		openLaunch(t, app, l)
		waitSession(t, app, l.SessionID, "running")
		return l.SessionID
	}
	state := func(id string) string { return sessionByID(t, app, id).State }
	stepDone := func(taskID string, closeSessions bool) adewire.Task {
		task, err := app.W.AdeTask.StepDone(ctx, adewire.StepDoneArgs{TaskID: taskID, CloseSessions: closeSessions})
		if err != nil {
			t.Fatalf("StepDone: %v", err)
		}
		return task
	}

	task := createTask(t, app, "Fix login", "flow", rec.ID)
	s1 := launch(task.ID)
	if got := stepDone(task.ID, false); got.EntryStepID != "build" || state(s1) != "running" {
		t.Fatalf("keep: entry %q, s1 %q, want build and running", got.EntryStepID, state(s1))
	}

	s2 := launch(task.ID)
	sessions, err := app.W.AdeTask.Sessions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	app.Contract(t, smScenario, "AdeTaskService.Sessions#stage-open", sessions, smMask)
	if got := stepDone(task.ID, true); got.EntryStepID != "ship" || state(s1) != "stopped" || state(s2) != "stopped" {
		t.Fatalf("close: entry %q, s1 %q, s2 %q, want ship and both stopped", got.EntryStepID, state(s1), state(s2))
	}
	app.Contract(t, smScenario, "AdeTaskService.StepDone#closed", taskOf(t, board(t, app), task.ID), smMask)

	// A branch session is no stage session; a move that leaves a session-less step closes nothing.
	row := branchOf(t, board(t, app), task.ID, rec.ID)
	br, err := app.W.AdeTask.SetTaskBranch(ctx, adewire.SetTaskBranchArgs{BranchID: row.ID, Name: "feat/api-login"})
	if err != nil {
		t.Fatal(err)
	}
	adhoc, err := app.W.AdeTask.StartAdHoc(ctx, adewire.StartAdHocArgs{BranchID: br.ID})
	if err != nil {
		t.Fatal(err)
	}
	openLaunch(t, app, adhoc)
	waitSession(t, app, adhoc.SessionID, "running")
	if _, err := app.W.AdeTask.SetTaskStep(ctx, adewire.SetTaskStepArgs{TaskID: task.ID, StepID: "build", FromStepID: "ship", CloseSessions: true}); err != nil {
		t.Fatal(err)
	}
	if state(adhoc.SessionID) != "running" {
		t.Fatalf("branch session = %q after a close-sessions move, want running", state(adhoc.SessionID))
	}
	s3 := launch(task.ID)
	moved, err := app.W.AdeTask.SetTaskStep(ctx, adewire.SetTaskStepArgs{TaskID: task.ID, StepID: "done", FromStepID: "build", CloseSessions: true})
	if err != nil || moved.EntryStepID != "done" {
		t.Fatalf("SetTaskStep = %+v, %v, want entry done", moved, err)
	}
	if state(s3) != "stopped" || state(adhoc.SessionID) != "running" {
		t.Fatalf("s3 %q, branch %q, want stopped and running", state(s3), state(adhoc.SessionID))
	}

	// Task with only a draft repo: its step session runs at the repo root and closes the same way.
	bare := createTask(t, app, "Plan trip", "flow", rec.ID)
	sb := launch(bare.ID)
	if got := stepDone(bare.ID, true); got.EntryStepID != "build" || state(sb) != "stopped" {
		t.Fatalf("draft-only: entry %q, session %q, want build and stopped", got.EntryStepID, state(sb))
	}
}

// J-RUN-031 (agreed part; refusing while a run is live is pending USER Q9, see stream-b follow-ups):
// a stage move with a TUI session, a session that already ended and a draft handover. Sessions marked
// closed are closed, no run row stays pending or running, the draft is kept as the handed row.
func TestStageMoveMixedSessions(t *testing.T) {
	dir := t.TempDir()
	flag := func(n string) string { return filepath.Join(dir, n) }
	app := flowharness.New(t, flowharness.WithTrackerGrace(time.Second))
	// Call 1 is run one, 2 the stage session that drafts, 3 a branch session that ends early, 4 one that keeps running.
	claude(app, map[string][]fakeagent.Action{"*": {
		{Name: "done"},
		{WaitBefore: flag("go"), MCP: []fakeagent.MCPCall{submitCall(false)}, WaitFile: flag("s1")},
		{WaitFile: flag("gone")},
		{WaitFile: flag("live")},
	}})
	t.Cleanup(func() {
		for _, n := range []string{"s1", "gone", "live"} {
			_ = os.WriteFile(flag(n), nil, 0o644)
		}
	})
	repo, _ := cloneWithMain(app, filepath.Join(app.Work, "api"))
	rec := importRepo(t, app, repo.Dir)
	saveWorkflow(t, app, "flow", flowYAML("flow", agentStep("one", "")+
		"  - id: work\n    name: Work\n    kind: manual\n    session: true\n"+
		"  - id: ship\n    name: Ship\n    kind: manual\n"))
	app.WindowMgr.OpenWindow(shell.WindowRecord{Key: sessionWindow})
	task := createTask(t, app, "Fix login", "flow", rec.ID)
	br := branchOf(t, board(t, app), task.ID, rec.ID)
	if _, err := app.W.AdeTask.StartRun(ctx, adewire.StartRunArgs{TaskID: task.ID, BranchNames: map[string]string{br.ID: "feat/api-login"}}); err != nil {
		t.Fatal(err)
	}
	waitRun(t, app, task.ID, "one", "done")

	launch := func() adewire.Launch {
		l, err := app.W.AdeTask.LaunchStep(ctx, adewire.LaunchStepArgs{TaskID: task.ID})
		if err != nil {
			t.Fatalf("LaunchStep: %v", err)
		}
		openLaunch(t, app, l)
		waitSession(t, app, l.SessionID, "running")
		return l
	}
	s1 := launch()
	writeTaskFile(t, app, task.ID, "handovers/draft-work.md", "Drafted before the move.")
	if err := os.WriteFile(flag("go"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	testx.WaitUntil(t, waitFor, func() bool {
		return len(taskOf(t, board(t, app), task.ID).HandoverDrafts) == 1
	})
	adhoc := func() adewire.Launch {
		l, err := app.W.AdeTask.StartAdHoc(ctx, adewire.StartAdHocArgs{BranchID: br.ID})
		if err != nil {
			t.Fatal(err)
		}
		openLaunch(t, app, l)
		waitSession(t, app, l.SessionID, "running")
		return l
	}
	gone := adhoc()
	if err := app.W.Terminal.Close(terminal.CloseArgs{TerminalID: gone.TerminalID}); err != nil {
		t.Fatal(err)
	}
	waitSession(t, app, gone.SessionID, "stopped")
	running := adhoc()

	moved, err := app.W.AdeTask.SetTaskStep(ctx, adewire.SetTaskStepArgs{TaskID: task.ID, StepID: "ship", FromStepID: "work", CloseSessions: true})
	if err != nil || moved.EntryStepID != "ship" {
		t.Fatalf("SetTaskStep = %+v, %v, want entry ship", moved, err)
	}
	for id, want := range map[string]string{s1.SessionID: "stopped", gone.SessionID: "stopped", running.SessionID: "running"} {
		if got := sessionByID(t, app, id).State; got != want {
			t.Fatalf("session %s = %q after the move, want %s", id, got, want)
		}
	}
	var live int
	if err := app.DB().QueryRow(`SELECT count(*) FROM ade_runs WHERE task_id = ? AND state IN ('pending', 'running')`, task.ID).Scan(&live); err != nil || live != 0 {
		t.Fatalf("pending or running run rows after the move = %d, %v, want 0", live, err)
	}
	var handed []adewire.Handover
	for _, h := range handovers(t, app, task.ID) {
		if h.Body == "Drafted before the move." {
			handed = append(handed, h)
		}
	}
	if len(handed) != 1 || handed[0].State == "draft" || handed[0].FromStepID != "work" {
		t.Fatalf("handovers with the drafted text = %+v, want one handed row from work", handed)
	}
}

// A stage move while a headless run is live is refused and leaves the run, the entry step and the
// sessions as they were. The run then finishes normally.
func TestStageMoveRefusedWhileRunLive(t *testing.T) {
	hold := filepath.Join(t.TempDir(), "hold")
	app := flowharness.New(t)
	claude(app, map[string][]fakeagent.Action{"*": {{WaitFile: hold, Name: "done"}}})
	t.Cleanup(func() { _ = os.WriteFile(hold, nil, 0o644) })
	repo, _ := cloneWithMain(app, filepath.Join(app.Work, "api"))
	rec := importRepo(t, app, repo.Dir)
	saveWorkflow(t, app, "flow", flowYAML("flow", agentStep("one", "")+"  - id: ship\n    name: Ship\n    kind: manual\n"))
	task := createTask(t, app, "Fix login", "flow", rec.ID)
	if _, err := app.W.AdeTask.StartRun(ctx, adewire.StartRunArgs{TaskID: task.ID}); err != nil {
		t.Fatal(err)
	}
	waitRun(t, app, task.ID, "one", "running")
	_, err := app.W.AdeTask.SetTaskStep(ctx, adewire.SetTaskStepArgs{TaskID: task.ID, StepID: "ship", CloseSessions: true})
	if errCode(err) != "E_BAD_REQUEST" {
		t.Fatalf("SetTaskStep with a live run: %v, want E_BAD_REQUEST", err)
	}
	if r, _ := latestRun(t, app, task.ID, "one"); r.State != "running" {
		t.Fatalf("run after the refused move = %q, want running", r.State)
	}
	if got := taskOf(t, board(t, app), task.ID).EntryStepID; got != "one" {
		t.Fatalf("entry after the refused move = %q, want one", got)
	}
	if err := os.WriteFile(hold, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	waitRun(t, app, task.ID, "one", "done")
	if _, err := app.W.AdeTask.SetTaskStep(ctx, adewire.SetTaskStepArgs{TaskID: task.ID, StepID: "ship"}); err != nil {
		t.Fatalf("SetTaskStep after the run: %v", err)
	}
}
