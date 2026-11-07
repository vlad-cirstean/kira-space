package usageflow_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

var ctx = context.Background()

const flowSteps = "id: flow\nname: Flow\nsteps:\n" +
	"  - id: one\n    name: one\n    kind: agent\n    runs_on: each repo\n    timeout: 1m\n    prompt: work on {branch} in {repo}\n" +
	"  - id: work\n    name: Work\n    kind: manual\n    status: in-progress\n    session: true\n"

// adeTask imports a clone, saves the two-step workflow and creates the task "Fix login".
func adeTask(t *testing.T, app *flowharness.App) (adewire.Task, model.CodeRepo) {
	t.Helper()
	dir := filepath.Join(app.Work, "api")
	bare := app.NewBare("api-remote")
	seed := app.NewRepo("api-seed")
	seed.Commit("base", map[string]string{"base.txt": "base\n"})
	bare.PushFrom(seed)
	repo := bare.Clone(dir)
	rec, err := app.W.CodeWorkspace.ImportRepo(ctx, bridge.CodeWorkspaceImportArgs{Path: repo.Dir})
	if err != nil {
		t.Fatalf("ImportRepo: %v", err)
	}
	entry, err := app.W.AdeTask.SaveWorkflowYaml(ctx, adewire.SaveWorkflowYamlArgs{FileName: "flow.yaml", Yaml: flowSteps})
	if err != nil || entry.Error != nil {
		t.Fatalf("SaveWorkflowYaml: %v %+v", err, entry.Error)
	}
	task, err := app.W.AdeTask.CreateTask(ctx, adewire.CreateTaskArgs{Title: "Fix login", CodeRepoIDs: []string{rec.ID}, WorkflowID: "flow"})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	return task, rec
}

func startRun(t *testing.T, app *flowharness.App, task adewire.Task, repoID string) {
	t.Helper()
	b, err := app.W.AdeTask.Board(ctx)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]string{}
	for _, br := range b.Branches {
		if br.TaskID == task.ID && br.CodeRepoID == repoID {
			names[br.ID] = "feat/api-work"
		}
	}
	if _, err := app.W.AdeTask.StartRun(ctx, adewire.StartRunArgs{TaskID: task.ID, BranchNames: names}); err != nil {
		t.Fatalf("StartRun: %v", err)
	}
}
