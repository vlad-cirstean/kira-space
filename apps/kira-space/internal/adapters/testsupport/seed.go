package testsupport

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// seedDatabase runs sql (the repo's own packages/db-fixtures/fixtures/0001_seed.sql, unmodified)
// against uri. Postgres's simple query protocol natively executes a semicolon-separated
// multi-statement body as one message; pgx's Exec sends the simple protocol when called with no
// arguments, so the whole seed file runs in one call, exactly as psql or node-postgres's own
// multi-statement query would.
func seedDatabase(ctx context.Context, uri, sql string) error {
	conn, err := pgx.Connect(ctx, uri)
	if err != nil {
		return err
	}
	defer conn.Close(context.Background())

	_, err = conn.Exec(ctx, sql)
	return err
}

// bigRowsCount is postgres.ts's BIG_ROWS.
const bigRowsCount = 1_000_000

// seedBigRows is postgres.ts's start()'s own big_rows population step: filling app.big_rows (an
// empty table by the time seedDatabase returns — its CREATE TABLE is the only statement the seed
// SQL itself carries for it) with a million rows and ANALYZE-ing it, so reltuples is populated for
// the tree's own "~N" detail and the table has real data for the paging acceptance cases.
func seedBigRows(ctx context.Context, uri string) error {
	conn, err := pgx.Connect(ctx, uri)
	if err != nil {
		return err
	}
	defer conn.Close(context.Background())

	if _, err := conn.Exec(ctx,
		"INSERT INTO app.big_rows SELECT i, md5(i::text) FROM generate_series(1, $1) i", bigRowsCount,
	); err != nil {
		return err
	}
	_, err = conn.Exec(ctx, "ANALYZE app.big_rows")
	return err
}
