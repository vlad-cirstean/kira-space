// Ported from packages/db-fixtures/mongo.spec.ts, case by case where practical — the spec's own
// numbering is kept in each test's name so the two can be diffed. The four cases that carry the
// most weight: the _id text/parser closure round-trip, byte-stable EJSON rendering, field order
// surviving a read, and cancel asserted server-side via killOp — all four are here, none softened.
package mongo_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	mongodriver "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	mongoadapter "github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters/mongo"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters/testsupport"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/page"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
)

// TestMain mirrors postgres_test.go's own: the container this whole package's tests share is torn
// down exactly once, after every test has run — never from an individual test's t.Cleanup.
func TestMain(m *testing.M) {
	code := m.Run()
	testsupport.StopMongo()
	os.Exit(code)
}

var (
	deps               = adapters.Deps{Log: func(level, message string) {}}
	regexpMongoVersion = testsupport.VersionPattern("MongoDB", testsupport.MongoServerMajor())
)

var (
	seg          = testsupport.Seg
	childNames   = testsupport.ChildNames
	containsName = testsupport.ContainsName
	docIDAt      = testsupport.DocIDAt
	docBodyAt    = testsupport.DocBodyAt
	strp         = testsupport.Strp
)

// slowFilter is a $where clause whose JS predicate runs a long busy loop — the one document in
// slow_probe makes this cheap to seed and still genuinely slow to evaluate server-side, giving
// TestMongo_Cancel_KillsServerSideOp a real window to observe the op in $currentOp and kill it
// before it finishes on its own.
func slowFilter() string {
	return `{"$where": "function() { var x = 0; for (var i = 0; i < 3000000000; i++) { x += i; } return true; }"}`
}

func derefStr(s *string) any {
	if s == nil {
		return nil
	}
	return *s
}

func newAdapter(t *testing.T) adapters.Adapter {
	t.Helper()
	a, err := adapters.CreateAdapter("mongodb", deps)
	if err != nil {
		t.Fatalf("CreateAdapter: %v", err)
	}
	return a
}

func connectedAdapter(t *testing.T, fixture *testsupport.MongoFixture) adapters.Adapter {
	t.Helper()
	a := newAdapter(t)
	if _, err := a.Connect(context.Background(), fixture.Config, adapters.NewOpCtx("connect")); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { _ = a.Disconnect(context.Background()) })
	return a
}

// nodePath is testsupport.FixtureNodePath instantiated for *MongoFixture (P107 I2-28).
var nodePath = testsupport.FixtureNodePath[*testsupport.MongoFixture]

// rootClient is a side connection for test-side assertions only (§1.6/C24: mutate_probe,
// literal_probe and slow_probe are created per test from the root client, never widgets).
func rootClient(t *testing.T, fixture *testsupport.MongoFixture) *mongodriver.Client {
	t.Helper()
	client, err := mongodriver.Connect(options.Client().ApplyURI(fixture.RootURI))
	if err != nil {
		t.Fatalf("root client connect: %v", err)
	}
	t.Cleanup(func() { _ = client.Disconnect(context.Background()) })
	return client
}

// 1. connect / disconnect
func TestMongo_ConnectDisconnect(t *testing.T) {
	fixture := testsupport.StartMongo(t)
	a := newAdapter(t)

	info, err := a.Connect(context.Background(), fixture.Config, adapters.NewOpCtx("op-1"))
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if !regexpMongoVersion.MatchString(info.ServerVersion) {
		t.Errorf("ServerVersion = %q, want to match %s", info.ServerVersion, regexpMongoVersion)
	}
	if info.Details["database"] != testsupport.MongoDatabase {
		t.Errorf("Details[database] = %q, want %q", info.Details["database"], testsupport.MongoDatabase)
	}

	if err := a.Disconnect(context.Background()); err != nil {
		t.Fatalf("Disconnect: %v", err)
	}
}

// P25 §2.1(1): the missing auth-failure case — wrong password is E_AUTH.
func TestMongo_Connect_AuthFailure(t *testing.T) {
	fixture := testsupport.StartMongo(t)
	badCfg := fixture.Config
	wrong := "definitely-wrong"
	badCfg.Password = &wrong

	a := newAdapter(t)
	_, err := a.Connect(context.Background(), badCfg, adapters.NewOpCtx("op-auth"))
	if err == nil {
		t.Fatal("want an error for a wrong password")
	}
	if code, _ := adapters.CodeOf(err); code != adapters.CodeAuth {
		t.Errorf("code = %v, want E_AUTH", code)
	}
}

// P25 §1.2/§2.1(2): a user created in `admin` with roles on kira_test cannot authenticate in
// fields mode without options.authSource — the URI path is MongoDB's own defaultauthdb, not merely
// the browse target. Regression test for buildURIFromFields's authSource handling.
func TestMongo_Connect_AuthSourceOption(t *testing.T) {
	fixture := testsupport.StartMongo(t)
	root := rootClient(t, fixture)

	const username, password = "p25_admin_scoped_user", "p25_pw"
	if err := root.Database("admin").RunCommand(context.Background(), bson.D{
		{Key: "createUser", Value: username},
		{Key: "pwd", Value: password},
		{Key: "roles", Value: bson.A{
			bson.D{{Key: "role", Value: "readWrite"}, {Key: "db", Value: testsupport.MongoDatabase}},
		}},
	}).Err(); err != nil {
		t.Fatalf("create admin-scoped user: %v", err)
	}
	t.Cleanup(func() {
		_ = root.Database("admin").RunCommand(context.Background(), bson.D{{Key: "dropUser", Value: username}}).Err()
	})

	cfg := fixture.Config
	cfg.Username = strp(username)
	cfg.Password = strp(password)
	cfg.Database = strp(testsupport.MongoDatabase)
	cfg.Options = map[string]any{"authSource": "admin"}

	a := newAdapter(t)
	info, err := a.Connect(context.Background(), cfg, adapters.NewOpCtx("op-authsource"))
	if err != nil {
		t.Fatalf("Connect with options.authSource=admin: %v", err)
	}
	t.Cleanup(func() { _ = a.Disconnect(context.Background()) })
	if info.Details["database"] != testsupport.MongoDatabase {
		t.Errorf("Details[database] = %q, want %q", info.Details["database"], testsupport.MongoDatabase)
	}
}

