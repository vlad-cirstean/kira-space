package clickhouse

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/page"
)

// ClassifyStatement satisfies adapters.StatementClassifier (M2) over the shared SQL classifier,
// with one ClickHouse-specific correction — see classifyClickHouseSQL.
func (a *Adapter) ClassifyStatement(_ context.Context, statement string) (adapters.OpClass, error) {
	return classifyClickHouseSQL(statement), nil
}

// classifyClickHouseSQL corrects ClassifySQL's own default for a bare (non-ANALYZE) EXPLAIN:
// on Postgres/MySQL/MariaDB/SQLite an EXPLAIN without ANALYZE never executes its
// target, so ClassifySQL classifies it ClassRead unconditionally — docs/HISTORY.md records that
// ClickHouse is "the one it does not clear", the reason queryplan.Explainable already
// refuses to ever compose a ClickHouse EXPLAIN over anything but SELECT/WITH. That restriction only
// covers explain_query's own composed statements, not run_query, which accepts raw SQL text a
// caller could type `EXPLAIN <DELETE ...>` into directly — classified ClassRead by the shared
// scanner, letting a connection with write:deny run a write through it. Reclassify by the
// EXPLAIN's own target instead, the same worst-case reasoning ClassifySQL already applies when
// ANALYZE is present, applied here unconditionally since this dialect's plain form is not cleared
// either.
func classifyClickHouseSQL(statement string) adapters.OpClass {
	// Embedded-semicolon guard against the RAW statement, same reasoning and same direction as
	// ClassifySQL's own — checked here too since the EXPLAIN-target recursion below
	// calls adapters.ClassifySQL on a substring that no longer includes whatever preceded the
	// EXPLAIN keyword itself.
	raw := adapters.StripOneTrailingSemicolon(strings.TrimSpace(statement))
	if strings.Contains(raw, ";") {
		return adapters.ClassUnknown
	}

	stripped := strings.TrimSpace(adapters.StripOneTrailingSemicolon(adapters.StripSQLComments(statement, adapters.ClickHouseDialect)))
	fields := strings.Fields(stripped)
	if len(fields) > 0 && strings.EqualFold(fields[0], "EXPLAIN") {
		rest := strings.TrimSpace(stripped[len(fields[0]):])
		// ClickHouse's own EXPLAIN grammar allows an optional kind keyword
		// (AST/SYNTAX/QUERY TREE/PLAN/PIPELINE/ESTIMATE/TABLE OVERRIDE), and PLAN accepts an
		// optional `k = v, ...` settings list, both before the actual target statement —
		// queryplan.StatementsFor composes exactly this shape ("EXPLAIN PLAN json = 1, ... SELECT
		// ..."). Left unstripped, ExplainAnalyzeTarget/ClassifySQL below see a leading "PLAN"/
		// "ESTIMATE" token instead of the real target and classify it ClassUnknown, so every
		// composed explain_query statement failed to classify.
		rest = stripClickHouseExplainKindAndSettings(rest)
		if hasAnalyze, target, ok := adapters.ExplainAnalyzeTarget(rest); ok && !hasAnalyze {
			return adapters.ClassifySQL(target, adapters.ClickHouseDialect)
		}
	}
	return adapters.ClassifySQL(statement, adapters.ClickHouseDialect)
}

// chExplainKindRE matches ClickHouse's optional EXPLAIN kind keyword. ANALYZE is deliberately not
// here — ExplainAnalyzeTarget already recognizes it, and stripping it here would defeat that check.
var chExplainKindRE = regexp.MustCompile(`(?i)^(QUERY\s+TREE|TABLE\s+OVERRIDE|AST|SYNTAX|PLAN|PIPELINE|ESTIMATE)\b`)

