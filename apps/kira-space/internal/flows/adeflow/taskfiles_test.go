package adeflow_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
)

// Backend half of the task Files tab. Contract ade-task-files: tests/ui/ade-v2-task-files.spec.ts
// reads these answers as its mock and asserts the same result.
const taskFilesScenario = "ade-task-files"

func TestTaskFiles(t *testing.T) {
	f, br := doneFixture(t, "feat/api-files")
	app, wt := f.app, br.Worktree
	if err := os.MkdirAll(filepath.Join(wt, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeIn(t, wt, "README.md", "# Readme\nthe needle is here\n")
	writeIn(t, wt, filepath.Join("src", "a.go"), "package a\n\n// needle\n")
	if err := os.Symlink(t.TempDir(), filepath.Join(wt, "escape")); err != nil {
		t.Fatal(err)
	}
	branch := adewire.BranchArgs{BranchID: br.ID}
	mask := flowharness.Replace(wt, "<worktree>")

	app.Contract(t, taskFilesScenario, "args:AdeTaskService.TaskFiles", branch)
	list, err := app.W.AdeTask.TaskFiles(ctx, branch)
	if err != nil {
		t.Fatal(err)
	}
	sort.Slice(list.Paths, func(i, j int) bool { return list.Paths[i] < list.Paths[j] })
	for _, want := range []string{"README.md", "src/a.go"} {
		if !slices.Contains(list.Paths, want) {
			t.Fatalf("listing %v lacks %s", list.Paths, want)
		}
	}
	app.Contract(t, taskFilesScenario, "list", list, mask)

	readArgs := bridge.AdeTaskFileArgs{BranchID: br.ID, Path: "README.md"}
	app.Contract(t, taskFilesScenario, "args:AdeTaskService.TaskFileRead", readArgs)
	read, err := app.W.AdeTask.TaskFileRead(ctx, readArgs)
	if err != nil || read.Kind != "found" || read.Text != "# Readme\nthe needle is here\n" {
		t.Fatalf("read = %+v, %v", read, err)
	}
	app.Contract(t, taskFilesScenario, "read", read, mask)

	searchArgs := bridge.AdeTaskFilesSearchArgs{BranchID: br.ID, WindowKey: "w1", Query: "needle"}
	app.Contract(t, taskFilesScenario, "args:AdeTaskService.TaskFilesSearch", searchArgs)
	if h, err := app.W.AdeTask.TaskFilesSearch(ctx, searchArgs); err != nil || h.SearchID == "" {
		t.Fatalf("search = %+v, %v", h, err)
	}
	var found map[string][]int
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		var done bool
		if found, done = taskSearchHits(app, "w1"); done {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if len(found) != 2 || found["README.md"][0] != 2 || found["src/a.go"][0] != 3 {
		t.Fatalf("search hits = %v, want README.md:2 and src/a.go:3", found)
	}
	app.Contract(t, taskFilesScenario, "search", found)

	for _, bad := range []string{"../x", "escape"} {
		_, err := app.W.AdeTask.TaskFileRead(ctx, bridge.AdeTaskFileArgs{BranchID: br.ID, Path: bad})
		if errCode(err) != "E_BAD_REQUEST" {
			t.Fatalf("read %q = %v, want E_BAD_REQUEST", bad, err)
		}
	}
	_, err = app.W.AdeTask.TaskFileRead(ctx, bridge.AdeTaskFileArgs{BranchID: br.ID, Path: "../x"})
	app.Contract(t, taskFilesScenario, "refusal", map[string]string{"code": errCode(err), "message": errMessage(err)})

	if err := app.W.AdeTask.TaskFileReveal(ctx, bridge.AdeTaskFileArgs{BranchID: br.ID}); err != nil {
		t.Fatal(err)
	}
	if err := app.W.AdeTask.TaskFileReveal(ctx, bridge.AdeTaskFileArgs{BranchID: br.ID, Path: "src/a.go"}); err != nil {
		t.Fatal(err)
	}
	if got, want := app.FileOpener.RevealedFiles(), []string{wt, filepath.Join(wt, "src", "a.go")}; !slices.Equal(got, want) {
		t.Fatalf("revealed = %v, want %v", got, want)
	}
	if err := app.W.AdeTask.TaskFileReveal(ctx, bridge.AdeTaskFileArgs{BranchID: br.ID, Path: "nope.txt"}); errCode(err) != "E_NOT_FOUND" {
		t.Fatalf("reveal of a missing file = %v, want E_NOT_FOUND", err)
	}

	if err := app.W.AdeTask.TaskFileOpenInEditor(ctx, bridge.AdeTaskFileArgs{BranchID: br.ID, Path: "src/a.go"}); err != nil {
		t.Fatal(err)
	}
	if got := app.Editor.Opened(); len(got) != 1 || !slices.Equal(got[0], []string{wt, filepath.Join(wt, "src", "a.go")}) {
		t.Fatalf("editor opened = %v, want the worktree and the file", got)
	}
	app.Editor.SetMissing(true)
	err = app.W.AdeTask.TaskFileOpenInEditor(ctx, bridge.AdeTaskFileArgs{BranchID: br.ID})
	if errCode(err) != "E_NOT_FOUND" {
		t.Fatalf("open with VS Code missing = %v, want E_NOT_FOUND", err)
	}
	app.Contract(t, taskFilesScenario, "editorMissing", map[string]string{"code": errCode(err), "message": errMessage(err)})

	l := app.W.AdeTask.TaskFilesLaunchers()
	if l.Reveal != "fileManager" || !l.Editor {
		t.Fatalf("launchers = %+v, want fileManager and an editor on linux", l)
	}
	app.Contract(t, taskFilesScenario, "launchers", l)

	if app.W.AdeBoard.TaskFilesPeek(br.ID) == nil {
		t.Fatal("no file session after reads")
	}
	if err := app.W.AdeTask.ArchiveTask(ctx, adewire.TaskArgs{TaskID: f.taskID}); err != nil {
		t.Fatal(err)
	}
	if app.W.AdeBoard.TaskFilesPeek(br.ID) != nil {
		t.Fatal("file session survived the archive")
	}
}

func errMessage(err error) string {
	var ie *ipcerr.Error
	if errors.As(err, &ie) {
		return ie.Message
	}
	return ""
}

// taskSearchHits folds the kira:code:search pushes of one window into path to match lines.
func taskSearchHits(app *flowharness.App, window string) (hits map[string][]int, done bool) {
	hits = map[string][]int{}
	for _, ev := range app.Events.Since(0, bridge.ChannelCodeSearch) {
		if ev.Window != window {
			continue
		}
		p := ev.Data.(bridge.CodeSearchEvent)
		for _, fm := range p.Files {
			for _, m := range fm.Matches {
				hits[fm.Path] = append(hits[fm.Path], m.Line)
			}
		}
		done = done || p.Done
	}
	return hits, done
}
