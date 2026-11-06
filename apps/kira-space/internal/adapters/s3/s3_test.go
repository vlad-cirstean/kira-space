// Ported from packages/db-fixtures/s3.spec.ts, case by case where practical — the spec's own
// numbering is kept in each test's name so the two can be diffed. The cases that carry the most
// weight: the four download scenarios (25-28, the only automated coverage of the
// temp-file-then-rename contract), a new mid-stream cancellation case extending 26, preview()
// rendering byte-exact text (17), an insert refusing an existing key only on a real 404 (24),
// update preserving ContentType/Metadata (20), and count excluding the Body row for an over-limit
// object written relatively (19). Every mutating case runs against MUTABLE_BUCKET.
package s3_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	_ "github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters/s3"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters/testsupport"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/page"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
)

func TestMain(m *testing.M) {
	code := m.Run()
	testsupport.StopS3()
	os.Exit(code)
}

var deps = adapters.Deps{Log: func(level, message string) {}}

func newAdapter(t *testing.T) adapters.Adapter {
	t.Helper()
	a, err := adapters.CreateAdapter("s3", deps)
	if err != nil {
		t.Fatalf("CreateAdapter: %v", err)
	}
	return a
}

func connectedAdapter(t *testing.T, fixture *testsupport.S3Fixture) adapters.Adapter {
	t.Helper()
	a := newAdapter(t)
	if _, err := a.Connect(context.Background(), fixture.Config, adapters.NewOpCtx("connect")); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { _ = a.Disconnect(context.Background()) })
	return a
}

// nodePath is testsupport.FixtureNodePath instantiated for *S3Fixture (P107 I2-28).
var nodePath = testsupport.FixtureNodePath[*testsupport.S3Fixture]

func bucketPath(fixture *testsupport.S3Fixture, name string) model.NodePath {
	return nodePath(fixture, testsupport.Seg("bucket", name))
}

// objectPath mirrors s3.spec.ts's own objectPath: resolveObjectTarget only ever reads the first
// (bucket) and last (object) segment.
func objectPath(fixture *testsupport.S3Fixture, bucket, key string) model.NodePath {
	return nodePath(fixture, testsupport.Seg("bucket", bucket), testsupport.Seg("object", key))
}

// offsetRead is s3's own fixed-pageSize:10 shape (P107 I2-28) — sqs's and kafka's own
// offsetRead(path, pageSize) is testsupport.OffsetRead directly; s3's never varies pageSize, so it
// keeps its own 1-arg wrapper over it rather than passing a literal 10 at every call site.
func offsetRead(path model.NodePath) adapters.ReadRequest {
	return testsupport.OffsetRead(path, 10)
}

// fieldsOf is s3.spec.ts's fieldsOf, via testsupport.KVPairs — a KeyValuePage's field/value pairs
// as a map, order-independent (the same reader mongo's/redis's own acceptance suites use).
func fieldsOf(t *testing.T, p page.KeyValuePage) map[string]string {
	t.Helper()
	return testsupport.KVPairs(t, p)
}

// 1. connect / disconnect
func TestS3_ConnectDisconnect(t *testing.T) {
	fixture := testsupport.StartS3(t)
	a := newAdapter(t)

	info, err := a.Connect(context.Background(), fixture.Config, adapters.NewOpCtx("op-1"))
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if info.ServerVersion != "Amazon S3" {
		t.Errorf("ServerVersion = %q, want Amazon S3", info.ServerVersion)
	}
	if err := a.Disconnect(context.Background()); err != nil {
		t.Fatalf("Disconnect: %v", err)
	}
	if _, err := a.Children(context.Background(), nodePath(fixture), adapters.NewOpCtx("op-1b")); err == nil {
		t.Fatal("Children after disconnect: want an error")
	} else if code, _ := adapters.CodeOf(err); code != adapters.CodeConnect {
		t.Errorf("code = %v, want E_CONNECT", code)
	}
}

// 2. an unparseable URI is rejected at connect time.
// P25 §1.5c/§2.8 row 3: this is a pre-connect configuration failure, not an ordinary query-time
// condition — E_CONNECT, not E_QUERY.
func TestS3_Connect_UnparseableURI(t *testing.T) {
	fixture := testsupport.StartS3(t)
	badCfg := fixture.Config
	bad := "not a valid uri at all"
	badCfg.URI = &bad

	a := newAdapter(t)
	_, err := a.Connect(context.Background(), badCfg, adapters.NewOpCtx("op-2"))
	if err == nil {
		t.Fatal("want an error")
	}
	if code, _ := adapters.CodeOf(err); code != adapters.CodeConnect {
		t.Errorf("code = %v, want E_CONNECT", code)
	}
}

