-- P760: Kibana is no longer a connection kind. Its rows go; foreign keys cascade to tabs, saved queries,
-- mask rules, reviewed columns, filter history and metadata cache, and null op_log.connection_id.
-- The connection file stays on disk and loads as invalid, with no last good value left to serve.
DELETE FROM connections WHERE kind = 'kibana';

-- Saved Discover queries existed only on kibana connections. Rebuild without the kind.
DROP INDEX connection_saved_queries_discover_name;

CREATE TABLE connection_saved_queries_new (
  id TEXT NOT NULL PRIMARY KEY CHECK (id <> ''),
  connection_id TEXT NOT NULL REFERENCES connections (id) ON DELETE CASCADE,
  path TEXT NOT NULL,
  name TEXT NOT NULL,
  kind TEXT NOT NULL CHECK (kind IN ('filter', 'console')),
  pinned INTEGER NOT NULL DEFAULT 0 CHECK (pinned IN (0, 1)),
  created_at INTEGER NOT NULL CHECK (created_at >= 0),
  used_at INTEGER CHECK (used_at IS NULL OR used_at >= 0),
  body TEXT NOT NULL
) STRICT;

INSERT INTO connection_saved_queries_new (id, connection_id, path, name, kind, pinned, created_at, used_at, body)
SELECT id, connection_id, path, name, kind, pinned, created_at, used_at, body
FROM connection_saved_queries WHERE kind <> 'discover';

DROP TABLE connection_saved_queries;
ALTER TABLE connection_saved_queries_new RENAME TO connection_saved_queries;

CREATE INDEX connection_saved_queries_path ON connection_saved_queries (connection_id, path, kind, pinned DESC, used_at DESC, name);
