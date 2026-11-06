package dbmcpflow

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/connections"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/dbmcp"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studioflowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiooplog"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
	"github.com/kirathecat/kira-studio/internal/approvals"
	"github.com/kirathecat/kira-studio/internal/configfs"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// P535 browse_table. UI half: tests/ui/connection-mcp-masked.spec.ts, contract dbmcp-browse.

func tablePath(db string, parts ...string) string {
	segs := []model.PathSegment{{Kind: "database", Name: db}}
	kinds := []string{"schema", "table"}
	if len(parts) == 1 {
		kinds = []string{"table"}
	}
	for i, p := range parts {
		segs = append(segs, model.PathSegment{Kind: kinds[i], Name: p})
	}
	return model.EncodePath(segs)
}

func (f *fixture) seedPeople(t *testing.T) {
	t.Helper()
	f.execSQL(t,
		"create table people (id integer primary key, name text, email text, note text)",
		"insert into people (name, email, note) values ('Ada', 'ada@x.io', 'a'), ('O''Brien', 'ob@x.io', 'b'), ('Cy', 'cy@x.io', null), ('Di', 'di@x.io', 'd'), ('Ed', 'ed@x.io', 'e')",
	)
}

func (f *fixture) maskPeople(t *testing.T, column string) {
	t.Helper()
	_, err := f.app.W.MaskRules.Upsert(bridge.MaskRulesUpsertArgs{ConnectionID: f.connID, Fields: model.MaskRuleFields{TableName: "people", ColumnName: column, Kind: "redact"}})
	if err != nil {
		t.Fatal(err)
	}
}

var peoplePath = tablePath("main", "people")

func browseCall(sess *mcp.ClientSession, connID string, args map[string]any) (*mcp.CallToolResult, error) {
	a := map[string]any{"connectionId": connID, "path": peoplePath}
	for k, v := range args {
		a[k] = v
	}
	return sess.CallTool(ctx, &mcp.CallToolParams{Name: "browse_table", Arguments: a})
}

func browse(t *testing.T, sess *mcp.ClientSession, connID string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	res, err := browseCall(sess, connID, args)
	if err != nil {
		t.Fatalf("browse_table %v: %v", args, err)
	}
	return res
}

func browseAsync(sess *mcp.ClientSession, connID string, args map[string]any) chan callOutcome {
	ch := make(chan callOutcome, 1)
	go func() {
		res, err := browseCall(sess, connID, args)
		ch <- callOutcome{res, err}
	}()
	return ch
}

type browseOut struct {
	Columns []struct {
		Name string `json:"name"`
	} `json:"columns"`
	Rows            [][]*string `json:"rows"`
	Returned        int         `json:"returned"`
	Offset          int         `json:"offset"`
	HasMore         bool        `json:"hasMore"`
	NextOffset      *int        `json:"nextOffset"`
	WithheldColumns []string    `json:"withheldColumns"`
}

func (o browseOut) col(name string) int {
	for i, c := range o.Columns {
		if c.Name == name {
			return i
		}
	}
	return -1
}

func (o browseOut) names(column string) []string {
	var out []string
	i := o.col(column)
	for _, r := range o.Rows {
		if i >= 0 && r[i] != nil {
			out = append(out, *r[i])
		}
	}
	return out
}

func decodeBrowse(t *testing.T, res *mcp.CallToolResult) browseOut {
	t.Helper()
	if res.IsError {
		t.Fatalf("browse_table = %q, want rows", text(res))
	}
	var out browseOut
	if err := json.Unmarshal([]byte(text(res)), &out); err != nil {
		t.Fatalf("result %q: %v", text(res), err)
	}
	return out
}

func wantRefusal(t *testing.T, res *mcp.CallToolResult, want ...string) {
	t.Helper()
	if !res.IsError {
		t.Fatalf("browse_table = %q, want a refusal", text(res))
	}
	for _, w := range want {
		if !strings.Contains(text(res), w) {
			t.Fatalf("refusal %q lacks %q", text(res), w)
		}
	}
}

// readOps counts the read ops of MCP calls that start, through the op log.
func (f *fixture) readOps() (count func() int, stop func()) {
	events, unsub := f.app.W.Router.Host().Subscribe()
	var mu sync.Mutex
	n := 0
	done := make(chan struct{})
	go func() {
		defer close(done)
		for e := range events {
			var p struct{ Kind, Source string }
			_ = json.Unmarshal(e.Payload, &p)
			if e.Topic == studiooplog.EventOpStart && p.Kind == "read" && p.Source == "mcp" {
				mu.Lock()
				n++
				mu.Unlock()
			}
		}
	}()
	return func() int { mu.Lock(); defer mu.Unlock(); return n }, func() { unsub(); <-done }
}

