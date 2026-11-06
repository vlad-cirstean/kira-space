package appflow_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/appevent"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
	"github.com/kirathecat/kira-studio/internal/prompts"
	"github.com/kirathecat/kira-studio/internal/scriptruns"
	"github.com/kirathecat/kira-studio/internal/scripts"
	"github.com/kirathecat/kira-studio/internal/shell"
	"github.com/kirathecat/kira-studio/internal/terminal"
	"github.com/kirathecat/kira-studio/internal/testx"
)

func quitPrompts(app *flowharness.App) []prompts.Routed {
	var out []prompts.Routed
	for _, p := range app.W.Prompts.List() {
		if p.Kind == prompts.KindQuit {
			out = append(out, p)
		}
	}
	return out
}

func TestQuitConfirm(t *testing.T) {
	app := flowharness.New(t)
	addWindows(t, app, "main")
	hold := filepath.Join(t.TempDir(), "hold")
	claude(app, map[string][]fakeagent.Action{"*": {{WaitFile: hold}, {Name: "done"}}})
	repo, _ := cloneWithMain(app, filepath.Join(app.Work, "api"))
	rec := importRepo(t, app, repo.Dir)
	if _, err := app.W.AdeTask.SaveWorkflowYaml(ctx, adewire.SaveWorkflowYamlArgs{FileName: "flow.yaml", Yaml: agentFlow}); err != nil {
		t.Fatal(err)
	}
	task, err := app.W.AdeTask.CreateTask(ctx, adewire.CreateTaskArgs{Title: "Fix login", CodeRepoIDs: []string{rec.ID}, WorkflowID: "flow"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := app.W.AdeTask.Board(ctx)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]string{}
	for _, br := range b.Branches {
		if br.TaskID == task.ID {
			names[br.ID] = "feat/api-quit"
		}
	}
	if _, err := app.W.AdeTask.StartRun(ctx, adewire.StartRunArgs{TaskID: task.ID, BranchNames: names}); err != nil {
		t.Fatal(err)
	}
	testx.WaitUntil(t, waitFor, func() bool {
		raw, _ := os.ReadFile(filepath.Join(app.FakeDir, "api-1.args"))
		return len(raw) > 0
	})
	if _, err := app.W.Terminal.Open(terminal.OpenArgs{
		TerminalID: "agent-1", WindowKey: "main", Cwd: app.Home, Cols: 100, Rows: 30,
		Command: "claude", LaunchKind: terminal.LaunchKindAgent,
	}); err != nil {
		t.Fatal(err)
	}
	script, err := app.W.CustomScripts.Create(bridge.CustomScriptsCreateArgs{Fields: scripts.CustomScriptFields{
		Name: "ask", Kind: scripts.KindSmart, Command: "Say hi", Color: "blue",
	}})
	if err != nil {
		t.Fatal(err)
	}
	scriptArgs := scriptruns.RunArgs{ScriptID: script.ID}
	preview, err := app.W.ScriptRuns.Preview(scriptArgs)
	if err != nil {
		t.Fatal(err)
	}
	scriptRun, err := app.W.ScriptRuns.Start(scriptruns.StartArgs{RunArgs: scriptArgs, Hash: preview.Hash})
	if err != nil {
		t.Fatal(err)
	}
	status := func() shell.QuitStatus {
		st, err := app.W.Lifecycle.QuitStatus()
		if err != nil {
			t.Fatal(err)
		}
		return st
	}
	testx.WaitUntil(t, waitFor, func() bool { return status().Agents == 1 && status().Runs == 1 && status().Scripts == 1 })
	if quitPrompts(app) != nil {
		t.Fatal("a quit prompt exists before any quit")
	}

	if app.W.Quitter.ShouldQuit() {
		t.Fatal("ShouldQuit allowed a quit with live work")
	}
	testx.WaitUntil(t, waitFor, func() bool { return len(quitPrompts(app)) == 1 })
	if p := quitPrompts(app)[0]; p.Target != "main" {
		t.Fatalf("quit prompt = %+v, want it on the main window", p)
	}
	asking := status()
	if want := (shell.QuitStatus{Asking: true, Agents: 1, Runs: 1, Scripts: 1}); asking != want {
		t.Fatalf("QuitStatus = %+v, want %+v", asking, want)
	}
	app.Contract(t, "quit-confirm", "LifecycleService.QuitStatus#asking", asking)
	if len(app.Events.Since(0, appevent.ChannelFlushBeforeClose)) != 0 {
		t.Fatal("the flush started before the user confirmed")
	}

	app.W.Lifecycle.CancelQuit()
	testx.WaitUntil(t, waitFor, func() bool { return len(quitPrompts(app)) == 0 })
	if status().Asking {
		t.Fatal("QuitStatus still asking after Cancel")
	}
	if _, err := app.W.Settings.GetAll(); err != nil {
		t.Fatalf("bound calls dead after Cancel: %v", err)
	}
	if app.W.Quitter.ShouldQuit() {
		t.Fatal("ShouldQuit allowed a quit after Cancel")
	}
	testx.WaitUntil(t, waitFor, func() bool { return len(quitPrompts(app)) == 1 })

	mark := app.Events.Mark()
	app.W.Lifecycle.ConfirmQuit()
	app.Events.WaitAfter(t, mark, appevent.ChannelFlushBeforeClose, nil, waitFor)
	if len(quitPrompts(app)) != 0 {
		t.Fatal("the quit prompt outlived Quit anyway")
	}
	app.W.Lifecycle.Flushed(bridge.LifecycleFlushedArgs{WindowKey: "main"})
	testx.WaitUntil(t, waitFor, app.W.Quitter.ShouldQuit)

	app.Restart()
	var run adewire.Run
	testx.WaitUntil(t, waitFor, func() bool {
		bd, err := app.W.AdeTask.Board(ctx)
		if err != nil {
			return false
		}
		for _, tk := range bd.Tasks {
			for _, r := range tk.Runs {
				run = r
			}
		}
		return run.Resumable
	})
	if got, err := app.W.ScriptRuns.Get(scriptruns.IDArgs{ID: scriptRun.RunID}); err != nil || !got.Resumable {
		t.Fatalf("script run after the quit = %+v, %v, want resumable", got, err)
	}
}

// A quit with nothing live raises no prompt.
func TestQuitWithoutLiveWorkAsksNothing(t *testing.T) {
	app := flowharness.New(t)
	addWindows(t, app, "main")
	if app.W.Quitter.ShouldQuit() {
		t.Fatal("first ShouldQuit allowed the quit before the flush")
	}
	app.Events.Wait(t, appevent.ChannelFlushBeforeClose, nil, waitFor)
	if p := quitPrompts(app); p != nil {
		t.Fatalf("quit prompts = %+v, want none", p)
	}
	if st, _ := app.W.Lifecycle.QuitStatus(); st.Asking {
		t.Fatalf("QuitStatus = %+v, want not asking", st)
	}
	app.W.Lifecycle.Flushed(bridge.LifecycleFlushedArgs{WindowKey: "main"})
	testx.WaitUntil(t, waitFor, app.W.Quitter.ShouldQuit)
}
