package mysqlfamily

import (
	"context"
	"strings"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
)

// literalRenderer is a MySQL/MariaDB-specific override of sql-mutate.go's shared LiteralRenderer
// (P2 R1): unlike Postgres under standard_conforming_strings=on (the default) and SQLite, both of
// which treat backslash as a plain literal character, MySQL and MariaDB treat it as a
// string-literal escape character by default, so a value containing one needs its own backslash
// doubled before the surrounding quotes are escaped, or the preview text would mis-render the
// statement that dialect would actually parse. This is a preview()-only (display) fix — mutate()'s
// executed path uses paramRenderer/NewParamRenderer with real placeholders and is unaffected.
func literalRenderer(name string, value *string, _ *[]any) (string, error) {
	if value == nil {
		return adapters.LiteralRenderer(name, value)
	}
	escaped := strings.ReplaceAll(*value, `\`, `\\`)
	return adapters.LiteralRenderer(name, &escaped)
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

// mutate is mutate.ts's mutate.
func mutate(ctx context.Context, conn Entry, op *adapters.OpCtx, track TrackQuery, readOnly bool, plan model.MutationPlan) (model.MutationResult, error) {
	resolve := func() (relationSQL, qualifiedName string, columns []model.ColumnMeta, primaryKey []string, err error) {
		database, table, err := adapters.ResolveDatabaseTablePath(plan.Path)
		if err != nil {
			return "", "", nil, nil, err
		}
		exec := execFor(conn, op, track)
		target, err := getReadTarget(ctx, exec, database, table)
		if err != nil {
			return "", "", nil, nil, err
		}
		relationSQL = quoteIdent(database) + "." + quoteIdent(table)
		qualifiedName = target.QualifiedName.Database + "." + target.QualifiedName.Table
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
		// F2: wait for every RunWithAbortRace goroutine this mutate's own statements spawned to
		// actually finish touching conn before issuing ROLLBACK on it.
		conn.waitInFlight()
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), endTransactionTimeout)
		defer cancel()
		_, _ = conn.ExecContext(cleanupCtx, "ROLLBACK")
	}

	return adapters.RunRelationalMutate(ctx, op, readOnly, plan, adapters.RelationalMutateDeps{
		Resolve: resolve, Quote: quoteIdent, Placeholder: questionPlaceholder,
		TypeClassFor: typeClassFor, LiteralRenderer: literalRenderer,
		BeginSQL: "START TRANSACTION", Exec: execCommand, Rollback: rollback,
	})
}
