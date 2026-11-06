package dbmcpflow

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapterhost"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/dbmcp"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/approvals"
)

// fakeRowsLimit trips on a marker in the statement: a deterministic stand-in for the EXPLAIN threshold.
type fakeRowsLimit struct{}

func (fakeRowsLimit) Name() string { return dbmcp.LimitExpensiveQuery }

func (fakeRowsLimit) Check(_ context.Context, c dbmcp.LimitCall) (*dbmcp.LimitHit, error) {
	if !strings.Contains(c.SQL, "/* big */") {
		return nil, nil
	}
	return &dbmcp.LimitHit{Limit: dbmcp.LimitExpensiveQuery, Measured: 5000, Max: 1000, Unit: dbmcp.UnitRows}, nil
}

// fakeTimeLimit stands in for the statement timeout: a marked, non-exempt statement is swapped for
// one the engine rejects, and Hit maps that rejection to a timeout.
type fakeTimeLimit struct{}

const fakeTimeoutTable = "kira_fake_timeout"

func (fakeTimeLimit) Name() string { return dbmcp.LimitStatementTimeout }

func (fakeTimeLimit) Apply(ctx context.Context, c dbmcp.LimitCall, req *adapterhost.ExecuteRequestWire) (context.Context, func()) {
	if !c.Exempt && strings.Contains(c.SQL, "/* slow */") {
		req.Statements = []string{"select * from " + fakeTimeoutTable}
	}
	return ctx, func() {}
}

func (fakeTimeLimit) Hit(_ dbmcp.LimitCall, err error, _ time.Duration) *dbmcp.LimitHit {
	if !strings.Contains(err.Error(), fakeTimeoutTable) {
		return nil
	}
	return &dbmcp.LimitHit{Limit: dbmcp.LimitStatementTimeout, Measured: 5200, Max: 5000, Unit: dbmcp.UnitMillis}
}

func newExemptionFixture(t *testing.T) (*fixture, *mcp.ClientSession) {
	t.Helper()
	f := newFixture(t, flowharness.WithDbMcpLimits(fakeRowsLimit{}, fakeTimeLimit{}))
	sess, err := f.connect(t, f.token(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sess.Close() })
	return f, sess
}

var exemptionIDRe = regexp.MustCompile(`exm_[0-9a-z]{12}`)

func exemptionID(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()
	if !res.IsError {
		t.Fatalf("result = %q, want a limit refusal", text(res))
	}
	id := exemptionIDRe.FindString(text(res))
	if id == "" {
		t.Fatalf("refusal has no exemption id: %q", text(res))
	}
	return id
}

// exemptAsync repeats a call with the exemption field.
func exemptAsync(sess *mcp.ClientSession, connID, sqlText, id, reason string) chan callOutcome {
	return exemptModeAsync(sess, connID, sqlText, modeOf(sqlText), id, reason)
}

func exemptModeAsync(sess *mcp.ClientSession, connID, sqlText, mode, id, reason string) chan callOutcome {
	ch := make(chan callOutcome, 1)
	go func() {
		res, err := sess.CallTool(ctx, &mcp.CallToolParams{Name: "run_query", Arguments: map[string]any{
			"connectionId": connID, "sql": sqlText, "path": "", "mode": mode,
			"exemption": map[string]any{"id": id, "reason": reason},
		}})
		ch <- callOutcome{res, err}
	}()
	return ch
}

type exemptionItem struct {
	Statement string `json:"statement"`
	Mode      string `json:"mode"`
	Reason    string `json:"reason"`
	Limits    []struct {
		Limit    string  `json:"limit"`
		Measured float64 `json:"measured"`
		Max      float64 `json:"max"`
		Unit     string  `json:"unit"`
	} `json:"limits"`
}

func decodeExemption(t *testing.T, it approvals.Item) exemptionItem {
	t.Helper()
	if it.Kind != approvals.KindDbExemption || it.Reason != "exemption" {
		t.Fatalf("item = %s reason %q, want a db.exemption exemption item", it.Kind, it.Reason)
	}
	var pl exemptionItem
	if err := json.Unmarshal(it.Payload, &pl); err != nil {
		t.Fatal(err)
	}
	return pl
}

