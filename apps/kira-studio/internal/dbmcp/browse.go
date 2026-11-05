package dbmcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/adapterhost"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/page"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/queryplan"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/storage/model"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/tableref"
	"github.com/kirathecat/kira-studio/internal/ids"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// browse_table limits (P535). The SDK enforces the schema maxima first; the handler re-checks them.
const (
	browseDefaultRows = 50
	browseMaxRows     = adapterhost.MaxBrowseRows
	browseMaxOffset   = 100000
	browseMaxColumns  = 200
	browseMaxFilters  = 20
	browseMaxInValues = 50
	browseMaxOrder    = 5
	browseMaxString   = 1000
)

// browseScalar is one filter value: a string, a boolean or a number kept as its exact JSON text.
type browseScalar struct{ v any }

func (b *browseScalar) UnmarshalJSON(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return err
	}
	switch v.(type) {
	case string, bool, json.Number:
		b.v = v
		return nil
	}
	return fmt.Errorf("a filter value is a string, number or boolean")
}

func (b browseScalar) MarshalJSON() ([]byte, error) { return json.Marshal(b.v) }

type browseFilter struct {
	Column string         `json:"column"`
	Op     string         `json:"op"`
	Value  *browseScalar  `json:"value,omitempty"`
	Values []browseScalar `json:"values,omitempty"`
}

type browseOrder struct {
	Column    string `json:"column"`
	Direction string `json:"direction"`
}

type browseTableArgs struct {
	ConnectionID string         `json:"connectionId"`
	Path         string         `json:"path"`
	Columns      []string       `json:"columns,omitempty"`
	Filters      []browseFilter `json:"filters,omitempty"`
	Order        []browseOrder  `json:"order,omitempty"`
	Limit        int            `json:"limit,omitempty"`
	Offset       int            `json:"offset,omitempty"`
	// Exemption asks the user to lift the limits of an earlier refusal for this exact call.
	Exemption *exemptionArgs `json:"exemption,omitempty"`
}

// browseOps maps each filter op to the number of values it takes: 0 none, 1 one, -1 several.
var browseOps = map[string]int{
	model.CondEq: 1, model.CondNe: 1, model.CondLt: 1, model.CondLte: 1, model.CondGt: 1, model.CondGte: 1,
	model.CondLike: 1, model.CondIn: -1, model.CondIsNull: 0, model.CondNotNull: 0,
}

// browseShape is the spec checked without the table's columns.
type browseShape struct {
	limit, offset int
	relation      string
}

// checkBrowseShape validates counts, ops, value shapes and that path is one relation. A non-empty msg
// refuses. No message carries a filter value.
func checkBrowseShape(summary model.ConnectionSummary, args browseTableArgs) (browseShape, string) {
	sh := browseShape{limit: args.Limit, offset: args.Offset}
	if sh.limit == 0 {
		sh.limit = browseDefaultRows
	}
	switch {
	case sh.limit < 0 || sh.limit > browseMaxRows:
		return sh, fmt.Sprintf("limit is 1 to %d", browseMaxRows)
	case sh.offset < 0 || sh.offset > browseMaxOffset:
		return sh, fmt.Sprintf("offset is 0 to %d", browseMaxOffset)
	case len(args.Columns) > browseMaxColumns:
		return sh, fmt.Sprintf("columns takes at most %d names", browseMaxColumns)
	case len(args.Filters) > browseMaxFilters:
		return sh, fmt.Sprintf("filters takes at most %d entries", browseMaxFilters)
	case len(args.Order) > browseMaxOrder:
		return sh, fmt.Sprintf("order takes at most %d entries", browseMaxOrder)
	}
	for i, f := range args.Filters {
		if msg := checkFilterShape(i, f); msg != "" {
			return sh, msg
		}
	}
	for i, o := range args.Order {
		if o.Direction != "asc" && o.Direction != "desc" {
			return sh, fmt.Sprintf("order[%d].direction is asc or desc", i)
		}
	}
	p, err := model.DecodePath(summary.ID, args.Path)
	ref, ok := tableref.FromPath(summary.Kind, p.Segments)
	if err != nil || !ok || len(p.Segments) != browseDepth(summary.Kind) {
		return sh, "browse_table needs a table path from list_children"
	}
	sh.relation = relationOf(summary.Kind, ref)
	return sh, ""
}