func TestDbMcpBrowseInherit(t *testing.T) {
	f := newFixture(t)
	f.seedPeople(t)
	sess := f.session(t)
	count, stop := f.readOps()
	defer stop()

	out := decodeBrowse(t, browse(t, sess, f.connID, nil))
	if out.Returned != 5 || len(out.Columns) != 4 || out.HasMore {
		t.Fatalf("browse = %+v, want 5 rows, 4 columns", out)
	}
	testx.WaitUntil(t, 5*time.Second, func() bool { return count() == 1 })

	f.mcpSet(t, map[string]string{"read": "deny"})
	wantRefusal(t, browse(t, sess, f.connID, nil), "does not allow masked reads over MCP")
	f.mcpSet(t, map[string]string{"read": "allow"})

	f.mcpSet(t, map[string]string{"masked": "deny"})
	wantRefusal(t, browse(t, sess, f.connID, nil), "does not allow masked reads over MCP", "Use run_query")
	if r := runQuery(t, sess, f.connID, "select id from people"); r.IsError {
		t.Fatalf("run_query with masked deny = %q, want it to run", text(r))
	}
}

func TestDbMcpBrowseMaskedOnly(t *testing.T) {
	f := newFixture(t)
	f.seedPeople(t)
	sess := f.session(t)
	f.mcpSet(t, map[string]string{"read": "deny", "masked": "allow"})
	f.maskPeople(t, "email")

	res := browse(t, sess, f.connID, nil)
	out := decodeBrowse(t, res)
	if out.Returned != 5 || strings.Contains(text(res), "ada@x.io") {
		t.Fatalf("browse = %q, want 5 rows with email masked", text(res))
	}
	r := runQuery(t, sess, f.connID, "select email from people")
	if !r.IsError || !strings.Contains(text(r), "browse_table") || strings.Contains(text(r), "ada@x.io") {
		t.Fatalf("run_query = %q, want a refusal pointing at browse_table", text(r))
	}
	if d, err := sess.CallTool(ctx, &mcp.CallToolParams{Name: "describe_table", Arguments: map[string]any{"connectionId": f.connID, "path": peoplePath}}); err != nil || d.IsError {
		t.Fatalf("describe_table = %q (%v), want it allowed", text(d), err)
	}
	ex, err := sess.CallTool(ctx, &mcp.CallToolParams{Name: "explain_query", Arguments: map[string]any{"connectionId": f.connID, "sql": "select 1", "path": ""}})
	if err != nil || !ex.IsError || !strings.Contains(text(ex), "deny read statements") {
		t.Fatalf("explain_query = %q (%v), want a read refusal", text(ex), err)
	}
}

