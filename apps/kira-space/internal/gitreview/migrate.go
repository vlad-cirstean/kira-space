package gitreview

import (
	"database/sql"
	"fmt"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitreview/migrations"
	"github.com/kirathecat/kira-studio/internal/sqlitex"
)

// migrate runs review.db's schema_version steps through sqlitex.Migrate. Its error is returned
// unwrapped: a *sqlitex.SchemaTooNewError or SchemaTooOldError must reach the caller as-is for
// internal/startupfail's Classify to recognise it (a direct type assertion, not errors.As).
func migrate(db *sql.DB) error {
	steps, err := migrations.All()
	if err != nil {
		return fmt.Errorf("gitreview: load migrations: %w", err)
	}
	return sqlitex.Migrate(db, steps)
}
