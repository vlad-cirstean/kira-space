package dbflow

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters/testsupport"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
)

const consoleMongoScenario = "studio-console-mongo"

// keepConsoleDate stops the contract's timestamp mask from rewriting dates inside the copy text;
// the UI half swaps the space back for T.
var keepConsoleDate = flowharness.Replace("2001-02-03T04:05:06.000Z", "2001-02-03 04:05:06.000Z")

// The two seeded documents as the console returns them (canonical extended JSON) and as each Copy
// all format must spell them. Written from the format definitions (Extended JSON v2 relaxed and
// canonical, mongosh shell literals, plain JSON), never from the UI's own output.
const (
	consoleDoc1 = `{"_id":{"$oid":"65f000000000000000000001"},"status":"open","big":{"$numberLong":"9007199254740993"},"dec":{"$numberDecimal":"1.10"},"when":{"$date":{"$numberLong":"981173106000"}},"bin":{"$binary":{"base64":"MDEyMzQ1Njc4OWFiY2RlZg==","subType":"04"}},"nested":[[{"$numberInt":"1"},[{"$numberInt":"2"}]],[]],"none":null}`
	consoleDoc2 = `{"_id":{"$oid":"65f000000000000000000002"},"status":"done","a.b":"dot"}`

	relaxedDoc1 = `{"_id":{"$oid":"65f000000000000000000001"},"status":"open","big":{"$numberLong":"9007199254740993"},"dec":{"$numberDecimal":"1.10"},"when":{"$date":"2001-02-03T04:05:06.000Z"},"bin":{"$binary":{"base64":"MDEyMzQ1Njc4OWFiY2RlZg==","subType":"04"}},"nested":[[1,[2]],[]],"none":null}`

	// Plain JSON: ObjectId and Date become strings, numeric wrappers numbers (the Int64 above 2^53
	// loses its last digit, the documented cost of a bare number), Binary keeps its object with the `$` dropped.
	plainDoc1 = `{"_id":"65f000000000000000000001","status":"open","big":9007199254740992,"dec":1.1,"when":"2001-02-03T04:05:06.000Z","bin":{"binary":{"base64":"MDEyMzQ1Njc4OWFiY2RlZg==","subType":"04"}},"nested":[[1,[2]],[]],"none":null}`
	plainDoc2 = `{"_id":"65f000000000000000000002","status":"done","a.b":"dot"}`

	// Shell mode, one compact document per line. Six types spell as mongosh literals (ObjectId,
	// NumberInt, NumberLong, NumberDecimal, ISODate, and so on); every other type, Binary here, stays
	// its canonical extended-JSON object so the text never loses a value.
	shellAll = `{"_id":ObjectId("65f000000000000000000001"),"status":"open","big":NumberLong("9007199254740993"),"dec":NumberDecimal("1.10"),"when":ISODate("2001-02-03T04:05:06.000Z"),"bin":{"$binary":{"base64":"MDEyMzQ1Njc4OWFiY2RlZg==","subType":"04"}},"nested":[[NumberInt(1),[NumberInt(2)]],[]],"none":null}
{"_id":ObjectId("65f000000000000000000002"),"status":"done","a.b":"dot"}`
)

