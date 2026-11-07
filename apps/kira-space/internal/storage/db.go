// Package storage owns Kira Space's SQLite database: opening it, migrating it, and the hot-path
// repo queries. Mirrors apps/kira-space/internal/storage's own shape, built on repo-root
// internal/sqlitex's shared open/pragma/migrate mechanics rather than duplicating
// them — see internal/sqlitex's own doc comment for what it owns versus what each app keeps for
// itself.
package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/config"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/migrations"
	"github.com/kirathecat/kira-studio/internal/sqlitex"
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
	// Not wrapped: a *sqlitex.SchemaTooNewError must reach the caller as-is — see
	// internal/startupfail's Classify.
	sqlDB, err := sqlitex.Open(path)
	if err != nil {
		return nil, fmt.Errorf("appstorage: %w", err)
	}
	backupDir, err := backupBeforeP552(sqlDB, home)
	if err != nil {
		_ = sqlDB.Close()
		return nil, err
	}
	startedMs := time.Now().UnixMilli()
	if err := sqlitex.Migrate(sqlDB, steps); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}
	warnQuarantine(sqlDB, backupDir, startedMs)
	return &DB{sqlDB}, nil
}

// p552Version is the first schema version P552 rewrites tables at.
const p552Version = 84

// backupBeforeP552 writes a VACUUM INTO copy of a pre-P552 database (tables present, version
// below p552Version) to <home>/backups/p552-<UTC ts>/kira.db. A backup already on disk, left by an
// interrupted run, is reused. A fresh database writes none. Returns the backup dir, "" for none.
func backupBeforeP552(db *sql.DB, home string) (string, error) {
	var version int
	err := db.QueryRow(`SELECT version FROM schema_version LIMIT 1`).Scan(&version)
	if err != nil {
		// No schema_version table or row yet: a fresh database.
		return "", nil
	}
	if version <= 0 || version >= p552Version {
		return "", nil
	}
	root := filepath.Join(home, "backups")
	if matches, _ := filepath.Glob(filepath.Join(root, "p552-*", "kira.db")); len(matches) > 0 {
		return filepath.Dir(matches[0]), nil
	}
	dir := filepath.Join(root, "p552-"+time.Now().UTC().Format("20060102T150405Z"))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("storage: p552 backup: %w", err)
	}
	dst := filepath.Join(dir, "kira.db")
	tmp := dst + ".tmp"
	if err := os.Remove(tmp); err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("storage: p552 backup: %w", err)
	}
	if _, err := db.Exec(`VACUUM INTO ?`, tmp); err != nil {
		return "", fmt.Errorf("storage: p552 backup: %w", err)
	}
	if err := os.Rename(tmp, dst); err != nil {
		return "", fmt.Errorf("storage: p552 backup: %w", err)
	}
	return dir, nil
}

// warnQuarantine logs one warning per table a P552 migration repaired or removed rows in since sinceMs.
func warnQuarantine(db *sql.DB, backupDir string, sinceMs int64) {
	rows, err := db.Query(`SELECT table_name, action, COUNT(*) FROM schema_quarantine
		WHERE migration >= ? AND recorded_at >= ? GROUP BY table_name, action`, p552Version, sinceMs)
	if err != nil {
		return
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var table, action string
		var n int
		if rows.Scan(&table, &action, &n) == nil {
			slog.Warn("storage: schema migration touched rows", "scope", "storage", "table", table, "action", action, "rows", n, "backup", backupDir)
		}
	}
}
