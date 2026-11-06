package testsupport

import (
	"context"
	"fmt"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	mongodriver "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
)

// packages/db-fixtures/support/mongo.ts's own constants, ported verbatim (C21) — this Go seeder re-expresses
// that TS function rather than reading a shared fixture file, since Mongo has no .sql-file seeding
// path.
//
// MongoImage stays on 7.0: 8.x/9.x exit at startup on kernels 6.19-7.0.13 (SERVER-125742). Do not bump
// until no runner kernel is in that range.
const (
	MongoImage             = "mongo:7.0.45"
	mongoRootUsername      = "root"
	mongoRootPassword      = FixturePassword
	MongoDatabase          = "kira_test"
	MongoAnalyticsDatabase = "kira_analytics"
	mongoUsername          = "kira"
	mongoPassword          = FixturePassword
	mongoPort              = "27017/tcp"
	mongoStartupTimeout    = 120 * time.Second

	// WidgetCount mirrors 0003_mongo_seed.ts's own WIDGET_COUNT.
	WidgetCount = 25
)

// MongoFixture is support/mongo.ts's MongoFixture.
type MongoFixture struct {
	Config    model.ResolvedConnectionConfig
	Host      string
	Port      int
	RootURI   string // authSource=admin, full cluster access — for test-side assertions only
	container testcontainers.Container
}

// ConnectionID satisfies hasConnectionID (P107 I2-28, FixtureNodePath's own constraint).
func (f MongoFixture) ConnectionID() string { return f.Config.ID }

var mongoMemo = fixture[MongoFixture]{reset: resetMongo}

// StartMongo is support/mongo.ts's startMongo. Skips the test when Docker is unreachable.
func StartMongo(t *testing.T) *MongoFixture {
	return mongoMemo.start(t, "mongo container", startMongo)
}

// StopMongo terminates the memoized container, if one was ever started. Call once, from the test
// binary's own TestMain, after m.Run() returns — never from an individual test.
func StopMongo() {
	mongoMemo.stop(func(f *MongoFixture) { _ = f.container.Terminate(context.Background()) })
}

func startMongo() (*MongoFixture, error) {
	ctx := context.Background()

	req := testcontainers.ContainerRequest{
		Image:        ImageFor("mongo", MongoImage),
		ExposedPorts: []string{mongoPort},
		Env: map[string]string{
			"MONGO_INITDB_ROOT_USERNAME": mongoRootUsername,
			"MONGO_INITDB_ROOT_PASSWORD": mongoRootPassword,
		},
		// With MONGO_INITDB_ROOT_USERNAME set, the entrypoint boots a
		// temporary auth-less instance to create the root user, shuts it down, then starts the
		// real instance with --auth — waiting for only the first "Waiting for connections" gets a
		// refused connection a moment later. The module's own default wait strategy resolves on
		// that throwaway first boot; this is spelled out explicitly instead.
		//
		// Case-insensitive on purpose. 4.4+ logs structured JSON with
		// "msg":"Waiting for connections" (capital W, verified against mongo:7.0.45); 4.2's plain-text
		// logging spells the identical line lowercase — "waiting for connections on port 27017"
		// (verified against mongo:4.2) — and the exact-case match above never saw it, timing out at
		// 0/2 occurrences. A regex match with (?i) covers both without needing two wait strategies.
		WaitingFor: wait.ForLog("(?i)waiting for connections").AsRegexp().WithOccurrence(2).WithStartupTimeout(mongoStartupTimeout),
	}
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		return nil, err
	}

	host, err := container.Host(ctx)
	if err != nil {
		return nil, err
	}
	mappedPort, err := container.MappedPort(ctx, mongoPort)
	if err != nil {
		return nil, err
	}
	port := int(mappedPort.Num())
	rootURI := fmt.Sprintf("mongodb://%s:%s@%s:%d/admin", mongoRootUsername, mongoRootPassword, host, port)

	root, err := mongodriver.Connect(options.Client().ApplyURI(rootURI))
	if err != nil {
		return nil, err
	}
	defer root.Disconnect(context.Background())

	// Scoped to both test databases so the tree-enumeration test sees two non-system databases
	// (listDatabases with a non-admin user only returns databases it is authorized on).
	err = root.Database(MongoDatabase).RunCommand(ctx, bson.D{
		{Key: "createUser", Value: mongoUsername},
		{Key: "pwd", Value: mongoPassword},
		{Key: "roles", Value: bson.A{
			bson.D{{Key: "role", Value: "readWrite"}, {Key: "db", Value: MongoDatabase}},
			bson.D{{Key: "role", Value: "readWrite"}, {Key: "db", Value: MongoAnalyticsDatabase}},
		}},
	}).Err()
	if err != nil {
		return nil, err
	}

	if err := seedMongoDatabases(ctx, root); err != nil {
		return nil, err
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	cfg := model.ResolvedConnectionConfig{
		ID: "test-mongo", SortOrder: 0, CreatedAt: now, UpdatedAt: now,
		Name: "Test Mongo", Kind: "mongodb", Color: "green", Mode: "fields", ReadOnly: false,
		Host: Strp(host), Port: intp(port), Database: Strp(MongoDatabase), Username: Strp(mongoUsername),
		Options: map[string]any{}, Password: Strp(mongoPassword),
	}
	return &MongoFixture{Config: cfg, Host: host, Port: port, RootURI: rootURI, container: container}, nil
}

