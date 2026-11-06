// The first test in this repo to exercise a migration against seeded data, because it is
// the first migration that touches data rather than adding to it (0010 is a
// real, irreversible rename of user data, not a fresh CREATE TABLE). CLAUDE.md's test bar admits
// this by name — "an irreversible data migration" — as a one-time proof, not ongoing CRUD coverage.
package migrations_test

import (
	"database/sql"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/migrations"
	"github.com/kirathecat/kira-studio/internal/sqlitex"
	_ "modernc.org/sqlite"
)

// openAt applies every migration up to and including maxVersion, in order — the same
// version-by-version loop storage.DB.migrate() runs, reimplemented here (rather than imported)
// because internal/storage imports internal/storage/migrations, and this test needs to stop
// partway through the sequence, which storage.Open() has no seam for.
func openAt(t *testing.T, maxVersion int) *sql.DB {
	t.Helper()
	dsn := "file:" + t.TempDir() + "/kira.sqlite?_foreign_keys=1"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	steps, err := migrations.All()
	if err != nil {
		t.Fatalf("migrations.All: %v", err)
	}
	for _, m := range steps {
		if m.Version > maxVersion {
			continue
		}
		if _, err := db.Exec(m.SQL); err != nil {
			t.Fatalf("apply migration %s (v%d): %v", m.Name, m.Version, err)
		}
	}
	return db
}

func mustExec(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.Exec(query, args...); err != nil {
		t.Fatalf("exec %q: %v", query, err)
	}
}

func scalarString(t *testing.T, db *sql.DB, query string, args ...any) string {
	t.Helper()
	var v string
	if err := db.QueryRow(query, args...).Scan(&v); err != nil {
		t.Fatalf("query %q: %v", query, err)
	}
	return v
}

func scalarInt(t *testing.T, db *sql.DB, query string, args ...any) int {
	t.Helper()
	var v int
	if err := db.QueryRow(query, args...).Scan(&v); err != nil {
		t.Fatalf("query %q: %v", query, err)
	}
	return v
}

