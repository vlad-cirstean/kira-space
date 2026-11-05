// Package agenthooks is a repo-root shared package: a per-process HTTP-over-unix-socket listener
// that a launched `claude` process's own lifecycle hooks report into. Go's internal/ visibility
// rule makes this
// importable from any app under apps/ but never the reverse, so it must not (and, structurally,
// cannot) import an apps/<app>/internal/... package such as internal/bridge — a host's own bridge
// package is the one place that turns Options.OnEvent's callback into a push-channel event and
// this package's plain error into an ipcerr response. Manager (manager.go) is the host's own
// entry point for lifecycle and launch composition; nothing here reads settings or knows which
// app is hosting it.
package hooks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"github.com/google/renameio/v2"
	"github.com/kirathecat/kira-studio/internal/harness"
	"github.com/kirathecat/kira-studio/internal/localsock"
)

// closeHTTPGraceTimeout mirrors internal/dbmcp/http.go's own bound on Close's graceful Shutdown
// wait — a hook's own request handler never blocks on anything but decoding a bounded body, so
// this is a generous backstop, not a value tuned against a known slow path.
const closeHTTPGraceTimeout = 5 * time.Second

// Server owns, for its own lifetime: a 0700 temp directory, the generated hooks.json, the
// shim script, a 0600 unix socket and the http.Server serving POST /hook over it. One
// instance per enable — internal/bridge/agenthooks.go's AgentHooksService constructs and closes
// one each time the setting toggles on, mirroring internal/dbmcp.Server's own start/stop shape.
type Server struct {
	hooksPath string
	// quotedHooksPath is hooksPath, single-quoted once here so Server.Compose has no error
	// path of its own (§2.1's own design note): hooks.json shares ln.Dir with the shim, whose
	// quoting New already succeeds on above, so quoting this path can never fail either.
	quotedHooksPath string
	shimPath        string
	quotedShim      string
	onEvent         func(harness.AgentEvent)
	onStatusLine    func(string, harness.RateLimits)
	statusLine      func() bool

	ln   *localsock.Listener
	http *http.Server
}

