// A database seeded before migration 0014 (window mode) migrates with every existing
// window landing on 'studio' — the app's own default mode (state/mode.ts's defaultMode), applied
// via the column's own DEFAULT rather than a hand-rolled backfill (0012's own precedent for
// exactly this shape).
package migrations_test

import (
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/migrations"
)

func TestMigration14AddsWindowModeDefaultingToStudio(t *testing.T) {
	db := openAt(t, 13)

	// 0002_p8_windows.sql's own seed already inserted a 'main' row (order 0) at v13 — a second
	// window, at the next order, is what exercises the migration against a real multi-row table.
	mustExec(t, db, `INSERT INTO windows (key, "order", bounds_json) VALUES ('w2', 1, NULL)`)

	// Apply migration 14 on top of the already-open (v13) database.
	steps, err := migrations.All()
	if err != nil {
		t.Fatalf("migrations.All: %v", err)
	}
	for _, m := range steps {
		if m.Version != 14 {
			continue
		}
		if _, err := db.Exec(m.SQL); err != nil {
			t.Fatalf("apply migration %s (v%d): %v", m.Name, m.Version, err)
		}
	}

	for _, key := range []string{"main", "w2"} {
		if got := scalarString(t, db, `SELECT mode FROM windows WHERE key = ?`, key); got != "studio" {
			t.Errorf(
				"mode(%s) = %q, want 'studio' (the column's own DEFAULT, applied to a pre-existing row)",
				key, got,
			)
		}
	}
}