// A fields-mode connection naming a nonexistent shared-config profile must be
// E_AUTH, not E_QUERY — the regression test for awscfg.Resolve routing LoadDefaultConfig's error
// through MapError instead of stringifying it. Needs no container behaviour LocalStack cannot
// provide: the failure happens before any request reaches it.
func TestS3_Connect_ProfileNotFoundIsAuthError(t *testing.T) {
	testsupport.StartS3(t) // Docker gate only — the connect below never reaches the container

	a := newAdapter(t)
	cfg := model.ResolvedConnectionConfig{
		ID: "test-s3-profile-not-found", Kind: "s3", Mode: "fields",
		Database: testsupport.Strp("us-east-1"),
		Username: testsupport.Strp("kira-p25-nonexistent-profile"),
		Options:  map[string]any{},
	}
	_, err := a.Connect(context.Background(), cfg, adapters.NewOpCtx("op-profile"))
	if err == nil {
		t.Fatal("want an error for a nonexistent profile")
	}
	if code, _ := adapters.CodeOf(err); code != adapters.CodeAuth {
		t.Errorf("code = %v, want E_AUTH", code)
	}
}

// 3. cap honesty.
func TestS3_Caps(t *testing.T) {
	a := newAdapter(t)
	c := a.Caps()
	if c.Tabular {
		t.Error("Tabular = true, want false")
	}
	if !c.KeyValue || !c.KeyBrowser {
		t.Errorf("KeyValue/KeyBrowser = %v/%v, want true/true", c.KeyValue, c.KeyBrowser)
	}
	if c.Definition || c.SQL {
		t.Errorf("Definition/SQL = %v/%v, want false/false", c.Definition, c.SQL)
	}
	if !c.ExactCount {
		t.Error("ExactCount = false, want true")
	}
	if c.Pagination != adapters.PaginationToken {
		t.Errorf("Pagination = %v, want token", c.Pagination)
	}
	if !c.CanInsert || !c.CanUpdate || !c.CanDelete || !c.Writable {
		t.Errorf("CanInsert/CanUpdate/CanDelete/Writable = %v/%v/%v/%v, want all true", c.CanInsert, c.CanUpdate, c.CanDelete, c.Writable)
	}
	if !c.Cancel || !c.FileTransfer {
		t.Errorf("Cancel/FileTransfer = %v/%v, want true/true", c.Cancel, c.FileTransfer)
	}
	if c.SchemaColumns {
		t.Error("SchemaColumns = true, want false (an S3 object has no column-shaped schema)")
	}
}

// P22c §4.1: the non-SQL kinds report the capability false and the method is not implemented.
func TestS3_SchemaColumns_Unsupported(t *testing.T) {
	a := newAdapter(t)
	_, err := a.SchemaColumns(context.Background(), model.NodePath{}, adapters.NewOpCtx("op-schema-columns"))
	if code, _ := adapters.CodeOf(err); code != adapters.CodeUnsupported {
		t.Fatalf("got %v, want E_UNSUPPORTED", err)
	}
}

// 4. tree enumeration: root is a flat bucket list.
func TestS3_Children_RootIsFlatBucketList(t *testing.T) {
	fixture := testsupport.StartS3(t)
	a := connectedAdapter(t, fixture)

	children, err := a.Children(context.Background(), nodePath(fixture), adapters.NewOpCtx("op-4"))
	if err != nil {
		t.Fatalf("Children: %v", err)
	}
	for _, want := range []string{testsupport.S3EmptyBucket, testsupport.S3MainBucket, testsupport.S3MutableBucket} {
		if !testsupport.ContainsName(testsupport.ChildNames(t, children), want) {
			t.Errorf("children = %v, want to contain %q", testsupport.ChildNames(t, children), want)
		}
	}
	for _, n := range children.Nodes {
		if n.Kind != "bucket" || n.HasChildren {
			t.Errorf("node %+v: want kind=bucket, hasChildren=false", n)
		}
	}
}

// 5. tree enumeration: a bucket root lists objects and prefixes, delimiter-grouped.
func TestS3_Children_BucketRootDelimiterGrouped(t *testing.T) {
	fixture := testsupport.StartS3(t)
	a := connectedAdapter(t, fixture)

	result, err := a.Children(context.Background(), bucketPath(fixture, testsupport.S3MainBucket), adapters.NewOpCtx("op-5"))
	if err != nil {
		t.Fatalf("Children: %v", err)
	}
	if result.Truncated != nil {
		t.Errorf("Truncated = %v, want nil for an ordinary listing", *result.Truncated)
	}
	wantKindsAndNames := []struct{ Kind, Name string }{
		{"prefix", "reports"}, {"prefix", "sizes"}, {"object", testsupport.S3RootObjectKey},
	}
	if len(result.Nodes) != len(wantKindsAndNames) {
		t.Fatalf("Nodes = %+v, want %d entries", result.Nodes, len(wantKindsAndNames))
	}
	for i, want := range wantKindsAndNames {
		if result.Nodes[i].Kind != want.Kind || result.Nodes[i].Name != want.Name {
			t.Errorf("Nodes[%d] = %+v, want kind=%s name=%s", i, result.Nodes[i], want.Kind, want.Name)
		}
	}
}

