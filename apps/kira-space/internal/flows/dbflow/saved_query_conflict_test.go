package dbflow

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters/testsupport"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/connections"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
)

const savedConflictScenario = "saved-query-two-windows"

// TestSavedQueryTwoWindows is J-CONS-027 and decision 3: two windows hold the same saved query.
// The second window's write names the value it read; a changed query answers E_CONFLICT with the
// `changed` detail, and only the resent unguarded write lands.
// Contract saved-query-two-windows: tests/ui/studio/saved-queries-conflict.spec.ts replays it.
func TestSavedQueryTwoWindows(t *testing.T) {
	app := flowharness.New(t)
	conn := createConn(t, app, connections.Input{ConnectionFields: model.ConnectionFields{
		Name: "two windows", Kind: "kibana", Color: "indigo", Mode: "fields",
		Host: ptr("127.0.0.1"), Port: ptr(5601), Options: map[string]any{},
		McpReadMode: "deny", McpWriteMode: "deny", McpDdlMode: "deny",
	}})
	q := app.W.Queries
	mask := flowharness.Mask("id", "connectionId", "createdAt", "usedAt")

	changed := func(err error) map[string]any {
		t.Helper()
		var ie *ipcerr.Error
		if !errors.As(err, &ie) || ie.Code != string(ipcerr.CodeConflict) {
			t.Fatalf("error = %v, want %s", err, ipcerr.CodeConflict)
		}
		var details map[string]any
		if err := json.Unmarshal(ie.Details, &details); err != nil || details["changed"] != true {
			t.Fatalf("details = %s, want changed:true", ie.Details)
		}
		return map[string]any{"code": ie.Code, "message": ie.Message, "details": details}
	}

	// Console query: window A renames, window B (still holding the old name) pins and renames.
	saved, err := q.SaveConsole(bridge.QueriesSaveConsoleArgs{ConnectionID: conn.ID, Path: "", Name: "report", Body: model.ConsoleBody{Text: "select 1"}})
	if err != nil {
		t.Fatalf("SaveConsole: %v", err)
	}
	loaded := bridge.QueriesExpect{Name: &saved.Name, Pinned: &saved.Pinned}
	nameA, nameB := "report a", "report b"
	if _, err := q.Update(bridge.QueriesUpdateArgs{ID: saved.ID, Name: &nameA, Expect: &loaded}); err != nil {
		t.Fatalf("window A rename: %v", err)
	}
	guardedB := bridge.QueriesUpdateArgs{ID: saved.ID, Name: &nameB, Expect: &loaded}
	app.Contract(t, savedConflictScenario, "args:Update#guarded", guardedB, mask)
	_, err = q.Update(guardedB)
	app.Contract(t, savedConflictScenario, "Update#changed", changed(err))
	list, _ := q.ListConsole(bridge.QueriesListArgs{ConnectionID: conn.ID, Path: ""})
	if len(list) != 1 || list[0].Name != nameA {
		t.Fatalf("after the refused write: %+v, want window A's name", list)
	}
	forced := bridge.QueriesUpdateArgs{ID: saved.ID, Name: &nameB}
	app.Contract(t, savedConflictScenario, "args:Update#confirmed", forced, mask)
	got, err := q.Update(forced)
	if err != nil || got.Name != nameB {
		t.Fatalf("confirmed write = %+v (%v), want %q", got, err, nameB)
	}
	app.Contract(t, savedConflictScenario, "Update#confirmed", got, mask)

	// A pin another window flipped also trips the guard, and a guard that still holds lands.
	pin := true
	if _, err := q.Update(bridge.QueriesUpdateArgs{ID: saved.ID, Pinned: &pin}); err != nil {
		t.Fatal(err)
	}
	if _, err := q.Update(bridge.QueriesUpdateArgs{ID: saved.ID, Pinned: &pin, Expect: &bridge.QueriesExpect{Pinned: ptr(false)}}); err == nil {
		t.Fatal("stale pin guard succeeded")
	} else {
		changed(err)
	}
	unpin := false
	if _, err := q.Update(bridge.QueriesUpdateArgs{ID: saved.ID, Pinned: &unpin, Expect: &bridge.QueriesExpect{Name: &nameB, Pinned: &pin}}); err != nil {
		t.Fatalf("current guard refused: %v", err)
	}

	// A deleted query is not a conflict.
	if err := q.Delete(bridge.QueriesIDArgs{ID: saved.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := q.Update(bridge.QueriesUpdateArgs{ID: saved.ID, Name: &nameA, Expect: &bridge.QueriesExpect{Name: &nameB}}); err == nil {
		t.Fatal("update of a deleted query succeeded")
	} else if ie := new(ipcerr.Error); !errors.As(err, &ie) || ie.Code != string(ipcerr.CodeNotFound) {
		t.Fatalf("deleted query: %v, want %s", err, ipcerr.CodeNotFound)
	}

	// Discover query: the body is the guarded value.
	view := testsupport.KibanaLogsView
	bodyA := model.DiscoverBody{
		Query: "level:error", DataViewID: view, DataViewName: &view, Language: "kuery",
		Time:    model.KibanaTimeSelection{Mode: "preset", Preset: "last15m"},
		Filters: []model.KibanaFilter{}, Columns: []string{"level"}, Sort: []model.KibanaSort{},
		SampleSize: 500, Interval: "auto",
	}
	bodyB := bodyA
	bodyB.Columns = []string{"level", "status"}
	bodyC := bodyA
	bodyC.Query = "level:warn"
	d, err := q.SaveDiscover(bridge.QueriesSaveDiscoverArgs{ConnectionID: conn.ID, Name: "errors", Body: bodyA})
	if err != nil {
		t.Fatalf("SaveDiscover: %v", err)
	}
	if _, err := q.UpdateDiscover(bridge.QueriesUpdateDiscoverArgs{ID: d.ID, Body: bodyB, Expect: &bodyA}); err != nil {
		t.Fatalf("window A body save: %v", err)
	}
	guardedC := bridge.QueriesUpdateDiscoverArgs{ID: d.ID, Body: bodyC, Expect: &bodyA}
	app.Contract(t, savedConflictScenario, "args:UpdateDiscover#guarded", guardedC, mask)
	_, err = q.UpdateDiscover(guardedC)
	app.Contract(t, savedConflictScenario, "UpdateDiscover#changed", changed(err))
	forcedC := bridge.QueriesUpdateDiscoverArgs{ID: d.ID, Body: bodyC}
	app.Contract(t, savedConflictScenario, "args:UpdateDiscover#confirmed", forcedC, mask)
	got2, err := q.UpdateDiscover(forcedC)
	if err != nil {
		t.Fatalf("confirmed body save: %v", err)
	}
	app.Contract(t, savedConflictScenario, "UpdateDiscover#confirmed", got2, mask)
}
