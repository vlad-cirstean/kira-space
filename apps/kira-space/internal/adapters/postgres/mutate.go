package postgres

import (
	"context"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
)

// literalRenderer adapts sql-mutate.go's LiteralRenderer to the adapters.ValueRenderer shape
// RenderRowOp expects — preview() never touches params, but the signature still takes one, so
// this closure just ignores it.
func literalRenderer(name string, value *string, _ *[]any) (string, error) {
	return adapters.LiteralRenderer(name, value)
}

// resolveTablePath is mutate.ts's own resolveTablePath — postgres's three-segment
// database/schema/table form, distinct from sql-mutate.go's ResolveDatabaseTablePath (the
// two-segment form clickhouse/mysql-family/sqlite share). The database segment stays
// kind-unconstrained, same as before: Mutate's own root check already ran by the time this
// is called.
func resolveTablePath(path model.NodePath) (schema, table string, err error) {
	segs, err := adapters.RequirePath(path, "mutate", adapters.AnySeg("database"), adapters.Seg("schema"), adapters.Seg("table"))
	if err != nil {
		return "", "", err
	}
	return segs[1].Name, segs[2].Name, nil
}

// preview is mutate.ts's preview — synchronous (D6): no catalog lookup, no network, trusts the
// plan's column names as given.
func preview(plan model.MutationPlan) ([]string, error) {
	schema, table, err := resolveTablePath(plan.Path)
	if err != nil {
		return nil, err
	}
	relationSQL := quoteIdent(schema) + "." + quoteIdent(table)
	return adapters.PreviewSQLMutation(plan, relationSQL, literalRenderer, quoteIdent)
}

// mutate is mutate.ts's mutate.
func mutate(ctx context.Context, conn *trackedConn, op *adapters.OpCtx, track TrackQuery, readOnly bool, plan model.MutationPlan) (model.MutationResult, error) {
	resolve := func() (relationSQL, qualifiedName string, columns []model.ColumnMeta, primaryKey []string, err error) {
		schema, table, err := resolveTablePath(plan.Path)
		if err != nil {
			return "", "", nil, nil, err
		}
		// Fresh in this same op (D7, mirrors resolveProjection's P2 D10 discipline) — never trusts a
		// column name the renderer sent without re-checking it against the catalog right now.
		exec := execFor(conn, op, track)
		target, err := getReadTarget(ctx, exec, schema, table)
		if err != nil {
			return "", "", nil, nil, err
		}
		relationSQL = quoteIdent(schema) + "." + quoteIdent(table)
		qualifiedName = target.QualifiedName.Schema + "." + target.QualifiedName.Relation
		return relationSQL, qualifiedName, target.Columns, target.PrimaryKey, nil
	}

	execCommand := func(sql string, params []any) (int64, error) {
		return runCommand(ctx, conn, sql, params, op, track, CommandOptions{SuppressCommand: true})
	}

	// Rollback runs on its own detached, timeout-bounded ctx (RunSQLMutation's own rule) so it
	// always reaches the server even when the caller's own ctx is already cancelled — the ad-hoc
	// `_, _ = execCommand("ROLLBACK", nil)` this replaced ran through runCommand's CheckNotStarted,
	// which refuses outright on an already-cancelled ctx, leaving conn (pinned for this adapter's
	// lifetime) stuck inside a stale, still-open transaction for whatever op runs on it next.
	rollback := func(ctx context.Context) {
		// Wait for every RunWithAbortRace goroutine this mutate's own statements spawned to
		// finish touching conn before issuing ROLLBACK on it — otherwise an aborted
		// statement's own background goroutine (still running conn.Exec) races this cleanup on the
		// same non-concurrency-safe *pgx.Conn.
		conn.waitInFlight()
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), endTransactionTimeout)
		defer cancel()
		_, _ = conn.Exec(cleanupCtx, "ROLLBACK")
	}

	// §8.12's standard: enforced by RunRelationalMutate's own AssertWritable, not only greyed out in
	// the UI (P5 D11).
	return adapters.RunRelationalMutate(ctx, op, readOnly, plan, adapters.RelationalMutateDeps{
		Resolve: resolve, Quote: quoteIdent, Placeholder: dollarPlaceholder,
		TypeClassFor: typeClassFor, LiteralRenderer: literalRenderer,
		BeginSQL: "BEGIN", Exec: execCommand, Rollback: rollback,
	})
}
