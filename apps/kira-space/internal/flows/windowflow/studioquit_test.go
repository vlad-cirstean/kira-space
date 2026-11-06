package windowflow_test

import (
	"path/filepath"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/appevent"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
	"github.com/kirathecat/kira-studio/internal/prompts"
	"github.com/kirathecat/kira-studio/internal/scriptruns"
	"github.com/kirathecat/kira-studio/internal/scripts"
	"github.com/kirathecat/kira-studio/internal/shell"
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
	app.W.Windows.Add("main", 0, nil, func() {})
	hold := filepath.Join(t.TempDir(), "hold")
	app.Scenario(fakeagent.Scenario{Claude: map[string][]fakeagent.Action{"*": {{WaitFile: hold}, {Name: "done"}}}})
	rec, err := app.W.CustomScripts.Create(bridge.CustomScriptsCreateArgs{Fields: scripts.CustomScriptFields{
		Name: "ask", Kind: scripts.KindSmart, Command: "Say hi", Color: "blue",
	}})
	if err != nil {
		t.Fatal(err)
	}
	args := scriptruns.RunArgs{ScriptID: rec.ID}
	pv, err := app.W.ScriptRuns.Preview(args)
	if err != nil {
		t.Fatal(err)
	}
	started, err := app.W.ScriptRuns.Start(scriptruns.StartArgs{RunArgs: args, Hash: pv.Hash})
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
	testx.WaitUntil(t, wait, func() bool { return status().Scripts == 1 })
	if quitPrompts(app) != nil {
		t.Fatal("a quit prompt exists before any quit")
	}

	if app.W.Quitter.ShouldQuit() {
		t.Fatal("ShouldQuit allowed a quit with live work")
	}
	testx.WaitUntil(t, wait, func() bool { return len(quitPrompts(app)) == 1 })
	if p := quitPrompts(app)[0]; p.Target != "main" {
		t.Fatalf("quit prompt = %+v, want it on the main window", p)
	}
	asking := status()
	if want := (shell.QuitStatus{Asking: true, Scripts: 1}); asking != want {
		t.Fatalf("QuitStatus = %+v, want %+v", asking, want)
	}
	app.Contract(t, "quit-confirm", "LifecycleService.QuitStatus#asking", asking)
	if len(app.Events.Since(0, appevent.ChannelFlushBeforeClose)) != 0 {
		t.Fatal("the flush started before the user confirmed")
	}

	app.W.Lifecycle.CancelQuit()
	testx.WaitUntil(t, wait, func() bool { return len(quitPrompts(app)) == 0 })
	if status().Asking {
		t.Fatal("QuitStatus still asking after Cancel")
	}
	if _, err := app.W.StudioSettings.GetAll(); err != nil {
		t.Fatalf("bound calls dead after Cancel: %v", err)
	}
	if app.W.Quitter.ShouldQuit() {
		t.Fatal("ShouldQuit allowed a quit after Cancel")
	}
	testx.WaitUntil(t, wait, func() bool { return len(quitPrompts(app)) == 1 })

	mark := app.Events.Mark()
	app.W.Lifecycle.ConfirmQuit()
	app.Events.WaitAfter(t, mark, appevent.ChannelFlushBeforeClose, nil, wait)
	if len(quitPrompts(app)) != 0 {
		t.Fatal("the quit prompt outlived Quit anyway")
	}
	app.W.Lifecycle.Flushed(bridge.LifecycleFlushedArgs{WindowKey: "main"})
	testx.WaitUntil(t, wait, app.W.Quitter.ShouldQuit)

	app.Restart()
	run, err := app.W.ScriptRuns.Get(scriptruns.IDArgs{ID: started.RunID})
	if err != nil || !run.Resumable {
		t.Fatalf("run after the quit = %+v, %v, want resumable", run, err)
	}
}
