CREATE TABLE "ade_branch_marks" (
  branch_id TEXT NOT NULL REFERENCES ade_task_branches (id) ON DELETE CASCADE,
  kind TEXT NOT NULL CHECK (kind IN ('target', 'env')),
  name TEXT NOT NULL,
  merged_tip TEXT NOT NULL,
  recorded INTEGER NOT NULL DEFAULT 0 CHECK (recorded IN (0, 1)),
  updated_at INTEGER NOT NULL CHECK (updated_at >= 0),
  PRIMARY KEY (branch_id, kind, name)
) STRICT;

CREATE TABLE "ade_db_audit" (
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

CREATE TABLE "ade_env_state" (
  code_repo_id TEXT NOT NULL REFERENCES code_repos (id) ON DELETE CASCADE,
  env TEXT NOT NULL,
  sha TEXT NOT NULL DEFAULT '',
  prev_sha TEXT NOT NULL DEFAULT '',
  error TEXT NOT NULL DEFAULT '',
  checked_at INTEGER NOT NULL CHECK (checked_at >= 0),
  PRIMARY KEY (code_repo_id, env)
) STRICT;

CREATE TABLE "ade_folders" (
  path TEXT NOT NULL PRIMARY KEY CHECK (path <> ''),
  watch INTEGER NOT NULL DEFAULT 0 CHECK (watch IN (0, 1)),
  hidden INTEGER NOT NULL DEFAULT 0 CHECK (hidden IN (0, 1))
) STRICT;

CREATE TABLE "ade_log_chunks" (
  kind TEXT NOT NULL,
  id TEXT NOT NULL,
  seq INTEGER NOT NULL CHECK (seq >= 0),
  at INTEGER NOT NULL CHECK (at >= 0),
  stream TEXT NOT NULL CHECK (stream IN ('stdout', 'stderr', 'event')),
  text TEXT NOT NULL,
  PRIMARY KEY (kind, id, seq),
  FOREIGN KEY (kind, id) REFERENCES ade_logs (kind, id) ON DELETE CASCADE
) STRICT;

CREATE TABLE "ade_logs" (
  kind TEXT NOT NULL CHECK (kind IN ('run', 'setup')),
  id TEXT NOT NULL,
  task_id TEXT NOT NULL REFERENCES ade_tasks (id) ON DELETE CASCADE,
  next_seq INTEGER NOT NULL DEFAULT 1 CHECK (next_seq >= 0),
  bytes INTEGER NOT NULL DEFAULT 0 CHECK (bytes >= 0),
  truncated INTEGER NOT NULL DEFAULT 0 CHECK (truncated IN (0, 1)),
  PRIMARY KEY (kind, id)
) STRICT;

CREATE TABLE "ade_repo_config" (
  code_repo_id TEXT NOT NULL PRIMARY KEY REFERENCES code_repos (id) ON DELETE CASCADE,
  source TEXT NOT NULL DEFAULT 'added'
) STRICT;

CREATE TABLE "ade_review_windows" (
  window_key TEXT NOT NULL PRIMARY KEY REFERENCES windows (key) ON DELETE CASCADE,
  task_id TEXT NOT NULL REFERENCES ade_tasks (id) ON DELETE CASCADE,
  branch_id TEXT NOT NULL UNIQUE REFERENCES ade_task_branches (id) ON DELETE CASCADE,
  created_at INTEGER NOT NULL CHECK (created_at >= 0)
) STRICT;

CREATE TABLE "ade_reviews" (
  run_id TEXT NOT NULL PRIMARY KEY REFERENCES ade_runs (id) ON DELETE CASCADE,
  route TEXT NOT NULL CHECK (route IN ('implementer', 'new')),
  rounds INTEGER NOT NULL DEFAULT 0 CHECK (rounds >= 0),
  target_run_id TEXT NOT NULL DEFAULT '',
  dedicated_claude_id TEXT NOT NULL DEFAULT ''
) STRICT;

CREATE TABLE "ade_run_sandbox" (
  run_id TEXT NOT NULL PRIMARY KEY REFERENCES ade_runs (id) ON DELETE CASCADE,
  access TEXT NOT NULL CHECK (access IN ('write', 'read', 'off')),
  enforced INTEGER NOT NULL DEFAULT 0 CHECK (enforced IN (0, 1)),
  denial_count INTEGER NOT NULL DEFAULT 0 CHECK (denial_count >= 0),
  reason TEXT NOT NULL DEFAULT '',
  profile TEXT NOT NULL DEFAULT '',
  policy_json TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(policy_json)),
  denials_json TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(denials_json))
) STRICT;

