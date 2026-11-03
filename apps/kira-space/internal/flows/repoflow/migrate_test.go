package repoflow_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/repocfg"
)

// legacySchema is the pre-P474 tables, as migrations 0001 and 0008 made them.
var legacySchema = []string{
	`CREATE TABLE git_repo_settings (repo_id TEXT NOT NULL, key TEXT NOT NULL, value TEXT NOT NULL, PRIMARY KEY (repo_id, key))`,
	`ALTER TABLE ade_repo_config ADD COLUMN nickname TEXT NOT NULL DEFAULT ''`,
	`CREATE TABLE ade_repo_integration (
		code_repo_id TEXT NOT NULL REFERENCES code_repos (id) ON DELETE CASCADE,
		branch TEXT NOT NULL, position INTEGER NOT NULL, PRIMARY KEY (code_repo_id, branch))`,
	`CREATE TABLE ade_repo_envs (
		code_repo_id TEXT NOT NULL REFERENCES code_repos (id) ON DELETE CASCADE,
		name TEXT NOT NULL, deployed_sha_script TEXT NOT NULL DEFAULT '', position INTEGER NOT NULL,
		PRIMARY KEY (code_repo_id, name))`,
}

func TestRepoConfigMigration(t *testing.T) {
	app := flowharness.New(t)
	plain := app.NewRepo("plain")
	plain.Commit("init", map[string]string{"a.txt": "a\n"})
	legacy := app.NewRepo("legacy")
	legacy.Commit("init", map[string]string{"a.txt": "a\n"})
	plainRec := importVia(t, app, plain.Dir)
	rec := importVia(t, app, legacy.Dir)
	db := app.DB()

	stmts := append([]string{}, legacySchema...)
	stmts = append(stmts,
		`INSERT INTO git_repo_settings (repo_id, key, value) VALUES
			('`+rec.RepoID+`', 'graphPageSize', '200'),
			('`+rec.RepoID+`', 'worktreePrepareScript', '"make"'),
			('`+rec.RepoID+`', 'worktreeBasePath', '"relative/path"'),
			('`+rec.RepoID+`', 'gitHostProvider', '"not a provider"')`,
		`INSERT INTO ade_repo_config (code_repo_id, nickname) VALUES ('`+rec.ID+`', 'Legacy')
			ON CONFLICT (code_repo_id) DO UPDATE SET nickname = excluded.nickname`,
		`INSERT INTO ade_repo_integration (code_repo_id, branch, position) VALUES ('`+rec.ID+`', 'develop', 0)`,
		`INSERT INTO ade_repo_envs (code_repo_id, name, deployed_sha_script, position) VALUES ('`+rec.ID+`', 'prod', 'echo x', 0)`,
		`INSERT INTO ade_env_state (code_repo_id, env, sha, prev_sha, error, checked_at) VALUES ('`+rec.ID+`', 'prod', 'abc', '', '', 1)`,
		`DELETE FROM settings WHERE key = 'config.migrated.repos'`,
	)
	for _, q := range stmts {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("seed %.40s: %v", q, err)
		}
	}
	dir := filepath.Join(app.SpaceHome, repocfg.Folder)
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	app.Restart()

	got := repoWire(t, app, rec.ID)
	if got.Nickname != "Legacy" || got.PrepareScript != "make" || got.File == "" ||
		!slices.Equal(got.IntegrationBranches, []string{"develop"}) || len(got.Environments) != 1 || got.Environments[0].DeployedShaScript != "echo x" {
		t.Fatalf("Repos = %+v, want the legacy values read from the new file", got)
	}
	if got.WorktreeBasePath != "" {
		t.Fatalf("worktree base path = %q, want the relative legacy value dropped", got.WorktreeBasePath)
	}
	s, err := app.W.RepoConfig.Settings(rec.RepoID)
	if err != nil || s.GraphPageSize != 200 || s.GitHostProvider != "auto" {
		t.Fatalf("Settings = %+v, %v, want page size 200 and the invalid provider back at auto", s, err)
	}
	if plainRec := repoWire(t, app, plainRec.ID); plainRec.File != "" {
		t.Fatalf("a repo with only defaults got file %q", plainRec.File)
	}

	db = app.DB()
	for _, table := range []string{"git_repo_settings", "ade_repo_integration", "ade_repo_envs"} {
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name = ?`, table).Scan(&n); err != nil || n != 0 {
			t.Fatalf("legacy table %s still exists (%d, %v)", table, n, err)
		}
	}
	var sha string
	if err := db.QueryRow(`SELECT sha FROM ade_env_state WHERE code_repo_id = ? AND env = 'prod'`, rec.ID).Scan(&sha); err != nil || sha != "abc" {
		t.Fatalf("env state = %q, %v, want it kept across the export", sha, err)
	}

	if err := os.Remove(filepath.Join(dir, got.File+".yaml")); err != nil {
		t.Fatal(err)
	}
	app.Restart()
	if again := repoWire(t, app, rec.ID); again.File != "" || again.Nickname != "" {
		t.Fatalf("after deleting the file and a restart = %+v, want no second export", again)
	}
}
