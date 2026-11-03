package termflow

import (
	"path/filepath"
	"sort"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
	"github.com/kirathecat/kira-studio/internal/scriptruns"
	"github.com/kirathecat/kira-studio/internal/scripts"
	"github.com/kirathecat/kira-studio/internal/scripts/scriptstest"
	"github.com/kirathecat/kira-studio/internal/terminal"
)

// startTerminalRun starts a normal script through Start and opens its terminal with the token.
func startTerminalRun(t *testing.T, app *flowharness.App, terminalID string, args scriptruns.RunArgs) {
	t.Helper()
	started, err := app.W.ScriptRuns.Start(startArgs(args, previewOf(t, app, args)))
	if err != nil || started.Terminal == nil {
		t.Fatalf("Start = %+v, %v", started, err)
	}
	openWithToken(t, app, terminalID, args.ScriptID, started.Terminal.Token)
	waitRun(t, app, terminalID, "running")
}

func openWithToken(t *testing.T, app *flowharness.App, terminalID, scriptID, token string) {
	t.Helper()
	if _, err := app.W.Terminal.Open(terminal.OpenArgs{
		TerminalID: terminalID, WindowKey: "w1", Cols: 80, Rows: 24, LaunchKind: terminal.LaunchKindScript, ScriptID: scriptID, ScriptLaunchToken: token,
	}); err != nil {
		t.Fatal(err)
	}
}

func restoreState(t *testing.T, app *flowharness.App) scriptruns.RestoreState {
	t.Helper()
	st, err := app.W.ScriptRuns.RestoreState()
	if err != nil {
		t.Fatalf("RestoreState: %v", err)
	}
	return st
}

func restoreItem(t *testing.T, st scriptruns.RestoreState, name string) scriptruns.RestoreItem {
	t.Helper()
	for _, it := range st.Items {
		if it.ScriptName == name {
			return it
		}
	}
	t.Fatalf("no restore item for %q in %+v", name, st.Items)
	return scriptruns.RestoreItem{}
}

func choice(it scriptruns.RestoreItem) scriptruns.RestoreChoice {
	return scriptruns.RestoreChoice{RunID: it.RunID, Hash: it.Hash}
}

