package migrations

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

// P472: migration 0064 turns collections into names and extracts the schedule target.
func TestMigration64RunsFiles(t *testing.T) {
	db, err := sql.Open("sqlite", "file:"+t.TempDir()+"/kira.sqlite?_foreign_keys=1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	steps, err := All()
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range steps {
		if m.Version <= 63 {
			if _, err := db.Exec(m.SQL); err != nil {
				t.Fatalf("apply v%d: %v", m.Version, err)
			}
		}
	}
	exec := func(q string) {
		t.Helper()
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("exec %q: %v", q, err)
		}
	}
	exec(`INSERT INTO custom_script_collections (id, name, sort_order, created_at, updated_at) VALUES
		('c1', 'Ops', 3, '2026-01-01T00:00:00.000Z', '2026-01-01T00:00:00.000Z'), ('c2', 'Ops', 1, 'x', 'x'), ('c3', 'Dev', 2, 'x', 'x')`)
	exec(`INSERT INTO custom_scripts (id, name, command, sort_order, created_at, updated_at, collection_id, schedule_json)
		VALUES ('s1', 'one', 'true', 0, '2026-01-01T00:00:00.000Z', '2026-01-01T00:00:00.000Z', 'c1', '{"cron":"* * * * *","taskId":"t1","branchId":"b1"}'),
		       ('s2', 'two', 'true', 1, '2026-01-01T00:00:00.000Z', '2026-01-01T00:00:00.000Z', NULL, '')`)
	for _, m := range steps {
		if m.Version == 64 {
			exec(m.SQL)
		}
	}
	one := func(q string) string {
		t.Helper()
		var v string
		if err := db.QueryRow(q).Scan(&v); err != nil {
			t.Fatalf("query %q: %v", q, err)
		}
		return v
	}
	if got := one(`SELECT collection FROM custom_scripts WHERE id = 's1'`); got != "Ops" {
		t.Errorf("collection = %q, want Ops", got)
	}
	if got := one(`SELECT schedule_task_id || '/' || schedule_branch_id FROM custom_scripts WHERE id = 's1'`); got != "t1/b1" {
		t.Errorf("target = %q, want t1/b1", got)
	}
	if got := one(`SELECT COUNT(*) FROM custom_scripts`); got != "2" {
		t.Errorf("rows = %s, want 2", got)
	}
	if got := one(`SELECT sort_order FROM custom_script_collections WHERE name = 'Ops'`); got != "1" {
		t.Errorf("merged Ops order = %s, want 1", got)
	}
	if got := one(`SELECT COUNT(*) FROM custom_script_collections`); got != "2" {
		t.Errorf("collections = %s, want 2", got)
	}
}
