package adeflow_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/ade"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/repocfg"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/sessionrestore"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
	"github.com/kirathecat/kira-studio/internal/harness"
	"github.com/kirathecat/kira-studio/internal/scriptruns"
	"github.com/kirathecat/kira-studio/internal/scripts"
	"github.com/kirathecat/kira-studio/internal/shell"
	"github.com/kirathecat/kira-studio/internal/terminal"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// Contract session-restore: tests/ui/session-restore.spec.ts reads these.
// The dialog offers what a restart cut: a terminal script, a held headless run and two open TUI
// sessions (one mid-turn, one idle).

var restoreMask = flowharness.Mask("hash", "token", "endedAt", "terminalId", "command")

type restoreFix struct {
	app      *flowharness.App
	repoID   string
	hold     string
	script   scripts.CustomScript
	runTask  string
	busyTerm string
	busy     adewire.Launch
	idle     adewire.Launch
}

// newRestoreFix opens, in order: a held headless run (call 1), a busy TUI session (call 2), an idle
// TUI session (call 3) and a running terminal script. Later calls are the restored agents.
func newRestoreFix(t *testing.T, opts ...flowharness.Opt) *restoreFix {
	t.Helper()
	hold := filepath.Join(t.TempDir(), "hold")
	app := flowharness.New(t, append([]flowharness.Opt{flowharness.WithTrackerGrace(time.Second)}, opts...)...)
	claude(app, map[string][]fakeagent.Action{"*": {
		{WaitFile: hold}, {WaitFile: hold}, {WaitFile: hold}, {Name: "sleep"}, {Name: "sleep"}, {Name: "sleep"}, {Name: "sleep"},
	}})
	t.Cleanup(func() { _ = os.WriteFile(hold, nil, 0o644) })
	repo, _ := cloneWithMain(app, filepath.Join(app.Work, "api"))
	rec := importRepo(t, app, repo.Dir)
	saveWorkflow(t, app, "flow", flowYAML("flow", agentStep("one", "")))
	app.WindowMgr.OpenWindow(shell.WindowRecord{Key: sessionWindow})
	f := &restoreFix{app: app, repoID: rec.ID, hold: hold}

	run := createTask(t, app, "Run work", "flow", rec.ID)
	f.runTask = run.ID
	br := branchOf(t, board(t, app), run.ID, rec.ID)
	if _, err := app.W.AdeTask.StartRun(ctx, adewire.StartRunArgs{TaskID: run.ID, BranchNames: map[string]string{br.ID: "feat/api-run"}}); err != nil {
		t.Fatal(err)
	}
	waitRun(t, app, run.ID, "one", "running")
	testx.WaitUntil(t, waitFor, func() bool { _, err := os.Stat(filepath.Join(app.FakeDir, "api-1.args")); return err == nil })

	f.busy = f.adHoc(t, "Busy work", "feat/api-busy")
	f.idle = f.adHoc(t, "Idle work", "feat/api-idle")
	f.busyTerm = f.busy.TerminalID
	app.W.Tracker.HandleEvent(harness.AgentEvent{TerminalID: f.busy.TerminalID, Event: harness.EventPrompt})
	app.W.Tracker.HandleEvent(harness.AgentEvent{TerminalID: f.idle.TerminalID, Event: harness.EventPrompt})
	app.W.Tracker.HandleEvent(harness.AgentEvent{TerminalID: f.idle.TerminalID, Event: harness.EventTurnEnd})

	f.script = newScript(t, app, scripts.CustomScriptFields{Name: "plain", Command: "sleep 60", Color: "blue"})
	if _, err := app.W.Terminal.Open(terminal.OpenArgs{
		TerminalID: "t-plain", WindowKey: sessionWindow, Cols: 80, Rows: 24, LaunchKind: terminal.LaunchKindScript, ScriptID: f.script.ID,
	}); err != nil {
		t.Fatal(err)
	}
	testx.WaitUntil(t, waitFor, func() bool {
		runs, _ := app.W.ScriptRuns.List(scriptruns.ListArgs{})
		return slices.ContainsFunc(runs, func(r scriptruns.Run) bool { return r.TerminalID == "t-plain" && r.State == "running" })
	})
	return f
}

func newScript(t *testing.T, app *flowharness.App, f scripts.CustomScriptFields) scripts.CustomScript {
	t.Helper()
	rec, err := app.W.CustomScripts.Create(bridge.CustomScriptsCreateArgs{Fields: f})
	if err != nil {
		t.Fatal(err)
	}
	return rec
}

