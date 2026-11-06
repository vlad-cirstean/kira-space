// The table-driven oracle for literal.go, written before literal.go itself.
// Case groups mirror docs/HISTORY.md's bar exactly: every escape, both
// quote styles, both comment forms, trailing commas, the six constructors with and without an
// argument, the thirteen EJSON wrapper keys, and the position-bearing error messages ported
// byte-for-byte from src/engine/adapters/mongo/literal.ts.
package mongo_test

import (
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters/mongo"
)

func mustParseValue(t *testing.T, text string) any {
	t.Helper()
	v, err := mongo.ParseJSON5Literal(text)
	if err != nil {
		t.Fatalf("ParseJSON5Literal(%q): unexpected error: %v", text, err)
	}
	return v
}

func wantQueryError(t *testing.T, err error, wantMessage string) {
	t.Helper()
	if err == nil {
		t.Fatalf("want error %q, got nil", wantMessage)
	}
	code, ok := adapters.CodeOf(err)
	if !ok || code != adapters.CodeQuery {
		t.Fatalf("want E_QUERY, got %v (%v)", code, err)
	}
	if err.Error() != wantMessage {
		t.Fatalf("error = %q, want %q", err.Error(), wantMessage)
	}
}

// --- strings: quote styles, escapes, comments ---------------------------------------------------

func TestLiteral_StringQuoteStyles(t *testing.T) {
	if v := mustParseValue(t, `"double"`); v != "double" {
		t.Errorf("double-quoted = %v, want double", v)
	}
	if v := mustParseValue(t, `'single'`); v != "single" {
		t.Errorf("single-quoted = %v, want single", v)
	}
}

func TestLiteral_StringEscapes(t *testing.T) {
	cases := map[string]string{
		`"\n"`: "\n",
		`"\t"`: "\t",
		`"\r"`: "\r",
		`"\b"`: "\b",
		`"\f"`: "\f",
		`"\\"`: "\\",
		`"\""`: "\"",
		`'\''`: "'",
		`"\/"`: "/",
		`"A"`:  "A",
		`"\q"`: "q", // unknown escape yields the escaped character itself
	}
	for input, want := range cases {
		t.Run(input, func(t *testing.T) {
			v := mustParseValue(t, input)
			if v != want {
				t.Errorf("parse(%q) = %q, want %q", input, v, want)
			}
		})
	}
}

func TestLiteral_UnterminatedString(t *testing.T) {
	_, err := mongo.ParseJSON5Literal(`"abc`)
	wantQueryError(t, err, "Unterminated string literal.")
}

func TestLiteral_Comments(t *testing.T) {
	v := mustParseValue(t, "// a leading comment\n42")
	if v != float64(42) {
		t.Errorf("line comment: got %v", v)
	}
	v = mustParseValue(t, "/* block */ 42")
	if v != float64(42) {
		t.Errorf("block comment: got %v", v)
	}
	// An unterminated block comment is ignored, not rejected (literal.ts:46) — walks off the end,
	// leaving no more tokens before EOF.
	v, err := mongo.ParseJSON5Literal("42 /* unterminated")
	if err != nil {
		t.Fatalf("unterminated block comment: unexpected error: %v", err)
	}
	if v != float64(42) {
		t.Errorf("unterminated block comment: got %v", v)
	}
}

// --- numbers (C5) ----------------------------------------------------------------------------

func TestLiteral_UnadornedNumberIsFloat64(t *testing.T) {
	v := mustParseValue(t, "42")
	if _, ok := v.(float64); !ok {
		t.Fatalf("42 parsed as %T, want float64", v)
	}
	if v != float64(42) {
		t.Errorf("42 = %v, want 42", v)
	}
	v = mustParseValue(t, "-3.5")
	if v != float64(-3.5) {
		t.Errorf("-3.5 = %v, want -3.5", v)
	}
}

func TestLiteral_InvalidNumber(t *testing.T) {
	_, err := mongo.ParseJSON5Literal("1.2.3e+-4")
	wantQueryError(t, err, `Invalid number “1.2.3e+-4” at position 0.`)
}

// --- objects, arrays, trailing commas ----------------------------------------------------------

func TestLiteral_ObjectTrailingComma(t *testing.T) {
	v := mustParseValue(t, `{a: 1, b: 2,}`)
	d, ok := v.(bson.D)
	if !ok {
		t.Fatalf("got %T, want bson.D", v)
	}
	if len(d) != 2 || d[0].Key != "a" || d[1].Key != "b" {
		t.Errorf("d = %+v", d)
	}
}

