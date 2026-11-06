// P426: migration 0048 rebuilds proof_steps and proof_view_state for the 'chart' view. A wrong
// rename or drop order would cascade-delete the view state or leave its foreign key dangling.
package migrations_test

import (
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/migrations"
)

func TestMigration48RebuildsProofTables(t *testing.T) {
	db := openAt(t, 47)
	mustExec(t, db, `INSERT INTO proof_sessions (id, name, name_key, state, current_rev, created_at, updated_at) VALUES ('s1', 'S', 's', 'closed', 1, 1, 1)`)
	mustExec(t, db, `INSERT INTO proof_revisions (session_id, rev, state, created_at) VALUES ('s1', 1, 'current', 1)`)
	for _, ord := range []int{1, 2} {
		mustExec(t, db, `INSERT INTO proof_steps (session_id, rev, ordinal, description, view, payload, payload_bytes) VALUES ('s1', 1, ?, 'd', 'table', x'7b7d', 2)`, ord)
	}
	mustExec(t, db, `INSERT INTO proof_view_state (session_id, rev, ordinal, state, updated_at) VALUES ('s1', 1, 1, '{"grid":1}', 5)`)

	steps, err := migrations.All()
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range steps {
		if m.Version == 48 {
			mustExec(t, db, m.SQL)
		}
	}

	if n := scalarInt(t, db, `SELECT count(*) FROM proof_steps`); n != 2 {
		t.Errorf("steps = %d, want 2", n)
	}
	if got := scalarString(t, db, `SELECT state FROM proof_view_state WHERE ordinal = 1`); got != `{"grid":1}` {
		t.Errorf("view state = %q", got)
	}
	mustExec(t, db, `INSERT INTO proof_steps (session_id, rev, ordinal, description, view, payload, payload_bytes) VALUES ('s1', 1, 3, 'd', 'chart', x'7b7d', 2)`)
	if _, err := db.Exec(`INSERT INTO proof_steps (session_id, rev, ordinal, description, view, payload, payload_bytes) VALUES ('s1', 1, 4, 'd', 'heatmap', x'7b7d', 2)`); err == nil {
		t.Error("view 'heatmap' inserted, want CHECK failure")
	}
	if got := scalarString(t, db, `SELECT "table" FROM pragma_foreign_key_list('proof_view_state')`); got != "proof_steps" {
		t.Errorf("view state foreign key targets %q, want proof_steps", got)
	}
	mustExec(t, db, `DELETE FROM proof_steps WHERE ordinal = 1`)
	if n := scalarInt(t, db, `SELECT count(*) FROM proof_view_state`); n != 0 {
		t.Errorf("view state rows after step delete = %d, want 0", n)
	}
}
