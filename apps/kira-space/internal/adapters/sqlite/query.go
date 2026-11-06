package sqlite

import (
	"context"
	"database/sql"
	"regexp"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
)

// The three patterns assertSingleStatement strips from the remainder after the first top-level
// ';' — query.ts's own "whitespace, one trailing ';', and SQL comments are the only things
// tolerated there", ported verbatim as regexes since the remainder, by construction, opens
// outside any string literal.
var (
	commentLine           = regexp.MustCompile(`--[^\n]*`)
	commentBlock          = regexp.MustCompile(`(?s)/\*.*?\*/`)
	whitespaceOrSemicolon = regexp.MustCompile(`[\s;]`)
)

const (
	scanNormal = iota
	scanLineComment
	scanBlockComment
	scanSingle
	scanDouble
	scanBacktick
)

// scanNormalArm handles the top-level state: comment/quote openers switch state, ';' is the match.
// A two-char opener (`--`, `/*`) reports nextIndex one past itself, same as the loop's own i++, so
// the caller's for-loop increment lands past both chars together.
func scanNormalArm(s string, i int) (nextIndex, nextState int, found bool) {
	c := s[i]
	switch {
	case c == '-' && i+1 < len(s) && s[i+1] == '-':
		return i + 1, scanLineComment, false
	case c == '/' && i+1 < len(s) && s[i+1] == '*':
		return i + 1, scanBlockComment, false
	case c == '\'':
		return i, scanSingle, false
	case c == '"':
		return i, scanDouble, false
	case c == '`':
		return i, scanBacktick, false
	case c == ';':
		return i, scanNormal, true
	}
	return i, scanNormal, false
}

func scanLineCommentArm(s string, i int) (nextIndex, nextState int) {
	if s[i] == '\n' {
		return i, scanNormal
	}
	return i, scanLineComment
}

func scanBlockCommentArm(s string, i int) (nextIndex, nextState int) {
	if s[i] == '*' && i+1 < len(s) && s[i+1] == '/' {
		return i + 1, scanNormal
	}
	return i, scanBlockComment
}

// scanQuoteArm is the shared body of the three quote states: a doubled quote char is an escaped
// quote (stays in state, consumes both), a lone one closes the literal.
func scanQuoteArm(s string, i int, quote byte, state int) (nextIndex, nextState int) {
	if s[i] == quote {
		if i+1 < len(s) && s[i+1] == quote {
			return i + 1, state
		}
		return i, scanNormal
	}
	return i, state
}

func scanSingleArm(s string, i int) (nextIndex, nextState int) {
	return scanQuoteArm(s, i, '\'', scanSingle)
}

func scanDoubleArm(s string, i int) (nextIndex, nextState int) {
	return scanQuoteArm(s, i, '"', scanDouble)
}

func scanBacktickArm(s string, i int) (nextIndex, nextState int) {
	return scanQuoteArm(s, i, '`', scanBacktick)
}

// firstTopLevelSemicolon scans sqlText once, tracking comment/string-literal state, and returns
// the byte index of the first ';' that appears outside a comment or a quoted literal — or -1 if
// there is none. This is Go's answer to query.ts's own guard, which instead compared the full text
// against `StatementSync.sourceSQL` (the exact substring node:sqlite's own prepare() compiled)
// — database/sql exposes no such "how much did the driver consume" signal for any
// driver, modernc.org/sqlite included (verified against its own stmt.go: a multi-statement string
// is detected internally via SQLite's own pzTail, but that fact never crosses the driver.Stmt
// interface), so the boundary is found by parsing the SQL text directly instead.
func firstTopLevelSemicolon(s string) int {
	state := scanNormal
	for i := 0; i < len(s); i++ {
		var next, nextState int
		var found bool
		switch state {
		case scanNormal:
			next, nextState, found = scanNormalArm(s, i)
		case scanLineComment:
			next, nextState = scanLineCommentArm(s, i)
		case scanBlockComment:
			next, nextState = scanBlockCommentArm(s, i)
		case scanSingle:
			next, nextState = scanSingleArm(s, i)
		case scanDouble:
			next, nextState = scanDoubleArm(s, i)
		case scanBacktick:
			next, nextState = scanBacktickArm(s, i)
		}
		if found {
			return next
		}
		i = next
		state = nextState
	}
	return -1
}

// assertSingleStatement is query.ts's own guard: whitespace, one trailing ';', and SQL comments
// after the first top-level ';' are the only things tolerated; anything else means a smuggled
// second statement — the console's own contract is one page per statement, and modernc.org/sqlite
// executes a multi-statement string in full rather than silently dropping the tail the way
// node:sqlite's prepare() did, so the enforcement has to happen here, before the driver ever sees
// the text.
func assertSingleStatement(sqlText string) error {
	idx := firstTopLevelSemicolon(sqlText)
	if idx < 0 {
		return nil
	}
	remainder := commentLine.ReplaceAllString(sqlText[idx+1:], "")
	remainder = commentBlock.ReplaceAllString(remainder, "")
	remainder = whitespaceOrSemicolon.ReplaceAllString(remainder, "")
	if remainder != "" {
		return adapters.T(adapters.CodeQuery, "backend.studio.adapter.multipleStatements.body", nil, nil)
	}
	return nil
}

