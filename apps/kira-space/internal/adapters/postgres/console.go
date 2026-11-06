package postgres

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/page"
)

// endTransactionTimeout bounds the BEGIN READ ONLY wrap's own cleanup COMMIT (P2 R2): it must run
// even when the op's own ctx is already cancelled (Stop pressed mid-batch), since skipping it
// would leave this connection's next op running inside a stale, likely-aborted transaction.
const endTransactionTimeout = 5 * time.Second

// ClassifyStatement satisfies adapters.StatementClassifier (M2) over the shared SQL classifier.
func (a *Adapter) ClassifyStatement(_ context.Context, statement string) (adapters.OpClass, error) {
	return adapters.ClassifySQL(statement, adapters.PostgresDialect), nil
}

// rawField is console.ts's RawField.
type rawField struct {
	name        string
	dataTypeOID uint32
}

// rawResult is console.ts's RawResult.
type rawResult struct {
	rows    [][]*string
	fields  []rawField
	command string
	// truncated: the row cap stopped the read with at least one more row available.
	truncated bool
	// bytes is the cell text held, charged against the batch's shared byte budget.
	bytes int
}

// runRaw is console.ts's runRaw: the low-level runner, deliberately separate from
// runArrayQuery/runCommand — the console needs full field metadata (name + type OID) those
// discard, and it must not call op.SetCommand() per statement — execute() below calls it once for
// the whole batch. Always text-mode (mirrors read.go's identity type parsing)
// so every cell arrives as the server's own text representation, with no per-type Go conversion to
// undo.
func runRaw(ctx context.Context, conn *trackedConn, sql string, params []any, op *adapters.OpCtx, track TrackQuery, limit page.ResultCap) (rawResult, error) {
	if err := adapters.CheckNotStarted(ctx); err != nil {
		return rawResult{}, err
	}
	release := track(conn.running())
	done := conn.track()

	return adapters.RunWithAbortRace(ctx, func() { release(); done() }, func(queryCtx context.Context) (rawResult, error) {
		rows, err := conn.Query(queryCtx, sql, queryArgs(true, params)...)
		if err != nil {
			return rawResult{}, mapError(err)
		}
		defer rows.Close()

		descs := rows.FieldDescriptions()
		fields := make([]rawField, len(descs))
		for i, d := range descs {
			fields[i] = rawField{name: d.Name, dataTypeOID: d.DataTypeOID}
		}

		var out [][]*string
		held := 0
		truncated := false
		for rows.Next() {
			// Next just proved another row exists; stopping here makes truncated mean "more existed".
			if limit.Reached(len(out), held) {
				truncated = true
				break
			}
			dest := make([]any, len(descs))
			cells := make([]*string, len(descs))
			for i := range cells {
				dest[i] = &cells[i]
			}
			if err := rows.Scan(dest...); err != nil {
				return rawResult{}, mapError(err)
			}
			// held mirrors what the page builder will keep. A cell past MaxCellBytes keeps one extra
			// byte, enough for the builder to clip and flag it, so a huge cell is not held whole.
			for i, c := range cells {
				if c == nil {
					continue
				}
				if len(*c) > page.MaxCellBytes {
					clipped := strings.Clone((*c)[:page.MaxCellBytes+1])
					cells[i] = &clipped
					held += page.MaxCellBytes
				} else {
					held += len(*c)
				}
			}
			out = append(out, cells)
		}
		// Close before reading the tag or error: it drains what the cap left on the wire.
		rows.Close()
		if err := rows.Err(); err != nil {
			return rawResult{}, mapError(err)
		}

		// pgconn's own CommandTag string is the closest Go equivalent of Postgres's real
		// command-complete wire tag (e.g. "INSERT 0 3", "UPDATE 1") — used directly as the single
		// status cell's text for a statement with no output columns (buildPage, below), which is at
		// least as faithful as console.ts's own documented approximation (`${command} ${rowCount}`),
		// closer in fact since it is the server's own tag rather than a reconstruction of it.
		return rawResult{rows: out, fields: fields, command: rows.CommandTag().String(), truncated: truncated, bytes: held}, nil
	})
}