// checkFilterShape validates filter i's op, value count and value contents.
func checkFilterShape(i int, f browseFilter) string {
	want, ok := browseOps[f.Op]
	if !ok {
		return fmt.Sprintf("filters[%d].op %q is not one of eq, ne, lt, lte, gt, gte, in, like, isNull, notNull", i, f.Op)
	}
	switch {
	case want == 0 && (f.Value != nil || len(f.Values) > 0):
		return fmt.Sprintf("filters[%d]: %s takes no value", i, f.Op)
	case want == 1 && (f.Value == nil || len(f.Values) > 0):
		return fmt.Sprintf("filters[%d]: %s takes one value", i, f.Op)
	case want == -1 && (f.Value != nil || len(f.Values) == 0 || len(f.Values) > browseMaxInValues):
		return fmt.Sprintf("filters[%d]: in takes values with 1 to %d entries", i, browseMaxInValues)
	}
	vals := f.Values
	if f.Value != nil {
		vals = []browseScalar{*f.Value}
	}
	for _, v := range vals {
		if s, isStr := v.v.(string); isStr && (len([]rune(s)) > browseMaxString || strings.ContainsRune(s, 0)) {
			return fmt.Sprintf("filters[%d]: a string value is at most %d characters and holds no NUL", i, browseMaxString)
		}
	}
	return ""
}

// browseDepth is the path depth of one relation: database, schema, table on postgres; database, table on
// mysql, mariadb, sqlite (database main) and clickhouse.
func browseDepth(kind string) int {
	if kind == "postgres" {
		return 3
	}
	return 2
}

func relationOf(kind string, r tableref.Ref) string {
	switch kind {
	case "postgres":
		return quoteIdent(kind, r.Schema) + "." + quoteIdent(kind, r.Name)
	case "sqlite":
		return quoteIdent(kind, r.Name)
	}
	return quoteIdent(kind, r.Database) + "." + quoteIdent(kind, r.Name)
}

func quoteIdent(kind, name string) string {
	q := "`"
	if kind == "postgres" || kind == "sqlite" {
		q = `"`
	}
	return q + strings.ReplaceAll(name, q, q+q) + q
}

// browsePlan is a validated spec resolved against the table's columns and the mask state.
type browsePlan struct {
	relation   string
	projection []string
	withheld   []string
	conditions []model.Condition
	sort       *model.SortSpec
	limit      int
	offset     int
}

const maxListedColumns = 100

func unknownColumnMessage(relation, column string, meta model.ObjectMeta) string {
	names := make([]string, 0, len(meta.Columns))
	for _, c := range meta.Columns {
		names = append(names, c.Name)
	}
	more := ""
	if len(names) > maxListedColumns {
		more = fmt.Sprintf(" (+%d more)", len(names)-maxListedColumns)
		names = names[:maxListedColumns]
	}
	return fmt.Sprintf("column %q is not on %s. Columns: %s%s", column, relation, strings.Join(names, ", "), more)
}

// planBrowse applies the masking rules to a shape-checked spec. A non-empty refusal refuses the call.
// Names are matched exactly against the catalog; masked and withheld names fold to lower case like render.
func planBrowse(summary model.ConnectionSummary, sh browseShape, meta model.ObjectMeta, args browseTableArgs, mk maskset) (browsePlan, string) {
	p := browsePlan{relation: sh.relation, limit: sh.limit, offset: sh.offset}
	if (meta.Kind == "view" || meta.Kind == "matview") && !mk.Empty() {
		return p, "browse_table reads base tables only on a connection with masking: a view can rename a masked column. Use run_query where read is allowed."
	}
	r := browseRules{relation: sh.relation, meta: meta, mk: mk, known: make(map[string]bool, len(meta.Columns))}
	for _, c := range meta.Columns {
		r.known[c.Name] = true
	}
	if msg := r.project(&p, args.Columns); msg != "" {
		return p, msg
	}
	if msg := r.filter(&p, args.Filters); msg != "" {
		return p, msg
	}
	if msg := r.order(&p, args.Order); msg != "" {
		return p, msg
	}
	return p, ""
}

