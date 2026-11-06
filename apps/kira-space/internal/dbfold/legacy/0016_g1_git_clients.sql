-- G1 D7: the trust store for paired VS Code extensions (SPEC §3.3). One row per approved client
-- identity; only the salted hash of its token is ever stored, never the plaintext (D6). Timestamps
-- are epoch-millisecond integers rather than this schema's usual ISO TEXT, since this table is
-- G1's own addition and has no prior rows anywhere to stay consistent with.
CREATE TABLE git_clients (
  id           TEXT PRIMARY KEY,
  label        TEXT NOT NULL,
  token_hash   BLOB NOT NULL,
  token_salt   BLOB NOT NULL,
  created_at   INTEGER NOT NULL,
  last_seen_at INTEGER NOT NULL,
  revoked_at   INTEGER
);

CREATE INDEX git_clients_last_seen ON git_clients(last_seen_at DESC);
