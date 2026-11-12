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
	// AgentID is set on a subagent's own events.
	AgentID         string           `json:"agent_id"`
	BackgroundTasks []backgroundTask `json:"background_tasks"`
}

// backgroundTask decodes id, type and status only. command and description can carry user text:
// never decode them (P86 privacy rule).
type backgroundTask struct {
	ID     string `json:"id"`
	Type   string `json:"type"`
	Status string `json:"status"`
}

// liveBackground counts running or pending tasks. A stopping subagent still lists itself as
// running: skip its own entry.
func liveBackground(req hookRequest, kind harness.EventKind) int {
	if kind != harness.EventTurnEnd && kind != harness.EventSubagentEnd {
		return 0
	}
	n := 0
	for _, t := range req.BackgroundTasks {
		if t.Status != "running" && t.Status != "pending" {
			continue
		}
		if kind == harness.EventSubagentEnd && t.ID == req.AgentID {
			continue
		}
		n++
	}
	return n
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
	"SubagentStop":     harness.EventSubagentEnd,
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
	var reply harness.HookReply
	if known && (s.onEvent != nil || s.reply != nil) {
		ev := harness.AgentEvent{
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
			AgentID:              req.AgentID,
			Background:           liveBackground(req, kind),
			Wakes:                req.AgentID == "" && kind == harness.EventToolStart && wakeTools[req.ToolName],
			NeedsInput:           kind == harness.EventNotification && needsInput(req),
		}
		// Reply first: a Stop that carries a message must not also count as idle in OnEvent.
		if s.reply != nil {
			reply = s.reply(ev)
		}
		if s.onEvent != nil {
			s.onEvent(ev)
		}
	}
	writeReply(w, kind, reply)
}

// writeReply answers 200, with a JSON body only for a Stop block or a prompt context, the two
// outputs Claude Code reads from a hook's stdout.
func writeReply(w http.ResponseWriter, kind harness.EventKind, reply harness.HookReply) {
	var out any
	switch {
	case kind == harness.EventTurnEnd && reply.Block != "":
		out = map[string]string{"decision": "block", "reason": reply.Block}
	case kind == harness.EventPrompt && reply.Context != "":
		out = map[string]any{"hookSpecificOutput": map[string]string{
			"hookEventName":     "UserPromptSubmit",
			"additionalContext": reply.Context,
		}}
	}
	if out == nil {
		w.WriteHeader(http.StatusOK)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(out)
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
