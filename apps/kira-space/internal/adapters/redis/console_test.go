// The console tokenizer, unit-level, no container (C12, per docs/HISTORY.md (v1 P58c)
// §5.5): three interacting rules and one error, and it is the console's entire input path.
package redis

import (
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters/testsupport"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/page"
)

func TestTokenize_QuotedAndUnquoted(t *testing.T) {
	tokens, err := tokenize(`SET foo "bar baz"`)
	if err != nil {
		t.Fatalf("tokenize: %v", err)
	}
	want := []string{"SET", "foo", "bar baz"}
	if len(tokens) != len(want) {
		t.Fatalf("tokens = %v, want %v", tokens, want)
	}
	for i, w := range want {
		if tokens[i] != w {
			t.Errorf("tokens[%d] = %q, want %q", i, tokens[i], w)
		}
	}
}

func TestTokenize_SingleQuotes(t *testing.T) {
	tokens, err := tokenize(`SET foo 'bar baz'`)
	if err != nil {
		t.Fatalf("tokenize: %v", err)
	}
	if len(tokens) != 3 || tokens[2] != "bar baz" {
		t.Errorf("tokens = %v", tokens)
	}
}

func TestTokenize_EscapeInsideQuotes(t *testing.T) {
	tokens, err := tokenize(`SET foo "bar\"baz"`)
	if err != nil {
		t.Fatalf("tokenize: %v", err)
	}
	if len(tokens) != 3 || tokens[2] != `bar"baz` {
		t.Errorf("tokens = %v, want the escaped quote preserved literally", tokens)
	}
}

func TestTokenize_EscapeOutsideQuotesNotHonoured(t *testing.T) {
	// Outside quotes, a backslash is just another non-whitespace character — no escape handling.
	tokens, err := tokenize(`SET foo bar\baz`)
	if err != nil {
		t.Fatalf("tokenize: %v", err)
	}
	if len(tokens) != 3 || tokens[2] != `bar\baz` {
		t.Errorf("tokens = %v, want the backslash preserved literally", tokens)
	}
}

// TestTokenize_NewlineEscapeInsideDoubleQuotes is the regression: a `\n` inside a
// double-quoted token must become a real newline byte, matching redis-cli's own sdssplitargs —
// the previous tokenizer took the escaped rune literally (`\n` became the two-byte text "an"
// glued to the quote, i.e. "n" itself, not a newline), so `SET k "a\nb"` stored the literal text
// "anb" instead of a newline.
func TestTokenize_NewlineEscapeInsideDoubleQuotes(t *testing.T) {
	tokens, err := tokenize(`SET k "a\nb"`)
	if err != nil {
		t.Fatalf("tokenize: %v", err)
	}
	if len(tokens) != 3 || tokens[2] != "a\nb" {
		t.Errorf("tokens = %q, want the middle token to contain a real newline", tokens)
	}
}

// TestTokenize_HexEscapeInsideDoubleQuotes covers F12's \xHH support (redis-cli's own
// sdssplitargs), including a byte that isn't valid standalone UTF-8 — exercising the
// binary-unsafe-key round trip the review's own edge-case list calls out.
func TestTokenize_HexEscapeInsideDoubleQuotes(t *testing.T) {
	tokens, err := tokenize(`SET k "a\xffb"`)
	if err != nil {
		t.Fatalf("tokenize: %v", err)
	}
	want := "a\xffb"
	if len(tokens) != 3 || tokens[2] != want {
		t.Errorf("tokens = %q, want %q as the middle token", tokens, want)
	}
}

// TestTokenize_SingleQuoteOnlyEscapesApostrophe: F12's own single-quote rule — only \' is special;
// every other backslash sequence (including \n) stays completely literal, unlike double quotes.
func TestTokenize_SingleQuoteOnlyEscapesApostrophe(t *testing.T) {
	tokens, err := tokenize(`SET k 'a\nb'`)
	if err != nil {
		t.Fatalf("tokenize: %v", err)
	}
	if len(tokens) != 3 || tokens[2] != `a\nb` {
		t.Errorf("tokens = %q, want the backslash-n preserved literally inside single quotes", tokens)
	}

	tokens, err = tokenize(`SET k 'it\'s'`)
	if err != nil {
		t.Fatalf("tokenize: %v", err)
	}
	if len(tokens) != 3 || tokens[2] != `it's` {
		t.Errorf("tokens = %q, want \\' resolved to a literal apostrophe", tokens)
	}
}

// TestTokenize_TrailingContentAfterClosingQuoteIsError: redis-cli itself rejects a closing quote
// immediately followed by a non-space character (F12) — glued-on trailing content is never silently
// merged into the token or split into a separate one.
func TestTokenize_TrailingContentAfterClosingQuoteIsError(t *testing.T) {
	_, err := tokenize(`SET k "bar"baz`)
	if err == nil {
		t.Fatal("want an error, got nil")
	}
	code, _ := adapters.CodeOf(err)
	if code != adapters.CodeQuery {
		t.Errorf("code = %v, want E_QUERY", code)
	}
}

