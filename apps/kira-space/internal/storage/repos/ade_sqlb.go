package repos

import (
	"database/sql"

	"github.com/huandu/go-sqlbuilder"

	"github.com/kirathecat/kira-studio/internal/kiraschema"
	"github.com/kirathecat/kira-studio/internal/sqlb"
	"github.com/kirathecat/kira-studio/internal/sqlitex"
)

// adeOne runs b and hands the one row to scan; (nil, nil) when no row matches.
func adeOne[T any](q sqlb.Querier, b sqlb.Builder, scan func(*sql.Row) (*T, error)) (*T, error) {
	query, args := b.Build()
	return sqlitex.QueryOne(q, scan, query, args...)
}

// adeQual prefixes every column with alias.
func adeQual(alias string, cols []string) []string {
	out := make([]string, len(cols))
	for i, c := range cols {
		out[i] = alias + "." + c
	}
	return out
}

// adeCount runs SELECT COUNT(*) FROM table WHERE where(sb)...
func adeCount(q sqlb.Querier, d sqlb.Dialect, table string, where func(sb *sqlbuilder.SelectBuilder) []string) (int, error) {
	sb := d.Select("COUNT(*)").From(table)
	if where != nil {
		sb.Where(where(sb)...)
	}
	var n int
	err := sqlb.QueryRow(q, sb).Scan(&n)
	return n, err
}

// adeLiveTaskIDs selects the ids of unarchived tasks, for IN subqueries.
func adeLiveTaskIDs(d sqlb.Dialect) *sqlbuilder.SelectBuilder {
	t := kiraschema.AdeTasks
	sb := d.Select(t.ID).From(t.Table)
	sb.Where(sb.IsNull(t.ArchivedAt))
	return sb
}
