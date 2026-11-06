package dbmcpflow

import (
	"database/sql"
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/connections"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/oplog"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/ids"
	"github.com/kirathecat/kira-studio/internal/testx"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Backend half of tests/ui/connection-mcp-timeout.spec.ts: a connection's statement timeout stops an
// agent's statement, names the exemption path, and an approved exemption runs with no timeout.

// cte returns SQLite work scaling with n (about 0.4 s per million rows on a quiet machine).
func cte(n int) string {
	return `WITH RECURSIVE c(x) AS (SELECT 1 UNION ALL SELECT x+1 FROM c) SELECT count(*) FROM (SELECT x FROM c LIMIT ` + strconv.Itoa(n) + `)`
}

const (
	slowRead  = "WITH RECURSIVE c(x) AS (SELECT 1 UNION ALL SELECT x+1 FROM c) SELECT count(*) FROM (SELECT x FROM c LIMIT 400000000)"
	mediumCTE = 8_000_000 // longer than a 1 s limit, still finishes
)

// timeoutConn swaps the fixture to a twin connection with the given timeout and throttle.
func (f *fixture) timeoutConn(t *testing.T, seconds int, throttle float64) {
	t.Helper()
	conn, err := f.app.W.Connections.Create(connections.Input{ConnectionFields: model.ConnectionFields{
		Name: "timeout db", Kind: "sqlite", Color: "teal", Mode: "fields", Database: ptr(f.dbPath), Options: map[string]any{},
		McpEnabled: true, McpReadMode: "allow", McpWriteMode: "allow", McpDdlMode: "deny",
		McpStatementTimeoutSec: seconds, ThrottlePerSec: throttle,
	}})
	if err != nil {
		t.Fatal(err)
	}
	f.connID = conn.ID
	if _, err := f.app.W.Connections.Connect(bridge.ConnectionsIDArgs{ID: f.connID}); err != nil {
		t.Fatal(err)
	}
}

type opEndMark struct{ Status, Error string }

// mcpExecEnds records the op:end of every MCP execute op.
func (f *fixture) mcpExecEnds() (get func() []opEndMark, stop func()) {
	events, unsub := f.app.W.Router.Host().Subscribe()
	var mu sync.Mutex
	mcpOps := map[string]bool{}
	var ends []opEndMark
	done := make(chan struct{})
	go func() {
		defer close(done)
		for e := range events {
			var p struct {
				OpID, Kind, Source, Status string
				Error                      *string
			}
			_ = json.Unmarshal(e.Payload, &p)
			mu.Lock()
			switch e.Topic {
			case oplog.DbEventOpStart:
				mcpOps[p.OpID] = p.Kind == "execute" && p.Source == "mcp"
			case oplog.DbEventOpEnd:
				if mcpOps[p.OpID] {
					m := opEndMark{Status: p.Status}
					if p.Error != nil {
						m.Error = *p.Error
					}
					ends = append(ends, m)
				}
			}
			mu.Unlock()
		}
	}()
	return func() []opEndMark {
			mu.Lock()
			defer mu.Unlock()
			return append([]opEndMark(nil), ends...)
		}, func() {
			unsub()
			<-done
		}
}

var measuredRe = regexp.MustCompile(`measured (\d+) ms, limit (\d+) ms`)

func timeoutFixture(t *testing.T, seconds int, throttle float64) *fixture {
	t.Helper()
	f := newFixture(t)
	f.timeoutConn(t, seconds, throttle)
	return f
}

func TestDbMcpStatementTimeoutRefuses(t *testing.T) {
	f := timeoutFixture(t, 1, 0)
	sess, err := f.connect(t, f.token(t))
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	ends, stop := f.mcpExecEnds()
	defer stop()

	res := runQuery(t, sess, f.connID, slowRead)
	got := text(res)
	m := measuredRe.FindStringSubmatch(got)
	if !res.IsError || m == nil || !strings.Contains(got, "statement_timeout") || !strings.Contains(got, `exemption {"id":"exm_`) {
		t.Fatalf("slow read = %q, want a statement_timeout refusal naming the exemption field", got)
	}
	if measured, _ := strconv.Atoi(m[1]); measured < 1000 || m[2] != "1000" {
		t.Errorf("refusal %q: want measured >= 1000 against limit 1000", got)
	}
	testx.WaitUntil(t, 10*time.Second, func() bool { return len(ends()) == 1 })
	if e := ends()[0]; e.Status != "error" || !strings.Contains(e.Error, "timeout") {
		t.Errorf("op:end = %+v, want status error with the timeout text", e)
	}
	if res := runQuery(t, sess, f.connID, "select 1"); res.IsError {
		t.Fatalf("read after a timeout = %q, want the lane usable", text(res))
	}
}

func TestDbMcpStatementTimeoutWrite(t *testing.T) {
	f := timeoutFixture(t, 1, 0)
	sess, err := f.connect(t, f.token(t))
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()

	res := runQuery(t, sess, f.connID, "insert into notes (body) select 'x' from ("+strings.Replace(slowRead, "count(*)", "x", 1)+")")
	if !res.IsError || !strings.Contains(text(res), "statement_timeout") {
		t.Fatalf("slow write = %q, want a statement_timeout refusal", text(res))
	}
	// The interrupted statement may still be unwinding its rollback when the refusal returns.
	testx.WaitUntil(t, 10*time.Second, func() bool {
		db, err := sql.Open("sqlite", f.dbPath)
		if err != nil {
			return false
		}
		defer db.Close()
		var n int
		return db.QueryRow("select count(*) from notes").Scan(&n) == nil && n == 2
	})
}

func TestDbMcpStatementTimeoutExemption(t *testing.T) {
	f := timeoutFixture(t, 1, 0)
	sess, err := f.connect(t, f.token(t))
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	stmt := cte(mediumCTE)

	id := exemptionID(t, runQuery(t, sess, f.connID, stmt))
	call := exemptAsync(sess, f.connID, stmt, id, "the count is needed")
	items := f.items(t, 1)
	pl := decodeExemption(t, items[0])
	if len(pl.Limits) != 1 || pl.Limits[0].Limit != "statement_timeout" || pl.Limits[0].Max != 1000 || pl.Limits[0].Unit != "ms" {
		t.Fatalf("payload = %+v", pl)
	}
	f.app.Contract(t, "dbmcp-statement-timeout", "item", items[0], flowharness.Mask("createdAt", "expiresAt", "measured"), flowharness.Replace(f.dbPath, "/data/mcp.db"))
	f.answerItems(t, true, items[0].ID)
	start := time.Now()
	res := wait(t, call)
	if res.IsError || !strings.Contains(text(res), strconv.Itoa(mediumCTE)) {
		t.Fatalf("exempt call = %q, want the count", text(res))
	}
	if d := time.Since(start); d < time.Second {
		t.Errorf("exempt call took %v, want a run past the 1 s limit", d)
	}

	if res := runQuery(t, sess, f.connID, stmt); !res.IsError || !strings.Contains(text(res), "statement_timeout") {
		t.Fatalf("same call without exemption = %q, want refused again", text(res))
	}
}

func TestDbMcpStatementTimeoutOff(t *testing.T) {
	f := timeoutFixture(t, 0, 0)
	sess, err := f.connect(t, f.token(t))
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	start := time.Now()
	res := runQuery(t, sess, f.connID, cte(mediumCTE))
	if res.IsError || !strings.Contains(text(res), strconv.Itoa(mediumCTE)) {
		t.Fatalf("timeout off: %q, want the count", text(res))
	}
	if d := time.Since(start); d < time.Second {
		t.Errorf("statement took %v, want one that a 1 s limit would have stopped", d)
	}
}

// Queue wait never counts: a read queued behind a user op for longer than the limit still runs.
func TestDbMcpStatementTimeoutQueueNotCounted(t *testing.T) {
	f := timeoutFixture(t, 1, 1000)
	sess, err := f.connect(t, f.token(t))
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	marks, stop := f.recordOps()
	defer stop()

	userOp := ids.NewID()
	userDone := make(chan error, 1)
	go func() { userDone <- f.exec(userOp, cte(mediumCTE)) }()
	testx.WaitUntil(t, 10*time.Second, func() bool { return indexOf(marks(), oplog.DbEventOpStart, userOp) >= 0 })

	start := time.Now()
	res := runQuery(t, sess, f.connID, "select 1")
	if res.IsError {
		t.Fatalf("queued read = %q, want it to run after the user op", text(res))
	}
	if d := time.Since(start); d < 1100*time.Millisecond {
		t.Skipf("user op held the slot only %v: too fast to prove queue wait is not counted", d)
	}
	if err := <-userDone; err != nil {
		t.Fatal(err)
	}
}

func TestDbMcpListConnectionsStatementTimeout(t *testing.T) {
	f := newFixture(t)
	sess, err := f.connect(t, f.token(t))
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	for i, kind := range []string{"postgres", "mysql", "mariadb", "clickhouse", "sqlite", "mongodb", "redis", "kibana", "kafka"} {
		seconds := model.McpStatementTimeoutDefault
		if i == 0 {
			seconds = 0
		}
		in := model.ConnectionFields{
			Name: "t-" + kind, Kind: kind, Color: "teal", Mode: "fields", Host: ptr("localhost"), Port: ptr(1234), Options: map[string]any{},
			McpEnabled: true, McpReadMode: "allow", McpWriteMode: "prompt", McpDdlMode: "deny", McpStatementTimeoutSec: seconds,
		}
		if kind == "sqlite" {
			in.Host, in.Database = nil, ptr(f.dbPath)
		}
		if _, err := f.app.W.Connections.Create(connections.Input{ConnectionFields: in}); err != nil {
			t.Fatalf("create %s: %v", kind, err)
		}
	}
	res, err := sess.CallTool(ctx, &mcp.CallToolParams{Name: "list_connections", Arguments: map[string]any{}})
	if err != nil || res.IsError {
		t.Fatalf("list_connections = %v, %v", res, err)
	}
	var views []struct {
		Name             string          `json:"name"`
		Kind             string          `json:"kind"`
		StatementTimeout json.RawMessage `json:"statementTimeout"`
	}
	if err := json.Unmarshal([]byte(text(res)), &views); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"t-postgres":   `{"seconds":0,"enforcement":{"ddl":"session","read":"session","write":"session"}}`,
		"t-mysql":      `{"seconds":10,"enforcement":{"ddl":"app","read":"session","write":"app"}}`,
		"t-mariadb":    `{"seconds":10,"enforcement":{"ddl":"app","read":"session","write":"session"}}`,
		"t-clickhouse": `{"seconds":10,"enforcement":{"ddl":"app","read":"session","write":"app"}}`,
		"t-sqlite":     `{"seconds":10,"enforcement":{"ddl":"app","read":"app","write":"app"}}`,
		"t-mongodb":    `{"seconds":10,"enforcement":{"ddl":"app","read":"app","write":"app"}}`,
		"t-redis":      `{"seconds":10,"enforcement":{"ddl":"app","read":"app","write":"app"}}`,
		"t-kibana":     `{"seconds":10,"enforcement":{"ddl":"none","read":"app","write":"none"}}`,
		"t-kafka":      `{"seconds":10,"enforcement":{"ddl":"none","read":"none","write":"none"}}`,
	}
	seen := 0
	for _, v := range views {
		exp, ok := want[v.Name]
		if !ok {
			continue
		}
		seen++
		if string(v.StatementTimeout) != exp {
			t.Errorf("%s statementTimeout = %s, want %s", v.Name, v.StatementTimeout, exp)
		}
	}
	if seen != len(want) {
		t.Fatalf("listed %d of %d created connections", seen, len(want))
	}
	var all any
	_ = json.Unmarshal([]byte(text(res)), &all)
	f.app.Contract(t, "dbmcp-statement-timeout", "list_connections", all, flowharness.Mask("id"), flowharness.Replace(f.dbPath, "/data/mcp.db"))
}
