// referenced_by_internal_test.go: listReferencedBy used to run one
// `SELECT * FROM pragma_foreign_key_list(?)` per table in the database looking for one that
// pointed back at the target — describing one table in a 500-table file was ~501 serialised
// round trips on the single pinned connection. fetchReferencingForeignKeys now joins every table's
// own pragma_foreign_key_list against sqlite_master in one query.
package sqlite

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

// countingExecutor wraps a real *sql.Conn as a QueryExecutor, counting how many queries actually
// ran — the direct measure of the N+1 this finding is about.
func countingExecutor(conn *sql.Conn) (QueryExecutor, *int) {
	calls := 0
	exec := func(query string, params []any, scan func(*sql.Rows) error) error {
		calls++
		rows, err := conn.QueryContext(context.Background(), query, params...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			if err := scan(rows); err != nil {
				return err
			}
		}
		return rows.Err()
	}
	return exec, &calls
}

func mustExec(t *testing.T, conn *sql.Conn, stmt string) {
	t.Helper()
	if _, err := conn.ExecContext(context.Background(), stmt); err != nil {
		t.Fatalf("exec %q: %v", stmt, err)
	}
}

func setupReferencedBySchema(t *testing.T) *sql.Conn {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	conn, err := db.Conn(context.Background())
	if err != nil {
		t.Fatalf("Conn: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	mustExec(t, conn, `CREATE TABLE orders (id INTEGER PRIMARY KEY)`)
	mustExec(t, conn, `CREATE TABLE order_items (
		id INTEGER PRIMARY KEY,
		order_id INTEGER REFERENCES orders(id) ON DELETE CASCADE
	)`)
	mustExec(t, conn, `CREATE TABLE refunds (
		id INTEGER PRIMARY KEY,
		order_id INTEGER REFERENCES orders(id) ON DELETE SET NULL
	)`)
	mustExec(t, conn, `CREATE TABLE products (id INTEGER PRIMARY KEY, name TEXT)`) // no FK to orders
	mustExec(t, conn, `CREATE TABLE categories (
		id INTEGER PRIMARY KEY,
		parent_id INTEGER REFERENCES categories(id)
	)`) // self-referencing
	mustExec(t, conn, `CREATE VIEW orders_view AS SELECT * FROM orders`)
	return conn
}

func TestListReferencedBy_OneQueryFindsEveryReferencingTable(t *testing.T) {
	conn := setupReferencedBySchema(t)
	exec, calls := countingExecutor(conn)

	allTables, err := listAllTableNames(exec, "main")
	if err != nil {
		t.Fatalf("listAllTableNames: %v", err)
	}
	*calls = 0 // only count the calls listReferencedBy itself makes

	referencedBy, err := listReferencedBy(exec, "main", "orders", allTables)
	if err != nil {
		t.Fatalf("listReferencedBy: %v", err)
	}

	if *calls != 1 {
		t.Errorf("query count = %d, want exactly 1 (was one per table before the fix)", *calls)
	}

	if len(referencedBy) != 2 {
		t.Fatalf("len(referencedBy) = %d, want 2 (order_items, refunds); got %+v", len(referencedBy), referencedBy)
	}
	byName := map[string]bool{}
	for _, fk := range referencedBy {
		byName[fk.Name] = true
		// Columns is the *described* table's own column (orders.id); ReferencedColumns is the
		// referencing table's column (order_items.order_id / refunds.order_id) — the same
		// from/to convention listForeignKeys uses in the opposite direction.
		if len(fk.Columns) != 1 || fk.Columns[0] != "id" {
			t.Errorf("fk %s: Columns = %v, want [id]", fk.Name, fk.Columns)
		}
		if len(fk.ReferencedColumns) != 1 || fk.ReferencedColumns[0] != "order_id" {
			t.Errorf("fk %s: ReferencedColumns = %v, want [order_id]", fk.Name, fk.ReferencedColumns)
		}
	}
	if !byName["order_items_order_id_fkey"] {
		t.Errorf("missing order_items' own FK in %+v", referencedBy)
	}
	if !byName["refunds_order_id_fkey"] {
		t.Errorf("missing refunds' own FK in %+v", referencedBy)
	}
}

func TestListReferencedBy_SelfReferencingTableAppearsInItsOwnReferencedBy(t *testing.T) {
	conn := setupReferencedBySchema(t)
	exec, _ := countingExecutor(conn)

	allTables, err := listAllTableNames(exec, "main")
	if err != nil {
		t.Fatalf("listAllTableNames: %v", err)
	}

	referencedBy, err := listReferencedBy(exec, "main", "categories", allTables)
	if err != nil {
		t.Fatalf("listReferencedBy: %v", err)
	}
	if len(referencedBy) != 1 || referencedBy[0].Name != "categories_parent_id_fkey" {
		t.Fatalf("referencedBy = %+v, want exactly one self-referencing FK", referencedBy)
	}
}

func TestListReferencedBy_TableWithNoReferencersIsEmpty(t *testing.T) {
	conn := setupReferencedBySchema(t)
	exec, _ := countingExecutor(conn)

	allTables, err := listAllTableNames(exec, "main")
	if err != nil {
		t.Fatalf("listAllTableNames: %v", err)
	}

	referencedBy, err := listReferencedBy(exec, "main", "products", allTables)
	if err != nil {
		t.Fatalf("listReferencedBy: %v", err)
	}
	if len(referencedBy) != 0 {
		t.Errorf("referencedBy = %+v, want none — nothing references products", referencedBy)
	}
}
