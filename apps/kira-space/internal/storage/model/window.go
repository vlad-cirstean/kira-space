package model

import (
	"github.com/kirathecat/kira-studio/internal/appstorage"
)

// WindowBounds is a plain screen rectangle, shared by every window's stored geometry — a plain
// alias of appstorage.WindowBounds (P107 I2-3: the two were already field-for-field identical,
// JSON tags included).
type WindowBounds = appstorage.WindowBounds

// WindowRecord is one row of the `windows` table's identity/geometry columns — a durable,
// shell-minted identity for one workbench. Bounds is nil until the window has been moved or
// resized at least once (a freshly minted window with no stored rectangle inherits its cascade
// position instead).
//
// This app's own `windows` table gained a `mode` column (migration 0003_p128_window_mode.sql,
// the git/automations/ade module registry) — read and written through WindowsRepo.GetMode/SetMode
// below, not through this struct, so WindowRecord itself stays field-for-field identical to
// appstorage.WindowRecord and a plain alias of it, Validate() included; internal/shell/window.go's
// own Options()/Attach() never read a mode, only .Key/.Bounds.
type WindowRecord = appstorage.WindowRecord

// WindowModes is this app's own mode vocabulary (workbench/modes.ts's SpaceMode) — an unrecognised
// or since-removed mode degrades to Default via appstorage.WindowModes.Normalize rather than
// failing to read/write the row.
var WindowModes = appstorage.WindowModes{Default: "git", Valid: []string{"git", "automations", "ade", "memory", "studio", "api", "docker", "kubernetes"}}
