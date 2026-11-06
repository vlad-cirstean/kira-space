// Package mcpinstall registers MCP servers with Claude Code's own CLI — the one client the apps
// install for. Every real spawn Install makes is argv-only, never a shell, never writes another
// program's config file directly. Command is the one exception: it renders shell syntax (`;`,
// redirection) purely as copy-paste TEXT for the pane's fallback UI, never executed by this
// package itself.
package mcpinstall

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kirathecat/kira-studio/internal/harness"
	"github.com/kirathecat/kira-studio/internal/loginshell"
	"github.com/kirathecat/kira-studio/internal/toolexec"
)

// spawnTimeout is this package's own bound on the one spawn Install makes (`claude mcp add`) — a
// local, fast operation with no network round trip of its own, so a wedged child is the only
// failure mode a bound needs to guard against.
const spawnTimeout = 30 * time.Second

// Deps are the three independent seams this package needs: finding `claude`, checking a candidate
// exists, and spawning one. Function fields rather than an interface, so a test can inject each
// independently. A zero-value field falls back to the real OS implementation (New).
type Deps struct {
	LookPath func(string) (string, error)
	Stat     func(string) (os.FileInfo, error)
	Run      func(ctx context.Context, path string, args []string) error
	// RunEnv is Run with the environment a spawn gets, used when InstallJSON names one.
	RunEnv func(ctx context.Context, path string, args, env []string) error
	// Candidates are the absolute paths probed after PATH.
	Candidates func() []string
}

// Installer is this package's whole surface, constructed once (main.go) and shared for the app's
// life — every call re-resolves `claude` fresh, so an install that happens after Status was last
// read (installing the CLI after the pane rendered) still works on the next click.
type Installer struct {
	lookPath func(string) (string, error)
	stat     func(string) (os.FileInfo, error)
	run      func(ctx context.Context, path string, args []string) error
	runEnv   func(ctx context.Context, path string, args, env []string) error
	cands    func() []string
}

// New constructs an Installer over d, substituting the real OS implementation for any zero-value
// field.
func New(d Deps) *Installer {
	i := &Installer{lookPath: d.LookPath, stat: d.Stat, run: d.Run, runEnv: d.RunEnv, cands: d.Candidates}
	if i.lookPath == nil {
		i.lookPath = realLookPath
	}
	if i.stat == nil {
		i.stat = realStat
	}
	if i.run == nil {
		i.run = toolexec.Run
	}
	if i.runEnv == nil {
		i.runEnv = func(ctx context.Context, path string, args, env []string) error {
			_, err := toolexec.RunIO(ctx, path, args, "", env, nil)
			return err
		}
	}
	if i.cands == nil {
		i.cands = func() []string { return nil }
	}
	return i
}

// locateClaude checks PATH first, then every absolute candidate in order, every path considered
// recorded in probed regardless of outcome.
func (i *Installer) locateClaude() (path string, probed []string, found bool) {
	return toolexec.Locate(i.lookPath, i.stat, "claude", i.cands())
}

// Status is the pane's own pre-click read.
func (i *Installer) Status() harness.InstallStatus {
	path, probed, _ := i.locateClaude()
	return harness.InstallStatus{Path: path, Probed: probed}
}

// serverJSON is `claude mcp add-json`'s own input shape, restricted to the one field combination
// this package ever sends — Type/URL/HeadersHelper, never a plaintext Headers map (F2's whole
// point).
type serverJSON struct {
	Type          string `json:"type"`
	URL           string `json:"url"`
	HeadersHelper string `json:"headersHelper"`
	// Timeout is the per-call limit in ms; without it claude cuts an MCP call at 60 s.
	Timeout int64 `json:"timeout,omitempty"`
}

func httpServer(url, helperPath string) serverJSON {
	return serverJSON{Type: "http", URL: url, HeadersHelper: loginshell.Quote(helperPath), Timeout: harness.MCPCallTimeout.Milliseconds()}
}

// Command renders the exact two-step argv Install spawns (F3: remove, ignoring "not registered",
// then add-json) — shown with shell-style quoting for copy-paste rather than passed through a
// shell. No token anywhere in this string (F2): helperPath names a local script, never a secret
// itself, so unlike the old `--header "Authorization: Bearer <token>"` form this is safe to display
// and safe to have landed in a shell history.
//
// F10: headersHelper is shell-quoted here, the same way Install below quotes it, so the displayed
// command matches what Install actually sends — a helperPath containing a space (the app home
// or $HOME with one, common on macOS for a custom home) would otherwise break into two shell
// words once Claude Code later runs the stored headersHelper string through a shell of its own,
// silently sending no Authorization header and failing every call with an unexplained 401.
func (*Installer) Command(name, url, helperPath string) string {
	payload, _ := json.Marshal(httpServer(url, helperPath))
	return "claude mcp remove --scope user " + loginshell.Quote(name) + " 2>/dev/null; claude mcp add-json --scope user " +
		loginshell.Quote(name) + " " + loginshell.Quote(string(payload))
}

func detailFor(ctx context.Context, err error) string {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return "timed out"
	}
	var runErr *toolexec.ExecError
	if errors.As(err, &runErr) {
		return runErr.Error()
	}
	return err.Error()
}

