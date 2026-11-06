package bridge

import (
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// Studio-side channels. Strings that Space also uses with a different payload carry a "studio:"
// prefix; the rest are unique to Studio and keep their strings.
const (
	ChannelNewConnection  = "kira:menu:newConnection"
	ChannelNewRequest     = "kira:menu:newRequest"
	ChannelImportPostman  = "kira:menu:importPostman"
	ChannelImportDataGrip = "kira:menu:importDataGrip"
	ChannelViewRefresh    = "kira:menu:viewRefresh"
	ChannelViewRun        = "kira:menu:viewRun"
	ChannelViewRunAll     = "kira:menu:viewRunAll"
	ChannelViewFormat     = "kira:menu:viewFormat"

	ChannelConnectionState     = "kira:connection:state"
	ChannelMetadataInvalidated = "kira:connection:metadataInvalidated"
	ChannelConnectionsChanged  = "kira:connections:changed"
	// ChannelConnectionConfigChanged is payload-free: a connection file or the problem set changed, or a
	// secret was set or cleared. The config panels refetch.
	ChannelConnectionConfigChanged = "kira:connectionConfig:changed"
	// ChannelConnectionFiltersChanged carries {connectionId, visibility}: a file changed one connection's
	// left panel visibility.
	ChannelConnectionFiltersChanged = "kira:connection:filtersChanged"
	ChannelSchemaChanged            = "kira:schema:changed"
	// ChannelStudioOpUpdate carries Studio's own payload
	// (StudioOpsService).
	ChannelStudioOpUpdate = "studio:kira:op:update"
	// ChannelGrpcCall is a server-streaming call's coalesced message batches, delivered with EmitTo
	// (one window only) by the RunOp closure calling grpcclient.ServerStream.
	ChannelGrpcCall = "kira:grpc:call"
	// ChannelMaskRulesChanged carries one connection's rule set after an Upsert/Remove/RegenerateKey.
	ChannelMaskRulesChanged = "kira:maskRules:changed"
	// ChannelApiDataChanged is emitted by every API-client mutation with the scopes it touched, so
	// every window's TanStack Query cache invalidates exactly those keys (apidata.go).
	ChannelApiDataChanged = "kira:api:dataChanged"
	// ChannelSavedQueriesChanged fires after every saved query write.
	ChannelSavedQueriesChanged = "kira:savedQueries:changed"
	// ChannelFilterHistoryChanged fires after QueriesService.HistoryRecord.
	ChannelFilterHistoryChanged = "kira:filterHistory:changed"
)

// ChannelEngineState is declared for completeness and deliberately never emitted.
const ChannelEngineState = "kira:engine:state"

// DbSources are the Studio push producers left as seams. Metrics and approvals are
// process-wide and attach once through Events.AttachMetrics and AttachApprovals.
type DbSources struct {
	Connections interface {
		OnStateChange(func(model.ConnectionState)) func()
		OnMetadataInvalidated(func(string)) func()
		OnListChanged(func([]model.ConnectionSummary)) func()
	}
	Oplog interface {
		OnUpdate(func(model.DbOpRecord)) func()
	}
}

// AttachDb subscribes to every producer in s and returns one detach that unsubscribes all of
// them. Detach it first in the quit teardown so nothing emits into a half-torn-down app.
func (ev *Events) AttachDb(s DbSources) (detach func()) {
	unsubState := s.Connections.OnStateChange(func(st model.ConnectionState) {
		ev.emit.Emit(ChannelConnectionState, st)
	})
	unsubInvalidated := s.Connections.OnMetadataInvalidated(func(connectionID string) {
		ev.emit.Emit(ChannelMetadataInvalidated, connectionID)
	})
	unsubList := s.Connections.OnListChanged(func(list []model.ConnectionSummary) {
		ev.emit.Emit(ChannelConnectionsChanged, list)
	})
	unsubOplog := s.Oplog.OnUpdate(func(rec model.DbOpRecord) {
		ev.emit.Emit(ChannelStudioOpUpdate, rec)
	})
	return func() {
		unsubState()
		unsubInvalidated()
		unsubList()
		unsubOplog()
	}
}
