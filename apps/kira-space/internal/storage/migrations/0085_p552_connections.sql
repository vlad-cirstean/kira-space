-- kira:foreign-keys off
-- P552 connection cluster (connections rebuilt by Func). Rebuilds each table STRICT with checked types; rows that violate a new
-- constraint are repaired or removed and recorded in schema_quarantine.

-- connection_mask_rules
CREATE TABLE connection_mask_rules__n (
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
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'connection_mask_rules', json_object('id', "id", 'connection_id', "connection_id", 'table_name', "table_name", 'column_name', "column_name", 'mask_kind', "mask_kind", 'keep_hint', "keep_hint", 'correlate', "correlate", 'created_at', "created_at", 'updated_at', "updated_at"), 'empty id', 'removed', 85, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM connection_mask_rules WHERE id = '';
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'connection_mask_rules', json_object('id', "id", 'connection_id', "connection_id", 'table_name', "table_name", 'column_name', "column_name", 'mask_kind', "mask_kind", 'keep_hint', "keep_hint", 'correlate', "correlate", 'created_at', "created_at", 'updated_at', "updated_at"), 'orphan connection_id', 'removed', 85, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM connection_mask_rules WHERE NOT EXISTS (SELECT 1 FROM connections p WHERE p.id = connection_mask_rules."connection_id");
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'connection_mask_rules', json_object('id', "id", 'connection_id', "connection_id", 'table_name', "table_name", 'column_name', "column_name", 'mask_kind', "mask_kind", 'keep_hint', "keep_hint", 'correlate', "correlate", 'created_at', "created_at", 'updated_at', "updated_at"), 'invalid mask_kind', 'removed', 85, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM connection_mask_rules WHERE mask_kind NOT IN ('name', 'email', 'text', 'number', 'date', 'redact');
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'connection_mask_rules', json_object('id', "id", 'connection_id', "connection_id", 'table_name', "table_name", 'column_name', "column_name", 'mask_kind', "mask_kind", 'keep_hint', "keep_hint", 'correlate', "correlate", 'created_at', "created_at", 'updated_at', "updated_at"), 'non-boolean keep_hint', 'repaired', 85, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM connection_mask_rules WHERE "keep_hint" NOT IN (0, 1);
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'connection_mask_rules', json_object('id', "id", 'connection_id', "connection_id", 'table_name', "table_name", 'column_name', "column_name", 'mask_kind', "mask_kind", 'keep_hint', "keep_hint", 'correlate', "correlate", 'created_at', "created_at", 'updated_at', "updated_at"), 'non-boolean correlate', 'repaired', 85, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM connection_mask_rules WHERE "correlate" NOT IN (0, 1);
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'connection_mask_rules', json_object('id', "id", 'connection_id', "connection_id", 'table_name', "table_name", 'column_name', "column_name", 'mask_kind', "mask_kind", 'keep_hint', "keep_hint", 'correlate', "correlate", 'created_at', "created_at", 'updated_at', "updated_at"), 'unparsable instant created_at', 'repaired', 85, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM connection_mask_rules WHERE unixepoch("created_at") IS NULL;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'connection_mask_rules', json_object('id', "id", 'connection_id', "connection_id", 'table_name', "table_name", 'column_name', "column_name", 'mask_kind', "mask_kind", 'keep_hint', "keep_hint", 'correlate', "correlate", 'created_at', "created_at", 'updated_at', "updated_at"), 'unparsable instant updated_at', 'repaired', 85, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM connection_mask_rules WHERE unixepoch("updated_at") IS NULL;
INSERT INTO connection_mask_rules__n (rowid, id, connection_id, table_name, column_name, mask_kind, keep_hint, correlate, created_at, updated_at)
  SELECT rowid, id, connection_id, table_name, column_name, mask_kind, COALESCE("keep_hint" <> 0, 1), COALESCE("correlate" <> 0, 1), COALESCE(MAX(0, CAST(ROUND(unixepoch("created_at",'subsec')*1000) AS INTEGER)), 0), COALESCE(MAX(0, CAST(ROUND(unixepoch("updated_at",'subsec')*1000) AS INTEGER)), 0)
  FROM connection_mask_rules WHERE NOT ((id = '')
    OR (NOT EXISTS (SELECT 1 FROM connections p WHERE p.id = connection_mask_rules."connection_id"))
    OR (mask_kind NOT IN ('name', 'email', 'text', 'number', 'date', 'redact')));
DROP TABLE connection_mask_rules;
ALTER TABLE connection_mask_rules__n RENAME TO connection_mask_rules;
CREATE UNIQUE INDEX connection_mask_rules_unique ON connection_mask_rules (connection_id, lower(table_name), lower(column_name));

-- connection_mask_reviewed
CREATE TABLE connection_mask_reviewed__n (
  connection_id TEXT NOT NULL REFERENCES connections (id) ON DELETE CASCADE,
  column_ref TEXT NOT NULL,
  PRIMARY KEY (connection_id, column_ref)
) STRICT;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'connection_mask_reviewed', json_object('connection_id', "connection_id", 'column_ref', "column_ref"), 'orphan connection_id', 'removed', 85, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM connection_mask_reviewed WHERE NOT EXISTS (SELECT 1 FROM connections p WHERE p.id = connection_mask_reviewed."connection_id");
INSERT INTO connection_mask_reviewed__n (rowid, connection_id, column_ref)
  SELECT rowid, connection_id, column_ref
  FROM connection_mask_reviewed WHERE NOT ((NOT EXISTS (SELECT 1 FROM connections p WHERE p.id = connection_mask_reviewed."connection_id")));
DROP TABLE connection_mask_reviewed;
ALTER TABLE connection_mask_reviewed__n RENAME TO connection_mask_reviewed;

-- saved_queries -> connection_saved_queries
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
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'connection_saved_queries', json_object('id', "id", 'connection_id', "connection_id", 'path', "path", 'name', "name", 'kind', "kind", 'body', "body", 'created_at', "created_at", 'used_at', "used_at", 'pinned', "pinned"), 'empty id', 'removed', 85, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM saved_queries WHERE id = '';
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'connection_saved_queries', json_object('id', "id", 'connection_id', "connection_id", 'path', "path", 'name', "name", 'kind', "kind", 'body', "body", 'created_at', "created_at", 'used_at', "used_at", 'pinned', "pinned"), 'orphan connection_id', 'removed', 85, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM saved_queries WHERE NOT EXISTS (SELECT 1 FROM connections p WHERE p.id = saved_queries."connection_id");
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'connection_saved_queries', json_object('id', "id", 'connection_id', "connection_id", 'path', "path", 'name', "name", 'kind', "kind", 'body', "body", 'created_at', "created_at", 'used_at', "used_at", 'pinned', "pinned"), 'invalid kind', 'removed', 85, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM saved_queries WHERE kind NOT IN ('filter', 'console', 'discover');
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'connection_saved_queries', json_object('id', "id", 'connection_id', "connection_id", 'path', "path", 'name', "name", 'kind', "kind", 'body', "body", 'created_at', "created_at", 'used_at', "used_at", 'pinned', "pinned"), 'non-boolean pinned', 'repaired', 85, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM saved_queries WHERE "pinned" NOT IN (0, 1);
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'connection_saved_queries', json_object('id', "id", 'connection_id', "connection_id", 'path', "path", 'name', "name", 'kind', "kind", 'body', "body", 'created_at', "created_at", 'used_at', "used_at", 'pinned', "pinned"), 'unparsable instant created_at', 'repaired', 85, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM saved_queries WHERE unixepoch("created_at") IS NULL;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'connection_saved_queries', json_object('id', "id", 'connection_id', "connection_id", 'path', "path", 'name', "name", 'kind', "kind", 'body', "body", 'created_at', "created_at", 'used_at', "used_at", 'pinned', "pinned"), 'unparsable instant used_at', 'repaired', 85, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM saved_queries WHERE "used_at" IS NOT NULL AND unixepoch("used_at") IS NULL;
INSERT INTO connection_saved_queries (rowid, id, connection_id, path, name, kind, pinned, created_at, used_at, body)
  SELECT rowid, id, connection_id, path, name, kind, COALESCE("pinned" <> 0, 0), COALESCE(MAX(0, CAST(ROUND(unixepoch("created_at",'subsec')*1000) AS INTEGER)), 0), MAX(0, CAST(ROUND(unixepoch("used_at",'subsec')*1000) AS INTEGER)), body
  FROM saved_queries WHERE NOT ((id = '')
    OR (NOT EXISTS (SELECT 1 FROM connections p WHERE p.id = saved_queries."connection_id"))
    OR (kind NOT IN ('filter', 'console', 'discover')));
DROP TABLE saved_queries;
CREATE INDEX connection_saved_queries_path ON connection_saved_queries (connection_id, path, kind, pinned DESC, used_at DESC, name);
CREATE UNIQUE INDEX connection_saved_queries_discover_name ON connection_saved_queries (connection_id, lower(name)) WHERE kind = 'discover';

-- filter_history -> connection_filter_history
CREATE TABLE connection_filter_history (
  id TEXT NOT NULL PRIMARY KEY CHECK (id <> ''),
  connection_id TEXT NOT NULL REFERENCES connections (id) ON DELETE CASCADE,
  path TEXT NOT NULL,
  used_at INTEGER NOT NULL CHECK (used_at >= 0),
  where_text TEXT,
  order_by_json TEXT CHECK (order_by_json IS NULL OR json_valid(order_by_json))
) STRICT;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'connection_filter_history', json_object('id', "id", 'connection_id', "connection_id", 'path', "path", 'where_text', "where_text", 'order_by_json', "order_by_json", 'used_at', "used_at"), 'empty id', 'removed', 85, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM filter_history WHERE id = '';
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'connection_filter_history', json_object('id', "id", 'connection_id', "connection_id", 'path', "path", 'where_text', "where_text", 'order_by_json', "order_by_json", 'used_at', "used_at"), 'orphan connection_id', 'removed', 85, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM filter_history WHERE NOT EXISTS (SELECT 1 FROM connections p WHERE p.id = filter_history."connection_id");
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'connection_filter_history', json_object('id', "id", 'connection_id', "connection_id", 'path', "path", 'where_text', "where_text", 'order_by_json', "order_by_json", 'used_at', "used_at"), 'unparsable instant used_at', 'repaired', 85, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM filter_history WHERE unixepoch("used_at") IS NULL;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'connection_filter_history', json_object('id', "id", 'connection_id', "connection_id", 'path', "path", 'where_text', "where_text", 'order_by_json', "order_by_json", 'used_at', "used_at"), 'invalid JSON order_by_json', 'repaired', 85, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM filter_history WHERE order_by_json IS NOT NULL AND NOT json_valid(order_by_json);
INSERT INTO connection_filter_history (rowid, id, connection_id, path, used_at, where_text, order_by_json)
  SELECT rowid, id, connection_id, path, COALESCE(MAX(0, CAST(ROUND(unixepoch("used_at",'subsec')*1000) AS INTEGER)), 0), where_text, CASE WHEN order_by_json IS NULL OR json_valid(order_by_json) THEN order_by_json ELSE NULL END
  FROM filter_history WHERE NOT ((id = '')
    OR (NOT EXISTS (SELECT 1 FROM connections p WHERE p.id = filter_history."connection_id")));
DROP TABLE filter_history;
CREATE INDEX connection_filter_history_target ON connection_filter_history (connection_id, path, used_at);

-- metadata_cache -> connection_metadata_cache
CREATE TABLE connection_metadata_cache (
  connection_id TEXT NOT NULL REFERENCES connections (id) ON DELETE CASCADE,
  path TEXT NOT NULL,
  kind TEXT NOT NULL,
  fetched_at INTEGER NOT NULL CHECK (fetched_at >= 0),
  etag TEXT,
  payload_json TEXT NOT NULL
) STRICT;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'connection_metadata_cache', json_object('connection_id', "connection_id", 'path', "path", 'kind', "kind", 'payload_json', "payload_json", 'fetched_at', "fetched_at", 'etag', "etag"), 'orphan connection_id', 'removed', 85, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM metadata_cache WHERE NOT EXISTS (SELECT 1 FROM connections p WHERE p.id = metadata_cache."connection_id");
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'connection_metadata_cache', json_object('connection_id', "connection_id", 'path', "path", 'kind', "kind", 'payload_json', "payload_json", 'fetched_at', "fetched_at", 'etag', "etag"), 'unparsable instant fetched_at', 'repaired', 85, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM metadata_cache WHERE unixepoch("fetched_at") IS NULL;
INSERT INTO connection_metadata_cache (rowid, connection_id, path, kind, fetched_at, etag, payload_json)
  SELECT rowid, connection_id, path, kind, COALESCE(MAX(0, CAST(ROUND(unixepoch("fetched_at",'subsec')*1000) AS INTEGER)), 0), etag, payload_json
  FROM metadata_cache WHERE NOT ((NOT EXISTS (SELECT 1 FROM connections p WHERE p.id = metadata_cache."connection_id")));
DROP TABLE metadata_cache;
CREATE UNIQUE INDEX connection_metadata_cache_path ON connection_metadata_cache (connection_id, path);

-- op_log
CREATE TABLE op_log__n (
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
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'op_log', json_object('id', "id", 'started_ms', "started_ms", 'duration_ms', "duration_ms", 'category', "category", 'kind', "kind", 'status', "status", 'module', "module", 'trigger_', "trigger_", 'source', "source", 'subject', "subject", 'repo_root', "repo_root", 'repo_name', "repo_name", 'task_id', "task_id", 'parent_id', "parent_id", 'command', "command", 'error', "error", 'steps_json', "steps_json", 'steps_dropped', "steps_dropped", 'background', "background", 'repeat_count', "repeat_count", 'connection_id', "connection_id", 'tab_id', "tab_id", 'rows', "rows", 'command_truncated', "command_truncated", 'path', "path"), 'empty id', 'removed', 85, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM op_log WHERE id = '';
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'op_log', json_object('id', "id", 'started_ms', "started_ms", 'duration_ms', "duration_ms", 'category', "category", 'kind', "kind", 'status', "status", 'module', "module", 'trigger_', "trigger_", 'source', "source", 'subject', "subject", 'repo_root', "repo_root", 'repo_name', "repo_name", 'task_id', "task_id", 'parent_id', "parent_id", 'command', "command", 'error', "error", 'steps_json', "steps_json", 'steps_dropped', "steps_dropped", 'background', "background", 'repeat_count', "repeat_count", 'connection_id', "connection_id", 'tab_id', "tab_id", 'rows', "rows", 'command_truncated', "command_truncated", 'path', "path"), 'negative started_ms', 'repaired', 85, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM op_log WHERE started_ms < 0;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'op_log', json_object('id', "id", 'started_ms', "started_ms", 'duration_ms', "duration_ms", 'category', "category", 'kind', "kind", 'status', "status", 'module', "module", 'trigger_', "trigger_", 'source', "source", 'subject', "subject", 'repo_root', "repo_root", 'repo_name', "repo_name", 'task_id', "task_id", 'parent_id', "parent_id", 'command', "command", 'error', "error", 'steps_json', "steps_json", 'steps_dropped', "steps_dropped", 'background', "background", 'repeat_count', "repeat_count", 'connection_id', "connection_id", 'tab_id', "tab_id", 'rows', "rows", 'command_truncated', "command_truncated", 'path', "path"), 'negative duration_ms', 'repaired', 85, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM op_log WHERE duration_ms < 0;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'op_log', json_object('id', "id", 'started_ms', "started_ms", 'duration_ms', "duration_ms", 'category', "category", 'kind', "kind", 'status', "status", 'module', "module", 'trigger_', "trigger_", 'source', "source", 'subject', "subject", 'repo_root', "repo_root", 'repo_name', "repo_name", 'task_id', "task_id", 'parent_id', "parent_id", 'command', "command", 'error', "error", 'steps_json', "steps_json", 'steps_dropped', "steps_dropped", 'background', "background", 'repeat_count', "repeat_count", 'connection_id', "connection_id", 'tab_id', "tab_id", 'rows', "rows", 'command_truncated', "command_truncated", 'path', "path"), 'orphan connection_id', 'repaired', 85, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM op_log WHERE connection_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM connections c WHERE c.id = op_log.connection_id);
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'op_log', json_object('id', "id", 'started_ms', "started_ms", 'duration_ms', "duration_ms", 'category', "category", 'kind', "kind", 'status', "status", 'module', "module", 'trigger_', "trigger_", 'source', "source", 'subject', "subject", 'repo_root', "repo_root", 'repo_name', "repo_name", 'task_id', "task_id", 'parent_id', "parent_id", 'command', "command", 'error', "error", 'steps_json', "steps_json", 'steps_dropped', "steps_dropped", 'background', "background", 'repeat_count', "repeat_count", 'connection_id', "connection_id", 'tab_id', "tab_id", 'rows', "rows", 'command_truncated', "command_truncated", 'path', "path"), 'out-of-range rows', 'repaired', 85, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM op_log WHERE "rows" < 0;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'op_log', json_object('id', "id", 'started_ms', "started_ms", 'duration_ms', "duration_ms", 'category', "category", 'kind', "kind", 'status', "status", 'module', "module", 'trigger_', "trigger_", 'source', "source", 'subject', "subject", 'repo_root', "repo_root", 'repo_name', "repo_name", 'task_id', "task_id", 'parent_id', "parent_id", 'command', "command", 'error', "error", 'steps_json', "steps_json", 'steps_dropped', "steps_dropped", 'background', "background", 'repeat_count', "repeat_count", 'connection_id', "connection_id", 'tab_id', "tab_id", 'rows', "rows", 'command_truncated', "command_truncated", 'path', "path"), 'out-of-range steps_dropped', 'repaired', 85, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM op_log WHERE "steps_dropped" < 0;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'op_log', json_object('id', "id", 'started_ms', "started_ms", 'duration_ms', "duration_ms", 'category', "category", 'kind', "kind", 'status', "status", 'module', "module", 'trigger_', "trigger_", 'source', "source", 'subject', "subject", 'repo_root', "repo_root", 'repo_name', "repo_name", 'task_id', "task_id", 'parent_id', "parent_id", 'command', "command", 'error', "error", 'steps_json', "steps_json", 'steps_dropped', "steps_dropped", 'background', "background", 'repeat_count', "repeat_count", 'connection_id', "connection_id", 'tab_id', "tab_id", 'rows', "rows", 'command_truncated', "command_truncated", 'path', "path"), 'non-boolean background', 'repaired', 85, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM op_log WHERE "background" NOT IN (0, 1);
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'op_log', json_object('id', "id", 'started_ms', "started_ms", 'duration_ms', "duration_ms", 'category', "category", 'kind', "kind", 'status', "status", 'module', "module", 'trigger_', "trigger_", 'source', "source", 'subject', "subject", 'repo_root', "repo_root", 'repo_name', "repo_name", 'task_id', "task_id", 'parent_id', "parent_id", 'command', "command", 'error', "error", 'steps_json', "steps_json", 'steps_dropped', "steps_dropped", 'background', "background", 'repeat_count', "repeat_count", 'connection_id', "connection_id", 'tab_id', "tab_id", 'rows', "rows", 'command_truncated', "command_truncated", 'path', "path"), 'out-of-range repeat_count', 'repaired', 85, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM op_log WHERE "repeat_count" < 1;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'op_log', json_object('id', "id", 'started_ms', "started_ms", 'duration_ms', "duration_ms", 'category', "category", 'kind', "kind", 'status', "status", 'module', "module", 'trigger_', "trigger_", 'source', "source", 'subject', "subject", 'repo_root', "repo_root", 'repo_name', "repo_name", 'task_id', "task_id", 'parent_id', "parent_id", 'command', "command", 'error', "error", 'steps_json', "steps_json", 'steps_dropped', "steps_dropped", 'background', "background", 'repeat_count', "repeat_count", 'connection_id', "connection_id", 'tab_id', "tab_id", 'rows', "rows", 'command_truncated', "command_truncated", 'path', "path"), 'non-boolean command_truncated', 'repaired', 85, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM op_log WHERE "command_truncated" NOT IN (0, 1);
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'op_log', json_object('id', "id", 'started_ms', "started_ms", 'duration_ms', "duration_ms", 'category', "category", 'kind', "kind", 'status', "status", 'module', "module", 'trigger_', "trigger_", 'source', "source", 'subject', "subject", 'repo_root', "repo_root", 'repo_name', "repo_name", 'task_id', "task_id", 'parent_id', "parent_id", 'command', "command", 'error', "error", 'steps_json', "steps_json", 'steps_dropped', "steps_dropped", 'background', "background", 'repeat_count', "repeat_count", 'connection_id', "connection_id", 'tab_id', "tab_id", 'rows', "rows", 'command_truncated', "command_truncated", 'path', "path"), 'invalid JSON steps_json', 'repaired', 85, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM op_log WHERE NOT json_valid(steps_json);
INSERT INTO op_log__n (rowid, id, started_ms, duration_ms, category, kind, status, module, trigger_, source, subject, repo_root, repo_name, task_id, parent_id, connection_id, tab_id, path, rows, steps_dropped, background, repeat_count, command_truncated, command, error, steps_json)
  SELECT rowid, id, MAX(0, started_ms), MAX(0, duration_ms), category, kind, status, module, trigger_, source, subject, repo_root, repo_name, task_id, parent_id, CASE WHEN connection_id IS NULL OR EXISTS (SELECT 1 FROM connections c WHERE c.id = op_log.connection_id) THEN connection_id ELSE NULL END, tab_id, path, MAX(0, "rows"), MAX(0, COALESCE("steps_dropped", 0)), COALESCE("background" <> 0, 0), MAX(1, COALESCE("repeat_count", 1)), COALESCE("command_truncated" <> 0, 0), command, error, CASE WHEN json_valid(steps_json) THEN steps_json ELSE '[]' END
  FROM op_log WHERE NOT ((id = ''));
DROP TABLE op_log;
ALTER TABLE op_log__n RENAME TO op_log;
CREATE INDEX op_log_connection ON op_log (connection_id);
CREATE INDEX op_log_started ON op_log (started_ms, id);
CREATE INDEX op_log_category ON op_log (category, started_ms, id);
