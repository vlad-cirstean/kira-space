package termflow

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studioflowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/internal/scriptruns"
	"github.com/kirathecat/kira-studio/internal/scripts"
	"github.com/kirathecat/kira-studio/internal/terminal"
	"github.com/kirathecat/kira-studio/internal/testx"
)

func newScript(t *testing.T, app *studioflowharness.App, f scripts.CustomScriptFields) scripts.CustomScript {
	t.Helper()
	if f.Name == "" {
		f.Name = "job"
	}
	if f.Color == "" {
		f.Color = "blue"
	}
	rec, err := app.W.CustomScripts.Create(bridge.CustomScriptsCreateArgs{Fields: f})
	if err != nil {
		t.Fatal(err)
	}
	return rec
}

func openScript(t *testing.T, app *studioflowharness.App, window, id, scriptID string) {
	t.Helper()
	if _, err := app.W.Terminal.Open(terminal.OpenArgs{
		TerminalID: id, WindowKey: window, Cols: 80, Rows: 24, LaunchKind: terminal.LaunchKindScript, ScriptID: scriptID,
	}); err != nil {
		t.Fatal(err)
	}
}

func runOf(t *testing.T, app *studioflowharness.App, terminalID string) scriptruns.Run {
	t.Helper()
	runs, err := app.W.ScriptRuns.List(scriptruns.ListArgs{})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range runs {
		if r.TerminalID == terminalID {
			return r
		}
	}
	t.Fatalf("no run for terminal %q in %+v", terminalID, runs)
	return scriptruns.Run{}
}

func waitRun(t *testing.T, app *studioflowharness.App, terminalID, state string) scriptruns.Run {
	t.Helper()
	var r scriptruns.Run
	testx.WaitUntil(t, wait, func() bool {
		runs, _ := app.W.ScriptRuns.List(scriptruns.ListArgs{})
		for _, x := range runs {
			if x.TerminalID == terminalID && x.State == state {
				r = x
				return true
			}
		}
		return false
	})
	return r
}

func TestScriptRunLifecycle(t *testing.T) {
	app := studioflowharness.New(t)
	rec := newScript(t, app, scripts.CustomScriptFields{Command: "echo started; read x; echo done"})
	mark := app.Events.Mark()
	openScript(t, app, "w1", "ok", rec.ID)

	running := waitRun(t, app, "ok", "running")
	app.Events.WaitAfter(t, mark, bridge.ChannelScriptRunsChanged, nil, wait)
	got, err := app.W.ScriptRuns.Get(scriptruns.IDArgs{ID: running.ID})
	if err != nil || got.State != "running" || got.Outcome != nil || got.Command != rec.Command || got.ScriptName != "job" {
		t.Fatalf("mid-run Get = %+v, %v", got, err)
	}
	if want := filepath.Join(app.KiraHome, "automations", rec.ID); got.Cwd != want {
		t.Fatalf("cwd = %q, want %q", got.Cwd, want)
	}

	waitMatch(t, app, mark, "w1", "ok", line("started"))
	write(t, app, "ok", "\n")
	done := waitRun(t, app, "ok", "done")
	// Contract automations: tests/ui/automations-runs.spec.ts "contract: a run ends Succeeded, Failed
	// or Cancelled" reads the same fixture.
	app.Contract(t, "automations", "CustomScriptsService.Create#done", rec)
	app.Contract(t, "automations", "ScriptRunsService.Get#done", done, studioflowharness.Mask("createdAt", "startedAt", "finishedAt"))
	if done.Outcome == nil || done.Outcome.Status != "done" || done.Outcome.ExitCode == nil || *done.Outcome.ExitCode != 0 || done.FinishedAt == nil {
		t.Fatalf("finished run = %+v", done)
	}
	if done.Outcome.OutputTail != "" {
		t.Fatalf("done run kept an output tail: %q", done.Outcome.OutputTail)
	}
	if _, err := app.W.ScriptRuns.Get(scriptruns.IDArgs{ID: "missing"}); err == nil {
		t.Fatal("Get of an unknown run succeeded")
	}
}

