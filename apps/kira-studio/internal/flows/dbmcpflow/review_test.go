package dbmcpflow

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/connections"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/approvals"
	"github.com/kirathecat/kira-studio/internal/configfs"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// Backend half of tests/ui/dbmcp-review-mode.spec.ts (contract dbmcp-review-mode): result review per
// MCP mode, on request from an approval prompt, and the limits of a held result.

const (
	readStmt  = "select id, body from notes order by id"
	marker    = "ZQ-MARKER-731"
	withheldR = `the user withheld the result of this query on "notes db"; it ran, but you get no rows. Do not rerun it to get the result another way.`
)

// mcpSet writes keys of the connection file's mcp section.
func (f *fixture) mcpSet(t *testing.T, kv map[string]string) {
	t.Helper()
	var sets []configfs.NodeSet
	for k, v := range kv {
		sets = append(sets, configfs.NodeSet{Path: append([]string{"mcp"}, strings.Split(k, ".")...), Value: v})
	}
	if err := f.app.W.ConnectionFiles.Patch(f.connID, sets...); err != nil {
		t.Fatal(err)
	}
}

func (f *fixture) summary(t *testing.T) model.ConnectionSummary {
	t.Helper()
	list, err := f.app.W.Connections.List()
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range list {
		if c.ID == f.connID {
			return c
		}
	}
	t.Fatal("fixture connection missing")
	return model.ConnectionSummary{}
}

