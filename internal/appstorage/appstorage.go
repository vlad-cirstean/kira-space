// Package appstorage holds the storage helpers Kira Space's own internal/storage packages share:
// the `windows`/`ui_layout` table helpers, the tab-state envelope check and statement closing.
package appstorage

import (
	"database/sql"
	"fmt"
)

// CloseStmts closes every prepared statement a Repos aggregate holds, stopping at (and returning)
// the first error.
func CloseStmts(stmts []*sql.Stmt) error {
	for _, stmt := range stmts {
		if err := stmt.Close(); err != nil {
			return fmt.Errorf("appstorage: close statement: %w", err)
		}
	}
	return nil
}
