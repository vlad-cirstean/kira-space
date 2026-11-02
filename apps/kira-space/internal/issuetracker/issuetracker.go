// Package issuetracker is the provider-neutral contract for an issue tracker (Jira, ...). A provider
// keeps its own CLI and credentials; Kira holds none. Callers depend on this package only; composition
// roots import issuetracker/providers.
package issuetracker

import (
	"context"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/clisetup"
)

// Ref names one issue.
type Ref struct {
	Provider string `json:"provider"`
	Key      string `json:"key"`  // PROJ-12
	URL      string `json:"url"`  // browse URL; '' for a bare key until the provider fills it from its signed-in site
	Site     string `json:"site"` // host of a pasted URL; '' for a bare key
}

// ChangeRequestSummary is the tracker's own count of associated change requests.
type ChangeRequestSummary struct {
	Known     bool     `json:"known"` // the tracker reported a summary at all
	Count     int      `json:"count"`
	State     string   `json:"state"` // lower-cased overall state: open | merged | declined | ''
	Providers []string `json:"providers"`
}

// Issue is what a tracker reports for one issue.
type Issue struct {
	Ref            Ref
	Title          string
	Status         string // workflow status name
	StatusCategory string // new | indeterminate | done | ''
	Summary        ChangeRequestSummary
}

// Status kinds, the same words as githost.Status.
const (
	KindOK              = "ok"
	KindNotFound        = "notFound"
	KindUnauthenticated = "unauthenticated"
)

// Status is a tracker CLI's discovery and sign-in state.
type Status struct {
	Kind    string   `json:"kind"`
	Path    string   `json:"path,omitempty"`
	Version string   `json:"version,omitempty"`
	Account string   `json:"account,omitempty"`
	Site    string   `json:"site,omitempty"`
	Reason  string   `json:"reason,omitempty"`
	Probed  []string `json:"probed,omitempty"`
}

// OK reports whether the CLI is installed and signed in.
func (s Status) OK() bool { return s.Kind == KindOK }

// Terms is a provider's display text.
type Terms struct {
	Label string `json:"label"` // "Jira"
	Issue string `json:"issue"` // "Work item"
	CLI   string `json:"cli"`   // "Atlassian CLI (acli)"
}

// Info is what the UI needs to name and set up one provider.
type Info struct {
	ID string `json:"id"`
	Terms
	Setup clisetup.Setup `json:"setup"`
}

// Provider is one issue tracker.
type Provider interface {
	// ID is the registry key, e.g. "jira".
	ID() string
	Terms() Terms
	Setup() clisetup.Setup
	// ParseRef recognises a bare key or a pasted issue URL. It makes no CLI call.
	ParseRef(raw string) (Ref, bool)
	// Status probes the CLI. refresh drops the cached probe first.
	Status(ctx context.Context, refresh bool) Status
	// Issue reads one issue. Failures are the typed errors of errors.go, so a caller can name the gap.
	Issue(ctx context.Context, ref Ref) (Issue, error)
}
