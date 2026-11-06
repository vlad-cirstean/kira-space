package bridge

import (
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiotmp/appcore"
	"github.com/kirathecat/kira-studio/internal/appevent"
	"github.com/kirathecat/kira-studio/internal/approvals"
	"github.com/kirathecat/kira-studio/internal/metrics"
)

// Channel holds today's exact IPC channel strings (packages/shared/protocol/ipc.ts's IPC const),
// which are the Wails event names verbatim — the renderer's subscribe mechanism changes, the
// wire name does not. The ones re-exported below are byte-identical
// strings shared with Kira Space, now defined once in repo-root internal/appevent — re-exported
// under their own names here so no call site anywhere in this package has to change.
const (
	ChannelOpenSettings           = appevent.ChannelOpenSettings
	ChannelCheckForUpdates        = appevent.ChannelCheckForUpdates
	ChannelNewConnection          = "kira:menu:newConnection"
	ChannelNewRequest             = "kira:menu:newRequest"
	ChannelImportPostman          = "kira:menu:importPostman"
	ChannelImportDataGrip         = "kira:menu:importDataGrip"
	ChannelToggleProjectPanel     = appevent.ChannelToggleProjectPanel
	ChannelToggleOperationsPanel  = appevent.ChannelToggleOperationsPanel
	ChannelCommandPalette         = "kira:menu:commandPalette"
	ChannelTabNext                = appevent.ChannelTabNext
	ChannelTabPrev                = appevent.ChannelTabPrev
	ChannelTabClose               = appevent.ChannelTabClose
	ChannelViewFind               = "kira:menu:viewFind"
	ChannelViewRefresh            = "kira:menu:viewRefresh"
	ChannelViewRun                = "kira:menu:viewRun"
	ChannelViewRunAll             = "kira:menu:viewRunAll"
	ChannelViewFormat             = "kira:menu:viewFormat"
	ChannelFlushBeforeClose       = appevent.ChannelFlushBeforeClose
	ChannelWindowFlushBeforeClose = appevent.ChannelWindowFlushBeforeClose
	ChannelConnectionState        = "kira:connection:state"
	ChannelMetadataInvalidated    = "kira:connection:metadataInvalidated"
	ChannelConnectionsChanged     = "kira:connections:changed"
	// ChannelConnectionConfigChanged is payload-free: a connection file or the problem set changed, or a
	// secret was set or cleared. The config panels refetch.
	ChannelConnectionConfigChanged = "kira:connectionConfig:changed"
	// ChannelConnectionFiltersChanged carries {connectionId, visibility}: a file changed one connection's
	// left panel visibility.
	ChannelConnectionFiltersChanged = "kira:connection:filtersChanged"
	ChannelSettingsChanged          = appevent.ChannelSettingsChanged
	ChannelLayoutChanged            = appevent.ChannelLayoutChanged
	ChannelOpUpdate                 = appevent.ChannelOpUpdate
	ChannelAppMetrics               = appevent.ChannelAppMetrics
	ChannelSchemaChanged            = "kira:schema:changed"
	// ChannelGrpcCall is the new push channel — a server-streaming call's coalesced
	// message batches, delivered with EmitTo (one window only, GrpcService.Call's own emitter
	// call) rather than through this file's Events/Sources machinery: it has exactly one producer
	// (the RunOp closure calling grpcclient.ServerStream), not a startup-wired long-lived source,
	// so it needs no Sources entry and no Attach subscription.
	ChannelGrpcCall = "kira:grpc:call"
	// ChannelApprovalsChanged broadcasts the full approvals list (internal/approvals).
	ChannelApprovalsChanged = appevent.ChannelApprovalsChanged
	// ChannelTerminal is P83's own push channel — one terminal's output and its exit, EmitTo'd to
	// the one window that opened it, flushed on the same 60ms/256-batch/terminal-event shape as
	// ChannelGrpcCall above (internal/bridge/terminal.go's own coalescer).
	ChannelTerminal = appevent.ChannelTerminal
	// ChannelCustomScriptsChanged is P85's own list-changed broadcast — CustomScriptsService's own
	// Create/Update/Remove Emit (not EmitTo) the full list, ChannelConnectionsChanged's own shape,
	// so every window's tab-strip dropdown stays in sync with a script added or removed elsewhere.
	ChannelCustomScriptsChanged = "kira:customScripts:changed"
	// ChannelMaskRulesChanged is the push channel — MaskRulesService's own
	// Upsert/Remove/RegenerateKey Emit (not EmitTo) one connection's own rule set,
	// ChannelSchemaChanged's own per-connection shape rather than ChannelCustomScriptsChanged's
	// flat app-wide list, so a second window's Privacy tab / grid header menu / grid preview stay
	// in sync with a rule changed elsewhere.
	ChannelMaskRulesChanged = "kira:maskRules:changed"
	// ChannelKeepAwake is the app-wide broadcast — the titlebar toggle's state, Emit'd
	// (not EmitTo): one machine, one assertion, so every window's titlebar button must agree.
	// Process-scoped, never persisted: an OS power assertion that outlives the reason a user made
	// it is a surprise, and the persistent half of this feature is the Settings toggle.
	// Byte-identical to Kira Space's own channel of the same name, hoisted to repo-root
	// internal/appevent. The two session-monitor channel constants, once defined right below
	// this, hoisted there too, alongside the rest of agent-activity monitoring — no longer
	// re-exported here since Studio no longer emits either.
	ChannelKeepAwake = appevent.ChannelKeepAwake
	// ChannelApiDataChanged is P112's own broadcast — every API-client mutation (collections,
	// saved requests, variables, environments) Emits it with the scopes that mutation touched, so
	// every window's TanStack Query cache invalidates exactly those keys (apidata.go).
	ChannelApiDataChanged = "kira:api:dataChanged"
	// ChannelSavedQueriesChanged is P433's broadcast — QueriesService Emits it after every saved
	// query write (save, update, delete, touch) so every window's saved menus refetch only then.
	ChannelSavedQueriesChanged = "kira:savedQueries:changed"
	// ChannelFilterHistoryChanged is P433's broadcast — QueriesService.HistoryRecord Emits it so
	// every window's filter history menu refetches only then.
	ChannelFilterHistoryChanged = "kira:filterHistory:changed"
)

