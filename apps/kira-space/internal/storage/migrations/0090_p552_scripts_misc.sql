-- kira:foreign-keys off
-- P552 scripts, git host, docker, kube. Rebuilds each table STRICT with checked types; rows that violate a new
-- constraint are repaired or removed and recorded in schema_quarantine.

-- custom_scripts -> scripts
CREATE TABLE scripts (
  id TEXT NOT NULL PRIMARY KEY CHECK (id <> ''),
  kind TEXT NOT NULL DEFAULT 'script' CHECK (kind IN ('script', 'smart')),
  dir_mode TEXT NOT NULL DEFAULT 'kira' CHECK (dir_mode IN ('kira', 'fixed', 'worktree', 'home')),
  use_ade_dir INTEGER NOT NULL DEFAULT 1 CHECK (use_ade_dir IN (0, 1)),
  sort_order INTEGER NOT NULL CHECK (sort_order >= 0),
  created_at INTEGER NOT NULL CHECK (created_at >= 0),
  updated_at INTEGER NOT NULL CHECK (updated_at >= 0),
  name TEXT NOT NULL,
  command TEXT NOT NULL,
  working_dir TEXT NOT NULL DEFAULT '',
  color TEXT NOT NULL DEFAULT 'none',
  collection TEXT NOT NULL DEFAULT '',
  schedule_task_id TEXT NOT NULL DEFAULT '',
  schedule_branch_id TEXT NOT NULL DEFAULT '',
  params_json TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(params_json)),
  smart_json TEXT NOT NULL DEFAULT '' CHECK (smart_json = '' OR json_valid(smart_json)),
  schedule_json TEXT NOT NULL DEFAULT '' CHECK (schedule_json = '' OR json_valid(schedule_json)),
  config_json TEXT NOT NULL DEFAULT '' CHECK (config_json = '' OR json_valid(config_json))
) STRICT;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'scripts', json_object('id', "id", 'name', "name", 'command', "command", 'working_dir', "working_dir", 'dir_mode', "dir_mode", 'color', "color", 'collection', "collection", 'sort_order', "sort_order", 'created_at', "created_at", 'updated_at', "updated_at", 'kind', "kind", 'params_json', "params_json", 'smart_json', "smart_json", 'use_ade_dir', "use_ade_dir", 'schedule_json', "schedule_json", 'schedule_task_id', "schedule_task_id", 'schedule_branch_id', "schedule_branch_id", 'config_json', "config_json"), 'empty id', 'removed', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM custom_scripts WHERE id = '';
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'scripts', json_object('id', "id", 'name', "name", 'command', "command", 'working_dir', "working_dir", 'dir_mode', "dir_mode", 'color', "color", 'collection', "collection", 'sort_order', "sort_order", 'created_at', "created_at", 'updated_at', "updated_at", 'kind', "kind", 'params_json', "params_json", 'smart_json', "smart_json", 'use_ade_dir', "use_ade_dir", 'schedule_json', "schedule_json", 'schedule_task_id', "schedule_task_id", 'schedule_branch_id', "schedule_branch_id", 'config_json', "config_json"), 'invalid kind', 'removed', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM custom_scripts WHERE kind NOT IN ('script', 'smart');
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'scripts', json_object('id', "id", 'name', "name", 'command', "command", 'working_dir', "working_dir", 'dir_mode', "dir_mode", 'color', "color", 'collection', "collection", 'sort_order', "sort_order", 'created_at', "created_at", 'updated_at', "updated_at", 'kind', "kind", 'params_json', "params_json", 'smart_json', "smart_json", 'use_ade_dir', "use_ade_dir", 'schedule_json', "schedule_json", 'schedule_task_id', "schedule_task_id", 'schedule_branch_id', "schedule_branch_id", 'config_json', "config_json"), 'invalid dir_mode', 'removed', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM custom_scripts WHERE dir_mode NOT IN ('kira', 'fixed', 'worktree', 'home');
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'scripts', json_object('id', "id", 'name', "name", 'command', "command", 'working_dir', "working_dir", 'dir_mode', "dir_mode", 'color', "color", 'collection', "collection", 'sort_order', "sort_order", 'created_at', "created_at", 'updated_at', "updated_at", 'kind', "kind", 'params_json', "params_json", 'smart_json', "smart_json", 'use_ade_dir', "use_ade_dir", 'schedule_json', "schedule_json", 'schedule_task_id', "schedule_task_id", 'schedule_branch_id', "schedule_branch_id", 'config_json', "config_json"), 'non-boolean use_ade_dir', 'repaired', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM custom_scripts WHERE "use_ade_dir" NOT IN (0, 1);
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'scripts', json_object('id', "id", 'name', "name", 'command', "command", 'working_dir', "working_dir", 'dir_mode', "dir_mode", 'color', "color", 'collection', "collection", 'sort_order', "sort_order", 'created_at', "created_at", 'updated_at', "updated_at", 'kind', "kind", 'params_json', "params_json", 'smart_json', "smart_json", 'use_ade_dir', "use_ade_dir", 'schedule_json', "schedule_json", 'schedule_task_id', "schedule_task_id", 'schedule_branch_id', "schedule_branch_id", 'config_json', "config_json"), 'out-of-range sort_order', 'repaired', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM custom_scripts WHERE "sort_order" < 0;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'scripts', json_object('id', "id", 'name', "name", 'command', "command", 'working_dir', "working_dir", 'dir_mode', "dir_mode", 'color', "color", 'collection', "collection", 'sort_order', "sort_order", 'created_at', "created_at", 'updated_at', "updated_at", 'kind', "kind", 'params_json', "params_json", 'smart_json', "smart_json", 'use_ade_dir', "use_ade_dir", 'schedule_json', "schedule_json", 'schedule_task_id', "schedule_task_id", 'schedule_branch_id', "schedule_branch_id", 'config_json', "config_json"), 'negative created_at', 'repaired', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM custom_scripts WHERE created_at < 0;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'scripts', json_object('id', "id", 'name', "name", 'command', "command", 'working_dir', "working_dir", 'dir_mode', "dir_mode", 'color', "color", 'collection', "collection", 'sort_order', "sort_order", 'created_at', "created_at", 'updated_at', "updated_at", 'kind', "kind", 'params_json', "params_json", 'smart_json', "smart_json", 'use_ade_dir', "use_ade_dir", 'schedule_json', "schedule_json", 'schedule_task_id', "schedule_task_id", 'schedule_branch_id', "schedule_branch_id", 'config_json', "config_json"), 'negative updated_at', 'repaired', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM custom_scripts WHERE updated_at < 0;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'scripts', json_object('id', "id", 'name', "name", 'command', "command", 'working_dir', "working_dir", 'dir_mode', "dir_mode", 'color', "color", 'collection', "collection", 'sort_order', "sort_order", 'created_at', "created_at", 'updated_at', "updated_at", 'kind', "kind", 'params_json', "params_json", 'smart_json', "smart_json", 'use_ade_dir', "use_ade_dir", 'schedule_json', "schedule_json", 'schedule_task_id', "schedule_task_id", 'schedule_branch_id', "schedule_branch_id", 'config_json', "config_json"), 'invalid JSON params_json', 'repaired', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM custom_scripts WHERE NOT json_valid(params_json);
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'scripts', json_object('id', "id", 'name', "name", 'command', "command", 'working_dir', "working_dir", 'dir_mode', "dir_mode", 'color', "color", 'collection', "collection", 'sort_order', "sort_order", 'created_at', "created_at", 'updated_at', "updated_at", 'kind', "kind", 'params_json', "params_json", 'smart_json', "smart_json", 'use_ade_dir', "use_ade_dir", 'schedule_json', "schedule_json", 'schedule_task_id', "schedule_task_id", 'schedule_branch_id', "schedule_branch_id", 'config_json', "config_json"), 'invalid JSON smart_json', 'repaired', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM custom_scripts WHERE smart_json <> '' AND NOT json_valid(smart_json);
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'scripts', json_object('id', "id", 'name', "name", 'command', "command", 'working_dir', "working_dir", 'dir_mode', "dir_mode", 'color', "color", 'collection', "collection", 'sort_order', "sort_order", 'created_at', "created_at", 'updated_at', "updated_at", 'kind', "kind", 'params_json', "params_json", 'smart_json', "smart_json", 'use_ade_dir', "use_ade_dir", 'schedule_json', "schedule_json", 'schedule_task_id', "schedule_task_id", 'schedule_branch_id', "schedule_branch_id", 'config_json', "config_json"), 'invalid JSON schedule_json', 'repaired', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM custom_scripts WHERE schedule_json <> '' AND NOT json_valid(schedule_json);
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'scripts', json_object('id', "id", 'name', "name", 'command', "command", 'working_dir', "working_dir", 'dir_mode', "dir_mode", 'color', "color", 'collection', "collection", 'sort_order', "sort_order", 'created_at', "created_at", 'updated_at', "updated_at", 'kind', "kind", 'params_json', "params_json", 'smart_json', "smart_json", 'use_ade_dir', "use_ade_dir", 'schedule_json', "schedule_json", 'schedule_task_id', "schedule_task_id", 'schedule_branch_id', "schedule_branch_id", 'config_json', "config_json"), 'invalid JSON config_json', 'repaired', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM custom_scripts WHERE config_json <> '' AND NOT json_valid(config_json);
INSERT INTO scripts (rowid, id, kind, dir_mode, use_ade_dir, sort_order, created_at, updated_at, name, command, working_dir, color, collection, schedule_task_id, schedule_branch_id, params_json, smart_json, schedule_json, config_json)
  SELECT rowid, id, kind, dir_mode, COALESCE("use_ade_dir" <> 0, 1), MAX(0, COALESCE("sort_order", 0)), MAX(0, created_at), MAX(0, updated_at), name, command, working_dir, color, collection, schedule_task_id, schedule_branch_id, CASE WHEN json_valid(params_json) THEN params_json ELSE '[]' END, CASE WHEN smart_json = '' OR json_valid(smart_json) THEN smart_json ELSE '' END, CASE WHEN schedule_json = '' OR json_valid(schedule_json) THEN schedule_json ELSE '' END, CASE WHEN config_json = '' OR json_valid(config_json) THEN config_json ELSE '' END
  FROM custom_scripts WHERE NOT ((id = '')
    OR (kind NOT IN ('script', 'smart'))
    OR (dir_mode NOT IN ('kira', 'fixed', 'worktree', 'home')));
