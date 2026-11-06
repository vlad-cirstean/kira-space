-- kira:foreign-keys off
-- P552 core tables. Rebuilds each table STRICT with checked types; rows that violate a new
-- constraint are repaired or removed and recorded in schema_quarantine.

-- settings
CREATE TABLE settings__n (
  key TEXT NOT NULL PRIMARY KEY CHECK (key <> ''),
  value TEXT NOT NULL
) STRICT;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'settings', json_object('key', "key", 'value', "value"), 'empty key', 'removed', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM settings WHERE key = '';
INSERT INTO settings__n (rowid, key, value)
  SELECT rowid, key, value
  FROM settings WHERE NOT ((key = ''));
DROP TABLE settings;
ALTER TABLE settings__n RENAME TO settings;

-- ui_layout
CREATE TABLE ui_layout__n (
  key TEXT NOT NULL PRIMARY KEY CHECK (key <> ''),
  value TEXT NOT NULL
) STRICT;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'ui_layout', json_object('key', "key", 'value', "value"), 'empty key', 'removed', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ui_layout WHERE key = '';
INSERT INTO ui_layout__n (rowid, key, value)
  SELECT rowid, key, value
  FROM ui_layout WHERE NOT ((key = ''));
DROP TABLE ui_layout;
ALTER TABLE ui_layout__n RENAME TO ui_layout;

-- windows
CREATE TABLE windows__n (
  key TEXT NOT NULL PRIMARY KEY CHECK (key <> ''),
  sort_order INTEGER NOT NULL CHECK (sort_order >= 0),
  mode TEXT NOT NULL DEFAULT 'git',
  bounds_json TEXT CHECK (bounds_json IS NULL OR json_valid(bounds_json))
) STRICT;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'windows', json_object('key', "key", 'order', "order", 'bounds_json', "bounds_json", 'mode', "mode"), 'empty key', 'removed', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM windows WHERE key = '';
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'windows', json_object('key', "key", 'order', "order", 'bounds_json', "bounds_json", 'mode', "mode"), 'out-of-range sort_order', 'repaired', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM windows WHERE "order" < 0;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'windows', json_object('key', "key", 'order', "order", 'bounds_json', "bounds_json", 'mode', "mode"), 'invalid JSON bounds_json', 'repaired', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM windows WHERE bounds_json IS NOT NULL AND NOT json_valid(bounds_json);
INSERT INTO windows__n (rowid, key, sort_order, mode, bounds_json)
  SELECT rowid, key, MAX(0, COALESCE("order", 0)), mode, CASE WHEN bounds_json IS NULL OR json_valid(bounds_json) THEN bounds_json ELSE NULL END
  FROM windows WHERE NOT ((key = ''));
DROP TABLE windows;
ALTER TABLE windows__n RENAME TO windows;

-- file_handler_previous
CREATE TABLE file_handler_previous__n (
  ext TEXT NOT NULL PRIMARY KEY CHECK (ext <> ''),
  app_path TEXT NOT NULL,
  bundle_id TEXT NOT NULL,
  saved_at INTEGER NOT NULL CHECK (saved_at >= 0)
) STRICT;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'file_handler_previous', json_object('ext', "ext", 'app_path', "app_path", 'bundle_id', "bundle_id", 'saved_at', "saved_at"), 'empty ext', 'removed', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM file_handler_previous WHERE ext = '';
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'file_handler_previous', json_object('ext', "ext", 'app_path', "app_path", 'bundle_id', "bundle_id", 'saved_at', "saved_at"), 'negative saved_at', 'repaired', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM file_handler_previous WHERE saved_at < 0;
INSERT INTO file_handler_previous__n (rowid, ext, app_path, bundle_id, saved_at)
  SELECT rowid, ext, app_path, bundle_id, MAX(0, saved_at)
  FROM file_handler_previous WHERE NOT ((ext = ''));
DROP TABLE file_handler_previous;
ALTER TABLE file_handler_previous__n RENAME TO file_handler_previous;

