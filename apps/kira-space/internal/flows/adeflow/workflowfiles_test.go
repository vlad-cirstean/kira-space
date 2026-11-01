package adeflow_test

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/configfs/configsvc"
	"github.com/kirathecat/kira-studio/internal/testx"
)

func lastValidRow(t *testing.T, app *flowharness.App, file string) (string, bool) {
	t.Helper()
	var js string
	err := app.DB().QueryRow(`SELECT workflow_json FROM ade_workflow_last_valid WHERE file_name = ?`, file).Scan(&js)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false
	}
	if err != nil {
		t.Fatal(err)
	}
	return js, true
}

func fileEntry(t *testing.T, app *flowharness.App, file string) (adewire.WorkflowEntry, bool) {
	t.Helper()
	list, err := app.W.AdeTask.Workflows(ctx)
	if err != nil {
		t.Fatalf("Workflows: %v", err)
	}
	for _, e := range list.Workflows {
		if e.FileName == file {
			return e, true
		}
	}
	return adewire.WorkflowEntry{}, false
}

// waitWorkflowsEvent waits for the watcher's workflows channel push after mark.
func waitWorkflowsEvent(t *testing.T, app *flowharness.App, mark int) {
	t.Helper()
	app.Events.WaitAfter(t, mark, adewire.ChannelWorkflows, nil, waitFor)
}

// TestWorkflowFilesSync: an external edit of a workflow file reaches the DB projection and the
// workflows channel, a broken edit keeps the last good version, and a deleted file is forgotten.
func TestWorkflowFilesSync(t *testing.T) {
	app := flowharness.New(t)
	dir := filepath.Join(app.SpaceHome, "workflows")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Give the watcher a moment to arm on the new dir before the first external write.
	time.Sleep(300 * time.Millisecond)
	path := filepath.Join(dir, "ext.yaml")
	good := flowYAML("ext", manualStep)

	mark := app.Events.Mark()
	if err := os.WriteFile(path, []byte(good), 0o644); err != nil {
		t.Fatal(err)
	}
	waitWorkflowsEvent(t, app, mark)
	testx.WaitUntil(t, waitFor, func() bool { _, ok := lastValidRow(t, app, "ext.yaml"); return ok })
	goodRow, _ := lastValidRow(t, app, "ext.yaml")
	if !strings.Contains(goodRow, `"id":"ext"`) {
		t.Fatalf("projected row = %s", goodRow)
	}

	bad := strings.Replace(good, "kind: manual", "kind: bogus", 1)
	badLine := 1 + strings.Count(bad[:strings.Index(bad, "kind: bogus")], "\n")
	mark = app.Events.Mark()
	if err := os.WriteFile(path, []byte(bad), 0o644); err != nil {
		t.Fatal(err)
	}
	waitWorkflowsEvent(t, app, mark)
	e, ok := fileEntry(t, app, "ext.yaml")
	if !ok || e.Error == nil || e.Error.Line != badLine {
		t.Fatalf("broken entry = %+v (want error at line %d)", e.Error, badLine)
	}
	if e.Workflow == nil || e.Workflow.ID != "ext" {
		t.Fatal("the last good workflow is not served while the file is broken")
	}
	if row, _ := lastValidRow(t, app, "ext.yaml"); row != goodRow {
		t.Fatalf("an invalid edit changed the projection: %s", row)
	}

	app.Restart()
	e, ok = fileEntry(t, app, "ext.yaml")
	if !ok || e.Error == nil || e.Workflow == nil {
		t.Fatalf("after restart: error %+v, workflow %v; want the last good workflow still served", e.Error, e.Workflow)
	}

	mark = app.Events.Mark()
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	waitWorkflowsEvent(t, app, mark)
	testx.WaitUntil(t, waitFor, func() bool { _, ok := lastValidRow(t, app, "ext.yaml"); return !ok })
}

// TestWorkflowSaveWritesAtomicallyOnce: an app save leaves no hidden temp file and one channel event.
func TestWorkflowSaveWritesAtomicallyOnce(t *testing.T) {
	app := flowharness.New(t)
	mark := app.Events.Mark()
	saveWorkflow(t, app, "saved", flowYAML("saved", manualStep))
	waitWorkflowsEvent(t, app, mark)
	time.Sleep(time.Second)
	if n := len(app.Events.Since(mark, adewire.ChannelWorkflows)); n != 1 {
		t.Errorf("%d workflows events for one save, want 1", n)
	}
	entries, err := os.ReadDir(filepath.Join(app.SpaceHome, "workflows"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() != "saved.yaml" {
			t.Errorf("unexpected file %s beside the saved workflow", e.Name())
		}
	}
}

// TestConfigFilesOpenReveal: the bound service resolves only a registered folder and a plain file.
func TestConfigFilesOpenReveal(t *testing.T) {
	app := flowharness.New(t)
	saveWorkflow(t, app, "shown", flowYAML("shown", manualStep))
	want := filepath.Join(app.SpaceHome, "workflows", "shown.yaml")
	args := configsvc.FileArgs{Folder: "workflows", File: "shown.yaml"}

	if err := app.W.ConfigFiles.Open(args); err != nil {
		t.Fatal(err)
	}
	if err := app.W.ConfigFiles.Reveal(args); err != nil {
		t.Fatal(err)
	}
	if got := app.FileOpener.OpenedFiles(); len(got) != 1 || got[0] != want {
		t.Errorf("opened = %v, want [%s]", got, want)
	}
	if got := app.FileOpener.RevealedFiles(); len(got) != 1 || got[0] != want {
		t.Errorf("revealed = %v, want [%s]", got, want)
	}

	for _, bad := range []configsvc.FileArgs{
		{Folder: "workflows", File: "../kira.db"},
		{Folder: "workflows", File: "/etc/passwd"},
		{Folder: "workflows", File: ".shown.yaml.tmp"},
		{Folder: "workflows", File: "missing.yaml"},
		{Folder: "nope", File: "shown.yaml"},
	} {
		if err := app.W.ConfigFiles.Open(bad); err == nil {
			t.Errorf("Open(%+v) accepted", bad)
		}
	}
	if got := app.FileOpener.OpenedFiles(); len(got) != 1 {
		t.Errorf("refused calls reached the OS: %v", got)
	}
}
