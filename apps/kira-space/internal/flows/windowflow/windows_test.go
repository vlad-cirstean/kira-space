package windowflow_test

import (
	"context"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/appevent"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
	"github.com/kirathecat/kira-studio/internal/prompts"
	"github.com/kirathecat/kira-studio/internal/shell"
	"github.com/kirathecat/kira-studio/internal/terminal"
	"github.com/kirathecat/kira-studio/internal/testx"
	"github.com/kirathecat/kira-studio/internal/windowsvc"
)

var ctx = context.Background()

const waitFor = 20 * time.Second

func ensure(t *testing.T, app *flowharness.App, key string) {
	t.Helper()
	if _, err := app.W.WindowsSvc.Ensure(windowsvc.EnsureArgs{WindowKey: key}); err != nil {
		t.Fatalf("Ensure %s: %v", key, err)
	}
}

func openShell(t *testing.T, app *flowharness.App, id, key string) {
	t.Helper()
	if _, err := app.W.Terminal.Open(terminal.OpenArgs{TerminalID: id, WindowKey: key, Cwd: app.Home, Cols: 80, Rows: 24}); err != nil {
		t.Fatalf("Terminal.Open %s in %s: %v", id, key, err)
	}
}

func tabIDs(t *testing.T, app *flowharness.App, key string) []string {
	t.Helper()
	tabs, err := app.W.Tabs.List(bridge.TabsListArgs{WindowKey: key})
	if err != nil {
		t.Fatalf("Tabs.List %s: %v", key, err)
	}
	ids := make([]string, 0, len(tabs))
	for _, tb := range tabs {
		ids = append(ids, tb.ID)
	}
	slices.Sort(ids)
	return ids
}

func TestTwoWindowsIsolation(t *testing.T) {
	app := flowharness.New(t)
	for _, key := range []string{"w-a", "w-b"} {
		ensure(t, app, key)
		app.WindowMgr.OpenWindow(shell.WindowRecord{Key: key})
	}
	openShell(t, app, "t-a1", "w-a")
	openShell(t, app, "t-a2", "w-a")
	openShell(t, app, "t-b1", "w-b")
	for id, want := range map[string]string{"t-a1": "w-a", "t-a2": "w-a", "t-b1": "w-b"} {
		if got, ok := app.W.TermRegistry.WindowOf(id); !ok || got != want {
			t.Fatalf("WindowOf(%s) = %q, %v, want %s", id, got, ok, want)
		}
	}

	save := func(key, id string) {
		t.Helper()
		ws := "ws"
		rec := model.TabRecord{ID: id, Path: "/" + id, Kind: "terminal", State: []byte(`{}`), Active: true, WorkspaceID: &ws}
		if err := app.W.Tabs.Save(bridge.TabsSaveArgs{WindowKey: key, Tabs: []model.TabRecord{rec}}); err != nil {
			t.Fatalf("Tabs.Save %s: %v", key, err)
		}
	}
	save("w-a", "tab-a")
	save("w-b", "tab-b")

	// A close ack for one window, with nothing parked on it, changes nothing for the other.
	app.W.Lifecycle.WindowFlushed(bridge.LifecycleWindowFlushedArgs{WindowKey: "w-a"})
	app.W.Lifecycle.WindowFlushed(bridge.LifecycleWindowFlushedArgs{WindowKey: "w-ghost"})

	mark := app.Events.Mark()
	app.W.TermRegistry.CloseWindow("w-a")
	for _, id := range []string{"t-a1", "t-a2"} {
		if _, ok := app.W.TermRegistry.WindowOf(id); ok {
			t.Fatalf("terminal %s outlived its window", id)
		}
	}
	if got, ok := app.W.TermRegistry.WindowOf("t-b1"); !ok || got != "w-b" {
		t.Fatalf("closing w-a touched w-b's terminal: %q, %v", got, ok)
	}
	if got := tabIDs(t, app, "w-a"); !slices.Equal(got, []string{"tab-a"}) {
		t.Fatalf("w-a tabs = %v", got)
	}
	if got := tabIDs(t, app, "w-b"); !slices.Equal(got, []string{"tab-b"}) {
		t.Fatalf("w-b tabs = %v", got)
	}

	// Settings are app-wide: one broadcast, not addressed to either window.
	on := true
	if _, err := app.W.Settings.Set(bridge.SettingsSetArgs{Patch: model.SettingsPatch{Agent: &model.AgentPatch{KeepAwakeWithAgents: &on}}}); err != nil {
		t.Fatal(err)
	}
	ev := app.Events.WaitAfter(t, mark, appevent.ChannelSettingsChanged, nil, waitFor)
	if ev.Window != "" || ev.Focused {
		t.Fatalf("settings change addressed to %q (focused %v), want a broadcast", ev.Window, ev.Focused)
	}
	app.Restart()
	if got := tabIDs(t, app, "w-b"); !slices.Equal(got, []string{"tab-b"}) {
		t.Fatalf("w-b tabs after restart = %v", got)
	}
}

