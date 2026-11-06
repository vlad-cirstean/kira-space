// Package storage owns the Go build's SQLite database: opening it, migrating it, and the
// hot-path repo queries. Go analogue of src/main/storage/{db,migrate}.ts.
package storage

import (
	"database/sql"
	"fmt"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/migrations"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiotmp/config"
	"github.com/kirathecat/kira-studio/internal/appstorage"
)

// DB wraps the single *sql.DB connection this app ever opens.
//
// modernc.org/sqlite is a pure-Go transpilation of the same upstream SQLite amalgamation
// mattn/go-sqlite3 links via cgo — same engine, no cgo toolchain needed to build this binary
// (the sqlite adapter package already made this switch for the external-file browsing path;
// this is the same driver, same DSN-based pragma convention, applied to the app's own database).
// SetMaxOpenConns(1): this app's database is a small, single-writer configuration store, and
// serialising every statement onto one connection removes the SQLITE_BUSY class of bug entirely
// at no measurable cost.
type DB struct {
	*sql.DB
}

// Open creates KIRA_HOME if needed, opens (or creates) the database file at the trimmed
// permissions the Electron build uses, applies the four startup pragmas, and runs every pending
// migration.
func Open() (*DB, error) {
	return OpenAt(config.KiraHome())
}

// OpenAt is Open against an explicit home dir instead of $KIRA_HOME. Exists so a test can pass its
// own t.TempDir() directly rather than t.Setenv("KIRA_HOME", …) — Go's testing package panics if
// t.Setenv runs in a test that called t.Parallel() (or whose parent did), so a test wanting both
// isolation and parallelism needs the dir threaded explicitly.
func OpenAt(home string) (*DB, error) {
	if err := config.EnsureLayoutAt(home); err != nil {
		return nil, fmt.Errorf("storage: ensure layout: %w", err)
	}

	path := config.DbPathAt(home)
	steps, err := migrations.All()
	if err != nil {
		return nil, fmt.Errorf("storage: load migrations: %w", err)
	}
	// appstorage.OpenAt runs the four steps this used to run inline (open with the six DSN
	// pragmas, SetMaxOpenConns(1), Ping to force the lazy first connection, chmod 0600) plus the
	// migration run — repo-root internal/appstorage, shared with apps/kira-space's own storage
	// package. Not wrapped: a *sqlitex.SchemaTooNewError must reach the
	// caller as-is, since internal/startupfail's Classify recognises it via a direct type assertion
	// (err.(schemaTooNew)), not errors.As.
	sqlDB, err := appstorage.OpenAt(path, steps)
	if err != nil {
		return nil, err
	}
	return &DB{sqlDB}, nil
}