// 6. tree enumeration: descending into a prefix mixes a sub-prefix and a sibling object.
func TestS3_Children_DescendIntoPrefix(t *testing.T) {
	fixture := testsupport.StartS3(t)
	a := connectedAdapter(t, fixture)

	result, err := a.Children(context.Background(), nodePath(fixture, testsupport.Seg("bucket", testsupport.S3MainBucket), testsupport.Seg("prefix", "reports")), adapters.NewOpCtx("op-6"))
	if err != nil {
		t.Fatalf("Children: %v", err)
	}
	want := []struct{ Kind, Name string }{{"prefix", "2024"}, {"object", "reports/notes.txt"}}
	if len(result.Nodes) != len(want) {
		t.Fatalf("Nodes = %+v, want %d entries", result.Nodes, len(want))
	}
	for i, w := range want {
		if result.Nodes[i].Kind != w.Kind || result.Nodes[i].Name != w.Name {
			t.Errorf("Nodes[%d] = %+v, want kind=%s name=%s", i, result.Nodes[i], w.Kind, w.Name)
		}
	}
}

// 7. children of a leaf (object) and of an empty bucket.
func TestS3_Children_LeafAndEmptyBucket(t *testing.T) {
	fixture := testsupport.StartS3(t)
	a := connectedAdapter(t, fixture)
	ctx := context.Background()

	leaf, err := a.Children(ctx, objectPath(fixture, testsupport.S3MainBucket, testsupport.S3RootObjectKey), adapters.NewOpCtx("op-7a"))
	if err != nil {
		t.Fatalf("Children (leaf): %v", err)
	}
	if len(leaf.Nodes) != 0 {
		t.Errorf("leaf.Nodes = %v, want empty", leaf.Nodes)
	}

	empty, err := a.Children(ctx, bucketPath(fixture, testsupport.S3EmptyBucket), adapters.NewOpCtx("op-7b"))
	if err != nil {
		t.Fatalf("Children (empty bucket): %v", err)
	}
	if len(empty.Nodes) != 0 {
		t.Errorf("empty.Nodes = %v, want empty", empty.Nodes)
	}
}

// 8. describe/definition are unsupported.
func TestS3_DescribeDefinitionUnsupported(t *testing.T) {
	fixture := testsupport.StartS3(t)
	a := connectedAdapter(t, fixture)
	ctx := context.Background()
	target := objectPath(fixture, testsupport.S3MainBucket, testsupport.S3RootObjectKey)

	if _, err := a.Describe(ctx, target, adapters.NewOpCtx("op-8a")); err == nil {
		t.Fatal("Describe: want an error")
	} else if code, _ := adapters.CodeOf(err); code != adapters.CodeUnsupported {
		t.Errorf("Describe code = %v, want E_UNSUPPORTED", code)
	}
	if _, err := a.Definition(ctx, target, adapters.NewOpCtx("op-8b")); err == nil {
		t.Fatal("Definition: want an error")
	} else if code, _ := adapters.CodeOf(err); code != adapters.CodeUnsupported {
		t.Errorf("Definition code = %v, want E_UNSUPPORTED", code)
	}
}

// 9. read: a root-level object comes back as a keyvalue page with metadata + body.
func TestS3_Read_RootObjectMetadataAndBody(t *testing.T) {
	fixture := testsupport.StartS3(t)
	a := connectedAdapter(t, fixture)

	p, err := a.Read(context.Background(), offsetRead(objectPath(fixture, testsupport.S3MainBucket, testsupport.S3RootObjectKey)), adapters.NewOpCtx("op-9"))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	kv := p.(page.KeyValuePage)
	if kv.RedisType != "object" || kv.Position.HasMore {
		t.Errorf("RedisType/HasMore = %q/%v, want object/false", kv.RedisType, kv.Position.HasMore)
	}
	fields := fieldsOf(t, kv)
	if fields["ContentType"] != "text/plain" {
		t.Errorf("ContentType = %q, want text/plain", fields["ContentType"])
	}
	if fields["Metadata.seeded"] != "true" {
		t.Errorf("Metadata.seeded = %q, want true", fields["Metadata.seeded"])
	}
	if fields["Body"] != testsupport.S3RootObjectBody {
		t.Errorf("Body = %q, want %q", fields["Body"], testsupport.S3RootObjectBody)
	}
}

