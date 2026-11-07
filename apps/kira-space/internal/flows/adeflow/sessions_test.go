package adeflow_test

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/oplog"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
	"github.com/kirathecat/kira-studio/internal/shell"
	"github.com/kirathecat/kira-studio/internal/terminal"
	"github.com/kirathecat/kira-studio/internal/testx"
)

const sessionWindow = "w-main"

// openLaunch does what the renderer does with a Launch: open the agent terminal it describes.
func openLaunch(t *testing.T, app *flowharness.App, l adewire.Launch) {
	t.Helper()
	_, err := app.W.Terminal.Open(terminal.OpenArgs{
		TerminalID: l.TerminalID, Cwd: l.Cwd, Cols: 100, Rows: 30, WindowKey: sessionWindow,
		Command: l.Command, LaunchKind: terminal.LaunchKindAgent,
	})
	if err != nil {
		t.Fatalf("Terminal.Open %s: %v", l.Command, err)
	}
}

func sessionByID(t *testing.T, app *flowharness.App, id string) adewire.Session {
	t.Helper()
	res, err := app.W.AdeTask.Sessions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range res.Sessions {
		if s.ID == id {
			return s
		}
	}
	t.Fatalf("no session %s in %+v", id, res.Sessions)
	return adewire.Session{}
}

// A stopped state waits out the tracker's grace window after the spawn (shortened by the test).
func waitSession(t *testing.T, app *flowharness.App, id, state string) adewire.Session {
	t.Helper()
	var got adewire.Session
	wait := waitFor
	if state == "stopped" {
		wait = 10 * time.Second
	}
	testx.WaitUntil(t, wait, func() bool {
		res, _ := app.W.AdeTask.Sessions(ctx)
		for _, s := range res.Sessions {
			if s.ID == id {
				got = s
				return s.State == state
			}
		}
		return false
	})
	return got
}

