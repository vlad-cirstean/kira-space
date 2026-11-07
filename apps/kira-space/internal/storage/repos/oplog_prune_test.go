package repos_test

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/oplog"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/repos"
	"github.com/kirathecat/kira-studio/internal/kiratime"
)

// Guards the count-prune boundary: go-sqlbuilder drops a bare OFFSET on SQLite, which would
// delete every row of the scope.
func TestOpLogRepo_Prune(t *testing.T) {
	base := time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC)
	seed := func(t *testing.T, r *repos.OpLogRepo) {
		t.Helper()
		put := func(id, category string, at time.Time) {
			rec := oplog.Record{ID: id, StartedAt: kiratime.FormatISO(at), Category: category, Kind: "k", Status: oplog.StatusOK,
				Module: "m", Trigger: "user", Source: "s", Steps: []oplog.Step{}, Repeat: 1}
			if err := r.Upsert(rec); err != nil {
				t.Fatal(err)
			}
		}
		put("old", oplog.CategoryMCP, base.Add(-48*time.Hour))
		for i := range 4 {
			put(fmt.Sprintf("git%d", i), "git", base.Add(time.Duration(i)*time.Minute))
			put(fmt.Sprintf("db%d", i), oplog.CategoryDB, base.Add(time.Duration(i)*time.Minute))
		}
	}
	ids := func(t *testing.T, r *repos.OpLogRepo) []string {
		t.Helper()
		recs, err := r.Recent(100)
		if err != nil {
			t.Fatal(err)
		}
		out := []string{}
		for _, rec := range recs {
			out = append(out, rec.ID)
		}
		slices.Sort(out)
		return out
	}
	cutoff := base.Add(-time.Hour)

	tests := []struct {
		name string
		run  func(r *repos.OpLogRepo) error
		want []string
	}{
		{"prune keeps newest non-db rows and all db rows", func(r *repos.OpLogRepo) error { return r.Prune(cutoff, 2) },
			[]string{"db0", "db1", "db2", "db3", "git2", "git3"}},
		{"prune with cap above row count keeps the rest", func(r *repos.OpLogRepo) error { return r.Prune(cutoff, 10) },
			[]string{"db0", "db1", "db2", "db3", "git0", "git1", "git2", "git3"}},
		{"prune category keeps newest of the category only", func(r *repos.OpLogRepo) error { return r.PruneCategory(oplog.CategoryDB, cutoff, 1) },
			[]string{"db3", "git0", "git1", "git2", "git3", "old"}},
		{"prune category deletes expired rows of its category", func(r *repos.OpLogRepo) error { return r.PruneCategory(oplog.CategoryMCP, cutoff, 5) },
			[]string{"db0", "db1", "db2", "db3", "git0", "git1", "git2", "git3"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := &repos.OpLogRepo{DB: newRepos(t).DB}
			seed(t, r)
			if err := tc.run(r); err != nil {
				t.Fatal(err)
			}
			if got := ids(t, r); !slices.Equal(got, tc.want) {
				t.Errorf("rows = %v, want %v", got, tc.want)
			}
		})
	}
}
