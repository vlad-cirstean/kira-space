-- P509: a user may file a branch link under a task's links, so the kind check admits 'branch'.
CREATE TABLE ade_task_artifacts_new (
  id TEXT PRIMARY KEY,
  task_id TEXT NOT NULL REFERENCES ade_tasks (id) ON DELETE CASCADE,
  kind TEXT NOT NULL CHECK (kind IN ('changeRequest', 'issue', 'branch', 'deployment', 'doc', 'link')),
  title TEXT NOT NULL,
  url TEXT NOT NULL DEFAULT '',
  ref TEXT NOT NULL DEFAULT '',
  source TEXT NOT NULL CHECK (source IN ('agent', 'user')),
  step_id TEXT NOT NULL DEFAULT '',
  run_id TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL,
  session_id TEXT NOT NULL DEFAULT '',
  CHECK (url <> '' OR ref <> '')
);
INSERT INTO ade_task_artifacts_new (id, task_id, kind, title, url, ref, source, step_id, run_id, created_at, updated_at, session_id)
SELECT id, task_id, kind, title, url, ref, source, step_id, run_id, created_at, updated_at, session_id FROM ade_task_artifacts;
DROP TABLE ade_task_artifacts;
ALTER TABLE ade_task_artifacts_new RENAME TO ade_task_artifacts;
CREATE UNIQUE INDEX ade_task_artifacts_url ON ade_task_artifacts (task_id, url) WHERE url <> '';
CREATE INDEX ade_task_artifacts_task ON ade_task_artifacts (task_id, created_at);