func TestScriptRunTwice(t *testing.T) {
	app := studioflowharness.New(t)
	rec := newScript(t, app, scripts.CustomScriptFields{Command: "echo started; read x; echo done"})

	mark := app.Events.Mark()
	openScript(t, app, "w1", "first", rec.ID)
	waitMatch(t, app, mark, "w1", "first", line("started"))
	write(t, app, "first", "\n")
	first := waitRun(t, app, "first", "done")

	openScript(t, app, "w1", "second", rec.ID)
	second := waitRun(t, app, "second", "running")

	runs, err := app.W.ScriptRuns.List(scriptruns.ListArgs{})
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 2 || runs[0].ID != second.ID || runs[1].ID != first.ID {
		t.Fatalf("List = %+v, want second then first", runs)
	}
	if runs[0].ScriptID != rec.ID || runs[1].ScriptID != rec.ID || runs[0].TerminalID == runs[1].TerminalID {
		t.Fatalf("runs share a terminal or miss the script: %+v", runs)
	}
	if runs[0].State != "running" || runs[1].State != "done" {
		t.Fatalf("states = %q, %q, want running, done", runs[0].State, runs[1].State)
	}
	// Contract automations: tests/ui/automations-tabs.spec.ts reads the same list.
	app.Contract(t, "automations", "ScriptRunsService.List#twice", runs, studioflowharness.Mask("createdAt", "startedAt", "finishedAt"))
}

func TestScriptRunNonZeroExit(t *testing.T) {
	app := studioflowharness.New(t)
	rec := newScript(t, app, scripts.CustomScriptFields{Command: "printf '\\033[31mboom\\033[0m\\r\\n' >&2; exit 3"})
	openScript(t, app, "w1", "bad", rec.ID)
	r := waitRun(t, app, "bad", "failed")
	app.Contract(t, "automations", "CustomScriptsService.Create#failed", rec)
	app.Contract(t, "automations", "ScriptRunsService.Get#failed", r, studioflowharness.Mask("createdAt", "startedAt", "finishedAt"), studioflowharness.Replace("\nlogout", ""))
	if r.Outcome.Reason != "exited with status 3" || r.Outcome.Source != "exit" || *r.Outcome.ExitCode != 3 {
		t.Fatalf("outcome = %+v, want exit 3", r.Outcome)
	}
	// Backend half of tests/ui/automations-ops.spec.ts: the stored tail is plain text and survives a restart.
	// A login shell may print "logout" after the command.
	if tail := r.Outcome.OutputTail; !strings.Contains(tail, "boom") || strings.ContainsAny(tail, "\x1b\r") {
		t.Fatalf("outputTail = %q, want boom with no escapes", tail)
	}
	app.Restart(t)
	again, err := app.W.ScriptRuns.Get(scriptruns.IDArgs{ID: r.ID})
	if err != nil || again.Outcome == nil || again.Outcome.OutputTail != r.Outcome.OutputTail {
		t.Fatalf("after restart = %+v, %v, want outputTail unchanged", again.Outcome, err)
	}
}

func TestScriptRunStopAndClose(t *testing.T) {
	app := studioflowharness.New(t)
	rec := newScript(t, app, scripts.CustomScriptFields{Command: "sleep 60"})

	openScript(t, app, "w1", "a", rec.ID)
	waitRun(t, app, "a", "running")
	if err := app.W.Terminal.Close(terminal.CloseArgs{TerminalID: "a"}); err != nil {
		t.Fatal(err)
	}
	if r := waitRun(t, app, "a", "cancelled"); r.Outcome.Reason != "stopped by you" || r.Outcome.Source != "user" || r.Outcome.OutputTail != "" {
		t.Fatalf("Close outcome = %+v", r.Outcome)
	}

	openScript(t, app, "w1", "b", rec.ID)
	r := waitRun(t, app, "b", "running")
	if err := app.W.ScriptRuns.Stop(scriptruns.IDArgs{ID: r.ID}); err != nil {
		t.Fatal(err)
	}
	if r := waitRun(t, app, "b", "cancelled"); r.Outcome.Reason != "stopped by you" {
		t.Fatalf("Stop outcome = %+v", r.Outcome)
	} else {
		app.Contract(t, "automations", "CustomScriptsService.Create#cancelled", rec)
		app.Contract(t, "automations", "ScriptRunsService.Get#cancelled", r, studioflowharness.Mask("createdAt", "startedAt", "finishedAt"))
	}
	if err := app.W.ScriptRuns.Stop(scriptruns.IDArgs{ID: r.ID}); err != nil {
		t.Fatalf("Stop of a finished run = %v, want a no-op", err)
	}
	if err := app.W.ScriptRuns.Stop(scriptruns.IDArgs{ID: "missing"}); err == nil {
		t.Fatal("Stop of an unknown run succeeded")
	}
}

