package dbmcp

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/mask"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/page"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestRenderTabularPageNullVersusEmptyString is the named hazard: a NULL row and an empty
// string both have Offsets[i] == Offsets[i+1], and only the Nulls bitset (page.IsNull) tells them
// apart. Getting cellAt backwards renders NULL as "" — valid JSON a model would reason over
// incorrectly, with nothing failing anywhere.
func TestRenderTabularPageNullVersusEmptyString(t *testing.T) {
	b := page.NewTabularPageBuilder([]page.ColumnDescriptor{
		{Name: "id", DataType: "int4", TypeClass: page.TypeClassNumber},
		{Name: "note", DataType: "text", TypeClass: page.TypeClassText},
	})
	one := "1"
	empty := ""
	if err := b.AppendRow([]*string{&one, &empty}); err != nil {
		t.Fatalf("AppendRow (empty string row): %v", err)
	}
	if err := b.AppendRow([]*string{nil, nil}); err != nil {
		t.Fatalf("AppendRow (NULL row): %v", err)
	}
	pg := b.Finish(page.UnpagedPosition(2))

	rendered := renderTabularPage(pg, 200, nil)
	if rendered.Kind != "tabular" {
		t.Fatalf("Kind = %q, want tabular", rendered.Kind)
	}
	if len(rendered.Rows) != 2 {
		t.Fatalf("len(Rows) = %d, want 2", len(rendered.Rows))
	}

	// Row 0: an explicit empty string, not NULL.
	if rendered.Rows[0][1] == nil {
		t.Fatal("row 0 col 1 (empty string) rendered as NULL, want a non-nil pointer to \"\"")
	}
	if *rendered.Rows[0][1] != "" {
		t.Fatalf("row 0 col 1 = %q, want empty string", *rendered.Rows[0][1])
	}

	// Row 1: a genuine NULL in both columns.
	if rendered.Rows[1][0] != nil {
		t.Fatalf("row 1 col 0 (NULL) rendered as %q, want nil", *rendered.Rows[1][0])
	}
	if rendered.Rows[1][1] != nil {
		t.Fatalf("row 1 col 1 (NULL) rendered as %q, want nil", *rendered.Rows[1][1])
	}
}

// TestRenderTabularPageTruncation covers maxRows capping and TruncatedCells surfacing (§5.2:
// "truncatedCells... surfaced so a client never treats a clipped value as complete").
func TestRenderTabularPageTruncation(t *testing.T) {
	b := page.NewTabularPageBuilder([]page.ColumnDescriptor{
		{Name: "v", DataType: "text", TypeClass: page.TypeClassText},
	})
	huge := make([]byte, page.MaxCellBytes+10)
	for i := range huge {
		huge[i] = 'x'
	}
	hugeStr := string(huge)
	if err := b.AppendRow([]*string{&hugeStr}); err != nil {
		t.Fatalf("AppendRow: %v", err)
	}
	for i := 0; i < 3; i++ {
		v := "small"
		if err := b.AppendRow([]*string{&v}); err != nil {
			t.Fatalf("AppendRow: %v", err)
		}
	}
	pg := b.Finish(page.UnpagedPosition(4))
	if pg.TruncatedCells == 0 {
		t.Fatal("builder did not truncate the oversized cell — test setup is wrong, not the code under test")
	}

	rendered := renderTabularPage(pg, 2, nil)
	if rendered.RowCount != 4 {
		t.Fatalf("RowCount = %d, want 4 (the full result, not the capped one)", rendered.RowCount)
	}
	if rendered.Returned != 2 {
		t.Fatalf("Returned = %d, want 2 (capped by maxRows)", rendered.Returned)
	}
	if len(rendered.Rows) != 2 {
		t.Fatalf("len(Rows) = %d, want 2", len(rendered.Rows))
	}
	if !rendered.Truncated {
		t.Fatal("Truncated = false, want true (Returned < RowCount)")
	}
	if rendered.TruncatedCells != pg.TruncatedCells {
		t.Fatalf("TruncatedCells = %d, want %d (the builder's own count, verbatim)", rendered.TruncatedCells, pg.TruncatedCells)
	}
}

func TestRenderDocumentPage(t *testing.T) {
	b := page.NewDocumentPageBuilder(false)
	b.Push("id-1", `{"a":1}`)
	b.Push("id-2", `{"b":2}`)
	pg := b.Finish(page.UnpagedPosition(2))

	rendered := renderDocumentPage(pg, 200)
	if rendered.Kind != "document" {
		t.Fatalf("Kind = %q, want document", rendered.Kind)
	}
	if len(rendered.Documents) != 2 {
		t.Fatalf("len(Documents) = %d, want 2", len(rendered.Documents))
	}
	if rendered.Documents[0].ID == nil || *rendered.Documents[0].ID != "id-1" {
		t.Fatalf("Documents[0].ID = %v, want \"id-1\"", rendered.Documents[0].ID)
	}
	if rendered.Documents[1].Body == nil || *rendered.Documents[1].Body != `{"b":2}` {
		t.Fatalf("Documents[1].Body = %v, want {\"b\":2}", rendered.Documents[1].Body)
	}
}