// chExplainSettingsRE matches PLAN's optional `k = v, k2 = v2, ...` settings list, requiring
// trailing whitespace before the target statement so it never over-consumes into it.
var chExplainSettingsRE = regexp.MustCompile(
	`(?i)^((?:[A-Za-z_][A-Za-z0-9_]*\s*=\s*(?:'[^']*'|[^,\s]+)\s*,\s*)*` +
		`[A-Za-z_][A-Za-z0-9_]*\s*=\s*(?:'[^']*'|[^,\s]+)\s+)`,
)

// stripClickHouseExplainKindAndSettings strips a leading EXPLAIN kind keyword and its optional
// settings list (see StatementsFor's own ClickHouse composition), leaving the actual target
// statement for ExplainAnalyzeTarget/ClassifySQL to classify.
func stripClickHouseExplainKindAndSettings(rest string) string {
	kind := chExplainKindRE.FindString(rest)
	if kind == "" {
		return rest
	}
	rest = strings.TrimSpace(rest[len(kind):])
	if settings := chExplainSettingsRE.FindString(rest); settings != "" {
		rest = strings.TrimSpace(rest[len(settings):])
	}
	return rest
}

// leadingCommentRE/rowReturningRE are console.ts's own — the HTTP interface gives no cheap
// "will this return rows" check before executing (unlike SQLite's own zero-column QueryContext
// signal or MariaDB's OkPacket-vs-rows shape) — a leading-keyword heuristic decides a streamed
// query vs a command, skipping past leading comments first. This matters beyond cosmetics:
// appending FORMAT to a non-SELECT statement would be wrong for an INSERT, whose own FORMAT names
// the *input* data's format.
var (
	leadingCommentRE = regexp.MustCompile(`(?s)^\s*(?:(?:--|#)[^\n]*\n|/\*.*?\*/\s*)*`)
	rowReturningRE   = regexp.MustCompile(`(?i)^\s*(?:\(|(?:SELECT|WITH|SHOW|DESCRIBE|DESC|EXPLAIN|EXISTS|FROM)\b)`)
)

func isRowReturning(sql string) bool {
	stripped := leadingCommentRE.ReplaceAllString(sql, "")
	return rowReturningRE.MatchString(stripped)
}

// cancelOnCloseSettings makes the server cancel a read when the client closes the connection at the
// cap; allowed alongside the console's readonly=2.
var cancelOnCloseSettings = map[string]string{"cancel_http_readonly_queries_on_client_close": "1"}

func runRowReturning(ctx context.Context, h *Handle, queryID string, op *adapters.OpCtx, track TrackQuery, sql string, limit page.ResultCap, extra map[string]string) (page.TabularPage, error) {
	var columns []page.ColumnDescriptor
	var builder *page.TabularPageBuilder
	truncated := false
	err := StreamQuery(ctx, h, queryID, sql, op, track, mergeSettings(cancelOnCloseSettings, extra), nil, func(names, types []string) {
		// §8.14's console never consults the catalog — nullability/PK-ness are unknowable here;
		// console results are always read-only regardless (mirrors mysql-family/console.go).
		columns = make([]page.ColumnDescriptor, len(names))
		for i, name := range names {
			t := "String"
			if i < len(types) {
				t = types[i]
			}
			columns[i] = page.ColumnDescriptor{Name: name, DataType: t, TypeClass: typeClassFor(t), Nullable: true}
		}
		builder = page.NewTabularPageBuilder(columns)
	}, func(values []*string) bool {
		if builder == nil {
			return true
		}
		// A row just arrived, so another existed; stopping makes truncated mean "more existed".
		if limit.Reached(builder.RowCount(), builder.Bytes()) {
			truncated = true
			return false
		}
		_ = builder.AppendRow(values)
		return true
	})
	if err != nil {
		return page.TabularPage{}, err
	}
	if builder == nil {
		builder = page.NewTabularPageBuilder(columns)
	}
	return builder.Finish(page.CappedPosition(builder.RowCount(), truncated)), nil
}

func mergeSettings(base, extra map[string]string) map[string]string {
	if len(extra) == 0 {
		return base
	}
	out := make(map[string]string, len(base)+len(extra))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range extra {
		out[k] = v
	}
	return out
}

