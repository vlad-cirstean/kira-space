package storage_test

import (
	"database/sql"
	"flag"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage"
)

var update = flag.Bool("update", false, "rewrite testdata/schema.sql")

const goldenPath = "testdata/schema.sql"

func openFresh(t *testing.T) *sql.DB {
	t.Helper()
	db, err := storage.OpenAt(t.TempDir())
	if err != nil {
		t.Fatalf("OpenAt: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db.DB
}

var spaces = regexp.MustCompile(`[ \t]+\n`)

// dumpSchema is the normalized sqlite_master of a fresh database, one statement per object.
func dumpSchema(t *testing.T, db *sql.DB) string {
	t.Helper()
	rows, err := db.Query(`SELECT type, name, sql FROM sqlite_master
		WHERE sql IS NOT NULL AND name NOT LIKE 'sqlite_%'
		ORDER BY CASE type WHEN 'table' THEN 0 ELSE 1 END, name`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var b strings.Builder
	for rows.Next() {
		var typ, name, stmt string
		if err := rows.Scan(&typ, &name, &stmt); err != nil {
			t.Fatal(err)
		}
		b.WriteString(spaces.ReplaceAllString(strings.TrimSpace(stmt), "\n"))
		b.WriteString(";\n\n")
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

// The golden file is the schema source of truth: a diff here is a schema change.
func TestSchemaSnapshot(t *testing.T) {
	got := dumpSchema(t, openFresh(t))
	if *update {
		if err := os.WriteFile(goldenPath, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Errorf("schema differs from %s; run go test ./apps/kira-space/internal/storage -update and review the diff", goldenPath)
	}
}

func TestSchemaInvariants(t *testing.T) {
	db := openFresh(t)

	var fk int
	if err := db.QueryRow(`PRAGMA foreign_keys`).Scan(&fk); err != nil || fk != 1 {
		t.Errorf("foreign_keys = %d (%v), want 1", fk, err)
	}

	rows, err := db.Query(`SELECT name FROM pragma_table_list WHERE schema = 'main' AND type = 'table'
		AND name NOT LIKE 'sqlite_%' AND name <> 'schema_version' AND strict = 0`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var name string
		_ = rows.Scan(&name)
		t.Errorf("table %s is not STRICT", name)
	}
	_ = rows.Close()

	// Every FK child column set must lead an index, or a parent delete scans the child.
	tables, err := db.Query(`SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'`)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for tables.Next() {
		var n string
		_ = tables.Scan(&n)
		names = append(names, n)
	}
	_ = tables.Close()
	for _, table := range names {
		for fkID, cols := range foreignKeys(t, db, table) {
			if unindexedFK[table+"."+cols[0]] {
				continue
			}
			if !hasLeadingIndex(t, db, table, cols) {
				t.Errorf("%s: foreign key %d %v has no index with those leading columns", table, fkID, cols)
			}
		}
	}
}

// unindexedFK lists deferred status FKs whose parent rows are deleted only by AdeStatusRepo.Save on
// tables of a few dozen rows; an index would cost every task write for nothing.
var unindexedFK = map[string]bool{
	"ade_tasks.status_id":                true,
	"ade_task_status_defaults.status_id": true,
}

func foreignKeys(t *testing.T, db *sql.DB, table string) map[int][]string {
	t.Helper()
	rows, err := db.Query(`SELECT id, "from" FROM pragma_foreign_key_list(?) ORDER BY id, seq`, table)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[int][]string{}
	for rows.Next() {
		var id int
		var col string
		if err := rows.Scan(&id, &col); err != nil {
			t.Fatal(err)
		}
		out[id] = append(out[id], col)
	}
	return out
}

// hasLeadingIndex reports whether the primary key, a unique constraint or an index starts with cols
// (a partial index on a nullable FK column serves the lookup, NULLs never match), in any order.
func hasLeadingIndex(t *testing.T, db *sql.DB, table string, cols []string) bool {
	t.Helper()
	want := map[string]bool{}
	for _, c := range cols {
		want[c] = true
	}
	var pk []string
	pkRows, err := db.Query(`SELECT name FROM pragma_table_info(?) WHERE pk > 0 ORDER BY pk`, table)
	if err != nil {
		t.Fatal(err)
	}
	for pkRows.Next() {
		var n string
		_ = pkRows.Scan(&n)
		pk = append(pk, n)
	}
	_ = pkRows.Close()
	if leads(pk, want) {
		return true
	}
	idx, err := db.Query(`SELECT name FROM pragma_index_list(?)`, table)
	if err != nil {
		t.Fatal(err)
	}
	var idxNames []string
	for idx.Next() {
		var n string
		_ = idx.Scan(&n)
		idxNames = append(idxNames, n)
	}
	_ = idx.Close()
	for _, n := range idxNames {
		ir, err := db.Query(`SELECT name FROM pragma_index_info(?) ORDER BY seqno`, n)
		if err != nil {
			t.Fatal(err)
		}
		var ic []string
		for ir.Next() {
			var c sql.NullString
			_ = ir.Scan(&c)
			ic = append(ic, c.String)
		}
		_ = ir.Close()
		if leads(ic, want) {
			return true
		}
	}
	return false
}

func leads(have []string, want map[string]bool) bool {
	if len(have) < len(want) {
		return false
	}
	for _, c := range have[:len(want)] {
		if !want[c] {
			return false
		}
	}
	return true
}

func planOf(t *testing.T, db *sql.DB, query string) string {
	t.Helper()
	rows, err := db.Query("EXPLAIN QUERY PLAN " + query)
	if err != nil {
		t.Fatalf("EXPLAIN %q: %v", query, err)
	}
	defer rows.Close()
	var parts []string
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		parts = append(parts, detail)
	}
	return strings.Join(parts, " | ")
}

// The hot queries P552 re-indexed keep their plan: no temp B-tree where the index orders the result.
func TestHotQueryPlans(t *testing.T) {
	db := openFresh(t)
	cases := []struct {
		name, query, want string
		noTemp            bool
	}{
		{"op_log recent", `SELECT * FROM op_log ORDER BY started_ms DESC, id DESC LIMIT 10`, "op_log_started", true},
		{"op_log prune by category", `SELECT id FROM op_log WHERE category = 'git' ORDER BY started_ms DESC, id DESC LIMIT -1 OFFSET 100`, "op_log_category", true},
		{"ade_runs pending by branch", `SELECT * FROM ade_runs WHERE branch_id = 'b' AND state = 'pending' ORDER BY id`, "ade_runs_branch", false},
		{"ade_sessions running TUI", `SELECT count(*) FROM ade_sessions WHERE state = 'running' AND mode = 'tui'`, "ade_sessions_running", false},
		{"script_runs recent", `SELECT * FROM script_runs ORDER BY created_at DESC, id DESC LIMIT 10`, "script_runs_created", true},
	}
	for _, c := range cases {
		plan := planOf(t, db, c.query)
		if !strings.Contains(plan, c.want) {
			t.Errorf("%s: plan %q lacks %s", c.name, plan, c.want)
		}
		if c.noTemp && strings.Contains(plan, "TEMP B-TREE") {
			t.Errorf("%s: plan %q has a temp B-tree", c.name, plan)
		}
	}
}
