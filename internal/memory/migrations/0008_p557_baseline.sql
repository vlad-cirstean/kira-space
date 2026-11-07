-- Kira Space v0.3.1 memory.db baseline (schema_version 8), without memory_legacy_ids.

CREATE VIRTUAL TABLE memories_fts USING fts5(
  fact, reason, keywords,
  content = 'memories', content_rowid = 'seq',
  tokenize = 'porter unicode61 remove_diacritics 2',
  prefix = '2 3'
);

CREATE TABLE memory_revision (id INTEGER PRIMARY KEY CHECK (id = 1), value INTEGER NOT NULL);

CREATE TABLE memories (
  seq              INTEGER PRIMARY KEY,
  id               TEXT NOT NULL UNIQUE,
  lineage_id       TEXT NOT NULL,
  version          INTEGER NOT NULL CHECK (version >= 1),
  fact             TEXT NOT NULL CHECK (length(fact) BETWEEN 1 AND 1000),
  reason           TEXT NOT NULL CHECK (length(reason) BETWEEN 1 AND 2000),
  keywords         TEXT NOT NULL DEFAULT '',
  origin           TEXT NOT NULL CHECK (origin IN ('user', 'agent', 'external')),
  source_tag       TEXT CHECK (source_tag IS NULL OR (length(source_tag) BETWEEN 1 AND 32 AND source_tag NOT GLOB '*[^a-z0-9-]*')),
  scope            TEXT NOT NULL CHECK (scope IN ('company', 'project', 'personal')),
  project          TEXT CHECK (project IS NULL OR (length(project) BETWEEN 1 AND 300
                     AND project = replace(replace(replace(project, ' ', ''), char(9), ''), char(10), ''))),
  session_id       TEXT CHECK (session_id IS NULL OR length(session_id) BETWEEN 1 AND 128),
  status           TEXT NOT NULL CHECK (status IN ('current', 'superseded')),
  supersedes_id    TEXT REFERENCES memories(id),
  superseded_by_id TEXT REFERENCES memories(id),
  fact_hash        TEXT NOT NULL,
  created_at       TEXT NOT NULL,
  superseded_at    TEXT, kind TEXT NOT NULL DEFAULT 'fact' CHECK (kind IN ('fact', 'rule')),
  UNIQUE (lineage_id, version),
  CHECK ((origin = 'external') = (source_tag IS NOT NULL)),
  CHECK ((scope = 'project') = (project IS NOT NULL)),
  CHECK (id = lineage_id || '.' || version)
);

CREATE TABLE memory_events (
  seq          INTEGER PRIMARY KEY,
  request_id   TEXT NOT NULL,
  source       TEXT NOT NULL CHECK (source IN ('mcp', 'ui', 'import')),
  source_ref   TEXT,
  action       TEXT NOT NULL CHECK (action IN ('add', 'update', 'noop')),
  lineage_id   TEXT NOT NULL,
  memory_id    TEXT NOT NULL REFERENCES memories(id),
  previous_id  TEXT REFERENCES memories(id),
  origin       TEXT NOT NULL CHECK (origin IN ('user', 'agent', 'external')),
  source_tag   TEXT CHECK (source_tag IS NULL OR (length(source_tag) BETWEEN 1 AND 32 AND source_tag NOT GLOB '*[^a-z0-9-]*')),
  session_id   TEXT CHECK (session_id IS NULL OR length(session_id) BETWEEN 1 AND 128),
  rationale    TEXT NOT NULL DEFAULT '',
  created_at   TEXT NOT NULL,
  CHECK ((source = 'import') = (source_ref IS NOT NULL)),
  CHECK ((origin = 'external') = (source_tag IS NOT NULL))
);

CREATE TABLE memory_embeddings (
  seq        INTEGER PRIMARY KEY REFERENCES memories(seq),
  model      TEXT NOT NULL,
  vec        BLOB NOT NULL CHECK (length(vec) > 0 AND length(vec) % 4 = 0),
  created_at TEXT NOT NULL
);

CREATE TABLE memory_groups (
  id         TEXT PRIMARY KEY,
  kind       TEXT NOT NULL CHECK (kind IN ('session', 'request', 'task', 'document', 'thread')),
  key        TEXT NOT NULL CHECK (length(key) BETWEEN 1 AND 4096),
  title      TEXT NOT NULL DEFAULT '' CHECK (length(title) <= 200),
  created_at INTEGER NOT NULL,
  UNIQUE (kind, key)
);

