package harness

import (
	"errors"
	"fmt"
	"sync"
)

// HookHost is a provider with a session-event listener for interactive sessions. A provider
// without it simply reports no activity for its sessions.
type HookHost interface {
	StartHooks(o HookOptions) (HookListener, error)
}

// HookOptions configures a listener.
type HookOptions struct {
	// OnEvent is called once per accepted event, from the listener's own goroutine, never after
	// Close returns.
	OnEvent func(AgentEvent)
	// OnStatusLine receives rate limits a TUI session reports.
	OnStatusLine func(terminalID string, rl RateLimits)
	// StatusLine reports whether a new launch gets the usage feed; read per launch.
	StatusLine func() bool
}

// HookListener is one running listener.
type HookListener interface {
	// Compose adds the listener to an interactive command and returns the env it needs. The env
	// can carry a secret: never return it from a bound method.
	Compose(terminalID, cwd, command string) (string, []string)
	Status() HookStatus
	Close() error
}

// HookStatus is a listener's state.
type HookStatus struct {
	Running bool
	// SettingsPath is the generated settings file, "" while stopped.
	SettingsPath string
}

// EventKind is a session event's neutral kind. The values are the wire strings the bridge and the
// frontend read.
type EventKind string

const (
	EventSessionStart EventKind = "SessionStart"
	EventSessionEnd   EventKind = "SessionEnd"
	EventPrompt       EventKind = "UserPromptSubmit"
	EventToolStart    EventKind = "PreToolUse"
	EventToolEnd      EventKind = "PostToolUse"
	EventNotification EventKind = "Notification"
	EventTurnEnd      EventKind = "Stop"
)

// AgentEvent is one session event. TerminalID comes from the launch, the rest from the agent's
// payload; only the fields below are kept.
type AgentEvent struct {
	TerminalID       string    `json:"terminalId"`
	Event            EventKind `json:"event"`
	SessionID        string    `json:"sessionId"`
	Cwd              string    `json:"cwd"`
	ToolName         string    `json:"toolName"`
	ToolUseID        string    `json:"toolUseId"`
	NotificationType string    `json:"notificationType"`
	Message          string    `json:"message"`
	Source           string    `json:"source"`
	Reason           string    `json:"reason"`
	// LastAssistantMessage is Stop's reply text, bounded like Message.
	LastAssistantMessage string `json:"lastAssistantMessage"`
	// Wakes: a tool start that arms a self-wake, so the turn end that follows is not "finished".
	Wakes bool `json:"-"`
	// NeedsInput: a notification that asks the user to act.
	NeedsInput bool `json:"-"`
}

// Hooks runs one listener per registered provider that is a HookHost and routes a launch to its
// provider's listener.
type Hooks struct {
	reg  *Registry
	opts HookOptions

	mu        sync.Mutex
	listeners map[string]HookListener
}

// NewHooks builds a hub; nothing is started yet.
func NewHooks(reg *Registry, opts HookOptions) *Hooks {
	return &Hooks{reg: reg, opts: opts, listeners: map[string]HookListener{}}
}

// Start starts every host that is not running. A host that fails does not stop the others; the
// failures are returned joined and sessions of that provider just report no activity.
func (h *Hooks) Start() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	var errs []error
	for _, id := range h.reg.IDs() {
		p, _ := h.reg.Get(id)
		host, ok := p.(HookHost)
		if !ok || h.listeners[id] != nil {
			continue
		}
		opts := h.opts
		if !ReportsUsage(p) {
			opts.OnStatusLine = nil
		}
		l, err := host.StartHooks(opts)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", id, err))
			continue
		}
		h.listeners[id] = l
	}
	return errors.Join(errs...)
}

// Stop closes every running listener.
func (h *Hooks) Stop() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	var errs []error
	for id, l := range h.listeners {
		if err := l.Close(); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", id, err))
		}
		delete(h.listeners, id)
	}
	return errors.Join(errs...)
}

// Compose returns command unchanged with a nil env for a provider without a running listener.
func (h *Hooks) Compose(provider, terminalID, cwd, command string) (string, []string) {
	h.mu.Lock()
	l := h.listeners[provider]
	h.mu.Unlock()
	if l == nil {
		return command, nil
	}
	return l.Compose(terminalID, cwd, command)
}

// Status is the provider's listener state; the zero value while it is stopped or has no hooks.
func (h *Hooks) Status(provider string) HookStatus {
	h.mu.Lock()
	l := h.listeners[provider]
	h.mu.Unlock()
	if l == nil {
		return HookStatus{}
	}
	return l.Status()
}

// UsageReporter is a provider whose runs and sessions report account rate-limit windows.
type UsageReporter interface {
	UsageWindows() bool
}

// ReportsUsage reports whether p feeds usage limits.
func ReportsUsage(p Provider) bool {
	u, ok := p.(UsageReporter)
	return ok && u.UsageWindows()
}
