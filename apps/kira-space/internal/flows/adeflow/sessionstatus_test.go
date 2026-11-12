package adeflow_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
	"github.com/kirathecat/kira-studio/internal/shell"
)

// hook pipes one hook payload through the real shim with the terminal's launch env, as a running
// claude does. curl returns after the listener answered; Flush waits for the hub worker to handle the event. It
// returns what the shim printed: the hook reply body, empty when the app has nothing to say.
func hook(t *testing.T, app *flowharness.App, terminalID, payload string) string {
	t.Helper()
	_, env, _ := app.W.AgentHooks.Compose("claude", terminalID, "", "claude", "")
	st := app.W.AgentHooks.Status("claude")
	if !st.Running || len(env) == 0 {
		t.Fatalf("hooks not running: %+v", st)
	}
	cmd := exec.Command(filepath.Join(filepath.Dir(st.SettingsPath), "hook"))
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdin = strings.NewReader(payload)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("hook shim: %v\n%s%s", err, out, stderr.String())
	}
	app.W.AgentHooks.Flush()
	return string(out)
}

func unseenAt(t *testing.T, app *flowharness.App, id string) int64 {
	t.Helper()
	var at int64
	if err := app.DB().QueryRow("SELECT unseen_at FROM ade_sessions WHERE id = ?", id).Scan(&at); err != nil {
		t.Fatal(err)
	}
	return at
}

var statusMask = flowharness.Mask("id", "agentSessionId", "terminalId", "taskId", "branchId", "stepId", "runId", "resumes", "cwd", "startedAt", "lastActiveAt")

