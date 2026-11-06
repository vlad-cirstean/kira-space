package dbflow

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/connections"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/connfiles"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/page"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/configfs"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
)

// journey drives one connection kind through the whole connection life: create, test, connect,
// browse, saved queries, history, filters, masking, duplicate/reorder/update, restart.
type journey struct {
	t         *testing.T
	app       *flowharness.App
	input     connections.Input
	table     string // table name as the tree shows it
	readSQL   string // returns rows from the table
	rows      int
	bad       *connections.Input // a draft that must fail Test
	tablePath string
}

func (j *journey) run() {
	t, app := j.t, j.app
	conn := createConn(t, app, j.input)
	if conn.ID == "" || conn.Name != j.input.Name {
		t.Fatalf("created %+v", conn)
	}

	res := app.W.Connections.Test(bridge.ConnectionsIDArgs{ID: conn.ID})
	if !res.OK || res.ServerVersion == nil {
		t.Fatalf("Test = %+v, want ok with a server version", res)
	}
	if j.bad != nil {
		if r := app.W.Connections.Test(bridge.ConnectionsIDArgs{ID: createConn(t, app, *j.bad).ID}); r.OK || r.Error == nil {
			t.Fatalf("Test of an unreachable target = %+v, want a failure", r)
		}
	}

	if st := connect(t, app, conn.ID); st.Status != "connected" {
		t.Fatalf("Connect = %+v, want connected", st)
	}
	states, err := app.W.Connections.States()
	if err != nil || len(states) != 1 || states[0].ConnectionID != conn.ID || states[0].Status != "connected" {
		t.Fatalf("States = %+v (%v)", states, err)
	}

	j.tablePath = j.browse(conn.ID)

	pages := execute(t, app, conn.ID, j.readSQL)
	if len(pages) != 1 {
		t.Fatalf("Execute returned %d pages, want 1", len(pages))
	}
	if tab, ok := pages[0].(page.TabularPage); !ok || tab.RowCount != j.rows {
		t.Fatalf("Execute page = %#v, want a tabular page with %d rows", pages[0], j.rows)
	}

	j.savedQueries(conn.ID, j.tablePath)
	j.filtersAndMasks(conn.ID)

	st, err := app.W.Connections.Disconnect(bridge.ConnectionsIDArgs{ID: conn.ID})
	if err != nil || st.Status == "connected" {
		t.Fatalf("Disconnect = %+v (%v)", st, err)
	}

	j.manage(conn)
}

