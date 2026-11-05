package dbmcp

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"sort"
	"strings"
	"unicode"

	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/mask"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/maskreview"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/page"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// maskset is the masking a render applies: the rules of mask.Set plus the columns withheld until the
// user reviews them (P534). Embedding keeps RuleFor, Masker and ApplyColumn as they were.
type maskset struct {
	mask.Set
	// withheld holds lowercased result column names to drop.
	withheld map[string]bool
}

// Empty reports whether nothing is masked and nothing is withheld.
func (m maskset) Empty() bool { return m.Set.Empty() && len(m.withheld) == 0 }

// Equal reports whether o applies the same rules, key and withheld names.
func (m maskset) Equal(o maskset) bool {
	return m.Set.Equal(o.Set) && maps.Equal(m.withheld, o.withheld)
}

// names is every column name the guards watch: rule names and withheld names, sorted and unique.
func (m maskset) names() []string {
	out := make([]string, 0, len(m.Rules)+len(m.withheld))
	for n := range m.withheld {
		out = append(out, n)
	}
	for n := range m.Rules {
		if !m.withheld[n] {
			out = append(out, n)
		}
	}
	sort.Strings(out)
	return out
}

// maskingRefusedError is render's own "cannot mask a document/stream page" refusal (plan §4.4) — a
// distinct type so runQuery can tell it apart from a genuine internal render fault and surface it
// as a caller-correctable IsError result (§5.3's split) rather than a raw Go error.
type maskingRefusedError struct{ msg string }

func (e *maskingRefusedError) Error() string { return e.msg }

// newMaskingRefusedError builds §4.4's own refusal text for pageKind ("document" or "stream"),
// bodyNoun naming what that page kind carries instead of columns ("a document body", "a stream
// message body").
func newMaskingRefusedError(pageKind, bodyNoun string) error {
	return &maskingRefusedError{msg: fmt.Sprintf(
		"this connection has PII masking rules, which cannot be applied to a %s result; masking rules address table columns, and %s has none — narrow the query to a projection, or remove the rules in the connection's Privacy tab",
		pageKind, bodyNoun,
	)}
}

// textResult wraps text as a successful *mcp.CallToolResult — Out is any everywhere here, so the
// SDK publishes no output schema and every answer is one TextContent.
func textResult(text string) (*mcp.CallToolResult, any, error) {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}, nil, nil
}

// errResult wraps text as a caller-correctable failure — §5.3's split, the SDK's own contract:
// IsError for a condition the caller can fix, a bare Go error only for a genuine internal fault.
func errResult(text string) (*mcp.CallToolResult, any, error) {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: text}},
		IsError: true,
	}, nil, nil
}

// jsonResult marshals v as compact JSON and wraps it as a successful result: the exact
// structs the frontend receives, no prose wrapper and no human formatting — the client is a model
// composing SQL against identifiers that can contain spaces, quotes and non-ASCII, and JSON
// round-trips them exactly.
func jsonResult(v any) (*mcp.CallToolResult, any, error) {
	encoded, err := json.Marshal(v)
	if err != nil {
		return nil, nil, fmt.Errorf("dbmcp: encode result: %w", err)
	}
	return textResult(string(encoded))
}

// toolError classifies err. A caller-correctable condition — an *adapters.Error
// carrying a real E_* code (adapters.CodeOf), or an *ipcerr.Error (e.g. ipcerr.Disconnected) —
// becomes an IsError result with the adapter's own message verbatim, prefixed with its code so a
// client can branch on it without parsing prose. Anything else is a genuine internal fault,
// returned as a Go error so the SDK reports it as such rather than a tool-level failure.
func toolError(err error) (*mcp.CallToolResult, any, error) {
	if code, ok := adapters.CodeOf(err); ok {
		return errResult(fmt.Sprintf("%s: %s", code, err.Error()))
	}
	var ipcErr *ipcerr.Error
	if errors.As(err, &ipcErr) {
		return errResult(fmt.Sprintf("%s: %s", ipcErr.Code, ipcErr.Message))
	}
	return nil, nil, err
}

// maskedToolError is toolError's own sibling for a query executed against a connection with
// active mask rules: a Postgres/MySQL driver error routinely embeds the value
// that failed — a type-cast error names the literal it couldn't parse, for instance — so passing
// the adapter's message straight through leaked the real, unmasked value one row at a time (e.g.
// SELECT email::int FROM customers LIMIT 1 OFFSET n). The code (whatever adapters.CodeOf/ipcerr
// already parsed out) is kept, since it carries no row data; the message is replaced with a fixed,
// value-free description. An internal fault (neither an *adapters.Error nor an *ipcerr.Error)
// still returns as a bare Go error, same as toolError — there is no adapter message to withhold.
func maskedToolError(err error) (*mcp.CallToolResult, any, error) {
	if code, ok := adapters.CodeOf(err); ok {
		return errResult(fmt.Sprintf("%s: query failed (message withheld — this connection has active PII masking rules)", code))
	}
	var ipcErr *ipcerr.Error
	if errors.As(err, &ipcErr) {
		return errResult(fmt.Sprintf("%s: query failed (message withheld — this connection has active PII masking rules)", ipcErr.Code))
	}
	return nil, nil, err
}

// --- list_connections' own response shape (§4.1) ---

// connectionCapabilities is list_connections' own "capabilities" object — present only when the
// connection is currently connected (capsOf's own ok=false otherwise omits the whole field).
type connectionCapabilities struct {
	Query         bool `json:"query"`
	Describe      bool `json:"describe"`
	SchemaColumns bool `json:"schemaColumns"`
}

