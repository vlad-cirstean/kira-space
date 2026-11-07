-- kira:foreign-keys off
-- P552 ADE runs, sessions, logs, artifacts, audit. Rebuilds each table STRICT with checked types; rows that violate a new
-- constraint are repaired or removed and recorded in schema_quarantine.

-- ade_runs
CREATE TABLE ade_runs__n (
  id TEXT NOT NULL PRIMARY KEY CHECK (id <> ''),
  task_id TEXT NOT NULL REFERENCES ade_tasks (id) ON DELETE CASCADE,
  state TEXT NOT NULL CHECK (state IN ('pending', 'running', 'stuck', 'failed', 'back', 'done', 'review')),
  purpose TEXT NOT NULL DEFAULT '' CHECK (purpose IN ('', 'rebase', 'review')),
  attempt INTEGER NOT NULL CHECK (attempt >= 1),
  loops INTEGER NOT NULL DEFAULT 0 CHECK (loops >= 0),
  exit_code INTEGER,
  started_at INTEGER CHECK (started_at IS NULL OR started_at >= 0),
  finished_at INTEGER CHECK (finished_at IS NULL OR finished_at >= 0),
  step_id TEXT NOT NULL,
  branch_id TEXT NOT NULL DEFAULT '',
  session_id TEXT NOT NULL DEFAULT '',
  note TEXT NOT NULL DEFAULT '',
  summary TEXT NOT NULL DEFAULT '',
  launch_note TEXT NOT NULL DEFAULT '',
  launch_resume_id TEXT NOT NULL DEFAULT '',
  launch_prompt TEXT NOT NULL DEFAULT '',
  launch_extra TEXT NOT NULL DEFAULT '',
  outcome_json TEXT NOT NULL DEFAULT '' CHECK (outcome_json = '' OR json_valid(outcome_json)),
  spec_json TEXT NOT NULL DEFAULT '' CHECK (spec_json = '' OR json_valid(spec_json))
) STRICT;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_runs', json_object('id', "id", 'task_id', "task_id", 'step_id', "step_id", 'branch_id', "branch_id", 'attempt', "attempt", 'state', "state", 'loops', "loops", 'note', "note", 'summary', "summary", 'session_id', "session_id", 'exit_code', "exit_code", 'started_at', "started_at", 'finished_at', "finished_at", 'launch_note', "launch_note", 'launch_resume_id', "launch_resume_id", 'launch_prompt', "launch_prompt", 'launch_extra', "launch_extra", 'outcome_json', "outcome_json", 'purpose', "purpose", 'spec_json', "spec_json"), 'empty id', 'removed', 88, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_runs WHERE id = '';
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_runs', json_object('id', "id", 'task_id', "task_id", 'step_id', "step_id", 'branch_id', "branch_id", 'attempt', "attempt", 'state', "state", 'loops', "loops", 'note', "note", 'summary', "summary", 'session_id', "session_id", 'exit_code', "exit_code", 'started_at', "started_at", 'finished_at', "finished_at", 'launch_note', "launch_note", 'launch_resume_id', "launch_resume_id", 'launch_prompt', "launch_prompt", 'launch_extra', "launch_extra", 'outcome_json', "outcome_json", 'purpose', "purpose", 'spec_json', "spec_json"), 'orphan task_id', 'removed', 88, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_runs WHERE NOT EXISTS (SELECT 1 FROM ade_tasks p WHERE p.id = ade_runs."task_id");
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_runs', json_object('id', "id", 'task_id', "task_id", 'step_id', "step_id", 'branch_id', "branch_id", 'attempt', "attempt", 'state', "state", 'loops', "loops", 'note', "note", 'summary', "summary", 'session_id', "session_id", 'exit_code', "exit_code", 'started_at', "started_at", 'finished_at', "finished_at", 'launch_note', "launch_note", 'launch_resume_id', "launch_resume_id", 'launch_prompt', "launch_prompt", 'launch_extra', "launch_extra", 'outcome_json', "outcome_json", 'purpose', "purpose", 'spec_json', "spec_json"), 'invalid state', 'removed', 88, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_runs WHERE state NOT IN ('pending', 'running', 'stuck', 'failed', 'back', 'done', 'review');
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_runs', json_object('id', "id", 'task_id', "task_id", 'step_id', "step_id", 'branch_id', "branch_id", 'attempt', "attempt", 'state', "state", 'loops', "loops", 'note', "note", 'summary', "summary", 'session_id', "session_id", 'exit_code', "exit_code", 'started_at', "started_at", 'finished_at', "finished_at", 'launch_note', "launch_note", 'launch_resume_id', "launch_resume_id", 'launch_prompt', "launch_prompt", 'launch_extra', "launch_extra", 'outcome_json', "outcome_json", 'purpose', "purpose", 'spec_json', "spec_json"), 'invalid purpose', 'removed', 88, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_runs WHERE purpose NOT IN ('', 'rebase', 'review');
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_runs', json_object('id', "id", 'task_id', "task_id", 'step_id', "step_id", 'branch_id', "branch_id", 'attempt', "attempt", 'state', "state", 'loops', "loops", 'note', "note", 'summary', "summary", 'session_id', "session_id", 'exit_code', "exit_code", 'started_at', "started_at", 'finished_at', "finished_at", 'launch_note', "launch_note", 'launch_resume_id', "launch_resume_id", 'launch_prompt', "launch_prompt", 'launch_extra', "launch_extra", 'outcome_json', "outcome_json", 'purpose', "purpose", 'spec_json', "spec_json"), 'out-of-range attempt', 'repaired', 88, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_runs WHERE "attempt" < 1;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_runs', json_object('id', "id", 'task_id', "task_id", 'step_id', "step_id", 'branch_id', "branch_id", 'attempt', "attempt", 'state', "state", 'loops', "loops", 'note', "note", 'summary', "summary", 'session_id', "session_id", 'exit_code', "exit_code", 'started_at', "started_at", 'finished_at', "finished_at", 'launch_note', "launch_note", 'launch_resume_id', "launch_resume_id", 'launch_prompt', "launch_prompt", 'launch_extra', "launch_extra", 'outcome_json', "outcome_json", 'purpose', "purpose", 'spec_json', "spec_json"), 'out-of-range loops', 'repaired', 88, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_runs WHERE "loops" < 0;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_runs', json_object('id', "id", 'task_id', "task_id", 'step_id', "step_id", 'branch_id', "branch_id", 'attempt', "attempt", 'state', "state", 'loops', "loops", 'note', "note", 'summary', "summary", 'session_id', "session_id", 'exit_code', "exit_code", 'started_at', "started_at", 'finished_at', "finished_at", 'launch_note', "launch_note", 'launch_resume_id', "launch_resume_id", 'launch_prompt', "launch_prompt", 'launch_extra', "launch_extra", 'outcome_json', "outcome_json", 'purpose', "purpose", 'spec_json', "spec_json"), 'negative started_at', 'repaired', 88, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_runs WHERE started_at < 0;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_runs', json_object('id', "id", 'task_id', "task_id", 'step_id', "step_id", 'branch_id', "branch_id", 'attempt', "attempt", 'state', "state", 'loops', "loops", 'note', "note", 'summary', "summary", 'session_id', "session_id", 'exit_code', "exit_code", 'started_at', "started_at", 'finished_at', "finished_at", 'launch_note', "launch_note", 'launch_resume_id', "launch_resume_id", 'launch_prompt', "launch_prompt", 'launch_extra', "launch_extra", 'outcome_json', "outcome_json", 'purpose', "purpose", 'spec_json', "spec_json"), 'negative finished_at', 'repaired', 88, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_runs WHERE finished_at < 0;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_runs', json_object('id', "id", 'task_id', "task_id", 'step_id', "step_id", 'branch_id', "branch_id", 'attempt', "attempt", 'state', "state", 'loops', "loops", 'note', "note", 'summary', "summary", 'session_id', "session_id", 'exit_code', "exit_code", 'started_at', "started_at", 'finished_at', "finished_at", 'launch_note', "launch_note", 'launch_resume_id', "launch_resume_id", 'launch_prompt', "launch_prompt", 'launch_extra', "launch_extra", 'outcome_json', "outcome_json", 'purpose', "purpose", 'spec_json', "spec_json"), 'invalid JSON outcome_json', 'repaired', 88, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_runs WHERE outcome_json <> '' AND NOT json_valid(outcome_json);
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_runs', json_object('id', "id", 'task_id', "task_id", 'step_id', "step_id", 'branch_id', "branch_id", 'attempt', "attempt", 'state', "state", 'loops', "loops", 'note', "note", 'summary', "summary", 'session_id', "session_id", 'exit_code', "exit_code", 'started_at', "started_at", 'finished_at', "finished_at", 'launch_note', "launch_note", 'launch_resume_id', "launch_resume_id", 'launch_prompt', "launch_prompt", 'launch_extra', "launch_extra", 'outcome_json', "outcome_json", 'purpose', "purpose", 'spec_json', "spec_json"), 'invalid JSON spec_json', 'repaired', 88, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_runs WHERE spec_json <> '' AND NOT json_valid(spec_json);
INSERT INTO ade_runs__n (rowid, id, task_id, state, purpose, attempt, loops, exit_code, started_at, finished_at, step_id, branch_id, session_id, note, summary, launch_note, launch_resume_id, launch_prompt, launch_extra, outcome_json, spec_json)
  SELECT rowid, id, task_id, state, purpose, MAX(1, COALESCE("attempt", 1)), MAX(0, COALESCE("loops", 0)), exit_code, MAX(0, started_at), MAX(0, finished_at), step_id, branch_id, session_id, note, summary, launch_note, launch_resume_id, launch_prompt, launch_extra, CASE WHEN outcome_json = '' OR json_valid(outcome_json) THEN outcome_json ELSE '' END, CASE WHEN spec_json = '' OR json_valid(spec_json) THEN spec_json ELSE '' END
  FROM ade_runs WHERE NOT ((id = '')
    OR (NOT EXISTS (SELECT 1 FROM ade_tasks p WHERE p.id = ade_runs."task_id"))
    OR (state NOT IN ('pending', 'running', 'stuck', 'failed', 'back', 'done', 'review'))
    OR (purpose NOT IN ('', 'rebase', 'review')));