// browseRules resolves names against one table's catalog and the mask state.
type browseRules struct {
	relation string
	meta     model.ObjectMeta
	mk       maskset
	known    map[string]bool
}

func (r browseRules) masked(name string) bool { _, ok := r.mk.RuleFor(name); return ok }

func (r browseRules) withheld(name string) bool { return r.mk.withheld[strings.ToLower(name)] }

// project fills p.projection (and p.withheld) from the requested columns, or every column.
func (r browseRules) project(p *browsePlan, requested []string) string {
	if len(requested) == 0 {
		for _, c := range r.meta.Columns {
			requested = append(requested, c.Name)
		}
	}
	seen := map[string]bool{}
	for _, name := range requested {
		if !r.known[name] {
			return unknownColumnMessage(r.relation, name, r.meta)
		}
		if seen[name] {
			return fmt.Sprintf("columns lists %q twice", name)
		}
		seen[name] = true
		if r.withheld(name) {
			p.withheld = append(p.withheld, name)
			continue
		}
		p.projection = append(p.projection, name)
	}
	if len(p.projection) == 0 {
		return fmt.Sprintf("every requested column on %s is withheld until the user reviews masking", r.relation)
	}
	return ""
}

// filter turns the filters into conditions. A masked column takes only the null tests: equality confirms a
// guess, a range allows a binary search and like allows prefix enumeration.
func (r browseRules) filter(p *browsePlan, filters []browseFilter) string {
	for _, f := range filters {
		if !r.known[f.Column] {
			return unknownColumnMessage(r.relation, f.Column, r.meta)
		}
		if r.withheld(f.Column) {
			return newColumnUnreviewedRefusedError(f.Column, false).Error()
		}
		if r.masked(f.Column) && f.Op != model.CondIsNull && f.Op != model.CondNotNull {
			return fmt.Sprintf("column %s on %s is masked; browse_table filters it only with isNull or notNull. A comparison would let rows chosen by value reveal it.", f.Column, r.relation)
		}
		c := model.Condition{Column: f.Column, Op: f.Op}
		if f.Value != nil {
			c.Values = []any{f.Value.v}
		}
		for _, v := range f.Values {
			c.Values = append(c.Values, v.v)
		}
		p.conditions = append(p.conditions, c)
	}
	return ""
}

// order fills p.sort. Order plus offset is a binary search over values, so a masked column cannot be sorted on.
func (r browseRules) order(p *browsePlan, order []browseOrder) string {
	if len(order) == 0 {
		return ""
	}
	terms := make([]model.SortTerm, 0, len(order))
	for _, o := range order {
		if !r.known[o.Column] {
			return unknownColumnMessage(r.relation, o.Column, r.meta)
		}
		if r.withheld(o.Column) {
			return newColumnUnreviewedRefusedError(o.Column, false).Error()
		}
		if r.masked(o.Column) {
			return fmt.Sprintf("column %s on %s is masked and cannot be sorted on; the order would reveal its values' order.", o.Column, r.relation)
		}
		terms = append(terms, model.SortTerm{Column: o.Column, Direction: o.Direction})
	}
	p.sort = &model.SortSpec{Kind: "structured", Terms: terms}
	return ""
}