// adHoc opens an ad hoc TUI session on a new task's branch and waits until it runs.
func (f *restoreFix) adHoc(t *testing.T, title, branch string) adewire.Launch {
	t.Helper()
	task := createTask(t, f.app, title, "flow", f.repoID)
	row := branchOf(t, board(t, f.app), task.ID, f.repoID)
	br, err := f.app.W.AdeTask.SetTaskBranch(ctx, adewire.SetTaskBranchArgs{BranchID: row.ID, Name: branch})
	if err != nil {
		t.Fatal(err)
	}
	l, err := f.app.W.AdeTask.StartAdHoc(ctx, adewire.StartAdHocArgs{BranchID: br.ID})
	if err != nil {
		t.Fatal(err)
	}
	openLaunch(t, f.app, l)
	waitSession(t, f.app, l.SessionID, "running")
	return l
}

func (f *restoreFix) state(t *testing.T) sessionrestore.State {
	t.Helper()
	st, err := f.app.W.SessionRestore.State(ctx)
	if err != nil {
		t.Fatalf("State: %v", err)
	}
	return st
}

// agent finds the offered agent of a session row, or of a run when sessionID is "".
func agentOf(t *testing.T, st sessionrestore.State, sessionID string) ade.AgentItem {
	t.Helper()
	for _, a := range st.Agents {
		if sessionID == "" && a.Kind == ade.AgentKindRun || sessionID != "" && a.SessionID == sessionID && a.Kind == ade.AgentKindSession {
			return a
		}
	}
	t.Fatalf("no agent for session %q in %+v", sessionID, st.Agents)
	return ade.AgentItem{}
}

func (f *restoreFix) restart(t *testing.T) {
	t.Helper()
	f.app.Restart()
	f.app.WindowMgr.OpenWindow(shell.WindowRecord{Key: sessionWindow})
	waitRun(t, f.app, f.runTask, "one", "stuck")
}

// continueLineArgs counts the fake claude calls whose argv carries the continue line.
func continueLineArgs(t *testing.T, app *flowharness.App) int {
	t.Helper()
	files, _ := filepath.Glob(filepath.Join(app.FakeDir, "*.args"))
	n := 0
	for _, p := range files {
		raw, err := os.ReadFile(p)
		if err == nil && strings.Contains(string(raw), "Kira restarted: continue where you left off.") {
			n++
		}
	}
	return n
}

func TestSessionRestoreAfterQuit(t *testing.T) {
	f := newRestoreFix(t)
	f.restart(t)

	st := f.state(t)
	if !st.Open || len(st.Scripts) != 1 || st.Docker.State != "none" || len(st.Agents) != 3 {
		t.Fatalf("State = %+v, want open with 1 script, no Docker and 3 agents", st)
	}
	run, busy, idle := agentOf(t, st, ""), agentOf(t, st, f.busy.SessionID), agentOf(t, st, f.idle.SessionID)
	if !run.ContinueDefault || !busy.ContinueDefault || idle.ContinueDefault || !busy.Working || idle.Working {
		t.Fatalf("continue defaults: run %+v busy %+v idle %+v, want true, true, false", run, busy, idle)
	}
	if run.Mode != ade.AgentModeHeadless || busy.Mode != ade.AgentModeTUI || run.State != ade.AgentReady || run.Continue != ade.AgentContinueOptional {
		t.Fatalf("agents = %+v", st.Agents)
	}
	f.app.Contract(t, "session-restore", "SessionRestoreService.State#afterQuit", st, restoreMask)

	args := sessionrestore.Args{
		Scripts: []scriptruns.RestoreChoice{{RunID: st.Scripts[0].RunID, Hash: st.Scripts[0].Hash}},
		Agents:  []ade.AgentChoice{{ID: run.ID}, {ID: busy.ID, Continue: true}, {ID: idle.ID}},
	}
	f.app.Contract(t, "session-restore", "args:SessionRestoreService.Restore#mixed", args, restoreMask)
	res, err := f.app.W.SessionRestore.Restore(ctx, args)
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	f.app.Contract(t, "session-restore", "SessionRestoreService.Restore#mixed", res, restoreMask)
	if len(res.Scripts) != 1 || res.Scripts[0].Started == nil || res.Scripts[0].Started.Terminal == nil {
		t.Fatalf("script outcome = %+v, want a terminal launch", res.Scripts)
	}
	if len(res.Agents) != 3 {
		t.Fatalf("agent outcomes = %+v", res.Agents)
	}
	for _, a := range res.Agents {
		if a.Error != "" || a.Launch == nil {
			t.Fatalf("agent outcome %+v, want a TUI launch", a)
		}
	}
	if res.Agents[0].RunID == "" || res.Agents[1].RunID != "" {
		t.Fatalf("run ids = %+v, want one on the run's outcome only", res.Agents)
	}
	for _, a := range res.Agents {
		openLaunch(t, f.app, *a.Launch)
	}
	testx.WaitUntil(t, waitFor, func() bool { return continueLineArgs(t, f.app) == 1 })
	time.Sleep(500 * time.Millisecond)
	if n := continueLineArgs(t, f.app); n != 1 {
		t.Fatalf("%d sessions got the continue line, want only the busy one", n)
	}

	for _, r := range taskOf(t, board(t, f.app), f.runTask).Runs {
		if r.Resumable {
			t.Fatalf("run %+v still resumable after its conversation opened in a terminal", r)
		}
	}
	if left := f.state(t); len(left.Agents) != 0 {
		t.Fatalf("agents left in the offer = %+v, want none", left.Agents)
	}
}