// connectionPermissions is list_connections' own "permissions" object (M2 §6.1) — always present,
// so a client that knows a write will be refused can say so instead of composing one.
type connectionPermissions struct {
	Read  string `json:"read"`
	Write string `json:"write"`
	DDL   string `json:"ddl"`
}

// connectionView is list_connections' own per-connection element.
type connectionView struct {
	ID            string                  `json:"id"`
	Name          string                  `json:"name"`
	Kind          string                  `json:"kind"`
	ReadOnly      bool                    `json:"readOnly"`
	Status        string                  `json:"status"`
	ServerVersion *string                 `json:"serverVersion,omitempty"`
	Capabilities  *connectionCapabilities `json:"capabilities,omitempty"`
	// Description is M2's free-text "what this DB is for", passed verbatim — omitted when unset.
	Description string `json:"description,omitempty"`
	// Permissions is M2's per-operation MCP mode, always present.
	Permissions connectionPermissions `json:"permissions"`
	// Access is what each declared run_query mode allows (accessOf); Enforcement says what backs
	// mode read: session, app or none.
	Access      connectionAccess `json:"access"`
	Enforcement string           `json:"enforcement"`
	// StatementTimeout is the run_query timeout: seconds (0 off) and what stops a statement per mode.
	StatementTimeout statementTimeoutView `json:"statementTimeout"`
	// ExpensiveQuery is the run_query row-estimate check: the app-wide threshold and whether this connection is checked.
	ExpensiveQuery expensiveQueryView `json:"expensiveQuery"`
	// MaskedColumns is "table.column: kind" per rule, sorted, capped at 50 with a trailing
	// "+N more" entry — so a client that does not know a column is masked does not misread a bucket
	// string (e.g. "[1000-10000)") as a literal value. Omitted when the connection
	// has no rules at all.
	MaskedColumns []string `json:"maskedColumns,omitempty"`
	// Tables is the table allow list (P532): all, or the exact tables MCP may touch.
	Tables *allowedTablesView `json:"tables"`
	// ColumnReview says whether new columns are withheld until the user reviews masking (P534). Omitted for
	// kinds without column review.
	ColumnReview *columnReviewView `json:"columnReview,omitempty"`
}

// columnReviewView is list_connections' column review summary, from cached state only.
type columnReviewView struct {
	On bool `json:"on"`
	// Withheld is the columns results drop, capped at maskedColumnsMaxListed; More counts the rest.
	Withheld   []string `json:"withheld,omitempty"`
	More       int      `json:"more,omitempty"`
	Incomplete bool     `json:"incomplete,omitempty"`
}

func (s *Server) columnReviewViewOf(c model.ConnectionSummary) *columnReviewView {
	if !maskreview.Applies(c.Kind) {
		return nil
	}
	v := &columnReviewView{On: c.MaskReviewOn}
	if !v.On {
		return v
	}
	w, ok := s.cfg.ColumnReview.Cached(c.ID)
	if !ok {
		return v
	}
	v.Incomplete = w.Incomplete != ""
	v.Withheld = w.Pending
	if len(v.Withheld) > maskedColumnsMaxListed {
		v.More = len(v.Withheld) - maskedColumnsMaxListed
		v.Withheld = v.Withheld[:maskedColumnsMaxListed]
	}
	return v
}

// maskedColumnsMaxListed is §4.5's own cap — 50 entries, then a trailing "+N more" summary rather
// than an unbounded list for a connection with hundreds of masked columns.
const maskedColumnsMaxListed = 50

// maskedColumnsFor formats rules as list_connections' own "table.column: kind" strings, sorted,
// capped per maskedColumnsMaxListed.
func maskedColumnsFor(rules []model.MaskRule) []string {
	if len(rules) == 0 {
		return nil
	}
	entries := make([]string, len(rules))
	for i, r := range rules {
		entries[i] = fmt.Sprintf("%s.%s: %s", r.TableName, r.ColumnName, r.Kind)
	}
	sort.Strings(entries)
	if len(entries) <= maskedColumnsMaxListed {
		return entries
	}
	out := append([]string{}, entries[:maskedColumnsMaxListed]...)
	out = append(out, fmt.Sprintf("+%d more", len(entries)-maskedColumnsMaxListed))
	return out
}

// --- run_query's own projection — a projection because rows cross to the frontend as
// FlatBuffers (page.Chunk), so no existing JSON row shape exists to reuse. One entry point
// (renderPage) so the anonymization filter has exactly one place to insert itself, between
// Query.Execute and this projection. ---

type tabularColumn struct {
	Name      string `json:"name"`
	DataType  string `json:"dataType"`
	TypeClass string `json:"typeClass"`
}

// planEnvelope is embedded in each result struct so `plan` inlines into the same JSON object the
// client already reads — run_query's response shape is otherwise unchanged. Omitted
// entirely when no plan was produced (omitempty on a nil pointer), never a `"plan": null` a
// client might read as "planned, found nothing".
type planEnvelope struct {
	Plan *planSummary `json:"plan,omitempty"`
	// AdditionalStatementResults/Note are withAdditionalStatementResultsNote's own fields, set
	// directly here rather than through a marshal/unmarshal/remarshal round trip — omitempty keeps
	// run_query's ordinary (single-statement) response shape byte-for-byte unchanged from before
	// this field existed.
	AdditionalStatementResults int    `json:"additionalStatementResults,omitempty"`
	Note                       string `json:"note,omitempty"`
}

