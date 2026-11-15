-- P643: Details content moves to files under <home>/tasks/<task>/. task_files indexes each file
-- (change detection and review state), task_file_intents journals file-then-DB writes, and
-- task_files_migration marks the one-time per-task export. ade_task_handovers.rel_path names the file
-- a handover row caches.
CREATE TABLE task_files (
  task_id TEXT NOT NULL REFERENCES ade_tasks (id) ON DELETE CASCADE,
  rel_path TEXT NOT NULL CHECK (rel_path <> ''),
  kind TEXT NOT NULL,
  size INTEGER NOT NULL DEFAULT 0 CHECK (size >= 0),
  version TEXT NOT NULL DEFAULT '',
  sha256 TEXT NOT NULL DEFAULT '',
  accepted_sha256 TEXT NOT NULL DEFAULT '',
  accepted_by TEXT NOT NULL DEFAULT '',
  accepted_at INTEGER NOT NULL DEFAULT 0 CHECK (accepted_at >= 0),
  review TEXT NOT NULL DEFAULT 'accepted' CHECK (review IN ('accepted', 'pending', 'invalid')),
  problem TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (task_id, rel_path)
) STRICT;

CREATE INDEX task_files_review ON task_files (task_id, review);

CREATE TABLE task_file_intents (
  task_id TEXT NOT NULL REFERENCES ade_tasks (id) ON DELETE CASCADE,
  rel_path TEXT NOT NULL CHECK (rel_path <> ''),
  sha256 TEXT NOT NULL,
  writer TEXT NOT NULL,
  created_at INTEGER NOT NULL CHECK (created_at >= 0),
  PRIMARY KEY (task_id, rel_path)
) STRICT;

CREATE TABLE task_files_migration (
  task_id TEXT NOT NULL REFERENCES ade_tasks (id) ON DELETE CASCADE,
  part TEXT NOT NULL CHECK (part <> ''),
  state TEXT NOT NULL CHECK (state IN ('running', 'done', 'failed')),
  done_at INTEGER NOT NULL DEFAULT 0 CHECK (done_at >= 0),
  verify_json TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (task_id, part)
) STRICT;

ALTER TABLE ade_task_handovers ADD COLUMN rel_path TEXT NOT NULL DEFAULT '';
