-- schema_version is created and seeded by storage/migrate.go, not here (forward-only bootstrap).

CREATE TABLE settings (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL
);

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
);

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

-- History is a separate, evicting, unnamed ring — distinct from saved_queries.name's NOT NULL
-- lifecycle (D19). Capped at 20 rows per (connection_id, path) by the repo, not by SQL.
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

CREATE TABLE op_log (
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
);

CREATE TABLE ui_layout (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL
);

CREATE TABLE tabs (
  id TEXT PRIMARY KEY,
  connection_id TEXT REFERENCES connections(id) ON DELETE CASCADE,
  path TEXT NOT NULL,
  kind TEXT NOT NULL,
  state_json TEXT NOT NULL,
  "order" INTEGER NOT NULL,
  active INTEGER NOT NULL DEFAULT 0
);

-- P28 D12: the checkbox filter's set model (replaces the earlier rule-list design). No synthetic
-- id and no ordering: a set has neither.
CREATE TABLE connection_tree_filters (
  connection_id TEXT NOT NULL REFERENCES connections(id) ON DELETE CASCADE,
  scope         TEXT NOT NULL,           -- 'kind' | 'path'
  value         TEXT NOT NULL,
  PRIMARY KEY (connection_id, scope, value)
);

CREATE UNIQUE INDEX metadata_cache_connection_path ON metadata_cache(connection_id, path);
CREATE INDEX op_log_started_at ON op_log(started_at);
CREATE INDEX saved_queries_connection_path ON saved_queries(connection_id, path);
CREATE INDEX tabs_order ON tabs("order");
CREATE INDEX filter_history_target ON filter_history(connection_id, path, used_at);
