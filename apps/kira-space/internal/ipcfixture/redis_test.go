package ipcfixture

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapterhost"
	_ "github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters/redis"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters/testsupport"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/page"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiotmp/bridge"
)

var redisServerVersion = regexp.MustCompile(`^Redis \d+\.\d+`)

func int64p(i int64) *int64 { return &i }

func sortedPtrStrings(in []*string) []string {
	out := make([]string, len(in))
	for i, s := range in {
		if s != nil {
			out[i] = *s
		}
	}
	sort.Strings(out)
	return out
}

func sortedMapKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// sortKeyValueFields is redis.backend.spec.ts's own sortKeyValueFields, ported: HSCAN's
// cursor-round order is not guaranteed stable across separate scans of a hashtable-encoded hash,
// so a hash's field/value rows are sorted by field name before ever reaching the fixture, exactly
// as the deleted TypeScript harness did — the UI never depends on HSCAN's own order either.
func sortKeyValueFields(p LogicalKeyValuePage) LogicalKeyValuePage {
	order := make([]int, len(p.Fields))
	for i := range order {
		order[i] = i
	}
	key := func(i int) string {
		if p.Fields[i] == nil {
			return ""
		}
		return *p.Fields[i]
	}
	sort.SliceStable(order, func(a, b int) bool { return key(order[a]) < key(order[b]) })
	fields := make([]*string, len(p.Fields))
	values := make([]*string, len(p.Values))
	for i, idx := range order {
		fields[i] = p.Fields[idx]
		values[i] = p.Values[idx]
	}
	p.Fields, p.Values = fields, values
	return p
}

// syntheticHashPage is redis.backend.spec.ts's own syntheticHashPage: a large (hashtable-encoded)
// hash's HSCAN boundary is not stable across separate captures even sorted (confirmed empirically
// by the TypeScript harness this replaces) — real backend behaviour (a page of rows, a working next
// cursor, a non-empty second page) is asserted against the real HSCAN result at each call site
// below; only the *fixture* — which the frontend half never inspects beyond a DOM row count — is
// replaced with this deterministic stand-in, so the committed file stops churning on every
// regeneration.
func syntheticHashPage(startIndex, count int, nextToken, prevToken *string, hasMore bool) LogicalKeyValuePage {
	fields := make([]*string, count)
	values := make([]*string, count)
	for i := 0; i < count; i++ {
		f := fmt.Sprintf("f%d", startIndex+i)
		v := fmt.Sprintf("v%d", startIndex+i)
		fields[i] = &f
		values[i] = &v
	}
	return LogicalKeyValuePage{
		Kind: "keyvalue", RedisType: "hash", TTLMs: nil, MemoryBytes: int64p(512),
		Fields: fields, Values: values,
		Position: page.PagePosition{
			Offset: nil, PageSize: count, HasMore: hasMore, NextToken: nextToken, PrevToken: prevToken,
			Strategy: "cursor",
		},
	}
}

// recordDataRead appends a DATA_OP.read port snapshot with payload/logical values the caller has
// already computed (sorted, frozen, or wholly synthetic) — used in place of the Recorder.DataRead
// convenience wrapper wherever the recorded entry must diverge from the real dispatch call's own
// decoded response (the hash-sort and big-hash-synthesis findings, both carried over verbatim
// from redis.backend.spec.ts).
func recordDataRead(rec *Recorder, payload any, logical any, source string) {
	response := struct {
		Kind   string `json:"kind"`
		Page   any    `json:"page"`
		Source string `json:"source"`
	}{Kind: "read", Page: logical, Source: source}
	rec.recordPort(dataOpRead, payload, response, nil)
}

