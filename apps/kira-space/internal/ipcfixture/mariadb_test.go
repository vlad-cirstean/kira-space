package ipcfixture

import (
	"context"
	"errors"
	"os"
	"regexp"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapterhost"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	_ "github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters/mariadb"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters/testsupport"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
)

func TestMain(m *testing.M) {
	testsupport.Prewarm("mariadb", "mysql", "clickhouse", "redis", "sqs", "kafka")
	code := m.Run()
	testsupport.StopConcurrently(
		testsupport.StopMariadb, testsupport.StopMysql, testsupport.StopClickHouse,
		testsupport.StopRedis, testsupport.StopSqs, testsupport.StopKafka,
	)
	os.Exit(code)
}

var mariadbServerVersion = regexp.MustCompile(`^MariaDB \d+\.\d+`)

func fieldsOf(cfg model.ResolvedConnectionConfig) model.ConnectionFields {
	return model.ConnectionFields{
		Name: cfg.Name, Kind: cfg.Kind, Color: cfg.Color, Mode: cfg.Mode, ReadOnly: cfg.ReadOnly,
		Host: cfg.Host, Port: cfg.Port, Database: cfg.Database, Username: cfg.Username, URI: cfg.URI,
		Options: cfg.Options,
	}
}

func nodeNamed(nodes []model.TreeNode, kind, name string) *model.TreeNode {
	for i := range nodes {
		if nodes[i].Kind == kind && nodes[i].Name == name {
			return &nodes[i]
		}
	}
	return nil
}

func strp(s string) *string { return &s }
func intp(i int) *int       { return &i }

