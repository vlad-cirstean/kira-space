-- kira:foreign-keys off
-- P552 code repos and ADE tasks (ade_repo_config rebuilt by Func). Rebuilds each table STRICT with checked types; rows that violate a new
-- constraint are repaired or removed and recorded in schema_quarantine.

-- A status set is never empty (AdeStatusMin); restore the seed if corruption left none, so repairs have a target.
INSERT INTO ade_task_statuses (id, name, color, position)
  SELECT 'todo', 'To do', 'grey', 0 WHERE NOT EXISTS (SELECT 1 FROM ade_task_statuses);

-- code_repos
CREATE TABLE code_repos__n (
  id TEXT NOT NULL PRIMARY KEY CHECK (id <> ''),
  name TEXT NOT NULL,
  root TEXT NOT NULL,
  repo_id TEXT NOT NULL,
  sort_order INTEGER NOT NULL CHECK (sort_order >= 0),
  color TEXT NOT NULL DEFAULT 'none',
  hidden INTEGER NOT NULL DEFAULT 0 CHECK (hidden IN (0, 1)),
  hidden_rev INTEGER NOT NULL DEFAULT 0 CHECK (hidden_rev >= 0),
  created_at INTEGER NOT NULL CHECK (created_at >= 0)
) STRICT;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'code_repos', json_object('id', "id", 'name', "name", 'root', "root", 'repo_id', "repo_id", 'sort_order', "sort_order", 'created_at', "created_at", 'color', "color", 'hidden', "hidden", 'hidden_rev', "hidden_rev"), 'empty id', 'removed', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM code_repos WHERE id = '';
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'code_repos', json_object('id', "id", 'name', "name", 'root', "root", 'repo_id', "repo_id", 'sort_order', "sort_order", 'created_at', "created_at", 'color', "color", 'hidden', "hidden", 'hidden_rev', "hidden_rev"), 'out-of-range sort_order', 'repaired', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM code_repos WHERE "sort_order" < 0;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'code_repos', json_object('id', "id", 'name', "name", 'root', "root", 'repo_id', "repo_id", 'sort_order', "sort_order", 'created_at', "created_at", 'color', "color", 'hidden', "hidden", 'hidden_rev', "hidden_rev"), 'non-boolean hidden', 'repaired', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM code_repos WHERE "hidden" NOT IN (0, 1);
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'code_repos', json_object('id', "id", 'name', "name", 'root', "root", 'repo_id', "repo_id", 'sort_order', "sort_order", 'created_at', "created_at", 'color', "color", 'hidden', "hidden", 'hidden_rev', "hidden_rev"), 'out-of-range hidden_rev', 'repaired', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM code_repos WHERE "hidden_rev" < 0;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'code_repos', json_object('id', "id", 'name', "name", 'root', "root", 'repo_id', "repo_id", 'sort_order', "sort_order", 'created_at', "created_at", 'color', "color", 'hidden', "hidden", 'hidden_rev', "hidden_rev"), 'unparsable instant created_at', 'repaired', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM code_repos WHERE unixepoch("created_at") IS NULL;
INSERT INTO code_repos__n (rowid, id, name, root, repo_id, sort_order, color, hidden, hidden_rev, created_at)
  SELECT rowid, id, name, root, repo_id, MAX(0, COALESCE("sort_order", 0)), color, COALESCE("hidden" <> 0, 0), MAX(0, COALESCE("hidden_rev", 0)), COALESCE(MAX(0, CAST(ROUND(unixepoch("created_at",'subsec')*1000) AS INTEGER)), 0)
  FROM code_repos WHERE NOT ((id = ''));
DROP TABLE code_repos;
ALTER TABLE code_repos__n RENAME TO code_repos;
CREATE UNIQUE INDEX code_repos_repo ON code_repos (repo_id);

-- ade_folders
CREATE TABLE ade_folders__n (
  path TEXT NOT NULL PRIMARY KEY CHECK (path <> ''),
  watch INTEGER NOT NULL DEFAULT 0 CHECK (watch IN (0, 1)),
  hidden INTEGER NOT NULL DEFAULT 0 CHECK (hidden IN (0, 1))
) STRICT;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_folders', json_object('path', "path", 'watch', "watch", 'hidden', "hidden"), 'empty path', 'removed', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_folders WHERE path = '';
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_folders', json_object('path', "path", 'watch', "watch", 'hidden', "hidden"), 'non-boolean watch', 'repaired', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_folders WHERE "watch" NOT IN (0, 1);
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_folders', json_object('path', "path", 'watch', "watch", 'hidden', "hidden"), 'non-boolean hidden', 'repaired', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_folders WHERE "hidden" NOT IN (0, 1);
INSERT INTO ade_folders__n (rowid, path, watch, hidden)
  SELECT rowid, path, COALESCE("watch" <> 0, 0), COALESCE("hidden" <> 0, 0)
  FROM ade_folders WHERE NOT ((path = ''));