// 3/4. tree: databases, excluding the three system databases
func TestMongo_Children_ListDatabases(t *testing.T) {
	fixture := testsupport.StartMongo(t)
	a := connectedAdapter(t, fixture)

	children, err := a.Children(context.Background(), nodePath(fixture), adapters.NewOpCtx("op-2"))
	if err != nil {
		t.Fatalf("Children: %v", err)
	}
	names := childNames(t, children)
	if !containsName(names, testsupport.MongoDatabase) || !containsName(names, testsupport.MongoAnalyticsDatabase) {
		t.Errorf("names = %v, want both %q and %q", names, testsupport.MongoDatabase, testsupport.MongoAnalyticsDatabase)
	}
	for _, sys := range []string{"admin", "local", "config"} {
		if containsName(names, sys) {
			t.Errorf("names = %v, must not contain system database %q", names, sys)
		}
	}
}

// tree: collections, sorted, a view/collection distinction via Detail
func TestMongo_Children_ListCollections(t *testing.T) {
	fixture := testsupport.StartMongo(t)
	a := connectedAdapter(t, fixture)

	children, err := a.Children(context.Background(), nodePath(fixture, seg("database", testsupport.MongoDatabase)), adapters.NewOpCtx("op-3"))
	if err != nil {
		t.Fatalf("Children: %v", err)
	}
	names := childNames(t, children)
	for _, want := range []string{"widgets", "empty_collection", "oversized_widgets", "big_widgets", "validated_widgets"} {
		if !containsName(names, want) {
			t.Errorf("names = %v, want to contain %q", names, want)
		}
	}
	for i := 1; i < len(names); i++ {
		if names[i-1] > names[i] {
			t.Errorf("names not sorted: %v", names)
			break
		}
	}
	for _, n := range children.Nodes {
		if n.Kind == "collection" && n.HasChildren {
			t.Errorf("collection %q: HasChildren = true, want false (a leaf, P19 D5)", n.Name)
		}
	}
}

// a collection is a leaf: Children returns [] (C16 — must marshal as [], never null)
func TestMongo_Children_CollectionIsLeaf(t *testing.T) {
	fixture := testsupport.StartMongo(t)
	a := connectedAdapter(t, fixture)

	children, err := a.Children(context.Background(),
		nodePath(fixture, seg("database", testsupport.MongoDatabase), seg("collection", "widgets")),
		adapters.NewOpCtx("op-4"))
	if err != nil {
		t.Fatalf("Children: %v", err)
	}
	if children.Nodes == nil {
		t.Fatal("Nodes is nil, want an empty non-nil slice")
	}
	if len(children.Nodes) != 0 {
		t.Errorf("Nodes = %v, want empty", children.Nodes)
	}
}

// describe: indexes, including the primary _id_ index and the unique name index
func TestMongo_Describe_Indexes(t *testing.T) {
	fixture := testsupport.StartMongo(t)
	a := connectedAdapter(t, fixture)

	meta, err := a.Describe(context.Background(),
		nodePath(fixture, seg("database", testsupport.MongoDatabase), seg("collection", "widgets")),
		adapters.NewOpCtx("op-5"))
	if err != nil {
		t.Fatalf("Describe: %v", err)
	}
	var sawPrimary, sawUniqueName bool
	for _, idx := range meta.Indexes {
		if idx.Name == "_id_" {
			sawPrimary = true
			if !idx.Primary {
				t.Errorf("_id_ index: Primary = false, want true")
			}
		}
		if len(idx.Columns) == 1 && idx.Columns[0] == "name" {
			sawUniqueName = true
			if !idx.Unique {
				t.Errorf("name index: Unique = false, want true")
			}
		}
	}
	if !sawPrimary {
		t.Error("no _id_ index found")
	}
	if !sawUniqueName {
		t.Error("no unique name index found")
	}
}

// definition: no options -> the NO_OPTIONS_NOTE
func TestMongo_Definition_NoOptions(t *testing.T) {
	fixture := testsupport.StartMongo(t)
	a := connectedAdapter(t, fixture)

	def, err := a.Definition(context.Background(),
		nodePath(fixture, seg("database", testsupport.MongoDatabase), seg("collection", "empty_collection")),
		adapters.NewOpCtx("op-6"))
	if err != nil {
		t.Fatalf("Definition: %v", err)
	}
	if len(def.Notes) != 1 || !strings.Contains(def.Notes[0], "no creation options set") {
		t.Errorf("Notes = %v, want the no-options note", def.Notes)
	}
	if def.DocumentSchema == nil || def.DocumentSchema.IsJSONSchema {
		t.Errorf("DocumentSchema = %+v, want a non-nil, non-json-schema schema", def.DocumentSchema)
	}
}

// definition: a real $jsonSchema renders as the Validation section's field table
func TestMongo_Definition_JSONSchema(t *testing.T) {
	fixture := testsupport.StartMongo(t)
	a := connectedAdapter(t, fixture)

	def, err := a.Definition(context.Background(),
		nodePath(fixture, seg("database", testsupport.MongoDatabase), seg("collection", "validated_widgets")),
		adapters.NewOpCtx("op-7"))
	if err != nil {
		t.Fatalf("Definition: %v", err)
	}
	if def.DocumentSchema == nil || !def.DocumentSchema.IsJSONSchema {
		t.Fatalf("DocumentSchema = %+v, want IsJSONSchema", def.DocumentSchema)
	}
	if def.DocumentSchema.ValidationLevel == nil || *def.DocumentSchema.ValidationLevel != "moderate" {
		t.Errorf("ValidationLevel = %v, want moderate", def.DocumentSchema.ValidationLevel)
	}
	if def.DocumentSchema.ValidationAction == nil || *def.DocumentSchema.ValidationAction != "warn" {
		t.Errorf("ValidationAction = %v, want warn", def.DocumentSchema.ValidationAction)
	}
	if def.DocumentSchema.Validator == nil || !strings.Contains(*def.DocumentSchema.Validator, "must be a string and is required") {
		t.Errorf("Validator = %v, want the schema's own description text", def.DocumentSchema.Validator)
	}
}

