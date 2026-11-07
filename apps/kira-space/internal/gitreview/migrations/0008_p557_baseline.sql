-- Kira Space v0.3.1 review.db baseline (schema_version 8).

CREATE TABLE review_session (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  repo_id      TEXT    NOT NULL,
  branch       TEXT    NOT NULL,
  created_at   INTEGER NOT NULL,          -- unix millis
  last_used_at INTEGER NOT NULL, pinned INTEGER NOT NULL DEFAULT 0,          -- unix millis; the reaper's own clock (D11)
  UNIQUE (repo_id, branch)
);

CREATE TABLE review_file (
  session_id      INTEGER NOT NULL REFERENCES review_session(id) ON DELETE CASCADE,
  path            TEXT    NOT NULL,
  state           TEXT    NOT NULL,       -- 'full' | 'partial'
  reviewed_at_sha TEXT    NOT NULL,       -- the commit the snapshot was taken at
  reviewed_at     INTEGER NOT NULL,       -- unix millis
  blob_oid        TEXT    NOT NULL,       -- git's own oid; '' when content_kind = 'absent'
  content_kind    TEXT    NOT NULL,       -- 'text' | 'binary' | 'tooLarge' | 'absent'
  content_bytes   INTEGER NOT NULL,       -- UNCOMPRESSED length; the decoder's own length check
  line_count      INTEGER NOT NULL,       -- text only; 0 otherwise
  content         BLOB, removal_checked TEXT NOT NULL DEFAULT '',                   -- flate(raw); NULL unless content_kind = 'text'
  PRIMARY KEY (session_id, path)
);

CREATE TABLE review_range (
  session_id INTEGER NOT NULL,
  path       TEXT    NOT NULL,
  start_line INTEGER NOT NULL,            -- 1-based, inclusive, in the SNAPSHOT's coordinates
  end_line   INTEGER NOT NULL,            -- inclusive
  PRIMARY KEY (session_id, path, start_line),
  FOREIGN KEY (session_id, path) REFERENCES review_file(session_id, path) ON DELETE CASCADE
);

CREATE TABLE review_working_file (
  session_id    INTEGER NOT NULL REFERENCES review_session(id) ON DELETE CASCADE,
  path          TEXT    NOT NULL,
  state         TEXT    NOT NULL,   -- 'full' | 'partial'
  head_sha      TEXT    NOT NULL,   -- worktree HEAD at mark time; '' for an unborn HEAD
  reviewed_at   INTEGER NOT NULL,   -- unix millis
  content_hash  TEXT    NOT NULL,   -- sha256 hex of the worktree bytes; '' when absent
  content_kind  TEXT    NOT NULL,   -- 'text' | 'binary' | 'tooLarge' | 'absent'
  content_bytes INTEGER NOT NULL,
  line_count    INTEGER NOT NULL,
  content       BLOB, removal_checked TEXT NOT NULL DEFAULT '',               -- flate(raw); NULL unless text
  PRIMARY KEY (session_id, path)
);

CREATE TABLE review_working_range (
  session_id INTEGER NOT NULL,
  path       TEXT    NOT NULL,
  start_line INTEGER NOT NULL,
  end_line   INTEGER NOT NULL,
  PRIMARY KEY (session_id, path, start_line),
  FOREIGN KEY (session_id, path) REFERENCES review_working_file(session_id, path) ON DELETE CASCADE
);

CREATE TABLE "review_comment" (
  id              TEXT    PRIMARY KEY,
  session_id      INTEGER NOT NULL REFERENCES review_session(id) ON DELETE CASCADE,
  path            TEXT    NOT NULL,
  start_line      INTEGER NOT NULL,
  end_line        INTEGER NOT NULL,
  body            TEXT    NOT NULL,
  anchor_sha      TEXT    NOT NULL,
  anchor_blob_oid TEXT    NOT NULL,
  created_at      INTEGER NOT NULL,
  kind            TEXT    NOT NULL DEFAULT 'change' CHECK (kind IN ('change', 'question')),
  state           TEXT    NOT NULL DEFAULT 'open' CHECK (state IN ('open', 'addressed')),
  reply           TEXT    NOT NULL DEFAULT '',
  replied_at      INTEGER,
  sent_at         INTEGER,
  origin          TEXT    NOT NULL DEFAULT 'commit' CHECK (origin IN ('commit', 'working')),
  anchor_content  BLOB
);

CREATE TABLE "review_removed" (
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

CREATE INDEX review_session_last_used ON review_session (last_used_at);

CREATE INDEX review_comment_session_path ON review_comment (session_id, path, start_line);

CREATE INDEX review_removed_source ON review_removed (session_id, origin, path, reviewed_at);