DROP TABLE ade_folders;
ALTER TABLE ade_folders__n RENAME TO ade_folders;

-- ade_env_state
CREATE TABLE ade_env_state__n (
  code_repo_id TEXT NOT NULL REFERENCES code_repos (id) ON DELETE CASCADE,
  env TEXT NOT NULL,
  sha TEXT NOT NULL DEFAULT '',
  prev_sha TEXT NOT NULL DEFAULT '',
  error TEXT NOT NULL DEFAULT '',
  checked_at INTEGER NOT NULL CHECK (checked_at >= 0),
  PRIMARY KEY (code_repo_id, env)
) STRICT;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_env_state', json_object('code_repo_id', "code_repo_id", 'env', "env", 'sha', "sha", 'prev_sha', "prev_sha", 'error', "error", 'checked_at', "checked_at"), 'orphan code_repo_id', 'removed', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_env_state WHERE NOT EXISTS (SELECT 1 FROM code_repos p WHERE p.id = ade_env_state."code_repo_id");
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_env_state', json_object('code_repo_id', "code_repo_id", 'env', "env", 'sha', "sha", 'prev_sha', "prev_sha", 'error', "error", 'checked_at', "checked_at"), 'negative checked_at', 'repaired', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_env_state WHERE checked_at < 0;
INSERT INTO ade_env_state__n (rowid, code_repo_id, env, sha, prev_sha, error, checked_at)
  SELECT rowid, code_repo_id, env, sha, prev_sha, error, MAX(0, checked_at)
  FROM ade_env_state WHERE NOT ((NOT EXISTS (SELECT 1 FROM code_repos p WHERE p.id = ade_env_state."code_repo_id")));
DROP TABLE ade_env_state;
ALTER TABLE ade_env_state__n RENAME TO ade_env_state;

-- ade_task_statuses
CREATE TABLE ade_task_statuses__n (
  id TEXT NOT NULL PRIMARY KEY CHECK (id <> ''),
  name TEXT NOT NULL CHECK (length(name) BETWEEN 1 AND 40),
  color TEXT NOT NULL CHECK (color IN ('grey', 'brown', 'red', 'orange', 'amber', 'yellow', 'lime', 'green', 'teal', 'cyan', 'sky', 'blue', 'indigo', 'violet', 'purple', 'pink')),
  position INTEGER NOT NULL CHECK (position >= 0)
) STRICT;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_task_statuses', json_object('id', "id", 'name', "name", 'color', "color", 'position', "position"), 'empty id', 'removed', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_task_statuses WHERE id = '';
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_task_statuses', json_object('id', "id", 'name', "name", 'color', "color", 'position', "position"), 'invalid color', 'removed', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_task_statuses WHERE color NOT IN ('grey', 'brown', 'red', 'orange', 'amber', 'yellow', 'lime', 'green', 'teal', 'cyan', 'sky', 'blue', 'indigo', 'violet', 'purple', 'pink');
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_task_statuses', json_object('id', "id", 'name', "name", 'color', "color", 'position', "position"), 'out-of-range position', 'repaired', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_task_statuses WHERE "position" < 0;
INSERT INTO ade_task_statuses__n (rowid, id, name, color, position)
  SELECT rowid, id, name, color, MAX(0, COALESCE("position", 0))
  FROM ade_task_statuses WHERE NOT ((id = '')
    OR (color NOT IN ('grey', 'brown', 'red', 'orange', 'amber', 'yellow', 'lime', 'green', 'teal', 'cyan', 'sky', 'blue', 'indigo', 'violet', 'purple', 'pink')));
DROP TABLE ade_task_statuses;
ALTER TABLE ade_task_statuses__n RENAME TO ade_task_statuses;

-- ade_task_status_defaults
CREATE TABLE ade_task_status_defaults__n (
  slot TEXT NOT NULL PRIMARY KEY CHECK (slot IN ('new', 'step', 'review', 'finished')),
  status_id TEXT NOT NULL REFERENCES ade_task_statuses (id) ON DELETE NO ACTION DEFERRABLE INITIALLY DEFERRED
) STRICT;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_task_status_defaults', json_object('slot', "slot", 'status_id', "status_id"), 'unknown status_id', 'removed', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_task_status_defaults WHERE NOT EXISTS (SELECT 1 FROM ade_task_statuses s WHERE s.id = ade_task_status_defaults.status_id);
INSERT INTO ade_task_status_defaults__n (rowid, slot, status_id)
  SELECT rowid, slot, status_id
  FROM ade_task_status_defaults WHERE NOT ((NOT EXISTS (SELECT 1 FROM ade_task_statuses s WHERE s.id = ade_task_status_defaults.status_id)));
DROP TABLE ade_task_status_defaults;
ALTER TABLE ade_task_status_defaults__n RENAME TO ade_task_status_defaults;