// 8/9. keyset pagination forward over widgets, ordered by _id ascending (the default, unsorted)
func TestMongo_Read_KeysetForward(t *testing.T) {
	fixture := testsupport.StartMongo(t)
	a := connectedAdapter(t, fixture)
	path := nodePath(fixture, seg("database", testsupport.MongoDatabase), seg("collection", "widgets"))

	req := adapters.ReadRequest{Path: path, PageSize: 5, Cursor: model.PageCursor{Mode: "offset", Offset: 0}}
	p, err := a.Read(context.Background(), req, adapters.NewOpCtx("op-8"))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	docPage, ok := p.(page.DocumentPage)
	if !ok {
		t.Fatalf("Read returned %T, want page.DocumentPage", p)
	}
	if docPage.RowCount != 5 {
		t.Fatalf("RowCount = %d, want 5", docPage.RowCount)
	}
	if !docPage.Position.HasMore {
		t.Error("HasMore = false, want true (25 widgets, page size 5)")
	}
	if docPage.Position.NextToken == nil {
		t.Fatal("NextToken is nil, want a token")
	}

	// widget-0's fixed hex _id sorts first — first page's first row must be it.
	firstBody := docBodyAt(t, docPage, 0)
	if firstBody == nil || !strings.Contains(*firstBody, `"name":"widget-0"`) {
		t.Errorf("first row body = %v, want to contain widget-0", derefStr(firstBody))
	}

	req2 := adapters.ReadRequest{Path: path, PageSize: 5, Cursor: model.PageCursor{Mode: "after", Token: *docPage.Position.NextToken}}
	p2, err := a.Read(context.Background(), req2, adapters.NewOpCtx("op-9"))
	if err != nil {
		t.Fatalf("Read (page 2): %v", err)
	}
	docPage2 := p2.(page.DocumentPage)
	body2 := docBodyAt(t, docPage2, 0)
	if body2 == nil || !strings.Contains(*body2, `"name":"widget-5"`) {
		t.Errorf("page 2 first row body = %v, want to contain widget-5", derefStr(body2))
	}
	if docPage2.Position.PrevToken == nil {
		t.Error("page 2 PrevToken is nil, want a token back to page 1")
	}
}

// offset pagination for a non-_id sort
func TestMongo_Read_OffsetSort(t *testing.T) {
	fixture := testsupport.StartMongo(t)
	a := connectedAdapter(t, fixture)
	path := nodePath(fixture, seg("database", testsupport.MongoDatabase), seg("collection", "widgets"))

	req := adapters.ReadRequest{
		Path: path, PageSize: 3,
		Sort:   &model.SortSpec{Kind: "structured", Terms: []model.SortTerm{{Column: "name", Direction: "desc"}}},
		Cursor: model.PageCursor{Mode: "offset", Offset: 0},
	}
	p, err := a.Read(context.Background(), req, adapters.NewOpCtx("op-10"))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	docPage := p.(page.DocumentPage)
	if docPage.Position.Strategy != "offset" {
		t.Errorf("Strategy = %q, want offset (non-_id sort)", docPage.Position.Strategy)
	}
	// Lexicographic descending: widget-9, widget-8, widget-24 ... "widget-9" > "widget-8" >
	// "widget-24" as strings.
	first := docBodyAt(t, docPage, 0)
	if first == nil || !strings.Contains(*first, `"name":"widget-9"`) {
		t.Errorf("first row = %v, want widget-9 (lexicographic max)", first)
	}
}

// projection: only the requested fields (plus _id) come back
func TestMongo_Read_Projection(t *testing.T) {
	fixture := testsupport.StartMongo(t)
	a := connectedAdapter(t, fixture)
	path := nodePath(fixture, seg("database", testsupport.MongoDatabase), seg("collection", "widgets"))

	req := adapters.ReadRequest{Path: path, Projection: []string{"name"}, PageSize: 1, Cursor: model.PageCursor{Mode: "offset", Offset: 0}}
	p, err := a.Read(context.Background(), req, adapters.NewOpCtx("op-11"))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	docPage := p.(page.DocumentPage)
	body := docBodyAt(t, docPage, 0)
	if body == nil {
		t.Fatal("body is nil")
	}
	if !strings.Contains(*body, `"name"`) || strings.Contains(*body, `"price"`) {
		t.Errorf("body = %s, want name present and price absent", *body)
	}
	if !strings.Contains(*body, `"_id"`) {
		t.Errorf("body = %s, want _id present even though it was not in the projection", *body)
	}
}

// server-side filter via literal.go's grammar
func TestMongo_Read_Filter(t *testing.T) {
	fixture := testsupport.StartMongo(t)
	a := connectedAdapter(t, fixture)
	path := nodePath(fixture, seg("database", testsupport.MongoDatabase), seg("collection", "widgets"))

	filter := `{ name: "widget-3" }`
	req := adapters.ReadRequest{Path: path, Filter: &filter, PageSize: 10, Cursor: model.PageCursor{Mode: "offset", Offset: 0}}
	p, err := a.Read(context.Background(), req, adapters.NewOpCtx("op-12"))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	docPage := p.(page.DocumentPage)
	if docPage.RowCount != 1 {
		t.Fatalf("RowCount = %d, want 1", docPage.RowCount)
	}
}

// A scalar _id filter (a single-document lookup) combined with a keyset
// pagination token used to be silently dropped — read.go's old merge only recognized an existing
// _id value shaped as an operator document, so a bare scalar failed that type assertion and the
// keyset boundary condition overwrote it outright. A hand-minted token (matching the fingerprint
// readPage itself computes from path/filter/sort/pageSize) exercises the exact request shape a
// client legitimately resuming keyset pagination alongside a scalar _id filter would send, without
// depending on the current UI's own token-reset behavior to ever construct one. widget-9's _id
// sorts past widget-4 (the keyset boundary chosen below): if the scalar filter survives the merge,
// only widget-9 comes back; if it was dropped, every widget past the boundary would match instead.
func TestMongo_Read_KeysetWithScalarIDFilter(t *testing.T) {
	fixture := testsupport.StartMongo(t)
	a := connectedAdapter(t, fixture)
	path := nodePath(fixture, seg("database", testsupport.MongoDatabase), seg("collection", "widgets"))

	filter := `{ _id: ObjectId("000000000000000000000009") }`
	fingerprint := adapters.RequestFingerprint(struct {
		Path     string          `json:"path"`
		Filter   *string         `json:"filter"`
		Sort     *model.SortSpec `json:"sort"`
		PageSize int             `json:"pageSize"`
	}{"widgets", &filter, nil, 10})
	boundary, err := bson.ObjectIDFromHex("000000000000000000000004") // widget-4
	if err != nil {
		t.Fatalf("ObjectIDFromHex: %v", err)
	}
	boundaryText, err := mongoadapter.IDText(boundary)
	if err != nil {
		t.Fatalf("IDText: %v", err)
	}
	token := adapters.EncodePageToken([]string{boundaryText}, fingerprint)

	req := adapters.ReadRequest{
		Path: path, Filter: &filter, PageSize: 10,
		Cursor: model.PageCursor{Mode: "after", Token: token},
	}
	p, err := a.Read(context.Background(), req, adapters.NewOpCtx("op-13"))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	docPage := p.(page.DocumentPage)
	if docPage.RowCount != 1 {
		t.Fatalf("RowCount = %d, want 1 (the scalar _id filter must survive the keyset merge)", docPage.RowCount)
	}
	body := docBodyAt(t, docPage, 0)
	if body == nil || !strings.Contains(*body, `"name":"widget-9"`) {
		t.Errorf("row body = %v, want widget-9", derefStr(body))
	}
}