DROP TABLE custom_scripts;

-- custom_script_collections -> script_collections
CREATE TABLE script_collections (
  name TEXT NOT NULL PRIMARY KEY,
  sort_order INTEGER NOT NULL CHECK (sort_order >= 0)
) STRICT, WITHOUT ROWID;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'script_collections', json_object('name', "name", 'sort_order', "sort_order"), 'out-of-range sort_order', 'repaired', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM custom_script_collections WHERE "sort_order" < 0;
INSERT INTO script_collections (name, sort_order)
  SELECT name, MAX(0, COALESCE("sort_order", 0))
  FROM custom_script_collections;
DROP TABLE custom_script_collections;

-- script_runs
CREATE TABLE script_runs__n (
  id TEXT NOT NULL PRIMARY KEY CHECK (id <> ''),
  trigger_kind TEXT NOT NULL CHECK (trigger_kind IN ('terminal', 'manual', 'ade', 'scheduled', 'mcp')),
  state TEXT NOT NULL CHECK (state IN ('waiting', 'running', 'done', 'failed', 'cancelled', 'blocked', 'skipped')),
  restore_pending INTEGER NOT NULL DEFAULT 0 CHECK (restore_pending IN (0, 1)),
  created_at INTEGER NOT NULL CHECK (created_at >= 0),
  started_at INTEGER CHECK (started_at IS NULL OR started_at >= 0),
  finished_at INTEGER CHECK (finished_at IS NULL OR finished_at >= 0),
  script_id TEXT NOT NULL,
  script_name TEXT NOT NULL,
  color TEXT NOT NULL DEFAULT 'none',
  kind TEXT NOT NULL DEFAULT 'script',
  terminal_id TEXT NOT NULL DEFAULT '',
  cwd TEXT NOT NULL DEFAULT '',
  command TEXT NOT NULL DEFAULT '',
  model TEXT NOT NULL DEFAULT '',
  session_id TEXT NOT NULL DEFAULT '',
  task_id TEXT NOT NULL DEFAULT '',
  task_title TEXT NOT NULL DEFAULT '',
  branch_id TEXT NOT NULL DEFAULT '',
  branch_label TEXT NOT NULL DEFAULT '',
  resume_id TEXT NOT NULL DEFAULT '',
  resumed_from TEXT NOT NULL DEFAULT '',
  provider TEXT NOT NULL DEFAULT 'claude',
  prompt TEXT NOT NULL DEFAULT '',
  params_json TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(params_json)),
  tools_json TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(tools_json)),
  outcome_json TEXT NOT NULL DEFAULT '' CHECK (outcome_json = '' OR json_valid(outcome_json))
) STRICT;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'script_runs', json_object('id', "id", 'script_id', "script_id", 'script_name', "script_name", 'color', "color", 'kind', "kind", 'trigger_kind', "trigger_kind", 'state', "state", 'terminal_id', "terminal_id", 'cwd', "cwd", 'command', "command", 'outcome_json', "outcome_json", 'created_at', "created_at", 'started_at', "started_at", 'finished_at', "finished_at", 'model', "model", 'session_id', "session_id", 'prompt', "prompt", 'params_json', "params_json", 'tools_json', "tools_json", 'task_id', "task_id", 'task_title', "task_title", 'branch_id', "branch_id", 'branch_label', "branch_label", 'resume_id', "resume_id", 'resumed_from', "resumed_from", 'provider', "provider", 'restore_pending', "restore_pending"), 'empty id', 'removed', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM script_runs WHERE id = '';
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'script_runs', json_object('id', "id", 'script_id', "script_id", 'script_name', "script_name", 'color', "color", 'kind', "kind", 'trigger_kind', "trigger_kind", 'state', "state", 'terminal_id', "terminal_id", 'cwd', "cwd", 'command', "command", 'outcome_json', "outcome_json", 'created_at', "created_at", 'started_at', "started_at", 'finished_at', "finished_at", 'model', "model", 'session_id', "session_id", 'prompt', "prompt", 'params_json', "params_json", 'tools_json', "tools_json", 'task_id', "task_id", 'task_title', "task_title", 'branch_id', "branch_id", 'branch_label', "branch_label", 'resume_id', "resume_id", 'resumed_from', "resumed_from", 'provider', "provider", 'restore_pending', "restore_pending"), 'invalid trigger_kind', 'removed', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM script_runs WHERE trigger_kind NOT IN ('terminal', 'manual', 'ade', 'scheduled', 'mcp');
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'script_runs', json_object('id', "id", 'script_id', "script_id", 'script_name', "script_name", 'color', "color", 'kind', "kind", 'trigger_kind', "trigger_kind", 'state', "state", 'terminal_id', "terminal_id", 'cwd', "cwd", 'command', "command", 'outcome_json', "outcome_json", 'created_at', "created_at", 'started_at', "started_at", 'finished_at', "finished_at", 'model', "model", 'session_id', "session_id", 'prompt', "prompt", 'params_json', "params_json", 'tools_json', "tools_json", 'task_id', "task_id", 'task_title', "task_title", 'branch_id', "branch_id", 'branch_label', "branch_label", 'resume_id', "resume_id", 'resumed_from', "resumed_from", 'provider', "provider", 'restore_pending', "restore_pending"), 'invalid state', 'removed', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM script_runs WHERE state NOT IN ('waiting', 'running', 'done', 'failed', 'cancelled', 'blocked', 'skipped');
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'script_runs', json_object('id', "id", 'script_id', "script_id", 'script_name', "script_name", 'color', "color", 'kind', "kind", 'trigger_kind', "trigger_kind", 'state', "state", 'terminal_id', "terminal_id", 'cwd', "cwd", 'command', "command", 'outcome_json', "outcome_json", 'created_at', "created_at", 'started_at', "started_at", 'finished_at', "finished_at", 'model', "model", 'session_id', "session_id", 'prompt', "prompt", 'params_json', "params_json", 'tools_json', "tools_json", 'task_id', "task_id", 'task_title', "task_title", 'branch_id', "branch_id", 'branch_label', "branch_label", 'resume_id', "resume_id", 'resumed_from', "resumed_from", 'provider', "provider", 'restore_pending', "restore_pending"), 'non-boolean restore_pending', 'repaired', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM script_runs WHERE "restore_pending" NOT IN (0, 1);
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'script_runs', json_object('id', "id", 'script_id', "script_id", 'script_name', "script_name", 'color', "color", 'kind', "kind", 'trigger_kind', "trigger_kind", 'state', "state", 'terminal_id', "terminal_id", 'cwd', "cwd", 'command', "command", 'outcome_json', "outcome_json", 'created_at', "created_at", 'started_at', "started_at", 'finished_at', "finished_at", 'model', "model", 'session_id', "session_id", 'prompt', "prompt", 'params_json', "params_json", 'tools_json', "tools_json", 'task_id', "task_id", 'task_title', "task_title", 'branch_id', "branch_id", 'branch_label', "branch_label", 'resume_id', "resume_id", 'resumed_from', "resumed_from", 'provider', "provider", 'restore_pending', "restore_pending"), 'negative created_at', 'repaired', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM script_runs WHERE created_at < 0;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'script_runs', json_object('id', "id", 'script_id', "script_id", 'script_name', "script_name", 'color', "color", 'kind', "kind", 'trigger_kind', "trigger_kind", 'state', "state", 'terminal_id', "terminal_id", 'cwd', "cwd", 'command', "command", 'outcome_json', "outcome_json", 'created_at', "created_at", 'started_at', "started_at", 'finished_at', "finished_at", 'model', "model", 'session_id', "session_id", 'prompt', "prompt", 'params_json', "params_json", 'tools_json', "tools_json", 'task_id', "task_id", 'task_title', "task_title", 'branch_id', "branch_id", 'branch_label', "branch_label", 'resume_id', "resume_id", 'resumed_from', "resumed_from", 'provider', "provider", 'restore_pending', "restore_pending"), 'negative started_at', 'repaired', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM script_runs WHERE started_at < 0;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'script_runs', json_object('id', "id", 'script_id', "script_id", 'script_name', "script_name", 'color', "color", 'kind', "kind", 'trigger_kind', "trigger_kind", 'state', "state", 'terminal_id', "terminal_id", 'cwd', "cwd", 'command', "command", 'outcome_json', "outcome_json", 'created_at', "created_at", 'started_at', "started_at", 'finished_at', "finished_at", 'model', "model", 'session_id', "session_id", 'prompt', "prompt", 'params_json', "params_json", 'tools_json', "tools_json", 'task_id', "task_id", 'task_title', "task_title", 'branch_id', "branch_id", 'branch_label', "branch_label", 'resume_id', "resume_id", 'resumed_from', "resumed_from", 'provider', "provider", 'restore_pending', "restore_pending"), 'negative finished_at', 'repaired', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM script_runs WHERE finished_at < 0;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'script_runs', json_object('id', "id", 'script_id', "script_id", 'script_name', "script_name", 'color', "color", 'kind', "kind", 'trigger_kind', "trigger_kind", 'state', "state", 'terminal_id', "terminal_id", 'cwd', "cwd", 'command', "command", 'outcome_json', "outcome_json", 'created_at', "created_at", 'started_at', "started_at", 'finished_at', "finished_at", 'model', "model", 'session_id', "session_id", 'prompt', "prompt", 'params_json', "params_json", 'tools_json', "tools_json", 'task_id', "task_id", 'task_title', "task_title", 'branch_id', "branch_id", 'branch_label', "branch_label", 'resume_id', "resume_id", 'resumed_from', "resumed_from", 'provider', "provider", 'restore_pending', "restore_pending"), 'invalid JSON params_json', 'repaired', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM script_runs WHERE NOT json_valid(params_json);
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'script_runs', json_object('id', "id", 'script_id', "script_id", 'script_name', "script_name", 'color', "color", 'kind', "kind", 'trigger_kind', "trigger_kind", 'state', "state", 'terminal_id', "terminal_id", 'cwd', "cwd", 'command', "command", 'outcome_json', "outcome_json", 'created_at', "created_at", 'started_at', "started_at", 'finished_at', "finished_at", 'model', "model", 'session_id', "session_id", 'prompt', "prompt", 'params_json', "params_json", 'tools_json', "tools_json", 'task_id', "task_id", 'task_title', "task_title", 'branch_id', "branch_id", 'branch_label', "branch_label", 'resume_id', "resume_id", 'resumed_from', "resumed_from", 'provider', "provider", 'restore_pending', "restore_pending"), 'invalid JSON tools_json', 'repaired', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM script_runs WHERE NOT json_valid(tools_json);
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'script_runs', json_object('id', "id", 'script_id', "script_id", 'script_name', "script_name", 'color', "color", 'kind', "kind", 'trigger_kind', "trigger_kind", 'state', "state", 'terminal_id', "terminal_id", 'cwd', "cwd", 'command', "command", 'outcome_json', "outcome_json", 'created_at', "created_at", 'started_at', "started_at", 'finished_at', "finished_at", 'model', "model", 'session_id', "session_id", 'prompt', "prompt", 'params_json', "params_json", 'tools_json', "tools_json", 'task_id', "task_id", 'task_title', "task_title", 'branch_id', "branch_id", 'branch_label', "branch_label", 'resume_id', "resume_id", 'resumed_from', "resumed_from", 'provider', "provider", 'restore_pending', "restore_pending"), 'invalid JSON outcome_json', 'repaired', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM script_runs WHERE outcome_json <> '' AND NOT json_valid(outcome_json);
INSERT INTO script_runs__n (rowid, id, trigger_kind, state, restore_pending, created_at, started_at, finished_at, script_id, script_name, color, kind, terminal_id, cwd, command, model, session_id, task_id, task_title, branch_id, branch_label, resume_id, resumed_from, provider, prompt, params_json, tools_json, outcome_json)
  SELECT rowid, id, trigger_kind, state, COALESCE("restore_pending" <> 0, 0), MAX(0, created_at), MAX(0, started_at), MAX(0, finished_at), script_id, script_name, color, kind, terminal_id, cwd, command, model, session_id, task_id, task_title, branch_id, branch_label, resume_id, resumed_from, provider, prompt, CASE WHEN json_valid(params_json) THEN params_json ELSE '[]' END, CASE WHEN json_valid(tools_json) THEN tools_json ELSE '{}' END, CASE WHEN outcome_json = '' OR json_valid(outcome_json) THEN outcome_json ELSE '' END
  FROM script_runs WHERE NOT ((id = '')
    OR (trigger_kind NOT IN ('terminal', 'manual', 'ade', 'scheduled', 'mcp'))
    OR (state NOT IN ('waiting', 'running', 'done', 'failed', 'cancelled', 'blocked', 'skipped')));