// browse walks the tree to the table, then reads its metadata, definition and schema columns.
func (j *journey) browse(connID string) string {
	t, app := j.t, j.app
	var tablePath string
	var walk func(path string, depth int)
	walk = func(path string, depth int) {
		kids, err := app.W.Tree.Children(bridge.TreeChildrenArgs{ConnectionID: connID, Path: path})
		if err != nil {
			t.Fatalf("Tree.Children %q: %v", path, err)
		}
		for _, n := range kids.Nodes {
			if n.Kind == "table" && n.Name == j.table {
				tablePath = n.Path
				return
			}
			if n.HasChildren && depth < 3 && tablePath == "" {
				walk(n.Path, depth+1)
			}
		}
	}
	walk("", 0)
	if tablePath == "" {
		t.Fatalf("table %q not found in the tree", j.table)
	}
	again, err := app.W.Tree.Children(bridge.TreeChildrenArgs{ConnectionID: connID, Path: ""})
	if err != nil || again.Source != "cache" {
		t.Fatalf("second Children = source %q (%v), want the cache", again.Source, err)
	}
	if err := app.W.Tree.Invalidate(bridge.TreeInvalidateArgs{ConnectionID: connID}); err != nil {
		t.Fatal(err)
	}
	fresh, err := app.W.Tree.Children(bridge.TreeChildrenArgs{ConnectionID: connID, Path: ""})
	if err != nil || fresh.Source != "server" {
		t.Fatalf("Children after Invalidate = source %q (%v), want the server", fresh.Source, err)
	}

	desc, err := app.W.Tree.Describe(bridge.TreeDescribeArgs{ConnectionID: connID, Path: tablePath})
	if err != nil || len(desc.Meta.Columns) < 2 {
		t.Fatalf("Describe = %+v (%v)", desc.Meta, err)
	}
	def, err := app.W.Tree.Definition(bridge.TreeDescribeArgs{ConnectionID: connID, Path: tablePath})
	if err != nil {
		t.Fatalf("Definition: %v", err)
	}
	if !strings.Contains(strings.ToLower(strings.Join(def.Definition.Statements, "\n")), "create table") {
		t.Fatalf("Definition = %+v, want the CREATE TABLE text", def.Definition)
	}
	parent := ""
	if i := strings.LastIndex(tablePath, "/"); i >= 0 {
		parent = tablePath[:i]
	}
	cols, err := app.W.Tree.SchemaColumns(bridge.TreeDescribeArgs{ConnectionID: connID, Path: parent})
	if err != nil {
		t.Fatalf("SchemaColumns: %v", err)
	}
	if !slices.ContainsFunc(cols.Relations, func(r model.RelationColumns) bool { return strings.EqualFold(r.Name, j.table) }) {
		t.Fatalf("SchemaColumns = %+v, want %s among the relations", cols.Relations, j.table)
	}
	// Relational adapters carry no key types; the call still answers cleanly rather than failing.
	_, err = app.W.Tree.KeyTypes(bridge.TreeKeyTypesArgs{ConnectionID: connID, Paths: []string{tablePath}})
	var ie *ipcerr.Error
	if !errors.As(err, &ie) || ie.Code != "E_UNSUPPORTED" {
		t.Fatalf("KeyTypes on a relational connection = %v, want E_UNSUPPORTED", err)
	}
	return tablePath
}

// savedEvents returns the saved-queries change events emitted since mark.
func (j *journey) savedEvents(mark int) []bridge.SavedQueriesChangedEvent {
	var out []bridge.SavedQueriesChangedEvent
	for _, ev := range j.app.Events.Since(mark, bridge.ChannelSavedQueriesChanged) {
		var d bridge.SavedQueriesChangedEvent
		ev.Decode(j.t, &d)
		out = append(out, d)
	}
	return out
}

func (j *journey) wantSavedEvent(mark int, connID, kind, after string) {
	j.t.Helper()
	got := j.savedEvents(mark)
	if len(got) != 1 || got[0].ConnectionID != connID || got[0].Kind != kind {
		j.t.Fatalf("saved-queries events after %s = %+v, want one {%s %s}", after, got, connID, kind)
	}
}