// preview is the SELECT the approval dialog shows and the EXPLAIN check plans: display only, never run.
// Literals are spelled so no string can end its quotes early under the dialect's escape rules.
func (p browsePlan) preview(kind string) string {
	cols := make([]string, len(p.projection))
	for i, c := range p.projection {
		cols[i] = quoteIdent(kind, c)
	}
	var b strings.Builder
	b.WriteString("SELECT " + strings.Join(cols, ", ") + " FROM " + p.relation)
	for i, c := range p.conditions {
		if i == 0 {
			b.WriteString(" WHERE ")
		} else {
			b.WriteString(" AND ")
		}
		b.WriteString(conditionPreview(kind, c))
	}
	if p.sort != nil {
		terms := make([]string, len(p.sort.Terms))
		for i, t := range p.sort.Terms {
			terms[i] = quoteIdent(kind, t.Column) + " " + strings.ToUpper(t.Direction)
		}
		b.WriteString(" ORDER BY " + strings.Join(terms, ", "))
	}
	b.WriteString(" LIMIT " + strconv.Itoa(p.limit) + " OFFSET " + strconv.Itoa(p.offset))
	return b.String()
}

var previewOps = map[string]string{
	model.CondEq: "=", model.CondNe: "<>", model.CondLt: "<", model.CondLte: "<=",
	model.CondGt: ">", model.CondGte: ">=", model.CondLike: "LIKE",
}

func conditionPreview(kind string, c model.Condition) string {
	col := quoteIdent(kind, c.Column)
	switch c.Op {
	case model.CondIsNull:
		return col + " IS NULL"
	case model.CondNotNull:
		return col + " IS NOT NULL"
	case model.CondIn:
		lits := make([]string, len(c.Values))
		for i, v := range c.Values {
			lits[i] = literalPreview(kind, v)
		}
		return col + " IN (" + strings.Join(lits, ", ") + ")"
	}
	return col + " " + previewOps[c.Op] + " " + literalPreview(kind, c.Values[0])
}

