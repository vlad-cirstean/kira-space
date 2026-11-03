package termflow_test

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/scriptruns"
	"github.com/kirathecat/kira-studio/internal/scripts"
	"github.com/kirathecat/kira-studio/internal/terminal"
	"github.com/kirathecat/kira-studio/internal/testx"
)

func newScript(t *testing.T, app *flowharness.App, f scripts.CustomScriptFields) scripts.CustomScript {
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

func openScript(t *testing.T, app *flowharness.App, window, id, scriptID string) {
	t.Helper()
	if _, err := app.W.Terminal.Open(terminal.OpenArgs{
		TerminalID: id, WindowKey: window, Cols: 80, Rows: 24, LaunchKind: terminal.LaunchKindScript, ScriptID: scriptID,
	}); err != nil {
		t.Fatal(err)
	}
}

func runOf(t *testing.T, app *flowharness.App, terminalID string) scriptruns.Run {
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

func waitRun(t *testing.T, app *flowharness.App, terminalID, state string) scriptruns.Run {
	t.Helper()
	var r scriptruns.Run
	testx.WaitUntil(t, waitFor, func() bool {
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
	app := flowharness.New(t)
	rec := newScript(t, app, scripts.CustomScriptFields{Command: "echo started; read x; echo done"})
	mark := app.Events.Mark()
	openScript(t, app, window, "ok", rec.ID)

	running := waitRun(t, app, "ok", "running")
	app.Events.WaitAfter(t, mark, bridge.ChannelScriptRunsChanged, nil, waitFor)
	got, err := app.W.ScriptRuns.Get(scriptruns.IDArgs{ID: running.ID})
	if err != nil || got.State != "running" || got.Outcome != nil || got.Command != rec.Command || got.ScriptName != "job" {
		t.Fatalf("mid-run Get = %+v, %v", got, err)
	}
	if want := filepath.Join(app.SpaceHome, "automations", rec.ID); got.Cwd != want {
		t.Fatalf("cwd = %q, want %q", got.Cwd, want)
	}

	testx.WaitUntil(t, waitFor, func() bool { return hasLine(output(app, "ok"), "started") })
	if err := app.W.Terminal.Write(terminal.WriteArgs{TerminalID: "ok", Data: base64.StdEncoding.EncodeToString([]byte("\n"))}); err != nil {
		t.Fatal(err)
	}
	done := waitRun(t, app, "ok", "done")
	// Contract automations: tests/ui/automations-runs.spec.ts "contract: a script run ends ..." reads the same fixture.
	app.Contract(t, "automations", "CustomScriptsService.Create#done", rec)
	app.Contract(t, "automations", "ScriptRunsService.Get#done", done, flowharness.Mask("createdAt", "startedAt", "finishedAt"))
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
	app := flowharness.New(t)
	rec := newScript(t, app, scripts.CustomScriptFields{Command: "echo started; read x; echo done"})

	openScript(t, app, window, "first", rec.ID)
	testx.WaitUntil(t, waitFor, func() bool { return hasLine(output(app, "first"), "started") })
	if err := app.W.Terminal.Write(terminal.WriteArgs{TerminalID: "first", Data: base64.StdEncoding.EncodeToString([]byte("\n"))}); err != nil {
		t.Fatal(err)
	}
	first := waitRun(t, app, "first", "done")

	openScript(t, app, window, "second", rec.ID)
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
	app.Contract(t, "automations", "ScriptRunsService.List#twice", runs, flowharness.Mask("createdAt", "startedAt", "finishedAt"))
}

func TestScriptRunNonZeroExit(t *testing.T) {
	app := flowharness.New(t)
	rec := newScript(t, app, scripts.CustomScriptFields{Command: "printf '\\033[31mboom\\033[0m\\r\\n' >&2; exit 3"})
	openScript(t, app, window, "bad", rec.ID)
	r := waitRun(t, app, "bad", "failed")
	app.Contract(t, "automations", "CustomScriptsService.Create#failed", rec)
	app.Contract(t, "automations", "ScriptRunsService.Get#failed", r, flowharness.Mask("createdAt", "startedAt", "finishedAt"), flowharness.Replace("\nlogout", ""))
	if r.Outcome.Reason != "exited with status 3" || r.Outcome.Source != "exit" || *r.Outcome.ExitCode != 3 {
		t.Fatalf("outcome = %+v, want exit 3", r.Outcome)
	}
	// Backend half of tests/ui/automations-ops.spec.ts: the stored tail is plain text and survives a restart.
	// A login shell may print "logout" after the command.
	if tail := r.Outcome.OutputTail; !strings.Contains(tail, "boom") || strings.ContainsAny(tail, "\x1b\r") {
		t.Fatalf("outputTail = %q, want boom with no escapes", tail)
	}
	app.Restart()
	again, err := app.W.ScriptRuns.Get(scriptruns.IDArgs{ID: r.ID})
	if err != nil || again.Outcome == nil || again.Outcome.OutputTail != r.Outcome.OutputTail {
		t.Fatalf("after restart = %+v, %v, want outputTail unchanged", again.Outcome, err)
	}
}

func TestScriptRunStopAndClose(t *testing.T) {
	app := flowharness.New(t)
	rec := newScript(t, app, scripts.CustomScriptFields{Command: "sleep 60"})

	openScript(t, app, window, "a", rec.ID)
	waitRun(t, app, "a", "running")
	if err := app.W.Terminal.Close(terminal.CloseArgs{TerminalID: "a"}); err != nil {
		t.Fatal(err)
	}
	if r := waitRun(t, app, "a", "cancelled"); r.Outcome.Reason != "stopped by you" || r.Outcome.Source != "user" || r.Outcome.OutputTail != "" {
		t.Fatalf("Close outcome = %+v", r.Outcome)
	}

	openScript(t, app, window, "b", rec.ID)
	r := waitRun(t, app, "b", "running")
	if err := app.W.ScriptRuns.Stop(scriptruns.IDArgs{ID: r.ID}); err != nil {
		t.Fatal(err)
	}
	if r := waitRun(t, app, "b", "cancelled"); r.Outcome.Reason != "stopped by you" {
		t.Fatalf("Stop outcome = %+v", r.Outcome)
	} else {
		app.Contract(t, "automations", "CustomScriptsService.Create#cancelled", rec)
		app.Contract(t, "automations", "ScriptRunsService.Get#cancelled", r, flowharness.Mask("createdAt", "startedAt", "finishedAt"))
	}
	if err := app.W.ScriptRuns.Stop(scriptruns.IDArgs{ID: r.ID}); err != nil {
		t.Fatalf("Stop of a finished run = %v, want a no-op", err)
	}
	if err := app.W.ScriptRuns.Stop(scriptruns.IDArgs{ID: "missing"}); err == nil {
		t.Fatal("Stop of an unknown run succeeded")
	}
}

func TestScriptRunWindowClose(t *testing.T) {
	app := flowharness.New(t)
	rec := newScript(t, app, scripts.CustomScriptFields{Command: "sleep 60"})
	openScript(t, app, "w2", "w", rec.ID)
	waitRun(t, app, "w", "running")
	app.W.TermRegistry.CloseWindow("w2")
	if r := waitRun(t, app, "w", "cancelled"); r.Outcome.Reason != "its window closed while it ran" {
		t.Fatalf("window close outcome = %+v", r.Outcome)
	}
}

func TestScriptRunQuitAndRestart(t *testing.T) {
	app := flowharness.New(t)
	rec := newScript(t, app, scripts.CustomScriptFields{Command: "sleep 60"})
	openScript(t, app, window, "q", rec.ID)
	waitRun(t, app, "q", "running")

	// A row the previous process left running with nothing to end it, as after a crash.
	if _, err := app.DB().Exec(`INSERT INTO script_runs (id, script_id, script_name, trigger_kind, state, terminal_id, created_at)
		VALUES ('stale', ?, 'job', 'terminal', 'running', 'gone', 1)`, rec.ID); err != nil {
		t.Fatal(err)
	}

	app.Restart()
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
			if r.State != "failed" || r.Outcome == nil || r.Outcome.Source != "restart" || !strings.Contains(r.Outcome.Reason, "Kira Space") {
				t.Fatalf("run %q after restart = %+v, want failed by restart naming the app", c.terminal, r)
			}
		}
		if !found {
			t.Fatalf("run for terminal %q missing after restart", c.terminal)
		}
	}
}

func TestScriptFolders(t *testing.T) {
	app := flowharness.New(t)
	runs := app.W.ScriptRuns

	base, err := runs.ResolveDir(scriptruns.ResolveDirArgs{})
	if err != nil || base.Base != app.SpaceHome || base.Mode != scripts.DirModeKira {
		t.Fatalf("ResolveDir of an unsaved script = %+v, %v", base, err)
	}

	kira := newScript(t, app, scripts.CustomScriptFields{Command: "pwd"})
	d, err := runs.ResolveDir(scriptruns.ResolveDirArgs{ScriptID: kira.ID})
	if want := filepath.Join(app.SpaceHome, "automations", kira.ID); err != nil || d.Path != want || d.Blocker != "" {
		t.Fatalf("kira dir = %+v, %v, want %s", d, err, want)
	}
	openScript(t, app, window, "k", kira.ID)
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
	// Contract script-folders: tests/ui automations-scripts "Choose folder…" reads the fixed record.
	app.Contract(t, "script-folders", "CustomScriptsService.Create#fixed", fixed, flowharness.Replace(filepath.Dir(missing), "<tmp>"))

	// A symlink where the folder belongs is refused, not followed.
	target := t.TempDir()
	link := newScript(t, app, scripts.CustomScriptFields{Command: "pwd"})
	if err := os.Symlink(target, filepath.Join(app.SpaceHome, "automations", link.ID)); err != nil {
		t.Fatal(err)
	}
	if _, err := app.W.Terminal.Open(terminal.OpenArgs{
		TerminalID: "sym", WindowKey: window, Cols: 80, Rows: 24, LaunchKind: terminal.LaunchKindScript, ScriptID: link.ID,
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

// TestScriptOrder is the backend half of the shared automationsOrder UI scenario (contract
// script-order).
func TestScriptOrder(t *testing.T) {
	app := flowharness.New(t)
	cs := app.W.CustomScripts
	s1 := newScript(t, app, scripts.CustomScriptFields{Name: "s1", Command: "echo s1"})
	s2 := newScript(t, app, scripts.CustomScriptFields{Name: "s2", Command: "echo s2"})
	s3 := newScript(t, app, scripts.CustomScriptFields{Name: "s3", Command: "echo s3"})
	t1 := newScript(t, app, scripts.CustomScriptFields{Name: "t1", Command: "echo t1", Collection: "X"})
	t2 := newScript(t, app, scripts.CustomScriptFields{Name: "t2", Command: "echo t2", Collection: "X"})
	newScript(t, app, scripts.CustomScriptFields{Name: "u1", Command: "echo u1", Collection: "Y"})

	ungrouped := bridge.CustomScriptsReorderArgs{IDs: []string{s3.ID, s1.ID, s2.ID}}
	inX := bridge.CustomScriptsReorderArgs{Collection: "X", IDs: []string{t2.ID, t1.ID}}
	colls := bridge.CustomScriptsReorderCollectionsArgs{Names: []string{"Y", "X"}}
	if err := cs.Reorder(ungrouped); err != nil {
		t.Fatal(err)
	}
	if err := cs.Reorder(inX); err != nil {
		t.Fatal(err)
	}
	if err := cs.ReorderCollections(colls); err != nil {
		t.Fatal(err)
	}
	app.Contract(t, "script-order", "args:CustomScriptsService.Reorder#ungrouped", ungrouped)
	app.Contract(t, "script-order", "args:CustomScriptsService.Reorder#collection", inX)
	app.Contract(t, "script-order", "args:CustomScriptsService.ReorderCollections", colls)

	names := func(snap scripts.Snapshot) (ung, inx, cols string) {
		var u, in, c []string
		for _, s := range snap.Scripts {
			switch s.Collection {
			case "":
				u = append(u, s.Name)
			case "X":
				in = append(in, s.Name)
			}
		}
		for _, col := range snap.Collections {
			c = append(c, col.Name)
		}
		return strings.Join(u, ","), strings.Join(in, ","), strings.Join(c, ",")
	}
	check := func(label string, snap scripts.Snapshot) {
		t.Helper()
		if ung, inx, cols := names(snap); ung != "s3,s1,s2" || inx != "t2,t1" || cols != "Y,X" {
			t.Fatalf("%s = ungrouped %s, X %s, collections %s; want s3,s1,s2, t2,t1, Y,X", label, ung, inx, cols)
		}
	}
	snap, err := cs.List()
	if err != nil {
		t.Fatal(err)
	}
	check("List", snap)

	if err := cs.Reorder(bridge.CustomScriptsReorderArgs{IDs: []string{t1.ID}}); err == nil {
		t.Fatal("Reorder with a script of another set succeeded")
	}
	if again, err := cs.List(); err != nil {
		t.Fatal(err)
	} else if !reflect.DeepEqual(again, snap) {
		t.Fatalf("a refused Reorder changed the order: %+v, want %+v", again, snap)
	}

	app.Restart()
	again, err := app.W.CustomScripts.List()
	if err != nil {
		t.Fatal(err)
	}
	check("after Restart", again)
}

func TestScriptLaunchIgnoresForgedCommand(t *testing.T) {
	app := flowharness.New(t)
	rec := newScript(t, app, scripts.CustomScriptFields{Command: "echo stored-command"})
	if _, err := app.W.Terminal.Open(terminal.OpenArgs{
		TerminalID: "f", WindowKey: window, Cwd: t.TempDir(), Cols: 80, Rows: 24, Command: "echo forged",
		LaunchKind: terminal.LaunchKindScript, ScriptID: rec.ID,
	}); err != nil {
		t.Fatal(err)
	}
	testx.WaitUntil(t, waitFor, func() bool { return hasLine(output(app, "f"), "stored-command") })
	if hasLine(output(app, "f"), "forged") {
		t.Fatalf("output ran the forged command: %q", output(app, "f"))
	}
	if r := runOf(t, app, "f"); r.Command != rec.Command {
		t.Fatalf("run command = %q, want the stored one", r.Command)
	}
	if _, err := app.W.Terminal.Open(terminal.OpenArgs{
		TerminalID: "nope", WindowKey: window, Cols: 80, Rows: 24, LaunchKind: terminal.LaunchKindScript, ScriptID: "missing",
	}); err == nil {
		t.Fatal("Open with an unknown script id succeeded")
	}
}

func TestScriptRunRetentionAndRemove(t *testing.T) {
	app := flowharness.New(t)
	rec := newScript(t, app, scripts.CustomScriptFields{Command: "true"})
	for i := 0; i < 510; i++ {
		if _, err := app.DB().Exec(`INSERT INTO script_runs (id, script_id, script_name, trigger_kind, state, terminal_id, created_at, finished_at)
			VALUES (?, ?, 'old', 'terminal', 'done', '', ?, ?)`, fmt.Sprintf("old-%d", i), rec.ID, i+1, i+1); err != nil {
			t.Fatal(err)
		}
	}
	openScript(t, app, window, "r", rec.ID)
	waitRun(t, app, "r", "done")
	var n int
	testx.WaitUntil(t, waitFor, func() bool {
		if err := app.DB().QueryRow(`SELECT COUNT(*) FROM script_runs`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n == 500
	})
	runs, err := app.W.ScriptRuns.List(scriptruns.ListArgs{Limit: 10})
	if err != nil || len(runs) != 10 || runs[0].TerminalID != "r" {
		t.Fatalf("List(10) = %d runs (newest %q), %v", len(runs), runs[0].TerminalID, err)
	}

	folder := filepath.Join(app.SpaceHome, "automations", rec.ID)
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
