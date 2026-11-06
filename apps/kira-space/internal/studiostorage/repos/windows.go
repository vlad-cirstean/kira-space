package repos

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
	"github.com/kirathecat/kira-studio/internal/appstorage"
	"github.com/kirathecat/kira-studio/internal/sqlitex"
)

// WindowsRepo reads and writes the `windows` table (P8 D1/D4) — one row per workbench that is
// either open right now or was the last time the app quit, and the table `tabs.window_key`
// scopes tab ownership against (F6's fix).
type WindowsRepo struct {
	DB *sql.DB
}

// shared is this repo's own row against repo-root internal/appstorage.WindowRepo, which owns
// Exists/Create/EnsureExists/SetBounds/Delete — everything below that doesn't
// touch the `mode` column stays a thin delegate to it. List stays its own dedicated query (below)
// since it alone needs the extra `mode` column in the same round trip.
func (r *WindowsRepo) shared() *appstorage.WindowRepo { return &appstorage.WindowRepo{DB: r.DB} }

// List returns every window record in `order`. Not a hot boot path (read once at startup, per
// window record), so — unlike SettingsRepo/LayoutRepo/TabsRepo — this has no prepared statement.
func (r *WindowsRepo) List() ([]model.WindowRecord, error) {
	rows, err := r.DB.Query(`SELECT key, "order", bounds_json, mode FROM windows ORDER BY "order" ASC`)
	return sqlitex.QueryAll(rows, err, func(rows *sql.Rows) (model.WindowRecord, bool, error) {
		var (
			key        string
			order      int
			boundsJSON sql.NullString
			mode       string
		)
		if err := rows.Scan(&key, &order, &boundsJSON, &mode); err != nil {
			return model.WindowRecord{}, false, err
		}
		rec := model.WindowRecord{Key: key, Order: order, Mode: model.WindowModes.Normalize(mode)}
		if boundsJSON.Valid && boundsJSON.String != "" {
			var b model.WindowBounds
			if err := json.Unmarshal([]byte(boundsJSON.String), &b); err == nil {
				rec.Bounds = &b
			}
		}
		return rec, true, nil
	})
}

// Exists reports whether key names a live `windows` row — the check bridge.TabsService uses to
// reject an unrecognised window key with a real E_BAD_REQUEST rather than letting a bad key
// surface as a raw FOREIGN KEY constraint failure from TabsRepo.Save's insert.
func (r *WindowsRepo) Exists(key string) (bool, error) {
	return r.shared().Exists(key)
}

// Create inserts a new window record. The caller mints the key (D2: a UUID the shell owns).
func (r *WindowsRepo) Create(rec model.WindowRecord) error {
	if err := rec.Validate(); err != nil {
		return fmt.Errorf("repos/windows: %w", err)
	}
	// model.WindowBounds is appstorage.WindowBounds (a plain alias, I2-3), so rec.Bounds needs no
	// conversion here.
	return r.shared().Create(appstorage.WindowRecord{Key: rec.Key, Order: rec.Order, Bounds: rec.Bounds})
}

// EnsureExists creates a `windows` row for key if one doesn't already exist, ordered after every
// row already present, with no stored bounds — idempotent, and a no-op when the row is already
// there.
//
// On the native shell every window's row already exists by the time this could ever be called:
// main.go's own openWindow/openNewWindow/reopenWindow always call Create before the window's URL
// (and therefore the renderer that would ask) exists at all (D2). A `-tags server` build has no
// such shell — nothing native ever creates a window, so a browser tab pointed at an arbitrary
// `?window=<key>` (tests/e2e-real's multiwindow-real.spec.ts, or a developer's own tab) is the
// only thing that ever tells the backend that key exists, and TabsService's own checkWindow
// rejects an unregistered key outright rather than silently writing orphan rows (C4) — so the
// renderer calls WindowsService.Ensure once at boot, before it asks for anything window-scoped,
// and this is what that becomes on the Go side. Runs inside one transaction (matching LayoutRepo.Set's
// C7 fix) so two concurrent Ensure calls for the same brand-new key can't both observe "absent"
// and then race each other's INSERT.
func (r *WindowsRepo) EnsureExists(key string) error {
	return r.shared().EnsureExists(key)
}

// GetMode reads one window's stored mode, normalised the same way List does — a thin
// delegate to appstorage.WindowRepo.GetMode, this app's own vocabulary supplied. Used
// by windowsvc.Service's bound Ensure — the one call the renderer already makes before it asks for
// anything window-scoped, so this is the boot-time seam that carries `mode` to the frontend without
// a second round trip.
func (r *WindowsRepo) GetMode(key string) (string, error) {
	return r.shared().GetMode(key, model.WindowModes)
}

// SetMode persists one window's app mode (P22 D12) — the per-window analogue of SetBounds below, a
// thin delegate to appstorage.WindowRepo.SetMode, written on shutdown/mode-debounce rather than on
// every mode click (F20's own invariant: a mode switch itself schedules no write).
func (r *WindowsRepo) SetMode(key string, mode string) error {
	return r.shared().SetMode(key, mode, model.WindowModes)
}

// SetBounds persists one window's rectangle — the per-window analogue of the single
// `window.bounds` leaf LayoutRepo used to own for every window there had ever been (F5).
func (r *WindowsRepo) SetBounds(key string, b model.WindowBounds) error {
	return r.shared().SetBounds(key, b)
}

// Delete removes one window's row, cascading its tabs (`tabs.window_key ... ON DELETE CASCADE`,
// foreign_keys is on — storage/db.go). D5: the caller decides whether deleting is the right move
// (only when another window remains) — this method just does it.
func (r *WindowsRepo) Delete(key string) error {
	return r.shared().Delete(key)
}
