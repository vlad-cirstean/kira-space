package journeyflow_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
	"github.com/kirathecat/kira-studio/internal/testx"
)

var ctx = context.Background()

const waitFor = 20 * time.Second

// workflowYAML: an automated step, then two manual steps.
const workflowYAML = `id: flow
name: Flow
steps:
  - id: one
    name: one
    kind: agent
    status: in-progress
    runs_on: each repo
    timeout: 1m
    prompt: work on {branch} in {repo}
  - id: review
    name: Review
    kind: manual
    status: in-review
    session: true
  - id: ship
    name: Ship
    kind: manual
    status: in-review
    session: true
`

func claude(app *flowharness.App, actions ...fakeagent.Action) {
	app.Scenario(fakeagent.Scenario{Claude: map[string][]fakeagent.Action{"*": actions}})
}

func importRepo(t *testing.T, app *flowharness.App, dir string) model.CodeRepo {
	t.Helper()
	rec, err := app.W.CodeWorkspace.ImportRepo(ctx, bridge.CodeWorkspaceImportArgs{Path: dir})
	if err != nil {
		t.Fatalf("ImportRepo %s: %v", dir, err)
	}
	return rec
}

// cloneWithMain returns a clone at dir of a fresh bare remote holding one commit on main.
func cloneWithMain(app *flowharness.App, dir string) (*flowharness.Repo, *flowharness.Bare) {
	name := filepath.Base(dir)
	bare := app.NewBare(name + "-remote")
	seed := app.NewRepo(name + "-seed")
	seed.Commit("base", map[string]string{"base.txt": "base\n"})
	bare.PushFrom(seed)
	return bare.Clone(dir), bare
}

func saveWorkflow(t *testing.T, app *flowharness.App, yaml string) {
	t.Helper()
	entry, err := app.W.AdeTask.SaveWorkflowYaml(ctx, adewire.SaveWorkflowYamlArgs{FileName: "flow.yaml", Yaml: yaml})
	if err != nil || entry.Error != nil {
		t.Fatalf("SaveWorkflowYaml: %v %+v", err, entry.Error)
	}
}

func board(t *testing.T, app *flowharness.App) adewire.Board {
	t.Helper()
	b, err := app.W.AdeTask.Board(ctx)
	if err != nil {
		t.Fatalf("Board: %v", err)
	}
	return b
}

func branchOf(t *testing.T, b adewire.Board, taskID, repoID string) adewire.Branch {
	t.Helper()
	for _, br := range b.Branches {
		if br.TaskID == taskID && br.CodeRepoID == repoID {
			return br
		}
	}
	t.Fatalf("board has no branch of task %s in repo %s", taskID, repoID)
	return adewire.Branch{}
}

func taskOf(t *testing.T, b adewire.Board, taskID string) adewire.Task {
	t.Helper()
	for _, tk := range b.Tasks {
		if tk.ID == taskID {
			return tk
		}
	}
	t.Fatalf("board has no task %s", taskID)
	return adewire.Task{}
}

func waitRun(t *testing.T, app *flowharness.App, taskID, stepID string, states ...string) adewire.Run {
	t.Helper()
	var got adewire.Run
	testx.WaitUntil(t, waitFor, func() bool {
		found := false
		for _, tk := range board(t, app).Tasks {
			if tk.ID != taskID {
				continue
			}
			for _, r := range tk.Runs {
				if r.StepID == stepID && (!found || r.Attempt >= got.Attempt) {
					got, found = r, true
				}
			}
		}
		if !found {
			return false
		}
		for _, s := range states {
			if got.State == s {
				return true
			}
		}
		return false
	})
	return got
}

func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s in %s: %v\n%s", strings.Join(args, " "), dir, err, out)
	}
	return strings.TrimSpace(string(out))
}

func exists(p string) bool { _, err := os.Stat(p); return err == nil }
