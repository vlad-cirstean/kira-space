package dbmcp

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/adapterhost"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/page"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/queryplan"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/ids"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// --- list_connections (§4.1) ---

type listConnectionsArgs struct{}

// listConnections returns every connection with mcp_enabled true — deny by default: a
// connection the user has not exposed is absent entirely, not listed-and-denied, since its
// existence is not the AI client's business.
//
// Deliberately not gated on read mode the way listChildren/describeTable/
// describeSchema now are. This tool's whole job is enumerating exposed connections *and their own
// configured permissions* (the Permissions field below) so a caller can see, up front, that a
// connection denies read/write/DDL before trying any of them — gating it on read mode would hide
// exactly the information it exists to surface, for a connection that denies reads but still
// allows writes or DDL.
func (s *Server) listConnections(_ context.Context, _ *mcp.CallToolRequest, _ listConnectionsArgs) (*mcp.CallToolResult, any, error) {
	conns, err := s.cfg.Conns.List()
	if err != nil {
		return nil, nil, err
	}
	out := make([]connectionView, 0, len(conns))
	for _, c := range conns {
		if !c.McpEnabled {
			continue
		}
		state := s.cfg.Conns.StateOf(c.ID)
		view := connectionView{
			ID: c.ID, Name: c.Name, Kind: c.Kind, ReadOnly: c.ReadOnly, Status: state.Status,
			Description: c.McpDescription,
			Permissions: connectionPermissions{Read: c.McpReadMode, Write: c.McpWriteMode, DDL: c.McpDdlMode},
			Access:      accessOf(c), Enforcement: enforcementOf(c.Kind), StatementTimeout: timeoutViewOf(c),
		}
		if state.ServerVersion != nil {
			view.ServerVersion = state.ServerVersion
		}
		if caps, ok := capsOf(state); ok {
			view.Capabilities = &connectionCapabilities{Query: caps.SQL, Describe: caps.Describe, SchemaColumns: caps.SchemaColumns}
		}
		// §4.5: cheap — one repo read per list_connections call, a human-frequency operation. A
		// read failure degrades to "no masked columns reported" rather than failing the whole
		// listing; the render path's own refusal (§4.4) is the real safety backstop regardless.
		set, serr := allowListOf(c)
		view.Tables = tablesView(c)
		if rules, err := s.cfg.MaskRules.List(c.ID); err == nil && serr == nil {
			view.MaskedColumns = maskedColumnsFor(filterMaskRules(set, rules))
		}
		out = append(out, view)
	}
	return jsonResult(out)
}

// resolveReadGated is list_children/describe_table/describe_schema's shared preamble:
// resolve the connection, then refuse if read mode denies. done is true whenever the caller should
// return (res, out, err) immediately, same convention as resolveVerdict/awaitApproval below.
//
// Schema-browsing tools are gated at read-mode's own minimum bar — deny refuses,
// same as every other read here. Not extended to prompt's own approval dialog: unlike run_query/
// explain_query, none of these run caller-supplied SQL against the connection, so there is no
// statement to show the human in an approval request.
func (s *Server) resolveReadGated(connectionID string) (summary model.ConnectionSummary, res *mcp.CallToolResult, out any, err error, done bool) {
	summary, err = s.resolveEnabled(connectionID)
	if err != nil {
		res, out, err = errResult(err.Error())
		return summary, res, out, err, true
	}
	if m := modesOf(summary); m.read == "deny" {
		res, out, err = errResult(fmt.Sprintf("connection %q's MCP permissions deny read statements; change them in the connection's MCP tab", summary.Name))
		return summary, res, out, err, true
	}
	return summary, nil, nil, nil, false
}

// databasePathKinds pick the database from the path's first segment; an empty path silently runs on
// the connection's primary database (mongodb: fails).
var databasePathKinds = map[string]bool{"postgres": true, "mysql": true, "mariadb": true, "mongodb": true}

// needsDatabasePath returns an error message when summary's kind needs a database path and path has none.
func needsDatabasePath(summary model.ConnectionSummary, path string) string {
	if !databasePathKinds[summary.Kind] {
		return ""
	}
	p, err := model.DecodePath(summary.ID, path)
	if err != nil || (len(p.Segments) > 0 && p.Segments[0].Kind == "database") {
		return ""
	}
	return fmt.Sprintf("path is required for connection %q (%s): pass a database path from list_children with this connectionId", summary.Name, summary.Kind)
}

