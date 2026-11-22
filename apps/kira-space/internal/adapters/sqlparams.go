package adapters

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// Bound parameters: values reach the server as bind parameters, never inserted into the statement
// text. This file holds what every SQL adapter and dbmcp share: the placeholder lexer, the decoded
// value shape and the op-log line.

// ParamStyle is how a kind writes a placeholder.
type ParamStyle int

const (
	ParamNone ParamStyle = iota
	// ParamDollar is Postgres `$1`.
	ParamDollar
	// ParamQuestion is `?`, MySQL, MariaDB and SQLite.
	ParamQuestion
	// ParamBraces is ClickHouse `{name:Type}`.
	ParamBraces
)

// ParamDialectOf returns the placeholder style and comment dialect of a connection kind; ok is false
// for a kind with no bind parameters.
func ParamDialectOf(kind string) (ParamStyle, SQLDialect, bool) {
	switch kind {
	case "postgres":
		return ParamDollar, PostgresDialect, true
	case "mysql", "mariadb":
		return ParamQuestion, MySQLDialect, true
	case "sqlite":
		return ParamQuestion, SQLiteDialect, true
	case "clickhouse":
		return ParamBraces, ClickHouseDialect, true
	}
	return ParamNone, SQLDialect{}, false
}

// Limits on bound values.
const (
	// MaxParamListItems bounds one list value.
	MaxParamListItems = 100_000
	// MaxPostgresParams is the protocol's parameter count limit.
	MaxPostgresParams = 65535
)

// PlaceholderSite is one placeholder in a statement, outside literals, quoted identifiers and comments.
type PlaceholderSite struct {
	// From and To are byte offsets into the statement.
	From, To int
	// Index is 1-based: the number of a Postgres `$n`, the occurrence of a `?`, the occurrence of a
	// ClickHouse `{name:Type}`.
	Index int
	// Name and Type are set for ClickHouse only.
	Name, Type string
}

// ParamSyntaxError is an app-authored refusal of a statement's placeholders. Its text names no value.
type ParamSyntaxError struct{ Msg string }

func (e *ParamSyntaxError) Error() string { return e.Msg }

func paramSyntax(format string, args ...any) error {
	return &ParamSyntaxError{Msg: fmt.Sprintf(format, args...)}
}

// codeMask marks the runes of statement that are plain code: outside quoted runs and comments.
func codeMask(r []rune, d SQLDialect) ([]bool, error) {
	mask := make([]bool, len(r))
	for i := 0; i < len(r); {
		c := r[i]
		switch {
		case isQuoteStart(c):
			end := scanQuote(r, i, false)
			if end-i < 2 || r[end-1] != c {
				return nil, paramSyntax("the statement has an unterminated quoted run; params need a statement Kira can read")
			}
			i = end
		case startsLineComment(r, i, d):
			for i < len(r) && r[i] != '\n' {
				i++
			}
		case startsBlockCommentOpen(r, i):
			depth := 1
			i += 2
			for i < len(r) && depth > 0 {
				switch {
				case startsBlockCommentClose(r, i):
					depth--
					i += 2
				case !d.FlatBlocks && startsBlockCommentOpen(r, i):
					depth++
					i += 2
				default:
					i++
				}
			}
			if depth > 0 {
				return nil, paramSyntax("the statement has an unterminated comment; params need a statement Kira can read")
			}
		default:
			mask[i] = true
			i++
		}
	}
	return mask, nil
}

// lexForParams runs the hazard check and builds the code mask.
func lexForParams(statement string, d SQLDialect) ([]rune, []bool, error) {
	if _, hazard := lexSQLComments(statement, d); hazard != "" {
		return nil, nil, paramSyntax("the statement has %s; params cannot be used with it", hazard)
	}
	r := []rune(statement)
	mask, err := codeMask(r, d)
	return r, mask, err
}

func isIdentStart(c rune) bool { return c == '_' || unicode.IsLetter(c) }

func isParamIdentRune(c rune) bool { return isIdentStart(c) || unicode.IsDigit(c) || c == '$' }

// PlaceholderSites lists the placeholders of statement in order. An error is a statement whose
// placeholders cannot be told apart from text for certain.
func PlaceholderSites(statement string, d SQLDialect, style ParamStyle) ([]PlaceholderSite, error) {
	r, mask, err := lexForParams(statement, d)
	if err != nil {
		return nil, err
	}
	// byteAt[i] is the byte offset of rune i; the last entry is len(statement).
	byteAt := make([]int, len(r)+1)
	for i, c := range r {
		byteAt[i+1] = byteAt[i] + utf8.RuneLen(c)
	}
	var sites []PlaceholderSite
	occurrence := 0
	for i := 0; i < len(r); i++ {
		if !mask[i] {
			continue
		}
		var site PlaceholderSite
		var end int
		var ok bool
		switch style {
		case ParamDollar:
			site, end, ok, err = dollarSite(r, i, byteAt)
		case ParamQuestion:
			site, end, ok, err = questionSite(r, i, d, byteAt, occurrence+1)
		case ParamBraces:
			site, end, ok = bracedSite(r, i, byteAt, occurrence+1)
		}
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		if style != ParamDollar {
			occurrence++
		}
		sites = append(sites, site)
		i = end - 1
	}
	return sites, nil
}

