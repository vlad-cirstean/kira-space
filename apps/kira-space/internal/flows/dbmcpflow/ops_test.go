package dbmcpflow

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapterhost"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/connections"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/ids"
	"github.com/kirathecat/kira-studio/internal/scriptruns"
	"github.com/kirathecat/kira-studio/internal/scripts"
	"github.com/kirathecat/kira-studio/internal/terminal"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// Bounded so a broken cancel fails on the test timeout instead of hanging.
const longQuery = `WITH RECURSIVE c(x) AS (SELECT 1 UNION ALL SELECT x+1 FROM c) SELECT count(*) FROM (SELECT x FROM c LIMIT 2000000000)`

type listedOp struct {
	OpID           string `json:"opId"`
	Type           string `json:"type"`
	Kind           string `json:"kind"`
	Source         string `json:"source"`
	ConnectionID   string `json:"connectionId"`
	ConnectionName string `json:"connectionName"`
	Name           string `json:"name"`
	State          string `json:"state"`
}

func listOps(t *testing.T, sess *mcp.ClientSession) []listedOp {
	t.Helper()
	res, err := sess.CallTool(ctx, &mcp.CallToolParams{Name: "list_operations"})
	if err != nil || res.IsError {
		t.Fatalf("list_operations = %q (%v)", text(res), err)
	}
	var out struct {
		Operations []listedOp `json:"operations"`
	}
	if err := json.Unmarshal([]byte(text(res)), &out); err != nil {
		t.Fatalf("list_operations JSON: %v", err)
	}
	return out.Operations
}

func cancelOp(t *testing.T, sess *mcp.ClientSession, id string) *mcp.CallToolResult {
	t.Helper()
	res, err := sess.CallTool(ctx, &mcp.CallToolParams{Name: "cancel_operation", Arguments: map[string]any{"opId": id}})
	if err != nil {
		t.Fatalf("cancel_operation %q: %v", id, err)
	}
	return res
}

func waitListed(t *testing.T, sess *mcp.ClientSession, match func(listedOp) bool) listedOp {
	t.Helper()
	var found listedOp
	testx.WaitUntil(t, 15*time.Second, func() bool {
		for _, o := range listOps(t, sess) {
			if match(o) {
				found = o
				return true
			}
		}
		return false
	})
	return found
}

func (f *fixture) opRow(t *testing.T, id string) (model.DbOpRecord, bool) {
	t.Helper()
	recent, err := f.app.W.StudioOps.Recent(bridge.StudioOpsRecentArgs{Limit: 50})
	if err != nil {
		t.Fatalf("Ops.Recent: %v", err)
	}
	for _, r := range recent {
		if r.ID == id {
			return r, true
		}
	}
	return model.DbOpRecord{}, false
}

func (f *fixture) waitStatus(t *testing.T, id, status string) model.DbOpRecord {
	t.Helper()
	var row model.DbOpRecord
	testx.WaitUntil(t, 10*time.Second, func() bool {
		r, ok := f.opRow(t, id)
		row = r
		return ok && r.Status == status
	})
	return row
}

