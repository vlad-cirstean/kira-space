-- P8 D4: a window is now addressable — the `windows` table gives each workbench a durable key,
-- and `tabs.window_key` scopes tab ownership by it (F6's fix). The single legacy `window.bounds`
-- row in `ui_layout` (one rectangle for every window there has ever been) seeds the first
-- `windows` row and is then left in place, inert — the same documented-orphan treatment
-- `advanced.engineMemoryCapMb` already got (docs/ARCHITECTURE.md's Storage section): a
-- schema-version bump to delete one harmless leaf row is not worth the migration-ordering risk.
CREATE TABLE windows (
  key         TEXT PRIMARY KEY,
  "order"     INTEGER NOT NULL,
  bounds_json TEXT
);

INSERT INTO windows (key, "order", bounds_json)
  SELECT 'main', 0, (SELECT value FROM ui_layout WHERE key = 'window.bounds');

-- `tabs.window_key` needs both NOT NULL and a foreign key into the table just created above, and
-- SQLite's ALTER TABLE ADD COLUMN refuses a REFERENCES column that also carries a non-NULL
-- DEFAULT ("Cannot add a REFERENCES column with non-NULL default value") — confirmed against the
-- driver this app actually ships (modernc.org/sqlite), not assumed from the docs. The standard
-- rebuild-and-swap works around it and gets the real constraint, not an unenforced approximation:
-- every existing row backfills to 'main' (the only window that can exist before this migration
-- runs), which already satisfies the new FK against the row inserted above, so no
-- foreign_keys-off dance is needed either.
CREATE TABLE tabs_new (
  id TEXT PRIMARY KEY,
  connection_id TEXT REFERENCES connections(id) ON DELETE CASCADE,
  path TEXT NOT NULL,
  kind TEXT NOT NULL,
  state_json TEXT NOT NULL,
  "order" INTEGER NOT NULL,
  active INTEGER NOT NULL DEFAULT 0,
  window_key TEXT NOT NULL DEFAULT 'main' REFERENCES windows(key) ON DELETE CASCADE
);
INSERT INTO tabs_new (id, connection_id, path, kind, state_json, "order", active, window_key)
  SELECT id, connection_id, path, kind, state_json, "order", active, 'main' FROM tabs;
DROP TABLE tabs;
ALTER TABLE tabs_new RENAME TO tabs;
CREATE INDEX tabs_order ON tabs("order");
CREATE INDEX tabs_window ON tabs(window_key, "order");