-- mobile_devices
CREATE TABLE mobile_devices__n (
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
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'mobile_devices', json_object('id', "id", 'label', "label", 'user_agent', "user_agent", 'created_at', "created_at", 'last_seen_at', "last_seen_at", 'last_ip', "last_ip", 'revoked_at', "revoked_at", 'can_write', "can_write", 'can_agent_input', "can_agent_input", 'expires_at', "expires_at"), 'empty id', 'removed', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM mobile_devices WHERE id = '';
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'mobile_devices', json_object('id', "id", 'label', "label", 'user_agent', "user_agent", 'created_at', "created_at", 'last_seen_at', "last_seen_at", 'last_ip', "last_ip", 'revoked_at', "revoked_at", 'can_write', "can_write", 'can_agent_input', "can_agent_input", 'expires_at', "expires_at"), 'negative created_at', 'repaired', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM mobile_devices WHERE created_at < 0;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'mobile_devices', json_object('id', "id", 'label', "label", 'user_agent', "user_agent", 'created_at', "created_at", 'last_seen_at', "last_seen_at", 'last_ip', "last_ip", 'revoked_at', "revoked_at", 'can_write', "can_write", 'can_agent_input', "can_agent_input", 'expires_at', "expires_at"), 'negative last_seen_at', 'repaired', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM mobile_devices WHERE last_seen_at < 0;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'mobile_devices', json_object('id', "id", 'label', "label", 'user_agent', "user_agent", 'created_at', "created_at", 'last_seen_at', "last_seen_at", 'last_ip', "last_ip", 'revoked_at', "revoked_at", 'can_write', "can_write", 'can_agent_input', "can_agent_input", 'expires_at', "expires_at"), 'negative revoked_at', 'repaired', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM mobile_devices WHERE revoked_at < 0;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'mobile_devices', json_object('id', "id", 'label', "label", 'user_agent', "user_agent", 'created_at', "created_at", 'last_seen_at', "last_seen_at", 'last_ip', "last_ip", 'revoked_at', "revoked_at", 'can_write', "can_write", 'can_agent_input', "can_agent_input", 'expires_at', "expires_at"), 'non-boolean can_write', 'repaired', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM mobile_devices WHERE "can_write" NOT IN (0, 1);
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'mobile_devices', json_object('id', "id", 'label', "label", 'user_agent', "user_agent", 'created_at', "created_at", 'last_seen_at', "last_seen_at", 'last_ip', "last_ip", 'revoked_at', "revoked_at", 'can_write', "can_write", 'can_agent_input', "can_agent_input", 'expires_at', "expires_at"), 'non-boolean can_agent_input', 'repaired', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM mobile_devices WHERE "can_agent_input" NOT IN (0, 1);
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'mobile_devices', json_object('id', "id", 'label', "label", 'user_agent', "user_agent", 'created_at', "created_at", 'last_seen_at', "last_seen_at", 'last_ip', "last_ip", 'revoked_at', "revoked_at", 'can_write', "can_write", 'can_agent_input', "can_agent_input", 'expires_at', "expires_at"), 'negative expires_at', 'repaired', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM mobile_devices WHERE expires_at < 0;
INSERT INTO mobile_devices__n (rowid, id, label, user_agent, token_hash, token_salt, created_at, last_seen_at, last_ip, revoked_at, can_write, can_agent_input, expires_at)
  SELECT rowid, id, label, user_agent, token_hash, token_salt, MAX(0, created_at), MAX(0, last_seen_at), last_ip, MAX(0, revoked_at), COALESCE("can_write" <> 0, 0), COALESCE("can_agent_input" <> 0, 0), MAX(0, expires_at)
  FROM mobile_devices WHERE NOT ((id = ''));
DROP TABLE mobile_devices;
ALTER TABLE mobile_devices__n RENAME TO mobile_devices;

-- mobile_trusted_network
CREATE TABLE mobile_trusted_network__n (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  subnet TEXT NOT NULL,
  router_ip TEXT NOT NULL,
  router_mac TEXT NOT NULL,
  interface TEXT NOT NULL,
  trusted_at INTEGER NOT NULL CHECK (trusted_at >= 0)
) STRICT;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'mobile_trusted_network', json_object('id', "id", 'subnet', "subnet", 'router_ip', "router_ip", 'router_mac', "router_mac", 'interface', "interface", 'trusted_at', "trusted_at"), 'negative trusted_at', 'repaired', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM mobile_trusted_network WHERE trusted_at < 0;
INSERT INTO mobile_trusted_network__n (id, subnet, router_ip, router_mac, interface, trusted_at)
  SELECT id, subnet, router_ip, router_mac, interface, MAX(0, trusted_at)
  FROM mobile_trusted_network;
DROP TABLE mobile_trusted_network;
ALTER TABLE mobile_trusted_network__n RENAME TO mobile_trusted_network;

-- secret_values
CREATE TABLE secret_values__n (
  scope TEXT NOT NULL,
  owner TEXT NOT NULL,
  name TEXT NOT NULL,
  value TEXT NOT NULL,
  updated_ms INTEGER NOT NULL CHECK (updated_ms >= 0),
  PRIMARY KEY (scope, owner, name)
) STRICT, WITHOUT ROWID;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'secret_values', json_object('scope', "scope", 'owner', "owner", 'name', "name", 'value', "value", 'updated_ms', "updated_ms"), 'negative updated_ms', 'repaired', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM secret_values WHERE updated_ms < 0;
INSERT INTO secret_values__n (scope, owner, name, value, updated_ms)
  SELECT scope, owner, name, value, MAX(0, updated_ms)
  FROM secret_values;
DROP TABLE secret_values;
ALTER TABLE secret_values__n RENAME TO secret_values;

-- config_last_valid
CREATE TABLE config_last_valid__n (
  folder TEXT NOT NULL,
  file TEXT NOT NULL,
  value_json TEXT NOT NULL CHECK (json_valid(value_json)),
  recorded_at INTEGER NOT NULL CHECK (recorded_at >= 0),
  PRIMARY KEY (folder, file)
) STRICT, WITHOUT ROWID;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'config_last_valid', json_object('folder', "folder", 'file', "file", 'value_json', "value_json", 'recorded_at', "recorded_at"), 'invalid JSON value_json', 'removed', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM config_last_valid WHERE NOT json_valid(value_json);
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'config_last_valid', json_object('folder', "folder", 'file', "file", 'value_json', "value_json", 'recorded_at', "recorded_at"), 'negative recorded_at', 'repaired', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM config_last_valid WHERE recorded_at < 0;
INSERT INTO config_last_valid__n (folder, file, value_json, recorded_at)
  SELECT folder, file, value_json, MAX(0, recorded_at)
  FROM config_last_valid WHERE NOT ((NOT json_valid(value_json)));
DROP TABLE config_last_valid;
ALTER TABLE config_last_valid__n RENAME TO config_last_valid;

INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'config_last_valid', json_object('file_name', file_name, 'recorded_at', recorded_at), 'invalid JSON workflow_json', 'removed', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM ade_workflow_last_valid WHERE NOT json_valid(workflow_json);
INSERT OR IGNORE INTO config_last_valid (folder, file, value_json, recorded_at)
  SELECT 'workflows', file_name, workflow_json, MAX(0, recorded_at) FROM ade_workflow_last_valid WHERE json_valid(workflow_json);
DROP TABLE ade_workflow_last_valid;


-- tabs
CREATE TABLE tabs__n (
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
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'tabs', json_object('id', "id", 'connection_id', "connection_id", 'path', "path", 'kind', "kind", 'state_json', "state_json", 'order', "order", 'active', "active", 'window_key', "window_key", 'workspace_id', "workspace_id"), 'empty id', 'removed', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM tabs WHERE id = '';
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'tabs', json_object('id', "id", 'connection_id', "connection_id", 'path', "path", 'kind', "kind", 'state_json', "state_json", 'order', "order", 'active', "active", 'window_key', "window_key", 'workspace_id', "workspace_id"), 'orphan window_key', 'removed', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM tabs WHERE NOT EXISTS (SELECT 1 FROM windows p WHERE p.key = tabs."window_key");
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'tabs', json_object('id', "id", 'connection_id', "connection_id", 'path', "path", 'kind', "kind", 'state_json', "state_json", 'order', "order", 'active', "active", 'window_key', "window_key", 'workspace_id', "workspace_id"), 'orphan connection_id', 'removed', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM tabs WHERE connection_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM connections p WHERE p.id = tabs."connection_id");
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'tabs', json_object('id', "id", 'connection_id', "connection_id", 'path', "path", 'kind', "kind", 'state_json', "state_json", 'order', "order", 'active', "active", 'window_key', "window_key", 'workspace_id', "workspace_id"), 'out-of-range sort_order', 'repaired', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM tabs WHERE "order" < 0;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'tabs', json_object('id', "id", 'connection_id', "connection_id", 'path', "path", 'kind', "kind", 'state_json', "state_json", 'order', "order", 'active', "active", 'window_key', "window_key", 'workspace_id', "workspace_id"), 'non-boolean active', 'repaired', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM tabs WHERE "active" NOT IN (0, 1);
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'tabs', json_object('id', "id", 'connection_id', "connection_id", 'path', "path", 'kind', "kind", 'state_json', "state_json", 'order', "order", 'active', "active", 'window_key', "window_key", 'workspace_id', "workspace_id"), 'invalid JSON state_json', 'removed', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM tabs WHERE NOT json_valid(state_json);
INSERT INTO tabs__n (rowid, id, window_key, connection_id, sort_order, active, kind, path, workspace_id, state_json)
  SELECT rowid, id, window_key, connection_id, MAX(0, COALESCE("order", 0)), COALESCE("active" <> 0, 0), kind, path, workspace_id, state_json
  FROM tabs WHERE NOT ((id = '')
    OR (NOT EXISTS (SELECT 1 FROM windows p WHERE p.key = tabs."window_key"))
    OR (connection_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM connections p WHERE p.id = tabs."connection_id"))
    OR (NOT json_valid(state_json)));
DROP TABLE tabs;
ALTER TABLE tabs__n RENAME TO tabs;
CREATE INDEX tabs_window ON tabs (window_key, sort_order);
CREATE INDEX tabs_connection ON tabs (connection_id);