func TestRenderKeyValuePage(t *testing.T) {
	ttl := int64(1000)
	mem := int64(2048)
	b := page.NewKeyValuePageBuilder("hash", &ttl, &mem, false)
	b.Push("field1", "value1")
	pg := b.Finish(page.UnpagedPosition(1))

	rendered := renderKeyValuePage(pg, 200, nil)
	if rendered.Kind != "keyvalue" {
		t.Fatalf("Kind = %q, want keyvalue", rendered.Kind)
	}
	if rendered.RedisType != "hash" {
		t.Fatalf("RedisType = %q, want hash", rendered.RedisType)
	}
	if rendered.TTLMs == nil || *rendered.TTLMs != 1000 {
		t.Fatalf("TTLMs = %v, want 1000", rendered.TTLMs)
	}
	if len(rendered.Entries) != 1 || *rendered.Entries[0].Field != "field1" || *rendered.Entries[0].Value != "value1" {
		t.Fatalf("Entries = %+v, want one {field1 value1}", rendered.Entries)
	}
}

func TestRenderStreamPage(t *testing.T) {
	b := page.NewStreamPageBuilder(nil)
	key := "k1"
	ts := "2024-01-01T00:00:00Z"
	payload := "payload"
	b.Push(page.StreamRow{Key: &key, Headers: "{}", Attrs: "{}", Timestamp: &ts, Body: &payload})
	pg := b.Finish(page.UnpagedPosition(1))

	rendered := renderStreamPage(pg, 200)
	if rendered.Kind != "stream" {
		t.Fatalf("Kind = %q, want stream", rendered.Kind)
	}
	if len(rendered.Messages) != 1 {
		t.Fatalf("len(Messages) = %d, want 1", len(rendered.Messages))
	}
	msg := rendered.Messages[0]
	if msg.Key == nil || *msg.Key != "k1" {
		t.Fatalf("Messages[0].Key = %v, want k1", msg.Key)
	}
	if msg.Body == nil || *msg.Body != "payload" {
		t.Fatalf("Messages[0].Body = %v, want payload", msg.Body)
	}
}

// TestRenderPageDispatchesByKind covers renderPage's own type switch for all four kinds it knows,
// and confirms an unrecognised page kind returns an error rather than a silent wrong render (§5.2:
// "implement it rather than panicking on the fourth arm of a type switch").
func TestRenderPageDispatchesByKind(t *testing.T) {
	tb := page.NewTabularPageBuilder(nil)
	tabular := tb.Finish(page.UnpagedPosition(0))
	if _, err := renderPage(tabular, 200, nil, nil, ""); err != nil {
		t.Fatalf("renderPage(TabularPage): %v", err)
	}

	db := page.NewDocumentPageBuilder(false)
	doc := db.Finish(page.UnpagedPosition(0))
	if _, err := renderPage(doc, 200, nil, nil, ""); err != nil {
		t.Fatalf("renderPage(DocumentPage): %v", err)
	}

	kvb := page.NewKeyValuePageBuilder("string", nil, nil, false)
	kv := kvb.Finish(page.UnpagedPosition(0))
	if _, err := renderPage(kv, 200, nil, nil, ""); err != nil {
		t.Fatalf("renderPage(KeyValuePage): %v", err)
	}

	sb := page.NewStreamPageBuilder(nil)
	stream := sb.Finish(page.UnpagedPosition(0))
	if _, err := renderPage(stream, 200, nil, nil, ""); err != nil {
		t.Fatalf("renderPage(StreamPage): %v", err)
	}

	if _, err := renderPage(unknownPage{}, 200, nil, nil, ""); err == nil {
		t.Fatal("renderPage(unrecognised kind) = nil error, want an error naming the unhandled type")
	}
}

// unknownPage satisfies page.Page but is none of the four kinds renderPage knows — exercising its
// default arm without needing a fifth real page kind to exist.
type unknownPage struct{}

func (unknownPage) PageKind() page.PageKind { return page.PageKind("unknown") }
func (unknownPage) Size() int               { return 0 }
func (unknownPage) Rows() int               { return 0 }

// --- M5 §8: masking applied per column, not per row; NULL untouched; a document/stream page
// refused when rules exist; a keyvalue entry masked by its Field name. ---

