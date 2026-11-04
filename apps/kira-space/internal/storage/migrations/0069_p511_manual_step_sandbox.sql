-- P511: manual session steps take a sandbox. An interactive session has no run row, so its sandbox
-- lives in its own table keyed by the session. An off session keeps no row.
CREATE TABLE ade_session_sandbox (
  session_id TEXT PRIMARY KEY REFERENCES ade_sessions (id) ON DELETE CASCADE,
  access TEXT NOT NULL CHECK (access IN ('write', 'read')),
  enforced INTEGER NOT NULL,
  reason TEXT NOT NULL DEFAULT '',
  policy_json TEXT NOT NULL,
  profile TEXT NOT NULL DEFAULT '',
  denial_count INTEGER NOT NULL DEFAULT 0,
  denials_json TEXT NOT NULL DEFAULT '[]'
);

-- Manual steps default to sandbox off. A started task's snapshot carries the old write default on
-- its manual steps, which the engine never applied; rewrite it to the off value so the snapshot
-- matches what runs. Other steps and the step order stay as they are.
UPDATE ade_tasks SET workflow_json = json_set(
  workflow_json,
  '$.steps',
  json((SELECT json_group_array(json(
      CASE WHEN json_extract(value, '$.kind') = 'manual'
        THEN json_set(value, '$.sandbox', json('{"access":"off","read":[],"allow":[],"network":true}'))
        ELSE value END))
   FROM (SELECT value FROM json_each(ade_tasks.workflow_json, '$.steps') ORDER BY key))))
WHERE workflow_json IS NOT NULL AND workflow_json <> '' AND json_valid(workflow_json)
  AND json_type(workflow_json, '$.steps') = 'array';