// --- list_children — replacing the fixed-name list_databases/list_schemas SPEC first named:
// the adapter layer's metadata primitive is one lazy level (Adapter.Children), and the levels
// differ per kind, so one tool that returns the node's own `kind` is strictly more
// information than a pair of fixed names could carry. ---

type listChildrenArgs struct {
	ConnectionID string `json:"connectionId"`
	Path         string `json:"path,omitempty"`
	Refresh      bool   `json:"refresh,omitempty"`
}

func (s *Server) listChildren(ctx context.Context, _ *mcp.CallToolRequest, args listChildrenArgs) (*mcp.CallToolResult, any, error) {
	summary, res, out, err, done := s.resolveReadGated(args.ConnectionID)
	if done {
		return res, out, err
	}
	set, serr := allowListOf(summary)
	if serr != nil {
		return badListResult(summary)
	}
	if msg := allowedPath(summary, set, args.Path, false); msg != "" {
		return errResult(msg)
	}
	result, err := s.cfg.Tree.Children(ctx, args.ConnectionID, args.Path, args.Refresh)
	if err != nil {
		return toolError(err)
	}
	return jsonResult(filterNodes(summary, set, result.Nodes))
}

// --- describe_table (§4.3) ---

type describeTableArgs struct {
	ConnectionID string `json:"connectionId"`
	Path         string `json:"path"`
	Refresh      bool   `json:"refresh,omitempty"`
}

// describeTable is gated on Caps().Describe only in the sense that an adapter which cannot serve
// it already answers E_UNSUPPORTED in its own words (kafka/s3/sqs/redis) — surfaced verbatim by
// toolError rather than an invented message here (§4.3).
func (s *Server) describeTable(ctx context.Context, _ *mcp.CallToolRequest, args describeTableArgs) (*mcp.CallToolResult, any, error) {
	summary, res, out, err, done := s.resolveReadGated(args.ConnectionID)
	if done {
		return res, out, err
	}
	set, serr := allowListOf(summary)
	if serr != nil {
		return badListResult(summary)
	}
	if msg := allowedPath(summary, set, args.Path, true); msg != "" {
		return errResult(msg)
	}
	result, err := s.cfg.Tree.Describe(ctx, args.ConnectionID, args.Path, args.Refresh, nil)
	if err != nil {
		return toolError(err)
	}
	return jsonResult(result.Meta)
}

// --- describe_schema (§4.4) ---

type describeSchemaArgs struct {
	ConnectionID string `json:"connectionId"`
	Path         string `json:"path"`
	Refresh      bool   `json:"refresh,omitempty"`
}

func (s *Server) describeSchema(ctx context.Context, _ *mcp.CallToolRequest, args describeSchemaArgs) (*mcp.CallToolResult, any, error) {
	summary, res, out, err, done := s.resolveReadGated(args.ConnectionID)
	if done {
		return res, out, err
	}
	set, serr := allowListOf(summary)
	if serr != nil {
		return badListResult(summary)
	}
	result, err := s.cfg.Tree.SchemaColumns(ctx, args.ConnectionID, args.Path, args.Refresh)
	if err != nil {
		return toolError(err)
	}
	return jsonResult(filterRelations(summary, set, args.Path, result.Relations))
}

// --- run_query (§4.5) ---

const (
	runQueryDefaultMaxRows = 200
	runQueryMaxMaxRows     = 2000
)

type runQueryArgs struct {
	ConnectionID string `json:"connectionId"`
	SQL          string `json:"sql"`
	Path         string `json:"path"`
	Mode         string `json:"mode"`
	MaxRows      int    `json:"maxRows,omitempty"`
	// Exemption asks the user to lift the limits of an earlier refusal for this exact call.
	Exemption *exemptionArgs `json:"exemption,omitempty"`
}

type exemptionArgs struct {
	ID     string `json:"id"`
	Reason string `json:"reason"`
}