// pgTypeMeta is lookupTypeNames' own per-OID result: the type's name plus whether pg_type reports
// it as composite (typtype = 'c') — a table or view's own implicit row type, or an explicit CREATE
// TYPE AS composite, either way a single result column carrying every one of ITS OWN columns'
// worth of data behind one name (`SELECT c FROM customers c`'s own "c" column has DataType
// "customers", which typeClassFor's name-based guess has no way to tell apart from an ordinary
// unrecognised scalar type — pg_type is the one place that distinction is knowable).
type pgTypeMeta struct {
	name      string
	composite bool
}

// buildPage is console.ts's buildPage.
func buildPage(result rawResult, typeMeta map[uint32]pgTypeMeta) page.TabularPage {
	if len(result.fields) == 0 {
		command := result.command
		if command == "" {
			command = "OK"
		}
		return adapters.SingleStatusPage(command, "text")
	}

	columns := make([]page.ColumnDescriptor, len(result.fields))
	for i, f := range result.fields {
		meta := typeMeta[f.dataTypeOID]
		dataType := meta.name
		if dataType == "" {
			dataType = "unknown"
		}
		typeClass := typeClassFor(dataType)
		if meta.composite {
			// Overrides typeClassFor's own guess outright — dbmcp/render.go's masking gate refuses
			// a TypeClassOther tabular column on a masked connection (finding #1), and a composite
			// value is exactly the "carries a whole row" shape that gate exists to catch.
			typeClass = page.TypeClassOther
		}
		columns[i] = page.ColumnDescriptor{
			Name: f.name, DataType: dataType, TypeClass: typeClass,
			// execute() never consults the catalog (no target relation to describe), so
			// nullability and PK-ness are unknowable here — console results are always
			// read-only regardless.
			Nullable: true, IsPrimaryKey: false, Generated: false,
		}
	}

	builder := page.NewTabularPageBuilder(columns)
	for _, row := range result.rows {
		values := make([]*string, len(row))
		for i, v := range row {
			// normalizeCellText only ever rewrites a TypeClassBinary cell — every other type class
			// returns its input string unchanged, so reusing the already-scanned *string directly
			// avoids a needless extra allocation + pointer per cell (P2 R1).
			if v == nil || columns[i].TypeClass != page.TypeClassBinary {
				values[i] = v
				continue
			}
			normalized := normalizeCellText(*v, columns[i].TypeClass)
			values[i] = &normalized
		}
		_ = builder.AppendRow(values)
	}
	return builder.Finish(page.CappedPosition(len(result.rows), result.truncated))
}

func lookupTypeNames(ctx context.Context, conn *trackedConn, op *adapters.OpCtx, track TrackQuery, oids []uint32) (map[uint32]pgTypeMeta, error) {
	result, err := runRaw(ctx, conn, "SELECT oid, typname, typtype FROM pg_type WHERE oid = ANY($1::oid[])", []any{oids}, op, track, page.ResultCap{})
	if err != nil {
		return nil, err
	}
	out := make(map[uint32]pgTypeMeta, len(result.rows))
	for _, row := range result.rows {
		if len(row) < 3 || row[0] == nil || row[1] == nil || row[2] == nil {
			continue
		}
		oid, err := parseUint32(*row[0])
		if err != nil {
			continue
		}
		out[oid] = pgTypeMeta{name: *row[1], composite: *row[2] == "c"}
	}
	return out, nil
}

func parseUint32(s string) (uint32, error) {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, adapters.New(adapters.CodeQuery, "not a number: "+s, nil)
		}
		n = n*10 + int(c-'0')
	}
	return uint32(n), nil
}

