package repos

import (
	"database/sql"
	"fmt"

	"github.com/kirathecat/kira-studio/internal/ids"
	"github.com/kirathecat/kira-studio/internal/kiraschema"
	"github.com/kirathecat/kira-studio/internal/kiratime"
	"github.com/kirathecat/kira-studio/internal/sqlb"
)

// historyTable is the read/adopt/orphan-sweep/single-delete shape grpc_history.go and
// response_history.go each rebuilt per table: identical SQL, differing only in table
// name, the timestamp column ORDER BY/tiebreak against (called_at vs. sent_at), the summary
// column list, and the entry type T decodes into. Record/Get/Clear stay outside this generic:
// each table's own Record applies a genuinely different cap policy and Get decodes a
// differently-shaped snapshot_json — folding either in would hide real per-table policy behind a
// shared shape that does not fit.
type historyTable[T any] struct {
	// table is the SQL table name; scope is the shorter name every error string here is already
	// prefixed with ("grpc_history" / "response_history" — kept distinct from table so error text
	// stays exactly what it was before this hoist).
	scope string
	cols  historyColumns
	// columns is the summary SELECT list (every column but snapshot_json).
	columns []string
	scan    func(rowScanner) (T, error)
}

// historyColumns names the columns of one history table that the shared SQL touches.
type historyColumns struct {
	Table, ID, ItemID, TabID, ScopeKey, StoredBytes, SnapshotJSON string
	Time                                                          string // called_at / sent_at
}

