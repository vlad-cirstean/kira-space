package appstorage

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/kirathecat/kira-studio/internal/kiraschema"
	"github.com/kirathecat/kira-studio/internal/sqlb"
)

// WindowBounds is a plain screen rectangle, shared by every window's stored geometry — field-for-
// field identical to each app's own storage/model.WindowBounds (and to internal/shell's own
// WindowBounds, which exists for the same reason: this package stays independent of any per-app
// storage package, so a caller converts at its own boundary, same idiom internal/shell/deps.go's
// own WindowBounds doc comment already established).
type WindowBounds struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// ValidateWindowBounds is the non-empty-identity envelope every app's own WindowRecord.Validate
// enforces before a write — key non-empty, order >= 0 — regardless of whatever extra fields that
// app's own record carries (Studio's own Mode; Space has none).
func ValidateWindowBounds(key string, order int) error {
	if key == "" {
		return errors.New("model: window: key is required")
	}
	if order < 0 {
		return fmt.Errorf("model: window %q: order must be >= 0", key)
	}
	return nil
}

// WindowRecord is one row of the `windows` table's identity/geometry columns, minus `mode` — both
// apps' own storage/model.WindowRecord carry Mode themselves instead (Studio's own
// 0014_p22_window_mode.sql; Space's own 0003_p128_window_mode.sql), so List/Delete below
// move here while GetMode/SetMode below stay separate, vocabulary-parametrised calls each app's own
// WindowsRepo delegates into — internal/shell's own WindowRecord (no per-app fields at all) also
// aliases this.
type WindowRecord struct {
	Key    string        `json:"key"`
	Order  int           `json:"order"`
	Bounds *WindowBounds `json:"bounds"`
}

// Validate is the same non-empty-identity envelope every app's own WindowRecord.Validate enforces.
func (w WindowRecord) Validate() error {
	return ValidateWindowBounds(w.Key, w.Order)
}

// WindowRepo reads and writes the `windows` table's identity/geometry columns, plus the shared,
// vocabulary-parametrised GetMode/SetMode below (both apps now carry a `mode` column).
// Each app's own WindowsRepo embeds one of these (constructed against its own DB) for Exists/
// Create/EnsureExists/SetBounds/GetMode/SetMode, and keeps its own List/Delete on top since those
// touch app-specific model types this package deliberately stays clear of.
type WindowRepo struct {
	DB      *sql.DB
	Dialect sqlb.Dialect
}

// WindowModes is one app's own mode vocabulary — its default and every value Normalize below
// accepts. Supplied by the caller (each app's own storage/model package) rather than owned here,
// so the vocabulary itself stays where each app's module registry does.
type WindowModes struct {
	Default string
	Valid   []string
}

// Normalize returns mode unchanged if it's one of m.Valid, else m.Default — the same drop-and-
// default posture an unrecognised enum gets elsewhere: a hand-edited database, or a value written
// by a since-removed module, should never make a window fail to open.
func (m WindowModes) Normalize(mode string) string {
	for _, v := range m.Valid {
		if v == mode {
			return mode
		}
	}
	return m.Default
}

// GetMode reads one window's stored mode, normalised against modes. Used by windowsvc.Service's
// bound Ensure — the one call the renderer already makes before it asks for anything window-scoped,
// so this is the boot-time seam that carries `mode` to the frontend without a second round trip.
func (r *WindowRepo) GetMode(key string, modes WindowModes) (string, error) {
	var mode string
	c := kiraschema.Windows
	sb := r.Dialect.Select(c.Mode).From(c.Table)
	sb.Where(sb.Equal(c.Key, key))
	err := sqlb.QueryRow(r.DB, sb).Scan(&mode)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("appstorage/windows: %s: no such window", key)
	}
	if err != nil {
		return "", fmt.Errorf("appstorage/windows: get mode %s: %w", key, err)
	}
	return modes.Normalize(mode), nil
}

// SetMode persists one window's app mode — written on shutdown/mode-debounce rather than on every
// mode click (a mode switch itself schedules no write).
func (r *WindowRepo) SetMode(key, mode string, modes WindowModes) error {
	c := kiraschema.Windows
	ub := r.Dialect.Update(c.Table)
	ub.Set(ub.Assign(c.Mode, modes.Normalize(mode))).Where(ub.Equal(c.Key, key))
	res, err := sqlb.Exec(r.DB, ub)
	if err != nil {
		return fmt.Errorf("appstorage/windows: update mode %s: %w", key, err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return fmt.Errorf("appstorage/windows: %s: no such window", key)
	}
	return nil
}

// Exists reports whether key names a live `windows` row.
func (r *WindowRepo) Exists(key string) (bool, error) {
	var one int
	c := kiraschema.Windows
	sb := r.Dialect.Select("1").From(c.Table)
	sb.Where(sb.Equal(c.Key, key))
	err := sqlb.QueryRow(r.DB, sb).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("appstorage/windows: exists %s: %w", key, err)
	}
	return true, nil
}

