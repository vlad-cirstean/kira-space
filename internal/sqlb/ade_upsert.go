package sqlb

import (
	"strings"

	"github.com/huandu/go-sqlbuilder"
)

// AdeUpsertPartial is UpsertSet for a partial unique index: conflictWhere repeats the index
// predicate verbatim (a predicate cannot bind arguments). Same text in SQLite and Postgres.
func AdeUpsertPartial(ib *sqlbuilder.InsertBuilder, conflict []string, conflictWhere string, sets []string) *sqlbuilder.InsertBuilder {
	return ib.SQL("ON CONFLICT (" + strings.Join(conflict, ", ") + ") WHERE " + conflictWhere +
		" DO UPDATE SET " + strings.Join(sets, ", "))
}