DROP TABLE script_runs;
ALTER TABLE script_runs__n RENAME TO script_runs;
CREATE INDEX script_runs_active ON script_runs (script_id) WHERE state IN ('running', 'waiting');
CREATE INDEX script_runs_created ON script_runs (created_at, id);
CREATE INDEX script_runs_task ON script_runs (task_id, created_at DESC);
CREATE INDEX script_runs_terminal ON script_runs (terminal_id) WHERE state = 'running';

-- script_run_logs
CREATE TABLE script_run_logs__n (
  run_id TEXT NOT NULL REFERENCES script_runs (id) ON DELETE CASCADE,
  seq INTEGER NOT NULL CHECK (seq >= 0),
  stream TEXT NOT NULL CHECK (stream IN ('stdout', 'stderr', 'event')),
  text TEXT NOT NULL,
  PRIMARY KEY (run_id, seq)
) STRICT;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'script_run_logs', json_object('run_id', "run_id", 'seq', "seq", 'stream', "stream", 'text', "text"), 'orphan run_id', 'removed', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM script_run_logs WHERE NOT EXISTS (SELECT 1 FROM script_runs p WHERE p.id = script_run_logs."run_id");
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'script_run_logs', json_object('run_id', "run_id", 'seq', "seq", 'stream', "stream", 'text', "text"), 'out-of-range seq', 'repaired', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM script_run_logs WHERE "seq" < 0;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'script_run_logs', json_object('run_id', "run_id", 'seq', "seq", 'stream', "stream", 'text', "text"), 'invalid stream', 'removed', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM script_run_logs WHERE stream NOT IN ('stdout', 'stderr', 'event');
INSERT INTO script_run_logs__n (rowid, run_id, seq, stream, text)
  SELECT rowid, run_id, MAX(0, COALESCE("seq", 0)), stream, text
  FROM script_run_logs WHERE NOT ((NOT EXISTS (SELECT 1 FROM script_runs p WHERE p.id = script_run_logs."run_id"))
    OR (stream NOT IN ('stdout', 'stderr', 'event')));