// runQuery builds an adapterhost.ExecuteRequestWire and calls Query.Execute — literally the method
// case "data:execute" calls for the console, so throttling, op-logging, cancellation, panic
// recovery and each adapter's own read-only wrap all apply with nothing re-implemented. Connects on
// demand: exposing a connection to MCP is the human's own explicit consent, and Connect is
// the same deduplicated path the UI uses, so the connection visibly comes up in the app rather than
// opening invisibly.
//
// The gate order: resolve the connection → check the declared mode → refuse before connecting when
// the declared mode's permission denies → clamp maxRows → connect → tighten-only classifier check
// (a statement that looks more powerful than its mode is refused, never run) → table allow list (P532) → forced EXPLAIN when
// McpAutoExplain is on (read mode only; a nil plan or any error degrades to "no plan") → heavy
// check → at most one approval, for a prompt verdict on the declared mode or a heavy plan →
// execute (read on the read lane, write and ddl on the main connection) → render.
//
// Limits (limits.go) slot in after tighten: a presented exemption id is claimed, every pre-limit not
// exempted is checked (a hit refuses before any approval), and an exempted call raises exactly one
// db.exemption approval in place of the db.query one, whatever the mode's verdict. Exec limits wrap the
// execute step; one that trips refuses with a fresh id.
func (s *Server) runQuery(ctx context.Context, _ *mcp.CallToolRequest, args runQueryArgs) (*mcp.CallToolResult, any, error) {
	rv, res, out, err, done := s.resolveVerdict(ctx, args)
	if done {
		return res, out, err
	}

	ex, res, out, err, done := s.claimExemption(args, rv)
	if done {
		return res, out, err
	}
	if res, out, err, done := s.checkLimits(ctx, args, rv, ex); done {
		return res, out, err
	}

	plan, heavy := s.maybeExplain(ctx, rv.summary, args)

	if ex != nil {
		if res, out, err, done := s.awaitExemption(ctx, args, &rv, ex, plan); done {
			return res, out, err
		}
	} else if res, out, err, done := s.awaitApproval(ctx, args, &rv, heavy, plan); done {
		return res, out, err
	}

	res, out, err, set, executed := s.executeAndRender(ctx, args, rv, plan, ex)
	if !executed || !s.reviewResults(rv.summary) {
		return res, out, err
	}
	return s.reviewResponse(ctx, args, rv, set, res, out, err)
}

// executeAndRender masks, executes and renders. executed is false when the call failed before
// Execute ran (no data was fetched, so nothing is held for review); set is the mask set the render
// used.
func (s *Server) executeAndRender(ctx context.Context, args runQueryArgs, rv queryResolution, plan *queryplan.Plan, ex *claimedExemption) (res *mcp.CallToolResult, out any, err error, set maskset, executed bool) {
	// M5 §4.3: resolved before Execute, so a key-store failure fails the call before the query runs,
	// not after — wasted adapter work on a call that cannot be rendered anyway.
	set, err = s.cfg.MaskRules.MaskSetFor(args.ConnectionID)
	if err != nil {
		return nil, nil, err, set, false
	}
	var mk *maskset
	if !set.Empty() {
		mk = &set
	}

	req := adapterhost.ExecuteRequestWire{
		OpID:         ids.NewID(),
		ConnectionID: args.ConnectionID,
		Path:         args.Path,
		Statements:   []string{args.SQL},
		Cap:          page.ResultCap{Rows: rv.maxRows},
	}
	// Exec limits shape the request and ctx for every call; an exempted one sees Exempt and relaxes.
	for _, l := range s.limits {
		el, ok := l.(ExecLimit)
		if !ok {
			continue
		}
		var done func()
		ctx, done = el.Apply(ctx, s.limitCall(args, rv, ex.covers(l.Name())), &req)
		defer done()
	}
	// The declared mode picks the connection: read runs on the read lane, where the engine
	// refuses writes; write and ddl run on the main connection.
	var resp adapterhost.ExecuteResponse
	started := time.Now()
	if rv.mode == modeRead {
		resp, err = s.cfg.Query.ExecuteRead(ctx, req)
	} else {
		resp, err = s.cfg.Query.Execute(ctx, req)
	}
	if err != nil {
		if hits := s.execHits(args, rv, ex, err, time.Since(started)); len(hits) > 0 {
			res, out, err = s.refuseByLimits(rv.summary, ex, hits, args)
			return res, out, err, set, false
		}
		if rv.mode == modeRead && isReadOnlyRefusal(err) {
			res, out, err = s.readOnlyRefusal(rv.summary, err, mk != nil)
			return res, out, err, set, true
		}
		// A Postgres/MySQL driver error routinely embeds the offending value (a
		// failed type-cast error names the literal it couldn't parse) — on a connection with
		// active mask rules, the adapter's raw message could leak a real, unmasked value one row
		// at a time (SELECT email::int FROM customers LIMIT 1 OFFSET n). Scoped to masked
		// connections only; an unmasked connection keeps full error detail for debuggability.
		if mk != nil {
			res, out, err = maskedToolError(err)
			return res, out, err, set, true
		}
		res, out, err = toolError(err)
		return res, out, err, set, true
	}
	if len(resp.Pages) == 0 {
		res, out, err = jsonResult(map[string]any{"kind": "empty", "rowCount": 0, "returned": 0})
		return res, out, err, set, true
	}
	rendered, err := renderPage(resp.Pages[0], rv.maxRows, summaryOf(plan, rv.threshold), mk, args.SQL)
	if err != nil {
		// §4.4/§5.3: a document/stream page under active masking rules is caller-correctable
		// (narrow the query, or remove the rules) — surfaced as an IsError result, not a raw Go
		// error. Anything else here is a genuine internal render fault.
		var refused *maskingRefusedError
		if errors.As(err, &refused) {
			res, out, err = errResult(refused.Error())
			return res, out, err, set, true
		}
		return nil, nil, err, set, true
	}
	// Finding #12, M6: a `;`-separated args.SQL can make the adapter execute more than one
	// statement, each with its own page — surface that rather than silently rendering only
	// resp.Pages[0] with no sign anything else ran.
	var result any = rendered
	if len(resp.Pages) > 1 {
		result = withAdditionalStatementResultsNote(rendered, len(resp.Pages)-1)
	}
	res, out, err = jsonResult(result)
	return res, out, err, set, true
}

