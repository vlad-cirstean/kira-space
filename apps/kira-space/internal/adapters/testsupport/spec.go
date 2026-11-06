package testsupport

import (
	"context"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/page"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// Seg builds one model.PathSegment — postgres_test.go's own seg(), generalised so mysqlfamily's,
// sqlite's and clickhouse's own acceptance specs do not each write it again.
func Seg(kind, name string) model.PathSegment { return model.PathSegment{Kind: kind, Name: name} }

// NodePath builds a model.NodePath for connectionID.
func NodePath(connectionID string, segments ...model.PathSegment) model.NodePath {
	return model.NodePath{ConnectionID: connectionID, Segments: segments}
}

// hasConnectionID is every per-adapter *Fixture type's own shared shape (P107 I2-28): a Config
// field whose ID names the connection.
type hasConnectionID interface {
	ConnectionID() string
}

// FixtureNodePath is NodePath for a fixture (redis/sqs/postgres/kafka/mongo/s3's own
// nodePath(fixture, segments...)) — each caller instantiates it once as
// `var nodePath = testsupport.FixtureNodePath[*testsupport.XFixture]`, so every existing call
// site (nodePath(fixture, seg(...))) is untouched.
func FixtureNodePath[F hasConnectionID](fixture F, segments ...model.PathSegment) model.NodePath {
	return NodePath(fixture.ConnectionID(), segments...)
}

// OffsetRead builds an offset-mode adapters.ReadRequest for path — sqs's and kafka's own
// offsetRead(path, pageSize) (P107 I2-28). s3's own offsetRead(path) fixes pageSize at 10 and
// stays its own thin wrapper over this (its own file, own reasoning).
func OffsetRead(path model.NodePath, pageSize int) adapters.ReadRequest {
	return adapters.ReadRequest{Path: path, PageSize: pageSize, Cursor: model.PageCursor{Mode: "offset", Offset: 0}}
}

// InsertDeleteRoundTrip is mysqlfamily's and sqlite's own "mutate: insert then delete
// round-trips" test body — insert two rows, count, delete one by key, count again,
// read the survivor back. path's own table must already exist and be empty; setup/cleanup (which
// differs per adapter — mysqlfamily provisions the scratch table over a side DSN connection,
// sqlite over the adapter's own Execute console) stays each caller's own.
func InsertDeleteRoundTrip(t *testing.T, a adapters.Adapter, path model.NodePath) {
	t.Helper()
	ctx := context.Background()

	insertPlan := model.MutationPlan{
		Path: path,
		Ops: []model.MutationRowOp{
			{Kind: "insert", Values: model.RowValues{{Name: "id", Value: Strp("1")}, {Name: "name", Value: Strp("first")}}},
			{Kind: "insert", Values: model.RowValues{{Name: "id", Value: Strp("2")}, {Name: "name", Value: Strp("second")}}},
		},
	}
	if _, err := a.Mutate(ctx, insertPlan, adapters.NewOpCtx("op-insdel-1")); err != nil {
		t.Fatalf("Mutate(insert): %v", err)
	}
	countAfterInsert, err := a.Count(ctx, adapters.CountRequest{Path: path}, adapters.NewOpCtx("op-insdel-2"))
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if countAfterInsert.Value != 2 {
		t.Fatalf("Count after insert = %d, want 2", countAfterInsert.Value)
	}

	deletePlan := model.MutationPlan{
		Path: path,
		Ops:  []model.MutationRowOp{{Kind: "delete", Key: model.RowValues{{Name: "id", Value: Strp("1")}}}},
	}
	result, err := a.Mutate(ctx, deletePlan, adapters.NewOpCtx("op-insdel-3"))
	if err != nil {
		t.Fatalf("Mutate(delete): %v", err)
	}
	if result.AffectedRows != 1 {
		t.Errorf("AffectedRows = %d, want 1", result.AffectedRows)
	}
	countAfterDelete, err := a.Count(ctx, adapters.CountRequest{Path: path}, adapters.NewOpCtx("op-insdel-4"))
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if countAfterDelete.Value != 1 {
		t.Fatalf("Count after delete = %d, want 1", countAfterDelete.Value)
	}

	read, err := a.Read(ctx, adapters.ReadRequest{
		Path: path, PageSize: 10, Cursor: model.PageCursor{Mode: "offset", Offset: 0},
	}, adapters.NewOpCtx("op-insdel-5"))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	readPage := read.(page.TabularPage)
	id := CellAt(t, readPage, 0, 0)
	if id == nil || *id != "2" {
		t.Errorf("surviving row id = %v, want 2", id)
	}
}

// ChildNames extracts every node's Name from a Children() result, in order.
func ChildNames(t *testing.T, children adapters.TreeChildren) []string {
	t.Helper()
	names := make([]string, len(children.Nodes))
	for i, n := range children.Nodes {
		names[i] = n.Name
	}
	return names
}

// ContainsName reports whether want is present in names.
func ContainsName(names []string, want string) bool {
	for _, n := range names {
		if n == want {
			return true
		}
	}
	return false
}

// CellAt reads one cell of a TabularPage as *string (nil for SQL NULL) — the Go analogue of
// packages/db-fixtures/support/page.ts's cellAt/isNull pair.
func CellAt(t *testing.T, p page.TabularPage, col, row int) *string {
	t.Helper()
	chunk := p.Chunks[col]
	if page.IsNull(chunk, row) {
		return nil
	}
	text := page.CellText(chunk, row)
	return &text
}

// chunkCellAt is CellAt's one-chunk shape, shared by the Document/KeyValue readers below — a
// page.Chunk's null-vs-empty-string distinction (P58a A4) matters here exactly as it does for a
// TabularPage's own cells, and getting it wrong in a per-package copy is the risk P58c §1.3 gap 3
// named.
func chunkCellAt(t *testing.T, chunk page.Chunk, row int) *string {
	t.Helper()
	if page.IsNull(chunk, row) {
		return nil
	}
	text := page.CellText(chunk, row)
	return &text
}

// DocIDAt reads row's pre-serialized EJSON _id text from a DocumentPage — the Go analogue of
// mongo.spec.ts's docIdAt.
func DocIDAt(t *testing.T, p page.DocumentPage, row int) *string {
	t.Helper()
	return chunkCellAt(t, p.IDs, row)
}

// DocBodyAt reads row's pre-serialized EJSON document body text from a DocumentPage — the Go
// analogue of mongo.spec.ts's docBodyAt.
func DocBodyAt(t *testing.T, p page.DocumentPage, row int) *string {
	t.Helper()
	return chunkCellAt(t, p.Bodies, row)
}

// KVPairs reads every field/value pair off a KeyValuePage into a map — the Go analogue of
// redis.spec.ts's kvPairs. A map, not a slice, because most callers (hash/set/zset reads) need to
// assert the exact *set* of pairs a page carries, order-independent — HSCAN's own round order is
// not stable across containers, so an ordered comparison would be flaky by
// construction. A row whose value is SQL/redis NULL has no representation here (redis has no NULL
// value concept at the KeyValuePage level); every field name in a real page is expected non-null.
func KVPairs(t *testing.T, p page.KeyValuePage) map[string]string {
	t.Helper()
	out := make(map[string]string, p.RowCount)
	for row := 0; row < p.RowCount; row++ {
		field := chunkCellAt(t, p.Fields, row)
		value := chunkCellAt(t, p.Values, row)
		if field == nil {
			t.Fatalf("KVPairs: field at row %d is null, want a real field name", row)
		}
		if value == nil {
			out[*field] = ""
			continue
		}
		out[*field] = *value
	}
	return out
}

// KVValueAt reads row's value text from a KeyValuePage directly, preserving row order — for the
// ordered cases KVPairs' map would lose (a list's absolute-index fields, a stream's per-entry
// rows), where the caller needs a specific row's value rather than the whole page as a set.
func KVValueAt(t *testing.T, p page.KeyValuePage, row int) *string {
	t.Helper()
	return chunkCellAt(t, p.Values, row)
}

// KVFieldAt reads row's field text from a KeyValuePage directly, preserving row order — the
// counterpart to KVValueAt for callers checking the "field" column itself, such as a redis set's
// synthetic per-row display index (its only field value, since a set member has no natural key).
func KVFieldAt(t *testing.T, p page.KeyValuePage, row int) *string {
	t.Helper()
	return chunkCellAt(t, p.Fields, row)
}

// Strp returns a pointer to s — every acceptance spec in this repo needs one somewhere and
// otherwise reinvents it under a different name.
func Strp(s string) *string { return &s }

// StreamKeyAt reads row's key text from a StreamPage — the Go analogue of sqs.spec.ts's own
// per-message key reads. SQS's MessageId is always present on a received message, but the reader
// stays nil-able (chunkCellAt, not a bare string) for the same reason DocIDAt does: a page.Chunk's
// null-vs-empty-string distinction is a property of the chunk format, not of any one producer's
// data, and a per-package copy that assumed "always present" would be the exact risk already
// named once for Document/KeyValue readers.
func StreamKeyAt(t *testing.T, p page.StreamPage, row int) *string {
	t.Helper()
	return chunkCellAt(t, p.Keys, row)
}

// StreamHeadersAt reads row's pre-serialized headers cell text from a StreamPage — P58d D8's
// hand-encoded JSON, asserted as a literal string by the tests that need the exact shape.
func StreamHeadersAt(t *testing.T, p page.StreamPage, row int) *string {
	t.Helper()
	return chunkCellAt(t, p.Headers, row)
}

// StreamAttrsAt reads row's pre-serialized system-attributes cell text from a StreamPage.
func StreamAttrsAt(t *testing.T, p page.StreamPage, row int) *string {
	t.Helper()
	return chunkCellAt(t, p.Attrs, row)
}

// StreamTimestampAt reads row's ISO-8601 timestamp cell text from a StreamPage.
func StreamTimestampAt(t *testing.T, p page.StreamPage, row int) *string {
	t.Helper()
	return chunkCellAt(t, p.Timestamps, row)
}

// StreamBodyAt reads row's body cell text from a StreamPage.
func StreamBodyAt(t *testing.T, p page.StreamPage, row int) *string {
	t.Helper()
	return chunkCellAt(t, p.Bodies, row)
}

// KeysetForwardIDs pages a table forward by following NextToken from an offset-0 first page and
// returns column 0's text of every row, failing on a runaway loop (a token that never advances).
// A binary-key keyset regression (P168 Part 3 F4) loops forever or repeats rows.
func KeysetForwardIDs(t *testing.T, a adapters.Adapter, path model.NodePath, sort *model.SortSpec, pageSize, wantRows int) []string {
	t.Helper()
	cursor := model.PageCursor{Mode: "offset", Offset: 0}
	var ids []string
	for pages := 0; ; pages++ {
		if pages > wantRows {
			t.Fatalf("keyset paging did not terminate after %d pages (ids so far %v)", pages, ids)
		}
		p, err := a.Read(context.Background(), adapters.ReadRequest{Path: path, Sort: sort, PageSize: pageSize, Cursor: cursor}, adapters.NewOpCtx("op-keyset-forward"))
		if err != nil {
			t.Fatalf("Read(%v): %v", cursor, err)
		}
		tp := p.(page.TabularPage)
		for row := 0; row < tp.RowCount; row++ {
			ids = append(ids, *CellAt(t, tp, 0, row))
		}
		if !tp.Position.HasMore {
			return ids
		}
		if tp.Position.NextToken == nil {
			t.Fatal("HasMore without a NextToken: table is not keyset-eligible")
		}
		cursor = model.PageCursor{Mode: "after", Token: *tp.Position.NextToken}
	}
}