// setAdditionalStatementResultsNote is withAdditionalStatementResultsNote's own per-result setter,
// promoted to every concrete *Result type below through planEnvelope's embedding.
func (e *planEnvelope) setAdditionalStatementResultsNote(extraPages int) {
	e.AdditionalStatementResults = extraPages
	e.Note = fmt.Sprintf(
		"this call ran %d additional statement(s) beyond the one shown here; only the first statement's result is returned",
		extraPages,
	)
}

type tabularResult struct {
	Kind           string          `json:"kind"`
	Columns        []tabularColumn `json:"columns"`
	Rows           [][]*string     `json:"rows"`
	RowCount       int             `json:"rowCount"`
	Returned       int             `json:"returned"`
	TruncatedCells int             `json:"truncatedCells"`
	Truncated      bool            `json:"truncated,omitempty"`
	// WithheldColumns names the result columns dropped because the user has not reviewed their masking
	// (P534). Names only, never values.
	WithheldColumns []string `json:"withheldColumns,omitempty"`
	WithheldNote    string   `json:"withheldNote,omitempty"`
	planEnvelope
}

type documentEntry struct {
	ID   *string `json:"id"`
	Body *string `json:"body"`
}

type documentResult struct {
	Kind      string          `json:"kind"`
	Documents []documentEntry `json:"documents"`
	RowCount  int             `json:"rowCount"`
	Returned  int             `json:"returned"`
	Truncated bool            `json:"truncated,omitempty"`
	planEnvelope
}

type keyValueEntry struct {
	Field *string `json:"field"`
	Value *string `json:"value"`
}

type keyValueResult struct {
	Kind        string          `json:"kind"`
	RedisType   string          `json:"redisType"`
	TTLMs       *int64          `json:"ttlMs,omitempty"`
	MemoryBytes *int64          `json:"memoryBytes,omitempty"`
	Entries     []keyValueEntry `json:"entries"`
	RowCount    int             `json:"rowCount"`
	Returned    int             `json:"returned"`
	Truncated   bool            `json:"truncated,omitempty"`
	planEnvelope
}

type streamMessage struct {
	Key       *string `json:"key"`
	Timestamp *string `json:"timestamp"`
	Headers   *string `json:"headers"`
	Attrs     *string `json:"attrs"`
	Body      *string `json:"body"`
}

type streamResult struct {
	Kind      string          `json:"kind"`
	Messages  []streamMessage `json:"messages"`
	RowCount  int             `json:"rowCount"`
	Returned  int             `json:"returned"`
	Truncated bool            `json:"truncated,omitempty"`
	planEnvelope
}

// cellAt returns row's own text from chunk, or nil for SQL NULL — a NULL row and an empty string
// both have Offsets[i] == Offsets[i+1], and only the Nulls bitset (page.IsNull) tells them apart.
// Getting this backwards renders NULL as "" — valid JSON a model would reason over incorrectly,
// with nothing failing anywhere.
func cellAt(chunk page.Chunk, row int) *string {
	if page.IsNull(chunk, row) {
		return nil
	}
	text := page.CellText(chunk, row)
	return &text
}

// statementIdentifierWords splits statement into its word-shaped tokens (letters/digits/
// underscore runs), lowercased — the same notion of "word" classify.go's own
// sqlWordBoundaryContainsAny uses, applied here so a masked column name is matched by whole
// identifier, never as a substring of an unrelated longer one (e.g. "email" must not match inside
// "emails_sent").
func statementIdentifierWords(statement string) map[string]bool {
	words := map[string]bool{}
	for _, f := range strings.FieldsFunc(statement, func(r rune) bool {
		return !(r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r))
	}) {
		words[strings.ToLower(f)] = true
	}
	return words
}

// maskedColumnRenamedOrHidden reports the first masked column name (M5 §4.2's Set.Rules, already
// lowercased) that statement's own text mentions by word but that the actual result set (columns)
// carries no exact-name column for — i.e. an alias (`SELECT email AS e`), a wrapping expression
// (`SELECT lower(email)`), or a subquery plausibly renamed or transformed it. Without this check,
// columnRules's exact-name-only match sees nothing to mask on that result column and the real
// value returns unmasked (finding #3, M6) — under-masking, the opposite of this design's stated
// over-mask-on-ambiguity intent (mask.Set.RuleFor's own doc comment).
//
// An exact-name column being present is not by itself proof every mention of that column was
// rendered under a name columnRules can match (finding #1, M7): `SELECT email, email AS leak FROM
// customers` carries "email" under its own name *and* a second, differently-named projection of
// the same column — the first occurrence must not excuse the second. maskColumnRenamedViaAlias
// catches that second case once the masked name is confirmed present.
//
// Residual, undetectable by a text scanner over the outer statement alone: a pre-existing view
// that itself renames a masked column (`CREATE VIEW v AS SELECT email AS e ...; SELECT e FROM v`)
// mentions the masked column's real name nowhere in the statement this function sees — the view
// definition isn't visible here. Documented as a known open item rather than chased further.
// The third return, viaAlias, distinguishes the two refusal shapes for the caller's error message:
// true when the masked column is present under its own name yet also re-projected under another
// (an actual rename/transform this scan found evidence of); false when it is simply absent from
// the result set but mentioned somewhere in the statement — a WHERE/JOIN filter is the common,
// entirely legitimate case, indistinguishable here from an actual rename this text scan can't see
// through, so both are refused, but only the alias case may claim a rename or transform.
func maskedColumnRenamedOrHidden(statement string, mk *maskset, columns []page.ColumnDescriptor) (name string, hidden, viaAlias bool) {
	if mk == nil || mk.Empty() || statement == "" {
		return "", false, false
	}
	present := make(map[string]bool, len(columns))
	for _, c := range columns {
		lower := strings.ToLower(c.Name)
		present[lower] = true
	}
	words := statementIdentifierWords(statement)
	// names is sorted: the error message is stable despite map iteration order.
	for _, name := range mk.names() {
		if !present[name] {
			if words[name] {
				return name, true, false
			}
			continue
		}
		if maskColumnRenamedViaAlias(statement, name) || setOpMovesColumn(tokenizeSQL(statement), name) {
			return name, true, true
		}
	}
	return "", false, false
}

