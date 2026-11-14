package dbflow

import (
	"errors"
	"path/filepath"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters/testsupport"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

const readonlyScenario = "studio-readonly"

// refusal is the wire error a read-only connection answers a write with.
func refusal(t *testing.T, label string, err error) map[string]string {
	t.Helper()
	var de *flowharness.DataError
	if !errors.As(err, &de) {
		t.Fatalf("%s: want a refusal, got %v", label, err)
	}
	return map[string]string{"code": de.Code, "message": de.Message}
}

// J-CONN-006 (+ J-GRID-037, J-DOC-012; contract studio-readonly): a read-only connection refuses a
// grid mutation and a console write in the backend, so no client but the UI's greyed buttons stands
// between a write and the engine. A second client sees the rows unchanged.
func TestReadOnlySQLiteRefusesWrites(t *testing.T) {
	app := flowharness.New(t)
	path := filepath.Join(t.TempDir(), "ro.db")
	sqliteExec(t, path, `create table notes (id integer primary key, body text)`, `insert into notes values (1,'one'),(2,'two')`)
	in := sqliteInput("ro", path)
	in.ReadOnly = true
	conn := createConn(t, app, in)
	connect(t, app, conn.ID)
	tp := tableNodePath(t, app, conn.ID, "notes")
	ds := app.DataStream(t)
	want := [][]any{{"1", "one"}, {"2", "two"}}

	ops := []model.MutationRowOp{upd(vals("id", "1"), vals("body", "changed")), ins(vals("id", "3", "body", "new")), del(vals("id", "2"))}
	_, err := ds.Mutate(conn.ID, tp, ops)
	app.Contract(t, readonlyScenario, "sqlite#mutate", refusal(t, "Mutate", err))
	wantRows(t, "rows after the refused mutate", sqliteRows(t, path, `select id, body from notes order by id`), want)

	for key, stmt := range map[string]string{
		"insert": `insert into notes values (9,'x')`,
		"drop":   `drop table notes`,
	} {
		_, err := ds.Execute(conn.ID, tp, stmt)
		r := refusal(t, stmt, err)
		r["statement"] = stmt
		app.Contract(t, readonlyScenario, "sqlite#console#"+key, r)
	}
	wantRows(t, "rows after the refused console writes", sqliteRows(t, path, `select id, body from notes order by id`), want)

	pages, err := ds.Execute(conn.ID, tp, `select body from notes order by id`)
	if err != nil || len(pages) != 1 || len(pages[0].Rows) != 2 {
		t.Fatalf("a read on a read-only connection = %+v (%v)", pages, err)
	}
}

func TestReadOnlyMongoRefusesEdit(t *testing.T) {
	flowharness.RequireDocker(t)
	fx := testsupport.StartMongo(t)
	coll := mongoSecondClient(t, fx).Collection("ro_docs")
	_ = coll.Drop(ctx)
	oid, _ := bson.ObjectIDFromHex("65f0000000000000000000a1")
	if _, err := coll.InsertOne(ctx, bson.D{{Key: "_id", Value: oid}, {Key: "status", Value: "open"}}); err != nil {
		t.Fatal(err)
	}
	app := flowharness.New(t)
	in := inputFromConfig(fx.Config)
	in.ReadOnly = true
	conn := createConn(t, app, in)
	connect(t, app, conn.ID)
	tp := tableNodePath(t, app, conn.ID, "ro_docs")
	ds := app.DataStream(t)

	_, err := ds.Mutate(conn.ID, tp, []model.MutationRowOp{docOp(`{"$oid":"65f0000000000000000000a1"}`, `{ _id: ObjectId("65f0000000000000000000a1"), status: "closed" }`)})
	app.Contract(t, readonlyScenario, "mongo#mutate", refusal(t, "Mutate", err))
	_, err = ds.Execute(conn.ID, tp, `db.ro_docs.updateMany({}, {$set: {status: "closed"}})`)
	app.Contract(t, readonlyScenario, "mongo#console", refusal(t, "console write", err))
	var got bson.M
	if err := coll.FindOne(ctx, bson.D{{Key: "_id", Value: oid}}).Decode(&got); err != nil || got["status"] != "open" {
		t.Fatalf("document after the refused writes = %v (%v), want status open", got, err)
	}
}

func TestReadOnlyRedisRefusesEdit(t *testing.T) {
	flowharness.RequireDocker(t)
	fx := testsupport.StartRedis(t)
	second := redisClient(t, fx)
	second.Set(ctx, "ro:key", "before", 0)
	app := flowharness.New(t)
	in := inputFromConfig(fx.Config)
	in.ReadOnly = true
	conn := createConn(t, app, in)
	connect(t, app, conn.ID)
	ds := app.DataStream(t)

	_, err := ds.Mutate(conn.ID, "database:db0", []model.MutationRowOp{upd(vals("_key", "ro:key"), vals("$value", "after"))})
	app.Contract(t, readonlyScenario, "redis#mutate", refusal(t, "Mutate", err))
	_, err = ds.Execute(conn.ID, "database:db0", `SET ro:key after`)
	app.Contract(t, readonlyScenario, "redis#console", refusal(t, "console write", err))
	if got, _ := second.Get(ctx, "ro:key").Result(); got != "before" {
		t.Fatalf("value after the refused writes = %q, want before", got)
	}
}
