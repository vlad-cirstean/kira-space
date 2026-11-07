package sqlb

import (
	"strings"

	"github.com/huandu/go-sqlbuilder"
)

// Upsert appends ON CONFLICT (conflict...) DO UPDATE SET c = excluded.c for each update column,
// then the optional WHERE. Call it before Returning. The text is valid in SQLite and Postgres.
func Upsert(ib *sqlbuilder.InsertBuilder, conflict, update []string, where string) *sqlbuilder.InsertBuilder {
	sets := make([]string, len(update))
	for i, c := range update {
		sets[i] = c + " = excluded." + c
	}
	return UpsertSet(ib, conflict, sets, where)
}

// UpsertSet is Upsert with literal assignments, e.g. "rounds = rounds + 1".
func UpsertSet(ib *sqlbuilder.InsertBuilder, conflict, sets []string, where string) *sqlbuilder.InsertBuilder {
	tail := "ON CONFLICT (" + strings.Join(conflict, ", ") + ") DO UPDATE SET " + strings.Join(sets, ", ")
	if where != "" {
		tail += " WHERE " + where
	}
	return ib.SQL(tail)
}

// UpsertNothing appends ON CONFLICT (conflict...) DO NOTHING.
func UpsertNothing(ib *sqlbuilder.InsertBuilder, conflict []string) *sqlbuilder.InsertBuilder {
	return ib.SQL("ON CONFLICT (" + strings.Join(conflict, ", ") + ") DO NOTHING")
}
