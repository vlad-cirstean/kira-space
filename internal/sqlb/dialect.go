// Package sqlb builds the app's SQL for one runtime dialect and runs it. It wraps
// github.com/huandu/go-sqlbuilder; repos keep database/sql scanning.
package sqlb

import "github.com/huandu/go-sqlbuilder"

// Dialect picks placeholder, quoting and clause syntax. The zero value is SQLite.
type Dialect struct{ flavor sqlbuilder.Flavor }

// SQLite is the dialect of kira.db.
var SQLite = Dialect{flavor: sqlbuilder.SQLite}

func (d Dialect) fl() sqlbuilder.Flavor {
	if d.flavor == 0 {
		return sqlbuilder.SQLite
	}
	return d.flavor
}

func (d Dialect) sqlite() bool { return d.fl() == sqlbuilder.SQLite }

// Select starts a SELECT of cols.
func (d Dialect) Select(cols ...string) *sqlbuilder.SelectBuilder {
	return d.fl().NewSelectBuilder().Select(cols...)
}

// InsertInto starts an INSERT.
func (d Dialect) InsertInto(table string) *sqlbuilder.InsertBuilder {
	return d.fl().NewInsertBuilder().InsertInto(table)
}

// InsertIgnoreInto starts an insert that skips a conflicting row.
func (d Dialect) InsertIgnoreInto(table string) *sqlbuilder.InsertBuilder {
	return d.fl().NewInsertBuilder().InsertIgnoreInto(table)
}

// Update starts an UPDATE.
func (d Dialect) Update(table string) *sqlbuilder.UpdateBuilder {
	return d.fl().NewUpdateBuilder().Update(table)
}

// DeleteFrom starts a DELETE.
func (d Dialect) DeleteFrom(table string) *sqlbuilder.DeleteBuilder {
	return d.fl().NewDeleteBuilder().DeleteFrom(table)
}
