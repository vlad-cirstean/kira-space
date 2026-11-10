package dbmcpflow

import (
	"database/sql"
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters/testsupport"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/configfs"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// allowEntry is the connection file's mcp.tables entry for the scratch relation: the kind's own
// level count (postgres db.schema.table, mysql family, ClickHouse and MongoDB db.table, SQLite table).
func (e engine) allowEntry(scratch string) string {
	if e.kind == "sqlite" {
		return scratch
	}
	return "kira_test." + scratch
}

// rowsIn counts the scratch relation's rows through MCP, or -1 where the engine has no count to take.
func (e engine) rowsIn(t *testing.T, sess *mcp.ClientSession, connID, scratch string) int {
	t.Helper()
	switch {
	case e.count != "":
		r := e.run(t, sess, connID, e.sql(e.count, scratch), "read")
		var out struct {
			Rows [][]*string `json:"rows"`
		}
		if err := json.Unmarshal([]byte(text(r)), &out); err != nil || len(out.Rows) != 1 || out.Rows[0][0] == nil {
			t.Fatalf("count = %q (%v)", text(r), err)
		}
		n, _ := strconv.Atoi(*out.Rows[0][0])
		return n
	case e.kind == "mongodb":
		return rowCountOf(t, e.run(t, sess, connID, e.sql(e.read, scratch), "read"))
	}
	return -1
}

// wantRows fails when the engine reports a different row count; engines with no count are skipped.
func (e engine) wantRows(t *testing.T, sess *mcp.ClientSession, connID, scratch string, want int, why string) {
	t.Helper()
	if got := e.rowsIn(t, sess, connID, scratch); got >= 0 && got != want {
		t.Fatalf("%s: %d rows in %s, want %d", why, got, scratch, want)
	}
}

// exposure flips the connection file's mcp.enabled, the way the MCP tab does.
func (f *fixture) exposure(t *testing.T, on bool) {
	t.Helper()
	cfg := f.app.W.ConnectionConfig
	file := f.summary(t).File
	detail, err := cfg.File(bridge.ConnectionFileArgs{File: file})
	if err != nil {
		t.Fatal(err)
	}
	from, to := "enabled: true", "enabled: false"
	if on {
		from, to = to, from
	}
	if !strings.Contains(detail.Text, from) {
		t.Fatalf("connection file lacks %q:\n%s", from, detail.Text)
	}
	if saved, err := cfg.SaveFile(bridge.ConnectionSaveFileArgs{File: file, Text: strings.Replace(detail.Text, from, to, 1), BaseHash: detail.Hash}); err != nil || saved.Problem != nil {
		t.Fatalf("SaveFile = %+v (%v)", saved, err)
	}
}

// rootExec runs one statement as the engine's root user (a server setting the connection's user cannot change).
func rootExec(t *testing.T, kind string, cfg model.ResolvedConnectionConfig, database, stmt string) {
	t.Helper()
	dsn := testsupport.RootMysqlDSN
	if kind == "mariadb" {
		dsn = testsupport.RootMariaDSN
	}
	db, err := sql.Open("mysql", dsn(*cfg.Host, *cfg.Port, database))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(stmt); err != nil {
		t.Fatalf("root %q: %v", stmt, err)
	}
}

// TestDbMcpEngineModes is the real-engine half of TestDbMcpMode*: the three permissions and their
// declared-mode tightening, the read lane, a write hidden behind a read, prompt approval, a read-only
// connection, the table allow list, cancel and the lane lifecycle on every engine.
func TestDbMcpEngineModes(t *testing.T) {
	engineTest(t, nil, func(t *testing.T, e engine, f *fixture, sess *mcp.ClientSession) {
		connID := f.engineConn(t, e, nil)
		scratch := e.scratch("modes")
		e.setup(t, sess, connID, scratch)
		sub := func(stmt string) string { return e.sql(stmt, scratch) }

		// 1. Allow all: the seed ran, a read returns it from the read lane.
		if r := e.run(t, sess, connID, sub(e.read), "read"); r.IsError {
			t.Fatalf("read = %q", text(r))
		}
		if !f.app.W.Router.ReadLaneOpen(connID) {
			t.Fatal("no read lane open after a mode=read query")
		}
		e.wantRows(t, sess, connID, scratch, 1, "after the seed write")

		// 2. A statement stronger than its declared mode is refused, and nothing changes.
		if r := e.run(t, sess, connID, sub(e.write2), "read"); !r.IsError {
			t.Fatalf("write declared read = %q, want a refusal", text(r))
		}
		if e.ddl != "" {
			if r := e.run(t, sess, connID, e.sql(e.ddl, scratch+"_x"), "write"); !r.IsError {
				t.Fatalf("ddl declared write = %q, want a refusal", text(r))
			}
		}
		e.wantRows(t, sess, connID, scratch, 1, "after refused tightening")

		// 3. A write hidden behind a read is stopped on the lane. Postgres owns this check in
		// TestDbMcpReadLanePostgres; ClickHouse has no SQL function that writes; MongoDB and Redis refuse
		// in Kira's own method check, covered by step 2.
		if e.hiddenCall != "" {
			// Created as root: the compound function body would trip the MCP path's multi-statement
			// check, and MySQL refuses a data-modifying function from a user without SUPER while the
			// binary log is on.
			cfg := e.config(t)
			rootExec(t, e.kind, cfg, "", "SET GLOBAL log_bin_trust_function_creators = 1")
			for _, stmt := range e.hiddenSetup {
				rootExec(t, e.kind, cfg, "kira_test", sub(stmt))
			}
			t.Cleanup(func() {
				rootExec(t, e.kind, cfg, "kira_test", "drop function if exists "+scratch+"_fn")
				rootExec(t, e.kind, cfg, "kira_test", "drop table if exists "+scratch+"_log")
			})
			if r := e.run(t, sess, connID, sub(e.hiddenCall), "read"); !r.IsError {
				t.Fatalf("hidden write under mode=read = %q, want the engine's refusal", text(r))
			}
			r := e.run(t, sess, connID, "select count(*) as n from "+sub(e.hiddenTable), "read")
			if r.IsError || !strings.Contains(text(r), `"0"`) {
				t.Fatalf("hidden write left rows: %q", text(r))
			}
		}

		// 4. Each permission denied in turn is refused with the mode named.
		modes := []struct{ key, mode, stmt string }{
			{"read", "read", sub(e.read)}, {"write", "write", sub(e.write2)},
		}
		if e.ddl != "" {
			modes = append(modes, struct{ key, mode, stmt string }{"ddl", "ddl", e.sql(e.ddl, scratch+"_y")})
		}
		for _, m := range modes {
			f.mcpSet(t, map[string]string{m.key: "deny"})
			if r := e.run(t, sess, connID, m.stmt, m.mode); !r.IsError || !strings.Contains(text(r), "does not allow "+m.mode) {
				t.Fatalf("%s denied: %q, want a refusal naming the mode", m.mode, text(r))
			}
			f.mcpSet(t, map[string]string{m.key: "allow"})
		}
		e.wantRows(t, sess, connID, scratch, 1, "after denied modes")

		// 5. Prompt: a denied write leaves the data alone, an approved one runs.
		f.mcpSet(t, map[string]string{"write": "prompt"})
		denied := e.runAsync(sess, connID, sub(e.write2), "write", nil)
		req := f.pending(t)
		if req.Class != "write" || req.ConnectionID != connID {
			t.Fatalf("pending approval = %+v, want a write on the connection", req)
		}
		f.answerItems(t, false, req.Item.ID)
		if res := wait(t, denied); !res.IsError {
			t.Fatalf("denied write = %q, want a refusal", text(res))
		}
		e.wantRows(t, sess, connID, scratch, 1, "after a denied prompt")
		approved := e.runAsync(sess, connID, sub(e.write2), "write", nil)
		f.answerItems(t, true, f.pending(t).Item.ID)
		if res := wait(t, approved); res.IsError {
			t.Fatalf("approved write = %q, want it to run", text(res))
		}
		e.wantRows(t, sess, connID, scratch, 2, "after an approved prompt")
		f.mcpSet(t, map[string]string{"write": "allow"})

		// 6. A read-only connection refuses write and ddl whatever the permissions say.
		frozen := f.engineConn(t, e, func(c *model.ConnectionFields) { c.Name += " frozen"; c.ReadOnly = true })
		if r := e.run(t, sess, frozen, sub(e.read), "read"); r.IsError {
			t.Fatalf("read on a read-only connection = %q, want it to run", text(r))
		}
		for _, mode := range []string{"write", "ddl"} {
			if r := e.run(t, sess, frozen, sub(e.write2), mode); !r.IsError || !strings.Contains(text(r), "does not allow "+mode) {
				t.Fatalf("mode=%s on a read-only connection = %q, want a refusal", mode, text(r))
			}
		}
		f.connID = connID

		// 7. Table allow list: only the scratch relation is readable (Redis keys have no list).
		if e.kind != "redis" {
			if err := f.app.W.ConnectionFiles.Patch(connID, configfs.NodeSet{Path: []string{"mcp", "tables"}, Value: []string{e.allowEntry(scratch)}}); err != nil {
				t.Fatalf("Patch mcp.tables: %v", err)
			}
			other := e.sql(e.read, scratch)
			switch {
			case e.kind == "mongodb":
				other = `db.p602_unlisted.find({})`
			case e.customers != "":
				other = "select * from " + e.customers
			}
			if r := e.run(t, sess, connID, sub(e.read), "read"); r.IsError {
				t.Fatalf("listed relation = %q, want it to run", text(r))
			}
			if r := e.run(t, sess, connID, other, "read"); !r.IsError || !strings.Contains(text(r), "not listed") {
				t.Fatalf("unlisted relation %q = %q, want a not-listed refusal", other, text(r))
			}
			if err := f.app.W.ConnectionFiles.Patch(connID, configfs.NodeSet{Path: []string{"mcp"}, Value: map[string]any{"enabled": true}}); err != nil {
				t.Fatalf("Patch remove list: %v", err)
			}
		}

		// 8. Cancel: operations cancel ends a running read quickly and the lane stays usable.
		other := f.session(t)
		pending := e.runAsync(sess, connID, sub(e.slowRead), "read", nil)
		op := waitListed(t, other, func(o listedOp) bool { return o.Type == "op" && o.Kind == "execute" && o.ConnectionID == connID })
		cancelledAt := time.Now()
		if res := cancelOp(t, other, op.OpID); res.IsError || !strings.Contains(text(res), `"cancelled":true`) {
			t.Fatalf("operations cancel = %q", text(res))
		}
		select {
		case out := <-pending:
			if out.res == nil || !out.res.IsError || !strings.Contains(strings.ToLower(text(out.res)), "cancel") {
				t.Fatalf("cancelled read = %+v, want a cancel error", out)
			}
		case <-time.After(20 * time.Second):
			t.Fatal("read still running after operations cancel")
		}
		if d := time.Since(cancelledAt); d > 3*time.Second {
			t.Errorf("cancel took %v to end the read, want under 3 s", d)
		}
		f.waitStatus(t, op.OpID, "cancelled")
		if r := e.run(t, sess, connID, sub(e.read), "read"); r.IsError {
			t.Fatalf("read after cancel = %q, want the lane usable", text(r))
		}

		// 9. Exposure off closes the lane, on again opens a fresh one.
		f.exposure(t, false)
		testx.WaitUntil(t, 10*time.Second, func() bool { return !f.app.W.Router.ReadLaneOpen(connID) })
		f.exposure(t, true)
		if r := e.run(t, sess, connID, sub(e.read), "read"); r.IsError {
			t.Fatalf("read after exposure back on = %q", text(r))
		}
		if !f.app.W.Router.ReadLaneOpen(connID) {
			t.Fatal("no read lane after exposure back on and a read")
		}
	})
}
