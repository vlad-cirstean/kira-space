package claude

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/kirathecat/kira-studio/internal/harness"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
	"github.com/kirathecat/kira-studio/internal/toolexec"
)

// ErrMissing means no `claude` CLI was found anywhere a launch could reach it.
var ErrMissing = harness.ErrCLIMissing

// Deps are Resolve's seams; the zero value uses the real process environment.
type Deps = harness.CLIDeps

func spec() harness.CLISpec {
	return harness.CLISpec{Name: "claude", Candidates: Candidates(), Product: "Claude Code"}
}

// Resolve finds the `claude` CLI through harness.ResolveCLI. A miss returns ErrMissing.
func Resolve(ctx context.Context, deps Deps) (path string, probed []string, err error) {
	return harness.ResolveCLI(ctx, spec(), deps)
}

// Missing is the user-facing error for a failed Resolve.
func Missing(probed []string) *ipcerr.Error { return harness.CLIMissing(spec(), probed) }

// Require resolves the CLI and returns the Kira error when it is missing.
func Require(ctx context.Context, deps Deps) (string, error) {
	return harness.RequireCLI(ctx, spec(), deps)
}

var systemPaths = []string{"/usr/local/bin/claude", "/opt/homebrew/bin/claude"}

// SetSystemPaths replaces the system-wide candidates and returns the restore func. A flow test
// passes none so the host's own install cannot change its outcome.
func SetSystemPaths(paths ...string) (restore func()) {
	prev := systemPaths
	systemPaths = paths
	return func() { systemPaths = prev }
}

// Candidates is the probe order after PATH for the `claude` CLI: the well-known absolute paths a
// Finder-launched app's launchd-inherited PATH (/usr/bin:/bin:/usr/sbin:/sbin) never includes.
func Candidates() []string {
	candidates := append([]string(nil), systemPaths...)
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		candidates = append([]string{
			filepath.Join(home, ".claude", "local", "claude"),
			filepath.Join(home, ".local", "bin", "claude"),
		}, candidates...)
	}
	return candidates
}

// Locate resolves the claude CLI the way a launch does. An absolute bin is the only path tried.
func (*Provider) Locate(ctx context.Context, bin string) (string, error) {
	return Require(ctx, Deps{Bin: bin})
}

// Available is the quick probe without the login-shell fallback.
func (p *Provider) Available() error {
	if _, _, found := p.find(); !found {
		return errNotFound
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
	return toolexec.Locate(lookPath, stat, "claude", Candidates())
}