func TestDbMcpBrowseFilters(t *testing.T) {
	f := newFixture(t)
	f.seedPeople(t)
	sess := f.session(t)
	f.maskPeople(t, "email")

	one := func(args map[string]any) browseOut { return decodeBrowse(t, browse(t, sess, f.connID, args)) }
	flt := func(col, op string, v any) map[string]any {
		f := map[string]any{"column": col, "op": op}
		if v != nil {
			f["value"] = v
		}
		return map[string]any{"filters": []map[string]any{f}}
	}

	if got := one(flt("name", "eq", "O'Brien")).names("name"); len(got) != 1 || got[0] != "O'Brien" {
		t.Fatalf("eq O'Brien = %v", got)
	}
	if got := one(map[string]any{"filters": []map[string]any{{"column": "id", "op": "in", "values": []any{1, 3}}}}).names("name"); len(got) != 2 {
		t.Fatalf("in = %v", got)
	}
	if got := one(flt("name", "like", "C%")).names("name"); len(got) != 1 || got[0] != "Cy" {
		t.Fatalf("like = %v", got)
	}
	if got := one(flt("id", "gte", 4)).names("name"); len(got) != 2 {
		t.Fatalf("gte 4 = %v, want 2 rows", got)
	}
	if got := one(flt("note", "isNull", nil)); got.Returned != 1 {
		t.Fatalf("isNull = %+v", got)
	}
	if got := one(flt("email", "notNull", nil)); got.Returned != 5 {
		t.Fatalf("masked notNull = %+v, want 5 rows", got)
	}

	page1 := one(map[string]any{"order": []map[string]any{{"column": "id", "direction": "desc"}}, "limit": 2})
	if got := page1.names("id"); len(got) != 2 || got[0] != "5" || !page1.HasMore || page1.NextOffset == nil || *page1.NextOffset != 2 {
		t.Fatalf("page 1 = %+v ids %v", page1, got)
	}
	page3 := one(map[string]any{"order": []map[string]any{{"column": "id", "direction": "desc"}}, "limit": 2, "offset": 4})
	if got := page3.names("id"); len(got) != 1 || got[0] != "1" || page3.HasMore || page3.NextOffset != nil {
		t.Fatalf("last page = %+v ids %v", page3, got)
	}

	// Masked columns take null tests only; the refusal names the column and never a value.
	for _, args := range []map[string]any{
		flt("email", "eq", "ada@x.io"),
		flt("email", "like", "ada%"),
		{"filters": []map[string]any{{"column": "email", "op": "in", "values": []any{"ada@x.io"}}}},
	} {
		r := browse(t, sess, f.connID, args)
		wantRefusal(t, r, "email", "is masked")
		if strings.Contains(text(r), "ada@x.io") || strings.Contains(text(r), "ada%") {
			t.Fatalf("refusal echoes the value: %q", text(r))
		}
	}
	wantRefusal(t, browse(t, sess, f.connID, map[string]any{"order": []map[string]any{{"column": "email", "direction": "asc"}}}), "cannot be sorted on")
	wantRefusal(t, browse(t, sess, f.connID, map[string]any{"columns": []string{"nope"}}), "Columns: id, name, email, note")
	wantRefusal(t, browse(t, sess, f.connID, map[string]any{"path": tablePath("main")}), "needs a table path")
	if res, err := browseCall(sess, f.connID, flt("id", "between", 1)); err == nil && !res.IsError {
		t.Fatalf("unknown op = %q, want the schema to reject it", text(res))
	}
	if n := f.queueLen(t); n != 0 {
		t.Fatalf("refused calls queued %d approvals", n)
	}
}

func TestDbMcpBrowseColumnReview(t *testing.T) {
	f := newFixture(t)
	f.seedPeople(t)
	sess := f.session(t)
	f.setReviewed(t, "people.id", "people.name", "people.email", "people.note", "notes.id", "notes.body")
	f.execSQL(t, "alter table people add column secret text", "update people set secret = 'hush-value'")
	f.refreshColumns(t)

	res := browse(t, sess, f.connID, nil)
	out := decodeBrowse(t, res)
	if len(out.Columns) != 4 || len(out.WithheldColumns) != 1 || out.WithheldColumns[0] != "secret" || strings.Contains(text(res), "hush-value") {
		t.Fatalf("browse = %q, want secret left out and named", text(res))
	}
	wantRefusal(t, browse(t, sess, f.connID, map[string]any{"filters": []map[string]any{{"column": "secret", "op": "isNull"}}}), "secret", "not reviewed")
	wantRefusal(t, browse(t, sess, f.connID, map[string]any{"order": []map[string]any{{"column": "secret", "direction": "asc"}}}), "secret")
	wantRefusal(t, browse(t, sess, f.connID, map[string]any{"columns": []string{"secret"}}), "every requested column")
}

func TestDbMcpBrowseTables(t *testing.T) {
	f := newFixture(t)
	f.seedPeople(t)
	sess := f.session(t)
	count, stop := f.readOps()
	defer stop()
	if err := f.app.W.ConnectionFiles.Patch(f.connID, configfs.NodeSet{Path: []string{"mcp", "tables"}, Value: []string{"notes"}}); err != nil {
		t.Fatal(err)
	}
	wantRefusal(t, browse(t, sess, f.connID, nil), "not listed: people")
	time.Sleep(200 * time.Millisecond)
	if n := count(); n != 0 {
		t.Fatalf("a refused browse ran %d read ops", n)
	}
}

func TestDbMcpBrowseView(t *testing.T) {
	f := newFixture(t)
	f.seedPeople(t)
	f.execSQL(t, "create view people_v as select id, email as e from people")
	sess := f.session(t)
	// A path that calls the view a table is still a view.
	args := map[string]any{"path": tablePath("main", "people_v")}

	if out := decodeBrowse(t, browse(t, sess, f.connID, args)); out.Returned != 5 {
		t.Fatalf("view without masking = %+v", out)
	}
	f.maskPeople(t, "email")
	res := browse(t, sess, f.connID, args)
	wantRefusal(t, res, "base tables only")
	if strings.Contains(text(res), "ada@x.io") {
		t.Fatalf("view leaked: %q", text(res))
	}
}

