-- P4 D2: collections live in this app's own database (docs/HISTORY.md (v1.2)'s P4 row), not as Postman
-- files on disk. A collection is a *normalized* folder/request tree — Postman's `item` is an
-- ordered array, so `sort_order` is data, not presentation — plus one opaque `origin_json` column
-- per row holding the original Postman object verbatim, which is what makes an export re-emit
-- everything this app does not model (auth, scripts, variables, saved examples) unchanged (D5).
CREATE TABLE http_collections (
  id          TEXT PRIMARY KEY,
  name        TEXT NOT NULL,
  sort_order  INTEGER NOT NULL,
  -- The whole original collection object, minus its `item` array (those are rows below) and minus
  -- `info.name` (the column above). '{}' for a collection created in this app.
  origin_json TEXT NOT NULL DEFAULT '{}',
  created_at  TEXT NOT NULL,
  updated_at  TEXT NOT NULL
);

CREATE TABLE http_items (
  id            TEXT PRIMARY KEY,
  collection_id TEXT NOT NULL REFERENCES http_collections(id) ON DELETE CASCADE,
  -- NULL = a direct child of the collection root. The self-reference cascades for real: db.go's
  -- DSN sets _foreign_keys=1 on every connection (F9), so deleting a folder deletes its subtree at
  -- any depth in one statement.
  parent_id     TEXT REFERENCES http_items(id) ON DELETE CASCADE,
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
);

CREATE INDEX http_items_tree ON http_items(collection_id, parent_id, sort_order);
