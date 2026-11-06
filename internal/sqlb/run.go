package sqlb

import (
	"database/sql"
	"errors"
)

// Querier is satisfied by *sql.DB and *sql.Tx.
type Querier interface {
	Exec(query string, args ...any) (sql.Result, error)
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
}

// Builder is any go-sqlbuilder statement builder.
type Builder interface {
	Build() (string, []any)
}

// ErrEmptyStatement means an UPDATE had no assignment.
var ErrEmptyStatement = errors.New("sqlb: empty statement")

func build(b Builder) (string, []any, error) {
	if u, ok := b.(interface{ NumAssignment() int }); ok && u.NumAssignment() == 0 {
		return "", nil, ErrEmptyStatement
	}
	q, args := b.Build()
	return q, args, nil
}

// Exec builds b and runs it.
func Exec(q Querier, b Builder) (sql.Result, error) {
	query, args, err := build(b)
	if err != nil {
		return nil, err
	}
	return q.Exec(query, args...)
}

// Query builds b and runs it.
func Query(q Querier, b Builder) (*sql.Rows, error) {
	query, args, err := build(b)
	if err != nil {
		return nil, err
	}
	return q.Query(query, args...)
}

// QueryRow builds b and runs it. Use it for SELECT builders; they never fail to build.
func QueryRow(q Querier, b Builder) *sql.Row {
	query, args := b.Build()
	return q.QueryRow(query, args...)
}