DROP TABLE script_run_logs;
ALTER TABLE script_run_logs__n RENAME TO script_run_logs;

-- git_host_change_requests
CREATE TABLE git_host_change_requests__n (
  repo_key TEXT NOT NULL,
  number INTEGER NOT NULL CHECK (number >= 0),
  draft INTEGER NOT NULL DEFAULT 0 CHECK (draft IN (0, 1)),
  updated_at INTEGER NOT NULL CHECK (updated_at >= 0),
  synced_at INTEGER NOT NULL CHECK (synced_at >= 0),
  comment_count INTEGER CHECK (comment_count IS NULL OR comment_count >= 0),
  node_id TEXT NOT NULL DEFAULT '',
  head_ref TEXT NOT NULL,
  head_owner TEXT NOT NULL COLLATE NOCASE,
  head_sha TEXT NOT NULL,
  base_ref TEXT NOT NULL,
  state TEXT NOT NULL,
  title TEXT NOT NULL,
  url TEXT NOT NULL,
  review_decision TEXT NOT NULL DEFAULT '',
  checks_state TEXT NOT NULL DEFAULT '',
  assignees TEXT CHECK (assignees IS NULL OR json_valid(assignees)),
  PRIMARY KEY (repo_key, number)
) STRICT;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'git_host_change_requests', json_object('repo_key', "repo_key", 'number', "number", 'node_id', "node_id", 'head_ref', "head_ref", 'head_owner', "head_owner", 'head_sha', "head_sha", 'base_ref', "base_ref", 'state', "state", 'draft', "draft", 'title', "title", 'url', "url", 'review_decision', "review_decision", 'checks_state', "checks_state", 'updated_at', "updated_at", 'synced_at', "synced_at", 'comment_count', "comment_count", 'assignees', "assignees"), 'out-of-range number', 'repaired', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM git_host_change_requests WHERE "number" < 0;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'git_host_change_requests', json_object('repo_key', "repo_key", 'number', "number", 'node_id', "node_id", 'head_ref', "head_ref", 'head_owner', "head_owner", 'head_sha', "head_sha", 'base_ref', "base_ref", 'state', "state", 'draft', "draft", 'title', "title", 'url', "url", 'review_decision', "review_decision", 'checks_state', "checks_state", 'updated_at', "updated_at", 'synced_at', "synced_at", 'comment_count', "comment_count", 'assignees', "assignees"), 'non-boolean draft', 'repaired', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM git_host_change_requests WHERE "draft" NOT IN (0, 1);
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'git_host_change_requests', json_object('repo_key', "repo_key", 'number', "number", 'node_id', "node_id", 'head_ref', "head_ref", 'head_owner', "head_owner", 'head_sha', "head_sha", 'base_ref', "base_ref", 'state', "state", 'draft', "draft", 'title', "title", 'url', "url", 'review_decision', "review_decision", 'checks_state', "checks_state", 'updated_at', "updated_at", 'synced_at', "synced_at", 'comment_count', "comment_count", 'assignees', "assignees"), 'negative updated_at', 'repaired', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM git_host_change_requests WHERE updated_at < 0;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'git_host_change_requests', json_object('repo_key', "repo_key", 'number', "number", 'node_id', "node_id", 'head_ref', "head_ref", 'head_owner', "head_owner", 'head_sha', "head_sha", 'base_ref', "base_ref", 'state', "state", 'draft', "draft", 'title', "title", 'url', "url", 'review_decision', "review_decision", 'checks_state', "checks_state", 'updated_at', "updated_at", 'synced_at', "synced_at", 'comment_count', "comment_count", 'assignees', "assignees"), 'negative synced_at', 'repaired', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM git_host_change_requests WHERE synced_at < 0;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'git_host_change_requests', json_object('repo_key', "repo_key", 'number', "number", 'node_id', "node_id", 'head_ref', "head_ref", 'head_owner', "head_owner", 'head_sha', "head_sha", 'base_ref', "base_ref", 'state', "state", 'draft', "draft", 'title', "title", 'url', "url", 'review_decision', "review_decision", 'checks_state', "checks_state", 'updated_at', "updated_at", 'synced_at', "synced_at", 'comment_count', "comment_count", 'assignees', "assignees"), 'out-of-range comment_count', 'repaired', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM git_host_change_requests WHERE "comment_count" < 0;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'git_host_change_requests', json_object('repo_key', "repo_key", 'number', "number", 'node_id', "node_id", 'head_ref', "head_ref", 'head_owner', "head_owner", 'head_sha', "head_sha", 'base_ref', "base_ref", 'state', "state", 'draft', "draft", 'title', "title", 'url', "url", 'review_decision', "review_decision", 'checks_state', "checks_state", 'updated_at', "updated_at", 'synced_at', "synced_at", 'comment_count', "comment_count", 'assignees', "assignees"), 'invalid JSON assignees', 'repaired', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM git_host_change_requests WHERE assignees IS NOT NULL AND NOT json_valid(assignees);
INSERT INTO git_host_change_requests__n (rowid, repo_key, number, draft, updated_at, synced_at, comment_count, node_id, head_ref, head_owner, head_sha, base_ref, state, title, url, review_decision, checks_state, assignees)
  SELECT rowid, repo_key, MAX(0, COALESCE("number", 0)), COALESCE("draft" <> 0, 0), MAX(0, updated_at), MAX(0, synced_at), MAX(0, "comment_count"), node_id, head_ref, head_owner, head_sha, base_ref, state, title, url, review_decision, checks_state, CASE WHEN assignees IS NULL OR json_valid(assignees) THEN assignees ELSE NULL END
  FROM git_host_change_requests;