-- ade_tasks
CREATE TABLE ade_tasks__n (
  id TEXT NOT NULL PRIMARY KEY CHECK (id <> ''),
  kind TEXT NOT NULL CHECK (kind IN ('task', 'review', 'parked')),
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
  workflow_json TEXT CHECK (workflow_json IS NULL OR json_valid(workflow_json))
) STRICT;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_tasks', json_object('id', "id", 'kind', "kind", 'title', "title", 'owner', "owner", 'jira_key', "jira_key", 'jira_url', "jira_url", 'host_url', "host_url", 'workflow_id', "workflow_id", 'entry_step_id', "entry_step_id", 'since_json', "since_json", 'est', "est", 'color', "color", 'created_at', "created_at", 'archived_at', "archived_at", 'workflow_json', "workflow_json", 'workflow_hash', "workflow_hash", 'status_id', "status_id", 'status_by', "status_by", 'status_step', "status_step"), 'empty id', 'removed', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_tasks WHERE id = '';
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_tasks', json_object('id', "id", 'kind', "kind", 'title', "title", 'owner', "owner", 'jira_key', "jira_key", 'jira_url', "jira_url", 'host_url', "host_url", 'workflow_id', "workflow_id", 'entry_step_id', "entry_step_id", 'since_json', "since_json", 'est', "est", 'color', "color", 'created_at', "created_at", 'archived_at', "archived_at", 'workflow_json', "workflow_json", 'workflow_hash', "workflow_hash", 'status_id', "status_id", 'status_by', "status_by", 'status_step', "status_step"), 'invalid kind', 'removed', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_tasks WHERE kind NOT IN ('task', 'review', 'parked');
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_tasks', json_object('id', "id", 'kind', "kind", 'title', "title", 'owner', "owner", 'jira_key', "jira_key", 'jira_url', "jira_url", 'host_url', "host_url", 'workflow_id', "workflow_id", 'entry_step_id', "entry_step_id", 'since_json', "since_json", 'est', "est", 'color', "color", 'created_at', "created_at", 'archived_at', "archived_at", 'workflow_json', "workflow_json", 'workflow_hash', "workflow_hash", 'status_id', "status_id", 'status_by', "status_by", 'status_step', "status_step"), 'unknown status_id', 'repaired', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_tasks WHERE NOT EXISTS (SELECT 1 FROM ade_task_statuses s WHERE s.id = ade_tasks.status_id);
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_tasks', json_object('id', "id", 'kind', "kind", 'title', "title", 'owner', "owner", 'jira_key', "jira_key", 'jira_url', "jira_url", 'host_url', "host_url", 'workflow_id', "workflow_id", 'entry_step_id', "entry_step_id", 'since_json', "since_json", 'est', "est", 'color', "color", 'created_at', "created_at", 'archived_at', "archived_at", 'workflow_json', "workflow_json", 'workflow_hash', "workflow_hash", 'status_id', "status_id", 'status_by', "status_by", 'status_step', "status_step"), 'invalid status_by', 'removed', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_tasks WHERE status_by NOT IN ('auto', 'user');
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_tasks', json_object('id', "id", 'kind', "kind", 'title', "title", 'owner', "owner", 'jira_key', "jira_key", 'jira_url', "jira_url", 'host_url', "host_url", 'workflow_id', "workflow_id", 'entry_step_id', "entry_step_id", 'since_json', "since_json", 'est', "est", 'color', "color", 'created_at', "created_at", 'archived_at', "archived_at", 'workflow_json', "workflow_json", 'workflow_hash', "workflow_hash", 'status_id', "status_id", 'status_by', "status_by", 'status_step', "status_step"), 'negative created_at', 'repaired', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_tasks WHERE created_at < 0;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_tasks', json_object('id', "id", 'kind', "kind", 'title', "title", 'owner', "owner", 'jira_key', "jira_key", 'jira_url', "jira_url", 'host_url', "host_url", 'workflow_id', "workflow_id", 'entry_step_id', "entry_step_id", 'since_json', "since_json", 'est', "est", 'color', "color", 'created_at', "created_at", 'archived_at', "archived_at", 'workflow_json', "workflow_json", 'workflow_hash', "workflow_hash", 'status_id', "status_id", 'status_by', "status_by", 'status_step', "status_step"), 'negative archived_at', 'repaired', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_tasks WHERE archived_at < 0;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_tasks', json_object('id', "id", 'kind', "kind", 'title', "title", 'owner', "owner", 'jira_key', "jira_key", 'jira_url', "jira_url", 'host_url', "host_url", 'workflow_id', "workflow_id", 'entry_step_id', "entry_step_id", 'since_json', "since_json", 'est', "est", 'color', "color", 'created_at', "created_at", 'archived_at', "archived_at", 'workflow_json', "workflow_json", 'workflow_hash', "workflow_hash", 'status_id', "status_id", 'status_by', "status_by", 'status_step', "status_step"), 'invalid JSON since_json', 'repaired', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_tasks WHERE since_json IS NOT NULL AND NOT json_valid(since_json);
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_tasks', json_object('id', "id", 'kind', "kind", 'title', "title", 'owner', "owner", 'jira_key', "jira_key", 'jira_url', "jira_url", 'host_url', "host_url", 'workflow_id', "workflow_id", 'entry_step_id', "entry_step_id", 'since_json', "since_json", 'est', "est", 'color', "color", 'created_at', "created_at", 'archived_at', "archived_at", 'workflow_json', "workflow_json", 'workflow_hash', "workflow_hash", 'status_id', "status_id", 'status_by', "status_by", 'status_step', "status_step"), 'invalid JSON workflow_json', 'repaired', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_tasks WHERE workflow_json IS NOT NULL AND NOT json_valid(workflow_json);
INSERT INTO ade_tasks__n (rowid, id, kind, status_id, status_by, status_step, color, created_at, archived_at, title, owner, jira_key, jira_url, host_url, workflow_id, entry_step_id, est, workflow_hash, since_json, workflow_json)
  SELECT rowid, id, kind, CASE WHEN EXISTS (SELECT 1 FROM ade_task_statuses s WHERE s.id = ade_tasks.status_id) THEN status_id ELSE COALESCE((SELECT d.status_id FROM ade_task_status_defaults d WHERE d.slot = 'new' AND EXISTS (SELECT 1 FROM ade_task_statuses s WHERE s.id = d.status_id)), (SELECT id FROM ade_task_statuses ORDER BY position, id LIMIT 1)) END, status_by, status_step, color, MAX(0, created_at), MAX(0, archived_at), title, owner, jira_key, jira_url, host_url, workflow_id, entry_step_id, est, workflow_hash, CASE WHEN since_json IS NULL OR json_valid(since_json) THEN since_json ELSE NULL END, CASE WHEN workflow_json IS NULL OR json_valid(workflow_json) THEN workflow_json ELSE NULL END
  FROM ade_tasks WHERE NOT ((id = '')
    OR (kind NOT IN ('task', 'review', 'parked'))
    OR (status_by NOT IN ('auto', 'user')));
