// A database seeded before migration 0012 (environment color) migrates with every existing
// environment landing at 'none' — the deliberate default ("no colour is the default"), not an
// empty string or NULL, so every pre-existing row is already a valid palette value with no
// backfill needed beyond the column's own DEFAULT.
package migrations_test

import (
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/migrations"
)

func TestMigration12AddsEnvironmentColorDefaultingToNone(t *testing.T) {
	db := openAt(t, 11)

	mustExec(t, db,
		`INSERT INTO api_environments (id, name, sort_order, is_active, description, created_at, updated_at)
		 VALUES ('env1', 'Staging', 0, 0, '', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`,
	)

	// Apply migration 12 on top of the already-open (v11) database.
	steps, err := migrations.All()
	if err != nil {
		t.Fatalf("migrations.All: %v", err)
	}
	for _, m := range steps {
		if m.Version != 12 {
			continue
		}
		if _, err := db.Exec(m.SQL); err != nil {
			t.Fatalf("apply migration %s (v%d): %v", m.Name, m.Version, err)
		}
	}

	if got := scalarString(t, db, `SELECT color FROM api_environments WHERE id = 'env1'`); got != "none" {
		t.Errorf("color = %q, want 'none' (the column's own DEFAULT, applied to a pre-existing row)", got)
	}
}
