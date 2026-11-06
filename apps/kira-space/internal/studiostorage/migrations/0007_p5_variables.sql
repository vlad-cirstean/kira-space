-- P5 D2/D4: variables are rows in this app's own database, owned by either a collection or an
-- environment — never both, never neither. Environments are top-level (docs/HISTORY.md (v1.2)'s P5 row
-- calls them "separate", and a scratch request tab belongs to no collection at all), so they carry
-- no collection foreign key.
CREATE TABLE http_environments (
  id         TEXT PRIMARY KEY,
  name       TEXT NOT NULL,
  sort_order INTEGER NOT NULL,
  -- D3: the app-global selection. The repo keeps at most one row set, in one transaction.
  is_active  INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE http_variables (
  id             TEXT PRIMARY KEY,
  -- Exactly one owner. Both cascade for real: db.go's DSN sets _foreign_keys=1 on every connection
  -- the pool opens (P4 F9), so deleting a collection or an environment deletes its variables.
  collection_id  TEXT REFERENCES http_collections(id) ON DELETE CASCADE,
  environment_id TEXT REFERENCES http_environments(id) ON DELETE CASCADE,
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
  updated_at     TEXT NOT NULL,
  CHECK ((collection_id IS NULL) <> (environment_id IS NULL)),
  CHECK (is_secret IN (0, 1)),
  CHECK ((is_secret = 0 AND secret_value IS NULL) OR (is_secret = 1 AND value = ''))
);

-- D13: the history of prior values, per entry, in both scopes. An environment's "history" is its
-- entries' histories — there is no second notion of one.
CREATE TABLE http_variable_history (
  id           TEXT PRIMARY KEY,
  variable_id  TEXT NOT NULL REFERENCES http_variables(id) ON DELETE CASCADE,
  value        TEXT NOT NULL DEFAULT '',
  is_secret    INTEGER NOT NULL DEFAULT 0,
  secret_value TEXT,
  recorded_at  TEXT NOT NULL
);

CREATE INDEX http_variables_collection  ON http_variables(collection_id, sort_order);
CREATE INDEX http_variables_environment ON http_variables(environment_id, sort_order);
CREATE INDEX http_variable_history_var  ON http_variable_history(variable_id, recorded_at);

-- D15/F5: 0 means "this collection's origin_json may still carry an unpromoted top-level
-- variable[]" — true for every row that existed before this migration. Every row P5's importer
-- writes stamps 1. VariablesRepo.PromoteImported is the one-shot that flips it.
ALTER TABLE http_collections ADD COLUMN variables_promoted INTEGER NOT NULL DEFAULT 0;
