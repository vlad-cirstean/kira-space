package ipcfixture

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapterhost"
	_ "github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters/clickhouse"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters/testsupport"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/tree"
)

var clickhouseServerVersion = regexp.MustCompile(`^ClickHouse 2\d\.`)

// TestFixture_ClickHouse is against tests/ipc/clickhouse/clickhouse.fixture.ts's own committed
// scenario: connect, tree (with a frozen `.inner_id.<uuid>` materialized-view backing table and a
// frozen InnoDB-style row-count estimate for the million-row table), a filter-by-value read, a
// definition (frozen generatedAt) and a console/SQL-mode data:execute.
func TestFixture_ClickHouse(t *testing.T) {
	fixture := testsupport.StartClickHouse(t)
	app := NewApp(t)
	cfg := fixture.Config

	app.SeedConnection(t, cfg.ID, fieldsOf(cfg), cfg.Password)
	rec := NewRecorder(app)

	list := rec.ConnectionsList(t)
	if len(list) != 1 || list[0].ID != cfg.ID {
		t.Fatalf("connections list = %+v, want exactly one row for %s", list, cfg.ID)
	}
	if states := rec.ConnectionsStates(t); len(states) != 0 {
		t.Fatalf("connections states = %+v, want none", states)
	}
	state := rec.ConnectionsConnect(t, cfg.ID)
	if !clickhouseServerVersion.MatchString(*state.ServerVersion) {
		t.Fatalf("serverVersion = %q, want to match %s", *state.ServerVersion, clickhouseServerVersion)
	}

	root := rec.TreeChildren(t, cfg.ID, "", false)
	dbNode := nodeByName(root.Nodes, *cfg.Database)
	if dbNode == nil {
		t.Fatalf("expected a database node named %s in %+v", *cfg.Database, root.Nodes)
	}
	for _, n := range root.Nodes {
		if strings.EqualFold(n.Name, "information_schema") {
			t.Fatalf("expected no INFORMATION_SCHEMA node, got %+v", root.Nodes)
		}
	}

	dbChildren := rec.TreeChildren(t, cfg.ID, dbNode.Path, false)
	frozenNodes := FreezeInnerIDNode(dbChildren.Nodes, dbNode.Path)
	frozenNodes = FreezeNodeDetail(frozenNodes, "big_rows", "~1M")
	rec.OverrideLastControlResponse(tree.ChildrenResult{
		Nodes: frozenNodes, Source: dbChildren.Source, Truncated: dbChildren.Truncated,
	})
	orderItemsNode := nodeByName(dbChildren.Nodes, "order_items")
	if orderItemsNode == nil {
		t.Fatalf("expected an order_items table node in %+v", dbChildren.Nodes)
	}
	var hasView, hasMatview bool
	for _, n := range dbChildren.Nodes {
		hasView = hasView || n.Kind == "view"
		hasMatview = hasMatview || n.Kind == "matview"
	}
	if !hasView || !hasMatview {
		t.Fatalf("expected both a view-kind and a matview-kind node, got %+v", dbChildren.Nodes)
	}

	readFirstIDThenFilterToOne(t, rec, cfg.ID, orderItemsNode.Path)

	definitionResult := rec.TreeDefinition(t, cfg.ID, orderItemsNode.Path, false, nil)
	if !regexp.MustCompile(`MergeTree`).MatchString(fmt.Sprint(definitionResult.Definition.Statements)) {
		t.Fatalf("definition statements = %+v, want to mention MergeTree", definitionResult.Definition.Statements)
	}
	rec.OverrideLastControlResponse(tree.DefinitionResult{
		Definition: FreezeDefinition(definitionResult.Definition), Source: definitionResult.Source,
	})

	executeReq := adapterhost.ExecuteRequestWire{
		OpID: "be-console-select1", ConnectionID: cfg.ID, Path: dbNode.Path, Statements: []string{"SELECT 1"},
	}
	executeResp := rec.DataExecute(t, executeReq)
	if len(executeResp.Pages) != 1 {
		t.Fatalf("execute pages = %d, want 1", len(executeResp.Pages))
	}

	if maybeWriteFixture(t, rec, "clickhouse") {
		return
	}
	assertMatchesCommittedJSONFixture(t, rec, "testdata/clickhouse.fixture.json")
}