// TestColumnRulesResolvesOncePerColumn pins the cost rule directly: columnRules builds one
// index-aligned []*mask.Rule from the column list, called exactly once by renderTabularPage before
// its row loop — this test is what would break if that call moved inside the loop
// and started re-resolving per cell.
func TestColumnRulesResolvesOncePerColumn(t *testing.T) {
	columns := []page.ColumnDescriptor{
		{Name: "id", DataType: "int4", TypeClass: page.TypeClassNumber},
		{Name: "Email", DataType: "text", TypeClass: page.TypeClassText},
		{Name: "note", DataType: "text", TypeClass: page.TypeClassText},
	}
	set := mask.Set{Masker: mask.New(nil), Rules: map[string]mask.Rule{
		"email": {Kind: mask.KindRedact},
	}}
	rules := columnRules(columns, &maskset{Set: set})
	if len(rules) != 3 {
		t.Fatalf("columnRules len = %d, want 3 (index-aligned with columns)", len(rules))
	}
	if rules[0] != nil {
		t.Fatalf("rules[0] (id, no rule) = %+v, want nil", rules[0])
	}
	if rules[1] == nil || rules[1].Kind != mask.KindRedact {
		t.Fatalf("rules[1] (Email, matches lowercase rule \"email\") = %+v, want a redact rule", rules[1])
	}
	if rules[2] != nil {
		t.Fatalf("rules[2] (note, no rule) = %+v, want nil", rules[2])
	}

	if got := columnRules(columns, nil); got != nil {
		t.Fatalf("columnRules(nil Set) = %+v, want nil", got)
	}
	empty := mask.Set{}
	if got := columnRules(columns, &maskset{Set: empty}); got != nil {
		t.Fatalf("columnRules(empty Set) = %+v, want nil", got)
	}
}

// TestRenderTabularPageMasksEveryRowByColumn confirms the per-column resolve is actually applied
// to every row, not just the first — masking must not silently stop after one row.
func TestRenderTabularPageMasksEveryRowByColumn(t *testing.T) {
	b := page.NewTabularPageBuilder([]page.ColumnDescriptor{
		{Name: "id", DataType: "int4", TypeClass: page.TypeClassNumber},
		{Name: "email", DataType: "text", TypeClass: page.TypeClassText},
	})
	for i := 0; i < 5; i++ {
		id := "1"
		email := "person@example.com"
		if err := b.AppendRow([]*string{&id, &email}); err != nil {
			t.Fatalf("AppendRow: %v", err)
		}
	}
	pg := b.Finish(page.UnpagedPosition(5))
	set := mask.Set{Masker: mask.New(nil), Rules: map[string]mask.Rule{"email": {Kind: mask.KindRedact}}}

	rendered := renderTabularPage(pg, 200, &maskset{Set: set})
	for r, row := range rendered.Rows {
		if row[0] == nil || *row[0] != "1" {
			t.Fatalf("row %d col 0 (unmasked) = %v, want unchanged \"1\"", r, row[0])
		}
		if row[1] == nil || *row[1] != "[redacted]" {
			t.Fatalf("row %d col 1 (masked) = %v, want \"[redacted]\"", r, row[1])
		}
	}
}

// TestRenderTabularPageNullPassesThroughUnmasked is §2.3's own universal rule, re-verified at the
// render seam: a NULL cell in a masked column must stay NULL, never become "[redacted]" or any
// other masked text — masking a NULL would invent a value that is not there.
func TestRenderTabularPageNullPassesThroughUnmasked(t *testing.T) {
	b := page.NewTabularPageBuilder([]page.ColumnDescriptor{
		{Name: "email", DataType: "text", TypeClass: page.TypeClassText},
	})
	if err := b.AppendRow([]*string{nil}); err != nil {
		t.Fatalf("AppendRow: %v", err)
	}
	pg := b.Finish(page.UnpagedPosition(1))
	set := mask.Set{Masker: mask.New(nil), Rules: map[string]mask.Rule{"email": {Kind: mask.KindRedact}}}

	rendered := renderTabularPage(pg, 200, &maskset{Set: set})
	if rendered.Rows[0][0] != nil {
		t.Fatalf("NULL cell in a masked column rendered as %q, want nil (NULL untouched)", *rendered.Rows[0][0])
	}
}

// TestRenderPageRefusesDocumentAndStreamPagesWhenRulesExist: a document or
// stream page is refused outright on a connection with at least one mask rule, never partially
// masked — a partial mask the user believes is total is worse than a clear refusal.
func TestRenderPageRefusesDocumentAndStreamPagesWhenRulesExist(t *testing.T) {
	set := mask.Set{Masker: mask.New(nil), Rules: map[string]mask.Rule{"ssn": {Kind: mask.KindRedact}}}

	db := page.NewDocumentPageBuilder(false)
	db.Push("id-1", `{"a":1}`)
	doc := db.Finish(page.UnpagedPosition(1))
	if _, err := renderPage(doc, 200, nil, &maskset{Set: set}, ""); err == nil {
		t.Fatal("renderPage(DocumentPage, rules exist) = nil error, want a refusal")
	} else if !strings.Contains(err.Error(), "document") {
		t.Fatalf("renderPage(DocumentPage) error = %q, want it to name the document page kind", err.Error())
	}

	sb := page.NewStreamPageBuilder(nil)
	payload := "payload"
	sb.Push(page.StreamRow{Body: &payload})
	stream := sb.Finish(page.UnpagedPosition(1))
	if _, err := renderPage(stream, 200, nil, &maskset{Set: set}, ""); err == nil {
		t.Fatal("renderPage(StreamPage, rules exist) = nil error, want a refusal")
	} else if !strings.Contains(err.Error(), "stream") {
		t.Fatalf("renderPage(StreamPage) error = %q, want it to name the stream page kind", err.Error())
	}

	// No rules at all (nil mk, or an empty Set): both page kinds render normally, unchanged.
	if _, err := renderPage(doc, 200, nil, nil, ""); err != nil {
		t.Fatalf("renderPage(DocumentPage, no rules) = %v, want no error", err)
	}
	empty := mask.Set{}
	if _, err := renderPage(stream, 200, nil, &maskset{Set: empty}, ""); err != nil {
		t.Fatalf("renderPage(StreamPage, empty Set) = %v, want no error", err)
	}
}

