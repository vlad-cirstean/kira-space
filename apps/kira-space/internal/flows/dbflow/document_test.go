package dbflow

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	mongodriver "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters/testsupport"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

const documentScenario = "studio-document"

func mongoSecondClient(t *testing.T, fx *testsupport.MongoFixture) *mongodriver.Database {
	t.Helper()
	c, err := mongodriver.Connect(options.Client().ApplyURI(fx.RootURI))
	if err != nil {
		t.Fatalf("mongo second client: %v", err)
	}
	t.Cleanup(func() { _ = c.Disconnect(context.Background()) })
	return c.Database(testsupport.MongoDatabase)
}

// keepDate stops the contract's timestamp mask from rewriting the ISODate inside the shell text; the
// UI half swaps the space back for T.
var keepDate = flowharness.Replace("1960-05-04T03:02:01.000Z", "1960-05-04 03:02:01.000Z")

func docOp(id, body string) model.MutationRowOp {
	return upd(vals("_id", id), vals("$document", body))
}

// J-DOC-010 (+ J-DOC-001, J-DOC-002): editing a document that holds every rich BSON type keeps each
// type, the field order and the untouched fields; an edited _id is refused; a document changed by
// another client since it was loaded is refused until the overwrite is confirmed (decision 3).
func TestDocumentEditKeepsTypesAndGuards(t *testing.T) {
	flowharness.RequireDocker(t)
	fx := testsupport.StartMongo(t)
	second := mongoSecondClient(t, fx)
	ctx := context.Background()
	coll := second.Collection("doc_edit")
	_ = coll.Drop(ctx)

	oid, _ := bson.ObjectIDFromHex("65f000000000000000000001")
	dec, _ := bson.ParseDecimal128("1.10")
	pre1970 := time.Date(1960, 5, 4, 3, 2, 1, 0, time.UTC)
	original := bson.D{
		{Key: "_id", Value: oid},
		{Key: "name", Value: "before"},
		{Key: "big", Value: int64(9007199254740993)},
		{Key: "dec", Value: dec},
		{Key: "when", Value: bson.NewDateTimeFromTime(pre1970)},
		{Key: "bin", Value: bson.Binary{Subtype: 4, Data: []byte("0123456789abcdef")}},
		{Key: "nested", Value: bson.A{bson.A{int32(1), bson.A{int32(2), int32(3)}}, bson.A{}}},
		{Key: "tail", Value: "t"},
	}
	if _, err := coll.InsertOne(ctx, original); err != nil {
		t.Fatal(err)
	}

	app := flowharness.New(t)
	conn := createConn(t, app, inputFromConfig(fx.Config))
	connect(t, app, conn.ID)
	tp := tableNodePath(t, app, conn.ID, "doc_edit")
	ds := app.DataStream(t)

	// Read the document as the editor loads it: id and body text.
	p, _, err := ds.Read(flowharness.ReadArgs{ConnectionID: conn.ID, Path: tp})
	if err != nil {
		t.Fatal(err)
	}
	if p.Kind != "document" || len(p.Rows) != 1 {
		t.Fatalf("read = %s with %d rows", p.Kind, len(p.Rows))
	}
	id, loaded := p.Rows[0][0].(string), p.Rows[0][1].(string)
	app.Contract(t, documentScenario, "read:row", map[string]any{"id": id, "body": loaded}, flowharness.Mask())
	if !strings.Contains(loaded, `"$oid":"65f000000000000000000001"`) {
		t.Fatalf("body %q does not hold the seeded _id", loaded)
	}

	// The editor shows the body as shell text (toShellText) and sends that text back: ObjectId,
	// NumberLong, ISODate wrappers, the rest as read. The UI half asserts it builds this same text.
	loadedShell := shellBody("before")
	edited := shellBody("after")
	guard := func(op model.MutationRowOp) model.MutationRowOp {
		op.Expect = vals("$document", loadedShell)
		return op
	}
	ops := []model.MutationRowOp{guard(docOp(id, edited))}
	app.Contract(t, documentScenario, "mutate:ops", flowharness.MutateWire(conn.ID, tp, ops), contractMask, keepDate)
	if n, err := ds.Mutate(conn.ID, tp, ops); err != nil || n != 1 {
		t.Fatalf("Mutate = %d, %v", n, err)
	}
	var got bson.D
	if err := coll.FindOne(ctx, bson.D{{Key: "_id", Value: oid}}).Decode(&got); err != nil {
		t.Fatal(err)
	}
	want := append(bson.D{}, original...)
	want[1].Value = "after"
	assertBSONEqual(t, "saved document", got, want)

	// An edited _id is refused: nothing changes and no second document appears.
	idEdit := strings.Replace(edited, `ObjectId("65f000000000000000000001")`, `ObjectId("000000000000000000000001")`, 1)
	idEditOps := []model.MutationRowOp{guard(docOp(id, idEdit))}
	app.Contract(t, documentScenario, "mutate:idEdit-ops", flowharness.MutateWire(conn.ID, tp, idEditOps), contractMask, keepDate)
	_, err = ds.Mutate(conn.ID, tp, idEditOps)
	var derr *flowharness.DataError
	if !errors.As(err, &derr) || !strings.Contains(derr.Message, "_id") {
		t.Fatalf("_id edit = %v, want a refusal naming _id", err)
	}
	app.Contract(t, documentScenario, "mutate:idEditError", derr, contractMask, flowharness.Mask("message"))
	if n, _ := coll.CountDocuments(ctx, bson.D{}); n != 1 {
		t.Fatalf("%d documents after the refused _id edit, want 1", n)
	}
	if err := coll.FindOne(ctx, bson.D{{Key: "_id", Value: oid}}).Decode(&got); err != nil {
		t.Fatal(err)
	}
	assertBSONEqual(t, "document after the refused _id edit", got, want)

	// Another client changes the document after it was loaded: the guarded replace is refused.
	if _, err := coll.UpdateOne(ctx, bson.D{{Key: "_id", Value: oid}}, bson.D{{Key: "$set", Value: bson.D{{Key: "tail", Value: "theirs"}}}}); err != nil {
		t.Fatal(err)
	}
	// The loaded body no longer matches: tail changed under the editor.
	mine := shellBody("mine")
	conflictOps := []model.MutationRowOp{guard(docOp(id, mine))}
	app.Contract(t, documentScenario, "mutate:guarded-ops", flowharness.MutateWire(conn.ID, tp, conflictOps), contractMask, keepDate)
	_, err = ds.Mutate(conn.ID, tp, conflictOps)
	if !errors.As(err, &derr) || derr.Code != "E_CONFLICT" {
		t.Fatalf("guarded replace of a changed document = %v, want E_CONFLICT", err)
	}
	app.Contract(t, documentScenario, "mutate:conflict", derr, contractMask, flowharness.Mask("message"))
	if err := coll.FindOne(ctx, bson.D{{Key: "_id", Value: oid}}).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if v, _ := field(got, "tail").(string); v != "theirs" {
		t.Fatalf("tail = %q after the refused replace, want the other client's value", v)
	}
	if v, _ := field(got, "name").(string); v != "after" {
		t.Fatalf("name = %q after the refused replace, want it unchanged", v)
	}
	// Confirmed overwrite: the same replace without the guard lands.
	app.Contract(t, documentScenario, "mutate:overwrite-ops", flowharness.MutateWire(conn.ID, tp, []model.MutationRowOp{docOp(id, mine)}), contractMask, keepDate)
	if _, err := ds.Mutate(conn.ID, tp, []model.MutationRowOp{docOp(id, mine)}); err != nil {
		t.Fatal(err)
	}
	if err := coll.FindOne(ctx, bson.D{{Key: "_id", Value: oid}}).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if v, _ := field(got, "name").(string); v != "mine" {
		t.Fatalf("name = %q after the confirmed overwrite", v)
	}

	// Insert a new document once, then delete it.
	if n, err := ds.Mutate(conn.ID, tp, []model.MutationRowOp{ins(vals("$document", `{"label": "fresh"}`))}); err != nil || n != 1 {
		t.Fatalf("insert = %d, %v", n, err)
	}
	if n, _ := coll.CountDocuments(ctx, bson.D{{Key: "label", Value: "fresh"}}); n != 1 {
		t.Fatalf("%d inserted documents, want 1", n)
	}
	var fresh bson.D
	_ = coll.FindOne(ctx, bson.D{{Key: "label", Value: "fresh"}}).Decode(&fresh)
	freshID := fresh[0].Value.(bson.ObjectID).Hex()
	if n, err := ds.Mutate(conn.ID, tp, []model.MutationRowOp{del(vals("_id", `{"$oid":"`+freshID+`"}`))}); err != nil || n != 1 {
		t.Fatalf("delete = %d, %v", n, err)
	}
	if n, _ := coll.CountDocuments(ctx, bson.D{{Key: "label", Value: "fresh"}}); n != 0 {
		t.Fatalf("deleted document still present")
	}
}

