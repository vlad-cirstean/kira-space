-- G18 D3: per-repo display settings a user edits from the git graph's own dialog, not VS Code's
-- settings.json — seven kiraVersion.* keys that moved out of contributes.configuration entirely.
-- Shaped like the existing `settings` table's own per-leaf-JSON-row pattern, plus a `repo_id`
-- column: one row per (repository, leaf), never a blob per repository. `repo_id` is the app's own
-- RepoID (internal/gitclient/repo.go) for six of the seven keys; the seventh (kiraVersion.log.level)
-- is not actually a per-repo fact (G18 D14), so GitRepoSettingsRepo.Get/Set substitute the reserved
-- sentinel repo_id = '' for that one key specifically — RepoID can never itself be empty (D14), so
-- '' is a permanently collision-free "this row is not scoped to any repository" marker, needing no
-- schema change beyond this table already having a `repo_id` column.
CREATE TABLE git_repo_settings (
  repo_id TEXT NOT NULL,
  key     TEXT NOT NULL,
  value   TEXT NOT NULL,
  PRIMARY KEY (repo_id, key)
);