func TestTokenize_UnterminatedQuoteIsError(t *testing.T) {
	_, err := tokenize(`SET foo "bar`)
	if err == nil {
		t.Fatal("want an error, got nil")
	}
	code, _ := adapters.CodeOf(err)
	if code != adapters.CodeQuery {
		t.Errorf("code = %v, want E_QUERY", code)
	}
	if err.Error() != "Unterminated quoted string." {
		t.Errorf("message = %q, want the verbatim adapter message", err.Error())
	}
}

// kvEntry reads back page pg's own Field/Value at row i, for the resultToPage tests below.
func kvEntry(t *testing.T, pg page.KeyValuePage, i int) (field, value string) {
	t.Helper()
	f, v := testsupport.KVFieldAt(t, pg, i), testsupport.KVValueAt(t, pg, i)
	if f == nil || v == nil {
		t.Fatalf("row %d: field/value = %v/%v, want both non-nil", i, f, v)
	}
	return *f, *v
}

// TestResultToPage_HGETCarriesRealFieldName: resultToPage previously named
// every scalar reply's Field after the upper-cased command ("HGET"), never the real hash field
// requested — so a mask rule on that field could never match it through run_query, though
// list_connections reported the connection as protected. HGET's own field name comes from the
// command's own argument, not the reply.
func TestResultToPage_HGETCarriesRealFieldName(t *testing.T) {
	pg := resultToPage("HGET", []string{"email"}, "person@example.com", page.ResultCap{})
	if !pg.FieldsAreColumns {
		t.Fatal("FieldsAreColumns = false, want true — HGET's field name is real")
	}
	field, value := kvEntry(t, pg, 0)
	if field != "email" || value != "person@example.com" {
		t.Fatalf("entry = {%q %q}, want {email person@example.com}", field, value)
	}
}

// TestResultToPage_HMGETCarriesRealFieldNamesPositionally confirms each reply element is paired
// with the field name at the same position in the request, not a bare index.
func TestResultToPage_HMGETCarriesRealFieldNamesPositionally(t *testing.T) {
	pg := resultToPage("HMGET", []string{"email", "plan"}, []any{"person@example.com", "premium"}, page.ResultCap{})
	if !pg.FieldsAreColumns {
		t.Fatal("FieldsAreColumns = false, want true — HMGET's field names are real")
	}
	if pg.RowCount != 2 {
		t.Fatalf("RowCount = %d, want 2", pg.RowCount)
	}
	f0, v0 := kvEntry(t, pg, 0)
	f1, v1 := kvEntry(t, pg, 1)
	if f0 != "email" || v0 != "person@example.com" {
		t.Fatalf("entry 0 = {%q %q}, want {email person@example.com}", f0, v0)
	}
	if f1 != "plan" || v1 != "premium" {
		t.Fatalf("entry 1 = {%q %q}, want {plan premium}", f1, v1)
	}
}

// TestResultToPage_HGETALLCarriesRealFieldNames is the finding's own literal repro: run_query
// "HGETALL user:1" against a connection with an "email" mask rule must produce a page whose Field
// values are the real hash field names (from the RESP2 flat field/value reply), so
// renderKeyValuePage's ApplyColumn(field, value) actually has something to match against.
func TestResultToPage_HGETALLCarriesRealFieldNames(t *testing.T) {
	pg := resultToPage("HGETALL", nil, []any{"email", "person@example.com", "plan", "premium"}, page.ResultCap{})
	if !pg.FieldsAreColumns {
		t.Fatal("FieldsAreColumns = false, want true — HGETALL's field names are real")
	}
	if pg.RowCount != 2 {
		t.Fatalf("RowCount = %d, want 2", pg.RowCount)
	}
	f0, v0 := kvEntry(t, pg, 0)
	f1, v1 := kvEntry(t, pg, 1)
	if f0 != "email" || v0 != "person@example.com" {
		t.Fatalf("entry 0 = {%q %q}, want {email person@example.com}", f0, v0)
	}
	if f1 != "plan" || v1 != "premium" {
		t.Fatalf("entry 1 = {%q %q}, want {plan premium}", f1, v1)
	}
}

// TestResultToPage_GenericReplyIsNotFieldsAreColumns pins the safe default for every command
// outside hashReadCommands: GET's reply carries no real field name at all (the command name is a
// display artifact, not a column), so dbmcp's renderPage must refuse to mask this page rather than
// silently pass a value through under a Field nothing could ever match.
func TestResultToPage_GenericReplyIsNotFieldsAreColumns(t *testing.T) {
	pg := resultToPage("GET", []string{"user:1:email"}, "person@example.com", page.ResultCap{})
	if pg.FieldsAreColumns {
		t.Fatal("FieldsAreColumns = true, want false — GET's reply has no real field name")
	}

	arr := resultToPage("SMEMBERS", nil, []any{"a", "b"}, page.ResultCap{})
	if arr.FieldsAreColumns {
		t.Fatal("FieldsAreColumns = true, want false — an array reply's index is not a real field name")
	}
}
