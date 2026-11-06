-- P543: user notes join the handover chain as state 'note' rows; ade_tasks.notes goes.
CREATE TABLE ade_task_handovers_new (
  id TEXT PRIMARY KEY,
  task_id TEXT NOT NULL REFERENCES ade_tasks (id) ON DELETE CASCADE,
  from_step_id TEXT NOT NULL,
  from_step_name TEXT NOT NULL,
  to_step_id TEXT NOT NULL DEFAULT '',
  to_step_name TEXT NOT NULL DEFAULT '',
  body TEXT NOT NULL,
  source TEXT NOT NULL CHECK (source IN ('agent', 'run', 'user')),
  state TEXT NOT NULL CHECK (state IN ('draft', 'handed', 'note')),
  finished INTEGER NOT NULL DEFAULT 0 CHECK (finished IN (0, 1)),
  dismissed INTEGER NOT NULL DEFAULT 0 CHECK (dismissed IN (0, 1)),
  edited INTEGER NOT NULL DEFAULT 0 CHECK (edited IN (0, 1)),
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL,
  session_id TEXT NOT NULL DEFAULT ''
);
INSERT INTO ade_task_handovers_new (id, task_id, from_step_id, from_step_name, to_step_id, to_step_name, body, source, state, finished, dismissed, edited, created_at, updated_at, session_id)
SELECT id, task_id, from_step_id, from_step_name, to_step_id, to_step_name, body, source, state, finished, dismissed, edited, created_at, updated_at, session_id FROM ade_task_handovers;
DROP TABLE ade_task_handovers;
ALTER TABLE ade_task_handovers_new RENAME TO ade_task_handovers;
CREATE UNIQUE INDEX ade_task_handovers_draft ON ade_task_handovers (task_id, from_step_id) WHERE state = 'draft';
CREATE INDEX ade_task_handovers_task ON ade_task_handovers (task_id, created_at);
INSERT INTO ade_task_handovers (id, task_id, from_step_id, from_step_name, to_step_id, to_step_name, body, source, state, finished, dismissed, edited, created_at, updated_at, session_id)
SELECT lower(hex(randomblob(16))), id, '', '', '', '', notes, 'user', 'note', 0, 0, 0, created_at, created_at, '' FROM ade_tasks WHERE trim(notes, ' ' || char(9) || char(10) || char(13)) <> '';
ALTER TABLE ade_tasks DROP COLUMN notes;
