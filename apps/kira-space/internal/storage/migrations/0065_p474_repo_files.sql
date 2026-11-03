-- P474: repo config is files. ade_env_state stops referencing ade_repo_envs (dropped by the Go export step,
-- which would otherwise cascade-delete these rows) and hangs off the code repo instead.
CREATE TABLE ade_env_state_new (
  code_repo_id TEXT NOT NULL REFERENCES code_repos (id) ON DELETE CASCADE,
  env TEXT NOT NULL,
  sha TEXT NOT NULL DEFAULT '', prev_sha TEXT NOT NULL DEFAULT '',
  error TEXT NOT NULL DEFAULT '', checked_at INTEGER NOT NULL,
  PRIMARY KEY (code_repo_id, env)
);
INSERT INTO ade_env_state_new (code_repo_id, env, sha, prev_sha, error, checked_at)
SELECT code_repo_id, env, sha, prev_sha, error, checked_at FROM ade_env_state
WHERE code_repo_id IN (SELECT id FROM code_repos);
DROP TABLE ade_env_state;
ALTER TABLE ade_env_state_new RENAME TO ade_env_state;
