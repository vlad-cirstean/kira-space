-- Drops state only pre-v0.3.1 builds wrote: numeric review comment ids, http tabs saved with
-- bodyMode 'json', the P552 repair log and the one-time fold and export markers.
UPDATE ade_runs
SET spec_json = json_set(
  spec_json, '$.commentIds',
  json((SELECT json_group_array(value) FROM json_each(ade_runs.spec_json, '$.commentIds') WHERE type = 'text'))
)
WHERE spec_json <> ''
  AND json_type(spec_json, '$.commentIds') = 'array'
  AND EXISTS (SELECT 1 FROM json_each(ade_runs.spec_json, '$.commentIds') WHERE type <> 'text');

DELETE FROM tabs WHERE json_extract(state_json, '$.bodyMode') = 'json';

DROP TABLE schema_quarantine;

DELETE FROM settings
WHERE key LIKE 'config.migrated.%' OR key IN ('homefold.p539', 'configfold.p540', 'dbfold.p544');