DROP TABLE ade_runs;
ALTER TABLE ade_runs__n RENAME TO ade_runs;
CREATE INDEX ade_runs_branch ON ade_runs (branch_id, state, purpose);
CREATE INDEX ade_runs_state ON ade_runs (state);
CREATE INDEX ade_runs_task ON ade_runs (task_id, purpose, attempt);

-- ade_reviews
CREATE TABLE ade_reviews__n (
  run_id TEXT NOT NULL PRIMARY KEY REFERENCES ade_runs (id) ON DELETE CASCADE,
  route TEXT NOT NULL CHECK (route IN ('implementer', 'new')),
  rounds INTEGER NOT NULL DEFAULT 0 CHECK (rounds >= 0),
  target_run_id TEXT NOT NULL DEFAULT '',
  dedicated_claude_id TEXT NOT NULL DEFAULT ''
) STRICT;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_reviews', json_object('run_id', "run_id", 'route', "route", 'target_run_id', "target_run_id", 'dedicated_claude_id', "dedicated_claude_id", 'rounds', "rounds"), 'orphan run_id', 'removed', 88, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_reviews WHERE NOT EXISTS (SELECT 1 FROM ade_runs p WHERE p.id = ade_reviews."run_id");
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_reviews', json_object('run_id', "run_id", 'route', "route", 'target_run_id', "target_run_id", 'dedicated_claude_id', "dedicated_claude_id", 'rounds', "rounds"), 'invalid route', 'removed', 88, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_reviews WHERE route NOT IN ('implementer', 'new');
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_reviews', json_object('run_id', "run_id", 'route', "route", 'target_run_id', "target_run_id", 'dedicated_claude_id', "dedicated_claude_id", 'rounds', "rounds"), 'out-of-range rounds', 'repaired', 88, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_reviews WHERE "rounds" < 0;
INSERT INTO ade_reviews__n (rowid, run_id, route, rounds, target_run_id, dedicated_claude_id)
  SELECT rowid, run_id, route, MAX(0, COALESCE("rounds", 0)), target_run_id, dedicated_claude_id
  FROM ade_reviews WHERE NOT ((NOT EXISTS (SELECT 1 FROM ade_runs p WHERE p.id = ade_reviews."run_id"))
    OR (route NOT IN ('implementer', 'new')));
DROP TABLE ade_reviews;
ALTER TABLE ade_reviews__n RENAME TO ade_reviews;

-- ade_run_sandbox
CREATE TABLE ade_run_sandbox__n (
  run_id TEXT NOT NULL PRIMARY KEY REFERENCES ade_runs (id) ON DELETE CASCADE,
  access TEXT NOT NULL CHECK (access IN ('write', 'read', 'off')),
  enforced INTEGER NOT NULL DEFAULT 0 CHECK (enforced IN (0, 1)),
  denial_count INTEGER NOT NULL DEFAULT 0 CHECK (denial_count >= 0),
  reason TEXT NOT NULL DEFAULT '',
  profile TEXT NOT NULL DEFAULT '',
  policy_json TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(policy_json)),
  denials_json TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(denials_json))
) STRICT;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_run_sandbox', json_object('run_id', "run_id", 'access', "access", 'enforced', "enforced", 'reason', "reason", 'policy_json', "policy_json", 'profile', "profile", 'denial_count', "denial_count", 'denials_json', "denials_json"), 'orphan run_id', 'removed', 88, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_run_sandbox WHERE NOT EXISTS (SELECT 1 FROM ade_runs p WHERE p.id = ade_run_sandbox."run_id");
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_run_sandbox', json_object('run_id', "run_id", 'access', "access", 'enforced', "enforced", 'reason', "reason", 'policy_json', "policy_json", 'profile', "profile", 'denial_count', "denial_count", 'denials_json', "denials_json"), 'invalid access', 'removed', 88, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_run_sandbox WHERE access NOT IN ('write', 'read', 'off');
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_run_sandbox', json_object('run_id', "run_id", 'access', "access", 'enforced', "enforced", 'reason', "reason", 'policy_json', "policy_json", 'profile', "profile", 'denial_count', "denial_count", 'denials_json', "denials_json"), 'non-boolean enforced', 'repaired', 88, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_run_sandbox WHERE "enforced" NOT IN (0, 1);
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_run_sandbox', json_object('run_id', "run_id", 'access', "access", 'enforced', "enforced", 'reason', "reason", 'policy_json', "policy_json", 'profile', "profile", 'denial_count', "denial_count", 'denials_json', "denials_json"), 'out-of-range denial_count', 'repaired', 88, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_run_sandbox WHERE "denial_count" < 0;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_run_sandbox', json_object('run_id', "run_id", 'access', "access", 'enforced', "enforced", 'reason', "reason", 'policy_json', "policy_json", 'profile', "profile", 'denial_count', "denial_count", 'denials_json', "denials_json"), 'invalid JSON policy_json', 'repaired', 88, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_run_sandbox WHERE NOT json_valid(policy_json);
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_run_sandbox', json_object('run_id', "run_id", 'access', "access", 'enforced', "enforced", 'reason', "reason", 'policy_json', "policy_json", 'profile', "profile", 'denial_count', "denial_count", 'denials_json', "denials_json"), 'invalid JSON denials_json', 'repaired', 88, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_run_sandbox WHERE NOT json_valid(denials_json);
INSERT INTO ade_run_sandbox__n (rowid, run_id, access, enforced, denial_count, reason, profile, policy_json, denials_json)
  SELECT rowid, run_id, access, COALESCE("enforced" <> 0, 0), MAX(0, COALESCE("denial_count", 0)), reason, profile, CASE WHEN json_valid(policy_json) THEN policy_json ELSE '{}' END, CASE WHEN json_valid(denials_json) THEN denials_json ELSE '[]' END
  FROM ade_run_sandbox WHERE NOT ((NOT EXISTS (SELECT 1 FROM ade_runs p WHERE p.id = ade_run_sandbox."run_id"))
    OR (access NOT IN ('write', 'read', 'off')));