func literalPreview(kind string, v any) string {
	switch x := v.(type) {
	case bool:
		switch {
		case kind == "sqlite" && x:
			return "1"
		case kind == "sqlite":
			return "0"
		case x:
			return "TRUE"
		}
		return "FALSE"
	case json.Number:
		return x.String()
	case string:
		// A backslash is doubled where it can escape: under a doubled pair the closing quote stays a quote
		// in every mode of the engines that accept backslash escapes.
		if kind != "sqlite" {
			x = strings.ReplaceAll(x, `\`, `\\`)
		}
		return "'" + strings.ReplaceAll(x, "'", "''") + "'"
	}
	return "NULL"
}

// browseCall is the call the exemption, limit and approval seams see: the preview stands in for the
// statement, so an exemption binds to this exact spec.
func browseCall(args browseTableArgs, preview string) runQueryArgs {
	return runQueryArgs{ConnectionID: args.ConnectionID, SQL: preview, Path: args.Path, Mode: modeMasked, Exemption: args.Exemption}
}

// browseTable is the browse_table handler. Gate order: resolve, kind, masked verdict, spec shape, table
// allow list, connect, describe, mask state, column rules, exemption claim, EXPLAIN check, limits, one
// approval, re-check, execute, render, result review.
func (s *Server) browseTable(ctx context.Context, _ *mcp.CallToolRequest, args browseTableArgs) (*mcp.CallToolResult, any, error) {
	rv, sh, res, out, err, done := s.resolveBrowse(args)
	if done {
		return res, out, err
	}
	set, serr := allowListOf(rv.summary)
	if serr != nil {
		return badListResult(rv.summary)
	}
	if msg := allowedPath(rv.summary, set, args.Path, true); msg != "" {
		return errResult(msg)
	}
	state, cerr := s.connectForQuery(ctx, args.ConnectionID)
	if cerr != nil {
		return toolError(cerr)
	}
	if state.Status != "connected" {
		return errResult(connectStateError(state))
	}
	meta, derr := s.describeRelation(ctx, rv.summary, args.Path)
	if derr != nil {
		return toolError(derr)
	}
	mk, refusal, merr := s.maskStateFor(ctx, rv.summary)
	if refusal != nil || merr != nil {
		return refusal, nil, merr
	}
	plan, msg := planBrowse(rv.summary, sh, meta, args, mk)
	if msg != "" {
		return errResult(msg)
	}
	preview := plan.preview(rv.summary.Kind)
	call := browseCall(args, preview)
	rv.maxRows = plan.limit

	ex, res, out, err, done := s.claimExemption(call, rv)
	if done {
		return res, out, err
	}
	s.planBrowseCall(ctx, &rv, preview, args.Path, ex)
	if res, out, err, done := s.checkLimits(ctx, call, rv, ex); done {
		return res, out, err
	}
	if ex != nil {
		if res, out, err, done := s.awaitExemption(ctx, call, &rv, ex, rv.plan); done {
			return res, out, err
		}
	} else if res, out, err, done := s.awaitApproval(ctx, call, &rv, rv.plan); done {
		return res, out, err
	}
	// The summary was re-resolved after the wait: the allow list may have changed.
	if set, serr = allowListOf(rv.summary); serr != nil {
		return badListResult(rv.summary)
	}
	if msg := allowedPath(rv.summary, set, args.Path, true); msg != "" {
		return errResult(msg)
	}

	res, out, err, rendered, executed := s.executeBrowse(ctx, call, args, rv, sh, preview, ex)
	if !executed {
		return res, out, err
	}
	if !rv.reviewRequested && !s.reviewForMode(rv.summary, modeMasked) {
		return res, out, err
	}
	return s.reviewResponse(ctx, call, rv, rendered, res, out, err)
}

// resolveBrowse is the part of the gate that needs no connection: resolve, kind, masked verdict and spec
// shape. done is true whenever browseTable should return.
func (s *Server) resolveBrowse(args browseTableArgs) (rv queryResolution, sh browseShape, res *mcp.CallToolResult, out any, err error, done bool) {
	rv.summary, err = s.resolveEnabled(args.ConnectionID)
	if err != nil {
		res, out, err = errResult(err.Error())
		return rv, sh, res, out, err, true
	}
	if !HasBrowse(rv.summary.Kind) {
		res, out, err = errResult(fmt.Sprintf("connection %q (%s) has no structured browse; use run_query (mode read) where read is allowed", rv.summary.Name, rv.summary.Kind))
		return rv, sh, res, out, err, true
	}
	rv.mode, rv.class = modeMasked, adapters.ClassRead
	rv.verdict = modeVerdict(rv.summary, modeMasked)
	if rv.verdict == "deny" {
		msg := fmt.Sprintf("connection %q does not allow masked reads over MCP. Allowed: %s", rv.summary.Name, accessOf(rv.summary).Summary)
		if modesOf(rv.summary).read != "deny" {
			msg += ". Use run_query (mode read)"
		}
		res, out, err = errResult(msg)
		return rv, sh, res, out, err, true
	}
	var msg string
	if sh, msg = checkBrowseShape(rv.summary, args); msg != "" {
		res, out, err = errResult(msg)
		return rv, sh, res, out, err, true
	}
	rv.threshold = s.cfg.ExplainThreshold()
	return rv, sh, nil, nil, nil, false
}

// describeRelation is the table's columns with its real kind. Describe reports the kind the path names, which
// the agent chose, so the kind comes from the parent's listing: a view reached through a "table" path is
// still a view.
func (s *Server) describeRelation(ctx context.Context, summary model.ConnectionSummary, path string) (model.ObjectMeta, error) {
	desc, err := s.cfg.Tree.Describe(ctx, summary.ID, path, false, nil)
	if err != nil {
		return model.ObjectMeta{}, err
	}
	meta := desc.Meta
	p, err := model.DecodePath(summary.ID, path)
	if err != nil || len(p.Segments) == 0 {
		return meta, adapters.Contract("browse_table needs a table path from list_children", err)
	}
	parent := model.EncodePath(p.Segments[:len(p.Segments)-1])
	name := p.Segments[len(p.Segments)-1].Name
	children, err := s.cfg.Tree.Children(ctx, summary.ID, parent, false)
	if err != nil {
		return meta, err
	}
	for _, n := range children.Nodes {
		if n.Name == name && tableref.IsRelationKind(n.Kind) {
			meta.Kind = n.Kind
			return meta, nil
		}
	}
	return meta, adapters.New(adapters.CodeNotFound, "table "+name+" is not listed under its schema", nil)
}

// planBrowseCall makes the call's one EXPLAIN of the preview when the connection checks plans and the engine
// has a row estimate. Engines with none (sqlite) skip, as run_query does for the expensive_query limit.
func (s *Server) planBrowseCall(ctx context.Context, rv *queryResolution, preview, path string, ex *claimedExemption) {
	if !rv.summary.McpAutoExplain || !queryplan.HasRowEstimate(rv.summary.Kind) {
		return
	}
	plan, err := s.planStatements(ctx, rv.summary, queryplan.StatementsFor(rv.summary.Kind, preview), path, rv.threshold)
	if err != nil && ex.covers(LimitExpensiveQuery) {
		s.log.Warn("dbmcp: browse_table: EXPLAIN failed on an exempted call, approving without a plan", "connectionId", rv.summary.ID, "err", err)
		return
	}
	rv.plan, rv.planErr = plan, err
}

// executeBrowse re-reads the mask state, re-plans on it (a column masked or withheld during an approval wait
// is caught), reads one page and renders it. executed is false when the call failed before data was fetched.
func (s *Server) executeBrowse(ctx context.Context, call runQueryArgs, args browseTableArgs, rv queryResolution, sh browseShape, preview string, ex *claimedExemption) (res *mcp.CallToolResult, out any, err error, rendered maskset, executed bool) {
	set, refusal, merr := s.maskStateFor(ctx, rv.summary)
	if refusal != nil || merr != nil {
		return refusal, nil, merr, set, false
	}
	meta, derr := s.describeRelation(ctx, rv.summary, args.Path)
	if derr != nil {
		res, out, err = toolError(derr)
		return res, out, err, set, false
	}
	plan, msg := planBrowse(rv.summary, sh, meta, args, set)
	if msg != "" || plan.preview(rv.summary.Kind) != preview {
		res, out, err = errResult(fmt.Sprintf("masking or the table's columns on %q changed while this call waited; run it again", rv.summary.Name))
		return res, out, err, set, false
	}
	var mk *maskset
	if !set.Empty() {
		mk = &set
	}

	req := adapterhost.ExecuteRequestWire{}
	for _, l := range s.limits {
		el, ok := l.(ExecLimit)
		if !ok {
			continue
		}
		var done func()
		ctx, done = el.Apply(ctx, s.limitCall(call, rv, ex.covers(l.Name())), &req)
		defer done()
	}
	started := time.Now()
	pg, rerr := s.cfg.Query.Browse(ctx, adapterhost.BrowseRequest{
		OpID: ids.NewID(), ConnectionID: args.ConnectionID, Path: args.Path,
		Projection: plan.projection, Conditions: plan.conditions, Sort: plan.sort,
		PageSize: plan.limit, Offset: plan.offset,
		StatementTimeout: req.StatementTimeout, TimeoutByEngine: req.TimeoutByEngine,
	})
	if rerr != nil {
		if hits := s.execHits(call, rv, ex, rerr, time.Since(started)); len(hits) > 0 {
			res, out, err = s.refuseByLimits(rv.summary, ex, hits, call)
			return res, out, err, set, false
		}
		if mk != nil {
			res, out, err = maskedToolError(rerr)
			return res, out, err, set, true
		}
		res, out, err = toolError(rerr)
		return res, out, err, set, true
	}
	tp, ok := pg.(page.TabularPage)
	if !ok {
		return nil, nil, fmt.Errorf("dbmcp: browse_table: engine returned a %T page", pg), set, true
	}
	res, out, err = jsonResult(renderBrowsePage(tp, plan, mk, summaryOf(rv.plan, rv.threshold)))
	return res, out, err, set, true
}