DROP TABLE git_host_change_requests;
ALTER TABLE git_host_change_requests__n RENAME TO git_host_change_requests;
CREATE INDEX git_host_change_requests_head ON git_host_change_requests (repo_key, head_owner, head_ref, updated_at);
CREATE INDEX git_host_change_requests_open ON git_host_change_requests (repo_key, updated_at) WHERE state = 'open';

-- git_host_sync_state
CREATE TABLE git_host_sync_state__n (
  repo_key TEXT NOT NULL PRIMARY KEY CHECK (repo_key <> ''),
  backfill_complete INTEGER NOT NULL DEFAULT 0 CHECK (backfill_complete IN (0, 1)),
  schema_version INTEGER NOT NULL DEFAULT 1 CHECK (schema_version >= 0),
  watermark_updated_at INTEGER NOT NULL DEFAULT 0 CHECK (watermark_updated_at >= 0),
  last_full_sync_at INTEGER NOT NULL DEFAULT 0 CHECK (last_full_sync_at >= 0),
  last_delta_at INTEGER NOT NULL DEFAULT 0 CHECK (last_delta_at >= 0),
  last_checks_at INTEGER NOT NULL DEFAULT 0 CHECK (last_checks_at >= 0),
  updated_at INTEGER NOT NULL DEFAULT 0 CHECK (updated_at >= 0),
  delta_floor INTEGER NOT NULL DEFAULT 0 CHECK (delta_floor >= 0),
  account TEXT NOT NULL,
  etag TEXT NOT NULL DEFAULT '',
  backfill_cursor TEXT NOT NULL DEFAULT ''
) STRICT;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'git_host_sync_state', json_object('repo_key', "repo_key", 'account', "account", 'watermark_updated_at', "watermark_updated_at", 'etag', "etag", 'last_full_sync_at', "last_full_sync_at", 'last_delta_at', "last_delta_at", 'last_checks_at', "last_checks_at", 'backfill_cursor', "backfill_cursor", 'backfill_complete', "backfill_complete", 'schema_version', "schema_version", 'updated_at', "updated_at", 'delta_floor', "delta_floor"), 'empty repo_key', 'removed', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM git_host_sync_state WHERE repo_key = '';
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'git_host_sync_state', json_object('repo_key', "repo_key", 'account', "account", 'watermark_updated_at', "watermark_updated_at", 'etag', "etag", 'last_full_sync_at', "last_full_sync_at", 'last_delta_at', "last_delta_at", 'last_checks_at', "last_checks_at", 'backfill_cursor', "backfill_cursor", 'backfill_complete', "backfill_complete", 'schema_version', "schema_version", 'updated_at', "updated_at", 'delta_floor', "delta_floor"), 'non-boolean backfill_complete', 'repaired', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM git_host_sync_state WHERE "backfill_complete" NOT IN (0, 1);
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'git_host_sync_state', json_object('repo_key', "repo_key", 'account', "account", 'watermark_updated_at', "watermark_updated_at", 'etag', "etag", 'last_full_sync_at', "last_full_sync_at", 'last_delta_at', "last_delta_at", 'last_checks_at', "last_checks_at", 'backfill_cursor', "backfill_cursor", 'backfill_complete', "backfill_complete", 'schema_version', "schema_version", 'updated_at', "updated_at", 'delta_floor', "delta_floor"), 'out-of-range schema_version', 'repaired', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM git_host_sync_state WHERE "schema_version" < 0;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'git_host_sync_state', json_object('repo_key', "repo_key", 'account', "account", 'watermark_updated_at', "watermark_updated_at", 'etag', "etag", 'last_full_sync_at', "last_full_sync_at", 'last_delta_at', "last_delta_at", 'last_checks_at', "last_checks_at", 'backfill_cursor', "backfill_cursor", 'backfill_complete', "backfill_complete", 'schema_version', "schema_version", 'updated_at', "updated_at", 'delta_floor', "delta_floor"), 'negative watermark_updated_at', 'repaired', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM git_host_sync_state WHERE watermark_updated_at < 0;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'git_host_sync_state', json_object('repo_key', "repo_key", 'account', "account", 'watermark_updated_at', "watermark_updated_at", 'etag', "etag", 'last_full_sync_at', "last_full_sync_at", 'last_delta_at', "last_delta_at", 'last_checks_at', "last_checks_at", 'backfill_cursor', "backfill_cursor", 'backfill_complete', "backfill_complete", 'schema_version', "schema_version", 'updated_at', "updated_at", 'delta_floor', "delta_floor"), 'negative last_full_sync_at', 'repaired', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM git_host_sync_state WHERE last_full_sync_at < 0;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'git_host_sync_state', json_object('repo_key', "repo_key", 'account', "account", 'watermark_updated_at', "watermark_updated_at", 'etag', "etag", 'last_full_sync_at', "last_full_sync_at", 'last_delta_at', "last_delta_at", 'last_checks_at', "last_checks_at", 'backfill_cursor', "backfill_cursor", 'backfill_complete', "backfill_complete", 'schema_version', "schema_version", 'updated_at', "updated_at", 'delta_floor', "delta_floor"), 'negative last_delta_at', 'repaired', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM git_host_sync_state WHERE last_delta_at < 0;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'git_host_sync_state', json_object('repo_key', "repo_key", 'account', "account", 'watermark_updated_at', "watermark_updated_at", 'etag', "etag", 'last_full_sync_at', "last_full_sync_at", 'last_delta_at', "last_delta_at", 'last_checks_at', "last_checks_at", 'backfill_cursor', "backfill_cursor", 'backfill_complete', "backfill_complete", 'schema_version', "schema_version", 'updated_at', "updated_at", 'delta_floor', "delta_floor"), 'negative last_checks_at', 'repaired', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM git_host_sync_state WHERE last_checks_at < 0;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'git_host_sync_state', json_object('repo_key', "repo_key", 'account', "account", 'watermark_updated_at', "watermark_updated_at", 'etag', "etag", 'last_full_sync_at', "last_full_sync_at", 'last_delta_at', "last_delta_at", 'last_checks_at', "last_checks_at", 'backfill_cursor', "backfill_cursor", 'backfill_complete', "backfill_complete", 'schema_version', "schema_version", 'updated_at', "updated_at", 'delta_floor', "delta_floor"), 'negative updated_at', 'repaired', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM git_host_sync_state WHERE updated_at < 0;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'git_host_sync_state', json_object('repo_key', "repo_key", 'account', "account", 'watermark_updated_at', "watermark_updated_at", 'etag', "etag", 'last_full_sync_at', "last_full_sync_at", 'last_delta_at', "last_delta_at", 'last_checks_at', "last_checks_at", 'backfill_cursor', "backfill_cursor", 'backfill_complete', "backfill_complete", 'schema_version', "schema_version", 'updated_at', "updated_at", 'delta_floor', "delta_floor"), 'negative delta_floor', 'repaired', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM git_host_sync_state WHERE delta_floor < 0;
INSERT INTO git_host_sync_state__n (rowid, repo_key, backfill_complete, schema_version, watermark_updated_at, last_full_sync_at, last_delta_at, last_checks_at, updated_at, delta_floor, account, etag, backfill_cursor)
  SELECT rowid, repo_key, COALESCE("backfill_complete" <> 0, 0), MAX(0, COALESCE("schema_version", 1)), MAX(0, watermark_updated_at), MAX(0, last_full_sync_at), MAX(0, last_delta_at), MAX(0, last_checks_at), MAX(0, updated_at), MAX(0, delta_floor), account, etag, backfill_cursor
  FROM git_host_sync_state WHERE NOT ((repo_key = ''));
