package dbflow

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters/testsupport"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
)

const consoleScriptScenario = "studio-console-script"

// sqliteScript is the J-CONS-001 script: a `;` in strings and comments, an escaped quote, an empty
// statement and a trigger whose BEGIN ... END body holds a `;`. consoleScriptStatements is what the
// console's splitter makes of it (the UI half asserts the same list from the typed text).
const sqliteScript = `-- seed; the notes
CREATE TABLE notes (id INTEGER PRIMARY KEY, body TEXT);
INSERT INTO notes (body) VALUES ('a;b'), ('it''s; fine');
;
/* block; comment */ INSERT INTO notes (body) VALUES ('c');
CREATE TRIGGER notes_ai AFTER INSERT ON notes BEGIN UPDATE notes SET body = body || '!' WHERE id = new.id; END;
INSERT INTO notes (body) VALUES ('d');
SELECT body FROM notes ORDER BY id`

var sqliteScriptStatements = []string{
	"-- seed; the notes\nCREATE TABLE notes (id INTEGER PRIMARY KEY, body TEXT)",
	"INSERT INTO notes (body) VALUES ('a;b'), ('it''s; fine')",
	"/* block; comment */ INSERT INTO notes (body) VALUES ('c')",
	"CREATE TRIGGER notes_ai AFTER INSERT ON notes BEGIN UPDATE notes SET body = body || '!' WHERE id = new.id; END",
	"INSERT INTO notes (body) VALUES ('d')",
	"SELECT body FROM notes ORDER BY id",
}

// pageSummary is the part of a result page a console shows: kind, column names, rows and whether a
// cap clipped it.
func pageSummary(p flowharness.DataPage) map[string]any {
	cols := make([]string, len(p.Columns))
	for i, c := range p.Columns {
		cols[i] = c.Name
	}
	return map[string]any{"kind": p.Kind, "columns": cols, "rows": p.Rows, "hasMore": p.HasMore, "truncated": p.PosTruncated}
}

func summaries(pages []flowharness.DataPage) []map[string]any {
	out := make([]map[string]any, len(pages))
	for i, p := range pages {
		out[i] = pageSummary(p)
	}
	return out
}

// J-CONS-001 (contract studio-console-script): a script with splitter traps runs as the statement
// list the console sends; a second client proves the effects. The UI half types the script and
// asserts the list it sends equals this one.
func TestConsoleScriptSQLite(t *testing.T) {
	app := flowharness.New(t)
	path := filepath.Join(t.TempDir(), "script.db")
	sqliteExec(t, path, "create table seed (id integer)")
	conn := createConn(t, app, sqliteInput("script", path))
	connect(t, app, conn.ID)
	ds := app.DataStream(t)

	app.Contract(t, consoleScriptScenario, "sqlite#script", sqliteScript)
	app.Contract(t, consoleScriptScenario, "sqlite#statements", sqliteScriptStatements)
	pages, err := ds.Execute(conn.ID, "", sqliteScriptStatements...)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(pages) != len(sqliteScriptStatements) {
		t.Fatalf("%d pages for %d statements", len(pages), len(sqliteScriptStatements))
	}
	app.Contract(t, consoleScriptScenario, "sqlite#pages", summaries(pages))
	wantRows(t, "notes", sqliteRows(t, path, `select id, body from notes order by id`),
		[][]any{{"1", "a;b"}, {"2", "it's; fine"}, {"3", "c"}, {"4", "d!"}})
	// The trigger from the script is a real trigger, not a mis-split statement.
	wantRows(t, "trigger", sqliteRows(t, path, `select name from sqlite_master where type = 'trigger'`), [][]any{{"notes_ai"}})
	app.Contract(t, consoleScriptScenario, "sqlite#effects", sqliteRows(t, path, `select id, body from notes order by id`))
}

