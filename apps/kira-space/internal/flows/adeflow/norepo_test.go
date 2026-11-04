package adeflow_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/agentmcp"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
	"github.com/kirathecat/kira-studio/internal/shell"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// P467 start a workflow without a repo. Contract ade-norepo: tests/ui/ade-v2-norepo.spec.ts reads these.

const nrScenario = "ade-norepo"

var nrMask = flowharness.Mask("terminalId", "command", "cwd", "createdAt", "expiresAt", "startedAt", "finishedAt", "addedAt", "updatedAt",
	"lastActiveAt", "worktree")

type nrFixture struct {
	app  *flowharness.App
	api  string
	task adewire.Task
	dir  string // the task folder
}

// newNRFixture makes a task with no repo on a workflow of steps, and one registered repo to pick later.
func newNRFixture(t *testing.T, scen map[string][]fakeagent.Action, steps string) *nrFixture {
	t.Helper()
	app := flowharness.New(t)
	claude(app, scen)
	repo, _ := cloneWithMain(app, filepath.Join(app.Work, "api"))
	api := importRepo(t, app, repo.Dir)
	saveWorkflow(t, app, "flow", flowYAML("flow", steps))
	task := createTask(t, app, "Fix login", "flow")
	return &nrFixture{app: app, api: api.ID, task: task, dir: filepath.Join(app.SpaceHome, "ade", "tasks", task.ID)}
}

// waitFakeText waits until the fake agent file name holds want.
func waitFakeText(t *testing.T, app *flowharness.App, name, want string) {
	t.Helper()
	testx.WaitUntil(t, waitFor, func() bool {
		raw, _ := os.ReadFile(filepath.Join(app.FakeDir, name))
		return strings.Contains(string(raw), want)
	})
}

func TestRepolessWorkflow(t *testing.T) {
	t.Run("agent stages: the first picks repos, the next runs on them", func(t *testing.T) {
		pick := fakeagent.MCPCall{Server: agentmcp.ServerName, Tool: "declare_repos", Args: map[string]any{"repos": []any{"api"}}}
		// The agent holds its MCP calls until the "started" snapshot is taken, else declare_repos races it.
		hold := filepath.Join(t.TempDir(), "hold")
		t.Cleanup(func() { _ = os.WriteFile(hold, nil, 0o644) })
		f := newNRFixture(t, map[string][]fakeagent.Action{
			"tasks": {{WaitBefore: hold, MCP: []fakeagent.MCPCall{pick, requestCall("api")}, Name: "done"}},
			"api":   {{Name: "done"}},
		}, onceStepYML+agentStep("two", ""))
		if _, err := f.app.W.AdeTask.StartRun(ctx, adewire.StartRunArgs{TaskID: f.task.ID, BranchNames: map[string]string{}}); err != nil {
			t.Fatalf("StartRun: %v", err)
		}
		one, _ := latestRun(t, f.app, f.task.ID, "one")
		if one.BranchID != "" {
			t.Fatalf("run branch = %q, want none", one.BranchID)
		}
		if s := sessionByID(t, f.app, one.SessionID); s.Cwd != f.dir {
			t.Fatalf("session cwd = %q, want the task folder %q", s.Cwd, f.dir)
		}
		if st, err := os.Stat(f.dir); err != nil || !st.IsDir() {
			t.Fatalf("task folder: %v", err)
		}
		waitFakeText(t, f.app, "tasks-1.prompt", "No repo yet. Folder: "+f.dir)
		f.app.Contract(t, nrScenario, "AdeTaskService.Board#started", taskOf(t, board(t, f.app), f.task.ID), nrMask)
		if err := os.WriteFile(hold, nil, 0o644); err != nil {
			t.Fatal(err)
		}

		var id string
		testx.WaitUntil(t, waitFor, func() bool {
			id = taskOf(t, board(t, f.app), f.task.ID).BranchRequestID
			return id != ""
		})
		if _, err := f.app.W.AdeTask.ApproveBranchRequest(adewire.ApproveBranchRequestArgs{ID: id, CodeRepoIDs: []string{f.api}}); err != nil {
			t.Fatal(err)
		}
		waitRun(t, f.app, f.task.ID, "one", "done")
		two := waitRun(t, f.app, f.task.ID, "two", "done")
		br := branchOf(t, board(t, f.app), f.task.ID, f.api)
		if br.Worktree == "" || two.BranchID != br.ID {
			t.Fatalf("stage two = %+v, want a run on the new branch %+v", two, br)
		}
		if one, _ = latestRun(t, f.app, f.task.ID, "one"); one.BranchID != "" || one.State != "done" {
			t.Fatalf("stage one = %+v, want it kept as the folder run", one)
		}
		f.app.Contract(t, nrScenario, "AdeTaskService.Board#advanced", taskOf(t, board(t, f.app), f.task.ID), nrMask)
	})

	t.Run("manual session stage opens in the task folder", func(t *testing.T) {
		f := newNRFixture(t, nil, "  - id: work\n    name: Work\n    kind: manual\n    session: true\n")
		f.app.WindowMgr.OpenWindow(shell.WindowRecord{Key: sessionWindow})
		launch, err := f.app.W.AdeTask.LaunchStep(ctx, adewire.LaunchStepArgs{TaskID: f.task.ID})
		if err != nil {
			t.Fatal(err)
		}
		if launch.Cwd != f.dir {
			t.Fatalf("launch cwd = %q, want %q", launch.Cwd, f.dir)
		}
		f.app.Contract(t, nrScenario, "AdeTaskService.LaunchStep", launch, nrMask)
		openLaunch(t, f.app, launch)
		if s := waitSession(t, f.app, launch.SessionID, "running"); s.BranchID != "" || s.Cwd != f.dir {
			t.Fatalf("session = %+v, want a task-level session in the task folder", s)
		}

		// Archive removes the folder.
		if err := f.app.W.AdeTask.ArchiveTask(ctx, adewire.TaskArgs{TaskID: f.task.ID}); err != nil {
			t.Fatalf("ArchiveTask: %v", err)
		}
		if _, err := os.Stat(f.dir); !os.IsNotExist(err) {
			t.Fatalf("task folder after archive: %v, want it removed", err)
		}
	})

	t.Run("review step needs a branch", func(t *testing.T) {
		f := newNRFixture(t, nil, "  - id: review\n    name: review\n    kind: review\n    runs_on: each repo\n    timeout: 1m\n    review:\n      send_to: new\n")
		_, err := f.app.W.AdeTask.StartRun(ctx, adewire.StartRunArgs{TaskID: f.task.ID})
		if err == nil || !strings.Contains(err.Error(), "the review step needs a branch to review") {
			t.Fatalf("StartRun = %v, want the review refusal", err)
		}
		f.app.Contract(t, nrScenario, "error:AdeTaskService.StartRun#review", err.Error())
	})
}

