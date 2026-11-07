package repoflow_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
)

var ctx = context.Background()

// importVia imports dir the way the ADE Repositories page does: scripted folder picker, then ImportRepo.
func importVia(t *testing.T, app *flowharness.App, dir string) model.CodeRepo {
	t.Helper()
	app.Dialogs.AnswerDirectory(dir)
	pick, err := app.W.Files.ChooseFolder(bridge.FilesChooseFolderArgs{Title: "Add repository"})
	if err != nil || pick.Canceled || pick.Path == nil {
		t.Fatalf("ChooseFolder = %+v, %v", pick, err)
	}
	rec, err := app.W.CodeWorkspace.ImportRepo(ctx, bridge.CodeWorkspaceImportArgs{Path: *pick.Path})
	if err != nil {
		t.Fatalf("ImportRepo %s: %v", dir, err)
	}
	return rec
}

func errCode(err error) string {
	var e *ipcerr.Error
	if errors.As(err, &e) {
		return e.Code
	}
	return ""
}

func real(t *testing.T, p string) string {
	t.Helper()
	r, err := filepath.EvalSymlinks(p)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func idArgs(id string) bridge.CodeWorkspaceIDArgs { return bridge.CodeWorkspaceIDArgs{ID: id} }

func waitFor(t *testing.T, d time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("condition not met within %s", d)
}

func mustWrite(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}