// 10. read: a nested object under two prefix levels resolves to the right key.
func TestS3_Read_NestedObject(t *testing.T) {
	fixture := testsupport.StartS3(t)
	a := connectedAdapter(t, fixture)

	p, err := a.Read(context.Background(), offsetRead(objectPath(fixture, testsupport.S3MainBucket, testsupport.S3NestedObjectKey)), adapters.NewOpCtx("op-10"))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	fields := fieldsOf(t, p.(page.KeyValuePage))
	if fields["ContentType"] != "application/json" {
		t.Errorf("ContentType = %q, want application/json", fields["ContentType"])
	}
	if fields["Body"] != `{"year":2024,"total":42}` {
		t.Errorf("Body = %q", fields["Body"])
	}
}

// 11. read: a sibling object one level up from the nested one is distinct.
func TestS3_Read_SiblingObjectDistinct(t *testing.T) {
	fixture := testsupport.StartS3(t)
	a := connectedAdapter(t, fixture)

	p, err := a.Read(context.Background(), offsetRead(objectPath(fixture, testsupport.S3MainBucket, testsupport.S3SiblingPrefixObjectKey)), adapters.NewOpCtx("op-11"))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if fieldsOf(t, p.(page.KeyValuePage))["Body"] == `{"year":2024,"total":42}` {
		t.Error("sibling object body matches the nested object's — resolveObjectTarget mixed them up")
	}
}

// 12. read: a nonexistent object is E_QUERY, not E_NOT_FOUND.
func TestS3_Read_NonexistentObjectIsQueryError(t *testing.T) {
	fixture := testsupport.StartS3(t)
	a := connectedAdapter(t, fixture)

	_, err := a.Read(context.Background(), offsetRead(objectPath(fixture, testsupport.S3MainBucket, "this-key-was-never-put.txt")), adapters.NewOpCtx("op-12"))
	if err == nil {
		t.Fatal("want an error")
	}
	if code, _ := adapters.CodeOf(err); code != adapters.CodeQuery {
		t.Errorf("code = %v, want E_QUERY", code)
	}
}

// 13. count: exact field-row count.
func TestS3_Count_Exact(t *testing.T) {
	fixture := testsupport.StartS3(t)
	a := connectedAdapter(t, fixture)

	result, err := a.Count(context.Background(), adapters.CountRequest{Path: objectPath(fixture, testsupport.S3MainBucket, testsupport.S3RootObjectKey)}, adapters.NewOpCtx("op-13"))
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if !result.Exact || result.Value <= 0 {
		t.Errorf("Count = %+v, want exact=true value>0", result)
	}
}

// 14. execute stays unsupported.
func TestS3_ExecuteUnsupported(t *testing.T) {
	fixture := testsupport.StartS3(t)
	a := connectedAdapter(t, fixture)

	req := model.ConsoleRequest{Path: objectPath(fixture, testsupport.S3MainBucket, testsupport.S3RootObjectKey), Statements: []string{"x"}}
	if _, err := a.Execute(context.Background(), req, adapters.NewOpCtx("op-14")); err == nil {
		t.Fatal("want an error")
	} else if code, _ := adapters.CodeOf(err); code != adapters.CodeUnsupported {
		t.Errorf("code = %v, want E_UNSUPPORTED", code)
	}
}

// 15. cancel is a permanent no-op.
func TestS3_Cancel_PermanentNoOp(t *testing.T) {
	fixture := testsupport.StartS3(t)
	a := connectedAdapter(t, fixture)

	ok, err := a.Cancel(context.Background(), "some-op-id")
	if err != nil || ok {
		t.Errorf("Cancel = %v/%v, want false/nil", ok, err)
	}
}

// 16. read: an already-cancelled context rejects before running anything.
func TestS3_Read_AlreadyCancelledContext(t *testing.T) {
	fixture := testsupport.StartS3(t)
	a := connectedAdapter(t, fixture)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := a.Read(ctx, offsetRead(objectPath(fixture, testsupport.S3MainBucket, testsupport.S3RootObjectKey)), adapters.NewOpCtx("op-16"))
	if err == nil {
		t.Fatal("want an error")
	}
	if code, _ := adapters.CodeOf(err); code != adapters.CodeCancelled {
		t.Errorf("code = %v, want E_CANCELLED", code)
	}
}

