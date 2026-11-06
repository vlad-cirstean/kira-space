package repos

import (
	"database/sql"
	"fmt"
	"log/slog"
)

// reclaimThresholdBytes is the trigger for running incremental_vacuum at startup — not
// zero, since incremental_vacuum moves pages and doing that for a few hundred kilobytes on every
// launch is churn with no user-visible benefit. 16 MiB is half op_log's own new table budget
// and an eighth of api_response_history's — "one of this app's caps fired and released
// something worth releasing."
const reclaimThresholdBytes = 16 * 1024 * 1024

// Maintenance is repository-adjacent housekeeping that acts on the database file as a whole
// rather than on one table — a separate type from Repos (repos.go) since it owns no table of its
// own.
type Maintenance struct {
	DB *sql.DB
}

// Reclaim, on a database opened with _auto_vacuum=INCREMENTAL (db.go's buildDSN), returns freed
// pages to the filesystem once the freelist has grown past reclaimThresholdBytes. Run once at
// startup, beside the two SweepOrphans calls already there (main.go) — quit is when a write must
// not be the thing that goes wrong, the same reason every other sweep in this app runs at launch
// rather than at exit.
func (m *Maintenance) Reclaim() error {
	var autoVacuum int
	if err := m.DB.QueryRow(`PRAGMA auto_vacuum`).Scan(&autoVacuum); err != nil {
		return fmt.Errorf("repos/maintenance: read auto_vacuum: %w", err)
	}
	if autoVacuum != 2 { // 2 = INCREMENTAL; a database opened before this phase reads 0 (NONE).
		return nil
	}

	var freelistPages, pageSize int64
	if err := m.DB.QueryRow(`PRAGMA freelist_count`).Scan(&freelistPages); err != nil {
		return fmt.Errorf("repos/maintenance: read freelist_count: %w", err)
	}
	if err := m.DB.QueryRow(`PRAGMA page_size`).Scan(&pageSize); err != nil {
		return fmt.Errorf("repos/maintenance: read page_size: %w", err)
	}

	freelistBytes := freelistPages * pageSize
	if freelistBytes <= reclaimThresholdBytes {
		return nil
	}

	if _, err := m.DB.Exec(`PRAGMA incremental_vacuum`); err != nil {
		return fmt.Errorf("repos/maintenance: incremental_vacuum: %w", err)
	}
	slog.Info("reclaimed freed pages", "scope", "storage/maintenance", "freedBytes", freelistBytes)
	return nil
}
