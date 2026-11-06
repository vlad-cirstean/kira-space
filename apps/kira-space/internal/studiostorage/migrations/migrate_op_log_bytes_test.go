// migrate_rename_test.go's own precedent (the one test in this package that
// seeds data rather than migrating an empty schema) applied to 0015_p23_op_log_bytes.sql — a
// migration that backfills a byte count from existing rows, which SQLite's plain length() would
// get wrong for non-ASCII text (it counts characters, not bytes), and that purges rows a
// per-row cap did not exist to prevent when they were written.
package migrations_test

import (
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/migrations"
	"github.com/kirathecat/kira-studio/internal/sqlitex"
)

func TestMigration15BackfillsByteCountAndPurgesOversizedRows(t *testing.T) {
	db := openAt(t, 14)
	now := "2026-01-01T00:00:00.000Z"

	// keep-ascii: a plain command, no error — stored_bytes should equal its byte length exactly.
	mustExec(t, db, `INSERT INTO op_log (id, connection_id, tab_id, started_at, duration_ms, kind, status, rows, command, error)
		VALUES ('keep-ascii', NULL, NULL, ?, 5, 'read', 'ok', NULL, 'select 1', NULL)`, now)

	// keep-nonascii: 100 copies of 'é' (U+00E9, 2 bytes in UTF-8) — SQLite's plain length() would
	// report 100 (characters), not 200 (bytes). The migration's CAST(... AS BLOB) must produce 200.
	nonASCII := strings.Repeat("é", 100)
	mustExec(t, db, `INSERT INTO op_log (id, connection_id, tab_id, started_at, duration_ms, kind, status, rows, command, error)
		VALUES ('keep-nonascii', NULL, NULL, ?, 5, 'read', 'ok', NULL, ?, NULL)`, now, nonASCII)

	// oversized: well past 73728 (64 KiB command cap + 8 KiB error cap) — written before any
	// per-row cap existed, and must be purged rather than truncated (D3: no per-row cap existed
	// when it was written, so it is exactly the pathological row the purge exists for).
	oversized := strings.Repeat("x", 80_000)
	mustExec(t, db, `INSERT INTO op_log (id, connection_id, tab_id, started_at, duration_ms, kind, status, rows, command, error)
		VALUES ('oversized', NULL, NULL, ?, 5, 'read', 'ok', NULL, ?, NULL)`, now, oversized)

	// Apply migration 15 on top of the already-open (v14) database.
	steps, err := migrations.All()
	if err != nil {
		t.Fatalf("migrations.All: %v", err)
	}
	var m15 *sqlitex.Migration
	for i := range steps {
		if steps[i].Version == 15 {
			m15 = &steps[i]
		}
	}
	if m15 == nil {
		t.Fatalf("no migration with Version == 15 registered")
	}
	if _, err := db.Exec(m15.SQL); err != nil {
		t.Fatalf("apply migration 15: %v", err)
	}

	if got := scalarInt(t, db, `SELECT stored_bytes FROM op_log WHERE id = 'keep-ascii'`); got != len("select 1") {
		t.Errorf("keep-ascii stored_bytes = %d, want %d", got, len("select 1"))
	}
	if got := scalarString(t, db, `SELECT command FROM op_log WHERE id = 'keep-ascii'`); got != "select 1" {
		t.Errorf("keep-ascii command = %q, want it kept verbatim", got)
	}

	if got := scalarInt(t, db, `SELECT stored_bytes FROM op_log WHERE id = 'keep-nonascii'`); got != len(nonASCII) {
		t.Errorf("keep-nonascii stored_bytes = %d, want %d (byte length, not the 100-character count length() alone would give)", got, len(nonASCII))
	}

	if n := scalarInt(t, db, `SELECT COUNT(*) FROM op_log WHERE id = 'oversized'`); n != 0 {
		t.Errorf("oversized row survived migration 15, want it purged (found %d)", n)
	}

	// command_truncated defaults to 0 for every backfilled row — the flag only ever means
	// something for a row Finish itself truncated after this migration.
	if got := scalarInt(t, db, `SELECT command_truncated FROM op_log WHERE id = 'keep-ascii'`); got != 0 {
		t.Errorf("keep-ascii command_truncated = %d, want 0", got)
	}

	if n := scalarInt(t, db, `SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = 'op_log_bytes'`); n != 1 {
		t.Errorf("op_log_bytes index: got count %d, want 1", n)
	}
}