// reviewResponse holds the result of a statement that already ran until the user sends or
// withholds it. Only an approval returns the held result, untouched; every other outcome returns
// an app-authored error with no row data.
func (s *Server) reviewResponse(ctx context.Context, args runQueryArgs, rv queryResolution, rendered maskset, res *mcp.CallToolResult, out any, err error) (*mcp.CallToolResult, any, error) {
	name := rv.summary.Name
	// go-sdk v1.8.0 (mcp/server.go toolForErr): a regular handler error reaches the client as an
	// IsError text result carrying err.Error(), so the preview of a bare Go error matches the wire.
	held := ApprovalResponse{IsError: true}
	if res != nil {
		held.IsError = res.IsError
		held.Text = resultText(res)
	} else if err != nil {
		held.Text = err.Error()
	}
	held.Bytes = len(held.Text)

	outcome := s.cfg.Approvals.Request(ctx, ApprovalRequest{
		ConnectionID: args.ConnectionID, ConnectionName: name, Kind: rv.summary.Kind,
		Class: rv.class, Mode: rv.mode, Statement: args.SQL, Reason: ApprovalReasonPermission,
		Target: targetOf(rv.summary, args.Path), Stage: ApprovalStageResponse, Response: &held, Origin: originFrom(ctx),
	})
	switch outcome {
	case ApprovalApproved:
		// The review can take up to ApprovalTimeout: exposure or masking rules may have changed since the
		// result was rendered.
		if _, rerr := s.resolveEnabled(args.ConnectionID); rerr != nil {
			return errResult(fmt.Sprintf("query against %q ran, but its result was withheld: %s", name, rerr.Error()))
		}
		now, merr := s.cfg.MaskRules.MaskSetFor(args.ConnectionID)
		if merr != nil {
			return errResult(fmt.Sprintf("query against %q ran, but its masking rules could not be re-checked and its result was withheld", name))
		}
		if !now.Equal(rendered) {
			return errResult(fmt.Sprintf("masking rules for %q changed while its result was in review; run the query again", name))
		}
		return res, out, err
	case ApprovalDenied:
		return errResult(fmt.Sprintf("query against %q ran, but the user withheld its result", name))
	case ApprovalTimedOut:
		return errResult(fmt.Sprintf("query against %q ran, but its result got no review within %s and was withheld", name, approvalWindow()))
	case ApprovalAbandoned:
		return errResult(fmt.Sprintf("the database MCP server stopped before the result of the query against %q was reviewed", name))
	case ApprovalQueueFull:
		return errResult("query ran; " + queueFullMessage)
	default:
		return errResult(fmt.Sprintf("query against %q ran, but its result was not approved", name))
	}
}

// resultText is the concatenated TextContent of res — the bytes the client receives.
func resultText(res *mcp.CallToolResult) string {
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}