func seedMongoDatabases(ctx context.Context, root *mongodriver.Client) error {
	if err := seedMongo(ctx, root.Database(MongoDatabase)); err != nil {
		return err
	}
	_, err := root.Database(MongoAnalyticsDatabase).Collection("events").InsertMany(ctx, []any{
		bson.D{{Key: "name", Value: "signup"}},
		bson.D{{Key: "name", Value: "login"}},
	})
	return err
}

// resetMongo drops every collection (not the databases: the kira user's grants stay) and reseeds.
func resetMongo(f *MongoFixture) error {
	ctx := context.Background()
	root, err := mongodriver.Connect(options.Client().ApplyURI(f.RootURI))
	if err != nil {
		return err
	}
	defer root.Disconnect(ctx)
	for _, name := range []string{MongoDatabase, MongoAnalyticsDatabase} {
		db := root.Database(name)
		cols, err := db.ListCollectionNames(ctx, bson.D{})
		if err != nil {
			return err
		}
		for _, c := range cols {
			if err := db.Collection(c).Drop(ctx); err != nil {
				return err
			}
		}
	}
	return seedMongoDatabases(ctx, root)
}

func intp(n int) *int { return &n }

// hexID mirrors 0003_mongo_seed.ts's own hexId.
func hexID(i int) string {
	return fmt.Sprintf("000000000000000000000%03x", i)
}

// bigHexID mirrors 0003_mongo_seed.ts's own bigHexId.
func bigHexID(i int) string {
	return fmt.Sprintf("0000000000000000000%05x", i)
}

func seedMongo(ctx context.Context, db *mongodriver.Database) error {
	widgets := db.Collection("widgets")
	docs := make([]any, WidgetCount)
	for i := 0; i < WidgetCount; i++ {
		id, err := bson.ObjectIDFromHex(hexID(i))
		if err != nil {
			return err
		}
		tags := bson.A{"blue"}
		if i%3 == 0 {
			tags = bson.A{"red", "small"}
		}
		var note any
		if i%5 != 0 {
			note = fmt.Sprintf("note-%d", i)
		}
		docs[i] = bson.D{
			{Key: "_id", Value: id},
			{Key: "name", Value: fmt.Sprintf("widget-%d", i)},
			{Key: "price", Value: float64(i+1) * 1.5},
			{Key: "active", Value: i%2 == 0},
			{Key: "createdAt", Value: time.Date(2024, 1, i+1, 0, 0, 0, 0, time.UTC)},
			{Key: "tags", Value: tags},
			{Key: "meta", Value: bson.D{{Key: "weight", Value: i}, {Key: "note", Value: note}}},
		}
	}
	if _, err := widgets.InsertMany(ctx, docs); err != nil {
		return err
	}
	if _, err := widgets.Indexes().CreateOne(ctx, mongodriver.IndexModel{
		Keys: bson.D{{Key: "name", Value: 1}}, Options: options.Index().SetUnique(true),
	}); err != nil {
		return err
	}

	if _, err := db.Collection("empty_collection").Indexes().CreateOne(ctx, mongodriver.IndexModel{
		Keys: bson.D{{Key: "_id", Value: 1}},
	}); err != nil {
		return err
	}

	// P27 §5/D22: a document well past MaxCellBytes (64 KB) on a multi-row page, so a
	// real read exercises the tree's raw-text fallback rather than a synthetic string.
	oversizedID, err := bson.ObjectIDFromHex(hexID(900))
	if err != nil {
		return err
	}
	oversizedNote := make([]byte, 100_000)
	for i := range oversizedNote {
		oversizedNote[i] = 'x'
	}
	if _, err := db.Collection("oversized_widgets").InsertOne(ctx, bson.D{
		{Key: "_id", Value: oversizedID},
		{Key: "name", Value: "giant-note"},
		{Key: "note", Value: string(oversizedNote)},
	}); err != nil {
		return err
	}

	const bigWidgetCount = 1200
	bigDocs := make([]any, bigWidgetCount)
	for i := 0; i < bigWidgetCount; i++ {
		id, err := bson.ObjectIDFromHex(bigHexID(i))
		if err != nil {
			return err
		}
		bigDocs[i] = bson.D{
			{Key: "_id", Value: id},
			{Key: "seq", Value: i},
			{Key: "label", Value: fmt.Sprintf("big-widget-%d", i)},
		}
	}
	if _, err := db.Collection("big_widgets").InsertMany(ctx, bigDocs); err != nil {
		return err
	}

	// P19 D17: a validated collection so the definition view's Validation section has both a real
	// $jsonSchema (rendered as a field table) and validationLevel/validationAction to show.
	validator := bson.D{{Key: "$jsonSchema", Value: bson.D{
		{Key: "bsonType", Value: "object"},
		{Key: "required", Value: bson.A{"name", "price"}},
		{Key: "properties", Value: bson.D{
			{Key: "name", Value: bson.D{{Key: "bsonType", Value: "string"}, {Key: "description", Value: "must be a string and is required"}}},
			{Key: "price", Value: bson.D{{Key: "bsonType", Value: "number"}, {Key: "minimum", Value: 0}, {Key: "description", Value: "must be a positive number"}}},
		}},
	}}}
	return db.CreateCollection(ctx, "validated_widgets",
		options.CreateCollection().SetValidator(validator).SetValidationLevel("moderate").SetValidationAction("warn"))
}
