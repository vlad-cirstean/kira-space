package journeyflow_test

import (
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/windowsvc"
)

// J-BC-071: a relaunch after a crash opens a sound database holding exactly the state at the crash
// moment. The crash is a byte snapshot of the home (database, WAL, shm, config) taken between two writes
// and put back after a quit, so the relaunch sees what a SIGKILL would leave.
func TestCrashRecovery(t *testing.T) {
	app := flowharness.New(t)
	ws := "ws"
	const win = "w-crash"
	if _, err := app.W.WindowsSvc.Ensure(windowsvc.EnsureArgs{WindowKey: win}); err != nil {
		t.Fatal(err)
	}
	saveTabs := func(ids ...string) {
		t.Helper()
		var tabs []model.TabRecord
		for i, id := range ids {
			tabs = append(tabs, model.TabRecord{ID: id, Path: "/" + id, Kind: "terminal", State: []byte(`{}`), Order: i, Active: i == 0, WorkspaceID: &ws})
		}
		if err := app.W.Tabs.Save(bridge.TabsSaveArgs{WindowKey: win, Tabs: tabs}); err != nil {
			t.Fatal(err)
		}
	}
	width, height := 520.0, 300.0
	if _, err := app.W.Layout.Set(bridge.LayoutSetArgs{Patch: model.LayoutPatch{Panel: &model.PanelsPatch{Ade: &model.PanelAdePatch{Width: &width}}}}); err != nil {
		t.Fatal(err)
	}
	saveTabs("t1", "t2")
	conn, err := app.W.Connections.Create(connInput("crash-pw"))
	if err != nil {
		t.Fatal(err)
	}

	snap := app.Snapshot()
	if !snap.Has(".db") {
		t.Fatal("snapshot holds no database file")
	}

	// Writes after the snapshot are lost by the crash.
	if _, err := app.W.Layout.Set(bridge.LayoutSetArgs{Patch: model.LayoutPatch{Panel: &model.PanelsPatch{CellEditor: &model.PanelCellEditorPatch{Height: &height}}}}); err != nil {
		t.Fatal(err)
	}
	saveTabs("t1", "t2", "t3")
	if _, err := app.W.Connections.Create(connInput("lost-pw")); err != nil {
		t.Fatal(err)
	}

	app.CrashTo(snap)

	var integrity string
	if err := app.DB().QueryRow("PRAGMA integrity_check").Scan(&integrity); err != nil || integrity != "ok" {
		t.Fatalf("integrity_check = %q, %v", integrity, err)
	}
	layout, err := app.W.Layout.GetAll()
	if err != nil || layout.Panel.Ade.Width != width || layout.Panel.CellEditor.Height == height {
		t.Fatalf("layout after crash = %+v, %v, want the snapshot's width and not the later height", layout.Panel, err)
	}
	tabs, err := app.W.Tabs.List(bridge.TabsListArgs{WindowKey: win})
	if err != nil || len(tabs) != 2 {
		t.Fatalf("tabs after crash = %+v, %v, want the two saved before the snapshot", tabs, err)
	}
	conns, err := app.W.Connections.List()
	if err != nil || len(conns) != 1 || conns[0].ID != conn.ID {
		t.Fatalf("connections after crash = %+v, %v, want the one saved before the snapshot", conns, err)
	}
	if r := app.W.Connections.Reveal(bridge.ConnectionsRevealArgs{ID: conn.ID, Confirmed: true}); r.Password == nil || *r.Password != "crash-pw" {
		t.Fatalf("saved password after crash = %+v", r)
	}

	// No half-written config file survives, and the app writes again.
	_ = filepath.WalkDir(app.SpaceHome, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && (strings.HasSuffix(p, ".tmp") || strings.Contains(filepath.Base(p), ".tmp-")) {
			t.Errorf("temp file left after crash: %s", p)
		}
		return nil
	})
	saveTabs("t1")
	if tabs, err := app.W.Tabs.List(bridge.TabsListArgs{WindowKey: win}); err != nil || len(tabs) != 1 {
		t.Fatalf("tabs after a post-crash save = %+v, %v", tabs, err)
	}
}
