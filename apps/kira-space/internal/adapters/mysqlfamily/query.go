package mysqlfamily

import (
	"context"
	"database/sql"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
)

// RunningQuery is query.ts's RunningQuery.
type RunningQuery struct {
	ThreadID uint32
	// Guard is the connection the query runs on: Cancel registers its pending cancel there so the
	// connection is not handed to the next op while the cancel can still reach its thread.
	Guard *adapters.ConnGuard
}

// TrackQuery is query.ts's TrackQuery: registers a running query and returns its own release,
// called once the statement settles. The identity-checked release lives in the caller
// (adapter.go's trackerFor), same as postgres's.
type TrackQuery func(RunningQuery) (release func())

// QueryOptions is query.ts's QueryOptions, minus rowsAsArray: runArrayQuery below always scans
// positionally, so there is no separate rows-as-object mode to select.
type QueryOptions struct {
	// LogParams appends the bound parameter values to the logged command, read path only.
	LogParams bool
}

var setCommand = adapters.SetCommand

// runArrayQuery is read.ts's/console.ts's own runQuery({rowsAsArray: true, ...}) call shape: every
// row scanned as []*string in column order, nil for SQL NULL, via the DatabaseTypeName-driven
// cellText — never a typeCast callback (the driver has none). The server-side kill is entirely
// adapter.go's Cancel (KILL QUERY over a side connection) — this function does not and must not
// try to make the query itself abort (adapters.RunWithAbortRace's own contract).
func runArrayQuery(ctx context.Context, conn Entry, query string, params []any, op *adapters.OpCtx, track TrackQuery, opts QueryOptions) ([][]*string, error) {
	setCommand(op, query, params, opts.LogParams)
	if err := adapters.CheckNotStarted(ctx); err != nil {
		return nil, err
	}

	release := track(conn.running())
	done := conn.track()

	return adapters.RunWithAbortRace(ctx, func() { release(); done() }, func(queryCtx context.Context) ([][]*string, error) {
		rows, err := conn.QueryContext(queryCtx, query, params...)
		if err != nil {
			return nil, mapError(err)
		}
		defer rows.Close()

		types, err := rows.ColumnTypes()
		if err != nil {
			return nil, mapError(err)
		}
		dbTypes := make([]string, len(types))
		for i, t := range types {
			dbTypes[i] = t.DatabaseTypeName()
		}

		// raw/dest are pure scan scratch: rows.Scan fills raw in place, and cellText copies every
		// non-nil cell into a new Go string this same iteration, so neither needs to outlive it —
		// one allocation of each for the whole query instead of one pair per row (P2 R2, task #95).
		raw := make([]sql.RawBytes, len(types))
		dest := make([]any, len(types))
		for i := range raw {
			dest[i] = &raw[i]
		}

		var out [][]*string
		for rows.Next() {
			if err := rows.Scan(dest...); err != nil {
				return nil, mapError(err)
			}
			cells := make([]*string, len(types))
			for i, r := range raw {
				if r == nil {
					continue
				}
				text := cellText(r, dbTypes[i])
				cells[i] = &text
			}
			out = append(out, cells)
		}
		if err := rows.Err(); err != nil {
			return nil, mapError(err)
		}
		return out, nil
	})
}

// streamArrayQuery is runArrayQuery's single-pass twin: onRow is called once per scanned
// row, in order, with a []*string the callback owns, instead of every row being materialized into
// a [][]*string only to be transposed into the page builder immediately after. runArrayQuery
// itself stays as the array-returning shape countRows and the catalog paths want.
func streamArrayQuery(ctx context.Context, conn Entry, query string, params []any, op *adapters.OpCtx, track TrackQuery, opts QueryOptions, onRow func(row []*string) error) error {
	setCommand(op, query, params, opts.LogParams)
	if err := adapters.CheckNotStarted(ctx); err != nil {
		return err
	}

	release := track(conn.running())
	done := conn.track()

	_, err := adapters.RunWithAbortRace(ctx, func() { release(); done() }, func(queryCtx context.Context) (struct{}, error) {
		rows, err := conn.QueryContext(queryCtx, query, params...)
		if err != nil {
			return struct{}{}, mapError(err)
		}
		defer rows.Close()

		types, err := rows.ColumnTypes()
		if err != nil {
			return struct{}{}, mapError(err)
		}
		dbTypes := make([]string, len(types))
		for i, t := range types {
			dbTypes[i] = t.DatabaseTypeName()
		}

		// raw/dest are pure scan scratch — see runArrayQuery's identical comment above.
		raw := make([]sql.RawBytes, len(types))
		dest := make([]any, len(types))
		for i := range raw {
			dest[i] = &raw[i]
		}

		for rows.Next() {
			if err := rows.Scan(dest...); err != nil {
				return struct{}{}, mapError(err)
			}
			cells := make([]*string, len(types))
			for i, r := range raw {
				if r == nil {
					continue
				}
				text := cellText(r, dbTypes[i])
				cells[i] = &text
			}
			if err := onRow(cells); err != nil {
				return struct{}{}, err
			}
		}
		if err := rows.Err(); err != nil {
			return struct{}{}, mapError(err)
		}
		return struct{}{}, nil
	})
	return err
}

// CommandOptions is query.ts's CommandOptions.
type CommandOptions struct {
	// SuppressCommand: setCommand() was already called once for the whole batch (P5 D9) — do not
	// call it again.
	SuppressCommand bool
}

// runCommand is query.ts's runCommand: an UPDATE/DELETE/INSERT/START TRANSACTION/COMMIT/ROLLBACK
// has no rows worth returning, only the number of rows it affected.
func runCommand(ctx context.Context, conn Entry, query string, params []any, op *adapters.OpCtx, track TrackQuery, opts CommandOptions) (int64, error) {
	if !opts.SuppressCommand {
		op.SetCommand(query)
	}
	if err := adapters.CheckNotStarted(ctx); err != nil {
		return 0, err
	}

	release := track(conn.running())
	done := conn.track()

	return adapters.RunWithAbortRace(ctx, func() { release(); done() }, func(queryCtx context.Context) (int64, error) {
		result, err := conn.ExecContext(queryCtx, query, params...)
		if err != nil {
			return 0, mapError(err)
		}
		n, err := result.RowsAffected()
		if err != nil {
			return 0, mapError(err)
		}
		return n, nil
	})
}