// P610 session status. Contract ade-session-status: tests/ui/ade-v2-session-status.spec.ts reads
// the session list after the first Stop (background 2), after the last Stop (unseen) and after the
// session was viewed, plus the Stop event itself.
func TestSessionStatusBackgroundAndUnseen(t *testing.T) {
	dir := t.TempDir()
	flag := func(n string) string { return filepath.Join(dir, n) }
	app := flowharness.New(t, flowharness.WithTrackerGrace(time.Second))
	// Call 1 is the interactive session; call 2 a run that finishes; call 3 a run held until stopped.
	claude(app, map[string][]fakeagent.Action{"*": {
		{WaitFile: flag("tui")}, {Name: "done"}, {WaitFile: flag("run")},
	}})
	t.Cleanup(func() {
		for _, n := range []string{"tui", "run"} {
			_ = os.WriteFile(flag(n), nil, 0o644)
		}
	})
	repo, _ := cloneWithMain(app, filepath.Join(app.Work, "api"))
	rec := importRepo(t, app, repo.Dir)
	saveWorkflow(t, app, "flow", flowYAML("flow", agentStep("one", "")))
	app.WindowMgr.OpenWindow(shell.WindowRecord{Key: sessionWindow})

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

	raw, err := os.ReadFile(filepath.Join("testdata", "background-hooks.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	// send pipes fixture lines first..last (1-based, inclusive) and returns the sessions events seen.
	send := func(first, last int) (mark int) {
		mark = app.Events.Mark()
		for _, l := range lines[first-1 : last] {
			hook(t, app, launch.TerminalID, l)
		}
		return mark
	}
	changed := func(mark int) bool { return len(app.Events.Since(mark, adewire.ChannelSessions)) > 0 }

	// Lines 1 to 6: a turn that starts a background shell and a subagent; no count yet.
	send(1, 6)
	if s := sessionByID(t, app, launch.SessionID); s.Background != 0 || s.Unseen {
		t.Fatalf("mid turn: background %d unseen %v, want 0 false", s.Background, s.Unseen)
	}

	// Line 7: the turn's Stop with two background tasks.
	mark := send(7, 7)
	if !changed(mark) {
		t.Fatal("no sessions event when the background count rose to 2")
	}
	res, err := app.W.AdeTask.Sessions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if s := sessionByID(t, app, launch.SessionID); s.Background != 2 || s.Unseen {
		t.Fatalf("after first Stop: background %d unseen %v, want 2 false", s.Background, s.Unseen)
	}
	app.Contract(t, "ade-session-status", "AdeTaskService.Sessions#background", res, statusMask)
	for _, ev := range app.Events.Since(mark, bridge.ChannelAgentEvent) {
		var got struct{ Event string }
		ev.Decode(t, &got)
		if got.Event == "Stop" {
			app.ContractEvent(t, "ade-session-status", ev, flowharness.Mask("terminalId"))
		}
	}

	// Lines 8 to 10: subagent tool hooks never move the count; its SubagentStop drops one.
	mark = send(8, 9)
	if changed(mark) || sessionByID(t, app, launch.SessionID).Background != 2 {
		t.Fatal("a subagent tool hook changed the background count")
	}
	mark = send(10, 10)
	if !changed(mark) {
		t.Fatal("no sessions event when the background count fell to 1")
	}
	if s := sessionByID(t, app, launch.SessionID); s.Background != 1 || s.Unseen {
		t.Fatalf("after SubagentStop: background %d unseen %v, want 1 false", s.Background, s.Unseen)
	}

	// Lines 11 and 12: the follow-up turn's Stop still lists the shell.
	send(11, 12)
	if s := sessionByID(t, app, launch.SessionID); s.Background != 1 || s.Unseen {
		t.Fatalf("after second Stop: background %d unseen %v, want 1 false", s.Background, s.Unseen)
	}

	// Lines 13 and 14: the last Stop lists nothing: the session finished and nobody viewed it.
	mark = send(13, 14)
	if !changed(mark) {
		t.Fatal("no sessions event when the session finished")
	}
	res, err = app.W.AdeTask.Sessions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if s := sessionByID(t, app, launch.SessionID); s.Background != 0 || !s.Unseen {
		t.Fatalf("after last Stop: background %d unseen %v, want 0 true", s.Background, s.Unseen)
	}
	if unseenAt(t, app, launch.SessionID) == 0 {
		t.Fatal("unseen_at not persisted")
	}
	app.Contract(t, "ade-session-status", "AdeTaskService.Sessions#unseen", res, statusMask)

	// Viewing clears it once; a repeat changes nothing.
	mark = app.Events.Mark()
	changedRow, err := app.W.AdeTask.MarkSessionSeen(ctx, adewire.MarkSessionSeenArgs{SessionID: launch.SessionID})
	if err != nil || !changedRow {
		t.Fatalf("MarkSessionSeen = %v, %v, want true", changedRow, err)
	}
	if !changed(mark) {
		t.Fatal("no sessions event when the session was marked seen")
	}
	res, err = app.W.AdeTask.Sessions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if sessionByID(t, app, launch.SessionID).Unseen || unseenAt(t, app, launch.SessionID) != 0 {
		t.Fatal("session still unseen after MarkSessionSeen")
	}
	app.Contract(t, "ade-session-status", "AdeTaskService.Sessions#seen", res, statusMask)
	if again, err := app.W.AdeTask.MarkSessionSeen(ctx, adewire.MarkSessionSeenArgs{SessionID: launch.SessionID}); err != nil || again {
		t.Fatalf("second MarkSessionSeen = %v, %v, want false", again, err)
	}

	// Headless: a run that finishes is unseen; a run the user stops is not.
	start := func(title, branch, state string) adewire.Run {
		tk := createTask(t, app, title, "flow", rec.ID)
		r := branchOf(t, board(t, app), tk.ID, rec.ID)
		if _, err := app.W.AdeTask.StartRun(ctx, adewire.StartRunArgs{TaskID: tk.ID, BranchNames: map[string]string{r.ID: branch}}); err != nil {
			t.Fatal(err)
		}
		return waitRun(t, app, tk.ID, "one", state)
	}
	finished := start("Fix login", "feat/api-login", "done")
	if s := waitSession(t, app, finished.SessionID, "stopped"); !s.Unseen {
		t.Fatalf("finished run's session %+v, want unseen", s)
	}
	held := start("Add search", "feat/api-search", "running")
	waitSession(t, app, held.SessionID, "running")
	if err := app.W.AdeTask.StopRun(ctx, adewire.RunArgs{RunID: held.ID}); err != nil {
		t.Fatal(err)
	}
	if s := waitSession(t, app, held.SessionID, "stopped"); s.Unseen {
		t.Fatalf("user-stopped run's session %+v, want seen", s)
	}
}
