package dbmcpflow

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/connections"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/oplog"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/ids"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// Backend half of tests/ui/connection-throttle.spec.ts: the throttle is one serial, paced gate per
// connection, shared by the user's console and the MCP read lane.

// About a second of SQLite work; the cancel test uses the unbounded-ish longQuery instead.
const slowQuery = `WITH RECURSIVE c(x) AS (SELECT 1 UNION ALL SELECT x+1 FROM c) SELECT count(*) FROM (SELECT x FROM c LIMIT 2500000)`

type opMark struct{ topic, source, kind, opID string }

// recordOps appends one mark per op:start/op:end of connID's ops, in arrival order.
func (f *fixture) recordOps() (get func() []opMark, stop func()) {
	events, unsub := f.app.W.Router.Host().Subscribe()
	var mu sync.Mutex
	var marks []opMark
	done := make(chan struct{})
	go func() {
		defer close(done)
		for e := range events {
			if e.Topic != oplog.EventOpStart && e.Topic != oplog.EventOpEnd {
				continue
			}
			var p struct {
				OpID   string `json:"opId"`
				Kind   string `json:"kind"`
				Source string `json:"source"`
			}
			_ = json.Unmarshal(e.Payload, &p)
			mu.Lock()
			marks = append(marks, opMark{e.Topic, p.Source, p.Kind, p.OpID})
			mu.Unlock()
		}
	}()
	return func() []opMark {
			mu.Lock()
			defer mu.Unlock()
			return append([]opMark(nil), marks...)
		}, func() {
			unsub()
			<-done
		}
}

// indexOf finds the first mark of topic by source ("mcp" execute ops only: the read lane's own
// connect op is exempt from the throttle by design), or by op id.
func indexOf(marks []opMark, topic, who string) int {
	for i, m := range marks {
		if m.topic == topic && ((m.source == who && m.kind == "execute") || m.opID == who) {
			return i
		}
	}
	return -1
}

// connectThrottled swaps the fixture to a twin connection whose config carries the throttle, so
// every later connect installs it (a manual SetThrottle would be reset by the next connect).
func (f *fixture) connectThrottled(t *testing.T, perSec float64) {
	t.Helper()
	conn, err := f.app.W.Connections.Create(connections.Input{ConnectionFields: model.ConnectionFields{
		Name: "throttled db", Kind: "sqlite", Color: "teal", Mode: "fields", Database: ptr(f.dbPath), Options: map[string]any{},
		McpEnabled: true, McpReadMode: "allow", McpWriteMode: "prompt", McpDdlMode: "deny", ThrottlePerSec: perSec,
	}})
	if err != nil {
		t.Fatal(err)
	}
	f.connID = conn.ID
	if _, err := f.app.W.Connections.Connect(bridge.ConnectionsIDArgs{ID: f.connID}); err != nil {
		t.Fatal(err)
	}
}

func (f *fixture) userSlow(opID string) chan error {
	done := make(chan error, 1)
	go func() { done <- f.exec(opID, slowQuery) }()
	return done
}

func TestThrottleSerialAcrossUserAndMcp(t *testing.T) {
	for _, tc := range []struct {
		name    string
		perSec  float64
		overlap bool
	}{{"throttled", 1000, false}, {"off", 0, true}} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			sess, err := f.connect(t, f.token(t))
			if err != nil {
				t.Fatal(err)
			}
			defer sess.Close()
			f.connectThrottled(t, tc.perSec)
			marks, stop := f.recordOps()
			defer stop()

			userOp := ids.NewID()
			userDone := f.userSlow(userOp)
			testx.WaitUntil(t, 10*time.Second, func() bool { return indexOf(marks(), oplog.EventOpStart, userOp) >= 0 })
			if res := runQuery(t, sess, f.connID, "select 1"); res.IsError {
				t.Fatalf("run_query = %q", text(res))
			}
			if err := <-userDone; err != nil {
				t.Fatal(err)
			}
			testx.WaitUntil(t, 10*time.Second, func() bool { return indexOf(marks(), oplog.EventOpEnd, userOp) >= 0 })

			got := marks()
			mcpStart, userEnd := indexOf(got, oplog.EventOpStart, "mcp"), indexOf(got, oplog.EventOpEnd, userOp)
			if mcpStart < 0 || userEnd < 0 {
				t.Fatalf("marks = %+v, want an mcp start and a user end", got)
			}
			if tc.overlap && mcpStart > userEnd {
				t.Errorf("throttle off: mcp op started after the user op ended, want them side by side: %+v", got)
			}
			if !tc.overlap && mcpStart < userEnd {
				t.Errorf("throttled: mcp op started before the user op ended: %+v", got)
			}
		})
	}
}

func TestThrottlePacesReads(t *testing.T) {
	f := newFixture(t)
	f.connectThrottled(t, 10) // burst 10
	start := time.Now()
	for i := 0; i < 15; i++ {
		if err := f.exec(ids.NewID(), "select 1"); err != nil {
			t.Fatal(err)
		}
	}
	if elapsed := time.Since(start); elapsed < 400*time.Millisecond {
		t.Errorf("15 reads at 10/s took %v, want at least ~0.4s (5 past the burst)", elapsed)
	}
}

func TestThrottleCancelFreesSlot(t *testing.T) {
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
	f.connectThrottled(t, 1000)

	pending := callAsync(a, f.connID, longQuery)
	op := waitListed(t, b, func(o listedOp) bool { return o.Type == "op" && o.Kind == "execute" })

	next := make(chan error, 1)
	go func() { next <- f.exec(ids.NewID(), "select 1") }()
	select {
	case err := <-next:
		t.Fatalf("user read ran while the slot was held: %v", err)
	case <-time.After(300 * time.Millisecond):
	}

	if res := cancelOp(t, b, op.OpID); res.IsError {
		t.Fatalf("cancel_operation = %q", text(res))
	}
	select {
	case err := <-next:
		if err != nil {
			t.Fatalf("user read after cancel: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("user read still waiting after the holder was cancelled")
	}
	wait(t, pending)
}