// 17. preview() renders exact commands for update/insert/delete without executing.
func TestS3_Preview_RendersByteExactCommands(t *testing.T) {
	fixture := testsupport.StartS3(t)
	a := connectedAdapter(t, fixture)

	newValue := `{"status":"final"}`
	plan := model.MutationPlan{
		Path: bucketPath(fixture, testsupport.S3MutableBucket),
		Ops: []model.MutationRowOp{
			{Kind: "update", Key: model.RowValues{{Name: "_key", Value: testsupport.Strp(testsupport.S3EditableObjectKey)}}, Changes: model.RowValues{{Name: "$value", Value: &newValue}}},
			{Kind: "insert", Values: model.RowValues{{Name: "_key", Value: testsupport.Strp(testsupport.S3UploadTargetKey)}, {Name: "$file", Value: testsupport.Strp("/tmp/whatever.txt")}}},
			{Kind: "delete", Key: model.RowValues{{Name: "_key", Value: testsupport.Strp(testsupport.S3DeleteTargetKey)}}},
		},
	}
	statements, err := a.Preview(plan)
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	want := []string{
		"PutObject s3://" + testsupport.S3MutableBucket + "/" + testsupport.S3EditableObjectKey + " (18 B)",
		"PutObject s3://" + testsupport.S3MutableBucket + "/" + testsupport.S3UploadTargetKey + " <- /tmp/whatever.txt",
		"DeleteObject s3://" + testsupport.S3MutableBucket + "/" + testsupport.S3DeleteTargetKey,
	}
	for i, w := range want {
		if statements[i] != w {
			t.Errorf("statements[%d] = %q, want %q", i, statements[i], w)
		}
	}

	if _, err := a.Preview(model.MutationPlan{Path: nodePath(fixture)}); err == nil {
		t.Fatal("Preview on a non-bucket-rooted path: want an error")
	} else if code, _ := adapters.CodeOf(err); code != adapters.CodeNotFound {
		t.Errorf("code = %v, want E_NOT_FOUND", code)
	}

	// preview() never executes — a re-read shows the object exactly as seeded.
	p, err := a.Read(context.Background(), offsetRead(objectPath(fixture, testsupport.S3MutableBucket, testsupport.S3EditableObjectKey)), adapters.NewOpCtx("op-17b"))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if body := fieldsOf(t, p.(page.KeyValuePage))["Body"]; body != testsupport.S3EditableObjectBody {
		t.Errorf("Body = %q, want %q (unmutated)", body, testsupport.S3EditableObjectBody)
	}
}

// 18. read: an object over the preview limit has no Body row and reports its size.
func TestS3_Read_OversizedObjectNoBodyRow(t *testing.T) {
	fixture := testsupport.StartS3(t)
	a := connectedAdapter(t, fixture)

	p, err := a.Read(context.Background(), offsetRead(objectPath(fixture, testsupport.S3MainBucket, testsupport.S3OversizedObjectKey)), adapters.NewOpCtx("op-18"))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	kv := p.(page.KeyValuePage)
	fields := fieldsOf(t, kv)
	if _, ok := fields["Body"]; ok {
		t.Error("Body row present, want none for an over-limit object")
	}
	if kv.MemoryBytes == nil || *kv.MemoryBytes != int64(testsupport.S3OversizedObjectBytes) {
		t.Errorf("MemoryBytes = %v, want %d", kv.MemoryBytes, testsupport.S3OversizedObjectBytes)
	}
	if fields["ContentType"] != "text/plain" {
		t.Errorf("ContentType = %q, want text/plain", fields["ContentType"])
	}
}

// 19. count: the Body row is excluded for an over-limit object (written relatively — P58d D16).
func TestS3_Count_ExcludesBodyRowForOversized(t *testing.T) {
	fixture := testsupport.StartS3(t)
	a := connectedAdapter(t, fixture)
	ctx := context.Background()

	small, err := a.Count(ctx, adapters.CountRequest{Path: objectPath(fixture, testsupport.S3MainBucket, testsupport.S3SmallForCountKey)}, adapters.NewOpCtx("op-19a"))
	if err != nil {
		t.Fatalf("Count (small): %v", err)
	}
	oversized, err := a.Count(ctx, adapters.CountRequest{Path: objectPath(fixture, testsupport.S3MainBucket, testsupport.S3OversizedObjectKey)}, adapters.NewOpCtx("op-19b"))
	if err != nil {
		t.Fatalf("Count (oversized): %v", err)
	}
	if oversized.Value != small.Value-1 {
		t.Errorf("oversized.Value = %d, want small.Value-1 = %d", oversized.Value, small.Value-1)
	}
}

