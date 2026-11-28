package storage_test

import (
	"path/filepath"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/migrations"
	"github.com/kirathecat/kira-studio/internal/sqlitex"
)

// 0112 folds task colour slots 10-19 onto 0-9 and leaves 0-9 alone.
func TestMigration0112ColorSlotsTen(t *testing.T) {
	db, err := sqlitex.Open(filepath.Join(t.TempDir(), "kira.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	all, err := migrations.All()
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlitex.Migrate(db, migrationsBefore(all, 112)); err != nil {
		t.Fatal(err)
	}
	for id, color := range map[string]int{"a": 3, "b": 13, "c": 19, "d": 9, "e": 10} {
		if _, err := db.Exec(`INSERT INTO ade_tasks (id, kind, color, created_at) VALUES (?, 'task', ?, 1)`, id, color); err != nil {
			t.Fatal(err)
		}
	}
	if err := sqlitex.Migrate(db, all); err != nil {
		t.Fatal(err)
	}
	var got string
	if err := db.QueryRow(`SELECT group_concat(id || ':' || color, ',') FROM (SELECT * FROM ade_tasks ORDER BY id)`).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if want := "a:3,b:3,c:9,d:9,e:0"; got != want {
		t.Fatalf("colours = %s, want %s", got, want)
	}
}