// dollarSite parses `$n` at r[i]; end is the rune index after it.
func dollarSite(r []rune, i int, byteAt []int) (site PlaceholderSite, end int, ok bool, err error) {
	if r[i] != '$' || i > 0 && isParamIdentRune(r[i-1]) {
		return site, 0, false, nil
	}
	j := i + 1
	for j < len(r) && r[j] >= '0' && r[j] <= '9' {
		j++
	}
	if j == i+1 {
		return site, 0, false, nil
	}
	n, perr := strconv.Atoi(string(r[i+1 : j]))
	if perr != nil || n < 1 || n > MaxPostgresParams {
		return site, 0, false, paramSyntax("placeholder %s is outside $1 to $%d", string(r[i:j]), MaxPostgresParams)
	}
	return PlaceholderSite{From: byteAt[i], To: byteAt[j], Index: n}, j, true, nil
}

// questionSite parses a plain `?` at r[i] (index is its occurrence number) and refuses the SQLite
// forms that are not plain `?`.
func questionSite(r []rune, i int, d SQLDialect, byteAt []int, index int) (site PlaceholderSite, end int, ok bool, err error) {
	c := r[i]
	if c == '?' {
		if d == SQLiteDialect && i+1 < len(r) && r[i+1] >= '0' && r[i+1] <= '9' {
			return site, 0, false, paramSyntax("numbered placeholders are not supported; use plain ? placeholders")
		}
		return PlaceholderSite{From: byteAt[i], To: byteAt[i+1], Index: index}, i + 1, true, nil
	}
	if d == SQLiteDialect && isSQLiteNamedStart(r, i) {
		return site, 0, false, paramSyntax("named placeholders are not supported; use plain ? placeholders")
	}
	return site, 0, false, nil
}

func isSQLiteNamedStart(r []rune, i int) bool {
	c := r[i]
	if c != ':' && c != '@' && c != '$' {
		return false
	}
	if i+1 >= len(r) || !isParamIdentRune(r[i+1]) {
		return false
	}
	return i == 0 || !isParamIdentRune(r[i-1]) && r[i-1] != ':'
}

// bracedSite parses `{name:Type}` at r[i] into a site with its byte span and occurrence index.
func bracedSite(r []rune, i int, byteAt []int, index int) (site PlaceholderSite, end int, ok bool) {
	if r[i] != '{' {
		return site, 0, false
	}
	site, end, ok = braceSite(r, i)
	if !ok {
		return site, 0, false
	}
	site.From, site.To, site.Index = byteAt[i], byteAt[end], index
	return site, end, true
}

// braceSite parses `{name:Type}` at r[i]; ok is false when r[i] opens something else.
func braceSite(r []rune, i int) (site PlaceholderSite, end int, ok bool) {
	j := i + 1
	for j < len(r) && r[j] == ' ' {
		j++
	}
	start := j
	if j >= len(r) || !isIdentStart(r[j]) {
		return site, 0, false
	}
	for j < len(r) && (isIdentStart(r[j]) || unicode.IsDigit(r[j])) {
		j++
	}
	name := string(r[start:j])
	for j < len(r) && r[j] == ' ' {
		j++
	}
	if j >= len(r) || r[j] != ':' {
		return site, 0, false
	}
	j++
	typeStart := j
	depth := 0
	for j < len(r) {
		switch r[j] {
		case '\'':
			// A quoted argument, as in DateTime64(3, 'UTC').
			end := scanQuote(r, j, false)
			if end-j < 2 || r[end-1] != '\'' {
				return site, 0, false
			}
			j = end
			continue
		case '(':
			depth++
		case ')':
			depth--
		case '}':
			if depth == 0 {
				typ := strings.TrimSpace(string(r[typeStart:j]))
				if typ == "" {
					return site, 0, false
				}
				return PlaceholderSite{Name: name, Type: typ}, j + 1, true
			}
		case ';', '\n':
			return site, 0, false
		}
		j++
	}
	return site, 0, false
}

// SecondStatement reports whether a `;` outside literals and comments is followed by anything but
// whitespace and comments.
func SecondStatement(statement string, d SQLDialect) bool {
	r, mask, err := lexForParams(statement, d)
	if err != nil {
		// A statement the lexer cannot read could hide a second one.
		return true
	}
	semi := false
	for i, c := range r {
		if !mask[i] || unicode.IsSpace(c) {
			continue
		}
		if semi {
			return true
		}
		semi = c == ';'
	}
	return false
}

// ParamKind is the type of a decoded value.
type ParamKind int

