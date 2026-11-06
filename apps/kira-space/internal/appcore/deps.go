// Package appcore holds Deps, embedded by value into each bound service.
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
	// DB is the one application database.
	DB *sql.DB
	// Home is the app data folder (KIRA_SPACE_HOME).
	Home      string
	StartedAt int64 // unix millis, for appInfo-style diagnostics
	Events    Emitter
	// GitRegistry is internal/gitsession's own Registry — bridge/github.go's GitHubService reads
	// it to build its own known-GitHub-hosts allowlist. May be nil in a fixture that never wires the git module.
	GitRegistry *gitsession.Registry

	Connections *connections.Service
	// ConnectionFiles is the connections folder: the source of every connection's config.
	ConnectionFiles *connfiles.Loader
	Tree            *tree.Service
	Router          *adapterhost.Router
	// ApiVars (it resolves gRPC targets too) is the gated variable/history reveal and stage 2 of
	// the two-stage {{name}} substitution.
	ApiVars *apivars.Service
	// MaskRules is the rule-management service — the bridge's own Privacy-tab/header-menu surface,
	// and (via dbmcp.Config.MaskRules) the DB MCP server's masking seam.
	MaskRules *maskrules.Service
	// MaskReview is the column review state: the MCP gate's withheld columns and the Privacy tab's live list.
	MaskReview *maskreview.Service
}