// TestRenderPageRefusesRenamedOrTransformedMaskedColumn: an alias, a wrapping expression, or a
// subquery all produce a result column name columnRules's exact-name match can't see — without a
// refusal, the real value would return unmasked, the opposite of this design's stated
// over-mask-on-ambiguity intent.
func TestRenderPageRefusesRenamedOrTransformedMaskedColumn(t *testing.T) {
	set := mask.Set{Masker: mask.New(nil), Rules: map[string]mask.Rule{"email": {Kind: mask.KindRedact}}}

	b := page.NewTabularPageBuilder([]page.ColumnDescriptor{
		{Name: "e", DataType: "text", TypeClass: page.TypeClassText},
	})
	v := "person@example.com"
	if err := b.AppendRow([]*string{&v}); err != nil {
		t.Fatalf("AppendRow: %v", err)
	}
	pg := b.Finish(page.UnpagedPosition(1))

	if _, err := renderPage(pg, 200, nil, &maskset{Set: set}, "SELECT email AS e FROM customers"); err == nil {
		t.Fatal("renderPage(aliased masked column) = nil error, want a refusal")
	} else if !strings.Contains(err.Error(), "email") {
		t.Fatalf("renderPage error = %q, want it to name the masked column", err.Error())
	}

	if _, err := renderPage(pg, 200, nil, &maskset{Set: set}, "SELECT lower(email) AS e FROM customers"); err == nil {
		t.Fatal("renderPage(masked column wrapped in an expression) = nil error, want a refusal")
	}

	// A statement that never mentions "email" at all (the query genuinely has nothing to do with
	// the masked column) must render normally — the refusal is about a mentioned-but-hidden
	// column, not about column e being unrecognised on its own.
	if _, err := renderPage(pg, 200, nil, &maskset{Set: set}, "SELECT id AS e FROM customers"); err != nil {
		t.Fatalf("renderPage(unrelated column, same alias) = %v, want no error", err)
	}

	// The masked column selected under its own exact name renders normally, unaffected.
	b2 := page.NewTabularPageBuilder([]page.ColumnDescriptor{
		{Name: "email", DataType: "text", TypeClass: page.TypeClassText},
	})
	if err := b2.AppendRow([]*string{&v}); err != nil {
		t.Fatalf("AppendRow: %v", err)
	}
	pg2 := b2.Finish(page.UnpagedPosition(1))
	if _, err := renderPage(pg2, 200, nil, &maskset{Set: set}, "SELECT email FROM customers"); err != nil {
		t.Fatalf("renderPage(masked column by its own name) = %v, want no error", err)
	}

	// A masked column selected once under its own name and again under a different alias must
	// still be refused: the exact-name occurrence alone must not excuse the
	// second, differently-named projection of the same value — the previous implementation's
	// "present under its own name" short-circuit skipped the check entirely here, returning
	// "leak" completely unmasked.
	b3 := page.NewTabularPageBuilder([]page.ColumnDescriptor{
		{Name: "email", DataType: "text", TypeClass: page.TypeClassText},
		{Name: "leak", DataType: "text", TypeClass: page.TypeClassText},
	})
	if err := b3.AppendRow([]*string{&v, &v}); err != nil {
		t.Fatalf("AppendRow: %v", err)
	}
	pg3 := b3.Finish(page.UnpagedPosition(1))
	if _, err := renderPage(pg3, 200, nil, &maskset{Set: set}, "SELECT email, email AS leak FROM customers"); err == nil {
		t.Fatal("renderPage(masked column duplicated under a second alias) = nil error, want a refusal")
	} else if !strings.Contains(err.Error(), "email") {
		t.Fatalf("renderPage error = %q, want it to name the masked column", err.Error())
	}

	// Same bypass shape via `SELECT *` — the star silently contributes the exact-name "email"
	// column with no textual mention of its own, so a naive word-occurrence-count comparison
	// would wrongly treat the explicit "email AS leak" mention as already accounted for.
	b4 := page.NewTabularPageBuilder([]page.ColumnDescriptor{
		{Name: "id", DataType: "int4", TypeClass: page.TypeClassNumber},
		{Name: "email", DataType: "text", TypeClass: page.TypeClassText},
		{Name: "leak", DataType: "text", TypeClass: page.TypeClassText},
	})
	id := "1"
	if err := b4.AppendRow([]*string{&id, &v, &v}); err != nil {
		t.Fatalf("AppendRow: %v", err)
	}
	pg4 := b4.Finish(page.UnpagedPosition(1))
	if _, err := renderPage(pg4, 200, nil, &maskset{Set: set}, "SELECT *, email AS leak FROM customers"); err == nil {
		t.Fatal("renderPage(SELECT *, email AS leak) = nil error, want a refusal")
	} else if !strings.Contains(err.Error(), "email") {
		t.Fatalf("renderPage error = %q, want it to name the masked column", err.Error())
	}

	// Second projections the old AS-only regex missed (P168 Part 6 F9): implicit alias, quoted alias,
	// function wrapper, operator expression.
	for _, stmt := range []string{
		"SELECT email, email leak FROM customers",
		`SELECT email, email AS "leak" FROM customers`,
		"SELECT email, lower(email) FROM customers",
		"SELECT email, email || '' FROM customers",
		"SELECT email, (SELECT email FROM c2 LIMIT 1) FROM customers",
	} {
		if _, err := renderPage(pg3, 200, nil, &maskset{Set: set}, stmt); err == nil {
			t.Errorf("renderPage(%q) = nil error, want a refusal", stmt)
		}
	}
	for _, stmt := range []string{
		"SELECT c.email, o.id FROM c JOIN o ON o.email = c.email",
		`SELECT "email" FROM c ORDER BY email`,
	} {
		if _, err := renderPage(pg2, 200, nil, &maskset{Set: set}, stmt); err != nil {
			t.Errorf("renderPage(%q) = %v, want no error", stmt, err)
		}
	}

	// Filtering on the masked column's own name while also selecting it under that same name
	// (no second alias anywhere) must still render normally — a WHERE-clause self-reference is
	// not a second, differently-named projection.
	if _, err := renderPage(pg2, 200, nil, &maskset{Set: set}, "SELECT email FROM customers WHERE email = 'x'"); err != nil {
		t.Fatalf("renderPage(masked column selected and filtered by its own name) = %v, want no error", err)
	}

	// Filtering on a masked column that is NOT selected at all is refused (the scanner can't tell
	// a WHERE-only mention from an actual rename it failed to catch), but must not claim to have
	// found a rename or transform it never saw — the WHERE-clause case gets its own,
	// accurate wording.
	b5 := page.NewTabularPageBuilder([]page.ColumnDescriptor{
		{Name: "id", DataType: "int4", TypeClass: page.TypeClassNumber},
	})
	if err := b5.AppendRow([]*string{&id}); err != nil {
		t.Fatalf("AppendRow: %v", err)
	}
	pg5 := b5.Finish(page.UnpagedPosition(1))
	if _, err := renderPage(pg5, 200, nil, &maskset{Set: set}, "SELECT id FROM customers WHERE email = 'x'"); err == nil {
		t.Fatal("renderPage(masked column filtered but not selected) = nil error, want a refusal")
	} else if strings.Contains(err.Error(), "renames or transforms") {
		t.Fatalf("renderPage error = %q, a WHERE-only mention must not claim a rename/transform it never found", err.Error())
	} else if !strings.Contains(err.Error(), "email") || !strings.Contains(err.Error(), "isn't in the result set") {
		t.Fatalf("renderPage error = %q, want it to name the column and explain it is absent from the result set", err.Error())
	}
}

