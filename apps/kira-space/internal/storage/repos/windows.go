package repos

import (
	"database/sql"
	"fmt"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/appstorage"
	"github.com/kirathecat/kira-studio/internal/kiraschema"
	"github.com/kirathecat/kira-studio/internal/sqlb"
	"github.com/kirathecat/kira-studio/internal/sqlitex"
)

// WindowsRepo reads and writes the `windows` table (P8 D1/D4) — one row per workbench that is
// either open right now or was the last time the app quit, plus this app's own per-window module
// mode (P128 §2.2's migration 0003_p128_window_mode.sql).
type WindowsRepo struct {
	DB      *sql.DB
	Dialect sqlb.Dialect
}

// shared is this repo's own row against repo-root internal/appstorage.WindowRepo, which owns
// every method below directly (P107 T2-10, I2-3) — this app's own WindowRecord/WindowBounds are
// plain aliases of appstorage's own, so nothing here needs its own query or conversion any more.
func (r *WindowsRepo) shared() *appstorage.WindowRepo {
	return &appstorage.WindowRepo{DB: r.DB, Dialect: r.Dialect}
}

// GetMode reads one window's stored mode, normalised against this app's own vocabulary — a thin
// delegate to appstorage.WindowRepo.GetMode (P128 §2.2). Used by windowsvc.Service's bound Ensure,
// the one call the renderer already makes before it asks for anything window-scoped.
func (r *WindowsRepo) GetMode(key string) (string, error) {
	return r.shared().GetMode(key, model.WindowModes)
}

// SetMode persists one window's own module mode — a thin delegate to appstorage.WindowRepo.SetMode,
// written on shutdown/mode-debounce rather than on every mode click.
func (r *WindowsRepo) SetMode(key string, mode string) error {
	return r.shared().SetMode(key, mode, model.WindowModes)
}

// List returns every restorable window record in `order` (review windows are ephemeral and excluded). Not a hot boot path (read once at startup, per
// window record), so this has no prepared statement.
func (r *WindowsRepo) List() ([]model.WindowRecord, error) {
	all, err := r.shared().List()
	if err != nil {
		return nil, err
	}
	rows, err := sqlb.Query(r.DB, r.Dialect.Select(kiraschema.AdeReviewWindows.WindowKey).From(kiraschema.AdeReviewWindows.Table))
	review, err := sqlitex.QueryAll(rows, err, func(rows *sql.Rows) (string, bool, error) {
		var k string
		err := rows.Scan(&k)
		return k, true, err
	})
	if err != nil {
		return nil, fmt.Errorf("repos: list review window keys: %w", err)
	}
	if len(review) == 0 {
		return all, nil
	}
	skip := make(map[string]bool, len(review))
	for _, k := range review {
		skip[k] = true
	}
	out := all[:0:0]
	for _, w := range all {
		if !skip[w.Key] {
			out = append(out, w)
		}
	}
	return out, nil
}

// Exists reports whether key names a live `windows` row.
func (r *WindowsRepo) Exists(key string) (bool, error) {
	return r.shared().Exists(key)
}

// Create inserts a new window record. The caller mints the key (D2: a UUID the shell owns).
func (r *WindowsRepo) Create(rec model.WindowRecord) error {
	if err := rec.Validate(); err != nil {
		return fmt.Errorf("repos: %w", err)
	}
	return r.shared().Create(rec)
}

// EnsureExists creates a `windows` row for key if one doesn't already exist, ordered after every
// row already present, with no stored bounds — idempotent, and a no-op when the row is already
// there. Runs inside one transaction so two concurrent Ensure calls for the same brand-new key
// can't both observe "absent" and then race each other's INSERT.
func (r *WindowsRepo) EnsureExists(key string) error {
	return r.shared().EnsureExists(key)
}

// SetBounds persists one window's rectangle.
func (r *WindowsRepo) SetBounds(key string, b model.WindowBounds) error {
	return r.shared().SetBounds(key, b)
}

// Delete removes one window's row. Kira Studio's own Delete relies on the same statement's
// `tabs.window_key ... ON DELETE CASCADE` to clean up that window's tabs too — a DB-level
// constraint, so this app's own tabs (0002_p100_tabs_layout.sql) cascade the same way regardless
// of which Go code issues the DELETE.
func (r *WindowsRepo) Delete(key string) error {
	return r.shared().Delete(key)
}
