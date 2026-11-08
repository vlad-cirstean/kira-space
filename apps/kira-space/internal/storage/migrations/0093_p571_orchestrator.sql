-- kira:foreign-keys off
-- P571: task kind 'orchestrator', ade_tasks.parent_id (a child's orchestrator), handover sources
-- 'orchestrator', 'pass' and 'child', ade_task_handovers.from_task_id (the child behind a mirrored row).
-- Eleven tables reference ade_tasks, so both tables rebuild in place with foreign keys off.
CREATE TABLE ade_tasks_new (
  id TEXT NOT NULL PRIMARY KEY CHECK (id <> ''),
  kind TEXT NOT NULL CHECK (kind IN ('task', 'review', 'parked', 'orchestrator')),
  status_id TEXT NOT NULL DEFAULT 'todo' REFERENCES ade_task_statuses (id) ON DELETE NO ACTION DEFERRABLE INITIALLY DEFERRED,
  status_by TEXT NOT NULL DEFAULT 'auto' CHECK (status_by IN ('auto', 'user')),
  status_step TEXT NOT NULL DEFAULT '',
  color INTEGER NOT NULL CHECK (color BETWEEN 0 AND 19),
  created_at INTEGER NOT NULL CHECK (created_at >= 0),
  archived_at INTEGER CHECK (archived_at IS NULL OR archived_at >= 0),
  title TEXT NOT NULL DEFAULT '',
  owner TEXT NOT NULL DEFAULT '',
  jira_key TEXT NOT NULL DEFAULT '',
  jira_url TEXT NOT NULL DEFAULT '',
  host_url TEXT NOT NULL DEFAULT '',
  workflow_id TEXT NOT NULL DEFAULT '',
  entry_step_id TEXT NOT NULL DEFAULT '',
  est TEXT NOT NULL DEFAULT '',
  workflow_hash TEXT NOT NULL DEFAULT '',
  since_json TEXT CHECK (since_json IS NULL OR json_valid(since_json)),
  workflow_json TEXT CHECK (workflow_json IS NULL OR json_valid(workflow_json)),
  parent_id TEXT REFERENCES ade_tasks (id) ON DELETE SET NULL
) STRICT;

INSERT INTO ade_tasks_new (id, kind, status_id, status_by, status_step, color, created_at, archived_at, title, owner, jira_key,
  jira_url, host_url, workflow_id, entry_step_id, est, workflow_hash, since_json, workflow_json, parent_id)
SELECT id, kind, status_id, status_by, status_step, color, created_at, archived_at, title, owner, jira_key,
  jira_url, host_url, workflow_id, entry_step_id, est, workflow_hash, since_json, workflow_json, NULL
FROM ade_tasks;

DROP TABLE ade_tasks;
ALTER TABLE ade_tasks_new RENAME TO ade_tasks;

CREATE INDEX ade_tasks_archived ON ade_tasks (archived_at) WHERE archived_at IS NOT NULL;
CREATE INDEX ade_tasks_parent ON ade_tasks (parent_id) WHERE parent_id IS NOT NULL;

CREATE TABLE ade_task_handovers_new (
  id TEXT NOT NULL PRIMARY KEY CHECK (id <> ''),
  task_id TEXT NOT NULL REFERENCES ade_tasks (id) ON DELETE CASCADE,
  source TEXT NOT NULL CHECK (source IN ('agent', 'run', 'user', 'orchestrator', 'pass', 'child')),
  state TEXT NOT NULL CHECK (state IN ('draft', 'handed', 'note')),
  finished INTEGER NOT NULL DEFAULT 0 CHECK (finished IN (0, 1)),
  dismissed INTEGER NOT NULL DEFAULT 0 CHECK (dismissed IN (0, 1)),
  edited INTEGER NOT NULL DEFAULT 0 CHECK (edited IN (0, 1)),
  created_at INTEGER NOT NULL CHECK (created_at >= 0),
  updated_at INTEGER NOT NULL CHECK (updated_at >= 0),
  from_step_id TEXT NOT NULL,
  from_step_name TEXT NOT NULL,
  to_step_id TEXT NOT NULL DEFAULT '',
  to_step_name TEXT NOT NULL DEFAULT '',
  session_id TEXT NOT NULL DEFAULT '',
  body TEXT NOT NULL,
  from_task_id TEXT REFERENCES ade_tasks (id) ON DELETE SET NULL
) STRICT;

INSERT INTO ade_task_handovers_new (id, task_id, source, state, finished, dismissed, edited, created_at, updated_at,
  from_step_id, from_step_name, to_step_id, to_step_name, session_id, body, from_task_id)
SELECT id, task_id, source, state, finished, dismissed, edited, created_at, updated_at,
  from_step_id, from_step_name, to_step_id, to_step_name, session_id, body, NULL
FROM ade_task_handovers;

DROP TABLE ade_task_handovers;
ALTER TABLE ade_task_handovers_new RENAME TO ade_task_handovers;

CREATE UNIQUE INDEX ade_task_handovers_draft ON ade_task_handovers (task_id, from_step_id) WHERE state = 'draft';
CREATE INDEX ade_task_handovers_task ON ade_task_handovers (task_id, created_at);
CREATE INDEX ade_task_handovers_from_task ON ade_task_handovers (from_task_id) WHERE from_task_id IS NOT NULL;
