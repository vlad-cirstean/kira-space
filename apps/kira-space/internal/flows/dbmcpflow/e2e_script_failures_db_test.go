package dbmcpflow

import (
	"database/sql"
	"strings"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/oplog"
	"github.com/kirathecat/kira-studio/internal/approvals"
)

// Backend half of tests/ui/studio/e2e-script-failures-db.spec.ts (contract e2e-script-failures-db): a result
// script that fails at each stage of run_query. The query ran in every case; the agent text equals the
// jsTransform op row's error, nothing is kept, and the engine slots free up for the next call.

const failuresKey = "e2e-script-failures-db"

const (
	ropeScript  = `let s = 'a'; for (let i = 0; i < 29; i++) s = s + s; return s === s + 'x'`
	stackScript = `function f() { return f() + 1 } return f()`
	// bigRows renders more than 8 MiB: 2000 rows of 5000 characters.
	bigRows = "with recursive c(x) as (select 1 union all select x+1 from c) select printf('%.*c', 5000, 'x') as v from c limit 2000"
)

// tableExists reads the fixture file's schema.
func (f *fixture) tableExists(t *testing.T, name string) bool {
	t.Helper()
	db, err := sql.Open("sqlite", f.dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var n int
	if err := db.QueryRow("select count(*) from sqlite_master where name = ?", name).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n == 1
}

// failedOp is the jsTransform row that ran source and failed.
func (f *fixture) failedOp(t *testing.T, source string) oplog.Record {
	t.Helper()
	row := f.scriptOpRow(t, "jsTransform", source)
	if row.Status != "error" || row.Error == nil {
		t.Fatalf("op row for %q = %+v, want an error row", source, row)
	}
	return row
}

func TestE2EScriptFailuresDB(t *testing.T) {
	f := newFixture(t)
	sess := f.session(t)
	f.mcpSet(t, map[string]string{"write": "allow", "ddl": "allow"})
	f.keep(t, externalScope, "word", `"added"`)
	var failures []oplog.Record

	// fails runs one failing call and holds its text to the op row, the kept store and the queue.
	fails := func(t *testing.T, key string, args map[string]any, script string, contains ...string) string {
		t.Helper()
		before := len(f.app.W.Kept.List(externalScope))
		res := callTool(t, sess, "run_query", args)
		msg := text(res)
		if !res.IsError {
			t.Fatalf("%s: result %q, want an error", key, msg)
		}
		for _, w := range append(contains, "The query ran; its result is not returned.") {
			if !strings.Contains(msg, w) {
				t.Errorf("%s: text %q lacks %q", key, msg, w)
			}
		}
		row := f.failedOp(t, script)
		if *row.Error != msg {
			t.Errorf("%s: op row error = %q, want the tool text %q", key, *row.Error, msg)
		}
		if got := len(f.app.W.Kept.List(externalScope)); got != before {
			t.Errorf("%s: kept entries %d -> %d, want none kept", key, before, got)
		}
		if n := f.queueLen(t); n != 0 {
			t.Errorf("%s: queue %d, want none", key, n)
		}
		failures = append(failures, row)
		f.app.Contract(t, failuresKey, "failureText:"+key, msg)
		f.app.Contract(t, failuresKey, "failure:"+key, map[string]any{"kind": row.Kind, "command": script})
		return msg
	}

	t.Run("a read that never ends times out", func(t *testing.T) {
		fails(t, "readTimeout", f.queryScriptArgs(scriptStmt, "read", `kira.storeValue("x", 1); while (true) {}`), `kira.storeValue("x", 1); while (true) {}`, "timeout")
	})
	t.Run("a read that exhausts memory", func(t *testing.T) {
		msg := fails(t, "readMemory", f.queryScriptArgs(scriptStmt, "read", ropeScript), ropeScript, "memory")
		if strings.Contains(msg, "line ") {
			t.Errorf("memory failure %q carries a line, want none", msg)
		}
	})
	t.Run("a read that overflows the stack", func(t *testing.T) {
		fails(t, "readStack", f.queryScriptArgs(scriptStmt, "read", stackScript), stackScript, "stack")
	})
	t.Run("a read whose output is over the cap", func(t *testing.T) {
		const script = `return 'x'.repeat(5 << 20)`
		fails(t, "readOutput", f.queryScriptArgs(scriptStmt, "read", script), script, "Return less")
	})
	t.Run("a result over the script input limit", func(t *testing.T) {
		args := f.queryScriptArgs(bigRows, "read", "return 1")
		args["maxRows"] = 2000
		fails(t, "readInput", args, "return 1", "over the 8 MB script input limit", "Ask for fewer rows or columns.")
	})

	t.Run("a write that bound a kept value keeps its changes", func(t *testing.T) {
		const script = `throw new Error("boom")`
		before := len(f.notesBodies(t))
		fails(t, "writeThrow", f.queryScriptArgs("insert into notes (body) values (?)", "write", script, "word"), script,
			"line 1", "boom", "Its changes stay. Do not rerun it.")
		if got := f.notesBodies(t); len(got) != before+1 || got[before] != "added" {
			t.Fatalf("notes = %q, want exactly one insert", got)
		}
		if again := f.notesBodies(t); len(again) != before+1 {
			t.Fatalf("notes = %q after the agent text, want the insert not repeated", again)
		}
	})
	t.Run("ddl keeps its changes", func(t *testing.T) {
		const script = `kira.storeValue("t", 1); throw new Error("boom ddl")`
		fails(t, "ddlThrow", f.queryScriptArgs("create table scratch (id integer)", "ddl", script), script, "boom ddl", "Its changes stay.")
		if !f.tableExists(t, "scratch") {
			t.Fatal("table scratch missing, want the statement to have run")
		}
	})

	t.Run("an engine error never reaches the script", func(t *testing.T) {
		before := len(f.transformOps(t))
		res := callTool(t, sess, "run_query", f.queryScriptArgs("select * from no_such_table", "read", "return 1"))
		if !res.IsError || !strings.Contains(text(res), "no_such_table") || strings.Contains(text(res), "script failed") {
			t.Fatalf("engine error = %q, want the engine's message", text(res))
		}
		if after := len(f.transformOps(t)); after != before {
			t.Fatalf("script ops %d -> %d, want no jsTransform row", before, after)
		}
	})

	t.Run("a failure under review is held with its raw result readable", func(t *testing.T) {
		const script = `throw new Error("boom held")`
		f.mcpSet(t, map[string]string{"review.read": "review"})
		pending := callToolAsync(sess, "run_query", f.queryScriptArgs(scriptStmt, "read", script))
		req := f.pending(t)
		if req.Stage != "response" {
			t.Fatalf("pending = %+v, want a held result", req)
		}
		held := f.pendingResponse(t, req.RequestID)
		if !held.IsError || !strings.Contains(held.Text, "boom held") || !strings.Contains(held.Raw, `["1","one"],["2","two"]`) || len(held.Keep) != 0 {
			t.Fatalf("held = %+v, want the failure text, the readable rows and nothing to keep", held)
		}
		opts := scriptChainOpts(f)
		f.app.Contract(t, failuresKey, "ApprovalsService.List#failed", []approvals.Item{req.Item}, opts...)
		f.app.Contract(t, failuresKey, "DbMcpService.PendingResponse#failed", held, opts...)
		f.app.Contract(t, failuresKey, "args:ApprovalsService.Answer#failed", approvals.AnswerArgs{IDs: []string{req.Item.ID}, Approve: true})
		f.answer(t, true, req.RequestID)
		res := wait(t, pending)
		if !res.IsError || text(res) != held.Text {
			t.Fatalf("sent = %q, want the held failure text %q", text(res), held.Text)
		}
		failures = append(failures, f.failedOp(t, script))
		f.mcpSet(t, map[string]string{"review.read": "send"})
	})

	t.Run("slots are free after the crashes and timeouts", func(t *testing.T) {
		var wg sync.WaitGroup
		results := make([]*mcp.CallToolResult, 6)
		for i := range results {
			wg.Add(1)
			go func() {
				defer wg.Done()
				res, err := sess.CallTool(ctx, &mcp.CallToolParams{Name: "run_query", Arguments: f.queryScriptArgs("select id, body from notes where id < 3 order by id", "read", pickBodies)})
				if err == nil {
					results[i] = res
				}
			}()
		}
		wg.Wait()
		for i, res := range results {
			if res == nil || res.IsError || text(res) != bodiesEnvelope {
				t.Errorf("concurrent read %d = %v, want %s", i, res, bodiesEnvelope)
			}
		}
	})

	f.app.Contract(t, failuresKey, "OpsService.Recent#failures", failures,
		flowharness.Mask("id", "startedAt", "durationMs", "connectionId"), flowharness.Replace(f.dbPath, "/data/mcp.db"))
}
