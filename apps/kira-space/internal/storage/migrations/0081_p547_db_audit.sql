-- P547: Kira DB audit trail. One row per session-server tool call and per approval, keyed by task.
-- connection_id has no FK: a deleted connection keeps its name in the trail.
CREATE TABLE ade_db_audit (
  id TEXT PRIMARY KEY,
  task_id TEXT NOT NULL REFERENCES ade_tasks(id) ON DELETE CASCADE,
  call_id TEXT REFERENCES ade_db_audit(id) ON DELETE CASCADE,
  kind TEXT NOT NULL CHECK (kind IN ('call', 'approval')),
  session_id TEXT NOT NULL, step_id TEXT NOT NULL, run_id TEXT NOT NULL,
  started_ms INTEGER NOT NULL, ended_ms INTEGER,
  status TEXT NOT NULL,
  tool TEXT NOT NULL DEFAULT '', connection_id TEXT, connection_name TEXT NOT NULL DEFAULT '',
  mode TEXT NOT NULL DEFAULT '', target TEXT NOT NULL DEFAULT '',
  statement TEXT, statement_len INTEGER NOT NULL DEFAULT 0,
  statement_withheld INTEGER NOT NULL DEFAULT 0, statement_truncated INTEGER NOT NULL DEFAULT 0,
  summary TEXT, error TEXT,
  subject TEXT NOT NULL DEFAULT '', reason TEXT NOT NULL DEFAULT '', stage TEXT NOT NULL DEFAULT '',
  exemption_json TEXT
);
CREATE INDEX ade_db_audit_task ON ade_db_audit (task_id, started_ms);
