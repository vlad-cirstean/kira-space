package storage_test

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/migrations"
	"github.com/kirathecat/kira-studio/internal/sqlitex"
)

// 0091 removes only the pre-v0.3.1 state: numeric comment ids in a run's spec, http tabs saved with
// bodyMode 'json', the quarantine table and the fold markers. Well-formed rows survive untouched.
func TestMigration0091DropsOnlyLegacyState(t *testing.T) {
	db, err := sqlitex.Open(filepath.Join(t.TempDir(), "kira.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	all, err := migrations.All()
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlitex.Migrate(db, all[:1]); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO windows (key, sort_order) VALUES ('w', 0)`,
		`INSERT INTO ade_tasks (id, kind, color, created_at) VALUES ('t', 'task', 0, 1)`,
		`INSERT INTO ade_runs (id, task_id, state, attempt, step_id, spec_json) VALUES
			('mixed', 't', 'done', 1, 's', '{"round":2,"commentIds":[1,"c-a",22,"c-b"]}'),
			('numeric', 't', 'done', 1, 's', '{"commentIds":[3]}'),
			('clean', 't', 'done', 1, 's', '{"commentIds":["c-c"]}'),
			('noids', 't', 'done', 1, 's', ''),
			('empty', 't', 'done', 1, 's', '{"commentIds":[]}')`,
		`INSERT INTO tabs (id, window_key, sort_order, kind, path, state_json) VALUES
			('old', 'w', 0, 'http', '/a', '{"bodyMode":"json","body":"{}"}'),
			('new', 'w', 1, 'http', '/b', '{"bodyMode":"code","code":"{}"}'),
			('other', 'w', 2, 'sql', '/c', '{}')`,
		`INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at) VALUES ('x', '{}', 'r', 'removed', 84, 1)`,
		`INSERT INTO settings (key, value) VALUES ('config.migrated.api', 'true'), ('homefold.p539', '{}'), ('configfold.p540', '{}'),
			('dbfold.p544', '{}'), ('keep.me', 'x')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("seed: %v\n%s", err, q)
		}
	}
	if err := sqlitex.Migrate(db, all); err != nil {
		t.Fatal(err)
	}

	ids := func(q string) string {
		t.Helper()
		var got sql.NullString
		if err := db.QueryRow(q).Scan(&got); err != nil {
			t.Fatal(err)
		}
		return got.String
	}
	for id, want := range map[string]string{
		"mixed": `["c-a","c-b"]`, "numeric": `[]`, "clean": `["c-c"]`, "empty": `[]`,
	} {
		if got := ids(`SELECT json_extract(spec_json, '$.commentIds') FROM ade_runs WHERE id = '` + id + `'`); got != want {
			t.Errorf("run %s commentIds = %s, want %s", id, got, want)
		}
	}
	if got := ids(`SELECT json_extract(spec_json, '$.round') FROM ade_runs WHERE id = 'mixed'`); got != "2" {
		t.Errorf("mixed round = %s, want 2", got)
	}
	if got := ids(`SELECT spec_json FROM ade_runs WHERE id = 'noids'`); got != "" {
		t.Errorf("empty spec rewritten to %q", got)
	}
	if got := ids(`SELECT group_concat(id) FROM (SELECT id FROM tabs ORDER BY id)`); got != "new,other" {
		t.Errorf("tabs = %s, want new,other", got)
	}
	if got := ids(`SELECT count(*) FROM sqlite_master WHERE name = 'schema_quarantine'`); got != "0" {
		t.Error("schema_quarantine survived")
	}
	if got := ids(`SELECT group_concat(key) FROM settings`); got != "keep.me" {
		t.Errorf("settings = %s, want keep.me", got)
	}
}
