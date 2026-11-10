package dbmcpflow

import (
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/oplog"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/ids"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// The throttle is one paced gate per connection on every engine: with 1 op per second, an MCP lane read
// starts at least ~1 s after the user console op that took the slot. SQLite has its own tests in
// throttle_test.go.
func TestDbMcpEngineThrottle(t *testing.T) {
	engineTest(t, []string{"postgres", "mysql", "mariadb", "clickhouse", "mongodb", "redis"}, func(t *testing.T, e engine, f *fixture, sess *mcp.ClientSession) {
		connID := f.engineConn(t, e, func(c *model.ConnectionFields) { c.ThrottlePerSec = 1 })
		scratch := e.scratch("throttle")
		e.setup(t, sess, connID, scratch)
		if _, err := f.app.W.Connections.Connect(bridge.ConnectionsIDArgs{ID: connID}); err != nil {
			t.Fatal(err)
		}
		marks, stop := f.recordOps()
		defer stop()

		userOp := ids.NewID()
		userDone := make(chan error, 1)
		go func() { userDone <- f.exec(userOp, e.sql(e.read, scratch)) }()
		testx.WaitUntil(t, 15*time.Second, func() bool { return indexOf(marks(), oplog.DbEventOpStart, userOp) >= 0 })
		if r := e.run(t, sess, connID, e.sql(e.read, scratch), "read"); r.IsError {
			t.Fatalf("%s run_query = %q", e.kind, text(r))
		}
		if err := <-userDone; err != nil {
			t.Fatalf("%s user op: %v", e.kind, err)
		}
		// Events reach the recorder after the calls return.
		testx.WaitUntil(t, 10*time.Second, func() bool {
			return indexOf(marks(), oplog.DbEventOpStart, "mcp") >= 0
		})

		got := marks()
		userStart := got[indexOf(got, oplog.DbEventOpStart, userOp)].startedAt
		mcpStart := got[indexOf(got, oplog.DbEventOpStart, "mcp")].startedAt
		if gap := mcpStart.Sub(userStart); gap < 900*time.Millisecond {
			t.Fatalf("%s mcp read started %v after the user op, want at least 900ms at 1 op/s: %+v", e.kind, gap, got)
		}
	})
}