// TestMaskedToolErrorWithholdsAdapterMessage: a Postgres/MySQL driver error
// routinely embeds the offending value (e.g. a failed type-cast error names the literal it could
// not parse) — over a connection with active mask rules, maskedToolError must keep the parsed
// error code (no row data) but never the adapter's own message text.
func TestMaskedToolErrorWithholdsAdapterMessage(t *testing.T) {
	leaky := adapters.New(adapters.CodeQuery, `invalid input syntax for type integer: "person@example.com"`, nil)

	result, _, err := maskedToolError(leaky)
	if err != nil {
		t.Fatalf("maskedToolError(adapter error) returned a Go error %v, want an IsError result (nil error)", err)
	}
	if !result.IsError {
		t.Fatal("maskedToolError(adapter error).IsError = false, want true")
	}
	text := result.Content[0].(*mcp.TextContent).Text
	if strings.Contains(text, "person@example.com") {
		t.Fatalf("maskedToolError leaked the adapter's raw message: %q", text)
	}
	if !strings.Contains(text, string(adapters.CodeQuery)) {
		t.Fatalf("maskedToolError dropped the error code: %q", text)
	}

	// A genuine internal fault (no adapter/ipcerr code) is unaffected — still a bare Go error,
	// same as toolError, since there is no adapter message to withhold in the first place.
	internal := errors.New("dbmcp: something else entirely")
	if _, _, err := maskedToolError(internal); err == nil {
		t.Fatal("maskedToolError(internal fault) = nil error, want the original error returned")
	}
}

