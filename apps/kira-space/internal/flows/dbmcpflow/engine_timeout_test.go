package dbmcpflow

import (
	"encoding/json"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters/testsupport"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// rowCountOf reads the rowCount of a run_query result.
func rowCountOf(t *testing.T, res *mcp.CallToolResult) int {
	t.Helper()
	var out struct {
		RowCount int `json:"rowCount"`
	}
	if err := json.Unmarshal([]byte(text(res)), &out); err != nil {
		t.Fatalf("result %q: %v", text(res), err)
	}
	return out.RowCount
}

// wantTimeoutRefusal checks a statement_timeout refusal: the measured value reached the 1 s limit
// (with slack for clock start), it names the exemption field, and the call ended well before the
// statement's own 5 s.
func wantTimeoutRefusal(t *testing.T, res *mcp.CallToolResult, took time.Duration) {
	t.Helper()
	got := text(res)
	m := measuredRe.FindStringSubmatch(got)
	if !res.IsError || m == nil || !strings.Contains(got, "statement_timeout") || !strings.Contains(got, `exemption {"id":"exm_`) {
		t.Fatalf("slow call = %q, want a statement_timeout refusal naming the exemption field", got)
	}
	if measured, _ := strconv.Atoi(m[1]); measured < 900 || m[2] != "1000" {
		t.Errorf("refusal %q: want measured >= 900 against limit 1000", got)
	}
	if took > 4*time.Second {
		t.Errorf("refusal took %v, want the 1 s limit to cut a 5 s statement", took)
	}
}

// TestDbMcpEngineTimeout is the real-engine half of TestDbMcpStatementTimeout*: mcp_statement_timeout_sec 1
// against a 5 s read, write and ddl on every engine, the exemption path, the off switch, browse_table on
// a slow view and the enforcement list_connections reports. Kibana is left out: its timeout is the same
// app timer sqlite, mongodb and redis exercise here, and no Kibana query sleeps on demand.
func TestDbMcpEngineTimeout(t *testing.T) {
	engineTest(t, nil, func(t *testing.T, e engine, f *fixture, sess *mcp.ClientSession) {
		connID := f.engineConn(t, e, func(c *model.ConnectionFields) { c.McpStatementTimeoutSec = 1 })
		scratch := e.scratch("timeout")
		e.setup(t, sess, connID, scratch)
		sub := func(stmt string) string { return e.sql(stmt, scratch) }

		// 1. A slow read is cut at 1 s.
		start := time.Now()
		readRes := e.run(t, sess, connID, sub(e.slowRead), "read")
		wantTimeoutRefusal(t, readRes, time.Since(start))

		// 2. A slow write is refused the same way; engines with a transaction roll the row back.
		start = time.Now()
		writeRes := e.run(t, sess, connID, sub(e.slowWrite), "write")
		wantTimeoutRefusal(t, writeRes, time.Since(start))
		switch e.kind {
		case "postgres", "mysql", "mariadb", "sqlite":
			testx.WaitUntil(t, 10*time.Second, func() bool {
				r := e.run(t, sess, connID, sub(e.read), "read")
				return !r.IsError && rowCountOf(t, r) == 1 && !strings.Contains(text(r), `"z"`)
			})
		}

		// 3. A slow ddl is refused; Postgres also leaves no table.
		if e.slowDDL != "" {
			ddlTable := scratch + "_ddl"
			t.Cleanup(func() {
				sess.CallTool(ctx, &mcp.CallToolParams{Name: "run_query", Arguments: e.args(connID, e.sql(e.drop, ddlTable), "ddl")}) //nolint:errcheck // best-effort cleanup
			})
			start = time.Now()
			ddlRes := e.run(t, sess, connID, e.sql(e.slowDDL, ddlTable), "ddl")
			wantTimeoutRefusal(t, ddlRes, time.Since(start))
			if e.kind == "postgres" {
				if r := e.run(t, sess, connID, "select to_regclass('"+ddlTable+"') is null as gone", "read"); r.IsError || !strings.Contains(text(r), "true") {
					t.Errorf("table after a timed-out CREATE TABLE AS = %q, want it absent", text(r))
				}
			}
		}

		// 4. Exemption: the id of a refused 2 s read, approved, runs past the limit; without it the call is refused again.
		medium := sub(e.medium)
		id := exemptionID(t, e.run(t, sess, connID, medium, "read"))
		exempt := e.runAsync(sess, connID, medium, "read", map[string]any{"exemption": map[string]any{"id": id, "reason": "the report needs the full run"}})
		items := f.items(t, 1)
		if pl := decodeExemption(t, items[0]); len(pl.Limits) != 1 || pl.Limits[0].Limit != "statement_timeout" || pl.Limits[0].Max != 1000 {
			t.Fatalf("payload = %+v", pl)
		}
		f.answerItems(t, true, items[0].ID)
		start = time.Now()
		if res := wait(t, exempt); res.IsError {
			t.Fatalf("exempt call = %q, want it to run", text(res))
		}
		if d := time.Since(start); d < time.Second {
			t.Errorf("exempt call took %v, want a run past the 1 s limit", d)
		}
		if r := e.run(t, sess, connID, medium, "read"); !r.IsError || !strings.Contains(text(r), "statement_timeout") {
			t.Fatalf("same call without exemption = %q, want refused again", text(r))
		}

		// 5. Off: the same 2 s read runs on a twin connection with the limit at 0.
		off := f.engineConn(t, e, func(c *model.ConnectionFields) { c.Name += " no limit"; c.McpStatementTimeoutSec = 0 })
		if r := e.run(t, sess, off, medium, "read"); r.IsError {
			t.Fatalf("read with the limit off = %q, want it to run", text(r))
		}
	})
}

// slowView creates a view whose read takes 5 s on the engines browse_table covers, as admin where the
// connection's own user cannot create views (ClickHouse). It returns the view's browse path.
func slowView(t *testing.T, e engine, sess *mcp.ClientSession, connID string) (string, bool) {
	t.Helper()
	const name = "p602_timeout_view"
	switch e.kind {
	case "postgres":
		stmt := "create view app." + name + " as select pg_sleep(5)::text as s"
		if r := e.run(t, sess, connID, stmt, "ddl"); r.IsError {
			t.Fatalf("create view = %q", text(r))
		}
		t.Cleanup(func() {
			sess.CallTool(ctx, &mcp.CallToolParams{Name: "run_query", Arguments: e.args(connID, "drop view if exists app."+name, "ddl")}) //nolint:errcheck // best-effort cleanup
		})
		return tablePath("kira_test", "app", name), true
	case "mysql", "mariadb":
		stmt := "create view " + name + " as select d.n as s from (select 1 as n union all select 2) d where sleep(5)"
		if r := e.run(t, sess, connID, stmt, "ddl"); r.IsError {
			t.Fatalf("create view = %q", text(r))
		}
		t.Cleanup(func() {
			sess.CallTool(ctx, &mcp.CallToolParams{Name: "run_query", Arguments: e.args(connID, "drop view if exists "+name, "ddl")}) //nolint:errcheck // best-effort cleanup
		})
		return tablePath("kira_test", name), true
	case "clickhouse":
		fx := testsupport.StartClickHouse(t)
		testsupport.AdminStatements(t, fx, "create view kira_test."+name+" as select sum(cityHash64(number)) as s from numbers(1000000000000)")
		t.Cleanup(func() { testsupport.AdminStatements(t, fx, "drop view if exists kira_test."+name) })
		return tablePath("kira_test", name), true
	}
	return "", false
}

// TestDbMcpEngineTimeoutBrowse: browse_table's page read carries the same limit (the adapter Read
// path, not Execute) on the SQL engines whose enforcement is in the session.
func TestDbMcpEngineTimeoutBrowse(t *testing.T) {
	engineTest(t, []string{"postgres", "mysql", "mariadb", "clickhouse"}, func(t *testing.T, e engine, f *fixture, sess *mcp.ClientSession) {
		connID := f.engineConn(t, e, func(c *model.ConnectionFields) { c.McpStatementTimeoutSec = 1 })
		path, ok := slowView(t, e, sess, connID)
		if !ok {
			t.Fatalf("no slow view for %s", e.kind)
		}
		start := time.Now()
		res := browse(t, sess, connID, map[string]any{"path": path})
		if !res.IsError || !strings.Contains(text(res), "statement_timeout") {
			t.Fatalf("browse_table of a slow view = %q, want a statement_timeout refusal", text(res))
		}
		if d := time.Since(start); d > 4*time.Second {
			t.Errorf("browse refusal took %v, want the 1 s limit to cut a 5 s read", d)
		}
	})
}

// TestDbMcpEngineTimeoutEnforcement pins which layer enforces the limit per engine, as
// list_connections reports it (timeoutEnforcementOf).
func TestDbMcpEngineTimeoutEnforcement(t *testing.T) {
	engineTest(t, nil, func(t *testing.T, e engine, f *fixture, sess *mcp.ClientSession) {
		connID := f.engineConn(t, e, func(c *model.ConnectionFields) { c.McpStatementTimeoutSec = 1 })
		var want map[string]string
		if err := json.Unmarshal([]byte(e.enforcement), &want); err != nil {
			t.Fatal(err)
		}
		for _, v := range listedStatementTimeouts(t, sess) {
			if v.ID != connID {
				continue
			}
			if v.StatementTimeout.Seconds != 1 || !reflect.DeepEqual(v.StatementTimeout.Enforcement, want) {
				t.Fatalf("statementTimeout = %+v, want 1 s enforced %v", v.StatementTimeout, want)
			}
			return
		}
		t.Fatal("connection missing from list_connections")
	})
}

type listedTimeout struct {
	ID               string `json:"id"`
	StatementTimeout struct {
		Seconds     int               `json:"seconds"`
		Enforcement map[string]string `json:"enforcement"`
	} `json:"statementTimeout"`
}

func listedStatementTimeouts(t *testing.T, sess *mcp.ClientSession) []listedTimeout {
	t.Helper()
	res, err := sess.CallTool(ctx, &mcp.CallToolParams{Name: "list_connections", Arguments: map[string]any{}})
	if err != nil || res.IsError {
		t.Fatalf("list_connections = %v, %v", res, err)
	}
	var out []listedTimeout
	if err := json.Unmarshal([]byte(text(res)), &out); err != nil {
		t.Fatal(err)
	}
	return out
}
