-- kira:foreign-keys off
-- P552 API tables. Rebuilds each table STRICT with checked types; rows that violate a new
-- constraint are repaired or removed and recorded in schema_quarantine.

-- api_collections
CREATE TABLE api_collections__n (
  id TEXT NOT NULL PRIMARY KEY CHECK (id <> ''),
  name TEXT NOT NULL,
  sort_order INTEGER NOT NULL CHECK (sort_order >= 0),
  variables_promoted INTEGER NOT NULL DEFAULT 0 CHECK (variables_promoted IN (0, 1)),
  stem TEXT,
  created_at INTEGER NOT NULL CHECK (created_at >= 0),
  updated_at INTEGER NOT NULL CHECK (updated_at >= 0),
  origin_json TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(origin_json))
) STRICT;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'api_collections', json_object('id', "id", 'name', "name", 'sort_order', "sort_order", 'origin_json', "origin_json", 'created_at', "created_at", 'updated_at', "updated_at", 'variables_promoted', "variables_promoted", 'stem', "stem"), 'empty id', 'removed', 86, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM api_collections WHERE id = '';
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'api_collections', json_object('id', "id", 'name', "name", 'sort_order', "sort_order", 'origin_json', "origin_json", 'created_at', "created_at", 'updated_at', "updated_at", 'variables_promoted', "variables_promoted", 'stem', "stem"), 'out-of-range sort_order', 'repaired', 86, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM api_collections WHERE "sort_order" < 0;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'api_collections', json_object('id', "id", 'name', "name", 'sort_order', "sort_order", 'origin_json', "origin_json", 'created_at', "created_at", 'updated_at', "updated_at", 'variables_promoted', "variables_promoted", 'stem', "stem"), 'non-boolean variables_promoted', 'repaired', 86, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM api_collections WHERE "variables_promoted" NOT IN (0, 1);
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'api_collections', json_object('id', "id", 'name', "name", 'sort_order', "sort_order", 'origin_json', "origin_json", 'created_at', "created_at", 'updated_at', "updated_at", 'variables_promoted', "variables_promoted", 'stem', "stem"), 'unparsable instant created_at', 'repaired', 86, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM api_collections WHERE unixepoch("created_at") IS NULL;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'api_collections', json_object('id', "id", 'name', "name", 'sort_order', "sort_order", 'origin_json', "origin_json", 'created_at', "created_at", 'updated_at', "updated_at", 'variables_promoted', "variables_promoted", 'stem', "stem"), 'unparsable instant updated_at', 'repaired', 86, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM api_collections WHERE unixepoch("updated_at") IS NULL;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'api_collections', json_object('id', "id", 'name', "name", 'sort_order', "sort_order", 'origin_json', "origin_json", 'created_at', "created_at", 'updated_at', "updated_at", 'variables_promoted', "variables_promoted", 'stem', "stem"), 'invalid JSON origin_json', 'repaired', 86, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM api_collections WHERE NOT json_valid(origin_json);
INSERT INTO api_collections__n (rowid, id, name, sort_order, variables_promoted, stem, created_at, updated_at, origin_json)
  SELECT rowid, id, name, MAX(0, COALESCE("sort_order", 0)), COALESCE("variables_promoted" <> 0, 0), stem, COALESCE(MAX(0, CAST(ROUND(unixepoch("created_at",'subsec')*1000) AS INTEGER)), 0), COALESCE(MAX(0, CAST(ROUND(unixepoch("updated_at",'subsec')*1000) AS INTEGER)), 0), CASE WHEN json_valid(origin_json) THEN origin_json ELSE '{}' END
  FROM api_collections WHERE NOT ((id = ''));
DROP TABLE api_collections;
ALTER TABLE api_collections__n RENAME TO api_collections;
CREATE UNIQUE INDEX api_collections_stem ON api_collections (stem) WHERE stem IS NOT NULL;

