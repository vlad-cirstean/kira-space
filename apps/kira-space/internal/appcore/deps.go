// Package appcore is Kira Space's own Deps struct — the trimmed sibling of Kira
// Studio's own apps/kira-space/internal/appcore, embedded by value into each bound service the
// same way. Kira Studio's Deps carries eight fields across connections/adapters/variables/mask
// rules/git; this app has no adapters, no connections, no HTTP variables — Repos, Events and
// GitRegistry are the whole surface the bridge services embedding Deps read.
package appcore

import (
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitsession"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/repos"
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
