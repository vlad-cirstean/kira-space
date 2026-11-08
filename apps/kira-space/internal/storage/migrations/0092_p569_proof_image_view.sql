-- Widen proof_steps.view to allow 'image'. SQLite cannot alter a CHECK, so rebuild children first
-- with foreign keys on: copy both tables, drop the child, then the parent, then rename.
CREATE TABLE proof_steps_new (
  session_id TEXT NOT NULL,
  rev INTEGER NOT NULL CHECK (rev >= 0),
  ordinal INTEGER NOT NULL CHECK (ordinal >= 0),
  view TEXT NOT NULL CHECK (view IN ('table', 'document', 'keyvalue', 'chart', 'diagram', 'image')),
  diff TEXT NOT NULL DEFAULT 'added' CHECK (diff IN ('added', 'unchanged', 'changed')),
  payload_bytes INTEGER NOT NULL CHECK (payload_bytes >= 0),
  origin_ordinal INTEGER CHECK (origin_ordinal IS NULL OR origin_ordinal >= 0),
  description TEXT NOT NULL,
  query TEXT NOT NULL DEFAULT '',
  language TEXT NOT NULL DEFAULT 'text',
  source TEXT NOT NULL DEFAULT '',
  payload BLOB NOT NULL,
  PRIMARY KEY (session_id, rev, ordinal),
  FOREIGN KEY (session_id, rev) REFERENCES proof_revisions (session_id, rev) ON DELETE CASCADE
) STRICT;

INSERT INTO proof_steps_new (session_id, rev, ordinal, view, diff, payload_bytes, origin_ordinal, description, query, language, source, payload)
SELECT session_id, rev, ordinal, view, diff, payload_bytes, origin_ordinal, description, query, language, source, payload FROM proof_steps;

CREATE TABLE proof_view_state_new (
  session_id TEXT NOT NULL,
  rev INTEGER NOT NULL CHECK (rev >= 0),
  ordinal INTEGER NOT NULL CHECK (ordinal >= 0),
  updated_at INTEGER NOT NULL CHECK (updated_at >= 0),
  state TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(state)),
  PRIMARY KEY (session_id, rev, ordinal),
  FOREIGN KEY (session_id, rev, ordinal) REFERENCES proof_steps_new (session_id, rev, ordinal) ON DELETE CASCADE
) STRICT;

INSERT INTO proof_view_state_new (session_id, rev, ordinal, updated_at, state)
SELECT session_id, rev, ordinal, updated_at, state FROM proof_view_state;

DROP TABLE proof_view_state;
DROP TABLE proof_steps;
ALTER TABLE proof_steps_new RENAME TO proof_steps;
ALTER TABLE proof_view_state_new RENAME TO proof_view_state;