// List is the summary projection, newest first — ≤ the caller's own per-scope cap by construction
// (each table's own Record trim).
func (h historyTable[T]) List(db *sql.DB, d sqlb.Dialect, scopeKey string) ([]T, error) {
	sb := d.Select(h.columns...).From(h.cols.Table)
	sb.Where(sb.Equal(h.cols.ScopeKey, scopeKey)).OrderBy(h.cols.Time+" DESC", sqlb.RowID+" DESC")
	rows, err := sqlb.Query(db, sb)
	if err != nil {
		return nil, fmt.Errorf("repos/%s: query list: %w", h.scope, err)
	}
	defer rows.Close()

	out := []T{}
	for rows.Next() {
		e, err := h.scan(rows)
		if err != nil {
			return nil, fmt.Errorf("repos/%s: scan list: %w", h.scope, err)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repos/%s: rows: %w", h.scope, err)
	}
	return out, nil
}

// Delete removes one entry.
func (h historyTable[T]) Delete(db *sql.DB, d sqlb.Dialect, id string) error {
	del := d.DeleteFrom(h.cols.Table)
	del.Where(del.Equal(h.cols.ID, id))
	if _, err := sqlb.Exec(db, del); err != nil {
		return fmt.Errorf("repos/%s: delete: %w", h.scope, err)
	}
	return nil
}

// Adopt moves a scratch tab's history onto a newly-saved request — one UPDATE, scope follows via
// the generated column.
func (h historyTable[T]) Adopt(db *sql.DB, d sqlb.Dialect, tabID, itemID string) (int, error) {
	ub := d.Update(h.cols.Table)
	ub.Set(ub.Assign(h.cols.ItemID, itemID)).Where(ub.IsNull(h.cols.ItemID), ub.Equal(h.cols.TabID, tabID))
	res, err := sqlb.Exec(db, ub)
	if err != nil {
		return 0, fmt.Errorf("repos/%s: adopt: %w", h.scope, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("repos/%s: adopt rows affected: %w", h.scope, err)
	}
	return int(n), nil
}

// SweepOrphans deletes a scratch tab's history once its tab is gone — run once at launch.
func (h historyTable[T]) SweepOrphans(db *sql.DB, d sqlb.Dialect) error {
	live := d.Select(kiraschema.Tabs.ID).From(kiraschema.Tabs.Table)
	del := d.DeleteFrom(h.cols.Table)
	del.Where(del.IsNull(h.cols.ItemID), del.NotIn(h.cols.TabID, live))
	if _, err := sqlb.Exec(db, del); err != nil {
		return fmt.Errorf("repos/%s: sweep orphans: %w", h.scope, err)
	}
	return nil
}

// historyRecordInput is Record's own common envelope — the three fields its shared chain
// (environment resolution, id minting, scope-key derivation) needs, out of each table's own
// larger, genuinely different rec type (grpc_history.go's own model.GrpcCallHistoryRecord vs
// response_history.go's own model.ResponseHistoryRecord).
type historyRecordInput struct {
	TabID, ItemID, EnvironmentID string
}

// Record is the transaction/environment-resolve/elide-once/insert/cap/sweep/commit chain both
// grpc_history.go's and response_history.go's own Record share — everything around
// the two tables' genuinely different snapshot shape and INSERT column list, which is exactly
// what build/elide/insert supply:
//
//   - build returns the caller's own snapshot, already marshaled — neither table's snapshot
//     struct carries the resolved environment name (only each INSERT's own `environment` column
//     does), so unlike the doc's own stated shape, build takes no argument.
//   - elide is called once, only if build's own result would push the row past half byteBudget —
//     it closes over the same snapshot build already constructed (a package-level generic method
//     cannot add its own type parameter beyond the receiver's T, so there is no shared Go type to
//     hand elide instead), drops whichever fields make the row oversized, and remarshals.
//   - insert receives everything the per-table INSERT needs beyond its own rec (id, itemID, the
//     resolved timestamp/environment, storedBytes, the final snapshotJSON) and runs its own
//     column-specific statement.
//
// perScopeCap/byteBudget are each table's own constant/var; the cap DELETE and
// the global byte sweep below are otherwise identical SQL, parameterized by h.table/h.timeColumn
// (already on this struct) the same way List/Delete/Adopt/SweepOrphans above already are.
func (h historyTable[T]) Record(
	db *sql.DB, d sqlb.Dialect, in historyRecordInput, perScopeCap, byteBudget int,
	build func() ([]byte, error),
	elide func() ([]byte, error),
	insert func(tx *sql.Tx, id string, itemID *string, timestamp int64, environment string, storedBytes int, snapshotJSON []byte) error,
) (string, error) {
	tx, err := db.Begin()
	if err != nil {
		return "", fmt.Errorf("repos/%s: begin: %w", h.scope, err)
	}
	defer tx.Rollback() //nolint:errcheck

	environment := ""
	if in.EnvironmentID != "" {
		env := kiraschema.APIEnvironments
		esb := d.Select(env.Name).From(env.Table)
		esb.Where(esb.Equal(env.ID, in.EnvironmentID))
		if err := sqlb.QueryRow(tx, esb).Scan(&environment); err != nil && err != sql.ErrNoRows {
			return "", fmt.Errorf("repos/%s: resolve environment: %w", h.scope, err)
		}
	}

	snapshotJSON, err := build()
	if err != nil {
		return "", fmt.Errorf("repos/%s: encode snapshot: %w", h.scope, err)
	}
	if len(snapshotJSON) > byteBudget/2 {
		snapshotJSON, err = elide()
		if err != nil {
			return "", fmt.Errorf("repos/%s: encode snapshot: %w", h.scope, err)
		}
	}

	id := ids.NewID()
	var itemID *string
	if in.ItemID != "" {
		itemID = &in.ItemID
	}
	timestamp := kiratime.NowMs()
	storedBytes := len(snapshotJSON)

	if err := insert(tx, id, itemID, timestamp, environment, storedBytes, snapshotJSON); err != nil {
		return "", fmt.Errorf("repos/%s: insert: %w", h.scope, err)
	}

	// Per-scope count cap.
	scopeKey := "tab:" + in.TabID
	if itemID != nil {
		scopeKey = *itemID
	}
	c := h.cols
	keep := d.Select(c.ID).From(c.Table)
	keep.Where(keep.Equal(c.ScopeKey, scopeKey)).OrderBy(c.Time+" DESC", sqlb.RowID+" DESC").Limit(perScopeCap)
	capDel := d.DeleteFrom(c.Table)
	capDel.Where(capDel.Equal(c.ScopeKey, scopeKey), capDel.NotIn(c.ID, keep))
	if _, err := sqlb.Exec(tx, capDel); err != nil {
		return "", fmt.Errorf("repos/%s: cap scope: %w", h.scope, err)
	}

	// Global byte budget, oldest-first across every scope — the per-entry caps each build/elide
	// closure enforces are what make this safe: no single row can exceed the budget by itself.
	var totalBytes int64
	sum := d.Select("COALESCE(SUM(" + c.StoredBytes + "), 0)").From(c.Table)
	if err := sqlb.QueryRow(tx, sum).Scan(&totalBytes); err != nil {
		return "", fmt.Errorf("repos/%s: sum stored_bytes: %w", h.scope, err)
	}
	if totalBytes > int64(byteBudget) {
		const running = "running"
		window := d.Select(c.ID, "SUM("+c.StoredBytes+") OVER (ORDER BY "+c.Time+" DESC, "+sqlb.RowID+" DESC"+
			" ROWS BETWEEN UNBOUNDED PRECEDING AND CURRENT ROW) AS "+running).From(c.Table)
		inRange := d.Select(c.ID)
		inRange.From(inRange.BuilderAs(window, "w"))
		inRange.Where(inRange.LessEqualThan(running, byteBudget))
		sweep := d.DeleteFrom(c.Table)
		sweep.Where(sweep.NotIn(c.ID, inRange))
		if _, err := sqlb.Exec(tx, sweep); err != nil {
			return "", fmt.Errorf("repos/%s: sweep budget: %w", h.scope, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("repos/%s: commit: %w", h.scope, err)
	}
	return id, nil
}

// GetHistory is the lookup-by-id shape both grpc_history.go's and response_history.go's own Get
// share: query the summary columns plus snapshot_json in one row and hand it to
// scanAndAssemble. A package-level function, not a historyTable[T] method: Get's result is never
// T (List's own summary Entry) but each table's own richer Snapshot, and Go allows no additional
// type parameter on a method beyond its receiver's own — the doc's own stated
// `Get(db, id string) (T, error)` does not typecheck against historyTable[T] for that reason.
// scanAndAssemble does the summary-column Scan, the snapshot_json Scan and its json.Unmarshal,
// and the final Snapshot struct literal in one closure, wrapping its own two distinct errors
// ("get" for a Scan failure, "decode snapshot %s" for an Unmarshal one) exactly as each table's
// own Get always did — collapsing them into one message here would lose that distinction.
func GetHistory[S any](db *sql.DB, d sqlb.Dialect, c historyColumns, columns []string, id string, scanAndAssemble func(rowScanner) (S, error)) (S, error) {
	sb := d.Select(append(append([]string{}, columns...), c.SnapshotJSON)...).From(c.Table)
	sb.Where(sb.Equal(c.ID, id))
	return scanAndAssemble(sqlb.QueryRow(db, sb))
}
