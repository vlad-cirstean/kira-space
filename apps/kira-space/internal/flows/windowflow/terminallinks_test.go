package windowflow_test

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/openedfiles"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
	"github.com/kirathecat/kira-studio/internal/terminal"
	"github.com/kirathecat/kira-studio/internal/testx"
)

const linksScenario = "terminal-file-links"

func TestTerminalFileLinks(t *testing.T) {
	app := flowharness.New(t)
	repo := app.NewRepo("proj")
	repo.Commit("init", map[string]string{
		"src/a.go":     "package a\n\nfunc a() {}\n",
		"src/Makefile": "all:\n",
		"notes.txt":    "prose\n",
		"My Docs/a.ts": "export {}\n",
		"src/sub/b.go": "package sub\n",
	})
	rec, err := app.W.CodeWorkspace.ImportRepo(ctx, bridge.CodeWorkspaceImportArgs{Path: repo.Dir})
	if err != nil {
		t.Fatalf("ImportRepo: %v", err)
	}
	notes := filepath.Join(app.Work, "notes")
	if err := os.MkdirAll(notes, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(p, text string) string {
		if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	loose := write(filepath.Join(notes, "b.json"), "{\"k\": 1}\n")
	write(filepath.Join(notes, "x.txt"), "prose\n")
	write(filepath.Join(app.Home, "k.json"), "{}\n")

	const term = "t1"
	if _, err := app.W.Terminal.Open(terminal.OpenArgs{
		TerminalID: term, Cwd: repo.Dir, Cols: 100, Rows: 30, WindowKey: "w1",
	}); err != nil {
		t.Fatalf("Terminal.Open: %v", err)
	}

	resolveFull := func(fallback string, paths ...string) bridge.OpenedFilesTerminalResolveResult {
		t.Helper()
		got, err := app.W.OpenedFilesSvc.TerminalResolve(bridge.OpenedFilesTerminalResolveArgs{
			TerminalID: term, FallbackCwd: fallback, Paths: paths,
		})
		if err != nil {
			t.Fatalf("TerminalResolve %v: %v", paths, err)
		}
		return got
	}
	resolve := func(fallback string, paths ...string) []bool {
		t.Helper()
		return resolveFull(fallback, paths...).Openable
	}
	writeTerm := func(data string) {
		t.Helper()
		if err := app.W.Terminal.Write(terminal.WriteArgs{TerminalID: term, Data: base64.StdEncoding.EncodeToString([]byte(data))}); err != nil {
			t.Fatal(err)
		}
	}
	windows := func() int { return len(app.WindowMgr.OpenedKeyed) }

	t.Run("resolve against the start cwd", func(t *testing.T) {
		args := bridge.OpenedFilesTerminalResolveArgs{
			TerminalID: term,
			Paths:      []string{"src/a.go", "./src/a.go", "missing.go", "src", loose, "x.txt", "~/k.json"},
		}
		app.Contract(t, linksScenario, "args:OpenedFilesService.TerminalResolve", args, flowharness.Mask("terminalId"))
		got, err := app.W.OpenedFilesSvc.TerminalResolve(args)
		if err != nil {
			t.Fatal(err)
		}
		app.Contract(t, linksScenario, "OpenedFilesService.TerminalResolve", got)
		if want := []bool{true, true, false, false, true, false, true}; !slices.Equal(got.Openable, want) {
			t.Fatalf("openable = %v, want %v", got.Openable, want)
		}
		wantPaths := []string{
			filepath.Join(repo.Dir, "src/a.go"), filepath.Join(repo.Dir, "src/a.go"), "", "", loose, "",
			filepath.Join(app.Home, "k.json"),
		}
		if !slices.Equal(got.Paths, wantPaths) {
			t.Fatalf("paths = %v, want %v", got.Paths, wantPaths)
		}
	})

	t.Run("any file type inside a repo", func(t *testing.T) {
		got := resolve("", "src/Makefile", "notes.txt", "notes/x.txt")
		if want := []bool{true, true, false}; !slices.Equal(got, want) {
			t.Fatalf("openable = %v, want %v", got, want)
		}
		if err := app.W.OpenedFilesSvc.TerminalOpen(bridge.OpenedFilesTerminalOpenArgs{TerminalID: term, Path: "src/Makefile"}); err != nil {
			t.Fatal(err)
		}
		got2 := pendingFor(t, app, app.WindowMgr.OpenedKeyed[0])
		want2 := []openedfiles.Target{{RepoID: rec.ID, RelPath: "src/Makefile"}}
		if !slices.Equal(got2, want2) {
			t.Fatalf("targets = %+v, want %+v", got2, want2)
		}
	})

	t.Run("git diff prefixes", func(t *testing.T) {
		got := resolveFull("", "a/src/a.go", "b/src/a.go", "a/missing.go")
		want := filepath.Join(repo.Dir, "src/a.go")
		if wantPaths := []string{want, want, ""}; !slices.Equal(got.Paths, wantPaths) {
			t.Fatalf("paths = %v, want %v", got.Paths, wantPaths)
		}
	})

	t.Run("spaces in a path", func(t *testing.T) {
		got := resolve("", "My Docs/a.ts")
		if !slices.Equal(got, []bool{true}) {
			t.Fatalf("My Docs/a.ts = %v, want [true]", got)
		}
	})

	t.Run("resolve follows cd", func(t *testing.T) {
		writeTerm("cd src\r")
		testx.WaitUntil(t, 10*time.Second, func() bool { return slices.Equal(resolve("", "a.go"), []bool{true}) })
		if got := resolve("", "src/a.go"); !slices.Equal(got, []bool{false}) {
			t.Fatalf("src/a.go after cd = %v, want [false]", got)
		}
		if got := resolve(repo.Dir, "src/a.go"); !slices.Equal(got, []bool{true}) {
			t.Fatalf("src/a.go after cd with start cwd fallback = %v, want [true]", got)
		}
	})

	t.Run("foreground tool cwd", func(t *testing.T) {
		writeTerm("(cd sub && sleep 30)\r")
		testx.WaitUntil(t, 10*time.Second, func() bool { return slices.Equal(resolve("", "b.go"), []bool{true}) })
		if got := resolve(repo.Dir, "src/a.go"); !slices.Equal(got, []bool{true}) {
			t.Fatalf("src/a.go via fallback during foreground tool = %v, want [true]", got)
		}
		writeTerm("\x03")
		testx.WaitUntil(t, 10*time.Second, func() bool { return slices.Equal(resolve("", "a.go"), []bool{true}) })
	})

	t.Run("open in a repo places the cursor", func(t *testing.T) {
		args := bridge.OpenedFilesTerminalOpenArgs{TerminalID: term, Path: "a.go", Line: 2, Col: 5}
		app.Contract(t, linksScenario, "args:OpenedFilesService.TerminalOpen", args, flowharness.Mask("terminalId"))
		if err := app.W.OpenedFilesSvc.TerminalOpen(args); err != nil {
			t.Fatal(err)
		}
		if windows() != 1 {
			t.Fatalf("keyed windows = %d, want 1", windows())
		}
		got := pendingFor(t, app, app.WindowMgr.OpenedKeyed[0])
		app.Contract(t, linksScenario, "OpenedFilesService.Pending:terminal", got)
		want := []openedfiles.Target{{RepoID: rec.ID, RelPath: "src/a.go", Line: 2, Col: 5}}
		if !slices.Equal(got, want) {
			t.Fatalf("targets = %+v, want %+v", got, want)
		}
	})

	t.Run("open outside the repos reuses the window", func(t *testing.T) {
		if err := app.W.OpenedFilesSvc.TerminalOpen(bridge.OpenedFilesTerminalOpenArgs{TerminalID: term, Path: loose, Line: 1}); err != nil {
			t.Fatal(err)
		}
		if windows() != 1 {
			t.Fatalf("keyed windows = %d, want 1 (reused)", windows())
		}
		if f := app.WindowMgr.Focused; len(f) == 0 || f[len(f)-1] != app.WindowMgr.OpenedKeyed[0] {
			t.Fatalf("focused = %v, want the files window raised", f)
		}
		got := pendingFor(t, app, app.WindowMgr.OpenedKeyed[0])
		want := []openedfiles.Target{{Path: loose, Line: 1}}
		if !slices.Equal(got, want) {
			t.Fatalf("targets = %+v, want %+v", got, want)
		}
	})

	t.Run("a path that is not a file opens nothing", func(t *testing.T) {
		err := app.W.OpenedFilesSvc.TerminalOpen(bridge.OpenedFilesTerminalOpenArgs{TerminalID: term, Path: "missing.go"})
		e := testx.AsIpcErr(t, err)
		if e.Code != string(ipcerr.CodeNotFound) {
			t.Fatalf("code = %s, want %s", e.Code, ipcerr.CodeNotFound)
		}
		app.Contract(t, linksScenario, "error:OpenedFilesService.TerminalOpen", err.Error())
		if windows() != 1 {
			t.Fatalf("keyed windows = %d, want 1", windows())
		}
	})

	t.Run("an exited terminal falls back to the given cwd", func(t *testing.T) {
		if err := app.W.Terminal.Close(terminal.CloseArgs{TerminalID: term}); err != nil {
			t.Fatal(err)
		}
		if got := resolve(repo.Dir, "src/a.go", loose); !slices.Equal(got, []bool{true, true}) {
			t.Fatalf("with fallback = %v, want [true true]", got)
		}
		if got := resolve("", "src/a.go", loose); !slices.Equal(got, []bool{false, true}) {
			t.Fatalf("without fallback = %v, want [false true]", got)
		}
	})

	t.Run("validation", func(t *testing.T) {
		twentyOne := make([]string, 21)
		for i := range twentyOne {
			twentyOne[i] = "a.go"
		}
		resolveCases := map[string]bridge.OpenedFilesTerminalResolveArgs{
			"empty terminalId": {Paths: []string{"a.go"}},
			"too many paths":   {TerminalID: term, Paths: twentyOne},
			"relative cwd":     {TerminalID: term, FallbackCwd: "rel", Paths: []string{"a.go"}},
		}
		for name, args := range resolveCases {
			_, err := app.W.OpenedFilesSvc.TerminalResolve(args)
			if e := testx.AsIpcErr(t, err); e.Code != string(ipcerr.CodeContract) {
				t.Errorf("%s: code = %s, want %s", name, e.Code, ipcerr.CodeContract)
			}
		}
		err := app.W.OpenedFilesSvc.TerminalOpen(bridge.OpenedFilesTerminalOpenArgs{TerminalID: term, Path: loose, Col: 3})
		if e := testx.AsIpcErr(t, err); e.Code != string(ipcerr.CodeContract) {
			t.Errorf("col without line: code = %s, want %s", e.Code, ipcerr.CodeContract)
		}
		if windows() != 1 {
			t.Fatalf("keyed windows = %d, want 1", windows())
		}
	})
}
