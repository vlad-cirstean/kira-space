// Package providers is the one place providers are registered.
package providers

import (
	"path/filepath"

	"github.com/kirathecat/kira-studio/internal/harness"
	"github.com/kirathecat/kira-studio/internal/harness/claude"
	"github.com/kirathecat/kira-studio/internal/harness/opencode"
)

// New builds the registry: Claude, OpenCode, then extra (a test or a later provider).
func New(extra ...harness.Provider) *harness.Registry {
	return harness.NewRegistry(append([]harness.Provider{claude.New(), opencode.New()}, extra...)...)
}

// StageSet is one provider's stage settings.
type StageSet struct {
	ID, Label string
	Settings  []harness.StageSetting
}

// StageCatalog lists every registered provider's stage settings in registration order; providers with none are left out.
func StageCatalog() []StageSet {
	var out []StageSet
	reg := New()
	for _, id := range reg.IDs() {
		p, _ := reg.Get(id)
		if s := p.StageSettings(); len(s) > 0 {
			out = append(out, StageSet{ID: id, Label: p.Label(), Settings: s})
		}
	}
	return out
}

// ConfigDirs are the home config folder names (".claude") of every registered provider. The set is
// static, so validation that runs without a registry can guard them.
func ConfigDirs() []string {
	const home = "/~home"
	var out []string
	reg := New()
	for _, id := range reg.IDs() {
		p, _ := reg.Get(id)
		for _, d := range p.SandboxNeeds(home, "").ConfigDirs {
			if rel, err := filepath.Rel(home, d); err == nil {
				out = append(out, rel)
			}
		}
	}
	return out
}
