package storage_test

import (
	"path/filepath"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/migrations"
	"github.com/kirathecat/kira-studio/internal/sqlitex"
)

// 0092 rebuilds proof_steps to admit 'image'. Dropping the parent with foreign keys on would
// cascade away proof_view_state, so the rebuild must carry both tables across byte for byte.
func TestMigration0092KeepsProofRows(t *testing.T) {
	db, err := sqlitex.Open(filepath.Join(t.TempDir(), "kira.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	all, err := migrations.All()
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlitex.Migrate(db, all[:2]); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO proof_sessions (id, state, current_rev, created_at, closed_at, updated_at, name, name_key)
			VALUES ('s', 'closed', 1, 1, 2, 2, 'Login', 'login')`,
		`INSERT INTO proof_revisions (session_id, rev, state, created_at, closed_at) VALUES ('s', 1, 'current', 1, 2)`,
		`INSERT INTO proof_steps (session_id, rev, ordinal, view, diff, payload_bytes, origin_ordinal, description, query, language, source, payload)
			VALUES ('s', 1, 0, 'table', 'added', 5, NULL, 'rows', 'select 1', 'sql', 'db', x'0001ff7b7d'),
			       ('s', 1, 1, 'diagram', 'changed', 3, 0, 'flow', '', 'text', '', x'7b7d20')`,
		`INSERT INTO proof_view_state (session_id, rev, ordinal, updated_at, state) VALUES ('s', 1, 1, 9, '{"a":1}')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("seed: %v\n%s", err, q)
		}
	}
	if err := sqlitex.Migrate(db, all); err != nil {
		t.Fatal(err)
	}

	scan := func(q string) string {
		t.Helper()
		var got string
		if err := db.QueryRow(q).Scan(&got); err != nil {
			t.Fatal(err)
		}
		return got
	}
	const steps = `SELECT group_concat(ordinal || '|' || view || '|' || diff || '|' || payload_bytes || '|' || ifnull(origin_ordinal, 'null') || '|' ||
		description || '|' || query || '|' || language || '|' || source || '|' || hex(payload), ';')
		FROM (SELECT * FROM proof_steps ORDER BY ordinal)`
	want := "0|table|added|5|null|rows|select 1|sql|db|0001FF7B7D;1|diagram|changed|3|0|flow||text||7B7D20"
	if got := scan(steps); got != want {
		t.Errorf("steps = %s, want %s", got, want)
	}
	if got := scan(`SELECT session_id || rev || ordinal || '|' || updated_at || '|' || state FROM proof_view_state`); got != "s11|9|{\"a\":1}" {
		t.Errorf("view state = %s", got)
	}
	if got := scan(`SELECT count(*) FROM pragma_foreign_key_check`); got != "0" {
		t.Errorf("foreign_key_check reports %s violations", got)
	}
	ins := func(view string) error {
		_, err := db.Exec(`INSERT INTO proof_steps (session_id, rev, ordinal, view, payload_bytes, description, payload)
			VALUES ('s', 1, 7, ?, 2, 'x', x'7b7d')`, view)
		return err
	}
	if err := ins("image"); err != nil {
		t.Errorf("image step refused: %v", err)
	}
	if err := ins("bogus"); err == nil {
		t.Error("view 'bogus' accepted")
	}
	if _, err := db.Exec(`INSERT INTO proof_view_state (session_id, rev, ordinal, updated_at) VALUES ('s', 1, 99, 1)`); err == nil {
		t.Error("view state for a missing step accepted; FK lost in rename")
	}
}
