package adapterhost

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/enginecache"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/oplog"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/page"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/storage/model"
)

// dataFakeAdapter is a fuller test double than host_test.go's fakeAdapter: every method the
// dispatcher can reach is overridable, so a test can assert both what was called and how many
// times. readCalls is atomic (a concurrency test now calls Read() from many goroutines at
// once against one shared fake).
type dataFakeAdapter struct {
	adapters.Adapter
	readCalls atomic.Int64
	readFn    func() (page.Page, error)
	// readCtxFn, when set, is used instead of readFn — for a test that needs to observe the ctx
	// Read() was actually called with (P2 R1: session cancellation propagation).
	readCtxFn func(ctx context.Context) (page.Page, error)
	mutateFn  func() (model.MutationResult, error)
	previewFn func() ([]string, error)
}

func (a *dataFakeAdapter) Read(ctx context.Context, req adapters.ReadRequest, op *adapters.OpCtx) (page.Page, error) {
	a.readCalls.Add(1)
	if a.readCtxFn != nil {
		return a.readCtxFn(ctx)
	}
	return a.readFn()
}
func (a *dataFakeAdapter) Mutate(ctx context.Context, plan model.MutationPlan, op *adapters.OpCtx) (model.MutationResult, error) {
	return a.mutateFn()
}
func (a *dataFakeAdapter) Preview(plan model.MutationPlan) ([]string, error) {
	return a.previewFn()
}
func newDispatcher() (*Dispatcher, *Host) {
	cache := enginecache.NewCache(enginecache.DefaultPageBudgetBytes, nil)
	host := NewHost(adapters.Deps{}, cache)
	return NewDispatcher(host, cache), host
}

const testConnID = "conn-read"

func readReq(pageSize int) ReadRequestWire {
	return ReadRequestWire{
		OpID: "ff6ba664-c8f9-7f08-844d-90db1fa6759e", ConnectionID: testConnID, Path: "database:app/table:t",
		PageSize: pageSize, Cursor: model.PageCursor{Mode: "offset", Offset: 0},
	}
}

// A cache miss runs the op (adapter.Read called, op:start/op:end emitted) and stores the result; a
// repeat of the exact same request is then a cache hit — server not touched again, and (the load-
// bearing assertion) no new op:start/op:end pair is emitted, because "a cache hit is not a
// database operation" (the contract: it must not appear in the op log).
func TestDispatcher_Read_CacheAsideDiscipline(t *testing.T) {
	d, host := newDispatcher()
	fake := &dataFakeAdapter{readFn: func() (page.Page, error) {
		return page.TabularPage{RowCount: 3, ByteSize: 30}, nil
	}}
	adapters.SetLiveAdapter(testConnID, fake)
	defer adapters.DeleteLiveAdapter(testConnID)

	events, unsubscribe := host.Subscribe()
	defer unsubscribe()

	resp1, err := d.Read(context.Background(), readReq(10))
	if err != nil {
		t.Fatalf("first Read: %v", err)
	}
	if resp1.Source != "server" {
		t.Errorf("first Read source = %q, want server", resp1.Source)
	}
	if fake.readCalls.Load() != 1 {
		t.Fatalf("adapter.Read calls = %d, want 1", fake.readCalls.Load())
	}
	<-events // op:start
	<-events // op:end

	resp2, err := d.Read(context.Background(), readReq(10))
	if err != nil {
		t.Fatalf("second Read: %v", err)
	}
	if resp2.Source != "cache" {
		t.Errorf("second Read source = %q, want cache", resp2.Source)
	}
	if fake.readCalls.Load() != 1 {
		t.Errorf("adapter.Read calls after cache hit = %d, want still 1", fake.readCalls.Load())
	}
	select {
	case e := <-events:
		t.Fatalf("a cache hit must not emit an op event, got %+v", e)
	default:
	}
}