DROP TABLE git_host_sync_state;
ALTER TABLE git_host_sync_state__n RENAME TO git_host_sync_state;

-- docker_restore_engines
CREATE TABLE docker_restore_engines__n (
  engine_id TEXT NOT NULL PRIMARY KEY CHECK (engine_id <> ''),
  state TEXT NOT NULL CHECK (state IN ('tracking', 'pending')),
  boot_at INTEGER NOT NULL CHECK (boot_at >= 0),
  detected_at INTEGER CHECK (detected_at IS NULL OR detected_at >= 0),
  updated_at INTEGER NOT NULL CHECK (updated_at >= 0),
  context TEXT NOT NULL,
  host TEXT NOT NULL,
  boot_id TEXT NOT NULL
) STRICT;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'docker_restore_engines', json_object('engine_id', "engine_id", 'context', "context", 'host', "host", 'boot_id', "boot_id", 'boot_at', "boot_at", 'state', "state", 'detected_at', "detected_at", 'updated_at', "updated_at"), 'empty engine_id', 'removed', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM docker_restore_engines WHERE engine_id = '';
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'docker_restore_engines', json_object('engine_id', "engine_id", 'context', "context", 'host', "host", 'boot_id', "boot_id", 'boot_at', "boot_at", 'state', "state", 'detected_at', "detected_at", 'updated_at', "updated_at"), 'invalid state', 'removed', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM docker_restore_engines WHERE state NOT IN ('tracking', 'pending');
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'docker_restore_engines', json_object('engine_id', "engine_id", 'context', "context", 'host', "host", 'boot_id', "boot_id", 'boot_at', "boot_at", 'state', "state", 'detected_at', "detected_at", 'updated_at', "updated_at"), 'negative boot_at', 'repaired', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM docker_restore_engines WHERE boot_at < 0;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'docker_restore_engines', json_object('engine_id', "engine_id", 'context', "context", 'host', "host", 'boot_id', "boot_id", 'boot_at', "boot_at", 'state', "state", 'detected_at', "detected_at", 'updated_at', "updated_at"), 'negative detected_at', 'repaired', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM docker_restore_engines WHERE detected_at < 0;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'docker_restore_engines', json_object('engine_id', "engine_id", 'context', "context", 'host', "host", 'boot_id', "boot_id", 'boot_at', "boot_at", 'state', "state", 'detected_at', "detected_at", 'updated_at', "updated_at"), 'negative updated_at', 'repaired', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM docker_restore_engines WHERE updated_at < 0;
INSERT INTO docker_restore_engines__n (rowid, engine_id, state, boot_at, detected_at, updated_at, context, host, boot_id)
  SELECT rowid, engine_id, state, MAX(0, boot_at), MAX(0, detected_at), MAX(0, updated_at), context, host, boot_id
  FROM docker_restore_engines WHERE NOT ((engine_id = '')
    OR (state NOT IN ('tracking', 'pending')));
