-- P510: script steps default to sandbox off. A started task's workflow snapshot carries the old
-- write default on its script steps, which the engine never applied; rewrite it to the off value so
-- the snapshot matches what runs. Other steps and the step order stay as they are.
UPDATE ade_tasks SET workflow_json = json_set(
  workflow_json,
  '$.steps',
  json((SELECT json_group_array(json(
      CASE WHEN json_extract(value, '$.kind') = 'script'
        THEN json_set(value, '$.sandbox', json('{"access":"off","read":[],"allow":[],"network":true}'))
        ELSE value END))
   FROM (SELECT value FROM json_each(ade_tasks.workflow_json, '$.steps') ORDER BY key))))
WHERE workflow_json IS NOT NULL AND workflow_json <> '' AND json_valid(workflow_json)
  AND json_type(workflow_json, '$.steps') = 'array';
