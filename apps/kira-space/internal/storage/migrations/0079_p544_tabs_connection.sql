-- P544: Studio tabs join the Space tabs table. A Studio tab carries a connection and no workspace.
-- Nothing references tabs, so the rebuild is safe with foreign keys on.
CREATE TABLE tabs_new (
  id            TEXT PRIMARY KEY,
  connection_id TEXT REFERENCES connections (id) ON DELETE CASCADE,
  path          TEXT NOT NULL,
  kind          TEXT NOT NULL,
  state_json    TEXT NOT NULL,
  "order"       INTEGER NOT NULL,
  active        INTEGER NOT NULL,
  window_key    TEXT NOT NULL REFERENCES windows (key) ON DELETE CASCADE,
  workspace_id  TEXT
);
INSERT INTO tabs_new (id, path, kind, state_json, "order", active, window_key, workspace_id)
SELECT id, path, kind, state_json, "order", active, window_key, workspace_id FROM tabs;
DROP TABLE tabs;
ALTER TABLE tabs_new RENAME TO tabs;
CREATE INDEX tabs_window ON tabs (window_key, "order");
CREATE INDEX tabs_order ON tabs ("order");