DROP TABLE docker_restore_engines;
ALTER TABLE docker_restore_engines__n RENAME TO docker_restore_engines;

-- docker_restore_containers
CREATE TABLE docker_restore_containers__n (
  engine_id TEXT NOT NULL REFERENCES docker_restore_engines (engine_id) ON DELETE CASCADE,
  container_id TEXT NOT NULL,
  added_at INTEGER NOT NULL CHECK (added_at >= 0),
  stopped_seen_at INTEGER CHECK (stopped_seen_at IS NULL OR stopped_seen_at >= 0),
  name TEXT NOT NULL,
  image TEXT NOT NULL,
  compose_project TEXT NOT NULL DEFAULT '',
  compose_service TEXT NOT NULL DEFAULT '',
  compose_depends_on TEXT NOT NULL DEFAULT '',
  restart_policy TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (engine_id, container_id)
) STRICT;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'docker_restore_containers', json_object('engine_id', "engine_id", 'container_id', "container_id", 'name', "name", 'image', "image", 'compose_project', "compose_project", 'compose_service', "compose_service", 'compose_depends_on', "compose_depends_on", 'restart_policy', "restart_policy", 'added_at', "added_at", 'stopped_seen_at', "stopped_seen_at"), 'orphan engine_id', 'removed', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM docker_restore_containers WHERE NOT EXISTS (SELECT 1 FROM docker_restore_engines p WHERE p.engine_id = docker_restore_containers."engine_id");
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'docker_restore_containers', json_object('engine_id', "engine_id", 'container_id', "container_id", 'name', "name", 'image', "image", 'compose_project', "compose_project", 'compose_service', "compose_service", 'compose_depends_on', "compose_depends_on", 'restart_policy', "restart_policy", 'added_at', "added_at", 'stopped_seen_at', "stopped_seen_at"), 'negative added_at', 'repaired', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM docker_restore_containers WHERE added_at < 0;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'docker_restore_containers', json_object('engine_id', "engine_id", 'container_id', "container_id", 'name', "name", 'image', "image", 'compose_project', "compose_project", 'compose_service', "compose_service", 'compose_depends_on', "compose_depends_on", 'restart_policy', "restart_policy", 'added_at', "added_at", 'stopped_seen_at', "stopped_seen_at"), 'negative stopped_seen_at', 'repaired', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM docker_restore_containers WHERE stopped_seen_at < 0;
INSERT INTO docker_restore_containers__n (rowid, engine_id, container_id, added_at, stopped_seen_at, name, image, compose_project, compose_service, compose_depends_on, restart_policy)
  SELECT rowid, engine_id, container_id, MAX(0, added_at), MAX(0, stopped_seen_at), name, image, compose_project, compose_service, compose_depends_on, restart_policy
  FROM docker_restore_containers WHERE NOT ((NOT EXISTS (SELECT 1 FROM docker_restore_engines p WHERE p.engine_id = docker_restore_containers."engine_id")));
