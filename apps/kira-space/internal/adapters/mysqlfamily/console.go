package mysqlfamily

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/go-sql-driver/mysql"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/page"
)

// endTransactionTimeout bounds the START TRANSACTION READ ONLY wrap's own cleanup COMMIT (P2 R2):
// it must run even when the op's own ctx is already cancelled (Stop pressed mid-batch), since
// skipping it would leave this connection's next op running inside a stale open transaction.
const endTransactionTimeout = 5 * time.Second

// ClassifyStatement satisfies adapters.StatementClassifier (M2) over the shared SQL classifier —
// serves both mysql and mariadb, since mysqlfamily.Adapter is registered under both kinds.
func (a *Adapter) ClassifyStatement(_ context.Context, statement string) (adapters.OpClass, error) {
	return adapters.ClassifySQL(statement, adapters.MySQLDialect), nil
}

// numberDBTypes/temporalDBTypes are console.ts's own NUMBER_TYPES/TEMPORAL_TYPES, spelled in
// go-sql-driver's own DatabaseTypeName() vocabulary (fields.go's typeDatabaseName) rather than the
// mariadb npm package's FieldInfo.type enum — the two name the same wire types differently, but
// the classification is the same.
var numberDBTypes = map[string]bool{
	"DECIMAL": true, "TINYINT": true, "UNSIGNED TINYINT": true, "SMALLINT": true, "UNSIGNED SMALLINT": true,
	"MEDIUMINT": true, "UNSIGNED MEDIUMINT": true, "INT": true, "UNSIGNED INT": true,
	"BIGINT": true, "UNSIGNED BIGINT": true, "FLOAT": true, "DOUBLE": true, "YEAR": true,
}
var temporalDBTypes = map[string]bool{"TIMESTAMP": true, "DATE": true, "TIME": true, "DATETIME": true}

// typeClassForField is console.ts's typeClassForField, minus its boolean case (go-sql-driver's
// ColumnTypeLength is commented out upstream, so a TINYINT console column has no display-width
// signal to distinguish tinyint(1) from any other TINYINT — it classifies as 'number', a documented
// capability loss recorded in docs/ARCHITECTURE.md, not an oversight).
func typeClassForField(dbType string) page.TypeClass {
	if binaryDatabaseTypes[dbType] {
		return page.TypeClassBinary
	}
	if numberDBTypes[dbType] {
		return page.TypeClassNumber
	}
	if temporalDBTypes[dbType] {
		return page.TypeClassTemporal
	}
	if dbType == "JSON" {
		return page.TypeClassJSON
	}
	return page.TypeClassText
}

// runRaw is console.ts's own low-level runner, deliberately separate from query.go's
// runArrayQuery/runCommand — it must not call op.SetCommand() per statement, and it needs full
// field metadata (name + DatabaseTypeName) those discard. QueryContext is used unconditionally,
// never ExecContext: a non-row-returning statement (UPDATE/INSERT/DDL) still comes
// back through QueryContext with zero columns, the same signal SQLite's own StatementSync gives —
// so the console needs no per-statement leading-keyword decision the way ClickHouse's does.
// Rows stream into the page builder and stop at limit; Close then drains the rest off the wire
// without holding it. A zero-column statement renders a generic "OK" status, not "<N> row(s)
// affected": go-sql-driver's Rows type exposes no affected-row count over QueryContext (confirmed
// against its own source — mysqlRows implements no driver.Result), a real, documented capability
// loss (docs/ARCHITECTURE.md's per-engine section), not an oversight.
func runRaw(ctx context.Context, conn Entry, query string, op *adapters.OpCtx, track TrackQuery, limit page.ResultCap) (page.TabularPage, error) {
	if err := adapters.CheckNotStarted(ctx); err != nil {
		return page.TabularPage{}, err
	}
	release := track(conn.running())
	done := conn.track()

	return adapters.RunWithAbortRace(ctx, func() { release(); done() }, func(queryCtx context.Context) (page.TabularPage, error) {
		sqlRows, err := conn.QueryContext(queryCtx, query)
		if err != nil {
			return page.TabularPage{}, mapError(err)
		}
		defer sqlRows.Close()

		types, err := sqlRows.ColumnTypes()
		if err != nil {
			return page.TabularPage{}, mapError(err)
		}
		if len(types) == 0 {
			return adapters.SingleStatusPage("OK", "text"), nil
		}
		dbTypes, columns := consoleColumns(types)
		builder := page.NewTabularPageBuilder(columns)
		truncated, err := appendConsoleRows(sqlRows, builder, dbTypes, limit)
		if err != nil {
			return page.TabularPage{}, err
		}
		// Close before reading the error: it drains what the cap left on the wire.
		sqlRows.Close()
		if err := sqlRows.Err(); err != nil {
			return page.TabularPage{}, mapError(err)
		}
		return builder.Finish(page.CappedPosition(builder.RowCount(), truncated)), nil
	})
}