-- api_environments
CREATE TABLE api_environments__n (
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
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'api_environments', json_object('id', "id", 'name', "name", 'sort_order', "sort_order", 'is_active', "is_active", 'created_at', "created_at", 'updated_at', "updated_at", 'description', "description", 'color', "color", 'stem', "stem"), 'empty id', 'removed', 86, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM api_environments WHERE id = '';
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'api_environments', json_object('id', "id", 'name', "name", 'sort_order', "sort_order", 'is_active', "is_active", 'created_at', "created_at", 'updated_at', "updated_at", 'description', "description", 'color', "color", 'stem', "stem"), 'out-of-range sort_order', 'repaired', 86, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM api_environments WHERE "sort_order" < 0;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'api_environments', json_object('id', "id", 'name', "name", 'sort_order', "sort_order", 'is_active', "is_active", 'created_at', "created_at", 'updated_at', "updated_at", 'description', "description", 'color', "color", 'stem', "stem"), 'more than one active environment', 'repaired', 86, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM api_environments WHERE is_active <> 0 AND rowid <> (SELECT MIN(e.rowid) FROM api_environments e WHERE e.is_active <> 0);
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'api_environments', json_object('id', "id", 'name', "name", 'sort_order', "sort_order", 'is_active', "is_active", 'created_at', "created_at", 'updated_at', "updated_at", 'description', "description", 'color', "color", 'stem', "stem"), 'unparsable instant created_at', 'repaired', 86, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM api_environments WHERE unixepoch("created_at") IS NULL;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'api_environments', json_object('id', "id", 'name', "name", 'sort_order', "sort_order", 'is_active', "is_active", 'created_at', "created_at", 'updated_at', "updated_at", 'description', "description", 'color', "color", 'stem', "stem"), 'unparsable instant updated_at', 'repaired', 86, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM api_environments WHERE unixepoch("updated_at") IS NULL;
INSERT INTO api_environments__n (rowid, id, name, sort_order, is_active, color, stem, created_at, updated_at, description)
  SELECT rowid, id, name, MAX(0, COALESCE("sort_order", 0)), CASE WHEN is_active <> 0 AND rowid = (SELECT MIN(e.rowid) FROM api_environments e WHERE e.is_active <> 0) THEN 1 ELSE 0 END, color, stem, COALESCE(MAX(0, CAST(ROUND(unixepoch("created_at",'subsec')*1000) AS INTEGER)), 0), COALESCE(MAX(0, CAST(ROUND(unixepoch("updated_at",'subsec')*1000) AS INTEGER)), 0), description
  FROM api_environments WHERE NOT ((id = ''));
DROP TABLE api_environments;
ALTER TABLE api_environments__n RENAME TO api_environments;
CREATE UNIQUE INDEX api_environments_stem ON api_environments (stem) WHERE stem IS NOT NULL;
CREATE UNIQUE INDEX api_environments_active ON api_environments (is_active) WHERE is_active = 1;

-- api_items
CREATE TABLE api_items__n (
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
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'api_items', json_object('id', "id", 'collection_id', "collection_id", 'parent_id', "parent_id", 'kind', "kind", 'name', "name", 'sort_order', "sort_order", 'method', "method", 'url', "url", 'request_json', "request_json", 'origin_json', "origin_json", 'created_at', "created_at", 'updated_at', "updated_at", 'protocol', "protocol", 'item_key', "item_key"), 'empty id', 'removed', 86, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM api_items WHERE id = '';
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'api_items', json_object('id', "id", 'collection_id', "collection_id", 'parent_id', "parent_id", 'kind', "kind", 'name', "name", 'sort_order', "sort_order", 'method', "method", 'url', "url", 'request_json', "request_json", 'origin_json', "origin_json", 'created_at', "created_at", 'updated_at', "updated_at", 'protocol', "protocol", 'item_key', "item_key"), 'orphan collection_id', 'removed', 86, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM api_items WHERE NOT EXISTS (SELECT 1 FROM api_collections p WHERE p.id = api_items."collection_id");
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'api_items', json_object('id', "id", 'collection_id', "collection_id", 'parent_id', "parent_id", 'kind', "kind", 'name', "name", 'sort_order', "sort_order", 'method', "method", 'url', "url", 'request_json', "request_json", 'origin_json', "origin_json", 'created_at', "created_at", 'updated_at', "updated_at", 'protocol', "protocol", 'item_key', "item_key"), 'invalid kind', 'removed', 86, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM api_items WHERE kind NOT IN ('folder', 'request');
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'api_items', json_object('id', "id", 'collection_id', "collection_id", 'parent_id', "parent_id", 'kind', "kind", 'name', "name", 'sort_order', "sort_order", 'method', "method", 'url', "url", 'request_json', "request_json", 'origin_json', "origin_json", 'created_at', "created_at", 'updated_at', "updated_at", 'protocol', "protocol", 'item_key', "item_key"), 'invalid protocol', 'removed', 86, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM api_items WHERE protocol NOT IN ('http', 'grpc');
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'api_items', json_object('id', "id", 'collection_id', "collection_id", 'parent_id', "parent_id", 'kind', "kind", 'name', "name", 'sort_order', "sort_order", 'method', "method", 'url', "url", 'request_json', "request_json", 'origin_json', "origin_json", 'created_at', "created_at", 'updated_at', "updated_at", 'protocol', "protocol", 'item_key', "item_key"), 'out-of-range sort_order', 'repaired', 86, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM api_items WHERE "sort_order" < 0;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'api_items', json_object('id', "id", 'collection_id', "collection_id", 'parent_id', "parent_id", 'kind', "kind", 'name', "name", 'sort_order', "sort_order", 'method', "method", 'url', "url", 'request_json', "request_json", 'origin_json', "origin_json", 'created_at', "created_at", 'updated_at', "updated_at", 'protocol', "protocol", 'item_key', "item_key"), 'unparsable instant created_at', 'repaired', 86, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM api_items WHERE unixepoch("created_at") IS NULL;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'api_items', json_object('id', "id", 'collection_id', "collection_id", 'parent_id', "parent_id", 'kind', "kind", 'name', "name", 'sort_order', "sort_order", 'method', "method", 'url', "url", 'request_json', "request_json", 'origin_json', "origin_json", 'created_at', "created_at", 'updated_at', "updated_at", 'protocol', "protocol", 'item_key', "item_key"), 'unparsable instant updated_at', 'repaired', 86, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM api_items WHERE unixepoch("updated_at") IS NULL;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'api_items', json_object('id', "id", 'collection_id', "collection_id", 'parent_id', "parent_id", 'kind', "kind", 'name', "name", 'sort_order', "sort_order", 'method', "method", 'url', "url", 'request_json', "request_json", 'origin_json', "origin_json", 'created_at', "created_at", 'updated_at', "updated_at", 'protocol', "protocol", 'item_key', "item_key"), 'invalid JSON request_json', 'repaired', 86, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM api_items WHERE request_json <> '' AND NOT json_valid(request_json);
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'api_items', json_object('id', "id", 'collection_id', "collection_id", 'parent_id', "parent_id", 'kind', "kind", 'name', "name", 'sort_order', "sort_order", 'method', "method", 'url', "url", 'request_json', "request_json", 'origin_json', "origin_json", 'created_at', "created_at", 'updated_at', "updated_at", 'protocol', "protocol", 'item_key', "item_key"), 'invalid JSON origin_json', 'repaired', 86, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM api_items WHERE NOT json_valid(origin_json);

CREATE TEMP TABLE api_items_bad AS WITH RECURSIVE bad(id) AS (
  SELECT id FROM api_items WHERE (id = '') OR (NOT EXISTS (SELECT 1 FROM api_collections p WHERE p.id = api_items."collection_id")) OR (kind NOT IN ('folder', 'request')) OR (protocol NOT IN ('http', 'grpc')) OR (parent_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM api_items p WHERE p.id = api_items.parent_id))
  UNION SELECT c.id FROM api_items c JOIN bad ON c.parent_id = bad.id
) SELECT id FROM bad;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'api_items', json_object('id', "id", 'collection_id', "collection_id", 'parent_id', "parent_id", 'kind', "kind", 'name', "name", 'sort_order', "sort_order", 'method', "method", 'url', "url", 'request_json', "request_json", 'origin_json', "origin_json", 'created_at', "created_at", 'updated_at', "updated_at", 'protocol', "protocol", 'item_key', "item_key"), 'removed with its parent', 'removed', 86, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM api_items WHERE id IN (SELECT id FROM api_items_bad);
INSERT INTO api_items__n (rowid, id, collection_id, parent_id, kind, protocol, name, sort_order, method, url, item_key, created_at, updated_at, request_json, origin_json)
  SELECT rowid, id, collection_id, parent_id, kind, protocol, name, MAX(0, COALESCE("sort_order", 0)), method, url, item_key, COALESCE(MAX(0, CAST(ROUND(unixepoch("created_at",'subsec')*1000) AS INTEGER)), 0), COALESCE(MAX(0, CAST(ROUND(unixepoch("updated_at",'subsec')*1000) AS INTEGER)), 0), CASE WHEN request_json = '' OR json_valid(request_json) THEN request_json ELSE '' END, CASE WHEN json_valid(origin_json) THEN origin_json ELSE '{}' END
  FROM api_items WHERE NOT ((id IN (SELECT id FROM api_items_bad)));
DROP TABLE api_items;
DROP TABLE api_items_bad;
ALTER TABLE api_items__n RENAME TO api_items;
CREATE UNIQUE INDEX api_items_key ON api_items (collection_id, item_key) WHERE item_key IS NOT NULL;
CREATE INDEX api_items_parent ON api_items (parent_id);
CREATE INDEX api_items_tree ON api_items (collection_id, parent_id, sort_order);

-- api_variables
CREATE TABLE api_variables__n (
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
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'api_variables', json_object('id', "id", 'collection_id', "collection_id", 'environment_id', "environment_id", 'name', "name", 'value', "value", 'is_secret', "is_secret", 'secret_value', "secret_value", 'sort_order', "sort_order", 'created_at', "created_at", 'updated_at', "updated_at", 'description', "description", 'secret_ref', "secret_ref"), 'empty id', 'removed', 86, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM api_variables WHERE id = '';
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'api_variables', json_object('id', "id", 'collection_id', "collection_id", 'environment_id', "environment_id", 'name', "name", 'value', "value", 'is_secret', "is_secret", 'secret_value', "secret_value", 'sort_order', "sort_order", 'created_at', "created_at", 'updated_at', "updated_at", 'description', "description", 'secret_ref', "secret_ref"), 'orphan collection_id', 'removed', 86, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM api_variables WHERE collection_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM api_collections p WHERE p.id = api_variables."collection_id");
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'api_variables', json_object('id', "id", 'collection_id', "collection_id", 'environment_id', "environment_id", 'name', "name", 'value', "value", 'is_secret', "is_secret", 'secret_value', "secret_value", 'sort_order', "sort_order", 'created_at', "created_at", 'updated_at', "updated_at", 'description', "description", 'secret_ref', "secret_ref"), 'orphan environment_id', 'removed', 86, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM api_variables WHERE environment_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM api_environments p WHERE p.id = api_variables."environment_id");
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'api_variables', json_object('id', "id", 'collection_id', "collection_id", 'environment_id', "environment_id", 'name', "name", 'value', "value", 'is_secret', "is_secret", 'secret_value', "secret_value", 'sort_order', "sort_order", 'created_at', "created_at", 'updated_at', "updated_at", 'description', "description", 'secret_ref', "secret_ref"), 'out-of-range sort_order', 'repaired', 86, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM api_variables WHERE "sort_order" < 0;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'api_variables', json_object('id', "id", 'collection_id', "collection_id", 'environment_id', "environment_id", 'name', "name", 'value', "value", 'is_secret', "is_secret", 'secret_value', "secret_value", 'sort_order', "sort_order", 'created_at', "created_at", 'updated_at', "updated_at", 'description', "description", 'secret_ref', "secret_ref"), 'non-boolean is_secret', 'repaired', 86, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM api_variables WHERE "is_secret" NOT IN (0, 1);
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'api_variables', json_object('id', "id", 'collection_id', "collection_id", 'environment_id', "environment_id", 'name', "name", 'value', "value", 'is_secret', "is_secret", 'secret_value', "secret_value", 'sort_order', "sort_order", 'created_at', "created_at", 'updated_at', "updated_at", 'description', "description", 'secret_ref', "secret_ref"), 'unparsable instant created_at', 'repaired', 86, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM api_variables WHERE unixepoch("created_at") IS NULL;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'api_variables', json_object('id', "id", 'collection_id', "collection_id", 'environment_id', "environment_id", 'name', "name", 'value', "value", 'is_secret', "is_secret", 'secret_value', "secret_value", 'sort_order', "sort_order", 'created_at', "created_at", 'updated_at', "updated_at", 'description', "description", 'secret_ref', "secret_ref"), 'unparsable instant updated_at', 'repaired', 86, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM api_variables WHERE unixepoch("updated_at") IS NULL;
INSERT INTO api_variables__n (rowid, id, collection_id, environment_id, name, sort_order, is_secret, value, description, secret_ref, created_at, updated_at, secret_value)
  SELECT rowid, id, collection_id, environment_id, name, MAX(0, COALESCE("sort_order", 0)), COALESCE("is_secret" <> 0, 0), value, description, secret_ref, COALESCE(MAX(0, CAST(ROUND(unixepoch("created_at",'subsec')*1000) AS INTEGER)), 0), COALESCE(MAX(0, CAST(ROUND(unixepoch("updated_at",'subsec')*1000) AS INTEGER)), 0), secret_value
  FROM api_variables WHERE NOT ((id = '')
    OR (collection_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM api_collections p WHERE p.id = api_variables."collection_id"))
    OR (environment_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM api_environments p WHERE p.id = api_variables."environment_id")));