// 20. mutate update replaces the body and preserves ContentType and user Metadata.
func TestS3_Mutate_UpdatePreservesAttributes(t *testing.T) {
	fixture := testsupport.StartS3(t)
	a := connectedAdapter(t, fixture)
	ctx := context.Background()

	newBody := `{"status":"final"}`
	plan := model.MutationPlan{
		Path: bucketPath(fixture, testsupport.S3MutableBucket),
		Ops:  []model.MutationRowOp{{Kind: "update", Key: model.RowValues{{Name: "_key", Value: testsupport.Strp(testsupport.S3EditableObjectKey)}}, Changes: model.RowValues{{Name: "$value", Value: &newBody}}}},
	}
	result, err := a.Mutate(ctx, plan, adapters.NewOpCtx("op-20a"))
	if err != nil {
		t.Fatalf("Mutate: %v", err)
	}
	if result.AffectedRows != 1 {
		t.Errorf("AffectedRows = %d, want 1", result.AffectedRows)
	}

	p, err := a.Read(ctx, offsetRead(objectPath(fixture, testsupport.S3MutableBucket, testsupport.S3EditableObjectKey)), adapters.NewOpCtx("op-20b"))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	fields := fieldsOf(t, p.(page.KeyValuePage))
	if fields["Body"] != newBody {
		t.Errorf("Body = %q, want %q", fields["Body"], newBody)
	}
	if fields["ContentType"] != "application/json" {
		t.Errorf("ContentType = %q, want application/json", fields["ContentType"])
	}
	if fields["Metadata.seeded"] != "true" {
		t.Errorf("Metadata.seeded = %q, want true", fields["Metadata.seeded"])
	}
}

// 21. mutate update on a read-only connection is refused and writes nothing.
func TestS3_Mutate_ReadOnlyConnectionRefused(t *testing.T) {
	fixture := testsupport.StartS3(t)
	roCfg := fixture.Config
	roCfg.ReadOnly = true
	roAdapter := newAdapter(t)
	ctx := context.Background()
	if _, err := roAdapter.Connect(ctx, roCfg, adapters.NewOpCtx("connect-ro")); err != nil {
		t.Fatalf("Connect (read-only): %v", err)
	}
	defer roAdapter.Disconnect(ctx)

	attempt := "attempted overwrite"
	plan := model.MutationPlan{
		Path: bucketPath(fixture, testsupport.S3MutableBucket),
		Ops:  []model.MutationRowOp{{Kind: "update", Key: model.RowValues{{Name: "_key", Value: testsupport.Strp(testsupport.S3ReadonlyTargetKey)}}, Changes: model.RowValues{{Name: "$value", Value: &attempt}}}},
	}
	if _, err := roAdapter.Mutate(ctx, plan, adapters.NewOpCtx("op-21a")); err == nil {
		t.Fatal("want an error")
	} else if code, _ := adapters.CodeOf(err); code != adapters.CodeUnsupported {
		t.Errorf("code = %v, want E_UNSUPPORTED", code)
	}

	a := connectedAdapter(t, fixture)
	p, err := a.Read(ctx, offsetRead(objectPath(fixture, testsupport.S3MutableBucket, testsupport.S3ReadonlyTargetKey)), adapters.NewOpCtx("op-21b"))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if body := fieldsOf(t, p.(page.KeyValuePage))["Body"]; body != testsupport.S3ReadonlyTargetBody {
		t.Errorf("Body = %q, want unchanged %q", body, testsupport.S3ReadonlyTargetBody)
	}
}

// 22. mutate delete removes the object; deleting a missing key is E_QUERY.
func TestS3_Mutate_DeleteThenSecondDeleteIsQueryError(t *testing.T) {
	fixture := testsupport.StartS3(t)
	a := connectedAdapter(t, fixture)
	ctx := context.Background()

	deletePlan := model.MutationPlan{
		Path: bucketPath(fixture, testsupport.S3MutableBucket),
		Ops:  []model.MutationRowOp{{Kind: "delete", Key: model.RowValues{{Name: "_key", Value: testsupport.Strp(testsupport.S3DeleteTargetKey)}}}},
	}
	result, err := a.Mutate(ctx, deletePlan, adapters.NewOpCtx("op-22a"))
	if err != nil {
		t.Fatalf("Mutate delete: %v", err)
	}
	if result.AffectedRows != 1 {
		t.Errorf("AffectedRows = %d, want 1", result.AffectedRows)
	}

	if _, err := a.Read(ctx, offsetRead(objectPath(fixture, testsupport.S3MutableBucket, testsupport.S3DeleteTargetKey)), adapters.NewOpCtx("op-22b")); err == nil {
		t.Fatal("Read after delete: want an error")
	} else if code, _ := adapters.CodeOf(err); code != adapters.CodeQuery {
		t.Errorf("code = %v, want E_QUERY", code)
	}

	// A second delete of the same key is a query-time condition, not silent success.
	if _, err := a.Mutate(ctx, deletePlan, adapters.NewOpCtx("op-22c")); err == nil {
		t.Fatal("second delete: want an error")
	} else if code, _ := adapters.CodeOf(err); code != adapters.CodeQuery {
		t.Errorf("code = %v, want E_QUERY", code)
	}
}

