package tree_test

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/kirathecat/kira-studio/internal/kiratime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/repos"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/tree"
)

// fakeStates is a Connected whose state a test sets directly, instead of driving a real connect.
// since lets a test place a connect epoch on either side of a cached write — zero for a
// connection a test never sets it on, which is the correct default (a floor of the Unix epoch,
// leaving every real payload fresh) for every case in this file that never touches it.
type fakeStates struct {
	status map[string]string
	since  map[string]int64
}

func (f *fakeStates) StateOf(connectionID string) model.ConnectionState {
	return model.ConnectionState{
		ConnectionID: connectionID,
		Status:       f.status[connectionID],
		Since:        f.since[connectionID],
	}
}

// fakeBackend replaces the real adapterhost.Router (via a real Node engine fixture) these tests
// used — every kind has been Go-native, so there is no more Node-served path left to exercise
// here, only tree.Backend's own contract. Children answers one leaf node, truncated whenever the
// requested path's last segment name starts with "trunc-" — the same fixture convention these
// tests were written against.
type fakeBackend struct {
	childrenN      atomic.Int64
	schemaColumnsN atomic.Int64
	keyTypesN      atomic.Int64
	// childrenErr, when set, makes the next Children call fail instead of answering — used by
	// TestStalePayloadIsNotDropped to prove a failed re-fetch never destroys the stale cache row.
	childrenErr error
	// childrenMidFlight, when set, runs right before Children returns its result — F7's own hook
	// for simulating a reconnect that lands while this fetch's own backend call is still in
	// flight (moving the connection's epoch forward between when Children captured it and when
	// this call actually returns).
	childrenMidFlight func()
}

func (b *fakeBackend) Children(ctx context.Context, connectionID string, path model.NodePath) (adapters.TreeChildren, error) {
	b.childrenN.Add(1)
	if b.childrenErr != nil {
		return adapters.TreeChildren{}, b.childrenErr
	}
	nodes := []model.TreeNode{{Kind: "table", Name: "x", Path: "table:x", HasChildren: false}}
	last := path.Segments[len(path.Segments)-1]
	if b.childrenMidFlight != nil {
		b.childrenMidFlight()
	}
	if strings.HasPrefix(last.Name, "trunc-") {
		t := true
		return adapters.TreeChildren{Nodes: nodes, Truncated: &t}, nil
	}
	return adapters.TreeChildren{Nodes: nodes}, nil
}

func (b *fakeBackend) Describe(ctx context.Context, connectionID string, path model.NodePath, tabID *string) (model.ObjectMeta, error) {
	return model.ObjectMeta{}, nil
}

func (b *fakeBackend) Definition(ctx context.Context, connectionID string, path model.NodePath, tabID *string) (model.ObjectDefinition, error) {
	return model.ObjectDefinition{}, nil
}

func (b *fakeBackend) SchemaColumns(ctx context.Context, connectionID string, path model.NodePath) ([]model.RelationColumns, error) {
	b.schemaColumnsN.Add(1)
	return []model.RelationColumns{
		{Name: "x", Kind: "table", Columns: []model.ColumnMeta{{Name: "id", Position: 1, DataType: "integer"}}},
	}, nil
}

// KeyTypes echoes each path's own last segment name as its "type" — enough for a caller to check
// order preservation without this fake needing to understand redis's real type vocabulary.
func (b *fakeBackend) KeyTypes(ctx context.Context, connectionID string, paths []model.NodePath) ([]string, error) {
	b.keyTypesN.Add(1)
	out := make([]string, len(paths))
	for i, p := range paths {
		out[i] = p.Segments[len(p.Segments)-1].Name
	}
	return out, nil
}

