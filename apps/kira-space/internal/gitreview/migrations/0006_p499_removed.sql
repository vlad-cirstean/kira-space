CREATE TABLE review_removed (
  id              INTEGER PRIMARY KEY AUTOINCREMENT,
  session_id      INTEGER NOT NULL REFERENCES review_session(id) ON DELETE CASCADE,
  origin          TEXT    NOT NULL CHECK (origin IN ('commit', 'working')),
  path            TEXT    NOT NULL,
  reviewed_at     INTEGER NOT NULL,  -- source record's reviewed_at, unix millis: one review event
  reviewed_at_sha TEXT    NOT NULL,  -- commit: reviewed_at_sha; working: head_sha
  snapshot_key    TEXT    NOT NULL,  -- commit: blob_oid; working: content_hash
  content_kind    TEXT    NOT NULL,
  start_line      INTEGER NOT NULL,  -- snapshot coordinates, inclusive; 0 for a non-text whole-file row
  end_line        INTEGER NOT NULL,
  content_bytes   INTEGER NOT NULL,  -- UNCOMPRESSED length; 0 when content is NULL
  content         BLOB,              -- flate(lines); NULL for non-text and once dismissed
  removed_at      INTEGER NOT NULL,  -- unix millis of capture
  dismissed_at    INTEGER            -- NULL while shown
);
CREATE INDEX review_removed_source ON review_removed (session_id, origin, path, reviewed_at);

ALTER TABLE review_file ADD COLUMN removal_checked TEXT NOT NULL DEFAULT '';
ALTER TABLE review_working_file ADD COLUMN removal_checked TEXT NOT NULL DEFAULT '';
