package opencode

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/kirathecat/kira-studio/internal/harness"
	"github.com/kirathecat/kira-studio/internal/toolexec"
)

var systemPaths = append([]string(nil), systemCandidates...)

// SetSystemPaths replaces the system-wide candidates and returns the restore func. A flow test
// passes none so the host's own install cannot change its outcome.
func SetSystemPaths(paths ...string) (restore func()) {
	prev := systemPaths
	systemPaths = paths
	return func() { systemPaths = prev }
}

// Candidates is the probe order after PATH: the home installs, then the system paths.
func Candidates() []string {
	var out []string
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		for _, rel := range homeCandidates {
			out = append(out, filepath.Join(home, filepath.FromSlash(rel)))
		}
	}
	return append(out, systemPaths...)
}

func spec() harness.CLISpec {
	return harness.CLISpec{Name: "opencode", Candidates: Candidates(), Product: "OpenCode"}
}

// Locate resolves the opencode CLI the way a launch does. An absolute bin is the only path tried.
func (*Provider) Locate(ctx context.Context, bin string) (string, error) {
	return harness.RequireCLI(ctx, spec(), harness.CLIDeps{Bin: bin})
}

// Available is the quick probe without the login-shell fallback.
func (p *Provider) Available() error {
	if _, _, found := p.find(); !found {
		return harness.CLIMissing(spec(), nil)
	}
	return nil
}

func (p *Provider) find() (string, []string, bool) {
	lookPath, stat := p.lookPath, p.stat
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	if stat == nil {
		stat = os.Stat
	}
	return toolexec.Locate(lookPath, stat, "opencode", Candidates())
}