DROP TABLE ade_run_sandbox;
ALTER TABLE ade_run_sandbox__n RENAME TO ade_run_sandbox;

-- ade_sessions
CREATE TABLE ade_sessions__n (
  id TEXT NOT NULL PRIMARY KEY CHECK (id <> ''),
  task_id TEXT NOT NULL REFERENCES ade_tasks (id) ON DELETE CASCADE,
  mode TEXT NOT NULL DEFAULT 'tui' CHECK (mode IN ('tui', 'headless')),
  state TEXT NOT NULL CHECK (state IN ('running', 'stopped')),
  purpose TEXT NOT NULL DEFAULT '' CHECK (purpose IN ('', 'review')),
  started_at INTEGER NOT NULL CHECK (started_at >= 0),
  last_active_at INTEGER NOT NULL CHECK (last_active_at >= 0),
  terminal_id TEXT,
  provider TEXT NOT NULL DEFAULT 'claude',
  branch_id TEXT NOT NULL DEFAULT '',
  step_id TEXT NOT NULL DEFAULT '',
  run_id TEXT NOT NULL DEFAULT '',
  resumes TEXT NOT NULL DEFAULT '',
  agent_session_id TEXT NOT NULL,
  cwd TEXT NOT NULL,
  CHECK (task_id <> '' AND (mode = 'tui' OR run_id <> ''))
) STRICT;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_sessions', json_object('id', "id", 'mode', "mode", 'task_id', "task_id", 'branch_id', "branch_id", 'step_id', "step_id", 'run_id', "run_id", 'resumes', "resumes", 'agent_session_id', "agent_session_id", 'cwd', "cwd", 'state', "state", 'terminal_id', "terminal_id", 'started_at', "started_at", 'last_active_at', "last_active_at", 'purpose', "purpose", 'provider', "provider"), 'empty id', 'removed', 88, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_sessions WHERE id = '';
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_sessions', json_object('id', "id", 'mode', "mode", 'task_id', "task_id", 'branch_id', "branch_id", 'step_id', "step_id", 'run_id', "run_id", 'resumes', "resumes", 'agent_session_id', "agent_session_id", 'cwd', "cwd", 'state', "state", 'terminal_id', "terminal_id", 'started_at', "started_at", 'last_active_at', "last_active_at", 'purpose', "purpose", 'provider', "provider"), 'orphan task_id', 'removed', 88, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_sessions WHERE NOT EXISTS (SELECT 1 FROM ade_tasks p WHERE p.id = ade_sessions."task_id");
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_sessions', json_object('id', "id", 'mode', "mode", 'task_id', "task_id", 'branch_id', "branch_id", 'step_id', "step_id", 'run_id', "run_id", 'resumes', "resumes", 'agent_session_id', "agent_session_id", 'cwd', "cwd", 'state', "state", 'terminal_id', "terminal_id", 'started_at', "started_at", 'last_active_at', "last_active_at", 'purpose', "purpose", 'provider', "provider"), 'invalid mode', 'removed', 88, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_sessions WHERE mode NOT IN ('tui', 'headless');
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_sessions', json_object('id', "id", 'mode', "mode", 'task_id', "task_id", 'branch_id', "branch_id", 'step_id', "step_id", 'run_id', "run_id", 'resumes', "resumes", 'agent_session_id', "agent_session_id", 'cwd', "cwd", 'state', "state", 'terminal_id', "terminal_id", 'started_at', "started_at", 'last_active_at', "last_active_at", 'purpose', "purpose", 'provider', "provider"), 'invalid state', 'removed', 88, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_sessions WHERE state NOT IN ('running', 'stopped');
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_sessions', json_object('id', "id", 'mode', "mode", 'task_id', "task_id", 'branch_id', "branch_id", 'step_id', "step_id", 'run_id', "run_id", 'resumes', "resumes", 'agent_session_id', "agent_session_id", 'cwd', "cwd", 'state', "state", 'terminal_id', "terminal_id", 'started_at', "started_at", 'last_active_at', "last_active_at", 'purpose', "purpose", 'provider', "provider"), 'invalid purpose', 'removed', 88, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_sessions WHERE purpose NOT IN ('', 'review');
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_sessions', json_object('id', "id", 'mode', "mode", 'task_id', "task_id", 'branch_id', "branch_id", 'step_id', "step_id", 'run_id', "run_id", 'resumes', "resumes", 'agent_session_id', "agent_session_id", 'cwd', "cwd", 'state', "state", 'terminal_id', "terminal_id", 'started_at', "started_at", 'last_active_at', "last_active_at", 'purpose', "purpose", 'provider', "provider"), 'negative started_at', 'repaired', 88, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_sessions WHERE started_at < 0;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_sessions', json_object('id', "id", 'mode', "mode", 'task_id', "task_id", 'branch_id', "branch_id", 'step_id', "step_id", 'run_id', "run_id", 'resumes', "resumes", 'agent_session_id', "agent_session_id", 'cwd', "cwd", 'state', "state", 'terminal_id', "terminal_id", 'started_at', "started_at", 'last_active_at', "last_active_at", 'purpose', "purpose", 'provider', "provider"), 'negative last_active_at', 'repaired', 88, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_sessions WHERE last_active_at < 0;
INSERT INTO ade_sessions__n (rowid, id, task_id, mode, state, purpose, started_at, last_active_at, terminal_id, provider, branch_id, step_id, run_id, resumes, agent_session_id, cwd)
  SELECT rowid, id, task_id, mode, state, purpose, MAX(0, started_at), MAX(0, last_active_at), terminal_id, provider, branch_id, step_id, run_id, resumes, agent_session_id, cwd
  FROM ade_sessions WHERE NOT ((id = '')
    OR (NOT EXISTS (SELECT 1 FROM ade_tasks p WHERE p.id = ade_sessions."task_id"))
    OR (mode NOT IN ('tui', 'headless'))
    OR (state NOT IN ('running', 'stopped'))
    OR (purpose NOT IN ('', 'review')));