// queryResolution is resolveVerdict's own bundle: everything runQuery needs from the
// resolve/check/gate/clamp/connect/tighten pipeline to proceed into maybeExplain/awaitApproval.
type queryResolution struct {
	summary model.ConnectionSummary
	// mode is the declared mode; verdict is its permission (allow or prompt once resolveVerdict passes).
	mode      string
	class     adapters.OpClass
	verdict   string
	maxRows   int
	threshold int
}

// resolveVerdict is runQuery's resolve → check mode → gate → clamp → connect → tighten → tables pipeline.
// done is true whenever runQuery should return (res, out, err) immediately: every gate here
// short-circuits execution, so no EXPLAIN work or approval prompt is spent on a refused statement.
func (s *Server) resolveVerdict(ctx context.Context, args runQueryArgs) (rv queryResolution, res *mcp.CallToolResult, out any, err error, done bool) {
	rv.summary, err = s.resolveEnabled(args.ConnectionID)
	if err != nil {
		res, out, err = errResult(err.Error())
		return rv, res, out, err, true
	}
	if msg := needsDatabasePath(rv.summary, args.Path); msg != "" {
		res, out, err = errResult(msg)
		return rv, res, out, err, true
	}
	if msg := checkMode(rv.summary, args.Mode); msg != "" {
		res, out, err = errResult(msg)
		return rv, res, out, err, true
	}
	rv.mode = args.Mode
	var msg string
	if rv.verdict, msg = gateMode(rv.summary, rv.mode); msg != "" {
		res, out, err = errResult(msg)
		return rv, res, out, err, true
	}

	rv.maxRows = args.MaxRows
	if rv.maxRows <= 0 {
		rv.maxRows = runQueryDefaultMaxRows
	}
	if rv.maxRows > runQueryMaxMaxRows {
		rv.maxRows = runQueryMaxMaxRows
	}

	// Read once per call, not per use: ExplainThreshold is read fresh on every call, never cached.
	rv.threshold = s.cfg.ExplainThreshold()

	state, cerr := s.connectForQuery(ctx, args.ConnectionID)
	if cerr != nil {
		res, out, err = toolError(cerr)
		return rv, res, out, err, true
	}
	if state.Status != "connected" {
		res, out, err = errResult(connectStateError(state))
		return rv, res, out, err, true
	}

	// A classifier error degrades to ClassUnknown rather than failing the call. The class can only
	// refuse: it never picks the connection and never relaxes the declared mode's gate.
	class, cerr := s.cfg.Query.ClassifyStatement(ctx, args.ConnectionID, args.SQL)
	if cerr != nil {
		s.log.Warn("dbmcp: run_query: classification failed, treating as unknown", "connectionId", args.ConnectionID, "err", cerr)
		class = adapters.ClassUnknown
	}
	rv.class = class
	if msg := tightenByClass(rv.summary, rv.mode, class); msg != "" {
		res, out, err = errResult(msg)
		return rv, res, out, err, true
	}
	// P532: table allow list, before any exemption, limit, EXPLAIN or approval.
	if res, out, err := s.checkStatementTables(ctx, rv.summary, args.Path, args.SQL); res != nil || err != nil {
		return rv, res, out, err, true
	}
	return rv, nil, nil, nil, false
}

// checkMode refuses a missing or unknown mode, naming what the connection allows.
func checkMode(summary model.ConnectionSummary, mode string) string {
	if modeRank[mode] == 0 && mode != modeRead {
		return fmt.Sprintf("run_query needs mode: read, write or ddl. Connection %q allows: %s", summary.Name, accessOf(summary).Summary)
	}
	return ""
}

// gateMode is the permission gate on the declared mode alone. It returns the verdict (allow or
// prompt) or, for a denied mode, the refusal text.
func gateMode(summary model.ConnectionSummary, mode string) (verdict, refusal string) {
	verdict = modeVerdict(summary, mode)
	if verdict == "deny" {
		return verdict, fmt.Sprintf("connection %q does not allow %s over MCP. Allowed: %s", summary.Name, mode, accessOf(summary).Summary)
	}
	return verdict, ""
}