// TestWithAdditionalStatementResultsNoteAddsCountWithoutLosingFields is finding #12, M6: a
// run_query call whose args.SQL ran more than one statement must say so rather than silently
// rendering only the first statement's page.
func TestWithAdditionalStatementResultsNoteAddsCountWithoutLosingFields(t *testing.T) {
	b := page.NewTabularPageBuilder([]page.ColumnDescriptor{
		{Name: "id", DataType: "int4", TypeClass: page.TypeClassNumber},
	})
	one := "1"
	if err := b.AppendRow([]*string{&one}); err != nil {
		t.Fatalf("AppendRow: %v", err)
	}
	pg := b.Finish(page.UnpagedPosition(1))

	rendered := renderTabularPage(pg, 10, nil)

	// M7 finding #17: fields are set directly on the concrete type now, no marshal round trip —
	// the result comes back as the same tabularResult, not a generic map.
	got := withAdditionalStatementResultsNote(rendered, 2)
	tr, ok := got.(tabularResult)
	if !ok {
		t.Fatalf("withAdditionalStatementResultsNote returned %T, want tabularResult", got)
	}
	if tr.Kind != "tabular" {
		t.Fatalf("lost the original result's own fields: %+v", tr)
	}
	if tr.RowCount != 1 || tr.Returned != 1 {
		t.Fatalf("lost the original result's own row fields: %+v", tr)
	}
	if tr.AdditionalStatementResults != 2 {
		t.Fatalf("AdditionalStatementResults = %d, want 2", tr.AdditionalStatementResults)
	}
	if !strings.Contains(tr.Note, "2 additional statement") {
		t.Fatalf("Note = %q, want it to mention 2 additional statements", tr.Note)
	}

	// The JSON wire shape carries both new fields too, not just the Go struct.
	encoded, err := json.Marshal(tr)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	var wire map[string]any
	if err := json.Unmarshal(encoded, &wire); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if n, ok := wire["additionalStatementResults"].(float64); !ok || n != 2 {
		t.Fatalf("wire additionalStatementResults = %v, want 2", wire["additionalStatementResults"])
	}
	if note, ok := wire["note"].(string); !ok || !strings.Contains(note, "2 additional statement") {
		t.Fatalf("wire note = %v, want it to mention 2 additional statements", wire["note"])
	}
}

// TestRenderKeyValuePageMasksByFieldName is §4.3's own extension: a Redis hash field (or an S3
// metadata key) is column-shaped, so a rule whose column_name matches keyValueEntry.Field masks
// that entry's Value — matched case-insensitively, the same as a tabular column.
func TestRenderKeyValuePageMasksByFieldName(t *testing.T) {
	b := page.NewKeyValuePageBuilder("hash", nil, nil, false)
	b.Push("Email", "person@example.com")
	b.Push("plan", "premium")
	pg := b.Finish(page.UnpagedPosition(2))
	set := mask.Set{Masker: mask.New(nil), Rules: map[string]mask.Rule{"email": {Kind: mask.KindRedact}}}

	rendered := renderKeyValuePage(pg, 200, &maskset{Set: set})
	if len(rendered.Entries) != 2 {
		t.Fatalf("len(Entries) = %d, want 2", len(rendered.Entries))
	}
	if *rendered.Entries[0].Field != "Email" || *rendered.Entries[0].Value != "[redacted]" {
		t.Fatalf("Entries[0] = %+v, want Field unchanged and Value masked", rendered.Entries[0])
	}
	if *rendered.Entries[1].Field != "plan" || *rendered.Entries[1].Value != "premium" {
		t.Fatalf("Entries[1] = %+v, want both fields unchanged (no matching rule)", rendered.Entries[1])
	}
}

// TestRenderPageRefusesRiskyResultColumns: a masked connection must refuse a tabular result column
// whose type can itself carry a whole row or several columns' worth of data (JSON, array,
// composite/"other") — matching by output column name alone cannot see a masked value smuggled
// inside one of these, and the existing statement text-scan (maskedColumnRenamedOrHidden) never
// fires either, since the masked column's own name never appears as plain text in a statement
// like `SELECT json_agg(t) FROM (...) t`.
func TestRenderPageRefusesRiskyResultColumns(t *testing.T) {
	set := mask.Set{Masker: mask.New(nil), Rules: map[string]mask.Rule{"email": {Kind: mask.KindRedact}}}

	cases := []struct {
		name      string
		typeClass page.TypeClass
	}{
		{"json_agg whole-row JSON column", page.TypeClassJSON},
		{"composite/row column (Postgres pg_type.typtype='c', tagged Other)", page.TypeClassOther},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := page.NewTabularPageBuilder([]page.ColumnDescriptor{
				{Name: "json_agg", DataType: "json", TypeClass: tc.typeClass},
			})
			leak := `[{"email":"person@example.com"}]`
			if err := b.AppendRow([]*string{&leak}); err != nil {
				t.Fatalf("AppendRow: %v", err)
			}
			pg := b.Finish(page.UnpagedPosition(1))

			if _, err := renderPage(pg, 200, nil, &maskset{Set: set}, "SELECT json_agg(t) FROM (SELECT * FROM customers LIMIT 10) t"); err == nil {
				t.Fatal("renderPage(risky-typed column, rules exist) = nil error, want a refusal")
			} else if !strings.Contains(err.Error(), "json_agg") {
				t.Fatalf("renderPage error = %q, want it to name the risky column", err.Error())
			}
		})
	}

	// No rules at all: the same risky-typed column renders normally, unaffected — this guard is
	// masking-gated, never a general restriction on JSON/array columns.
	b := page.NewTabularPageBuilder([]page.ColumnDescriptor{
		{Name: "metadata", DataType: "jsonb", TypeClass: page.TypeClassJSON},
	})
	v := `{"plan":"premium"}`
	if err := b.AppendRow([]*string{&v}); err != nil {
		t.Fatalf("AppendRow: %v", err)
	}
	pg := b.Finish(page.UnpagedPosition(1))
	if _, err := renderPage(pg, 200, nil, nil, "SELECT metadata FROM accounts"); err != nil {
		t.Fatalf("renderPage(JSON column, no rules) = %v, want no error", err)
	}
}