// count: estimate (no filter) vs exact (filtered)
func TestMongo_Count(t *testing.T) {
	fixture := testsupport.StartMongo(t)
	a := connectedAdapter(t, fixture)
	path := nodePath(fixture, seg("database", testsupport.MongoDatabase), seg("collection", "widgets"))

	unfiltered, err := a.Count(context.Background(), adapters.CountRequest{Path: path}, adapters.NewOpCtx("op-13"))
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if unfiltered.Exact {
		t.Error("unfiltered Count: Exact = true, want false (estimatedDocumentCount)")
	}
	if unfiltered.Value != testsupport.WidgetCount {
		t.Errorf("unfiltered Count = %d, want %d", unfiltered.Value, testsupport.WidgetCount)
	}

	filter := `{ active: true }`
	filtered, err := a.Count(context.Background(), adapters.CountRequest{Path: path, Filter: &filter}, adapters.NewOpCtx("op-14"))
	if err != nil {
		t.Fatalf("Count (filtered): %v", err)
	}
	if !filtered.Exact {
		t.Error("filtered Count: Exact = false, want true (countDocuments)")
	}
	if filtered.Value != 13 { // i % 2 == 0 for i in 0..24 -> 13 evens
		t.Errorf("filtered Count = %d, want 13", filtered.Value)
	}
}

// _id text round-trips through the parser (new, C3): for every widget (ObjectId _ids) and a
// probe with an integer _id, ParseFilterObject(IDText(doc))-as-a-filter must match exactly that
// one document.
func TestMongo_IDText_RoundTrips(t *testing.T) {
	fixture := testsupport.StartMongo(t)
	root := rootClient(t, fixture)
	db := root.Database(testsupport.MongoDatabase)

	cursor, err := db.Collection("widgets").Find(context.Background(), bson.D{})
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	var widgets []bson.D
	if err := cursor.All(context.Background(), &widgets); err != nil {
		t.Fatalf("cursor.All: %v", err)
	}
	if len(widgets) != testsupport.WidgetCount {
		t.Fatalf("len(widgets) = %d, want %d", len(widgets), testsupport.WidgetCount)
	}

	// mutate_probe: an integer-keyed document, created fresh from the root client (C24) — never
	// widgets, which six other scenarios assert against exactly.
	probe := db.Collection("id_probe")
	if _, err := probe.InsertOne(context.Background(), bson.D{{Key: "_id", Value: int32(42)}, {Key: "label", Value: "int-keyed"}}); err != nil {
		t.Fatalf("insert probe: %v", err)
	}
	var probeDoc bson.D
	if err := probe.FindOne(context.Background(), bson.D{{Key: "_id", Value: int32(42)}}).Decode(&probeDoc); err != nil {
		t.Fatalf("FindOne probe: %v", err)
	}

	// Two real consumers of IDText's own output exist, and both must accept it: mutate.go's
	// parseIdKey treats the text as a bare *value* (exactly
	// mutate.ts's own "an _id is a value, not a document" comment — this is what "Copy _id" then
	// "paste into the mutation key box" round-trips through); a filter box instead wraps it as
	// {_id: <text>}, a genuine document, which is what "Copy _id" then "paste into the filter box"
	// round-trips through. Neither path feeds the bare id text straight into ParseFilterObject:
	// for an ObjectId that text is itself a one-key document ({"$oid": "..."}), and resolving it
	// whole collapses to the scalar ObjectID, not a {_id: ...} filter — a real trap this test
	// exists to keep documented rather than rediscovered.
	check := func(name string, coll string, id any) {
		text, err := mongoadapter.IDText(id)
		if err != nil {
			t.Fatalf("%s: IDText: %v", name, err)
		}

		// parseIdKey's own path.
		value, err := mongoadapter.ParseJSON5Literal(text)
		if err != nil {
			t.Fatalf("%s: ParseJSON5Literal(%q): %v", name, text, err)
		}
		resolved := mongoadapter.ResolveEJSONWrappers(value)
		var foundByValue bson.D
		if err := db.Collection(coll).FindOne(context.Background(), bson.D{{Key: "_id", Value: resolved}}).Decode(&foundByValue); err != nil {
			t.Fatalf("%s: FindOne via parseIdKey's own path (id text %q): %v", name, text, err)
		}

		// The filter-box path.
		filterText := "{_id: " + text + "}"
		filterDoc, err := mongoadapter.ParseFilterObject(&filterText)
		if err != nil {
			t.Fatalf("%s: ParseFilterObject(%q): %v", name, filterText, err)
		}
		var foundByFilter bson.D
		if err := db.Collection(coll).FindOne(context.Background(), filterDoc).Decode(&foundByFilter); err != nil {
			t.Fatalf("%s: FindOne via the filter box's own path (filter %q): %v", name, filterText, err)
		}
	}
	for _, w := range widgets {
		check("widgets", "widgets", mustLookup(t, w, "_id"))
	}
	check("id_probe", "id_probe", mustLookup(t, probeDoc, "_id"))
}

func mustLookup(t *testing.T, d bson.D, key string) any {
	t.Helper()
	for _, e := range d {
		if e.Key == key {
			return e.Value
		}
	}
	t.Fatalf("key %q not found in %+v", key, d)
	return nil
}

// EJSON rendering is byte-stable across every fixture type (new, C2): a widget's body must
// contain the exact wrappers for its ObjectId, its Date, its double price, its array tags and its
// nested meta — asserted as literal strings.
func TestMongo_Read_EJSONByteStable(t *testing.T) {
	fixture := testsupport.StartMongo(t)
	a := connectedAdapter(t, fixture)
	path := nodePath(fixture, seg("database", testsupport.MongoDatabase), seg("collection", "widgets"))

	// widget-0: _id 000000000000000000000000, price 1.5, active true, createdAt 2024-01-01,
	// tags [red,small] (0 % 3 == 0), meta.note null (0 % 5 == 0).
	filter := `{ name: "widget-0" }`
	req := adapters.ReadRequest{Path: path, Filter: &filter, PageSize: 1, Cursor: model.PageCursor{Mode: "offset", Offset: 0}}
	p, err := a.Read(context.Background(), req, adapters.NewOpCtx("op-15"))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	docPage := p.(page.DocumentPage)
	body := docBodyAt(t, docPage, 0)
	if body == nil {
		t.Fatal("body is nil")
	}
	id := docIDAt(t, docPage, 0)
	if id == nil || *id != `{"$oid":"000000000000000000000000"}` {
		t.Errorf("id = %v, want the canonical $oid wrapper", id)
	}
	wantSubstrings := []string{
		`"_id":{"$oid":"000000000000000000000000"}`,
		`"price":{"$numberDouble":"1.5"}`,
		`"active":true`,
		`"createdAt":{"$date":{"$numberLong":"1704067200000"}}`,
		`"tags":["red","small"]`,
		`"meta":{"weight":{"$numberInt":"0"},"note":null}`,
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(*body, want) {
			t.Errorf("body = %s, want to contain %s", *body, want)
		}
	}
}

