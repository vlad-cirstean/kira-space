-- P420 (instants are unix milliseconds): validation proof sessions an MCP agent pushes and the user reviews in a window. A session
-- has a current revision and at most one pending (agent-proposed) revision; steps carry their data
-- as a validated JSON payload. Children cascade (foreign_keys is on for every connection).
CREATE TABLE proof_sessions (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  name_key TEXT NOT NULL UNIQUE,
  state TEXT NOT NULL CHECK (state IN ('open','closed')),
  current_rev INTEGER,
  pending_rev INTEGER,
  summary TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  closed_at INTEGER,
  seen_at INTEGER,
  updated_at INTEGER NOT NULL
);
CREATE INDEX proof_sessions_closed ON proof_sessions(closed_at);

CREATE TABLE proof_revisions (
  session_id TEXT NOT NULL REFERENCES proof_sessions(id) ON DELETE CASCADE,
  rev INTEGER NOT NULL,
  draft_id TEXT UNIQUE,
  reason TEXT NOT NULL DEFAULT '',
  state TEXT NOT NULL CHECK (state IN ('draft','current','pending')),
  created_at INTEGER NOT NULL,
  closed_at INTEGER,
  PRIMARY KEY (session_id, rev)
);

CREATE TABLE proof_steps (
  session_id TEXT NOT NULL,
  rev INTEGER NOT NULL,
  ordinal INTEGER NOT NULL,
  description TEXT NOT NULL,
  query TEXT NOT NULL DEFAULT '',
  language TEXT NOT NULL DEFAULT 'text',
  source TEXT NOT NULL DEFAULT '',
  view TEXT NOT NULL CHECK (view IN ('table','document','keyvalue')),
  payload BLOB NOT NULL,
  payload_bytes INTEGER NOT NULL,
  diff TEXT NOT NULL DEFAULT 'added',
  origin_ordinal INTEGER,
  PRIMARY KEY (session_id, rev, ordinal),
  FOREIGN KEY (session_id, rev) REFERENCES proof_revisions(session_id, rev) ON DELETE CASCADE
);

CREATE TABLE proof_view_state (
  session_id TEXT NOT NULL,
  rev INTEGER NOT NULL,
  ordinal INTEGER NOT NULL,
  state TEXT NOT NULL,
  updated_at INTEGER NOT NULL,
  PRIMARY KEY (session_id, rev, ordinal),
  FOREIGN KEY (session_id, rev, ordinal) REFERENCES proof_steps(session_id, rev, ordinal) ON DELETE CASCADE
);