DROP TABLE ade_tasks;
ALTER TABLE ade_tasks__n RENAME TO ade_tasks;
CREATE INDEX ade_tasks_archived ON ade_tasks (archived_at) WHERE archived_at IS NOT NULL;

-- ade_task_plan
CREATE TABLE ade_task_plan__n (
  task_id TEXT NOT NULL PRIMARY KEY REFERENCES ade_tasks (id) ON DELETE CASCADE,
  position INTEGER NOT NULL,
  backlog INTEGER NOT NULL DEFAULT 0 CHECK (backlog IN (0, 1)),
  day TEXT CHECK (day IS NULL OR date(day) = day)
) STRICT;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_task_plan', json_object('task_id', "task_id", 'day', "day", 'position', "position", 'backlog', "backlog"), 'orphan task_id', 'removed', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_task_plan WHERE NOT EXISTS (SELECT 1 FROM ade_tasks p WHERE p.id = ade_task_plan."task_id");
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_task_plan', json_object('task_id', "task_id", 'day', "day", 'position', "position", 'backlog', "backlog"), 'non-boolean backlog', 'repaired', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_task_plan WHERE "backlog" NOT IN (0, 1);
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_task_plan', json_object('task_id', "task_id", 'day', "day", 'position', "position", 'backlog', "backlog"), 'invalid day', 'repaired', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_task_plan WHERE day IS NOT NULL AND (date(day) IS NULL OR date(day) <> day);
INSERT INTO ade_task_plan__n (rowid, task_id, position, backlog, day)
  SELECT rowid, task_id, position, COALESCE("backlog" <> 0, 0), CASE WHEN day IS NULL OR date(day) = day THEN day ELSE NULL END
  FROM ade_task_plan WHERE NOT ((NOT EXISTS (SELECT 1 FROM ade_tasks p WHERE p.id = ade_task_plan."task_id")));
DROP TABLE ade_task_plan;
ALTER TABLE ade_task_plan__n RENAME TO ade_task_plan;

