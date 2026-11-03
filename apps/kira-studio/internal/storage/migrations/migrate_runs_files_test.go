// P472: migration 0054 turns collections into names and extracts the schedule target.
package migrations_test

import (
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/storage/migrations"
)

func TestMigration54RunsFiles(t *testing.T) {
	db := openAt(t, 53)
	mustExec(t, db, `INSERT INTO custom_script_collections (id, name, sort_order, created_at, updated_at) VALUES
		('c1', 'Ops', 3, '2026-01-01T00:00:00.000Z', '2026-01-01T00:00:00.000Z'), ('c2', 'Ops', 1, 'x', 'x'), ('c3', 'Dev', 2, 'x', 'x')`)
	mustExec(t, db, `INSERT INTO custom_scripts (id, name, command, sort_order, created_at, updated_at, collection_id, schedule_json)
		VALUES ('s1', 'one', 'true', 0, '2026-01-01T00:00:00.000Z', '2026-01-01T00:00:00.000Z', 'c1', '{"cron":"* * * * *","taskId":"t1","branchId":"b1"}'),
		       ('s2', 'two', 'true', 1, '2026-01-01T00:00:00.000Z', '2026-01-01T00:00:00.000Z', NULL, '')`)
	all, err := migrations.All()
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range all {
		if m.Version == 54 {
			mustExec(t, db, m.SQL)
		}
	}

	if got := scalarString(t, db, `SELECT collection FROM custom_scripts WHERE id = 's1'`); got != "Ops" {
		t.Errorf("collection = %q, want Ops", got)
	}
	if got := scalarString(t, db, `SELECT collection FROM custom_scripts WHERE id = 's2'`); got != "" {
		t.Errorf("ungrouped collection = %q, want empty", got)
	}
	if got := scalarString(t, db, `SELECT schedule_task_id || '/' || schedule_branch_id FROM custom_scripts WHERE id = 's1'`); got != "t1/b1" {
		t.Errorf("target = %q, want t1/b1", got)
	}
	if n := scalarInt(t, db, `SELECT COUNT(*) FROM custom_scripts`); n != 2 {
		t.Errorf("rows = %d, want 2", n)
	}
	if got := scalarInt(t, db, `SELECT sort_order FROM custom_script_collections WHERE name = 'Ops'`); got != 1 {
		t.Errorf("merged Ops order = %d, want 1", got)
	}
	if n := scalarInt(t, db, `SELECT COUNT(*) FROM custom_script_collections`); n != 2 {
		t.Errorf("collections = %d, want 2 (duplicate names merged)", n)
	}
}
