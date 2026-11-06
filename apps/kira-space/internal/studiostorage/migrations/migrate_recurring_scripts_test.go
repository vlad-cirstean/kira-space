// P242 Part 4: migration 0037 rebuilds script_runs; rows and indexes survive and the new states insert.
package migrations_test

import (
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/migrations"
)

func TestMigration37RebuildsScriptRuns(t *testing.T) {
	db := openAt(t, 36)
	mustExec(t, db, `INSERT INTO script_runs (id, script_id, script_name, kind, trigger_kind, state, created_at, task_id)
		VALUES ('r1', 's1', 's1', 'script', 'manual', 'done', 1, 't1')`)
	steps, err := migrations.All()
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range steps {
		if m.Version == 37 {
			mustExec(t, db, m.SQL)
		}
	}
	if got := scalarString(t, db, `SELECT task_id FROM script_runs WHERE id = 'r1'`); got != "t1" {
		t.Errorf("task_id = %q, want t1", got)
	}
	mustExec(t, db, `INSERT INTO script_runs (id, script_id, script_name, trigger_kind, state, created_at) VALUES ('r2', 's1', 's1', 'scheduled', 'waiting', 2)`)
	mustExec(t, db, `INSERT INTO script_runs (id, script_id, script_name, trigger_kind, state, created_at) VALUES ('r3', 's1', 's1', 'scheduled', 'skipped', 3)`)
	if n := scalarInt(t, db, `SELECT count(*) FROM sqlite_master WHERE type = 'index' AND name IN ('script_runs_created', 'script_runs_terminal', 'script_runs_task', 'script_runs_active')`); n != 4 {
		t.Errorf("indexes = %d, want 4", n)
	}
}
