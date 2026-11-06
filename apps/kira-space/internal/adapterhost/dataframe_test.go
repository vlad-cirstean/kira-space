package adapterhost

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	flatbuffers "github.com/google/flatbuffers/go"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/enginecache"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/page"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/page/wire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
)

func newTestRouter() *Router {
	deps := adapters.Deps{Log: func(level, message string) {}}
	return NewRouter(deps, enginecache.NewCache(enginecache.DefaultPageBudgetBytes, nil))
}

// mustFrame builds a renderer -> Go request frame, which stays JSON text (P11 D3) — only the
// response/event side (readSentFrame below) is FlatBuffers.
func mustFrame(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal frame: %v", err)
	}
	return b
}

func readSentFrame(t *testing.T, conn *fakeConn) *wire.Frame {
	t.Helper()
	return readSentFrameWithin(t, conn, time.Second)
}

func readSentFrameWithin(t *testing.T, conn *fakeConn, timeout time.Duration) *wire.Frame {
	t.Helper()
	select {
	case frame := <-conn.sent:
		if !wire.FrameBufferHasIdentifier(frame) {
			t.Fatalf("sent frame is missing the %q file identifier", wire.FrameIdentifier)
		}
		return wire.GetRootAsFrame(frame, 0)
	case <-time.After(timeout):
		t.Fatalf("no frame was sent to the session within %s", timeout)
		return nil
	}
}

// payloadTable extracts frame's union payload as a table pos'd at the concrete type payloadType
// says it is — Init it into the matching generated struct (wire.PingPayload, wire.CacheStats, ...).
func payloadTable(t *testing.T, frame *wire.Frame) flatbuffers.Table {
	t.Helper()
	var tab flatbuffers.Table
	if !frame.Payload(&tab) {
		t.Fatalf("frame has no payload table")
	}
	return tab
}

func assertNothingSent(t *testing.T, conn *fakeConn) {
	t.Helper()
	select {
	case frame := <-conn.sent:
		t.Fatalf("expected no frame, got %x", frame)
	case <-time.After(50 * time.Millisecond):
	}
}

// A connection's data:read is answered locally — the response frame is a real
// {kind:"res", ok:true, payload:{...}} built by the dispatcher.
func TestHandleDataFrame_NativeRead_RespondsLocally(t *testing.T) {
	r := newTestRouter()

	fake := &dataFakeAdapter{readFn: func() (page.Page, error) {
		return page.TabularPage{RowCount: 1, ByteSize: 5, Position: page.UnpagedPosition(1)}, nil
	}}
	adapters.SetLiveAdapter("conn-1", fake)
	defer adapters.DeleteLiveAdapter("conn-1")

	conn := newFakeConn()
	session, detach := r.AttachStream(conn)
	defer detach()

	frame := mustFrame(t, map[string]any{
		"kind": "req", "id": 1, "op": "data:read",
		"payload": map[string]any{
			"opId": "35aa4fd1-8a08-76b1-8044-5e6cf39f36fa", "tabId": nil, "connectionId": "conn-1", "path": "database:app/table:t",
			"projection": nil, "filter": nil, "sort": nil, "pageSize": 10,
			"cursor": map[string]any{"mode": "offset", "offset": 0},
		},
	})
	r.HandleDataFrame(session, frame)

	resp := readSentFrame(t, conn)
	if resp.Kind() != wire.FrameKindres || resp.Id() != 1 || !resp.Ok() {
		t.Fatalf("resp = kind:%v id:%d ok:%v, want a res/1/ok:true frame", resp.Kind(), resp.Id(), resp.Ok())
	}
	if fake.readCalls.Load() != 1 {
		t.Errorf("adapter.Read calls = %d, want 1", fake.readCalls.Load())
	}
}

// A connection with no live adapter attached still gets a real (error) response, not silence —
// there is no child left to forward an unrecognized connection's op to (P58f Phase 4).
func TestHandleDataFrame_NoLiveAdapter_RespondsWithError(t *testing.T) {
	r := newTestRouter()

	conn := newFakeConn()
	session, detach := r.AttachStream(conn)
	defer detach()

	frame := mustFrame(t, map[string]any{
		"kind": "req", "id": 2, "op": "data:read",
		"payload": map[string]any{"connectionId": "conn-2", "pageSize": 10, "cursor": map[string]any{"mode": "offset", "offset": 0}},
	})
	r.HandleDataFrame(session, frame)

	resp := readSentFrame(t, conn)
	if resp.Kind() != wire.FrameKindres || resp.Id() != 2 || resp.Ok() {
		t.Fatalf("resp = kind:%v id:%d ok:%v, want a res/2/ok:false frame", resp.Kind(), resp.Id(), resp.Ok())
	}
}

