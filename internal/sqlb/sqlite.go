package sqlb

import (
	"github.com/huandu/go-sqlbuilder"
)

// Every SQLite-only fragment lives here, with its Postgres counterpart.

// RowID is SQLite's implicit insertion-order key. Postgres needs a real ordering column.
const RowID = "rowid"

// OffsetOnly skips n rows with no row cap. Call it after OrderBy. go-sqlbuilder drops OFFSET
// without LIMIT on SQLite, which would select every row. SQLite: LIMIT -1 OFFSET n; Postgres: OFFSET n.
func OffsetOnly(d Dialect, sb *sqlbuilder.SelectBuilder, n int) *sqlbuilder.SelectBuilder {
	if d.sqlite() {
		return sb.SQL("LIMIT -1 OFFSET " + sb.Var(n))
	}
	return sb.Offset(n)
}
