-- kira:foreign-keys off
-- P552 proof tables. Rebuilds each table STRICT with checked types; rows that violate a new
-- constraint are repaired or removed and recorded in schema_quarantine.

-- proof_sessions
CREATE TABLE proof_sessions__n (
  id TEXT NOT NULL PRIMARY KEY CHECK (id <> ''),
  state TEXT NOT NULL CHECK (state IN ('open', 'closed')),
  current_rev INTEGER CHECK (current_rev IS NULL OR current_rev >= 0),
  pending_rev INTEGER CHECK (pending_rev IS NULL OR pending_rev >= 0),
  created_at INTEGER NOT NULL CHECK (created_at >= 0),
  closed_at INTEGER CHECK (closed_at IS NULL OR closed_at >= 0),
  seen_at INTEGER CHECK (seen_at IS NULL OR seen_at >= 0),
  updated_at INTEGER NOT NULL CHECK (updated_at >= 0),
  name TEXT NOT NULL,
  name_key TEXT NOT NULL UNIQUE,
  summary TEXT NOT NULL DEFAULT ''
) STRICT;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'proof_sessions', json_object('id', "id", 'name', "name", 'name_key', "name_key", 'state', "state", 'current_rev', "current_rev", 'pending_rev', "pending_rev", 'summary', "summary", 'created_at', "created_at", 'closed_at', "closed_at", 'seen_at', "seen_at", 'updated_at', "updated_at"), 'empty id', 'removed', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM proof_sessions WHERE id = '';
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'proof_sessions', json_object('id', "id", 'name', "name", 'name_key', "name_key", 'state', "state", 'current_rev', "current_rev", 'pending_rev', "pending_rev", 'summary', "summary", 'created_at', "created_at", 'closed_at', "closed_at", 'seen_at', "seen_at", 'updated_at', "updated_at"), 'invalid state', 'removed', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM proof_sessions WHERE state NOT IN ('open', 'closed');
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'proof_sessions', json_object('id', "id", 'name', "name", 'name_key', "name_key", 'state', "state", 'current_rev', "current_rev", 'pending_rev', "pending_rev", 'summary', "summary", 'created_at', "created_at", 'closed_at', "closed_at", 'seen_at', "seen_at", 'updated_at', "updated_at"), 'out-of-range current_rev', 'repaired', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM proof_sessions WHERE "current_rev" < 0;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'proof_sessions', json_object('id', "id", 'name', "name", 'name_key', "name_key", 'state', "state", 'current_rev', "current_rev", 'pending_rev', "pending_rev", 'summary', "summary", 'created_at', "created_at", 'closed_at', "closed_at", 'seen_at', "seen_at", 'updated_at', "updated_at"), 'out-of-range pending_rev', 'repaired', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM proof_sessions WHERE "pending_rev" < 0;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'proof_sessions', json_object('id', "id", 'name', "name", 'name_key', "name_key", 'state', "state", 'current_rev', "current_rev", 'pending_rev', "pending_rev", 'summary', "summary", 'created_at', "created_at", 'closed_at', "closed_at", 'seen_at', "seen_at", 'updated_at', "updated_at"), 'negative created_at', 'repaired', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM proof_sessions WHERE created_at < 0;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'proof_sessions', json_object('id', "id", 'name', "name", 'name_key', "name_key", 'state', "state", 'current_rev', "current_rev", 'pending_rev', "pending_rev", 'summary', "summary", 'created_at', "created_at", 'closed_at', "closed_at", 'seen_at', "seen_at", 'updated_at', "updated_at"), 'negative closed_at', 'repaired', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM proof_sessions WHERE closed_at < 0;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'proof_sessions', json_object('id', "id", 'name', "name", 'name_key', "name_key", 'state', "state", 'current_rev', "current_rev", 'pending_rev', "pending_rev", 'summary', "summary", 'created_at', "created_at", 'closed_at', "closed_at", 'seen_at', "seen_at", 'updated_at', "updated_at"), 'negative seen_at', 'repaired', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM proof_sessions WHERE seen_at < 0;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'proof_sessions', json_object('id', "id", 'name', "name", 'name_key', "name_key", 'state', "state", 'current_rev', "current_rev", 'pending_rev', "pending_rev", 'summary', "summary", 'created_at', "created_at", 'closed_at', "closed_at", 'seen_at', "seen_at", 'updated_at', "updated_at"), 'negative updated_at', 'repaired', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM proof_sessions WHERE updated_at < 0;
INSERT INTO proof_sessions__n (rowid, id, state, current_rev, pending_rev, created_at, closed_at, seen_at, updated_at, name, name_key, summary)
  SELECT rowid, id, state, MAX(0, "current_rev"), MAX(0, "pending_rev"), MAX(0, created_at), MAX(0, closed_at), MAX(0, seen_at), MAX(0, updated_at), name, name_key, summary
  FROM proof_sessions WHERE NOT ((id = '')
    OR (state NOT IN ('open', 'closed')));