// isNotRegisteredError reports whether err is `claude mcp remove`'s own "nothing to remove" outcome
// (verified against the installed CLI, 2.1.280: exit 1, stderr `No MCP server named "<name>" in
// user scope`) — the one remove failure Install must treat as success rather than propagate (F3).
func isNotRegisteredError(err error) bool {
	var runErr *toolexec.ExecError
	if !errors.As(err, &runErr) {
		return false
	}
	return strings.Contains(runErr.Stderr, "No MCP server named")
}

// Install re-resolves `claude` fresh and never returns a Go error — every outcome is a named
// Result value. helperPath is EnsureHeaderHelperScript's own
// return value — the local script's path, never a token.
//
// Removes any existing registration under name first: add-json fails outright on a name
// collision, and the headersHelper design means a token rotation never needs re-registration at
// all — but the Install button itself can still be clicked again (a stale registration from an
// older app version's `--header` form, or re-running it), so this stays idempotent
// regardless. A "nothing to remove" outcome is not a failure and is never surfaced as one.
func (i *Installer) Install(ctx context.Context, name, url, helperPath string) harness.InstallResult {
	// F10: a relative helperPath resolves against Claude Code's own cwd, not Kira's — reachable
	// when os.UserHomeDir fails and kirapaths.Home falls back to a relative path. Refuse outright
	// rather than register a headersHelper that silently reads the wrong file (or nothing).
	if !filepath.IsAbs(helperPath) {
		return harness.InstallResult{Outcome: harness.OutcomeInstallFailed, Detail: "internal error: header helper path is not absolute"}
	}

	// Claude Code runs the stored headersHelper string through a shell of its own at
	// connection time — an unquoted path containing a space (or any other shell metacharacter)
	// splits into more than one word there, so no Authorization header is ever sent and every call
	// gets a 401 with no hint why. Single-quoting here is the fix; EnsureHeaderHelperScript already
	// quotes the token path the same way for the same reason, one level down.
	return i.register(ctx, name, httpServer(url, helperPath), nil)
}

// stdioServerJSON is add-json's shape for a stdio server: a command and its argv.
type stdioServerJSON struct {
	Type    string   `json:"type"`
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

// StdioCommand renders the same remove-then-add-json pair for a stdio server, as copy-paste text.
func (*Installer) StdioCommand(name, command string, args []string) string {
	payload, _ := json.Marshal(stdioServerJSON{Type: "stdio", Command: command, Args: args})
	return "claude mcp remove --scope user " + loginshell.Quote(name) + " 2>/dev/null; claude mcp add-json --scope user " +
		loginshell.Quote(name) + " " + loginshell.Quote(string(payload))
}

// InstallStdio registers a stdio MCP server with Claude Code, replacing any registration of name.
func (i *Installer) InstallStdio(ctx context.Context, name, command string, args []string) harness.InstallResult {
	if !filepath.IsAbs(command) {
		return harness.InstallResult{Outcome: harness.OutcomeInstallFailed, Detail: "internal error: server command path is not absolute"}
	}
	return i.register(ctx, name, stdioServerJSON{Type: "stdio", Command: command, Args: args}, nil)
}

// InstallJSON registers a server from its own add-json document, replacing any registration of name.
// extraEnv is added to the process environment of both spawns (CLAUDE_CONFIG_DIR, so the CLI edits the
// file a caller backed up). A caller never puts a secret value in server: references only.
func (i *Installer) InstallJSON(ctx context.Context, name string, server json.RawMessage, extraEnv []string) harness.InstallResult {
	return i.register(ctx, name, server, extraEnv)
}

// register is the shared remove-then-add-json path.
func (i *Installer) register(ctx context.Context, name string, server any, extraEnv []string) harness.InstallResult {
	claudePath, probed, found := i.locateClaude()
	if !found {
		return harness.InstallResult{Outcome: harness.OutcomeNotFound, Probed: probed}
	}

	spawnCtx, cancel := context.WithTimeout(ctx, spawnTimeout)
	defer cancel()

	run := i.run
	if len(extraEnv) > 0 {
		run = func(ctx context.Context, path string, args []string) error {
			return i.runEnv(ctx, path, args, append(os.Environ(), extraEnv...))
		}
	}
	removeArgs := []string{"mcp", "remove", "--scope", "user", name}
	if err := run(spawnCtx, claudePath, removeArgs); err != nil && !isNotRegisteredError(err) {
		return harness.InstallResult{Outcome: harness.OutcomeInstallFailed, Detail: detailFor(spawnCtx, err), Probed: probed}
	}

	payload, err := json.Marshal(server)
	if err != nil {
		return harness.InstallResult{Outcome: harness.OutcomeInstallFailed, Detail: err.Error(), Probed: probed}
	}
	addArgs := []string{"mcp", "add-json", "--scope", "user", name, string(payload)}
	if err := run(spawnCtx, claudePath, addArgs); err != nil {
		return harness.InstallResult{Outcome: harness.OutcomeInstallFailed, Detail: detailFor(spawnCtx, err), Probed: probed}
	}
	return harness.InstallResult{Outcome: harness.OutcomeInstalled, Probed: probed}
}