DROP TABLE ade_sessions;
ALTER TABLE ade_sessions__n RENAME TO ade_sessions;
CREATE UNIQUE INDEX ade_sessions_review ON ade_sessions (task_id) WHERE purpose = 'review';
CREATE INDEX ade_sessions_task ON ade_sessions (task_id);
CREATE INDEX ade_sessions_running ON ade_sessions (task_id) WHERE state = 'running';

-- ade_session_sandbox
CREATE TABLE ade_session_sandbox__n (
  session_id TEXT NOT NULL PRIMARY KEY REFERENCES ade_sessions (id) ON DELETE CASCADE,
  access TEXT NOT NULL CHECK (access IN ('write', 'read')),
  enforced INTEGER NOT NULL DEFAULT 0 CHECK (enforced IN (0, 1)),
  denial_count INTEGER NOT NULL DEFAULT 0 CHECK (denial_count >= 0),
  reason TEXT NOT NULL DEFAULT '',
  profile TEXT NOT NULL DEFAULT '',
  policy_json TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(policy_json)),
  denials_json TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(denials_json))
) STRICT;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_session_sandbox', json_object('session_id', "session_id", 'access', "access", 'enforced', "enforced", 'reason', "reason", 'policy_json', "policy_json", 'profile', "profile", 'denial_count', "denial_count", 'denials_json', "denials_json"), 'orphan session_id', 'removed', 88, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_session_sandbox WHERE NOT EXISTS (SELECT 1 FROM ade_sessions p WHERE p.id = ade_session_sandbox."session_id");
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_session_sandbox', json_object('session_id', "session_id", 'access', "access", 'enforced', "enforced", 'reason', "reason", 'policy_json', "policy_json", 'profile', "profile", 'denial_count', "denial_count", 'denials_json', "denials_json"), 'invalid access', 'removed', 88, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_session_sandbox WHERE access NOT IN ('write', 'read');
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_session_sandbox', json_object('session_id', "session_id", 'access', "access", 'enforced', "enforced", 'reason', "reason", 'policy_json', "policy_json", 'profile', "profile", 'denial_count', "denial_count", 'denials_json', "denials_json"), 'non-boolean enforced', 'repaired', 88, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_session_sandbox WHERE "enforced" NOT IN (0, 1);
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_session_sandbox', json_object('session_id', "session_id", 'access', "access", 'enforced', "enforced", 'reason', "reason", 'policy_json', "policy_json", 'profile', "profile", 'denial_count', "denial_count", 'denials_json', "denials_json"), 'out-of-range denial_count', 'repaired', 88, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_session_sandbox WHERE "denial_count" < 0;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_session_sandbox', json_object('session_id', "session_id", 'access', "access", 'enforced', "enforced", 'reason', "reason", 'policy_json', "policy_json", 'profile', "profile", 'denial_count', "denial_count", 'denials_json', "denials_json"), 'invalid JSON policy_json', 'repaired', 88, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_session_sandbox WHERE NOT json_valid(policy_json);
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_session_sandbox', json_object('session_id', "session_id", 'access', "access", 'enforced', "enforced", 'reason', "reason", 'policy_json', "policy_json", 'profile', "profile", 'denial_count', "denial_count", 'denials_json', "denials_json"), 'invalid JSON denials_json', 'repaired', 88, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_session_sandbox WHERE NOT json_valid(denials_json);
INSERT INTO ade_session_sandbox__n (rowid, session_id, access, enforced, denial_count, reason, profile, policy_json, denials_json)
  SELECT rowid, session_id, access, COALESCE("enforced" <> 0, 0), MAX(0, COALESCE("denial_count", 0)), reason, profile, CASE WHEN json_valid(policy_json) THEN policy_json ELSE '{}' END, CASE WHEN json_valid(denials_json) THEN denials_json ELSE '[]' END
  FROM ade_session_sandbox WHERE NOT ((NOT EXISTS (SELECT 1 FROM ade_sessions p WHERE p.id = ade_session_sandbox."session_id"))
    OR (access NOT IN ('write', 'read')));
DROP TABLE ade_session_sandbox;
ALTER TABLE ade_session_sandbox__n RENAME TO ade_session_sandbox;

-- ade_logs
CREATE TABLE ade_logs__n (
  kind TEXT NOT NULL CHECK (kind IN ('run', 'setup')),
  id TEXT NOT NULL,
  task_id TEXT NOT NULL REFERENCES ade_tasks (id) ON DELETE CASCADE,
  next_seq INTEGER NOT NULL DEFAULT 1 CHECK (next_seq >= 0),
  bytes INTEGER NOT NULL DEFAULT 0 CHECK (bytes >= 0),
  truncated INTEGER NOT NULL DEFAULT 0 CHECK (truncated IN (0, 1)),
  PRIMARY KEY (kind, id)
) STRICT;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_logs', json_object('kind', "kind", 'id', "id", 'task_id', "task_id", 'next_seq', "next_seq", 'bytes', "bytes", 'truncated', "truncated"), 'invalid kind', 'removed', 88, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_logs WHERE kind NOT IN ('run', 'setup');
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_logs', json_object('kind', "kind", 'id', "id", 'task_id', "task_id", 'next_seq', "next_seq", 'bytes', "bytes", 'truncated', "truncated"), 'orphan task_id', 'removed', 88, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_logs WHERE NOT EXISTS (SELECT 1 FROM ade_tasks p WHERE p.id = ade_logs."task_id");
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_logs', json_object('kind', "kind", 'id', "id", 'task_id', "task_id", 'next_seq', "next_seq", 'bytes', "bytes", 'truncated', "truncated"), 'out-of-range next_seq', 'repaired', 88, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_logs WHERE "next_seq" < 0;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_logs', json_object('kind', "kind", 'id', "id", 'task_id', "task_id", 'next_seq', "next_seq", 'bytes', "bytes", 'truncated', "truncated"), 'out-of-range bytes', 'repaired', 88, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_logs WHERE "bytes" < 0;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_logs', json_object('kind', "kind", 'id', "id", 'task_id', "task_id", 'next_seq', "next_seq", 'bytes', "bytes", 'truncated', "truncated"), 'non-boolean truncated', 'repaired', 88, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_logs WHERE "truncated" NOT IN (0, 1);
INSERT INTO ade_logs__n (rowid, kind, id, task_id, next_seq, bytes, truncated)
  SELECT rowid, kind, id, task_id, MAX(0, COALESCE("next_seq", 1)), MAX(0, COALESCE("bytes", 0)), COALESCE("truncated" <> 0, 0)
  FROM ade_logs WHERE NOT ((kind NOT IN ('run', 'setup'))
    OR (NOT EXISTS (SELECT 1 FROM ade_tasks p WHERE p.id = ade_logs."task_id")));
DROP TABLE ade_logs;
ALTER TABLE ade_logs__n RENAME TO ade_logs;
CREATE INDEX ade_logs_task ON ade_logs (task_id);

