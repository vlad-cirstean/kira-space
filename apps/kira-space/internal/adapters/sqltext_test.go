package adapters

import (
	"errors"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
)

// Ported one for one from tests/unit/sql-text.spec.ts — see that file's own header for
// why these cases exist (computeEffectiveOrder/decodePageToken are reached by seven adapters and
// named in no live-server spec; the eligible-and-correct path is the only one Testcontainers-backed
// suites ever exercise).

func col(name string, position int, isPrimaryKey bool) model.ColumnMeta {
	return model.ColumnMeta{Name: name, Position: position, DataType: "int4", Nullable: false, IsPrimaryKey: isPrimaryKey}
}

func nullableCol(name string, position int) model.ColumnMeta {
	return model.ColumnMeta{Name: name, Position: position, DataType: "int4", Nullable: true, IsPrimaryKey: false}
}

func testColumns() []model.ColumnMeta {
	return []model.ColumnMeta{col("id", 0, true), col("name", 1, false), col("created_at", 2, false)}
}

func structuredSort(terms ...model.SortTerm) *model.SortSpec {
	return &model.SortSpec{Kind: "structured", Terms: terms}
}

func textSort(text string) *model.SortSpec {
	return &model.SortSpec{Kind: "text", Text: text}
}

// 1. a text sort is never keyset-eligible
func TestComputeEffectiveOrder_TextSortIneligible(t *testing.T) {
	got, err := ComputeEffectiveOrder(textSort("name asc"), testColumns(), []string{"id"})
	if err != nil {
		t.Fatal(err)
	}
	want := EffectiveOrder{Terms: nil, KeysetEligible: false, KeysetColumns: nil, KeysetDirection: "asc"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

// 2. mixed sort directions disqualify keyset, but keep both requested terms
func TestComputeEffectiveOrder_MixedDirectionsDisqualify(t *testing.T) {
	sort := structuredSort(
		model.SortTerm{Column: "name", Direction: "asc"},
		model.SortTerm{Column: "created_at", Direction: "desc"},
	)
	got, err := ComputeEffectiveOrder(sort, testColumns(), []string{"id"})
	if err != nil {
		t.Fatal(err)
	}
	if got.KeysetEligible {
		t.Error("expected KeysetEligible = false")
	}
	if len(got.KeysetColumns) != 0 {
		t.Errorf("expected no keyset columns, got %v", got.KeysetColumns)
	}
	want := []OrderTerm{{Column: "name", Direction: "asc"}, {Column: "created_at", Direction: "desc"}}
	if !reflect.DeepEqual(got.Terms, want) {
		t.Errorf("got terms %v, want %v", got.Terms, want)
	}
}

// 3. an absent tiebreaker disqualifies keyset but keeps the requested direction
func TestComputeEffectiveOrder_NoTiebreaker(t *testing.T) {
	sort := structuredSort(model.SortTerm{Column: "name", Direction: "desc"})
	got, err := ComputeEffectiveOrder(sort, testColumns(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.KeysetEligible {
		t.Error("expected KeysetEligible = false")
	}
	if got.KeysetDirection != "desc" {
		t.Errorf("got direction %q, want desc", got.KeysetDirection)
	}
}

// 4. the tiebreaker is appended in the requested direction, deduping a column already sorted by
func TestComputeEffectiveOrder_TiebreakerAppendedDeduped(t *testing.T) {
	sort := structuredSort(model.SortTerm{Column: "name", Direction: "desc"})
	got, err := ComputeEffectiveOrder(sort, testColumns(), []string{"name", "id"})
	if err != nil {
		t.Fatal(err)
	}
	if !got.KeysetEligible {
		t.Fatal("expected KeysetEligible = true")
	}
	want := []OrderTerm{{Column: "name", Direction: "desc"}, {Column: "id", Direction: "desc"}}
	if !reflect.DeepEqual(got.Terms, want) {
		t.Errorf("got terms %v, want %v", got.Terms, want)
	}
	if !reflect.DeepEqual(got.KeysetColumns, []string{"name", "id"}) {
		t.Errorf("got keyset columns %v", got.KeysetColumns)
	}
}

// A requested sort column that is nullable disqualifies keyset pagination even
// though a tiebreaker is present — `(col, pk) > (val, pk)` is SQL-UNKNOWN wherever col IS NULL, so
// a naive grant here would either silently drop every NULL row from all keyset pages forever, or
// hard-fail a page whose boundary row has a NULL sort value. Only the tiebreaker itself had to be
// non-nullable; the user's own requested sort columns were never checked.
func TestComputeEffectiveOrder_NullableSortColumnDisqualifiesKeyset(t *testing.T) {
	columns := []model.ColumnMeta{col("id", 0, true), nullableCol("name", 1), col("created_at", 2, false)}
	sort := structuredSort(model.SortTerm{Column: "name", Direction: "asc"})
	got, err := ComputeEffectiveOrder(sort, columns, []string{"id"})
	if err != nil {
		t.Fatal(err)
	}
	if got.KeysetEligible {
		t.Error("expected KeysetEligible = false for a nullable sort column")
	}
	if len(got.KeysetColumns) != 0 {
		t.Errorf("expected no keyset columns, got %v", got.KeysetColumns)
	}
	want := []OrderTerm{{Column: "name", Direction: "asc"}}
	if !reflect.DeepEqual(got.Terms, want) {
		t.Errorf("got terms %v, want %v", got.Terms, want)
	}
	if got.KeysetDirection != "asc" {
		t.Errorf("got direction %q, want asc", got.KeysetDirection)
	}
}

// BuildOrderBy (called from BuildScanOrderBy) uppercases Direction into the ORDER BY
// text, so an upper-case or mixed-case direction still *builds* correct SQL there — but the keyset
// comparison operator/reversal logic (BuildKeysetPredicate) compares the exact lowercase literal
// "asc", never normalizing case itself. Pre-fix, "ASC" sailed through ComputeEffectiveOrder,
// KeysetDirection ended up "ASC", and BuildKeysetPredicate's own `direction == "asc"` comparison
// then silently picked the wrong comparison operator — a real, silent mispaging bug, not merely a
// rejected input. ComputeEffectiveOrder must reject anything but the exact lowercase spelling
// before KeysetDirection is ever set from it.
func TestComputeEffectiveOrder_InvalidDirectionRejected(t *testing.T) {
	tests := []struct {
		name      string
		direction string
	}{
		{"upper-case ASC", "ASC"},
		{"upper-case DESC", "DESC"},
		{"mixed case Asc", "Asc"},
		{"trailing whitespace", "asc "},
		{"garbage value", "sideways"},
		{"empty string", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sort := structuredSort(model.SortTerm{Column: "name", Direction: tt.direction})
			_, err := ComputeEffectiveOrder(sort, testColumns(), []string{"id"})
			var ae *Error
			if !errors.As(err, &ae) || ae.Code != CodeQuery {
				t.Fatalf("ComputeEffectiveOrder(direction=%q) = %v, want an E_QUERY *Error", tt.direction, err)
			}
		})
	}
}

// The exact operator-mismatch BuildKeysetPredicate itself would have produced had an upper-case
// direction ever reached it (confirming the "silent mispaging", not just a rejected input,
// is what ComputeEffectiveOrder's new guard prevents): "after" with the lowercase-correct "asc"
// picks ">", but the same "after" with "ASC" — never matching the lowercase-only comparison —
// wrongly picks "<", reversing the page's own scan direction.
func TestBuildKeysetPredicate_CaseSensitiveDirectionMismatchDemonstratesF11(t *testing.T) {
	lower := BuildKeysetPredicate([]string{"id"}, "asc", "after", 1, func(i int) string { return "$" + strconv.Itoa(i) })
	upper := BuildKeysetPredicate([]string{"id"}, "ASC", "after", 1, func(i int) string { return "$" + strconv.Itoa(i) })
	if !strings.Contains(lower, ">") {
		t.Fatalf("lower-case asc/after predicate = %q, want a \">\" comparison", lower)
	}
	if strings.Contains(upper, ">") {
		t.Fatalf("upper-case ASC/after predicate = %q, want the (buggy) \"<\" comparison this test documents", upper)
	}
}

// a primary-key column whose own Nullable bit is (mistakenly, per SQLite's own pragma quirk for a
// bare `INTEGER PRIMARY KEY`) set true is exempted — sorting by the primary key, the single most
// common sort column in the whole catalog, must stay keyset-eligible.
func TestComputeEffectiveOrder_NullablePrimaryKeySortColumnStillEligible(t *testing.T) {
	pk := nullableCol("id", 0)
	pk.IsPrimaryKey = true
	columns := []model.ColumnMeta{pk, col("name", 1, false), col("created_at", 2, false)}
	sort := structuredSort(model.SortTerm{Column: "id", Direction: "asc"})
	got, err := ComputeEffectiveOrder(sort, columns, []string{"id"})
	if err != nil {
		t.Fatal(err)
	}
	if !got.KeysetEligible {
		t.Fatal("expected KeysetEligible = true when the nullable-per-metadata sort column is the primary key")
	}
}

// a nullable tiebreaker column itself (not a requested sort term) is unaffected by this check —
// AssertKeysetSupported / the caller's own tiebreaker selection is responsible for only ever
// naming a non-nullable column (D7); this guards the requested sort terms only.
func TestComputeEffectiveOrder_NullableNonSortColumnStillEligible(t *testing.T) {
	columns := []model.ColumnMeta{col("id", 0, true), col("name", 1, false), nullableCol("bio", 2)}
	sort := structuredSort(model.SortTerm{Column: "name", Direction: "asc"})
	got, err := ComputeEffectiveOrder(sort, columns, []string{"id"})
	if err != nil {
		t.Fatal(err)
	}
	if !got.KeysetEligible {
		t.Fatal("expected KeysetEligible = true when the unrelated nullable column isn't in the sort")
	}
}

// 5. no sort at all is ascending and eligible on the tiebreaker alone
func TestComputeEffectiveOrder_NoSortEligibleOnTiebreaker(t *testing.T) {
	got, err := ComputeEffectiveOrder(nil, testColumns(), []string{"id"})
	if err != nil {
		t.Fatal(err)
	}
	want := EffectiveOrder{
		Terms: []OrderTerm{{Column: "id", Direction: "asc"}}, KeysetEligible: true,
		KeysetColumns: []string{"id"}, KeysetDirection: "asc",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

// 8. a page token carries a fingerprint of the request that produced it, and decoding refuses one
// taken under a different filter/sort — without this the cursor silently pages through the wrong
// query's keyset.
func TestPageToken_MismatchedFingerprint(t *testing.T) {
	token := EncodePageToken([]string{"id", "42"}, RequestFingerprint(map[string]int{"a": 1}))
	_, err := DecodePageToken(token, RequestFingerprint(map[string]int{"a": 2}))
	code, ok := CodeOf(err)
	if !ok || code != CodeQuery {
		t.Fatalf("got %v, want E_QUERY", err)
	}
	if !strings.Contains(err.Error(), "doesn’t match") {
		t.Errorf("message %q does not mention the mismatch", err.Error())
	}
}

// 10. the operator flips with both direction and mode
func TestBuildKeysetPredicate_OperatorFlips(t *testing.T) {
	ph := func(i int) string { return "$" + strconv.FormatInt(int64(i), 10) }
	cases := []struct {
		direction, mode, want string
	}{
		{"asc", "after", `("id") > ($1)`},
		{"asc", "before", `("id") < ($1)`},
		{"desc", "after", `("id") < ($1)`},
		{"desc", "before", `("id") > ($1)`},
	}
	for _, c := range cases {
		got := BuildKeysetPredicate([]string{`"id"`}, c.direction, c.mode, 1, ph)
		if got != c.want {
			t.Errorf("%s/%s: got %q, want %q", c.direction, c.mode, got, c.want)
		}
	}
}

func TestBuildKeysetPosition(t *testing.T) {
	fp := RequestFingerprint(map[string]any{"path": "t", "pageSize": 2})
	eligibleOrder := EffectiveOrder{
		Terms: []OrderTerm{{Column: "id", Direction: "asc"}}, KeysetEligible: true,
		KeysetColumns: []string{"id"}, KeysetDirection: "asc",
	}
	keysetColumnIdx := map[string]int{"id": 0}
	cellAt := func(rows [][]string) func(int, int) *string {
		return func(row, col int) *string { return &rows[row][col] }
	}

	// 14. an 'after' page with a next page reports strategy 'keyset' and both tokens
	t.Run("14", func(t *testing.T) {
		rows := [][]string{{"1"}, {"2"}}
		pos, err := BuildKeysetPosition(KeysetPositionArgs{
			Cursor: model.PageCursor{Mode: "after", Token: "x"}, PageSize: 2, DisplayRowCount: 2,
			ProbedExtra: true, Order: eligibleOrder, KeysetColumnIdx: keysetColumnIdx,
			Fingerprint: fp, CellAt: cellAt(rows),
		})
		if err != nil {
			t.Fatal(err)
		}
		if pos.Strategy != "keyset" || pos.Offset != nil || !pos.HasMore {
			t.Errorf("got %+v", pos)
		}
		assertTokenDecodesTo(t, pos.NextToken, fp, []string{"2"})
		assertTokenDecodesTo(t, pos.PrevToken, fp, []string{"1"})
	})

	// 15. a 'before' page always reports hasMore true, regardless of probedExtra
	t.Run("15", func(t *testing.T) {
		rows := [][]string{{"5"}, {"6"}}
		pos, err := BuildKeysetPosition(KeysetPositionArgs{
			Cursor: model.PageCursor{Mode: "before", Token: "x"}, PageSize: 2, DisplayRowCount: 2,
			ProbedExtra: false, Order: eligibleOrder, KeysetColumnIdx: keysetColumnIdx,
			Fingerprint: fp, CellAt: cellAt(rows),
		})
		if err != nil {
			t.Fatal(err)
		}
		if !pos.HasMore || pos.PrevToken != nil {
			t.Errorf("got %+v", pos)
		}
		assertTokenDecodesTo(t, pos.NextToken, fp, []string{"6"})
	})

	// 16. an offset page at 0 never has a prevToken
	t.Run("16", func(t *testing.T) {
		rows := [][]string{{"1"}, {"2"}}
		pos, err := BuildKeysetPosition(KeysetPositionArgs{
			Cursor: model.PageCursor{Mode: "offset", Offset: 0}, PageSize: 2, DisplayRowCount: 2,
			ProbedExtra: true, Order: eligibleOrder, KeysetColumnIdx: keysetColumnIdx,
			Fingerprint: fp, CellAt: cellAt(rows),
		})
		if err != nil {
			t.Fatal(err)
		}
		if pos.Offset == nil || *pos.Offset != 0 || pos.PrevToken != nil {
			t.Errorf("got %+v", pos)
		}
		assertTokenDecodesTo(t, pos.NextToken, fp, []string{"2"})
	})

	// 17. an offset page at >0 gets a prevToken once keyset-eligible
	t.Run("17", func(t *testing.T) {
		rows := [][]string{{"21"}, {"22"}}
		pos, err := BuildKeysetPosition(KeysetPositionArgs{
			Cursor: model.PageCursor{Mode: "offset", Offset: 20}, PageSize: 2, DisplayRowCount: 2,
			ProbedExtra: false, Order: eligibleOrder, KeysetColumnIdx: keysetColumnIdx,
			Fingerprint: fp, CellAt: cellAt(rows),
		})
		if err != nil {
			t.Fatal(err)
		}
		if pos.Offset == nil || *pos.Offset != 20 || pos.HasMore || pos.NextToken != nil {
			t.Errorf("got %+v", pos)
		}
		assertTokenDecodesTo(t, pos.PrevToken, fp, []string{"21"})
	})
}

// 11. ResolveProjection returns ordinal order rather than request order, and dedups a repeated
// column — the two rules that make a projection's column list not simply the caller's own slice.
func TestResolveProjection_OrdinalOrderAndDedup(t *testing.T) {
	got, err := ResolveProjection(testColumns(), []string{"created_at", "id"})
	if err != nil {
		t.Fatal(err)
	}
	want := []model.ColumnMeta{col("id", 0, true), col("created_at", 2, false)}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}

	deduped, err := ResolveProjection(testColumns(), []string{"id", "id"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(deduped, []model.ColumnMeta{col("id", 0, true)}) {
		t.Errorf("got %+v, want a single id column", deduped)
	}
}

// P21 round 1 functional finding F1: a non-nil, empty requested slice used to resolve to "project
// zero columns" — every SQL adapter's selectList then became an empty string, `SELECT  FROM ...`.
// The renderer now avoids ever sending one, but ResolveProjection refuses it directly too.
func TestResolveProjection_EmptyNonNilRequestedIsRejected(t *testing.T) {
	if _, err := ResolveProjection(testColumns(), []string{}); err == nil {
		t.Fatal("ResolveProjection([]string{}): expected an error, got nil")
	}
}

// nil (as opposed to an empty slice) still means "every column" — the one distinction
// ResolveProjection's own contract rests on.
func TestResolveProjection_NilRequestedMeansEveryColumn(t *testing.T) {
	got, err := ResolveProjection(testColumns(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, testColumns()) {
		t.Errorf("got %+v, want every column", got)
	}
}

func assertTokenDecodesTo(t *testing.T, token *string, fp string, want []string) {
	t.Helper()
	if token == nil {
		t.Fatal("expected a non-nil token")
	}
	got, err := DecodePageToken(*token, fp)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// 13. strips exactly one trailing semicolon, with its trailing whitespace
func TestStripOneTrailingSemicolon(t *testing.T) {
	cases := map[string]string{
		"SELECT 1;":     "SELECT 1",
		"SELECT 1;  \n": "SELECT 1",
		"SELECT 1;;":    "SELECT 1;",
		"SELECT 1":      "SELECT 1",
	}
	for in, want := range cases {
		if got := StripOneTrailingSemicolon(in); got != want {
			t.Errorf("StripOneTrailingSemicolon(%q) = %q, want %q", in, got, want)
		}
	}
}

// Finding F10: a filter ending in a `--` line comment used to comment out WhereClause's own closing
// paren (`WHERE (x = 1 -- note)`), breaking every read/count built from it. The closing paren must
// survive on its own line regardless.
func TestWhereClause_TrailingLineCommentDoesNotEatClosingParen(t *testing.T) {
	filter := "x = 1 -- note"
	got := WhereClause(&filter)
	if !strings.HasSuffix(got, "\n)") {
		t.Fatalf("WhereClause(%q) = %q, want it to end with an unstripped closing paren on its own line", filter, got)
	}
	// The closing paren must not itself fall inside the filter's own trailing line comment — i.e.
	// there must be a real newline between "-- note" and the final ")".
	lastComment := strings.LastIndex(got, "--")
	if lastComment < 0 || !strings.Contains(got[lastComment:], "\n") {
		t.Fatalf("WhereClause(%q) = %q, the trailing `--` comment still swallows the closing paren", filter, got)
	}
}

// A console statement whose own last line ends in a line comment used to have its
// joining ";\n" separator land on that same line — the ";" reads as part of the comment, and
// sql-split.ts's own re-split then merges this statement with the next one (Operations panel
// re-run). JoinConsoleStatements must insert an extra "\n" whenever that would happen, for every
// dialect's own comment prefix(es), and must never insert one when no prefix is present.
func TestJoinConsoleStatements(t *testing.T) {
	cases := []struct {
		name     string
		stmts    []string
		prefixes []string
		want     string
	}{
		{
			name:     "no comment: plain join, unchanged",
			stmts:    []string{"SELECT 1", "SELECT 2"},
			prefixes: []string{"--"},
			want:     "SELECT 1;\nSELECT 2",
		},
		{
			name:     "trailing -- comment on the last line gets an extra newline before the separator",
			stmts:    []string{"SELECT 1 -- note", "SELECT 2"},
			prefixes: []string{"--"},
			want:     "SELECT 1 -- note\n;\nSELECT 2",
		},
		{
			name:     "a comment on an earlier line, not the last, needs no extra newline",
			stmts:    []string{"SELECT 1 -- note\nFROM t", "SELECT 2"},
			prefixes: []string{"--"},
			want:     "SELECT 1 -- note\nFROM t;\nSELECT 2",
		},
		{
			name:     "the final statement never gets a trailing separator, comment or not",
			stmts:    []string{"SELECT 1", "SELECT 2 -- note"},
			prefixes: []string{"--"},
			want:     "SELECT 1;\nSELECT 2 -- note",
		},
		{
			name:     "MySQL/ClickHouse: a trailing # comment is caught by its own prefix",
			stmts:    []string{"SELECT 1 # note", "SELECT 2"},
			prefixes: []string{"--", "#"},
			want:     "SELECT 1 # note\n;\nSELECT 2",
		},
		{
			name:     "Mongo: a trailing // comment is caught by its own prefix",
			stmts:    []string{"db.t.find({}) // note", "db.t.find({})"},
			prefixes: []string{"//"},
			want:     "db.t.find({}) // note\n;\ndb.t.find({})",
		},
		{
			name:     "three statements: only the commented middle one gets the extra newline",
			stmts:    []string{"SELECT 1", "SELECT 2 -- note", "SELECT 3"},
			prefixes: []string{"--"},
			want:     "SELECT 1;\nSELECT 2 -- note\n;\nSELECT 3",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := JoinConsoleStatements(tc.stmts, tc.prefixes...); got != tc.want {
				t.Errorf("JoinConsoleStatements(%q, %q) = %q, want %q", tc.stmts, tc.prefixes, got, tc.want)
			}
		})
	}
}