// 23. mutate insert uploads a local file with its length and content type.
func TestS3_Mutate_InsertUploadsLocalFile(t *testing.T) {
	fixture := testsupport.StartS3(t)
	a := connectedAdapter(t, fixture)
	ctx := context.Background()

	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "upload.txt")
	content := "uploaded from a local temp file"
	if err := os.WriteFile(tmpFile, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	plan := model.MutationPlan{
		Path: bucketPath(fixture, testsupport.S3MutableBucket),
		Ops: []model.MutationRowOp{{Kind: "insert", Values: model.RowValues{
			{Name: "_key", Value: testsupport.Strp(testsupport.S3UploadTargetKey)},
			{Name: "$file", Value: &tmpFile},
			{Name: "$contentType", Value: testsupport.Strp("text/plain")},
		}}},
	}
	result, err := a.Mutate(ctx, plan, adapters.NewOpCtx("op-23a"))
	if err != nil {
		t.Fatalf("Mutate: %v", err)
	}
	if result.AffectedRows != 1 {
		t.Errorf("AffectedRows = %d, want 1", result.AffectedRows)
	}

	p, err := a.Read(ctx, offsetRead(objectPath(fixture, testsupport.S3MutableBucket, testsupport.S3UploadTargetKey)), adapters.NewOpCtx("op-23b"))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	fields := fieldsOf(t, p.(page.KeyValuePage))
	if fields["Body"] != content {
		t.Errorf("Body = %q, want %q", fields["Body"], content)
	}
	if fields["ContentType"] != "text/plain" {
		t.Errorf("ContentType = %q, want text/plain", fields["ContentType"])
	}
}

// 24. mutate insert refuses an existing key (on a real 404 check, P58d D14) and a missing source file.
func TestS3_Mutate_InsertRefusesExistingKeyAndMissingSource(t *testing.T) {
	fixture := testsupport.StartS3(t)
	a := connectedAdapter(t, fixture)
	ctx := context.Background()

	collidePlan := model.MutationPlan{
		Path: bucketPath(fixture, testsupport.S3MutableBucket),
		Ops:  []model.MutationRowOp{{Kind: "insert", Values: model.RowValues{{Name: "_key", Value: testsupport.Strp(testsupport.S3ReadonlyTargetKey)}, {Name: "$file", Value: testsupport.Strp("/does/not/matter/for/this/case.txt")}}}},
	}
	if _, err := a.Mutate(ctx, collidePlan, adapters.NewOpCtx("op-24a")); err == nil {
		t.Fatal("want an error (key collision)")
	} else if code, _ := adapters.CodeOf(err); code != adapters.CodeQuery {
		t.Errorf("code = %v, want E_QUERY", code)
	}

	neverCreatedKey := "never-created-because-source-missing.txt"
	missingSourcePlan := model.MutationPlan{
		Path: bucketPath(fixture, testsupport.S3MutableBucket),
		Ops:  []model.MutationRowOp{{Kind: "insert", Values: model.RowValues{{Name: "_key", Value: &neverCreatedKey}, {Name: "$file", Value: testsupport.Strp("/does/not/exist/at/all.txt")}}}},
	}
	if _, err := a.Mutate(ctx, missingSourcePlan, adapters.NewOpCtx("op-24b")); err == nil {
		t.Fatal("want an error (missing source file)")
	} else if code, _ := adapters.CodeOf(err); code != adapters.CodeQuery {
		t.Errorf("code = %v, want E_QUERY", code)
	}

	if _, err := a.Read(ctx, offsetRead(objectPath(fixture, testsupport.S3MutableBucket, neverCreatedKey)), adapters.NewOpCtx("op-24c")); err == nil {
		t.Fatal("the refused insert must not have created the key")
	} else if code, _ := adapters.CodeOf(err); code != adapters.CodeQuery {
		t.Errorf("code = %v, want E_QUERY", code)
	}
}