// ChannelEngineState is declared for completeness and deliberately never emitted: nothing in
// src/main sends it and nothing in apps/kira-studio/frontend/src subscribes to it (P56 D5). P57 deletes it.
const ChannelEngineState = "kira:engine:state"

// Sources are the five push producers left as seams. Each is a one-method
// interface, small enough to drive with a plain recorder rather than a real service — there is
// no dedicated events_test.go in this package (the test-bar pruning removed it; this
// comment used to still claim otherwise).
type Sources struct {
	Connections interface {
		OnStateChange(func(model.ConnectionState)) func()
		OnMetadataInvalidated(func(string)) func()
		OnListChanged(func([]model.ConnectionSummary)) func()
	}
	Oplog interface {
		OnUpdate(func(model.OpRecord)) func()
	}
	Metrics interface {
		OnSample(func(metrics.Sample)) func()
	}
	Approvals interface {
		OnChange(func([]approvals.Item)) func()
	}
}

// Events is the Go analogue of the five push-side listeners src/main/index.ts wires (:58, :99-
// 102) plus the menu/quit signal channels. It is the only thing in this package that emits.
// Signal/SignalTo/Broadcast come from the embedded *appevent.Events core; emit is
// kept alongside it for Attach's five producer subscriptions and SettingsChanged below, neither of
// which the shared core exposes a raw Emit for.
type Events struct {
	*appevent.Events
	emit appcore.Emitter
}

func NewEvents(e appcore.Emitter) *Events {
	return &Events{Events: appevent.NewEvents(e), emit: e}
}

// Attach subscribes to every producer in s and returns one detach that unsubscribes all of them.
// It is called once at startup and detached first in the quit teardown (P56 D3), so nothing
// emits into a half-torn-down app.
func (ev *Events) Attach(s Sources) (detach func()) {
	unsubState := s.Connections.OnStateChange(func(st model.ConnectionState) {
		ev.emit.Emit(ChannelConnectionState, st)
	})
	unsubInvalidated := s.Connections.OnMetadataInvalidated(func(connectionID string) {
		ev.emit.Emit(ChannelMetadataInvalidated, connectionID)
	})
	unsubList := s.Connections.OnListChanged(func(list []model.ConnectionSummary) {
		ev.emit.Emit(ChannelConnectionsChanged, list)
	})
	unsubOplog := s.Oplog.OnUpdate(func(rec model.OpRecord) {
		ev.emit.Emit(ChannelOpUpdate, rec)
	})
	unsubMetrics := s.Metrics.OnSample(func(sample metrics.Sample) {
		ev.emit.Emit(ChannelAppMetrics, sample)
	})
	unsubApprovals := s.Approvals.OnChange(func(list []approvals.Item) {
		ev.emit.Emit(ChannelApprovalsChanged, list)
	})

	return func() {
		unsubState()
		unsubInvalidated()
		unsubList()
		unsubOplog()
		unsubMetrics()
		unsubApprovals()
	}
}

// Signal/SignalTo/Broadcast (nil payloads, not {}) come from the embedded *appevent.Events
// core — Electron's own sendToFocusedWindow(channel) (18fe7bb^:src/main/menu.ts:5-8; preload's
// onSignal discards arguments, src/preload/index.ts:35-39). The menu's twelve signal channels are
// Signal's only caller: a background window no longer reacts to a command aimed at
// whichever window the user was looking at — Cmd+W closing a tab in every open window, or
// Cmd+Return running a console statement in a window the user never touched, were the concrete
// symptoms. Broadcast (the quit handshake's own trigger) and
// the six state-change broadcasts in Attach/SettingsChanged/LayoutService.Set all stay on Emit —
// every window genuinely needs those, unlike a menu command.

// SettingsChanged broadcasts the merged settings unconditionally — SettingsService.Set's own
// job, factored out here so its own file stays a thin wrapper.
func (ev *Events) SettingsChanged(s model.Settings) {
	ev.emit.Emit(ChannelSettingsChanged, s)
}
