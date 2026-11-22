package keptvars

import (
	"encoding/json"
	"regexp"
	"strings"
)

// ScriptSite is one literal plain.x, plain["x"] or secret.x mention in a script. From and To are UTF-16 code
// units of the source, the unit an editor indexes by.
type ScriptSite struct {
	// Name is as written; it may not be a valid variable name (the run refuses it).
	Name     string
	Kind     string // "plain" or "secret"
	Op       string // "read", "write" or "delete"; a secret mention is always a write
	From, To int
}

// Script site kinds and operations.
const (
	KindPlain  = "plain"
	KindSecret = "secret"
	OpRead     = "read"
	OpWrite    = "write"
	OpDelete   = "delete"
)

var scriptRefRE = regexp.MustCompile(`\b(plain|secret)\s*(?:\.\s*([A-Za-z_$][\w$]*)|\[\s*(?:"([^"\\]*)"|'([^'\\]*)')\s*\])`)

// assignOps are the tokens that assign to the target on their left, longest first.
var assignOps = []string{">>>=", "**=", "<<=", ">>=", "&&=", "||=", "??=", "+=", "-=", "*=", "/=", "%=", "&=", "|=", "^="}

// ScanScript lists the literal plain and secret mentions of src. Comments and strings count too:
// over-listing is the safe side, because the script's globals hold only the names the scan found.
func ScanScript(src string) []ScriptSite {
	matches := scriptRefRE.FindAllStringSubmatchIndex(src, -1)
	out := make([]ScriptSite, 0, len(matches))
	for _, m := range matches {
		if memberOf(src, m[0]) {
			continue
		}
		start, end := m[4], m[5]
		if start < 0 {
			start, end = m[6], m[7]
		}
		if start < 0 {
			start, end = m[8], m[9]
		}
		kind := src[m[2]:m[3]]
		op := scriptOp(src, m[0], m[1])
		if kind == KindSecret {
			op = OpWrite
		}
		out = append(out, ScriptSite{
			Name: src[start:end], Kind: kind, Op: op,
			From: ToUTF16(src, m[0]), To: ToUTF16(src, m[1]),
		})
	}
	return out
}

// memberOf reports whether the identifier at i is a property of something (a.plain) or part of a longer name.
func memberOf(src string, i int) bool {
	j := i
	for j > 0 && (src[j-1] == ' ' || src[j-1] == '\t' || src[j-1] == '\n' || src[j-1] == '\r') {
		j--
	}
	return j > 0 && (src[j-1] == '.' && !strings.HasSuffix(src[:j], "..") || src[j-1] == '$')
}

// scriptOp classifies the mention src[from:to]: the member chain after it is skipped, so plain.x.y = 1 and
// plain.list[0]++ write x.
func scriptOp(src string, from, to int) string {
	before := strings.TrimRight(src[:from], " \t\r\n")
	end := skipChain(src, to)
	chained := end > to
	rest := strings.TrimLeft(src[end:], " \t\r\n")
	switch {
	case strings.HasSuffix(before, "++") || strings.HasSuffix(before, "--"),
		strings.HasPrefix(rest, "++"), strings.HasPrefix(rest, "--"):
		return OpWrite
	case strings.HasPrefix(rest, "=") && !strings.HasPrefix(rest, "==") && !strings.HasPrefix(rest, "=>"):
		return OpWrite
	}
	for _, op := range assignOps {
		if strings.HasPrefix(rest, op) {
			return OpWrite
		}
	}
	if strings.HasSuffix(before, "delete") && (len(before) == 6 || !isIdentByte(before[len(before)-7])) {
		if chained {
			return OpWrite
		}
		return OpDelete
	}
	return OpRead
}

func isIdentByte(c byte) bool {
	return c == '_' || c == '$' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

// skipChain returns the index past the .name and [expr] members that follow i.
func skipChain(src string, i int) int {
	for {
		j := skipBlank(src, i, " \t\n\r")
		var next int
		switch {
		case j < len(src) && src[j] == '.':
			next = skipDotMember(src, j)
		case j < len(src) && src[j] == '[':
			next = skipBracket(src, j)
		}
		if next == 0 {
			return i
		}
		i = next
	}
}

func skipBlank(src string, i int, set string) int {
	for i < len(src) && strings.IndexByte(set, src[i]) >= 0 {
		i++
	}
	return i
}

// skipDotMember returns the index past the .name at j, or 0 when no name follows the dot.
func skipDotMember(src string, j int) int {
	k := skipBlank(src, j+1, " \t")
	e := k
	for e < len(src) && isIdentByte(src[e]) {
		e++
	}
	if e == k {
		return 0
	}
	return e
}

// skipBracket returns the index past the balanced [expr] at j, or 0 when it never closes.
func skipBracket(src string, j int) int {
	depth := 0
	for k := j; k < len(src); k++ {
		switch src[k] {
		case '[':
			depth++
		case ']':
			if depth--; depth == 0 {
				return k + 1
			}
		}
	}
	return 0
}

// Describe reports what a value is without storing it: its compact size, JSON type and item count. The
// error is the one Put would give.
func Describe(raw json.RawMessage) (typ string, items, bytes int, err error) {
	compact, typ, items, err := analyze(raw)
	return typ, items, len(compact), err
}
