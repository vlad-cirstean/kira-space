package adapters

import (
	"context"
	"regexp"
	"strings"
)

// OpClass is what one console statement would do, as the connection's MCP permissions name it.
// A classification is about the statement's declared shape, not its effects: `SELECT
// write_function()` reads as ClassRead. This sits on top of the connection's own ReadOnly flag and
// each adapter's own enforcement (BEGIN READ ONLY, Mongo's per-method check, Redis's COMMAND
// check), never instead of them.
type OpClass string

const (
	ClassRead    OpClass = "read"
	ClassWrite   OpClass = "write"
	ClassDDL     OpClass = "ddl"
	ClassUnknown OpClass = "unknown" // classified by nothing; the caller applies its strictest rule
)

// StatementClassifier is optional adapter surface, deliberately not a method on Adapter: only the
// kinds with a console (Caps().SQL, or Mongo/Redis's own console grammar) can answer it at all,
// and adding a tenth method every adapter must stub would be the "hardcoded kind list" Caps exists
// to prevent, spelled differently.
type StatementClassifier interface {
	ClassifyStatement(ctx context.Context, statement string) (OpClass, error)
}

// RemoteClassifier is optional on a StatementClassifier: it reports that classifying can issue
// server round trips (redis fetches its COMMAND table on a cache miss). The router then takes the
// connection's throttle turn for the call; pure-parse classifiers stay free.
type RemoteClassifier interface {
	ClassifiesRemotely() bool
}

// sqlWriteKeywords is ClassifySQL's write vocabulary, matched against the statement's leading
// keyword only.
var sqlWriteKeywords = map[string]bool{
	"INSERT": true, "UPDATE": true, "DELETE": true, "MERGE": true, "REPLACE": true,
	"UPSERT": true, "COPY": true, "LOAD": true, "IMPORT": true,
}

// sqlDDLKeywords is ClassifySQL's DDL vocabulary, matched against the statement's leading keyword
// only. TRUNCATE counts as DDL, not write — the SQL standard's own placement.
var sqlDDLKeywords = map[string]bool{
	"CREATE": true, "ALTER": true, "DROP": true, "TRUNCATE": true, "RENAME": true,
	"COMMENT": true, "GRANT": true, "REVOKE": true, "ATTACH": true, "DETACH": true,
	"REINDEX": true, "VACUUM": true, "OPTIMIZE": true, "REFRESH": true, "CLUSTER": true,
	"ANALYZE": true,
}

// sqlReadKeywords is ClassifySQL's plain-read vocabulary, matched against the statement's leading
// keyword only. DESCRIBE/DESC stay out: they can wrap ANALYZE, so the EXPLAIN case below owns them.
var sqlReadKeywords = map[string]bool{
	"SHOW": true, "VALUES": true, "TABLE": true,
}

// sqlAnalyzeKeyword and sqlIntoKeyword are single-word sqlWordBoundaryContainsAny vocabularies,
// package-level for the same reason as sqlWriteKeywords/sqlDDLKeywords above: a literal
// map[string]bool{...} at each call site allocated fresh on every ExplainAnalyzeTarget/
// ClassifySQL call.
var sqlAnalyzeKeyword = map[string]bool{"ANALYZE": true, "ANALYSE": true}
var sqlIntoKeyword = map[string]bool{"INTO": true}

// sqlWordBoundaryContainsAny reports whether any upper-cased word in s (split on non-word runes)
// is a key of words — used where a keyword can appear anywhere in the statement (WITH bodies,
// SELECT ... INTO), not just leading.
func sqlWordBoundaryContainsAny(s string, words map[string]bool) bool {
	for _, field := range strings.FieldsFunc(s, isSQLWordBoundary) {
		if words[strings.ToUpper(field)] {
			return true
		}
	}
	return false
}

