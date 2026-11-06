package model

import (
	"github.com/kirathecat/kira-studio/internal/appstorage"
)

// WindowBounds is a plain screen rectangle, shared by every window's stored geometry — a plain
// alias of appstorage.WindowBounds (P107 I2-3: the two were already field-for-field identical,
// JSON tags included).
type WindowBounds = appstorage.WindowBounds

// WindowRecord is one row of the `windows` table — a durable, shell-minted identity
// for one workbench. Bounds is nil until the window has been moved or resized at least once
// (a freshly minted window with no stored rectangle inherits its cascade position instead).
// Mode is the app mode (packages/shared/domain/mode.ts's AppMode) this window was last
// closed in; a fresh row (no explicit mode column value on INSERT) reads back the migration's own
// DEFAULT 'studio'.
type WindowRecord struct {
	Key    string        `json:"key"`
	Order  int           `json:"order"`
	Bounds *WindowBounds `json:"bounds"`
	Mode   string        `json:"mode"`
}

// WindowModes is this app's own mode vocabulary (packages/shared/domain/mode.ts's AppMode) —
// the automations mode joined studio/api — windows.mode is unconstrained TEXT
// (no CHECK constraint, no migration needed), so an older binary reading an unrecognised mode still
// degrades cleanly through appstorage.WindowModes.Normalize; without an entry here, a window
// closed in the Automations module would silently reopen in Studio. A mode value stored by a
// since-removed module degrades to DefaultWindowMode the same way, no migration needed
// for the same reason (the vocabulary stays here, GetMode/SetMode's own bodies hoisted to
// appstorage.WindowRepo).
var WindowModes = appstorage.WindowModes{Default: DefaultWindowMode, Valid: []string{"studio", "api", "automations", "docker", "kubernetes"}}

// DefaultWindowMode is the app's own default mode — the migration's column DEFAULT and this
// constant deliberately agree, so there is exactly one place the default lives on each side.
const DefaultWindowMode = "studio"

// NormalizeMode returns mode unchanged if it's one of AppMode's known values, else
// DefaultWindowMode — the same drop-and-default posture an unrecognised enum gets elsewhere
// (ValidateObjectDefinition's own callers) rather than refusing to read or write the row: a
// hand-edited database or a future removed mode should never make a window fail to open.
func NormalizeMode(mode string) string {
	return WindowModes.Normalize(mode)
}

// Validate is the same non-empty-identity envelope TabRecord.Validate enforces (P2 R2's
// discipline applied here too): a bad row is refused at the write site, not left to silently
// round-trip and vanish on the next read.
func (w WindowRecord) Validate() error {
	return appstorage.ValidateWindowBounds(w.Key, w.Order)
}