func TestFocusSessionAcrossWindows(t *testing.T) {
	app := flowharness.New(t, flowharness.WithTrackerGrace(time.Second))
	app.Scenario(fakeagent.Scenario{Claude: map[string][]fakeagent.Action{"*": {{Name: "done"}, {Name: "sleep"}}}})
	bare := app.NewBare("api-remote")
	seed := app.NewRepo("api-seed")
	seed.Commit("base", map[string]string{"base.txt": "base\n"})
	bare.PushFrom(seed)
	repo := bare.Clone(filepath.Join(app.Work, "api"))
	rec, err := app.W.CodeWorkspace.ImportRepo(ctx, bridge.CodeWorkspaceImportArgs{Path: repo.Dir})
	if err != nil {
		t.Fatal(err)
	}
	const flow = "id: flow\nname: Flow\nsteps:\n  - id: one\n    name: one\n    kind: agent\n    runs_on: each repo\n    timeout: 1m\n    prompt: work on {branch} in {repo}\n"
	if entry, err := app.W.AdeTask.SaveWorkflowYaml(ctx, adewire.SaveWorkflowYamlArgs{FileName: "flow.yaml", Yaml: flow}); err != nil || entry.Error != nil {
		t.Fatalf("SaveWorkflowYaml: %v %+v", err, entry.Error)
	}
	task, err := app.W.AdeTask.CreateTask(ctx, adewire.CreateTaskArgs{Title: "Fix login", CodeRepoIDs: []string{rec.ID}, WorkflowID: "flow"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := app.W.AdeTask.Board(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var branchID string
	for _, br := range b.Branches {
		if br.TaskID == task.ID {
			branchID = br.ID
		}
	}
	for _, key := range []string{"w-a", "w-b"} {
		ensure(t, app, key)
		app.WindowMgr.OpenWindow(shell.WindowRecord{Key: key})
	}
	// Starting a branch session needs its worktree, which a started run creates.
	if _, err := app.W.AdeTask.StartRun(ctx, adewire.StartRunArgs{TaskID: task.ID, BranchNames: map[string]string{branchID: "feat/api-focus"}}); err != nil {
		t.Fatal(err)
	}
	testx.WaitUntil(t, waitFor, func() bool {
		bd, _ := app.W.AdeTask.Board(ctx)
		for _, tk := range bd.Tasks {
			for _, r := range tk.Runs {
				if r.State == "done" {
					return true
				}
			}
		}
		return false
	})
	var launch adewire.Launch
	testx.WaitUntil(t, waitFor, func() bool {
		var err error
		launch, err = app.W.AdeTask.StartBranch(ctx, adewire.StartBranchArgs{BranchID: branchID})
		return err == nil
	})
	if _, err := app.W.Terminal.Open(terminal.OpenArgs{
		TerminalID: launch.TerminalID, Cwd: launch.Cwd, Cols: 100, Rows: 30, WindowKey: "w-a",
		Command: launch.Command, LaunchKind: terminal.LaunchKindAgent,
	}); err != nil {
		t.Fatal(err)
	}
	testx.WaitUntil(t, waitFor, func() bool {
		res, _ := app.W.AdeTask.Sessions(ctx)
		for _, s := range res.Sessions {
			if s.ID == launch.SessionID {
				return s.State == "running"
			}
		}
		return false
	})

	mark := app.Events.Mark()
	focused, err := app.W.AdeTask.FocusSession(ctx, adewire.FocusSessionArgs{SessionID: launch.SessionID})
	if err != nil || !focused {
		t.Fatalf("FocusSession = %v, %v", focused, err)
	}
	if got := app.WindowMgr.Focused; len(got) == 0 || got[len(got)-1] != "w-a" {
		t.Fatalf("focused %v, want w-a", got)
	}
	app.Events.WaitAfter(t, mark, adewire.ChannelOpenSession, nil, waitFor)
	for _, ev := range app.Events.Since(mark, adewire.ChannelOpenSession) {
		if ev.Window != "w-a" {
			t.Fatalf("open-session reached %q, want only w-a", ev.Window)
		}
	}

	// A window that closed meanwhile reads as false, never an error.
	app.WindowMgr.CloseWindow("w-a")
	if focused, err := app.W.AdeTask.FocusSession(ctx, adewire.FocusSessionArgs{SessionID: launch.SessionID}); err != nil || focused {
		t.Fatalf("FocusSession after its window closed = %v, %v, want false", focused, err)
	}
}

func TestOpenNewWindow(t *testing.T) {
	app := flowharness.New(t)
	if err := app.W.WindowsSvc.OpenNew(); err != nil {
		t.Fatal(err)
	}
	if app.WindowMgr.NewWindows != 1 {
		t.Fatalf("OpenNewWindow calls = %d, want 1", app.WindowMgr.NewWindows)
	}
	// The page the shell opens registers itself on boot.
	ensure(t, app, "w-new")
	if res, err := app.W.WindowsSvc.Ensure(windowsvc.EnsureArgs{WindowKey: "w-new"}); err != nil || res.Mode != "git" {
		t.Fatalf("Ensure of the new window = %+v, %v, want mode git", res, err)
	}
	if err := (&windowsvc.Service{}).OpenNew(); err == nil {
		t.Fatal("OpenNew without a shell succeeded")
	}
}

func quitPrompts(app *flowharness.App) []prompts.Routed {
	var out []prompts.Routed
	for _, p := range app.W.Prompts.List() {
		if p.Kind == prompts.KindQuit {
			out = append(out, p)
		}
	}
	return out
}