// consoleColumns maps a result's column types to page descriptors, plus each column's DB type name.
func consoleColumns(types []*sql.ColumnType) ([]string, []page.ColumnDescriptor) {
	dbTypes := make([]string, len(types))
	columns := make([]page.ColumnDescriptor, len(types))
	for i, t := range types {
		dbTypes[i] = t.DatabaseTypeName()
		columns[i] = page.ColumnDescriptor{
			Name: t.Name(), DataType: dbTypes[i], TypeClass: typeClassForField(dbTypes[i]),
			Nullable: true, IsPrimaryKey: false, Generated: false,
		}
	}
	return dbTypes, columns
}

// appendConsoleRows streams rows into builder and stops at limit, reporting whether more existed.
func appendConsoleRows(sqlRows *sql.Rows, builder *page.TabularPageBuilder, dbTypes []string, limit page.ResultCap) (bool, error) {
	for sqlRows.Next() {
		// Next just proved another row exists; stopping here makes truncated mean "more existed".
		if limit.Reached(builder.RowCount(), builder.Bytes()) {
			return true, nil
		}
		raw := make([]sql.RawBytes, len(dbTypes))
		dest := make([]any, len(dbTypes))
		for i := range raw {
			dest[i] = &raw[i]
		}
		if err := sqlRows.Scan(dest...); err != nil {
			return false, mapError(err)
		}
		cells := make([]*string, len(dbTypes))
		for i, rb := range raw {
			if rb == nil {
				continue
			}
			text := cellText(rb, dbTypes[i])
			cells[i] = &text
		}
		if err := builder.AppendRow(cells); err != nil {
			return false, err
		}
	}
	return false, nil
}

// errTxCharacteristics is MySQL/MariaDB's "Transaction characteristics can't be changed while a
// transaction is in progress": a bare SET TRANSACTION fails with it only inside an open
// transaction, so it doubles as an in-transaction probe.
const errTxCharacteristics = 1568

// verifyReadOnlyWrap re-checks server state after every statement of a read-only batch: the wrap
// must still be an open transaction and the session default still read-only. Statement-text
// screening alone cannot be complete (implicit commits, comment-lexer drift, SET SESSION flips that
// take effect only once the wrap ends). On a violation, roll back and re-assert the session
// default. A probe that succeeds outside a transaction only arms the next transaction read-only.
func verifyReadOnlyWrap(ctx context.Context, conn Entry, op *adapters.OpCtx, track TrackQuery, connReadOnly bool) error {
	_, err := runRaw(ctx, conn, "SET TRANSACTION READ ONLY", op, track, page.ResultCap{})
	var myErr *mysql.MySQLError
	inTx := errors.As(err, &myErr) && myErr.Number == errTxCharacteristics
	if err != nil && !inTx {
		return err
	}
	// MySQL exposes no per-transaction flag to read back; on a writable connection (per-request
	// wrap) an open transaction is the invariant, since an implicit commit makes the probe succeed.
	if inTx && !connReadOnly {
		return nil
	}
	if inTx {
		vars, err := runRaw(ctx, conn, "SHOW SESSION VARIABLES WHERE Variable_name IN ('transaction_read_only', 'tx_read_only')", op, track, page.ResultCap{})
		if err != nil {
			return err
		}
		if sessionReadOnly(vars) {
			return nil
		}
	}
	conn.waitInFlight()
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), endTransactionTimeout)
	defer cancel()
	_, _ = conn.ExecContext(cleanupCtx, "ROLLBACK")
	// A writable connection must not keep the next-transaction read-only flag the probe armed.
	reassert := "SET TRANSACTION READ WRITE"
	if connReadOnly {
		reassert = "SET SESSION TRANSACTION READ ONLY"
	}
	if _, err := conn.ExecContext(cleanupCtx, reassert); err != nil {
		return mapError(err)
	}
	return adapters.ReadOnlyViolation(connReadOnly)
}

