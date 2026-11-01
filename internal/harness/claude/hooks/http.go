package hooks

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/kirathecat/kira-studio/internal/harness"
)

// maxHookPayloadBytes bounds one hook request's body — 4 MiB is generous for a real
// PreToolUse/PostToolUse payload (tool_input/tool_response can carry a large diff or command
// output) while still refusing an unbounded body. Over this, 413 and the event is dropped; the
// reducer is built to tolerate a dropped PostToolUse (paired by tool_use_id, reset by Stop and by
// a session leaving the registry), so a drop here can never strand the UI.
const maxHookPayloadBytes = 4 * 1024 * 1024

// maxHookMessageBytes truncates Notification's own message field (§6.1) — bounded before it ever
// reaches HookOptions.OnEvent, on a rune boundary so a multi-byte UTF-8 character is never split.
const maxHookMessageBytes = 200

// hookRequest is the body this listener actually decodes — exactly the ten payload fields §6's
// Event keeps, snake_case as the installed CLI's own hook payload names them. tool_input,
// tool_response and transcript_path have no matching field here, so encoding/json silently skips
// them on decode: nothing about them is ever retained, logged or reachable from HookOptions.OnEvent.
type hookRequest struct {
	HookEventName    string `json:"hook_event_name"`
	SessionID        string `json:"session_id"`
	Cwd              string `json:"cwd"`
	ToolName         string `json:"tool_name"`
	ToolUseID        string `json:"tool_use_id"`
	NotificationType string `json:"notification_type"`
	Message          string `json:"message"`
	Source           string `json:"source"`
	Reason           string `json:"reason"`
	// LastAssistantMessage is Stop's reply text.
	LastAssistantMessage string `json:"last_assistant_message"`
}

// eventKinds maps Claude's hook_event_name to the neutral kind; a name not listed is dropped.
var eventKinds = map[string]harness.EventKind{
	"SessionStart":     harness.EventSessionStart,
	"SessionEnd":       harness.EventSessionEnd,
	"UserPromptSubmit": harness.EventPrompt,
	"PreToolUse":       harness.EventToolStart,
	"PostToolUse":      harness.EventToolEnd,
	"Notification":     harness.EventNotification,
	"Stop":             harness.EventTurnEnd,
}

// wakeTools are the tools that arm a wake-up: a session that armed one waits, so its Stop is not
// "finished". Mirrors workbench agentActivity.ts WAKE_TOOLS.
var wakeTools = map[string]bool{"Monitor": true, "ScheduleWakeup": true}

// needsInput: a permission or elicitation prompt, or an untyped message from an older CLI.
func needsInput(req hookRequest) bool {
	switch req.NotificationType {
	case "permission_prompt", "elicitation_dialog":
		return true
	case "":
		return req.Message != ""
	}
	return false
}

func (s *Server) mux() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/hook", s.handleHook)
	mux.HandleFunc("/statusline", s.handleStatusLine)
	return mux
}

// handleHook is POST /hook's own handler: token check, then X-Kira-Terminal, then the
// bounded body — in that order, so a request failing the token check never reaches the body
// reader at all (an attacker who knows the socket path but not the token gets nothing back,
// including a size-limit response that would leak information about the bound).
func (s *Server) handleHook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if subtle.ConstantTimeCompare([]byte(bearerToken(r)), []byte(s.ln.Token)) != 1 {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	terminalID := r.Header.Get("X-Kira-Terminal")
	if terminalID == "" {
		// §6.1: an unknown or empty X-Kira-Terminal is a 400, dropped — the listener never invents
		// a terminal to attribute an orphaned hook to.
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	body := http.MaxBytesReader(w, r.Body, maxHookPayloadBytes)
	var req hookRequest
	if err := json.NewDecoder(body).Decode(&req); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			w.WriteHeader(http.StatusRequestEntityTooLarge)
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	kind, known := eventKinds[req.HookEventName]
	if s.onEvent != nil && known {
		s.onEvent(harness.AgentEvent{
			TerminalID:           terminalID,
			Event:                kind,
			SessionID:            req.SessionID,
			Cwd:                  req.Cwd,
			ToolName:             req.ToolName,
			ToolUseID:            req.ToolUseID,
			NotificationType:     req.NotificationType,
			Message:              truncateMessage(req.Message),
			Source:               req.Source,
			Reason:               req.Reason,
			LastAssistantMessage: truncateMessage(req.LastAssistantMessage),
			Wakes:                kind == harness.EventToolStart && wakeTools[req.ToolName],
			NeedsInput:           kind == harness.EventNotification && needsInput(req),
		})
	}
	w.WriteHeader(http.StatusOK)
}

// bearerToken extracts the token from "Authorization: Bearer <token>" — "" for any other shape,
// which ConstantTimeCompare below then simply rejects (a missing header is not a special case).
func bearerToken(r *http.Request) string {
	const prefix = "Bearer "
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, prefix) {
		return ""
	}
	return strings.TrimPrefix(h, prefix)
}

// truncateMessage bounds s to maxHookMessageBytes, backing off to the nearest earlier rune
// boundary so a multi-byte UTF-8 character is never split in half. A byte-blind slice at
// exactly maxHookMessageBytes can land inside a multi-byte character's own trailing bytes;
// DecodeLastRuneInString reports that as RuneError/size 1, so trimming one byte at a time until
// it stops doing so removes exactly the truncated fragment and nothing more.
func truncateMessage(s string) string {
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