func indented(t *testing.T, compact string) string {
	t.Helper()
	var b bytes.Buffer
	if err := json.Indent(&b, []byte(compact), "", "  "); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

// jsonArray is the Copy all array shape: every document pretty-printed and indented one level.
func jsonArray(t *testing.T, docs ...string) string {
	t.Helper()
	items := make([]string, len(docs))
	for i, d := range docs {
		items[i] = "  " + strings.ReplaceAll(indented(t, d), "\n", "\n  ")
	}
	return "[\n" + strings.Join(items, ",\n") + "\n]"
}

// J-CONS-024 (+ J-CONS-002 Mongo half, contract studio-console-mongo): statements through the
// console return documents as canonical extended JSON; the four Copy all formats of that page are
// recorded here for the UI half to match byte for byte.
func TestConsoleMongo(t *testing.T) {
	flowharness.RequireDocker(t)
	fx := testsupport.StartMongo(t)
	second := mongoSecondClient(t, fx)
	ctx := context.Background()
	coll := second.Collection("cons_docs")
	_ = coll.Drop(ctx)
	oid1, _ := bson.ObjectIDFromHex("65f000000000000000000001")
	oid2, _ := bson.ObjectIDFromHex("65f000000000000000000002")
	dec, _ := bson.ParseDecimal128("1.10")
	if _, err := coll.InsertMany(ctx, []any{
		bson.D{{Key: "_id", Value: oid1}, {Key: "status", Value: "open"}, {Key: "big", Value: int64(9007199254740993)}, {Key: "dec", Value: dec},
			{Key: "when", Value: bson.NewDateTimeFromTime(time.Date(2001, 2, 3, 4, 5, 6, 0, time.UTC))},
			{Key: "bin", Value: bson.Binary{Subtype: 4, Data: []byte("0123456789abcdef")}},
			{Key: "nested", Value: bson.A{bson.A{int32(1), bson.A{int32(2)}}, bson.A{}}}, {Key: "none", Value: nil}},
		bson.D{{Key: "_id", Value: oid2}, {Key: "status", Value: "done"}, {Key: "a.b", Value: "dot"}},
	}); err != nil {
		t.Fatal(err)
	}

	app := flowharness.New(t)
	conn := createConn(t, app, inputFromConfig(fx.Config))
	connect(t, app, conn.ID)
	tp := tableNodePath(t, app, conn.ID, "cons_docs")
	ds := app.DataStream(t)

	find, err := ds.Execute(conn.ID, tp, `db.cons_docs.find({})`)
	if err != nil || len(find) != 1 || find[0].Kind != "document" || len(find[0].Rows) != 2 {
		t.Fatalf("find = %+v (%v), want one document page of 2", find, err)
	}
	ids, bodies := make([]string, 2), make([]string, 2)
	for i, r := range find[0].Rows {
		ids[i], bodies[i] = r[0].(string), r[1].(string)
	}
	if bodies[0] != consoleDoc1 || bodies[1] != consoleDoc2 {
		t.Fatalf("bodies:\n%s\n%s", bodies[0], bodies[1])
	}
	app.Contract(t, consoleMongoScenario, "page", map[string]any{"statement": `db.cons_docs.find({})`, "ids": ids, "bodies": bodies})
	app.Contract(t, consoleMongoScenario, "copy", map[string]string{
		"canonical": jsonArray(t, consoleDoc1, consoleDoc2),
		"relaxed":   jsonArray(t, relaxedDoc1, consoleDoc2),
		"plain":     jsonArray(t, plainDoc1, plainDoc2),
		"shell":     shellAll,
	}, keepConsoleDate)

	// Aggregation: a $sum of the literal 1 comes back as a double (the console parses numbers as JS
	// doubles; follow-up F-A14), and the `a.b` key keeps its dot.
	agg, err := ds.Execute(conn.ID, tp, `db.cons_docs.aggregate([{$group: {_id: "$status", n: {$sum: 1}}}, {$sort: {_id: 1}}])`)
	if err != nil || len(agg[0].Rows) != 2 || agg[0].Rows[0][1] != `{"_id":"done","n":{"$numberDouble":"1.0"}}` {
		t.Fatalf("aggregate = %+v (%v)", agg, err)
	}

	// A missing collection is an empty page, not an error.
	none, err := ds.Execute(conn.ID, tp, `db.nope.find({})`)
	if err != nil || none[0].Kind != "document" || len(none[0].Rows) != 0 {
		t.Fatalf("missing collection = %+v (%v), want an empty document page", none, err)
	}

	// A write reports the engine's counts and a second client sees it; a cursor chain and an
	// unterminated statement are refused with a position.
	upd, err := ds.Execute(conn.ID, tp, `db.cons_docs.updateMany({status: "open"}, {$set: {flag: true}})`)
	if err != nil || upd[0].Rows[0][1] != `{"matchedCount":{"$numberLong":"1"},"modifiedCount":{"$numberLong":"1"}}` {
		t.Fatalf("updateMany = %+v (%v)", upd, err)
	}
	n, err := coll.CountDocuments(ctx, bson.D{{Key: "flag", Value: true}})
	if err != nil || n != 1 {
		t.Fatalf("flagged documents = %d (%v), want 1", n, err)
	}
	for key, stmt := range map[string]string{"chain": `db.cons_docs.find({}).sort({_id: 1})`, "unterminated": `db.cons_docs.find({status: "open"`} {
		_, err := ds.Execute(conn.ID, tp, stmt)
		var de *flowharness.DataError
		if !errors.As(err, &de) || de.Code != "E_QUERY" {
			t.Fatalf("%s: %v, want E_QUERY", key, err)
		}
		app.Contract(t, consoleMongoScenario, "refused#"+key, map[string]string{"code": de.Code, "message": de.Message})
	}
}