func TestDbMcpBrowsePrompt(t *testing.T) {
	f := newFixture(t)
	f.seedPeople(t)
	sess := f.session(t)
	f.mcpSet(t, map[string]string{"masked": "prompt"})
	f.maskPeople(t, "email")
	replace := studioflowharness.Replace(f.dbPath, "/data/mcp.db")
	f.app.Contract(t, "dbmcp-browse", "ConnectionsService.List#masked", f.summary(t), studioflowharness.Mask("createdAt", "updatedAt"), replace)

	args := map[string]any{"columns": []string{"id", "name"}, "filters": []map[string]any{{"column": "name", "op": "eq", "value": "O'Brien"}}, "order": []map[string]any{{"column": "id", "direction": "desc"}}, "limit": 10}
	done := browseAsync(sess, f.connID, args)
	req := f.pending(t)
	p := payloadOf(t, req.Item)
	if req.Item.Kind != approvals.KindDbQuery || p.Mode != "masked" || req.Class != "read" || !p.ReviewOffered {
		t.Fatalf("pending = %+v payload %+v, want a db.query read item, mode masked, review offered", req.Item, p)
	}
	wantSQL := `SELECT "id", "name" FROM "people" WHERE "name" = 'O''Brien' ORDER BY "id" DESC LIMIT 10 OFFSET 0`
	if req.Statement != wantSQL {
		t.Fatalf("statement = %q, want %q", req.Statement, wantSQL)
	}
	f.app.Contract(t, "dbmcp-browse", "ApprovalsService.List#browse", []approvals.Item{req.Item}, studioflowharness.Mask("createdAt", "expiresAt"), replace)
	f.answerItems(t, true, req.Item.ID)
	out := decodeBrowse(t, wait(t, done))
	if got := out.names("name"); len(got) != 1 || got[0] != "O'Brien" {
		t.Fatalf("approved browse = %v", got)
	}

	done = browseAsync(sess, f.connID, nil)
	req = f.pending(t)
	f.answerItems(t, false, req.Item.ID)
	if r := wait(t, done); !r.IsError || !strings.Contains(text(r), "was denied by the user") {
		t.Fatalf("denied browse = %q", text(r))
	}
}

func TestDbMcpBrowseReview(t *testing.T) {
	f := newFixture(t)
	f.seedPeople(t)
	sess := f.session(t)

	// The masked setting holds on its own.
	f.mcpSet(t, map[string]string{"review.masked": "review", "review.read": "send"})
	done := browseAsync(sess, f.connID, nil)
	req := f.pending(t)
	if p := payloadOf(t, req.Item); req.Stage != "response" || p.Mode != "masked" || p.Requested {
		t.Fatalf("pending = %+v payload %+v, want a held masked result", req, p)
	}
	f.answer(t, true, req.RequestID)
	decodeBrowse(t, wait(t, done))
	if rr := f.listReviewResults(t, sess); !rr["masked"] || rr["read"] {
		t.Fatalf("reviewResults = %v, want masked only", rr)
	}

	// Read's setting never forces review on browse.
	f.mcpSet(t, map[string]string{"review.masked": "send", "review.read": "review"})
	decodeBrowse(t, browse(t, sess, f.connID, nil))
	if n := f.queueLen(t); n != 0 {
		t.Fatalf("browse with masked send queued %d items", n)
	}

	// The approver can ask for a review on the browse item.
	f.mcpSet(t, map[string]string{"masked": "prompt"})
	done = browseAsync(sess, f.connID, nil)
	req = f.pending(t)
	args := approvals.AnswerArgs{IDs: []string{req.Item.ID}, Approve: true, Options: map[string]json.RawMessage{req.Item.ID: json.RawMessage(`{"reviewResult":true}`)}}
	if res, err := f.app.W.ApprovalsSvc.Answer(args); err != nil || res[0].Error != "" {
		t.Fatalf("answer = %+v (%v)", res, err)
	}
	testx.WaitUntil(t, 10*time.Second, func() bool { return f.stageNow(t) == "response" })
	resp := f.pending(t)
	if p := payloadOf(t, resp.Item); !p.Requested || p.Mode != "masked" {
		t.Fatalf("result item payload = %+v, want requested, mode masked", p)
	}
	f.answer(t, true, resp.RequestID)
	decodeBrowse(t, wait(t, done))
}

