-- P242 Part 1: the Automations rename, script folders and the script run store.
UPDATE windows SET mode = 'automations' WHERE mode = 'terminal';
UPDATE tabs SET workspace_id = 'automations' WHERE workspace_id = 'terminal';

-- A script's folder: 'kira' = <app home>/automations/<id>, 'fixed' = working_dir, 'home' = the old
-- $HOME default, kept only for rows that predate the choice.
ALTER TABLE custom_scripts ADD COLUMN dir_mode TEXT NOT NULL DEFAULT 'kira' CHECK (dir_mode IN ('kira', 'fixed', 'home'));
UPDATE custom_scripts SET dir_mode = CASE WHEN working_dir = '' THEN 'home' ELSE 'fixed' END;

CREATE TABLE script_runs (
  id           TEXT PRIMARY KEY,
  script_id    TEXT NOT NULL,
  script_name  TEXT NOT NULL,
  color        TEXT NOT NULL DEFAULT 'none',
  kind         TEXT NOT NULL DEFAULT 'script',
  trigger_kind TEXT NOT NULL CHECK (trigger_kind IN ('terminal', 'manual', 'ade', 'scheduled')),
  state        TEXT NOT NULL CHECK (state IN ('running', 'done', 'failed', 'cancelled', 'blocked')),
  terminal_id  TEXT NOT NULL DEFAULT '',
  cwd          TEXT NOT NULL DEFAULT '',
  command      TEXT NOT NULL DEFAULT '',
  outcome_json TEXT NOT NULL DEFAULT '',
  created_at   INTEGER NOT NULL,
  started_at   INTEGER,
  finished_at  INTEGER
);
CREATE INDEX script_runs_created ON script_runs (created_at DESC);
CREATE INDEX script_runs_terminal ON script_runs (terminal_id) WHERE state = 'running';