DROP TABLE api_variables;
ALTER TABLE api_variables__n RENAME TO api_variables;
CREATE INDEX api_variables_collection ON api_variables (collection_id, sort_order);
CREATE INDEX api_variables_environment ON api_variables (environment_id, sort_order);

-- api_variable_history
CREATE TABLE api_variable_history__n (
  id TEXT NOT NULL PRIMARY KEY CHECK (id <> ''),
  variable_id TEXT NOT NULL REFERENCES api_variables (id) ON DELETE CASCADE,
  is_secret INTEGER NOT NULL DEFAULT 0 CHECK (is_secret IN (0, 1)),
  recorded_at INTEGER NOT NULL CHECK (recorded_at >= 0),
  value TEXT NOT NULL DEFAULT '',
  secret_value TEXT
) STRICT;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'api_variable_history', json_object('id', "id", 'variable_id', "variable_id", 'value', "value", 'is_secret', "is_secret", 'secret_value', "secret_value", 'recorded_at', "recorded_at"), 'empty id', 'removed', 86, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM api_variable_history WHERE id = '';
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'api_variable_history', json_object('id', "id", 'variable_id', "variable_id", 'value', "value", 'is_secret', "is_secret", 'secret_value', "secret_value", 'recorded_at', "recorded_at"), 'orphan variable_id', 'removed', 86, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM api_variable_history WHERE NOT EXISTS (SELECT 1 FROM api_variables p WHERE p.id = api_variable_history."variable_id");
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'api_variable_history', json_object('id', "id", 'variable_id', "variable_id", 'value', "value", 'is_secret', "is_secret", 'secret_value', "secret_value", 'recorded_at', "recorded_at"), 'non-boolean is_secret', 'repaired', 86, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM api_variable_history WHERE "is_secret" NOT IN (0, 1);
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'api_variable_history', json_object('id', "id", 'variable_id', "variable_id", 'value', "value", 'is_secret', "is_secret", 'secret_value', "secret_value", 'recorded_at', "recorded_at"), 'unparsable instant recorded_at', 'repaired', 86, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM api_variable_history WHERE unixepoch("recorded_at") IS NULL;
INSERT INTO api_variable_history__n (rowid, id, variable_id, is_secret, recorded_at, value, secret_value)
  SELECT rowid, id, variable_id, COALESCE("is_secret" <> 0, 0), COALESCE(MAX(0, CAST(ROUND(unixepoch("recorded_at",'subsec')*1000) AS INTEGER)), 0), value, secret_value
  FROM api_variable_history WHERE NOT ((id = '')
    OR (NOT EXISTS (SELECT 1 FROM api_variables p WHERE p.id = api_variable_history."variable_id")));