DROP TABLE proof_sessions;
ALTER TABLE proof_sessions__n RENAME TO proof_sessions;
CREATE INDEX proof_sessions_closed ON proof_sessions (closed_at);

-- proof_revisions
CREATE TABLE proof_revisions__n (
  session_id TEXT NOT NULL REFERENCES proof_sessions (id) ON DELETE CASCADE,
  rev INTEGER NOT NULL CHECK (rev >= 0),
  state TEXT NOT NULL CHECK (state IN ('draft', 'current', 'pending')),
  created_at INTEGER NOT NULL CHECK (created_at >= 0),
  closed_at INTEGER CHECK (closed_at IS NULL OR closed_at >= 0),
  draft_id TEXT UNIQUE,
  reason TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (session_id, rev)
) STRICT;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'proof_revisions', json_object('session_id', "session_id", 'rev', "rev", 'draft_id', "draft_id", 'reason', "reason", 'state', "state", 'created_at', "created_at", 'closed_at', "closed_at"), 'orphan session_id', 'removed', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM proof_revisions WHERE NOT EXISTS (SELECT 1 FROM proof_sessions p WHERE p.id = proof_revisions."session_id");
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'proof_revisions', json_object('session_id', "session_id", 'rev', "rev", 'draft_id', "draft_id", 'reason', "reason", 'state', "state", 'created_at', "created_at", 'closed_at', "closed_at"), 'out-of-range rev', 'repaired', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM proof_revisions WHERE "rev" < 0;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'proof_revisions', json_object('session_id', "session_id", 'rev', "rev", 'draft_id', "draft_id", 'reason', "reason", 'state', "state", 'created_at', "created_at", 'closed_at', "closed_at"), 'invalid state', 'removed', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM proof_revisions WHERE state NOT IN ('draft', 'current', 'pending');
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'proof_revisions', json_object('session_id', "session_id", 'rev', "rev", 'draft_id', "draft_id", 'reason', "reason", 'state', "state", 'created_at', "created_at", 'closed_at', "closed_at"), 'negative created_at', 'repaired', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM proof_revisions WHERE created_at < 0;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'proof_revisions', json_object('session_id', "session_id", 'rev', "rev", 'draft_id', "draft_id", 'reason', "reason", 'state', "state", 'created_at', "created_at", 'closed_at', "closed_at"), 'negative closed_at', 'repaired', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM proof_revisions WHERE closed_at < 0;
INSERT INTO proof_revisions__n (rowid, session_id, rev, state, created_at, closed_at, draft_id, reason)
  SELECT rowid, session_id, MAX(0, COALESCE("rev", 0)), state, MAX(0, created_at), MAX(0, closed_at), draft_id, reason
  FROM proof_revisions WHERE NOT ((NOT EXISTS (SELECT 1 FROM proof_sessions p WHERE p.id = proof_revisions."session_id"))
    OR (state NOT IN ('draft', 'current', 'pending')));
DROP TABLE proof_revisions;
ALTER TABLE proof_revisions__n RENAME TO proof_revisions;