DROP TABLE docker_restore_containers;
ALTER TABLE docker_restore_containers__n RENAME TO docker_restore_containers;

-- kube_clusters
CREATE TABLE kube_clusters__n (
  id TEXT NOT NULL PRIMARY KEY CHECK (id <> ''),
  sort_order INTEGER CHECK (sort_order IS NULL OR sort_order >= 0),
  created_at INTEGER NOT NULL CHECK (created_at >= 0),
  last_seen_at INTEGER NOT NULL CHECK (last_seen_at >= 0),
  context TEXT NOT NULL,
  server TEXT NOT NULL,
  mcp_description TEXT NOT NULL DEFAULT '',
  mcp_discover_mode TEXT NOT NULL DEFAULT 'prompt',
  mcp_list_mode TEXT NOT NULL DEFAULT 'prompt',
  mcp_describe_mode TEXT NOT NULL DEFAULT 'prompt',
  mcp_logs_mode TEXT NOT NULL DEFAULT 'prompt',
  UNIQUE (context, server)
) STRICT;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'kube_clusters', json_object('id', "id", 'context', "context", 'server', "server", 'created_at', "created_at", 'last_seen_at', "last_seen_at", 'mcp_description', "mcp_description", 'mcp_discover_mode', "mcp_discover_mode", 'mcp_list_mode', "mcp_list_mode", 'mcp_describe_mode', "mcp_describe_mode", 'mcp_logs_mode', "mcp_logs_mode", 'sort_order', "sort_order"), 'empty id', 'removed', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM kube_clusters WHERE id = '';
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'kube_clusters', json_object('id', "id", 'context', "context", 'server', "server", 'created_at', "created_at", 'last_seen_at', "last_seen_at", 'mcp_description', "mcp_description", 'mcp_discover_mode', "mcp_discover_mode", 'mcp_list_mode', "mcp_list_mode", 'mcp_describe_mode', "mcp_describe_mode", 'mcp_logs_mode', "mcp_logs_mode", 'sort_order', "sort_order"), 'out-of-range sort_order', 'repaired', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM kube_clusters WHERE "sort_order" < 0;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'kube_clusters', json_object('id', "id", 'context', "context", 'server', "server", 'created_at', "created_at", 'last_seen_at', "last_seen_at", 'mcp_description', "mcp_description", 'mcp_discover_mode', "mcp_discover_mode", 'mcp_list_mode', "mcp_list_mode", 'mcp_describe_mode', "mcp_describe_mode", 'mcp_logs_mode', "mcp_logs_mode", 'sort_order', "sort_order"), 'negative created_at', 'repaired', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM kube_clusters WHERE created_at < 0;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'kube_clusters', json_object('id', "id", 'context', "context", 'server', "server", 'created_at', "created_at", 'last_seen_at', "last_seen_at", 'mcp_description', "mcp_description", 'mcp_discover_mode', "mcp_discover_mode", 'mcp_list_mode', "mcp_list_mode", 'mcp_describe_mode', "mcp_describe_mode", 'mcp_logs_mode', "mcp_logs_mode", 'sort_order', "sort_order"), 'negative last_seen_at', 'repaired', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM kube_clusters WHERE last_seen_at < 0;
INSERT INTO kube_clusters__n (rowid, id, sort_order, created_at, last_seen_at, context, server, mcp_description, mcp_discover_mode, mcp_list_mode, mcp_describe_mode, mcp_logs_mode)
  SELECT rowid, id, MAX(0, "sort_order"), MAX(0, created_at), MAX(0, last_seen_at), context, server, mcp_description, mcp_discover_mode, mcp_list_mode, mcp_describe_mode, mcp_logs_mode
  FROM kube_clusters WHERE NOT ((id = ''));
DROP TABLE kube_clusters;
ALTER TABLE kube_clusters__n RENAME TO kube_clusters;