-- ade_log_chunks
CREATE TABLE ade_log_chunks__n (
  kind TEXT NOT NULL,
  id TEXT NOT NULL,
  seq INTEGER NOT NULL CHECK (seq >= 0),
  at INTEGER NOT NULL CHECK (at >= 0),
  stream TEXT NOT NULL CHECK (stream IN ('stdout', 'stderr', 'event')),
  text TEXT NOT NULL,
  PRIMARY KEY (kind, id, seq),
  FOREIGN KEY (kind, id) REFERENCES ade_logs (kind, id) ON DELETE CASCADE
) STRICT;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_log_chunks', json_object('kind', "kind", 'id', "id", 'seq', "seq", 'at', "at", 'stream', "stream"), 'out-of-range seq', 'repaired', 88, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_log_chunks WHERE "seq" < 0;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_log_chunks', json_object('kind', "kind", 'id', "id", 'seq', "seq", 'at', "at", 'stream', "stream"), 'negative at', 'repaired', 88, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_log_chunks WHERE at < 0;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_log_chunks', json_object('kind', "kind", 'id', "id", 'seq', "seq", 'at', "at", 'stream', "stream"), 'invalid stream', 'removed', 88, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_log_chunks WHERE stream NOT IN ('stdout', 'stderr', 'event');
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_log_chunks', json_object('kind', "kind", 'id', "id", 'seq', "seq", 'at', "at", 'stream', "stream"), 'orphan log', 'removed', 88, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_log_chunks WHERE NOT EXISTS (SELECT 1 FROM ade_logs l WHERE l.kind = ade_log_chunks.kind AND l.id = ade_log_chunks.id);
INSERT INTO ade_log_chunks__n (rowid, kind, id, seq, at, stream, text)
  SELECT rowid, kind, id, MAX(0, COALESCE("seq", 0)), MAX(0, at), stream, text
  FROM ade_log_chunks WHERE NOT ((stream NOT IN ('stdout', 'stderr', 'event'))
    OR (NOT EXISTS (SELECT 1 FROM ade_logs l WHERE l.kind = ade_log_chunks.kind AND l.id = ade_log_chunks.id)));
DROP TABLE ade_log_chunks;
ALTER TABLE ade_log_chunks__n RENAME TO ade_log_chunks;

-- ade_task_artifacts
CREATE TABLE ade_task_artifacts__n (
  id TEXT NOT NULL PRIMARY KEY CHECK (id <> ''),
  task_id TEXT NOT NULL REFERENCES ade_tasks (id) ON DELETE CASCADE,
  kind TEXT NOT NULL CHECK (kind IN ('changeRequest', 'issue', 'branch', 'deployment', 'doc', 'link')),
  source TEXT NOT NULL CHECK (source IN ('agent', 'user')),
  created_at INTEGER NOT NULL CHECK (created_at >= 0),
  updated_at INTEGER NOT NULL CHECK (updated_at >= 0),
  title TEXT NOT NULL,
  url TEXT NOT NULL DEFAULT '',
  ref TEXT NOT NULL DEFAULT '',
  step_id TEXT NOT NULL DEFAULT '',
  run_id TEXT NOT NULL DEFAULT '',
  session_id TEXT NOT NULL DEFAULT '',
  CHECK (url <> '' OR ref <> '')
) STRICT;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_task_artifacts', json_object('id', "id", 'task_id', "task_id", 'kind', "kind", 'title', "title", 'url', "url", 'ref', "ref", 'source', "source", 'step_id', "step_id", 'run_id', "run_id", 'created_at', "created_at", 'updated_at', "updated_at", 'session_id', "session_id"), 'empty id', 'removed', 88, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_task_artifacts WHERE id = '';
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_task_artifacts', json_object('id', "id", 'task_id', "task_id", 'kind', "kind", 'title', "title", 'url', "url", 'ref', "ref", 'source', "source", 'step_id', "step_id", 'run_id', "run_id", 'created_at', "created_at", 'updated_at', "updated_at", 'session_id', "session_id"), 'orphan task_id', 'removed', 88, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_task_artifacts WHERE NOT EXISTS (SELECT 1 FROM ade_tasks p WHERE p.id = ade_task_artifacts."task_id");
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_task_artifacts', json_object('id', "id", 'task_id', "task_id", 'kind', "kind", 'title', "title", 'url', "url", 'ref', "ref", 'source', "source", 'step_id', "step_id", 'run_id', "run_id", 'created_at', "created_at", 'updated_at', "updated_at", 'session_id', "session_id"), 'invalid kind', 'removed', 88, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_task_artifacts WHERE kind NOT IN ('changeRequest', 'issue', 'branch', 'deployment', 'doc', 'link');
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_task_artifacts', json_object('id', "id", 'task_id', "task_id", 'kind', "kind", 'title', "title", 'url', "url", 'ref', "ref", 'source', "source", 'step_id', "step_id", 'run_id', "run_id", 'created_at', "created_at", 'updated_at', "updated_at", 'session_id', "session_id"), 'invalid source', 'removed', 88, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_task_artifacts WHERE source NOT IN ('agent', 'user');
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_task_artifacts', json_object('id', "id", 'task_id', "task_id", 'kind', "kind", 'title', "title", 'url', "url", 'ref', "ref", 'source', "source", 'step_id', "step_id", 'run_id', "run_id", 'created_at', "created_at", 'updated_at', "updated_at", 'session_id', "session_id"), 'negative created_at', 'repaired', 88, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_task_artifacts WHERE created_at < 0;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_task_artifacts', json_object('id', "id", 'task_id', "task_id", 'kind', "kind", 'title', "title", 'url', "url", 'ref', "ref", 'source', "source", 'step_id', "step_id", 'run_id', "run_id", 'created_at', "created_at", 'updated_at', "updated_at", 'session_id', "session_id"), 'negative updated_at', 'repaired', 88, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_task_artifacts WHERE updated_at < 0;
INSERT INTO ade_task_artifacts__n (rowid, id, task_id, kind, source, created_at, updated_at, title, url, ref, step_id, run_id, session_id)
  SELECT rowid, id, task_id, kind, source, MAX(0, created_at), MAX(0, updated_at), title, url, ref, step_id, run_id, session_id
  FROM ade_task_artifacts WHERE NOT ((id = '')
    OR (NOT EXISTS (SELECT 1 FROM ade_tasks p WHERE p.id = ade_task_artifacts."task_id"))
    OR (kind NOT IN ('changeRequest', 'issue', 'branch', 'deployment', 'doc', 'link'))
    OR (source NOT IN ('agent', 'user')));
DROP TABLE ade_task_artifacts;
ALTER TABLE ade_task_artifacts__n RENAME TO ade_task_artifacts;
CREATE INDEX ade_task_artifacts_task ON ade_task_artifacts (task_id, created_at);
CREATE UNIQUE INDEX ade_task_artifacts_url ON ade_task_artifacts (task_id, url) WHERE url <> '';

