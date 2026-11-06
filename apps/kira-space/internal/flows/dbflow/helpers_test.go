package dbflow

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapterhost"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/connections"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/page"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studioflowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
	"github.com/kirathecat/kira-studio/internal/ids"
	_ "modernc.org/sqlite"
)

var (
	ctx = context.Background()
)

func ptr[T any](v T) *T { return &v }

// newSQLiteFile creates a database with a users table (3 rows) and a view over it.
func newSQLiteFile(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fixture.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, stmt := range []string{
		"create table users (id integer primary key, name text not null, email text)",
		"insert into users (name, email) values ('ada', 'ada@example.com'), ('bob', 'bob@example.com'), ('cy', null)",
		"create view named_users as select id, name from users",
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	return path
}

func sqliteInput(name, path string) connections.Input {
	return connections.Input{ConnectionFields: model.ConnectionFields{
		Name: name, Kind: "sqlite", Color: "blue", Mode: "fields", Database: ptr(path),
		Options: map[string]any{}, McpReadMode: "deny", McpWriteMode: "deny", McpDdlMode: "deny",
	}}
}

func createConn(t *testing.T, app *studioflowharness.App, in connections.Input) model.ConnectionSummary {
	t.Helper()
	c, err := app.W.Connections.Create(in)
	if err != nil {
		t.Fatalf("Connections.Create: %v", err)
	}
	return c
}

func connect(t *testing.T, app *studioflowharness.App, id string) model.ConnectionState {
	t.Helper()
	st, err := app.W.Connections.Connect(bridge.ConnectionsIDArgs{ID: id})
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	return st
}

func execute(t *testing.T, app *studioflowharness.App, connID string, stmts ...string) []page.Page {
	t.Helper()
	resp, err := app.W.Router.Execute(ctx, adapterhost.ExecuteRequestWire{
		OpID: ids.NewID(), ConnectionID: connID, Statements: stmts,
	})
	if err != nil {
		t.Fatalf("Execute %v: %v", stmts, err)
	}
	return resp.Pages
}