// verifyReadOnlyWrap re-checks server state after every statement of a read-only batch: the
// statement-text screen (AssertNoTransactionEscalation) is a spelling blacklist, and Postgres has
// more spellings than it lists (parse_bool prefixes, quoted names, set_config(), ABORT then RESET).
// The wrap must still be an open transaction (TxStatus 'T') with both the transaction and the
// session default read-only. On a violation, roll back and re-assert the session default.
func verifyReadOnlyWrap(ctx context.Context, conn *trackedConn, op *adapters.OpCtx, track TrackQuery, connReadOnly bool) error {
	if conn.PgConn().TxStatus() == 'T' {
		res, err := runRaw(ctx, conn, "SELECT current_setting('transaction_read_only'), current_setting('default_transaction_read_only')", nil, op, track, page.ResultCap{})
		if err != nil {
			return err
		}
		if len(res.rows) == 1 && len(res.rows[0]) == 2 && res.rows[0][0] != nil && *res.rows[0][0] == "on" && (!connReadOnly || res.rows[0][1] != nil && *res.rows[0][1] == "on") {
			return nil
		}
	}
	conn.waitInFlight()
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), endTransactionTimeout)
	defer cancel()
	_, _ = conn.Exec(cleanupCtx, "ROLLBACK")
	if connReadOnly {
		if _, err := conn.Exec(cleanupCtx, "SET default_transaction_read_only = on"); err != nil {
			return mapError(err)
		}
	}
	return adapters.ReadOnlyViolation(connReadOnly)
}

// execute is console.ts's execute. readOnly closes the gap client.go's own
// SET default_transaction_read_only=on leaves open: that session default only governs
// transactions *not yet started*, so a statement in this very batch could otherwise flip the
// session's default (or, on Postgres specifically, the current transaction's own read-write mode
// via SET TRANSACTION READ WRITE) and have every following statement in the batch run writable.
// Wrapping the whole batch in an explicit BEGIN READ ONLY transaction closes every angle
// tried against a real server except that one specific statement, which
// AssertNoTransactionEscalation rejects outright before anything runs.
func execute(ctx context.Context, conn *trackedConn, op *adapters.OpCtx, track TrackQuery, readOnly, connReadOnly bool, statements []string, limit page.ResultCap, timeout time.Duration) (pages []page.Page, err error) {
	defer func() { err = timeoutError(ctx, timeout, err) }()
	if err := checkExecutable(conn, readOnly, connReadOnly, statements); err != nil {
		return nil, err
	}
	op.SetCommand(adapters.JoinConsoleStatements(statements, "--"))

	end, err := beginBatch(ctx, conn, op, track, readOnly, timeout)
	if err != nil {
		return nil, err
	}
	defer end()

	results := make([]rawResult, 0, len(statements))
	for _, sql := range statements {
		if err := adapters.CheckCancelled(ctx); err != nil {
			return nil, err
		}
		result, err := runRaw(ctx, conn, sql, nil, op, track, limit)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
		limit = adapters.RemainingCap(limit, result.bytes)
		if readOnly {
			if err := verifyReadOnlyWrap(ctx, conn, op, track, connReadOnly); err != nil {
				return nil, err
			}
		}
	}

	typeMeta, err := resultTypeMeta(ctx, conn, op, track, results)
	if err != nil {
		return nil, err
	}
	pages = make([]page.Page, len(results))
	for i, r := range results {
		pages[i] = buildPage(r, typeMeta)
	}
	return pages, nil
}

// checkExecutable runs the pre-flight guards before any statement touches conn.
func checkExecutable(conn *trackedConn, readOnly, connReadOnly bool, statements []string) error {
	if len(statements) == 0 {
		return adapters.T(adapters.CodeQuery, "backend.studio.adapter.noStatements.body", nil, nil)
	}
	if readOnly && connReadOnly {
		if err := adapters.AssertNoTransactionEscalation(statements, adapters.PostgresDialect); err != nil {
			return err
		}
	}
	// A per-request wrap on a writable connection would run inside, then roll back, a user transaction.
	if readOnly && !connReadOnly && conn.PgConn().TxStatus() != 'I' {
		return adapters.OpenTransaction()
	}
	return nil
}