type browseListView struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Permissions struct {
		Masked string `json:"masked"`
	} `json:"permissions"`
	Access struct {
		Masked struct {
			Allowed bool   `json:"allowed"`
			Reason  string `json:"reason"`
		} `json:"masked"`
	} `json:"access"`
	Browse struct {
		Supported bool `json:"supported"`
		MaxRows   int  `json:"maxRows"`
	} `json:"browse"`
}

func TestDbMcpBrowseList(t *testing.T) {
	f := newFixture(t)
	f.mcpSet(t, map[string]string{"masked": "allow", "read": "deny"})
	redis, err := f.app.W.Connections.Create(connections.Input{ConnectionFields: model.ConnectionFields{
		Name: "kv", Kind: "redis", Color: "teal", Mode: "fields", Host: ptr("localhost"), Port: ptr(1234), Options: map[string]any{},
		McpEnabled: true, McpReadMode: "allow", McpWriteMode: "deny", McpDdlMode: "deny",
	}})
	if err != nil {
		t.Fatal(err)
	}
	sess := f.session(t)
	res, err := sess.CallTool(ctx, &mcp.CallToolParams{Name: "list_connections"})
	if err != nil || res.IsError {
		t.Fatalf("list_connections = %q (%v)", text(res), err)
	}
	var views []browseListView
	if err := json.Unmarshal([]byte(text(res)), &views); err != nil {
		t.Fatal(err)
	}
	for _, v := range views {
		switch v.ID {
		case f.connID:
			if v.Permissions.Masked != "allow" || !v.Access.Masked.Allowed || !v.Browse.Supported || v.Browse.MaxRows != 100 {
				t.Errorf("sqlite view = %+v", v)
			}
		case redis.ID:
			if v.Permissions.Masked != "inherit" || v.Access.Masked.Allowed || v.Access.Masked.Reason != "no structured browse" || v.Browse.Supported {
				t.Errorf("redis view = %+v", v)
			}
		}
	}
	r, err := sess.CallTool(ctx, &mcp.CallToolParams{Name: "browse_table", Arguments: map[string]any{"connectionId": redis.ID, "path": "database:0"}})
	if err != nil {
		t.Fatal(err)
	}
	wantRefusal(t, r, "no structured browse")
}

func TestDbMcpBrowseInvalidMasked(t *testing.T) {
	f := newFixture(t)
	err := f.app.W.ConnectionFiles.Patch(f.connID, configfs.NodeSet{Path: []string{"mcp", "masked"}, Value: "sometimes"})
	if err == nil || !strings.Contains(err.Error(), "mcp.masked") {
		t.Fatalf("Patch mcp.masked = %v, want an mcp.masked problem", err)
	}
}

// fakeBrowseLimit stands in for the row estimate: a browse whose preview filters on 'big' is over the limit.
type fakeBrowseLimit struct{}

func (fakeBrowseLimit) Name() string { return dbmcp.LimitExpensiveQuery }

func (fakeBrowseLimit) Check(_ context.Context, c dbmcp.LimitCall) (*dbmcp.LimitHit, error) {
	if c.Mode != "masked" || !strings.Contains(c.SQL, "'big'") {
		return nil, nil
	}
	return &dbmcp.LimitHit{Limit: dbmcp.LimitExpensiveQuery, Measured: 5000, Max: 1000, Unit: dbmcp.UnitRows}, nil
}

// A browse over a limit takes the exemption path: the refusal tells how to narrow the call, the dialog is a
// db.exemption item with the preview, approve runs the page, deny keeps the limit.
func TestDbMcpBrowseExemption(t *testing.T) {
	f := newFixture(t, studioflowharness.WithDbMcpLimits(fakeBrowseLimit{}))
	f.seedPeople(t)
	f.execSQL(t, "insert into people (name, email) values ('big', 'big@x.io')")
	sess := f.session(t)
	args := map[string]any{"filters": []map[string]any{{"column": "name", "op": "eq", "value": "big"}}}

	r := browse(t, sess, f.connID, args)
	wantRefusal(t, r, "expensive_query", "Filter or order on an indexed column", `browse_table call with exemption {"id":"exm_`)
	id := exemptionID(t, r)

	withEx := func(id string) map[string]any {
		a := map[string]any{"exemption": map[string]any{"id": id, "reason": "the report needs it"}}
		for k, v := range args {
			a[k] = v
		}
		return a
	}
	done := browseAsync(sess, f.connID, withEx(id))
	items := f.items(t, 1)
	pl := decodeExemption(t, items[0])
	if pl.Mode != "masked" || len(pl.Limits) != 1 || pl.Limits[0].Limit != "expensive_query" || !strings.HasPrefix(pl.Statement, "SELECT ") {
		t.Fatalf("payload = %+v", pl)
	}
	f.answerItems(t, true, items[0].ID)
	if out := decodeBrowse(t, wait(t, done)); out.Returned != 1 {
		t.Fatalf("approved exempt browse = %+v", out)
	}

	// Another spec cannot use the id; a denied exemption keeps the limit.
	id = exemptionID(t, browse(t, sess, f.connID, args))
	other := browse(t, sess, f.connID, map[string]any{"limit": 3, "exemption": map[string]any{"id": id, "reason": "x"}})
	wantRefusal(t, other, "issued for a different call")
	id = exemptionID(t, browse(t, sess, f.connID, args))
	done = browseAsync(sess, f.connID, withEx(id))
	items = f.items(t, 1)
	f.answerItems(t, false, items[0].ID)
	wantRefusal(t, wait(t, done), "kept limit expensive_query")
}