// maskColumnRenamedViaAlias reports whether statement projects the masked column name (already
// present in the result under its own name) in any form other than a bare item: inside a function
// call or operator expression, or followed by an alias with or without AS, quoted or not. Such a
// second projection comes back under a name columnRules cannot match, so the real value would
// return unmasked (`SELECT email, email leak`, `SELECT email, lower(email)`). WHERE/JOIN/ORDER BY
// occurrences are fine. A bare item is the name, optionally qualified, between SELECT/`,` and
// `,`/FROM/end of its list.
func maskColumnRenamedViaAlias(statement, name string) bool {
	toks := tokenizeSQL(statement)
	stack := []projLevel{{}}
	for i, t := range toks {
		stack = advanceProjStack(stack, t)
		if t.text != name || t.kind == 'p' || tokIsPunct(toks, i+1, ".") || tokIsPunct(toks, i+1, "(") {
			continue
		}
		l := stack[len(stack)-1]
		if !l.proj {
			continue
		}
		if l.wrapped || !isBareProjection(toks, i) {
			return true
		}
	}
	return false
}

var setOpWords = map[string]bool{"union": true, "intersect": true, "except": true}

// setOpMovesColumn reports whether a UNION/INTERSECT/EXCEPT in toks (at any paren depth) has
// branches that project the bare column name at different ordinals. Set-operation results take the
// first branch's column names, so a later branch selecting a masked column under another
// position's name lands unmasked (`SELECT name, email ... UNION ALL SELECT email, name ...`).
func setOpMovesColumn(toks []sqlToken, name string) bool {
	branches := splitTopLevel(toks, func(t sqlToken) bool { return t.kind == 'w' && setOpWords[t.text] })
	var ref []int
	for i, b := range branches {
		b = unwrapParens(b)
		if len(branches) > 1 {
			ords := bareProjectionOrdinals(b, name)
			if i == 0 {
				ref = ords
			} else if !slices.Equal(ref, ords) {
				return true
			}
		}
		for _, inner := range parenGroups(b) {
			if setOpMovesColumn(inner, name) {
				return true
			}
		}
	}
	return false
}

// splitTopLevel splits toks at paren-depth-0 tokens matching sep, dropping the separators.
func splitTopLevel(toks []sqlToken, sep func(sqlToken) bool) [][]sqlToken {
	var out [][]sqlToken
	depth, start := 0, 0
	for i, t := range toks {
		switch {
		case t.kind == 'p' && t.text == "(":
			depth++
		case t.kind == 'p' && t.text == ")":
			depth--
		case depth == 0 && sep(t):
			out = append(out, toks[start:i])
			start = i + 1
		}
	}
	return append(out, toks[start:])
}

// unwrapParens strips parentheses that enclose the whole of toks.
func unwrapParens(toks []sqlToken) []sqlToken {
	for len(toks) >= 2 && tokIsPunct(toks, 0, "(") && tokIsPunct(toks, len(toks)-1, ")") {
		g := parenGroups(toks)
		if len(g) != 1 || len(g[0]) != len(toks)-2 {
			break
		}
		toks = g[0]
	}
	return toks
}

// parenGroups returns the contents of each depth-0 parenthesised group in toks.
func parenGroups(toks []sqlToken) [][]sqlToken {
	var out [][]sqlToken
	depth, start := 0, 0
	for i, t := range toks {
		switch {
		case t.kind == 'p' && t.text == "(":
			if depth == 0 {
				start = i + 1
			}
			depth++
		case t.kind == 'p' && t.text == ")" && depth > 0:
			depth--
			if depth == 0 {
				out = append(out, toks[start:i])
			}
		}
	}
	return out
}

// bareProjectionOrdinals lists the ordinals of select-list items in one branch that are exactly
// the column name, optionally qualified.
func bareProjectionOrdinals(branch []sqlToken, name string) []int {
	sel := -1
	depth := 0
	for i, t := range branch {
		switch {
		case t.kind == 'p' && t.text == "(":
			depth++
		case t.kind == 'p' && t.text == ")":
			depth--
		case depth == 0 && t.kind == 'w' && t.text == "select":
			sel = i
		}
		if sel >= 0 {
			break
		}
	}
	if sel < 0 {
		return nil
	}
	list := branch[sel+1:]
	for len(list) > 0 && tokIsWord(list, 0, "distinct", "all") {
		list = list[1:]
	}
	end, depth := len(list), 0
	for i, t := range list {
		switch {
		case t.kind == 'p' && t.text == "(":
			depth++
		case t.kind == 'p' && t.text == ")":
			depth--
		case depth == 0 && t.kind == 'w' && endsProjection[t.text]:
			end = i
		}
		if end != len(list) {
			break
		}
	}
	var ords []int
	for i, item := range splitTopLevel(list[:end], func(t sqlToken) bool { return t.kind == 'p' && t.text == "," }) {
		n := len(item)
		if n > 0 && item[n-1].kind == 'w' && item[n-1].text == name && (n == 1 || (n == 3 && tokIsPunct(item, 1, "."))) {
			ords = append(ords, i)
		}
	}
	return ords
}