// shellBody is toShellText of the seeded document with its name replaced.
func shellBody(name string) string {
	return `{
  "_id": ObjectId("65f000000000000000000001"),
  "name": "` + name + `",
  "big": NumberLong("9007199254740993"),
  "dec": NumberDecimal("1.10"),
  "when": ISODate("1960-05-04T03:02:01.000Z"),
  "bin": {"$binary":{"base64":"MDEyMzQ1Njc4OWFiY2RlZg==","subType":"04"}},
  "nested": [
    [
      NumberInt(1),
      [
        NumberInt(2),
        NumberInt(3)
      ]
    ],
    []
  ],
  "tail": "t"
}`
}

func assertBSONEqual(t *testing.T, label string, got, want bson.D) {
	t.Helper()
	gb, _ := bson.Marshal(got)
	wb, _ := bson.Marshal(want)
	if !bytes.Equal(gb, wb) {
		t.Fatalf("%s differs (types, values or field order):\n got %v\nwant %v", label, got, want)
	}
}

func field(d bson.D, name string) any {
	for _, e := range d {
		if e.Key == name {
			return e.Value
		}
	}
	return nil
}

// OQ-DOC-3 and J-DOC-011: a projected read carries only the projected fields, so what the grid
// copies cannot leak a hidden field, and a document of about 4 MiB reads in full and still saves.
func TestDocumentProjectionAndLargeDocument(t *testing.T) {
	flowharness.RequireDocker(t)
	fx := testsupport.StartMongo(t)
	second := mongoSecondClient(t, fx)
	ctx := context.Background()
	coll := second.Collection("doc_projection")
	_ = coll.Drop(ctx)
	huge := strings.Repeat("x", 4<<20)
	if _, err := coll.InsertMany(ctx, []any{
		bson.D{{Key: "_id", Value: int32(1)}, {Key: "a", Value: "shown"}, {Key: "secret", Value: "hidden"}, {Key: "nested", Value: bson.D{{Key: "k", Value: "v"}, {Key: "hid", Value: "den"}}}},
		bson.D{{Key: "_id", Value: int32(2)}, {Key: "a", Value: "big"}, {Key: "payload", Value: huge}},
	}); err != nil {
		t.Fatal(err)
	}
	app := flowharness.New(t)
	conn := createConn(t, app, inputFromConfig(fx.Config))
	connect(t, app, conn.ID)
	tp := tableNodePath(t, app, conn.ID, "doc_projection")
	ds := app.DataStream(t)

	p, _, err := ds.Read(flowharness.ReadArgs{ConnectionID: conn.ID, Path: tp, Projection: []string{"a", "nested.k"}})
	if err != nil {
		t.Fatal(err)
	}
	app.Contract(t, documentScenario, "read:projected", map[string]any{"rows": p.Rows}, contractMask)
	for _, row := range p.Rows {
		body := row[1].(string)
		for _, hidden := range []string{"secret", "hidden", "payload", `"hid"`} {
			if strings.Contains(body, hidden) {
				t.Fatalf("projected body %.80q leaks %q", body, hidden)
			}
		}
	}
	if !strings.Contains(p.Rows[0][1].(string), `"a"`) || !strings.Contains(p.Rows[0][1].(string), `"k"`) {
		t.Fatalf("projected body %q lost a projected field", p.Rows[0][1])
	}

	full, _, err := ds.Read(flowharness.ReadArgs{ConnectionID: conn.ID, Path: tp})
	if err != nil {
		t.Fatal(err)
	}
	// The read clips the 4 MiB body to the 64 KiB cell cap and says so; the UI refuses to edit a
	// clipped body (useDocumentGate), because saving a prefix would replace the document with it.
	found := false
	for i, row := range full.Rows {
		if !strings.Contains(row[1].(string), `"big"`) {
			continue
		}
		found = true
		if i >= len(full.Truncated) || !full.Truncated[i] {
			t.Fatalf("a 4 MiB document read is not marked truncated")
		}
		if n := len(row[1].(string)); n > 64<<10 || n < 32<<10 {
			t.Fatalf("clipped body is %d bytes, want at most the 64 KiB cap", n)
		}
	}
	if !found {
		t.Fatal("large document missing from the read")
	}
	if n, _ := coll.CountDocuments(ctx, bson.D{{Key: "payload", Value: huge}}); n != 1 {
		t.Fatalf("large document no longer intact on the server")
	}
}
