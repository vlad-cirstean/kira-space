// Package storage owns Kira Space's SQLite database: opening it, migrating it, and the hot-path
// repo queries. Mirrors apps/kira-space/internal/storage's own shape, built on repo-root
// internal/sqlitex's shared open/pragma/migrate mechanics rather than duplicating
// them — see internal/sqlitex's own doc comment for what it owns versus what each app keeps for
// itself.
package storage

import (
	"database/sql"
	"fmt"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/config"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/migrations"
	"github.com/kirathecat/kira-studio/internal/appstorage"
)

// DB wraps the single *sql.DB connection this app ever opens — its own kira.db, under its own
// home, never Kira Studio's.
type DB struct {
	*sql.DB
}

// Open creates KIRA_SPACE_HOME if needed, opens (or creates) the database file at 0600, applies
// the six startup pragmas, and runs every pending migration.
func Open() (*DB, error) {
	return OpenAt(config.KiraSpaceHome())
}

// OpenAt is Open against an explicit home dir instead of $KIRA_SPACE_HOME — lets a test pass its
// own t.TempDir() directly rather than t.Setenv (v1.4 P1's same reasoning).
func OpenAt(home string) (*DB, error) {
	if err := config.EnsureLayoutAt(home); err != nil {
		return nil, fmt.Errorf("storage: ensure layout: %w", err)
	}

	path := config.DbPathAt(home)
	steps, err := migrations.All()
	if err != nil {
		return nil, fmt.Errorf("storage: load migrations: %w", err)
	}
	// appstorage.OpenAt: same shared open+migrate mechanics Kira Studio's own storage.OpenAt uses
	// (P107 T2-10). Not wrapped: a *sqlitex.SchemaTooNewError must reach the caller as-is — see
	// internal/startupfail's Classify.
	sqlDB, err := appstorage.OpenAt(path, steps)
	if err != nil {
		return nil, err
	}
	return &DB{sqlDB}, nil
}
