// Package hooks is the OpenCode session-event listener: an HTTP-over-unix-socket server that a
// small Kira plugin, loaded into a launched `opencode` through its config, reports into.
package hooks

import (
	"context"
	"crypto/subtle"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/renameio/v2"

	"github.com/kirathecat/kira-studio/internal/harness"
	"github.com/kirathecat/kira-studio/internal/localsock"
)

//go:embed plugin.js
var pluginJS []byte

const (
	closeHTTPGraceTimeout = 5 * time.Second
	// maxHookPayloadBytes bounds one request body; the plugin sends a few short fields.
	maxHookPayloadBytes = 64 * 1024
	maxHookMessageBytes = 200
)

// hookEvents maps the plugin's event type to the neutral kind (U12, UNVERIFIED); a type not listed
// is dropped.
var hookEvents = map[string]harness.EventKind{
	"session.created":     harness.EventSessionStart,
	"session.idle":        harness.EventTurnEnd,
	"session.deleted":     harness.EventSessionEnd,
	"tool.execute.before": harness.EventToolStart,
	"tool.execute.after":  harness.EventToolEnd,
	"permission.updated":  harness.EventNotification,
	"message.updated":     harness.EventPrompt,
}

// hookRequest is the body the plugin posts: only these fields are ever read.
type hookRequest struct {
	Type      string `json:"type"`
	SessionID string `json:"sessionID"`
	Tool      string `json:"tool"`
	Message   string `json:"message"`
}

// Server owns a 0700 temp directory holding the plugin and the 0600 socket, and the http.Server
// serving POST /hook over it. Wakes is never set: OpenCode has no self-wake tool.
type Server struct {
	pluginPath string
	onEvent    func(harness.AgentEvent)
	ln         *localsock.Listener
	http       *http.Server
}

// New writes the plugin, binds the socket and starts serving.
func New(opts harness.HookOptions) (*Server, error) {
	ln, err := localsock.Listen(localsock.Options{DirPrefix: "kira-agent-", TokenBytes: 32})
	if err != nil {
		return nil, fmt.Errorf("opencode hooks: %w", err)
	}
	pluginPath := filepath.Join(ln.Dir, "plugin.js")
	if err := renameio.WriteFile(pluginPath, pluginJS, 0o600); err != nil {
		_ = ln.Close()
		return nil, fmt.Errorf("opencode hooks: write plugin: %w", err)
	}
	s := &Server{pluginPath: pluginPath, onEvent: opts.OnEvent, ln: ln}
	mux := http.NewServeMux()
	mux.HandleFunc("/hook", s.handleHook)
	s.http = &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 120 * time.Second}
	go func() { _ = s.http.Serve(s.ln) }()
	return s, nil
}

// Status reports the running listener; SettingsPath is the plugin file.
func (s *Server) Status() harness.HookStatus {
	return harness.HookStatus{Running: true, SettingsPath: s.pluginPath}
}

// Compose returns command unchanged and the env that loads the plugin and points it at this
// server. OPENCODE_CONFIG_CONTENT belongs to hooks alone on interactive launches. The env carries
// the token: never return it from a bound method.
func (s *Server) Compose(terminalID, _, command string) (string, []string) {
	content, _ := json.Marshal(map[string]any{"plugin": []string{"file://" + s.pluginPath}})
	return command, []string{
		"KIRA_TERMINAL_ID=" + terminalID,
		"KIRA_AGENT_HOOK_SOCKET=" + s.ln.SockPath,
		"KIRA_AGENT_HOOK_TOKEN=" + s.ln.Token,
		"OPENCODE_CONFIG_CONTENT=" + string(content),
	}
}

// Close shuts the server down and removes the temp directory.
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

// handleHook checks the token, then the terminal header, then reads the bounded body, so a request
// without the token never reaches the body.
func (s *Server) handleHook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	token, _ := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if subtle.ConstantTimeCompare([]byte(token), []byte(s.ln.Token)) != 1 {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	terminalID := r.Header.Get("X-Kira-Terminal")
	if terminalID == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	var req hookRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxHookPayloadBytes)).Decode(&req); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			w.WriteHeader(http.StatusRequestEntityTooLarge)
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if kind, known := hookEvents[req.Type]; known && s.onEvent != nil {
		s.onEvent(harness.AgentEvent{
			TerminalID: terminalID, Event: kind, SessionID: req.SessionID, ToolName: req.Tool,
			Message: truncate(req.Message), NeedsInput: kind == harness.EventNotification,
		})
	}
	w.WriteHeader(http.StatusOK)
}

// truncate bounds s to maxHookMessageBytes on a rune boundary.
func truncate(s string) string {
	if len(s) <= maxHookMessageBytes {
		return s
	}
	b := s[:maxHookMessageBytes]
	for len(b) > 0 {
		r, size := utf8.DecodeLastRuneInString(b)
		if r != utf8.RuneError || size != 1 {
			break
		}
		b = b[:len(b)-size]
	}
	return b
}
