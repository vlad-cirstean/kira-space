package dbflow

import (
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters/testsupport"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
)

// J-GRID-015 (contract studio-copy-insert): Copy as INSERT must rebuild every cell on the far end.
// The test reads a typed source table through the grid read path, writes the INSERT the grid copy
// emits for those rows (quoted text per value, bare NULL, a dialect binary literal for binary
// columns), executes those exact bytes into an empty clone and compares source and clone through
// a second client. The UI half (grid-copy-insert.spec.ts) replays the contract columns and rows
// and asserts Copy as INSERT writes the same text.

type insertColumn struct {
	Name         string `json:"name"`
	DataType     string `json:"dataType"`
	TypeClass    string `json:"typeClass"`
	Nullable     bool   `json:"nullable"`
	IsPrimaryKey bool   `json:"isPrimaryKey"`
	Generated    bool   `json:"generated"`
}

func sqlQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }

// insertText mirrors rowsToInsert (clipboardFormats.ts) for the postgres and sqlite dialects.
func insertText(dialect, qualified string, cols []flowharness.DataColumn, rows [][]any) string {
	names := make([]string, len(cols))
	for i, c := range cols {
		names[i] = `"` + strings.ReplaceAll(c.Name, `"`, `""`) + `"`
	}
	lines := make([]string, len(rows))
	for r, row := range rows {
		vals := make([]string, len(cols))
		for i, c := range cols {
			switch v := row[i].(type) {
			case nil:
				vals[i] = "NULL"
			case string:
				if c.TypeClass == "binary" && strings.HasPrefix(v, "0x") {
					if dialect == "postgres" {
						vals[i] = `'\x` + v[2:] + `'`
					} else {
						vals[i] = "X'" + v[2:] + "'"
					}
				} else {
					vals[i] = sqlQuote(v)
				}
			default:
				panic("unexpected cell type")
			}
		}
		lines[r] = "  (" + strings.Join(vals, ", ") + ")"
	}
	return "INSERT INTO " + qualified + " (" + strings.Join(names, ", ") + ")\nVALUES\n" + strings.Join(lines, ",\n") + ";"
}

func insertColumns(cols []flowharness.DataColumn) []insertColumn {
	out := make([]insertColumn, len(cols))
	for i, c := range cols {
		out[i] = insertColumn{c.Name, c.DataType, c.TypeClass, c.Nullable, c.PK, c.Generated}
	}
	return out
}

func TestGridCopyInsertReplayPostgres(t *testing.T) {
	fx := testsupport.StartPostgres(t)
	app := flowharness.New(t)
	conn := pgFixtureConn(t, app, fx)
	ddl := func(name string) string {
		return `create table app.` + name + ` (id integer primary key, num numeric(38,10), big bigint, tstz timestamptz,
			flag boolean, js jsonb, bin bytea, txt text, uni text)`
	}
	pgExec(t, fx.URI, `drop table if exists app.type_matrix`, `drop table if exists app.type_clone`,
		ddl("type_matrix"), ddl("type_clone"),
		`insert into app.type_matrix values
		 (1, 12345678901234567890123456.1234567891, 9223372036854775807, '2024-03-10 12:34:56.789012+05:30', true,
		  '{"a":[1,2,{"b":null}],"s":"it''s"}', '\xdeadbeef', E'it''s a "quote" \\ back\\slash', 'héllo 日本語 🙂'),
		 (2, null, null, null, null, null, null, null, null),
		 (3, 0, -9223372036854775808, '1970-01-01 00:00:00+00', false, '[]', '\x', '', E'tab\there\nnewline')`)
	ds := app.DataStream(t)
	page, _, err := ds.Read(flowharness.ReadArgs{ConnectionID: conn.ID, Path: tableNodePath(t, app, conn.ID, "type_matrix")})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Rows) != 3 {
		t.Fatalf("rows = %d, want 3", len(page.Rows))
	}
	for i, c := range page.Columns {
		if c.Name == "bin" && c.TypeClass != "binary" {
			t.Fatalf("column %d bin typeClass = %q, want binary", i, c.TypeClass)
		}
	}
	// The UI names the table app.composite_pk (its mock tree); the clone replaces only that name.
	insert := insertText("postgres", "app.composite_pk", page.Columns, page.Rows)
	app.Contract(t, "studio-copy-insert", "columns", insertColumns(page.Columns))
	app.Contract(t, "studio-copy-insert", "rows", page.Rows, contractMask)
	app.Contract(t, "studio-copy-insert", "copy:insert", insert)

	execute(t, app, conn.ID, strings.Replace(insert, "app.composite_pk", "app.type_clone", 1))
	const dump = `select to_jsonb(t)::text from app.%s t order by id`
	want := pgRows(t, fx.URI, strings.Replace(dump, "%s", "type_matrix", 1))
	got := pgRows(t, fx.URI, strings.Replace(dump, "%s", "type_clone", 1))
	wantRows(t, "clone equals source", got, want)
	// NULL stays NULL and '' stays '' (not collapsed into each other).
	wantRows(t, "null vs empty", pgRows(t, fx.URI, `select id, txt is null, txt = '' from app.type_clone where id in (2, 3) order by id`),
		[][]any{{"2", "true", nil}, {"3", "false", "true"}})
	pgExec(t, fx.URI, `drop table app.type_matrix`, `drop table app.type_clone`)
}

func TestGridCopyInsertReplaySQLite(t *testing.T) {
	app := flowharness.New(t)
	path := newSQLiteFile(t)
	ddl := func(name string) string {
		return `create table ` + name + ` (id integer primary key, n numeric, big integer, ts timestamp, flag boolean, js json, bin blob, txt text, uni text)`
	}
	sqliteExec(t, path, ddl("type_matrix"), ddl("type_clone"),
		`insert into type_matrix values
		 (1, 12.5, 9223372036854775807, '2024-03-10 12:34:56.789', 1, '{"a":[1,2]}', X'deadbeef', 'it''s a "quote" \ back\slash', 'héllo 日本語 🙂'),
		 (2, null, null, null, null, null, null, null, null),
		 (3, 0, -9223372036854775808, '1970-01-01 00:00:00', 0, '[]', X'', '', 'tab'||char(9)||'here'||char(10)||'newline')`)
	conn := createConn(t, app, sqliteInput("types", path))
	connect(t, app, conn.ID)
	ds := app.DataStream(t)
	page, _, err := ds.Read(flowharness.ReadArgs{ConnectionID: conn.ID, Path: tableNodePath(t, app, conn.ID, "type_matrix")})
	if err != nil {
		t.Fatal(err)
	}
	insert := insertText("sqlite", "type_clone", page.Columns, page.Rows)
	execute(t, app, conn.ID, insert)
	const dump = `select id, n, big, ts, flag, js, hex(bin), typeof(bin), txt, typeof(txt), uni from %s order by id`
	want := sqliteRows(t, path, strings.Replace(dump, "%s", "type_matrix", 1))
	got := sqliteRows(t, path, strings.Replace(dump, "%s", "type_clone", 1))
	wantRows(t, "clone equals source", got, want)
}
