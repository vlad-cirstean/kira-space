-- P523 F-R2-3: review_removed.id crosses the bridge (dismiss), so it is a UUIDv7 string, not an autoincrement
-- integer. Old rows get 00000000-0000-7000-8000-<old id as 12 hex>; nothing references the table.
CREATE TABLE review_removed_new (
  id              TEXT    PRIMARY KEY,
  session_id      INTEGER NOT NULL REFERENCES review_session(id) ON DELETE CASCADE,
  origin          TEXT    NOT NULL CHECK (origin IN ('commit', 'working')),
  path            TEXT    NOT NULL,
  reviewed_at     INTEGER NOT NULL,
  reviewed_at_sha TEXT    NOT NULL,
  snapshot_key    TEXT    NOT NULL,
  content_kind    TEXT    NOT NULL,
  start_line      INTEGER NOT NULL,
  end_line        INTEGER NOT NULL,
  content_bytes   INTEGER NOT NULL,
  content         BLOB,
  removed_at      INTEGER NOT NULL,
  dismissed_at    INTEGER
);

INSERT INTO review_removed_new
  (id, session_id, origin, path, reviewed_at, reviewed_at_sha, snapshot_key, content_kind,
   start_line, end_line, content_bytes, content, removed_at, dismissed_at)
SELECT printf('00000000-0000-7000-8000-%012x', id), session_id, origin, path, reviewed_at, reviewed_at_sha,
       snapshot_key, content_kind, start_line, end_line, content_bytes, content, removed_at, dismissed_at
FROM review_removed ORDER BY id;

DROP TABLE review_removed;
ALTER TABLE review_removed_new RENAME TO review_removed;
CREATE INDEX review_removed_source ON review_removed (session_id, origin, path, reviewed_at);