// refuseOpenTransaction probes for an open transaction on a writable connection. The probe arms the
// next transaction read-only when it succeeds; the wrap's START TRANSACTION READ ONLY consumes that.
func refuseOpenTransaction(ctx context.Context, conn Entry, op *adapters.OpCtx, track TrackQuery) error {
	_, err := runRaw(ctx, conn, "SET TRANSACTION READ ONLY", op, track, page.ResultCap{})
	if err == nil {
		return nil
	}
	var myErr *mysql.MySQLError
	if errors.As(err, &myErr) && myErr.Number == errTxCharacteristics {
		return adapters.OpenTransaction()
	}
	return err
}

// sessionReadOnly reports whether every variable row (MariaDB lists both spellings) is ON, and at
// least one exists.
func sessionReadOnly(p page.TabularPage) bool {
	if len(p.Chunks) != 2 {
		return false
	}
	for r := 0; r < p.RowCount; r++ {
		if page.IsNull(p.Chunks[1], r) {
			return false
		}
		if v := page.CellText(p.Chunks[1], r); v != "ON" && v != "1" {
			return false
		}
	}
	return p.RowCount > 0
}

// execute is console.ts's execute. readOnly closes the gap client.go's own
// SET SESSION TRANSACTION READ ONLY leaves open: that session default only governs
// transactions not yet started, so a statement in this batch could flip it back for whatever
// runs after. Wrapping the batch in an explicit START TRANSACTION READ ONLY is a hard server-side
// backstop here — confirmed against a real server that, unlike Postgres, MariaDB/MySQL refuse
// outright ("Transaction characteristics can't be changed while a transaction is in progress") to
// let any statement flip an already-open transaction's own read-only mode. A statement can still
// end the wrap (COMMIT through a comment-lexer gap, an implicit commit) or flip the session default
// for what follows, so verifyReadOnlyWrap re-checks after every statement;
// AssertNoTransactionEscalation is the cheap first line of defense.
func execute(ctx context.Context, conn Entry, op *adapters.OpCtx, track TrackQuery, readOnly, connReadOnly bool, statements []string, limit page.ResultCap, timeout time.Duration, mariadb bool) (pages []page.Page, err error) {
	defer func() { err = timeoutError(ctx, timeout, err) }()
	if len(statements) == 0 {
		return nil, adapters.T(adapters.CodeQuery, "backend.studio.adapter.noStatements.body", nil, nil)
	}
	if readOnly && connReadOnly {
		if err := adapters.AssertNoTransactionEscalation(statements, adapters.MySQLDialect); err != nil {
			return nil, err
		}
	}
	// START TRANSACTION implicitly commits an open user transaction; refuse before it runs.
	if readOnly && !connReadOnly {
		if err := refuseOpenTransaction(ctx, conn, op, track); err != nil {
			return nil, err
		}
	}
	op.SetCommand(adapters.JoinConsoleStatements(statements, "--", "#"))

	// MySQL's max_execution_time covers SELECT only, so only the read lane gets it; the host
	// backstop stops everything else. Registered before the wrap so the restore runs after COMMIT.
	if timeout > 0 && (mariadb || readOnly) {
		restore, err := setSessionTimeout(ctx, conn, timeout, mariadb)
		if err != nil {
			return nil, err
		}
		defer restore()
	}

	if readOnly {
		// P113 G1: adapters.BeginReadOnlyConsole carries the shared BEGIN/COMMIT wrap postgres and
		// mysqlfamily hand-rolled identically. F2: waitInFlight blocks on every RunWithAbortRace
		// goroutine this batch spawned still touching conn before COMMIT.
		cleanup, err := adapters.BeginReadOnlyConsole(ctx, "START TRANSACTION READ ONLY", func(ctx context.Context, sql string) error {
			_, err := conn.ExecContext(ctx, sql)
			return err
		}, conn.waitInFlight, func(err error) error { return mapError(err) })
		if err != nil {
			if !connReadOnly {
				cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), endTransactionTimeout)
				defer cancel()
				_, _ = conn.ExecContext(cleanupCtx, "SET TRANSACTION READ WRITE")
			}
			return nil, err
		}
		defer cleanup()
	}

	pages = make([]page.Page, len(statements))
	for i, stmt := range statements {
		if err := adapters.CheckCancelled(ctx); err != nil {
			return nil, err
		}
		p, err := runRaw(ctx, conn, stmt, op, track, limit)
		if err != nil {
			return nil, err
		}
		pages[i] = p
		limit = adapters.RemainingCap(limit, p.Size())
		if readOnly {
			if err := verifyReadOnlyWrap(ctx, conn, op, track, connReadOnly); err != nil {
				return nil, err
			}
		}
	}
	return pages, nil
}

