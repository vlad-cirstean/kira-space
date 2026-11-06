// P242: migration 0034 renames the 'terminal' mode and tab workspace to 'automations', classifies
// each script's folder, and creates the run store.
package migrations_test

import (
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/migrations"
)

func TestMigration34RenamesTerminalModeAndClassifiesScriptFolders(t *testing.T) {
	db := openAt(t, 33)
	mustExec(t, db, `INSERT INTO windows (key, "order", bounds_json, mode) VALUES ('w2', 1, NULL, 'terminal')`)
	mustExec(t, db, `INSERT INTO tabs (id, path, kind, state_json, "order", active, window_key, workspace_id)
		VALUES ('t1', '', 'terminal', '{}', 0, 1, 'w2', 'terminal')`)
	for _, row := range [][2]string{{"home", ""}, {"fixed", "/srv/build"}} {
		mustExec(t, db, `INSERT INTO custom_scripts (id, name, command, working_dir, color, sort_order, created_at, updated_at)
			VALUES (?, ?, 'true', ?, 'none', 0, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`, row[0], row[0], row[1])
	}

	steps, err := migrations.All()
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range steps {
		if m.Version == 34 {
			mustExec(t, db, m.SQL)
		}
	}

	if got := scalarString(t, db, `SELECT mode FROM windows WHERE key = 'w2'`); got != "automations" {
		t.Errorf("window mode = %q, want automations", got)
	}
	if got := scalarString(t, db, `SELECT workspace_id FROM tabs WHERE id = 't1'`); got != "automations" {
		t.Errorf("tab workspace = %q, want automations", got)
	}
	for id, want := range map[string]string{"home": "home", "fixed": "fixed"} {
		if got := scalarString(t, db, `SELECT dir_mode FROM custom_scripts WHERE id = ?`, id); got != want {
			t.Errorf("dir_mode(%s) = %q, want %q", id, got, want)
		}
	}
	if n := scalarInt(t, db, `SELECT COUNT(*) FROM script_runs`); n != 0 {
		t.Errorf("script_runs holds %d rows, want an empty table", n)
	}
}
