package dbmcpflow

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/connections"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// vanish moves the database file away, so a connection that was up cannot answer. Returns the restore.
func (f *fixture) vanish(t *testing.T) (restore func()) {
	t.Helper()
	away := f.dbPath + ".away"
	if err := os.Rename(f.dbPath, away); err != nil {
		t.Fatal(err)
	}
	return func() {
		t.Helper()
		if err := os.Rename(away, f.dbPath); err != nil {
			t.Fatal(err)
		}
	}
}

func (f *fixture) connectConn(t *testing.T, id string) {
	t.Helper()
	if st, err := f.app.W.Connections.Connect(bridge.ConnectionsIDArgs{ID: id}); err != nil || st.Status != "connected" {
		t.Fatalf("Connect %s = %+v (%v), want connected", id, st, err)
	}
}

func (f *fixture) stateOf(t *testing.T, id string) model.ConnectionState {
	t.Helper()
	all, err := f.app.W.Connections.States()
	if err != nil {
		t.Fatal(err)
	}
	for _, st := range all {
		if st.ConnectionID == id {
			return st
		}
	}
	return model.ConnectionState{ConnectionID: id, Status: "disconnected"}
}

// wantUnreachable asserts the call failed on the connection and raised no approval, no window raise.
func (f *fixture) wantUnreachable(t *testing.T, res *mcp.CallToolResult, connID string) {
	t.Helper()
	if !res.IsError || !strings.Contains(text(res), "no approval was asked") || !strings.Contains(text(res), "Nothing ran") {
		t.Fatalf("result = %q, want an unreachable-connection error", text(res))
	}
	if n := f.queueLen(t); n != 0 {
		t.Fatalf("queue = %d, want empty", n)
	}
	if keys := f.app.Focus.Keys(); len(keys) != 0 {
		t.Fatalf("window raised %v, want no raise", keys)
	}
	st := f.stateOf(t, connID)
	if st.Status != "error" || st.Error == nil || !strings.Contains(*st.Error, "Connection lost") {
		t.Fatalf("state = %+v, want error with the lost message", st)
	}
}

// A statement that would prompt returns the connection error and raises no approval when the
// connection does not answer. Contract dbmcp-unreachable: tests/ui/studio/dbmcp-unreachable.spec.ts
// renders the empty queue and the lost state.
func TestDbMcpUnreachableRaisesNoApproval(t *testing.T) {
	f := newFixture(t, flowharness.WithWindowFocus())
	f.app.W.Windows.Add("w1", 0, nil, func() {})
	sess := f.session(t)
	promptConn := f.addConn(t, "prompt all", "sqlite", "prompt", "prompt", "deny", false)
	maskedConn := func() string {
		c, err := f.app.W.Connections.Create(connections.Input{ConnectionFields: model.ConnectionFields{
			Name: "masked prompt", Kind: "sqlite", Color: "teal", Mode: "fields", Database: ptr(f.dbPath), Options: map[string]any{},
			McpEnabled: true, McpReadMode: "deny", McpMaskedMode: "prompt", McpWriteMode: "deny", McpDdlMode: "deny",
		}})
		if err != nil {
			t.Fatal(err)
		}
		return c.ID
	}()
	f.seedPeople(t)

	const write = "insert into notes (body) values ('three')"
	cases := []struct {
		name string
		conn string
		call func() *mcp.CallToolResult
	}{
		{"write", f.connID, func() *mcp.CallToolResult { return runQuery(t, sess, f.connID, write) }},
		{"read on the lane", promptConn, func() *mcp.CallToolResult { return runQuery(t, sess, promptConn, "select * from notes") }},
		{"explain", promptConn, func() *mcp.CallToolResult {
			return call(t, sess, map[string]any{"connectionId": promptConn, "sql": "select * from notes", "path": "", "mode": "read", "explain": true})
		}},
		{"browse_table masked", maskedConn, func() *mcp.CallToolResult {
			return browse(t, sess, maskedConn, map[string]any{})
		}},
	}
	for i, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f.connectConn(t, c.conn)
			restore := f.vanish(t)
			defer restore()
			res := c.call()
			f.wantUnreachable(t, res, c.conn)
			if i == 0 {
				f.app.Contract(t, "dbmcp-unreachable", "ApprovalsService.List", f.list(t))
				ev := f.app.Events.Wait(t, bridge.ChannelConnectionState, func(e flowharness.Event) bool {
					var st model.ConnectionState
					e.Decode(t, &st)
					return st.ConnectionID == c.conn && st.Status == "error"
				}, 10*time.Second)
				f.app.Contract(t, "dbmcp-unreachable", "event:"+ev.Channel+"#lost", ev.Data,
					flowharness.Mask("since"), flowharness.Replace(f.dbPath, "/data/mcp.db"))
			}
		})
	}

	t.Run("exempted call", func(t *testing.T) {
		ef, esess := newExemptionFixture(t)
		const stmt = "select * from notes /* big */"
		id := exemptionID(t, runQuery(t, esess, ef.connID, stmt))
		ef.connectConn(t, ef.connID)
		restore := ef.vanish(t)
		defer restore()
		res := runQueryWithExemption(t, esess, ef.connID, stmt, id)
		if !res.IsError || !strings.Contains(text(res), "no approval was asked") {
			t.Fatalf("exempted call = %q, want an unreachable-connection error", text(res))
		}
		assertNoItem(t, ef)
	})

	t.Run("never connected", func(t *testing.T) {
		nf := newFixture(t)
		nsess := nf.session(t)
		restore := nf.vanish(t)
		defer restore()
		res := runQuery(t, nsess, nf.connID, write)
		if !res.IsError || !strings.Contains(text(res), "connect failed") {
			t.Fatalf("result = %q, want connect failed", text(res))
		}
		assertNoItem(t, nf)
	})

	t.Run("recovers once the server is back", func(t *testing.T) {
		f.connectConn(t, f.connID)
		restore := f.vanish(t)
		f.wantUnreachable(t, runQuery(t, sess, f.connID, write), f.connID)
		restore()
		done := callAsync(sess, f.connID, write)
		req := f.pending(t)
		if req.Stage != "query" || req.Statement != write {
			t.Fatalf("item = %+v, want the write's approval", req)
		}
		f.answer(t, false, req.RequestID)
		wait(t, done)
	})
}