func (f *fixture) seedMarker(t *testing.T) {
	t.Helper()
	db, err := sql.Open("sqlite", f.dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec("insert into notes (body) values (?)", marker); err != nil {
		t.Fatal(err)
	}
}

func callModeAsync(sess *mcp.ClientSession, connID, sqlText, mode string) chan callOutcome {
	ch := make(chan callOutcome, 1)
	go func() {
		res, err := sess.CallTool(ctx, &mcp.CallToolParams{Name: "run_query", Arguments: map[string]any{"connectionId": connID, "sql": sqlText, "path": "", "mode": mode}})
		ch <- callOutcome{res, err}
	}()
	return ch
}

type reviewPayload struct {
	Mode          string `json:"mode"`
	Requested     bool   `json:"requested"`
	ReviewOffered bool   `json:"reviewOffered"`
	ReviewResult  bool   `json:"reviewResult"`
}

func payloadOf(t *testing.T, it approvals.Item) reviewPayload {
	t.Helper()
	var p reviewPayload
	if err := json.Unmarshal(it.Payload, &p); err != nil {
		t.Fatal(err)
	}
	return p
}

func (f *fixture) listReviewResults(t *testing.T, sess *mcp.ClientSession) map[string]bool {
	t.Helper()
	res, err := sess.CallTool(ctx, &mcp.CallToolParams{Name: "list_connections"})
	if err != nil || res.IsError {
		t.Fatalf("list_connections = %q (%v)", text(res), err)
	}
	var conns []struct {
		ID            string          `json:"id"`
		ReviewResults map[string]bool `json:"reviewResults"`
	}
	if err := json.Unmarshal([]byte(text(res)), &conns); err != nil {
		t.Fatal(err)
	}
	for _, c := range conns {
		if c.ID == f.connID {
			return c.ReviewResults
		}
	}
	t.Fatal("connection not listed")
	return nil
}

// Each mode holds or sends on its own; inherit follows the app switch.
func TestDbMcpReviewPerMode(t *testing.T) {
	f := newFixture(t)
	sess := f.session(t)
	f.mcpSet(t, map[string]string{"write": "allow", "review.read": "review", "review.write": "send"})
	got := f.summary(t)
	if got.McpReview != (model.McpReview{Read: "review", Write: "send", Ddl: "inherit"}) {
		t.Fatalf("review = %+v", got.McpReview)
	}
	replace := flowharness.Replace(f.dbPath, "/data/mcp.db")
	f.app.Contract(t, "dbmcp-review-mode", "ConnectionsService.List#review", got, flowharness.Mask("createdAt", "updatedAt"), replace)

	if rr := f.listReviewResults(t, sess); !rr["read"] || rr["write"] || rr["ddl"] {
		t.Fatalf("reviewResults = %v, want read only", rr)
	}
	f.setPreview(t, true)
	if rr := f.listReviewResults(t, sess); !rr["read"] || rr["write"] || !rr["ddl"] {
		t.Fatalf("reviewResults with the app switch on = %v, want read and ddl (inherit)", rr)
	}
	f.setPreview(t, false)

	done := callAsync(sess, f.connID, readStmt)
	req := f.pending(t)
	if p := payloadOf(t, req.Item); req.Stage != "response" || p.Mode != "read" || p.Requested {
		t.Fatalf("pending = %+v payload %+v, want a read result held by setting", req, p)
	}
	f.app.Contract(t, "dbmcp-review-mode", "ApprovalsService.List#response", []approvals.Item{req.Item}, flowharness.Mask("createdAt", "expiresAt"), replace)
	f.answer(t, true, req.RequestID)
	if sent := wait(t, done); sent.IsError || !strings.Contains(text(sent), "two") {
		t.Fatalf("sent read = %q, want the rows", text(sent))
	}

	if w := runQuery(t, sess, f.connID, "insert into notes (body) values ('three')"); w.IsError {
		t.Fatalf("write on send = %q", text(w))
	}
	if n := f.queueLen(t); n != 0 {
		t.Fatalf("a write on send queued %d", n)
	}
}

// A weaker declared mode never skips review of what the statement looks like.
func TestDbMcpReviewDeclaredModeDodge(t *testing.T) {
	f := newFixture(t)
	sess := f.session(t)
	f.mcpSet(t, map[string]string{"write": "allow", "review.read": "review", "review.write": "send"})

	done := callModeAsync(sess, f.connID, readStmt, "write")
	req := f.pending(t)
	if req.Stage != "response" || payloadOf(t, req.Item).Mode != "write" {
		t.Fatalf("pending = %+v, want the SELECT declared write held because it is a read", req)
	}
	f.answer(t, true, req.RequestID)
	if res := wait(t, done); res.IsError {
		t.Fatalf("sent = %q", text(res))
	}
}

// Withhold gives the agent app text only; no data reaches the op log, the queue payload or a prompt.
func TestDbMcpReviewWithhold(t *testing.T) {
	f := newFixture(t)
	sess := f.session(t)
	f.seedMarker(t)
	f.mcpSet(t, map[string]string{"write": "allow", "review.read": "review", "review.write": "review"})

	done := callAsync(sess, f.connID, readStmt)
	req := f.pending(t)
	held, err := f.app.W.DbMcp.PendingResponse(bridge.DbMcpApprovalArgs{RequestID: req.RequestID})
	if err != nil || !strings.Contains(held.Text, marker) {
		t.Fatalf("PendingResponse = %+v (%v), want the held rows", held, err)
	}
	if strings.Contains(string(req.Item.Payload), marker) {
		t.Fatal("the queue payload carries a cell value")
	}
	entries := f.dbmcpEntries(t, 1)
	if blob, _ := json.Marshal(entries); strings.Contains(string(blob), marker) {
		t.Fatalf("the prompt carries a cell value: %s", blob)
	}
	f.answer(t, false, req.RequestID)
	if res := wait(t, done); !res.IsError || text(res) != withheldR {
		t.Fatalf("withheld read = %q, want %q", text(res), withheldR)
	}

	done = callModeAsync(sess, f.connID, "update notes set body = body where id = 1", "write")
	req = f.pending(t)
	f.answer(t, false, req.RequestID)
	if res := wait(t, done); !res.IsError || text(res) != withheldR+" Its changes stay." {
		t.Fatalf("withheld write = %q, want the text with the changes note", text(res))
	}

	recent, err := f.app.W.Ops.Recent(bridge.OpsRecentArgs{Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	blob, _ := json.Marshal(recent)
	if strings.Contains(string(blob), marker) || !strings.Contains(string(blob), "select id, body from notes") {
		t.Fatalf("op log = %s, want the statement and no cell value", blob)
	}
}

// An approver can ask to review one call whatever the setting says.
func TestDbMcpReviewOnRequest(t *testing.T) {
	f := newFixture(t)
	sess := f.session(t)
	replace := flowharness.Replace(f.dbPath, "/data/mcp.db")
	const write = "insert into notes (body) values ('three')"

	done := callAsync(sess, f.connID, write)
	req := f.pending(t)
	if p := payloadOf(t, req.Item); req.Stage != "query" || !p.ReviewOffered || p.ReviewResult {
		t.Fatalf("pending = %+v payload %+v, want a query item offering review, not yet on", req, payloadOf(t, req.Item))
	}
	f.app.Contract(t, "dbmcp-review-mode", "ApprovalsService.List#query", []approvals.Item{req.Item}, flowharness.Mask("createdAt", "expiresAt"), replace)
	args := approvals.AnswerArgs{IDs: []string{req.Item.ID}, Approve: true, Options: map[string]json.RawMessage{req.Item.ID: json.RawMessage(`{"reviewResult":true}`)}}
	f.app.Contract(t, "dbmcp-review-mode", "args:ApprovalsService.Answer#review", args)

	bad := approvals.AnswerArgs{IDs: args.IDs, Approve: true, Options: map[string]json.RawMessage{req.Item.ID: json.RawMessage(`{"reviewResult":true,"extra":1}`)}}
	if res, err := f.app.W.ApprovalsSvc.Answer(bad); err != nil || len(res) != 1 || res[0].Error == "" {
		t.Fatalf("answer with an unknown option = %+v (%v), want a contract error", res, err)
	}
	if n := f.queueLen(t); n != 1 {
		t.Fatalf("a refused answer left %d items, want the item pending", n)
	}
	if res, err := f.app.W.ApprovalsSvc.Answer(args); err != nil || res[0].Error != "" {
		t.Fatalf("answer = %+v (%v)", res, err)
	}

	testx.WaitUntil(t, 10*time.Second, func() bool { return f.stageNow(t) == "response" })
	resp := f.pending(t)
	if p := payloadOf(t, resp.Item); !p.Requested || p.Mode != "write" {
		t.Fatalf("result item payload = %+v, want requested, mode write", p)
	}
	f.app.Contract(t, "dbmcp-review-mode", "ApprovalsService.List#requested", []approvals.Item{resp.Item}, flowharness.Mask("createdAt", "expiresAt"), replace)
	f.answer(t, true, resp.RequestID)
	if res := wait(t, done); res.IsError {
		t.Fatalf("sent write = %q", text(res))
	}
	if n := f.noteCount(t); n != 3 {
		t.Fatalf("notes = %d, want the write to have run", n)
	}

	// Without the tick the result goes straight back.
	done = callAsync(sess, f.connID, write)
	f.answerItems(t, true, f.pending(t).Item.ID)
	if res := wait(t, done); res.IsError || f.queueLen(t) != 0 {
		t.Fatalf("unticked write = %q, queue %d, want no result item", text(res), f.queueLen(t))
	}

	// A result item takes no options.
	f.mcpSet(t, map[string]string{"review.read": "review"})
	done = callAsync(sess, f.connID, readStmt)
	held := f.pending(t)
	opt := approvals.AnswerArgs{IDs: []string{held.Item.ID}, Approve: true, Options: map[string]json.RawMessage{held.Item.ID: json.RawMessage(`{"reviewResult":true}`)}}
	if res, err := f.app.W.ApprovalsSvc.Answer(opt); err != nil || res[0].Error == "" {
		t.Fatalf("options on a result item = %+v (%v), want a contract error", res, err)
	}
	f.answer(t, false, held.RequestID)
	wait(t, done)
}

// The same checkbox rides an exemption.
func TestDbMcpReviewOnRequestExemption(t *testing.T) {
	f, sess := newExemptionFixture(t)
	const stmt = "select * from notes /* big */"
	id := exemptionID(t, runQuery(t, sess, f.connID, stmt))

	call := exemptAsync(sess, f.connID, stmt, id, "the report needs every row")
	items := f.items(t, 1)
	if p := payloadOf(t, items[0]); items[0].Kind != approvals.KindDbExemption || !p.ReviewOffered || p.ReviewResult {
		t.Fatalf("item = %s payload %+v, want an exemption offering review", items[0].Kind, p)
	}
	f.app.Contract(t, "dbmcp-review-mode", "ApprovalsService.List#exemption", items, flowharness.Mask("createdAt", "expiresAt"), flowharness.Replace(f.dbPath, "/data/mcp.db"))
	args := approvals.AnswerArgs{IDs: []string{items[0].ID}, Approve: true, Options: map[string]json.RawMessage{items[0].ID: json.RawMessage(`{"reviewResult":true}`)}}
	if res, err := f.app.W.ApprovalsSvc.Answer(args); err != nil || res[0].Error != "" {
		t.Fatalf("answer = %+v (%v)", res, err)
	}
	testx.WaitUntil(t, 10*time.Second, func() bool { return f.stageNow(t) == "response" })
	resp := f.pending(t)
	if !payloadOf(t, resp.Item).Requested {
		t.Fatalf("result item = %s, want requested", resp.Item.Payload)
	}
	f.answer(t, true, resp.RequestID)
	if res := wait(t, call); res.IsError || !strings.Contains(text(res), "two") {
		t.Fatalf("sent = %q, want the rows", text(res))
	}
}

// A call the settings already review shows the checkbox on and locked.
func TestDbMcpReviewOnRequestAlreadyOn(t *testing.T) {
	f := newFixture(t)
	sess := f.session(t)
	f.mcpSet(t, map[string]string{"read": "prompt", "review.read": "review"})

	done := callAsync(sess, f.connID, readStmt)
	req := f.pending(t)
	if p := payloadOf(t, req.Item); !p.ReviewOffered || !p.ReviewResult {
		t.Fatalf("payload = %+v, want review offered and already on", p)
	}
	f.app.Contract(t, "dbmcp-review-mode", "ApprovalsService.List#on", []approvals.Item{req.Item}, flowharness.Mask("createdAt", "expiresAt"), flowharness.Replace(f.dbPath, "/data/mcp.db"))
	f.answerItems(t, true, req.Item.ID)
	testx.WaitUntil(t, 10*time.Second, func() bool { return f.stageNow(t) == "response" })
	f.answer(t, true, f.pending(t).RequestID)
	if res := wait(t, done); res.IsError {
		t.Fatalf("sent = %q", text(res))
	}
}

// A result over the per-result cap, or past the total, is withheld unseen.
func TestDbMcpReviewCaps(t *testing.T) {
	const per, total = 200, 300
	f := newFixture(t, flowharness.WithApprovalHeldCaps(per, total))
	sess := f.session(t)
	f.mcpSet(t, map[string]string{"review.read": "review"})
	big := "select '" + strings.Repeat("x", 300) + "' as n"
	if res := runQuery(t, sess, f.connID, big); !res.IsError ||
		!strings.Contains(text(res), "over the 8 MB review limit") || !strings.Contains(text(res), "withheld unseen") {
		t.Fatalf("over the cap = %q, want the too-large refusal", text(res))
	}
	if n := f.queueLen(t); n != 0 {
		t.Fatalf("too-large result raised %d items", n)
	}

	small := "select 1 as n"
	a, b := callAsync(sess, f.connID, small), callAsync(sess, f.connID, small)
	_ = f.items(t, 2)
	res := runQuery(t, sess, f.connID, small)
	if !res.IsError || !strings.Contains(text(res), "too many results are waiting") {
		t.Fatalf("third result = %q, want the held-full refusal", text(res))
	}
	for _, it := range f.list(t) {
		f.answerItems(t, true, it.ID)
	}
	for _, ch := range []chan callOutcome{a, b} {
		if r := wait(t, ch); r.IsError {
			t.Fatalf("held result = %q", text(r))
		}
	}
}

// A client that cancels while its result waits takes the held bytes with it.
func TestDbMcpReviewAbandon(t *testing.T) {
	f := newFixture(t)
	sess := f.session(t)
	f.mcpSet(t, map[string]string{"review.read": "review"})

	abandon, cancel := context.WithCancel(ctx)
	go func() {
		_, _ = sess.CallTool(abandon, &mcp.CallToolParams{Name: "run_query", Arguments: map[string]any{"connectionId": f.connID, "sql": readStmt, "path": "", "mode": "read"}})
	}()
	req := f.pending(t)
	if req.Stage != "response" {
		t.Fatalf("pending = %+v, want a held result", req)
	}
	cancel()
	testx.WaitUntil(t, 10*time.Second, func() bool { return f.queueLen(t) == 0 })
	if _, err := f.app.W.DbMcp.PendingResponse(bridge.DbMcpApprovalArgs{RequestID: req.RequestID}); err == nil {
		t.Fatal("PendingResponse still answers after the client left")
	}
}

// The review wait is outside the statement timeout and the throttle: a held result frees the
// connection and survives longer than the timeout.
func TestDbMcpReviewTimeoutAndThrottle(t *testing.T) {
	f := timeoutFixture(t, 1, 1000)
	sess := f.session(t)
	f.mcpSet(t, map[string]string{"review.read": "review"})

	a := callAsync(sess, f.connID, readStmt)
	_ = f.items(t, 1)
	time.Sleep(1500 * time.Millisecond)
	b := callAsync(sess, f.connID, readStmt)
	items := f.items(t, 2)
	f.answerItems(t, true, items[0].ID)
	if res := wait(t, a); res.IsError || !strings.Contains(text(res), "two") {
		t.Fatalf("first result after longer than the timeout = %q, want the rows", text(res))
	}
	f.answerItems(t, false, items[1].ID)
	if res := wait(t, b); !res.IsError {
		t.Fatalf("withheld second result = %q", text(res))
	}
}

// A file that still carries review_results reads as that value in every mode, and a new file written from
// it uses the per-mode key.
func TestDbMcpReviewLegacyKey(t *testing.T) {
	f := newFixture(t)
	sess := f.session(t)
	f.mcpSet(t, map[string]string{"review_results": "review"})
	got := f.summary(t)
	if want := (model.McpReview{Read: "review", Write: "review", Ddl: "review"}); got.McpReview != want {
		t.Fatalf("review = %+v, want %+v", got.McpReview, want)
	}
	if rr := f.listReviewResults(t, sess); !rr["read"] || !rr["write"] || !rr["ddl"] {
		t.Fatalf("reviewResults = %v, want all true", rr)
	}

	copied, err := f.app.W.Connections.Create(connections.Input{ConnectionFields: model.ConnectionFields{
		Name: "legacy copy", Kind: "sqlite", Color: "teal", Mode: "fields", Database: ptr(f.dbPath), Options: map[string]any{},
		McpEnabled: true, McpReadMode: "allow", McpWriteMode: "prompt", McpDdlMode: "deny", McpReview: got.McpReview,
	}})
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.ReadFile(filepath.Join(f.app.W.ConnectionFiles.Dir(), copied.File))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(file), "review_results") || !strings.Contains(string(file), "review:") {
		t.Fatalf("new file:\n%s\nwant the per-mode review key and no review_results", file)
	}
}
