package dbmcpflow

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/connections"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// Backend half of tests/ui/studio/connection-explain-threshold.spec.ts: a connection's own
// explain_threshold_rows replaces the app default in the summary and in list_connections, live, and
// an out-of-range value is a problem that leaves the row alone. Contract connection-explain-threshold.
func TestDbMcpConnectionExplainThreshold(t *testing.T) {
	f := newFixture(t)
	f.setExpensiveThreshold(t, expensiveThreshold)
	create := func(name string) model.ConnectionSummary {
		t.Helper()
		c, err := f.app.W.Connections.Create(connections.Input{ConnectionFields: model.ConnectionFields{
			Name: name, Kind: "postgres", Color: "teal", Mode: "fields", Host: ptr("localhost"), Port: ptr(1234), Options: map[string]any{},
			McpEnabled: true, McpAutoExplain: true, McpReadMode: "allow", McpWriteMode: "prompt", McpDdlMode: "deny",
		}})
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		return c
	}
	own, plain := create("Own"), create("Plain")
	sess := f.session(t)

	summaryRows := func(id string) int {
		t.Helper()
		list, err := f.app.W.Connections.List()
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range list {
			if c.ID == id {
				return c.ExplainThresholdRows
			}
		}
		t.Fatalf("connection %s not listed", id)
		return 0
	}
	listed := func() map[string]int {
		t.Helper()
		out := map[string]int{}
		for _, v := range listViews(t, sess) {
			out[v.Name] = v.ExpensiveQuery.RowsThreshold
		}
		return out
	}
	// The file watcher projects a saved file asynchronously; poll until list_connections agrees.
	waitListed := func(name string, want int) {
		t.Helper()
		var got int
		testx.WaitUntil(t, 10*time.Second, func() bool { got = listed()[name]; return got == want })
		if got != want {
			t.Fatalf("list_connections %s rowsThreshold = %d, want %d", name, got, want)
		}
	}

	if p := f.setExplainThreshold(t, own.ID, 5000); p.Message != "" {
		t.Fatalf("set 5000: %+v", p)
	}
	waitListed("Own", 5000)
	if got := summaryRows(own.ID); got != 5000 {
		t.Errorf("Own summary explainThresholdRows = %d, want 5000", got)
	}
	if got, rows := summaryRows(plain.ID), listed()["Plain"]; got != 0 || rows != expensiveThreshold {
		t.Errorf("Plain summary = %d, list = %d, want 0 and %d", got, rows, expensiveThreshold)
	}
	res, err := sess.CallTool(ctx, &mcp.CallToolParams{Name: "list_connections", Arguments: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	var all any
	_ = json.Unmarshal([]byte(text(res)), &all)
	f.app.Contract(t, "connection-explain-threshold", "list_connections", all, flowharness.Mask("id"))

	// Live edit: the same session sees it, no restart.
	if p := f.setExplainThreshold(t, own.ID, 20_000); p.Message != "" {
		t.Fatalf("set 20000: %+v", p)
	}
	waitListed("Own", 20_000)
	if p := f.setExplainThreshold(t, own.ID, 0); p.Message != "" {
		t.Fatalf("clear: %+v", p)
	}
	waitListed("Own", expensiveThreshold)

	// Out of range: a problem at the key, the row keeps its last good value.
	if p := f.setExplainThreshold(t, own.ID, 20_000); p.Message != "" {
		t.Fatal("restore 20000")
	}
	waitListed("Own", 20_000)
	bad := f.setExplainThreshold(t, own.ID, 10)
	if bad.Line == 0 || !strings.Contains(bad.Message, "explain_threshold_rows must be at least 1000") {
		t.Fatalf("set 10 problem = %+v, want one with a line and the schema bound", bad)
	}
	f.app.Contract(t, "connection-explain-threshold", "problem", bad)
	if got := summaryRows(own.ID); got != 20_000 {
		t.Errorf("Own summary after a bad save = %d, want 20000 kept", got)
	}
}
