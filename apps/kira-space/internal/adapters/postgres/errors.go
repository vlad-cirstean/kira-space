// Package postgres is the Go analogue of src/engine/adapters/postgres/: pgx/v5's native interface
// (*pgx.Conn, never database/sql, never pgxpool), one Conn per (connection, database) in
// an 8-entry LRU (client.go's ConnSet). File-for-file with the TypeScript it replaces:
// index.ts -> adapter.go (Go has no "index" convention), every other file keeps its TS name.
package postgres

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
)

// mapError is errors.ts's mapError. Go's error model has no single ".code" string the way a pg
// driver error does, so the three categories map from different sources: a *pgconn.PgError's own
// SQLSTATE code for E_AUTH/E_CANCELLED (28P01/28000 invalid password/authorization, 57014
// query_canceled — the same three SQLSTATEs errors.ts checks), and Go's own network-error types
// for E_CONNECT (a DNS failure, ECONNREFUSED, or a timeout during dial — errors.ts's
// ECONNREFUSED/ENOTFOUND/ETIMEDOUT, which are Node's names for the same three failure shapes).
// Anything else is E_QUERY, same as errors.ts's own fallback.
func mapError(err error) *adapters.Error {
	if err == nil {
		return nil
	}
	message := err.Error()

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "28P01", "28000":
			return adapters.New(adapters.CodeAuth, message, err)
		case "57014":
			return adapters.New(adapters.CodeCancelled, message, err)
		case "25006":
			return adapters.New(adapters.CodeQuery, message, adapters.RefusedWrite(err))
		}
		return adapters.New(adapters.CodeQuery, message, err)
	}

	if ne := adapters.ClassifyNetError(err); ne.DNS || ne.Refused || ne.NetTimeout {
		return adapters.New(adapters.CodeConnect, message, err)
	}

	return adapters.New(adapters.CodeQuery, message, err)
}