-- ade_task_handovers
CREATE TABLE ade_task_handovers__n (
  id TEXT NOT NULL PRIMARY KEY CHECK (id <> ''),
  task_id TEXT NOT NULL REFERENCES ade_tasks (id) ON DELETE CASCADE,
  source TEXT NOT NULL CHECK (source IN ('agent', 'run', 'user')),
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
  body TEXT NOT NULL
) STRICT;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_task_handovers', json_object('id', "id", 'task_id', "task_id", 'from_step_id', "from_step_id", 'from_step_name', "from_step_name", 'to_step_id', "to_step_id", 'to_step_name', "to_step_name", 'body', "body", 'source', "source", 'state', "state", 'finished', "finished", 'dismissed', "dismissed", 'edited', "edited", 'created_at', "created_at", 'updated_at', "updated_at", 'session_id', "session_id"), 'empty id', 'removed', 88, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_task_handovers WHERE id = '';
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_task_handovers', json_object('id', "id", 'task_id', "task_id", 'from_step_id', "from_step_id", 'from_step_name', "from_step_name", 'to_step_id', "to_step_id", 'to_step_name', "to_step_name", 'body', "body", 'source', "source", 'state', "state", 'finished', "finished", 'dismissed', "dismissed", 'edited', "edited", 'created_at', "created_at", 'updated_at', "updated_at", 'session_id', "session_id"), 'orphan task_id', 'removed', 88, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_task_handovers WHERE NOT EXISTS (SELECT 1 FROM ade_tasks p WHERE p.id = ade_task_handovers."task_id");
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_task_handovers', json_object('id', "id", 'task_id', "task_id", 'from_step_id', "from_step_id", 'from_step_name', "from_step_name", 'to_step_id', "to_step_id", 'to_step_name', "to_step_name", 'body', "body", 'source', "source", 'state', "state", 'finished', "finished", 'dismissed', "dismissed", 'edited', "edited", 'created_at', "created_at", 'updated_at', "updated_at", 'session_id', "session_id"), 'invalid source', 'removed', 88, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_task_handovers WHERE source NOT IN ('agent', 'run', 'user');
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_task_handovers', json_object('id', "id", 'task_id', "task_id", 'from_step_id', "from_step_id", 'from_step_name', "from_step_name", 'to_step_id', "to_step_id", 'to_step_name', "to_step_name", 'body', "body", 'source', "source", 'state', "state", 'finished', "finished", 'dismissed', "dismissed", 'edited', "edited", 'created_at', "created_at", 'updated_at', "updated_at", 'session_id', "session_id"), 'invalid state', 'removed', 88, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_task_handovers WHERE state NOT IN ('draft', 'handed', 'note');
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_task_handovers', json_object('id', "id", 'task_id', "task_id", 'from_step_id', "from_step_id", 'from_step_name', "from_step_name", 'to_step_id', "to_step_id", 'to_step_name', "to_step_name", 'body', "body", 'source', "source", 'state', "state", 'finished', "finished", 'dismissed', "dismissed", 'edited', "edited", 'created_at', "created_at", 'updated_at', "updated_at", 'session_id', "session_id"), 'non-boolean finished', 'repaired', 88, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_task_handovers WHERE "finished" NOT IN (0, 1);
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_task_handovers', json_object('id', "id", 'task_id', "task_id", 'from_step_id', "from_step_id", 'from_step_name', "from_step_name", 'to_step_id', "to_step_id", 'to_step_name', "to_step_name", 'body', "body", 'source', "source", 'state', "state", 'finished', "finished", 'dismissed', "dismissed", 'edited', "edited", 'created_at', "created_at", 'updated_at', "updated_at", 'session_id', "session_id"), 'non-boolean dismissed', 'repaired', 88, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_task_handovers WHERE "dismissed" NOT IN (0, 1);
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_task_handovers', json_object('id', "id", 'task_id', "task_id", 'from_step_id', "from_step_id", 'from_step_name', "from_step_name", 'to_step_id', "to_step_id", 'to_step_name', "to_step_name", 'body', "body", 'source', "source", 'state', "state", 'finished', "finished", 'dismissed', "dismissed", 'edited', "edited", 'created_at', "created_at", 'updated_at', "updated_at", 'session_id', "session_id"), 'non-boolean edited', 'repaired', 88, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_task_handovers WHERE "edited" NOT IN (0, 1);
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_task_handovers', json_object('id', "id", 'task_id', "task_id", 'from_step_id', "from_step_id", 'from_step_name', "from_step_name", 'to_step_id', "to_step_id", 'to_step_name', "to_step_name", 'body', "body", 'source', "source", 'state', "state", 'finished', "finished", 'dismissed', "dismissed", 'edited', "edited", 'created_at', "created_at", 'updated_at', "updated_at", 'session_id', "session_id"), 'negative created_at', 'repaired', 88, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_task_handovers WHERE created_at < 0;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_task_handovers', json_object('id', "id", 'task_id', "task_id", 'from_step_id', "from_step_id", 'from_step_name', "from_step_name", 'to_step_id', "to_step_id", 'to_step_name', "to_step_name", 'body', "body", 'source', "source", 'state', "state", 'finished', "finished", 'dismissed', "dismissed", 'edited', "edited", 'created_at', "created_at", 'updated_at', "updated_at", 'session_id', "session_id"), 'negative updated_at', 'repaired', 88, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_task_handovers WHERE updated_at < 0;
INSERT INTO ade_task_handovers__n (rowid, id, task_id, source, state, finished, dismissed, edited, created_at, updated_at, from_step_id, from_step_name, to_step_id, to_step_name, session_id, body)
  SELECT rowid, id, task_id, source, state, COALESCE("finished" <> 0, 0), COALESCE("dismissed" <> 0, 0), COALESCE("edited" <> 0, 0), MAX(0, created_at), MAX(0, updated_at), from_step_id, from_step_name, to_step_id, to_step_name, session_id, body
  FROM ade_task_handovers WHERE NOT ((id = '')
    OR (NOT EXISTS (SELECT 1 FROM ade_tasks p WHERE p.id = ade_task_handovers."task_id"))
    OR (source NOT IN ('agent', 'run', 'user'))
    OR (state NOT IN ('draft', 'handed', 'note')));
DROP TABLE ade_task_handovers;
ALTER TABLE ade_task_handovers__n RENAME TO ade_task_handovers;
CREATE UNIQUE INDEX ade_task_handovers_draft ON ade_task_handovers (task_id, from_step_id) WHERE state = 'draft';
CREATE INDEX ade_task_handovers_task ON ade_task_handovers (task_id, created_at);