// projLevel is one paren depth of maskColumnRenamedViaAlias's scan: proj when inside a projection
// list, wrapped when that list feeds a function call or expression, sel once a SELECT opened it.
type projLevel struct{ proj, wrapped, sel bool }

func advanceProjStack(stack []projLevel, t sqlToken) []projLevel {
	top := &stack[len(stack)-1]
	switch {
	case t.kind == 'p' && t.text == "(":
		return append(stack, projLevel{proj: top.proj, wrapped: top.proj || top.wrapped})
	case t.kind == 'p' && t.text == ")":
		if len(stack) > 1 {
			return stack[:len(stack)-1]
		}
	case t.kind == 'w' && (t.text == "select" || t.text == "returning"):
		top.proj, top.sel = true, true
	case t.kind == 'w' && top.sel && endsProjection[t.text]:
		top.proj = false
	}
	return stack
}

func tokIsPunct(toks []sqlToken, i int, p string) bool {
	return i >= 0 && i < len(toks) && toks[i].kind == 'p' && toks[i].text == p
}

func tokIsWord(toks []sqlToken, i int, ws ...string) bool {
	if i < 0 || i >= len(toks) || toks[i].kind != 'w' {
		return false
	}
	for _, w := range ws {
		if toks[i].text == w {
			return true
		}
	}
	return false
}

// isBareProjection reports whether toks[i] (optionally qualified) is a whole projection item.
func isBareProjection(toks []sqlToken, i int) bool {
	j := i
	for tokIsPunct(toks, j-1, ".") && j-2 >= 0 && toks[j-2].kind != 'p' {
		j -= 2
	}
	prevOK := tokIsPunct(toks, j-1, ",") || tokIsWord(toks, j-1, "select", "distinct", "all", "returning")
	nextOK := i+1 == len(toks) || tokIsPunct(toks, i+1, ",") || tokIsPunct(toks, i+1, ")") ||
		tokIsPunct(toks, i+1, ";") || tokIsWord(toks, i+1, "from", "into")
	return prevOK && nextOK
}

var endsProjection = map[string]bool{
	"from": true, "where": true, "group": true, "having": true, "order": true, "limit": true,
	"offset": true, "union": true, "intersect": true, "except": true, "window": true,
	"qualify": true, "fetch": true, "into": true,
}

type sqlToken struct {
	kind byte // 'w' word or quoted identifier, 'p' punctuation
	text string
}

// tokenizeSQL splits statement into lowercased words, quoted identifiers and single-rune
// punctuation, dropping whitespace, comments and string literals.
func tokenizeSQL(statement string) []sqlToken {
	var toks []sqlToken
	rs := []rune(statement)
	for i := 0; i < len(rs); {
		r := rs[i]
		switch {
		case unicode.IsSpace(r):
			i++
		case r == '-' && i+1 < len(rs) && rs[i+1] == '-':
			for i < len(rs) && rs[i] != '\n' {
				i++
			}
		case r == '/' && i+1 < len(rs) && rs[i+1] == '*':
			i += 2
			for i < len(rs) && !(rs[i] == '*' && i+1 < len(rs) && rs[i+1] == '/') {
				i++
			}
			i += 2
		case r == '\'':
			i = skipQuoted(rs, i, '\'')
		case r == '"' || r == '`' || r == '[':
			closer := r
			if r == '[' {
				closer = ']'
			}
			end := skipQuoted(rs, i, closer)
			inner := rs[i+1 : max(i+1, end-1)]
			toks = append(toks, sqlToken{'w', strings.ToLower(string(inner))})
			i = end
		case r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r):
			j := i
			for j < len(rs) && (rs[j] == '_' || unicode.IsLetter(rs[j]) || unicode.IsDigit(rs[j])) {
				j++
			}
			toks = append(toks, sqlToken{'w', strings.ToLower(string(rs[i:j]))})
			i = j
		default:
			toks = append(toks, sqlToken{'p', string(r)})
			i++
		}
	}
	return toks
}

// skipQuoted returns the index after the quoted run opened at rs[start], treating a doubled
// closer as an escape.
func skipQuoted(rs []rune, start int, closer rune) int {
	i := start + 1
	for i < len(rs) {
		if rs[i] == closer {
			if i+1 < len(rs) && rs[i+1] == closer {
				i += 2
				continue
			}
			return i + 1
		}
		i++
	}
	return i
}

// newMaskRenameRefusedError is §4.4's refusal for the aliasing/expression case (finding #3): the
// masked column is present in the result set under its own name AND ALSO re-projected under
// another, actual evidence of a rename or transform this scan found.
func newMaskRenameRefusedError(column string) error {
	return &maskingRefusedError{msg: fmt.Sprintf(
		"this query renames or transforms a masked column (%q) — select it by its own name to see the masked value; the maskless raw form isn't permitted over this connection",
		column,
	)}
}

// newMaskMentionedButAbsentRefusedError is maskedColumnRenamedOrHidden's other refusal shape: the
// masked column is not in the result set at all, but its name appears somewhere in the statement
// — most often a WHERE/JOIN filter with no rename involved. A word-level text scan can't tell that
// apart from a rename this function's other checks failed to catch, so it refuses either way, but
// this message never claims to have found a rename or transform it didn't see (the previous shared
// wording read as a false positive to an AI client filtering on, but not selecting, a masked
// column).
func newMaskMentionedButAbsentRefusedError(column string) error {
	return &maskingRefusedError{msg: fmt.Sprintf(
		"this query references a masked column (%q) that isn't in the result set — refused because a text scan can't tell whether it was filtered, renamed, or transformed; select it by its own name to see the masked value",
		column,
	)}
}

