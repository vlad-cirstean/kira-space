package dbflow

import (
	"database/sql"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters/testsupport"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/connections"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// Second clients: connections opened outside the app, so a write is proved by the far end and never
// by app state.

// sqliteRows runs query on its own connection to path; NULL is nil, other values their text form.
func sqliteRows(t *testing.T, path, query string, args ...any) [][]any {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	return scanRows(t, query, func() (*sql.Rows, error) { return db.Query(query, args...) })
}

func scanRows(t *testing.T, query string, run func() (*sql.Rows, error)) [][]any {
	t.Helper()
	rows, err := run()
	if err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	defer rows.Close()
	cols, _ := rows.Columns()
	var out [][]any
	for rows.Next() {
		raw := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range raw {
			ptrs[i] = &raw[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			t.Fatal(err)
		}
		row := make([]any, len(cols))
		for i, v := range raw {
			switch x := v.(type) {
			case nil:
			case []byte:
				row[i] = string(x)
			default:
				row[i] = fmt.Sprint(x)
			}
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

// sqliteExec runs statements on a second connection to path.
func sqliteExec(t *testing.T, path string, stmts ...string) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("%s: %v", s, err)
		}
	}
}

// pgRows runs query on a pgx connection of its own to the fixture.
func pgRows(t *testing.T, uri, query string, args ...any) [][]any {
	t.Helper()
	conn, err := pgx.Connect(ctx, uri)
	if err != nil {
		t.Fatalf("pg second client: %v", err)
	}
	defer conn.Close(ctx)
	rows, err := conn.Query(ctx, query, args...)
	if err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	defer rows.Close()
	var out [][]any
	for rows.Next() {
		vals, err := rows.Values()
		if err != nil {
			t.Fatal(err)
		}
		row := make([]any, len(vals))
		for i, v := range vals {
			switch x := v.(type) {
			case nil:
			case []byte:
				row[i] = string(x)
			default:
				row[i] = fmt.Sprint(x)
			}
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

// pgExec runs statements on a pgx connection of its own.
func pgExec(t *testing.T, uri string, stmts ...string) {
	t.Helper()
	conn, err := pgx.Connect(ctx, uri)
	if err != nil {
		t.Fatalf("pg second client: %v", err)
	}
	defer conn.Close(ctx)
	for _, s := range stmts {
		if _, err := conn.Exec(ctx, s); err != nil {
			t.Fatalf("%s: %v", s, err)
		}
	}
}

// pgFixtureConn creates and connects an app connection to the shared Postgres fixture.
func pgFixtureConn(t *testing.T, app *flowharness.App, fx *testsupport.PgFixture) model.ConnectionSummary {
	t.Helper()
	in := startPgInput(fx)
	c := createConn(t, app, in)
	if st := connect(t, app, c.ID); st.Status != "connected" {
		t.Fatalf("pg connect = %+v", st)
	}
	return c
}

// tableNodePath walks the tree to the table called name and returns its encoded path.
func tableNodePath(t *testing.T, app *flowharness.App, connID, name string) string {
	t.Helper()
	var found string
	var walk func(path string, depth int)
	walk = func(path string, depth int) {
		kids, err := app.W.Tree.Children(bridge.TreeChildrenArgs{ConnectionID: connID, Path: path})
		if err != nil {
			t.Fatalf("Tree.Children %q: %v", path, err)
		}
		for _, n := range kids.Nodes {
			if found != "" {
				return
			}
			if (n.Kind == "table" || n.Kind == "collection") && n.Name == name {
				found = n.Path
				return
			}
			if n.HasChildren && depth < 3 {
				walk(n.Path, depth+1)
			}
		}
	}
	walk("", 0)
	if found == "" {
		t.Fatalf("table %q not found in the tree", name)
	}
	return found
}

// vals builds RowValues from alternating name, value pairs; a nil value is NULL.
func vals(kv ...any) model.RowValues {
	out := make(model.RowValues, 0, len(kv)/2)
	for i := 0; i+1 < len(kv); i += 2 {
		rv := model.RowValue{Name: kv[i].(string)}
		if s, ok := kv[i+1].(string); ok {
			rv.Value = &s
		}
		out = append(out, rv)
	}
	return out
}

// startPgInput turns the shared fixture's resolved config into an app connection input.
func startPgInput(fx *testsupport.PgFixture) connections.Input {
	return pgInputFor(fx.Config)
}

// pgInputFor is startPgInput for any resolved config, such as one re-pointed at a proxy.
func pgInputFor(c model.ResolvedConnectionConfig) connections.Input { return inputFromConfig(c) }

// inputFromConfig turns a fixture's resolved config into an app connection input.
func inputFromConfig(c model.ResolvedConnectionConfig) connections.Input {
	opts := map[string]any{}
	for k, v := range c.Options {
		opts[k] = v
	}
	if c.Kind == "postgres" {
		opts["sslmode"] = "disable"
	}
	return connections.Input{ConnectionFields: model.ConnectionFields{
		Name: "flow " + c.Kind + " fixture", Kind: c.Kind, Color: "indigo", Mode: c.Mode,
		Host: c.Host, Port: c.Port, Database: c.Database, Username: c.Username, URI: c.URI,
		Options: opts, McpReadMode: "deny", McpWriteMode: "deny", McpDdlMode: "deny",
	}, Password: c.Password}
}

type locker struct {
	db *sql.DB
	c  *sql.Conn
	t  *testing.T
}

// openLocker holds BEGIN EXCLUSIVE on path from a second client until release.
func openLocker(t *testing.T, path string) *locker {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	c, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.ExecContext(ctx, "BEGIN EXCLUSIVE"); err != nil {
		t.Fatal(err)
	}
	l := &locker{db: db, c: c, t: t}
	t.Cleanup(l.release)
	return l
}

func (l *locker) release() {
	if l.c == nil {
		return
	}
	_, _ = l.c.ExecContext(ctx, "ROLLBACK")
	_ = l.c.Close()
	_ = l.db.Close()
	l.c = nil
}
