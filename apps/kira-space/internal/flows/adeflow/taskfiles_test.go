package adeflow_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/codeworkspace"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
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
	sort.Strings(list.Paths)
	wantPaths := strings.Split(gitOut(t, wt, "ls-files", "--cached", "--others", "--exclude-standard"), "\n")
	sort.Strings(wantPaths)
	if !slices.Equal(list.Paths, wantPaths) {
		t.Fatalf("paths = %v, want git's %v", list.Paths, wantPaths)
	}
	if list.Status["README.md"] == "" || list.Status["base.txt"] != "" {
		t.Fatalf("status overlay = %v, want README.md marked and base.txt clean", list.Status)
	}
	for _, want := range []string{"README.md", "src/a.go"} {
		if !slices.Contains(list.Paths, want) {
			t.Fatalf("listing %v lacks %s", list.Paths, want)
		}
	}
	if list.Root != wt || list.Main {
		t.Fatalf("root = %q main = %v, want %q and false", list.Root, list.Main, wt)
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
	waitSearchDone(t, app, "w1")
	found, _, _ := taskSearchHits(app, "w1")
	if len(found) != 2 || found["README.md"][0] != 2 || found["src/a.go"][0] != 3 {
		t.Fatalf("search hits = %v, want README.md:2 and src/a.go:3", found)
	}
	for _, line := range strings.Split(gitOut(t, wt, "grep", "-n", "--untracked", "needle"), "\n") {
		parts := strings.SplitN(line, ":", 3)
		n, _ := strconv.Atoi(parts[1])
		if !slices.Contains(found[parts[0]], n) {
			t.Fatalf("git grep hit %s:%d missing from %v", parts[0], n, found)
		}
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
	goneDir := bridge.AdeTaskFileArgs{BranchID: br.ID, Path: "gone/dir/x.go"}
	if gone, err := app.W.AdeTask.TaskFileRead(ctx, goneDir); err != nil || gone.Kind != "missing" {
		t.Fatalf("read under a deleted directory = %+v, %v, want kind missing", gone, err)
	}
	if err := app.W.AdeTask.TaskFileReveal(ctx, goneDir); errCode(err) != "E_NOT_FOUND" {
		t.Fatalf("reveal under a deleted directory = %v, want E_NOT_FOUND", err)
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
	if err := archiveConfirmed(t, app, f.taskID); err != nil {
		t.Fatal(err)
	}
	if app.W.AdeBoard.TaskFilesPeek(br.ID) != nil {
		t.Fatal("file session survived the archive")
	}
}

// A row without a worktree opens its repo's main worktree; a missing main checkout is refused.
func TestTaskFilesMainWorktree(t *testing.T) {
	f := newRunFixture(t, []fakeagent.Action{{Name: "done"}}, agentStep("one", ""))
	app, dir := f.app, f.repo.Dir
	br := branchOf(t, board(t, app), f.taskID, f.repoID)
	if br.Name != "" || br.Worktree != "" {
		t.Fatalf("branch = %+v, want no name and no worktree", br)
	}
	if err := os.MkdirAll(filepath.Join(dir, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	commitIn(t, dir, filepath.Join("src", "main.go"))
	branch := adewire.BranchArgs{BranchID: br.ID}
	list, err := app.W.AdeTask.TaskFiles(ctx, branch)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Split(gitOut(t, dir, "ls-files", "--cached", "--others", "--exclude-standard"), "\n")
	got := slices.Clone(list.Paths)
	sort.Strings(got)
	sort.Strings(want)
	if !slices.Equal(got, want) || !slices.Contains(got, "src/main.go") {
		t.Fatalf("paths = %v, want git's %v", got, want)
	}
	if list.Root != dir || !list.Main {
		t.Fatalf("root = %q main = %v, want %q and true", list.Root, list.Main, dir)
	}
	app.Contract(t, taskFilesScenario, "list#main", list, flowharness.Replace(dir, "<main>"))

	if c, err := app.W.AdeTask.TaskFileRead(ctx, bridge.AdeTaskFileArgs{BranchID: br.ID, Path: "src/main.go"}); err != nil || c.Kind != "found" {
		t.Fatalf("read = %+v, %v", c, err)
	}
	if err := app.W.AdeTask.TaskFileReveal(ctx, bridge.AdeTaskFileArgs{BranchID: br.ID}); err != nil {
		t.Fatal(err)
	}
	if got := app.FileOpener.RevealedFiles(); !slices.Equal(got, []string{dir}) {
		t.Fatalf("revealed = %v, want %v", got, []string{dir})
	}

	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	_, err = app.W.AdeTask.TaskFiles(ctx, branch)
	if errCode(err) != "E_NOT_FOUND" {
		t.Fatalf("missing main = %v, want E_NOT_FOUND", err)
	}
	app.Contract(t, taskFilesScenario, "noMainWorktree", map[string]string{"code": errCode(err), "message": errMessage(err)})
}

func errMessage(err error) string {
	var ie *ipcerr.Error
	if errors.As(err, &ie) {
		return ie.Message
	}
	return ""
}

// waitSearchDone blocks until the window's search pushed its terminal event.
func waitSearchDone(t *testing.T, app *flowharness.App, window string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if _, done, _ := taskSearchHits(app, window); done {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("search in window %s never finished", window)
}

// taskSearchHits folds the kira:code:search pushes of one window into path to match lines, plus the
// total match count and whether the terminal event arrived.
func taskSearchHits(app *flowharness.App, window string) (hits map[string][]int, done bool, total int) {
	hits = map[string][]int{}
	for _, ev := range app.Events.Since(0, bridge.ChannelCodeSearch) {
		if ev.Window != window {
			continue
		}
		p := ev.Data.(bridge.CodeSearchEvent)
		for _, fm := range p.Files {
			for _, m := range fm.Matches {
				hits[fm.Path] = append(hits[fm.Path], m.Line)
				total++
			}
		}
		done = done || p.Done
	}
	return hits, done, total
}

// A search fills the worktree with matches; a bad regex is refused up front, Cancel and Close end a
// running search quietly (no error event, no more events).
func TestTaskFilesSearch(t *testing.T) {
	f, br := doneFixture(t, "feat/api-search")
	app, wt := f.app, br.Worktree
	line := strings.Repeat("x", 80) + " needle\n"
	for i := range 600 {
		dir := filepath.Join(wt, "d"+strconv.Itoa(i%20))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		writeIn(t, dir, "f"+strconv.Itoa(i)+".txt", strings.Repeat(line, 400))
	}
	start := func(window string) {
		t.Helper()
		if _, err := app.W.AdeTask.TaskFilesSearch(ctx, bridge.AdeTaskFilesSearchArgs{BranchID: br.ID, WindowKey: window, Query: "needle"}); err != nil {
			t.Fatal(err)
		}
	}
	quiet := func(window string) {
		t.Helper()
		waitSearchDone(t, app, window)
		var total int
		for _, ev := range app.Events.Since(0, bridge.ChannelCodeSearch) {
			if p := ev.Data.(bridge.CodeSearchEvent); ev.Window == window && p.Error != nil {
				t.Fatalf("stop surfaced as an error: %+v", p.Error)
			}
		}
		_, _, total = taskSearchHits(app, window)
		time.Sleep(300 * time.Millisecond)
		if _, _, after := taskSearchHits(app, window); after != total {
			t.Fatalf("events kept coming after done: %d -> %d", total, after)
		}
		if total >= codeworkspace.MaxSearchMatches {
			t.Fatalf("search ran to its cap (%d matches) instead of stopping", total)
		}
	}

	_, err := app.W.AdeTask.TaskFilesSearch(ctx, bridge.AdeTaskFilesSearchArgs{BranchID: br.ID, WindowKey: "w0", Query: "(", Regex: true})
	if errCode(err) != "E_BAD_REQUEST" {
		t.Fatalf("bad regex = %v, want E_BAD_REQUEST", err)
	}

	start("w1")
	if err := app.W.AdeTask.TaskFilesCancelSearch(adewire.BranchArgs{BranchID: br.ID}); err != nil {
		t.Fatal(err)
	}
	quiet("w1")

	start("w2")
	if err := app.W.AdeTask.TaskFilesClose(adewire.BranchArgs{BranchID: br.ID}); err != nil {
		t.Fatal(err)
	}
	quiet("w2")
}