// J-CONS-022: ATTACH, a pragma and a second client's write lock. The attached file keeps its rows,
// the pragma persists on the connection, and a busy file refuses the whole script with nothing
// written.
func TestConsoleSQLiteAttachPragmaBusy(t *testing.T) {
	app := flowharness.New(t)
	dir := t.TempDir()
	path, other := filepath.Join(dir, "main.db"), filepath.Join(dir, "other.db")
	sqliteExec(t, path, "create table t (n integer)")
	conn := createConn(t, app, sqliteInput("attach", path))
	connect(t, app, conn.ID)
	ds := app.DataStream(t)

	pages, err := ds.Execute(conn.ID, "",
		"ATTACH DATABASE '"+strings.ReplaceAll(other, "'", "''")+"' AS other",
		"CREATE TABLE other.log (n integer)",
		"INSERT INTO other.log VALUES (1), (2)",
		"PRAGMA user_version = 7",
		"PRAGMA user_version",
		"SELECT name FROM pragma_database_list ORDER BY seq")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(pages) != 6 || pages[4].Rows[0][0] != "7" {
		t.Fatalf("pragma page = %+v, want user_version 7", pages)
	}
	if got := pages[5].Rows; len(got) != 2 || got[0][0] != "main" || got[1][0] != "other" {
		t.Fatalf("database list = %v, want main and other", got)
	}
	wantRows(t, "attached file", sqliteRows(t, other, `select n from log order by n`), [][]any{{"1"}, {"2"}})
	// The attachment lives on the connection: a later script still sees it.
	if p, err := ds.Execute(conn.ID, "", "SELECT count(*) FROM other.log"); err != nil || p[0].Rows[0][0] != "2" {
		t.Fatalf("attachment after the script = %+v (%v)", p, err)
	}

	lock := openLocker(t, path)
	_, err = ds.Execute(conn.ID, "", "INSERT INTO t VALUES (1)", "INSERT INTO t VALUES (2)")
	var de *flowharness.DataError
	if !errors.As(err, &de) {
		t.Fatalf("Execute on a locked file = %v, want a data error", err)
	}
	app.Contract(t, consoleScriptScenario, "sqlite#busy", map[string]string{"code": de.Code, "message": de.Message})
	wantRows(t, "t while locked", sqliteRows(t, other, `select count(*) from log`), [][]any{{"2"}})
	lock.release()
	wantRows(t, "t after the refused script", sqliteRows(t, path, `select count(*) from t`), [][]any{{"0"}})
	if _, err := ds.Execute(conn.ID, "", "INSERT INTO t VALUES (1)", "INSERT INTO t VALUES (2)"); err != nil {
		t.Fatalf("retry after release: %v", err)
	}
	wantRows(t, "t after the retry", sqliteRows(t, path, `select count(*) from t`), [][]any{{"2"}})
}

// pgMigration is the J-CONS-020 script: a transaction whose third statement fails.
var pgMigration = []string{
	"BEGIN",
	"CREATE TABLE app.mig (id int PRIMARY KEY, label text)",
	"INSERT INTO app.mig VALUES ('not a number', 'x')",
	"INSERT INTO app.mig VALUES (2, 'b')",
	"COMMIT",
}