func TestLiteral_ArrayTrailingComma(t *testing.T) {
	v := mustParseValue(t, `[1, 2,]`)
	a, ok := v.(bson.A)
	if !ok {
		t.Fatalf("got %T, want bson.A", v)
	}
	if len(a) != 2 || a[0] != float64(1) || a[1] != float64(2) {
		t.Errorf("a = %+v", a)
	}
}

func TestLiteral_ObjectFieldOrderPreserved(t *testing.T) {
	v := mustParseValue(t, `{z: 1, a: 2, m: {y: 1, b: 2}}`)
	d := v.(bson.D)
	gotKeys := []string{d[0].Key, d[1].Key, d[2].Key}
	if gotKeys[0] != "z" || gotKeys[1] != "a" || gotKeys[2] != "m" {
		t.Errorf("top-level key order = %v, want [z a m]", gotKeys)
	}
	nested := d[2].Value.(bson.D)
	if nested[0].Key != "y" || nested[1].Key != "b" {
		t.Errorf("nested key order = %+v, want [y b]", nested)
	}
}

func TestLiteral_ObjectNumericKeyRejected(t *testing.T) {
	_, err := mongo.ParseJSON5Literal(`{ 1: "x" }`)
	wantQueryError(t, err, `Expected an object key at position 2.`)
}

// --- identifiers, literals, bare words -----------------------------------------------------

func TestLiteral_BooleanAndNullLiterals(t *testing.T) {
	if v := mustParseValue(t, "true"); v != true {
		t.Errorf("true = %v", v)
	}
	if v := mustParseValue(t, "false"); v != false {
		t.Errorf("false = %v", v)
	}
	if v := mustParseValue(t, "null"); v != nil {
		t.Errorf("null = %v, want nil", v)
	}
	if v := mustParseValue(t, "undefined"); v != nil {
		t.Errorf("undefined = %v, want nil", v)
	}
}

func TestLiteral_BareIdentifierRejected(t *testing.T) {
	_, err := mongo.ParseJSON5Literal("notAThing")
	wantQueryError(t, err, `Unrecognized identifier “notAThing” at position 0.`)
}

func TestLiteral_ObjectIdWithoutParensRejected(t *testing.T) {
	// No call parentheses: the lookahead in ParseValue means a bare `ObjectId` is not a
	// constructor call and falls through to the same rejection as any other bare word
	// (literal.ts:177).
	_, err := mongo.ParseJSON5Literal("ObjectId")
	wantQueryError(t, err, `Unrecognized identifier “ObjectId” at position 0.`)
}

// --- constructors ----------------------------------------------------------------------------

func TestLiteral_ObjectIdConstructor(t *testing.T) {
	hex := "507f191e810c19729de860ea"
	v := mustParseValue(t, `ObjectId("`+hex+`")`)
	id, ok := v.(bson.ObjectID)
	if !ok {
		t.Fatalf("got %T, want bson.ObjectID", v)
	}
	if id.Hex() != hex {
		t.Errorf("id.Hex() = %s, want %s", id.Hex(), hex)
	}

	// No argument: a fresh id, not an error.
	v2 := mustParseValue(t, "ObjectId()")
	if _, ok := v2.(bson.ObjectID); !ok {
		t.Fatalf("ObjectId() = %T, want bson.ObjectID", v2)
	}
}

func TestLiteral_ISODateConstructor(t *testing.T) {
	v := mustParseValue(t, `ISODate("2024-01-15T10:30:00Z")`)
	tm, ok := v.(time.Time)
	if !ok {
		t.Fatalf("got %T, want time.Time", v)
	}
	want := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	if !tm.Equal(want) {
		t.Errorf("ISODate = %v, want %v", tm, want)
	}
}

func TestLiteral_ISODateInvalidIsError(t *testing.T) {
	_, err := mongo.ParseJSON5Literal(`ISODate("not a date")`)
	wantQueryError(t, err, `Invalid date “not a date” at position 0.`)
}