func TestRunsRestore(t *testing.T) {
	restoreMask := flowharness.Mask("endedAt", "hash", "token")

	t.Run("quit", func(t *testing.T) {
		app := flowharness.New(t)
		hold := filepath.Join(t.TempDir(), "hold")
		app.Scenario(scenario(fakeagent.Action{WaitFile: hold}, fakeagent.Action{Name: "done"}))

		plain := newScript(t, app, scripts.CustomScriptFields{Name: "plain", Command: "sleep 60"})
		openScript(t, app, "w1", "t-plain", plain.ID)
		waitRun(t, app, "t-plain", "running")

		param := newScript(t, app, scripts.CustomScriptFields{
			Name: "param", Command: "sleep 60", Params: []scripts.Param{{Name: "who", Type: scripts.ParamText}},
		})
		startTerminalRun(t, app, "t-param", scriptruns.RunArgs{ScriptID: param.ID, Params: map[string][]string{"who": {"world"}}})

		secret := newScript(t, app, scripts.CustomScriptFields{
			Name: "secret", Command: "sleep 60", Params: []scripts.Param{{Name: "token", Type: scripts.ParamText, Secret: true}},
		})
		startTerminalRun(t, app, "t-secret", scriptruns.RunArgs{ScriptID: secret.ID, Params: map[string][]string{"token": {"s3cret"}}})

		smart := newSmart(t, app, "Say hi", func(f *scripts.CustomScriptFields) { f.Name = "smart" })
		smartID, _ := startSmart(t, app, scriptruns.RunArgs{ScriptID: smart.ID})
		waitSmart(t, app, smartID, "running")
		waitCall(t, app, 1)

		app.Restart(t)
		waitSmart(t, app, smartID, "failed")

		st := restoreState(t, app)
		if len(st.Items) != 4 {
			t.Fatalf("RestoreState = %+v, want 4 items", st.Items)
		}
		for name, want := range map[string]string{
			"plain": scriptruns.RestoreReady, "param": scriptruns.RestoreReady, "smart": scriptruns.RestoreReady, "secret": scriptruns.RestoreNeedsInput,
		} {
			if got := restoreItem(t, st, name).Status; got != want {
				t.Errorf("%s status = %q, want %q", name, got, want)
			}
		}
		if !restoreItem(t, st, "smart").Resume || restoreItem(t, st, "plain").Resume {
			t.Errorf("resume flags wrong in %+v", st.Items)
		}
		if got := restoreItem(t, st, "param").Params; len(got) != 1 || got[0].Value != "world" {
			t.Errorf("param params = %+v, want who=world", got)
		}

		sorted := append([]scriptruns.RestoreItem(nil), st.Items...)
		sort.Slice(sorted, func(i, j int) bool { return sorted[i].ScriptName < sorted[j].ScriptName })
		app.Contract(t, "runs-restore", "ScriptRunsService.RestoreState#pending", scriptruns.RestoreState{Items: sorted}, restoreMask)

		// The user keeps plain and smart checked and unchecks param.
		args := scriptruns.RestoreArgs{Items: []scriptruns.RestoreChoice{choice(restoreItem(t, st, "plain")), choice(restoreItem(t, st, "smart"))}}
		app.Contract(t, "runs-restore", "args:ScriptRunsService.Restore", args, restoreMask)
		res, err := app.W.ScriptRuns.Restore(args)
		if err != nil {
			t.Fatalf("Restore: %v", err)
		}
		app.Contract(t, "runs-restore", "ScriptRunsService.Restore", res, restoreMask)
		if len(res.Items) != 2 || res.Items[0].Started == nil || res.Items[0].Started.Terminal == nil ||
			res.Items[1].Started == nil || res.Items[1].Started.RunID == "" {
			t.Fatalf("Restore = %+v, want a terminal token and a smart run id", res)
		}

		openWithToken(t, app, "t-plain2", plain.ID, res.Items[0].Started.Terminal.Token)
		if r := waitRun(t, app, "t-plain2", "running"); r.Trigger != "manual" {
			t.Errorf("restored plain run trigger = %q, want manual", r.Trigger)
		}
		resumed := waitSmart(t, app, res.Items[1].Started.RunID, "done")
		if resumed.ResumedFrom != smartID {
			t.Errorf("restored smart run = %+v, want resumedFrom %s", resumed, smartID)
		}
		if a, _ := resumeCall(t, app); argAfter(a, "--resume") == "" {
			t.Errorf("fake claude call = %q, want --resume", a)
		}

		after := restoreState(t, app)
		if len(after.Items) != 2 || restoreItem(t, after, "param").Status != scriptruns.RestoreReady ||
			restoreItem(t, after, "secret").Status != scriptruns.RestoreNeedsInput {
			t.Fatalf("state after Restore = %+v, want param and secret left", after.Items)
		}
		if _, err := app.W.ScriptRuns.Restore(args); errCode(err) != "E_BAD_REQUEST" {
			t.Errorf("Restore of items no longer offered = %v, want E_BAD_REQUEST", err)
		}
		if err := app.W.ScriptRuns.RestoreDismiss(); err != nil {
			t.Fatal(err)
		}
		if got := restoreState(t, app); len(got.Items) != 0 {
			t.Fatalf("state after RestoreDismiss = %+v, want empty", got.Items)
		}
	})

	t.Run("crash", func(t *testing.T) {
		app := flowharness.New(t)
		app.Scenario(scenario(fakeagent.Action{Name: "done"}))
		rec := newSmart(t, app, "Say hi", nil)
		id, _ := startSmart(t, app, scriptruns.RunArgs{ScriptID: rec.ID})
		waitSmart(t, app, id, "done")
		// A crashed process leaves the row running; Recover ends it on the next boot.
		if _, err := app.DB().Exec(`UPDATE script_runs SET state = 'running', outcome_json = '', finished_at = NULL, resume_id = '' WHERE id = ?`, id); err != nil {
			t.Fatal(err)
		}
		app.Restart(t)
		waitSmart(t, app, id, "failed")
		st := restoreState(t, app)
		if len(st.Items) != 1 || st.Items[0].RunID != id || st.Items[0].Status != scriptruns.RestoreReady || !st.Items[0].Resume {
			t.Fatalf("state after crash = %+v, want the interrupted smart run, ready", st.Items)
		}
	})

	t.Run("conflict", func(t *testing.T) {
		app := flowharness.New(t)
		rec := newScript(t, app, scripts.CustomScriptFields{Name: "plain", Command: "sleep 60"})
		openScript(t, app, "w1", "t1", rec.ID)
		waitRun(t, app, "t1", "running")
		app.Restart(t)

		old := restoreItem(t, restoreState(t, app), "plain")
		edit := scripts.CustomScriptFields{Name: rec.Name, Kind: rec.Kind, Command: "sleep 61", Color: rec.Color, DirMode: rec.DirMode}
		scriptstest.Update(t, app.W.CustomScripts, rec.ID, edit)
		res, err := app.W.ScriptRuns.Restore(scriptruns.RestoreArgs{Items: []scriptruns.RestoreChoice{choice(old)}})
		if err != nil || len(res.Items) != 1 || res.Items[0].Code != "E_CONFLICT" || res.Items[0].Started != nil {
			t.Fatalf("Restore with the old hash = %+v, %v, want E_CONFLICT", res, err)
		}
		now := restoreItem(t, restoreState(t, app), "plain")
		if now.Hash == old.Hash || now.Command != "sleep 61" {
			t.Fatalf("item after edit = %+v, want a new hash and command", now)
		}
		res, err = app.W.ScriptRuns.Restore(scriptruns.RestoreArgs{Items: []scriptruns.RestoreChoice{choice(now)}})
		if err != nil || res.Items[0].Started == nil {
			t.Fatalf("Restore with the new hash = %+v, %v", res, err)
		}
	})

	t.Run("live key", func(t *testing.T) {
		app := flowharness.New(t)
		rec := newScript(t, app, scripts.CustomScriptFields{Name: "plain", Command: "sleep 60"})
		openScript(t, app, "w1", "t1", rec.ID)
		waitRun(t, app, "t1", "running")
		app.Restart(t)

		startTerminalRun(t, app, "t2", scriptruns.RunArgs{ScriptID: rec.ID})
		it := restoreItem(t, restoreState(t, app), "plain")
		if it.Status != scriptruns.RestoreRunning {
			t.Fatalf("item = %+v, want running", it)
		}
		res, err := app.W.ScriptRuns.Restore(scriptruns.RestoreArgs{Items: []scriptruns.RestoreChoice{choice(it)}})
		if err != nil || res.Items[0].Code != "E_CONFLICT" || res.Items[0].Started != nil {
			t.Fatalf("Restore of a live key = %+v, %v, want E_CONFLICT", res, err)
		}
	})

	t.Run("stop is not offered", func(t *testing.T) {
		app := flowharness.New(t)
		rec := newScript(t, app, scripts.CustomScriptFields{Name: "plain", Command: "sleep 60"})
		openScript(t, app, "w1", "stopped", rec.ID)
		waitRun(t, app, "stopped", "running")
		if err := app.W.Terminal.Close(terminal.CloseArgs{TerminalID: "stopped"}); err != nil {
			t.Fatal(err)
		}
		waitRun(t, app, "stopped", "cancelled")
		other := newScript(t, app, scripts.CustomScriptFields{Name: "other", Command: "sleep 60"})
		openScript(t, app, "w1", "quit", other.ID)
		waitRun(t, app, "quit", "running")

		app.Restart(t)
		if st := restoreState(t, app); len(st.Items) != 1 || st.Items[0].ScriptName != "other" {
			t.Fatalf("state = %+v, want only the run the quit ended", st.Items)
		}
		app.Restart(t)
		if st := restoreState(t, app); len(st.Items) != 0 {
			t.Fatalf("state after a second restart = %+v, want empty", st.Items)
		}
	})
}