// beginBatch opens the read-only wrap and/or applies the statement timeout. The returned func ends both.
func beginBatch(ctx context.Context, conn *trackedConn, op *adapters.OpCtx, track TrackQuery, readOnly bool, timeout time.Duration) (func(), error) {
	if !readOnly {
		if timeout <= 0 {
			return func() {}, nil
		}
		return setSessionTimeout(ctx, conn, op, track, timeout)
	}
	// P113 G1: adapters.BeginReadOnlyConsole carries the shared BEGIN/COMMIT wrap postgres and
	// mysqlfamily hand-rolled identically. Confirmed empirically that Postgres ends an
	// already-aborted transaction's COMMIT with a ROLLBACK command tag, not an error — safe to
	// call unconditionally. F2: waitInFlight blocks on every RunWithAbortRace goroutine this
	// batch spawned still touching conn (the same non-concurrency-safe *pgx.Conn) before COMMIT.
	cleanup, err := adapters.BeginReadOnlyConsole(ctx, "BEGIN READ ONLY", func(ctx context.Context, sql string) error {
		_, err := conn.Exec(ctx, sql)
		return err
	}, conn.waitInFlight, func(err error) error { return mapError(err) })
	if err != nil {
		return nil, err
	}
	if timeout > 0 {
		// SET LOCAL reverts at the wrap's COMMIT, which runs on every exit: nothing to restore.
		if _, err := conn.Exec(ctx, "SET LOCAL statement_timeout = "+strconv.FormatInt(timeout.Milliseconds(), 10)); err != nil {
			cleanup()
			return nil, mapError(err)
		}
	}
	return cleanup, nil
}

// resultTypeMeta resolves type names for every column OID across results.
func resultTypeMeta(ctx context.Context, conn *trackedConn, op *adapters.OpCtx, track TrackQuery, results []rawResult) (map[uint32]pgTypeMeta, error) {
	oidSet := map[uint32]struct{}{}
	for _, r := range results {
		for _, f := range r.fields {
			oidSet[f.dataTypeOID] = struct{}{}
		}
	}
	if len(oidSet) == 0 {
		return map[uint32]pgTypeMeta{}, nil
	}
	oids := make([]uint32, 0, len(oidSet))
	for o := range oidSet {
		oids = append(oids, o)
	}
	return lookupTypeNames(ctx, conn, op, track, oids)
}

// setSessionTimeout applies timeout for the rest of this batch on the pinned connection and returns
// the restore. The entry lock is held until release, so no other op sees the changed setting.
func setSessionTimeout(ctx context.Context, conn *trackedConn, op *adapters.OpCtx, track TrackQuery, timeout time.Duration) (func(), error) {
	cur, err := runRaw(ctx, conn, "SELECT current_setting('statement_timeout')", nil, op, track, page.ResultCap{})
	if err != nil {
		return nil, err
	}
	if len(cur.rows) != 1 || len(cur.rows[0]) != 1 || cur.rows[0][0] == nil {
		return nil, adapters.Contract("statement_timeout read returned no value", nil)
	}
	saved := *cur.rows[0][0]
	const setSQL = "SELECT set_config('statement_timeout', $1, false)"
	if _, err := runRaw(ctx, conn, setSQL, []any{fmt.Sprintf("%dms", timeout.Milliseconds())}, op, track, page.ResultCap{}); err != nil {
		return nil, err
	}
	return func() {
		conn.waitInFlight()
		if conn.IsClosed() {
			return
		}
		// A failed statement in a user transaction leaves it aborted: closing would destroy the
		// transaction, and the set_config reverts at the rollback the user must issue anyway.
		if conn.PgConn().TxStatus() == 'E' {
			return
		}
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), endTransactionTimeout)
		defer cancel()
		if _, err := conn.Exec(cleanupCtx, setSQL, pgx.QueryExecModeSimpleProtocol, saved); err != nil {
			// Never leave a live connection carrying the agent's limit: the pool drops a closed one.
			_ = conn.Close(cleanupCtx)
		}
	}, nil
}

// timeoutError maps the engine's statement_timeout cancel to the typed timeout. Stop and a timeout
// share SQLSTATE 57014 and the message is localised, so the op ctx tells them apart: Stop cancels
// it first, a statement timeout leaves it live.
func timeoutError(ctx context.Context, timeout time.Duration, err error) error {
	if err == nil || timeout <= 0 || ctx.Err() != nil {
		return err
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "57014" {
		return adapters.StatementTimeout(timeout, 0, err)
	}
	return err
}
