-- P751: acli and the issue tracker are gone. Drop the Jira issue snapshots, the tracking.issue setting
-- and the acli call history. ade_tracked_items rebuilds without the 'issue' kind; no table references it.
CREATE TABLE ade_tracked_items_new (
  task_id TEXT NOT NULL REFERENCES ade_tasks (id) ON DELETE CASCADE,
  item_key TEXT NOT NULL,
  kind TEXT NOT NULL CHECK (kind IN ('changeRequest', 'branch')),
  seen_at INTEGER NOT NULL CHECK (seen_at >= 0),
  observed_at INTEGER NOT NULL CHECK (observed_at >= 0),
  changed_at INTEGER NOT NULL DEFAULT 0 CHECK (changed_at >= 0),
  seen_json TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(seen_json)),
  current_json TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(current_json)),
  PRIMARY KEY (task_id, item_key)
) STRICT;

INSERT INTO ade_tracked_items_new (task_id, item_key, kind, seen_at, observed_at, changed_at, seen_json, current_json)
SELECT task_id, item_key, kind, seen_at, observed_at, changed_at, seen_json, current_json
FROM ade_tracked_items WHERE kind <> 'issue';

DROP TABLE ade_tracked_items;
ALTER TABLE ade_tracked_items_new RENAME TO ade_tracked_items;

DELETE FROM settings WHERE key = 'tracking.issue';
DELETE FROM op_log WHERE category = 'tracker';