// TestMigration10RenamesTablesAndPreservesData seeds a v9-shaped database — a collection, a
// folder, an HTTP request item, a gRPC request item, a collection variable, an environment plus
// its own variable, a variable-history row and a response-history row, plus a grpc_call_history
// row (which is NOT renamed) — applies migration 10, and checks every row survives under the
// new table names with identical values, that both generated scope_key columns still compute,
// that grpc_call_history's own foreign key now names api_items, and that a collection delete
// still cascades all the way down.
func TestMigration10RenamesTablesAndPreservesData(t *testing.T) {
	db := openAt(t, 9)

	now := "2026-01-01T00:00:00Z"

	// A collection with a folder and an HTTP request item inside it, plus a gRPC request item at
	// the collection root — http_items.protocol distinguishes the two (0009_p11_grpc.sql).
	mustExec(t, db, `INSERT INTO http_collections (id, name, sort_order, created_at, updated_at)
		VALUES ('col1', 'Collection One', 0, ?, ?)`, now, now)
	mustExec(t, db, `INSERT INTO http_items (id, collection_id, parent_id, kind, name, sort_order, method, url, request_json, protocol, created_at, updated_at)
		VALUES ('folder1', 'col1', NULL, 'folder', 'Folder', 0, '', '', '', 'http', ?, ?)`, now, now)
	mustExec(t, db, `INSERT INTO http_items (id, collection_id, parent_id, kind, name, sort_order, method, url, request_json, protocol, created_at, updated_at)
		VALUES ('httpitem1', 'col1', 'folder1', 'request', 'HTTP Request', 0, 'GET', 'https://api.example.com', '{}', 'http', ?, ?)`, now, now)
	mustExec(t, db, `INSERT INTO http_items (id, collection_id, parent_id, kind, name, sort_order, method, url, request_json, protocol, created_at, updated_at)
		VALUES ('grpcitem1', 'col1', NULL, 'request', 'gRPC Request', 1, '', '', '{}', 'grpc', ?, ?)`, now, now)

	// A collection-owned variable, plus an environment and its own variable.
	mustExec(t, db, `INSERT INTO http_variables (id, collection_id, environment_id, name, value, is_secret, secret_value, sort_order, created_at, updated_at)
		VALUES ('var1', 'col1', NULL, 'baseUrl', 'https://api.example.com', 0, NULL, 0, ?, ?)`, now, now)
	mustExec(t, db, `INSERT INTO http_environments (id, name, sort_order, is_active, created_at, updated_at)
		VALUES ('env1', 'Production', 0, 1, ?, ?)`, now, now)
	mustExec(t, db, `INSERT INTO http_variables (id, collection_id, environment_id, name, value, is_secret, secret_value, sort_order, created_at, updated_at)
		VALUES ('envvar1', NULL, 'env1', 'token', 'secret-token', 0, NULL, 0, ?, ?)`, now, now)

	// A history row for the collection variable.
	mustExec(t, db, `INSERT INTO http_variable_history (id, variable_id, value, is_secret, secret_value, recorded_at)
		VALUES ('hist1', 'var1', 'https://old.example.com', 0, NULL, ?)`, now)

	// A response-history row scoped to the HTTP item, and a grpc_call_history row scoped to the
	// gRPC item — grpc_call_history is never renamed (D14), but its FK into http_items/api_items
	// is what D15's own PRAGMA foreign_key_list check exists to prove.
	mustExec(t, db, `INSERT INTO http_response_history (id, item_id, tab_id, sent_at, method, url, environment, status, status_text, elapsed_ms, body_bytes, stored_bytes, snapshot_json)
		VALUES ('resp1', 'httpitem1', 'tab1', ?, 'GET', 'https://api.example.com', 'Production', 200, 'OK', 42, 10, 10, '{}')`, now)
	mustExec(t, db, `INSERT INTO grpc_call_history (id, item_id, tab_id, called_at, target, method, streaming, environment, code, code_name, status_message, elapsed_ms, message_count, message_bytes, stored_bytes, snapshot_json)
		VALUES ('call1', 'grpcitem1', 'tab2', ?, 'localhost:50051', 'pkg.Service/Method', 'unary', 'Production', 0, 'OK', '', 15, 1, 20, 20, '{}')`, now)

	// Apply migration 10.
	steps, err := migrations.All()
	if err != nil {
		t.Fatalf("migrations.All: %v", err)
	}
	var m10 *sqlitex.Migration
	for i := range steps {
		if steps[i].Version == 10 {
			m10 = &steps[i]
		}
	}
	if m10 == nil {
		t.Fatalf("no migration with Version == 10 registered")
	}
	if _, err := db.Exec(m10.SQL); err != nil {
		t.Fatalf("apply migration 10: %v", err)
	}

	// Every row survives, under the new table names, with identical values.
	if got := scalarString(t, db, `SELECT name FROM api_collections WHERE id = 'col1'`); got != "Collection One" {
		t.Errorf("api_collections.name = %q, want %q", got, "Collection One")
	}
	if got := scalarString(t, db, `SELECT name FROM api_items WHERE id = 'folder1'`); got != "Folder" {
		t.Errorf("api_items(folder1).name = %q, want %q", got, "Folder")
	}
	if got := scalarString(t, db, `SELECT protocol FROM api_items WHERE id = 'httpitem1'`); got != "http" {
		t.Errorf("api_items(httpitem1).protocol = %q, want %q", got, "http")
	}
	if got := scalarString(t, db, `SELECT protocol FROM api_items WHERE id = 'grpcitem1'`); got != "grpc" {
		t.Errorf("api_items(grpcitem1).protocol = %q, want %q", got, "grpc")
	}
	if got := scalarString(t, db, `SELECT value FROM api_variables WHERE id = 'var1'`); got != "https://api.example.com" {
		t.Errorf("api_variables(var1).value = %q, want %q", got, "https://api.example.com")
	}
	if got := scalarString(t, db, `SELECT value FROM api_variables WHERE id = 'envvar1'`); got != "secret-token" {
		t.Errorf("api_variables(envvar1).value = %q, want %q", got, "secret-token")
	}
	if got := scalarString(t, db, `SELECT name FROM api_environments WHERE id = 'env1'`); got != "Production" {
		t.Errorf("api_environments.name = %q, want %q", got, "Production")
	}
	if got := scalarString(t, db, `SELECT value FROM api_variable_history WHERE id = 'hist1'`); got != "https://old.example.com" {
		t.Errorf("api_variable_history.value = %q, want %q", got, "https://old.example.com")
	}
	if got := scalarString(t, db, `SELECT snapshot_json FROM api_response_history WHERE id = 'resp1'`); got != "{}" {
		t.Errorf("api_response_history.snapshot_json = %q, want %q", got, "{}")
	}
	if got := scalarString(t, db, `SELECT snapshot_json FROM grpc_call_history WHERE id = 'call1'`); got != "{}" {
		t.Errorf("grpc_call_history.snapshot_json = %q, want %q", got, "{}")
	}

	// Both scope_key generated columns still compute (scope_key = item_id when present).
	if got := scalarString(t, db, `SELECT scope_key FROM api_response_history WHERE id = 'resp1'`); got != "httpitem1" {
		t.Errorf("api_response_history.scope_key = %q, want %q", got, "httpitem1")
	}
	if got := scalarString(t, db, `SELECT scope_key FROM grpc_call_history WHERE id = 'call1'`); got != "grpcitem1" {
		t.Errorf("grpc_call_history.scope_key = %q, want %q", got, "grpcitem1")
	}

	// grpc_call_history's own foreign key now names api_items, not http_items — the property the
	// rename could most plausibly break silently (a REFERENCES clause left pointing at a dropped
	// name does not error until a delete fails to cascade, possibly months later).
	var fkTable string
	if err := db.QueryRow(`SELECT "table" FROM pragma_foreign_key_list('grpc_call_history') WHERE "from" = 'item_id'`).Scan(&fkTable); err != nil {
		t.Fatalf("pragma_foreign_key_list(grpc_call_history): %v", err)
	}
	if fkTable != "api_items" {
		t.Errorf("grpc_call_history's item_id FK references %q, want %q", fkTable, "api_items")
	}

	// Every renamed index exists under its new name, and none of the old http_* index names
	// survive.
	wantIndexes := []string{
		"api_items_tree",
		"api_variables_collection",
		"api_variables_environment",
		"api_variable_history_var",
		"api_response_history_scope",
		"api_response_history_age",
		"api_response_history_tab",
	}
	for _, idx := range wantIndexes {
		count := scalarInt(t, db, `SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = ?`, idx)
		if count != 1 {
			t.Errorf("index %q: got count %d, want 1", idx, count)
		}
	}
	oldIndexes := []string{
		"http_items_tree",
		"http_variables_collection",
		"http_variables_environment",
		"http_variable_history_var",
		"http_response_history_scope",
		"http_response_history_age",
		"http_response_history_tab",
	}
	for _, idx := range oldIndexes {
		count := scalarInt(t, db, `SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = ?`, idx)
		if count != 0 {
			t.Errorf("stale index %q still exists after the rename", idx)
		}
	}
	oldTables := []string{
		"http_collections", "http_items", "http_environments",
		"http_variables", "http_variable_history", "http_response_history",
	}
	for _, tbl := range oldTables {
		count := scalarInt(t, db, `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, tbl)
		if count != 0 {
			t.Errorf("stale table %q still exists after the rename", tbl)
		}
	}

	// A collection delete still cascades: its items, its own variable, that variable's history,
	// and the response-history row scoped to one of its items all disappear in one statement.
	// The environment and its own variable are untouched — they belong to no collection.
	mustExec(t, db, `DELETE FROM api_collections WHERE id = 'col1'`)

	if n := scalarInt(t, db, `SELECT COUNT(*) FROM api_items WHERE collection_id = 'col1'`); n != 0 {
		t.Errorf("api_items: %d rows survived the collection delete, want 0", n)
	}
	if n := scalarInt(t, db, `SELECT COUNT(*) FROM api_variables WHERE id = 'var1'`); n != 0 {
		t.Errorf("api_variables(var1): %d rows survived the collection delete, want 0", n)
	}
	if n := scalarInt(t, db, `SELECT COUNT(*) FROM api_variable_history WHERE id = 'hist1'`); n != 0 {
		t.Errorf("api_variable_history(hist1): %d rows survived the variable's own cascade, want 0", n)
	}
	if n := scalarInt(t, db, `SELECT COUNT(*) FROM api_response_history WHERE id = 'resp1'`); n != 0 {
		t.Errorf("api_response_history(resp1): %d rows survived the item's own cascade, want 0", n)
	}
	if n := scalarInt(t, db, `SELECT COUNT(*) FROM grpc_call_history WHERE id = 'call1'`); n != 0 {
		t.Errorf("grpc_call_history(call1): %d rows survived the item's own cascade, want 0", n)
	}
	if n := scalarInt(t, db, `SELECT COUNT(*) FROM api_environments WHERE id = 'env1'`); n != 1 {
		t.Errorf("api_environments(env1): %d rows, want 1 (unrelated to the deleted collection)", n)
	}
	if n := scalarInt(t, db, `SELECT COUNT(*) FROM api_variables WHERE id = 'envvar1'`); n != 1 {
		t.Errorf("api_variables(envvar1): %d rows, want 1 (owned by the environment, not the collection)", n)
	}
}
