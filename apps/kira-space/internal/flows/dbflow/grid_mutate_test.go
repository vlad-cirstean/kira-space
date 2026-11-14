package dbflow

import (
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

const gridMutateScenario = "studio-grid-mutate"

func upd(key, changes model.RowValues) model.MutationRowOp {
	return model.MutationRowOp{Kind: "update", Key: key, Changes: changes}
}
func ins(values model.RowValues) model.MutationRowOp {
	return model.MutationRowOp{Kind: "insert", Values: values}
}
func del(key model.RowValues) model.MutationRowOp {
	return model.MutationRowOp{Kind: "delete", Key: key}
}

func readRows(t *testing.T, ds *flowharness.DataStream, connID, path string) [][]any {
	t.Helper()
	p, _, err := ds.Read(flowharness.ReadArgs{ConnectionID: connID, Path: path, Sort: nil})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	return p.Rows
}

func wantRows(t *testing.T, label string, got, want [][]any) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%s:\n got %q\nwant %q", label, got, want)
	}
}

var contractMask = flowharness.Mask("opId", "tabId", "connectionId", "path")

// J-GRID-020 (+ J-GRID-022, J-GRID-025, J-BC-051): a staged change set on a table whose name and
// columns need quoting, committed through the data plane, proved by a second client.
func TestGridMutateChangeSet(t *testing.T) {
	app := flowharness.New(t)
	path := filepath.Join(t.TempDir(), "weird.db")
	sqliteExec(t, path,
		`create table "we""ird t" (id integer primary key, "select" text, "ünï" integer null, note text)`,
		`insert into "we""ird t" values (1,'one',10,'n1'),(2,'two',20,'n2'),(3,'three',null,'n3')`,
		`create table comp (a integer, b text, v text, primary key (a, b))`,
		`insert into comp values (1,'x','v1'),(1,'y','v2'),(2,'x','v3')`)
	conn := createConn(t, app, sqliteInput("weird", path))
	connect(t, app, conn.ID)
	tp := tableNodePath(t, app, conn.ID, `we"ird t`)
	ds := app.DataStream(t)

	wantRows(t, "initial read", readRows(t, ds, conn.ID, tp), [][]any{
		{"1", "one", "10", "n1"}, {"2", "two", "20", "n2"}, {"3", "three", nil, "n3"}})

	multi := "a\r\nb\n"
	ops := []model.MutationRowOp{
		upd(vals("id", "1"), vals("select", `x'y"z`, "ünï", nil)),
		ins(vals("id", "4", "select", "ins", "ünï", "7", "note", multi)),
		del(vals("id", "2")),
	}
	app.Contract(t, gridMutateScenario, "mutate:ops", flowharness.MutateWire(conn.ID, tp, ops), contractMask)

	stmts, err := ds.Preview(conn.ID, tp, ops)
	if err != nil {
		t.Fatal(err)
	}
	app.Contract(t, gridMutateScenario, "preview:statements", stmts, contractMask)
	for _, s := range stmts {
		if !strings.Contains(s, `"we""ird t"`) {
			t.Fatalf("preview %q does not quote the table name", s)
		}
	}

	n, err := ds.Mutate(conn.ID, tp, ops)
	if err != nil || n != 3 {
		t.Fatalf("Mutate = %d, %v, want 3 rows", n, err)
	}
	want := [][]any{{"1", `x'y"z`, nil, "n1"}, {"3", "three", nil, "n3"}, {"4", "ins", "7", multi}}
	wantRows(t, "second client after plan 1", sqliteRows(t, path, `select id, "select", "ünï", note from "we""ird t" order by id`), want)
	// Cache invalidated: the next read sees the new values, not the cached page.
	wantRows(t, "read after mutate", readRows(t, ds, conn.ID, tp), want)

	// Plan 2: the second op violates the primary key; the transaction leaves nothing changed.
	bad := []model.MutationRowOp{
		upd(vals("id", "1"), vals("note", "changed")),
		ins(vals("id", "3", "select", "dup")),
	}
	_, err = ds.Mutate(conn.ID, tp, bad)
	var derr *flowharness.DataError
	if !errors.As(err, &derr) {
		t.Fatalf("Mutate of a violating plan = %v, want a wire error", err)
	}
	app.Contract(t, gridMutateScenario, "mutate:error", derr, contractMask, flowharness.Mask("message"))
	wantRows(t, "second client after failed plan", sqliteRows(t, path, `select id, "select", "ünï", note from "we""ird t" order by id`), want)

	// Composite key: an update keyed by the full key lands on one row.
	n, err = ds.Mutate(conn.ID, tableNodePath(t, app, conn.ID, "comp"), []model.MutationRowOp{
		upd(vals("a", "1", "b", "y"), vals("v", "")),
		del(vals("a", "2", "b", "x")),
	})
	if err != nil || n != 2 {
		t.Fatalf("composite Mutate = %d, %v", n, err)
	}
	wantRows(t, "composite", sqliteRows(t, path, `select a, b, v from comp order by a, b`), [][]any{{"1", "x", "v1"}, {"1", "y", ""}})

	// J-GRID-022: delete every row, then insert into the empty table.
	n, err = ds.Mutate(conn.ID, tp, []model.MutationRowOp{del(vals("id", "1")), del(vals("id", "3")), del(vals("id", "4"))})
	if err != nil || n != 3 {
		t.Fatalf("delete all = %d, %v", n, err)
	}
	if rows := readRows(t, ds, conn.ID, tp); len(rows) != 0 {
		t.Fatalf("read of emptied table = %q", rows)
	}
	if _, err := ds.Mutate(conn.ID, tp, []model.MutationRowOp{ins(vals("id", "9", "select", "again"))}); err != nil {
		t.Fatal(err)
	}
	wantRows(t, "insert on empty", readRows(t, ds, conn.ID, tp), [][]any{{"9", "again", nil, nil}})
}

