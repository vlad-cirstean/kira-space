package adapters

import (
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/page"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// This file is the Go analogue of sql-text.ts: the genuinely shared, driver-agnostic glue the SQL
// adapters' read.go and catalog.go modules call. Everything dialect-shaped (quoting, LIMIT syntax,
// catalog SQL) stays in each adapter's own package.

// OrderTerm is one column/direction pair.
type OrderTerm struct {
	Column    string
	Direction string // "asc" | "desc"
}

// QuoteIdentDouble is read.ts's quoteIdent for the double-quote dialects (Postgres, SQLite): NUL
// is unreachable through any real identifier but panics rather than threading an error return
// through every call site, matching each adapter's own note (the recover() boundary turns
// this into a failed op, never a crash).
func QuoteIdentDouble(name string) string {
	if strings.ContainsRune(name, '\x00') {
		panic(Contract("identifier contains a NUL byte", nil))
	}
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// QuoteIdentBacktick is read.ts's quoteIdent for MySQL/MariaDB's backtick dialect.
func QuoteIdentBacktick(name string) string {
	if strings.ContainsRune(name, '\x00') {
		panic(Contract("identifier contains a NUL byte", nil))
	}
	return "`" + strings.ReplaceAll(name, "`", "``") + "`"
}

// ReadReq is adapter.ts's ReadRequest minus Path — the request shape every relational adapter's
// readPage actually consumes.
type ReadReq struct {
	Projection []string
	Filter     *string
	Sort       *model.SortSpec
	PageSize   int
	Cursor     model.PageCursor
	// Conditions are ANDed after Filter; values bind as parameters (P535).
	Conditions []model.Condition
}

// BuildKeysetWhereSQL extends whereSQL with the keyset boundary predicate decoded from the cursor
// token, when the request wants one. addParam appends a value and returns its 1-based
// placeholder index (a "?"-style placeholder ignores the index; a "$N"-style one needs it).
// Token values are cell display text; isBinary names the columns whose "0x<hex>" display text must
// be decoded back to raw bytes before binding, or the predicate compares the column with the ASCII
// of the display text and pages wrongly.
func BuildKeysetWhereSQL(req ReadReq, order EffectiveOrder, fingerprint, whereSQL string, quote func(string) string, placeholder func(int) string, addParam func(any) int, isBinary func(string) bool) (string, error) {
	keyValues, err := DecodePageToken(req.Cursor.Token, fingerprint)
	if err != nil {
		return "", err
	}
	if len(keyValues) != len(order.KeysetColumns) {
		return "", Contract("page token key length does not match the sort key", nil)
	}
	firstIndex := 0
	for i, v := range keyValues {
		var bound any = v
		if isBinary(order.KeysetColumns[i]) {
			decoded, err := DecodeBinaryCellText(v)
			if err != nil {
				return "", New(CodeQuery, "page token key "+order.KeysetColumns[i]+": "+err.Error(), nil)
			}
			bound = decoded
		}
		idx := addParam(bound)
		if i == 0 {
			firstIndex = idx
		}
	}
	quotedKeyColumns := make([]string, len(order.KeysetColumns))
	for i, c := range order.KeysetColumns {
		quotedKeyColumns[i] = quote(c)
	}
	predicate := BuildKeysetPredicate(quotedKeyColumns, order.KeysetDirection, req.Cursor.Mode, firstIndex, placeholder)
	if whereSQL != "" {
		return whereSQL + " AND " + predicate, nil
	}
	return "WHERE " + predicate, nil
}

// BuildPageSQL assembles the final SELECT text: SELECT/FROM, the (already keyset-extended) WHERE,
// ORDER BY and LIMIT/OFFSET, in that order. The LIMIT param is bound before the OFFSET one — params
// must line up with the placeholders left to right for a "?"-style dialect.
func BuildPageSQL(relationSQL, selectList, whereSQL, orderBySQL string, req ReadReq, placeholder func(int) string, addParam func(any) int) string {
	// D24: fetch pageSize + 1 to compute hasMore without a count.
	limitIdx := addParam(req.PageSize + 1)
	offsetSQL := ""
	if req.Cursor.Mode == "offset" {
		idx := addParam(req.Cursor.Offset)
		offsetSQL = " OFFSET " + placeholder(idx)
	}

	sqlParts := []string{"SELECT " + selectList, "FROM " + relationSQL}
	if whereSQL != "" {
		sqlParts = append(sqlParts, whereSQL)
	}
	if orderBySQL != "" {
		sqlParts = append(sqlParts, "ORDER BY "+orderBySQL)
	}
	sqlParts = append(sqlParts, "LIMIT "+placeholder(limitIdx)+offsetSQL)
	return strings.Join(sqlParts, "\n")
}

// NewParamAccumulator returns an addParam closure over its own params slice — the shape
// BuildKeysetWhereSQL/BuildPageSQL expect. A "?"-style placeholder ignores the returned index, so
// only append order (left to right, matching the "?"s in the SQL text) matters for it; a "$N"-style
// one uses the index directly.
func NewParamAccumulator() (addParam func(any) int, params *[]any) {
	var p []any
	return func(v any) int {
		p = append(p, v)
		return len(p)
	}, &p
}

// BuildCountSQL is read.ts's countRows SQL assembly: "SELECT count(*) AS n FROM <relation>
// [WHERE ...]" — every relational adapter's countRows builds this identically.
func BuildCountSQL(relationSQL string, filter *string) string {
	sqlParts := []string{"SELECT count(*) AS n", "FROM " + relationSQL}
	if where := WhereClause(filter); where != "" {
		sqlParts = append(sqlParts, where)
	}
	return strings.Join(sqlParts, "\n")
}

// SetCommand is query.ts's own setCommand: the op-log command text, with bound params appended as
// a trailing JSON comment when the caller opts in (LogParams) and there are any to show.
func SetCommand(op *OpCtx, sqlText string, params []any, logParams bool) {
	if logParams && len(params) > 0 {
		if b, err := json.Marshal(params); err == nil {
			op.SetCommand(sqlText + " -- params: " + string(b))
			return
		}
	}
	op.SetCommand(sqlText)
}

// BuildOrderBy ports sql-text.ts's buildOrderBy.
func BuildOrderBy(terms []OrderTerm, quote func(string) string) string {
	parts := make([]string, len(terms))
	for i, t := range terms {
		parts[i] = quote(t.Column) + " " + strings.ToUpper(t.Direction)
	}
	return strings.Join(parts, ", ")
}

// BuildKeysetPredicate ports sql-text.ts's buildKeysetPredicate: a row-value comparison for a
// keyset boundary, e.g. (col1, col2) > (p1, p2). columns are already quoted identifiers.
// direction/mode select the operator: 'after' compares forward in the requested direction;
// 'before' — having flipped the ORDER BY and reversed the builder — needs the mirror-image
// comparison for that same flip.
func BuildKeysetPredicate(columns []string, direction, mode string, firstParamIndex int, placeholder func(int) string) string {
	operator := "<"
	if (mode == "after") == (direction == "asc") {
		operator = ">"
	}
	rhs := make([]string, len(columns))
	for i := range columns {
		rhs[i] = placeholder(firstParamIndex + i)
	}
	return fmt.Sprintf("(%s) %s (%s)", strings.Join(columns, ", "), operator, strings.Join(rhs, ", "))
}

type pageTokenPayload struct {
	V int      `json:"v"`
	K []string `json:"k"`
	F string   `json:"f"`
}

// EncodePageToken ports sql-text.ts's encodePageToken. base64.RawURLEncoding matches Node's
// unpadded 'base64url' encoding.
func EncodePageToken(key []string, fingerprint string) string {
	payload := pageTokenPayload{V: 1, K: key, F: fingerprint}
	raw, err := json.Marshal(payload)
	if err != nil {
		// payload is a plain struct of strings; json.Marshal cannot fail on it.
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(raw)
}

// DecodePageToken ports sql-text.ts's decodePageToken. Returns E_QUERY when the token is
// malformed or its fingerprint no longer matches.
func DecodePageToken(token, expectedFingerprint string) ([]string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return nil, Contract("malformed page token", err)
	}
	var payload pageTokenPayload
	if err := json.Unmarshal(raw, &payload); err != nil || payload.V != 1 || payload.K == nil {
		return nil, Contract("malformed page token", err)
	}
	if payload.F != expectedFingerprint {
		return nil, T(CodeQuery, "backend.studio.adapter.pageTokenMismatch.body", nil, nil)
	}
	return payload.K, nil
}

// RequestFingerprint ports sql-text.ts's requestFingerprint: sha1 -> hex -> first 16 chars.
// Deterministic within a process is all that is required — a token is only ever decoded by the
// process that minted it.
func RequestFingerprint(parts any) string {
	raw, err := json.Marshal(parts)
	if err != nil {
		panic(err)
	}
	sum := sha1.Sum(raw)
	return hex.EncodeToString(sum[:])[:16]
}

// ResolveProjection ports sql-text.ts's resolveProjection. requested == nil returns the input
// slice itself (identity, not a copy — callers rely on this).
func ResolveProjection(columns []model.ColumnMeta, requested []string) ([]model.ColumnMeta, error) {
	if requested == nil {
		return columns, nil
	}
	byName := make(map[string]model.ColumnMeta, len(columns))
	for _, c := range columns {
		byName[c.Name] = c
	}
	seen := make(map[string]struct{}, len(requested))
	resolved := make([]model.ColumnMeta, 0, len(requested))
	for _, name := range requested {
		if _, dup := seen[name]; dup {
			continue
		}
		seen[name] = struct{}{}
		col, ok := byName[name]
		if !ok {
			return nil, New(CodeNotFound, "unknown column in projection: "+name, nil)
		}
		resolved = append(resolved, col)
	}
	sort.Slice(resolved, func(i, j int) bool { return resolved[i].Position < resolved[j].Position })
	// Belt-and-braces. The renderer now maps every "would end up with zero
	// selected columns" case back to null ("all columns") at its own choke points, but a resolved
	// projection of zero columns has no honest reading, and every SQL adapter's selectList would
	// otherwise become an empty string — SELECT  FROM ... — a syntax error rather than a
	// classified, actionable one.
	if len(resolved) == 0 {
		return nil, Contract("projection selects zero columns", nil)
	}
	return resolved, nil
}

// SafeInt ports sql-text.ts's safeInt. Go's int is already integral, so only the negative check
// survives — app-generated integers only (pageSize+1, a port-validated offset), inlined into SQL
// rather than bound, per each adapter's own note.
func SafeInt(value int, label string) (int, error) {
	if value < 0 {
		return 0, Contract(fmt.Sprintf("invalid %s: %d", label, value), nil)
	}
	return value, nil
}

// WhereClause ports sql-text.ts's whereClause: WHERE (<filter>) or "" — always parenthesised so a
// keyset predicate joined by a bare AND never silently changes the user's own filter's meaning. The
// closing paren sits on its own line, after a newline: a filter ending in a `--` line
// comment (`x = 1 -- note`) would otherwise comment out the closing paren itself — and, for a
// keyset request, everything BuildKeysetWhereSQL appends after it too — breaking every read/count
// against that filter. A `/* */` block comment has no such gap (it terminates itself), only a
// trailing line comment does, since nothing before this function's own appended text can close it.
func WhereClause(filter *string) string {
	if filter == nil || strings.TrimSpace(*filter) == "" {
		return ""
	}
	return "WHERE (" + *filter + "\n)"
}

// ParseCountValue ports sql-text.ts's parseCountValue. int64 rather than float64: count(*) is a
// 64-bit integer and JS's Number was the lossy half of this port, not a design choice to keep.
func ParseCountValue(raw any) (int64, error) {
	switch v := raw.(type) {
	case int64:
		return v, nil
	case int:
		return int64(v), nil
	case string:
		var n int64
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil {
			return n, nil
		}
	}
	return 0, Contract(fmt.Sprintf("count returned a non-numeric result: %v", raw), nil)
}

// PrimaryKeyFromIndexes ports sql-text.ts's primaryKeyFromIndexes. nil when there is none.
func PrimaryKeyFromIndexes(indexes []model.IndexMeta) []string {
	for _, idx := range indexes {
		if idx.Primary {
			return idx.Columns
		}
	}
	return nil
}

// MarkPrimaryKey stamps IsPrimaryKey on each of cols per pk (P113 G1) — postgres and mysqlfamily's
// own Describe both built a pkColumns set from a PrimaryKeyFromIndexes result and re-stamped every
// column against it identically.
func MarkPrimaryKey(cols []model.ColumnMeta, pk []string) []model.ColumnMeta {
	pkColumns := make(map[string]struct{}, len(pk))
	for _, c := range pk {
		pkColumns[c] = struct{}{}
	}
	marked := make([]model.ColumnMeta, len(cols))
	for i, c := range cols {
		_, isPK := pkColumns[c.Name]
		c.IsPrimaryKey = isPK
		marked[i] = c
	}
	return marked
}

// KeyShape is the Go analogue of resolveKeyShape's return object.
type KeyShape struct {
	Columns    []model.ColumnMeta
	PrimaryKey []string
	UniqueKeys [][]string
}

// ResolveKeyShape ports sql-text.ts's resolveKeyShape.
func ResolveKeyShape(raw []model.ColumnMeta, indexes []model.IndexMeta) KeyShape {
	primaryKey := PrimaryKeyFromIndexes(indexes)
	pkColumns := make(map[string]struct{}, len(primaryKey))
	for _, c := range primaryKey {
		pkColumns[c] = struct{}{}
	}
	columns := make([]model.ColumnMeta, len(raw))
	nullableByName := make(map[string]bool, len(raw))
	for i, c := range raw {
		_, isPK := pkColumns[c.Name]
		c.IsPrimaryKey = isPK
		columns[i] = c
		nullableByName[c.Name] = c.Nullable
	}
	var uniqueKeys [][]string
	for _, idx := range indexes {
		if !idx.Unique {
			continue
		}
		allNotNull := true
		for _, c := range idx.Columns {
			if nullableByName[c] {
				allNotNull = false
				break
			}
		}
		if allNotNull {
			uniqueKeys = append(uniqueKeys, idx.Columns)
		}
	}
	return KeyShape{Columns: columns, PrimaryKey: primaryKey, UniqueKeys: uniqueKeys}
}

var trailingSemicolonRE = regexp.MustCompile(`;\s*$`)

// StripOneTrailingSemicolon ports sql-text.ts's stripOneTrailingSemicolon.
func StripOneTrailingSemicolon(text string) string {
	loc := trailingSemicolonRE.FindStringIndex(text)
	if loc == nil {
		return text
	}
	return text[:loc[0]]
}

// JoinConsoleStatements is every console adapter's own op-log join: statements separated by ";\n",
// the same separator "Run all" sent — this is what the Operations panel's re-run (P108 Part 11 F5)
// re-splits with sql-split.ts. A statement whose own last line contains one of lineCommentPrefixes
// (a trailing "--"/"#"/"//" line comment) swallows the very ";\n" separator that follows it on that
// same line — the semicolon reads as part of the comment, and re-splitting merges this statement
// with the next one. Appending an extra "\n" before such a statement's own separator moves the ";"
// onto its own line, out of the comment's reach. Over-triggering on a false positive (the prefix
// text appearing inside a string literal on the last line, not actually a comment) only adds a
// harmless blank line, never changes what re-splits into — so this stays a cheap substring check
// rather than a real lexical scan.
func JoinConsoleStatements(statements []string, lineCommentPrefixes ...string) string {
	parts := make([]string, len(statements))
	for i, stmt := range statements {
		parts[i] = stmt
		if i == len(statements)-1 {
			continue
		}
		lastLine := stmt
		if idx := strings.LastIndexByte(stmt, '\n'); idx >= 0 {
			lastLine = stmt[idx+1:]
		}
		for _, prefix := range lineCommentPrefixes {
			if strings.Contains(lastLine, prefix) {
				parts[i] += "\n"
				break
			}
		}
	}
	return strings.Join(parts, ";\n")
}

// BeginReadOnlyConsole opens a console batch's BEGIN READ ONLY wrap: postgres and
// mysqlfamily's own console.go each hand-rolled this identically apart from the BEGIN SQL text and
// how their own conn type executes a statement and maps its error. exec runs one statement against
// that connection; waitInFlight is the connection's own RunWithAbortRace drain (every
// in-flight background goroutine touching conn must finish before COMMIT); mapErr is the adapter's
// own driver-error mapper.
//
// On success, the returned cleanup must be deferred unconditionally — issuing COMMIT against an
// already-aborted transaction is itself how these engines end one, regardless of the batch's own
// outcome. cleanup is detached from ctx (context.WithoutCancel) so a cancelled op still ends the
// transaction, or the pinned connection is left inside it for whatever op runs next.
func BeginReadOnlyConsole(ctx context.Context, beginSQL string, exec func(context.Context, string) error, waitInFlight func(), mapErr func(error) error) (cleanup func(), err error) {
	if execErr := exec(ctx, beginSQL); execErr != nil {
		return nil, mapErr(execErr)
	}
	return func() {
		waitInFlight()
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = exec(cleanupCtx, "COMMIT")
	}, nil
}

// SingleStatusPage ports sql-text.ts's singleStatusPage: the one-column, one-row "status" page a
// console statement with no result set returns. dataType varies (ClickHouse spells it "String").
func SingleStatusPage(text, dataType string) page.TabularPage {
	columns := []page.ColumnDescriptor{{
		Name: "status", DataType: dataType, TypeClass: page.TypeClassText,
		Nullable: false, IsPrimaryKey: false, Generated: false,
	}}
	builder := page.NewTabularPageBuilder(columns)
	_ = builder.AppendRow([]*string{&text})
	return builder.Finish(page.UnpagedPosition(1))
}

// AssertKeysetSupported ports sql-text.ts's assertKeysetSupported.
func AssertKeysetSupported(wantsKeyset, isTextSort, eligible bool) error {
	if wantsKeyset && (isTextSort || !eligible) {
		return Contract("keyset pagination is unavailable for this sort; the client must use an offset cursor", nil)
	}
	return nil
}

// EffectiveOrder is the Go analogue of sql-text.ts's EffectiveOrder.
type EffectiveOrder struct {
	Terms           []OrderTerm
	KeysetEligible  bool
	KeysetColumns   []string
	KeysetDirection string
}

// ValidateRequestedTerms is ComputeEffectiveOrder's own column-existence and direction check, split
// out to keep that function's own cognitive complexity down (golangci-lint's gocognit), and shared
// with ClickHouse's own sort validation, which has no keyset pipeline of its own
// but needs the identical column-exists/direction check. BuildOrderBy uppercases Direction for
// the ORDER BY text itself (so "ASC"/"Asc" still produce correct SQL there), but the keyset
// comparison further down the pipeline (BuildKeysetPredicate) compares the lowercase literal "asc"
// — an upper-case direction would build the ORDER BY correctly while silently mismatching the
// keyset operator/reversal logic, mispaging. Rejecting anything but the exact lowercase spelling
// here closes that gap at the source, before EffectiveOrder.KeysetDirection is ever set from it.
func ValidateRequestedTerms(terms []OrderTerm, columnByName map[string]model.ColumnMeta) error {
	for _, t := range terms {
		if _, ok := columnByName[t.Column]; !ok {
			return New(CodeNotFound, "unknown column in sort: "+t.Column, nil)
		}
		if t.Direction != "asc" && t.Direction != "desc" {
			return New(CodeQuery, "invalid sort direction: "+t.Direction, nil)
		}
	}
	return nil
}

// ComputeEffectiveOrder ports sql-text.ts's computeEffectiveOrder — the D7 keyset-eligibility rule.
func ComputeEffectiveOrder(sort_ *model.SortSpec, columns []model.ColumnMeta, tiebreaker []string) (EffectiveOrder, error) {
	if sort_ != nil && sort_.Kind == "text" {
		return EffectiveOrder{KeysetDirection: "asc"}, nil
	}

	var requestedTerms []OrderTerm
	if sort_ != nil && sort_.Kind == "structured" {
		for _, t := range sort_.Terms {
			requestedTerms = append(requestedTerms, OrderTerm{Column: t.Column, Direction: t.Direction})
		}
	}
	columnByName := make(map[string]model.ColumnMeta, len(columns))
	for _, c := range columns {
		columnByName[c.Name] = c
	}
	if len(requestedTerms) > 0 {
		if err := ValidateRequestedTerms(requestedTerms, columnByName); err != nil {
			return EffectiveOrder{}, err
		}
	}

	uniform := true
	for _, t := range requestedTerms {
		if t.Direction != requestedTerms[0].Direction {
			uniform = false
			break
		}
	}
	if !uniform {
		return EffectiveOrder{Terms: requestedTerms, KeysetDirection: "asc"}, nil
	}
	direction := "asc"
	if len(requestedTerms) > 0 {
		direction = requestedTerms[0].Direction
	}

	if tiebreaker == nil {
		return EffectiveOrder{Terms: requestedTerms, KeysetDirection: direction}, nil
	}

	// P2 R2 (task #89): a NULL value in a requested sort column breaks keyset comparison —
	// `(col, pk) > (val, pk)` is SQL-UNKNOWN wherever col IS NULL, so NULL rows are silently
	// excluded from every keyset page forever, or (if a page-boundary row itself has NULL) the
	// cursor build hard-fails with "keyset tiebreaker column was NULL". D7 only requires the
	// tiebreaker column to be non-nullable; it never checked the user's own requested sort
	// columns. Falling back to the same non-keyset-eligible shape the no-tiebreaker branch above
	// already returns keeps every downstream consumer (AssertKeysetSupported, the offset-strategy
	// fallback, the renderer's D7 rule) working unchanged — they already handle "not eligible".
	//
	// A primary-key column is exempted even when its own Nullable bit is set: SQLite's
	// `pragma_table_xinfo` reports notnull=0 for a bare `INTEGER PRIMARY KEY` column (the rowid
	// alias) whenever the DDL doesn't spell out an explicit NOT NULL, even though SQLite itself
	// guarantees that column can never actually hold NULL — inserting NULL into it is exactly what
	// assigns a fresh rowid. Postgres/MySQL/MariaDB already report every primary-key column as
	// NOT NULL, so this exemption is a no-op for them; without it, sorting by the single most
	// common column in the whole catalog (the primary key) would wrongly lose keyset pagination on
	// SQLite alone.
	for _, t := range requestedTerms {
		if col, ok := columnByName[t.Column]; ok && col.Nullable && !col.IsPrimaryKey {
			return EffectiveOrder{Terms: requestedTerms, KeysetDirection: direction}, nil
		}
	}

	already := make(map[string]struct{}, len(requestedTerms))
	for _, t := range requestedTerms {
		already[t.Column] = struct{}{}
	}
	terms := append([]OrderTerm{}, requestedTerms...)
	var keysetColumns []string
	for _, t := range requestedTerms {
		keysetColumns = append(keysetColumns, t.Column)
	}
	for _, c := range tiebreaker {
		if _, ok := already[c]; ok {
			continue
		}
		terms = append(terms, OrderTerm{Column: c, Direction: direction})
		keysetColumns = append(keysetColumns, c)
	}
	return EffectiveOrder{
		Terms: terms, KeysetEligible: true, KeysetColumns: keysetColumns, KeysetDirection: direction,
	}, nil
}

// FetchColumns is the Go analogue of sql-text.ts's resolveFetchColumns return object.
type FetchColumns struct {
	Columns         []model.ColumnMeta
	KeysetColumnIdx map[string]int
}

// ResolveFetchColumns ports sql-text.ts's resolveFetchColumns. resolveHidden nil uses the default
// by-name lookup; sqlite passes its own rowid-aware resolver in P58b.
func ResolveFetchColumns(projected, all []model.ColumnMeta, order EffectiveOrder, resolveHidden func(string) (model.ColumnMeta, error)) (FetchColumns, error) {
	projectedNames := make(map[string]struct{}, len(projected))
	for _, c := range projected {
		projectedNames[c.Name] = struct{}{}
	}
	columnByName := make(map[string]model.ColumnMeta, len(all))
	for _, c := range all {
		columnByName[c.Name] = c
	}
	resolve := resolveHidden
	if resolve == nil {
		resolve = func(name string) (model.ColumnMeta, error) {
			col, ok := columnByName[name]
			if !ok {
				return model.ColumnMeta{}, New(CodeQuery, "keyset tiebreaker column not found: "+name, nil)
			}
			return col, nil
		}
	}
	fetchColumns := append([]model.ColumnMeta{}, projected...)
	if order.KeysetEligible {
		for _, name := range order.KeysetColumns {
			if _, already := projectedNames[name]; already {
				continue
			}
			col, err := resolve(name)
			if err != nil {
				return FetchColumns{}, err
			}
			fetchColumns = append(fetchColumns, col)
		}
	}
	keysetColumnIdx := make(map[string]int, len(order.KeysetColumns))
	for _, name := range order.KeysetColumns {
		idx := -1
		for i, c := range fetchColumns {
			if c.Name == name {
				idx = i
				break
			}
		}
		keysetColumnIdx[name] = idx
	}
	return FetchColumns{Columns: fetchColumns, KeysetColumnIdx: keysetColumnIdx}, nil
}

// BuildScanOrderBy ports sql-text.ts's buildScanOrderBy: a text sort verbatim, else the effective
// terms with every direction flipped when the fetch runs backwards for a 'before' cursor.
func BuildScanOrderBy(sort_ *model.SortSpec, order EffectiveOrder, reverseRows bool, quote func(string) string) string {
	if sort_ != nil && sort_.Kind == "text" {
		return sort_.Text
	}
	if len(order.Terms) == 0 {
		return ""
	}
	scanTerms := order.Terms
	if reverseRows {
		scanTerms = make([]OrderTerm, len(order.Terms))
		for i, t := range order.Terms {
			d := "asc"
			if t.Direction == "asc" {
				d = "desc"
			}
			scanTerms[i] = OrderTerm{Column: t.Column, Direction: d}
		}
	}
	return BuildOrderBy(scanTerms, quote)
}

// KeysetPositionArgs is the Go analogue of sql-text.ts's buildKeysetPosition argument object.
// A8: DisplayRowCount + CellAt replaces the TS generic Row type — the same dependency (a fetched
// row's cell as text), spelled without a type parameter every caller would otherwise have to name.
type KeysetPositionArgs struct {
	Cursor          model.PageCursor
	PageSize        int
	DisplayRowCount int
	ProbedExtra     bool
	Order           EffectiveOrder
	KeysetColumnIdx map[string]int
	Fingerprint     string
	CellAt          func(row, col int) *string
}

// keysetValueAt reads the keyset tiebreaker values for one displayed row, erroring if any
// tiebreaker column came back NULL — a keyset token cannot be built from a NULL component.
func keysetValueAt(args KeysetPositionArgs, row int) ([]string, error) {
	values := make([]string, len(args.Order.KeysetColumns))
	for i, name := range args.Order.KeysetColumns {
		idx, ok := args.KeysetColumnIdx[name]
		var v *string
		if ok && idx >= 0 {
			v = args.CellAt(row, idx)
		}
		if v == nil {
			return nil, Contract(fmt.Sprintf("keyset tiebreaker column %q was NULL", name), nil)
		}
		values[i] = *v
	}
	return values, nil
}

// nextPrevTokens builds the next/prev keyset tokens for a keyset-eligible, non-empty page,
// following the same forward/backward availability rule per cursor mode BuildKeysetPosition always
// used.
func nextPrevTokens(args KeysetPositionArgs, rowCount int) (nextToken, prevToken *string, err error) {
	hasForward := args.ProbedExtra
	if args.Cursor.Mode == "before" {
		hasForward = true
	}
	var hasBackward bool
	switch args.Cursor.Mode {
	case "before":
		hasBackward = args.ProbedExtra
	case "after":
		hasBackward = true
	default:
		hasBackward = args.Cursor.Offset > 0
	}
	if hasForward {
		values, err := keysetValueAt(args, rowCount-1)
		if err != nil {
			return nil, nil, err
		}
		token := EncodePageToken(values, args.Fingerprint)
		nextToken = &token
	}
	if hasBackward {
		values, err := keysetValueAt(args, 0)
		if err != nil {
			return nil, nil, err
		}
		token := EncodePageToken(values, args.Fingerprint)
		prevToken = &token
	}
	return nextToken, prevToken, nil
}

// BuildKeysetPosition ports sql-text.ts's buildKeysetPosition — D7's whole forward-and-backward
// token rule.
func BuildKeysetPosition(args KeysetPositionArgs) (page.PagePosition, error) {
	rowCount := args.DisplayRowCount

	strategy := "offset"
	if args.Order.KeysetEligible {
		strategy = "keyset"
	}

	hasMore := false
	if rowCount != 0 {
		if args.Cursor.Mode == "before" {
			hasMore = true
		} else {
			hasMore = args.ProbedExtra
		}
	}

	var nextToken, prevToken *string
	if args.Order.KeysetEligible && rowCount > 0 {
		var err error
		nextToken, prevToken, err = nextPrevTokens(args, rowCount)
		if err != nil {
			return page.PagePosition{}, err
		}
	}

	var offset *int
	if args.Cursor.Mode == "offset" {
		o := args.Cursor.Offset
		offset = &o
	}

	return page.PagePosition{
		Offset: offset, PageSize: args.PageSize, HasMore: hasMore,
		NextToken: nextToken, PrevToken: prevToken, Strategy: strategy,
	}, nil
}
