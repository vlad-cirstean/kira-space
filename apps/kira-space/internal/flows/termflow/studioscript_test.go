package termflow_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/scripts"
	"github.com/kirathecat/kira-studio/internal/scripts/scriptstest"
	"github.com/kirathecat/kira-studio/internal/terminal"
)

func TestScriptInPickedFolder(t *testing.T) {
	app := flowharness.New(t)

	canceled, err := app.W.Files.ChooseFolder(bridge.FilesChooseFolderArgs{})
	if err != nil || !canceled.Canceled || canceled.Path != nil {
		t.Fatalf("cancelled picker = %+v, %v, want canceled with no path", canceled, err)
	}
	// Contract terminal: tests/ui/automations-module.spec.ts "contract: Choose… fills the folder".
	app.StudioContract(t, "terminal", "FilesService.ChooseFolder#cancelled", canceled)

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "marker.txt"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	app.Dialogs.AnswerDirectory(dir)
	picked, err := app.W.Files.ChooseFolder(bridge.FilesChooseFolderArgs{Title: "Pick"})
	if err != nil || picked.Canceled || picked.Path == nil || *picked.Path != dir {
		t.Fatalf("picker = %+v, %v, want %s", picked, err, dir)
	}

	picks := flowharness.Replace(dir, "<tmp>/picked")
	app.StudioContract(t, "terminal", "FilesService.ChooseFolder#picked", picked, picks)
	mark := app.Events.Mark()
	cmd := "ls marker.txt; exit 4"
	rec, err := app.W.CustomScripts.Create(bridge.CustomScriptsCreateArgs{Fields: scripts.CustomScriptFields{
		Name: "list", Command: cmd, DirMode: scripts.DirModeFixed, WorkingDir: *picked.Path, Color: "blue",
	}})
	if err != nil {
		t.Fatal(err)
	}
	app.StudioContract(t, "terminal", "CustomScriptsService.Create#fixed", rec, picks)
	ev := app.Events.WaitAfter(t, mark, bridge.ChannelCustomScriptsChanged, nil, wait)
	var snap scripts.Snapshot
	ev.Decode(t, &snap)
	if len(snap.Scripts) != 1 || snap.Scripts[0].ID != rec.ID || snap.Scripts[0].WorkingDir != dir {
		t.Fatalf("changed snapshot = %+v, want the new command in %s", snap, dir)
	}

	if _, err := app.W.Terminal.Open(terminal.OpenArgs{
		TerminalID: "qc", WindowKey: "w1", Cwd: rec.WorkingDir, Cols: 80, Rows: 24, Command: rec.Command, LaunchKind: terminal.LaunchKindScript,
	}); err != nil {
		t.Fatal(err)
	}
	waitMatch(t, app, mark, "w1", "qc", line("marker.txt"))
	if code := waitExit(t, app, mark, "w1", "qc"); code != 4 {
		t.Fatalf("exit code = %d, want 4", code)
	}
}

func TestScriptCollections(t *testing.T) {
	app := flowharness.New(t)
	cs := app.W.CustomScripts
	mk := func(name, coll string) scripts.CustomScript {
		t.Helper()
		rec, err := cs.Create(bridge.CustomScriptsCreateArgs{Fields: scripts.CustomScriptFields{Name: name, Command: "echo " + name, Color: "blue", Collection: coll}})
		if err != nil {
			t.Fatal(err)
		}
		return rec
	}
	s1, s2 := mk("one", "A"), mk("two", "A")

	scriptstest.Update(t, cs, s1.ID, scripts.CustomScriptFields{
		Name: s1.Name, Kind: s1.Kind, Command: s1.Command, Color: s1.Color, Collection: "B",
	})
	scriptstest.Update(t, cs, s2.ID, scripts.CustomScriptFields{
		Name: s2.Name, Kind: s2.Kind, Command: s2.Command, Color: s2.Color,
	})
	snap := lastSnapshot(t, app)
	byName := map[string]scripts.CustomScript{}
	for _, s := range snap.Scripts {
		byName[s.Name] = s
	}
	if byName["one"].Collection != "B" || byName["two"].Collection != "" {
		t.Fatalf("one in %q, two in %q, want B and ungrouped", byName["one"].Collection, byName["two"].Collection)
	}
	if len(snap.Collections) != 1 || snap.Collections[0].Name != "B" {
		t.Fatalf("collections = %+v, want only B (A emptied and pruned)", snap.Collections)
	}

	if err := cs.Delete(bridge.CustomScriptsDeleteArgs{ID: s1.ID}); err != nil {
		t.Fatal(err)
	}
	listed, err := cs.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.Scripts) != 1 || listed.Scripts[0].ID != s2.ID || len(listed.Collections) != 0 {
		t.Fatalf("after delete = %+v, want only the ungrouped command and no collection", listed)
	}
	if got := lastSnapshot(t, app); !sameJSON(got, listed) {
		t.Fatalf("event snapshot %+v differs from List %+v", got, listed)
	}

	app.Restart()
	again, err := app.W.CustomScripts.List()
	if err != nil {
		t.Fatal(err)
	}
	if !sameJSON(again, listed) {
		t.Fatalf("after Restart = %+v, want %+v", again, listed)
	}
}