// field order survives a read (new, C2): a widgets document's rendered body has its keys in
// insertion order. A bson.M decode would pass every other test in this file and fail this one.
func TestMongo_Read_FieldOrderSurvives(t *testing.T) {
	fixture := testsupport.StartMongo(t)
	a := connectedAdapter(t, fixture)
	path := nodePath(fixture, seg("database", testsupport.MongoDatabase), seg("collection", "widgets"))

	filter := `{ name: "widget-1" }`
	req := adapters.ReadRequest{Path: path, Filter: &filter, PageSize: 1, Cursor: model.PageCursor{Mode: "offset", Offset: 0}}
	p, err := a.Read(context.Background(), req, adapters.NewOpCtx("op-16"))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	docPage := p.(page.DocumentPage)
	body := docBodyAt(t, docPage, 0)
	if body == nil {
		t.Fatal("body is nil")
	}
	wantOrder := []string{`"_id"`, `"name"`, `"price"`, `"active"`, `"createdAt"`, `"tags"`, `"meta"`}
	lastIdx := -1
	for _, key := range wantOrder {
		idx := strings.Index(*body, key)
		if idx < 0 {
			t.Fatalf("body = %s, missing key %s", *body, key)
		}
		if idx < lastIdx {
			t.Errorf("body = %s, key %s appears out of insertion order", *body, key)
		}
		lastIdx = idx
	}
}

// mutate: insert / update / delete, against a fresh probe collection (C24)
func TestMongo_Mutate_InsertUpdateDelete(t *testing.T) {
	fixture := testsupport.StartMongo(t)
	a := connectedAdapter(t, fixture)
	path := nodePath(fixture, seg("database", testsupport.MongoDatabase), seg("collection", "mutate_probe"))

	insertPlan := model.MutationPlan{Path: path, Ops: []model.MutationRowOp{{
		Kind:   "insert",
		Values: model.RowValues{{Name: "$document", Value: strp(`{ name: "probe-1", n: 1 }`)}},
	}}}
	preview, err := a.Preview(insertPlan)
	if err != nil {
		t.Fatalf("Preview (insert): %v", err)
	}
	if len(preview) != 1 || !strings.Contains(preview[0], "insertOne") {
		t.Errorf("Preview = %v, want an insertOne() line", preview)
	}
	result, err := a.Mutate(context.Background(), insertPlan, adapters.NewOpCtx("op-17"))
	if err != nil {
		t.Fatalf("Mutate (insert): %v", err)
	}
	if result.AffectedRows != 1 {
		t.Fatalf("AffectedRows = %d, want 1", result.AffectedRows)
	}

	root := rootClient(t, fixture)
	var inserted bson.D
	if err := root.Database(testsupport.MongoDatabase).Collection("mutate_probe").FindOne(context.Background(), bson.D{{Key: "name", Value: "probe-1"}}).Decode(&inserted); err != nil {
		t.Fatalf("FindOne after insert: %v", err)
	}
	id := mustLookup(t, inserted, "_id")
	idText, err := mongoadapter.IDText(id)
	if err != nil {
		t.Fatalf("idText: %v", err)
	}

	updatePlan := model.MutationPlan{Path: path, Ops: []model.MutationRowOp{{
		Kind:    "update",
		Key:     model.RowValues{{Name: "_id", Value: &idText}},
		Changes: model.RowValues{{Name: "$document", Value: strp(`{ name: "probe-1-updated", n: 2 }`)}},
	}}}
	if _, err := a.Mutate(context.Background(), updatePlan, adapters.NewOpCtx("op-18")); err != nil {
		t.Fatalf("Mutate (update): %v", err)
	}
	var updated bson.D
	if err := root.Database(testsupport.MongoDatabase).Collection("mutate_probe").FindOne(context.Background(), bson.D{{Key: "name", Value: "probe-1-updated"}}).Decode(&updated); err != nil {
		t.Fatalf("FindOne after update: %v", err)
	}

	deletePlan := model.MutationPlan{Path: path, Ops: []model.MutationRowOp{{
		Kind: "delete",
		Key:  model.RowValues{{Name: "_id", Value: &idText}},
	}}}
	if _, err := a.Mutate(context.Background(), deletePlan, adapters.NewOpCtx("op-19")); err != nil {
		t.Fatalf("Mutate (delete): %v", err)
	}
	count, err := root.Database(testsupport.MongoDatabase).Collection("mutate_probe").CountDocuments(context.Background(), bson.D{{Key: "_id", Value: id}})
	if err != nil {
		t.Fatalf("CountDocuments after delete: %v", err)
	}
	if count != 0 {
		t.Errorf("count after delete = %d, want 0", count)
	}
}

// mutate: an insert whose values carry no $document sentinel is E_UNSUPPORTED (rewritten per
// §1.12 — mongo.spec.ts 16's own title was stale; mongoCaps.canInsert is true).
func TestMongo_Mutate_InsertWithoutDocumentSentinelIsUnsupported(t *testing.T) {
	fixture := testsupport.StartMongo(t)
	a := connectedAdapter(t, fixture)
	path := nodePath(fixture, seg("database", testsupport.MongoDatabase), seg("collection", "mutate_probe"))

	plan := model.MutationPlan{Path: path, Ops: []model.MutationRowOp{{
		Kind:   "insert",
		Values: model.RowValues{{Name: "name", Value: strp("no-sentinel")}},
	}}}
	_, err := a.Mutate(context.Background(), plan, adapters.NewOpCtx("op-20"))
	if err == nil {
		t.Fatal("Mutate: want an error, got nil")
	}
	code, _ := adapters.CodeOf(err)
	if code != adapters.CodeContract {
		t.Errorf("code = %v, want E_CONTRACT", code)
	}
	if err.Error() != "document mutation requires a $document body" {
		t.Errorf("message = %q", err.Error())
	}
}