-- ade_task_branches
CREATE TABLE ade_task_branches__n (
  id TEXT NOT NULL PRIMARY KEY CHECK (id <> ''),
  task_id TEXT NOT NULL REFERENCES ade_tasks (id) ON DELETE CASCADE,
  code_repo_id TEXT NOT NULL REFERENCES code_repos (id) ON DELETE CASCADE,
  kind TEXT NOT NULL CHECK (kind IN ('mine', 'review', 'parked')),
  origin TEXT NOT NULL DEFAULT '' CHECK (origin IN ('', 'agent')),
  position INTEGER NOT NULL CHECK (position >= 0),
  had_commits INTEGER NOT NULL DEFAULT 0 CHECK (had_commits IN (0, 1)),
  added_at INTEGER NOT NULL CHECK (added_at >= 0),
  merged_at INTEGER CHECK (merged_at IS NULL OR merged_at >= 0),
  archived_at INTEGER CHECK (archived_at IS NULL OR archived_at >= 0),
  name TEXT NOT NULL DEFAULT '',
  base TEXT NOT NULL DEFAULT '',
  queued_after TEXT NOT NULL DEFAULT '',
  base_branch_id TEXT NOT NULL DEFAULT '',
  base_pending_from TEXT NOT NULL DEFAULT '',
  worktree_dir TEXT NOT NULL DEFAULT '',
  cr_number INTEGER CHECK (cr_number IS NULL OR cr_number >= 0),
  cr_title TEXT,
  cr_state TEXT,
  cr_url TEXT,
  review_comments TEXT CHECK (review_comments IS NULL OR json_valid(review_comments))
) STRICT;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_task_branches', json_object('id', "id", 'task_id', "task_id", 'code_repo_id', "code_repo_id", 'name', "name", 'kind', "kind", 'base', "base", 'queued_after', "queued_after", 'position', "position", 'had_commits', "had_commits", 'added_at', "added_at", 'merged_at', "merged_at", 'archived_at', "archived_at", 'origin', "origin", 'base_branch_id', "base_branch_id", 'base_pending_from', "base_pending_from", 'cr_number', "cr_number", 'cr_title', "cr_title", 'cr_state', "cr_state", 'cr_url', "cr_url", 'review_comments', "review_comments", 'worktree_dir', "worktree_dir"), 'empty id', 'removed', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_task_branches WHERE id = '';
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_task_branches', json_object('id', "id", 'task_id', "task_id", 'code_repo_id', "code_repo_id", 'name', "name", 'kind', "kind", 'base', "base", 'queued_after', "queued_after", 'position', "position", 'had_commits', "had_commits", 'added_at', "added_at", 'merged_at', "merged_at", 'archived_at', "archived_at", 'origin', "origin", 'base_branch_id', "base_branch_id", 'base_pending_from', "base_pending_from", 'cr_number', "cr_number", 'cr_title', "cr_title", 'cr_state', "cr_state", 'cr_url', "cr_url", 'review_comments', "review_comments", 'worktree_dir', "worktree_dir"), 'orphan task_id', 'removed', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_task_branches WHERE NOT EXISTS (SELECT 1 FROM ade_tasks p WHERE p.id = ade_task_branches."task_id");
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_task_branches', json_object('id', "id", 'task_id', "task_id", 'code_repo_id', "code_repo_id", 'name', "name", 'kind', "kind", 'base', "base", 'queued_after', "queued_after", 'position', "position", 'had_commits', "had_commits", 'added_at', "added_at", 'merged_at', "merged_at", 'archived_at', "archived_at", 'origin', "origin", 'base_branch_id', "base_branch_id", 'base_pending_from', "base_pending_from", 'cr_number', "cr_number", 'cr_title', "cr_title", 'cr_state', "cr_state", 'cr_url', "cr_url", 'review_comments', "review_comments", 'worktree_dir', "worktree_dir"), 'orphan code_repo_id', 'removed', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_task_branches WHERE NOT EXISTS (SELECT 1 FROM code_repos p WHERE p.id = ade_task_branches."code_repo_id");
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_task_branches', json_object('id', "id", 'task_id', "task_id", 'code_repo_id', "code_repo_id", 'name', "name", 'kind', "kind", 'base', "base", 'queued_after', "queued_after", 'position', "position", 'had_commits', "had_commits", 'added_at', "added_at", 'merged_at', "merged_at", 'archived_at', "archived_at", 'origin', "origin", 'base_branch_id', "base_branch_id", 'base_pending_from', "base_pending_from", 'cr_number', "cr_number", 'cr_title', "cr_title", 'cr_state', "cr_state", 'cr_url', "cr_url", 'review_comments', "review_comments", 'worktree_dir', "worktree_dir"), 'invalid kind', 'removed', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_task_branches WHERE kind NOT IN ('mine', 'review', 'parked');
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_task_branches', json_object('id', "id", 'task_id', "task_id", 'code_repo_id', "code_repo_id", 'name', "name", 'kind', "kind", 'base', "base", 'queued_after', "queued_after", 'position', "position", 'had_commits', "had_commits", 'added_at', "added_at", 'merged_at', "merged_at", 'archived_at', "archived_at", 'origin', "origin", 'base_branch_id', "base_branch_id", 'base_pending_from', "base_pending_from", 'cr_number', "cr_number", 'cr_title', "cr_title", 'cr_state', "cr_state", 'cr_url', "cr_url", 'review_comments', "review_comments", 'worktree_dir', "worktree_dir"), 'invalid origin', 'removed', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_task_branches WHERE origin NOT IN ('', 'agent');
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_task_branches', json_object('id', "id", 'task_id', "task_id", 'code_repo_id', "code_repo_id", 'name', "name", 'kind', "kind", 'base', "base", 'queued_after', "queued_after", 'position', "position", 'had_commits', "had_commits", 'added_at', "added_at", 'merged_at', "merged_at", 'archived_at', "archived_at", 'origin', "origin", 'base_branch_id', "base_branch_id", 'base_pending_from', "base_pending_from", 'cr_number', "cr_number", 'cr_title', "cr_title", 'cr_state', "cr_state", 'cr_url', "cr_url", 'review_comments', "review_comments", 'worktree_dir', "worktree_dir"), 'out-of-range position', 'repaired', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_task_branches WHERE "position" < 0;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_task_branches', json_object('id', "id", 'task_id', "task_id", 'code_repo_id', "code_repo_id", 'name', "name", 'kind', "kind", 'base', "base", 'queued_after', "queued_after", 'position', "position", 'had_commits', "had_commits", 'added_at', "added_at", 'merged_at', "merged_at", 'archived_at', "archived_at", 'origin', "origin", 'base_branch_id', "base_branch_id", 'base_pending_from', "base_pending_from", 'cr_number', "cr_number", 'cr_title', "cr_title", 'cr_state', "cr_state", 'cr_url', "cr_url", 'review_comments', "review_comments", 'worktree_dir', "worktree_dir"), 'non-boolean had_commits', 'repaired', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_task_branches WHERE "had_commits" NOT IN (0, 1);
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_task_branches', json_object('id', "id", 'task_id', "task_id", 'code_repo_id', "code_repo_id", 'name', "name", 'kind', "kind", 'base', "base", 'queued_after', "queued_after", 'position', "position", 'had_commits', "had_commits", 'added_at', "added_at", 'merged_at', "merged_at", 'archived_at', "archived_at", 'origin', "origin", 'base_branch_id', "base_branch_id", 'base_pending_from', "base_pending_from", 'cr_number', "cr_number", 'cr_title', "cr_title", 'cr_state', "cr_state", 'cr_url', "cr_url", 'review_comments', "review_comments", 'worktree_dir', "worktree_dir"), 'negative added_at', 'repaired', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_task_branches WHERE added_at < 0;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_task_branches', json_object('id', "id", 'task_id', "task_id", 'code_repo_id', "code_repo_id", 'name', "name", 'kind', "kind", 'base', "base", 'queued_after', "queued_after", 'position', "position", 'had_commits', "had_commits", 'added_at', "added_at", 'merged_at', "merged_at", 'archived_at', "archived_at", 'origin', "origin", 'base_branch_id', "base_branch_id", 'base_pending_from', "base_pending_from", 'cr_number', "cr_number", 'cr_title', "cr_title", 'cr_state', "cr_state", 'cr_url', "cr_url", 'review_comments', "review_comments", 'worktree_dir', "worktree_dir"), 'negative merged_at', 'repaired', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_task_branches WHERE merged_at < 0;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_task_branches', json_object('id', "id", 'task_id', "task_id", 'code_repo_id', "code_repo_id", 'name', "name", 'kind', "kind", 'base', "base", 'queued_after', "queued_after", 'position', "position", 'had_commits', "had_commits", 'added_at', "added_at", 'merged_at', "merged_at", 'archived_at', "archived_at", 'origin', "origin", 'base_branch_id', "base_branch_id", 'base_pending_from', "base_pending_from", 'cr_number', "cr_number", 'cr_title', "cr_title", 'cr_state', "cr_state", 'cr_url', "cr_url", 'review_comments', "review_comments", 'worktree_dir', "worktree_dir"), 'negative archived_at', 'repaired', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_task_branches WHERE archived_at < 0;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_task_branches', json_object('id', "id", 'task_id', "task_id", 'code_repo_id', "code_repo_id", 'name', "name", 'kind', "kind", 'base', "base", 'queued_after', "queued_after", 'position', "position", 'had_commits', "had_commits", 'added_at', "added_at", 'merged_at', "merged_at", 'archived_at', "archived_at", 'origin', "origin", 'base_branch_id', "base_branch_id", 'base_pending_from', "base_pending_from", 'cr_number', "cr_number", 'cr_title', "cr_title", 'cr_state', "cr_state", 'cr_url', "cr_url", 'review_comments', "review_comments", 'worktree_dir', "worktree_dir"), 'out-of-range cr_number', 'repaired', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_task_branches WHERE "cr_number" < 0;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_task_branches', json_object('id', "id", 'task_id', "task_id", 'code_repo_id', "code_repo_id", 'name', "name", 'kind', "kind", 'base', "base", 'queued_after', "queued_after", 'position', "position", 'had_commits', "had_commits", 'added_at', "added_at", 'merged_at', "merged_at", 'archived_at', "archived_at", 'origin', "origin", 'base_branch_id', "base_branch_id", 'base_pending_from', "base_pending_from", 'cr_number', "cr_number", 'cr_title', "cr_title", 'cr_state', "cr_state", 'cr_url', "cr_url", 'review_comments', "review_comments", 'worktree_dir', "worktree_dir"), 'invalid JSON review_comments', 'repaired', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_task_branches WHERE review_comments IS NOT NULL AND NOT json_valid(review_comments);
INSERT INTO ade_task_branches__n (rowid, id, task_id, code_repo_id, kind, origin, position, had_commits, added_at, merged_at, archived_at, name, base, queued_after, base_branch_id, base_pending_from, worktree_dir, cr_number, cr_title, cr_state, cr_url, review_comments)
  SELECT rowid, id, task_id, code_repo_id, kind, origin, MAX(0, COALESCE("position", 0)), COALESCE("had_commits" <> 0, 0), MAX(0, added_at), MAX(0, merged_at), MAX(0, archived_at), name, base, queued_after, base_branch_id, base_pending_from, worktree_dir, MAX(0, "cr_number"), cr_title, cr_state, cr_url, CASE WHEN review_comments IS NULL OR json_valid(review_comments) THEN review_comments ELSE NULL END
  FROM ade_task_branches WHERE NOT ((id = '')
    OR (NOT EXISTS (SELECT 1 FROM ade_tasks p WHERE p.id = ade_task_branches."task_id"))
    OR (NOT EXISTS (SELECT 1 FROM code_repos p WHERE p.id = ade_task_branches."code_repo_id"))
    OR (kind NOT IN ('mine', 'review', 'parked'))
    OR (origin NOT IN ('', 'agent')));