func assertNoItem(t *testing.T, f *fixture) {
	t.Helper()
	if n := f.queueLen(t); n != 0 {
		t.Fatalf("queue = %d, want empty", n)
	}
}

// The agent is refused over a limit, asks for an exemption, and the user approves although the
// connection allows reads without approval. Contract approvals-exemption:
// tests/ui/approvals-exemption.spec.ts renders this queue.
func TestDbMcpExemptionApproved(t *testing.T) {
	f, sess := newExemptionFixture(t)
	const stmt = "select * from notes /* big */"

	refusal := runQuery(t, sess, f.connID, stmt)
	id := exemptionID(t, refusal)
	for _, want := range []string{"expensive_query", "5000", "1000", `exemption {"id":"exm_`, `"reason"`, "10 minutes"} {
		if !strings.Contains(text(refusal), want) {
			t.Fatalf("refusal %q lacks %q", text(refusal), want)
		}
	}
	assertNoItem(t, f)

	call := exemptAsync(sess, f.connID, stmt, id, "the report needs every row")
	items := f.items(t, 1)
	pl := decodeExemption(t, items[0])
	if pl.Statement != stmt || pl.Mode != "read" || pl.Reason != "the report needs every row" ||
		len(pl.Limits) != 1 || pl.Limits[0].Limit != "expensive_query" || pl.Limits[0].Measured != 5000 || pl.Limits[0].Max != 1000 || pl.Limits[0].Unit != "rows" {
		t.Fatalf("payload = %+v", pl)
	}
	f.app.Contract(t, "approvals-exemption", "ApprovalsService.List", items, flowharness.Mask("createdAt", "expiresAt"), flowharness.Replace(f.dbPath, "/data/mcp.db"))
	f.app.Contract(t, "approvals-exemption", "args:ApprovalsService.Answer", approvals.AnswerArgs{IDs: []string{items[0].ID}, Approve: true})

	f.answerItems(t, true, items[0].ID)
	if res := wait(t, call); res.IsError || !strings.Contains(text(res), "one") {
		t.Fatalf("approved exempt call = %q, want the rows", text(res))
	}

	replay := runQueryWithExemption(t, sess, f.connID, stmt, id)
	if !replay.IsError || !strings.Contains(text(replay), "unknown, used or expired") {
		t.Fatalf("replayed id = %q", text(replay))
	}
	assertNoItem(t, f)
}

func runQueryWithExemption(t *testing.T, sess *mcp.ClientSession, connID, sqlText, id string) *mcp.CallToolResult {
	t.Helper()
	return wait(t, exemptAsync(sess, connID, sqlText, id, "retry"))
}

func TestDbMcpExemptionDenied(t *testing.T) {
	f, sess := newExemptionFixture(t)
	const stmt = "delete from notes /* big */"

	id := exemptionID(t, runQuery(t, sess, f.connID, stmt))
	call := exemptAsync(sess, f.connID, stmt, id, "clean up")
	items := f.items(t, 1)
	decodeExemption(t, items[0])
	f.answerItems(t, false, items[0].ID)

	res := wait(t, call)
	if !res.IsError || !strings.Contains(text(res), "kept limit expensive_query") || !strings.Contains(text(res), "did not run") {
		t.Fatalf("denied exemption = %q", text(res))
	}
	if n := f.noteCount(t); n != 2 {
		t.Fatalf("notes = %d, want the denied delete not run", n)
	}
}

func TestDbMcpExemptionMismatch(t *testing.T) {
	f, sess := newExemptionFixture(t)
	const issued = "select * from notes /* big */"

	id := exemptionID(t, runQuery(t, sess, f.connID, issued))
	res := runQueryWithExemption(t, sess, f.connID, "select id from notes /* big */", id)
	if !res.IsError || !strings.Contains(text(res), "issued for a different call") {
		t.Fatalf("mismatched exemption = %q", text(res))
	}
	assertNoItem(t, f)

	res = runQueryWithExemption(t, sess, f.connID, issued, id)
	if !res.IsError || !strings.Contains(text(res), "unknown, used or expired") {
		t.Fatalf("id after mismatch = %q, want spent", text(res))
	}
	assertNoItem(t, f)

	for _, bad := range []struct{ id, reason, want string }{
		{"nope", "why", "not an id from a limit refusal"},
		{id, "   ", "needs one sentence"},
		{id, strings.Repeat("x", 301), "300"},
	} {
		res := wait(t, exemptAsync(sess, f.connID, issued, bad.id, bad.reason))
		if !res.IsError || !strings.Contains(text(res), bad.want) {
			t.Fatalf("exemption %+v = %q, want %q", bad, text(res), bad.want)
		}
	}
	assertNoItem(t, f)
}