func (j *journey) savedQueries(connID, tablePath string) {
	t, app := j.t, j.app
	mark := app.Events.Mark()
	where := "id > 1"
	saved, err := app.W.Queries.Save(bridge.QueriesSaveArgs{
		ConnectionID: connID, Path: tablePath, Name: "recent", Body: model.FilterBody{Where: &where},
	})
	if err != nil {
		t.Fatalf("Queries.Save: %v", err)
	}
	j.wantSavedEvent(mark, connID, saved.Kind, "Save")
	mark = app.Events.Mark()
	console, err := app.W.Queries.SaveConsole(bridge.QueriesSaveConsoleArgs{
		ConnectionID: connID, Path: "", Name: "scratch", Body: model.ConsoleBody{Text: j.readSQL},
	})
	if err != nil {
		t.Fatalf("Queries.SaveConsole: %v", err)
	}
	j.wantSavedEvent(mark, connID, console.Kind, "SaveConsole")
	mark = app.Events.Mark()
	renamed, err := app.W.Queries.Update(bridge.QueriesUpdateArgs{ID: saved.ID, Name: ptr("recent two"), Pinned: ptr(true)})
	if err != nil || renamed.Name != "recent two" || !renamed.Pinned {
		t.Fatalf("Queries.Update = %+v (%v)", renamed, err)
	}
	j.wantSavedEvent(mark, connID, saved.Kind, "Update")
	mark = app.Events.Mark()
	if err := app.W.Queries.Touch(bridge.QueriesIDArgs{ID: saved.ID}); err != nil {
		t.Fatalf("Queries.Touch: %v", err)
	}
	j.wantSavedEvent(mark, connID, saved.Kind, "Touch")
	filters, err := app.W.Queries.List(bridge.QueriesListArgs{ConnectionID: connID, Path: tablePath})
	if err != nil || len(filters) != 1 || filters[0].ID != saved.ID {
		t.Fatalf("Queries.List = %+v (%v), want the one filter", filters, err)
	}
	consoles, err := app.W.Queries.ListConsole(bridge.QueriesListArgs{ConnectionID: connID, Path: ""})
	if err != nil || len(consoles) != 1 || consoles[0].ID != console.ID {
		t.Fatalf("Queries.ListConsole = %+v (%v), want the one console", consoles, err)
	}

	doomed, err := app.W.Queries.Save(bridge.QueriesSaveArgs{
		ConnectionID: connID, Path: tablePath, Name: "doomed", Body: model.FilterBody{Where: &where},
	})
	if err != nil {
		t.Fatalf("Queries.Save doomed: %v", err)
	}
	mark = app.Events.Mark()
	if err := app.W.Queries.Delete(bridge.QueriesIDArgs{ID: doomed.ID}); err != nil {
		t.Fatalf("Queries.Delete: %v", err)
	}
	j.wantSavedEvent(mark, connID, doomed.Kind, "Delete")
	filters, err = app.W.Queries.List(bridge.QueriesListArgs{ConnectionID: connID, Path: tablePath})
	if err != nil || len(filters) != 1 || filters[0].ID != saved.ID {
		t.Fatalf("Queries.List after Delete = %+v (%v), want the one filter", filters, err)
	}

	mark = app.Events.Mark()
	for _, w := range []string{"id = 1", "id = 2", "id = 1"} {
		if err := app.W.Queries.HistoryRecord(bridge.QueriesHistoryRecordArgs{ConnectionID: connID, Path: tablePath, Where: &w}); err != nil {
			t.Fatalf("HistoryRecord %q: %v", w, err)
		}
	}
	var recorded []bridge.FilterHistoryChangedEvent
	for _, ev := range app.Events.Since(mark, bridge.ChannelFilterHistoryChanged) {
		var d bridge.FilterHistoryChangedEvent
		ev.Decode(t, &d)
		recorded = append(recorded, d)
	}
	if len(recorded) != 3 || recorded[0].ConnectionID != connID || recorded[0].Path != tablePath {
		t.Fatalf("filter-history events = %+v, want 3 for %s %s", recorded, connID, tablePath)
	}
	hist, err := app.W.Queries.HistoryList(bridge.QueriesHistoryListArgs{ConnectionID: connID, Path: tablePath, Limit: 10})
	if err != nil || len(hist) != 2 {
		t.Fatalf("HistoryList = %d entries (%v), want 2 (duplicates fold)", len(hist), err)
	}
}

func (j *journey) filtersAndMasks(connID string) {
	t, app := j.t, j.app
	vis := model.TreeVisibility{HiddenKinds: []string{"view"}, HiddenPaths: []string{}}
	got, err := app.W.Filters.Replace(bridge.FiltersReplaceArgs{ConnectionID: connID, Visibility: vis})
	if err != nil || !reflect.DeepEqual(got.HiddenKinds, vis.HiddenKinds) {
		t.Fatalf("Filters.Replace = %+v (%v)", got, err)
	}
	listed, err := app.W.Filters.List(bridge.FiltersListArgs{ConnectionID: connID})
	if err != nil || !reflect.DeepEqual(listed.HiddenKinds, vis.HiddenKinds) {
		t.Fatalf("Filters.List = %+v (%v)", listed, err)
	}

	rule, err := app.W.MaskRules.Upsert(bridge.MaskRulesUpsertArgs{ConnectionID: connID, Fields: model.MaskRuleFields{
		TableName: j.table, ColumnName: "email", Kind: model.MaskKindEmail, Correlate: true,
	}})
	if err != nil {
		t.Fatalf("MaskRules.Upsert: %v", err)
	}
	counts, err := app.W.MaskRules.Counts()
	if err != nil || counts[connID] != 1 {
		t.Fatalf("Counts = %v (%v), want 1 for the connection", counts, err)
	}
	key, err := app.W.MaskRules.CorrelationKey(bridge.MaskRulesListArgs{ConnectionID: connID})
	if err != nil || key == "" {
		t.Fatalf("CorrelationKey = %q (%v), want a key for a correlating rule", key, err)
	}
	if err := app.W.MaskRules.RegenerateKey(bridge.MaskRulesRegenerateKeyArgs{ConnectionID: connID}); err != nil {
		t.Fatalf("RegenerateKey: %v", err)
	}
	if next, err := app.W.MaskRules.CorrelationKey(bridge.MaskRulesListArgs{ConnectionID: connID}); err != nil || next == key {
		t.Fatalf("key after regenerate = %q (%v), want a new one", next, err)
	}
	if err := app.W.MaskRules.Delete(bridge.MaskRulesDeleteArgs{ID: rule.ID}); err != nil {
		t.Fatalf("MaskRules.Delete: %v", err)
	}
	if counts, _ := app.W.MaskRules.Counts(); counts[connID] != 0 {
		t.Fatalf("Counts after Remove = %v, want none", counts)
	}
}

