package migrations

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

// P544: migration 0080 moves db_op_log rows into op_log with the millisecond start, the connection name
// as subject and every database column kept.
func TestMigration80FoldsDbOpLog(t *testing.T) {
	db, err := sql.Open("sqlite", "file:"+t.TempDir()+"/kira.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	steps, err := All()
	if err != nil {
		t.Fatal(err)
	}
	apply := func(version int) {
		t.Helper()
		for _, m := range steps {
			if m.Version == version {
				if _, err := db.Exec(m.SQL); err != nil {
					t.Fatalf("apply v%d: %v", version, err)
				}
			}
		}
	}
	for v := 1; v <= 79; v++ {
		apply(v)
	}
	exec := func(q string) {
		t.Helper()
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO connections (id, name, kind, color, mode, created_at, updated_at) VALUES ('c1', 'Orders', 'sqlite', '#fff', 'file', 'x', 'x')`)
	exec(`INSERT INTO db_op_log (id, connection_id, tab_id, started_at, duration_ms, kind, status, rows, command, error, command_truncated, path, source)
		VALUES ('op1', 'c1', 'tab1', '2026-01-02T03:04:05.250Z', 12, 'execute', 'ok', 7, 'SELECT 1', NULL, 1, '/a.db', 'agent')`)

	apply(80)

	var started, dur, rows, trunc int64
	var category, module, subject, trigger, source, conn, tab, path string
	if err := db.QueryRow(`SELECT started_ms, duration_ms, rows, command_truncated, category, module, subject, trigger_, source, connection_id, tab_id, path
		FROM op_log WHERE id = 'op1'`).Scan(&started, &dur, &rows, &trunc, &category, &module, &subject, &trigger, &source, &conn, &tab, &path); err != nil {
		t.Fatal(err)
	}
	if started != 1767323045250 || dur != 12 || rows != 7 || trunc != 1 || category != "db" || module != "database" ||
		subject != "Orders" || trigger != "agent" || source != "agent" || conn != "c1" || tab != "tab1" || path != "/a.db" {
		t.Errorf("folded row = %d %d %d %d %q %q %q %q %q %q %q %q", started, dur, rows, trunc, category, module, subject, trigger, source, conn, tab, path)
	}
	if _, err := db.Exec(`SELECT 1 FROM db_op_log`); err == nil {
		t.Error("db_op_log still exists")
	}
}