func TestInteractiveSessions(t *testing.T) {
	dir := t.TempDir()
	flag := func(n string) string { return filepath.Join(dir, n) }
	app := flowharness.New(t, flowharness.WithTrackerGrace(time.Second))
	// Call 1 is the headless run; calls 2 to 4 are interactive Claude Code processes held open by a flag file.
	claude(app, map[string][]fakeagent.Action{"*": {
		{Name: "done"}, {WaitFile: flag("takeover")}, {WaitFile: flag("step")}, {Name: "sleep"},
	}})
	repo, _ := cloneWithMain(app, filepath.Join(app.Work, "api"))
	rec := importRepo(t, app, repo.Dir)
	saveWorkflow(t, app, "flow", flowYAML("flow", agentStep("one", "")+
		"  - id: work\n    name: Work\n    kind: manual\n    status: in-progress\n    session: true\n"))
	task := createTask(t, app, "Fix login", "flow", rec.ID)
	br := branchOf(t, board(t, app), task.ID, rec.ID)
	if _, err := app.W.AdeTask.StartRun(ctx, adewire.StartRunArgs{TaskID: task.ID, BranchNames: map[string]string{br.ID: "feat/api-work"}}); err != nil {
		t.Fatal(err)
	}
	run := waitRun(t, app, task.ID, "one", "done")
	headless := run.SessionID
	if headless == "" || sessionByID(t, app, headless).Mode != "headless" {
		t.Fatalf("run %+v has no headless session", run)
	}
	br = branchOf(t, board(t, app), task.ID, rec.ID)
	app.WindowMgr.OpenWindow(shell.WindowRecord{Key: sessionWindow})

	// Take over the finished run: its conversation resumes in the same worktree.
	launch, err := app.W.AdeTask.TakeOver(ctx, adewire.TakeOverArgs{SessionID: headless})
	if err != nil {
		t.Fatal(err)
	}
	if launch.Cwd != br.Worktree || !strings.Contains(launch.Command, "--resume") {
		t.Fatalf("take over launch = %+v, want a resume in %s", launch, br.Worktree)
	}
	openLaunch(t, app, launch)
	tui := waitSession(t, app, launch.SessionID, "running")
	if tui.Mode != "tui" || tui.TaskID != task.ID || tui.TerminalID != launch.TerminalID {
		t.Fatalf("taken-over session = %+v", tui)
	}
	if _, err := app.W.AdeTask.TakeOver(ctx, adewire.TakeOverArgs{SessionID: headless}); err == nil {
		t.Fatal("second TakeOver of an open conversation succeeded")
	}
	if err := os.WriteFile(flag("takeover"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	waitSession(t, app, tui.ID, "stopped")

	// The manual step opens its own session; Send reaches the agent's terminal, FocusSession its window.
	step, err := app.W.AdeTask.LaunchStep(ctx, adewire.LaunchStepArgs{TaskID: task.ID})
	if err != nil {
		t.Fatal(err)
	}
	if step.Cwd != br.Worktree {
		t.Fatalf("step launch cwd = %q, want the task worktree %q", step.Cwd, br.Worktree)
	}
	openLaunch(t, app, step)
	waitSession(t, app, step.SessionID, "running")
	if _, err := app.W.AdeTask.LaunchStep(ctx, adewire.LaunchStepArgs{TaskID: task.ID}); err == nil {
		t.Fatal("second LaunchStep while one session runs succeeded")
	}
	if err := app.W.AdeTask.Send(ctx, adewire.SendArgs{SessionID: step.SessionID, Message: "hello from the board"}); err != nil {
		t.Fatal(err)
	}
	testx.WaitUntil(t, waitFor, func() bool {
		raw, _ := os.ReadFile(filepath.Join(app.FakeDir, "api-3.prompt"))
		return strings.Contains(string(raw), "hello from the board")
	})
	if err := app.W.AdeTask.Send(ctx, adewire.SendArgs{SessionID: headless, Message: "x"}); err == nil {
		t.Fatal("Send to a headless session succeeded")
	}
	mark := app.Events.Mark()
	focused, err := app.W.AdeTask.FocusSession(ctx, adewire.FocusSessionArgs{SessionID: step.SessionID})
	if err != nil || !focused {
		t.Fatalf("FocusSession = %v, %v, want the window brought forward", focused, err)
	}
	if got := app.WindowMgr.Focused; len(got) == 0 || got[len(got)-1] != sessionWindow {
		t.Fatalf("window manager focused %v, want %s", got, sessionWindow)
	}
	ev := app.Events.WaitAfter(t, mark, adewire.ChannelOpenSession, nil, waitFor)
	var open adewire.OpenSessionEvent
	ev.Decode(t, &open)
	if ev.Window != sessionWindow || open.SessionID != step.SessionID || open.TaskID != task.ID {
		t.Fatalf("open-session event = %+v to %q", open, ev.Window)
	}
	if err := os.WriteFile(flag("step"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	waitSession(t, app, step.SessionID, "stopped")
	if focused, err := app.W.AdeTask.FocusSession(ctx, adewire.FocusSessionArgs{SessionID: step.SessionID}); err != nil || focused {
		t.Fatalf("FocusSession on a stopped session = %v, %v, want false", focused, err)
	}

	// A branch session on the worktree.
	started, err := app.W.AdeTask.StartBranch(ctx, adewire.StartBranchArgs{BranchID: br.ID})
	if err != nil {
		t.Fatal(err)
	}
	if started.Cwd != br.Worktree {
		t.Fatalf("branch launch cwd = %q, want %q", started.Cwd, br.Worktree)
	}
	openLaunch(t, app, started)
	waitSession(t, app, started.SessionID, "running")
	if _, err := app.W.AdeTask.StartBranch(ctx, adewire.StartBranchArgs{BranchID: br.ID}); err == nil {
		t.Fatal("second StartBranch while one session runs succeeded")
	}
}

func TestRestartSession(t *testing.T) {
	dir := t.TempDir()
	flag := func(n string) string { return filepath.Join(dir, n) }
	// A minute-long grace proves Stop bypasses Reconcile's wait.
	app := flowharness.New(t, flowharness.WithTrackerGrace(time.Minute))
	// Call 1 is the headless run; calls 2 and 3 are the interactive session and its restart, held open.
	claude(app, map[string][]fakeagent.Action{"*": {
		{Name: "done"}, {WaitFile: flag("first")}, {WaitFile: flag("second")}, {WaitFile: flag("bg")}, {WaitFile: flag("bg2")},
	}})
	t.Cleanup(func() {
		for _, n := range []string{"first", "second", "bg", "bg2"} {
			_ = os.WriteFile(flag(n), nil, 0o644)
		}
	})
	repo, _ := cloneWithMain(app, filepath.Join(app.Work, "api"))
	rec := importRepo(t, app, repo.Dir)
	saveWorkflow(t, app, "flow", flowYAML("flow", agentStep("one", "")))
	task := createTask(t, app, "Fix login", "flow", rec.ID)
	br := branchOf(t, board(t, app), task.ID, rec.ID)
	if _, err := app.W.AdeTask.StartRun(ctx, adewire.StartRunArgs{TaskID: task.ID, BranchNames: map[string]string{br.ID: "feat/api-work"}}); err != nil {
		t.Fatal(err)
	}
	run := waitRun(t, app, task.ID, "one", "done")
	headless := run.SessionID
	app.WindowMgr.OpenWindow(shell.WindowRecord{Key: sessionWindow})

	first, err := app.W.AdeTask.TakeOver(ctx, adewire.TakeOverArgs{SessionID: headless})
	if err != nil {
		t.Fatal(err)
	}
	openLaunch(t, app, first)
	old := waitSession(t, app, first.SessionID, "running")

	restarted, err := app.W.AdeTask.RestartSession(ctx, adewire.RestartSessionArgs{SessionID: old.ID})
	if err != nil {
		t.Fatal(err)
	}
	if restarted.SessionID != old.ID || restarted.TerminalID == old.TerminalID || restarted.Cwd != old.Cwd ||
		!strings.Contains(restarted.Command, "--resume") {
		t.Fatalf("restart launch = %+v, want the same session resumed in a new terminal (old %+v)", restarted, old)
	}
	if got := sessionByID(t, app, old.ID).State; got != "stopped" {
		t.Fatalf("session state before the reopen = %q, want stopped at once", got)
	}
	if _, err := app.W.AdeTask.RestartSession(ctx, adewire.RestartSessionArgs{SessionID: old.ID}); err == nil {
		t.Fatal("RestartSession of a stopped session succeeded")
	}
	openLaunch(t, app, restarted)
	if got := waitSession(t, app, old.ID, "running"); got.TerminalID != restarted.TerminalID {
		t.Fatalf("reopened session terminal = %q, want %q", got.TerminalID, restarted.TerminalID)
	}
	app.Contract(t, "ade-session-restart", "AdeTaskService.RestartSession", restarted,
		flowharness.Mask("terminalId", "sessionId", "command"))

	if _, err := app.W.AdeTask.RestartSession(ctx, adewire.RestartSessionArgs{SessionID: headless}); err == nil {
		t.Fatal("RestartSession of a finished headless session succeeded")
	}
	if _, err := app.W.AdeTask.RestartSession(ctx, adewire.RestartSessionArgs{SessionID: "missing"}); err == nil {
		t.Fatal("RestartSession of an unknown session succeeded")
	}

	// A running background run: stopped, then its next attempt resumes the same conversation.
	task2 := createTask(t, app, "Add search", "flow", rec.ID)
	row := branchOf(t, board(t, app), task2.ID, rec.ID)
	if _, err := app.W.AdeTask.StartRun(ctx, adewire.StartRunArgs{TaskID: task2.ID, BranchNames: map[string]string{row.ID: "feat/api-search"}}); err != nil {
		t.Fatal(err)
	}
	bg := waitRun(t, app, task2.ID, "one", "running")
	bgOld := waitSession(t, app, bg.SessionID, "running")
	testx.WaitUntil(t, waitFor, func() bool { return sessionByID(t, app, bg.SessionID).AgentSessionID != "" })
	bgOld = sessionByID(t, app, bgOld.ID)
	bgRestart, err := app.W.AdeTask.RestartSession(ctx, adewire.RestartSessionArgs{SessionID: bgOld.ID})
	if err != nil {
		t.Fatal(err)
	}
	if bgRestart.SessionID == "" || bgRestart.SessionID == bgOld.ID || bgRestart.TerminalID != "" || bgRestart.Command != "" {
		t.Fatalf("headless restart launch = %+v, want a new session and no terminal", bgRestart)
	}
	if got := sessionByID(t, app, bgOld.ID).State; got != "stopped" {
		t.Fatalf("old headless session state = %q, want stopped", got)
	}
	next := waitRun(t, app, task2.ID, "one", "running")
	if next.Attempt != bg.Attempt+1 || next.SessionID != bgRestart.SessionID {
		t.Fatalf("restarted run = %+v, want attempt %d on session %s", next, bg.Attempt+1, bgRestart.SessionID)
	}
	if got := waitSession(t, app, bgRestart.SessionID, "running"); got.Resumes != bgOld.AgentSessionID {
		t.Fatalf("restarted session resumes %q, want %q", got.Resumes, bgOld.AgentSessionID)
	}
	var prev adewire.Run
	for _, r := range taskOf(t, board(t, app), task2.ID).Runs {
		if r.Attempt == bg.Attempt {
			prev = r
		}
	}
	if prev.State != "stuck" {
		t.Fatalf("previous attempt state = %q, want stuck", prev.State)
	}
	app.Contract(t, "ade-session-restart-headless", "AdeTaskService.RestartSession", bgRestart,
		flowharness.Mask("sessionId", "cwd"))
}

func TestCloseSession(t *testing.T) {
	dir := t.TempDir()
	flag := func(n string) string { return filepath.Join(dir, n) }
	// A minute-long grace proves the close bypasses Reconcile's wait.
	app := flowharness.New(t, flowharness.WithTrackerGrace(time.Minute))
	// Call 1 is the headless run, call 2 the interactive session, call 3 its resume; all held open.
	claude(app, map[string][]fakeagent.Action{"*": {
		{WaitFile: flag("run")}, {WaitFile: flag("tui")}, {WaitFile: flag("again")},
	}})
	t.Cleanup(func() {
		for _, n := range []string{"run", "tui", "again"} {
			_ = os.WriteFile(flag(n), nil, 0o644)
		}
	})
	repo, _ := cloneWithMain(app, filepath.Join(app.Work, "api"))
	rec := importRepo(t, app, repo.Dir)
	saveWorkflow(t, app, "flow", flowYAML("flow", agentStep("one", "")))
	app.WindowMgr.OpenWindow(shell.WindowRecord{Key: sessionWindow})

	task := createTask(t, app, "Fix login", "flow", rec.ID)
	row := branchOf(t, board(t, app), task.ID, rec.ID)
	if _, err := app.W.AdeTask.StartRun(ctx, adewire.StartRunArgs{TaskID: task.ID, BranchNames: map[string]string{row.ID: "feat/api-login"}}); err != nil {
		t.Fatal(err)
	}
	run := waitRun(t, app, task.ID, "one", "running")
	waitSession(t, app, run.SessionID, "running")
	if err := app.W.AdeTask.CloseSession(ctx, adewire.CloseSessionArgs{SessionID: run.SessionID}); err != nil {
		t.Fatal(err)
	}
	waitRun(t, app, task.ID, "one", "stuck")
	if got := sessionByID(t, app, run.SessionID).State; got != "stopped" {
		t.Fatalf("closed headless session state = %q, want stopped", got)
	}

	task2 := createTask(t, app, "Add search", "flow", rec.ID)
	row2 := branchOf(t, board(t, app), task2.ID, rec.ID)
	br, err := app.W.AdeTask.SetTaskBranch(ctx, adewire.SetTaskBranchArgs{BranchID: row2.ID, Name: "feat/api-search"})
	if err != nil {
		t.Fatal(err)
	}
	launch, err := app.W.AdeTask.StartAdHoc(ctx, adewire.StartAdHocArgs{BranchID: br.ID})
	if err != nil {
		t.Fatal(err)
	}
	openLaunch(t, app, launch)
	waitSession(t, app, launch.SessionID, "running")
	if err := app.W.AdeTask.CloseSession(ctx, adewire.CloseSessionArgs{SessionID: launch.SessionID}); err != nil {
		t.Fatal(err)
	}
	if got := sessionByID(t, app, launch.SessionID).State; got != "stopped" {
		t.Fatalf("closed session state = %q, want stopped at once", got)
	}
	testx.WaitUntil(t, waitFor, func() bool {
		r, ok := findOp(opsNow(t, app), oplog.CategorySession, "stop")
		return ok && r.Trigger == oplog.TriggerUser && r.Status == oplog.StatusOK
	})
	if err := app.W.AdeTask.CloseSession(ctx, adewire.CloseSessionArgs{SessionID: launch.SessionID}); err == nil {
		t.Fatal("CloseSession of a stopped session succeeded")
	}
	if err := app.W.AdeTask.CloseSession(ctx, adewire.CloseSessionArgs{SessionID: "missing"}); err == nil {
		t.Fatal("CloseSession of an unknown session succeeded")
	}
	again, err := app.W.AdeTask.TakeOver(ctx, adewire.TakeOverArgs{SessionID: launch.SessionID})
	if err != nil {
		t.Fatalf("TakeOver of a closed session: %v", err)
	}
	openLaunch(t, app, again)
	waitSession(t, app, launch.SessionID, "running")
}

// P414 status bar agents. Contract ade-sessions-running: tests/ui/status-agents.spec.ts reads this
// list: a headless run and an interactive session running, one finished run stopped.
func TestRunningSessionsList(t *testing.T) {
	dir := t.TempDir()
	flag := func(n string) string { return filepath.Join(dir, n) }
	app := flowharness.New(t, flowharness.WithTrackerGrace(time.Second))
	// Call 1 finishes at once; call 2 (headless) and call 3 (interactive) are held open.
	claude(app, map[string][]fakeagent.Action{"*": {
		{Name: "done"}, {WaitFile: flag("run")}, {WaitFile: flag("tui")},
	}})
	t.Cleanup(func() {
		for _, n := range []string{"run", "tui"} {
			_ = os.WriteFile(flag(n), nil, 0o644)
		}
	})
	repo, _ := cloneWithMain(app, filepath.Join(app.Work, "api"))
	rec := importRepo(t, app, repo.Dir)
	saveWorkflow(t, app, "flow", flowYAML("flow", agentStep("one", "")))
	app.WindowMgr.OpenWindow(shell.WindowRecord{Key: sessionWindow})

	startRun := func(title, branch, state string) adewire.Run {
		task := createTask(t, app, title, "flow", rec.ID)
		row := branchOf(t, board(t, app), task.ID, rec.ID)
		if _, err := app.W.AdeTask.StartRun(ctx, adewire.StartRunArgs{TaskID: task.ID, BranchNames: map[string]string{row.ID: branch}}); err != nil {
			t.Fatal(err)
		}
		return waitRun(t, app, task.ID, "one", state)
	}
	finished := startRun("Fix login", "feat/api-login", "done")
	waitSession(t, app, finished.SessionID, "stopped")
	held := startRun("Add search", "feat/api-search", "running")
	waitSession(t, app, held.SessionID, "running")

	task := createTask(t, app, "Tidy docs", "flow", rec.ID)
	row := branchOf(t, board(t, app), task.ID, rec.ID)
	br, err := app.W.AdeTask.SetTaskBranch(ctx, adewire.SetTaskBranchArgs{BranchID: row.ID, Name: "feat/api-docs"})
	if err != nil {
		t.Fatal(err)
	}
	launch, err := app.W.AdeTask.StartAdHoc(ctx, adewire.StartAdHocArgs{BranchID: br.ID})
	if err != nil {
		t.Fatal(err)
	}
	openLaunch(t, app, launch)
	waitSession(t, app, launch.SessionID, "running")

	res, err := app.W.AdeTask.Sessions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var running, stopped []string
	for _, s := range res.Sessions {
		if s.State == "running" {
			running = append(running, s.Mode)
		} else {
			stopped = append(stopped, s.State)
		}
	}
	sort.Strings(running)
	if strings.Join(running, ",") != "headless,tui" || len(stopped) != 1 || stopped[0] != "stopped" {
		t.Fatalf("sessions running %v stopped %v, want headless+tui running and one stopped:\n%+v", running, stopped, res.Sessions)
	}
	app.Contract(t, "ade-sessions-running", "AdeTaskService.Sessions#mixed", res,
		flowharness.Mask("id", "agentSessionId", "terminalId", "taskId", "branchId", "stepId", "runId", "resumes", "cwd", "startedAt", "lastActiveAt"))
}

func TestStageSessionResume(t *testing.T) {
	dir := t.TempDir()
	flag := func(n string) string { return filepath.Join(dir, n) }
	app := flowharness.New(t, flowharness.WithTrackerGrace(time.Second))
	claude(app, map[string][]fakeagent.Action{"*": {{WaitFile: flag("first")}, {WaitFile: flag("again")}}})
	t.Cleanup(func() {
		for _, n := range []string{"first", "again"} {
			_ = os.WriteFile(flag(n), nil, 0o644)
		}
	})
	repo, _ := cloneWithMain(app, filepath.Join(app.Work, "api"))
	rec := importRepo(t, app, repo.Dir)
	saveWorkflow(t, app, "flow", flowYAML("flow",
		"  - id: work\n    name: Work\n    kind: manual\n    status: in-progress\n    session: true\n"))
	task := createTask(t, app, "Fix login", "flow", rec.ID)
	app.WindowMgr.OpenWindow(shell.WindowRecord{Key: sessionWindow})

	// The stage's session ends; the board offers Resume for the stopped row.
	step, err := app.W.AdeTask.LaunchStep(ctx, adewire.LaunchStepArgs{TaskID: task.ID})
	if err != nil {
		t.Fatal(err)
	}
	openLaunch(t, app, step)
	waitSession(t, app, step.SessionID, "running")
	if err := os.WriteFile(flag("first"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	waitSession(t, app, step.SessionID, "stopped")
	testx.WaitUntil(t, waitFor, func() bool { return sessionByID(t, app, step.SessionID).AgentSessionID != "" })
	row := sessionByID(t, app, step.SessionID)
	if row.Mode != "tui" || row.StepID != "work" || row.Purpose != "" {
		t.Fatalf("stopped stage session = %+v, want a tui row on step work", row)
	}
	app.Contract(t, "ade-stage-resume", "AdeTaskService.Sessions#stopped-step", row,
		flowharness.Mask("id", "agentSessionId", "terminalId", "taskId", "branchId", "runId", "resumes",
			"cwd", "startedAt", "lastActiveAt"))

	// Resume takes the stopped row over in place: the same session id, the same conversation.
	args := adewire.TakeOverArgs{SessionID: row.ID, StopIfRunning: true}
	app.Contract(t, "ade-stage-resume", "args:AdeTaskService.TakeOver", args)
	launch, err := app.W.AdeTask.TakeOver(ctx, args)
	if err != nil {
		t.Fatal(err)
	}
	if launch.SessionID != row.ID || launch.Cwd != row.Cwd || !strings.Contains(launch.Command, "--resume '"+row.AgentSessionID+"'") {
		t.Fatalf("resume launch = %+v, want a resume of %s in %s", launch, row.AgentSessionID, row.Cwd)
	}
	openLaunch(t, app, launch)
	again := waitSession(t, app, row.ID, "running")
	if again.StepID != "work" {
		t.Fatalf("resumed session step = %q, want work", again.StepID)
	}
	if err := os.WriteFile(flag("again"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
}