// mutate: exact-affected-row guards, message verbatim
func TestMongo_Mutate_DeleteZeroRowsIsError(t *testing.T) {
	fixture := testsupport.StartMongo(t)
	a := connectedAdapter(t, fixture)
	path := nodePath(fixture, seg("database", testsupport.MongoDatabase), seg("collection", "mutate_probe"))

	missingID, err := mongoadapter.IDText(bson.NewObjectID())
	if err != nil {
		t.Fatalf("idText: %v", err)
	}
	plan := model.MutationPlan{Path: path, Ops: []model.MutationRowOp{{
		Kind: "delete",
		Key:  model.RowValues{{Name: "_id", Value: &missingID}},
	}}}
	_, err = a.Mutate(context.Background(), plan, adapters.NewOpCtx("op-21"))
	if err == nil {
		t.Fatal("want an error, got nil")
	}
	if err.Error() != "expected delete to affect exactly one document, deleted 0" {
		t.Errorf("message = %q, want the verbatim adapter message", err.Error())
	}
}

// mongo/adapter.go:60 sets a.readOnly from the config and nothing anywhere asserted
// it — mongo and redis are the two writable adapters with no such test (every other writable
// adapter has one: postgres, mysqlfamily, clickhouse, sqlite, s3). Preview is deliberately not
// asserted here: unlike Mutate, no adapter's Preview ever consults readOnly — it is pure local text
// generation that "never executes and never touches the network" by design (adapter.go's own
// Adapter interface doc), so there is nothing for a read-only connection to change about its
// output.
func TestMongo_ReadOnlyConnectionCannotWrite(t *testing.T) {
	fixture := testsupport.StartMongo(t)
	ro := fixture.Config
	ro.ReadOnly = true
	a := newAdapter(t)
	if _, err := a.Connect(context.Background(), ro, adapters.NewOpCtx("op-ro-connect")); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer a.Disconnect(context.Background())

	// widget-0's own _id, per support/mongo.go's hexID(0) — unexported, so spelled out verbatim.
	id, err := bson.ObjectIDFromHex("000000000000000000000000")
	if err != nil {
		t.Fatalf("ObjectIDFromHex: %v", err)
	}
	idText, err := mongoadapter.IDText(id)
	if err != nil {
		t.Fatalf("IDText: %v", err)
	}

	path := nodePath(fixture, seg("database", testsupport.MongoDatabase), seg("collection", "widgets"))
	plan := model.MutationPlan{Path: path, Ops: []model.MutationRowOp{{
		Kind:    "update",
		Key:     model.RowValues{{Name: "_id", Value: &idText}},
		Changes: model.RowValues{{Name: "$document", Value: strp(`{ name: "should-not-land", n: 999 }`)}},
	}}}
	_, err = a.Mutate(context.Background(), plan, adapters.NewOpCtx("op-ro-mutate"))
	if err == nil {
		t.Fatal("Mutate: want an error, got nil")
	}
	code, _ := adapters.CodeOf(err)
	if code != adapters.CodeUnsupported {
		t.Errorf("code = %v, want E_UNSUPPORTED", code)
	}

	root := rootClient(t, fixture)
	var doc bson.D
	if err := root.Database(testsupport.MongoDatabase).Collection("widgets").FindOne(context.Background(), bson.D{{Key: "_id", Value: id}}).Decode(&doc); err != nil {
		t.Fatalf("FindOne: %v", err)
	}
	if name := mustLookup(t, doc, "name"); name != "widget-0" {
		t.Errorf("name = %v, want unchanged widget-0 (a read-only Mutate must never land)", name)
	}
}

// Mongo's whole DDL analogue — Mutate an insert into a collection name that does not exist yet,
// then Children(database) lists it, then Read returns the document, then Mutate deletes it. The
// closest thing Mongo has to "does DDL reach the catalog", exercising catalog.go's
// ListCollectionSpecifications against an object created in the same test.
func TestMongo_Mutate_InsertCreatesTheCollectionAndItAppearsInTheTree(t *testing.T) {
	fixture := testsupport.StartMongo(t)
	a := connectedAdapter(t, fixture)
	ctx := context.Background()
	const collectionName = "p26_scratch"
	databasePath := nodePath(fixture, seg("database", testsupport.MongoDatabase))
	collectionPath := nodePath(fixture, seg("database", testsupport.MongoDatabase), seg("collection", collectionName))

	root := rootClient(t, fixture)
	t.Cleanup(func() {
		_ = root.Database(testsupport.MongoDatabase).Collection(collectionName).Drop(context.Background())
	})

	insertPlan := model.MutationPlan{Path: collectionPath, Ops: []model.MutationRowOp{{
		Kind:   "insert",
		Values: model.RowValues{{Name: "$document", Value: strp(`{ name: "scratch-1", n: 1 }`)}},
	}}}
	if _, err := a.Mutate(ctx, insertPlan, adapters.NewOpCtx("op-ddl-1")); err != nil {
		t.Fatalf("Mutate(insert): %v", err)
	}

	children, err := a.Children(ctx, databasePath, adapters.NewOpCtx("op-ddl-2"))
	if err != nil {
		t.Fatalf("Children: %v", err)
	}
	if !containsName(childNames(t, children), collectionName) {
		t.Fatalf("Children(%s) = %v, want %s present", testsupport.MongoDatabase, childNames(t, children), collectionName)
	}

	read, err := a.Read(ctx, adapters.ReadRequest{
		Path: collectionPath, PageSize: 10, Cursor: model.PageCursor{Mode: "offset", Offset: 0},
	}, adapters.NewOpCtx("op-ddl-3"))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	docPage := read.(page.DocumentPage)
	if docPage.RowCount != 1 {
		t.Fatalf("RowCount = %d, want 1", docPage.RowCount)
	}
	body := docBodyAt(t, docPage, 0)
	if body == nil || !strings.Contains(*body, "scratch-1") {
		t.Errorf("body = %v, want it to contain scratch-1", derefStr(body))
	}
	id := docIDAt(t, docPage, 0)
	if id == nil {
		t.Fatal("id is nil")
	}

	deletePlan := model.MutationPlan{Path: collectionPath, Ops: []model.MutationRowOp{{
		Kind: "delete", Key: model.RowValues{{Name: "_id", Value: id}},
	}}}
	if _, err := a.Mutate(ctx, deletePlan, adapters.NewOpCtx("op-ddl-4")); err != nil {
		t.Fatalf("Mutate(delete): %v", err)
	}
	count, err := root.Database(testsupport.MongoDatabase).Collection(collectionName).CountDocuments(ctx, bson.D{})
	if err != nil {
		t.Fatalf("CountDocuments after delete: %v", err)
	}
	if count != 0 {
		t.Errorf("count after delete = %d, want 0", count)
	}
}

