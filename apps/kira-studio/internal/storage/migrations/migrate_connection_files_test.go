// P475: migration 0056 adds the file-binding columns and keeps every existing row.
package migrations_test

import (
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/storage/migrations"
)

func TestMigration56ConnectionFiles(t *testing.T) {
	db := openAt(t, 55)
	mustExec(t, db, `INSERT INTO connections (id, name, kind, color, mode, created_at, updated_at) VALUES ('c1', 'Orders', 'sqlite', 'none', 'fields', 'x', 'x')`)
	all, err := migrations.All()
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range all {
		if m.Version == 56 {
			mustExec(t, db, m.SQL)
		}
	}
	if n := scalarInt(t, db, `SELECT COUNT(*) FROM connections WHERE stem IS NULL AND config_json = ''`); n != 1 {
		t.Errorf("kept rows with no stem = %d, want 1", n)
	}
	mustExec(t, db, `UPDATE connections SET stem = 'orders' WHERE id = 'c1'`)
	if _, err := db.Exec(`INSERT INTO connections (id, name, kind, color, mode, created_at, updated_at, stem) VALUES ('c2', 'Dup', 'sqlite', 'none', 'fields', 'x', 'x', 'orders')`); err == nil {
		t.Error("a second connection with the same stem was accepted")
	}
}
