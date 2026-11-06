// Package appcore holds the Deps structs embedded by value into each bound service: Deps for the
// Space services, StudioDeps for the Studio services (connections, adapters, variables, mask
// rules) until the DB merge folds them.
package appcore

import (
	"database/sql"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapterhost"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/apivars"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/connections"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/connfiles"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitsession"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/maskreview"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/maskrules"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/repos"
	studiorepos "github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/repos"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/tree"
	"github.com/kirathecat/kira-studio/internal/appevent"
	"github.com/kirathecat/kira-studio/internal/llmprompts"
)

// Emitter is the Go->renderer push seam — repo-root internal/appevent.Emitter (identical in both
// apps). internal/shell implements it over *application.App's Event.Emit/DispatchWailsEvent;
// bridge stays the only real consumer package, so no bridge file has to import Wails.
type Emitter = appevent.Emitter

// Deps is embedded by value into every bound service struct.
type Deps struct {
	// Prompts resolves the editable LLM prompt texts.
	Prompts llmprompts.Source
	Repos   *repos.Repos
	// Home is the app data folder (KIRA_SPACE_HOME).
	Home   string
	Events Emitter
	// GitRegistry is internal/gitsession's own Registry — bridge/github.go's GitHubService reads
	// it to build its own known-GitHub-hosts allowlist (the same seam Kira Studio's own Deps
	// carries it for). May be nil in a fixture that never wires the git module.
	GitRegistry *gitsession.Registry
}

// StudioDeps is the Studio-side sibling of Deps, embedded by value into every bound service struct, matching src/main/ipc/deps.ts's
// IpcDeps shape as closely as each phase's scope allows. The deps.ts row also lists Secrets and
// Log; neither is added here: the cipher reaches the bridge through Connections.SecretsStatus()
// exactly as ipc/connections.ts:44 does today, and logging is slog.Default() (backed by
// internal/logging). internal/oplog and the metrics ticker have no bridge service yet so they
// are wired directly in main.go, not carried here.
type StudioDeps struct {
	// Prompts resolves the editable LLM prompt texts.
	Prompts llmprompts.Source
	DB      *sql.DB
	// Home is the app data folder (KIRA_HOME).
	Home      string
	StartedAt int64 // unix millis, for appInfo-style diagnostics later

	Repos       *studiorepos.Repos
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
