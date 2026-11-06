// P521: migration 0058 rebuilds script_runs; rows and indexes survive and the mcp trigger inserts.
package legacy_test

import (
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/dbfold/legacy"
)

func TestMigration58AddsMCPTrigger(t *testing.T) {
	db := openAt(t, 57)
	mustExec(t, db, `INSERT INTO script_runs (id, script_id, script_name, kind, trigger_kind, state, created_at, provider, restore_pending)
		VALUES ('r1', 's1', 's1', 'script', 'manual', 'done', 1, 'codex', 1)`)
	steps, err := legacy.All()
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range steps {
		if m.Version == 58 {
			mustExec(t, db, m.SQL)
		}
	}
	if got := scalarString(t, db, `SELECT provider FROM script_runs WHERE id = 'r1'`); got != "codex" {
		t.Errorf("provider = %q, want codex", got)
	}
	if n := scalarInt(t, db, `SELECT restore_pending FROM script_runs WHERE id = 'r1'`); n != 1 {
		t.Errorf("restore_pending = %d, want 1", n)
	}
	mustExec(t, db, `INSERT INTO script_runs (id, script_id, script_name, trigger_kind, state, created_at) VALUES ('r2', 's1', 's1', 'mcp', 'running', 2)`)
	if n := scalarInt(t, db, `SELECT count(*) FROM sqlite_master WHERE type = 'index' AND name IN ('script_runs_created', 'script_runs_terminal', 'script_runs_task', 'script_runs_active')`); n != 4 {
		t.Errorf("indexes = %d, want 4", n)
	}
}