CREATE TABLE "ade_runs" (
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

CREATE TABLE "ade_session_sandbox" (
  session_id TEXT NOT NULL PRIMARY KEY REFERENCES ade_sessions (id) ON DELETE CASCADE,
  access TEXT NOT NULL CHECK (access IN ('write', 'read')),
  enforced INTEGER NOT NULL DEFAULT 0 CHECK (enforced IN (0, 1)),
  denial_count INTEGER NOT NULL DEFAULT 0 CHECK (denial_count >= 0),
  reason TEXT NOT NULL DEFAULT '',
  profile TEXT NOT NULL DEFAULT '',
  policy_json TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(policy_json)),
  denials_json TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(denials_json))
) STRICT;

CREATE TABLE "ade_sessions" (
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

CREATE TABLE "ade_task_artifacts" (
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

CREATE TABLE "ade_task_branches" (
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

CREATE TABLE "ade_task_handovers" (
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

CREATE TABLE "ade_task_plan" (
  task_id TEXT NOT NULL PRIMARY KEY REFERENCES ade_tasks (id) ON DELETE CASCADE,
  position INTEGER NOT NULL,
  backlog INTEGER NOT NULL DEFAULT 0 CHECK (backlog IN (0, 1)),
  day TEXT CHECK (day IS NULL OR date(day) = day)
) STRICT;

CREATE TABLE "ade_task_status_defaults" (
  slot TEXT NOT NULL PRIMARY KEY CHECK (slot IN ('new', 'step', 'review', 'finished')),
  status_id TEXT NOT NULL REFERENCES ade_task_statuses (id) ON DELETE NO ACTION DEFERRABLE INITIALLY DEFERRED
) STRICT;

CREATE TABLE "ade_task_statuses" (
  id TEXT NOT NULL PRIMARY KEY CHECK (id <> ''),
  name TEXT NOT NULL CHECK (length(name) BETWEEN 1 AND 40),
  color TEXT NOT NULL CHECK (color IN ('grey', 'brown', 'red', 'orange', 'amber', 'yellow', 'lime', 'green', 'teal', 'cyan', 'sky', 'blue', 'indigo', 'violet', 'purple', 'pink')),
  position INTEGER NOT NULL CHECK (position >= 0)
) STRICT;

CREATE TABLE "ade_tasks" (
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

CREATE TABLE "ade_tracked_items" (
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

CREATE TABLE "ade_worktree_setup" (
  branch_id TEXT NOT NULL PRIMARY KEY REFERENCES ade_task_branches (id) ON DELETE CASCADE,
  state TEXT NOT NULL CHECK (state IN ('running', 'ready', 'failed')),
  started_at INTEGER NOT NULL CHECK (started_at >= 0),
  finished_at INTEGER CHECK (finished_at IS NULL OR finished_at >= 0),
  exit_code INTEGER,
  note TEXT NOT NULL DEFAULT ''
) STRICT;

CREATE TABLE "api_collections" (
  id TEXT NOT NULL PRIMARY KEY CHECK (id <> ''),
  name TEXT NOT NULL,
  sort_order INTEGER NOT NULL CHECK (sort_order >= 0),
  variables_promoted INTEGER NOT NULL DEFAULT 0 CHECK (variables_promoted IN (0, 1)),
  stem TEXT,
  created_at INTEGER NOT NULL CHECK (created_at >= 0),
  updated_at INTEGER NOT NULL CHECK (updated_at >= 0),
  origin_json TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(origin_json))
) STRICT;

CREATE TABLE "api_environments" (
  id TEXT NOT NULL PRIMARY KEY CHECK (id <> ''),
  name TEXT NOT NULL,
  sort_order INTEGER NOT NULL CHECK (sort_order >= 0),
  is_active INTEGER NOT NULL DEFAULT 0 CHECK (is_active IN (0, 1)),
  color TEXT NOT NULL DEFAULT 'none',
  stem TEXT,
  created_at INTEGER NOT NULL CHECK (created_at >= 0),
  updated_at INTEGER NOT NULL CHECK (updated_at >= 0),
  description TEXT NOT NULL DEFAULT ''
) STRICT;

CREATE TABLE api_grpc_history (
  id TEXT NOT NULL PRIMARY KEY CHECK (id <> ''),
  item_id TEXT REFERENCES api_items (id) ON DELETE CASCADE,
  tab_id TEXT NOT NULL,
  scope_key TEXT GENERATED ALWAYS AS (COALESCE(item_id, 'tab:' || tab_id)) VIRTUAL,
  called_at INTEGER NOT NULL CHECK (called_at >= 0),
  streaming TEXT NOT NULL CHECK (streaming IN ('unary', 'server')),
  code INTEGER NOT NULL CHECK (code >= 0),
  elapsed_ms INTEGER NOT NULL CHECK (elapsed_ms >= 0),
  message_count INTEGER NOT NULL CHECK (message_count >= 0),
  message_bytes INTEGER NOT NULL CHECK (message_bytes >= 0),
  stored_bytes INTEGER NOT NULL CHECK (stored_bytes >= 0),
  target TEXT NOT NULL,
  method TEXT NOT NULL,
  environment TEXT NOT NULL DEFAULT '',
  code_name TEXT NOT NULL,
  status_message TEXT NOT NULL DEFAULT '',
  snapshot_json TEXT NOT NULL
) STRICT;

CREATE TABLE "api_items" (
  id TEXT NOT NULL PRIMARY KEY CHECK (id <> ''),
  collection_id TEXT NOT NULL REFERENCES api_collections (id) ON DELETE CASCADE,
  parent_id TEXT REFERENCES api_items (id) ON DELETE CASCADE,
  kind TEXT NOT NULL CHECK (kind IN ('folder', 'request')),
  protocol TEXT NOT NULL DEFAULT 'http' CHECK (protocol IN ('http', 'grpc')),
  name TEXT NOT NULL,
  sort_order INTEGER NOT NULL CHECK (sort_order >= 0),
  method TEXT NOT NULL DEFAULT '',
  url TEXT NOT NULL DEFAULT '',
  item_key TEXT,
  created_at INTEGER NOT NULL CHECK (created_at >= 0),
  updated_at INTEGER NOT NULL CHECK (updated_at >= 0),
  request_json TEXT NOT NULL DEFAULT '' CHECK (request_json = '' OR json_valid(request_json)),
  origin_json TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(origin_json))
) STRICT;

CREATE TABLE "api_response_history" (
  id TEXT NOT NULL PRIMARY KEY CHECK (id <> ''),
  item_id TEXT REFERENCES api_items (id) ON DELETE CASCADE,
  tab_id TEXT NOT NULL,
  scope_key TEXT GENERATED ALWAYS AS (COALESCE(item_id, 'tab:' || tab_id)) VIRTUAL,
  sent_at INTEGER NOT NULL CHECK (sent_at >= 0),
  status INTEGER NOT NULL CHECK (status >= 0),
  elapsed_ms INTEGER NOT NULL CHECK (elapsed_ms >= 0),
  body_bytes INTEGER NOT NULL CHECK (body_bytes >= 0),
  stored_bytes INTEGER NOT NULL CHECK (stored_bytes >= 0),
  method TEXT NOT NULL,
  url TEXT NOT NULL,
  environment TEXT NOT NULL DEFAULT '',
  status_text TEXT NOT NULL,
  snapshot_json TEXT NOT NULL
) STRICT;

CREATE TABLE "api_variable_history" (
  id TEXT NOT NULL PRIMARY KEY CHECK (id <> ''),
  variable_id TEXT NOT NULL REFERENCES api_variables (id) ON DELETE CASCADE,
  is_secret INTEGER NOT NULL DEFAULT 0 CHECK (is_secret IN (0, 1)),
  recorded_at INTEGER NOT NULL CHECK (recorded_at >= 0),
  value TEXT NOT NULL DEFAULT '',
  secret_value TEXT
) STRICT;

CREATE TABLE "api_variables" (
  id TEXT NOT NULL PRIMARY KEY CHECK (id <> ''),
  collection_id TEXT REFERENCES api_collections (id) ON DELETE CASCADE,
  environment_id TEXT REFERENCES api_environments (id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  sort_order INTEGER NOT NULL CHECK (sort_order >= 0),
  is_secret INTEGER NOT NULL DEFAULT 0 CHECK (is_secret IN (0, 1)),
  value TEXT NOT NULL DEFAULT '',
  description TEXT NOT NULL DEFAULT '',
  secret_ref TEXT,
  created_at INTEGER NOT NULL CHECK (created_at >= 0),
  updated_at INTEGER NOT NULL CHECK (updated_at >= 0),
  secret_value TEXT,
  CHECK ((collection_id IS NULL) <> (environment_id IS NULL)),
  CHECK ((is_secret = 0 AND secret_value IS NULL) OR (is_secret = 1 AND value = ''))
) STRICT;

CREATE TABLE "code_repos" (
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

CREATE TABLE "config_last_valid" (
  folder TEXT NOT NULL,
  file TEXT NOT NULL,
  value_json TEXT NOT NULL CHECK (json_valid(value_json)),
  recorded_at INTEGER NOT NULL CHECK (recorded_at >= 0),
  PRIMARY KEY (folder, file)
) STRICT, WITHOUT ROWID;

CREATE TABLE connection_filter_history (
  id TEXT NOT NULL PRIMARY KEY CHECK (id <> ''),
  connection_id TEXT NOT NULL REFERENCES connections (id) ON DELETE CASCADE,
  path TEXT NOT NULL,
  used_at INTEGER NOT NULL CHECK (used_at >= 0),
  where_text TEXT,
  order_by_json TEXT CHECK (order_by_json IS NULL OR json_valid(order_by_json))
) STRICT;

CREATE TABLE "connection_mask_reviewed" (
  connection_id TEXT NOT NULL REFERENCES connections (id) ON DELETE CASCADE,
  column_ref TEXT NOT NULL,
  PRIMARY KEY (connection_id, column_ref)
) STRICT;

CREATE TABLE "connection_mask_rules" (
  id TEXT NOT NULL PRIMARY KEY CHECK (id <> ''),
  connection_id TEXT NOT NULL REFERENCES connections (id) ON DELETE CASCADE,
  table_name TEXT NOT NULL,
  column_name TEXT NOT NULL,
  mask_kind TEXT NOT NULL CHECK (mask_kind IN ('name', 'email', 'text', 'number', 'date', 'redact')),
  keep_hint INTEGER NOT NULL DEFAULT 1 CHECK (keep_hint IN (0, 1)),
  correlate INTEGER NOT NULL DEFAULT 1 CHECK (correlate IN (0, 1)),
  created_at INTEGER NOT NULL CHECK (created_at >= 0),
  updated_at INTEGER NOT NULL CHECK (updated_at >= 0)
) STRICT;

CREATE TABLE connection_metadata_cache (
  connection_id TEXT NOT NULL REFERENCES connections (id) ON DELETE CASCADE,
  path TEXT NOT NULL,
  kind TEXT NOT NULL,
  fetched_at INTEGER NOT NULL CHECK (fetched_at >= 0),
  etag TEXT,
  payload_json TEXT NOT NULL
) STRICT;

CREATE TABLE connection_saved_queries (
  id TEXT NOT NULL PRIMARY KEY CHECK (id <> ''),
  connection_id TEXT NOT NULL REFERENCES connections (id) ON DELETE CASCADE,
  path TEXT NOT NULL,
  name TEXT NOT NULL,
  kind TEXT NOT NULL CHECK (kind IN ('filter', 'console', 'discover')),
  pinned INTEGER NOT NULL DEFAULT 0 CHECK (pinned IN (0, 1)),
  created_at INTEGER NOT NULL CHECK (created_at >= 0),
  used_at INTEGER CHECK (used_at IS NULL OR used_at >= 0),
  body TEXT NOT NULL
) STRICT;

CREATE TABLE "connections" (
  id TEXT NOT NULL PRIMARY KEY CHECK (id <> ''),
  name TEXT NOT NULL,
  kind TEXT NOT NULL,
  mode TEXT NOT NULL,
  color TEXT NOT NULL,
  sort_order INTEGER NOT NULL DEFAULT 0 CHECK (sort_order >= 0),
  read_only INTEGER NOT NULL DEFAULT 0 CHECK (read_only IN (0, 1)),
  host TEXT,
  port INTEGER CHECK (port IS NULL OR port BETWEEN 1 AND 65535),
  database TEXT,
  username TEXT,
  stem TEXT,
  preconnect TEXT,
  preconnect_sidecar INTEGER NOT NULL DEFAULT 0 CHECK (preconnect_sidecar IN (0, 1)),
  preconnect_delay_sec INTEGER NOT NULL DEFAULT 0 CHECK (preconnect_delay_sec >= 0),
  auto_explain INTEGER NOT NULL DEFAULT 0 CHECK (auto_explain IN (0, 1)),
  throttle_per_sec REAL NOT NULL DEFAULT 0 CHECK (throttle_per_sec >= 0),
  mcp_enabled INTEGER NOT NULL DEFAULT 0 CHECK (mcp_enabled IN (0, 1)),
  mcp_auto_explain INTEGER NOT NULL DEFAULT 1 CHECK (mcp_auto_explain IN (0, 1)),
  mcp_read_mode TEXT NOT NULL DEFAULT 'allow' CHECK (mcp_read_mode IN ('deny', 'allow', 'prompt')),
  mcp_write_mode TEXT NOT NULL DEFAULT 'prompt' CHECK (mcp_write_mode IN ('deny', 'allow', 'prompt')),
  mcp_ddl_mode TEXT NOT NULL DEFAULT 'deny' CHECK (mcp_ddl_mode IN ('deny', 'allow', 'prompt')),
  mcp_masked_mode TEXT NOT NULL DEFAULT 'inherit' CHECK (mcp_masked_mode IN ('inherit', 'deny', 'allow', 'prompt')),
  mcp_review_read TEXT NOT NULL DEFAULT 'inherit' CHECK (mcp_review_read IN ('inherit', 'review', 'send')),
  mcp_review_write TEXT NOT NULL DEFAULT 'inherit' CHECK (mcp_review_write IN ('inherit', 'review', 'send')),
  mcp_review_ddl TEXT NOT NULL DEFAULT 'inherit' CHECK (mcp_review_ddl IN ('inherit', 'review', 'send')),
  mcp_review_masked TEXT NOT NULL DEFAULT 'inherit' CHECK (mcp_review_masked IN ('inherit', 'review', 'send')),
  mcp_statement_timeout_sec INTEGER NOT NULL DEFAULT 10 CHECK (mcp_statement_timeout_sec >= 0),
  mask_review_on INTEGER NOT NULL DEFAULT 0 CHECK (mask_review_on IN (0, 1)),
  mask_correlation_key TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL CHECK (created_at >= 0),
  updated_at INTEGER NOT NULL CHECK (updated_at >= 0),
  mcp_description TEXT NOT NULL DEFAULT '',
  options_json TEXT CHECK (options_json IS NULL OR json_valid(options_json)),
  mcp_tables_json TEXT CHECK (mcp_tables_json IS NULL OR json_valid(mcp_tables_json)),
  config_json TEXT NOT NULL DEFAULT '' CHECK (config_json = '' OR json_valid(config_json))
) STRICT;

CREATE TABLE "docker_restore_containers" (
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

CREATE TABLE "docker_restore_engines" (
  engine_id TEXT NOT NULL PRIMARY KEY CHECK (engine_id <> ''),
  state TEXT NOT NULL CHECK (state IN ('tracking', 'pending')),
  boot_at INTEGER NOT NULL CHECK (boot_at >= 0),
  detected_at INTEGER CHECK (detected_at IS NULL OR detected_at >= 0),
  updated_at INTEGER NOT NULL CHECK (updated_at >= 0),
  context TEXT NOT NULL,
  host TEXT NOT NULL,
  boot_id TEXT NOT NULL
) STRICT;

CREATE TABLE "file_handler_previous" (
  ext TEXT NOT NULL PRIMARY KEY CHECK (ext <> ''),
  app_path TEXT NOT NULL,
  bundle_id TEXT NOT NULL,
  saved_at INTEGER NOT NULL CHECK (saved_at >= 0)
) STRICT;

CREATE TABLE "git_host_change_requests" (
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

CREATE TABLE "git_host_sync_state" (
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

CREATE TABLE "kube_clusters" (
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

CREATE TABLE "mobile_devices" (
  id TEXT NOT NULL PRIMARY KEY CHECK (id <> ''),
  label TEXT NOT NULL,
  user_agent TEXT NOT NULL,
  token_hash BLOB NOT NULL,
  token_salt BLOB NOT NULL,
  created_at INTEGER NOT NULL CHECK (created_at >= 0),
  last_seen_at INTEGER NOT NULL CHECK (last_seen_at >= 0),
  last_ip TEXT NOT NULL,
  revoked_at INTEGER CHECK (revoked_at IS NULL OR revoked_at >= 0),
  can_write INTEGER NOT NULL DEFAULT 0 CHECK (can_write IN (0, 1)),
  can_agent_input INTEGER NOT NULL DEFAULT 0 CHECK (can_agent_input IN (0, 1)),
  expires_at INTEGER NOT NULL DEFAULT 0 CHECK (expires_at >= 0)
) STRICT;

CREATE TABLE "mobile_trusted_network" (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  subnet TEXT NOT NULL,
  router_ip TEXT NOT NULL,
  router_mac TEXT NOT NULL,
  interface TEXT NOT NULL,
  trusted_at INTEGER NOT NULL CHECK (trusted_at >= 0)
) STRICT;

CREATE TABLE "op_log" (
  id TEXT NOT NULL PRIMARY KEY CHECK (id <> ''),
  started_ms INTEGER NOT NULL CHECK (started_ms >= 0),
  duration_ms INTEGER CHECK (duration_ms IS NULL OR duration_ms >= 0),
  category TEXT NOT NULL,
  kind TEXT NOT NULL,
  status TEXT NOT NULL,
  module TEXT NOT NULL,
  trigger_ TEXT NOT NULL,
  source TEXT NOT NULL,
  subject TEXT NOT NULL,
  repo_root TEXT NOT NULL,
  repo_name TEXT NOT NULL,
  task_id TEXT NOT NULL,
  parent_id TEXT NOT NULL,
  connection_id TEXT REFERENCES connections (id) ON DELETE SET NULL,
  tab_id TEXT,
  path TEXT,
  rows INTEGER CHECK (rows IS NULL OR rows >= 0),
  steps_dropped INTEGER NOT NULL DEFAULT 0 CHECK (steps_dropped >= 0),
  background INTEGER NOT NULL DEFAULT 0 CHECK (background IN (0, 1)),
  repeat_count INTEGER NOT NULL DEFAULT 1 CHECK (repeat_count >= 1),
  command_truncated INTEGER NOT NULL DEFAULT 0 CHECK (command_truncated IN (0, 1)),
  command TEXT,
  error TEXT,
  steps_json TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(steps_json))
) STRICT;

CREATE TABLE "proof_revisions" (
  session_id TEXT NOT NULL REFERENCES proof_sessions (id) ON DELETE CASCADE,
  rev INTEGER NOT NULL CHECK (rev >= 0),
  state TEXT NOT NULL CHECK (state IN ('draft', 'current', 'pending')),
  created_at INTEGER NOT NULL CHECK (created_at >= 0),
  closed_at INTEGER CHECK (closed_at IS NULL OR closed_at >= 0),
  draft_id TEXT UNIQUE,
  reason TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (session_id, rev)
) STRICT;

CREATE TABLE "proof_sessions" (
  id TEXT NOT NULL PRIMARY KEY CHECK (id <> ''),
  state TEXT NOT NULL CHECK (state IN ('open', 'closed')),
  current_rev INTEGER CHECK (current_rev IS NULL OR current_rev >= 0),
  pending_rev INTEGER CHECK (pending_rev IS NULL OR pending_rev >= 0),
  created_at INTEGER NOT NULL CHECK (created_at >= 0),
  closed_at INTEGER CHECK (closed_at IS NULL OR closed_at >= 0),
  seen_at INTEGER CHECK (seen_at IS NULL OR seen_at >= 0),
  updated_at INTEGER NOT NULL CHECK (updated_at >= 0),
  name TEXT NOT NULL,
  name_key TEXT NOT NULL UNIQUE,
  summary TEXT NOT NULL DEFAULT ''
) STRICT;

CREATE TABLE "proof_steps" (
  session_id TEXT NOT NULL,
  rev INTEGER NOT NULL CHECK (rev >= 0),
  ordinal INTEGER NOT NULL CHECK (ordinal >= 0),
  view TEXT NOT NULL CHECK (view IN ('table', 'document', 'keyvalue', 'chart', 'diagram')),
  diff TEXT NOT NULL DEFAULT 'added' CHECK (diff IN ('added', 'unchanged', 'changed')),
  payload_bytes INTEGER NOT NULL CHECK (payload_bytes >= 0),
  origin_ordinal INTEGER CHECK (origin_ordinal IS NULL OR origin_ordinal >= 0),
  description TEXT NOT NULL,
  query TEXT NOT NULL DEFAULT '',
  language TEXT NOT NULL DEFAULT 'text',
  source TEXT NOT NULL DEFAULT '',
  payload BLOB NOT NULL,
  PRIMARY KEY (session_id, rev, ordinal),
  FOREIGN KEY (session_id, rev) REFERENCES proof_revisions (session_id, rev) ON DELETE CASCADE
) STRICT;

CREATE TABLE "proof_task_links" (
  session_id TEXT NOT NULL PRIMARY KEY REFERENCES proof_sessions (id) ON DELETE CASCADE,
  task_id TEXT NOT NULL REFERENCES ade_tasks (id) ON DELETE CASCADE,
  source TEXT NOT NULL CHECK (source IN ('agent', 'user')),
  linked_at INTEGER NOT NULL CHECK (linked_at >= 0),
  kira_session_id TEXT NOT NULL DEFAULT '',
  step_id TEXT NOT NULL DEFAULT '',
  step_name TEXT NOT NULL DEFAULT ''
) STRICT;

CREATE TABLE "proof_view_state" (
  session_id TEXT NOT NULL,
  rev INTEGER NOT NULL CHECK (rev >= 0),
  ordinal INTEGER NOT NULL CHECK (ordinal >= 0),
  updated_at INTEGER NOT NULL CHECK (updated_at >= 0),
  state TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(state)),
  PRIMARY KEY (session_id, rev, ordinal),
  FOREIGN KEY (session_id, rev, ordinal) REFERENCES proof_steps (session_id, rev, ordinal) ON DELETE CASCADE
) STRICT;

CREATE TABLE schema_version (version INTEGER NOT NULL);

CREATE TABLE script_collections (
  name TEXT NOT NULL PRIMARY KEY,
  sort_order INTEGER NOT NULL CHECK (sort_order >= 0)
) STRICT, WITHOUT ROWID;

CREATE TABLE "script_run_logs" (
  run_id TEXT NOT NULL REFERENCES script_runs (id) ON DELETE CASCADE,
  seq INTEGER NOT NULL CHECK (seq >= 0),
  stream TEXT NOT NULL CHECK (stream IN ('stdout', 'stderr', 'event')),
  text TEXT NOT NULL,
  PRIMARY KEY (run_id, seq)
) STRICT;

CREATE TABLE "script_runs" (
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

CREATE TABLE "secret_values" (
  scope TEXT NOT NULL,
  owner TEXT NOT NULL,
  name TEXT NOT NULL,
  value TEXT NOT NULL,
  updated_ms INTEGER NOT NULL CHECK (updated_ms >= 0),
  PRIMARY KEY (scope, owner, name)
) STRICT, WITHOUT ROWID;

CREATE TABLE "settings" (
  key TEXT NOT NULL PRIMARY KEY CHECK (key <> ''),
  value TEXT NOT NULL
) STRICT;

CREATE TABLE "tabs" (
  id TEXT NOT NULL PRIMARY KEY CHECK (id <> ''),
  window_key TEXT NOT NULL REFERENCES windows (key) ON DELETE CASCADE,
  connection_id TEXT REFERENCES connections (id) ON DELETE CASCADE,
  sort_order INTEGER NOT NULL CHECK (sort_order >= 0),
  active INTEGER NOT NULL DEFAULT 0 CHECK (active IN (0, 1)),
  kind TEXT NOT NULL,
  path TEXT NOT NULL,
  workspace_id TEXT,
  state_json TEXT NOT NULL CHECK (json_valid(state_json))
) STRICT;

CREATE TABLE "ui_layout" (
  key TEXT NOT NULL PRIMARY KEY CHECK (key <> ''),
  value TEXT NOT NULL
) STRICT;

CREATE TABLE "windows" (
  key TEXT NOT NULL PRIMARY KEY CHECK (key <> ''),
  sort_order INTEGER NOT NULL CHECK (sort_order >= 0),
  mode TEXT NOT NULL DEFAULT 'git',
  bounds_json TEXT CHECK (bounds_json IS NULL OR json_valid(bounds_json))
) STRICT;

CREATE INDEX ade_db_audit_call ON ade_db_audit (call_id) WHERE call_id IS NOT NULL;

CREATE INDEX ade_db_audit_task ON ade_db_audit (task_id, started_ms);

CREATE INDEX ade_logs_task ON ade_logs (task_id);

CREATE INDEX ade_review_windows_task ON ade_review_windows (task_id);

CREATE INDEX ade_runs_branch ON ade_runs (branch_id, state, purpose);

CREATE INDEX ade_runs_state ON ade_runs (state);

CREATE INDEX ade_runs_task ON ade_runs (task_id, purpose, attempt);

CREATE UNIQUE INDEX ade_sessions_review ON ade_sessions (task_id) WHERE purpose = 'review';

CREATE INDEX ade_sessions_running ON ade_sessions (task_id) WHERE state = 'running';

CREATE INDEX ade_sessions_task ON ade_sessions (task_id);

CREATE INDEX ade_task_artifacts_task ON ade_task_artifacts (task_id, created_at);

CREATE UNIQUE INDEX ade_task_artifacts_url ON ade_task_artifacts (task_id, url) WHERE url <> '';

CREATE UNIQUE INDEX ade_task_branches_live_name ON ade_task_branches (code_repo_id, name) WHERE name <> '' AND archived_at IS NULL;

CREATE INDEX ade_task_branches_repo ON ade_task_branches (code_repo_id);

CREATE INDEX ade_task_branches_task ON ade_task_branches (task_id);

CREATE UNIQUE INDEX ade_task_handovers_draft ON ade_task_handovers (task_id, from_step_id) WHERE state = 'draft';

CREATE INDEX ade_task_handovers_task ON ade_task_handovers (task_id, created_at);

CREATE INDEX ade_tasks_archived ON ade_tasks (archived_at) WHERE archived_at IS NOT NULL;

CREATE UNIQUE INDEX api_collections_stem ON api_collections (stem) WHERE stem IS NOT NULL;

CREATE UNIQUE INDEX api_environments_active ON api_environments (is_active) WHERE is_active = 1;

CREATE UNIQUE INDEX api_environments_stem ON api_environments (stem) WHERE stem IS NOT NULL;

CREATE INDEX api_grpc_history_age ON api_grpc_history (called_at);

CREATE INDEX api_grpc_history_bytes ON api_grpc_history (stored_bytes);

CREATE INDEX api_grpc_history_item ON api_grpc_history (item_id);

CREATE INDEX api_grpc_history_scope ON api_grpc_history (scope_key, called_at);

CREATE INDEX api_grpc_history_tab ON api_grpc_history (tab_id);

CREATE UNIQUE INDEX api_items_key ON api_items (collection_id, item_key) WHERE item_key IS NOT NULL;

CREATE INDEX api_items_parent ON api_items (parent_id);

CREATE INDEX api_items_tree ON api_items (collection_id, parent_id, sort_order);

CREATE INDEX api_response_history_age ON api_response_history (sent_at);

CREATE INDEX api_response_history_bytes ON api_response_history (stored_bytes);

CREATE INDEX api_response_history_item ON api_response_history (item_id);

CREATE INDEX api_response_history_scope ON api_response_history (scope_key, sent_at);

CREATE INDEX api_response_history_tab ON api_response_history (tab_id);

CREATE INDEX api_variable_history_var ON api_variable_history (variable_id, recorded_at);

CREATE INDEX api_variables_collection ON api_variables (collection_id, sort_order);

CREATE INDEX api_variables_environment ON api_variables (environment_id, sort_order);

CREATE UNIQUE INDEX code_repos_repo ON code_repos (repo_id);

CREATE INDEX connection_filter_history_target ON connection_filter_history (connection_id, path, used_at);

CREATE UNIQUE INDEX connection_mask_rules_unique ON connection_mask_rules (connection_id, lower(table_name), lower(column_name));

CREATE UNIQUE INDEX connection_metadata_cache_path ON connection_metadata_cache (connection_id, path);

CREATE UNIQUE INDEX connection_saved_queries_discover_name ON connection_saved_queries (connection_id, lower(name)) WHERE kind = 'discover';

CREATE INDEX connection_saved_queries_path ON connection_saved_queries (connection_id, path, kind, pinned DESC, used_at DESC, name);

CREATE UNIQUE INDEX connections_stem ON connections (stem) WHERE stem IS NOT NULL;

CREATE INDEX git_host_change_requests_head ON git_host_change_requests (repo_key, head_owner, head_ref, updated_at);

CREATE INDEX git_host_change_requests_open ON git_host_change_requests (repo_key, updated_at) WHERE state = 'open';

CREATE INDEX op_log_category ON op_log (category, started_ms, id);

CREATE INDEX op_log_connection ON op_log (connection_id);

CREATE INDEX op_log_started ON op_log (started_ms, id);

CREATE INDEX proof_sessions_closed ON proof_sessions (closed_at);

CREATE INDEX proof_task_links_task ON proof_task_links (task_id);

CREATE INDEX script_runs_active ON script_runs (script_id) WHERE state IN ('running', 'waiting');

CREATE INDEX script_runs_created ON script_runs (created_at, id);

CREATE INDEX script_runs_task ON script_runs (task_id, created_at DESC);

CREATE INDEX script_runs_terminal ON script_runs (terminal_id) WHERE state = 'running';

CREATE INDEX tabs_connection ON tabs (connection_id);

CREATE INDEX tabs_window ON tabs (window_key, sort_order);