// Server error numbers for a statement stopped by its time limit.
const (
	errQueryTimeout     = 3024 // MySQL ER_QUERY_TIMEOUT, max_execution_time
	errStatementTimeout = 1969 // MariaDB ER_STATEMENT_TIMEOUT, max_statement_time
)

// timeoutError maps the engine's own timeout to the typed one. Stop cancels ctx first; a timeout
// leaves it live.
func timeoutError(ctx context.Context, timeout time.Duration, err error) error {
	if err == nil || timeout <= 0 || ctx.Err() != nil {
		return err
	}
	var myErr *mysql.MySQLError
	if errors.As(err, &myErr) && (myErr.Number == errQueryTimeout || myErr.Number == errStatementTimeout) {
		return adapters.StatementTimeout(timeout, 0, err)
	}
	return err
}

// setSessionTimeout sets the session timeout variable for this batch and returns the restore. The
// entry lock is held until release, so no other op sees the changed variable. Session variables are
// not transactional: the restore applies inside a user transaction too.
func setSessionTimeout(ctx context.Context, conn Entry, timeout time.Duration, mariadb bool) (func(), error) {
	variable, value := "max_execution_time", strconv.FormatInt(timeout.Milliseconds(), 10)
	if mariadb {
		variable, value = "max_statement_time", strconv.FormatFloat(timeout.Seconds(), 'f', -1, 64)
	}
	var raw string
	if err := conn.QueryRowContext(ctx, "SELECT @@SESSION."+variable).Scan(&raw); err != nil {
		return nil, mapError(err)
	}
	// Re-rendered from a parsed number so the restore statement never carries server-supplied text.
	n, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return nil, adapters.Contract(fmt.Sprintf("%s read returned %q", variable, raw), err)
	}
	saved := strconv.FormatFloat(n, 'f', -1, 64)
	if _, err := conn.ExecContext(ctx, "SET SESSION "+variable+" = "+value); err != nil {
		return nil, mapError(err)
	}
	return func() {
		conn.waitInFlight()
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), endTransactionTimeout)
		defer cancel()
		if _, err := conn.ExecContext(cleanupCtx, "SET SESSION "+variable+" = "+saved); err != nil {
			// Never leave a live connection carrying the agent's limit: ConnSet.Acquire drops a dead one.
			_ = conn.Raw(func(any) error { return driver.ErrBadConn })
		}
	}, nil
}
