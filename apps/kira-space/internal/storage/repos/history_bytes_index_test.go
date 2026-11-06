package repos_test

import (
	"database/sql"
	"strings"
	"testing"
)

// P21 round 3 performance finding 11: the byte-budget guard's own comment in
// response_history.go/grpc_history.go claimed `SELECT COALESCE(SUM(stored_bytes), 0) FROM ...`
// was "a cheap indexed aggregate" — it wasn't; neither table had an index on stored_bytes, so
// SQLite full-scanned the table's own b-tree on every completed send/call regardless. Migration
// 0013_p21r3_history_bytes_index.sql adds a covering index on exactly that column to each table;
// this pins that SQLite's query planner actually uses it (an index-only scan), not just that the
// index exists.

func explainPlanDetail(t *testing.T, db *sql.DB, query string) string {
	t.Helper()
	rows, err := db.Query(query)
	if err != nil {
		t.Fatalf("EXPLAIN QUERY PLAN: %v", err)
	}
	defer rows.Close()
	var details []string
	for rows.Next() {
		var id, parent, notused int
		var detail string
		if err := rows.Scan(&id, &parent, &notused, &detail); err != nil {
			t.Fatalf("scan plan row: %v", err)
		}
		details = append(details, detail)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("plan rows: %v", err)
	}
	return strings.Join(details, " | ")
}

func TestResponseHistory_ByteBudgetSumUsesCoveringIndex(t *testing.T) {
	_, db := newResponseHistoryRepo(t)
	plan := explainPlanDetail(t, db,
		"EXPLAIN QUERY PLAN SELECT COALESCE(SUM(stored_bytes), 0) FROM api_response_history")
	if !strings.Contains(plan, "COVERING INDEX api_response_history_bytes") {
		t.Errorf("query plan = %q, want it to use the covering index (an index-only scan), not a full table scan", plan)
	}
}

func TestGrpcHistory_ByteBudgetSumUsesCoveringIndex(t *testing.T) {
	repo := newGrpcHistoryRepo(t)
	plan := explainPlanDetail(t, repo.DB,
		"EXPLAIN QUERY PLAN SELECT COALESCE(SUM(stored_bytes), 0) FROM grpc_call_history")
	if !strings.Contains(plan, "COVERING INDEX grpc_call_history_bytes") {
		t.Errorf("query plan = %q, want it to use the covering index (an index-only scan), not a full table scan", plan)
	}
}

// P23 D2/D3: op_log is the third table given this same shape — 0015_p23_op_log_bytes.sql's
// op_log_bytes index is what OpsRepo.Prune's own SUM(stored_bytes) gate needs to stay an
// index-only scan instead of degrading into the exact full-scan finding 11 caught on the other
// two tables.
func TestOpLog_ByteBudgetSumUsesCoveringIndex(t *testing.T) {
	ops := newOpsRepo(t)
	plan := explainPlanDetail(t, ops.DB,
		"EXPLAIN QUERY PLAN SELECT COALESCE(SUM(stored_bytes), 0) FROM op_log")
	if !strings.Contains(plan, "COVERING INDEX op_log_bytes") {
		t.Errorf("query plan = %q, want it to use the covering index (an index-only scan), not a full table scan", plan)
	}
}
