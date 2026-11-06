-- P452: last good value of a config file whose folder has no module table. value_json is the decoded value.
CREATE TABLE config_last_valid (
  folder      TEXT NOT NULL,
  file        TEXT NOT NULL,
  value_json  TEXT NOT NULL,
  recorded_at INTEGER NOT NULL,
  PRIMARY KEY (folder, file)
) WITHOUT ROWID;
