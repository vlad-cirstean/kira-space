package adapterhost

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/enginecache"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/oplog"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/page"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// Cancel must find an op this router's own scheduler started.
func TestRouter_Cancel_FindsInProcessOp(t *testing.T) {
	r := NewRouter(adapters.Deps{}, enginecache.NewCache(enginecache.DefaultPageBudgetBytes, nil))
	started := make(chan struct{})
	done := make(chan struct{})
	go func() {
		_, _, _ = r.host.RunOp(context.Background(), OpSpec{OpID: "7151ae70-f947-7c7b-8319-be012b23b17b", Kind: "read"},
			func(ctx context.Context, op *adapters.OpCtx) (any, error) {
				close(started)
				<-ctx.Done()
				return nil, ctx.Err()
			})
		close(done)
	}()
	<-started

	ok, err := r.Cancel(context.Background(), "7151ae70-f947-7c7b-8319-be012b23b17b")
	if err != nil || !ok {
		t.Fatalf("Cancel = %v, %v, want true, nil", ok, err)
	}
	<-done
}

// CancelRunning must not remember a miss, must refuse a Cancellable id, and must cancel a listed op.
func TestRouter_CancelRunning(t *testing.T) {
	r := NewRouter(adapters.Deps{}, enginecache.NewCache(enginecache.DefaultPageBudgetBytes, nil))
	ctx := context.Background()
	if ok, _ := r.CancelRunning(ctx, "e2494bee-5f55-76bb-881f-852d4507fee7"); ok {
		t.Fatal("CancelRunning on unknown id = true")
	}
	if _, _, err := r.host.RunOp(ctx, OpSpec{OpID: "e2494bee-5f55-76bb-881f-852d4507fee7", Kind: "read"},
		func(context.Context, *adapters.OpCtx) (any, error) { return nil, nil }); err != nil {
		t.Fatalf("op after missed CancelRunning: %v", err)
	}

	_, release, err := r.host.Cancellable(ctx, "0268031b-c591-7d20-83a9-540ab9658bd6")
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if ok, _ := r.CancelRunning(ctx, "0268031b-c591-7d20-83a9-540ab9658bd6"); ok || len(r.RunningOps()) != 0 {
		t.Fatal("Cancellable id must be neither cancellable nor listed")
	}

	started := make(chan struct{})
	var runErr error
	done := make(chan struct{})
	go func() {
		_, _, runErr = r.host.RunOp(ctx, OpSpec{OpID: "f2496cb9-9e25-7601-8d37-1ca42becacb8", Kind: "execute"},
			func(c context.Context, _ *adapters.OpCtx) (any, error) {
				close(started)
				<-c.Done()
				return nil, c.Err()
			})
		close(done)
	}()
	<-started
	ops := r.RunningOps()
	if len(ops) != 1 || ops[0].OpID != "f2496cb9-9e25-7601-8d37-1ca42becacb8" || ops[0].Kind != "execute" {
		t.Fatalf("RunningOps = %+v", ops)
	}
	if ok, err := r.CancelRunning(ctx, "f2496cb9-9e25-7601-8d37-1ca42becacb8"); !ok || err != nil {
		t.Fatalf("CancelRunning = %v, %v", ok, err)
	}
	<-done
	if runErr == nil {
		t.Fatal("cancelled op returned no error")
	}
}

// childrenNilAdapter returns a nil Nodes slice, the idiomatic-Go shape (`var result
// []model.TreeNode`, never appended to) that json.Marshal renders as `null`.
type childrenNilAdapter struct {
	adapters.Adapter
}

func (childrenNilAdapter) Children(ctx context.Context, path model.NodePath, op *adapters.OpCtx) (adapters.TreeChildren, error) {
	var nodes []model.TreeNode
	return adapters.TreeChildren{Nodes: nodes}, nil
}

// A native adapter's nil Nodes slice must cross the wire as `[]`, never `null` — the same
// hazard Describe/Definition were already fixed for. Asserting on the
// marshalled JSON, not len(nodes)==0, is the point: len() is 0 for both nil and empty slices, so a
// test that only checked length would pass on the very bug this guards against.
func TestRouter_Children_NilNodesMarshalAsEmptyArray(t *testing.T) {
	const connID = "conn-children-nil"
	adapters.SetLiveAdapter(connID, childrenNilAdapter{})
	defer adapters.DeleteLiveAdapter(connID)

	r := NewRouter(adapters.Deps{}, enginecache.NewCache(enginecache.DefaultPageBudgetBytes, nil))
	children, err := r.Children(context.Background(), connID, model.NodePath{ConnectionID: connID})
	if err != nil {
		t.Fatalf("Children: %v", err)
	}
	if children.Nodes == nil {
		t.Fatal("Children must normalize a nil Nodes slice before returning")
	}

	b, err := json.Marshal(children)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !strings.Contains(string(b), `"Nodes":[]`) {
		t.Errorf("marshalled = %s, want a \"Nodes\":[] field, not null", b)
	}
}

// describeDefinitionFakeAdapter answers Describe/Definition with minimal fixtures.
type describeDefinitionFakeAdapter struct {
	adapters.Adapter
}