// P510 outside-repo steps run once in the task folder, even on a task with branches. Contract ade-norepo.
func TestOutsideRepoStep(t *testing.T) {
	const outsideAgent = "  - id: one\n    name: one\n    kind: agent\n    outside_repo: true\n    timeout: 1m\n    prompt: survey the folder\n"
	taskDir := func(f *runFixture) string { return filepath.Join(f.app.SpaceHome, "ade", "tasks", f.taskID) }

	t.Run("agent step runs in the task folder, the next on the branch", func(t *testing.T) {
		f := newRunFixture(t, []fakeagent.Action{{Name: "done"}}, outsideAgent+agentStep("two", ""))
		f.start(t, "feat/api-out")
		one := waitRun(t, f.app, f.taskID, "one", "done")
		two := waitRun(t, f.app, f.taskID, "two", "done")
		br := branchOf(t, board(t, f.app), f.taskID, f.repoID)
		if one.BranchID != "" || two.BranchID != br.ID {
			t.Fatalf("branches = %q and %q, want none and %q", one.BranchID, two.BranchID, br.ID)
		}
		if s := sessionByID(t, f.app, one.SessionID); s.Cwd != taskDir(f) {
			t.Fatalf("session cwd = %q, want the task folder %q", s.Cwd, taskDir(f))
		}
		prompts, _ := filepath.Glob(filepath.Join(f.app.FakeDir, "*.prompt"))
		var outside, noRepo int
		for _, p := range prompts {
			raw, _ := os.ReadFile(p)
			if strings.Contains(string(raw), "Outside the repos. Folder: "+taskDir(f)) {
				outside++
			}
			if strings.Contains(string(raw), "No repo yet") {
				noRepo++
			}
		}
		if outside != 1 || noRepo != 0 {
			t.Fatalf("prompts: %d outside-repo, %d no-repo, want 1 and 0 (%v)", outside, noRepo, prompts)
		}
		f.app.Contract(t, nrScenario, "AdeTaskService.Board#outside", taskOf(t, board(t, f.app), f.taskID), nrMask)
	})

	t.Run("script step runs in the task folder", func(t *testing.T) {
		step := "  - id: one\n    name: one\n    kind: script\n    outside_repo: true\n    timeout: 1m\n    command: 'printenv KIRA_WORKTREE_PATH > .where'\n"
		f := newRunFixture(t, nil, step)
		f.start(t, "feat/api-out")
		if r := waitRun(t, f.app, f.taskID, "one", "done"); r.BranchID != "" {
			t.Fatalf("run branch = %q, want none", r.BranchID)
		}
		raw, err := os.ReadFile(filepath.Join(taskDir(f), ".where"))
		if err != nil || strings.TrimSpace(string(raw)) != taskDir(f) {
			t.Fatalf("script folder = %q (%v), want %q", raw, err, taskDir(f))
		}
	})

	t.Run("manual session step opens in the task folder", func(t *testing.T) {
		f := newRunFixture(t, nil, "  - id: work\n    name: Work\n    kind: manual\n    session: true\n    outside_repo: true\n")
		f.app.WindowMgr.OpenWindow(shell.WindowRecord{Key: sessionWindow})
		launch, err := f.app.W.AdeTask.LaunchStep(ctx, adewire.LaunchStepArgs{TaskID: f.taskID})
		if err != nil {
			t.Fatal(err)
		}
		if launch.Cwd != taskDir(f) {
			t.Fatalf("launch cwd = %q, want %q", launch.Cwd, taskDir(f))
		}
	})
}