// 25. downloadObject writes the exact bytes and returns the count.
func TestS3_DownloadObject_ExactBytes(t *testing.T) {
	fixture := testsupport.StartS3(t)
	a := connectedAdapter(t, fixture)

	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "downloaded.json")
	result, err := a.DownloadObject(context.Background(), model.ObjectDownloadRequest{
		Path: objectPath(fixture, testsupport.S3MainBucket, testsupport.S3NestedObjectKey), DestPath: destPath,
	}, adapters.NewOpCtx("op-25"))
	if err != nil {
		t.Fatalf("DownloadObject: %v", err)
	}
	want := `{"year":2024,"total":42}`
	if result.Bytes != int64(len(want)) {
		t.Errorf("Bytes = %d, want %d", result.Bytes, len(want))
	}
	written, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(written) != want {
		t.Errorf("written = %q, want %q", written, want)
	}
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "downloaded.json" {
		t.Errorf("tmpDir entries = %v, want exactly [downloaded.json] — no .kira-partial-* sibling", entries)
	}
}

// 26. downloadObject with an already-cancelled context leaves no file behind.
func TestS3_DownloadObject_AlreadyCancelledLeavesNoFile(t *testing.T) {
	fixture := testsupport.StartS3(t)
	a := connectedAdapter(t, fixture)

	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "never-written.txt")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := a.DownloadObject(ctx, model.ObjectDownloadRequest{
		Path: objectPath(fixture, testsupport.S3MainBucket, testsupport.S3RootObjectKey), DestPath: destPath,
	}, adapters.NewOpCtx("op-26"))
	if err == nil {
		t.Fatal("want an error")
	}
	if code, _ := adapters.CodeOf(err); code != adapters.CodeCancelled {
		t.Errorf("code = %v, want E_CANCELLED", code)
	}
	assertEmptyDir(t, tmpDir)
}

// A cancelled download mid-stream leaves no file — the case no probe and no static assertion can
// reach, and the one that extends scenario 26 to cover the actual load-bearing claim: an
// already-cancelled ctx never reaches io.Copy at all, so 26 alone never exercises the
// mid-stream cleanup path.
func TestS3_DownloadObject_CancelledMidStreamLeavesNoFile(t *testing.T) {
	fixture := testsupport.StartS3(t)
	a := connectedAdapter(t, fixture)

	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "never-finished.bin")
	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		_, err := a.DownloadObject(ctx, model.ObjectDownloadRequest{
			Path: objectPath(fixture, testsupport.S3MainBucket, testsupport.S3OversizedObjectKey), DestPath: destPath,
		}, adapters.NewOpCtx("op-26mid"))
		errCh <- err
	}()
	cancel() // races the copy — either wins, both leave no file behind (P58d D3's whole point)

	err := <-errCh
	if err == nil {
		t.Fatal("want an error")
	}
	if code, _ := adapters.CodeOf(err); code != adapters.CodeCancelled && code != adapters.CodeQuery {
		t.Errorf("code = %v, want E_CANCELLED or E_QUERY (a lost race against a fast HeadObject)", code)
	}
	assertEmptyDir(t, tmpDir)
}

func assertEmptyDir(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("dir entries = %v, want empty", entries)
	}
}

// 27. downloadObject on a nonexistent object is E_QUERY and creates no file.
func TestS3_DownloadObject_NonexistentObject(t *testing.T) {
	fixture := testsupport.StartS3(t)
	a := connectedAdapter(t, fixture)

	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "never-written.txt")
	_, err := a.DownloadObject(context.Background(), model.ObjectDownloadRequest{
		Path: objectPath(fixture, testsupport.S3MainBucket, "this-key-was-never-put.txt"), DestPath: destPath,
	}, adapters.NewOpCtx("op-27"))
	if err == nil {
		t.Fatal("want an error")
	}
	if code, _ := adapters.CodeOf(err); code != adapters.CodeQuery {
		t.Errorf("code = %v, want E_QUERY", code)
	}
	assertEmptyDir(t, tmpDir)
}

// 28. downloadObject reads the full body of an object too large to preview.
func TestS3_DownloadObject_OversizedObjectFullBody(t *testing.T) {
	fixture := testsupport.StartS3(t)
	a := connectedAdapter(t, fixture)

	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "oversized.bin")
	result, err := a.DownloadObject(context.Background(), model.ObjectDownloadRequest{
		Path: objectPath(fixture, testsupport.S3MainBucket, testsupport.S3OversizedObjectKey), DestPath: destPath,
	}, adapters.NewOpCtx("op-28"))
	if err != nil {
		t.Fatalf("DownloadObject: %v", err)
	}
	if result.Bytes != int64(testsupport.S3OversizedObjectBytes) {
		t.Errorf("Bytes = %d, want %d", result.Bytes, testsupport.S3OversizedObjectBytes)
	}
	written, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if len(written) != testsupport.S3OversizedObjectBytes {
		t.Errorf("len(written) = %d, want %d", len(written), testsupport.S3OversizedObjectBytes)
	}
	for i, b := range written {
		if b != 'x' {
			t.Fatalf("written[%d] = %q, want 'x'", i, b)
		}
	}
}