func TestScriptRunWindowClose(t *testing.T) {
	app := studioflowharness.New(t)
	rec := newScript(t, app, scripts.CustomScriptFields{Command: "sleep 60"})
	openScript(t, app, "w2", "w", rec.ID)
	waitRun(t, app, "w", "running")
	app.CloseWindow("w2")
	if r := waitRun(t, app, "w", "cancelled"); r.Outcome.Reason != "its window closed while it ran" {
		t.Fatalf("window close outcome = %+v", r.Outcome)
	}
}

func TestScriptRunQuitAndRestart(t *testing.T) {
	app := studioflowharness.New(t)
	rec := newScript(t, app, scripts.CustomScriptFields{Command: "sleep 60"})
	openScript(t, app, "w1", "q", rec.ID)
	waitRun(t, app, "q", "running")

	// A row the previous process left running with nothing to end it, as after a crash.
	if _, err := app.DB().Exec(`INSERT INTO script_runs (id, script_id, script_name, trigger_kind, state, terminal_id, created_at)
		VALUES ('stale', ?, 'job', 'terminal', 'running', 'gone', 1)`, rec.ID); err != nil {
		t.Fatal(err)
	}

	app.Restart(t)
	for _, c := range []struct{ id, terminal string }{{"", "q"}, {"stale", "gone"}} {
		runs, err := app.W.ScriptRuns.List(scriptruns.ListArgs{})
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, r := range runs {
			if r.TerminalID != c.terminal {
				continue
			}
			found = true
			if r.State != "failed" || r.Outcome == nil || r.Outcome.Source != "restart" || !strings.Contains(r.Outcome.Reason, "Kira Studio") {
				t.Fatalf("run %q after restart = %+v, want failed by restart naming the app", c.terminal, r)
			}
		}
		if !found {
			t.Fatalf("run for terminal %q missing after restart", c.terminal)
		}
	}
}

func TestScriptFolders(t *testing.T) {
	app := studioflowharness.New(t)
	runs := app.W.ScriptRuns

	base, err := runs.ResolveDir(scriptruns.ResolveDirArgs{})
	if err != nil || base.Base != app.KiraHome || base.Mode != scripts.DirModeKira {
		t.Fatalf("ResolveDir of an unsaved script = %+v, %v", base, err)
	}

	kira := newScript(t, app, scripts.CustomScriptFields{Command: "pwd"})
	d, err := runs.ResolveDir(scriptruns.ResolveDirArgs{ScriptID: kira.ID})
	if want := filepath.Join(app.KiraHome, "automations", kira.ID); err != nil || d.Path != want || d.Blocker != "" {
		t.Fatalf("kira dir = %+v, %v, want %s", d, err, want)
	}
	openScript(t, app, "w1", "k", kira.ID)
	waitRun(t, app, "k", "done")
	if info, err := os.Stat(d.Path); err != nil || !info.IsDir() || info.Mode().Perm() != 0o700 {
		t.Fatalf("automations folder = %v, %v, want a 0700 directory", info, err)
	}

	missing := filepath.Join(t.TempDir(), "gone")
	fixed := newScript(t, app, scripts.CustomScriptFields{Command: "pwd", DirMode: scripts.DirModeFixed, WorkingDir: missing})
	d, err = runs.ResolveDir(scriptruns.ResolveDirArgs{ScriptID: fixed.ID})
	if err != nil || d.Blocker != missing+" does not exist." {
		t.Fatalf("fixed dir = %+v, %v, want a does-not-exist blocker", d, err)
	}

	// A symlink where the folder belongs is refused, not followed.
	target := t.TempDir()
	link := newScript(t, app, scripts.CustomScriptFields{Command: "pwd"})
	if err := os.Symlink(target, filepath.Join(app.KiraHome, "automations", link.ID)); err != nil {
		t.Fatal(err)
	}
	if _, err := app.W.Terminal.Open(terminal.OpenArgs{
		TerminalID: "sym", WindowKey: "w1", Cols: 80, Rows: 24, LaunchKind: terminal.LaunchKindScript, ScriptID: link.ID,
	}); err == nil || !strings.Contains(err.Error(), "is not a real folder") {
		t.Fatalf("Open into a symlinked folder = %v, want a not-a-real-folder refusal", err)
	}
	if r := runOf(t, app, "sym"); r.State != "failed" || r.Outcome == nil || r.Outcome.Source != "start" {
		t.Fatalf("blocked run = %+v, want failed at start", r)
	}

	if _, err := runs.ResolveDir(scriptruns.ResolveDirArgs{ScriptID: "missing"}); err == nil {
		t.Fatal("ResolveDir of an unknown script succeeded")
	}
}