// TestFixture_MariaDB is the apparatus plus one pilot adapter, in read mode, proven byte-for-byte
// against the fixture tests/ipc/mariadb/mariadb.fixture.ts already committed by the TypeScript
// generator this package replaces. Nothing is written and nothing is deleted — the strongest
// possible proof of the port. The scenario mirrors tests/ipc/mariadb/mariadb.backend.spec.ts
// exactly, one call site at a time; where a call is deliberately not part of the committed fixture
// (the cache-hit probe calls, the in-flight cancel target), it is made directly against the app
// stack rather than through the Recorder.
func TestFixture_MariaDB(t *testing.T) {
	fixture := testsupport.StartMariadb(t)
	app := NewApp(t)
	cfg := fixture.Config

	app.SeedConnection(t, cfg.ID, fieldsOf(cfg), cfg.Password)
	rec := NewRecorder(app)

	// --- 1/2: connect ------------------------------------------------------------------------
	list := rec.ConnectionsList(t)
	if len(list) != 1 || list[0].ID != cfg.ID {
		t.Fatalf("connections list = %+v, want exactly one row for %s", list, cfg.ID)
	}
	if states := rec.ConnectionsStates(t); len(states) != 0 {
		t.Fatalf("connections states = %+v, want none (nothing connected yet)", states)
	}
	state := rec.ConnectionsConnect(t, cfg.ID)
	if !mariadbServerVersion.MatchString(*state.ServerVersion) {
		t.Fatalf("serverVersion = %q, want to match %s", *state.ServerVersion, mariadbServerVersion)
	}

	// --- 3: tree — database, then table, no schema level -----------------------------------
	root := rec.TreeChildren(t, cfg.ID, "", false)
	if root.Source != "server" {
		t.Fatalf("root children source = %q, want server", root.Source)
	}
	dbNode := nodeNamed(root.Nodes, "database", *cfg.Database)
	if dbNode == nil {
		t.Fatalf("expected a database node named %s in %+v", *cfg.Database, root.Nodes)
	}

	dbChildrenFirst := rec.TreeChildren(t, cfg.ID, dbNode.Path, false)
	if dbChildrenFirst.Source != "server" {
		t.Fatalf("db children (first) source = %q, want server", dbChildrenFirst.Source)
	}
	// The cache-hit transition is not itself part of the committed fixture (packages/db-fixtures/ structurally
	// cannot reach it either, F2) — probed directly against tree.Service, not through the Recorder
	// (which would otherwise append a second, uncommitted treeChildren snapshot).
	dbChildrenSecond, err := app.Tree.Children(context.Background(), cfg.ID, dbNode.Path, false)
	if err != nil {
		t.Fatalf("db children (second): %v", err)
	}
	if dbChildrenSecond.Source != "cache" {
		t.Fatalf("db children (second) source = %q, want cache", dbChildrenSecond.Source)
	}

	orderItemsNode := nodeNamed(dbChildrenFirst.Nodes, "table", "order_items")
	if orderItemsNode == nil {
		t.Fatalf("expected an order_items table node in %+v", dbChildrenFirst.Nodes)
	}
	bigRowsNode := nodeNamed(dbChildrenFirst.Nodes, "table", "big_rows")
	if bigRowsNode == nil {
		t.Fatalf("expected a big_rows table node in %+v", dbChildrenFirst.Nodes)
	}

	// --- 4: first page of order_items --------------------------------------------------------
	readReq := adapterhost.ReadRequestWire{
		OpID: "be-read-order-items", ConnectionID: cfg.ID, Path: orderItemsNode.Path,
		PageSize: 100, Cursor: model.PageCursor{Mode: "offset", Offset: 0},
	}
	readResp := rec.DataRead(t, readReq, nil)
	if readResp.Source != "server" || readResp.Page.Rows() != 3 {
		t.Fatalf("read order_items = source=%s rows=%d, want server/3", readResp.Source, readResp.Page.Rows())
	}

	// --- 5: count, twice (server then cache) -------------------------------------------------
	countReq := adapterhost.CountRequestWire{OpID: "be-count-order-items", ConnectionID: cfg.ID, Path: orderItemsNode.Path}
	countFirst := rec.DataCount(t, countReq)
	if countFirst.Value != 3 || countFirst.Source != "server" {
		t.Fatalf("count order_items (first) = %+v, want value=3 source=server", countFirst)
	}
	countSecond, err := app.Dispatcher.Count(context.Background(), WireCount(countReq))
	if err != nil {
		t.Fatalf("count order_items (second): %v", err)
	}
	if countSecond.Source != "cache" {
		t.Fatalf("count order_items (second) source = %q, want cache", countSecond.Source)
	}

	// --- 6: filtered read (quantity > 1) -----------------------------------------------------
	filteredReq := readReq
	filteredReq.OpID = "be-read-order-items-filtered"
	filteredReq.Filter = strp("quantity > 1")
	filteredResp := rec.DataRead(t, filteredReq, nil)
	if filteredResp.Page.Rows() != 2 {
		t.Fatalf("filtered read rows = %d, want 2", filteredResp.Page.Rows())
	}

	// --- the frontend half's stop-button scenario replays this exact read against big_rows, with
	// an artificial delay standing in for the real SLEEP()-based slow filter (D7) ----------------
	bigRowsReq := adapterhost.ReadRequestWire{
		OpID: "fe-read-big-rows", ConnectionID: cfg.ID, Path: bigRowsNode.Path,
		PageSize: 100, Cursor: model.PageCursor{Mode: "offset", Offset: 0},
	}
	rec.DataRead(t, bigRowsReq, intp(500))

	// --- 7: cancel — a real running op, a real cancelOp -------------------------------------
	slowReq := bigRowsReq
	slowReq.OpID = "be-cancel-target"
	slowReq.Filter = strp("id != 1 OR (SELECT SLEEP(2)) IS NOT NULL")
	inFlight := make(chan error, 1)
	go func() {
		_, err := app.Dispatcher.Read(context.Background(), WireRead(slowReq))
		inFlight <- err
	}()
	time.Sleep(100 * time.Millisecond)
	cancelled := rec.OpsCancel(t, slowReq.OpID)
	if !cancelled {
		t.Fatalf("cancel %s: expected cancelled=true", slowReq.OpID)
	}
	select {
	case err := <-inFlight:
		var ae *adapters.Error
		if !errors.As(err, &ae) || ae.Code != adapters.CodeCancelled {
			t.Fatalf("in-flight read error = %v, want an E_CANCELLED *adapters.Error", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("in-flight read never returned after cancel")
	}

	if maybeWriteFixture(t, rec, "mariadb") {
		return
	}
	assertMatchesCommittedJSONFixture(t, rec, "testdata/mariadb.fixture.json")
}