func TestDbMcpCancelOperation(t *testing.T) {
	f := newFixture(t)
	a, err := f.connect(t, f.token(t))
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	b, err := f.connect(t, f.token(t))
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	pending := callAsync(a, f.connID, longQuery)
	op := waitListed(t, b, func(o listedOp) bool { return o.Type == "op" && o.Kind == "execute" })
	if op.Source != "mcp" || op.ConnectionID != f.connID || op.ConnectionName != "notes db" {
		t.Fatalf("listed op = %+v, want execute from mcp on the fixture connection", op)
	}
	row := f.waitStatus(t, op.OpID, "running")
	// Contract ops-running: tests/ui/running-ops-status.spec.ts renders this running row.
	f.app.Contract(t, "ops-running", "StudioOpsService.Recent", []model.DbOpRecord{row}, flowharness.Mask("id", "startedAt", "durationMs", "connectionId"))

	if res := cancelOp(t, b, op.OpID); res.IsError || !strings.Contains(text(res), `"cancelled":true`) {
		t.Fatalf("cancel_operation = %q", text(res))
	}
	select {
	case out := <-pending:
		if out.err != nil || out.res == nil || !out.res.IsError || !strings.Contains(strings.ToLower(text(out.res)), "cancel") {
			t.Fatalf("cancelled run_query = %+v (%v), want a cancel error", out.res, out.err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("run_query still running after cancel_operation")
	}
	f.waitStatus(t, op.OpID, "cancelled")
	if res := cancelOp(t, b, op.OpID); !res.IsError || !strings.Contains(text(res), "no running operation") {
		t.Fatalf("second cancel_operation = %q, want the no-running-operation error", text(res))
	}

	hidden, err := f.app.W.Connections.Create(connections.Input{ConnectionFields: model.ConnectionFields{
		Name: "hidden db", Kind: "sqlite", Color: "teal", Mode: "fields", Database: ptr(f.dbPath), Options: map[string]any{},
		McpEnabled: false,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.app.W.Connections.Connect(bridge.ConnectionsIDArgs{ID: hidden.ID}); err != nil {
		t.Fatal(err)
	}
	hiddenOp := ids.NewID()
	done := make(chan error, 1)
	go func() {
		_, err := f.app.W.Router.Execute(ctx, adapterhost.ExecuteRequestWire{OpID: hiddenOp, ConnectionID: hidden.ID, Statements: []string{longQuery}})
		done <- err
	}()
	f.waitStatus(t, hiddenOp, "running")
	for _, o := range listOps(t, b) {
		if o.OpID == hiddenOp {
			t.Fatalf("list_operations leaks hidden op: %+v", o)
		}
	}
	if res := cancelOp(t, b, hiddenOp); !res.IsError || !strings.Contains(text(res), "no running operation") {
		t.Fatalf("cancel of hidden op = %q, want the no-running-operation error", text(res))
	}
	if err := f.app.W.StudioOps.Cancel(bridge.StudioOpsCancelArgs{OpID: hiddenOp}); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("hidden op finished without error after the panel cancel")
		}
	case <-time.After(15 * time.Second):
		t.Fatal("hidden op still running after Ops.Cancel")
	}
	f.waitStatus(t, hiddenOp, "cancelled")
}

func TestDbMcpCancelScriptRun(t *testing.T) {
	f := newFixture(t)
	sess, err := f.connect(t, f.token(t))
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()

	rec, err := f.app.W.CustomScripts.Create(bridge.CustomScriptsCreateArgs{Fields: scripts.CustomScriptFields{Name: "job", Color: "blue", Command: "sleep 60", MCP: allowMCP("")}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.app.W.Terminal.Open(terminal.OpenArgs{
		TerminalID: "mcp-run", WindowKey: "w1", Cols: 80, Rows: 24, LaunchKind: terminal.LaunchKindScript, ScriptID: rec.ID,
	}); err != nil {
		t.Fatal(err)
	}
	var runID string
	testx.WaitUntil(t, 20*time.Second, func() bool {
		runs, _ := f.app.W.ScriptRuns.List(scriptruns.ListArgs{})
		for _, r := range runs {
			if r.TerminalID == "mcp-run" && r.State == "running" {
				runID = r.ID
				return true
			}
		}
		return false
	})
	entry := waitListed(t, sess, func(o listedOp) bool { return o.OpID == "run:"+runID })
	if entry.Type != "scriptRun" || entry.State != "running" || entry.Name != "job" {
		t.Fatalf("listed run = %+v", entry)
	}
	if res := cancelOp(t, sess, entry.OpID); res.IsError || !strings.Contains(text(res), `"stopped":true`) {
		t.Fatalf("cancel_operation = %q", text(res))
	}
	testx.WaitUntil(t, 20*time.Second, func() bool {
		r, err := f.app.W.ScriptRuns.Get(scriptruns.IDArgs{ID: runID})
		return err == nil && r.State == "cancelled" && r.Outcome != nil && r.Outcome.Reason == "stopped by you"
	})
	if res := cancelOp(t, sess, entry.OpID); !res.IsError {
		t.Fatalf("second cancel of a finished run = %q, want an error", text(res))
	}
}
