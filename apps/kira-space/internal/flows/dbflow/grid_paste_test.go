package dbflow

import (
	"path/filepath"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// J-GRID-013, J-GRID-035 (OQ-GRID-1): the ops a pasted TSV block stages. An empty field is NULL in a
// nullable column and stays ” where NULL is refused; whitespace is kept exactly. The UI half
// (grid-clipboard-roundtrip.spec.ts) pastes "2\t1\t\n3\t3\t" at row 2 of the same table shape and
// sends exactly `mutate:ops`. Contract studio-grid-paste.
func TestGridPasteNullAndEmpty(t *testing.T) {
	app := flowharness.New(t)
	path := filepath.Join(t.TempDir(), "paste.db")
	sqliteExec(t, path,
		`create table composite_pk (tenant_id integer not null, entity_id integer not null, name text, primary key (tenant_id, entity_id))`,
		`insert into composite_pk values (1,1,'tenant 1 / entity 1'),(1,2,'tenant 1 / entity 2'),(2,1,'tenant 2 / entity 1')`,
		`create table pasted (id integer primary key, n integer, ts timestamp, t text, ws text not null)`,
		`insert into pasted values (1, 5, '2024-01-01 00:00:00', 'keep', 'x')`)
	conn := createConn(t, app, sqliteInput("paste", path))
	connect(t, app, conn.ID)
	ds := app.DataStream(t)

	// The block the UI half pastes: an empty last field on an existing row and on a new row.
	cp := tableNodePath(t, app, conn.ID, "composite_pk")
	ops := []model.MutationRowOp{
		guarded(upd(vals("tenant_id", "2", "entity_id", "1"), vals("name", nil)), vals("name", "tenant 2 / entity 1")),
		ins(vals("tenant_id", "3", "entity_id", "3", "name", nil)),
	}
	app.Contract(t, "studio-grid-paste", "mutate:ops", flowharness.MutateWire(conn.ID, cp, ops), contractMask)
	if _, err := ds.Mutate(conn.ID, cp, ops); err != nil {
		t.Fatal(err)
	}
	wantRows(t, "composite_pk after paste", sqliteRows(t, path, `select tenant_id, entity_id, typeof(name), name from composite_pk order by 1, 2`),
		[][]any{{"1", "1", "text", "tenant 1 / entity 1"}, {"1", "2", "text", "tenant 1 / entity 2"},
			{"2", "1", "null", nil}, {"3", "3", "null", nil}})

	// Per column type: nullable int, timestamp and text hold NULL; a NOT NULL text column keeps ''
	// and whitespace exactly.
	pp := tableNodePath(t, app, conn.ID, "pasted")
	more := []model.MutationRowOp{
		upd(vals("id", "1"), vals("n", nil, "ts", nil, "t", nil, "ws", "")),
		ins(vals("id", "2", "n", nil, "ts", nil, "t", nil, "ws", " \t pad ")),
	}
	if _, err := ds.Mutate(conn.ID, pp, more); err != nil {
		t.Fatal(err)
	}
	wantRows(t, "types after paste", sqliteRows(t, path,
		`select id, typeof(n), typeof(ts), typeof(t), typeof(ws), ws from pasted order by id`),
		[][]any{{"1", "null", "null", "null", "text", ""}, {"2", "null", "null", "null", "text", " \t pad "}})
}
