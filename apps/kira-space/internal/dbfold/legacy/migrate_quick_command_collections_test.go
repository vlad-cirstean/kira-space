// P219: 0033 turns the free-text custom_scripts.collection into custom_script_collections rows,
// an irreversible data migration (same bar as migrate_rename_test.go). Kira Space runs the same SQL.
package legacy_test

import (
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/dbfold/legacy"
)

func TestMigration33ConvertsCollectionNamesToRows(t *testing.T) {
	db := openAt(t, 32)
	for i, c := range []struct{ name, collection string }{
		{"a", ""}, {"b", "Backend"}, {"c", "Backend"}, {"d", "Web"},
	} {
		mustExec(t, db,
			`INSERT INTO custom_scripts (id, name, command, sort_order, created_at, updated_at, collection)
			 VALUES (?, ?, 'x', ?, 't', 't', ?)`, c.name, c.name, i, c.collection)
	}

	steps, err := legacy.All()
	if err != nil {
		t.Fatalf("migrations.All: %v", err)
	}
	for _, m := range steps {
		if m.Version != 33 {
			continue
		}
		if _, err := db.Exec(m.SQL); err != nil {
			t.Fatalf("apply migration %s (v%d): %v", m.Name, m.Version, err)
		}
	}

	if got := scalarInt(t, db, `SELECT COUNT(*) FROM custom_script_collections`); got != 2 {
		t.Errorf("collections = %d, want 2", got)
	}
	for id, want := range map[string]string{"b": "Backend", "c": "Backend", "d": "Web"} {
		got := scalarString(t, db,
			`SELECT c.name FROM custom_scripts s JOIN custom_script_collections c ON c.id = s.collection_id WHERE s.id = ?`, id)
		if got != want {
			t.Errorf("script %s collection = %q, want %q", id, got, want)
		}
	}
	if got := scalarInt(t, db, `SELECT COUNT(*) FROM custom_scripts WHERE id = 'a' AND collection_id IS NULL`); got != 1 {
		t.Errorf("ungrouped script lost its NULL collection_id")
	}
	if got := scalarInt(t, db, `SELECT COUNT(*) FROM pragma_table_info('custom_scripts') WHERE name = 'collection'`); got != 0 {
		t.Errorf("collection column still present")
	}
}
