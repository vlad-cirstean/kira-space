package dbmcpflow

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/testx"
)

const engineExpensiveThreshold = 100_000

// planRows is the estimatedRowsRead of an explain:true result (the plan's fields sit at the top level).
func planRows(t *testing.T, res *mcp.CallToolResult) (float64, bool) {
	t.Helper()
	var out struct {
		EstimatedRowsRead *float64 `json:"estimatedRowsRead"`
	}
	if err := json.Unmarshal([]byte(text(res)), &out); err != nil {
		t.Fatalf("result %q: %v", text(res), err)
	}
	if out.EstimatedRowsRead == nil {
		return 0, false
	}
	return *out.EstimatedRowsRead, true
}

// TestDbMcpEngineExpensive is the real-engine half of TestDbMcpExpensiveQueryPostgres: the seeded
// 1M-row big_rows against a 100000-row threshold on MySQL, MariaDB and ClickHouse (Postgres owns its own
// contract test). Each engine's EXPLAIN is composed, run on the lane and parsed for real.
func TestDbMcpEngineExpensive(t *testing.T) {
	engineTest(t, []string{"mysql", "mariadb", "clickhouse"}, func(t *testing.T, e engine, f *fixture, sess *mcp.ClientSession) {
		f.setExpensiveThreshold(t, engineExpensiveThreshold)
		connID := f.engineConn(t, e, func(c *model.ConnectionFields) { c.McpAutoExplain = true })
		full := "select * from " + e.big
		point := full + " where id = 1"
		cmds, stop := f.mcpCommands()
		defer stop()
		waitCmds := func(n int) []string {
			t.Helper()
			var got []string
			testx.WaitUntil(t, 10*time.Second, func() bool { got = cmds(); return len(got) >= n })
			return got
		}
		before := len(cmds())

		// 1. Refused before it runs: one op, the EXPLAIN.
		r := e.run(t, sess, connID, full, "read")
		got := text(r)
		m := rowsMeasuredRe.FindStringSubmatch(got)
		if !r.IsError || m == nil || m[2] != strconv.Itoa(engineExpensiveThreshold) || !strings.Contains(got, "expensive_query") || !strings.Contains(got, `exemption {"id":"exm_`) {
			t.Fatalf("big read = %q, want an expensive_query refusal with the exemption path", got)
		}
		if measured, _ := strconv.Atoi(m[1]); measured < engineExpensiveThreshold {
			t.Errorf("refusal %q: want measured >= %d", got, engineExpensiveThreshold)
		}
		if ops := waitCmds(before + 1)[before:]; len(ops) != 1 || !strings.HasPrefix(ops[0], "EXPLAIN") {
			t.Fatalf("ops of the refused call = %v, want exactly one EXPLAIN", ops)
		}

		// 2. A selective read passes and reports its plan.
		r = e.run(t, sess, connID, point, "read")
		if rows, ok := resultPlan(t, r); r.IsError || !ok || rows >= engineExpensiveThreshold {
			t.Fatalf("selective read = %q (plan rows %v), want rows with an estimate under the limit", text(r), rows)
		}

		// 3. Exemption: approved, it runs and the result is capped at maxRows.
		id := exemptionID(t, e.run(t, sess, connID, full, "read"))
		exCall := e.runAsync(sess, connID, full, "read", map[string]any{
			"maxRows": 10, "exemption": map[string]any{"id": id, "reason": "the report needs the whole table"},
		})
		items := f.items(t, 1)
		if pl := decodeExemption(t, items[0]); len(pl.Limits) != 1 || pl.Limits[0].Limit != "expensive_query" || pl.Limits[0].Max != engineExpensiveThreshold || pl.Limits[0].Measured < engineExpensiveThreshold {
			t.Fatalf("payload = %+v", pl)
		}
		f.answerItems(t, true, items[0].ID)
		if res := wait(t, exCall); res.IsError || !strings.Contains(text(res), `"truncated"`) || rowCountOf(t, res) < 10 {
			t.Fatalf("approved exempt call = %q, want rows capped at maxRows", text(res))
		}

		// 4. The connection's own threshold replaces the app default for this connection.
		if p := f.setExplainThreshold(t, connID, 2_000_000); p.Message != "" {
			t.Fatalf("set explain_threshold_rows: %+v", p)
		}
		if r := e.run(t, sess, connID, full, "read"); r.IsError {
			t.Fatalf("big read under a 2000000-row connection threshold = %q, want rows", text(r))
		}
		if p := f.setExplainThreshold(t, connID, 0); p.Message != "" {
			t.Fatalf("clear explain_threshold_rows: %+v", p)
		}
		if r := e.run(t, sess, connID, full, "read"); !r.IsError || !strings.Contains(text(r), "expensive_query") {
			t.Fatalf("big read after clearing the override = %q, want refused at the app default", text(r))
		}

		// 5. explain: true renders the plan with its estimate and runs nothing.
		ex := call(t, sess, mergeArgs(e.args(connID, full, "read"), map[string]any{"explain": true}))
		if rows, ok := planRows(t, ex); ex.IsError || !ok || rows < engineExpensiveThreshold {
			t.Fatalf("explain of the full read = %q (rows %v), want a plan estimating the whole table", text(ex), rows)
		}

		// 6. browse_table runs the same check.
		if r := browse(t, sess, connID, map[string]any{"path": e.bigPath}); !r.IsError || !strings.Contains(text(r), "expensive_query") {
			t.Fatalf("browse_table of big_rows = %q, want an expensive_query refusal", text(r))
		}

		// 7. Check off for the connection: no EXPLAIN op, the statement runs.
		f.setAutoExplain(t, connID, false)
		before = len(cmds())
		if r := e.run(t, sess, connID, full, "read"); r.IsError {
			t.Fatalf("big read with the check off = %q, want rows", text(r))
		}
		for _, c := range waitCmds(before + 1)[before:] {
			if strings.HasPrefix(c, "EXPLAIN") {
				t.Errorf("op %q ran with the check off", c)
			}
		}
	})
}

func mergeArgs(base, extra map[string]any) map[string]any {
	for k, v := range extra {
		base[k] = v
	}
	return base
}

// TestDbMcpEngineNoExplain: MongoDB and Redis have no EXPLAIN this app can parse. explain:true is
// refused, list_connections reports no check, and a read is never refused as expensive.
func TestDbMcpEngineNoExplain(t *testing.T) {
	engineTest(t, []string{"mongodb", "redis"}, func(t *testing.T, e engine, f *fixture, sess *mcp.ClientSession) {
		f.setExpensiveThreshold(t, 1000)
		connID := f.engineConn(t, e, func(c *model.ConnectionFields) { c.McpAutoExplain = true })
		scratch := e.scratch("explain")
		e.setup(t, sess, connID, scratch)

		ex := call(t, sess, mergeArgs(e.args(connID, e.sql(e.read, scratch), "read"), map[string]any{"explain": true}))
		if !ex.IsError || !strings.Contains(text(ex), "has no EXPLAIN this app can parse") {
			t.Fatalf("explain = %q, want the no-EXPLAIN refusal", text(ex))
		}
		if r := e.run(t, sess, connID, e.sql(e.read, scratch), "read"); r.IsError {
			t.Fatalf("read = %q, want it never refused as expensive", text(r))
		}
		for _, v := range listViews(t, sess) {
			if v.ID == connID && v.ExpensiveQuery.Check != "none" {
				t.Errorf("%s expensiveQuery = %+v, want check none", e.kind, v.ExpensiveQuery)
			}
		}
	})
}