// console: find / insertOne / countDocuments through the shell statement grammar
func TestMongo_Console_Execute(t *testing.T) {
	fixture := testsupport.StartMongo(t)
	a := connectedAdapter(t, fixture)
	path := nodePath(fixture, seg("database", testsupport.MongoDatabase))

	pages, err := a.Execute(context.Background(), model.ConsoleRequest{
		Path:       path,
		Statements: []string{`db.console_probe.insertOne({ n: 1 })`, `db.console_probe.countDocuments({})`},
	}, adapters.NewOpCtx("op-22"))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(pages) != 2 {
		t.Fatalf("len(pages) = %d, want 2", len(pages))
	}
	countPage, ok := pages[1].(page.DocumentPage)
	if !ok {
		t.Fatalf("pages[1] = %T, want page.DocumentPage", pages[1])
	}
	body := docBodyAt(t, countPage, 0)
	if body == nil || !strings.Contains(*body, `"count":{"$numberLong":"1"}`) {
		t.Errorf("countDocuments status body = %v, want count:1 (canonical EJSON)", derefStr(body))
	}
}

// P174: find stops at the result cap; exactly-cap results are not flagged.
func TestMongo_Console_StopsAtResultCap(t *testing.T) {
	fixture := testsupport.StartMongo(t)
	a := connectedAdapter(t, fixture)
	path := nodePath(fixture, seg("database", testsupport.MongoDatabase))
	if _, err := a.Execute(context.Background(), model.ConsoleRequest{
		Path:       path,
		Statements: []string{`db.cap_probe.insertMany([{n:1},{n:2},{n:3},{n:4},{n:5},{n:6}])`},
	}, adapters.NewOpCtx("op-cap-seed")); err != nil {
		t.Fatalf("seed: %v", err)
	}
	find := func(limit int) page.DocumentPage {
		pages, err := a.Execute(context.Background(), model.ConsoleRequest{
			Path: path, Statements: []string{`db.cap_probe.find({})`}, Cap: page.ResultCap{Rows: limit},
		}, adapters.NewOpCtx("op-cap-find"))
		if err != nil {
			t.Fatalf("find: %v", err)
		}
		return pages[0].(page.DocumentPage)
	}
	if p := find(5); p.RowCount != 5 || !p.Position.Truncated {
		t.Errorf("cap 5 of 6: RowCount = %d, Truncated = %v, want 5, true", p.RowCount, p.Position.Truncated)
	}
	if p := find(6); p.RowCount != 6 || p.Position.Truncated {
		t.Errorf("cap 6 of 6: RowCount = %d, Truncated = %v, want 6, false", p.RowCount, p.Position.Truncated)
	}
}

// console: an unsupported method is rejected
func TestMongo_Console_UnsupportedMethod(t *testing.T) {
	fixture := testsupport.StartMongo(t)
	a := connectedAdapter(t, fixture)
	path := nodePath(fixture, seg("database", testsupport.MongoDatabase))

	_, err := a.Execute(context.Background(), model.ConsoleRequest{
		Path:       path,
		Statements: []string{`db.widgets.drop()`},
	}, adapters.NewOpCtx("op-23"))
	if err == nil {
		t.Fatal("want an error, got nil")
	}
	code, _ := adapters.CodeOf(err)
	if code != adapters.CodeUnsupported {
		t.Errorf("code = %v, want E_UNSUPPORTED", code)
	}
}

// cap honesty: mongoCaps's literal, one comparison
func TestMongo_Caps(t *testing.T) {
	a := newAdapter(t)
	c := a.Caps()
	if !c.Documents || c.Tabular || c.KeyValue || c.Stream {
		t.Errorf("Caps page-kind flags = %+v, want documents-only", c)
	}
	if c.Pagination != adapters.PaginationCursor {
		t.Errorf("Pagination = %v, want cursor", c.Pagination)
	}
	if c.ExactCount {
		t.Error("ExactCount = true, want false (estimate-only default)")
	}
	if !c.Cancel {
		t.Error("Cancel = false, want true")
	}
	if c.SchemaColumns {
		t.Error("SchemaColumns = true, want false (a Mongo collection has no field-level schema, F11)")
	}
}

// P22c §4.1/D8: Mongo has no field-level schema — SchemaColumns is unreachable, and its own D8
// mechanism (field names sampled from loaded documents) is a different one entirely.
func TestMongo_SchemaColumns_Unsupported(t *testing.T) {
	a := newAdapter(t)
	_, err := a.SchemaColumns(context.Background(), model.NodePath{}, adapters.NewOpCtx("op-schema-columns"))
	code, _ := adapters.CodeOf(err)
	if code != adapters.CodeUnsupported {
		t.Errorf("code = %v, want E_UNSUPPORTED", code)
	}
}

// an already-cancelled context rejects before running anything (Adapter rule 2)
func TestMongo_Read_AlreadyCancelledRejectsBeforeRunning(t *testing.T) {
	fixture := testsupport.StartMongo(t)
	a := connectedAdapter(t, fixture)
	path := nodePath(fixture, seg("database", testsupport.MongoDatabase), seg("collection", "widgets"))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := a.Read(ctx, adapters.ReadRequest{Path: path, PageSize: 5, Cursor: model.PageCursor{Mode: "offset", Offset: 0}}, adapters.NewOpCtx("op-24"))
	if err == nil {
		t.Fatal("want an error, got nil")
	}
	code, _ := adapters.CodeOf(err)
	if code != adapters.CodeCancelled {
		t.Errorf("code = %v, want E_CANCELLED", code)
	}
}

