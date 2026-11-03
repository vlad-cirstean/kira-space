// P473: migration 0055 adds the file-binding columns and keeps every existing row.
package migrations_test

import (
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/storage/migrations"
)

func TestMigration55ApiFiles(t *testing.T) {
	db := openAt(t, 54)
	mustExec(t, db, `INSERT INTO api_collections (id, name, sort_order, created_at, updated_at) VALUES ('c1', 'Orders', 0, 'x', 'x')`)
	mustExec(t, db, `INSERT INTO api_items (id, collection_id, kind, name, sort_order, created_at, updated_at) VALUES ('i1', 'c1', 'folder', 'f', 0, 'x', 'x')`)
	all, err := migrations.All()
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range all {
		if m.Version == 55 {
			mustExec(t, db, m.SQL)
		}
	}
	if n := scalarInt(t, db, `SELECT COUNT(*) FROM api_collections WHERE stem IS NULL`); n != 1 {
		t.Errorf("collections with no stem = %d, want 1", n)
	}
	mustExec(t, db, `UPDATE api_collections SET stem = 'orders' WHERE id = 'c1'`)
	if _, err := db.Exec(`INSERT INTO api_collections (id, name, sort_order, created_at, updated_at, stem) VALUES ('c2', 'Dup', 1, 'x', 'x', 'orders')`); err == nil {
		t.Error("a second collection with the same stem was accepted")
	}
	mustExec(t, db, `UPDATE api_items SET item_key = 'f' WHERE id = 'i1'`)
	if _, err := db.Exec(`INSERT INTO api_items (id, collection_id, kind, name, sort_order, created_at, updated_at, item_key) VALUES ('i2', 'c1', 'folder', 'g', 1, 'x', 'x', 'f')`); err == nil {
		t.Error("a second item with the same key in one collection was accepted")
	}
}