// Setting names the engine timeout sends; a refusal naming either means the profile forbids them.
const (
	settingMaxExecutionTime    = "max_execution_time"
	settingTimeoutOverflowMode = "timeout_overflow_mode"
)

// engineTimeoutSettings is the per-request timeout settings, nil when none apply.
func engineTimeoutSettings(h *Handle, timeout time.Duration) map[string]string {
	if timeout <= 0 || h.noEngineTimeout.Load() {
		return nil
	}
	seconds := (timeout + time.Second - 1) / time.Second
	return map[string]string{settingMaxExecutionTime: strconv.FormatInt(int64(seconds), 10), settingTimeoutOverflowMode: "throw"}
}

// settingsRefused reports a server refusal of the timeout settings themselves (a readonly=1
// profile: code 164 naming the setting, or 452 constraint violation).
func settingsRefused(err error) bool {
	code := codeOf(err)
	if code != readonlyCode && code != settingViolation {
		return false
	}
	return strings.Contains(err.Error(), settingMaxExecutionTime) || strings.Contains(err.Error(), settingTimeoutOverflowMode)
}

// withEngineTimeout runs one request with the engine timeout settings. Settings are validated before
// execution, so a refusal means nothing ran and one retry without them is safe; later requests
// skip them and only the host backstop enforces the limit.
func withEngineTimeout[T any](h *Handle, timeout time.Duration, log func(level, msg string), run func(extra map[string]string) (T, error)) (T, error) {
	extra := engineTimeoutSettings(h, timeout)
	v, err := run(extra)
	if extra != nil && err != nil && settingsRefused(err) {
		if h.noEngineTimeout.CompareAndSwap(false, true) && log != nil {
			log("warn", "clickhouse: server refused the statement timeout settings; the app timer enforces the limit")
		}
		return run(nil)
	}
	return v, err
}

// execute is console.ts's own execute.
func execute(ctx context.Context, h *Handle, op *adapters.OpCtx, track TrackQuery, statements []string, nextQueryID func() string, limit page.ResultCap, timeout time.Duration, log func(level, msg string)) (pages []page.Page, err error) {
	defer func() {
		// Stop cancels ctx first; a timeout leaves it live.
		if err != nil && timeout > 0 && ctx.Err() == nil && codeOf(err) == timeoutExceeded {
			err = adapters.StatementTimeout(timeout, 0, err)
		}
	}()
	if len(statements) == 0 {
		return nil, adapters.T(adapters.CodeQuery, "backend.studio.adapter.noStatements.body", nil, nil)
	}
	// One op-log row for the whole batch (P5 D9's precedent) — StreamQuery/RunCommand deliberately
	// never call op.SetCommand() themselves so this one call is authoritative.
	op.SetCommand(adapters.JoinConsoleStatements(statements, "--", "#"))

	pages = make([]page.Page, len(statements))
	for i, sql := range statements {
		if err := adapters.CheckCancelled(ctx); err != nil {
			return nil, err
		}
		if isRowReturning(sql) {
			p, err := withEngineTimeout(h, timeout, log, func(extra map[string]string) (page.TabularPage, error) {
				return runRowReturning(ctx, h, nextQueryID(), op, track, sql, limit, extra)
			})
			if err != nil {
				return nil, err
			}
			pages[i] = p
			limit = adapters.RemainingCap(limit, p.Size())
		} else {
			written, err := withEngineTimeout(h, timeout, log, func(extra map[string]string) (int64, error) {
				return RunCommand(ctx, h, nextQueryID(), sql, op, track, extra)
			})
			if err != nil {
				return nil, err
			}
			pages[i] = adapters.SingleStatusPage(strconv.Itoa(int(written))+" row(s) written", "String")
		}
	}
	return pages, nil
}

// RedactStatement satisfies adapters.StatementRedactor: string literals masked, comments dropped.
func (a *Adapter) RedactStatement(statement string) (string, bool) {
	return adapters.MaskSQLLiterals(statement, adapters.ClickHouseDialect)
}
