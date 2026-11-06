package sqlite

import (
	"context"
	"database/sql"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// endTransactionTimeout bounds mutate's own cleanup ROLLBACK, mirroring postgres/console.go
// and mysqlfamily/console.go's identically-named const: it must run even when the op's own ctx is
// already cancelled (Stop pressed mid-batch, or the op's deadline expired), since database/sql's
// ExecContext refuses outright on an already-done ctx without ever reaching the driver — skipping
// this would leave the op's *sql.Conn (returned to runOnConn's pool, not closed) sitting inside a
// stale, still-open BEGIN IMMEDIATE for whatever op the pool hands that conn to next.
const endTransactionTimeout = 5 * time.Second

// literalRenderer adapts sqlmutate.go's LiteralRenderer to the adapters.ValueRenderer shape
// RenderRowOp expects — preview() never touches params, so this closure just ignores it.
func literalRenderer(name string, value *string, _ *[]any) (string, error) {
	return adapters.LiteralRenderer(name, value)
}

// preview is mutate.ts's preview — synchronous (D6): no catalog lookup, no network.
func preview(plan model.MutationPlan) ([]string, error) {
	database, table, err := adapters.ResolveDatabaseTablePath(plan.Path)
	if err != nil {
		return nil, err
	}
	relationSQL := quoteIdent(database) + "." + quoteIdent(table)
	return adapters.PreviewSQLMutation(plan, relationSQL, literalRenderer, quoteIdent)
}

// mutate is mutate.ts's own — D25: BEGIN IMMEDIATE, not a deferred BEGIN, so a contended file fails
// before a single row has changed rather than mid-batch. Issued on the op's own dedicated
// *sql.Conn (the one runOnConn already obtained), which is what makes the transaction real.
func mutate(ctx context.Context, conn *sql.Conn, op *adapters.OpCtx, readOnly bool, plan model.MutationPlan) (model.MutationResult, error) {
	resolve := func() (relationSQL, qualifiedName string, columns []model.ColumnMeta, primaryKey []string, err error) {
		database, table, err := adapters.ResolveDatabaseTablePath(plan.Path)
		if err != nil {
			return "", "", nil, nil, err
		}
		// Fresh in this same op — never trusts a column name the renderer sent without re-checking it
		// against the catalog right now (same discipline resolveProjection uses on the read path).
		exec := execFor(ctx, conn, op)
		target, err := getReadTarget(exec, database, table)
		if err != nil {
			return "", "", nil, nil, err
		}
		relationSQL = quoteIdent(database) + "." + quoteIdent(table)
		qualifiedName = target.QualifiedName.Database + "." + target.QualifiedName.Table
		return relationSQL, qualifiedName, target.Columns, target.PrimaryKey, nil
	}

	execCommand := func(sqlText string, params []any) (int64, error) {
		return runCommand(ctx, conn, sqlText, params, op, true)
	}
	// P2 R2: rollback runs on its own detached, timeout-bounded ctx (RunSQLMutation's own rule) so it
	// always runs regardless of the caller's own ctx state — the original ad-hoc rollback ran on the
	// same ctx as everything else, and database/sql's ExecContext refuses outright on an
	// already-cancelled one without ever reaching the driver, so a cancellation mid-loop or racing
	// COMMIT left neither COMMIT nor ROLLBACK executed and conn (returned to runOnConn's pool, not
	// closed) stuck inside that BEGIN IMMEDIATE for whatever op the pool hands it to next.
	rollback := func(ctx context.Context) {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), endTransactionTimeout)
		defer cancel()
		_, _ = runCommand(cleanupCtx, conn, "ROLLBACK", nil, op, true)
	}

	// D23: the table's own rowid, even when it exists and is used internally for keyset paging, is
	// never an acceptable key here — it is not a column the renderer ever shows. ValidateMutationOps
	// (inside RunRelationalMutate) enforces that against target.Columns/PrimaryKey as before.
	return adapters.RunRelationalMutate(ctx, op, readOnly, plan, adapters.RelationalMutateDeps{
		Resolve: resolve, Quote: quoteIdent, Placeholder: questionPlaceholder,
		TypeClassFor: typeClassFor, LiteralRenderer: literalRenderer,
		BeginSQL: "BEGIN IMMEDIATE", Exec: execCommand, Rollback: rollback,
	})
}
