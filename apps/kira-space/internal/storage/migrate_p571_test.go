package storage_test

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/migrations"
	"github.com/kirathecat/kira-studio/internal/sqlitex"
)

// 0093 rebuilds ade_tasks and ade_task_handovers with foreign keys off. Rows of both and of the tables that
// reference ade_tasks survive, the new values pass the widened CHECKs, and the new links null on parent delete.
func TestMigrateP571(t *testing.T) {
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
	exec := func(q string) {
		t.Helper()
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("%v\n%s", err, q)
		}
	}
	for _, q := range []string{
		`INSERT INTO code_repos (id, name, root, repo_id, sort_order, created_at) VALUES ('r', 'api', '/r', 'rid', 0, 1)`,
		`INSERT INTO ade_tasks (id, kind, color, created_at, title) VALUES ('t1', 'task', 0, 1, 'one'), ('t2', 'parked', 1, 2, 'two')`,
		`INSERT INTO ade_task_plan (task_id, position) VALUES ('t1', 0)`,
		`INSERT INTO ade_task_branches (id, task_id, code_repo_id, kind, position, added_at) VALUES ('b1', 't1', 'r', 'mine', 0, 1)`,
		`INSERT INTO ade_runs (id, task_id, state, attempt, step_id) VALUES ('run1', 't1', 'done', 1, 's')`,
		`INSERT INTO ade_task_handovers (id, task_id, source, state, created_at, updated_at, from_step_id, from_step_name, body)
			VALUES ('h1', 't1', 'user', 'draft', 5, 6, 's', 'Step', 'draft text'), ('h2', 't1', 'run', 'handed', 7, 7, 's', 'Step', 'run text')`,
	} {
		exec(q)
	}
	if err := sqlitex.Migrate(db, all); err != nil {
		t.Fatal(err)
	}

	one := func(q string) string {
		t.Helper()
		var got sql.NullString
		if err := db.QueryRow(q).Scan(&got); err != nil {
			t.Fatalf("%v\n%s", err, q)
		}
		return got.String
	}
	for q, want := range map[string]string{
		`SELECT group_concat(id || ':' || kind || ':' || title || ':' || coalesce(parent_id, 'null'), ',') FROM (SELECT * FROM ade_tasks ORDER BY id)`: "t1:task:one:null,t2:parked:two:null",
		`SELECT count(*) FROM ade_task_plan`:     "1",
		`SELECT count(*) FROM ade_task_branches`: "1",
		`SELECT count(*) FROM ade_runs`:          "1",
		`SELECT group_concat(id || ':' || body || ':' || coalesce(from_task_id, 'null'), ',') FROM (SELECT * FROM ade_task_handovers ORDER BY id)`: "h1:draft text:null,h2:run text:null",
		`PRAGMA foreign_keys`:                           "1",
		`SELECT count(*) FROM pragma_foreign_key_check`: "0",
	} {
		if got := one(q); got != want {
			t.Errorf("%s\n got %q, want %q", q, got, want)
		}
	}

	for _, q := range []string{
		`INSERT INTO ade_tasks (id, kind, color, created_at) VALUES ('orc', 'orchestrator', 2, 3)`,
		`INSERT INTO ade_tasks (id, kind, color, created_at, parent_id) VALUES ('kid', 'task', 3, 4, 'orc')`,
		`INSERT INTO ade_task_handovers (id, task_id, source, state, created_at, updated_at, from_step_id, from_step_name, body, from_task_id)
			VALUES ('brief', 'kid', 'orchestrator', 'handed', 8, 8, '', 'orc', 'do it', 'orc'),
			       ('pass', 'orc', 'pass', 'handed', 9, 9, 'done', '', 'again', NULL),
			       ('mirror', 'orc', 'child', 'note', 10, 10, '', 'Step', '', 'kid')`,
	} {
		exec(q)
	}
	if _, err := db.Exec(`INSERT INTO ade_tasks (id, kind, color, created_at) VALUES ('bad', 'swarm', 4, 5)`); err == nil {
		t.Error("unknown kind accepted")
	}
	if _, err := db.Exec(`INSERT INTO ade_task_handovers (id, task_id, source, state, created_at, updated_at, from_step_id, from_step_name, body)
		VALUES ('bad', 't1', 'robot', 'note', 1, 1, '', '', '')`); err == nil {
		t.Error("unknown source accepted")
	}

	exec(`DELETE FROM ade_tasks WHERE id = 'orc'`)
	if got := one(`SELECT coalesce(parent_id, 'null') FROM ade_tasks WHERE id = 'kid'`); got != "null" {
		t.Errorf("child parent_id after parent delete = %s, want null", got)
	}
	if got := one(`SELECT coalesce(from_task_id, 'null') FROM ade_task_handovers WHERE id = 'brief'`); got != "null" {
		t.Errorf("brief from_task_id after parent delete = %s, want null", got)
	}
	exec(`DELETE FROM ade_tasks WHERE id = 't1'`)
	if got := one(`SELECT count(*) FROM ade_runs`); got != "0" {
		t.Errorf("runs after task delete = %s, want 0 (cascade intact)", got)
	}
}