// isSQLWordBoundary splits on anything that is not a letter, digit or underscore — the same
// notion of "word" a regexp \b boundary uses, applied by hand so ClassifySQL needs no additional
// regexp beyond the ones sqlReadWrite already defines in errors.go.
func isSQLWordBoundary(r rune) bool {
	return !(r == '_' || (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z'))
}

// ExplainAnalyzeTarget inspects the (already comment/semicolon-stripped) text following the
// EXPLAIN keyword and reports whether ANALYZE was requested — either the bare `EXPLAIN ANALYZE
// ...` form or Postgres's parenthesised option-list form, `EXPLAIN (ANALYZE) ...` / `EXPLAIN
// (ANALYZE, BUFFERS) ...` — since either genuinely executes the target statement, unlike a plain
// EXPLAIN (the parenthesised form was previously missed entirely, since the token
// right after EXPLAIN there is `(ANALYZE)`/`(ANALYZE,`, never exactly "ANALYZE"). target is what
// remains to classify when hasAnalyze is true. ok is false only when rest opens with `(` but the
// option list never balances — the caller must not default to ClassRead over text it can't parse,
// since an unparseable option list might be hiding ANALYZE inside it.
//
// Exported so a per-dialect ClassifyStatement — clickhouse/console.go's own — can
// reuse this same parsing rather than re-implementing it, for a dialect where ClassifySQL's default
// EXPLAIN-without-ANALYZE-is-a-read assumption does not hold.
func ExplainAnalyzeTarget(rest string) (hasAnalyze bool, target string, ok bool) {
	if strings.HasPrefix(rest, "(") {
		r := []rune(rest)
		depth := 0
		end := -1
		for i, ch := range r {
			switch ch {
			case '(':
				depth++
			case ')':
				depth--
				if depth == 0 {
					end = i
				}
			}
			if end >= 0 {
				break
			}
		}
		if end < 0 {
			return false, "", false
		}
		options := string(r[1:end])
		target = strings.TrimSpace(string(r[end+1:]))
		return sqlWordBoundaryContainsAny(options, sqlAnalyzeKeyword), target, true
	}
	fields := strings.Fields(rest)
	if len(fields) > 0 && sqlAnalyzeKeyword[strings.ToUpper(fields[0])] {
		return true, strings.TrimSpace(rest[len(fields[0]):]), true
	}
	return false, rest, true
}

// ClassifySQL classifies statement by its leading keyword over comment-stripped text. It is
// leading-keyword classification, not a parser — see the package doc note on OpClass for what
// that does and does not guarantee.
//
// No SQL parser is used here, deliberately: this app runs five SQL dialects through one console
// path, and the question asked is one bit wide — what the statement's leading verb is. A
// dialect-bound parser (pg_query_go for Postgres, vitess's sqlparser for MySQL) would add more
// failure surface than it removes, and still answer nothing for ClickHouse.
func ClassifySQL(statement string, d SQLDialect) OpClass {
	stripped := strings.TrimSpace(StripOneTrailingSemicolon(StripSQLComments(statement, d)))

	// Embedded-semicolon guard: a leading keyword says nothing about a second statement smuggled
	// behind it, and whether a driver executes both is a per-driver DSN detail this classifier must
	// not depend on. Run against the RAW statement — never the
	// comment-stripped stripped above: StripSQLComments' quote scanner can itself be fooled by a
	// backslash-escaped quote in a dialect whose strings honour backslash (Postgres
	// E'...'/standard_conforming_strings=off, MySQL/MariaDB's own default), closing a quoted run
	// early and exposing a real trailing `;` as though it sat inside a `--` comment it never
	// was — checking raw text side-steps that class of bug entirely. A semicolon inside a
	// legitimate string literal (or genuinely inside a real comment) costs a false ClassUnknown (a
	// prompt); a missed second statement costs a silent write. Only one of those is acceptable.
	raw := StripOneTrailingSemicolon(strings.TrimSpace(statement))
	if strings.Contains(raw, ";") {
		return ClassUnknown
	}

	fields := strings.Fields(stripped)
	if len(fields) == 0 {
		return ClassUnknown
	}
	keyword := strings.ToUpper(fields[0])

	switch {
	case keyword == "SELECT":
		// SELECT ... INTO creates a table on Postgres and writes a file on MySQL.
		if sqlWordBoundaryContainsAny(stripped, sqlIntoKeyword) {
			return ClassWrite
		}
		return ClassRead
	case keyword == "WITH":
		// Postgres allows a data-modifying CTE, and a DDL statement can follow a CTE, so the whole
		// statement is scanned rather than just the leading keyword.
		if sqlWordBoundaryContainsAny(stripped, sqlDDLKeywords) {
			return ClassDDL
		}
		if sqlWordBoundaryContainsAny(stripped, sqlWriteKeywords) || sqlWordBoundaryContainsAny(stripped, sqlIntoKeyword) {
			return ClassWrite
		}
		return ClassRead
	case keyword == "ANALYZE":
		// MariaDB's ANALYZE [FORMAT=JSON] <stmt> executes <stmt>; Postgres/MySQL ANALYZE <table> is DDL.
		if target, ok := mariadbAnalyzeTarget(strings.TrimSpace(stripped[len(fields[0]):])); ok {
			return ClassifySQL(target, d)
		}
		return ClassDDL
	case sqlReadKeywords[keyword]:
		return ClassRead
	case keyword == "EXPLAIN" || keyword == "DESCRIBE" || keyword == "DESC":
		rest := strings.TrimSpace(stripped[len(fields[0]):])
		hasAnalyze, target, ok := ExplainAnalyzeTarget(rest)
		if !ok {
			// A leading parenthesised option list this scanner can't balance might be hiding
			// ANALYZE inside it — never assume ClassRead over an option list it can't parse.
			return ClassUnknown
		}
		if hasAnalyze {
			// EXPLAIN/DESCRIBE ANALYZE (bare or Postgres's `EXPLAIN (ANALYZE, ...)` form) genuinely runs
			// the statement — classify what follows.
			return ClassifySQL(target, d)
		}
		return ClassRead
	case sqlWriteKeywords[keyword]:
		return ClassWrite
	case sqlDDLKeywords[keyword]:
		return ClassDDL
	default:
		// PRAGMA, SET, USE, BEGIN, COMMIT, ROLLBACK, CALL, DO, EXEC and an empty statement all land
		// here. PRAGMA is called out because `PRAGMA journal_mode=WAL` writes; SET because a session
		// change is neither a read nor a write in this vocabulary; the transaction verbs because
		// AssertNoTransactionEscalation already rejects them on a read-only connection and they
		// express no data intent here.
		return ClassUnknown
	}
}

var (
	mariadbAnalyzeFormat = regexp.MustCompile(`(?i)^FORMAT\s*=\s*\w+\s+`)
	mariadbAnalyzeVerbs  = map[string]bool{"SELECT": true, "UPDATE": true, "DELETE": true, "INSERT": true, "REPLACE": true, "WITH": true}
)

// mariadbAnalyzeTarget returns the statement a MariaDB ANALYZE [FORMAT=...] prefix executes, when
// the remainder starts with a DML verb.
func mariadbAnalyzeTarget(rest string) (string, bool) {
	rest = mariadbAnalyzeFormat.ReplaceAllString(rest, "")
	f := strings.Fields(rest)
	return rest, len(f) > 0 && mariadbAnalyzeVerbs[strings.ToUpper(f[0])]
}
