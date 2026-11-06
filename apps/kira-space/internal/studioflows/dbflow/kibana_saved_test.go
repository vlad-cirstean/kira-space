package dbflow

import (
	"errors"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters/testsupport"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/connections"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studioflowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
)

// TestKibanaSavedQueriesJourney is the backend half of the saved-query scenario: save, list,
// update in place, save as, rename, reject duplicates and bad bodies, delete. Saved queries are
// local, so no Kibana runs and the connection is never connected.
// Contract kibana-saved: tests/ui/kibana-saved.spec.ts answers its calls with these results.
func TestKibanaSavedQueriesJourney(t *testing.T) {
	app := studioflowharness.New(t)
	conn := createConn(t, app, connections.Input{ConnectionFields: model.ConnectionFields{
		Name: "saved kibana", Kind: "kibana", Color: "indigo", Mode: "fields",
		Host: ptr("127.0.0.1"), Port: ptr(5601), Options: map[string]any{},
		McpReadMode: "deny", McpWriteMode: "deny", McpDdlMode: "deny",
	}})
	q := app.W.Queries
	view := testsupport.KibanaLogsView
	viewName := view
	mask := studioflowharness.Mask("createdAt", "usedAt")

	bodyA := model.DiscoverBody{
		Query: "level:error", DataViewID: view, DataViewName: &viewName, Language: "kuery",
		Time:    model.KibanaTimeSelection{Mode: "preset", Preset: "last15m"},
		Filters: []model.KibanaFilter{}, Columns: []string{"level"}, Sort: []model.KibanaSort{},
		SampleSize: 500, Interval: "auto",
	}
	bodyB := bodyA
	bodyB.Columns = []string{"level", "status"}
	bodyB.Sort = []model.KibanaSort{{Field: "status", Order: "desc"}}
	bodyB.Filters = []model.KibanaFilter{{Field: "status", Operator: "exists", Values: []string{}}}

	errOf := func(err error) *ipcerr.Error {
		t.Helper()
		var ie *ipcerr.Error
		if !errors.As(err, &ie) {
			t.Fatalf("error %T is not an ipcerr.Error: %v", err, err)
		}
		return ie
	}
	codeMessage := func(ie *ipcerr.Error) map[string]string {
		return map[string]string{"code": ie.Code, "message": ie.Message}
	}

	first, err := q.SaveDiscover(bridge.QueriesSaveDiscoverArgs{ConnectionID: conn.ID, Name: "errors last hour", Body: bodyA})
	if err != nil {
		t.Fatalf("SaveDiscover: %v", err)
	}
	app.Contract(t, "kibana-saved", "QueriesService.SaveDiscover#first", first, mask)

	list, err := q.ListDiscover(bridge.QueriesListDiscoverArgs{ConnectionID: conn.ID})
	if err != nil || len(list) != 1 || list[0].ID != first.ID {
		t.Fatalf("ListDiscover = %+v (%v), want the saved query", list, err)
	}
	app.Contract(t, "kibana-saved", "QueriesService.ListDiscover#afterSave", list, mask)

	_, err = q.SaveDiscover(bridge.QueriesSaveDiscoverArgs{ConnectionID: conn.ID, Name: "Errors Last Hour", Body: bodyA})
	if ie := errOf(err); ie.Code != string(ipcerr.CodeConflict) {
		t.Fatalf("duplicate name: %v, want %s", err, ipcerr.CodeConflict)
	} else {
		app.Contract(t, "kibana-saved", "QueriesService.SaveDiscover#nameTaken", codeMessage(ie))
	}

	updated, err := q.UpdateDiscover(bridge.QueriesUpdateDiscoverArgs{ID: first.ID, Body: bodyB})
	if err != nil || updated.ID != first.ID {
		t.Fatalf("UpdateDiscover = %+v (%v), want the same id", updated, err)
	}
	app.Contract(t, "kibana-saved", "QueriesService.UpdateDiscover#updated", updated, mask)

	list, err = q.ListDiscover(bridge.QueriesListDiscoverArgs{ConnectionID: conn.ID})
	if err != nil || len(list) != 1 || list[0].ID != first.ID {
		t.Fatalf("after update: %+v (%v), want one row with the same id", list, err)
	}
	app.Contract(t, "kibana-saved", "QueriesService.ListDiscover#afterUpdate", list, mask)

	copied, err := q.SaveDiscover(bridge.QueriesSaveDiscoverArgs{ConnectionID: conn.ID, Name: "errors copy", Body: bodyB})
	if err != nil {
		t.Fatalf("save as: %v", err)
	}
	app.Contract(t, "kibana-saved", "QueriesService.SaveDiscover#saveAs", copied, mask)
	if list, _ = q.ListDiscover(bridge.QueriesListDiscoverArgs{ConnectionID: conn.ID}); len(list) != 2 {
		t.Fatalf("list has %d rows, want 2", len(list))
	}

	taken := "errors last hour"
	_, err = q.Update(bridge.QueriesUpdateArgs{ID: copied.ID, Name: &taken})
	if ie := errOf(err); ie.Code != string(ipcerr.CodeConflict) {
		t.Fatalf("rename onto a taken name: %v", err)
	} else {
		app.Contract(t, "kibana-saved", "QueriesService.Update#nameTaken", codeMessage(ie))
	}

	renamed, pin := "5xx", true
	renamedRow, err := q.Update(bridge.QueriesUpdateArgs{ID: copied.ID, Name: &renamed, Pinned: &pin})
	if err != nil || renamedRow.Name != "5xx" || !renamedRow.Pinned {
		t.Fatalf("rename = %+v (%v)", renamedRow, err)
	}
	app.Contract(t, "kibana-saved", "QueriesService.Update#renamed", renamedRow, mask)
	list, _ = q.ListDiscover(bridge.QueriesListDiscoverArgs{ConnectionID: conn.ID})
	if len(list) != 2 || list[0].ID != copied.ID {
		t.Fatalf("pinned query must list first: %+v", list)
	}
	app.Contract(t, "kibana-saved", "QueriesService.ListDiscover#afterRename", list, mask)

	bad := bodyB
	bad.SampleSize = 7
	_, err = q.UpdateDiscover(bridge.QueriesUpdateDiscoverArgs{ID: first.ID, Body: bad})
	if ie := errOf(err); ie.Code != string(ipcerr.CodeBadRequest) {
		t.Fatalf("invalid body: %v", err)
	} else {
		app.Contract(t, "kibana-saved", "QueriesService.UpdateDiscover#invalid", codeMessage(ie))
	}

	filters, _ := q.List(bridge.QueriesListArgs{ConnectionID: conn.ID, Path: ""})
	console, _ := q.ListConsole(bridge.QueriesListArgs{ConnectionID: conn.ID, Path: ""})
	if len(filters) != 0 || len(console) != 0 {
		t.Fatalf("discover rows leaked into other kinds: %d filter, %d console", len(filters), len(console))
	}

	if err := q.Delete(bridge.QueriesIDArgs{ID: copied.ID}); err != nil {
		t.Fatal(err)
	}
	_, err = q.UpdateDiscover(bridge.QueriesUpdateDiscoverArgs{ID: copied.ID, Body: bodyB})
	if ie := errOf(err); ie.Code != string(ipcerr.CodeNotFound) {
		t.Fatalf("update of a deleted query: %v", err)
	} else {
		app.Contract(t, "kibana-saved", "QueriesService.UpdateDiscover#gone", codeMessage(ie))
	}
	list, _ = q.ListDiscover(bridge.QueriesListDiscoverArgs{ConnectionID: conn.ID})
	if len(list) != 1 || list[0].ID != first.ID {
		t.Fatalf("after delete: %+v, want the first query only", list)
	}
	app.Contract(t, "kibana-saved", "QueriesService.ListDiscover#afterDelete", list, mask)

	if err := q.Touch(bridge.QueriesIDArgs{ID: first.ID}); err != nil {
		t.Fatalf("Touch: %v", err)
	}
}