func TestSessionRestoreAfterCrash(t *testing.T) {
	f := newRestoreFix(t)
	snap := f.app.Snapshot()
	f.app.CrashTo(snap)
	f.app.WindowMgr.OpenWindow(shell.WindowRecord{Key: sessionWindow})
	waitRun(t, f.app, f.runTask, "one", "stuck")

	st := f.state(t)
	if len(st.Agents) != 3 {
		t.Fatalf("State = %+v, want 3 agents", st)
	}
	run, busy, idle := agentOf(t, st, ""), agentOf(t, st, f.busy.SessionID), agentOf(t, st, f.idle.SessionID)
	if !run.ContinueDefault || busy.ContinueDefault || idle.ContinueDefault || busy.Working {
		t.Fatalf("after a crash: run %+v busy %+v idle %+v, want Continue off for both TUI rows", run, busy, idle)
	}
	f.app.Contract(t, "session-restore", "SessionRestoreService.State#afterCrash", st, restoreMask)

	res, err := f.app.W.SessionRestore.Restore(ctx, sessionrestore.Args{Agents: []ade.AgentChoice{{ID: run.ID, Continue: true}}})
	if err != nil || len(res.Agents) != 1 || res.Agents[0].Error != "" || res.Agents[0].Launch != nil {
		t.Fatalf("Restore = %+v, %v, want the run queued in the background", res, err)
	}
	next := waitRun(t, f.app, f.runTask, "one", "running")
	if next.Attempt != 2 {
		t.Fatalf("resumed run = %+v, want attempt 2", next)
	}
	testx.WaitUntil(t, waitFor, func() bool {
		files, _ := filepath.Glob(filepath.Join(f.app.FakeDir, "api-*.prompt"))
		for _, p := range files {
			raw, _ := os.ReadFile(p)
			if strings.Contains(string(raw), "closed while you worked") {
				return true
			}
		}
		return false
	})
}

func TestSessionRestorePartial(t *testing.T) {
	f := newRestoreFix(t)
	f.restart(t)
	st := f.state(t)
	if err := f.app.W.CustomScripts.Delete(scripts.DeleteArgs{ID: f.script.ID}); err != nil {
		t.Fatal(err)
	}
	busy := agentOf(t, st, f.busy.SessionID)
	args := sessionrestore.Args{
		Scripts: []scriptruns.RestoreChoice{{RunID: st.Scripts[0].RunID, Hash: st.Scripts[0].Hash}},
		Agents:  []ade.AgentChoice{{ID: busy.ID}, {ID: "session:missing"}},
	}
	res, err := f.app.W.SessionRestore.Restore(ctx, args)
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	f.app.Contract(t, "session-restore", "SessionRestoreService.Restore#partial", res, restoreMask)
	if len(res.Scripts) != 1 || res.Scripts[0].Started != nil || res.Scripts[0].Error == "" {
		t.Fatalf("script outcome = %+v, want its own error", res.Scripts)
	}
	if len(res.Agents) != 2 || res.Agents[0].Launch == nil || res.Agents[0].Error != "" || res.Agents[1].Error == "" {
		t.Fatalf("agent outcomes = %+v, want the busy session restored and the unknown id refused", res.Agents)
	}
}

