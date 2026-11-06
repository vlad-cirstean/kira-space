package journeyflow_test

import (
	"encoding/base64"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitrpc"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitsession"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
	"github.com/kirathecat/kira-studio/internal/terminal"
	"github.com/kirathecat/kira-studio/internal/testx"
	"github.com/kirathecat/kira-studio/internal/windowsvc"
)

const window = "w-main"

// graphSubjects pages one graph.stream and returns the commit subjects, newest first.
func graphSubjects(t *testing.T, gs *flowharness.GitStream, repoID string) []string {
	t.Helper()
	var out []string
	for _, c := range gs.Stream("graph.stream", map[string]any{"repoId": repoID}, 1).Drain() {
		_, rows := c.Graph(t)
		for _, r := range rows {
			out = append(out, r.Subject)
		}
	}
	return out
}

// A new user's first session: import, commit in a shell tab, push, run a task through its steps,
// archive it, relaunch. Everything the user set up must read back the same.
func TestFirstRunJourney(t *testing.T) {
	app := flowharness.New(t)
	claude(app, fakeagent.Action{Name: "done"})

	if res, err := app.W.WindowsSvc.Ensure(windowsvc.EnsureArgs{WindowKey: window}); err != nil || res.Mode != "git" {
		t.Fatalf("first Ensure = %+v, %v, want mode git", res, err)
	}
	repo, bare := cloneWithMain(app, filepath.Join(app.Work, "api"))
	rec := importRepo(t, app, repo.Dir)

	gs := app.OpenGitStream()
	var open gitrpc.RepoOpenResult
	gs.MustRequest("repo.open", gitrpc.RepoOpenParams{Path: repo.Dir}, &open)
	if open.Kind != "ok" || open.Repo == nil {
		t.Fatalf("repo.open = %+v", open)
	}
	gitID := open.Repo.RepoID
	if got := graphSubjects(t, gs, gitID); !slices.Equal(got, []string{"base"}) {
		t.Fatalf("graph before the commit = %v", got)
	}

	// The commit is made the way the user makes it: in a shell tab.
	if _, err := app.W.Terminal.Open(terminal.OpenArgs{TerminalID: "tab-1", WindowKey: window, Cwd: repo.Dir, Cols: 100, Rows: 30}); err != nil {
		t.Fatal(err)
	}
	cmd := "echo hello > hello.txt && git add -A && git commit -q -m 'first change'\n"
	if err := app.W.Terminal.Write(terminal.WriteArgs{TerminalID: "tab-1", Data: base64.StdEncoding.EncodeToString([]byte(cmd))}); err != nil {
		t.Fatal(err)
	}
	testx.WaitUntil(t, waitFor, func() bool {
		out, err := repo.GitErr("log", "-1", "--format=%s")
		return err == nil && out == "first change"
	})
	testx.WaitUntil(t, waitFor, func() bool {
		var res gitrpc.GraphRefreshResult
		return gs.Request("graph.refresh", gitrpc.GraphRefreshParams{RepoID: gitID}, &res) == nil &&
			slices.Equal(graphSubjects(t, gs, gitID), []string{"first change", "base"})
	})
	var push gitsession.RemoteOpResult
	gs.MustRequest("remote.run", gitrpc.RemoteRunParams{RepoID: gitID, RemoteOpParams: gitsession.RemoteOpParams{Kind: "push", Remote: "origin", Branch: "main"}}, &push)
	if !push.OK || bare.Git("rev-parse", "main") != repo.Git("rev-parse", "HEAD") {
		t.Fatalf("push = %+v, remote main %s, local %s", push, bare.Git("rev-parse", "main"), repo.Git("rev-parse", "HEAD"))
	}

	// A task on two repos.
	second, _ := cloneWithMain(app, filepath.Join(app.Work, "web"))
	rec2 := importRepo(t, app, second.Dir)
	saveWorkflow(t, app, workflowYAML)
	task, err := app.W.AdeTask.CreateTask(ctx, adewire.CreateTaskArgs{Title: "Fix login", CodeRepoIDs: []string{rec.ID}, WorkflowID: "flow"})
	if err != nil {
		t.Fatal(err)
	}
	title, notes, est := "Fix login flow", "see ticket", "2d"
	if upd, err := app.W.AdeTask.UpdateTask(ctx, adewire.UpdateTaskArgs{TaskID: task.ID, Patch: adewire.TaskPatch{Title: &title, Est: &est}}); err != nil ||
		upd.Title != title || upd.Est != est {
		t.Fatalf("UpdateTask = %+v, %v", upd, err)
	}
	if _, err := app.W.AdeTask.AddNote(ctx, adewire.AddNoteArgs{TaskID: task.ID, Body: notes}); err != nil {
		t.Fatalf("AddNote: %v", err)
	}
	if _, err := app.W.AdeTask.AddTaskRepo(ctx, adewire.AddTaskRepoArgs{TaskID: task.ID, CodeRepoID: rec2.ID}); err != nil {
		t.Fatalf("AddTaskRepo: %v", err)
	}
	b := board(t, app)
	if got := taskOf(t, b, task.ID); len(got.BranchIDs) != 2 {
		t.Fatalf("task branches = %v, want one per repo", got.BranchIDs)
	}
	names := map[string]string{
		branchOf(t, b, task.ID, rec.ID).ID:  "feat/api-login",
		branchOf(t, b, task.ID, rec2.ID).ID: "feat/web-login",
	}
	if _, err := app.W.AdeTask.StartRun(ctx, adewire.StartRunArgs{TaskID: task.ID, BranchNames: names}); err != nil {
		t.Fatal(err)
	}
	if run := waitRun(t, app, task.ID, "one", "done"); run.Summary != "summary-done" {
		t.Fatalf("run = %+v", run)
	}
	// waitRun saw one branch's run end; the step moves on only once every branch's run has.
	testx.WaitUntil(t, waitFor, func() bool {
		for _, r := range taskOf(t, board(t, app), task.ID).Runs {
			if r.StepID == "one" && r.State != "done" {
				return false
			}
		}
		return true
	})
	for _, want := range []string{"ship", "done"} {
		if got, err := app.W.AdeTask.StepDone(ctx, adewire.StepDoneArgs{TaskID: task.ID}); err != nil || got.EntryStepID != want {
			t.Fatalf("StepDone = %+v, %v, want entry step %s", got, err, want)
		}
	}
	if _, err := app.W.AdeTask.SetTaskStep(ctx, adewire.SetTaskStepArgs{TaskID: task.ID, StepID: "review", FromStepID: "done"}); err != nil {
		t.Fatalf("SetTaskStep back: %v", err)
	}
	if _, err := app.W.AdeTask.SetTaskStep(ctx, adewire.SetTaskStepArgs{TaskID: task.ID, StepID: "one", FromStepID: "ship"}); err == nil {
		t.Fatal("SetTaskStep from a stale step succeeded")
	}

	// A second task stays on the board; this one is archived.
	keep, err := app.W.AdeTask.CreateTask(ctx, adewire.CreateTaskArgs{Title: "Keep me", CodeRepoIDs: []string{rec.ID}, WorkflowID: "flow"})
	if err != nil {
		t.Fatal(err)
	}
	if err := app.W.AdeTask.ArchiveTask(ctx, adewire.TaskArgs{TaskID: task.ID}); err != nil {
		t.Fatalf("ArchiveTask: %v", err)
	}

	// Window state the user left behind.
	if err := app.W.WindowsSvc.SetMode(windowsvc.SetModeArgs{WindowKey: window, Mode: "ade"}); err != nil {
		t.Fatal(err)
	}
	ws := "ws"
	tabs := []model.TabRecord{{ID: "tab-1", Path: "/tab-1", Kind: "terminal", State: []byte(`{}`), Active: true, WorkspaceID: &ws}}
	if err := app.W.Tabs.Save(bridge.TabsSaveArgs{WindowKey: window, Tabs: tabs}); err != nil {
		t.Fatal(err)
	}
	width, on := 321.0, true
	if _, err := app.W.Layout.Set(bridge.LayoutSetArgs{Patch: model.LayoutPatch{Modes: map[string]model.PanelSizesPatch{"ade": {Project: &model.ProjectSizePatch{Width: &width}}}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.W.Settings.Set(bridge.SettingsSetArgs{Patch: model.SettingsPatch{Agent: &model.AgentPatch{KeepAwakeWithAgents: &on}}}); err != nil {
		t.Fatal(err)
	}
	snap := func() (adewire.Board, model.Layout, model.Settings, string, []string, []model.CodeRepo) {
		b := board(t, app)
		lay, err := app.W.Layout.GetAll()
		if err != nil {
			t.Fatal(err)
		}
		set, err := app.W.Settings.GetAll()
		if err != nil {
			t.Fatal(err)
		}
		ens, err := app.W.WindowsSvc.Ensure(windowsvc.EnsureArgs{WindowKey: window})
		if err != nil {
			t.Fatal(err)
		}
		list, err := app.W.Tabs.List(bridge.TabsListArgs{WindowKey: window})
		if err != nil {
			t.Fatal(err)
		}
		ids := []string{}
		for _, tb := range list {
			ids = append(ids, tb.ID)
		}
		repos, err := app.W.CodeWorkspace.ListRepos()
		if err != nil {
			t.Fatal(err)
		}
		return b, lay, set, ens.Mode, ids, repos
	}
	b1, lay1, set1, mode1, tabs1, repos1 := snap()
	if len(b1.Tasks) != 1 || b1.Tasks[0].ID != keep.ID || len(b1.History) != 1 || b1.History[0].TaskID != task.ID {
		t.Fatalf("board before restart: tasks %d, history %+v", len(b1.Tasks), b1.History)
	}

	gs.Close()
	app.Restart()
	b2, lay2, set2, mode2, tabs2, repos2 := snap()
	if !reflect.DeepEqual(b1.Tasks, b2.Tasks) || !reflect.DeepEqual(b1.History, b2.History) || !reflect.DeepEqual(b1.Branches, b2.Branches) {
		t.Fatalf("board changed across restart\n before: %+v\n after:  %+v", b1, b2)
	}
	if !reflect.DeepEqual(lay1, lay2) || lay2.Modes["ade"].Project.Width != 321 {
		t.Fatalf("layout = %+v, want %+v", lay2, lay1)
	}
	if !reflect.DeepEqual(set1, set2) || !set2.Agent.KeepAwakeWithAgents {
		t.Fatalf("settings changed across restart: %+v", set2)
	}
	if mode2 != mode1 || mode2 != "ade" || !slices.Equal(tabs1, tabs2) || len(tabs2) != 1 {
		t.Fatalf("window state after restart: mode %q tabs %v, was %q %v", mode2, tabs2, mode1, tabs1)
	}
	if !reflect.DeepEqual(repos1, repos2) || len(repos2) != 2 {
		t.Fatalf("repos after restart = %+v, want %+v", repos2, repos1)
	}
	gs2 := app.OpenGitStream()
	var reopen gitrpc.RepoOpenResult
	gs2.MustRequest("repo.open", gitrpc.RepoOpenParams{Path: repo.Dir}, &reopen)
	if reopen.Kind != "ok" || !slices.Equal(graphSubjects(t, gs2, reopen.Repo.RepoID), []string{"first change", "base"}) {
		t.Fatalf("graph after restart: %+v", reopen)
	}
}