// Real EXPLAIN on the app-built statement: an unfiltered read of a big table is refused with the browse hint,
// a selective one passes and reports its plan.
func TestDbMcpBrowseExpensiveQueryPostgres(t *testing.T) {
	studioflowharness.Complete(t)
	studioflowharness.RequireDocker(t)
	const pass, dbName = "flowpass", "flowdb"
	host, port := startPostgres(t, pass, dbName)
	f := newFixture(t)
	f.setExpensiveThreshold(t, expensiveThreshold)
	pg, err := f.app.W.Connections.Create(connections.Input{ConnectionFields: model.ConnectionFields{
		Name: "flow pg", Kind: "postgres", Color: "indigo", Mode: "fields",
		Host: ptr(host), Port: ptr(port), Database: ptr(dbName), Username: ptr("postgres"),
		Options: map[string]any{"sslmode": "disable"}, McpEnabled: true, McpAutoExplain: true,
		McpReadMode: "deny", McpMaskedMode: "allow", McpWriteMode: "allow", McpDdlMode: "allow",
	}, Password: ptr(pass)})
	if err != nil {
		t.Fatal(err)
	}
	sess := f.session(t)
	path := "database:" + dbName
	for _, s := range []struct{ sql, mode string }{
		{"create schema app", "ddl"},
		{"create table app.big (id int primary key, v text)", "ddl"},
		{"insert into app.big select g, 'x' from generate_series(1, 5000) g", "write"},
		{"analyze app.big", "ddl"},
	} {
		if r := call(t, sess, map[string]any{"connectionId": pg.ID, "sql": s.sql, "path": path, "mode": s.mode}); r.IsError {
			t.Fatalf("%q = %q", s.sql, text(r))
		}
	}
	bigPath := tablePath(dbName, "app", "big")
	run := func(a map[string]any) *mcp.CallToolResult {
		t.Helper()
		a["connectionId"], a["path"] = pg.ID, bigPath
		res, err := sess.CallTool(ctx, &mcp.CallToolParams{Name: "browse_table", Arguments: a})
		if err != nil {
			t.Fatal(err)
		}
		return res
	}

	r := run(map[string]any{})
	m := rowsMeasuredRe.FindStringSubmatch(text(r))
	if !r.IsError || m == nil || m[2] != strconv.Itoa(expensiveThreshold) || !strings.Contains(text(r), "Filter or order on an indexed column") {
		t.Fatalf("unfiltered browse = %q, want an expensive_query refusal with the browse hint", text(r))
	}
	// A value holding ';' reaches the EXPLAIN as data.
	sel := run(map[string]any{"filters": []map[string]any{{"column": "id", "op": "eq", "value": 7}}})
	if rows, ok := resultPlan(t, sel); sel.IsError || !ok || rows >= expensiveThreshold {
		t.Fatalf("selective browse = %q (plan rows %v), want rows with an estimate under the limit", text(sel), rows)
	}
	semi := run(map[string]any{"filters": []map[string]any{{"column": "id", "op": "eq", "value": 7}, {"column": "v", "op": "eq", "value": "x;y"}}})
	if semi.IsError && !strings.Contains(text(semi), "could not plan") {
		t.Fatalf("browse with a ';' value = %q, want rows or the plan refusal", text(semi))
	}
	f.setAutoExplain(t, pg.ID, false)
	if r := run(map[string]any{}); r.IsError {
		t.Fatalf("unfiltered browse with the check off = %q, want rows", text(r))
	}
}
