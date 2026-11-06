-- P544: Studio's schema joins the Space sequence, as it stands after Studio 0066.
-- Excluded: tables Space already owns (config_last_valid, custom_script_collections, custom_scripts,
-- script_runs, script_run_logs, secret_values, settings, ui_layout, windows, tabs).
-- Studio op_log becomes db_op_log until P544 Part 2 folds it into op_log.
-- Risk: proof_task_links.task_id cascades from ade_tasks. A later migration that rebuilds or drops
-- ade_tasks with foreign keys on deletes these rows; it must run PRAGMA foreign_keys=OFF first
-- (sqlitex runs it outside the tx), as the P543 rebuilds do.

CREATE TABLE connections (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  kind TEXT NOT NULL,
  color TEXT NOT NULL,
  mode TEXT NOT NULL,
  read_only INTEGER NOT NULL DEFAULT 0,
  host TEXT,
  port INTEGER,
  database TEXT,
  username TEXT,
  password TEXT,
  uri TEXT,
  options_json TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  sort_order INTEGER NOT NULL DEFAULT 0,
  -- NULL = no pre-connect script. Deliberately its own column rather than an options_json key:
  -- options_json round-trips through the connection URI (and the Copy URI menu item), and a shell
  -- command must never be settable by pasting a URI.
  preconnect TEXT,
  -- Explicit per-connection override of the settle-window auto-detection. 0 = "run each time it
  -- tries to connect" (default). 1 = "run once, and disconnect the db when it dies" — always
  -- arm() after connect.
  preconnect_sidecar INTEGER NOT NULL DEFAULT 0
, auto_explain INTEGER NOT NULL DEFAULT 0, throttle_per_sec REAL NOT NULL DEFAULT 0, mcp_enabled INTEGER NOT NULL DEFAULT 0, mcp_description TEXT NOT NULL DEFAULT '', mcp_read_mode TEXT NOT NULL DEFAULT 'allow', mcp_write_mode TEXT NOT NULL DEFAULT 'prompt', mcp_ddl_mode TEXT NOT NULL DEFAULT 'deny', mcp_auto_explain INTEGER NOT NULL DEFAULT 1, mask_correlation_key TEXT NOT NULL DEFAULT '', preconnect_delay_sec INTEGER NOT NULL DEFAULT 0, mcp_review_results TEXT NOT NULL DEFAULT 'inherit', stem TEXT, config_json TEXT NOT NULL DEFAULT '', mcp_statement_timeout_sec INTEGER NOT NULL DEFAULT 10, mcp_tables_json TEXT, mask_review_on INTEGER NOT NULL DEFAULT 0, mcp_review_read TEXT NOT NULL DEFAULT 'inherit', mcp_review_write TEXT NOT NULL DEFAULT 'inherit', mcp_review_ddl TEXT NOT NULL DEFAULT 'inherit', mcp_masked_mode TEXT NOT NULL DEFAULT 'inherit', mcp_review_masked TEXT NOT NULL DEFAULT 'inherit');

