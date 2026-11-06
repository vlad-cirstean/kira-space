// Package appcore is the Go analogue of src/main/ipc/deps.ts — the one Deps struct embedded by
// value into each bound service, and the startup-ordering entry point (src/main/index.ts).
package appcore

import (
	"database/sql"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapterhost"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/apivars"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/connections"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/connfiles"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/maskreview"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/maskrules"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/repos"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/tree"
	"github.com/kirathecat/kira-studio/internal/appevent"
	"github.com/kirathecat/kira-studio/internal/llmprompts"
)

// Emitter is the Go→renderer push seam (P56 D1/D4) — repo-root internal/appevent.Emitter
// (P103 Part 3: identical in both apps, hoisted there). internal/shell implements it over
// *application.App's Event.Emit/DispatchWailsEvent; bridge/events.go is its one real consumer
// package, so no bridge file has to import Wails. Deps carries the bare interface, not a
// *bridge.Events, since bridge already imports appcore — a *bridge.Events field here would be an
// import cycle. EmitTo (P8 C6) delivers to exactly one window by key — the mechanism the
// per-window close-flush handshake builds on. EmitFocused (P8 C9/D6) delivers to whichever window
// is currently key/focused, the successor to Electron's own sendToFocusedWindow — the menu's
// twelve signal channels use it so a background window no longer reacts to a command the user
// aimed at the window they were actually looking at.
type Emitter = appevent.Emitter

// Deps is embedded by value into every bound service struct, matching src/main/ipc/deps.ts's
// IpcDeps shape as closely as each phase's scope allows. The deps.ts row also lists Secrets and
// Log; neither is added here: the cipher reaches the bridge through Connections.SecretsStatus()
// exactly as ipc/connections.ts:44 does today, and logging is slog.Default() (backed by
// internal/logging). internal/oplog and the metrics ticker have no bridge service yet so they
// are wired directly in main.go, not carried here.
type Deps struct {
	// Prompts resolves the editable LLM prompt texts.
	Prompts llmprompts.Source
	DB      *sql.DB
	// Home is the app data folder (KIRA_HOME).
	Home      string
	StartedAt int64 // unix millis, for appInfo-style diagnostics later

	Repos       *repos.Repos
	Connections *connections.Service
	// ConnectionFiles is the connections folder: the source of every connection's config.
	ConnectionFiles *connfiles.Loader
	Tree            *tree.Service
	Router          *adapterhost.Router // PushCacheConfig's Go-side half (bridge/settings.go); A17
	Events          Emitter
	// ApiVars (it resolves gRPC targets too) is the gated variable/history reveal and stage 2 of
	// the two-stage {{name}} substitution
	// (bridge/http.go's Send calls ResolveRequest directly; every other VariablesService method
	// wraps Repos.Variables instead, the same split CollectionsService already has between
	// Repos.Collections and nothing else).
	ApiVars *apivars.Service
	// MaskRules is M5's rule-management service (docs/HISTORY.md (v1.7 M5))
	// — the bridge's own Privacy-tab/header-menu surface, and (via dbmcp.Config.MaskRules) the DB
	// MCP server's masking seam.
	MaskRules *maskrules.Service
	// MaskReview is P534's column review state: the MCP gate's withheld columns and the Privacy tab's live list.
	MaskReview *maskreview.Service
}
