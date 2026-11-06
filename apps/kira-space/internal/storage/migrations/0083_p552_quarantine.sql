-- P552: rows a later P552 migration repaired or removed because they violate the tightened schema.
-- Written only by those migrations, so growth is bounded by one upgrade.
CREATE TABLE schema_quarantine (
  id          INTEGER PRIMARY KEY,
  table_name  TEXT NOT NULL,
  row_json    TEXT NOT NULL CHECK (json_valid(row_json)),
  reason      TEXT NOT NULL,
  action      TEXT NOT NULL CHECK (action IN ('repaired', 'removed')),
  migration   INTEGER NOT NULL,
  recorded_at INTEGER NOT NULL CHECK (recorded_at >= 0)
) STRICT;
