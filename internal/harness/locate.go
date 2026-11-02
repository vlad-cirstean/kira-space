package harness

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/kirathecat/kira-studio/internal/ipcerr"
	"github.com/kirathecat/kira-studio/internal/toolexec"
)

// ErrCLIMissing means an agent CLI was found nowhere a launch could reach it.
var ErrCLIMissing = errors.New("agent CLI not found")

const shellProbeTimeout = 5 * time.Second

// CLISpec names the CLI ResolveCLI looks for.
type CLISpec struct {
	// Name is the command ("claude").
	Name string
	// Candidates are the absolute paths probed after PATH.
	Candidates []string
	// Product is the product's full name for the not-found message ("Claude Code").
	Product string
}

// CLIDeps are ResolveCLI's seams; the zero value uses the real process environment.
type CLIDeps struct {
	// Bin, when absolute, is the only path considered (a test points it at a fake).
	Bin      string
	LookPath func(string) (string, error)
	Stat     func(string) (os.FileInfo, error)
	Getenv   func(string) string
	// RunShell runs script under the shell as a login, interactive shell and returns its stdout.
	RunShell func(ctx context.Context, shell, script string) ([]byte, error)
}

func (d CLIDeps) withDefaults() CLIDeps {
	if d.LookPath == nil {
		d.LookPath = exec.LookPath
	}
	if d.Stat == nil {
		d.Stat = os.Stat
	}
	if d.Getenv == nil {
		d.Getenv = os.Getenv
	}
	if d.RunShell == nil {
		d.RunShell = func(ctx context.Context, shell, script string) ([]byte, error) {
			return toolexec.RunIO(ctx, shell, []string{"-l", "-i", "-c", script}, "", os.Environ(), nil)
		}
	}
	return d
}

// ResolveCLI finds spec's CLI: PATH and the candidate install paths first, then the user's login
// shell, because a launch runs through that shell and a GUI app's own PATH can differ from it.
// probed lists every place considered. A miss returns ErrCLIMissing.
func ResolveCLI(ctx context.Context, spec CLISpec, deps CLIDeps) (path string, probed []string, err error) {
	deps = deps.withDefaults()
	if filepath.IsAbs(deps.Bin) {
		if toolexec.IsExecutable(deps.Stat, deps.Bin) {
			return deps.Bin, []string{deps.Bin}, nil
		}
		return "", []string{deps.Bin}, ErrCLIMissing
	}
	path, probed, found := toolexec.Locate(deps.LookPath, deps.Stat, spec.Name, spec.Candidates)
	if found {
		return path, probed, nil
	}
	shell := deps.Getenv("SHELL")
	if !filepath.IsAbs(shell) || !toolexec.IsExecutable(deps.Stat, shell) {
		return "", probed, ErrCLIMissing
	}
	probed = append(probed, spec.Name+" (in "+shell+" login shell)")
	sctx, cancel := context.WithTimeout(ctx, shellProbeTimeout)
	defer cancel()
	out, _ := deps.RunShell(sctx, shell, "command -v "+spec.Name)
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if filepath.IsAbs(line) && toolexec.IsExecutable(deps.Stat, line) {
			return line, probed, nil
		}
	}
	return "", probed, ErrCLIMissing
}

// CLIMissing is the user-facing error for a failed ResolveCLI.
func CLIMissing(spec CLISpec, probed []string) *ipcerr.Error {
	return ipcerr.T(ipcerr.CodeNotFound, "backend.core.agent.missing.body", map[string]string{"product": spec.Product, "probed": strings.Join(probed, ", ")})
}

// RequireCLI resolves the CLI and returns the Kira error when it is missing.
func RequireCLI(ctx context.Context, spec CLISpec, deps CLIDeps) (string, error) {
	path, probed, err := ResolveCLI(ctx, spec, deps)
	if err != nil {
		if errors.Is(err, ErrCLIMissing) {
			return "", CLIMissing(spec, probed)
		}
		return "", err
	}
	return path, nil
}
