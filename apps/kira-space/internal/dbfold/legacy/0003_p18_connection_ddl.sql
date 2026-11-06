-- P18 (v1.1) D2: a per-connection DDL document for the SQL language service — the user pastes a
-- schema dump here, and everything the console completes/diagnoses/hovers is parsed from it,
-- never from a live connection. One row per connection, absent until the user writes one.
-- ON DELETE CASCADE matches connection_filters/saved_queries: deleting a connection cannot leave
-- an orphaned document behind.
CREATE TABLE connection_ddl (
  connection_id TEXT PRIMARY KEY REFERENCES connections(id) ON DELETE CASCADE,
  ddl           TEXT NOT NULL,
  updated_at    TEXT NOT NULL
);