// J-CONS-020, J-CONS-005 and decision 8: a migration in a transaction stops at its failing
// statement and, rolled back, leaves nothing; the transaction lives on the connection across
// executes, so BEGIN then ROLLBACK through data:execute leaves no row for a second client; a
// result past the row cap says so.
func TestConsolePostgresTransactionAndCap(t *testing.T) {
	flowharness.RequireDocker(t)
	fx := testsupport.StartPostgres(t)
	pgExec(t, fx.URI, `drop table if exists app.mig`, `drop table if exists app.tx_rows`, `create table app.tx_rows (n int)`)
	app := flowharness.New(t)
	conn := pgFixtureConn(t, app, fx)
	ds := app.DataStream(t)

	_, err := ds.Execute(conn.ID, "", pgMigration...)
	var de *flowharness.DataError
	if !errors.As(err, &de) {
		t.Fatalf("migration = %v, want a data error", err)
	}
	// The error names the statement it stopped at and says the transaction is still open (the
	// aborted state below): the console prompts on close from it.
	if de.Details != `{"inTransaction":true,"statementIndex":2}` {
		t.Fatalf("migration error details = %q, want the failed statement index 2 and an open transaction", de.Details)
	}
	app.Contract(t, consoleScriptScenario, "pg#migrationError", map[string]string{"code": de.Code, "message": de.Message, "details": de.Details})
	app.Contract(t, consoleScriptScenario, "pg#migration", pgMigration)
	// The failed statement left the transaction aborted: even a valid statement is refused until it ends.
	if _, err := ds.Execute(conn.ID, "", "SELECT 1"); err == nil {
		t.Fatal("statement after the failure ran inside the aborted transaction")
	}
	if _, err := ds.Execute(conn.ID, "", "ROLLBACK"); err != nil {
		t.Fatalf("ROLLBACK: %v", err)
	}
	wantRows(t, "migration table after rollback", pgRows(t, fx.URI, `select to_regclass('app.mig')::text`), [][]any{{nil}})
	if _, err := ds.Execute(conn.ID, "", "SELECT 1"); err != nil {
		t.Fatalf("connection stayed unusable after ROLLBACK: %v", err)
	}

	// A transaction spans executes; ROLLBACK ends it and nothing it wrote is visible.
	if _, err := ds.Execute(conn.ID, "", "BEGIN", "INSERT INTO app.tx_rows VALUES (1), (2)"); err != nil {
		t.Fatal(err)
	}
	inTx, err := ds.Execute(conn.ID, "", "SELECT count(*) FROM app.tx_rows")
	if err != nil || inTx[0].Rows[0][0] != "2" {
		t.Fatalf("count inside the transaction = %+v (%v), want 2", inTx, err)
	}
	wantRows(t, "second client before rollback", pgRows(t, fx.URI, `select count(*) from app.tx_rows`), [][]any{{"0"}})
	if _, err := ds.Execute(conn.ID, "", "ROLLBACK"); err != nil {
		t.Fatal(err)
	}
	wantRows(t, "second client after rollback", pgRows(t, fx.URI, `select count(*) from app.tx_rows`), [][]any{{"0"}})
	after, err := ds.Execute(conn.ID, "", "SELECT pg_current_xact_id_if_assigned() IS NULL")
	if err != nil || after[0].Rows[0][0] != "t" {
		t.Fatalf("transaction after ROLLBACK = %+v (%v), want none open", after, err)
	}

	// Past the cap the page says so.
	capped, err := ds.Execute(conn.ID, "", "SELECT g FROM generate_series(1, 10050) g")
	if err != nil {
		t.Fatal(err)
	}
	app.Contract(t, consoleScriptScenario, "pg#cap", map[string]any{"rows": len(capped[0].Rows), "hasMore": capped[0].HasMore, "truncated": capped[0].PosTruncated})
	if len(capped[0].Rows) != 10_000 || !capped[0].PosTruncated {
		t.Fatalf("capped page = %d rows, truncated=%v, want 10000 and the cap flag", len(capped[0].Rows), capped[0].PosTruncated)
	}
}

// J-CONS-002 (SQL half): repeated result column names reach the page as sent; the UI renames the
// copies x, x_2.
func TestConsoleDuplicateColumns(t *testing.T) {
	app := flowharness.New(t)
	path := filepath.Join(t.TempDir(), "dup.db")
	sqliteExec(t, path, "create table seed (id integer)")
	conn := createConn(t, app, sqliteInput("dup", path))
	connect(t, app, conn.ID)
	const statement = "SELECT 1 AS x, 2 AS x"
	pages, err := app.DataStream(t).Execute(conn.ID, "", statement)
	if err != nil || len(pages) != 1 {
		t.Fatalf("pages=%d err=%v", len(pages), err)
	}
	app.Contract(t, consoleScriptScenario, "sqlite#dupColumns", map[string]any{"statement": statement, "page": pageSummary(pages[0])})
}