// TestRefuseRiskyStatementSyntax is the second and third guards: three concrete SQL shapes verified
// against real engines that rename or transform every selected column with no per-column textual
// trace maskedColumnRenamedOrHidden's word-scan could ever find.
func TestRefuseRiskyStatementSyntax(t *testing.T) {
	risky := []struct {
		name      string
		statement string
	}{
		{"Postgres Unicode-escaped identifier", `SELECT U&"\0065mail" AS x FROM customers`},
		{"FROM-clause positional column-alias list", `SELECT e FROM customers AS c(i, n, e)`},
		{"ClickHouse APPLY column transform", `SELECT * APPLY(toString) FROM customers`},
		{"ClickHouse COLUMNS transform", `SELECT COLUMNS('.*') APPLY(toString) FROM customers`},
		{"Postgres E-string backslash quote", `SELECT email, E'\'' AS a, email AS leak FROM t`},
		{"MySQL backslash-escaped quote", `SELECT email, '\'' AS a, email AS leak FROM t`},
		{"Postgres dollar quote", `SELECT email, $$'$$ AS a, email AS leak FROM t`},
		{"Postgres tagged dollar quote", `SELECT email, $q$'$q$ AS a, email AS leak FROM t`},
		{"MySQL executable comment", "SELECT email /*!, email AS leak */ FROM t"},
		{"MariaDB executable comment", "SELECT email /*M!100000 , email AS leak */ FROM t"},
		{"executable comment after quote in comment", "SELECT email /* ' */ /*!, email AS leak */ FROM t"},
	}
	for _, tc := range risky {
		t.Run(tc.name, func(t *testing.T) {
			if _, found := refuseRiskyStatementSyntax(tc.statement); !found {
				t.Fatalf("refuseRiskyStatementSyntax(%q) = not found, want a refusal", tc.statement)
			}
		})
	}

	safe := []struct {
		name      string
		statement string
	}{
		{"ordinary CAST with precision", "SELECT CAST(price AS numeric(10,2)) FROM orders"},
		{"ordinary CAST to varchar with length", "SELECT CAST(name AS varchar(255)) FROM customers"},
		{"plain select, no risky syntax", "SELECT id, email FROM customers WHERE id = 1"},
		{"placeholder and ordinary comment", "SELECT id /* note */ FROM customers WHERE id = $1 AND n = 'it''s'"},
	}
	for _, tc := range safe {
		t.Run(tc.name, func(t *testing.T) {
			if reason, found := refuseRiskyStatementSyntax(tc.statement); found {
				t.Fatalf("refuseRiskyStatementSyntax(%q) = refused (%q), want not found", tc.statement, reason)
			}
		})
	}

	// End to end through renderPage: the Unicode-escaped-identifier bypass reaches a masked column
	// ("email") under an output name ("x") with no plain-text trace of "email" anywhere in the
	// statement — the pre-existing word-scan refusal (maskedColumnRenamedOrHidden) cannot fire here
	// at all, so this statement-syntax guard is the only thing that catches it.
	set := mask.Set{Masker: mask.New(nil), Rules: map[string]mask.Rule{"email": {Kind: mask.KindRedact}}}
	b := page.NewTabularPageBuilder([]page.ColumnDescriptor{
		{Name: "x", DataType: "text", TypeClass: page.TypeClassText},
	})
	v := "person@example.com"
	if err := b.AppendRow([]*string{&v}); err != nil {
		t.Fatalf("AppendRow: %v", err)
	}
	pg := b.Finish(page.UnpagedPosition(1))
	if _, err := renderPage(pg, 200, nil, &maskset{Set: set}, `SELECT U&"\0065mail" AS x FROM customers`); err == nil {
		t.Fatal(`renderPage(U&"..." identifier bypass) = nil error, want a refusal`)
	}

	// No rules at all: the same risky syntax renders normally, unaffected.
	if _, err := renderPage(pg, 200, nil, nil, `SELECT U&"\0065mail" AS x FROM customers`); err != nil {
		t.Fatalf("renderPage(risky syntax, no rules) = %v, want no error", err)
	}
}

