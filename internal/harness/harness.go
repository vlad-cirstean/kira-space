// Package harness is the provider-neutral surface of the coding agent Kira drives. A Provider
// implements role interfaces (structured calls, headless runs, interactive launches, MCP wiring,
// sandbox needs); callers depend on the narrowest role and never name an agent. Adding a provider
// means implementing Provider and registering it in internal/harness/providers.
package harness

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"

	"github.com/kirathecat/kira-studio/packages/theme/src/i18n"
)

// DefaultProvider is the provider used when none is configured or the configured one is unknown.
const DefaultProvider = "claude"

// Provider is one agent harness. ID is the agent.provider setting value. Structured and Headless
// are optional roles: reach them through StructuredOf and HeadlessOf.
type Provider interface {
	ID() string
	// Label is the display name in Go-rendered text ("Claude").
	Label() string
	// Command is the agent CLI's name as a user types it ("claude").
	Command() string
	// ProductLabel is the product's full name ("Claude Code").
	ProductLabel() string
	// HeadlessLabel is the command line of a background run as a user reads it ("claude -p").
	HeadlessLabel() string
	// ModelLabel is a model alias as the UI shows it.
	ModelLabel(alias string) string
	// Models is the provider's model aliases.
	Models() Catalog
	// StageSettings are the per-stage settings the provider takes; empty means none.
	StageSettings() []StageSetting
	Locator
	Interactive
	Sandboxed
	MCPHost
}

// Locator finds the provider's CLI.
type Locator interface {
	// Locate resolves the agent CLI the way a launch does (PATH, install paths, login shell). An
	// absolute bin is the only path tried (a test seam). A miss is the Kira not-found ipcerr.
	Locate(ctx context.Context, bin string) (string, error)
	// Available is the quick probe without the login-shell fallback.
	Available() error
}

// Catalog is a provider's model aliases; Default is used when none is chosen.
type Catalog struct {
	Aliases []string
	Default string
}

// Valid reports whether m is one of the aliases.
func (c Catalog) Valid(m string) bool { return slices.Contains(c.Aliases, m) }

// Registry holds the registered providers in registration order.
type Registry struct {
	byID map[string]Provider
	ids  []string
}

// NewRegistry registers ps in order. A duplicate ID panics: it is a programming error.
func NewRegistry(ps ...Provider) *Registry {
	r := &Registry{byID: map[string]Provider{}}
	for _, p := range ps {
		id := p.ID()
		if _, dup := r.byID[id]; dup {
			panic("harness: duplicate provider " + id)
		}
		r.byID[id] = p
		r.ids = append(r.ids, id)
	}
	return r
}

// Get returns the provider registered under id.
func (r *Registry) Get(id string) (Provider, bool) {
	p, ok := r.byID[id]
	return p, ok
}

// IDs lists the registered provider ids in registration order.
func (r *Registry) IDs() []string { return slices.Clone(r.ids) }

// Models is the union of every registered catalog in registration order, deduplicated, with the
// default provider's default (the first provider's when it is not registered).
func (r *Registry) Models() Catalog {
	var out Catalog
	for _, id := range r.ids {
		c := r.byID[id].Models()
		for _, a := range c.Aliases {
			if !slices.Contains(out.Aliases, a) {
				out.Aliases = append(out.Aliases, a)
			}
		}
		if out.Default == "" || id == DefaultProvider {
			out.Default = c.Default
		}
	}
	return out
}

// Selector resolves the active provider per call, so a settings change applies to the next call.
type Selector struct {
	Registry *Registry
	// ID reads agent.provider; nil means DefaultProvider.
	ID func() (string, error)
}

// Active falls back to DefaultProvider, with a warning, on a read error, "" or an unknown id.
func (s Selector) Active() Provider {
	id := DefaultProvider
	if s.ID != nil {
		got, err := s.ID()
		switch {
		case err != nil:
			slog.Warn("agent: read provider, using default", "scope", "harness", "err", err)
		case got != "":
			id = got
		}
	}
	if p, ok := s.Registry.Get(id); ok {
		return p
	}
	slog.Warn("agent: unknown provider, using default", "scope", "harness", "provider", id)
	p, ok := s.Registry.Get(DefaultProvider)
	if !ok {
		panic("harness: default provider " + DefaultProvider + " is not registered")
	}
	return p
}

// ErrProviderGone: the provider a stored conversation ran under is not registered any more.
var ErrProviderGone = errors.New("harness: provider gone")

// For resolves the provider a stored conversation ran under. "" means DefaultProvider, the id rows
// written before providers existed carry.
func (s Selector) For(id string) (Provider, error) {
	if id == "" {
		id = DefaultProvider
	}
	if p, ok := s.Registry.Get(id); ok {
		return p, nil
	}
	return nil, fmt.Errorf("%w: %s", ErrProviderGone, id)
}

// Failure kinds a provider reports; match with errors.Is.
var (
	ErrNotFound    = errors.New("harness: not found")
	ErrAuth        = errors.New("harness: auth")
	ErrBudget      = errors.New("harness: budget")
	ErrMaxTurns    = errors.New("harness: max turns")
	ErrTimeout     = errors.New("harness: timeout")
	ErrOutput      = errors.New("harness: output")
	ErrOutdated    = errors.New("harness: outdated")
	ErrRateLimited = errors.New("harness: rate limited")
	ErrUsageLimit  = errors.New("harness: usage limit")
	// ErrUnsupported: the provider does not implement an optional role.
	ErrUnsupported = errors.New("harness: unsupported")
)

// MissingRole is the refusal for a role the provider does not implement.
type MissingRole struct{ Agent, Role string }

func (e *MissingRole) Error() string {
	return i18n.T("backend.core.agent.missingRole.body", map[string]string{"agent": e.Agent, "role": e.Role})
}

// Is matches ErrUnsupported.
func (e *MissingRole) Is(target error) bool { return target == ErrUnsupported }

// StructuredOf is p's Structured role, if it has one.
func StructuredOf(p Provider) (Structured, bool) {
	s, ok := p.(Structured)
	return s, ok
}

// HeadlessOf is p's Headless role, if it has one.
func HeadlessOf(p Provider) (Headless, bool) {
	h, ok := p.(Headless)
	return h, ok
}

// Error is a provider's user-facing error: Msg is shown, Kind matches errors.Is.
type Error struct {
	Kind error
	Msg  string
}

func (e *Error) Error() string { return e.Msg }

// Unwrap lets errors.Is match Kind.
func (e *Error) Unwrap() error { return e.Kind }