DROP TABLE ade_task_branches;
ALTER TABLE ade_task_branches__n RENAME TO ade_task_branches;
CREATE UNIQUE INDEX ade_task_branches_live_name ON ade_task_branches (code_repo_id, name) WHERE name <> '' AND archived_at IS NULL;
CREATE INDEX ade_task_branches_task ON ade_task_branches (task_id);
CREATE INDEX ade_task_branches_repo ON ade_task_branches (code_repo_id);

-- ade_branch_marks
CREATE TABLE ade_branch_marks__n (
  branch_id TEXT NOT NULL REFERENCES ade_task_branches (id) ON DELETE CASCADE,
  kind TEXT NOT NULL CHECK (kind IN ('target', 'env')),
  name TEXT NOT NULL,
  merged_tip TEXT NOT NULL,
  recorded INTEGER NOT NULL DEFAULT 0 CHECK (recorded IN (0, 1)),
  updated_at INTEGER NOT NULL CHECK (updated_at >= 0),
  PRIMARY KEY (branch_id, kind, name)
) STRICT;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_branch_marks', json_object('branch_id', "branch_id", 'kind', "kind", 'name', "name", 'merged_tip', "merged_tip", 'recorded', "recorded", 'updated_at', "updated_at"), 'orphan branch_id', 'removed', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_branch_marks WHERE NOT EXISTS (SELECT 1 FROM ade_task_branches p WHERE p.id = ade_branch_marks."branch_id");
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_branch_marks', json_object('branch_id', "branch_id", 'kind', "kind", 'name', "name", 'merged_tip', "merged_tip", 'recorded', "recorded", 'updated_at', "updated_at"), 'invalid kind', 'removed', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_branch_marks WHERE kind NOT IN ('target', 'env');
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_branch_marks', json_object('branch_id', "branch_id", 'kind', "kind", 'name', "name", 'merged_tip', "merged_tip", 'recorded', "recorded", 'updated_at', "updated_at"), 'non-boolean recorded', 'repaired', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_branch_marks WHERE "recorded" NOT IN (0, 1);
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_branch_marks', json_object('branch_id', "branch_id", 'kind', "kind", 'name', "name", 'merged_tip', "merged_tip", 'recorded', "recorded", 'updated_at', "updated_at"), 'negative updated_at', 'repaired', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_branch_marks WHERE updated_at < 0;
INSERT INTO ade_branch_marks__n (rowid, branch_id, kind, name, merged_tip, recorded, updated_at)
  SELECT rowid, branch_id, kind, name, merged_tip, COALESCE("recorded" <> 0, 0), MAX(0, updated_at)
  FROM ade_branch_marks WHERE NOT ((NOT EXISTS (SELECT 1 FROM ade_task_branches p WHERE p.id = ade_branch_marks."branch_id"))
    OR (kind NOT IN ('target', 'env')));