func TestLiteral_DateConstructor(t *testing.T) {
	before := time.Now().UTC()
	v := mustParseValue(t, "Date()")
	tm, ok := v.(time.Time)
	if !ok {
		t.Fatalf("Date() = %T, want time.Time", v)
	}
	if tm.Before(before) || tm.After(time.Now().UTC().Add(time.Minute)) {
		t.Errorf("Date() = %v, want close to now", tm)
	}

	v2 := mustParseValue(t, `Date("2024-01-15")`)
	tm2 := v2.(time.Time)
	if tm2.Year() != 2024 || tm2.Month() != 1 || tm2.Day() != 15 {
		t.Errorf("Date(\"2024-01-15\") = %v", tm2)
	}
}

func TestLiteral_NumberLongConstructor(t *testing.T) {
	v := mustParseValue(t, `NumberLong("9007199254740993")`)
	n, ok := v.(int64)
	if !ok {
		t.Fatalf("got %T, want int64", v)
	}
	if n != 9007199254740993 {
		t.Errorf("NumberLong = %d", n)
	}
}

func TestLiteral_NumberIntConstructor(t *testing.T) {
	v := mustParseValue(t, `NumberInt("5")`)
	n, ok := v.(int32)
	if !ok {
		t.Fatalf("got %T, want int32", v)
	}
	if n != 5 {
		t.Errorf("NumberInt = %d", n)
	}
}

func TestLiteral_NumberDecimalConstructor(t *testing.T) {
	v := mustParseValue(t, `NumberDecimal("19.99")`)
	d, ok := v.(bson.Decimal128)
	if !ok {
		t.Fatalf("got %T, want bson.Decimal128", v)
	}
	if d.String() != "19.99" {
		t.Errorf("NumberDecimal = %s, want 19.99", d.String())
	}
}

// --- EJSON wrappers ----------------------------------------------------------------------------

func TestLiteral_EJSONWrapperKeys(t *testing.T) {
	hex := "507f191e810c19729de860ea"
	cases := map[string]string{
		`{"$oid": "` + hex + `"}`:                                   "$oid",
		`{"$date": "2024-01-15T10:30:00Z"}`:                         "$date",
		`{"$numberInt": "5"}`:                                       "$numberInt",
		`{"$numberLong": "9007199254740993"}`:                       "$numberLong",
		`{"$numberDouble": "3.14"}`:                                 "$numberDouble",
		`{"$numberDecimal": "19.99"}`:                               "$numberDecimal",
		`{"$binary": {"base64": "ZGF0YQ==", "subType": "00"}}`:      "$binary",
		`{"$timestamp": {"t": 1700000000, "i": 1}}`:                 "$timestamp",
		`{"$regularExpression": {"pattern": "^a", "options": "i"}}`: "$regularExpression",
		`{"$code": "function() {}"}`:                                "$code",
		`{"$minKey": 1}`:                                            "$minKey",
		`{"$maxKey": 1}`:                                            "$maxKey",
	}
	for input, key := range cases {
		t.Run(key, func(t *testing.T) {
			d, err := mongo.ParseDocumentLiteral(`{v: ` + input + `}`)
			if err != nil {
				t.Fatalf("ParseDocumentLiteral(%q): %v", input, err)
			}
			if len(d) != 1 || d[0].Key != "v" {
				t.Fatalf("d = %+v", d)
			}
			// A resolved wrapper is never itself a bson.D any more — it has become the scalar
			// (or sub-document, for $ref) BSON value the wrapper denoted.
			if sub, ok := d[0].Value.(bson.D); ok && looksStillWrapped(sub, key) {
				t.Errorf("%s: value still looks like the raw wrapper: %+v", key, sub)
			}
		})
	}
}

func looksStillWrapped(d bson.D, key string) bool {
	for _, e := range d {
		if e.Key == key {
			return true
		}
	}
	return false
}

func TestLiteral_EJSONRefWrapper(t *testing.T) {
	// $ref/$id is the one wrapper shaped as a sub-document rather than a scalar (a DBRef) — still
	// exercised so all thirteen keys have a case, per §5.5.
	d, err := mongo.ParseDocumentLiteral(`{v: {"$ref": "widgets", "$id": "` + `507f191e810c19729de860ea` + `"}}`)
	if err != nil {
		t.Fatalf("ParseDocumentLiteral: %v", err)
	}
	if len(d) != 1 {
		t.Fatalf("d = %+v", d)
	}
}

