-- P447: sealed secret values per owner. value is a kira:v3: envelope under scope, never plaintext.
CREATE TABLE secret_values (
  scope      TEXT NOT NULL,
  owner      TEXT NOT NULL,
  name       TEXT NOT NULL,
  value      TEXT NOT NULL,
  updated_ms  INTEGER NOT NULL,
  PRIMARY KEY (scope, owner, name)
) WITHOUT ROWID;
