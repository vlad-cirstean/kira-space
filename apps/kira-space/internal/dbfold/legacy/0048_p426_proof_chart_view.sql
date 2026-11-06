-- P426: proof steps may use the 'chart' view. SQLite cannot alter a CHECK, so both tables are
-- rebuilt. Children first (view state, then steps) so no cascade ever fires; renaming the new
-- steps table rewrites the new view-state table's foreign key.
CREATE TABLE proof_steps_new (
  session_id TEXT NOT NULL,
  rev INTEGER NOT NULL,
  ordinal INTEGER NOT NULL,
  description TEXT NOT NULL,
  query TEXT NOT NULL DEFAULT '',
  language TEXT NOT NULL DEFAULT 'text',
  source TEXT NOT NULL DEFAULT '',
  view TEXT NOT NULL CHECK (view IN ('table','document','keyvalue','chart')),
  payload BLOB NOT NULL,
  payload_bytes INTEGER NOT NULL,
  diff TEXT NOT NULL DEFAULT 'added',
  origin_ordinal INTEGER,
  PRIMARY KEY (session_id, rev, ordinal),
  FOREIGN KEY (session_id, rev) REFERENCES proof_revisions(session_id, rev) ON DELETE CASCADE
);
INSERT INTO proof_steps_new SELECT session_id, rev, ordinal, description, query, language, source, view, payload, payload_bytes, diff, origin_ordinal FROM proof_steps;

CREATE TABLE proof_view_state_new (
  session_id TEXT NOT NULL,
  rev INTEGER NOT NULL,
  ordinal INTEGER NOT NULL,
  state TEXT NOT NULL,
  updated_at INTEGER NOT NULL,
  PRIMARY KEY (session_id, rev, ordinal),
  FOREIGN KEY (session_id, rev, ordinal) REFERENCES proof_steps_new(session_id, rev, ordinal) ON DELETE CASCADE
);
INSERT INTO proof_view_state_new SELECT session_id, rev, ordinal, state, updated_at FROM proof_view_state;

DROP TABLE proof_view_state;
DROP TABLE proof_steps;
ALTER TABLE proof_steps_new RENAME TO proof_steps;
ALTER TABLE proof_view_state_new RENAME TO proof_view_state;
