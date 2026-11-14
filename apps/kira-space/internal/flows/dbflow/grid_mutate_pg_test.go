package dbflow

import (
	"fmt"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters/testsupport"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// J-GRID-030: a bulk update staged across pages of a keyset-paged table. The first commit fails on
// a unique violation and changes nothing; after the fix the retry lands, touching exactly the
// intended rows once.
func TestGridMutatePgKeysetRetry(t *testing.T) {
	flowharness.RequireDocker(t)
	fx := testsupport.StartPostgres(t)
	pgExec(t, fx.URI,
		`drop table if exists app.keyset_retry`,
		`create table app.keyset_retry (id int primary key, n int not null, tag text unique)`,
		`insert into app.keyset_retry select g, 0, 't' || g from generate_series(1, 2000) g`)
	app := flowharness.New(t)
	conn := pgFixtureConn(t, app, fx)
	tp := tableNodePath(t, app, conn.ID, "keyset_retry")
	ds := app.DataStream(t)

	// Page through by keyset cursor and stage an update for every row that is a multiple of 400,
	// which sit on different pages.
	var stagedIDs []string
	args := flowharness.ReadArgs{ConnectionID: conn.ID, Path: tp, PageSize: 100}
	pages := 0
	for {
		p, _, err := ds.Read(args)
		if err != nil {
			t.Fatalf("Read page %d: %v", pages, err)
		}
		pages++
		for _, row := range p.Rows {
			var id int
			fmt.Sscan(row[0].(string), &id)
			if id%400 == 0 {
				stagedIDs = append(stagedIDs, row[0].(string))
			}
		}
		if p.NextToken == "" {
			break
		}
		args.Cursor = model.PageCursor{Mode: "after", Token: p.NextToken}
	}
	if pages != 20 || len(stagedIDs) != 5 {
		t.Fatalf("paged %d pages and staged %d ops, want 20 and 5", pages, len(stagedIDs))
	}

	plan := func(tag func(i int) string) []model.MutationRowOp {
		var ops []model.MutationRowOp
		for i, id := range stagedIDs {
			ops = append(ops, upd(vals("id", id), vals("n", "1", "tag", tag(i))))
		}
		return ops
	}
	staged := plan(func(int) string { return "t1" })

	// Every staged row gets the same unique tag: the second op violates it, so nothing changes.
	if _, err := ds.Mutate(conn.ID, tp, staged); err == nil {
		t.Fatal("mutate with a unique violation succeeded")
	}
	wantRows(t, "second client after the failed plan", pgRows(t, fx.URI, `select count(*) from app.keyset_retry where n <> 0`), [][]any{{"0"}})

	// Fix the plan (distinct tags) and retry.
	staged = plan(func(i int) string { return fmt.Sprintf("fixed%d", i) })
	if n, err := ds.Mutate(conn.ID, tp, staged); err != nil || n != 5 {
		t.Fatalf("retry = %d, %v, want 5 rows", n, err)
	}
	wantRows(t, "second client after the retry",
		pgRows(t, fx.URI, `select id, n, tag from app.keyset_retry where n <> 0 order by id`),
		[][]any{{"400", "1", "fixed0"}, {"800", "1", "fixed1"}, {"1200", "1", "fixed2"}, {"1600", "1", "fixed3"}, {"2000", "1", "fixed4"}})
	wantRows(t, "untouched rows", pgRows(t, fx.URI, `select count(*) from app.keyset_retry where n = 0`), [][]any{{"1995"}})
}
