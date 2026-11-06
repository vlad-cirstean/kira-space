package repos

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/huandu/go-sqlbuilder"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage"
	"github.com/kirathecat/kira-studio/internal/scriptruns"
)

func planOf(t *testing.T, db *sql.DB, query string, args []any) string {
	t.Helper()
	rows, err := db.Query("EXPLAIN QUERY PLAN "+query, args...)
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

// The hot queries P552 re-indexed keep their plan on the SQL the repos build: no temp B-tree where
// the index orders the result, and the intended index serves the lookup.
func TestHotQueryPlans(t *testing.T) {
	sdb, err := storage.OpenAt(t.TempDir())
	if err != nil {
		t.Fatalf("OpenAt: %v", err)
	}
	t.Cleanup(func() { _ = sdb.Close() })
	db := sdb.DB

	oplog := &OpLogRepo{DB: db}
	tasks := &AdeTaskRepo{DB: db}
	sessions := &AdeSessionsRepo{DB: db}
	saved := &SavedQueriesRepo{DB: db}
	crs := &GitHostChangeRequestsRepo{DB: db}
	category := func(c *sqlbuilder.Cond) string { return c.Equal("category", "git") }

	cases := []struct {
		name   string
		b      interface{ Build() (string, []any) }
		want   string
		noTemp bool
	}{
		{"op_log recent", oplog.recentQuery(10), "SCAN op_log USING INDEX op_log_started", true},
		{"op_log prune by category", oplog.pruneCountQuery(category, 100), "USING COVERING INDEX op_log_category (category=?)", true},
		{"ade_runs pending by branch", tasks.pendingOnBranchQuery("b", ""), "USING INDEX ade_runs_branch (branch_id=? AND state=? AND purpose=?)", false},
		{"ade_runs by task and purpose", tasks.reviewRoundsQuery("t"), "USING INDEX ade_runs_task (task_id=? AND purpose=?)", false},
		{"ade_sessions running TUI", sessions.runningTUIQuery(), "SCAN ade_sessions USING INDEX ade_sessions_running", false},
		{"script_runs recent", (&scriptruns.Repo{DB: db}).ListQuery(10), "SCAN script_runs USING INDEX script_runs_created", true},
		{"saved queries list", saved.listByKindQuery("c", "p", "filter"), "USING INDEX connection_saved_queries_path (connection_id=? AND path=? AND kind=?)", true},
		{"change requests by head", crs.byHeadQuery("r", "o", "h"), "USING INDEX git_host_change_requests_head (repo_key=? AND head_owner=? AND head_ref=?)", false},
		{"change requests open", crs.openQuery("r"), "USING INDEX git_host_change_requests_open (repo_key=?)", false},
	}
	for _, c := range cases {
		q, args := c.b.Build()
		plan := planOf(t, db, q, args)
		if !strings.Contains(plan, c.want) {
			t.Errorf("%s: plan %q lacks %q", c.name, plan, c.want)
		}
		if c.noTemp && strings.Contains(plan, "TEMP B-TREE") {
			t.Errorf("%s: plan %q has a temp B-tree", c.name, plan)
		}
		// by head: the index lacks the trailing tiebreak column (number), so a last-term sort of one
		// index range is expected; a full sort is not.
		if c.name == "change requests by head" && strings.Contains(plan, "TEMP B-TREE FOR ORDER BY") {
			t.Errorf("%s: plan %q sorts the whole result", c.name, plan)
		}
	}
}