// New resolves curl, builds the temp directory/shim/socket/hooks.json, starts serving in its own
// goroutine, and returns. A failure here is never fatal to the caller — AgentHooksService.
// SetEnabled/startLocked surfaces it as AgentHooksStatus.Error, the same "enabling fails
// loudly, nothing silently degrades" posture DbMcpService already takes for a bind failure.
func New(opts harness.HookOptions) (*Server, error) {
	curlPath, err := exec.LookPath("curl")
	if err != nil {
		return nil, errors.New("curl not found; hooks cannot report")
	}

	// localsock.Listen is D8's own security boundary: no other OS user can read the shim, the
	// token or reach the socket (P107 T2-9).
	ln, err := localsock.Listen(localsock.Options{DirPrefix: "kira-agent-", TokenBytes: 32})
	if err != nil {
		return nil, fmt.Errorf("claude hooks: %w", err)
	}

	shimBody, err := buildShim(curlPath, ln.SockPath)
	if err != nil {
		_ = ln.Close()
		return nil, err
	}
	shimPath := filepath.Join(ln.Dir, "hook")
	if err := renameio.WriteFile(shimPath, []byte(shimBody), 0o700); err != nil {
		_ = ln.Close()
		return nil, fmt.Errorf("claude hooks: write shim: %w", err)
	}

	quotedShim, err := shellSingleQuote(shimPath)
	if err != nil {
		_ = ln.Close()
		return nil, err
	}
	hooksDoc, err := buildHooksDocument(quotedShim, nil)
	if err != nil {
		_ = ln.Close()
		return nil, err
	}
	hooksPath := filepath.Join(ln.Dir, "hooks.json")
	if err := renameio.WriteFile(hooksPath, hooksDoc, 0o600); err != nil {
		_ = ln.Close()
		return nil, fmt.Errorf("claude hooks: write hooks.json: %w", err)
	}
	quotedHooksPath, err := shellSingleQuote(hooksPath)
	if err != nil {
		_ = ln.Close()
		return nil, err
	}

	s := &Server{
		hooksPath: hooksPath, quotedHooksPath: quotedHooksPath, shimPath: shimPath,
		quotedShim: quotedShim, onEvent: opts.OnEvent, onStatusLine: opts.OnStatusLine, statusLine: opts.StatusLine,
		ln: ln,
	}
	s.http = &http.Server{
		Handler: s.mux(),
		// Same bounds as internal/dbmcp/http.go's own bindHTTP — a slowloris-shaped header and a
		// connection that never issues a second request, applied to this socket instead of a TCP
		// listener.
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	go func() {
		if err := s.http.Serve(s.ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			_ = err // Serve's own error after Close is expected (listener closed); nothing to log.
		}
	}()

	return s, nil
}

// Status reports the running listener.
func (s *Server) Status() harness.HookStatus {
	return harness.HookStatus{Running: true, SettingsPath: s.hooksPath}
}

// MCPToolTimeoutEnv lifts the CLI's 60 s limit on one MCP tool call: a tool that waits for the user, such
// as Kira Space's request_branches, holds its call open for up to 30 minutes.
var MCPToolTimeoutEnv = "MCP_TOOL_TIMEOUT=" + strconv.FormatInt(harness.MCPCallTimeout.Milliseconds(), 10)

// Compose returns command with " --settings '<hooks.json>'" appended and the hook env vars a
// launch's shell needs (Env). With the statusline option on, the settings file is per terminal. The
// env carries KIRA_AGENT_HOOK_TOKEN in plain text: never return it from a bound method. Which
// launches get hooks at all (an agent launch, never a plain shell or script) is the host's call, made
// before this is reached.
func (s *Server) Compose(terminalID, _, command string) (string, []string) {
	env := s.Env(terminalID)
	if s.statusLine != nil && s.statusLine() {
		quoted, err := s.composeStatusLine(terminalID)
		if err == nil {
			return command + " --settings " + quoted, env
		}
		slog.Warn("agent hooks: statusline settings", "scope", "ade", "err", err)
	}
	return command + " --settings " + s.quotedHooksPath, env
}

// ComposeInto adds Kira's hooks to the settings file Kira wrote for this launch (the app's agent
// settings), in place: Kira's entry comes first in every event's array and the user's entries follow.
// With the statusline option on, the file's own statusLine becomes the user statusline the wrapper
// runs. It returns command unchanged: the file is already a --settings flag. A read or write error
// fails the launch, since Kira's hooks are always on.
func (s *Server) ComposeInto(terminalID, _, command, configFile string) (string, []string, error) {
	raw, err := os.ReadFile(configFile)
	if err != nil {
		return "", nil, fmt.Errorf("claude hooks: read settings: %w", err)
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(raw, &doc); err != nil {
		return "", nil, fmt.Errorf("claude hooks: decode settings: %w", err)
	}
	userHooks := map[string][]json.RawMessage{}
	if h, ok := doc["hooks"]; ok {
		if err := json.Unmarshal(h, &userHooks); err != nil {
			return "", nil, fmt.Errorf("claude hooks: decode settings hooks: %w", err)
		}
	}
	kira, err := json.Marshal(hookMatcherEntry{Hooks: []hookCommand{{Type: "command", Command: s.quotedShim, Timeout: hookCommandTimeoutSeconds}}})
	if err != nil {
		return "", nil, fmt.Errorf("claude hooks: encode hook: %w", err)
	}
	for _, event := range hookEvents {
		userHooks[event] = append([]json.RawMessage{kira}, userHooks[event]...)
	}
	if doc["hooks"], err = json.Marshal(userHooks); err != nil {
		return "", nil, fmt.Errorf("claude hooks: encode hooks: %w", err)
	}
	env := s.Env(terminalID)
	if s.statusLine != nil && s.statusLine() {
		entry, extra := s.wrapStatusLine(userStatusLine(doc["statusLine"]))
		if doc["statusLine"], err = json.Marshal(entry); err != nil {
			return "", nil, fmt.Errorf("claude hooks: encode statusline: %w", err)
		}
		env = append(env, extra...)
	}
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return "", nil, fmt.Errorf("claude hooks: encode settings: %w", err)
	}
	if err := renameio.WriteFile(configFile, out, 0o600); err != nil {
		return "", nil, fmt.Errorf("claude hooks: write settings: %w", err)
	}
	return command, env, nil
}

// SettingsPath is the generated hooks.json's own absolute path — internal/bridge/terminal.go's
// `--settings` flag value, and the Settings section's own "outside your project" proof (§9.3).
func (s *Server) SettingsPath() string {
	return s.hooksPath
}

// Env is the three variables a Claude Code launch's shell needs to reach this server (§5.4):
// KIRA_TERMINAL_ID correlates every hook back to terminalID's own tab (§5.3), the other two point
// the shim at this instance's own socket and token.
func (s *Server) Env(terminalID string) []string {
	return []string{
		"KIRA_TERMINAL_ID=" + terminalID,
		"KIRA_AGENT_HOOK_SOCKET=" + s.ln.SockPath,
		"KIRA_AGENT_HOOK_TOKEN=" + s.ln.Token,
		MCPToolTimeoutEnv,
	}
}

// Close shuts the HTTP server down with a bounded context (dbmcp/http.go's own closeHTTP
// precedent) and removes the whole temp directory — the shim, hooks.json and the socket file
// along with it. A running Claude Code session whose hooks now post into a closed socket is
// unaffected: the shim always exits 0.
func (s *Server) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), closeHTTPGraceTimeout)
	defer cancel()
	var err error
	if shutdownErr := s.http.Shutdown(ctx); shutdownErr != nil {
		_ = s.http.Close()
		err = shutdownErr
	}
	if rmErr := os.RemoveAll(s.ln.Dir); err == nil {
		err = rmErr
	}
	return err
}