DROP TABLE ade_branch_marks;
ALTER TABLE ade_branch_marks__n RENAME TO ade_branch_marks;

-- ade_worktree_setup
CREATE TABLE ade_worktree_setup__n (
  branch_id TEXT NOT NULL PRIMARY KEY REFERENCES ade_task_branches (id) ON DELETE CASCADE,
  state TEXT NOT NULL CHECK (state IN ('running', 'ready', 'failed')),
  started_at INTEGER NOT NULL CHECK (started_at >= 0),
  finished_at INTEGER CHECK (finished_at IS NULL OR finished_at >= 0),
  exit_code INTEGER,
  note TEXT NOT NULL DEFAULT ''
) STRICT;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_worktree_setup', json_object('branch_id', "branch_id", 'state', "state", 'started_at', "started_at", 'finished_at', "finished_at", 'exit_code', "exit_code", 'note', "note"), 'orphan branch_id', 'removed', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_worktree_setup WHERE NOT EXISTS (SELECT 1 FROM ade_task_branches p WHERE p.id = ade_worktree_setup."branch_id");
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_worktree_setup', json_object('branch_id', "branch_id", 'state', "state", 'started_at', "started_at", 'finished_at', "finished_at", 'exit_code', "exit_code", 'note', "note"), 'invalid state', 'removed', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_worktree_setup WHERE state NOT IN ('running', 'ready', 'failed');
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_worktree_setup', json_object('branch_id', "branch_id", 'state', "state", 'started_at', "started_at", 'finished_at', "finished_at", 'exit_code', "exit_code", 'note', "note"), 'negative started_at', 'repaired', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_worktree_setup WHERE started_at < 0;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_worktree_setup', json_object('branch_id', "branch_id", 'state', "state", 'started_at', "started_at", 'finished_at', "finished_at", 'exit_code', "exit_code", 'note', "note"), 'negative finished_at', 'repaired', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_worktree_setup WHERE finished_at < 0;
INSERT INTO ade_worktree_setup__n (rowid, branch_id, state, started_at, finished_at, exit_code, note)
  SELECT rowid, branch_id, state, MAX(0, started_at), MAX(0, finished_at), exit_code, note
  FROM ade_worktree_setup WHERE NOT ((NOT EXISTS (SELECT 1 FROM ade_task_branches p WHERE p.id = ade_worktree_setup."branch_id"))
    OR (state NOT IN ('running', 'ready', 'failed')));
