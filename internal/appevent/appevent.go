// Package appevent is the Go->renderer push seam and the small set of IPC channel
// strings both apps' menu/quit/close-flush handshakes emit on, shared verbatim:
// apps/kira-studio/internal/appcore.Emitter and apps/kira-space/internal/appcore.Emitter were
// identical three-method interfaces, and bridge.Events' Signal/SignalTo/Broadcast/NewEvents core
// was byte-identical in both apps too. Each app's own internal/bridge/events.go still owns its
// own channel constants and its own Attach — this package is only the shared core they embed.
package appevent

// Emitter is the Go->renderer push seam — internal/shell implements it over *application.App's
// Event.Emit/DispatchWailsEvent; each app's bridge.Events is its one real consumer, so no bridge
// file has to import Wails. EmitTo delivers to exactly one window by key — the mechanism the
// per-window close-flush handshake builds on. EmitFocused delivers to whichever window is
// currently key/focused, the successor to Electron's own sendToFocusedWindow.
type Emitter interface {
	Emit(name string, data any)
	EmitTo(windowKey string, name string, data any)
	EmitFocused(name string, data any)
}

// Events is the shared core of both apps' bridge.Events: the signal/broadcast primitives every
// push producer built on top of it (menu commands, the quit handshake, the per-window close-flush
// handshake) needs, with nothing app-specific — no channel constants, no Attach.
type Events struct {
	emit Emitter
}

func NewEvents(e Emitter) *Events {
	return &Events{emit: e}
}

// Signal emits a payload-free channel (nil, not {}) to the focused window only — Electron's own
// sendToFocusedWindow(channel). A background window no longer reacts to a command aimed at
// whichever window the user was actually looking at.
func (ev *Events) Signal(channel string) {
	ev.emit.EmitFocused(channel, nil)
}

// SignalTo is Signal's single-window analogue, aimed by key rather than by focus — the per-window
// close-flush handshake's own trigger.
func (ev *Events) SignalTo(windowKey, channel string) {
	ev.emit.EmitTo(windowKey, channel, nil)
}

// Broadcast emits a payload-free channel to every window — the quit handshake's own trigger: every
// window genuinely must flush before quitting, not only the focused one, so this deliberately does
// not go through Signal's focused-only delivery.
func (ev *Events) Broadcast(channel string) {
	ev.emit.Emit(channel, nil)
}

// The channels below are byte-identical strings in both apps today — the menu/quit/close-flush/
// window-chrome machinery's own wire vocabulary, never app-specific. Each app's bridge/events.go
// re-exports these under its own ChannelX names so no call site outside this hoist has to change.
const (
	ChannelFlushBeforeClose       = "kira:app:flushBeforeClose"
	ChannelWindowFlushBeforeClose = "kira:window:flushBeforeClose"
	ChannelSettingsChanged        = "kira:settings:changed"
	ChannelLayoutChanged          = "kira:layout:changed"
	ChannelTerminal               = "kira:terminal:data"
	ChannelCodeSearch             = "kira:code:search"
	// The seven below are P116's own hoist — Kira Studio's native-menu/window-chrome channels that
	// Kira Space now wires too (window-chrome-parity G1-G5/G7), byte-identical strings on both sides.
	ChannelOpenSettings = "kira:menu:openSettings"
	// P328: App menu > Check for Updates…, shared by both apps via shell.AppMenuHead.
	ChannelCheckForUpdates    = "kira:menu:checkForUpdates"
	ChannelToggleProjectPanel = "kira:menu:toggleProjectPanel"
	ChannelTabNext            = "kira:menu:tabNext"
	ChannelTabPrev            = "kira:menu:tabPrev"
	ChannelTabClose           = "kira:menu:tabClose"
	ChannelKeepAwake          = "kira:keepAwake:changed"
	ChannelAppMetrics         = "kira:app:metrics"
	// ChannelAgentSessions/ChannelAgentEvent are the hoist — Claude Code agent-activity
	// monitoring's two wire channels moved here with internal/agenthooks and its Manager (no app
	// wires either channel, TS's own CHANNEL.agentSessions/
	// agentEvent in packages/shared/protocol/events.ts). ChannelAgentSessions is every live Claude
	// Code session across every window; ChannelAgentEvent is one hook firing for one tab. Both
	// Emit'd (not EmitTo), ChannelKeepAwake's own shape: one process-wide fact, not
	// window-addressed.
	ChannelAgentSessions = "kira:agent:sessions"
	ChannelAgentEvent    = "kira:agent:event"
	// P132 Part 2's hoist: Space's op log dock shares Studio's toggle channel and update push.
	ChannelToggleOperationsPanel = "kira:menu:toggleOperationsPanel"
	ChannelOpUpdate              = "kira:op:update"
	ChannelOpBatch               = "kira:op:batch"
	// P246: the routed popup list (broadcast) and a reveal for one window.
	ChannelPromptsChanged = "kira:prompts:changed"
	ChannelPromptsReveal  = "kira:prompts:reveal"
	// ChannelApprovalsChanged broadcasts the full pending approvals list (internal/approvals).
	ChannelApprovalsChanged = "kira:approvals:changed"
	// ChannelLlmPromptsChanged broadcasts {revision} after the prompt overrides file changed.
	ChannelLlmPromptsChanged = "kira:llmPrompts:changed"
)
