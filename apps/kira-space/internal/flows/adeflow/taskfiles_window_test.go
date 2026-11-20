package adeflow_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/ade"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
)

// Backend half of the task Files window. Contract ade-task-files: tests/ui/ade-v2-task-files-window.spec.ts
// reads the open args and the window target as its mock and asserts the same result.
func TestTaskFilesWindow(t *testing.T) {
	f, br := doneFixture(t, "feat/api-files-window")
	app, svc := f.app, f.app.W.AdeTask
	taskArgs := adewire.TaskArgs{TaskID: f.taskID}
	opened := func() []string {
		var keys []string
		for _, r := range app.WindowMgr.Opened {
			if strings.HasPrefix(r.Key, ade.FilesWindowKeyPrefix) {
				keys = append(keys, r.Key)
			}
		}
		return keys
	}
	target := func(key string) *adewire.TaskFilesWindowTarget {
		t.Helper()
		got, err := svc.FilesWindowTarget(ctx, adewire.WindowKeyArgs{WindowKey: key})
		if err != nil {
			t.Fatal(err)
		}
		return got
	}

	app.Contract(t, taskFilesScenario, "args:AdeTaskService.OpenFilesWindow", taskArgs)
	if ok, err := svc.OpenFilesWindow(ctx, taskArgs); err != nil || !ok {
		t.Fatalf("first open = %v, %v, want true", ok, err)
	}
	keys := opened()
	if len(keys) != 1 {
		t.Fatalf("opened files windows = %v, want one", keys)
	}
	key := keys[0]
	if title := app.WindowMgr.Titles[key]; !strings.HasPrefix(title, "Files · ") {
		t.Fatalf("title = %q, want the Files · prefix", title)
	}
	if ok, err := svc.OpenFilesWindow(ctx, taskArgs); err != nil || ok {
		t.Fatalf("second open = %v, %v, want false (focus)", ok, err)
	}
	if got := app.WindowMgr.Focused; len(got) == 0 || got[len(got)-1] != key || len(opened()) != 1 {
		t.Fatalf("focused = %v, opened = %v, want a focus on %s and no second window", got, opened(), key)
	}
	got := target(key)
	if got == nil || got.TaskID != f.taskID {
		t.Fatalf("target = %+v, want task %s", got, f.taskID)
	}
	app.Contract(t, taskFilesScenario, "window", got)
	if target("no-such-window") != nil {
		t.Fatal("an unknown key has a target")
	}

	// A native close releases the file sessions and forgets the key.
	if _, err := svc.TaskFiles(ctx, adewire.BranchArgs{BranchID: br.ID}); err != nil {
		t.Fatal(err)
	}
	if app.W.AdeBoard.TaskFilesPeek(br.ID) == nil {
		t.Fatal("no file session after a listing")
	}
	app.CloseWindow(key)
	if app.W.AdeBoard.TaskFilesPeek(br.ID) != nil || target(key) != nil {
		t.Fatal("window close left a file session or a target")
	}
	if ok, err := svc.OpenFilesWindow(ctx, taskArgs); err != nil || !ok {
		t.Fatalf("reopen = %v, %v, want true", ok, err)
	}
	keys = opened()
	if len(keys) != 2 || keys[1] == key {
		t.Fatalf("opened = %v, want a new key after a close", keys)
	}
	key = keys[1]

	if err := svc.ArchiveTask(ctx, adewire.ArchiveTaskArgs{TaskID: taskArgs.TaskID}); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(app.WindowMgr.Closed, key) || target(key) != nil {
		t.Fatalf("archive left the files window: closed = %v", app.WindowMgr.Closed)
	}
	if _, err := svc.OpenFilesWindow(ctx, taskArgs); err == nil {
		t.Fatal("open on an archived task succeeded")
	}
}
