-- M5: per-column PII masking rules, and the per-connection correlation key they use.
--
-- A table, not an options_json key and not a blob on `connections`: the rule set is queried by
-- (connection_id, column_name) on every run_query render (plan §4.2), listed in two UIs, and edited
-- one row at a time. 0021's own reasoning also applies — `options` round-trips through the
-- connection URI and the Copy URI menu item, and a privacy rule must not be settable, or
-- clearable, by pasting a URI.
CREATE TABLE connection_mask_rules (
  id            TEXT PRIMARY KEY,
  connection_id TEXT NOT NULL REFERENCES connections(id) ON DELETE CASCADE,
  -- '*' matches any table. A concrete name is kept even though matching is by column name alone
  -- (plan §4.2): it is what makes a rule legible in the UI and what the grid's header menu writes.
  table_name    TEXT NOT NULL,
  column_name   TEXT NOT NULL,
  -- name | email | text | number | date | redact
  mask_kind     TEXT NOT NULL,
  -- Keep this kind's own small hint: the initials for `name`, the domain for `email`, the year for
  -- `date`. Meaningless for text/number/redact, stored anyway so the column stays one flag rather
  -- than three kind-specific ones.
  keep_hint     INTEGER NOT NULL DEFAULT 1,
  -- Emit the keyed correlation tag (plan §2.4). Forced 0 for `number` (a bucket is many-to-one,
  -- so tagging it would be dishonest — internal/mask.Apply enforces this regardless of this flag).
  correlate     INTEGER NOT NULL DEFAULT 1,
  created_at    TEXT NOT NULL,
  updated_at    TEXT NOT NULL
);

-- Matching is case-insensitive (plan §4.2), so uniqueness must be too — otherwise `Email` and
-- `email` are two rows that collapse to one at render time, and which one wins is arbitrary.
CREATE UNIQUE INDEX connection_mask_rules_unique
  ON connection_mask_rules(connection_id, lower(table_name), lower(column_name));
CREATE INDEX connection_mask_rules_conn ON connection_mask_rules(connection_id);

-- The per-connection correlation key (plan §2.5), stored as a kira:v3 envelope under the new
-- `mask-key` secret scope. '' means "no key yet" — minted lazily on the first correlating render.
-- Read and written only by MaskKeysRepo, the same single-owner discipline SecretsRepo has for
-- `password`; deliberately absent from ConnectionFields (model/connection.go D9).
ALTER TABLE connections ADD COLUMN mask_correlation_key TEXT NOT NULL DEFAULT '';