// newColumnUnreviewedRefusedError is the refusal for a statement that renames, transforms, filters or sorts on a
// column the user has not reviewed for masking (P534). The column is named, no value ever.
func newColumnUnreviewedRefusedError(column string, viaAlias bool) error {
	use := "filtered or sorted on"
	if viaAlias {
		use = "renamed, transformed or reordered"
	}
	return &maskingRefusedError{msg: fmt.Sprintf(
		"this connection has a column the user has not reviewed for masking: %s. It cannot be %s until the user reviews it in the connection's Privacy tab",
		column, use,
	)}
}

// riskyResultTypeClasses is the generic guard: a column's
// own name matching a mask rule is not the only way a masked value reaches the client — any
// column whose type can itself carry a whole row or several columns' worth of data bypasses
// name-based matching entirely once the real value is wrapped inside it, with no per-column
// textual trace maskedColumnRenamedOrHidden's word-scan could ever catch: verified real against
// Postgres (`SELECT json_agg(t) FROM (SELECT * FROM customers LIMIT 10) t`, `SELECT row_to_json(c)
// FROM customers c`, `SELECT to_jsonb(c) FROM customers c`, `SELECT c FROM customers c` — the last
// one a composite/row value, which postgres/console.go's own pg_type.typtype check now tags
// TypeClassOther specifically so it lands here too) and ClickHouse (an array/JSON aggregate over
// *). Fails closed, with no "is this actually safe" exception: this project's document/stream page
// refusal (renderPage's DocumentPage/StreamPage cases, above) already takes exactly this stance —
// refused outright on any masked connection, never conditionally.
func riskyResultTypeClasses(columns []page.ColumnDescriptor) (name string, found bool) {
	for _, c := range columns {
		if c.TypeClass == page.TypeClassJSON || c.TypeClass == page.TypeClassOther {
			return c.Name, true
		}
	}
	return "", false
}

func newMaskRiskyColumnRefusedError(column string) error {
	return &maskingRefusedError{msg: fmt.Sprintf(
		"this connection has PII masking rules, and result column %q has a type (JSON, array, composite/row, or another type this app cannot inspect column-by-column) that can carry a whole row or several columns' worth of data — masking rules match by column name and cannot see inside a value like that, so it could hide a masked value unredacted; select individual columns by their real names instead, or remove the rules in the connection's Privacy tab",
		column,
	)}
}

// castTypeNames excludes an ordinary `CAST(x AS numeric(10,2))`/`CAST(x AS varchar(255))` shape
// from riskyPositionalColumnAliasList's own match below — a type name immediately followed by a
// parenthesized precision/scale is extremely common, ordinary SQL, not a FROM-clause column-alias
// list; every entry here is a type keyword that commonly takes a parenthesized argument.
var castTypeNames = map[string]bool{
	"char": true, "character": true, "varchar": true, "nchar": true, "nvarchar": true,
	"decimal": true, "numeric": true, "float": true, "double": true, "real": true,
	"number": true, "bit": true, "interval": true, "time": true, "timestamp": true,
	"varbinary": true, "binary": true, "varying": true,
}

var (
	riskyUnicodeEscapedIdent       = regexp.MustCompile(`(?i)U&"`)
	riskyPositionalColumnAliasList = regexp.MustCompile(`(?i)\bas\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(`)
	riskyClickHouseColumnTransform = regexp.MustCompile(`(?i)\b(apply|columns)\s*\(`)
)

// hasPositionalColumnAliasList reports whether statement contains a FROM-clause column-alias list
// (`FROM t AS a(b, c, d)`, Postgres/standard SQL) — every selected column is renamed positionally
// with no "AS <realname>" textual trace at all, so a masked column reached this way carries no
// evidence maskedColumnRenamedOrHidden's word-scan could ever find.
func hasPositionalColumnAliasList(statement string) bool {
	for _, m := range riskyPositionalColumnAliasList.FindAllStringSubmatch(statement, -1) {
		if !castTypeNames[strings.ToLower(m[1])] {
			return true
		}
	}
	return false
}

// refuseRiskyStatementSyntax is the third guard: three concrete SQL shapes verified against
// real engines that rename or transform selected columns with no per-column textual trace a
// word-scan could catch (a Postgres Unicode-escaped identifier reaching a masked column under a
// name that never appears in the statement as plain text; the FROM-clause positional alias list
// above; ClickHouse's own APPLY(fn)/COLUMNS(...) transforming every column a query selects).
// Refused outright on a masked connection regardless of what the result set's own columns look
// like — the statement's own syntax is the evidence, not the result.
func refuseRiskyStatementSyntax(statement string) (reason string, found bool) {
	if reason, found := adapters.SQLQuotingHazard(statement); found {
		return reason, true
	}
	switch {
	case riskyUnicodeEscapedIdent.MatchString(statement):
		return `a Postgres Unicode-escaped identifier (U&"...")`, true
	case hasPositionalColumnAliasList(statement):
		return "a FROM-clause positional column-alias list (`AS alias(...)`)", true
	case riskyClickHouseColumnTransform.MatchString(statement):
		return "ClickHouse's APPLY(...)/COLUMNS(...) column transform", true
	default:
		return "", false
	}
}

