// Package storage owns Kira Space's SQLite database: opening it, migrating it, and the hot-path
// repo queries, built on repo-root internal/sqlitex's shared open/pragma/migrate mechanics.
package storage

import (
	"database/sql"
	"fmt"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/config"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/migrations"
	"github.com/kirathecat/kira-studio/internal/sqlitex"
)

// DB wraps the single *sql.DB connection this app ever opens: its own kira.db under its own home.
type DB struct {
	*sql.DB
}

// Open creates KIRA_SPACE_HOME if needed, opens (or creates) the database file at 0600, applies
// the six startup pragmas, and runs every pending migration.
func Open() (*DB, error) {
	return OpenAt(config.KiraSpaceHome())
}

// OpenAt is Open against an explicit home dir instead of $KIRA_SPACE_HOME, so a test passes its
// own t.TempDir() directly rather than t.Setenv.
func OpenAt(home string) (*DB, error) {
	if err := config.EnsureLayoutAt(home); err != nil {
		return nil, fmt.Errorf("storage: ensure layout: %w", err)
	}

	steps, err := migrations.All()
	if err != nil {
		return nil, fmt.Errorf("storage: load migrations: %w", err)
	}
	sqlDB, err := sqlitex.Open(config.DbPathAt(home))
	if err != nil {
		return nil, fmt.Errorf("appstorage: %w", err)
	}
	// Not wrapped: a *sqlitex.SchemaTooNewError or SchemaTooOldError must reach the caller as-is,
	// see internal/startupfail's Classify.
	if err := sqlitex.Migrate(sqlDB, steps); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}
	return &DB{sqlDB}, nil
}