type harness struct {
	svc     *tree.Service
	repos   *repos.Repos
	backend *fakeBackend
	fake    *fakeStates
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	t.Setenv("KIRA_SPACE_HOME", t.TempDir())
	db, err := studiostorage.Open()
	if err != nil {
		t.Fatalf("storage.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	r, err := repos.New(db.DB)
	if err != nil {
		t.Fatalf("repos.New: %v", err)
	}
	t.Cleanup(func() { _ = r.Close() })

	fake := &fakeStates{status: map[string]string{}, since: map[string]int64{}}
	backend := &fakeBackend{}
	svc := tree.New(r.Connections, r.Metadata, backend, fake)
	return &harness{svc: svc, repos: r, backend: backend, fake: fake}
}

// seedConnection inserts a bare connection row so requireConnected's not-connected fallback has
// a real name to read.
func (h *harness) seedConnection(t *testing.T, id, name string) {
	t.Helper()
	now := kiratime.NowISO()
	if _, err := h.repos.Connections.DB.Exec(
		`INSERT INTO connections (id, name, kind, color, mode, read_only, created_at, updated_at, sort_order)
		 VALUES (?, ?, ?, 'blue', 'fields', 0, ?, ?, 0)`,
		id, name, "kafka", now, now,
	); err != nil {
		t.Fatalf("seed connection: %v", err)
	}
	h.fake.status[id] = "connected"
}

// advanceSince marks id's connection as (re-)established right now — real wall-clock time, so
// the freshness floor for it becomes "now": anything already cached is stale, and anything the
// service writes from this point on is fresh. A 5ms sleep first guarantees a real, strictly-later
// timestamp string even though jsISOFormat's precision is only milliseconds — otherwise a payload
// written moments earlier in the same test could share the exact same millisecond and compare
// equal (fresh, not stale), making the staleness assertion that follows flaky.
func (h *harness) advanceSince(t *testing.T, id string) {
	t.Helper()
	time.Sleep(5 * time.Millisecond)
	h.fake.since[id] = time.Now().UnixMilli()
}

func requestCount(t *testing.T, h *harness, op string) int {
	t.Helper()
	switch op {
	case "adapter:children":
		return int(h.backend.childrenN.Load())
	case "adapter:schemaColumns":
		return int(h.backend.schemaColumnsN.Load())
	default:
		t.Fatalf("requestCount: unsupported op %q", op)
		return 0
	}
}

// TestSchemaMismatchDropsRow covers the validate-before-serve half of the cache-aside path: a
// cached row that no longer parses against the current domain shape must be treated as a miss AND
// actively dropped, not merely skipped — otherwise it is re-validated and re-rejected forever.
func TestSchemaMismatchDropsRow(t *testing.T) {
	h := newHarness(t)
	h.seedConnection(t, "c1", "Conn One")
	path := model.EncodePath([]model.PathSegment{{Kind: "database", Name: "app"}})

	if err := h.repos.Metadata.Put("c1", path, "children", json.RawMessage(`[{"kind":"nonsense"}]`)); err != nil {
		t.Fatalf("seed bad row: %v", err)
	}

	result, err := h.svc.Children(context.Background(), "c1", path, false)
	if err != nil {
		t.Fatalf("Children: %v", err)
	}
	if result.Source != "server" {
		t.Errorf("Source = %q, want server (the bad cache row must be treated as a miss)", result.Source)
	}
	if got, _, _ := h.repos.Metadata.Get("c1", path, "children"); string(got) == `[{"kind":"nonsense"}]` {
		t.Errorf("bad cache row survived: %s", got)
	}
}

// TestTruncatedRefreshDropsOlderCompleteRow covers the two truncation rules together: a truncated
// listing is never cached, and it must also DISPLACE any older complete row for the same path —
// without the drop, the next ordinary load would keep serving a stale complete listing forever.
func TestTruncatedRefreshDropsOlderCompleteRow(t *testing.T) {
	h := newHarness(t)
	h.seedConnection(t, "c1", "Conn One")
	// The fixture keys truncation off the last path segment's name, and the cache key is that
	// same encoded path string — so seeding a "complete" row under a trunc- path and then
	// refreshing it is exactly "a complete row that a later truncated answer must displace".
	path := model.EncodePath([]model.PathSegment{{Kind: "database", Name: "trunc-app"}})
	if err := h.repos.Metadata.Put("c1", path, "children",
		json.RawMessage(`[{"kind":"table","name":"x","path":"table:x","hasChildren":false}]`),
	); err != nil {
		t.Fatalf("seed complete row: %v", err)
	}

	if _, err := h.svc.Children(context.Background(), "c1", path, true); err != nil {
		t.Fatalf("Children (truncated refresh): %v", err)
	}
	if got, _, _ := h.repos.Metadata.Get("c1", path, "children"); got != nil {
		t.Errorf("older complete row survived a truncated refresh: %s", got)
	}

	// The next ordinary (non-refresh) load must go to the server, not serve the dropped row.
	before := requestCount(t, h, "adapter:children")
	if _, err := h.svc.Children(context.Background(), "c1", path, false); err != nil {
		t.Fatalf("Children (post-drop): %v", err)
	}
	if requestCount(t, h, "adapter:children") != before+1 {
		t.Errorf("post-drop load did not go to the server")
	}
}

// P22c §4.2: SchemaColumns gets the identical cache-aside coverage Children already has above,
// plus the two properties this design most depends on — a cache hit needs no live connection
// (F7), and it shares one metadata_cache row with "children" rather than adding a new one (F8).

func TestSchemaColumnsMissThenCacheHit(t *testing.T) {
	h := newHarness(t)
	h.seedConnection(t, "c1", "Conn One")
	path := model.EncodePath([]model.PathSegment{{Kind: "database", Name: "app"}, {Kind: "schema", Name: "s1"}})

	result, err := h.svc.SchemaColumns(context.Background(), "c1", path, false)
	if err != nil {
		t.Fatalf("SchemaColumns: %v", err)
	}
	if result.Source != "server" || len(result.Relations) != 1 {
		t.Fatalf("first call: got %+v, want one relation from the server", result)
	}
	if got := requestCount(t, h, "adapter:schemaColumns"); got != 1 {
		t.Fatalf("backend calls after first load = %d, want 1", got)
	}

	result, err = h.svc.SchemaColumns(context.Background(), "c1", path, false)
	if err != nil {
		t.Fatalf("SchemaColumns (cached): %v", err)
	}
	if result.Source != "cache" {
		t.Errorf("second call: Source = %q, want cache", result.Source)
	}
	if got := requestCount(t, h, "adapter:schemaColumns"); got != 1 {
		t.Errorf("backend calls after cached load = %d, want still 1 (no backend call)", got)
	}
}

func TestSchemaColumnsCacheHitNeedsNoConnection(t *testing.T) {
	h := newHarness(t)
	h.seedConnection(t, "c1", "Conn One")
	path := model.EncodePath([]model.PathSegment{{Kind: "database", Name: "app"}})

	if _, err := h.svc.SchemaColumns(context.Background(), "c1", path, false); err != nil {
		t.Fatalf("warm the cache: %v", err)
	}

	// F7's own property: a cache hit is served before the connection is even checked.
	h.fake.status["c1"] = "disconnected"
	result, err := h.svc.SchemaColumns(context.Background(), "c1", path, false)
	if err != nil {
		t.Fatalf("SchemaColumns while disconnected: %v", err)
	}
	if result.Source != "cache" {
		t.Errorf("Source = %q, want cache (readable with no live connection)", result.Source)
	}
}

func TestSchemaColumnsRefreshBypassesCache(t *testing.T) {
	h := newHarness(t)
	h.seedConnection(t, "c1", "Conn One")
	path := model.EncodePath([]model.PathSegment{{Kind: "database", Name: "app"}})

	if _, err := h.svc.SchemaColumns(context.Background(), "c1", path, false); err != nil {
		t.Fatalf("warm the cache: %v", err)
	}
	result, err := h.svc.SchemaColumns(context.Background(), "c1", path, true)
	if err != nil {
		t.Fatalf("SchemaColumns (refresh): %v", err)
	}
	if result.Source != "server" {
		t.Errorf("Source = %q, want server (refresh must bypass the cache)", result.Source)
	}
	if got := requestCount(t, h, "adapter:schemaColumns"); got != 2 {
		t.Errorf("backend calls after refresh = %d, want 2", got)
	}
}

func TestSchemaColumnsInvalidPayloadDroppedAndRefetched(t *testing.T) {
	h := newHarness(t)
	h.seedConnection(t, "c1", "Conn One")
	path := model.EncodePath([]model.PathSegment{{Kind: "database", Name: "app"}})

	if err := h.repos.Metadata.Put("c1", path, "columns", json.RawMessage(`[{"kind":"nonsense"}]`)); err != nil {
		t.Fatalf("seed bad row: %v", err)
	}

	result, err := h.svc.SchemaColumns(context.Background(), "c1", path, false)
	if err != nil {
		t.Fatalf("SchemaColumns: %v", err)
	}
	if result.Source != "server" {
		t.Errorf("Source = %q, want server (an invalid cached payload must be treated as a miss)", result.Source)
	}
	if got, _, _ := h.repos.Metadata.Get("c1", path, "columns"); string(got) == `[{"kind":"nonsense"}]` {
		t.Errorf("bad cache row survived: %s", got)
	}
}

func TestSchemaColumnsInvalidateClears(t *testing.T) {
	h := newHarness(t)
	h.seedConnection(t, "c1", "Conn One")
	path := model.EncodePath([]model.PathSegment{{Kind: "database", Name: "app"}})

	if _, err := h.svc.SchemaColumns(context.Background(), "c1", path, false); err != nil {
		t.Fatalf("warm the cache: %v", err)
	}
	if err := h.svc.Invalidate("c1", nil); err != nil {
		t.Fatalf("Invalidate: %v", err)
	}
	result, err := h.svc.SchemaColumns(context.Background(), "c1", path, false)
	if err != nil {
		t.Fatalf("SchemaColumns (post-invalidate): %v", err)
	}
	if result.Source != "server" {
		t.Errorf("Source = %q, want server (Invalidate must have cleared the cache)", result.Source)
	}
}

// TestChildrenAndSchemaColumnsShareOneRow proves the zero-net-row property: a container's
// "children" and "columns" payloads are two kinds merged into ONE metadata_cache row (the
// (connection_id, path) unique index), not two competing rows that would otherwise pressure each
// other out of the 200-row-per-connection budget.
func TestChildrenAndSchemaColumnsShareOneRow(t *testing.T) {
	h := newHarness(t)
	h.seedConnection(t, "c1", "Conn One")
	path := model.EncodePath([]model.PathSegment{{Kind: "database", Name: "app"}})

	if _, err := h.svc.Children(context.Background(), "c1", path, false); err != nil {
		t.Fatalf("Children: %v", err)
	}
	if _, err := h.svc.SchemaColumns(context.Background(), "c1", path, false); err != nil {
		t.Fatalf("SchemaColumns: %v", err)
	}

	var rowCount int
	if err := h.repos.Metadata.DB.QueryRow(
		`SELECT COUNT(*) FROM metadata_cache WHERE connection_id = ? AND path = ?`, "c1", path,
	).Scan(&rowCount); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	if rowCount != 1 {
		t.Fatalf("metadata_cache rows for (c1, %q) = %d, want 1 (children and columns must share one row)", path, rowCount)
	}

	// Both kinds must still read back correctly out of that one shared row.
	childrenResult, err := h.svc.Children(context.Background(), "c1", path, false)
	if err != nil {
		t.Fatalf("Children (after sharing the row): %v", err)
	}
	if childrenResult.Source != "cache" {
		t.Errorf("Children Source = %q, want cache", childrenResult.Source)
	}
	columnsResult, err := h.svc.SchemaColumns(context.Background(), "c1", path, false)
	if err != nil {
		t.Fatalf("SchemaColumns (after sharing the row): %v", err)
	}
	if columnsResult.Source != "cache" || len(columnsResult.Relations) != 1 {
		t.Errorf("SchemaColumns after sharing the row = %+v, want a cached single relation", columnsResult)
	}
}

// The staleness rules, and how they interact. seedConnection's default Since (0, the Unix epoch) is
// left untouched by every test above this line — that is a floor no real payload can ever be older
// than, which is exactly why every "cache hit" assertion already in this file keeps passing
// unmodified. The tests below are the ones that move Since.

// TestFreshCacheHitIsServedWhileConnected pins that navigating a live connection never re-fetches
// what it already has, regardless of the freshness rule — the payload is written after the
// connection's (zero, default) epoch, so it stays fresh for the rest of the session.
func TestFreshCacheHitIsServedWhileConnected(t *testing.T) {
	h := newHarness(t)
	h.seedConnection(t, "c1", "Conn One")
	path := model.EncodePath([]model.PathSegment{{Kind: "database", Name: "app"}})

	result, err := h.svc.Children(context.Background(), "c1", path, false)
	if err != nil {
		t.Fatalf("Children (miss): %v", err)
	}
	if result.Source != "server" {
		t.Fatalf("first call: Source = %q, want server", result.Source)
	}
	if got := requestCount(t, h, "adapter:children"); got != 1 {
		t.Fatalf("backend calls after first load = %d, want 1", got)
	}

	result, err = h.svc.Children(context.Background(), "c1", path, false)
	if err != nil {
		t.Fatalf("Children (cached): %v", err)
	}
	if result.Source != "cache" {
		t.Errorf("second call: Source = %q, want cache", result.Source)
	}
	if got := requestCount(t, h, "adapter:children"); got != 1 {
		t.Errorf("backend calls after cached load = %d, want still 1 (no backend call)", got)
	}
}

// TestPayloadOlderThanTheConnectionIsRefetched is P24's core rule: a payload written before the
// connection's current epoch is stale and must be re-fetched. Fails on main, which has no
// staleness concept at all (F2/F3) — a cache hit there wins at any age.
func TestPayloadOlderThanTheConnectionIsRefetched(t *testing.T) {
	h := newHarness(t)
	h.seedConnection(t, "c1", "Conn One")
	path := model.EncodePath([]model.PathSegment{{Kind: "database", Name: "app"}})

	if _, err := h.svc.Children(context.Background(), "c1", path, false); err != nil {
		t.Fatalf("seed payload: %v", err)
	}
	before := requestCount(t, h, "adapter:children")

	h.advanceSince(t, "c1") // the connection is (re-)established after the payload above was written

	result, err := h.svc.Children(context.Background(), "c1", path, false)
	if err != nil {
		t.Fatalf("Children: %v", err)
	}
	if result.Source != "server" {
		t.Errorf("Source = %q, want server (a payload older than the connection's epoch must be stale)", result.Source)
	}
	if got := requestCount(t, h, "adapter:children"); got != before+1 {
		t.Errorf("backend calls = %d, want %d", got, before+1)
	}
}

// TestRefetchRewritesFreshnessAndTheNextReadHits proves the re-fetch triggered by staleness
// happens once per path per connection session, not on every read: the payload it writes carries
// a real "now" fetchedAt, which is always at or after the epoch that made the old one stale.
func TestRefetchRewritesFreshnessAndTheNextReadHits(t *testing.T) {
	h := newHarness(t)
	h.seedConnection(t, "c1", "Conn One")
	path := model.EncodePath([]model.PathSegment{{Kind: "database", Name: "app"}})

	if _, err := h.svc.Children(context.Background(), "c1", path, false); err != nil {
		t.Fatalf("seed payload: %v", err)
	}
	h.advanceSince(t, "c1")
	before := requestCount(t, h, "adapter:children")

	result, err := h.svc.Children(context.Background(), "c1", path, false)
	if err != nil {
		t.Fatalf("Children (stale, re-fetches): %v", err)
	}
	if result.Source != "server" {
		t.Fatalf("first post-epoch call: Source = %q, want server", result.Source)
	}
	if got := requestCount(t, h, "adapter:children"); got != before+1 {
		t.Fatalf("backend calls after re-fetch = %d, want %d", got, before+1)
	}

	result, err = h.svc.Children(context.Background(), "c1", path, false)
	if err != nil {
		t.Fatalf("Children (should be fresh now): %v", err)
	}
	if result.Source != "cache" {
		t.Errorf("second post-epoch call: Source = %q, want cache", result.Source)
	}
	if got := requestCount(t, h, "adapter:children"); got != before+1 {
		t.Errorf("backend calls after second read = %d, want still %d (no second re-fetch)", got, before+1)
	}
}

// TestStalePayloadIsStillServedWhileDisconnected is D5: no connection means no floor, so a payload
// staler than any live connection's epoch is still served — the property that makes the tree
// instant on launch and lets completion work over a cached container with nothing connected.
func TestStalePayloadIsStillServedWhileDisconnected(t *testing.T) {
	h := newHarness(t)
	h.seedConnection(t, "c1", "Conn One")
	path := model.EncodePath([]model.PathSegment{{Kind: "database", Name: "app"}})

	if _, err := h.svc.Children(context.Background(), "c1", path, false); err != nil {
		t.Fatalf("seed payload: %v", err)
	}
	h.advanceSince(t, "c1") // would make it stale if anything were still connected

	h.fake.status["c1"] = "disconnected"
	before := requestCount(t, h, "adapter:children")
	result, err := h.svc.Children(context.Background(), "c1", path, false)
	if err != nil {
		t.Fatalf("Children while disconnected: %v", err)
	}
	if result.Source != "cache" {
		t.Errorf("Source = %q, want cache (no floor applies while disconnected)", result.Source)
	}
	if got := requestCount(t, h, "adapter:children"); got != before {
		t.Errorf("backend was called while disconnected: %d new call(s)", got-before)
	}
}

// TestStalePayloadIsNotDropped: a stale payload is bypassed, never deleted (D2/G3's own
// discipline) — a failing re-fetch must leave the cache exactly as it was, or a transient backend
// error would permanently destroy metadata a later, successful retry could have served from.
func TestStalePayloadIsNotDropped(t *testing.T) {
	h := newHarness(t)
	h.seedConnection(t, "c1", "Conn One")
	path := model.EncodePath([]model.PathSegment{{Kind: "database", Name: "app"}})

	if _, err := h.svc.Children(context.Background(), "c1", path, false); err != nil {
		t.Fatalf("seed payload: %v", err)
	}
	h.advanceSince(t, "c1")

	h.backend.childrenErr = errors.New("boom")
	if _, err := h.svc.Children(context.Background(), "c1", path, false); err == nil {
		t.Fatal("Children: want an error from the failing backend, got nil")
	}
	h.backend.childrenErr = nil

	got, _, err := h.repos.Metadata.Get("c1", path, "children")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got == nil {
		t.Error("stale payload was dropped after a failing re-fetch, want it to survive on disk")
	}
}

// TestRefreshingOneKindDoesNotFreshenAnother is the single most important case in this file: it
// fails against a row-level fetched_at implementation (re-fetching 'children'
// would stamp the whole row, wrongly freshening 'columns' too) and passes against the per-kind
// fetchedAt map.
func TestRefreshingOneKindDoesNotFreshenAnother(t *testing.T) {
	h := newHarness(t)
	h.seedConnection(t, "c1", "Conn One")
	path := model.EncodePath([]model.PathSegment{{Kind: "database", Name: "app"}})

	if _, err := h.svc.Children(context.Background(), "c1", path, false); err != nil {
		t.Fatalf("seed children: %v", err)
	}
	if _, err := h.svc.SchemaColumns(context.Background(), "c1", path, false); err != nil {
		t.Fatalf("seed columns: %v", err)
	}
	h.advanceSince(t, "c1") // both kinds are now stale, sharing one row

	childrenBefore := requestCount(t, h, "adapter:children")
	columnsBefore := requestCount(t, h, "adapter:schemaColumns")

	result, err := h.svc.Children(context.Background(), "c1", path, false)
	if err != nil {
		t.Fatalf("Children: %v", err)
	}
	if result.Source != "server" {
		t.Fatalf("children Source = %q, want server", result.Source)
	}
	if got := requestCount(t, h, "adapter:children"); got != childrenBefore+1 {
		t.Fatalf("children backend calls = %d, want %d", got, childrenBefore+1)
	}

	// columns must STILL read as stale — a row-level fetched_at would have been stamped by the
	// children write above (same row) and wrongly made this look freshly fetched too.
	colResult, err := h.svc.SchemaColumns(context.Background(), "c1", path, false)
	if err != nil {
		t.Fatalf("SchemaColumns: %v", err)
	}
	if colResult.Source != "server" {
		t.Errorf("columns Source = %q, want server (a row-level fetchedAt would wrongly freshen this)", colResult.Source)
	}
	if got := requestCount(t, h, "adapter:schemaColumns"); got != columnsBefore+1 {
		t.Errorf("columns backend calls = %d, want %d", got, columnsBefore+1)
	}
}

// TestPreP24RowIsStaleWhenConnectedAndServedWhenNot is D2's upgrade path: a row written before
// this phase (no fetchedAt key at all) compares as "", less than any real epoch — stale the moment
// a connection is live, but still servable with none.
func TestPreP24RowIsStaleWhenConnectedAndServedWhenNot(t *testing.T) {
	h := newHarness(t)
	h.seedConnection(t, "c1", "Conn One")
	path := model.EncodePath([]model.PathSegment{{Kind: "database", Name: "app"}})

	legacyChildren := `[{"kind":"table","name":"x","path":"table:x","hasChildren":false}]`
	if _, err := h.repos.Metadata.DB.Exec(
		`INSERT INTO metadata_cache (connection_id, path, kind, payload_json, fetched_at, etag)
		 VALUES (?, ?, 'children', ?, ?, NULL)`,
		"c1", path, `{"children":`+legacyChildren+`}`, kiratime.NowISO(),
	); err != nil {
		t.Fatalf("seed legacy row: %v", err)
	}

	// Disconnected: served regardless of the missing fetchedAt (D5 — no floor at all).
	h.fake.status["c1"] = "disconnected"
	before := requestCount(t, h, "adapter:children")
	result, err := h.svc.Children(context.Background(), "c1", path, false)
	if err != nil {
		t.Fatalf("Children while disconnected: %v", err)
	}
	if result.Source != "cache" {
		t.Errorf("Source = %q, want cache while disconnected", result.Source)
	}
	if got := requestCount(t, h, "adapter:children"); got != before {
		t.Errorf("backend was called while disconnected: %d new call(s)", got-before)
	}

	// Reconnected: the same still-legacy row (the disconnected read above wrote nothing) is stale
	// against any real epoch.
	h.fake.status["c1"] = "connected"
	h.fake.since["c1"] = time.Now().UnixMilli()
	result, err = h.svc.Children(context.Background(), "c1", path, false)
	if err != nil {
		t.Fatalf("Children while connected: %v", err)
	}
	if result.Source != "server" {
		t.Errorf("Source = %q, want server (a pre-P24 row must be stale once connected)", result.Source)
	}
	if got := requestCount(t, h, "adapter:children"); got != before+1 {
		t.Errorf("backend calls = %d, want %d", got, before+1)
	}
}

// TestInvalidateDropsEveryKindOnTheRow is F8's gap from the Go side: Invalidate on one path drops
// all four kinds sharing that row, not just whichever kind a caller happened to touch.
func TestInvalidateDropsEveryKindOnTheRow(t *testing.T) {
	h := newHarness(t)
	h.seedConnection(t, "c1", "Conn One")
	path := model.EncodePath([]model.PathSegment{{Kind: "database", Name: "app"}})

	if _, err := h.svc.Children(context.Background(), "c1", path, false); err != nil {
		t.Fatalf("seed children: %v", err)
	}
	if _, err := h.svc.Describe(context.Background(), "c1", path, false, nil); err != nil {
		t.Fatalf("seed describe: %v", err)
	}
	if _, err := h.svc.Definition(context.Background(), "c1", path, false, nil); err != nil {
		t.Fatalf("seed definition: %v", err)
	}
	if _, err := h.svc.SchemaColumns(context.Background(), "c1", path, false); err != nil {
		t.Fatalf("seed columns: %v", err)
	}

	if err := h.svc.Invalidate("c1", &path); err != nil {
		t.Fatalf("Invalidate: %v", err)
	}

	childrenResult, err := h.svc.Children(context.Background(), "c1", path, false)
	if err != nil {
		t.Fatalf("Children (post-invalidate): %v", err)
	}
	if childrenResult.Source != "server" {
		t.Errorf("children Source = %q, want server after Invalidate", childrenResult.Source)
	}
	describeResult, err := h.svc.Describe(context.Background(), "c1", path, false, nil)
	if err != nil {
		t.Fatalf("Describe (post-invalidate): %v", err)
	}
	if describeResult.Source != "server" {
		t.Errorf("describe Source = %q, want server after Invalidate", describeResult.Source)
	}
	definitionResult, err := h.svc.Definition(context.Background(), "c1", path, false, nil)
	if err != nil {
		t.Fatalf("Definition (post-invalidate): %v", err)
	}
	if definitionResult.Source != "server" {
		t.Errorf("definition Source = %q, want server after Invalidate", definitionResult.Source)
	}
	columnsResult, err := h.svc.SchemaColumns(context.Background(), "c1", path, false)
	if err != nil {
		t.Fatalf("SchemaColumns (post-invalidate): %v", err)
	}
	if columnsResult.Source != "server" {
		t.Errorf("columns Source = %q, want server after Invalidate", columnsResult.Source)
	}
}

// TestInvalidateConnectionDropsEveryPath is F9's gap: Invalidate with path == nil drops every row
// for that connection, and leaves every other connection's rows alone.
func TestInvalidateConnectionDropsEveryPath(t *testing.T) {
	h := newHarness(t)
	h.seedConnection(t, "c1", "Conn One")
	h.seedConnection(t, "c2", "Conn Two")
	path1 := model.EncodePath([]model.PathSegment{{Kind: "database", Name: "app"}})
	path2 := model.EncodePath([]model.PathSegment{{Kind: "database", Name: "other"}})

	if _, err := h.svc.Children(context.Background(), "c1", path1, false); err != nil {
		t.Fatalf("seed c1/path1: %v", err)
	}
	if _, err := h.svc.Children(context.Background(), "c1", path2, false); err != nil {
		t.Fatalf("seed c1/path2: %v", err)
	}
	if _, err := h.svc.Children(context.Background(), "c2", path1, false); err != nil {
		t.Fatalf("seed c2/path1: %v", err)
	}

	if err := h.svc.Invalidate("c1", nil); err != nil {
		t.Fatalf("Invalidate: %v", err)
	}

	r1, err := h.svc.Children(context.Background(), "c1", path1, false)
	if err != nil {
		t.Fatalf("Children c1/path1 (post-invalidate): %v", err)
	}
	if r1.Source != "server" {
		t.Errorf("c1/path1 Source = %q, want server after connection-wide Invalidate", r1.Source)
	}
	r2, err := h.svc.Children(context.Background(), "c1", path2, false)
	if err != nil {
		t.Fatalf("Children c1/path2 (post-invalidate): %v", err)
	}
	if r2.Source != "server" {
		t.Errorf("c1/path2 Source = %q, want server after connection-wide Invalidate", r2.Source)
	}

	before := requestCount(t, h, "adapter:children")
	r3, err := h.svc.Children(context.Background(), "c2", path1, false)
	if err != nil {
		t.Fatalf("Children c2/path1 (should be untouched): %v", err)
	}
	if r3.Source != "cache" {
		t.Errorf("c2/path1 Source = %q, want cache — c1's Invalidate must not touch c2", r3.Source)
	}
	if got := requestCount(t, h, "adapter:children"); got != before {
		t.Errorf("c2's cache was invalidated by c1's connection-wide Invalidate")
	}
}

// TestFetchRacingAReconnectIsNotCachedAsFresh: a fetch that captured the
// connection's epoch before calling the backend must not have its own result cached as fresh once
// a reconnect moves that epoch forward before the fetch returns — the result would otherwise get
// its metadata_cache row's fetchedAt stamped at Put time (after the new epoch), which reads as
// fresh against the connection's new state under freshnessFloor's own comparison, wrongly serving
// a listing that belongs to the connection's old target.
func TestFetchRacingAReconnectIsNotCachedAsFresh(t *testing.T) {
	h := newHarness(t)
	h.seedConnection(t, "c1", "Conn One")
	path := model.EncodePath([]model.PathSegment{{Kind: "database", Name: "app"}})

	// Simulates a reconnect landing while this fetch's own backend call is still in flight — the
	// epoch changes after Children captured it but before Children returns.
	h.backend.childrenMidFlight = func() { h.advanceSince(t, "c1") }
	result, err := h.svc.Children(context.Background(), "c1", path, false)
	if err != nil {
		t.Fatalf("Children (racing a reconnect): %v", err)
	}
	if result.Source != "server" {
		t.Fatalf("Source = %q, want server", result.Source)
	}
	h.backend.childrenMidFlight = nil

	if got, _, _ := h.repos.Metadata.Get("c1", path, "children"); got != nil {
		t.Errorf("a fetch racing a reconnect must not have cached its result, got a row: %s", got)
	}

	// The next read (now against the connection's already-current, post-reconnect epoch) must
	// still go to the server — a cache hit here would mean the race's own stale result got served
	// back as fresh.
	before := requestCount(t, h, "adapter:children")
	if _, err := h.svc.Children(context.Background(), "c1", path, false); err != nil {
		t.Fatalf("Children (post-race): %v", err)
	}
	if got := requestCount(t, h, "adapter:children"); got != before+1 {
		t.Errorf("backend calls after the race = %d, want %d (the raced fetch's result must not have been served as a cache hit)", got, before+1)
	}
}

// TestExplicitRefreshBeatsAFreshPayload: behaviour 3 must not be defeated by the new freshness
// rule — a payload written after the connection's epoch (fresh by D1) still re-fetches when the
// caller explicitly asks for refresh:true.
func TestExplicitRefreshBeatsAFreshPayload(t *testing.T) {
	h := newHarness(t)
	h.seedConnection(t, "c1", "Conn One")
	path := model.EncodePath([]model.PathSegment{{Kind: "database", Name: "app"}})

	if _, err := h.svc.Children(context.Background(), "c1", path, false); err != nil {
		t.Fatalf("seed payload: %v", err)
	}
	before := requestCount(t, h, "adapter:children")

	result, err := h.svc.Children(context.Background(), "c1", path, true)
	if err != nil {
		t.Fatalf("Children (refresh): %v", err)
	}
	if result.Source != "server" {
		t.Errorf("Source = %q, want server (refresh must bypass the cache even for a fresh payload)", result.Source)
	}
	if got := requestCount(t, h, "adapter:children"); got != before+1 {
		t.Errorf("backend calls = %d, want %d", got, before+1)
	}
}