func newMaskRiskySyntaxRefusedError(reason string) error {
	return &maskingRefusedError{msg: fmt.Sprintf(
		"this connection has PII masking rules, and this statement uses %s — a construct this app's column-name-based masking cannot safely see through; rewrite the query to select individual columns by their real names, or remove the rules in the connection's Privacy tab",
		reason,
	)}
}

// newMaskColumnlessKeyValueRefusedError is renderKeyValuePage's own refusal: a
// keyvalue page whose Field values carry no genuine per-entry identifier (page.KeyValuePage's own
// FieldsAreColumns=false — a Redis list/set's synthetic display index, a stream entry id, a
// generic console reply with no per-command shape) can never be checked against a mask rule at
// all: RuleFor/ApplyColumn would never match, passing the real value straight through while
// list_connections still reports the connection as protected. Same fail-closed posture as the
// document/stream page refusal above: refuse outright rather than silently skip masking.
func newMaskColumnlessKeyValueRefusedError() error {
	return &maskingRefusedError{msg: "this connection has PII masking rules, which cannot be applied to this result — it carries no real per-entry field name a mask rule's column could match (e.g. a list/set display index, a stream entry id, or a command reply with no per-command field shape); re-read it through a command whose reply names real fields (e.g. HGET/HMGET/HGETALL on a hash), or remove the rules in the connection's Privacy tab"}
}

// renderPage projects one page.Page into its own JSON-ready envelope, capped at maxRows. plan is
// run_query's own auto-force-explain result (nil when auto-force-explain is off, the statement
// was not explainable, or the plan-only EXPLAIN failed) — now taking a plan summary alongside the
// page. mk is the connection's resolved masking state; nil means no rules on this connection, and
// the projection behaves exactly as it does today. statement is the original SQL text — used only
// to catch a masked column reaching the result set renamed or transformed past columnRules's
// exact-name match; empty for a page kind rendered on some other path that has none to offer.
func renderPage(p page.Page, maxRows int, plan *planSummary, mk *maskset, statement string) (any, error) {
	switch pg := p.(type) {
	case page.TabularPage:
		if mk != nil && !mk.Empty() {
			if reason, found := refuseRiskyStatementSyntax(statement); found {
				return nil, newMaskRiskySyntaxRefusedError(reason)
			}
			if name, found := riskyResultTypeClasses(pg.Columns); found {
				return nil, newMaskRiskyColumnRefusedError(name)
			}
		}
		if name, hidden, viaAlias := maskedColumnRenamedOrHidden(statement, mk, pg.Columns); hidden {
			if mk.withheld[name] {
				return nil, newColumnUnreviewedRefusedError(name, viaAlias)
			}
			if viaAlias {
				return nil, newMaskRenameRefusedError(name)
			}
			return nil, newMaskMentionedButAbsentRefusedError(name)
		}
		r := renderTabularPage(pg, maxRows, mk)
		r.Plan = plan
		return r, nil
	case page.DocumentPage:
		if mk != nil && !mk.Empty() {
			return nil, newMaskingRefusedError("document", "a document body")
		}
		r := renderDocumentPage(pg, maxRows)
		r.Plan = plan
		return r, nil
	case page.KeyValuePage:
		if mk != nil && !mk.Empty() && !pg.FieldsAreColumns {
			return nil, newMaskColumnlessKeyValueRefusedError()
		}
		r := renderKeyValuePage(pg, maxRows, mk)
		r.Plan = plan
		return r, nil
	case page.StreamPage:
		if mk != nil && !mk.Empty() {
			return nil, newMaskingRefusedError("stream", "a stream message body")
		}
		r := renderStreamPage(pg, maxRows)
		r.Plan = plan
		return r, nil
	default:
		// §5.2: "implement it rather than panicking on the fourth arm of a type switch" — reachable
		// only if a future adapter kind gains Caps().SQL with a page kind this projection has not
		// been taught yet. A clear internal error beats a silent wrong render.
		return nil, fmt.Errorf("dbmcp: render: unhandled page kind %T", p)
	}
}

// withAdditionalStatementResultsNote marks rendered (a run_query result, always one of the
// concrete *Result structs above) with the count of further statement results the adapter
// produced but this call did not render — e.g. `args.SQL` held more than one
// `;`-separated statement, and only the first one's page is ever projected. Without this, extra
// statements ran (a write among them, possibly) with no sign in the response that anything but
// the shown result happened.
//
// Sets planEnvelope's own fields directly on rendered's concrete type instead of
// a marshal/unmarshal/remarshal round trip through a generic map: the previous shape serialized
// the whole result three times over — once here to get JSON bytes, once back into a map to add two
// fields, and a third time in jsonResult's own final Marshal — real, avoidable cost proportional to
// the result's own size (a run_query response can carry up to 2000 rows). A type switch over the
// four concrete result types renderPage can produce replaces that with one direct field write.
func withAdditionalStatementResultsNote(rendered any, extraPages int) any {
	switch r := rendered.(type) {
	case tabularResult:
		r.setAdditionalStatementResultsNote(extraPages)
		return r
	case documentResult:
		r.setAdditionalStatementResultsNote(extraPages)
		return r
	case keyValueResult:
		r.setAdditionalStatementResultsNote(extraPages)
		return r
	case streamResult:
		r.setAdditionalStatementResultsNote(extraPages)
		return r
	default:
		// Unreached in practice — renderPage's own return type is always one of the four above —
		// kept so a future fifth result type fails safe (the note simply doesn't attach) rather
		// than panicking.
		return rendered
	}
}

func cappedReturned(rowCount, maxRows int) int {
	if rowCount > maxRows {
		return maxRows
	}
	return rowCount
}