var setCommand = adapters.SetCommand

// runRows is query.ts's runQuery generalized with a per-row scan callback, the same shape
// mysqlfamily/catalog.go's queryExec uses — catalog.go's typed pragma reads and read.go's own
// positional row reads both go through this. ctx is always the adapter-owned driverCtx
// (adapter.go's runOnConn), never the op's own context.
func runRows(ctx context.Context, conn *sql.Conn, sqlText string, params []any, op *adapters.OpCtx, logParams bool, scan func(*sql.Rows) error) error {
	setCommand(op, sqlText, params, logParams)
	if err := assertSingleStatement(sqlText); err != nil {
		return err
	}
	if err := adapters.CheckNotStarted(ctx); err != nil {
		return err
	}

	rows, err := conn.QueryContext(ctx, sqlText, params...)
	if err != nil {
		return mapError(err)
	}
	defer rows.Close()
	for rows.Next() {
		if err := scan(rows); err != nil {
			return mapError(err)
		}
	}
	if err := rows.Err(); err != nil {
		return mapError(err)
	}
	return nil
}

// runArrayQuery is read.ts's runQuery({rowsAsArray: true}) — every row scanned positionally into
// []any, the driver's own native Go value for each cell (nil/int64/float64/string/[]byte, plus the
// one coercion the driver performs itself for a DATE/DATETIME/TIMESTAMP-declared column — see
// read.go's selectExpr and toCellText). The setReadBigInts has no Go analogue to opt into: an
// int64 is already exact, so there is no equivalent to node:sqlite's ERR_OUT_OF_RANGE to avoid.
func runArrayQuery(ctx context.Context, conn *sql.Conn, sqlText string, params []any, op *adapters.OpCtx, logParams bool) ([][]any, error) {
	var out [][]any
	// The column count is fixed for the life of rows — rows.Columns() itself allocates a fresh
	// []string every call, so calling it once (P2 R2, task #95) instead of on every row cuts one
	// allocation the row count never actually needs.
	var numCols int
	var gotCols bool
	err := runRows(ctx, conn, sqlText, params, op, logParams, func(rows *sql.Rows) error {
		if !gotCols {
			cols, err := rows.Columns()
			if err != nil {
				return err
			}
			numCols = len(cols)
			gotCols = true
		}
		vals := make([]any, numCols)
		dest := make([]any, numCols)
		for i := range vals {
			dest[i] = &vals[i]
		}
		if err := rows.Scan(dest...); err != nil {
			return err
		}
		out = append(out, vals)
		return nil
	})
	return out, err
}

// streamArrayQuery is runArrayQuery's single-pass twin: onRow is called once per scanned
// row, in order, with a []any the callback owns, instead of every row being materialized into a
// [][]any only to be transposed into the page builder immediately after. runArrayQuery itself
// stays as the array-returning shape countRows wants.
func streamArrayQuery(ctx context.Context, conn *sql.Conn, sqlText string, params []any, op *adapters.OpCtx, logParams bool, onRow func(row []any) error) error {
	// See runArrayQuery's identical comment above: the column count never changes mid-query.
	var numCols int
	var gotCols bool
	return runRows(ctx, conn, sqlText, params, op, logParams, func(rows *sql.Rows) error {
		if !gotCols {
			cols, err := rows.Columns()
			if err != nil {
				return err
			}
			numCols = len(cols)
			gotCols = true
		}
		vals := make([]any, numCols)
		dest := make([]any, numCols)
		for i := range vals {
			dest[i] = &vals[i]
		}
		if err := rows.Scan(dest...); err != nil {
			return err
		}
		return onRow(vals)
	})
}

// runCommand is mutate.ts's runQuery-for-writes counterpart — INSERT/UPDATE/DELETE, never SELECT.
// suppressCommand mirrors mutate.ts's own "setCommand() was already called once for the whole
// batch" rule (P5 D9's precedent).
func runCommand(ctx context.Context, conn *sql.Conn, sqlText string, params []any, op *adapters.OpCtx, suppressCommand bool) (int64, error) {
	if !suppressCommand {
		op.SetCommand(sqlText)
	}
	if err := assertSingleStatement(sqlText); err != nil {
		return 0, err
	}
	if err := adapters.CheckNotStarted(ctx); err != nil {
		return 0, err
	}

	result, err := conn.ExecContext(ctx, sqlText, params...)
	if err != nil {
		return 0, mapError(err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return 0, mapError(err)
	}
	return n, nil
}