-- ade_tracked_items
CREATE TABLE ade_tracked_items__n (
  task_id TEXT NOT NULL REFERENCES ade_tasks (id) ON DELETE CASCADE,
  item_key TEXT NOT NULL,
  kind TEXT NOT NULL CHECK (kind IN ('changeRequest', 'issue', 'branch')),
  seen_at INTEGER NOT NULL CHECK (seen_at >= 0),
  observed_at INTEGER NOT NULL CHECK (observed_at >= 0),
  changed_at INTEGER NOT NULL DEFAULT 0 CHECK (changed_at >= 0),
  seen_json TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(seen_json)),
  current_json TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(current_json)),
  PRIMARY KEY (task_id, item_key)
) STRICT;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_tracked_items', json_object('task_id', "task_id", 'item_key', "item_key", 'kind', "kind", 'seen_json', "seen_json", 'current_json', "current_json", 'seen_at', "seen_at", 'observed_at', "observed_at", 'changed_at', "changed_at"), 'orphan task_id', 'removed', 88, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_tracked_items WHERE NOT EXISTS (SELECT 1 FROM ade_tasks p WHERE p.id = ade_tracked_items."task_id");
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_tracked_items', json_object('task_id', "task_id", 'item_key', "item_key", 'kind', "kind", 'seen_json', "seen_json", 'current_json', "current_json", 'seen_at', "seen_at", 'observed_at', "observed_at", 'changed_at', "changed_at"), 'invalid kind', 'removed', 88, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_tracked_items WHERE kind NOT IN ('changeRequest', 'issue', 'branch');
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_tracked_items', json_object('task_id', "task_id", 'item_key', "item_key", 'kind', "kind", 'seen_json', "seen_json", 'current_json', "current_json", 'seen_at', "seen_at", 'observed_at', "observed_at", 'changed_at', "changed_at"), 'negative seen_at', 'repaired', 88, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_tracked_items WHERE seen_at < 0;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_tracked_items', json_object('task_id', "task_id", 'item_key', "item_key", 'kind', "kind", 'seen_json', "seen_json", 'current_json', "current_json", 'seen_at', "seen_at", 'observed_at', "observed_at", 'changed_at', "changed_at"), 'negative observed_at', 'repaired', 88, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_tracked_items WHERE observed_at < 0;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_tracked_items', json_object('task_id', "task_id", 'item_key', "item_key", 'kind', "kind", 'seen_json', "seen_json", 'current_json', "current_json", 'seen_at', "seen_at", 'observed_at', "observed_at", 'changed_at', "changed_at"), 'negative changed_at', 'repaired', 88, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_tracked_items WHERE changed_at < 0;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_tracked_items', json_object('task_id', "task_id", 'item_key', "item_key", 'kind', "kind", 'seen_json', "seen_json", 'current_json', "current_json", 'seen_at', "seen_at", 'observed_at', "observed_at", 'changed_at', "changed_at"), 'invalid JSON seen_json', 'repaired', 88, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_tracked_items WHERE NOT json_valid(seen_json);
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_tracked_items', json_object('task_id', "task_id", 'item_key', "item_key", 'kind', "kind", 'seen_json', "seen_json", 'current_json', "current_json", 'seen_at', "seen_at", 'observed_at', "observed_at", 'changed_at', "changed_at"), 'invalid JSON current_json', 'repaired', 88, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_tracked_items WHERE NOT json_valid(current_json);
INSERT INTO ade_tracked_items__n (rowid, task_id, item_key, kind, seen_at, observed_at, changed_at, seen_json, current_json)
  SELECT rowid, task_id, item_key, kind, MAX(0, seen_at), MAX(0, observed_at), MAX(0, changed_at), CASE WHEN json_valid(seen_json) THEN seen_json ELSE '{}' END, CASE WHEN json_valid(current_json) THEN current_json ELSE '{}' END
  FROM ade_tracked_items WHERE NOT ((NOT EXISTS (SELECT 1 FROM ade_tasks p WHERE p.id = ade_tracked_items."task_id"))
    OR (kind NOT IN ('changeRequest', 'issue', 'branch')));
DROP TABLE ade_tracked_items;
ALTER TABLE ade_tracked_items__n RENAME TO ade_tracked_items;

