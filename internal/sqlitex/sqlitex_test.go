package sqlitex

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
)

func openTemp(t *testing.T) *sql.DB {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "x.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func scalar(t *testing.T, db *sql.DB, q string) string {
	t.Helper()
	var v sql.NullString
	if err := db.QueryRow(q).Scan(&v); err != nil {
		t.Fatalf("query %q: %v", q, err)
	}
	return v.String
}

// A database below the baseline refuses, at the baseline is a no-op, above it applies later
// steps, and above the highest known version refuses as too new.
func TestSchemaTooOld(t *testing.T) {
	steps := []Migration{
		{Version: 90, SQL: `CREATE TABLE a (id INTEGER) STRICT;`},
		{Version: 91, SQL: `CREATE TABLE b (id INTEGER) STRICT;`},
	}
	db := openTemp(t)
	if err := Migrate(db, steps); err != nil {
		t.Fatal(err)
	}
	if got := scalar(t, db, `SELECT version FROM schema_version`); got != "91" {
		t.Fatalf("fresh version = %s, want 91", got)
	}

	at := openTemp(t)
	if err := Migrate(at, steps[:1]); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(at, steps[:1]); err != nil {
		t.Fatalf("baseline rerun: %v", err)
	}
	if err := Migrate(at, steps); err != nil {
		t.Fatalf("forward step: %v", err)
	}

	old := openTemp(t)
	if err := Migrate(old, []Migration{{Version: 82, SQL: `CREATE TABLE a (id INTEGER) STRICT;`}}); err != nil {
		t.Fatal(err)
	}
	var tooOld *SchemaTooOldError
	if err := Migrate(old, steps); !errors.As(err, &tooOld) || tooOld.Found != 82 || tooOld.Baseline != 90 {
		t.Fatalf("v82 error = %v, want SchemaTooOldError{82, 90}", err)
	}

	var tooNew *SchemaTooNewError
	if err := Migrate(db, steps[:1]); !errors.As(err, &tooNew) {
		t.Fatalf("v91 against baseline 90 error = %v, want SchemaTooNewError", err)
	}
}

// A foreign-key step that leaves an orphan rolls back whole and keeps the version.
func TestForeignKeysOffStepRollsBackOnViolation(t *testing.T) {
	db := openTemp(t)
	steps := []Migration{
		{Version: 1, SQL: `CREATE TABLE p (id TEXT PRIMARY KEY) STRICT; CREATE TABLE c (pid TEXT REFERENCES p (id)) STRICT;`},
		{Version: 2, ForeignKeysOff: true, SQL: `INSERT INTO c (pid) VALUES ('missing');`},
	}
	if err := Migrate(db, steps); err == nil {
		t.Fatal("Migrate accepted a step that leaves an orphan")
	}
	if got := scalar(t, db, `SELECT MAX(version) FROM schema_version`); got != "1" {
		t.Errorf("version = %s, want 1", got)
	}
	if got := scalar(t, db, `SELECT COUNT(*) FROM c`); got != "0" {
		t.Error("the failed step's rows survived")
	}
	if got := scalar(t, db, `PRAGMA foreign_keys`); got != "1" {
		t.Errorf("foreign_keys = %s after a failed step, want 1", got)
	}
}
