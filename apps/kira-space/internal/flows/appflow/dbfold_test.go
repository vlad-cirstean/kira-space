package appflow_test

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/config"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/dbfold"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/dbfold/legacy"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage"
	"github.com/kirathecat/kira-studio/internal/appstorage"
)

func execAll(t *testing.T, db *sql.DB, stmts ...string) {
	t.Helper()
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("%s: %v", s, err)
		}
	}
}

func scalar(t *testing.T, db *sql.DB, q string) string {
	t.Helper()
	var v sql.NullString
	if err := db.QueryRow(q).Scan(&v); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
	return v.String
}

// newLegacyDB writes <home>/studio.db at the legacy schema with one connection, an op, a tab on a window
// Space does not know, Studio-only settings and a sealed connection secret.
func newLegacyDB(t *testing.T, home string) {
	t.Helper()
	steps, err := legacy.All()
	if err != nil {
		t.Fatal(err)
	}
	db, err := appstorage.OpenAt(config.ModuleDbPathAt(home), steps)
	if err != nil {
		t.Fatal(err)
	}
	execAll(t, db,
		`INSERT INTO connections (id, name, kind, color, mode, created_at, updated_at) VALUES ('c1', 'Main', 'postgres', 'red', 'rw', '2026-01-02T03:04:05.678Z', '2026-01-02T03:04:05.678Z')`,
		`INSERT INTO op_log (id, connection_id, started_at, kind, status) VALUES ('o1', 'c1', '2026-01-02T03:04:05.678Z', 'execute', 'ok')`,
		`INSERT INTO windows (key, "order") VALUES ('w-old', 0)`,
		`INSERT INTO tabs (id, connection_id, window_key, path, kind, state_json, "order", active, workspace_id)
			VALUES ('t1', 'c1', 'w-old', 'p', 'query', '{}', 0, 1, 'ws')`,
		`INSERT INTO settings (key, value) VALUES ('dbMcp.serverEnabled', 'true'), ('appearance.fontSize', '17'), ('config.migrated.runs', '1')`,
		`INSERT INTO ui_layout (key, value) VALUES ('modes.api.sidebar', '300'), ('panel.cellEditor.height', '120'), ('other', '1')`,
		`INSERT INTO secret_values (scope, owner, name, value, updated_ms) VALUES ('connection', 'c1', 'password', 'kira:v3:connection:aaa', 1)`,
		`INSERT INTO proof_sessions (id, name, name_key, state, created_at, updated_at) VALUES ('s1', 'one', 'one', 'closed', 1, 1), ('s2', 'two', 'two', 'closed', 1, 1)`,
		`INSERT INTO proof_task_links (session_id, task_id, source, linked_at) VALUES ('s1', 't-live', 'user', 1), ('s2', 't-gone', 'user', 1)`,
	)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
}

func assertFolded(t *testing.T, db *sql.DB, home string) {
	t.Helper()
	for q, want := range map[string]string{
		`SELECT name FROM connections WHERE id = 'c1'`:                                                   "Main",
		`SELECT kind || category || subject FROM op_log WHERE id = 'o1'`:                                 "executedbMain",
		`SELECT created_at || ',' || typeof(created_at) FROM connections WHERE id = 'c1'`:                "1767323045678,integer",
		`SELECT value FROM settings WHERE key = 'dbMcp.serverEnabled'`:                                   "true",
		`SELECT value FROM settings WHERE key = 'appearance.fontSize'`:                                   "17",
		`SELECT COUNT(*) FROM settings WHERE key = 'config.migrated.runs'`:                               "0",
		`SELECT COUNT(*) FROM ui_layout WHERE key LIKE 'modes.api.%' OR key = 'panel.cellEditor.height'`: "2",
		`SELECT COUNT(*) FROM ui_layout WHERE key = 'other'`:                                             "0",
		`SELECT value FROM secret_values WHERE scope = 'connection'`:                                     "kira:v3:connection:aaa",
		`SELECT COUNT(*) FROM tabs WHERE id = 't1'`:                                                      "1",
		`SELECT json_extract(value, '$.state') FROM settings WHERE key = 'dbfold.p544'`:                  "done",
	} {
		if got := scalar(t, db, q); got != want {
			t.Errorf("%s = %q, want %q", q, got, want)
		}
	}
	if got := scalar(t, db, `SELECT group_concat(session_id) FROM proof_task_links`); got != "s1" {
		t.Errorf("proof links = %q, want only the one whose task exists", got)
	}
	if got := scalar(t, db, `SELECT COUNT(*) FROM tabs t JOIN windows w ON w.key = t.window_key WHERE t.id = 't1'`); got != "1" {
		t.Errorf("orphan tab not on a Space window: %s", got)
	}
	if _, err := os.Stat(config.ModuleDbPathAt(home)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("studio.db still there: %v", err)
	}
}

