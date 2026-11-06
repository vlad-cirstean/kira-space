-- P543: a closed proof session attached to a Space task. task_id names a Space DB ade_tasks row
-- (no foreign key across the two databases until they merge); the link cascades with its session.
CREATE TABLE proof_task_links (
  session_id TEXT PRIMARY KEY REFERENCES proof_sessions(id) ON DELETE CASCADE,
  task_id TEXT NOT NULL,
  source TEXT NOT NULL CHECK (source IN ('agent', 'user')),
  kira_session_id TEXT NOT NULL DEFAULT '',
  step_id TEXT NOT NULL DEFAULT '',
  step_name TEXT NOT NULL DEFAULT '',
  linked_at INTEGER NOT NULL
);
CREATE INDEX proof_task_links_task ON proof_task_links(task_id);