// cancel, asserted server-side via killOp (must not be softened): a slow operation is started, the
// test polls $currentOp through a separate root client until it appears, Cancel is called, Cancel
// returns true, and the op rejects with E_QUERY (the server killed it — not E_CANCELLED, which
// would mean the local abort won).
func TestMongo_Cancel_KillsServerSideOp(t *testing.T) {
	fixture := testsupport.StartMongo(t)
	a := connectedAdapter(t, fixture)
	path := nodePath(fixture, seg("database", testsupport.MongoDatabase), seg("collection", "slow_probe"))

	root := rootClient(t, fixture)
	if _, err := root.Database(testsupport.MongoDatabase).Collection("slow_probe").InsertOne(context.Background(), bson.D{{Key: "n", Value: 1}}); err != nil {
		t.Fatalf("seed slow_probe: %v", err)
	}

	const opID = "op-cancel-25"
	filter := slowFilter()
	errCh := make(chan error, 1)
	go func() {
		_, err := a.Read(context.Background(), adapters.ReadRequest{
			Path: path, Filter: &filter, PageSize: 10, Cursor: model.PageCursor{Mode: "offset", Offset: 0},
		}, adapters.NewOpCtx(opID))
		errCh <- err
	}()

	deadline := time.Now().Add(20 * time.Second)
	var seen bool
	for time.Now().Before(deadline) {
		var ops []bson.D
		cursor, err := root.Database("admin").Aggregate(context.Background(), mongodriver.Pipeline{
			{{Key: "$currentOp", Value: bson.D{{Key: "allUsers", Value: true}, {Key: "idleConnections", Value: false}}}},
			{{Key: "$match", Value: bson.D{{Key: "command.comment", Value: opID}}}},
		})
		if err == nil {
			_ = cursor.All(context.Background(), &ops)
			cursor.Close(context.Background())
		}
		if len(ops) > 0 {
			seen = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !seen {
		t.Fatal("slow op never appeared in $currentOp")
	}

	killed, err := a.Cancel(context.Background(), opID)
	if err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if !killed {
		t.Fatal("Cancel returned false, want true")
	}

	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("Read: want an error (server killed the op), got nil")
		}
		code, _ := adapters.CodeOf(err)
		if code != adapters.CodeQuery {
			t.Errorf("code = %v, want E_QUERY (server-side kill, not a local cancel)", code)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("Read never returned after Cancel")
	}

	// The second poll is the assertion: the op must actually be gone, not merely killOp-acked.
	deadline = time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		var ops []bson.D
		cursor, err := root.Database("admin").Aggregate(context.Background(), mongodriver.Pipeline{
			{{Key: "$currentOp", Value: bson.D{{Key: "allUsers", Value: true}, {Key: "idleConnections", Value: false}}}},
			{{Key: "$match", Value: bson.D{{Key: "command.comment", Value: opID}}}},
		})
		if err == nil {
			_ = cursor.All(context.Background(), &ops)
			cursor.Close(context.Background())
		}
		if len(ops) == 0 {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatal("slow op still visible in $currentOp after being killed")
}

// F4: a delete with no filter argument must refuse like mongosh, never widen to {}.
func TestMongo_Console_DeleteRequiresFilter(t *testing.T) {
	fixture := testsupport.StartMongo(t)
	a := connectedAdapter(t, fixture)
	path := nodePath(fixture, seg("database", testsupport.MongoDatabase))
	exec := func(stmt string) ([]page.Page, error) {
		return a.Execute(context.Background(), model.ConsoleRequest{Path: path, Statements: []string{stmt}}, adapters.NewOpCtx("op-del-filter"))
	}
	if _, err := exec(`db.del_filter_probe.insertMany([{n: 1}, {n: 2}])`); err != nil {
		t.Fatalf("seed: %v", err)
	}
	for _, stmt := range []string{`db.del_filter_probe.deleteMany()`, `db.del_filter_probe.deleteOne()`} {
		_, err := exec(stmt)
		if code, _ := adapters.CodeOf(err); code != adapters.CodeQuery {
			t.Fatalf("%s: err = %v, want E_QUERY", stmt, err)
		}
	}
	pages, err := exec(`db.del_filter_probe.countDocuments({})`)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	body := docBodyAt(t, pages[0].(page.DocumentPage), 0)
	if body == nil || !strings.Contains(*body, `"count":{"$numberLong":"2"}`) {
		t.Fatalf("count after refused deletes = %v, want 2", derefStr(body))
	}
	if _, err := exec(`db.del_filter_probe.deleteMany({})`); err != nil {
		t.Fatalf("explicit {} deleteMany: %v", err)
	}
}

// F3: a Stop issued while no command is executing server-side (response withheld here, as between
// getMore batches) must still end the detached op: killOp alone cannot, so the op's own ctx is
// cancelled and Disconnect's drain returns at once instead of waiting out its deadline.
func TestMongo_Cancel_StopsDetachedOpBetweenCommands(t *testing.T) {
	fixture := testsupport.StartMongo(t)
	proxy := testsupport.StartPausableProxy(t, fixture.Config)
	a := newAdapter(t)
	if _, err := a.Connect(context.Background(), proxy.Config(), adapters.NewOpCtx("stop-connect")); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { proxy.Resume(); _ = a.Disconnect(context.Background()) })
	path := nodePath(fixture, seg("database", testsupport.MongoDatabase))

	proxy.Pause()
	opCtx, cancelOp := context.WithCancel(context.Background())
	errc := make(chan error, 1)
	go func() {
		_, err := a.Execute(opCtx, model.ConsoleRequest{Path: path, Statements: []string{`db.widgets.find()`}}, adapters.NewOpCtx("stop-op"))
		errc <- err
	}()
	time.Sleep(500 * time.Millisecond)
	cancelOp()
	cancelCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := a.Cancel(cancelCtx, "stop-op"); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if err := <-errc; err == nil {
		t.Fatal("Execute: want E_CANCELLED, got nil")
	}

	drainCtx, cancelDrain := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelDrain()
	start := time.Now()
	_ = a.Disconnect(drainCtx)
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("Disconnect took %v: the cancelled op is still running detached", elapsed)
	}
}

func TestMongo_Connect_CancelledCtxReturns(t *testing.T) {
	fixture := testsupport.StartMongo(t)
	proxy := testsupport.StartPausableProxy(t, fixture.Config)
	testsupport.ConnectCancelScenario(t, newAdapter(t), proxy.Config(), proxy)
}

// DB MCP read lane: mongo has no read-only session, so the lane relies on Kira's own method check.
func TestMongo_ReadLane(t *testing.T) {
	fixture := testsupport.StartMongo(t)
	a := connectedAdapter(t, fixture)
	path := nodePath(fixture, seg("database", testsupport.MongoDatabase))
	testsupport.RunScenarios(t, a, fixture.Config,
		testsupport.ReadLaneRefusesWrite(testsupport.ReadLaneCase{
			Path: path, Write: `db.lane_probe.insertOne({ n: 1 })`, WantErr: "read-only",
			Unchanged: func(t *testing.T, main adapters.Adapter) {
				t.Helper()
				pages, err := main.Execute(context.Background(), model.ConsoleRequest{
					Path: path, Statements: []string{`db.lane_probe.countDocuments({})`},
				}, adapters.NewOpCtx("lane-verify"))
				if err != nil {
					t.Fatalf("countDocuments: %v", err)
				}
				body := docBodyAt(t, pages[0].(page.DocumentPage), 0)
				if body == nil || !strings.Contains(*body, `"$numberLong":"0"`) {
					t.Errorf("lane_probe count body = %v, want 0", derefStr(body))
				}
			},
		}),
	)
}