// An exemption lifts a limit only: every gate still refuses first, and a prompt mode raises one
// db.exemption item, not two.
func TestDbMcpExemptionNeverSkipsGates(t *testing.T) {
	f, sess := newExemptionFixture(t)
	const readStmt = "select * from notes /* big */"
	id := exemptionID(t, runQuery(t, sess, f.connID, readStmt))

	// ddl is denied on this connection: the mode gate refuses before the id is claimed.
	res := wait(t, exemptModeAsync(sess, f.connID, "create table x (a int) /* big */", "ddl", id, "need it"))
	if !res.IsError || !strings.Contains(text(res), "does not allow ddl") {
		t.Fatalf("denied mode with exemption = %q", text(res))
	}
	assertNoItem(t, f)
	call := exemptAsync(sess, f.connID, readStmt, id, "still valid")
	items := f.items(t, 1)
	decodeExemption(t, items[0])
	f.answerItems(t, false, items[0].ID)
	wait(t, call)

	const writeStmt = "update notes set body = 'x' /* big */"
	wid := exemptionID(t, runQuery(t, sess, f.connID, writeStmt))
	assertNoItem(t, f)
	call = exemptAsync(sess, f.connID, writeStmt, wid, "bulk fix")
	items = f.items(t, 1)
	decodeExemption(t, items[0])
	time.Sleep(200 * time.Millisecond)
	if n := f.queueLen(t); n != 1 {
		t.Fatalf("queue = %d for a prompt-mode exempt write, want the one db.exemption item", n)
	}
	f.answerItems(t, true, items[0].ID)
	if res := wait(t, call); res.IsError {
		t.Fatalf("approved exempt write = %q", text(res))
	}
}

// A second limit tripping after the first is lifted refuses with an id covering both; the next
// dialog lists both.
func TestDbMcpExemptionUnion(t *testing.T) {
	f, sess := newExemptionFixture(t)
	const stmt = "select * from notes /* big */ /* slow */"

	refusal := runQuery(t, sess, f.connID, stmt)
	if text(refusal) == "" || strings.Contains(text(refusal), "statement_timeout") {
		t.Fatalf("first refusal = %q, want the rows limit only", text(refusal))
	}
	id := exemptionID(t, refusal)

	call := exemptAsync(sess, f.connID, stmt, id, "need the full table")
	items := f.items(t, 1)
	if pl := decodeExemption(t, items[0]); len(pl.Limits) != 1 {
		t.Fatalf("first dialog = %+v", pl)
	}
	f.answerItems(t, true, items[0].ID)
	second := wait(t, call)
	for _, want := range []string{"statement_timeout", "5200", "5000", "expensive_query"} {
		if !strings.Contains(text(second), want) {
			t.Fatalf("second refusal %q lacks %q", text(second), want)
		}
	}
	id2 := exemptionID(t, second)
	if id2 == id {
		t.Fatal("second refusal reused the spent id")
	}

	call = exemptAsync(sess, f.connID, stmt, id2, "it is a slow table")
	items = f.items(t, 1)
	pl := decodeExemption(t, items[0])
	if len(pl.Limits) != 2 || pl.Limits[0].Limit != "expensive_query" || pl.Limits[1].Limit != "statement_timeout" {
		t.Fatalf("union dialog = %+v", pl)
	}
	f.app.Contract(t, "approvals-exemption", "item#union", items[0], flowharness.Mask("createdAt", "expiresAt"), flowharness.Replace(f.dbPath, "/data/mcp.db"))
	f.answerItems(t, true, items[0].ID)
	if res := wait(t, call); res.IsError || !strings.Contains(text(res), "one") {
		t.Fatalf("fully exempt call = %q, want the rows", text(res))
	}
}