func TestSessionRestoreClaimAndClose(t *testing.T) {
	f := newRestoreFix(t, flowharness.WithSessionRestoreWindow(4*time.Second))
	f.restart(t)
	if !f.state(t).Open {
		t.Fatal("State.open = false right after start")
	}
	first, err := f.app.W.SessionRestore.Claim(ctx)
	second, err2 := f.app.W.SessionRestore.Claim(ctx)
	if err != nil || err2 != nil || !first || second {
		t.Fatalf("Claim = %v, %v (errors %v, %v), want true then false", first, second, err, err2)
	}
	if f.state(t).Open {
		t.Fatal("State.open stays true after a claim")
	}
	if err := f.app.W.SessionRestore.Close(ctx); err != nil {
		t.Fatal(err)
	}
	st := f.state(t)
	if len(st.Agents) != 0 || len(st.Scripts) != 1 {
		t.Fatalf("after Close: %d agents, %d scripts, want the agent offer gone and the script offer kept", len(st.Agents), len(st.Scripts))
	}
	runs, err := f.app.W.ScriptRuns.RestoreState()
	if err != nil || len(runs.Items) != 1 {
		t.Fatalf("ScriptRuns.RestoreState = %+v, %v, want the banner's offer intact", runs, err)
	}
	// The board still resumes the run on its own.
	if err := f.app.W.AdeTask.ResumeRun(ctx, adewire.RunArgs{RunID: stuckRunID(t, f)}); err != nil {
		t.Fatalf("ResumeRun after Close: %v", err)
	}

	// A start older than the window offers nothing, even to a window that never claimed.
	f.app.Restart()
	time.Sleep(4200 * time.Millisecond)
	if open := f.state(t).Open; open {
		t.Fatal("State.open = true after the offer window")
	}
	if ok, _ := f.app.W.SessionRestore.Claim(ctx); ok {
		t.Fatal("Claim after the offer window = true")
	}
}

func stuckRunID(t *testing.T, f *restoreFix) string {
	t.Helper()
	for _, r := range taskOf(t, board(t, f.app), f.runTask).Runs {
		if r.State == "stuck" && r.Resumable {
			return r.ID
		}
	}
	t.Fatal("no resumable stuck run")
	return ""
}

func TestSessionRestoreServicesGate(t *testing.T) {
	f := newRestoreFix(t)
	setRepoFile(t, f.app, f.repoID, func(rf *repocfg.File) {
		rf.Services = []model.ScriptRef{{Script: "plain"}}
	})
	f.restart(t)
	taskID := f.runTask
	services, err := f.app.W.AdeTask.RequiredServices(ctx, adewire.TaskArgs{TaskID: taskID})
	if err != nil || len(services.Items) != 1 || services.Items[0].State != adewire.ServiceStopped {
		t.Fatalf("RequiredServices before = %+v, %v, want the script stopped", services, err)
	}
	st := f.state(t)
	res, err := f.app.W.SessionRestore.Restore(ctx, sessionrestore.Args{
		Scripts: []scriptruns.RestoreChoice{{RunID: st.Scripts[0].RunID, Hash: st.Scripts[0].Hash}},
	})
	if err != nil || len(res.Scripts) != 1 || res.Scripts[0].Started == nil || res.Scripts[0].Started.Terminal == nil {
		t.Fatalf("Restore = %+v, %v", res, err)
	}
	if _, err := f.app.W.Terminal.Open(terminal.OpenArgs{
		TerminalID: "t-plain2", WindowKey: sessionWindow, Cols: 80, Rows: 24, LaunchKind: terminal.LaunchKindScript,
		ScriptID: f.script.ID, ScriptLaunchToken: res.Scripts[0].Started.Terminal.Token,
	}); err != nil {
		t.Fatal(err)
	}
	testx.WaitUntil(t, waitFor, func() bool {
		got, err := f.app.W.AdeTask.RequiredServices(ctx, adewire.TaskArgs{TaskID: taskID})
		return err == nil && len(got.Items) == 1 && got.Items[0].State == adewire.ServiceRunning
	})
}

func TestSessionRestoreUserStopNotOffered(t *testing.T) {
	f := newRestoreFix(t)
	// The user closes the idle session and stops the held run before quitting.
	if err := f.app.W.AdeTask.CloseSession(ctx, adewire.CloseSessionArgs{SessionID: f.idle.SessionID}); err != nil {
		t.Fatal(err)
	}
	waitSession(t, f.app, f.idle.SessionID, "stopped")
	run := waitRun(t, f.app, f.runTask, "one", "running")
	if err := f.app.W.AdeTask.StopRun(ctx, adewire.RunArgs{RunID: run.ID}); err != nil {
		t.Fatal(err)
	}
	waitRun(t, f.app, f.runTask, "one", "stuck", "failed")

	f.app.Restart()
	f.app.WindowMgr.OpenWindow(shell.WindowRecord{Key: sessionWindow})
	st := f.state(t)
	if len(st.Agents) != 1 || st.Agents[0].SessionID != f.busy.SessionID {
		t.Fatalf("agents = %+v, want only the busy session still open at quit", st.Agents)
	}
}