// manage covers the list-level operations, then proves every row survives a relaunch.
func (j *journey) manage(conn model.ConnectionSummary) {
	t, app := j.t, j.app
	dup, err := app.W.Connections.Duplicate(bridge.ConnectionsIDArgs{ID: conn.ID})
	if err != nil || dup.ID == conn.ID {
		t.Fatalf("Duplicate = %+v (%v)", dup, err)
	}
	renamed := j.input.Name + " renamed"
	if err := app.W.ConnectionFiles.Patch(conn.ID,
		configfs.NodeSet{Path: []string{"name"}, Value: renamed},
		configfs.NodeSet{Path: []string{"color"}, Value: "green"},
	); err != nil {
		t.Fatalf("Patch: %v", err)
	}
	updated, err := app.W.Connections.List()
	if err != nil {
		t.Fatal(err)
	}
	if i := slices.IndexFunc(updated, func(c model.ConnectionSummary) bool { return c.ID == conn.ID }); i < 0 || updated[i].Name != renamed || updated[i].Color != "green" {
		t.Fatalf("List after the file edit = %+v, want the renamed green connection", updated)
	}
	if err := app.W.Connections.Delete(bridge.ConnectionsIDArgs{ID: dup.ID}); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	before := j.snapshot(conn.ID)
	app.Restart()
	if after := j.snapshot(conn.ID); !reflect.DeepEqual(before, after) {
		t.Fatalf("rows changed across a relaunch:\nbefore %+v\nafter  %+v", before, after)
	}
	if st := connect(t, app, conn.ID); st.Status != "connected" {
		t.Fatalf("Connect after relaunch = %+v", st)
	}
}

type persisted struct {
	Conns   []model.ConnectionSummary
	Queries []model.SavedQuery
	Console []model.SavedQuery
	History []model.FilterHistoryEntry
	Vis     model.TreeVisibility
}

