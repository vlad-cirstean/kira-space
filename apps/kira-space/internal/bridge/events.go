package bridge

import (
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/oplog"
	"github.com/kirathecat/kira-studio/internal/appevent"
	"github.com/kirathecat/kira-studio/internal/metrics"
)

// ChannelCodeSearch is the push channel — a repository-wide search's coalesced file
// groups, delivered with EmitTo (one window only). Byte-identical to Kira Studio's
// own channel of the same name, hoisted to repo-root internal/appevent and re-exported here so no
// call site in this package has to change.
const ChannelCodeSearch = appevent.ChannelCodeSearch

// ChannelMobilePairing, ChannelMobileDevices and ChannelMobileStatus are P212's three desktop-only
// channels: the phone pairing prompt's queue snapshot, the paired phones list and the server state.
// None is on the mobileweb event allowlist.
const (
	ChannelMobilePairing = "kira:mobile:pairing"
	ChannelMobileDevices = "kira:mobile:devices"
	ChannelMobileStatus  = "kira:mobile:status"
	// ChannelMobileOpenLaunch asks one desktop window to open a phone-started launch's terminal
	// (EmitTo, never Emit); ChannelMobileTerminals pushes the phone-held terminals list.
	ChannelMobileOpenLaunch = "kira:mobile:openLaunch"
	ChannelMobileTerminals  = "kira:mobile:terminals"
)

// ChannelCustomScriptsChanged is CustomScriptsService's list-changed broadcast (Emit to every
// window) — Kira Studio's channel of the same name.
const ChannelCustomScriptsChanged = "kira:customScripts:changed"

// ChannelGitCredential is P178's credential relay snapshot (every pending prompt a socket client
// or the ADE board raised), broadcast to every window.
const ChannelGitCredential = "kira:git:credential"

// ChannelSettingsChanged/ChannelLayoutChanged are SettingsService.Set/LayoutService.Set's own
// broadcasts — Kira Studio's own two channels of the same name, unchanged shape.
const (
	ChannelSettingsChanged = appevent.ChannelSettingsChanged
	ChannelLayoutChanged   = appevent.ChannelLayoutChanged
)

// ChannelFlushBeforeClose/ChannelWindowFlushBeforeClose are the quit-wide and per-window flush
// handshakes' own trigger channels — Kira Studio's own two channels of the same name
// (internal/shell/quit.go, internal/shell/closeflush.go).
const (
	ChannelFlushBeforeClose       = appevent.ChannelFlushBeforeClose
	ChannelWindowFlushBeforeClose = appevent.ChannelWindowFlushBeforeClose
)

// ChannelTerminal is the embedded terminal's own push channel — Kira Studio's own
// ChannelTerminal (internal/bridge/terminal.go's own coalescer), EmitTo'd to the one window that
// opened it.
const ChannelTerminal = appevent.ChannelTerminal

// ChannelAgentSessions/ChannelAgentEvent are P127's own two channels, re-exported here — the
// same "re-export so no call site outside the hoist has to change" precedent every other constant
// above follows.
const (
	ChannelAgentSessions = appevent.ChannelAgentSessions
	ChannelAgentEvent    = appevent.ChannelAgentEvent
)

// ChannelAgentRevealTerminal and ChannelAgentRevealTask are P238's notification-click pushes
// (EmitTo, one window): show that terminal tab, or that ADE task. Space only, not on the phone
// allowlist.
const (
	ChannelAgentRevealTerminal = "kira:agent:revealTerminal"
	ChannelAgentRevealTask     = "kira:agent:revealTask"
	// ChannelAgentRevealScriptRun opens a script run's tab; payload {runId, label}.
	ChannelAgentRevealScriptRun = "kira:agent:revealScriptRun"
)

// The five below are P116's own window-chrome-parity channels (G1-G5/G7) — Kira Studio's own
// channels of the same name, hoisted to repo-root internal/appevent.
const (
	ChannelOpenSettings       = appevent.ChannelOpenSettings
	ChannelCheckForUpdates    = appevent.ChannelCheckForUpdates
	ChannelToggleProjectPanel = appevent.ChannelToggleProjectPanel
	ChannelTabNext            = appevent.ChannelTabNext
	ChannelTabPrev            = appevent.ChannelTabPrev
	ChannelTabClose           = appevent.ChannelTabClose
	ChannelKeepAwake          = appevent.ChannelKeepAwake
	ChannelAppMetrics         = appevent.ChannelAppMetrics
	ChannelCommandPalette     = "kira:menu:commandPalette"
	ChannelViewFind           = "kira:menu:viewFind"
	// ChannelReviewCode is the Review Code menu item (ade.reviewCode): opens the review for the
	// focused task or branch on the Plan board.
	ChannelReviewCode = "kira:menu:reviewCode"
)

// P132 Part 2's op log channels, hoisted to internal/appevent (shared with Kira Studio's own
// bridge).
const (
	ChannelToggleOperationsPanel = appevent.ChannelToggleOperationsPanel
	ChannelOpUpdate              = appevent.ChannelOpUpdate
	ChannelOpBatch               = appevent.ChannelOpBatch
)

// Events is the Go->renderer push wrapper every bridge service that emits goes through — Kira
// Studio's own bridge.Events (internal/bridge/events.go), trimmed: this app has no
// Connections/Oplog/DbMcp producers to Attach. Signal/SignalTo/Broadcast come entirely from the
// embedded *appevent.Events core; emit is kept alongside it for AttachMetrics below, the one
// raw-Emit producer this app has.
type Events struct {
	*appevent.Events
	emit appevent.Emitter
}

func NewEvents(e appevent.Emitter) *Events {
	return &Events{Events: appevent.NewEvents(e), emit: e}
}

// AttachMetrics subscribes m's own OnSample and forwards each sample to ChannelAppMetrics — Kira
// Studio's own Events.Attach's Metrics producer (internal/bridge/events.go), the one producer this
// app needs.
func (ev *Events) AttachMetrics(m *metrics.Ticker) (detach func()) {
	return m.OnSample(func(sample metrics.Sample) {
		ev.emit.Emit(ChannelAppMetrics, sample)
	})
}

// AttachOpLog broadcasts every op record change to every window: one process-wide log, so an op
// from any window or client is every window's news. Foreground changes go one by one; background
// changes arrive coalesced on ChannelOpBatch.
func (ev *Events) AttachOpLog(l *oplog.Log) (detach func()) {
	d1 := l.OnUpdate(func(r oplog.Record) {
		ev.emit.Emit(ChannelOpUpdate, r)
	})
	d2 := l.OnBatch(func(rs []oplog.Record) {
		ev.emit.Emit(ChannelOpBatch, rs)
	})
	return func() { d1(); d2() }
}

// GitEventPrefix prefixes every git event name (repo.changed, remote.progress, ...) pushed to one
// window with EmitTo.
const GitEventPrefix = "kira:git:"
