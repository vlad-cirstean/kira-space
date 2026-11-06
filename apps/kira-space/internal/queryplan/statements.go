package queryplan

import (
	"regexp"
	"strings"
)

// leadingCommentRE strips leading SQL comments the same way explain.ts's own does (clickhouse/
// console.go's leadingCommentRE precedent) — a line comment or a block comment, repeated, before
// the leading keyword check.
var leadingCommentRE = regexp.MustCompile(`(?s)^\s*(?:--[^\n]*\n|/\*.*?\*/\s*)*`)

var explainableRE = regexp.MustCompile(`(?i)^(SELECT|WITH)\b`)
var trailingSemicolonRE = regexp.MustCompile(`;\s*$`)

// Explainable ports explain.ts's isExplainable verbatim:
// strips leading comments, then requires a leading SELECT/WITH. Reused, not re-decided — see
// explain_query's own refusal of anything else.
//
// A leading SELECT/WITH says nothing about a second statement smuggled behind it — the
// splitter that decided this was "one statement" can itself be fooled, and Postgres's simple
// protocol runs every command in one Query message, so a merged statement would execute past the
// EXPLAIN wrapper. Checked against the RAW text, after stripping at most one trailing `;` — never
// the caller's own span boundaries — mirroring adapters.ClassifySQL's own embedded-semicolon guard
// (internal/adapters/classify.go). A `;` genuinely inside a string literal costs a disabled Explain
// action, never a write.
func Explainable(sql string) bool {
	stripped := strings.TrimLeft(leadingCommentRE.ReplaceAllString(sql, ""), " \t\r\n")
	if !explainableRE.MatchString(stripped) {
		return false
	}
	raw := trailingSemicolonRE.ReplaceAllString(sql, "")
	return !strings.Contains(raw, ";")
}

// StatementsFor ports explain.ts's explainStatementsFor verbatim, keyed on kind exactly
// as the TypeScript side is — MariaDB and MySQL share the 'mysql' dialect elsewhere in this app,
// but report two genuinely different EXPLAIN FORMAT=JSON schemas, so the finer-grained kind
// is required here too. Never composes ANALYZE on any dialect.
func StatementsFor(kind, sql string) []string {
	switch kind {
	case "postgres":
		return []string{"EXPLAIN (FORMAT JSON, COSTS TRUE, VERBOSE FALSE, SETTINGS FALSE, BUFFERS FALSE) " + sql}
	case "mysql", "mariadb":
		return []string{"EXPLAIN FORMAT=JSON " + sql}
	case "sqlite":
		return []string{"EXPLAIN QUERY PLAN " + sql}
	case "clickhouse":
		return []string{
			"EXPLAIN PLAN json = 1, indexes = 1, description = 1 " + sql,
			"EXPLAIN ESTIMATE " + sql,
		}
	default:
		return []string{}
	}
}

// Supported reports whether kind has an EXPLAIN this package can compose and parse — postgres,
// mysql, mariadb, sqlite, clickhouse (M3 §7). One function, no second list: it is exactly
// len(StatementsFor(kind, "x")) > 0.
func Supported(kind string) bool {
	return len(StatementsFor(kind, "x")) > 0
}

// HasRowEstimate reports whether kind's plan carries an estimated rows read: Supported minus sqlite, whose
// EXPLAIN QUERY PLAN has none. Mirrored by connection.ts ROW_ESTIMATE_KINDS.
func HasRowEstimate(kind string) bool {
	return Supported(kind) && kind != "sqlite"
}