func (j *journey) snapshot(connID string) persisted {
	t, app := j.t, j.app
	var p persisted
	var err error
	if p.Conns, err = app.W.Connections.List(); err != nil {
		t.Fatal(err)
	}
	if p.Queries, err = app.W.Queries.List(bridge.QueriesListArgs{ConnectionID: connID, Path: j.tablePath}); err != nil {
		t.Fatal(err)
	}
	if p.Console, err = app.W.Queries.ListConsole(bridge.QueriesListArgs{ConnectionID: connID, Path: ""}); err != nil {
		t.Fatal(err)
	}
	if p.History, err = app.W.Queries.HistoryList(bridge.QueriesHistoryListArgs{ConnectionID: connID, Path: j.tablePath, Limit: 10}); err != nil {
		t.Fatal(err)
	}
	if p.Vis, err = app.W.Filters.List(bridge.FiltersListArgs{ConnectionID: connID}); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestSQLiteJourney(t *testing.T) {
	app := flowharness.New(t)
	path := newSQLiteFile(t)
	bad := sqliteInput("missing", path+".missing")
	j := &journey{t: t, app: app, input: sqliteInput("fixture", path), table: "users", readSQL: "select id, name, email from users order by id", rows: 3, bad: &bad}
	j.run()
}

func TestSecretsStatusAndReveal(t *testing.T) {
	app := flowharness.New(t)
	status, err := app.W.Connections.SecretsStatus()
	if err != nil || !status.Available || !status.InsecureFallback || status.Backend != "basic_text" {
		t.Fatalf("SecretsStatus = %+v (%v), want the insecure basic_text fallback", status, err)
	}

	in := connections.Input{ConnectionFields: model.ConnectionFields{
		Name: "pg", Kind: "postgres", Color: "none", Mode: "fields", Host: ptr("127.0.0.1"), Port: ptr(5432),
		Database: ptr("app"), Username: ptr("kira"), Options: map[string]any{},
		McpReadMode: "deny", McpWriteMode: "deny", McpDdlMode: "deny",
	}, Password: ptr("s3cret")}
	conn := createConn(t, app, in)

	reveal := func(confirmed bool) connections.RevealResult {
		return app.W.Connections.Reveal(bridge.ConnectionsRevealArgs{ID: conn.ID, Confirmed: confirmed})
	}
	if r := reveal(false); r.Outcome != "confirmation-required" || r.Password != nil {
		t.Fatalf("unconfirmed Reveal = %+v, want confirmation-required without a password", r)
	}
	wantPassword := func(label, want string) {
		t.Helper()
		r := reveal(true)
		if r.Outcome != "revealed" {
			t.Fatalf("%s: Reveal = %+v", label, r)
		}
		got := ""
		if r.Password != nil {
			got = *r.Password
		}
		if got != want {
			t.Fatalf("%s: password %q, want %q", label, got, want)
		}
	}
	wantPassword("after create", "s3cret")

	list, err := app.W.Connections.List()
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := json.Marshal(list); strings.Contains(string(b), "s3cret") {
		t.Fatal("List exposes the password")
	}

	update := func(password *string) {
		t.Helper()
		if err := app.W.Connections.UpdateCredentials(bridge.ConnectionsCredentialsArgs{ID: conn.ID, Password: password}); err != nil {
			t.Fatalf("UpdateCredentials: %v", err)
		}
	}
	update(nil)
	wantPassword("nil leaves it unchanged", "s3cret")
	update(ptr("rotated"))
	wantPassword("a value replaces it", "rotated")
	app.Restart()
	wantPassword("after a relaunch", "rotated")
	if err := app.W.ConnectionFiles.DeleteSecret(connfiles.KindPassword, "pg.password"); err != nil {
		t.Fatal(err)
	}
	wantPassword("a removed secret reveals nothing", "")
}

func TestSchemaDDLSetGet(t *testing.T) {
	app := flowharness.New(t)
	conn, err := app.W.Connections.Create(connections.Input{ConnectionFields: model.ConnectionFields{
		Name: "ddl", Kind: "sqlite", Color: "teal", Mode: "fields", Database: ptr(filepath.Join(t.TempDir(), "d.db")), Options: map[string]any{},
		McpReadMode: "allow", McpWriteMode: "prompt", McpDdlMode: "deny",
	}})
	if err != nil {
		t.Fatal(err)
	}
	set, err := app.W.Schema.Set(bridge.SchemaSetArgs{ConnectionID: conn.ID, DDL: "create table t (id integer);"})
	if err != nil {
		t.Fatalf("Schema.Set: %v", err)
	}
	got, err := app.W.Schema.Get(bridge.SchemaGetArgs{ConnectionID: conn.ID})
	if err != nil || !reflect.DeepEqual(got, set) {
		t.Fatalf("Schema.Get = %+v (%v), want %+v", got, err, set)
	}
	var ie *ipcerr.Error
	if _, err := app.W.Schema.Get(bridge.SchemaGetArgs{}); !errors.As(err, &ie) || ie.Code != "E_CONTRACT" {
		t.Fatalf("Schema.Get without a connection = %v, want E_CONTRACT", err)
	}
}
