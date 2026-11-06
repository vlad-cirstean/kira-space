package adapters

// StatementRedactor is optional adapter surface: it returns a statement safe to keep in an audit
// trail (no literal data). ok is false when the statement cannot be redacted with certainty.
// Like StatementClassifier it is not a method on Adapter, so no kind list lives in a consumer.
type StatementRedactor interface {
	RedactStatement(statement string) (redacted string, ok bool)
}

const maskedLiteral = "'…'"

// MaskSQLLiterals replaces every string literal in a SQL statement with '…' and drops comments.
// Identifiers, keywords and numbers stay. MySQL-style dialects (HashComments) also treat "..." as a
// string. ok is false, and the statement must be withheld, when the scanner cannot be sure of a
// literal's extent: an SQLQuotingHazard (backslash escape, dollar quote, executable comment) or an
// unterminated quote.
func MaskSQLLiterals(statement string, d SQLDialect) (masked string, ok bool) {
	stripped, hazard := lexSQLComments(statement, d)
	if hazard != "" {
		return "", false
	}
	r := []rune(stripped)
	out := make([]rune, 0, len(r))
	for i := 0; i < len(r); {
		c := r[i]
		isString := c == '\'' || (c == '"' && d.HashComments)
		if !isString && (c == '"' || c == '`') {
			end := scanQuote(r, i, false)
			if end-i < 2 || r[end-1] != c {
				return "", false
			}
			out = append(out, r[i:end]...)
			i = end
			continue
		}
		if !isString {
			out = append(out, c)
			i++
			continue
		}
		end := scanQuote(r, i, false)
		if end-i < 2 || r[end-1] != c {
			return "", false
		}
		out = append(out, []rune(maskedLiteral)...)
		i = end
	}
	return string(out), true
}
