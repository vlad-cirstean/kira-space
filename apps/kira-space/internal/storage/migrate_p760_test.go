package storage_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/migrations"
	"github.com/kirathecat/kira-studio/internal/sqlitex"
)

// 0111 removes kibana connections with everything that hangs off them, keeps other connections and
// their children, and narrows saved-query kinds.
func TestMigration0111DropKibana(t *testing.T) {
	db, err := sqlitex.Open(filepath.Join(t.TempDir(), "kira.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	all, err := migrations.All()
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlitex.Migrate(db, migrationsBefore(all, 111)); err != nil {
		t.Fatal(err)
	}
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	exec(`INSERT INTO windows (key, sort_order) VALUES ('w1', 0)`)
	for _, c := range []struct{ id, kind string }{{"kb", "kibana"}, {"pg", "postgres"}} {
		exec(`INSERT INTO connections (id, name, kind, mode, color, created_at, updated_at)
			VALUES (?, ?, ?, 'fields', 'blue', 1, 1)`, c.id, c.id, c.kind)
		exec(`INSERT INTO tabs (id, window_key, connection_id, sort_order, kind, path, state_json)
			VALUES (?, 'w1', ?, 0, 'console', '', '{}')`, "tab-"+c.id, c.id)
		exec(`INSERT INTO connection_saved_queries (id, connection_id, path, name, kind, created_at, body)
			VALUES (?, ?, '', 'q', 'console', 1, '{}')`, "sq-"+c.id, c.id)
		exec(`INSERT INTO connection_mask_rules (id, connection_id, table_name, column_name, mask_kind, created_at, updated_at)
			VALUES (?, ?, 't', 'c', 'name', 1, 1)`, "mr-"+c.id, c.id)
		exec(`INSERT INTO op_log (id, started_ms, category, kind, status, module, trigger_, source, subject,
			repo_root, repo_name, task_id, parent_id, connection_id)
			VALUES (?, 1, 'db', 'k', 's', 'm', 't', 's', 'x', '', '', '', '', ?)`, "op-"+c.id, c.id)
	}
	exec(`INSERT INTO connection_saved_queries (id, connection_id, path, name, kind, created_at, body)
		VALUES ('sq-discover', 'kb', '', 'd', 'discover', 1, '{}')`)

	if err := sqlitex.Migrate(db, all); err != nil {
		t.Fatal(err)
	}

	count := func(q string, args ...any) int {
		t.Helper()
		var n int
		if err := db.QueryRow(q, args...).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if n := count(`SELECT count(*) FROM connections WHERE id = 'kb'`); n != 0 {
		t.Errorf("kibana connection kept")
	}
	for _, table := range []string{"tabs", "connection_saved_queries", "connection_mask_rules"} {
		if n := count(`SELECT count(*) FROM ` + table + ` WHERE connection_id = 'kb'`); n != 0 {
			t.Errorf("%s: %d kibana rows kept", table, n)
		}
		if n := count(`SELECT count(*) FROM ` + table + ` WHERE connection_id = 'pg'`); n != 1 {
			t.Errorf("%s: %d postgres rows, want 1", table, n)
		}
	}
	if n := count(`SELECT count(*) FROM connections WHERE id = 'pg'`); n != 1 {
		t.Errorf("postgres connection lost")
	}
	if n := count(`SELECT count(*) FROM op_log WHERE id = 'op-kb' AND connection_id IS NULL`); n != 1 {
		t.Errorf("kibana op_log row not kept with null connection_id")
	}
	if n := count(`SELECT count(*) FROM op_log WHERE id = 'op-pg' AND connection_id = 'pg'`); n != 1 {
		t.Errorf("postgres op_log row changed")
	}
	_, err = db.Exec(`INSERT INTO connection_saved_queries (id, connection_id, path, name, kind, created_at, body)
		VALUES ('sq-new', 'pg', '', 'd', 'discover', 1, '{}')`)
	if err == nil || !strings.Contains(err.Error(), "CHECK") {
		t.Errorf("discover saved query insert err = %v, want CHECK failure", err)
	}
}
