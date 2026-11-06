-- P219: quick-command collections become rows (api_collections' shape); scripts reference one by id.
CREATE TABLE custom_script_collections (
  id         TEXT PRIMARY KEY,
  name       TEXT NOT NULL,
  sort_order INTEGER NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
INSERT INTO custom_script_collections (id, name, sort_order, created_at, updated_at)
SELECT lower(hex(randomblob(16))), collection, ROW_NUMBER() OVER (ORDER BY collection) - 1,
       strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
  FROM (SELECT DISTINCT collection FROM custom_scripts WHERE collection <> '');
ALTER TABLE custom_scripts ADD COLUMN collection_id TEXT REFERENCES custom_script_collections(id) ON DELETE CASCADE;
UPDATE custom_scripts
   SET collection_id = (SELECT c.id FROM custom_script_collections c WHERE c.name = custom_scripts.collection)
 WHERE collection <> '';
ALTER TABLE custom_scripts DROP COLUMN collection;
CREATE INDEX custom_scripts_collection ON custom_scripts(collection_id);