// tightenByClass refuses a statement that looks more powerful than its declared mode. A lower class
// than declared runs under the declared mode's gate and connection.
func tightenByClass(summary model.ConnectionSummary, mode string, class adapters.OpClass) string {
	if classRank(class) <= modeRank[mode] {
		return ""
	}
	allowed := accessOf(summary).Summary
	if class == adapters.ClassUnknown {
		return fmt.Sprintf("could not tell what this statement does; declare mode=ddl if it changes schema. Connection %q allows: %s", summary.Name, allowed)
	}
	return fmt.Sprintf("this statement looks like %s; run it with mode=%s. Connection %q allows: %s", class, classMode(class), summary.Name, allowed)
}

// isReadOnlyRefusal reports whether err is the engine (or, on app lanes, Kira's own check) refusing
// a write on the read lane: postgres 25006, mysql 1792/1290, sqlite readonly (8), clickhouse 164.
func isReadOnlyRefusal(err error) bool { return adapters.IsReadOnlyRefusal(err) }

// readOnlyRefusal words a lane refusal for the agent. On a masked connection the engine text is
// withheld like any masked driver error.
func (s *Server) readOnlyRefusal(summary model.ConnectionSummary, err error, masked bool) (*mcp.CallToolResult, any, error) {
	allowed := accessOf(summary).Summary
	if masked {
		return errResult(fmt.Sprintf("the database refused a write on the read-only connection (message withheld, this connection has active PII masking rules). Use mode=write or mode=ddl. Connection %q allows: %s", summary.Name, allowed))
	}
	return errResult(fmt.Sprintf("the database refused a write on the read-only connection: %s. Use mode=write or mode=ddl. Connection %q allows: %s", err.Error(), summary.Name, allowed))
}

// maybeExplain runs auto-force-explain (a plan-only EXPLAIN, never ANALYZE) when the connection
// has McpAutoExplain on — the minimum information needed to tell the human what they are approving
// before a "prompt" verdict asks them. Any failure degrades to "no plan"; a run_query call
// must not start failing because a plan could not be parsed. heavy is the flag,
// overThreshold only, never "any warn issue" — SQLite reports no row estimate at all, so borrowing
// the console's isFlaggedPlan rule would raise a modal on nearly every SQLite query.
func (s *Server) maybeExplain(ctx context.Context, summary model.ConnectionSummary, args runQueryArgs) (plan *queryplan.Plan, heavy bool) {
	if summary.McpAutoExplain && args.Mode == modeRead {
		p, planErr := s.planFor(ctx, summary, args.SQL, args.Path)
		if planErr != nil {
			s.log.Warn("dbmcp: run_query: auto-force-explain failed, running without a plan", "connectionId", args.ConnectionID, "err", planErr)
		} else {
			plan = p
		}
	}
	return plan, plan != nil && plan.OverThreshold
}

// awaitApproval is the "at most one approval prompt per call" gate: when both a
// permission-prompt and a heavy plan apply, one request carries Reason: ApprovalReasonPermission
// (the stricter reason — the user said "ask me about every write") with the plan evidence riding
// along. A no-op (done false) when neither verdict == "prompt" nor heavy applies. done is true
// whenever runQuery should return (res, out, err) immediately.
func (s *Server) awaitApproval(ctx context.Context, args runQueryArgs, rv *queryResolution, heavy bool, plan *queryplan.Plan) (res *mcp.CallToolResult, out any, err error, done bool) {
	if rv.verdict != "prompt" && !heavy {
		return nil, nil, nil, false
	}

	reason := ApprovalReasonHeavy
	if rv.verdict == "prompt" {
		reason = ApprovalReasonPermission
	}
	outcome := s.cfg.Approvals.Request(ctx, ApprovalRequest{
		ConnectionID: args.ConnectionID, ConnectionName: rv.summary.Name, Kind: rv.summary.Kind,
		Class: rv.class, Mode: rv.mode, Enforcement: enforcementFor(rv.summary, rv.mode),
		Statement: args.SQL, Reason: reason,
		Target: targetOf(rv.summary, args.Path),
		Plan:   approvalPlanFrom(plan, rv.threshold), Origin: originFrom(ctx),
	})
	switch outcome {
	case ApprovalApproved:
		// rv.summary was resolved before this (up to ApprovalTimeout) wait. If the user reacted to
		// the prompt by revoking the connection's MCP exposure or denying this mode, an Approve
		// landing after that must not run the statement under permissions that no longer hold.
		return s.recheckAfterApproval(args, rv)
	default:
		res, out, err = approvalOutcomeError(outcome, fmt.Sprintf("query against %q", rv.summary.Name))
		return res, out, err, true
	}
}

