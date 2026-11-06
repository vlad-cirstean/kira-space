-- P472: runs are config files. custom_scripts becomes the projection of <home>/runs/*.yaml: collection is
-- a name, config_json the last good file value, the schedule target a state column. Collections are
-- order rows keyed by name. SQLite refuses DROP COLUMN on the collection_id foreign key, so both tables are rebuilt.
CREATE TABLE custom_scripts_new (
  id                 TEXT PRIMARY KEY,
  name               TEXT NOT NULL,
  command            TEXT NOT NULL,
  working_dir        TEXT NOT NULL DEFAULT '',
  dir_mode           TEXT NOT NULL DEFAULT 'kira',
  color              TEXT NOT NULL DEFAULT 'none',
  collection         TEXT NOT NULL DEFAULT '',
  sort_order         INTEGER NOT NULL,
  created_at         INTEGER NOT NULL,
  updated_at         INTEGER NOT NULL,
  kind               TEXT NOT NULL DEFAULT 'script' CHECK (kind IN ('script', 'smart')),
  params_json        TEXT NOT NULL DEFAULT '[]',
  smart_json         TEXT NOT NULL DEFAULT '',
  use_ade_dir        INTEGER NOT NULL DEFAULT 1 CHECK (use_ade_dir IN (0, 1)),
  schedule_json      TEXT NOT NULL DEFAULT '',
  schedule_task_id   TEXT NOT NULL DEFAULT '',
  schedule_branch_id TEXT NOT NULL DEFAULT '',
  config_json        TEXT NOT NULL DEFAULT ''
);
INSERT INTO custom_scripts_new (id, name, command, working_dir, dir_mode, color, collection, sort_order, created_at, updated_at,
                                kind, params_json, smart_json, use_ade_dir, schedule_json, schedule_task_id, schedule_branch_id)
SELECT s.id, s.name, s.command, s.working_dir, s.dir_mode, s.color,
       COALESCE((SELECT c.name FROM custom_script_collections c WHERE c.id = s.collection_id), ''),
       s.sort_order, COALESCE(CAST(strftime('%s', s.created_at) AS INTEGER) * 1000, 0), COALESCE(CAST(strftime('%s', s.updated_at) AS INTEGER) * 1000, 0), s.kind, s.params_json, s.smart_json, s.use_ade_dir, s.schedule_json,
       CASE WHEN s.schedule_json = '' THEN '' ELSE COALESCE(json_extract(s.schedule_json, '$.taskId'), '') END,
       CASE WHEN s.schedule_json = '' THEN '' ELSE COALESCE(json_extract(s.schedule_json, '$.branchId'), '') END
  FROM custom_scripts s;
DROP TABLE custom_scripts;
ALTER TABLE custom_scripts_new RENAME TO custom_scripts;

CREATE TABLE custom_script_collections_new (
  name       TEXT PRIMARY KEY,
  sort_order INTEGER NOT NULL
) WITHOUT ROWID;
INSERT INTO custom_script_collections_new (name, sort_order)
SELECT name, MIN(sort_order) FROM custom_script_collections GROUP BY name;
DROP TABLE custom_script_collections;
ALTER TABLE custom_script_collections_new RENAME TO custom_script_collections;