func TestLiteral_EJSONWrapperWrongTypeFallsThroughAsPlainObject(t *testing.T) {
	// { $oid: 123 } — the shape matches but the value doesn't; falls through to a plain object
	// rather than erroring (literal.ts:307-310, C4).
	d, err := mongo.ParseDocumentLiteral(`{ $oid: 123 }`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(d) != 1 || d[0].Key != "$oid" || d[0].Value != float64(123) {
		t.Errorf("d = %+v, want the plain object {$oid: 123}", d)
	}
}

func TestLiteral_ResolvedBsonInstanceNestedPassesThroughUntouched(t *testing.T) {
	d, err := mongo.ParseDocumentLiteral(`{ a: ObjectId("507f191e810c19729de860ea") }`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(d) != 1 || d[0].Key != "a" {
		t.Fatalf("d = %+v", d)
	}
	if _, ok := d[0].Value.(bson.ObjectID); !ok {
		t.Errorf("d[0].Value = %T, want bson.ObjectID (untouched)", d[0].Value)
	}
}

// --- ParseDocumentLiteral / ParseFilterObject ---------------------------------------------------

func TestParseDocumentLiteral_RejectsNonObject(t *testing.T) {
	_, err := mongo.ParseDocumentLiteral("[1, 2]")
	wantQueryError(t, err, "document must be a JSON object")
}

func TestParseFilterObject_NilOrBlankIsEmptyDocument(t *testing.T) {
	d, err := mongo.ParseFilterObject(nil)
	if err != nil || len(d) != 0 {
		t.Fatalf("ParseFilterObject(nil) = %+v, %v", d, err)
	}
	blank := "   "
	d, err = mongo.ParseFilterObject(&blank)
	if err != nil || len(d) != 0 {
		t.Fatalf("ParseFilterObject(blank) = %+v, %v", d, err)
	}
}

func TestParseFilterObject_RejectsNonObject(t *testing.T) {
	text := "42"
	_, err := mongo.ParseFilterObject(&text)
	wantQueryError(t, err, `filter must be a JSON object literal, e.g. { field: "value" }`)
}

// --- position-bearing error messages, byte for byte ---------------------------------------------

func TestLiteral_PositionBearingErrorMessages(t *testing.T) {
	cases := []struct {
		name string
		text string
		want string
	}{
		{"unexpected character", "{a: 1} #", `Unexpected character "#" at position 7.`},
		{"expected punct", "{a: 1", `Expected “}” at position 5.`},
		{"unrecognized identifier", "foo(1)", `Unrecognized identifier “foo” at position 0.`},
		{"trailing content", "1 2", "Unexpected trailing content after the literal."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := mongo.ParseJSON5Literal(tc.text)
			wantQueryError(t, err, tc.want)
		})
	}
}

func TestLiteral_DeepNestingReturnsErrorNotOverflow(t *testing.T) {
	for name, open := range map[string]string{"array": "[", "object": "{a:", "ctor": "ObjectId("} {
		_, err := mongo.ParseJSON5Literal(strings.Repeat(open, 2_000_000))
		if code, ok := adapters.CodeOf(err); !ok || code != adapters.CodeQuery || !strings.Contains(err.Error(), "nested too deep") {
			t.Errorf("%s: err = %v, want E_QUERY nested too deep", name, err)
		}
	}
	if _, err := mongo.ParseJSON5Literal(strings.Repeat("[", 150) + strings.Repeat("]", 150)); err != nil {
		t.Errorf("150 levels: %v", err)
	}
}

func TestLiteral_NumberIntRejectsOutOfRangeAndFractions(t *testing.T) {
	for _, in := range []string{`NumberInt("3000000000")`, `NumberInt("-2147483649")`, `NumberInt("1.5")`} {
		if _, err := mongo.ParseJSON5Literal(in); err == nil {
			t.Errorf("%s: want error", in)
		}
	}
	if v := mustParseValue(t, `NumberInt("-2147483648")`); v != int32(-2147483648) {
		t.Errorf("min int32 = %v", v)
	}
}

func TestLiteral_DeprecatedEJSONTypesResolve(t *testing.T) {
	d, err := mongo.ParseDocumentLiteral(`{s: {"$symbol": "x"}, u: {"$undefined": true}}`)
	if err != nil {
		t.Fatalf("ParseDocumentLiteral: %v", err)
	}
	if _, ok := d[0].Value.(bson.Symbol); !ok {
		t.Errorf("$symbol = %T, want bson.Symbol", d[0].Value)
	}
	if _, ok := d[1].Value.(bson.Undefined); !ok {
		t.Errorf("$undefined = %T, want bson.Undefined", d[1].Value)
	}
}
