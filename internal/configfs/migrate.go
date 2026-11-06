package configfs

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/kirathecat/kira-studio/internal/kiraschema"
	"github.com/kirathecat/kira-studio/internal/sqlb"
)

// MigrateOnce runs fn once per app DB: the settings row `config.migrated.<name>` marks it done. A
// failed or interrupted fn leaves the row unset, so fn must be safe to run again.
func MigrateOnce(db *sql.DB, name string, fn func() error) error {
	key := "config.migrated." + name
	var done string
	c := kiraschema.Settings
	sel := sqlb.SQLite.Select(c.Value).From(c.Table)
	sel.Where(sel.Equal(c.Key, key))
	err := sqlb.QueryRow(db, sel).Scan(&done)
	if err == nil && done == "true" {
		return nil
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("configfs: read migration marker %s: %w", name, err)
	}
	if err := fn(); err != nil {
		return fmt.Errorf("configfs: migrate %s: %w", name, err)
	}
	ib := sqlb.SQLite.InsertInto(c.Table).Cols(c.Key, c.Value).Values(key, "true")
	sqlb.Upsert(ib, []string{c.Key}, []string{c.Value}, "")
	if _, err := sqlb.Exec(db, ib); err != nil {
		return fmt.Errorf("configfs: write migration marker %s: %w", name, err)
	}
	return nil
}