-- proof_steps
CREATE TABLE proof_steps__n (
  session_id TEXT NOT NULL,
  rev INTEGER NOT NULL CHECK (rev >= 0),
  ordinal INTEGER NOT NULL CHECK (ordinal >= 0),
  view TEXT NOT NULL CHECK (view IN ('table', 'document', 'keyvalue', 'chart', 'diagram')),
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
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'proof_steps', json_object('session_id', "session_id", 'rev', "rev", 'ordinal', "ordinal", 'description', "description", 'query', "query", 'language', "language", 'source', "source", 'view', "view", 'payload_bytes', "payload_bytes", 'diff', "diff", 'origin_ordinal', "origin_ordinal"), 'out-of-range rev', 'repaired', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM proof_steps WHERE "rev" < 0;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'proof_steps', json_object('session_id', "session_id", 'rev', "rev", 'ordinal', "ordinal", 'description', "description", 'query', "query", 'language', "language", 'source', "source", 'view', "view", 'payload_bytes', "payload_bytes", 'diff', "diff", 'origin_ordinal', "origin_ordinal"), 'out-of-range ordinal', 'repaired', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM proof_steps WHERE "ordinal" < 0;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'proof_steps', json_object('session_id', "session_id", 'rev', "rev", 'ordinal', "ordinal", 'description', "description", 'query', "query", 'language', "language", 'source', "source", 'view', "view", 'payload_bytes', "payload_bytes", 'diff', "diff", 'origin_ordinal', "origin_ordinal"), 'invalid view', 'removed', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM proof_steps WHERE view NOT IN ('table', 'document', 'keyvalue', 'chart', 'diagram');
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'proof_steps', json_object('session_id', "session_id", 'rev', "rev", 'ordinal', "ordinal", 'description', "description", 'query', "query", 'language', "language", 'source', "source", 'view', "view", 'payload_bytes', "payload_bytes", 'diff', "diff", 'origin_ordinal', "origin_ordinal"), 'invalid diff', 'removed', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM proof_steps WHERE diff NOT IN ('added', 'unchanged', 'changed');
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'proof_steps', json_object('session_id', "session_id", 'rev', "rev", 'ordinal', "ordinal", 'description', "description", 'query', "query", 'language', "language", 'source', "source", 'view', "view", 'payload_bytes', "payload_bytes", 'diff', "diff", 'origin_ordinal', "origin_ordinal"), 'out-of-range payload_bytes', 'repaired', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM proof_steps WHERE "payload_bytes" < 0;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'proof_steps', json_object('session_id', "session_id", 'rev', "rev", 'ordinal', "ordinal", 'description', "description", 'query', "query", 'language', "language", 'source', "source", 'view', "view", 'payload_bytes', "payload_bytes", 'diff', "diff", 'origin_ordinal', "origin_ordinal"), 'out-of-range origin_ordinal', 'repaired', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM proof_steps WHERE "origin_ordinal" < 0;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'proof_steps', json_object('session_id', "session_id", 'rev', "rev", 'ordinal', "ordinal", 'description', "description", 'query', "query", 'language', "language", 'source', "source", 'view', "view", 'payload_bytes', "payload_bytes", 'diff', "diff", 'origin_ordinal', "origin_ordinal"), 'orphan revision', 'removed', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM proof_steps WHERE NOT EXISTS (SELECT 1 FROM proof_revisions r WHERE r.session_id = proof_steps.session_id AND r.rev = proof_steps.rev);
INSERT INTO proof_steps__n (rowid, session_id, rev, ordinal, view, diff, payload_bytes, origin_ordinal, description, query, language, source, payload)
  SELECT rowid, session_id, MAX(0, COALESCE("rev", 0)), MAX(0, COALESCE("ordinal", 0)), view, diff, MAX(0, COALESCE("payload_bytes", 0)), MAX(0, "origin_ordinal"), description, query, language, source, payload
  FROM proof_steps WHERE NOT ((view NOT IN ('table', 'document', 'keyvalue', 'chart', 'diagram'))
    OR (diff NOT IN ('added', 'unchanged', 'changed'))
    OR (NOT EXISTS (SELECT 1 FROM proof_revisions r WHERE r.session_id = proof_steps.session_id AND r.rev = proof_steps.rev)));
DROP TABLE proof_steps;
ALTER TABLE proof_steps__n RENAME TO proof_steps;

-- proof_view_state
CREATE TABLE proof_view_state__n (
  session_id TEXT NOT NULL,
  rev INTEGER NOT NULL CHECK (rev >= 0),
  ordinal INTEGER NOT NULL CHECK (ordinal >= 0),
  updated_at INTEGER NOT NULL CHECK (updated_at >= 0),
  state TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(state)),
  PRIMARY KEY (session_id, rev, ordinal),
  FOREIGN KEY (session_id, rev, ordinal) REFERENCES proof_steps (session_id, rev, ordinal) ON DELETE CASCADE
) STRICT;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'proof_view_state', json_object('session_id', "session_id", 'rev', "rev", 'ordinal', "ordinal", 'state', "state", 'updated_at', "updated_at"), 'out-of-range rev', 'repaired', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM proof_view_state WHERE "rev" < 0;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'proof_view_state', json_object('session_id', "session_id", 'rev', "rev", 'ordinal', "ordinal", 'state', "state", 'updated_at', "updated_at"), 'out-of-range ordinal', 'repaired', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM proof_view_state WHERE "ordinal" < 0;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'proof_view_state', json_object('session_id', "session_id", 'rev', "rev", 'ordinal', "ordinal", 'state', "state", 'updated_at', "updated_at"), 'negative updated_at', 'repaired', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM proof_view_state WHERE updated_at < 0;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'proof_view_state', json_object('session_id', "session_id", 'rev', "rev", 'ordinal', "ordinal", 'state', "state", 'updated_at', "updated_at"), 'invalid JSON state', 'repaired', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM proof_view_state WHERE NOT json_valid(state);
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'proof_view_state', json_object('session_id', "session_id", 'rev', "rev", 'ordinal', "ordinal", 'state', "state", 'updated_at', "updated_at"), 'orphan step', 'removed', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM proof_view_state WHERE NOT EXISTS (SELECT 1 FROM proof_steps s WHERE s.session_id = proof_view_state.session_id AND s.rev = proof_view_state.rev AND s.ordinal = proof_view_state.ordinal);
INSERT INTO proof_view_state__n (rowid, session_id, rev, ordinal, updated_at, state)
  SELECT rowid, session_id, MAX(0, COALESCE("rev", 0)), MAX(0, COALESCE("ordinal", 0)), MAX(0, updated_at), CASE WHEN json_valid(state) THEN state ELSE '{}' END
  FROM proof_view_state WHERE NOT ((NOT EXISTS (SELECT 1 FROM proof_steps s WHERE s.session_id = proof_view_state.session_id AND s.rev = proof_view_state.rev AND s.ordinal = proof_view_state.ordinal)));