// recheckAfterApproval refuses when the connection lost exposure or the mode became denied while the
// approval was pending. Otherwise it stores the re-resolved summary in rv, so settings read at
// execute (the statement timeout) are current after a long wait.
func (s *Server) recheckAfterApproval(args runQueryArgs, rv *queryResolution) (res *mcp.CallToolResult, out any, err error, done bool) {
	resummary, rerr := s.resolveEnabled(args.ConnectionID)
	if rerr != nil {
		res, out, err = errResult(rerr.Error())
		return res, out, err, true
	}
	if v := modeVerdict(resummary, rv.mode); v == "deny" {
		res, out, err = errResult(fmt.Sprintf("connection %q now denies %s over MCP. Allowed: %s", resummary.Name, rv.mode, accessOf(resummary).Summary))
		return res, out, err, true
	}
	rv.summary = resummary
	return nil, nil, nil, false
}

// enforcementFor is the enforcement label the approval shows for a read item; other modes carry none.
func enforcementFor(summary model.ConnectionSummary, mode string) string {
	if mode != modeRead {
		return ""
	}
	return enforcementOf(summary.Kind)
}

const queueFullMessage = "too many queries are already waiting for approval; retry after the user answers them"

// approvalOutcomeError words every outcome of a request that must not run; subject names what was
// asked about ("query against "x"").
func approvalOutcomeError(outcome ApprovalOutcome, subject string) (*mcp.CallToolResult, any, error) {
	switch outcome {
	case ApprovalDenied:
		return errResult(subject + " was denied by the user")
	case ApprovalTimedOut:
		return errResult(subject + " got no answer within " + approvalWindow())
	case ApprovalAbandoned:
		return errResult("the database MCP server stopped before the " + subject + " was answered")
	case ApprovalQueueFull:
		return errResult(queueFullMessage)
	default:
		return errResult(subject + " was not approved")
	}
}

// --- explain_query (§4) ---

type explainQueryArgs struct {
	ConnectionID string `json:"connectionId"`
	SQL          string `json:"sql"`
	Path         string `json:"path"`
	IncludeRaw   bool   `json:"includeRaw,omitempty"`
}

// explainQueryResult embeds queryplan.Plan so its own fields inline directly into the result
// object — the same shape planModel.ts's QueryPlan is, plus ThresholdRows: an MCP client
// cannot otherwise interpret OverThreshold, since the threshold itself lives in this app's own
// settings, never sent to the client any other way.
type explainQueryResult struct {
	queryplan.Plan
	ThresholdRows int `json:"thresholdRows"`
}