// Dispatcher.Preview deliberately never goes through Host.RunOp (it must not appear in the
// op log), so it never gets RunOp's own safeRun recover() boundary — and neither does Invalidate,
// the cache fast paths, or a wrong type assertion on a RunOp result, all of which also run on this
// frame's own goroutine outside RunOp. HandleDataFrame's own recover (dataframe.go) is what has to
// catch a panic anywhere in this dispatch instead: a panicking adapter must turn into an ordinary
// E_INTERNAL error response for this one request, not take the whole process down.
func TestHandleDataFrame_PreviewPanics_RespondsWithErrorInsteadOfCrashing(t *testing.T) {
	r := newTestRouter()

	fake := &dataFakeAdapter{previewFn: func() ([]string, error) {
		panic("boom")
	}}
	adapters.SetLiveAdapter("conn-preview-panic", fake)
	defer adapters.DeleteLiveAdapter("conn-preview-panic")

	conn := newFakeConn()
	session, detach := r.AttachStream(conn)
	defer detach()

	frame := mustFrame(t, map[string]any{
		"kind": "req", "id": 3, "op": "data:preview",
		"payload": map[string]any{
			"connectionId": "conn-preview-panic", "path": "database:app/table:t", "ops": []any{},
		},
	})
	r.HandleDataFrame(session, frame)

	resp := readSentFrame(t, conn)
	if resp.Kind() != wire.FrameKindres || resp.Id() != 3 || resp.Ok() {
		t.Fatalf("resp = kind:%v id:%d ok:%v, want a res/3/ok:false frame", resp.Kind(), resp.Id(), resp.Ok())
	}
	errObj := resp.Error(nil)
	if errObj == nil || string(errObj.Code()) != "E_INTERNAL" {
		t.Fatalf("resp error = %+v, want code E_INTERNAL", errObj)
	}

	// The panic must not have taken the session (or process) down — a follow-up request on the
	// same session still gets served normally.
	frame2 := mustFrame(t, map[string]any{"kind": "req", "id": 4, "op": "ping"})
	r.HandleDataFrame(session, frame2)
	resp2 := readSentFrame(t, conn)
	if resp2.Kind() != wire.FrameKindres || resp2.Id() != 4 || !resp2.Ok() {
		t.Fatalf("resp2 = kind:%v id:%d ok:%v, want the session still alive and answering ping", resp2.Kind(), resp2.Id(), resp2.Ok())
	}
}

// ping is answered locally — the engine is this process now (P58f D11) — with the same
// PingPayload shape rpc.ts's ping handler used to return.
func TestHandleDataFrame_Ping_AnsweredLocally(t *testing.T) {
	r := newTestRouter()
	conn := newFakeConn()
	session, detach := r.AttachStream(conn)
	defer detach()

	r.HandleDataFrame(session, mustFrame(t, map[string]any{"kind": "req", "id": 3, "op": "ping"}))
	resp := readSentFrame(t, conn)
	if resp.Kind() != wire.FrameKindres || resp.Id() != 3 || !resp.Ok() {
		t.Fatalf("resp = kind:%v id:%d ok:%v, want a res/3/ok:true frame", resp.Kind(), resp.Id(), resp.Ok())
	}
	if resp.PayloadType() != wire.PayloadPingPayload {
		t.Fatalf("resp.payloadType = %v, want PingPayload", resp.PayloadType())
	}
	var ping wire.PingPayload
	tab := payloadTable(t, resp)
	ping.Init(tab.Bytes, tab.Pos)
	if !ping.Pong() {
		t.Errorf("payload.pong = %v, want true", ping.Pong())
	}
	if int(ping.EnginePid()) != os.Getpid() {
		t.Errorf("payload.enginePid = %v, want %d", ping.EnginePid(), os.Getpid())
	}
	if ping.At() <= 0 {
		t.Errorf("payload.at = %v, want a positive unix-millis timestamp", ping.At())
	}
}

// cache:clear clears the Go-native cache synchronously and answers with a real {} response.
func TestHandleDataFrame_CacheClear_ClearsGoCache(t *testing.T) {
	r := newTestRouter()
	cacheReq := enginecache.ReadRequest{ConnectionID: "c", Path: "p", PageSize: 10, Cursor: model.PageCursor{Mode: "offset"}}
	key, label := enginecache.PageCacheKey(cacheReq)
	r.cache.StorePage(key, label, cacheReq, page.TabularPage{ByteSize: 5})
	if r.cache.Stats().L2Entries != 1 {
		t.Fatal("setup: expected the seeded page to be cached")
	}

	conn := newFakeConn()
	session, detach := r.AttachStream(conn)
	defer detach()
	r.HandleDataFrame(session, mustFrame(t, map[string]any{"kind": "req", "id": 4, "op": "cache:clear"}))

	if r.cache.Stats().L2Entries != 0 {
		t.Error("cache:clear must clear the Go-native cache")
	}
}

