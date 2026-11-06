-- P242 Part 4: recurring scripts. A script may carry a schedule; a run may wait for the user's
-- answer ('waiting') or be skipped by the scheduler ('skipped').
ALTER TABLE custom_scripts ADD COLUMN schedule_json TEXT NOT NULL DEFAULT '';

CREATE TABLE script_runs_new (
  id           TEXT PRIMARY KEY,
  script_id    TEXT NOT NULL,
  script_name  TEXT NOT NULL,
  color        TEXT NOT NULL DEFAULT 'none',
  kind         TEXT NOT NULL DEFAULT 'script',
  trigger_kind TEXT NOT NULL CHECK (trigger_kind IN ('terminal', 'manual', 'ade', 'scheduled')),
  state        TEXT NOT NULL CHECK (state IN ('waiting', 'running', 'done', 'failed', 'cancelled', 'blocked', 'skipped')),
  terminal_id  TEXT NOT NULL DEFAULT '',
  cwd          TEXT NOT NULL DEFAULT '',
  command      TEXT NOT NULL DEFAULT '',
  outcome_json TEXT NOT NULL DEFAULT '',
  created_at   INTEGER NOT NULL,
  started_at   INTEGER,
  finished_at  INTEGER,
  model        TEXT NOT NULL DEFAULT '',
  session_id   TEXT NOT NULL DEFAULT '',
  prompt       TEXT NOT NULL DEFAULT '',
  params_json  TEXT NOT NULL DEFAULT '[]',
  tools_json   TEXT NOT NULL DEFAULT '{}',
  task_id      TEXT NOT NULL DEFAULT '',
  task_title   TEXT NOT NULL DEFAULT '',
  branch_id    TEXT NOT NULL DEFAULT '',
  branch_label TEXT NOT NULL DEFAULT ''
);
INSERT INTO script_runs_new (id, script_id, script_name, color, kind, trigger_kind, state, terminal_id, cwd, command, outcome_json,
  created_at, started_at, finished_at, model, session_id, prompt, params_json, tools_json, task_id, task_title, branch_id, branch_label)
SELECT id, script_id, script_name, color, kind, trigger_kind, state, terminal_id, cwd, command, outcome_json,
  created_at, started_at, finished_at, model, session_id, prompt, params_json, tools_json, task_id, task_title, branch_id, branch_label
FROM script_runs;
DROP TABLE script_runs;
ALTER TABLE script_runs_new RENAME TO script_runs;
CREATE INDEX script_runs_created ON script_runs (created_at DESC);
CREATE INDEX script_runs_terminal ON script_runs (terminal_id) WHERE state = 'running';
CREATE INDEX script_runs_task ON script_runs (task_id, created_at DESC);
CREATE INDEX script_runs_active ON script_runs (script_id) WHERE state IN ('running', 'waiting');
