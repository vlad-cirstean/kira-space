package redis

import (
	"context"
	"testing"

	goredis "github.com/redis/go-redis/v9"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
)

// fakeScanner ports catalog-listing.spec.ts's fakeRedis (P58f D14): a scripted round per call,
// driving the truncation conjunction without a live server.
type fakeScanner struct {
	round func(call int) (keys []string, nextCursor uint64)
	calls []struct {
		cursor uint64
		match  string
		count  int64
	}
}

func (f *fakeScanner) Scan(_ context.Context, cursor uint64, match string, count int64) *goredis.ScanCmd {
	f.calls = append(f.calls, struct {
		cursor uint64
		match  string
		count  int64
	}{cursor, match, count})
	keys, nextCursor := f.round(len(f.calls) - 1)
	return goredis.NewScanCmdResult(keys, nextCursor, nil)
}

func TestListNamespaceChildren_SplitsOnFirstColonAfterPrefix(t *testing.T) {
	fake := &fakeScanner{round: func(int) ([]string, uint64) {
		return []string{"zebra:1", "apple:1", "counter", "banana"}, 0
	}}
	result, err := listNamespaceChildren(context.Background(), fake, "db0", nil, adapters.NewOpCtx("op1"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := make([][2]string, len(result.Nodes))
	for i, n := range result.Nodes {
		got[i] = [2]string{n.Kind, n.Name}
	}
	want := [][2]string{{"namespace", "apple"}, {"namespace", "zebra"}, {"key", "banana"}, {"key", "counter"}}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestListNamespaceChildren_NestedLevelJoinsPrefixAndDedups(t *testing.T) {
	fake := &fakeScanner{round: func(int) ([]string, uint64) {
		return []string{"a:b:x", "a:b:y", "a:c"}, 0
	}}
	result, err := listNamespaceChildren(context.Background(), fake, "db0", []string{"a"}, adapters.NewOpCtx("op1"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fake.calls) != 1 || fake.calls[0].match != "a:*" || fake.calls[0].count != scanCount {
		t.Fatalf("unexpected scan args: %+v", fake.calls)
	}
	got := make([][2]string, len(result.Nodes))
	for i, n := range result.Nodes {
		got[i] = [2]string{n.Kind, n.Name}
	}
	// deduped: "b" seen twice (a:b:x, a:b:y), kept once. A key node's own name is the full key, not
	// the local segment.
	want := [][2]string{{"namespace", "b"}, {"key", "a:c"}}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestListNamespaceChildren_UnresolvedCursorRunsMaxRoundsAndTruncates(t *testing.T) {
	fake := &fakeScanner{round: func(call int) ([]string, uint64) {
		return nil, uint64(call + 1)
	}}
	result, err := listNamespaceChildren(context.Background(), fake, "db0", nil, adapters.NewOpCtx("op1"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fake.calls) != maxScanRounds {
		t.Fatalf("got %d calls, want %d", len(fake.calls), maxScanRounds)
	}
	if result.Truncated == nil || !*result.Truncated {
		t.Fatalf("expected truncated=true, got %v", result.Truncated)
	}
}

func TestListNamespaceChildren_CompletesWithinCapReportsNoTruncation(t *testing.T) {
	fake := &fakeScanner{round: func(int) ([]string, uint64) {
		return []string{"onlykey"}, 0
	}}
	result, err := listNamespaceChildren(context.Background(), fake, "db0", nil, adapters.NewOpCtx("op1"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fake.calls) != 1 {
		t.Fatalf("got %d calls, want 1", len(fake.calls))
	}
	if result.Truncated != nil {
		t.Fatalf("expected no truncation flag, got %v", *result.Truncated)
	}
}

// fakePipeliner scripts TYPE's own reply per key without a live server — mirrors fakeScanner's
// own discipline (go-redis's NewStatusCmd + SetVal build a real, already-answered *StatusCmd;
// `Pipeliner` is embedded nil purely so this struct satisfies the huge interface, since keyTypes
// only ever calls Type/Exec on it — no other method is reachable from this package's own code).
type fakePipeliner struct {
	goredis.Pipeliner
	results map[string]string // key -> TYPE's reply ("none" for a key gone by the time TYPE runs)
	execErr error
}

func (f *fakePipeliner) Type(ctx context.Context, key string) *goredis.StatusCmd {
	cmd := goredis.NewStatusCmd(ctx)
	cmd.SetVal(f.results[key])
	return cmd
}

func (f *fakePipeliner) Exec(context.Context) ([]goredis.Cmder, error) {
	return nil, f.execErr
}

type fakeTyperConn struct {
	pipe *fakePipeliner
}

func (f *fakeTyperConn) Pipeline() goredis.Pipeliner { return f.pipe }

func TestKeyTypes_PreservesInputOrder(t *testing.T) {
	conn := &fakeTyperConn{pipe: &fakePipeliner{results: map[string]string{
		"a": "string", "b": "hash", "c": "list",
	}}}
	got, err := keyTypes(context.Background(), conn, []string{"c", "a", "b"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"list", "string", "hash"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestKeyTypes_DeletedKeyReportsNone(t *testing.T) {
	// TYPE never errors for a missing key — it replies "none" — the same reply a key deleted
	// between the SCAN that found it and this call would get.
	conn := &fakeTyperConn{pipe: &fakePipeliner{results: map[string]string{
		"gone": "none", "live": "string",
	}}}
	got, err := keyTypes(context.Background(), conn, []string{"gone", "live"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got[0] != "none" || got[1] != "string" {
		t.Fatalf("got %v, want [none string]", got)
	}
}

func TestKeyTypes_EmptyBatchIsANoop(t *testing.T) {
	conn := &fakeTyperConn{pipe: &fakePipeliner{results: map[string]string{}}}
	got, err := keyTypes(context.Background(), conn, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %v, want an empty slice", got)
	}
}

// TestGroupByDB_HonoursDBIndexAndOrder covers Adapter.KeyTypes' own routing discipline — every
// path's result must land back at its own original index regardless of which db index it
// resolved to, and each db index visited exactly once, in first-seen order (so a caller iterating
// `order` never issues two pipelines for the same db index).
func TestGroupByDB_HonoursDBIndexAndOrder(t *testing.T) {
	// Paths (by index): 0->db1, 1->db0, 2->db1, 3->db0, 4->db2.
	order, byDB := groupByDB([]int{1, 0, 1, 0, 2})

	wantOrder := []int{1, 0, 2}
	if len(order) != len(wantOrder) {
		t.Fatalf("got order %v, want %v", order, wantOrder)
	}
	for i := range wantOrder {
		if order[i] != wantOrder[i] {
			t.Fatalf("got order %v, want %v", order, wantOrder)
		}
	}

	wantByDB := map[int][]int{1: {0, 2}, 0: {1, 3}, 2: {4}}
	for db, want := range wantByDB {
		got := byDB[db]
		if len(got) != len(want) {
			t.Fatalf("db %d: got %v, want %v", db, got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("db %d: got %v, want %v", db, got, want)
			}
		}
	}
	if len(byDB) != len(wantByDB) {
		t.Fatalf("got %d db groups, want %d", len(byDB), len(wantByDB))
	}
}

func TestListNamespaceChildren_EscapesGlobMetacharactersInPrefix(t *testing.T) {
	fake := &fakeScanner{round: func(int) ([]string, uint64) {
		return []string{"a*b:x"}, 0
	}}
	_, err := listNamespaceChildren(context.Background(), fake, "db0", []string{"a*b"}, adapters.NewOpCtx("op1"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fake.calls) != 1 {
		t.Fatalf("expected 1 scan call, got %d", len(fake.calls))
	}
	want := `a\*b:*`
	if fake.calls[0].match != want {
		t.Fatalf("MATCH pattern = %q, want %q (literal '*' in the segment must be escaped)", fake.calls[0].match, want)
	}
}

func TestListNamespaceChildren_SkipsKeyThatDoesNotActuallyStartWithPrefix(t *testing.T) {
	// Defensive: even if a server ever returned a key that MATCH found but that doesn't literally
	// start with the requested prefix, slicing it must not panic — the key is skipped instead.
	fake := &fakeScanner{round: func(int) ([]string, uint64) {
		return []string{"other", "a:x"}, 0
	}}
	result, err := listNamespaceChildren(context.Background(), fake, "db0", []string{"a"}, adapters.NewOpCtx("op1"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Nodes) != 1 || result.Nodes[0].Name != "a:x" {
		t.Fatalf("got %v, want only the key actually under prefix \"a:\"", result.Nodes)
	}
}

func TestListNamespaceChildren_AlreadyCancelledContextFailsBeforeFirstScan(t *testing.T) {
	fake := &fakeScanner{round: func(int) ([]string, uint64) { return nil, 0 }}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := listNamespaceChildren(ctx, fake, "db0", nil, adapters.NewOpCtx("op1"))
	if code, ok := adapters.CodeOf(err); !ok || code != adapters.CodeCancelled {
		t.Fatalf("expected E_CANCELLED, got %v", err)
	}
	if len(fake.calls) != 0 {
		t.Fatalf("expected zero scan calls, got %d", len(fake.calls))
	}
}
