package ipcfixture

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapterhost"
	_ "github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters/sqs"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters/testsupport"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiotmp/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/tree"
)

// TestFixture_SQS is against tests/ipc/sqs/sqs.fixture.ts's own
// committed scenario: connect, a synthetic opsRecent snapshot (the TypeScript harness never issued
// a real backend call for this channel either — see the file header this ports), a flat queue
// tree, a poll read (its own randomly-generated MessageIds/timestamps frozen after being validated
// live), an invalidate, an approximate count against an empty queue (whose own wire payload omits
// "refresh" entirely — masked away by frozen.go's own refresh:false rule), and the orders queue's
// definition captured twice (an unrefreshed then an explicitly-refreshed read of the same real
// result, matching the mocked frontend's own onMounted/onRefresh pair — no second live call).
func TestFixture_SQS(t *testing.T) {
	fixture := testsupport.StartSqs(t)
	app := NewApp(t)
	cfg := fixture.Config

	app.SeedConnection(t, cfg.ID, fieldsOf(cfg), cfg.Password)
	rec := NewRecorder(app)

	// --- connect -------------------------------------------------------------------------------
	list := rec.ConnectionsList(t)
	if len(list) != 1 || list[0].ID != cfg.ID {
		t.Fatalf("connections list = %+v, want exactly one row for %s", list, cfg.ID)
	}
	if states := rec.ConnectionsStates(t); len(states) != 0 {
		t.Fatalf("connections states = %+v, want none", states)
	}
	state := rec.ConnectionsConnect(t, cfg.ID)
	if state.ServerVersion == nil || *state.ServerVersion != "Amazon SQS" {
		t.Fatalf("serverVersion = %v, want Amazon SQS", state.ServerVersion)
	}

	// opsRecent: a synthetic control snapshot, not a real backend call — this backend half never
	// issues an ENGINE_OP.describe-equivalent call for sqs at all (describe:false), so there is no
	// real op to list; the mocked frontend's own opsRecent is stubbed to `[]` by construction.
	rec.recordControl(channelOpsRecent, nil, []model.OpRecord{})

	// --- tree: a flat queue list, no nested level under any queue -------------------------------
	root := rec.TreeChildren(t, cfg.ID, "", false)
	if root.Source != "server" {
		t.Fatalf("root children source = %q, want server", root.Source)
	}
	ordersQueueNode := nodeByName(root.Nodes, testsupport.SQSOrdersQueue)
	emptyQueueNode := nodeByName(root.Nodes, testsupport.SQSEmptyQueue)
	if ordersQueueNode == nil || emptyQueueNode == nil || nodeByName(root.Nodes, testsupport.SQSDrainQueue) == nil {
		t.Fatalf("expected %s/%s/%s queue nodes in %+v", testsupport.SQSOrdersQueue, testsupport.SQSEmptyQueue, testsupport.SQSDrainQueue, root.Nodes)
	}

	// --- open the orders queue: batch pagination never auto-loads -------------------------------
	pollReq := adapterhost.ReadRequestWire{
		OpID: "be-poll-orders", ConnectionID: cfg.ID, Path: ordersQueueNode.Path,
		PageSize: 100, Cursor: model.PageCursor{Mode: "offset", Offset: 0},
	}
	pollResp, err := app.Dispatcher.Read(context.Background(), WireRead(pollReq))
	if err != nil {
		t.Fatalf("poll orders queue: %v", err)
	}
	pollLogical, err := DecodePage(pollResp.Page)
	if err != nil {
		t.Fatalf("decode poll page: %v", err)
	}
	pollStream, ok := pollLogical.(LogicalStreamPage)
	if !ok || len(pollStream.Keys) == 0 || pollStream.VisibilityTimeoutSeconds == nil {
		t.Fatalf("expected a non-empty stream page with a visibility timeout, got %+v", pollLogical)
	}
	// Three real, run-to-run-volatile values, all frozen the same way: the key column is SQS's own
	// randomly-generated MessageId (a fresh UUID every SendMessage call); timestamps is the receive
	// time; and attrs' own SentTimestamp/ApproximateFirstReceiveTimestamp are wall-clock epoch-ms
	// embedded inside each row's JSON string (ApproximateReceiveCount is real, stable data — kept).
	for i := range pollStream.Keys {
		k := fmt.Sprintf("msg-%d", i)
		pollStream.Keys[i] = &k
		ts := "2024-01-01T00:00:00.000Z"
		pollStream.Timestamps[i] = &ts
		if pollStream.Attrs[i] == nil {
			continue
		}
		var parsed map[string]any
		if err := json.Unmarshal([]byte(*pollStream.Attrs[i]), &parsed); err != nil {
			t.Fatalf("parse poll attrs %d: %v", i, err)
		}
		if _, ok := parsed["SentTimestamp"]; ok {
			parsed["SentTimestamp"] = "1700000000000"
		}
		if _, ok := parsed["ApproximateFirstReceiveTimestamp"]; ok {
			parsed["ApproximateFirstReceiveTimestamp"] = "1700000000000"
		}
		b, err := json.Marshal(parsed)
		if err != nil {
			t.Fatalf("marshal poll attrs %d: %v", i, err)
		}
		s := string(b)
		pollStream.Attrs[i] = &s
	}
	recordDataRead(rec, pollReq, pollStream, pollResp.Source)

	// A Poll click on the cell-editor-open scenario invalidates before reloading.
	rec.DataInvalidate(t, adapterhost.InvalidateRequestWire{ConnectionID: cfg.ID, Path: ordersQueueNode.Path})

	// --- empty queue: approximate count carries a stale-looking "~0 total" ----------------------
	// stream/state.ts's own runCount never sends a `refresh` field at all — CountRequestWire's own
	// zero value already matches, and frozen.go's refresh:false masking drops it from both sides.
	countReq := adapterhost.CountRequestWire{OpID: "be-count-empty", ConnectionID: cfg.ID, Path: emptyQueueNode.Path}
	rec.DataCount(t, countReq)

	// --- the orders queue's definition — an Attributes section, no console button --------------
	definitionResult, err := app.TreeSvc.Definition(bridge.TreeDescribeArgs{ConnectionID: cfg.ID, Path: ordersQueueNode.Path, Refresh: false, TabID: nil})
	if err != nil {
		t.Fatalf("orders queue definition: %v", err)
	}
	if len(definitionResult.Definition.Statements) == 0 {
		t.Fatalf("expected a non-empty definition statement")
	}
	frozenResult := tree.DefinitionResult{
		Definition: FreezeQueueTimestamps(FreezeDefinition(definitionResult.Definition)),
		Source:     definitionResult.Source,
	}
	// definition/state.ts's own load() only passes refresh:true on an explicit Refresh click — the
	// initial onMounted call omits the key entirely, so this first snapshot must too (masked away
	// by frozen.go's refresh:false rule); the explicit Refresh re-issues the identical request, so
	// one real call serves both snapshots.
	rec.recordControl(channelTreeDefinition, bridge.TreeDescribeArgs{ConnectionID: cfg.ID, Path: ordersQueueNode.Path, Refresh: false, TabID: nil}, frozenResult)
	rec.recordControl(channelTreeDefinition, bridge.TreeDescribeArgs{ConnectionID: cfg.ID, Path: ordersQueueNode.Path, Refresh: true, TabID: nil}, frozenResult)

	if maybeWriteFixture(t, rec, "sqs") {
		return
	}
	assertMatchesCommittedJSONFixture(t, rec, "testdata/sqs.fixture.json")
}
