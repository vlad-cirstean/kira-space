package migrations_test

import (
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/migrations"
)

// P312: migration 0039 seeds every module's panel sizes from the old shared leaves and drops them.
func TestMigration39SeedsPerModePanelSizes(t *testing.T) {
	db := openAt(t, 38)
	mustExec(t, db, `INSERT INTO ui_layout (key, value) VALUES ('panel.project.width', '333'), ('panel.operations.height', '277'), ('panel.project.visible', 'false')`)

	steps, err := migrations.All()
	if err != nil {
		t.Fatalf("migrations.All: %v", err)
	}
	for _, m := range steps {
		if m.Version != 39 {
			continue
		}
		if _, err := db.Exec(m.SQL); err != nil {
			t.Fatalf("apply migration %s (v%d): %v", m.Name, m.Version, err)
		}
	}

	for _, mode := range []string{"studio", "api", "automations", "docker"} {
		if got := scalarString(t, db, `SELECT value FROM ui_layout WHERE key = ?`, "modes."+mode+".project.width"); got != "333" {
			t.Errorf("%s width = %q, want 333", mode, got)
		}
		if got := scalarString(t, db, `SELECT value FROM ui_layout WHERE key = ?`, "modes."+mode+".operations.height"); got != "277" {
			t.Errorf("%s height = %q, want 277", mode, got)
		}
	}
	if n := scalarInt(t, db, `SELECT count(*) FROM ui_layout WHERE key IN ('panel.project.width', 'panel.operations.height')`); n != 0 {
		t.Errorf("legacy leaves left = %d, want 0", n)
	}
	if got := scalarString(t, db, `SELECT value FROM ui_layout WHERE key = 'panel.project.visible'`); got != "false" {
		t.Errorf("visibility = %q, want it untouched", got)
	}
}