func (describeDefinitionFakeAdapter) Describe(ctx context.Context, path model.NodePath, op *adapters.OpCtx) (model.ObjectMeta, error) {
	return model.ObjectMeta{}, nil
}

func (describeDefinitionFakeAdapter) Definition(ctx context.Context, path model.NodePath, op *adapters.OpCtx) (model.ObjectDefinition, error) {
	return model.ObjectDefinition{}, nil
}

// Describe/Definition must thread tabID through to the op:start event the same way every
// other tab-scoped op does — the frontend's per-tab op log and status bar key off it. router.go
// used to accept tabID from tree.Backend's own signature and then never put it in the OpSpec it
// built, so op:start always reported a nil TabID for these two kinds.
func TestRouter_DescribeAndDefinition_ThreadTabIDIntoOpStart(t *testing.T) {
	const connID = "conn-tabid"
	adapters.SetLiveAdapter(connID, describeDefinitionFakeAdapter{})
	defer adapters.DeleteLiveAdapter(connID)

	r := NewRouter(adapters.Deps{}, enginecache.NewCache(enginecache.DefaultPageBudgetBytes, nil))
	events, unsubscribe := r.Host().Subscribe()
	defer unsubscribe()

	awaitOpStartTabID := func(t *testing.T, kind string) *string {
		t.Helper()
		for {
			select {
			case evt := <-events:
				if evt.Topic != oplog.DbEventOpStart {
					continue
				}
				var payload struct {
					Kind  string  `json:"kind"`
					TabID *string `json:"tabId"`
				}
				if err := json.Unmarshal(evt.Payload, &payload); err != nil {
					t.Fatalf("unmarshal op:start payload: %v", err)
				}
				if payload.Kind != kind {
					continue
				}
				return payload.TabID
			case <-time.After(time.Second):
				t.Fatalf("no op:start event for kind %q within 1s", kind)
				return nil
			}
		}
	}

	tabID := "tab-1"
	if _, err := r.Describe(context.Background(), connID, model.NodePath{ConnectionID: connID}, &tabID); err != nil {
		t.Fatalf("Describe: %v", err)
	}
	if got := awaitOpStartTabID(t, "describe"); got == nil || *got != tabID {
		t.Errorf("describe op:start TabID = %v, want %q", got, tabID)
	}

	if _, err := r.Definition(context.Background(), connID, model.NodePath{ConnectionID: connID}, &tabID); err != nil {
		t.Fatalf("Definition: %v", err)
	}
	if got := awaitOpStartTabID(t, "definition"); got == nil || *got != tabID {
		t.Errorf("definition op:start TabID = %v, want %q", got, tabID)
	}
}

// reconnectFakeAdapter is just enough of adapters.Adapter to exercise Connect's own
// existing-live-adapter branch — Connect/Disconnect/Caps, nothing else (every other method panics
// via the embedded nil interface if a test accidentally reaches it).
type reconnectFakeAdapter struct {
	adapters.Adapter
}

func (reconnectFakeAdapter) Connect(context.Context, model.ResolvedConnectionConfig, *adapters.OpCtx) (adapters.ConnectInfo, error) {
	return adapters.ConnectInfo{}, nil
}
func (reconnectFakeAdapter) Disconnect(context.Context) error { return nil }
func (reconnectFakeAdapter) Caps() adapters.Caps              { return adapters.Caps{} }

// Connect's own "a live adapter is already registered" branch is a real reconnect path
// (reached whenever connectionsConnect races ahead of onPreconnectExit's async Disconnect) —
// unlike Disconnect, it never called cache.DropConnection, so L2 pages and L3 counts keyed by the
// old connectionId survived the reconnect and could be served back as stale results against the
// newly connected adapter.
func TestRouter_Connect_ReconnectDropsStaleCache(t *testing.T) {
	const kind = "test-reconnect-fake"
	adapters.Register(kind, func(adapters.Deps) (adapters.Adapter, error) {
		return reconnectFakeAdapter{}, nil
	})
	const connID = "conn-reconnect"
	adapters.DeleteLiveAdapter(connID)

	r := NewRouter(adapters.Deps{}, enginecache.NewCache(enginecache.DefaultPageBudgetBytes, nil))
	cfg := model.ResolvedConnectionConfig{ID: connID, Kind: kind}

	if _, err := r.Connect(context.Background(), cfg); err != nil {
		t.Fatalf("first Connect: %v", err)
	}

	cacheReq := enginecache.ReadRequest{ConnectionID: connID, Path: "p", PageSize: 10, Cursor: model.PageCursor{Mode: "offset"}}
	key, label := enginecache.PageCacheKey(cacheReq)
	r.cache.StorePage(key, label, cacheReq, page.TabularPage{ByteSize: 5})
	if r.cache.Stats().L2Entries != 1 {
		t.Fatal("setup: expected the seeded page to be cached")
	}

	// Reconnect while the first adapter is still registered live — Connect's own
	// existing-live-adapter branch, not Disconnect.
	if _, err := r.Connect(context.Background(), cfg); err != nil {
		t.Fatalf("second Connect (reconnect): %v", err)
	}

	if r.cache.Stats().L2Entries != 0 {
		t.Error("reconnect must drop the old connection's cached pages, not just swap the live adapter")
	}
}