DROP TABLE proof_view_state;
ALTER TABLE proof_view_state__n RENAME TO proof_view_state;

-- proof_task_links
CREATE TABLE proof_task_links__n (
  session_id TEXT NOT NULL PRIMARY KEY REFERENCES proof_sessions (id) ON DELETE CASCADE,
  task_id TEXT NOT NULL REFERENCES ade_tasks (id) ON DELETE CASCADE,
  source TEXT NOT NULL CHECK (source IN ('agent', 'user')),
  linked_at INTEGER NOT NULL CHECK (linked_at >= 0),
  kira_session_id TEXT NOT NULL DEFAULT '',
  step_id TEXT NOT NULL DEFAULT '',
  step_name TEXT NOT NULL DEFAULT ''
) STRICT;
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'proof_task_links', json_object('session_id', "session_id", 'task_id', "task_id", 'source', "source", 'kira_session_id', "kira_session_id", 'step_id', "step_id", 'step_name', "step_name", 'linked_at', "linked_at"), 'orphan session_id', 'removed', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM proof_task_links WHERE NOT EXISTS (SELECT 1 FROM proof_sessions p WHERE p.id = proof_task_links."session_id");
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'proof_task_links', json_object('session_id', "session_id", 'task_id', "task_id", 'source', "source", 'kira_session_id', "kira_session_id", 'step_id', "step_id", 'step_name', "step_name", 'linked_at', "linked_at"), 'orphan task_id', 'removed', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM proof_task_links WHERE NOT EXISTS (SELECT 1 FROM ade_tasks p WHERE p.id = proof_task_links."task_id");
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'proof_task_links', json_object('session_id', "session_id", 'task_id', "task_id", 'source', "source", 'kira_session_id', "kira_session_id", 'step_id', "step_id", 'step_name', "step_name", 'linked_at', "linked_at"), 'invalid source', 'removed', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM proof_task_links WHERE source NOT IN ('agent', 'user');
INSERT INTO schema_quarantine (table_name, row_json, reason, action, migration, recorded_at)
  SELECT 'proof_task_links', json_object('session_id', "session_id", 'task_id', "task_id", 'source', "source", 'kira_session_id', "kira_session_id", 'step_id', "step_id", 'step_name', "step_name", 'linked_at', "linked_at"), 'negative linked_at', 'repaired', 84, CAST(unixepoch('now','subsec')*1000 AS INTEGER)
  FROM proof_task_links WHERE linked_at < 0;
INSERT INTO proof_task_links__n (rowid, session_id, task_id, source, linked_at, kira_session_id, step_id, step_name)
  SELECT rowid, session_id, task_id, source, MAX(0, linked_at), kira_session_id, step_id, step_name
  FROM proof_task_links WHERE NOT ((NOT EXISTS (SELECT 1 FROM proof_sessions p WHERE p.id = proof_task_links."session_id"))
    OR (NOT EXISTS (SELECT 1 FROM ade_tasks p WHERE p.id = proof_task_links."task_id"))
    OR (source NOT IN ('agent', 'user')));
DROP TABLE proof_task_links;
ALTER TABLE proof_task_links__n RENAME TO proof_task_links;
CREATE INDEX proof_task_links_task ON proof_task_links (task_id);
