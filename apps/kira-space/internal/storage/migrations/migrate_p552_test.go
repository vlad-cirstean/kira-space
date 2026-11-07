package migrations

import (
	"database/sql"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/internal/sqlitex"
)

const isoStamp = "2026-01-02T03:04:05.678Z"

func openAt82(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sqlitex.Open(filepath.Join(t.TempDir(), "kira.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	all, err := All()
	if err != nil {
		t.Fatal(err)
	}
	var upTo82 []sqlitex.Migration
	for _, m := range all {
		if m.Version <= 82 {
			upTo82 = append(upTo82, m)
		}
	}
	if err := sqlitex.Migrate(db, upTo82); err != nil {
		t.Fatal(err)
	}
	return db
}

func mustExec(t *testing.T, db *sql.DB, q string, args ...any) {
	t.Helper()
	if _, err := db.Exec(q, args...); err != nil {
		t.Fatalf("exec %q: %v", q, err)
	}
}

func scalar(t *testing.T, db *sql.DB, q string, args ...any) string {
	t.Helper()
	var v sql.NullString
	if err := db.QueryRow(q, args...).Scan(&v); err != nil {
		t.Fatalf("query %q: %v", q, err)
	}
	return v.String
}

func upgrade(t *testing.T, db *sql.DB) {
	t.Helper()
	all, err := All()
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlitex.Migrate(db, all); err != nil {
		t.Fatalf("upgrade: %v", err)
	}
}

// P552: a v82 database with bad rows upgrades with exact instants, repaired rows, a populated
// quarantine and no foreign key violation.
func TestP552Upgrade(t *testing.T) {
	db := openAt82(t)
	mustExec(t, db, `INSERT INTO api_environments (id, name, sort_order, is_active, created_at, updated_at)
		VALUES ('e1', 'one', 0, 1, ?, ?), ('e2', 'two', 1, 1, ?, ?)`, isoStamp, isoStamp, isoStamp, isoStamp)
	mustExec(t, db, `INSERT INTO ade_workflow_last_valid (file_name, workflow_json, recorded_at) VALUES ('flow.yaml', '{"id":"flow"}', 5)`)
	mustExec(t, db, `INSERT INTO ade_sessions (id, task_id, agent_session_id, cwd, state, started_at, last_active_at)
		VALUES ('s-orphan', 'no-such-task', 'a', '/', 'running', 1, 1)`)
	mustExec(t, db, `INSERT INTO script_run_logs (run_id, seq, stream, text) VALUES ('no-such-run', 1, 'stdout', 'x')`)
	mustExec(t, db, `INSERT INTO connections (id, name, kind, color, mode, password, created_at, updated_at)
		VALUES ('c1', 'Legacy', 'postgres', 'red', 'fields', 'sealed', ?, ?)`, isoStamp, isoStamp)

	upgrade(t, db)

	wantMs := time.Date(2026, 1, 2, 3, 4, 5, 678_000_000, time.UTC).UnixMilli()
	if got := scalar(t, db, `SELECT created_at FROM api_environments WHERE id = 'e1'`); got != itoa(wantMs) {
		t.Errorf("created_at = %s, want %d", got, wantMs)
	}
	if got := scalar(t, db, `SELECT group_concat(id || ':' || is_active) FROM (SELECT * FROM api_environments ORDER BY rowid)`); got != "e1:1,e2:0" {
		t.Errorf("active environments = %q, want only the first kept", got)
	}
	if got := scalar(t, db, `SELECT value_json FROM config_last_valid WHERE folder = 'workflows' AND file = 'flow.yaml'`); got != `{"id":"flow"}` {
		t.Errorf("workflow last valid = %q", got)
	}
	if got := scalar(t, db, `SELECT COUNT(*) FROM sqlite_master WHERE name = 'ade_workflow_last_valid'`); got != "0" {
		t.Error("ade_workflow_last_valid survived the merge")
	}
	if got := scalar(t, db, `SELECT COUNT(*) FROM ade_sessions`); got != "0" {
		t.Errorf("orphan session kept: %s rows", got)
	}
	if got := scalar(t, db, `SELECT COUNT(*) FROM script_run_logs`); got != "0" {
		t.Errorf("orphan log kept: %s rows", got)
	}
	for _, c := range []struct{ table, action string }{
		{"api_environments", "repaired"}, {"ade_sessions", "removed"}, {"script_run_logs", "removed"},
	} {
		if got := scalar(t, db, `SELECT COUNT(*) FROM schema_quarantine WHERE table_name = ? AND action = ?`, c.table, c.action); got == "0" {
			t.Errorf("no %s quarantine row for %s", c.action, c.table)
		}
	}
	if got := scalar(t, db, `SELECT password FROM connections WHERE id = 'c1'`); got != "sealed" {
		t.Errorf("legacy password = %q, want it carried while the export marker is absent", got)
	}
	if got := scalar(t, db, `SELECT COUNT(*) FROM pragma_foreign_key_check`); got != "0" {
		t.Errorf("foreign_key_check rows = %s", got)
	}
	if got := scalar(t, db, `PRAGMA foreign_keys`); got != "1" {
		t.Errorf("foreign_keys = %s after upgrade, want 1", got)
	}
	if got := scalar(t, db, `SELECT COUNT(*) FROM pragma_table_list WHERE schema = 'main' AND name = 'api_environments' AND strict = 1`); got != "1" {
		t.Error("api_environments is not STRICT")
	}
}

// With the connection export marker set the superseded columns and tables are gone.
func TestP552UpgradeDropsExportedLegacy(t *testing.T) {
	db := openAt82(t)
	mustExec(t, db, `INSERT INTO settings (key, value) VALUES ('config.migrated.connections', 'true')`)
	mustExec(t, db, `INSERT INTO connections (id, name, kind, color, mode, password, created_at, updated_at)
		VALUES ('c1', 'Done', 'postgres', 'red', 'fields', 'sealed', ?, ?)`, isoStamp, isoStamp)
	upgrade(t, db)
	if got := scalar(t, db, `SELECT COUNT(*) FROM pragma_table_info('connections') WHERE name IN ('password', 'uri')`); got != "0" {
		t.Errorf("legacy columns left: %s", got)
	}
	if got := scalar(t, db, `SELECT COUNT(*) FROM sqlite_master WHERE name IN ('connection_tree_filters', 'connection_ddl')`); got != "0" {
		t.Errorf("legacy tables left: %s", got)
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