DROP TABLE api_variable_history;
ALTER TABLE api_variable_history__n RENAME TO api_variable_history;
CREATE INDEX api_variable_history_var ON api_variable_history (variable_id, recorded_at);

-- api_response_history
CREATE TABLE api_response_history__n (
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
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'api_response_history', json_object('id', "id", 'item_id', "item_id", 'tab_id', "tab_id", 'sent_at', "sent_at", 'method', "method", 'url', "url", 'environment', "environment", 'status', "status", 'status_text', "status_text", 'elapsed_ms', "elapsed_ms", 'body_bytes', "body_bytes", 'stored_bytes', "stored_bytes", 'snapshot_json', "snapshot_json"), 'empty id', 'removed', 86, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM api_response_history WHERE id = '';
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'api_response_history', json_object('id', "id", 'item_id', "item_id", 'tab_id', "tab_id", 'sent_at', "sent_at", 'method', "method", 'url', "url", 'environment', "environment", 'status', "status", 'status_text', "status_text", 'elapsed_ms', "elapsed_ms", 'body_bytes', "body_bytes", 'stored_bytes', "stored_bytes", 'snapshot_json', "snapshot_json"), 'orphan item_id', 'removed', 86, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM api_response_history WHERE item_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM api_items p WHERE p.id = api_response_history."item_id");
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'api_response_history', json_object('id', "id", 'item_id', "item_id", 'tab_id', "tab_id", 'sent_at', "sent_at", 'method', "method", 'url', "url", 'environment', "environment", 'status', "status", 'status_text', "status_text", 'elapsed_ms', "elapsed_ms", 'body_bytes', "body_bytes", 'stored_bytes', "stored_bytes", 'snapshot_json', "snapshot_json"), 'unparsable instant sent_at', 'repaired', 86, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM api_response_history WHERE unixepoch("sent_at") IS NULL;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'api_response_history', json_object('id', "id", 'item_id', "item_id", 'tab_id', "tab_id", 'sent_at', "sent_at", 'method', "method", 'url', "url", 'environment', "environment", 'status', "status", 'status_text', "status_text", 'elapsed_ms', "elapsed_ms", 'body_bytes', "body_bytes", 'stored_bytes', "stored_bytes", 'snapshot_json', "snapshot_json"), 'out-of-range status', 'repaired', 86, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM api_response_history WHERE "status" < 0;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'api_response_history', json_object('id', "id", 'item_id', "item_id", 'tab_id', "tab_id", 'sent_at', "sent_at", 'method', "method", 'url', "url", 'environment', "environment", 'status', "status", 'status_text', "status_text", 'elapsed_ms', "elapsed_ms", 'body_bytes', "body_bytes", 'stored_bytes', "stored_bytes", 'snapshot_json', "snapshot_json"), 'out-of-range elapsed_ms', 'repaired', 86, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM api_response_history WHERE "elapsed_ms" < 0;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'api_response_history', json_object('id', "id", 'item_id', "item_id", 'tab_id', "tab_id", 'sent_at', "sent_at", 'method', "method", 'url', "url", 'environment', "environment", 'status', "status", 'status_text', "status_text", 'elapsed_ms', "elapsed_ms", 'body_bytes', "body_bytes", 'stored_bytes', "stored_bytes", 'snapshot_json', "snapshot_json"), 'out-of-range body_bytes', 'repaired', 86, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM api_response_history WHERE "body_bytes" < 0;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'api_response_history', json_object('id', "id", 'item_id', "item_id", 'tab_id', "tab_id", 'sent_at', "sent_at", 'method', "method", 'url', "url", 'environment', "environment", 'status', "status", 'status_text', "status_text", 'elapsed_ms', "elapsed_ms", 'body_bytes', "body_bytes", 'stored_bytes', "stored_bytes", 'snapshot_json', "snapshot_json"), 'out-of-range stored_bytes', 'repaired', 86, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM api_response_history WHERE "stored_bytes" < 0;
INSERT INTO api_response_history__n (rowid, id, item_id, tab_id, sent_at, status, elapsed_ms, body_bytes, stored_bytes, method, url, environment, status_text, snapshot_json)
  SELECT rowid, id, item_id, tab_id, COALESCE(MAX(0, CAST(ROUND(unixepoch("sent_at",'subsec')*1000) AS INTEGER)), 0), MAX(0, COALESCE("status", 0)), MAX(0, COALESCE("elapsed_ms", 0)), MAX(0, COALESCE("body_bytes", 0)), MAX(0, COALESCE("stored_bytes", 0)), method, url, environment, status_text, snapshot_json
  FROM api_response_history WHERE NOT ((id = '')
    OR (item_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM api_items p WHERE p.id = api_response_history."item_id")));
