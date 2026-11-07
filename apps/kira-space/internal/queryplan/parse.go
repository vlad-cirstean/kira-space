package queryplan

import (
	"errors"
	"fmt"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/page"
)

// ErrTruncated mirrors plan.ts's ExplainTruncatedError — a plan over page.MaxCellBytes arrives
// with its single cell clipped mid-JSON (postgres/mysql/mariadb/clickhouse all return their whole
// plan as one cell). A distinct, named condition lets a caller show something specific instead of
// an opaque JSON-parse failure.
var ErrTruncated = errors.New("the query plan was too large to parse")

func cellAt(pg page.TabularPage, row, col int) *string {
	if col < 0 || col >= len(pg.Chunks) {
		return nil
	}
	chunk := pg.Chunks[col]
	if page.IsNull(chunk, row) {
		return nil
	}
	text := page.CellText(chunk, row)
	return &text
}

func cellText(pg page.TabularPage, row, col int) string {
	if v := cellAt(pg, row, col); v != nil {
		return *v
	}
	return ""
}

// firstCellTruncated mirrors plan.ts's own — true when the single cell postgres/mysql/mariadb/
// clickhouse parse the whole plan from was clipped on the wire (page.MaxCellBytes).
func firstCellTruncated(pg page.TabularPage) bool {
	if len(pg.Chunks) == 0 {
		return false
	}
	return page.IsTruncated(pg.Chunks[0], 0)
}

func columnIndex(pg page.TabularPage, name string) int {
	for i, c := range pg.Columns {
		if c.Name == name {
			return i
		}
	}
	return -1
}

// FromPages ports plan.ts's parseExplainPages verbatim: turns the page.Page(s) an EXPLAIN batch
// returned back into one normalized Plan — the glue between statements.go's own composer and this
// package's five dialect parsers. Returns ErrTruncated for a clipped single-cell plan; any
// other page shape mismatch is a plain error naming what was expected. Both explain and
// auto-force-explain (internal/dbmcp's own explain.go) run this and must treat any error as "no
// plan" rather than failing the call outright — that posture lives in dbmcp, not here.
func FromPages(kind string, pages []page.Page, thresholdRows int) (Plan, error) {
	if len(pages) == 0 {
		return Plan{}, fmt.Errorf("queryplan: EXPLAIN returned no pages")
	}
	first, ok := pages[0].(page.TabularPage)
	if !ok {
		return Plan{}, fmt.Errorf("queryplan: EXPLAIN did not return a tabular result")
	}

	// A page the console cap stopped is an incomplete plan.
	for _, p := range pages {
		if tp, ok := p.(page.TabularPage); ok && tp.Position.Truncated {
			return Plan{}, ErrTruncated
		}
	}

	switch kind {
	case "postgres":
		if firstCellTruncated(first) {
			return Plan{}, ErrTruncated
		}
		return parsePostgresPlan(cellText(first, 0, 0), thresholdRows)
	case "mysql":
		if firstCellTruncated(first) {
			return Plan{}, ErrTruncated
		}
		return parseMysqlPlan(cellText(first, 0, 0), thresholdRows)
	case "mariadb":
		if firstCellTruncated(first) {
			return Plan{}, ErrTruncated
		}
		return parseMariadbPlan(cellText(first, 0, 0), thresholdRows)
	case "sqlite":
		idCol := columnIndex(first, "id")
		parentCol := columnIndex(first, "parent")
		detailCol := columnIndex(first, "detail")
		rows := make([]sqliteExplainRow, 0, first.RowCount)
		for r := 0; r < first.RowCount; r++ {
			rows = append(rows, sqliteExplainRow{
				id:     parseIntCell(cellText(first, r, idCol)),
				parent: parseIntCell(cellText(first, r, parentCol)),
				detail: cellText(first, r, detailCol),
			})
		}
		return parseSqlitePlan(rows), nil
	case "clickhouse":
		if firstCellTruncated(first) {
			return Plan{}, ErrTruncated
		}
		var estimateRows []estimateRow
		if len(pages) > 1 {
			if estimatePage, ok := pages[1].(page.TabularPage); ok {
				rowsCol := columnIndex(estimatePage, "rows")
				estimateRows = make([]estimateRow, 0, estimatePage.RowCount)
				for r := 0; r < estimatePage.RowCount; r++ {
					estimateRows = append(estimateRows, estimateRow{rows: parseFloatCell(cellText(estimatePage, r, rowsCol))})
				}
			}
		}
		return parseClickhousePlan(cellText(first, 0, 0), estimateRows, thresholdRows)
	default:
		return Plan{}, fmt.Errorf("queryplan: no EXPLAIN parser for connection kind %q", kind)
	}
}

func parseIntCell(s string) int {
	var n int
	_, _ = fmt.Sscanf(s, "%d", &n)
	return n
}

func parseFloatCell(s string) float64 {
	var f float64
	_, _ = fmt.Sscanf(s, "%g", &f)
	return f
}