const (
	ParamNull ParamKind = iota
	ParamBool
	// ParamInt is an integer that fits int64.
	ParamInt
	// ParamNumber is any other JSON number, kept as its exact text.
	ParamNumber
	ParamString
	ParamList
)

// ParamValue is a kept value decoded for binding.
type ParamValue struct {
	Kind ParamKind
	Bool bool
	Int  int64
	// Text is a Number's exact JSON text, a String's value, or an Int's decimal text.
	Text string
	// Elem is a list's element kind (ParamNull for an empty list); Items its elements.
	Elem  ParamKind
	Items []ParamValue
	// JSON is the compact JSON the value decoded from.
	JSON string
}

// DecodeParam decodes one kept value. name only words the refusal. An object, a nested list, a null
// element or a list mixing types is refused.
func DecodeParam(name string, raw json.RawMessage) (ParamValue, error) {
	v, err := decodeScalarOrList(name, raw, true)
	if err != nil {
		return ParamValue{}, err
	}
	v.JSON = string(raw)
	return v, nil
}

func objectErr(name string) error {
	return paramSyntax("temporary variable %q is an object; only text, number, boolean or a list of one type binds as a parameter", name)
}

func decodeScalarOrList(name string, raw json.RawMessage, top bool) (ParamValue, error) {
	b := strings.TrimSpace(string(raw))
	if b == "" {
		return ParamValue{}, paramSyntax("temporary variable %q is empty", name)
	}
	switch b[0] {
	case '{':
		return ParamValue{}, objectErr(name)
	case '[':
		if !top {
			return ParamValue{}, paramSyntax("temporary variable %q holds a nested list; only a flat list of one type binds as a parameter", name)
		}
		return decodeList(name, raw)
	case 'n':
		if !top {
			return ParamValue{}, paramSyntax("temporary variable %q holds a null item; list items must be text, numbers or booleans", name)
		}
		return ParamValue{Kind: ParamNull}, nil
	case 't', 'f':
		return ParamValue{Kind: ParamBool, Bool: b == "true"}, nil
	case '"':
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return ParamValue{}, paramSyntax("temporary variable %q is not valid text", name)
		}
		return ParamValue{Kind: ParamString, Text: s}, nil
	}
	if n, err := strconv.ParseInt(b, 10, 64); err == nil {
		return ParamValue{Kind: ParamInt, Int: n, Text: strconv.FormatInt(n, 10)}, nil
	}
	return ParamValue{Kind: ParamNumber, Text: b}, nil
}

func decodeList(name string, raw json.RawMessage) (ParamValue, error) {
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return ParamValue{}, paramSyntax("temporary variable %q is not a valid list", name)
	}
	if len(items) > MaxParamListItems {
		return ParamValue{}, paramSyntax("temporary variable %q has %d items; a bound list takes at most %d", name, len(items), MaxParamListItems)
	}
	out := ParamValue{Kind: ParamList, Items: make([]ParamValue, 0, len(items))}
	for _, it := range items {
		v, err := decodeScalarOrList(name, it, false)
		if err != nil {
			return ParamValue{}, err
		}
		out.Items = append(out.Items, v)
		switch {
		case out.Elem == ParamNull:
			out.Elem = v.Kind
		case out.Elem == v.Kind:
		case (out.Elem == ParamInt || out.Elem == ParamNumber) && (v.Kind == ParamInt || v.Kind == ParamNumber):
			out.Elem = ParamNumber
		default:
			return ParamValue{}, paramSyntax("temporary variable %q mixes types in its list; a bound list holds one of text, numbers or booleans", name)
		}
	}
	return out, nil
}

// DecodeBinds decodes every bound value of a request.
func DecodeBinds(params []model.BindParam) ([]ParamValue, error) {
	out := make([]ParamValue, len(params))
	for i, p := range params {
		v, err := DecodeParam(p.Label, p.Value)
		if err != nil {
			return nil, err
		}
		out[i] = v
	}
	return out, nil
}

// CommandWithParams is the op-log command of a parameterised statement: the statement, then one
// comment line naming each placeholder and the label of the value bound to it. Never a value.
func CommandWithParams(command string, params []model.BindParam, style ParamStyle) string {
	if len(params) == 0 {
		return command
	}
	parts := make([]string, len(params))
	for i, p := range params {
		switch style {
		case ParamDollar:
			parts[i] = "$" + strconv.Itoa(i+1) + " = " + p.Label
		case ParamBraces:
			parts[i] = p.Name + " = " + p.Label
		default:
			parts[i] = "? = " + p.Label
		}
	}
	return command + "\n-- params: " + strings.Join(parts, ", ")
}

// RefuseParams is the first line of an adapter that takes no bind parameters.
func RefuseParams(kind string, req model.ConsoleRequest) error {
	if len(req.Params) == 0 {
		return nil
	}
	return T(CodeUnsupported, "backend.studio.adapter.noBoundParams.body", map[string]string{"kind": kind}, errors.New("bound parameters on a non-SQL console"))
}
