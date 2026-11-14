package dbmcpflow

import (
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/oplog"
	"github.com/kirathecat/kira-studio/internal/approvals"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// Concurrent writes are independent queue items: each answers alone, in any order. Contract
// approvals-studio: tests/ui/approvals.spec.ts renders this queue.
func TestApprovalsPerItem(t *testing.T) {
	f := newFixture(t)
	sess, err := f.connect(t, f.token(t))
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()

	stmts := []string{
		"insert into notes (body) values ('a')",
		"insert into notes (body) values ('b')",
		"insert into notes (body) values ('c')",
	}
	var calls []chan callOutcome
	for i, s := range stmts {
		calls = append(calls, callAsync(sess, f.connID, s))
		f.items(t, i+1)
	}
	items := f.items(t, 3)
	for i, it := range items {
		if it.Kind != approvals.KindDbQuery || decodeItem(t, it).Statement != stmts[i] {
			t.Fatalf("item %d = %+v, want the %q write in arrival order", i, it, stmts[i])
		}
		if win := time.Duration(it.ExpiresAt-it.CreatedAt) * time.Millisecond; win != 10*time.Minute {
			t.Fatalf("item %d window = %v, want 10m", i, win)
		}
		if it.Origin.Client != "Claude Code 2.1.318" {
			t.Fatalf("item %d origin = %+v", i, it.Origin)
		}
	}
	f.app.Contract(t, "approvals-studio", "ApprovalsService.List", items, flowharness.Mask("createdAt", "expiresAt"), flowharness.Replace(f.dbPath, "/data/mcp.db"))
	f.app.Contract(t, "approvals-studio", "args:ApprovalsService.Answer", approvals.AnswerArgs{IDs: []string{items[1].ID}, Approve: false})

	f.answerItems(t, false, items[1].ID)
	if res := wait(t, calls[1]); !res.IsError {
		t.Fatalf("denied write = %q, want a tool error", text(res))
	}
	if n := f.queueLen(t); n != 2 {
		t.Fatalf("queue = %d after one answer, want the other two", n)
	}
	f.answerItems(t, true, items[0].ID)
	if res := wait(t, calls[0]); res.IsError {
		t.Fatalf("approved write = %q", text(res))
	}
	if n := f.noteCount(t); n != 3 {
		t.Fatalf("notes = %d, want only the approved write applied", n)
	}
	f.answerItems(t, false, items[2].ID)
	if res := wait(t, calls[2]); !res.IsError {
		t.Fatalf("third write = %q, want denied", text(res))
	}

	// A second answer to a settled item reports not-found without touching the queue.
	res, err := f.app.W.ApprovalsSvc.Answer(approvals.AnswerArgs{IDs: []string{items[0].ID}, Approve: true})
	if err != nil || len(res) != 1 || res[0].Code == "" || res[0].Error == "" {
		t.Fatalf("stale answer = %+v (%v), want a coded error", res, err)
	}
}

// One Answer call can settle a query item, a held response and a stale id; each reports alone.
func TestApprovalsAnswerAllMixed(t *testing.T) {
	f := newFixture(t)
	sess, err := f.connect(t, f.token(t))
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	f.setPreview(t, true)

	write := callAsync(sess, f.connID, "insert into notes (body) values ('x')")
	f.items(t, 1)
	read := callAsync(sess, f.connID, "select id, body from notes order by id")
	items := f.items(t, 2)
	kinds := map[string]string{}
	for _, it := range items {
		kinds[it.Kind] = it.ID
	}
	if kinds[approvals.KindDbQuery] == "" || kinds[approvals.KindDbResponse] == "" {
		t.Fatalf("kinds = %v, want one query and one response item", kinds)
	}

	ids := []string{items[0].ID, "db.query:missing", items[1].ID}
	res, err := f.app.W.ApprovalsSvc.Answer(approvals.AnswerArgs{IDs: ids, Approve: true})
	if err != nil || len(res) != 3 {
		t.Fatalf("answer = %+v (%v)", res, err)
	}
	if res[0].Error != "" || res[2].Error != "" || res[1].Code == "" || !strings.Contains(res[1].ID, "missing") {
		t.Fatalf("results = %+v, want the stale id alone to fail", res)
	}
	if r := wait(t, read); r.IsError {
		t.Fatalf("read = %q", text(r))
	}
	// The approved write runs and its own response is then held for review.
	f.items(t, 1)
	f.answerItems(t, true, f.items(t, 1)[0].ID)
	if r := wait(t, write); r.IsError {
		t.Fatalf("write = %q", text(r))
	}
}

// J-MCP-001: an agent's write waits for approval. A rejected write never runs, an approved one
// lands for every client, and the op log names each statement with its source and outcome.
func TestMcpWriteJourneyLeavesOpLogTrail(t *testing.T) {
	f := newFixture(t)
	sess := f.session(t)
	const rejected, approved = "insert into notes (body) values ('rejected')", "insert into notes (body) values ('approved')"

	call := callAsync(sess, f.connID, rejected)
	req := f.pending(t)
	if req.Statement != rejected {
		t.Fatalf("pending = %q, want the rejected write", req.Statement)
	}
	f.answerItems(t, false, req.Item.ID)
	if res := wait(t, call); !res.IsError {
		t.Fatalf("rejected write = %q, want a tool error", text(res))
	}
	if n := f.noteCount(t); n != 2 {
		t.Fatalf("rejected write changed the table: %d rows", n)
	}

	call = callAsync(sess, f.connID, approved)
	req = f.pending(t)
	f.answerItems(t, true, req.Item.ID)
	if res := wait(t, call); res.IsError {
		t.Fatalf("approved write = %q", text(res))
	}
	db, err := sql.Open("sqlite", f.dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var body string
	if err := db.QueryRow("select body from notes where body = 'approved'").Scan(&body); err != nil {
		t.Fatalf("a second client does not see the approved row: %v", err)
	}

	var trail []oplog.Record
	testx.WaitUntil(t, 10*time.Second, func() bool {
		recent, err := f.app.W.Ops.Recent(bridge.OpsRecentArgs{Limit: 50})
		if err != nil {
			t.Fatal(err)
		}
		trail = trail[:0]
		for _, r := range recent {
			if r.Kind == "execute" && r.Status != "running" && strings.Contains(stmtOf(r), "insert into notes") {
				trail = append(trail, r)
			}
		}
		return len(trail) > 0
	})
	for _, r := range trail {
		if r.Source != "mcp" || !strings.Contains(stmtOf(r), "'approved'") {
			t.Fatalf("op log row = %+v, want only the approved statement, from mcp", r)
		}
	}
	if len(trail) != 1 {
		t.Fatalf("op log holds %d insert rows, want the approved one only (the rejected write never ran)", len(trail))
	}
}

// stmtOf is the statement text of a database op row: the command when the log keeps one, else the subject.
func stmtOf(r oplog.Record) string {
	if r.Command != nil {
		return *r.Command
	}
	return r.Subject
}
