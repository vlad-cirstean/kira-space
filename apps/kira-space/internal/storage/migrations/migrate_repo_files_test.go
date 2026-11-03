package migrations

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

// P474: migration 0065 moves ade_env_state's foreign key from the dropped env rows to the code repo.
func TestMigration65RepoFiles(t *testing.T) {
	db, err := sql.Open("sqlite", "file:"+t.TempDir()+"/kira.sqlite?_foreign_keys=1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	steps, err := All()
	if err != nil {
		t.Fatal(err)
	}
	exec := func(q string) {
		t.Helper()
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("exec %q: %v", q, err)
		}
	}
	for _, m := range steps {
		if m.Version <= 64 {
			exec(m.SQL)
		}
	}
	exec(`INSERT INTO code_repos (id, name, root, repo_id, sort_order, created_at) VALUES ('r1', 'shop', '/s', '/s', 0, 0)`)
	exec(`INSERT INTO ade_repo_envs (code_repo_id, name, deployed_sha_script, position) VALUES ('r1', 'staging', 'true', 0)`)
	exec(`INSERT INTO ade_env_state (code_repo_id, env, sha, checked_at) VALUES ('r1', 'staging', 'abc', 1)`)
	for _, m := range steps {
		if m.Version == 65 {
			exec(m.SQL)
		}
	}
	count := func() int {
		t.Helper()
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM ade_env_state`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if got := count(); got != 1 {
		t.Fatalf("env state rows = %d, want 1", got)
	}
	exec(`DROP TABLE ade_repo_envs`)
	if got := count(); got != 1 {
		t.Fatalf("env state rows after dropping ade_repo_envs = %d, want 1", got)
	}
	exec(`DELETE FROM code_repos WHERE id = 'r1'`)
	if got := count(); got != 0 {
		t.Fatalf("env state rows after deleting the repo = %d, want 0", got)
	}
}