// TestRenderPageRefusesColumnlessKeyValuePage: a keyvalue page whose Field
// values carry no real per-entry identifier (page.KeyValuePage.FieldsAreColumns=false — a Redis
// list/set's synthetic display index, a generic console reply with no per-command shape) must be
// refused when the connection has active mask rules, never silently rendered unmasked under a
// Field no rule could ever legitimately match — the same fail-closed posture as the document/
// stream page refusal above.
func TestRenderPageRefusesColumnlessKeyValuePage(t *testing.T) {
	set := mask.Set{Masker: mask.New(nil), Rules: map[string]mask.Rule{"email": {Kind: mask.KindRedact}}}

	b := page.NewKeyValuePageBuilder("set", nil, nil, false)
	b.SetFieldsAreColumns(false)
	b.Push("0", "person@example.com")
	pg := b.Finish(page.UnpagedPosition(1))

	if _, err := renderPage(pg, 200, nil, &maskset{Set: set}, "SMEMBERS emails"); err == nil {
		t.Fatal("renderPage(columnless keyvalue page, rules exist) = nil error, want a refusal")
	}

	// No rules at all: renders normally, unaffected.
	if _, err := renderPage(pg, 200, nil, nil, "SMEMBERS emails"); err != nil {
		t.Fatalf("renderPage(columnless keyvalue page, no rules) = %v, want no error", err)
	}

	// A page whose fields ARE real columns (the default) is unaffected by this refusal, even with
	// rules active — columnRules/ApplyColumn's own per-entry match still applies normally.
	hb := page.NewKeyValuePageBuilder("hash", nil, nil, false)
	hb.Push("email", "person@example.com")
	hashPage := hb.Finish(page.UnpagedPosition(1))
	if _, err := renderPage(hashPage, 200, nil, &maskset{Set: set}, "HGETALL user:1"); err != nil {
		t.Fatalf("renderPage(hash keyvalue page, real field names) = %v, want no error", err)
	}
}

func TestSetOpMovesMaskedColumn(t *testing.T) {
	cols := []page.ColumnDescriptor{{Name: "name"}, {Name: "email"}}
	set := mask.Set{Masker: mask.New(nil), Rules: map[string]mask.Rule{"email": {Kind: mask.KindRedact}}}
	mk := &maskset{Set: set}
	tests := []struct {
		name, stmt string
		refuse     bool
	}{
		{"swapped second branch", "SELECT name, email FROM t UNION ALL SELECT email, name FROM t", true},
		{"intersect", "SELECT name, email FROM t INTERSECT SELECT email, name FROM t", true},
		{"except", "SELECT name, email FROM t EXCEPT SELECT email, name FROM t", true},
		{"third branch", "SELECT name, email FROM t UNION SELECT name, email FROM u UNION SELECT email, name FROM v", true},
		{"parenthesised branch", "(SELECT name, email FROM t) UNION ALL (SELECT email, name FROM t)", true},
		{"nested subquery", "SELECT name, email FROM (SELECT name, email FROM t UNION ALL SELECT email, name FROM t) s", true},
		{"same position", "SELECT name, email FROM t UNION ALL SELECT name, email FROM u", false},
		{"qualified same position", "SELECT t.name, t.email FROM t UNION SELECT u.name, u.email FROM u", false},
		{"commas in function", "SELECT coalesce(name, 'x') AS name, email FROM t UNION SELECT name, email FROM u", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, hidden, _ := maskedColumnRenamedOrHidden(tc.stmt, mk, cols)
			if hidden != tc.refuse {
				t.Fatalf("refuse = %v, want %v", hidden, tc.refuse)
			}
		})
	}
}

func TestRenderPageWithheldColumns(t *testing.T) {
	mk := &maskset{
		Set:      mask.Set{Masker: mask.New(nil), Rules: map[string]mask.Rule{"email": {Kind: mask.KindRedact}}},
		withheld: map[string]bool{"secret": true},
	}
	cols := []page.ColumnDescriptor{
		{Name: "id", DataType: "int4", TypeClass: page.TypeClassNumber},
		{Name: "email", DataType: "text", TypeClass: page.TypeClassText},
		{Name: "secret", DataType: "text", TypeClass: page.TypeClassText},
	}
	build := func(names ...string) page.TabularPage {
		var use []page.ColumnDescriptor
		row := []*string{}
		for _, n := range names {
			for _, c := range cols {
				if c.Name == n {
					use = append(use, c)
					v := "x"
					row = append(row, &v)
				}
			}
		}
		b := page.NewTabularPageBuilder(use)
		if err := b.AppendRow(row); err != nil {
			t.Fatalf("AppendRow: %v", err)
		}
		return b.Finish(page.UnpagedPosition(1))
	}

	got, err := renderPage(build("id", "email", "secret"), 200, nil, mk, "SELECT * FROM t")
	if err != nil {
		t.Fatalf("renderPage(SELECT *): %v", err)
	}
	res := got.(tabularResult)
	if len(res.Columns) != 2 || len(res.WithheldColumns) != 1 || res.WithheldColumns[0] != "secret" || res.WithheldNote == "" {
		t.Fatalf("columns=%d withheld=%v note=%q, want secret dropped with a note", len(res.Columns), res.WithheldColumns, res.WithheldNote)
	}
	if c := res.Rows[0][1]; c != nil && *c == "x" {
		t.Fatalf("masked column still carries its value: %v", res.Rows[0])
	}

	for _, stmt := range []string{
		"SELECT secret AS s FROM t",
		"SELECT upper(secret) FROM t",
		"SELECT id FROM t WHERE secret = 'a'",
		"SELECT id FROM t ORDER BY secret",
	} {
		if _, err := renderPage(build("id"), 200, nil, mk, stmt); err == nil {
			t.Fatalf("renderPage(%q) = nil error, want a refusal", stmt)
		}
	}
}
