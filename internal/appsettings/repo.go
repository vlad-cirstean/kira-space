package appsettings

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/kirathecat/kira-studio/internal/kiraschema"
	"github.com/kirathecat/kira-studio/internal/sqlb"
)

// UpsertLeaf mirrors both apps' own upsertSettingsLeaf verbatim — the settings table's own
// per-leaf-row upsert (`${section}.${key}`, never a blob per section), so a row written before a
// key existed still parses on the next read (P52 §4.3).
func UpsertLeaf(tx *sql.Tx, key string, value any) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("appsettings: encode %s: %w", key, err)
	}
	c := kiraschema.Settings
	ib := sqlb.SQLite.InsertInto(c.Table).Cols(c.Key, c.Value).Values(key, string(encoded))
	sqlb.Upsert(ib, []string{c.Key}, []string{c.Value}, "")
	if _, err := sqlb.Exec(tx, ib); err != nil {
		return fmt.Errorf("appsettings: upsert %s: %w", key, err)
	}
	return nil
}

// UpsertOptional upserts key from *v when v is non-nil, a no-op otherwise — the `if p.X != nil {
// UpsertLeaf(tx, key, *p.X) }` guard every section-upsert function (here and each app's own
// settings/layout repo) repeated per field (P107 I2-1).
func UpsertOptional[T any](tx *sql.Tx, key string, v *T) error {
	if v == nil {
		return nil
	}
	return UpsertLeaf(tx, key, *v)
}

// UpsertAppearance mirrors both apps' own former upsertAppearanceSection verbatim.
func UpsertAppearance(tx *sql.Tx, a *AppearancePatch) error {
	if a == nil {
		return nil
	}
	if err := UpsertOptional(tx, "appearance.fontFamily", a.FontFamily); err != nil {
		return err
	}
	if err := UpsertOptional(tx, "appearance.fontSize", a.FontSize); err != nil {
		return err
	}
	if err := UpsertOptional(tx, "appearance.rowDensity", a.RowDensity); err != nil {
		return err
	}
	if err := UpsertOptional(tx, "appearance.wordWrap", a.WordWrap); err != nil {
		return err
	}
	return UpsertOptional(tx, "appearance.rowColoring", a.RowColoring)
}

// Leaf mirrors both apps' own former leaf[T] verbatim: overwrites *dst with the stored value for
// key if present, leaving the caller's default in place otherwise. An unparseable stored value is
// a hand-edited or stale-shape row; it is left at its default rather than propagated, the same
// "fail closed to a known-good value" discipline the TS build's zod parse enforces.
func Leaf[T any](stored map[string]json.RawMessage, key string, dst *T) {
	raw, ok := stored[key]
	if !ok {
		return
	}
	var v T
	if err := json.Unmarshal(raw, &v); err != nil {
		return
	}
	*dst = v
}

// LeafValid mirrors both apps' own former leafValid[T]: Leaf plus semantic validation — a stored
// value that parses but fails valid falls back to the default too.
func LeafValid[T any](stored map[string]json.RawMessage, key string, dst *T, valid func(T) bool) {
	raw, ok := stored[key]
	if !ok {
		return
	}
	var v T
	if err := json.Unmarshal(raw, &v); err != nil {
		return
	}
	if !valid(v) {
		return
	}
	*dst = v
}

// AlwaysValid mirrors both apps' own former alwaysValid[T] — appearance.fontSize's own "no bound
// in the TS schema either" validator.
func AlwaysValid[T any](T) bool { return true }

// ReadAppearance reads every appearance.* leaf from stored on top of DefaultAppearance(), mirroring
// both apps' own former GetAll's appearance.* block verbatim.
func ReadAppearance(stored map[string]json.RawMessage) Appearance {
	result := DefaultAppearance()
	Leaf(stored, "appearance.fontFamily", &result.FontFamily)
	LeafValid(stored, "appearance.fontSize", &result.FontSize, AlwaysValid[int])
	LeafValid(stored, "appearance.rowDensity", &result.RowDensity, ValidRowDensity)
	Leaf(stored, "appearance.wordWrap", &result.WordWrap)
	Leaf(stored, "appearance.rowColoring", &result.RowColoring)
	return result
}