// Create inserts a new window record. The caller mints key (a UUID the shell owns); bounds may
// be nil (a freshly minted window with no stored rectangle yet). Never writes `mode` — Studio's
// own mode column always falls back to the migration's own DEFAULT 'studio' on insert, set only
// afterward via Studio's own SetMode, so dropping it here changes nothing observable.
func (r *WindowRepo) Create(rec WindowRecord) error {
	if err := rec.Validate(); err != nil {
		return fmt.Errorf("appstorage/windows: %w", err)
	}
	var boundsJSON any
	if rec.Bounds != nil {
		encoded, err := json.Marshal(rec.Bounds)
		if err != nil {
			return fmt.Errorf("appstorage/windows: encode bounds: %w", err)
		}
		boundsJSON = string(encoded)
	}
	c := kiraschema.Windows
	ib := r.Dialect.InsertInto(c.Table).Cols(c.Key, c.SortOrder, c.BoundsJSON).Values(rec.Key, rec.Order, boundsJSON)
	if _, err := sqlb.Exec(r.DB, ib); err != nil {
		return fmt.Errorf("appstorage/windows: insert %s: %w", rec.Key, err)
	}
	return nil
}

// List returns every window record in `order`, minus `mode` (WindowRecord's own doc comment) —
// Kira Space's own WindowsRepo.List delegates here outright; Kira Studio's own List stays a
// dedicated query so it can read its own extra `mode` column in the same round trip.
func (r *WindowRepo) List() ([]WindowRecord, error) {
	c := kiraschema.Windows
	sb := r.Dialect.Select(c.Key, c.SortOrder, c.BoundsJSON).From(c.Table).OrderBy(c.SortOrder + " ASC")
	rows, err := sqlb.Query(r.DB, sb)
	if err != nil {
		return nil, fmt.Errorf("appstorage/windows: query: %w", err)
	}
	defer rows.Close()

	out := []WindowRecord{}
	for rows.Next() {
		var (
			key        string
			order      int
			boundsJSON sql.NullString
		)
		if err := rows.Scan(&key, &order, &boundsJSON); err != nil {
			return nil, fmt.Errorf("appstorage/windows: scan: %w", err)
		}
		rec := WindowRecord{Key: key, Order: order}
		if boundsJSON.Valid && boundsJSON.String != "" {
			var b WindowBounds
			if err := json.Unmarshal([]byte(boundsJSON.String), &b); err == nil {
				rec.Bounds = &b
			}
		}
		out = append(out, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("appstorage/windows: rows: %w", err)
	}
	return out, nil
}

// Delete removes one window's row — both apps' own WindowsRepo.Delete ran this identical
// statement (Studio's own row also cascades its tabs via `tabs.window_key ... ON DELETE CASCADE`,
// a DB-level constraint this call triggers the same way regardless of which Go code issues it).
func (r *WindowRepo) Delete(key string) error {
	c := kiraschema.Windows
	db := r.Dialect.DeleteFrom(c.Table)
	db.Where(db.Equal(c.Key, key))
	if _, err := sqlb.Exec(r.DB, db); err != nil {
		return fmt.Errorf("appstorage/windows: delete %s: %w", key, err)
	}
	return nil
}

// EnsureExists creates a `windows` row for key if one doesn't already exist, ordered after every
// row already present, with no stored bounds — idempotent, and a no-op when the row is already
// there. Runs inside one transaction so two concurrent Ensure calls for the same brand-new key
// can't both observe "absent" and then race each other's INSERT.
func (r *WindowRepo) EnsureExists(key string) error {
	tx, err := r.DB.Begin()
	if err != nil {
		return fmt.Errorf("appstorage/windows: ensure begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	c := kiraschema.Windows
	var exists bool
	probe := r.Dialect.Select("1").From(c.Table)
	probe.Where(probe.Equal(c.Key, key))
	check := r.Dialect.Select()
	check.Select(check.Exists(probe))
	if err := sqlb.QueryRow(tx, check).Scan(&exists); err != nil {
		return fmt.Errorf("appstorage/windows: ensure exists %s: %w", key, err)
	}
	if !exists {
		var maxOrder sql.NullInt64
		if err := sqlb.QueryRow(tx, r.Dialect.Select("MAX("+c.SortOrder+")").From(c.Table)).Scan(&maxOrder); err != nil {
			return fmt.Errorf("appstorage/windows: ensure max order: %w", err)
		}
		order := 0
		if maxOrder.Valid {
			order = int(maxOrder.Int64) + 1
		}
		ib := r.Dialect.InsertInto(c.Table).Cols(c.Key, c.SortOrder, c.BoundsJSON).Values(key, order, nil)
		if _, err := sqlb.Exec(tx, ib); err != nil {
			return fmt.Errorf("appstorage/windows: ensure insert %s: %w", key, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("appstorage/windows: ensure commit: %w", err)
	}
	return nil
}

// SetBounds persists one window's rectangle.
func (r *WindowRepo) SetBounds(key string, b WindowBounds) error {
	encoded, err := json.Marshal(b)
	if err != nil {
		return fmt.Errorf("appstorage/windows: encode bounds: %w", err)
	}
	c := kiraschema.Windows
	ub := r.Dialect.Update(c.Table)
	ub.Set(ub.Assign(c.BoundsJSON, string(encoded))).Where(ub.Equal(c.Key, key))
	res, err := sqlb.Exec(r.DB, ub)
	if err != nil {
		return fmt.Errorf("appstorage/windows: update %s: %w", key, err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return fmt.Errorf("appstorage/windows: %s: no such window", key)
	}
	return nil
}