// TestFixture_Redis is the same discipline as the other four adapters, against
// tests/ipc/redis/redis.fixture.ts's own committed scenario — connect, tree traversal through
// db0/db1 and the namespace levels under db0, a small-hash read (sorted for determinism), a big
// (hashtable-encoded) hash's cursor pagination (its fixture entries replaced with a deterministic
// synthetic stand-in, its real dispatch calls still asserted for the real invariants), two
// data:invalidate calls, a list read, a TTL-key read (its volatile ttlMs/memoryBytes frozen after
// being validated live), a delete mutation, a before/after-delete pair of tree listings captured
// out of natural call order, and a console DBSIZE.
func TestFixture_Redis(t *testing.T) {
	fixture := testsupport.StartRedis(t)
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
	if !redisServerVersion.MatchString(*state.ServerVersion) {
		t.Fatalf("serverVersion = %q, want to match %s", *state.ServerVersion, redisServerVersion)
	}

	// --- 1: root -> db0, db1, both leaves (no key-browsing twisty) ------------------------------
	root := rec.TreeChildren(t, cfg.ID, "", false)
	if root.Source != "server" {
		t.Fatalf("root children source = %q, want server", root.Source)
	}
	db0Node := nodeByName(root.Nodes, "db0")
	db1Node := nodeByName(root.Nodes, "db1")
	if db0Node == nil || db1Node == nil {
		t.Fatalf("expected db0 and db1 database nodes in %+v", root.Nodes)
	}

	// --- 2: db1's own browse level — one namespace, no truncated strip --------------------------
	db1Children := rec.TreeChildren(t, cfg.ID, db1Node.Path, false)
	if db1Children.Truncated {
		t.Fatalf("db1 children truncated = true, want false")
	}
	if nodeByName(db1Children.Nodes, "other-db") == nil {
		t.Fatalf("expected an other-db namespace node in %+v", db1Children.Nodes)
	}

	// --- 3: db0 -> user -> 1 -> the hash key, one children() call per level ---------------------
	db0Children := rec.TreeChildren(t, cfg.ID, db0Node.Path, false)
	userNsNode := nodeByName(db0Children.Nodes, "user")
	queueNsNode := nodeByName(db0Children.Nodes, "queue")
	sessionNsNode := nodeByName(db0Children.Nodes, "session")
	if userNsNode == nil || queueNsNode == nil || sessionNsNode == nil {
		t.Fatalf("expected user/queue/session namespace nodes in %+v", db0Children.Nodes)
	}

	userChildren := rec.TreeChildren(t, cfg.ID, userNsNode.Path, false)
	user1NsNode := nodeByName(userChildren.Nodes, "1")
	if user1NsNode == nil {
		t.Fatalf("expected a 1 namespace node in %+v", userChildren.Nodes)
	}

	user1Children := rec.TreeChildren(t, cfg.ID, user1NsNode.Path, false)
	if len(user1Children.Nodes) != 4 {
		t.Fatalf("user/1 children = %+v, want exactly 4 (name, email, profile, bighash)", user1Children.Nodes)
	}
	hashKeyNode := nodeByName(user1Children.Nodes, testsupport.RedisHashKey)
	bigHashKeyNode := nodeByName(user1Children.Nodes, testsupport.RedisBigHashKey)
	if hashKeyNode == nil || bigHashKeyNode == nil {
		t.Fatalf("expected %s and %s key nodes in %+v", testsupport.RedisHashKey, testsupport.RedisBigHashKey, user1Children.Nodes)
	}

	// --- 4: hash key tab — type badge, field/value rows -----------------------------------------
	hashReq := adapterhost.ReadRequestWire{
		OpID: "be-read-hash", ConnectionID: cfg.ID, Path: hashKeyNode.Path,
		PageSize: 100, Cursor: model.PageCursor{Mode: "offset", Offset: 0},
	}
	hashResp, err := app.Dispatcher.Read(context.Background(), WireRead(hashReq))
	if err != nil {
		t.Fatalf("read hash: %v", err)
	}
	hashLogical, err := DecodePage(hashResp.Page)
	if err != nil {
		t.Fatalf("decode hash page: %v", err)
	}
	hashKV, ok := hashLogical.(LogicalKeyValuePage)
	if !ok || hashKV.RedisType != "hash" {
		t.Fatalf("expected a hash keyvalue page, got %+v", hashLogical)
	}
	gotFields := sortedPtrStrings(hashKV.Fields)
	wantFields := sortedMapKeys(testsupport.RedisHashFields)
	if !stringSlicesEqual(gotFields, wantFields) {
		t.Fatalf("hash fields = %v, want %v", gotFields, wantFields)
	}
	recordDataRead(rec, hashReq, sortKeyValueFields(hashKV), hashResp.Source)

	// --- 5: a keyvalue reload calls data:invalidate before re-reading ---------------------------
	rec.DataInvalidate(t, adapterhost.InvalidateRequestWire{ConnectionID: cfg.ID, Path: hashKeyNode.Path})

	// --- 6: big hash key — cursor pagination, two pages forward then a Refresh back to one -------
	bigHashBaseReq := adapterhost.ReadRequestWire{
		OpID: "be-read-bighash-1", ConnectionID: cfg.ID, Path: bigHashKeyNode.Path,
		PageSize: 100, Cursor: model.PageCursor{Mode: "offset", Offset: 0},
	}
	bigHashResp1, err := app.Dispatcher.Read(context.Background(), WireRead(bigHashBaseReq))
	if err != nil {
		t.Fatalf("read bighash page 1: %v", err)
	}
	bigHashLogical1, err := DecodePage(bigHashResp1.Page)
	if err != nil {
		t.Fatalf("decode bighash page 1: %v", err)
	}
	bigHashKV1, ok := bigHashLogical1.(LogicalKeyValuePage)
	if !ok {
		t.Fatalf("expected a keyvalue page, got %+v", bigHashLogical1)
	}
	// HSCAN's COUNT is a hint, not a guarantee — the only real invariant is "some rows, more to
	// come" (redis.backend.spec.ts's own comment on this exact call).
	if len(bigHashKV1.Fields) == 0 || !bigHashKV1.Position.HasMore {
		t.Fatalf("bighash page 1 = fields=%d hasMore=%v, want >0/true", len(bigHashKV1.Fields), bigHashKV1.Position.HasMore)
	}
	syntheticToken1 := "synthetic-bighash-cursor-1"
	// Refresh re-sends this exact same page-one request (a cursor page cannot be resumed) — one
	// snapshot serves both; its fields/values/tokens are synthetic, not the real HSCAN result.
	recordDataRead(rec, bigHashBaseReq, syntheticHashPage(0, 100, &syntheticToken1, nil, true), "server")

	if bigHashKV1.Position.NextToken == nil || *bigHashKV1.Position.NextToken == "" {
		t.Fatalf("expected the first bighash page to have a next cursor token")
	}
	bigHashReq2 := bigHashBaseReq
	bigHashReq2.OpID = "be-read-bighash-2"
	bigHashReq2.Cursor = model.PageCursor{Mode: "after", Token: *bigHashKV1.Position.NextToken}
	bigHashResp2, err := app.Dispatcher.Read(context.Background(), WireRead(bigHashReq2))
	if err != nil {
		t.Fatalf("read bighash page 2: %v", err)
	}
	bigHashLogical2, err := DecodePage(bigHashResp2.Page)
	if err != nil {
		t.Fatalf("decode bighash page 2: %v", err)
	}
	bigHashKV2, ok := bigHashLogical2.(LogicalKeyValuePage)
	if !ok || len(bigHashKV2.Fields) == 0 {
		t.Fatalf("expected a non-empty keyvalue page, got %+v", bigHashLogical2)
	}
	// The frontend half's own "next" click sends the cursor token it read off page 1's mocked
	// response — the synthetic token above, not the real one — so the fixture's page-2 entry is
	// keyed by that same synthetic token, not bigHashReq2's real one.
	syntheticPage2Req := bigHashBaseReq
	syntheticPage2Req.OpID = "be-read-bighash-2"
	syntheticPage2Req.Cursor = model.PageCursor{Mode: "after", Token: syntheticToken1}
	recordDataRead(rec, syntheticPage2Req, syntheticHashPage(100, 100, nil, nil, false), "server")

	// The frontend half's own Refresh click on the big-hash tab invalidates this path too, same as
	// the small hash's did above.
	rec.DataInvalidate(t, adapterhost.InvalidateRequestWire{ConnectionID: cfg.ID, Path: bigHashKeyNode.Path})

	// --- 8: list key — one page holds every seeded job, pager both-disabled ---------------------
	queueChildren := rec.TreeChildren(t, cfg.ID, queueNsNode.Path, false)
	listKeyNode := nodeByName(queueChildren.Nodes, testsupport.RedisListKey)
	if listKeyNode == nil {
		t.Fatalf("expected a %s key node in %+v", testsupport.RedisListKey, queueChildren.Nodes)
	}
	listReq := adapterhost.ReadRequestWire{
		OpID: "be-read-list", ConnectionID: cfg.ID, Path: listKeyNode.Path,
		PageSize: 100, Cursor: model.PageCursor{Mode: "offset", Offset: 0},
	}
	listResp := rec.DataRead(t, listReq, nil)
	listLogical, err := DecodePage(listResp.Page)
	if err != nil {
		t.Fatalf("decode list page: %v", err)
	}
	listKV, ok := listLogical.(LogicalKeyValuePage)
	if !ok || listKV.RedisType != "list" || len(listKV.Fields) != testsupport.RedisListLength || listKV.Position.HasMore {
		t.Fatalf("list page = %+v, want redisType=list fields=%d hasMore=false", listLogical, testsupport.RedisListLength)
	}

	// --- 9: TTL key — badges populated -----------------------------------------------------------
	sessionChildrenBefore, err := app.TreeSvc.Children(bridge.TreeChildrenArgs{ConnectionID: cfg.ID, Path: sessionNsNode.Path, Refresh: false})
	if err != nil {
		t.Fatalf("session children (before): %v", err)
	}
	if sessionChildrenBefore.Nodes == nil {
		sessionChildrenBefore.Nodes = []model.TreeNode{}
	}
	ttlKeyNode := nodeByName(sessionChildrenBefore.Nodes, testsupport.RedisTTLKey)
	if ttlKeyNode == nil {
		t.Fatalf("expected a %s key node in %+v", testsupport.RedisTTLKey, sessionChildrenBefore.Nodes)
	}
	ttlReq := adapterhost.ReadRequestWire{
		OpID: "be-read-ttl", ConnectionID: cfg.ID, Path: ttlKeyNode.Path,
		PageSize: 100, Cursor: model.PageCursor{Mode: "offset", Offset: 0},
	}
	ttlResp, err := app.Dispatcher.Read(context.Background(), WireRead(ttlReq))
	if err != nil {
		t.Fatalf("read ttl key: %v", err)
	}
	ttlLogical, err := DecodePage(ttlResp.Page)
	if err != nil {
		t.Fatalf("decode ttl page: %v", err)
	}
	ttlKV, ok := ttlLogical.(LogicalKeyValuePage)
	if !ok || ttlKV.RedisType != "string" {
		t.Fatalf("expected a string keyvalue page, got %+v", ttlLogical)
	}
	if ttlKV.TTLMs == nil || *ttlKV.TTLMs <= 0 {
		t.Fatalf("ttlMs = %v, want a positive value", ttlKV.TTLMs)
	}
	if ttlKV.MemoryBytes == nil || *ttlKV.MemoryBytes <= 0 {
		t.Fatalf("memoryBytes = %v, want a positive value", ttlKV.MemoryBytes)
	}
	// Frozen to fixed placeholders once the real values are validated above: ttlMs is
	// wall-clock-derived and memoryBytes is Redis's own internal object-encoding estimate, neither
	// of which the frontend half needs to be real — only "populated".
	ttlKV.TTLMs = int64p(9_999_000)
	ttlKV.MemoryBytes = int64p(64)
	recordDataRead(rec, ttlReq, ttlKV, ttlResp.Source)

	// --- 10: delete the TTL key -> the session level's own second listing omits it --------------
	ttlKeyValue := testsupport.RedisTTLKey
	mutateReq := adapterhost.MutateRequestWire{
		OpID: "be-delete-ttl", ConnectionID: cfg.ID, Path: ttlKeyNode.Path,
		Ops: []model.MutationRowOp{{Kind: "delete", Key: model.RowValues{{Name: "_key", Value: &ttlKeyValue}}}},
	}
	mutateResp := rec.DataMutate(t, mutateReq)
	if mutateResp.AffectedRows != 1 {
		t.Fatalf("delete ttl key affectedRows = %d, want 1", mutateResp.AffectedRows)
	}

	// browseInvalidate()'s cross-tab effect reloads with refresh:true (a hard bypass of the L1
	// cache), so the post-delete listing is a genuinely different (channel, args) pair from the
	// pre-delete one — each gets its own snapshot, captured here (out of dispatch order: the
	// "before" listing was fetched earlier, to find ttlKeyNode) to match the committed fixture's
	// own capture order exactly.
	sessionChildrenAfter, err := app.TreeSvc.Children(bridge.TreeChildrenArgs{ConnectionID: cfg.ID, Path: sessionNsNode.Path, Refresh: true})
	if err != nil {
		t.Fatalf("session children (after): %v", err)
	}
	if sessionChildrenAfter.Nodes == nil {
		sessionChildrenAfter.Nodes = []model.TreeNode{}
	}
	if nodeByName(sessionChildrenAfter.Nodes, testsupport.RedisTTLKey) != nil {
		t.Fatalf("expected %s to be gone after delete, got %+v", testsupport.RedisTTLKey, sessionChildrenAfter.Nodes)
	}
	rec.recordControl(channelTreeChildren, bridge.TreeChildrenArgs{ConnectionID: cfg.ID, Path: sessionNsNode.Path, Refresh: false}, sessionChildrenBefore)
	rec.recordControl(channelTreeChildren, bridge.TreeChildrenArgs{ConnectionID: cfg.ID, Path: sessionNsNode.Path, Refresh: true}, sessionChildrenAfter)

	// --- 11: console — DBSIZE against db0 -> one kv row ------------------------------------------
	executeReq := adapterhost.ExecuteRequestWire{
		OpID: "be-console-dbsize", ConnectionID: cfg.ID, Path: db0Node.Path, Statements: []string{"DBSIZE"},
	}
	executeResp := rec.DataExecute(t, executeReq)
	if len(executeResp.Pages) != 1 {
		t.Fatalf("execute pages = %d, want 1", len(executeResp.Pages))
	}

	if maybeWriteFixture(t, rec, "redis") {
		return
	}
	assertMatchesCommittedJSONFixture(t, rec, "testdata/redis.fixture.json")
}