// Mutate's cache invalidation must run even when the mutation itself fails (P43 F12/D17): six of
// the eleven adapters mutate without a transaction, so a partially-applied plan still changed the
// server, and the pre-mutation page must not be left cached as a (now wrong) hit.
func TestDispatcher_Mutate_InvalidatesEvenOnFailure(t *testing.T) {
	d, _ := newDispatcher()
	fake := &dataFakeAdapter{mutateFn: func() (model.MutationResult, error) {
		return model.MutationResult{}, adapters.New(adapters.CodeQuery, "boom", nil)
	}}
	adapters.SetLiveAdapter(testConnID, fake)
	defer adapters.DeleteLiveAdapter(testConnID)

	// Seed a page and a count for this target directly through the same Cache the dispatcher
	// uses, so the test can observe whether Mutate's defer actually ran.
	req := readReq(10)
	key, label := enginecache.PageCacheKey(enginecache.ReadRequest{
		ConnectionID: req.ConnectionID, Path: req.Path, PageSize: req.PageSize, Cursor: req.Cursor,
	})
	d.cache.StorePage(key, label, enginecache.ReadRequest{ConnectionID: req.ConnectionID, Path: req.Path, PageSize: req.PageSize, Cursor: req.Cursor}, page.TabularPage{ByteSize: 10})

	_, err := d.Mutate(context.Background(), MutateRequestWire{
		OpID: "2cd6ccfb-07e4-75b8-88ad-2b0969075511", ConnectionID: testConnID, Path: req.Path,
		Ops: []model.MutationRowOp{{Kind: "delete", Key: model.RowValues{{Name: "id", Value: strp("1")}}}},
	})
	if err == nil {
		t.Fatal("expected the mutate failure to propagate")
	}
	if _, ok := d.cache.ReadPage(key); ok {
		t.Error("a failed mutate must still drop the target's cached pages")
	}
}

// A Read whose cache miss is still in flight when a concurrent mutation
// invalidates its exact target must not cache its own now-stale, pre-invalidation result once it
// finally returns. Genuinely concurrent (a real goroutine race, not a simulated ordering) —
// confirmed hanging on nothing pre-fix but *failing* the final assertion pre-fix via a scoped
// `git stash` on enginecache/{cache,generation}.go and adapterhost/data.go (pre-fix, the read's
// result lands in cache regardless of the concurrent Invalidate).
func TestDispatcher_Read_DoesNotCacheStaleResultRacingConcurrentInvalidate(t *testing.T) {
	d, _ := newDispatcher()
	const connID, path = "conn-race", "database:app/table:t"

	readStarted := make(chan struct{})
	proceed := make(chan struct{})
	fake := &dataFakeAdapter{readCtxFn: func(ctx context.Context) (page.Page, error) {
		close(readStarted)
		<-proceed // held open until the test has invalidated this exact target underneath it
		return page.TabularPage{RowCount: 1, ByteSize: 10}, nil
	}}
	adapters.SetLiveAdapter(connID, fake)
	defer adapters.DeleteLiveAdapter(connID)

	req := ReadRequestWire{
		OpID: "97344332-9e99-7dad-8342-8cf8696c9c6e", ConnectionID: connID, Path: path, PageSize: 10,
		Cursor: model.PageCursor{Mode: "offset", Offset: 0},
	}

	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		if _, err := d.Read(context.Background(), req); err != nil {
			t.Errorf("Read: %v", err)
		}
	}()
	<-readStarted

	// A concurrent mutation on the very same target, landing while the read above is still
	// mid-flight — this is the exact race F4 guards against.
	d.cache.InvalidateAfterMutation(connID, path)
	close(proceed)
	<-readDone

	key, _ := enginecache.PageCacheKey(enginecache.ReadRequest{
		ConnectionID: connID, Path: path, PageSize: req.PageSize, Cursor: req.Cursor,
	})
	if _, ok := d.cache.ReadPage(key); ok {
		t.Error("a Read racing a concurrent InvalidateAfterMutation must not cache its own stale result")
	}
}