CREATE TABLE memory_group_members (
  group_id   TEXT NOT NULL REFERENCES memory_groups(id),
  lineage_id TEXT NOT NULL,
  added_at   INTEGER NOT NULL,
  PRIMARY KEY (group_id, lineage_id)
);

CREATE TABLE import_jobs (
  id            TEXT PRIMARY KEY,
  roots         TEXT NOT NULL,
  base          TEXT NOT NULL,
  state         TEXT NOT NULL CHECK (state IN ('scanning', 'awaiting', 'running', 'paused', 'done', 'cancelled', 'failed')),
  reason        TEXT NOT NULL DEFAULT '',
  truncated     INTEGER NOT NULL DEFAULT 0,
  ignored_count INTEGER NOT NULL DEFAULT 0,
  est_tokens    INTEGER NOT NULL DEFAULT 0,
  est_chunks    INTEGER NOT NULL DEFAULT 0,
  calls         INTEGER NOT NULL DEFAULT 0,
  cost_usd      REAL NOT NULL DEFAULT 0,
  created_at NOT NULL,
  started_at,
  finished_at,
  dismissed_at,
  channel       TEXT NOT NULL DEFAULT 'ui' CHECK (channel IN ('ui', 'mcp')),
  origin        TEXT NOT NULL DEFAULT 'external' CHECK (origin IN ('user', 'agent', 'external')),
  source_tag    TEXT CHECK (source_tag IS NULL OR (length(source_tag) BETWEEN 1 AND 32 AND source_tag NOT GLOB '*[^a-z0-9-]*')),
  scope         TEXT NOT NULL DEFAULT 'personal' CHECK (scope IN ('company', 'project', 'personal')),
  project       TEXT CHECK (project IS NULL OR (length(project) BETWEEN 1 AND 300
                  AND project = replace(replace(replace(project, ' ', ''), char(9), ''), char(10), ''))),
  session_id    TEXT CHECK (session_id IS NULL OR length(session_id) BETWEEN 1 AND 128),
  groups_json   TEXT NOT NULL DEFAULT '[]',
  auto_start    INTEGER NOT NULL DEFAULT 0 CHECK (auto_start IN (0, 1)),
  write_doc     INTEGER NOT NULL DEFAULT 0 CHECK (write_doc IN (0, 1)),
  CHECK ((origin = 'external') = (source_tag IS NOT NULL)),
  CHECK ((scope = 'project') = (project IS NOT NULL))
);

CREATE TABLE import_files (
  id                TEXT PRIMARY KEY,
  job_id            TEXT NOT NULL REFERENCES import_jobs(id) ON DELETE CASCADE,
  path              TEXT NOT NULL,
  rel_path          TEXT NOT NULL,
  kind              TEXT NOT NULL CHECK (kind IN ('markdown', 'text', 'json', 'jsonl', 'memdoc')),
  size              INTEGER NOT NULL,
  content_hash      TEXT NOT NULL DEFAULT '',
  state             TEXT NOT NULL CHECK (state IN ('pending', 'skipped', 'extracting', 'extracted', 'verifying', 'finalizing', 'done', 'failed', 'cancelled')),
  reason            TEXT NOT NULL DEFAULT '',
  title             TEXT NOT NULL DEFAULT '',
  chunk_count       INTEGER NOT NULL DEFAULT 0,
  est_tokens        INTEGER NOT NULL DEFAULT 0,
  fact_count        INTEGER NOT NULL DEFAULT 0,
  finalize_attempts INTEGER NOT NULL DEFAULT 0,
  added             INTEGER NOT NULL DEFAULT 0,
  updated           INTEGER NOT NULL DEFAULT 0,
  noop              INTEGER NOT NULL DEFAULT 0,
  unresolved        TEXT NOT NULL DEFAULT '[]',
  dropped           TEXT NOT NULL DEFAULT '[]',
  cost_usd          REAL NOT NULL DEFAULT 0,
  updated_at NOT NULL,
  verify_json       TEXT NOT NULL DEFAULT '{}',
  verified          INTEGER NOT NULL DEFAULT 0,
  orphaned          INTEGER NOT NULL DEFAULT 0,
  group_id          TEXT NOT NULL DEFAULT '',
  prov_json         TEXT NOT NULL DEFAULT '',
  doc_path          TEXT NOT NULL DEFAULT '',
  doc_error         TEXT NOT NULL DEFAULT '', verify_pending INTEGER NOT NULL DEFAULT -1,
  UNIQUE (job_id, rel_path)
);