// J-GRID-032: SQLite shapes the grid can meet. A rowid table with no primary key refuses a keyed
// write (the rowid is not a column the grid shows, D23); a generated column is refused by the
// engine, never dropped; a file locked by a second client returns a busy error and the retry lands once.
func TestGridMutateSQLiteRowid(t *testing.T) {
	app := flowharness.New(t)
	path := filepath.Join(t.TempDir(), "shapes.db")
	sqliteExec(t, path,
		`create table logs (msg text, n integer)`,
		`insert into logs values ('a',1),('b',2)`,
		`create table kv (k1 text, k2 text, v text, primary key (k1, k2)) without rowid`,
		`insert into kv values ('a','b','v1')`,
		`create table calc (id integer primary key, base integer, dbl integer generated always as (base * 2) stored)`,
		`insert into calc (id, base) values (1, 5)`)
	conn := createConn(t, app, sqliteInput("shapes", path))
	connect(t, app, conn.ID)
	ds := app.DataStream(t)
	logs, kv, calc := tableNodePath(t, app, conn.ID, "logs"), tableNodePath(t, app, conn.ID, "kv"), tableNodePath(t, app, conn.ID, "calc")

	_, err := ds.Mutate(conn.ID, logs, []model.MutationRowOp{upd(vals("msg", "a"), vals("n", "9"))})
	if err == nil || !strings.Contains(err.Error(), "no primary key") {
		t.Fatalf("keyed write to a table without primary key = %v, want a no-primary-key refusal", err)
	}
	wantRows(t, "logs untouched", sqliteRows(t, path, `select msg, n from logs order by rowid`), [][]any{{"a", "1"}, {"b", "2"}})

	// WITHOUT ROWID: the composite key must be given whole; a partial key is refused, the full key lands.
	if _, err := ds.Mutate(conn.ID, kv, []model.MutationRowOp{upd(vals("k1", "a"), vals("v", "x"))}); err == nil {
		t.Fatal("partial key update was accepted")
	}
	if _, err := ds.Mutate(conn.ID, kv, []model.MutationRowOp{upd(vals("k1", "a", "k2", "b"), vals("v", "v2", "k2", "c"))}); err != nil {
		t.Fatal(err)
	}
	wantRows(t, "kv", sqliteRows(t, path, `select k1, k2, v from kv`), [][]any{{"a", "c", "v2"}})

	// Generated column: edit base lands and the value recomputes; writing the generated one is an error.
	if _, err := ds.Mutate(conn.ID, calc, []model.MutationRowOp{upd(vals("id", "1"), vals("base", "6"))}); err != nil {
		t.Fatal(err)
	}
	wantRows(t, "calc", sqliteRows(t, path, `select base, dbl from calc`), [][]any{{"6", "12"}})
	if _, err := ds.Mutate(conn.ID, calc, []model.MutationRowOp{upd(vals("id", "1"), vals("dbl", "99"))}); err == nil {
		t.Fatal("write to a generated column was accepted")
	}
	wantRows(t, "calc after refused write", sqliteRows(t, path, `select base, dbl from calc`), [][]any{{"6", "12"}})
	if _, err := ds.Mutate(conn.ID, calc, []model.MutationRowOp{ins(vals("id", "2", "base", "1", "dbl", "5"))}); err == nil {
		t.Fatal("insert of a generated column was accepted")
	}

	// Locked file: a second client holds BEGIN EXCLUSIVE.
	lock := openLocker(t, path)
	_, err = ds.Mutate(conn.ID, calc, []model.MutationRowOp{upd(vals("id", "1"), vals("base", "7"))})
	if err == nil {
		t.Fatal("mutate on a locked file succeeded")
	}
	lock.release()
	if _, err := ds.Mutate(conn.ID, calc, []model.MutationRowOp{upd(vals("id", "1"), vals("base", "7"))}); err != nil {
		t.Fatalf("retry after release: %v", err)
	}
	wantRows(t, "calc after retry", sqliteRows(t, path, `select base, dbl from calc`), [][]any{{"7", "14"}})
}
