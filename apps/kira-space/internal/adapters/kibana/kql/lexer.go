// Package kql compiles Kibana Query Language to Elasticsearch query DSL.
//
// Hand-written because no maintained Go KQL library exists: Kibana's own grammar (@kbn/es-query)
// is unpublished and licensed Elastic License 2.0 / SSPL / AGPL, so it is neither fully open
// source nor copied here, and a parser generator would add a module. The grammar below follows
// Kibana's public KQL documentation.
package kql

import (
	"strings"
	"unicode"
)

type tokKind int

const (
	tEOF tokKind = iota
	tLParen
	tRParen
	tLBrace
	tRBrace
	tColon
	tLt
	tLte
	tGt
	tGte
	tAnd
	tOr
	tNot
	tQuoted
	tWord
)

// part is a run of a value: literal text, or an unescaped * wildcard.
type part struct {
	text string
	star bool
}

type value struct {
	parts  []part
	quoted bool
}

// literal is the value with each wildcard shown as *.
func (v value) literal() string {
	var b strings.Builder
	for _, p := range v.parts {
		if p.star {
			b.WriteByte('*')
		} else {
			b.WriteString(p.text)
		}
	}
	return b.String()
}

func (v value) hasWildcard() bool {
	if v.quoted {
		return false
	}
	for _, p := range v.parts {
		if p.star {
			return true
		}
	}
	return false
}

// onlyWildcard reports a value that is a single *.
func (v value) onlyWildcard() bool {
	return !v.quoted && len(v.parts) == 1 && v.parts[0].star
}

type token struct {
	kind  tokKind
	pos   int    // 1-based character position
	space string // whitespace before the token
	val   value  // tQuoted, tWord
	raw   string // source text, for error messages
}

func (k tokKind) String() string {
	switch k {
	case tEOF:
		return "end of input"
	case tLParen:
		return "“(”"
	case tRParen:
		return "“)”"
	case tLBrace:
		return "“{”"
	case tRBrace:
		return "“}”"
	case tColon:
		return "“:”"
	case tLt:
		return "“<”"
	case tLte:
		return "“<=”"
	case tGt:
		return "“>”"
	case tGte:
		return "“>=”"
	case tAnd:
		return "“and”"
	case tOr:
		return "“or”"
	case tNot:
		return "“not”"
	case tQuoted:
		return "a quoted value"
	}
	return "a value"
}

var keywords = map[string]tokKind{"and": tAnd, "or": tOr, "not": tNot}

func isSpecial(r rune) bool {
	switch r {
	case '(', ')', ':', '<', '>', '"', '{', '}':
		return true
	}
	return false
}

// lex splits src into tokens. Unquoted words keep their escapes resolved and * wildcards marked.
func lex(src string) ([]token, error) {
	rs := []rune(src)
	var toks []token
	i := 0
	for {
		start := i
		for i < len(rs) && unicode.IsSpace(rs[i]) {
			i++
		}
		space := string(rs[start:i])
		if i >= len(rs) {
			toks = append(toks, token{kind: tEOF, pos: i + 1, space: space})
			return toks, nil
		}
		pos := i + 1
		c := rs[i]
		simple := func(k tokKind, n int) {
			toks = append(toks, token{kind: k, pos: pos, space: space, raw: string(rs[i : i+n])})
			i += n
		}
		switch {
		case c == '(':
			simple(tLParen, 1)
		case c == ')':
			simple(tRParen, 1)
		case c == '{':
			simple(tLBrace, 1)
		case c == '}':
			simple(tRBrace, 1)
		case c == ':':
			simple(tColon, 1)
		case c == '<' || c == '>':
			simple(cmpKind(rs, i))
		case c == '"':
			text, j, closed := lexQuoted(rs, i)
			if !closed {
				return nil, &SyntaxError{Pos: len(rs) + 1, Expected: []string{"“\"”"}, Found: "end of input"}
			}
			toks = append(toks, token{
				kind: tQuoted, pos: pos, space: space, raw: string(rs[i : j+1]),
				val: value{parts: []part{{text: text}}, quoted: true},
			})
			i = j + 1
		default:
			parts, j, escaped := lexWord(rs, i)
			raw := string(rs[i:j])
			tok := token{kind: tWord, pos: pos, space: space, raw: raw, val: value{parts: parts}}
			if !escaped {
				if k, ok := keywords[strings.ToLower(raw)]; ok {
					tok.kind = k
				}
			}
			toks = append(toks, tok)
			i = j
		}
	}
}

// cmpKind returns the comparison token kind at rs[i] and its length.
func cmpKind(rs []rune, i int) (tokKind, int) {
	lt := rs[i] == '<'
	if i+1 < len(rs) && rs[i+1] == '=' {
		if lt {
			return tLte, 2
		}
		return tGte, 2
	}
	if lt {
		return tLt, 1
	}
	return tGt, 1
}

// lexQuoted scans the quoted string opening at rs[i]. It returns the unescaped text, the index of the closing quote, and whether one was found.
func lexQuoted(rs []rune, i int) (string, int, bool) {
	j := i + 1
	var b strings.Builder
	for j < len(rs) {
		if rs[j] == '\\' && j+1 < len(rs) && (rs[j+1] == '"' || rs[j+1] == '\\') {
			b.WriteRune(rs[j+1])
			j += 2
			continue
		}
		if rs[j] == '"' {
			return b.String(), j, true
		}
		b.WriteRune(rs[j])
		j++
	}
	return b.String(), j, false
}

// lexWord scans the unquoted word starting at rs[i]. It returns its parts, the end index, and whether any escape appeared.
func lexWord(rs []rune, i int) ([]part, int, bool) {
	j := i
	var parts []part
	var cur strings.Builder
	flush := func() {
		if cur.Len() > 0 {
			parts = append(parts, part{text: cur.String()})
			cur.Reset()
		}
	}
	escaped := false
	for j < len(rs) && !unicode.IsSpace(rs[j]) && !isSpecial(rs[j]) {
		ch := rs[j]
		switch {
		case ch == '*':
			flush()
			parts = append(parts, part{star: true})
			j++
		case ch == '\\' && j+1 < len(rs):
			escaped = true
			j += lexEscape(rs[j+1:], &cur)
		default:
			cur.WriteRune(ch)
			j++
		}
	}
	flush()
	return parts, j, escaped
}

// lexEscape resolves the escape whose backslash precedes rest, writes it to cur, and returns the runes consumed including the backslash.
func lexEscape(rest []rune, cur *strings.Builder) int {
	next := rest[0]
	switch {
	case escapedKeyword(rest) != "":
		kw := escapedKeyword(rest)
		cur.WriteString(kw)
		return 1 + len(kw)
	case isSpecial(next) || next == '*' || next == '\\':
		cur.WriteRune(next)
	case next == 't':
		cur.WriteByte('\t')
	case next == 'n':
		cur.WriteByte('\n')
	case next == 'r':
		cur.WriteByte('\r')
	default:
		cur.WriteByte('\\')
		return 1
	}
	return 2
}

// escapedKeyword returns the keyword (in its original case) when rs starts with and/or/not.
func escapedKeyword(rs []rune) string {
	for _, kw := range []string{"and", "or", "not"} {
		if len(rs) >= len(kw) && strings.EqualFold(string(rs[:len(kw)]), kw) {
			return string(rs[:len(kw)])
		}
	}
	return ""
}