CREATE TABLE saved_queries (
  id TEXT PRIMARY KEY,
  connection_id TEXT NOT NULL REFERENCES connections(id) ON DELETE CASCADE,
  path TEXT NOT NULL,
  name TEXT NOT NULL,
  kind TEXT NOT NULL,
  body TEXT NOT NULL,
  created_at TEXT NOT NULL,
  used_at TEXT,
  -- Saved filters are rows with kind = 'filter' and a JSON `body` of { where, orderBy }. `pinned`
  -- sorts them above the rest of a table's saved set (D19).
  pinned INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE filter_history (
  id TEXT PRIMARY KEY,
  connection_id TEXT NOT NULL REFERENCES connections(id) ON DELETE CASCADE,
  path TEXT NOT NULL,
  where_text TEXT,
  order_by_json TEXT,
  used_at TEXT NOT NULL
);

CREATE TABLE metadata_cache (
  connection_id TEXT NOT NULL REFERENCES connections(id) ON DELETE CASCADE,
  path TEXT NOT NULL,
  kind TEXT NOT NULL,
  payload_json TEXT NOT NULL,
  fetched_at TEXT NOT NULL,
  etag TEXT
);

CREATE TABLE db_op_log (
  id TEXT PRIMARY KEY,
  connection_id TEXT REFERENCES connections(id) ON DELETE SET NULL,
  tab_id TEXT,
  started_at TEXT NOT NULL,
  duration_ms INTEGER,
  kind TEXT NOT NULL,
  status TEXT NOT NULL,
  rows INTEGER,
  command TEXT,
  error TEXT
, stored_bytes INTEGER NOT NULL DEFAULT 0, command_truncated INTEGER NOT NULL DEFAULT 0, path TEXT, source TEXT NOT NULL DEFAULT 'user');

CREATE TABLE connection_tree_filters (
  connection_id TEXT NOT NULL REFERENCES connections(id) ON DELETE CASCADE,
  scope         TEXT NOT NULL,           -- 'kind' | 'path'
  value         TEXT NOT NULL,
  PRIMARY KEY (connection_id, scope, value)
);

CREATE TABLE connection_ddl (
  connection_id TEXT PRIMARY KEY REFERENCES connections(id) ON DELETE CASCADE,
  ddl           TEXT NOT NULL,
  updated_at    TEXT NOT NULL
);

CREATE TABLE "api_collections" (
  id          TEXT PRIMARY KEY,
  name        TEXT NOT NULL,
  sort_order  INTEGER NOT NULL,
  -- The whole original collection object, minus its `item` array (those are rows below) and minus
  -- `info.name` (the column above). '{}' for a collection created in this app.
  origin_json TEXT NOT NULL DEFAULT '{}',
  created_at  TEXT NOT NULL,
  updated_at  TEXT NOT NULL
, variables_promoted INTEGER NOT NULL DEFAULT 0, stem TEXT);

CREATE TABLE "api_items" (
  id            TEXT PRIMARY KEY,
  collection_id TEXT NOT NULL REFERENCES "api_collections"(id) ON DELETE CASCADE,
  -- NULL = a direct child of the collection root. The self-reference cascades for real: db.go's
  -- DSN sets _foreign_keys=1 on every connection (F9), so deleting a folder deletes its subtree at
  -- any depth in one statement.
  parent_id     TEXT REFERENCES "api_items"(id) ON DELETE CASCADE,
  kind          TEXT NOT NULL,               -- 'folder' | 'request'
  name          TEXT NOT NULL,
  sort_order    INTEGER NOT NULL,            -- dense index within this parent's own item[] array
  -- Denormalized out of request_json so the tree renders a method chip and searches URLs without
  -- reading (potentially large) request bodies. '' for a folder. repos/collections.go is the only
  -- writer of either and always writes both together.
  method        TEXT NOT NULL DEFAULT '',
  url           TEXT NOT NULL DEFAULT '',
  -- kind='request': model.SavedRequest (D4) — the request half of the renderer's own
  -- httpRequestTabStateSchema, and the only thing this app actually edits. '' for a folder.
  request_json  TEXT NOT NULL DEFAULT '',
  -- The original Postman item object verbatim, minus its own `item` array (D5). '{}' for an item
  -- created in this app, and individually shed member-by-member as the user edits (D6).
  origin_json   TEXT NOT NULL DEFAULT '{}',
  created_at    TEXT NOT NULL,
  updated_at    TEXT NOT NULL
, protocol TEXT NOT NULL DEFAULT 'http', item_key TEXT);

CREATE TABLE "api_environments" (
  id         TEXT PRIMARY KEY,
  name       TEXT NOT NULL,
  sort_order INTEGER NOT NULL,
  -- D3: the app-global selection. The repo keeps at most one row set, in one transaction.
  is_active  INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
, description TEXT NOT NULL DEFAULT '', color TEXT NOT NULL DEFAULT 'none', stem TEXT);

CREATE TABLE "api_variables" (
  id             TEXT PRIMARY KEY,
  -- Exactly one owner. Both cascade for real: db.go's DSN sets _foreign_keys=1 on every connection
  -- the pool opens (P4 F9), so deleting a collection or an environment deletes its variables.
  collection_id  TEXT REFERENCES "api_collections"(id) ON DELETE CASCADE,
  environment_id TEXT REFERENCES "api_environments"(id) ON DELETE CASCADE,
  name           TEXT NOT NULL,
  -- D5: a non-secret's plaintext. '' whenever is_secret = 1 — a plaintext value and a secret value
  -- never share a column, so the list projection below can be trusted by construction rather than
  -- by a per-row branch in Go.
  value          TEXT NOT NULL DEFAULT '',
  is_secret      INTEGER NOT NULL DEFAULT 0,
  -- D5: the internal/secrets kira:v2: AES-256-GCM envelope, the same one connections.password
  -- carries. NULL whenever is_secret = 0. NO query that feeds the renderer's list ever selects it.
  secret_value   TEXT,
  sort_order     INTEGER NOT NULL,
  created_at     TEXT NOT NULL,
  updated_at     TEXT NOT NULL, description TEXT NOT NULL DEFAULT '', secret_ref TEXT,
  CHECK ((collection_id IS NULL) <> (environment_id IS NULL)),
  CHECK (is_secret IN (0, 1)),
  CHECK ((is_secret = 0 AND secret_value IS NULL) OR (is_secret = 1 AND value = ''))
);

CREATE TABLE "api_variable_history" (
  id           TEXT PRIMARY KEY,
  variable_id  TEXT NOT NULL REFERENCES "api_variables"(id) ON DELETE CASCADE,
  value        TEXT NOT NULL DEFAULT '',
  is_secret    INTEGER NOT NULL DEFAULT 0,
  secret_value TEXT,
  recorded_at  TEXT NOT NULL
);

CREATE TABLE "api_response_history" (
  id            TEXT PRIMARY KEY,
  -- The saved request this belongs to, or NULL for a scratch tab's own history. Cascades for real:
  -- db.go's DSN sets _foreign_keys=1 on every connection (P4 F9), so deleting a request -- or the
  -- folder or collection above it -- takes its history with it in one statement.
  item_id       TEXT REFERENCES "api_items"(id) ON DELETE CASCADE,
  -- The tab that sent it. Deliberately NOT a foreign key into `tabs` (F4): TabsRepo.Save deletes
  -- and re-inserts a window's whole tab set on a 1 s debounce that fires on every keystroke in the
  -- URL field, so ON DELETE CASCADE here would erase a scratch tab's history a second after the
  -- user typed. SweepOrphans() at startup is the bound instead (D7).
  tab_id        TEXT NOT NULL,
  -- The one axis List/trim/Clear key on: the saved request when there is one, else the tab.
  -- GENERATED rather than written, so it cannot disagree with its two sources and so Adopt (D14)
  -- is a single UPDATE of item_id. Verified indexable, and used by the planner, on
  -- modernc.org/sqlite 3.53.3 (F8).
  scope_key     TEXT GENERATED ALWAYS AS (COALESCE(item_id, 'tab:' || tab_id)) VIRTUAL,
  sent_at       TEXT NOT NULL,
  -- Denormalized out of snapshot_json so the list renders without reading a single body (the same
  -- reason http_items carries method/url). `environment` is the environment's NAME at send time,
  -- not its id: a frozen name still reads correctly after the environment is deleted, which is
  -- exactly when the user wants to know which one it was. '' when none was active.
  method        TEXT NOT NULL,
  url           TEXT NOT NULL,
  environment   TEXT NOT NULL DEFAULT '',
  status        INTEGER NOT NULL,
  status_text   TEXT NOT NULL,
  elapsed_ms    INTEGER NOT NULL,
  -- What the server sent (httpclient.Response.BodyBytes) vs. what this row actually costs. The
  -- second is the byte budget's unit (D6) and is len(snapshot_json), not a body length.
  body_bytes    INTEGER NOT NULL,
  stored_bytes  INTEGER NOT NULL,
  -- model.ResponseHistorySnapshot: the stage-1 request, the full response, and the two storage
  -- flags. Last column, and List never selects it -- SQLite stores columns in declaration order
  -- and spills the tail into overflow pages (P4 D2's own projection reasoning).
  snapshot_json TEXT NOT NULL
);

CREATE TABLE grpc_call_history (
  id            TEXT PRIMARY KEY,
  item_id       TEXT REFERENCES "api_items"(id) ON DELETE CASCADE,
  -- Deliberately NOT a foreign key into `tabs` (P8 F4's own reasoning, applied verbatim) --
  -- TabsRepo.Save deletes and re-inserts a window's whole tab set on a 1 s debounce, so
  -- ON DELETE CASCADE here would erase a scratch tab's history a second after the user typed.
  -- SweepOrphans() at startup is the bound instead.
  tab_id        TEXT NOT NULL,
  scope_key     TEXT GENERATED ALWAYS AS (COALESCE(item_id, 'tab:' || tab_id)) VIRTUAL,
  called_at     TEXT NOT NULL,
  -- STAGE 1: a secret is still spelled {{name}} -- recorded from the bridge's own unresolved args,
  -- never from the resolver's output (P8 D2's rule verbatim).
  target        TEXT NOT NULL,
  method        TEXT NOT NULL,             -- fully-qualified: pkg.Service/Method
  streaming     TEXT NOT NULL,             -- 'unary' | 'server'
  environment   TEXT NOT NULL DEFAULT '',  -- the environment's NAME at call time, not its id
  code          INTEGER NOT NULL,          -- codes.Code: 0 = OK
  code_name     TEXT NOT NULL,             -- 'OK', 'PermissionDenied', …
  status_message TEXT NOT NULL DEFAULT '',
  elapsed_ms    INTEGER NOT NULL,
  message_count INTEGER NOT NULL,          -- messages actually received
  message_bytes INTEGER NOT NULL,          -- total wire bytes received
  stored_bytes  INTEGER NOT NULL,
  -- model.GrpcCallSnapshot: the stage-1 request, the messages actually kept (D11's own 100-message
  -- cap), header/trailer, and the storage-cap flags. Last column, so SQLite spills the tail into
  -- overflow pages (P4 D2's own projection reasoning).
  snapshot_json TEXT NOT NULL
);

CREATE TABLE connection_mask_rules (
  id            TEXT PRIMARY KEY,
  connection_id TEXT NOT NULL REFERENCES connections(id) ON DELETE CASCADE,
  -- '*' matches any table. A concrete name is kept even though matching is by column name alone
  -- (plan §4.2): it is what makes a rule legible in the UI and what the grid's header menu writes.
  table_name    TEXT NOT NULL,
  column_name   TEXT NOT NULL,
  -- name | email | text | number | date | redact
  mask_kind     TEXT NOT NULL,
  -- Keep this kind's own small hint: the initials for `name`, the domain for `email`, the year for
  -- `date`. Meaningless for text/number/redact, stored anyway so the column stays one flag rather
  -- than three kind-specific ones.
  keep_hint     INTEGER NOT NULL DEFAULT 1,
  -- Emit the keyed correlation tag (plan §2.4). Forced 0 for `number` (a bucket is many-to-one,
  -- so tagging it would be dishonest — internal/mask.Apply enforces this regardless of this flag).
  correlate     INTEGER NOT NULL DEFAULT 1,
  created_at    TEXT NOT NULL,
  updated_at    TEXT NOT NULL
);

CREATE TABLE docker_restore_engines (
  engine_id   TEXT PRIMARY KEY,
  context     TEXT NOT NULL,
  host        TEXT NOT NULL,
  boot_id     TEXT NOT NULL,
  boot_at     INTEGER NOT NULL,
  state       TEXT NOT NULL CHECK (state IN ('tracking', 'pending')),
  detected_at INTEGER,
  updated_at  INTEGER NOT NULL
);

CREATE TABLE docker_restore_containers (
  engine_id          TEXT NOT NULL REFERENCES docker_restore_engines(engine_id) ON DELETE CASCADE,
  container_id       TEXT NOT NULL,
  name               TEXT NOT NULL,
  image              TEXT NOT NULL,
  compose_project    TEXT NOT NULL DEFAULT '',
  compose_service    TEXT NOT NULL DEFAULT '',
  compose_depends_on TEXT NOT NULL DEFAULT '',
  restart_policy     TEXT NOT NULL DEFAULT '',
  added_at           INTEGER NOT NULL,
  stopped_seen_at    INTEGER,
  PRIMARY KEY (engine_id, container_id)
);

CREATE TABLE proof_sessions (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  name_key TEXT NOT NULL UNIQUE,
  state TEXT NOT NULL CHECK (state IN ('open','closed')),
  current_rev INTEGER,
  pending_rev INTEGER,
  summary TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  closed_at INTEGER,
  seen_at INTEGER,
  updated_at INTEGER NOT NULL
);

CREATE TABLE proof_revisions (
  session_id TEXT NOT NULL REFERENCES proof_sessions(id) ON DELETE CASCADE,
  rev INTEGER NOT NULL,
  draft_id TEXT UNIQUE,
  reason TEXT NOT NULL DEFAULT '',
  state TEXT NOT NULL CHECK (state IN ('draft','current','pending')),
  created_at INTEGER NOT NULL,
  closed_at INTEGER,
  PRIMARY KEY (session_id, rev)
);

CREATE TABLE kube_clusters (
  id TEXT PRIMARY KEY,
  context TEXT NOT NULL,
  server TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  last_seen_at INTEGER NOT NULL, mcp_description TEXT NOT NULL DEFAULT '', mcp_discover_mode TEXT NOT NULL DEFAULT 'prompt', mcp_list_mode TEXT NOT NULL DEFAULT 'prompt', mcp_describe_mode TEXT NOT NULL DEFAULT 'prompt', mcp_logs_mode TEXT NOT NULL DEFAULT 'prompt', sort_order INTEGER,
  UNIQUE (context, server)
);

CREATE TABLE connection_mask_reviewed (
  connection_id TEXT NOT NULL REFERENCES connections(id) ON DELETE CASCADE,
  column_ref    TEXT NOT NULL,
  PRIMARY KEY (connection_id, column_ref)
);

CREATE TABLE "proof_steps" (
  session_id TEXT NOT NULL,
  rev INTEGER NOT NULL,
  ordinal INTEGER NOT NULL,
  description TEXT NOT NULL,
  query TEXT NOT NULL DEFAULT '',
  language TEXT NOT NULL DEFAULT 'text',
  source TEXT NOT NULL DEFAULT '',
  view TEXT NOT NULL CHECK (view IN ('table','document','keyvalue','chart','diagram')),
  payload BLOB NOT NULL,
  payload_bytes INTEGER NOT NULL,
  diff TEXT NOT NULL DEFAULT 'added',
  origin_ordinal INTEGER,
  PRIMARY KEY (session_id, rev, ordinal),
  FOREIGN KEY (session_id, rev) REFERENCES proof_revisions(session_id, rev) ON DELETE CASCADE
);

CREATE TABLE "proof_view_state" (
  session_id TEXT NOT NULL,
  rev INTEGER NOT NULL,
  ordinal INTEGER NOT NULL,
  state TEXT NOT NULL,
  updated_at INTEGER NOT NULL,
  PRIMARY KEY (session_id, rev, ordinal),
  FOREIGN KEY (session_id, rev, ordinal) REFERENCES "proof_steps"(session_id, rev, ordinal) ON DELETE CASCADE
);

CREATE TABLE proof_task_links (
  session_id TEXT PRIMARY KEY REFERENCES proof_sessions(id) ON DELETE CASCADE,
  task_id TEXT NOT NULL REFERENCES ade_tasks(id) ON DELETE CASCADE,
  source TEXT NOT NULL CHECK (source IN ('agent', 'user')),
  kira_session_id TEXT NOT NULL DEFAULT '',
  step_id TEXT NOT NULL DEFAULT '',
  step_name TEXT NOT NULL DEFAULT '',
  linked_at INTEGER NOT NULL
);

CREATE UNIQUE INDEX metadata_cache_connection_path ON metadata_cache(connection_id, path);

CREATE INDEX db_op_log_started_at ON db_op_log(started_at);

CREATE INDEX saved_queries_connection_path ON saved_queries(connection_id, path);

CREATE INDEX filter_history_target ON filter_history(connection_id, path, used_at);

CREATE INDEX grpc_call_history_scope ON grpc_call_history(scope_key, called_at);

CREATE INDEX grpc_call_history_age   ON grpc_call_history(called_at);

CREATE INDEX grpc_call_history_tab   ON grpc_call_history(tab_id);

CREATE INDEX api_items_tree ON api_items(collection_id, parent_id, sort_order);

CREATE INDEX api_variables_collection ON api_variables(collection_id, sort_order);

CREATE INDEX api_variables_environment ON api_variables(environment_id, sort_order);

CREATE INDEX api_variable_history_var ON api_variable_history(variable_id, recorded_at);

CREATE INDEX api_response_history_scope ON api_response_history(scope_key, sent_at);

CREATE INDEX api_response_history_age ON api_response_history(sent_at);

CREATE INDEX api_response_history_tab ON api_response_history(tab_id);

CREATE INDEX api_response_history_bytes ON api_response_history(stored_bytes);

CREATE INDEX grpc_call_history_bytes ON grpc_call_history(stored_bytes);

CREATE INDEX db_op_log_bytes ON db_op_log(stored_bytes);

CREATE UNIQUE INDEX connection_mask_rules_unique
  ON connection_mask_rules(connection_id, lower(table_name), lower(column_name));

CREATE INDEX connection_mask_rules_conn ON connection_mask_rules(connection_id);

CREATE INDEX api_items_parent ON api_items(parent_id);

CREATE INDEX api_response_history_item ON api_response_history(item_id);

CREATE INDEX grpc_call_history_item ON grpc_call_history(item_id);

CREATE INDEX db_op_log_connection ON db_op_log(connection_id);

CREATE INDEX proof_sessions_closed ON proof_sessions(closed_at);

CREATE UNIQUE INDEX saved_queries_discover_name
  ON saved_queries(connection_id, lower(name)) WHERE kind = 'discover';

CREATE UNIQUE INDEX api_collections_stem ON api_collections(stem) WHERE stem IS NOT NULL;

CREATE UNIQUE INDEX api_environments_stem ON api_environments(stem) WHERE stem IS NOT NULL;

CREATE UNIQUE INDEX api_items_key ON api_items(collection_id, item_key) WHERE item_key IS NOT NULL;

CREATE UNIQUE INDEX connections_stem ON connections(stem) WHERE stem IS NOT NULL;

CREATE INDEX proof_task_links_task ON proof_task_links(task_id);
