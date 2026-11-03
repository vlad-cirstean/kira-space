package windowflow_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/openedfiles"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
	"github.com/kirathecat/kira-studio/internal/testx"
)

func pendingFor(t *testing.T, app *flowharness.App, key string) []openedfiles.Target {
	t.Helper()
	got, err := app.W.OpenedFilesSvc.Pending(bridge.OpenedFilesPendingArgs{WindowKey: key})
	if err != nil {
		t.Fatalf("Pending %s: %v", key, err)
	}
	return got
}

func TestOpenedFilesWindow(t *testing.T) {
	app := flowharness.New(t)
	repo := app.NewRepo("proj")
	repo.Commit("init", map[string]string{"src/a.go": "package a\n"})
	rec, err := app.W.CodeWorkspace.ImportRepo(ctx, bridge.CodeWorkspaceImportArgs{Path: repo.Dir})
	if err != nil {
		t.Fatalf("ImportRepo: %v", err)
	}

	notes := filepath.Join(app.Work, "notes")
	if err := os.MkdirAll(filepath.Join(notes, "dir.json"), 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name, text string) string {
		p := filepath.Join(notes, name)
		if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	loose := write("b.json", "{\"k\": 1}\n")
	txt := write("x.txt", "prose\n")
	link := filepath.Join(notes, "link.go")
	if err := os.Symlink(filepath.Join(repo.Dir, "src", "a.go"), link); err != nil {
		t.Fatal(err)
	}
	keyed := func() []string { return app.WindowMgr.OpenedKeyed }

	app.W.OpenedFiles.Open([]string{filepath.Join(repo.Dir, "src", "a.go"), loose, txt, filepath.Join(notes, "dir.json")})
	if len(keyed()) != 1 {
		t.Fatalf("keyed windows = %d, want 1", len(keyed()))
	}
	key := keyed()[0]
	if !strings.HasPrefix(key, openedfiles.WindowKeyPrefix) {
		t.Fatalf("key = %q, want prefix %q", key, openedfiles.WindowKeyPrefix)
	}
	if got := app.WindowMgr.Titles[key]; got != "Opened files" {
		t.Fatalf("title = %q, want Opened files", got)
	}
	targets := pendingFor(t, app, key)
	app.Contract(t, "opened-files", "OpenedFilesService.Pending", targets)
	if len(targets) != 2 ||
		targets[0].RepoID != rec.ID || targets[0].RelPath != "src/a.go" || targets[0].Path != "" ||
		targets[1].Path != loose || targets[1].RepoID != "" {
		t.Fatalf("targets = %+v, want repo src/a.go then loose %s", targets, loose)
	}
	if again := pendingFor(t, app, key); len(again) != 0 {
		t.Fatalf("second Pending = %+v, want empty", again)
	}

	t.Run("read", func(t *testing.T) {
		got, err := app.W.OpenedFilesSvc.Read(bridge.OpenedFilesReadArgs{Path: loose})
		if err != nil || got.Kind != "found" || got.Language != "json" || got.Text != "{\"k\": 1}\n" {
			t.Fatalf("Read = %+v, %v, want found json", got, err)
		}
		app.Contract(t, "opened-files", "OpenedFilesService.Read", got)
		for _, bad := range []string{"notes/b.json", txt} {
			if _, err := app.W.OpenedFilesSvc.Read(bridge.OpenedFilesReadArgs{Path: bad}); err == nil {
				t.Fatalf("Read(%q) succeeded, want refusal", bad)
			}
		}
	})

	t.Run("window kind", func(t *testing.T) {
		args := bridge.OpenedFilesWindowArgs{WindowKey: key}
		app.Contract(t, "opened-files", "args:OpenedFilesService.IsFilesWindow", args, flowharness.Mask("windowKey"))
		got, err := app.W.OpenedFilesSvc.IsFilesWindow(args)
		if err != nil || !got {
			t.Fatalf("IsFilesWindow(%s) = %v, %v, want true", key, got, err)
		}
		app.Contract(t, "opened-files", "OpenedFilesService.IsFilesWindow", got)
		other, err := app.W.OpenedFilesSvc.IsFilesWindow(bridge.OpenedFilesWindowArgs{WindowKey: "main"})
		if err != nil || other {
			t.Fatalf("IsFilesWindow(main) = %v, %v, want false", other, err)
		}
		app.Contract(t, "opened-files", "OpenedFilesService.IsFilesWindow:other", other)
		_, err = app.W.OpenedFilesSvc.IsFilesWindow(bridge.OpenedFilesWindowArgs{})
		if e := testx.AsIpcErr(t, err); e.Code != string(ipcerr.CodeContract) {
			t.Fatalf("empty key code = %s, want %s", e.Code, ipcerr.CodeContract)
		}
	})

	t.Run("a later open reuses the window", func(t *testing.T) {
		mark := app.Events.Mark()
		app.W.OpenedFiles.Open([]string{link})
		if len(keyed()) != 1 {
			t.Fatalf("keyed windows = %d, want 1", len(keyed()))
		}
		if f := app.WindowMgr.Focused; len(f) == 0 || f[len(f)-1] != key {
			t.Fatalf("focused = %v, want last %s", f, key)
		}
		evs := app.Events.Since(mark, openedfiles.ChannelArrived)
		if len(evs) != 1 || evs[0].Window != key {
			t.Fatalf("arrived events = %+v, want one to %s", evs, key)
		}
		got := pendingFor(t, app, key)
		app.Contract(t, "opened-files", "OpenedFilesService.Pending:reuse", got)
		if len(got) != 1 || got[0].RepoID != rec.ID || got[0].RelPath != "src/a.go" {
			t.Fatalf("targets = %+v, want repo src/a.go", got)
		}
	})

	t.Run("nothing qualifying touches no window", func(t *testing.T) {
		mark := app.Events.Mark()
		focused := len(app.WindowMgr.Focused)
		app.W.OpenedFiles.Open([]string{txt})
		if len(keyed()) != 1 || len(app.WindowMgr.Focused) != focused || len(app.Events.Since(mark, openedfiles.ChannelArrived)) != 0 {
			t.Fatalf("a non-qualifying open touched a window")
		}
	})

	t.Run("a closed window is replaced", func(t *testing.T) {
		app.WindowMgr.CloseWindow(key)
		app.W.OpenedFiles.Forget(key)
		app.W.OpenedFiles.Open([]string{loose})
		if len(keyed()) != 2 {
			t.Fatalf("keyed windows = %d, want 2", len(keyed()))
		}
		fresh := keyed()[1]
		if fresh == key || !openedfiles.IsWindowKey(fresh) {
			t.Fatalf("fresh key = %q, old %q", fresh, key)
		}
		if got := pendingFor(t, app, key); len(got) != 0 {
			t.Fatalf("old Pending = %+v, want empty", got)
		}
		if got := pendingFor(t, app, fresh); len(got) != 1 || got[0].Path != loose {
			t.Fatalf("new Pending = %+v, want %s", got, loose)
		}
	})

	t.Run("a stale window keeps its targets", func(t *testing.T) {
		stale := keyed()[1]
		delete(app.WindowMgr.Known, stale)
		app.W.OpenedFiles.Open([]string{loose})
		if len(keyed()) != 3 {
			t.Fatalf("keyed windows = %d, want 3", len(keyed()))
		}
		fresh := keyed()[2]
		if got := pendingFor(t, app, stale); len(got) != 0 {
			t.Fatalf("stale Pending = %+v, want empty", got)
		}
		if got := pendingFor(t, app, fresh); len(got) != 1 || got[0].Path != loose {
			t.Fatalf("new Pending = %+v, want %s", got, loose)
		}
	})

	t.Run("forget before pull clears the entry", func(t *testing.T) {
		k := keyed()[2]
		app.W.OpenedFiles.Open([]string{loose})
		app.W.OpenedFiles.Forget(k)
		if got := pendingFor(t, app, k); len(got) != 0 {
			t.Fatalf("Pending after Forget = %+v, want empty", got)
		}
		app.W.OpenedFiles.Open([]string{loose})
		if len(keyed()) != 4 {
			t.Fatalf("keyed windows = %d, want 4", len(keyed()))
		}
	})
}
