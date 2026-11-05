-- P523 F-R1-5: review_comment.id crosses the bridge, so it is a UUIDv7 string, not an autoincrement
-- integer. Old rows get the v7-shaped id 00000000-0000-7000-8000-<old id as 12 hex>, which
-- model.LegacyCommentID recomputes so round specs stored with integer ids still resolve. Nothing references the table.
CREATE TABLE review_comment_new (
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

INSERT INTO review_comment_new
  (id, session_id, path, start_line, end_line, body, anchor_sha, anchor_blob_oid, created_at, kind, state, reply, replied_at, sent_at, origin, anchor_content)
SELECT
  printf('00000000-0000-7000-8000-%012x', c.id),
  c.session_id, c.path, c.start_line, c.end_line, c.body, c.anchor_sha, c.anchor_blob_oid, c.created_at,
  c.kind, c.state, c.reply, c.replied_at, c.sent_at, c.origin, c.anchor_content
FROM review_comment c
ORDER BY c.id;

DROP TABLE review_comment;
ALTER TABLE review_comment_new RENAME TO review_comment;
CREATE INDEX review_comment_session_path ON review_comment (session_id, path, start_line);
