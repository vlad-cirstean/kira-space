// Package agentsvc is the bound read of the agent provider's display facts: the names, models and
// optional roles the UI renders. Each app embeds Service in a thin bridge type, so binding names
// stay app-local.
package agentsvc

import (
	"github.com/kirathecat/kira-studio/internal/harness"
)

// ModelOption is one model alias with its display label.
type ModelOption struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// Info is what the UI needs to name and configure one provider.
type Info struct {
	ID            string        `json:"id"`
	Label         string        `json:"label"`
	ProductLabel  string        `json:"productLabel"`
	Command       string        `json:"command"`
	HeadlessLabel string        `json:"headlessLabel"`
	Models        []ModelOption `json:"models"`
	DefaultModel  string        `json:"defaultModel"`
	// Hooks: interactive sessions report events (needs-input, finished).
	Hooks bool `json:"hooks"`
	// Usage: the provider reports account rate-limit windows.
	Usage bool `json:"usage"`
}

// Service reads the active provider, or a stored conversation's, per call.
type Service struct {
	Agents harness.Selector
}

// Info describes the active provider.
func (s *Service) Info() (Info, error) { return describe(s.Agents.Active()), nil }

// InfoFor describes the provider a stored conversation ran under ("" is the default provider).
func (s *Service) InfoFor(id string) (Info, error) {
	p, err := s.Agents.For(id)
	if err != nil {
		return Info{}, err
	}
	return describe(p), nil
}

// Entry is one registered provider with whether its CLI is installed.
type Entry struct {
	Info
	Available bool `json:"available"`
}

// List describes every registered provider in registration order. Available is the quick probe
// without the login-shell fallback.
func (s *Service) List() ([]Entry, error) {
	reg := s.Agents.Registry
	ids := reg.IDs()
	out := make([]Entry, 0, len(ids))
	for _, id := range ids {
		p, _ := reg.Get(id)
		out = append(out, Entry{Info: describe(p), Available: p.Available() == nil})
	}
	return out, nil
}

func describe(p harness.Provider) Info {
	cat := p.Models()
	models := make([]ModelOption, 0, len(cat.Aliases))
	for _, a := range cat.Aliases {
		models = append(models, ModelOption{ID: a, Label: p.ModelLabel(a)})
	}
	_, hooks := p.(harness.HookHost)
	return Info{
		ID: p.ID(), Label: p.Label(), ProductLabel: p.ProductLabel(), Command: p.Command(),
		HeadlessLabel: p.HeadlessLabel(), Models: models, DefaultModel: cat.Default,
		Hooks: hooks, Usage: harness.ReportsUsage(p),
	}
}