DROP TABLE api_response_history;
ALTER TABLE api_response_history__n RENAME TO api_response_history;
CREATE INDEX api_response_history_age ON api_response_history (sent_at);
CREATE INDEX api_response_history_bytes ON api_response_history (stored_bytes);
CREATE INDEX api_response_history_item ON api_response_history (item_id);
CREATE INDEX api_response_history_scope ON api_response_history (scope_key, sent_at);
CREATE INDEX api_response_history_tab ON api_response_history (tab_id);

-- grpc_call_history -> api_grpc_history
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
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'api_grpc_history', json_object('id', "id", 'item_id', "item_id", 'tab_id', "tab_id", 'called_at', "called_at", 'target', "target", 'method', "method", 'streaming', "streaming", 'environment', "environment", 'code', "code", 'code_name', "code_name", 'status_message', "status_message", 'elapsed_ms', "elapsed_ms", 'message_count', "message_count", 'message_bytes', "message_bytes", 'stored_bytes', "stored_bytes", 'snapshot_json', "snapshot_json"), 'empty id', 'removed', 86, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM grpc_call_history WHERE id = '';
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'api_grpc_history', json_object('id', "id", 'item_id', "item_id", 'tab_id', "tab_id", 'called_at', "called_at", 'target', "target", 'method', "method", 'streaming', "streaming", 'environment', "environment", 'code', "code", 'code_name', "code_name", 'status_message', "status_message", 'elapsed_ms', "elapsed_ms", 'message_count', "message_count", 'message_bytes', "message_bytes", 'stored_bytes', "stored_bytes", 'snapshot_json', "snapshot_json"), 'orphan item_id', 'removed', 86, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM grpc_call_history WHERE item_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM api_items p WHERE p.id = grpc_call_history."item_id");
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'api_grpc_history', json_object('id', "id", 'item_id', "item_id", 'tab_id', "tab_id", 'called_at', "called_at", 'target', "target", 'method', "method", 'streaming', "streaming", 'environment', "environment", 'code', "code", 'code_name', "code_name", 'status_message', "status_message", 'elapsed_ms', "elapsed_ms", 'message_count', "message_count", 'message_bytes', "message_bytes", 'stored_bytes', "stored_bytes", 'snapshot_json', "snapshot_json"), 'unparsable instant called_at', 'repaired', 86, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM grpc_call_history WHERE unixepoch("called_at") IS NULL;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'api_grpc_history', json_object('id', "id", 'item_id', "item_id", 'tab_id', "tab_id", 'called_at', "called_at", 'target', "target", 'method', "method", 'streaming', "streaming", 'environment', "environment", 'code', "code", 'code_name', "code_name", 'status_message', "status_message", 'elapsed_ms', "elapsed_ms", 'message_count', "message_count", 'message_bytes', "message_bytes", 'stored_bytes', "stored_bytes", 'snapshot_json', "snapshot_json"), 'invalid streaming', 'removed', 86, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM grpc_call_history WHERE streaming NOT IN ('unary', 'server');
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'api_grpc_history', json_object('id', "id", 'item_id', "item_id", 'tab_id', "tab_id", 'called_at', "called_at", 'target', "target", 'method', "method", 'streaming', "streaming", 'environment', "environment", 'code', "code", 'code_name', "code_name", 'status_message', "status_message", 'elapsed_ms', "elapsed_ms", 'message_count', "message_count", 'message_bytes', "message_bytes", 'stored_bytes', "stored_bytes", 'snapshot_json', "snapshot_json"), 'out-of-range code', 'repaired', 86, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM grpc_call_history WHERE "code" < 0;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'api_grpc_history', json_object('id', "id", 'item_id', "item_id", 'tab_id', "tab_id", 'called_at', "called_at", 'target', "target", 'method', "method", 'streaming', "streaming", 'environment', "environment", 'code', "code", 'code_name', "code_name", 'status_message', "status_message", 'elapsed_ms', "elapsed_ms", 'message_count', "message_count", 'message_bytes', "message_bytes", 'stored_bytes', "stored_bytes", 'snapshot_json', "snapshot_json"), 'out-of-range elapsed_ms', 'repaired', 86, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM grpc_call_history WHERE "elapsed_ms" < 0;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'api_grpc_history', json_object('id', "id", 'item_id', "item_id", 'tab_id', "tab_id", 'called_at', "called_at", 'target', "target", 'method', "method", 'streaming', "streaming", 'environment', "environment", 'code', "code", 'code_name', "code_name", 'status_message', "status_message", 'elapsed_ms', "elapsed_ms", 'message_count', "message_count", 'message_bytes', "message_bytes", 'stored_bytes', "stored_bytes", 'snapshot_json', "snapshot_json"), 'out-of-range message_count', 'repaired', 86, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM grpc_call_history WHERE "message_count" < 0;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'api_grpc_history', json_object('id', "id", 'item_id', "item_id", 'tab_id', "tab_id", 'called_at', "called_at", 'target', "target", 'method', "method", 'streaming', "streaming", 'environment', "environment", 'code', "code", 'code_name', "code_name", 'status_message', "status_message", 'elapsed_ms', "elapsed_ms", 'message_count', "message_count", 'message_bytes', "message_bytes", 'stored_bytes', "stored_bytes", 'snapshot_json', "snapshot_json"), 'out-of-range message_bytes', 'repaired', 86, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM grpc_call_history WHERE "message_bytes" < 0;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'api_grpc_history', json_object('id', "id", 'item_id', "item_id", 'tab_id', "tab_id", 'called_at', "called_at", 'target', "target", 'method', "method", 'streaming', "streaming", 'environment', "environment", 'code', "code", 'code_name', "code_name", 'status_message', "status_message", 'elapsed_ms', "elapsed_ms", 'message_count', "message_count", 'message_bytes', "message_bytes", 'stored_bytes', "stored_bytes", 'snapshot_json', "snapshot_json"), 'out-of-range stored_bytes', 'repaired', 86, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM grpc_call_history WHERE "stored_bytes" < 0;
INSERT INTO api_grpc_history (rowid, id, item_id, tab_id, called_at, streaming, code, elapsed_ms, message_count, message_bytes, stored_bytes, target, method, environment, code_name, status_message, snapshot_json)
  SELECT rowid, id, item_id, tab_id, COALESCE(MAX(0, CAST(ROUND(unixepoch("called_at",'subsec')*1000) AS INTEGER)), 0), streaming, MAX(0, COALESCE("code", 0)), MAX(0, COALESCE("elapsed_ms", 0)), MAX(0, COALESCE("message_count", 0)), MAX(0, COALESCE("message_bytes", 0)), MAX(0, COALESCE("stored_bytes", 0)), target, method, environment, code_name, status_message, snapshot_json
  FROM grpc_call_history WHERE NOT ((id = '')
    OR (item_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM api_items p WHERE p.id = grpc_call_history."item_id"))
    OR (streaming NOT IN ('unary', 'server')));
DROP TABLE grpc_call_history;
CREATE INDEX api_grpc_history_age ON api_grpc_history (called_at);
CREATE INDEX api_grpc_history_bytes ON api_grpc_history (stored_bytes);
CREATE INDEX api_grpc_history_item ON api_grpc_history (item_id);
CREATE INDEX api_grpc_history_scope ON api_grpc_history (scope_key, called_at);
CREATE INDEX api_grpc_history_tab ON api_grpc_history (tab_id);