func lastSnapshot(t *testing.T, app *flowharness.App) scripts.Snapshot {
	t.Helper()
	evs := app.Events.Since(0, bridge.ChannelCustomScriptsChanged)
	if len(evs) == 0 {
		t.Fatal("no customScripts:changed event")
	}
	var snap scripts.Snapshot
	evs[len(evs)-1].Decode(t, &snap)
	return snap
}

func sameJSON(a, b any) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return bytes.Equal(x, y)
}

func TestCustomScriptUpdateRemove(t *testing.T) {
	app := flowharness.New(t)
	cs := app.W.CustomScripts
	rec, err := cs.Create(bridge.CustomScriptsCreateArgs{Fields: scripts.CustomScriptFields{Name: "old", Command: "echo old", Color: "blue"}})
	if err != nil {
		t.Fatal(err)
	}
	keep, err := cs.Create(bridge.CustomScriptsCreateArgs{Fields: scripts.CustomScriptFields{Name: "keep", Command: "echo keep", Color: "green"}})
	if err != nil {
		t.Fatal(err)
	}

	mark := app.Events.Mark()
	dir := t.TempDir()
	upd := scriptstest.Update(t, cs, rec.ID, scripts.CustomScriptFields{
		Name: "new", Command: "echo new; exit 3", DirMode: scripts.DirModeFixed, WorkingDir: dir, Color: "red",
	})
	if upd.Name != "new" || upd.Command != "echo new; exit 3" || upd.WorkingDir != dir || upd.Color != "red" {
		t.Fatalf("Update = %+v", upd)
	}
	app.Events.WaitAfter(t, mark, bridge.ChannelCustomScriptsChanged, nil, wait)

	// The updated command is what a terminal now runs.
	if _, err := app.W.Terminal.Open(terminal.OpenArgs{
		TerminalID: "upd", WindowKey: "w1", Cwd: upd.WorkingDir, Cols: 80, Rows: 24, Command: upd.Command, LaunchKind: terminal.LaunchKindScript,
	}); err != nil {
		t.Fatal(err)
	}
	waitMatch(t, app, mark, "w1", "upd", line("new"))
	if code := waitExit(t, app, mark, "w1", "upd"); code != 3 {
		t.Fatalf("exit code = %d, want 3", code)
	}

	mark = app.Events.Mark()
	if err := cs.Delete(bridge.CustomScriptsDeleteArgs{ID: rec.ID}); err != nil {
		t.Fatal(err)
	}
	var snap scripts.Snapshot
	app.Events.WaitAfter(t, mark, bridge.ChannelCustomScriptsChanged, nil, wait).Decode(t, &snap)
	if len(snap.Scripts) != 1 || snap.Scripts[0].ID != keep.ID {
		t.Fatalf("snapshot after Remove = %+v, want only %q", snap, keep.Name)
	}
	if err := cs.Delete(bridge.CustomScriptsDeleteArgs{ID: rec.ID}); err == nil {
		t.Fatal("removing an already removed command succeeded")
	}

	app.Restart()
	again, err := app.W.CustomScripts.List()
	if err != nil || len(again.Scripts) != 1 || again.Scripts[0].ID != keep.ID {
		t.Fatalf("after Restart = %+v (%v)", again, err)
	}
}
