package mobileflow_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
	"github.com/kirathecat/kira-studio/internal/terminal"
	"github.com/kirathecat/kira-studio/internal/testx"
)

const (
	desktopWindow = "w-main"
	adeFlow       = `id: flow
name: Flow
steps:
  - id: one
    name: one
    kind: agent
    runs_on: each repo
    timeout: 1m
    prompt: work on {branch} in {repo}
  - id: review
    name: Review
    kind: manual
    status: in-review
    session: true
`
)

// fixture is a task on a real repo with a worktree branch, a finished headless run and a
// registered desktop window. Interactive sessions started later print the banner and wait for hold.
type fixture struct {
	app    *flowharness.App
	task   adewire.Task
	branch adewire.Branch
	runID  string
	banner string
	hold   string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	dir := t.TempDir()
	f := &fixture{banner: "fake agent ready\n", hold: filepath.Join(dir, "hold")}
	bannerFile := filepath.Join(dir, "banner")
	if err := os.WriteFile(bannerFile, []byte(f.banner), 0o644); err != nil {
		t.Fatal(err)
	}
	app := flowharness.New(t)
	f.app = app
	app.Scenario(fakeagent.Scenario{Claude: map[string][]fakeagent.Action{"*": {
		{Name: "done"}, {Emit: bannerFile, WaitFile: f.hold},
	}}})
	bare := app.NewBare("api-remote")
	seed := app.NewRepo("api-seed")
	seed.Commit("base", map[string]string{"base.txt": "base\n"})
	bare.PushFrom(seed)
	repo := bare.Clone(filepath.Join(app.Work, "api"))
	rec, err := app.W.CodeWorkspace.ImportRepo(ctx, bridge.CodeWorkspaceImportArgs{Path: repo.Dir})
	if err != nil {
		t.Fatal(err)
	}
	entry, err := app.W.AdeTask.SaveWorkflowYaml(ctx, adewire.SaveWorkflowYamlArgs{FileName: "flow.yaml", Yaml: adeFlow})
	if err != nil || entry.Error != nil {
		t.Fatalf("SaveWorkflowYaml: %v %+v", err, entry.Error)
	}
	if f.task, err = app.W.AdeTask.CreateTask(ctx, adewire.CreateTaskArgs{Title: "Fix login", CodeRepoIDs: []string{rec.ID}, WorkflowID: "flow"}); err != nil {
		t.Fatal(err)
	}
	f.branch = f.branchOf(t)
	if _, err := app.W.AdeTask.StartRun(ctx, adewire.StartRunArgs{TaskID: f.task.ID, BranchNames: map[string]string{f.branch.ID: "feat/api-work"}}); err != nil {
		t.Fatal(err)
	}
	testx.WaitUntil(t, waitFor, func() bool {
		for _, r := range f.taskNow(t).Runs {
			if r.StepID == "one" && r.State == "done" {
				f.runID = r.ID
				return true
			}
		}
		return false
	})
	f.branch = f.branchOf(t)
	if f.branch.Worktree == "" {
		t.Fatalf("branch has no worktree: %+v", f.branch)
	}
	app.W.Windows.Add(desktopWindow, 0, nil, func() {})
	return f
}

func (f *fixture) board(t *testing.T) adewire.Board {
	t.Helper()
	b, err := f.app.W.AdeTask.Board(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func (f *fixture) branchOf(t *testing.T) adewire.Branch {
	t.Helper()
	for _, br := range f.board(t).Branches {
		if br.TaskID == f.task.ID {
			return br
		}
	}
	t.Fatal("task has no branch")
	return adewire.Branch{}
}

func (f *fixture) taskNow(t *testing.T) adewire.Task {
	t.Helper()
	for _, tk := range f.board(t).Tasks {
		if tk.ID == f.task.ID {
			return tk
		}
	}
	t.Fatal("task missing from the board")
	return adewire.Task{}
}

// openLaunch does what the desktop window does with a Launch: open the agent terminal it describes.
func (f *fixture) openLaunch(t *testing.T, l adewire.Launch) {
	t.Helper()
	_, err := f.app.W.Terminal.Open(terminal.OpenArgs{
		TerminalID: l.TerminalID, Cwd: l.Cwd, Cols: 100, Rows: 30, WindowKey: desktopWindow,
		Command: l.Command, LaunchKind: terminal.LaunchKindAgent,
	})
	if err != nil {
		t.Fatalf("Terminal.Open: %v", err)
	}
}

// startSession starts an interactive session on the task branch and waits until it runs.
func (f *fixture) startSession(t *testing.T) adewire.Launch {
	t.Helper()
	l, err := f.app.W.AdeTask.StartBranch(ctx, adewire.StartBranchArgs{BranchID: f.branch.ID})
	if err != nil {
		t.Fatal(err)
	}
	f.openLaunch(t, l)
	testx.WaitUntil(t, waitFor, func() bool {
		res, _ := f.app.W.AdeTask.Sessions(ctx)
		for _, s := range res.Sessions {
			if s.ID == l.SessionID {
				return s.State == "running"
			}
		}
		return false
	})
	return l
}

func (f *fixture) release(t *testing.T) {
	t.Helper()
	if err := os.WriteFile(f.hold, nil, 0o644); err != nil {
		t.Fatal(err)
	}
}