// columnRules resolves columns against mk once — §4.2's own cost rule ("the lookup is per column,
// once per call, never per row"). A nil mk or one with no matching column yields a nil slice, so
// the row loop below pays one nil check per cell and nothing more.
func columnRules(columns []page.ColumnDescriptor, mk *maskset) []*mask.Rule {
	if mk == nil || mk.Empty() {
		return nil
	}
	rules := make([]*mask.Rule, len(columns))
	matched := false
	for i, c := range columns {
		if r, ok := mk.RuleFor(c.Name); ok {
			rr := r
			rules[i] = &rr
			matched = true
		}
	}
	if !matched {
		return nil
	}
	return rules
}

// keptColumns splits the result columns into the indexes kept and the distinct names withheld (P534).
func keptColumns(columns []page.ColumnDescriptor, mk *maskset) (keep []int, withheld []string) {
	keep = make([]int, 0, len(columns))
	seen := map[string]bool{}
	for i, c := range columns {
		lower := strings.ToLower(c.Name)
		if mk == nil || !mk.withheld[lower] {
			keep = append(keep, i)
			continue
		}
		if !seen[lower] {
			seen[lower] = true
			withheld = append(withheld, c.Name)
		}
	}
	return keep, withheld
}

// withheldNoteFor is the agent-facing reason a column is missing from a result.
func withheldNoteFor(n int) string {
	if n == 1 {
		return "1 column was withheld: it is new on this connection and the user has not reviewed its masking yet. The user has been notified. Do not try to read it another way."
	}
	return fmt.Sprintf("%d columns were withheld: they are new on this connection and the user has not reviewed their masking yet. The user has been notified. Do not try to read them another way.", n)
}

func renderTabularPage(pg page.TabularPage, maxRows int, mk *maskset) tabularResult {
	returned := cappedReturned(pg.RowCount, maxRows)
	keep, withheld := keptColumns(pg.Columns, mk)
	columns := make([]tabularColumn, len(keep))
	for k, i := range keep {
		c := pg.Columns[i]
		columns[k] = tabularColumn{Name: c.Name, DataType: c.DataType, TypeClass: string(c.TypeClass)}
	}
	rules := columnRules(pg.Columns, mk)
	var masker *mask.Masker
	if rules != nil {
		masker = mk.Masker
		if masker == nil {
			masker = mask.New(nil)
		}
	}
	rows := make([][]*string, returned)
	for r := 0; r < returned; r++ {
		row := make([]*string, len(keep))
		for k, c := range keep {
			v := cellAt(pg.Chunks[c], r)
			if rules != nil && rules[c] != nil {
				v = masker.MaskNullable(*rules[c], v)
			}
			row[k] = v
		}
		rows[r] = row
	}
	res := tabularResult{
		Kind: "tabular", Columns: columns, Rows: rows,
		RowCount: pg.RowCount, Returned: returned, TruncatedCells: pg.TruncatedCells,
		Truncated: returned < pg.RowCount || pg.Position.Truncated,
	}
	if len(withheld) > 0 {
		res.WithheldColumns, res.WithheldNote = withheld, withheldNoteFor(len(withheld))
	}
	return res
}

func renderDocumentPage(pg page.DocumentPage, maxRows int) documentResult {
	returned := cappedReturned(pg.RowCount, maxRows)
	docs := make([]documentEntry, returned)
	for r := 0; r < returned; r++ {
		docs[r] = documentEntry{ID: cellAt(pg.IDs, r), Body: cellAt(pg.Bodies, r)}
	}
	return documentResult{Kind: "document", Documents: docs, RowCount: pg.RowCount, Returned: returned, Truncated: returned < pg.RowCount || pg.Position.Truncated}
}

// renderKeyValuePage also masks: a Redis hash field or an S3 metadata key is column-shaped,
// so a rule whose column_name matches keyValueEntry.Field masks that entry's Value. Resolved per
// entry, not once per call like renderTabularPage's columns — a keyvalue page has no fixed column
// set (Field varies row to row), so there is no per-column list to precompute against.
func renderKeyValuePage(pg page.KeyValuePage, maxRows int, mk *maskset) keyValueResult {
	returned := cappedReturned(pg.RowCount, maxRows)
	entries := make([]keyValueEntry, returned)
	for r := 0; r < returned; r++ {
		field := cellAt(pg.Fields, r)
		value := cellAt(pg.Values, r)
		if mk != nil && !mk.Empty() && field != nil {
			if masked, matched := mk.ApplyColumn(*field, value); matched {
				value = masked
			}
		}
		entries[r] = keyValueEntry{Field: field, Value: value}
	}
	return keyValueResult{
		Kind: "keyvalue", RedisType: pg.RedisType, TTLMs: pg.TTLMs, MemoryBytes: pg.MemoryBytes,
		Entries: entries, RowCount: pg.RowCount, Returned: returned, Truncated: returned < pg.RowCount || pg.Position.Truncated,
	}
}

func renderStreamPage(pg page.StreamPage, maxRows int) streamResult {
	returned := cappedReturned(pg.RowCount, maxRows)
	messages := make([]streamMessage, returned)
	for r := 0; r < returned; r++ {
		messages[r] = streamMessage{
			Key: cellAt(pg.Keys, r), Timestamp: cellAt(pg.Timestamps, r),
			Headers: cellAt(pg.Headers, r), Attrs: cellAt(pg.Attrs, r), Body: cellAt(pg.Bodies, r),
		}
	}
	return streamResult{Kind: "stream", Messages: messages, RowCount: pg.RowCount, Returned: returned, Truncated: returned < pg.RowCount || pg.Position.Truncated}
}