func TestScriptLaunchIgnoresForgedCommand(t *testing.T) {
	app := studioflowharness.New(t)
	rec := newScript(t, app, scripts.CustomScriptFields{Command: "echo stored-command"})
	mark := app.Events.Mark()
	if _, err := app.W.Terminal.Open(terminal.OpenArgs{
		TerminalID: "f", WindowKey: "w1", Cwd: t.TempDir(), Cols: 80, Rows: 24, Command: "echo forged",
		LaunchKind: terminal.LaunchKindScript, ScriptID: rec.ID,
	}); err != nil {
		t.Fatal(err)
	}
	waitMatch(t, app, mark, "w1", "f", line("stored-command"))
	if text, _, _ := out(app, mark, "w1", "f"); strings.Contains(text, "\rforged") || strings.Contains(text, "\nforged") {
		t.Fatalf("output ran the forged command: %q", text)
	}
	if r := runOf(t, app, "f"); r.Command != rec.Command {
		t.Fatalf("run command = %q, want the stored one", r.Command)
	}
	if _, err := app.W.Terminal.Open(terminal.OpenArgs{
		TerminalID: "nope", WindowKey: "w1", Cols: 80, Rows: 24, LaunchKind: terminal.LaunchKindScript, ScriptID: "missing",
	}); err == nil {
		t.Fatal("Open with an unknown script id succeeded")
	}
}

func TestScriptRunRetentionAndRemove(t *testing.T) {
	app := studioflowharness.New(t)
	rec := newScript(t, app, scripts.CustomScriptFields{Command: "true"})
	for i := 0; i < 510; i++ {
		if _, err := app.DB().Exec(`INSERT INTO script_runs (id, script_id, script_name, trigger_kind, state, terminal_id, created_at, finished_at)
			VALUES (?, ?, 'old', 'terminal', 'done', '', ?, ?)`, fmt.Sprintf("old-%d", i), rec.ID, i+1, i+1); err != nil {
			t.Fatal(err)
		}
	}
	openScript(t, app, "w1", "r", rec.ID)
	waitRun(t, app, "r", "done")
	var n int
	testx.WaitUntil(t, wait, func() bool {
		if err := app.DB().QueryRow(`SELECT COUNT(*) FROM script_runs`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n == 500
	})
	runs, err := app.W.ScriptRuns.List(scriptruns.ListArgs{Limit: 10})
	if err != nil || len(runs) != 10 || runs[0].TerminalID != "r" {
		t.Fatalf("List(10) = %d runs (newest %q), %v", len(runs), runs[0].TerminalID, err)
	}

	folder := filepath.Join(app.KiraHome, "automations", rec.ID)
	if _, err := os.Stat(folder); err != nil {
		t.Fatalf("automations folder missing before Remove: %v", err)
	}
	if err := app.W.CustomScripts.Delete(bridge.CustomScriptsDeleteArgs{ID: rec.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(folder); !os.IsNotExist(err) {
		t.Fatalf("automations folder after Remove: %v, want it gone", err)
	}
}