DROP TABLE ade_worktree_setup;
ALTER TABLE ade_worktree_setup__n RENAME TO ade_worktree_setup;

-- ade_review_windows
CREATE TABLE ade_review_windows__n (
  window_key TEXT NOT NULL PRIMARY KEY REFERENCES windows (key) ON DELETE CASCADE,
  task_id TEXT NOT NULL REFERENCES ade_tasks (id) ON DELETE CASCADE,
  branch_id TEXT NOT NULL UNIQUE REFERENCES ade_task_branches (id) ON DELETE CASCADE,
  created_at INTEGER NOT NULL CHECK (created_at >= 0)
) STRICT;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_review_windows', json_object('window_key', "window_key", 'task_id', "task_id", 'branch_id', "branch_id", 'created_at', "created_at"), 'orphan window_key', 'removed', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_review_windows WHERE NOT EXISTS (SELECT 1 FROM windows p WHERE p.key = ade_review_windows."window_key");
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_review_windows', json_object('window_key', "window_key", 'task_id', "task_id", 'branch_id', "branch_id", 'created_at', "created_at"), 'orphan task_id', 'removed', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_review_windows WHERE NOT EXISTS (SELECT 1 FROM ade_tasks p WHERE p.id = ade_review_windows."task_id");
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_review_windows', json_object('window_key', "window_key", 'task_id', "task_id", 'branch_id', "branch_id", 'created_at', "created_at"), 'orphan branch_id', 'removed', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_review_windows WHERE NOT EXISTS (SELECT 1 FROM ade_task_branches p WHERE p.id = ade_review_windows."branch_id");
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ade_review_windows', json_object('window_key', "window_key", 'task_id', "task_id", 'branch_id', "branch_id", 'created_at', "created_at"), 'negative created_at', 'repaired', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_review_windows WHERE created_at < 0;
INSERT INTO ade_review_windows__n (rowid, window_key, task_id, branch_id, created_at)
  SELECT rowid, window_key, task_id, branch_id, MAX(0, created_at)
  FROM ade_review_windows WHERE NOT ((NOT EXISTS (SELECT 1 FROM windows p WHERE p.key = ade_review_windows."window_key"))
    OR (NOT EXISTS (SELECT 1 FROM ade_tasks p WHERE p.id = ade_review_windows."task_id"))
    OR (NOT EXISTS (SELECT 1 FROM ade_task_branches p WHERE p.id = ade_review_windows."branch_id")));
DROP TABLE ade_review_windows;
ALTER TABLE ade_review_windows__n RENAME TO ade_review_windows;
CREATE INDEX ade_review_windows_task ON ade_review_windows (task_id);