CREATE TABLE import_chunks (
  file_id      TEXT NOT NULL REFERENCES import_files(id) ON DELETE CASCADE,
  idx          INTEGER NOT NULL,
  heading_path TEXT NOT NULL DEFAULT '',
  context      TEXT NOT NULL DEFAULT '',
  text         TEXT NOT NULL,
  tokens       INTEGER NOT NULL,
  state        TEXT NOT NULL CHECK (state IN ('pending', 'running', 'done', 'failed')),
  attempts     INTEGER NOT NULL DEFAULT 0,
  error        TEXT NOT NULL DEFAULT '',
  facts        TEXT NOT NULL DEFAULT '[]',
  cost_usd     REAL NOT NULL DEFAULT 0,
  PRIMARY KEY (file_id, idx)
);

CREATE UNIQUE INDEX memories_one_current ON memories(lineage_id) WHERE status = 'current';

CREATE INDEX memories_status_created ON memories(status, created_at DESC);

CREATE INDEX memories_current_hash ON memories(fact_hash) WHERE status = 'current';

CREATE INDEX memories_supersedes ON memories(supersedes_id);

CREATE INDEX memories_superseded_by ON memories(superseded_by_id);

CREATE INDEX memories_scope_project ON memories(scope, project) WHERE status = 'current';

CREATE INDEX memory_events_lineage ON memory_events(lineage_id, seq);

CREATE INDEX memory_events_memory ON memory_events(memory_id);

CREATE INDEX memory_events_previous ON memory_events(previous_id);

CREATE INDEX memory_events_source_ref ON memory_events(source_ref) WHERE source_ref IS NOT NULL;

CREATE INDEX memory_group_members_lineage ON memory_group_members(lineage_id);

CREATE INDEX import_files_job_state ON import_files(job_id, state);

CREATE INDEX import_files_done_hash ON import_files(content_hash) WHERE state = 'done';

CREATE INDEX memory_embeddings_model ON memory_embeddings(model);

CREATE TRIGGER memories_stay_superseded
BEFORE UPDATE OF status ON memories WHEN OLD.status = 'superseded'
BEGIN SELECT RAISE(ABORT, 'a superseded memory stays superseded'); END;

CREATE TRIGGER memories_no_delete
BEFORE DELETE ON memories
BEGIN SELECT RAISE(ABORT, 'memories are never deleted'); END;

CREATE TRIGGER memories_fts_insert AFTER INSERT ON memories
BEGIN INSERT INTO memories_fts(rowid, fact, reason, keywords) VALUES (new.seq, new.fact, new.reason, new.keywords); END;

CREATE TRIGGER memories_bump_revision AFTER INSERT ON memories
BEGIN UPDATE memory_revision SET value = value + 1 WHERE id = 1; END;

CREATE TRIGGER memory_groups_no_update BEFORE UPDATE ON memory_groups
BEGIN SELECT RAISE(ABORT, 'memory groups are append-only'); END;

CREATE TRIGGER memory_groups_no_delete BEFORE DELETE ON memory_groups
BEGIN SELECT RAISE(ABORT, 'memory groups are append-only'); END;

CREATE TRIGGER memory_group_members_no_update BEFORE UPDATE ON memory_group_members
BEGIN SELECT RAISE(ABORT, 'memory groups are append-only'); END;

CREATE TRIGGER memory_group_members_no_delete BEFORE DELETE ON memory_group_members
BEGIN SELECT RAISE(ABORT, 'memory groups are append-only'); END;

CREATE TRIGGER memories_immutable
BEFORE UPDATE OF id, lineage_id, version, fact, reason, keywords, origin, source_tag, scope, project, session_id,
  fact_hash, created_at, supersedes_id, kind ON memories
BEGIN SELECT RAISE(ABORT, 'memory values are immutable'); END;

INSERT INTO "memory_revision" ("id", "value") VALUES (1, 0);