// A first launch imports studio.db, backs both databases up and removes it; an interrupted run resumes to
// the same end state, and a further run changes nothing.
func TestDbFold(t *testing.T) {
	for _, interrupt := range []string{"", "upgrade", "import", "remove"} {
		t.Run("interrupt-"+interrupt, func(t *testing.T) {
			home := t.TempDir()
			newLegacyDB(t, home)
			space, err := storage.OpenAt(home)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = space.Close() })
			execAll(t, space.DB, `INSERT INTO ade_tasks (id, kind, color, created_at) VALUES ('t-live', 'task', 0, 1)`)
			opts := dbfold.Options{Home: home, Space: space.DB}
			if interrupt != "" {
				opts.Step = func(name string) error {
					if name == interrupt {
						return errors.New("interrupted")
					}
					return nil
				}
				if err := dbfold.Run(opts); err == nil {
					t.Fatalf("run did not stop at %s", interrupt)
				}
				opts.Step = nil
			}
			if err := dbfold.Run(opts); err != nil {
				t.Fatal(err)
			}
			assertFolded(t, space.DB, home)
			if err := dbfold.Run(opts); err != nil {
				t.Fatal(err)
			}
			assertFolded(t, space.DB, home)
			execAll(t, space.DB, `DELETE FROM ade_tasks WHERE id = 't-live'`)
			if got := scalar(t, space.DB, `SELECT COUNT(*) FROM proof_task_links`); got != "0" {
				t.Errorf("proof links after the task went = %s, want 0", got)
			}
			backups, _ := filepath.Glob(filepath.Join(home, "backups", "p544-*", "*.db"))
			if len(backups) != 2 {
				t.Errorf("backups = %v", backups)
			}
		})
	}
}

// No studio.db (a Space-only user or a fresh install) records a fresh fold.
func TestDbFoldFresh(t *testing.T) {
	home := t.TempDir()
	space, err := storage.OpenAt(home)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = space.Close() })
	if err := dbfold.Run(dbfold.Options{Home: home, Space: space.DB}); err != nil {
		t.Fatal(err)
	}
	if got := scalar(t, space.DB, `SELECT value FROM settings WHERE key = 'dbfold.p544'`); got != `{"state":"done","fresh":true}` {
		t.Errorf("marker = %s", got)
	}
}

// Rows the P552 schema rejects are repaired like the migrations repair them; a row whose parent is not
// taken is quarantined with its children instead of failing the import at COMMIT.
func TestDbFoldRepairsAndOrphans(t *testing.T) {
	home := t.TempDir()
	steps, err := legacy.All()
	if err != nil {
		t.Fatal(err)
	}
	db, err := appstorage.OpenAt(config.ModuleDbPathAt(home), steps)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	execAll(t, db,
		`PRAGMA foreign_keys = OFF`,
		`INSERT INTO connections (id, name, kind, color, mode, created_at, updated_at, port, read_only, mcp_read_mode)
			VALUES ('c-bad', 'Bad', 'postgres', 'red', 'rw', '2026-01-02T03:04:05Z', '2026-01-02T03:04:05Z', 0, 5, 'bogus'),
			       ('c-text', 'Text', 'postgres', 'red', 'rw', '2026-01-02T03:04:05Z', 'not a time', 'abc', 0, 'allow'),
			       ('', 'NoId', 'postgres', 'red', 'rw', '2026-01-02T03:04:05Z', '2026-01-02T03:04:05Z', 5432, 0, 'allow')`,
		`INSERT INTO saved_queries (id, connection_id, path, name, kind, body, created_at) VALUES
			('q-ok', 'c-bad', 'p', 'ok', 'console', '', '2026-01-02T03:04:05Z'),
			('q-orph', '', 'p', 'orphan', 'console', '', '2026-01-02T03:04:05Z')`,
		`INSERT INTO connection_mask_rules (id, connection_id, table_name, column_name, mask_kind, created_at, updated_at)
			VALUES ('m-orph', '', '*', 'email', 'email', '2026-01-02T03:04:05Z', '2026-01-02T03:04:05Z')`,
		`INSERT INTO windows (key, "order") VALUES ('w-old', 0)`,
		`INSERT INTO tabs (id, connection_id, window_key, path, kind, state_json, "order", active)
			VALUES ('t-orph', '', 'w-old', 'p', 'query', '{}', 0, 1)`,
	)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	space, err := storage.OpenAt(home)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = space.Close() })
	if err := dbfold.Run(dbfold.Options{Home: home, Space: space.DB}); err != nil {
		t.Fatalf("fold failed: %v", err)
	}
	for q, want := range map[string]string{
		`SELECT COALESCE(port, 'null') || ',' || read_only || ',' || mcp_read_mode FROM connections WHERE id = 'c-bad'`:                                "null,1,allow",
		`SELECT COALESCE(port, 'null') || ',' || updated_at FROM connections WHERE id = 'c-text'`:                                                      "null,0",
		`SELECT COUNT(*) FROM connections WHERE id = ''`:                                                                                               "0",
		`SELECT group_concat(id) FROM connection_saved_queries`:                                                                                        "q-ok",
		`SELECT COUNT(*) FROM connection_mask_rules`:                                                                                                   "0",
		`SELECT COALESCE(connection_id, 'null') FROM tabs WHERE id = 't-orph'`:                                                                         "null",
		`SELECT group_concat(table_name) FROM (SELECT table_name FROM schema_quarantine WHERE reason = 'parent row not imported' ORDER BY table_name)`: "connection_mask_rules,connection_saved_queries",
		`SELECT json_extract(value, '$.state') FROM settings WHERE key = 'dbfold.p544'`:                                                                "done",
	} {
		if got := scalar(t, space.DB, q); got != want {
			t.Errorf("%s = %q, want %q", q, got, want)
		}
	}
}