// D12: a session subscribes to the Go cache's stats-changed notifications on attach and gets an
// unsolicited cache:stats event frame when they fire; detach unsubscribes.
func TestAttachStream_PushesCacheStatsOnChange(t *testing.T) {
	r := newTestRouter()
	conn := newFakeConn()
	_, detach := r.AttachStream(conn)

	cacheReq := enginecache.ReadRequest{ConnectionID: "c", Path: "p", PageSize: 10, Cursor: model.PageCursor{Mode: "offset"}}
	key, label := enginecache.PageCacheKey(cacheReq)
	r.cache.StorePage(key, label, cacheReq, page.TabularPage{ByteSize: 5})

	// The cache's own emit is throttled to at most 1 Hz (scheduleEmitLocked); give it real margin
	// past that so this isn't a race against the cache's own timer.
	evt := readSentFrameWithin(t, conn, 3*time.Second)
	if evt.Kind() != wire.FrameKindevt || string(evt.Topic()) != "cache:stats" {
		t.Fatalf("evt = kind:%v topic:%q, want a cache:stats event frame", evt.Kind(), evt.Topic())
	}
	if evt.PayloadType() != wire.PayloadCacheStats {
		t.Fatalf("evt.payloadType = %v, want CacheStats", evt.PayloadType())
	}
	var stats wire.CacheStats
	tab := payloadTable(t, evt)
	stats.Init(tab.Bytes, tab.Pos)
	if stats.L2Entries() != 1 {
		t.Errorf("l2Entries = %v, want 1", stats.L2Entries())
	}

	detach()
	r.cache.Clear()
	assertNothingSent(t, conn)
}

// A response payload too large to ever fit in a data frame must still settle its pending
// request — respond() replaces it with a small error response rather than trying (and failing) to
// enqueue the oversized frame, which a renderer-side pending request (no client-side timeout of
// its own) would then wait on forever.
func TestRespond_OversizedPayloadBecomesErrorResponse(t *testing.T) {
	r := newTestRouter()
	conn := newFakeConn()
	session, detach := r.AttachStream(conn)
	defer detach()

	huge := strings.Repeat("a", maxResponsePayloadBytes+1024)
	r.respond(session, 9, PreviewResponse{Statements: []string{huge}}, nil)

	resp := readSentFrame(t, conn)
	if resp.Kind() != wire.FrameKindres || resp.Id() != 9 || resp.Ok() {
		t.Fatalf("resp = kind:%v id:%d ok:%v, want a res/9/ok:false frame", resp.Kind(), resp.Id(), resp.Ok())
	}
}

// TestRespond_OversizedPayloadDoesNotPanicWithNilLog: adapters.Deps.Log
// is a bare func field with no constructor enforcing it, and this package used to nil-check it in
// some call sites (safeRun's panic recovery) but not others (respond's own error-path logging,
// right here) — production always supplies one (main.go), so this was latent, but exactly the kind
// of inconsistency that turns a caller like this test — adapters.Deps{} with no Log, the same
// style host_test.go/throttle_test.go/data_test.go already use throughout this package — into a
// nil-func panic on the oversized-response path, the path least likely to ever be exercised by
// hand. NewRouter now normalises a nil Log to a no-op, so this must not panic.
func TestRespond_OversizedPayloadDoesNotPanicWithNilLog(t *testing.T) {
	r := NewRouter(adapters.Deps{}, enginecache.NewCache(enginecache.DefaultPageBudgetBytes, nil))
	conn := newFakeConn()
	session, detach := r.AttachStream(conn)
	defer detach()

	huge := strings.Repeat("a", maxResponsePayloadBytes+1024)
	r.respond(session, 9, PreviewResponse{Statements: []string{huge}}, nil)

	resp := readSentFrame(t, conn)
	if resp.Kind() != wire.FrameKindres || resp.Id() != 9 || resp.Ok() {
		t.Fatalf("resp = kind:%v id:%d ok:%v, want a res/9/ok:false frame", resp.Kind(), resp.Id(), resp.Ok())
	}
}

// respond() used to always call encodeResponse (allocating and copying the whole
// FlatBuffers frame, ~1.06x the page's own byte size per docs/PERF.md) before ever checking
// whether the result would be too large to send — for a ReadResponse/ExecuteResponse, the page's
// own Size() already answers that question. This proves the pre-check runs before
// encoding, not just that an oversized *encoded* frame is rejected (TestRespond_
// OversizedPayloadBecomesErrorResponse, above, already covers that): a page reporting a huge
// ByteSize but genuinely empty Chunks/Columns would encode to a tiny frame if encodeResponse ran
// on it — so the only way this can produce an error response is the pre-check firing on Size()
// alone.
func TestRespond_OversizedPageIsRejectedBeforeEncoding(t *testing.T) {
	r := newTestRouter()
	conn := newFakeConn()
	session, detach := r.AttachStream(conn)
	defer detach()

	hugePage := page.TabularPage{
		Kind:     page.PageKindTabular,
		ByteSize: maxResponsePayloadBytes + 1024, // reported size only — no data backs this
	}
	r.respond(session, 11, ReadResponse{Page: hugePage, Source: "server"}, nil)

	resp := readSentFrame(t, conn)
	if resp.Kind() != wire.FrameKindres || resp.Id() != 11 || resp.Ok() {
		t.Fatalf("resp = kind:%v id:%d ok:%v, want a res/11/ok:false frame", resp.Kind(), resp.Id(), resp.Ok())
	}
	errObj := resp.Error(nil)
	if errObj == nil || string(errObj.Code()) != string(adapters.CodeQuery) {
		t.Fatalf("resp error = %+v, want code %s", errObj, adapters.CodeQuery)
	}
}