// Invalidate's two scopes: "pages" drops only L2 (the post-mutation reload must leave the stale
// count mark intact); anything else (including the empty default) drops both, hard.
func TestDispatcher_Invalidate_ScopeRouting(t *testing.T) {
	d, _ := newDispatcher()
	const connID, path = "conn-inv", "database:app/table:t"
	filter := "x = 1"

	seed := func() string {
		req := ReadRequestWire{ConnectionID: connID, Path: path, PageSize: 10, Cursor: model.PageCursor{Mode: "offset"}}
		key, label := enginecache.PageCacheKey(enginecache.ReadRequest{ConnectionID: connID, Path: path, PageSize: 10, Cursor: req.Cursor})
		d.cache.StorePage(key, label, enginecache.ReadRequest{ConnectionID: connID, Path: path, PageSize: 10, Cursor: req.Cursor}, page.TabularPage{ByteSize: 5})
		d.cache.StoreCount(connID, path, &filter, 7, true)
		return key
	}

	key := seed()
	d.Invalidate(InvalidateRequestWire{ConnectionID: connID, Path: path, Scope: "pages"})
	if _, ok := d.cache.ReadPage(key); ok {
		t.Error("scope=pages must drop the cached page")
	}
	if _, ok := d.cache.Count(connID, path, &filter); !ok {
		t.Error("scope=pages must leave the count intact")
	}

	key = seed()
	d.Invalidate(InvalidateRequestWire{ConnectionID: connID, Path: path})
	if _, ok := d.cache.ReadPage(key); ok {
		t.Error("the default scope must drop the cached page")
	}
	if _, ok := d.cache.Count(connID, path, &filter); ok {
		t.Error("the default scope must drop the count too")
	}
}

type opIDFake struct{ dataFakeAdapter }

func (*opIDFake) Count(context.Context, adapters.CountRequest, *adapters.OpCtx) (adapters.CountResult, error) {
	return adapters.CountResult{Value: 1, Exact: true}, nil
}

func (*opIDFake) DownloadObject(context.Context, model.ObjectDownloadRequest, *adapters.OpCtx) (model.ObjectTransferResult, error) {
	return model.ObjectTransferResult{Bytes: 1}, nil
}

// Count, Mutate and ObjectDownload take no opId from the renderer: Go mints a UUIDv7 for each.
func TestDispatcher_EmptyOpIDMinted(t *testing.T) {
	d, host := newDispatcher()
	fake := &opIDFake{dataFakeAdapter{mutateFn: func() (model.MutationResult, error) { return model.MutationResult{}, nil }}}
	adapters.SetLiveAdapter(testConnID, fake)
	defer adapters.DeleteLiveAdapter(testConnID)
	events, unsubscribe := host.Subscribe()
	defer unsubscribe()

	ctx := context.Background()
	path := readReq(10).Path
	if _, err := d.Count(ctx, CountRequestWire{ConnectionID: testConnID, Path: path, Refresh: true}); err != nil {
		t.Fatalf("Count: %v", err)
	}
	del := []model.MutationRowOp{{Kind: "delete", Key: model.RowValues{{Name: "id", Value: strp("1")}}}}
	if _, err := d.Mutate(ctx, MutateRequestWire{ConnectionID: testConnID, Path: path, Ops: del}); err != nil {
		t.Fatalf("Mutate: %v", err)
	}
	if _, err := d.ObjectDownload(ctx, ObjectDownloadRequestWire{ConnectionID: testConnID, Path: path, DestPath: "/tmp/x"}); err != nil {
		t.Fatalf("ObjectDownload: %v", err)
	}
	for _, kind := range []string{"count", "mutate", "transfer"} {
		e := <-events
		var p opStartPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil || e.Topic != oplog.EventOpStart || p.Kind != kind {
			t.Fatalf("event %+v (%v), want op:start for %s", e, err, kind)
		}
		if u, err := uuid.Parse(p.OpID); err != nil || u.Version() != 7 {
			t.Errorf("%s op id %q is not a v7 UUID", kind, p.OpID)
		}
		<-events // op:end
	}
}