// explainQuery plans one SELECT/WITH statement without running it (the gate order):
// resolve → kind support → explainability → read-mode deny (before connecting) → connect →
// classify-assert every composed statement as a read → verdict on the read mode (prompt
// raises the approval, carrying the composed statement) → execute → parse → project, dropping Raw
// unless IncludeRaw.
func (s *Server) explainQuery(ctx context.Context, _ *mcp.CallToolRequest, args explainQueryArgs) (*mcp.CallToolResult, any, error) {
	summary, err := s.resolveEnabled(args.ConnectionID)
	if err != nil {
		return errResult(err.Error())
	}
	if msg := needsDatabasePath(summary, args.Path); msg != "" {
		return errResult(msg)
	}

	// §7: unsupported kinds are refused before connecting — nothing this call can do will succeed.
	if !queryplan.Supported(summary.Kind) {
		return errResult(fmt.Sprintf("connection %q (%s) has no EXPLAIN this app can parse", summary.Name, summary.Kind))
	}
	// §8.1: explain_query accepts only what queryplan.Explainable accepts — a DELETE/UPDATE never
	// gets its own EXPLAIN path (ClickHouse's EXPLAIN can execute its target on some forms).
	if !queryplan.Explainable(args.SQL) {
		return errResult("explain_query only accepts a SELECT or WITH statement")
	}

	m := modesOf(summary)
	if m.read == "deny" {
		return errResult(fmt.Sprintf("connection %q's MCP permissions deny read statements; change them in the connection's MCP tab", summary.Name))
	}

	state, err := s.connectForQuery(ctx, args.ConnectionID)
	if err != nil {
		return toolError(err)
	}
	if state.Status != "connected" {
		return errResult(connectStateError(state))
	}
	if res, out, err := s.checkStatementTables(ctx, summary, args.Path, args.SQL); res != nil || err != nil {
		return res, out, err
	}

	statements := queryplan.StatementsFor(summary.Kind, args.SQL)
	// §8.2/§8.3: the composed EXPLAIN is gated as a read through M2's existing classifier, not a
	// new permission vocabulary — a non-read verdict here is an internal refusal, this app's own
	// composer producing something it does not trust, never something the caller did wrong.
	if err := s.assertComposedStatementsAreReads(ctx, args.ConnectionID, statements); err != nil {
		return nil, nil, err
	}

	switch verdict := verdictFor(m, adapters.ClassRead); verdict {
	case "allow":
		// fall through to Execute below.
	case "prompt":
		outcome := s.cfg.Approvals.Request(ctx, ApprovalRequest{
			ConnectionID: args.ConnectionID, ConnectionName: summary.Name, Kind: summary.Kind,
			Class: adapters.ClassRead, Statement: strings.Join(statements, "\n"),
			Reason: ApprovalReasonPermission, Origin: originFrom(ctx),
		})
		switch outcome {
		case ApprovalApproved:
			// F6: mirror runQuery's own awaitApproval re-check — summary/m above were resolved
			// before this (up to ApprovalTimeout) wait, so an Approve landing after the user revoked MCP
			// exposure or tightened read mode to deny must not still run the composed EXPLAIN.
			resummary, rerr := s.resolveEnabled(args.ConnectionID)
			if rerr != nil {
				return errResult(rerr.Error())
			}
			if rm := modesOf(resummary); rm.read == "deny" {
				return errResult(fmt.Sprintf("connection %q's MCP permissions now deny read statements; change them in the connection's MCP tab", resummary.Name))
			}
			summary = resummary
			// fall through to Execute below.
		default:
			return approvalOutcomeError(outcome, fmt.Sprintf("query against %q", summary.Name))
		}
	case "deny":
		// Already handled above (m.read == "deny", before connecting) — unreachable in practice,
		// kept so a future change to verdictFor cannot silently skip the read-mode gate here.
		return errResult(fmt.Sprintf("connection %q's MCP permissions deny read statements; change them in the connection's MCP tab", summary.Name))
	default:
		return errResult("this connection's MCP permissions are misconfigured for read statements; change them in the connection's MCP tab")
	}

	// Finding #4, M6: same masked-error scoping as run_query — a composed EXPLAIN still plans
	// against the connection's real data, so keep the adapter's driver error value-free here too
	// when this connection has active mask rules.
	set, err := s.cfg.MaskRules.MaskSetFor(args.ConnectionID)
	if err != nil {
		return nil, nil, err
	}
	var mk *maskset
	if !set.Empty() {
		mk = &set
	}

	req := adapterhost.ExecuteRequestWire{
		OpID:         ids.NewID(),
		ConnectionID: args.ConnectionID,
		Path:         args.Path,
		Statements:   statements,
	}
	applyStatementTimeout(&req, summary, modeRead)
	resp, err := s.cfg.Query.ExecuteRead(ctx, req)
	if err != nil {
		if mk != nil {
			return maskedToolError(err)
		}
		return toolError(err)
	}

	threshold := s.cfg.ExplainThreshold()
	plan, err := queryplan.FromPages(summary.Kind, resp.Pages, threshold)
	if err != nil {
		if errors.Is(err, queryplan.ErrTruncated) {
			return errResult("the query plan was too large to parse — try a narrower statement")
		}
		if mk != nil {
			// F5: a parse failure carries the server's own raw EXPLAIN text in err — the same
			// value-bearing data Raw itself is, withheld below for the successful-parse case.
			return errResult("could not parse the EXPLAIN result")
		}
		return errResult(fmt.Sprintf("could not parse the EXPLAIN result: %s", err.Error()))
	}
	// On a masked connection, MySQL/MariaDB's attached_condition (Node.Detail) and
	// ClickHouse's index condition metrics substitute real row values from const-evaluated tables
	// into the plan text, and Raw is the server's own EXPLAIN output verbatim — none of that is
	// column-name maskable, so all of it is stripped outright, IncludeRaw notwithstanding.
	if mk != nil {
		plan = maskPlanForMaskedConnection(plan)
	} else if !args.IncludeRaw {
		plan.Raw = ""
	}
	return jsonResult(explainQueryResult{Plan: plan, ThresholdRows: threshold})
}
