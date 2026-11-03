package configfs

import (
	"database/sql"
	"errors"
	"fmt"
)

// MigrateOnce runs fn once per app DB: the settings row `config.migrated.<name>` marks it done. A
// failed or interrupted fn leaves the row unset, so fn must be safe to run again.
func MigrateOnce(db *sql.DB, name string, fn func() error) error {
	key := "config.migrated." + name
	var done string
	err := db.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&done)
	if err == nil && done == "true" {
		return nil
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("configfs: read migration marker %s: %w", name, err)
	}
	if err := fn(); err != nil {
		return fmt.Errorf("configfs: migrate %s: %w", name, err)
	}
	if _, err := db.Exec(`INSERT INTO settings (key, value) VALUES (?, 'true')
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key); err != nil {
		return fmt.Errorf("configfs: write migration marker %s: %w", name, err)
	}
	return nil
}