-- ade_db_audit
CREATE TABLE ade_db_audit__n (
  id TEXT NOT NULL PRIMARY KEY CHECK (id <> ''),
  task_id TEXT NOT NULL REFERENCES ade_tasks (id) ON DELETE CASCADE,
  call_id TEXT REFERENCES ade_db_audit (id) ON DELETE CASCADE,
  kind TEXT NOT NULL CHECK (kind IN ('call', 'approval')),
  started_ms INTEGER NOT NULL CHECK (started_ms >= 0),
  ended_ms INTEGER CHECK (ended_ms IS NULL OR ended_ms >= 0),
  statement_len INTEGER NOT NULL DEFAULT 0 CHECK (statement_len >= 0),
  statement_withheld INTEGER NOT NULL DEFAULT 0 CHECK (statement_withheld IN (0, 1)),
  statement_truncated INTEGER NOT NULL DEFAULT 0 CHECK (statement_truncated IN (0, 1)),
  session_id TEXT NOT NULL,
  step_id TEXT NOT NULL,
  run_id TEXT NOT NULL,
  status TEXT NOT NULL,
  tool TEXT NOT NULL DEFAULT '',
  connection_id TEXT,
  connection_name TEXT NOT NULL DEFAULT '',
  mode TEXT NOT NULL DEFAULT '',
  target TEXT NOT NULL DEFAULT '',
  subject TEXT NOT NULL DEFAULT '',
  reason TEXT NOT NULL DEFAULT '',
  stage TEXT NOT NULL DEFAULT '',
  exemption_json TEXT CHECK (exemption_json IS NULL OR json_valid(exemption_json)),
  statement TEXT,
  summary TEXT,
  error TEXT
) STRICT;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_db_audit', json_object('id', "id", 'task_id', "task_id", 'call_id', "call_id", 'kind', "kind", 'session_id', "session_id", 'step_id', "step_id", 'run_id', "run_id", 'started_ms', "started_ms", 'ended_ms', "ended_ms", 'status', "status", 'tool', "tool", 'connection_id', "connection_id", 'connection_name', "connection_name", 'mode', "mode", 'target', "target", 'statement', "statement", 'statement_len', "statement_len", 'statement_withheld', "statement_withheld", 'statement_truncated', "statement_truncated", 'summary', "summary", 'error', "error", 'subject', "subject", 'reason', "reason", 'stage', "stage", 'exemption_json', "exemption_json"), 'empty id', 'removed', 88, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_db_audit WHERE id = '';
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_db_audit', json_object('id', "id", 'task_id', "task_id", 'call_id', "call_id", 'kind', "kind", 'session_id', "session_id", 'step_id', "step_id", 'run_id', "run_id", 'started_ms', "started_ms", 'ended_ms', "ended_ms", 'status', "status", 'tool', "tool", 'connection_id', "connection_id", 'connection_name', "connection_name", 'mode', "mode", 'target', "target", 'statement', "statement", 'statement_len', "statement_len", 'statement_withheld', "statement_withheld", 'statement_truncated', "statement_truncated", 'summary', "summary", 'error', "error", 'subject', "subject", 'reason', "reason", 'stage', "stage", 'exemption_json', "exemption_json"), 'orphan task_id', 'removed', 88, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_db_audit WHERE NOT EXISTS (SELECT 1 FROM ade_tasks p WHERE p.id = ade_db_audit."task_id");
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_db_audit', json_object('id', "id", 'task_id', "task_id", 'call_id', "call_id", 'kind', "kind", 'session_id', "session_id", 'step_id', "step_id", 'run_id', "run_id", 'started_ms', "started_ms", 'ended_ms', "ended_ms", 'status', "status", 'tool', "tool", 'connection_id', "connection_id", 'connection_name', "connection_name", 'mode', "mode", 'target', "target", 'statement', "statement", 'statement_len', "statement_len", 'statement_withheld', "statement_withheld", 'statement_truncated', "statement_truncated", 'summary', "summary", 'error', "error", 'subject', "subject", 'reason', "reason", 'stage', "stage", 'exemption_json', "exemption_json"), 'invalid kind', 'removed', 88, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_db_audit WHERE kind NOT IN ('call', 'approval');
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_db_audit', json_object('id', "id", 'task_id', "task_id", 'call_id', "call_id", 'kind', "kind", 'session_id', "session_id", 'step_id', "step_id", 'run_id', "run_id", 'started_ms', "started_ms", 'ended_ms', "ended_ms", 'status', "status", 'tool', "tool", 'connection_id', "connection_id", 'connection_name', "connection_name", 'mode', "mode", 'target', "target", 'statement', "statement", 'statement_len', "statement_len", 'statement_withheld', "statement_withheld", 'statement_truncated', "statement_truncated", 'summary', "summary", 'error', "error", 'subject', "subject", 'reason', "reason", 'stage', "stage", 'exemption_json', "exemption_json"), 'negative started_ms', 'repaired', 88, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_db_audit WHERE started_ms < 0;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_db_audit', json_object('id', "id", 'task_id', "task_id", 'call_id', "call_id", 'kind', "kind", 'session_id', "session_id", 'step_id', "step_id", 'run_id', "run_id", 'started_ms', "started_ms", 'ended_ms', "ended_ms", 'status', "status", 'tool', "tool", 'connection_id', "connection_id", 'connection_name', "connection_name", 'mode', "mode", 'target', "target", 'statement', "statement", 'statement_len', "statement_len", 'statement_withheld', "statement_withheld", 'statement_truncated', "statement_truncated", 'summary', "summary", 'error', "error", 'subject', "subject", 'reason', "reason", 'stage', "stage", 'exemption_json', "exemption_json"), 'negative ended_ms', 'repaired', 88, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_db_audit WHERE ended_ms < 0;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_db_audit', json_object('id', "id", 'task_id', "task_id", 'call_id', "call_id", 'kind', "kind", 'session_id', "session_id", 'step_id', "step_id", 'run_id', "run_id", 'started_ms', "started_ms", 'ended_ms', "ended_ms", 'status', "status", 'tool', "tool", 'connection_id', "connection_id", 'connection_name', "connection_name", 'mode', "mode", 'target', "target", 'statement', "statement", 'statement_len', "statement_len", 'statement_withheld', "statement_withheld", 'statement_truncated', "statement_truncated", 'summary', "summary", 'error', "error", 'subject', "subject", 'reason', "reason", 'stage', "stage", 'exemption_json', "exemption_json"), 'out-of-range statement_len', 'repaired', 88, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_db_audit WHERE "statement_len" < 0;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_db_audit', json_object('id', "id", 'task_id', "task_id", 'call_id', "call_id", 'kind', "kind", 'session_id', "session_id", 'step_id', "step_id", 'run_id', "run_id", 'started_ms', "started_ms", 'ended_ms', "ended_ms", 'status', "status", 'tool', "tool", 'connection_id', "connection_id", 'connection_name', "connection_name", 'mode', "mode", 'target', "target", 'statement', "statement", 'statement_len', "statement_len", 'statement_withheld', "statement_withheld", 'statement_truncated', "statement_truncated", 'summary', "summary", 'error', "error", 'subject', "subject", 'reason', "reason", 'stage', "stage", 'exemption_json', "exemption_json"), 'non-boolean statement_withheld', 'repaired', 88, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_db_audit WHERE "statement_withheld" NOT IN (0, 1);
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_db_audit', json_object('id', "id", 'task_id', "task_id", 'call_id', "call_id", 'kind', "kind", 'session_id', "session_id", 'step_id', "step_id", 'run_id', "run_id", 'started_ms', "started_ms", 'ended_ms', "ended_ms", 'status', "status", 'tool', "tool", 'connection_id', "connection_id", 'connection_name', "connection_name", 'mode', "mode", 'target', "target", 'statement', "statement", 'statement_len', "statement_len", 'statement_withheld', "statement_withheld", 'statement_truncated', "statement_truncated", 'summary', "summary", 'error', "error", 'subject', "subject", 'reason', "reason", 'stage', "stage", 'exemption_json', "exemption_json"), 'non-boolean statement_truncated', 'repaired', 88, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_db_audit WHERE "statement_truncated" NOT IN (0, 1);
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_db_audit', json_object('id', "id", 'task_id', "task_id", 'call_id', "call_id", 'kind', "kind", 'session_id', "session_id", 'step_id', "step_id", 'run_id', "run_id", 'started_ms', "started_ms", 'ended_ms', "ended_ms", 'status', "status", 'tool', "tool", 'connection_id', "connection_id", 'connection_name', "connection_name", 'mode', "mode", 'target', "target", 'statement', "statement", 'statement_len', "statement_len", 'statement_withheld', "statement_withheld", 'statement_truncated', "statement_truncated", 'summary', "summary", 'error', "error", 'subject', "subject", 'reason', "reason", 'stage', "stage", 'exemption_json', "exemption_json"), 'invalid JSON exemption_json', 'repaired', 88, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_db_audit WHERE exemption_json IS NOT NULL AND NOT json_valid(exemption_json);

CREATE TEMP TABLE ade_db_audit_bad AS WITH RECURSIVE bad(id) AS (
  SELECT id FROM ade_db_audit WHERE (id = '') OR (NOT EXISTS (SELECT 1 FROM ade_tasks p WHERE p.id = ade_db_audit."task_id")) OR (kind NOT IN ('call', 'approval')) OR (call_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM ade_db_audit p WHERE p.id = ade_db_audit.call_id))
  UNION SELECT c.id FROM ade_db_audit c JOIN bad ON c.call_id = bad.id
) SELECT id FROM bad;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_db_audit', json_object('id', "id", 'task_id', "task_id", 'call_id', "call_id", 'kind', "kind", 'session_id', "session_id", 'step_id', "step_id", 'run_id', "run_id", 'started_ms', "started_ms", 'ended_ms', "ended_ms", 'status', "status", 'tool', "tool", 'connection_id', "connection_id", 'connection_name', "connection_name", 'mode', "mode", 'target', "target", 'statement', "statement", 'statement_len', "statement_len", 'statement_withheld', "statement_withheld", 'statement_truncated', "statement_truncated", 'summary', "summary", 'error', "error", 'subject', "subject", 'reason', "reason", 'stage', "stage", 'exemption_json', "exemption_json"), 'removed with its parent', 'removed', 88, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_db_audit WHERE id IN (SELECT id FROM ade_db_audit_bad);
INSERT INTO ade_db_audit__n (rowid, id, task_id, call_id, kind, started_ms, ended_ms, statement_len, statement_withheld, statement_truncated, session_id, step_id, run_id, status, tool, connection_id, connection_name, mode, target, subject, reason, stage, exemption_json, statement, summary, error)
  SELECT rowid, id, task_id, call_id, kind, MAX(0, started_ms), MAX(0, ended_ms), MAX(0, COALESCE("statement_len", 0)), COALESCE("statement_withheld" <> 0, 0), COALESCE("statement_truncated" <> 0, 0), session_id, step_id, run_id, status, tool, connection_id, connection_name, mode, target, subject, reason, stage, CASE WHEN exemption_json IS NULL OR json_valid(exemption_json) THEN exemption_json ELSE NULL END, statement, summary, error
  FROM ade_db_audit WHERE NOT ((id IN (SELECT id FROM ade_db_audit_bad)));
DROP TABLE ade_db_audit;
DROP TABLE ade_db_audit_bad;
ALTER TABLE ade_db_audit__n RENAME TO ade_db_audit;
CREATE INDEX ade_db_audit_call ON ade_db_audit (call_id) WHERE call_id IS NOT NULL;
CREATE INDEX ade_db_audit_task ON ade_db_audit (task_id, started_ms);
