package termflow_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/scripts"
)

func lifecycleRun(name, collection, extra string) string {
	s := "name: " + name + "\ncommand: echo " + strings.ToLower(name) + "\n"
	if collection != "" {
		s += "collection: " + collection + "\n"
	}
	return s + extra
}

func collectionOrder(snap scripts.Snapshot, collection string) []string {
	var ids []string
	for _, s := range snap.Scripts {
		if s.Collection == collection {
			ids = append(ids, s.ID)
		}
	}
	return ids
}

func collectionNames(snap scripts.Snapshot) []string {
	var names []string
	for _, c := range snap.Collections {
		names = append(names, c.Name)
	}
	return names
}

func fileText(t *testing.T, app *flowharness.App, file string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(app.SpaceHome, "runs", file))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// TestRunsLifecycle: Move, RenameCollection and DeleteCollection are run-file edits that keep
// comments, order rows follow, a broken member refuses a rename without writing, and a delete takes
// every member.
func TestRunsLifecycle(t *testing.T) {
	app := flowharness.New(t)
	cs := app.W.CustomScripts
	writeRun(t, app, "x.yaml", lifecycleRun("X", "A", "# keep me\n"))
	writeRun(t, app, "y.yaml", lifecycleRun("Y", "A", ""))
	writeRun(t, app, "z.yaml", lifecycleRun("Z", "", ""))
	writeRun(t, app, "zb.yaml", lifecycleRun("W", "B", ""))
	snap := waitSnapshot(t, app, 0, func(s scripts.Snapshot) bool { return len(s.Scripts) == 4 && len(s.Collections) == 2 })
	app.Contract(t, "runs-lifecycle", "snapshot#start", snap, runsMasks...)

	before := "x"
	moveArgs := bridge.CustomScriptsMoveArgs{ID: "z", Collection: "A", BeforeID: &before}
	app.Contract(t, "runs-lifecycle", "args:CustomScriptsService.Move", moveArgs, runsMasks...)
	if err := cs.Move(moveArgs); err != nil {
		t.Fatal(err)
	}
	snap = waitSnapshot(t, app, 0, func(s scripts.Snapshot) bool {
		o := collectionOrder(s, "A")
		return len(o) == 3 && o[0] == "z" && o[1] == "x" && o[2] == "y"
	})
	if txt := fileText(t, app, "z.yaml"); !strings.Contains(txt, "collection: A") {
		t.Fatalf("z.yaml after Move:\n%s", txt)
	}
	if txt := fileText(t, app, "x.yaml"); !strings.Contains(txt, "# keep me") {
		t.Fatalf("x.yaml lost its comment:\n%s", txt)
	}
	app.Contract(t, "runs-lifecycle", "snapshot#moved", snap, runsMasks...)

	out := bridge.CustomScriptsMoveArgs{ID: "z", Collection: ""}
	if err := cs.Move(out); err != nil {
		t.Fatal(err)
	}
	snap = waitSnapshot(t, app, 0, func(s scripts.Snapshot) bool { return len(collectionOrder(s, "")) == 1 && len(collectionOrder(s, "A")) == 2 })
	if txt := fileText(t, app, "z.yaml"); strings.Contains(txt, "collection:") {
		t.Fatalf("z.yaml still names a collection:\n%s", txt)
	}
	fresh := bridge.CustomScriptsMoveArgs{ID: "z", Collection: "A", BeforeID: &before}
	if err := cs.Move(fresh); err != nil {
		t.Fatal(err)
	}
	waitSnapshot(t, app, 0, func(s scripts.Snapshot) bool { return len(collectionOrder(s, "A")) == 3 })

	writeRun(t, app, "y.yaml", lifecycleRun("Y", "A", "kind: bogus\n"))
	waitSnapshot(t, app, 0, func(s scripts.Snapshot) bool { return len(s.Problems) == 1 })
	rename := bridge.CustomScriptsRenameCollectionArgs{Name: "A", NewName: "C"}
	app.Contract(t, "runs-lifecycle", "args:CustomScriptsService.RenameCollection", rename, runsMasks...)
	err := cs.RenameCollection(rename)
	if err == nil || !strings.Contains(err.Error(), "y.yaml") {
		t.Fatalf("RenameCollection with a broken member = %v, want an error naming y.yaml", err)
	}
	for _, f := range []string{"x.yaml", "z.yaml"} {
		if txt := fileText(t, app, f); !strings.Contains(txt, "collection: A") {
			t.Fatalf("%s written despite the refusal:\n%s", f, txt)
		}
	}
	writeRun(t, app, "y.yaml", lifecycleRun("Y", "A", ""))
	waitSnapshot(t, app, 0, func(s scripts.Snapshot) bool { return len(s.Problems) == 0 })

	if err := cs.RenameCollection(bridge.CustomScriptsRenameCollectionArgs{Name: "A", NewName: "B"}); err == nil {
		t.Fatal("RenameCollection onto a taken name succeeded")
	}
	if err := cs.RenameCollection(rename); err != nil {
		t.Fatal(err)
	}
	snap = waitSnapshot(t, app, 0, func(s scripts.Snapshot) bool { return len(collectionOrder(s, "C")) == 3 })
	if names := collectionNames(snap); strings.Join(names, ",") != "C,B" {
		t.Fatalf("collections after rename = %v, want C then B", names)
	}
	if o := collectionOrder(snap, "C"); strings.Join(o, ",") != "z,x,y" {
		t.Fatalf("order after rename = %v", o)
	}
	for _, f := range []string{"x.yaml", "y.yaml", "z.yaml"} {
		if txt := fileText(t, app, f); !strings.Contains(txt, "collection: C") {
			t.Fatalf("%s after rename:\n%s", f, txt)
		}
	}
	app.Contract(t, "runs-lifecycle", "snapshot#renamed", snap, runsMasks...)

	del := bridge.CustomScriptsDeleteCollectionArgs{Name: "C"}
	app.Contract(t, "runs-lifecycle", "args:CustomScriptsService.DeleteCollection", del, runsMasks...)
	if err := cs.DeleteCollection(del); err != nil {
		t.Fatal(err)
	}
	snap = waitSnapshot(t, app, 0, func(s scripts.Snapshot) bool { return len(s.Scripts) == 1 })
	if names := collectionNames(snap); strings.Join(names, ",") != "B" {
		t.Fatalf("collections after delete = %v, want B", names)
	}
	for _, f := range []string{"x.yaml", "y.yaml", "z.yaml"} {
		if _, err := os.Stat(filepath.Join(app.SpaceHome, "runs", f)); !os.IsNotExist(err) {
			t.Fatalf("%s survived DeleteCollection: %v", f, err)
		}
	}
	app.Contract(t, "runs-lifecycle", "snapshot#deleted", snap, runsMasks...)
}
