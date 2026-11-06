package ade

import (
	"context"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitcred"
	"github.com/kirathecat/kira-studio/internal/configfs"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adeflow"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitclient"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitsession"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/repos"
)

func waitUntil(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(45 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// Workflow files: app writes and external edits both reach Workflows and the channel; a broken
// edit keeps the last valid copy.
func TestWorkflows_writesAndExternalEdits(t *testing.T) {
	db, err := storage.OpenAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	r, err := repos.New(db.DB)
	if err != nil {
		t.Fatal(err)
	}
	registry := gitsession.NewRegistry(gitclient.NewExecRunner())
	t.Cleanup(registry.Close)
	dir := filepath.Join(t.TempDir(), "workflows")
	var emitted atomic.Int32
	b := NewTaskBoard(TaskBoardDeps{
		Agent:       testAgents,
		Credentials: gitcred.New(),
		Tasks:       r.AdeTasks, RepoConfig: r.AdeRepoConfig, Facts: r.AdeFacts, CodeRepos: r.CodeRepos,
		Logs:   r.AdeLogs,
		Runner: gitclient.NewExecRunner(), Registry: registry,
		GitStatus: func(context.Context) gitclient.GitStatus { return gitclient.GitStatus{Kind: "ok", Path: "git"} },
		Workflows: &adeflow.Reader{Dir: dir, Store: configfs.TableProjection{DB: db.DB, Folder: adeflow.WorkflowsFolder}}, OnWorkflows: func() { emitted.Add(1) },
		AutofetchMinutes: func() int { return 5 },
	})
	t.Cleanup(b.Close)
	b.Start()
	ctx := context.Background()

	e, err := b.NewWorkflow(ctx, adewire.NewWorkflowArgs{Name: "smoke"})
	if err != nil {
		t.Fatalf("NewWorkflow: %v", err)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("workflows dir not created: %v", err)
	}
	waitUntil(t, "emit after NewWorkflow", func() bool { return emitted.Load() >= 1 })

	valid, err := os.ReadFile(e.Path)
	if err != nil {
		t.Fatal(err)
	}
	before := emitted.Load()
	edited := strings.Replace(string(valid), "name:", "name: External", 1)
	if err := os.WriteFile(e.Path, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, "emit after external edit", func() bool { return emitted.Load() > before })
	res, err := b.Workflows(ctx)
	if err != nil || len(res.Workflows) != 1 || res.Workflows[0].Error != nil {
		t.Fatalf("Workflows after edit: %+v, %v", res, err)
	}

	before = emitted.Load()
	if err := os.WriteFile(e.Path, []byte("id: smoke\nsteps: [\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, "emit after broken edit", func() bool { return emitted.Load() > before })
	res, err = b.Workflows(ctx)
	if err != nil || len(res.Workflows) != 1 {
		t.Fatalf("Workflows after break: %+v, %v", res, err)
	}
	if got := res.Workflows[0]; got.Error == nil || got.Workflow == nil {
		t.Fatalf("broken edit must report the error and keep the last valid workflow: %+v", got)
	}
}
