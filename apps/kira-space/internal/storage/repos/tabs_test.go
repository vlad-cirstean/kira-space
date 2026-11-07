package repos_test

import (
	"encoding/json"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/repos"
)

func newTabsRepo(t *testing.T) *repos.TabsRepo {
	r := &repos.TabsRepo{DB: newRepos(t).DB}
	if _, err := r.DB.Exec(`INSERT INTO windows (key, sort_order) VALUES ('main', 0)`); err != nil {
		t.Fatalf("seed main window: %v", err)
	}
	return r
}

func validTabRecord(id string) model.TabRecord {
	return model.TabRecord{ID: id, Path: "database:app/table:t", Kind: "data", State: json.RawMessage(`{}`)}
}

// Save previously wrote a record with no validation at all — an invalid row round-tripped
// silently (write succeeded, then List's own guards dropped it on the very next read, with
// nothing at the write site to say why). Save must now reject the same shapes List already
// refuses to return, and it must reject the whole batch rather than partially writing it.
func TestTabsRepoSaveRejectsInvalidRecords(t *testing.T) {
	r := newTabsRepo(t)

	cases := []struct {
		name string
		rec  model.TabRecord
	}{
		{"empty id", model.TabRecord{ID: "", Path: "p", Kind: "data", State: json.RawMessage(`{}`)}},
		{"empty path", model.TabRecord{ID: "t1", Path: "", Kind: "data", State: json.RawMessage(`{}`)}},
		{"unrecognised kind", model.TabRecord{ID: "t1", Path: "p", Kind: "ddl", State: json.RawMessage(`{}`)}},
		{"nil state", model.TabRecord{ID: "t1", Path: "p", Kind: "data", State: nil}},
		{"array state", model.TabRecord{ID: "t1", Path: "p", Kind: "data", State: json.RawMessage(`[1,2]`)}},
		{"scalar state", model.TabRecord{ID: "t1", Path: "p", Kind: "data", State: json.RawMessage(`"x"`)}},
		{"malformed state", model.TabRecord{ID: "t1", Path: "p", Kind: "data", State: json.RawMessage(`{`)}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := r.Save("main", []model.TabRecord{tc.rec}); err == nil {
				t.Fatalf("Save(%+v) = nil, want a validation error", tc.rec)
			}
		})
	}
}

// A batch with one bad record must not partially apply — the good record alongside it must not
// be written either, matching Save's existing "clear then reinsert the whole set" semantics.
func TestTabsRepoSaveRejectsWholeBatchOnOneInvalidRecord(t *testing.T) {
	r := newTabsRepo(t)

	good := validTabRecord("t-good")
	bad := model.TabRecord{ID: "t-bad", Path: "p", Kind: "not-a-real-kind", State: json.RawMessage(`{}`)}
	if err := r.Save("main", []model.TabRecord{good, bad}); err == nil {
		t.Fatal("Save with one invalid record = nil, want an error")
	}

	got, err := r.List("main")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("List() after a rejected Save = %+v, want no rows written", got)
	}
}

// The straightforward success path: a valid batch round-trips through Save/List unchanged.
func TestTabsRepoSaveAndListRoundTrip(t *testing.T) {
	r := newTabsRepo(t)

	recs := []model.TabRecord{validTabRecord("t1"), validTabRecord("t2")}
	if err := r.Save("main", recs); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := r.List("main")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("List() = %+v, want 2 rows", got)
	}
	if got[0].ID != "t1" || got[1].ID != "t2" {
		t.Errorf("List() ids = [%s, %s], want [t1, t2]", got[0].ID, got[1].ID)
	}
}

// Save moved from "DELETE every row for the window, then
// re-INSERT every record" to an upsert (ON CONFLICT(id) DO UPDATE) plus a prune scoped to what
// left the set. This is the regression risk that rewrite carries: the original fix
// scoped its DELETE by window_key specifically because an earlier version deleted every window's
// tabs outright. This proves the rewrite still respects that scope, on both write paths (the
// upsert and the prune) — saving one window's tabs must never touch another window's rows, and
// removing a tab from one window's own set must not remove it from a different window that
// happens to hold a same-named (but distinctly-id'd) tab.
func TestTabsRepoSaveScopesUpsertAndPruneByWindow(t *testing.T) {
	r := newTabsRepo(t)

	// A second window ("other") needs its own `windows` row — `tabs.window_key` is a foreign key onto it.
	if _, err := r.DB.Exec(`INSERT INTO windows (key, sort_order, bounds_json) VALUES ('other', 1, NULL)`); err != nil {
		t.Fatalf("seed windows row: %v", err)
	}

	if err := r.Save("main", []model.TabRecord{validTabRecord("t1"), validTabRecord("t2")}); err != nil {
		t.Fatalf("Save(main): %v", err)
	}
	if err := r.Save("other", []model.TabRecord{validTabRecord("t3")}); err != nil {
		t.Fatalf("Save(other): %v", err)
	}

	// Saving "other" must not have touched "main"'s own rows.
	mainGot, err := r.List("main")
	if err != nil {
		t.Fatalf("List(main): %v", err)
	}
	if len(mainGot) != 2 {
		t.Fatalf("List(main) after Save(other) = %+v, want 2 rows untouched", mainGot)
	}

	// Dropping t2 from "main"'s own set (a tab close) must prune only t2, and must never reach
	// "other"'s own t3.
	if err := r.Save("main", []model.TabRecord{validTabRecord("t1")}); err != nil {
		t.Fatalf("Save(main) dropping t2: %v", err)
	}
	mainGot, err = r.List("main")
	if err != nil {
		t.Fatalf("List(main): %v", err)
	}
	if len(mainGot) != 1 || mainGot[0].ID != "t1" {
		t.Fatalf("List(main) after dropping t2 = %+v, want only t1", mainGot)
	}
	otherGot, err := r.List("other")
	if err != nil {
		t.Fatalf("List(other): %v", err)
	}
	if len(otherGot) != 1 || otherGot[0].ID != "t3" {
		t.Fatalf("List(other) after Save(main) pruned t2 = %+v, want t3 untouched", otherGot)
	}
}

// The upsert path's own point: re-saving the same id with new content updates that one row in
// place rather than duplicating it or silently keeping the old state_json.
func TestTabsRepoSaveUpsertsAnExistingRecordInPlace(t *testing.T) {
	r := newTabsRepo(t)

	rec := validTabRecord("t1")
	rec.State = json.RawMessage(`{"scrollTop":0}`)
	if err := r.Save("main", []model.TabRecord{rec}); err != nil {
		t.Fatalf("Save (first): %v", err)
	}

	rec.State = json.RawMessage(`{"scrollTop":42}`)
	if err := r.Save("main", []model.TabRecord{rec}); err != nil {
		t.Fatalf("Save (second): %v", err)
	}

	got, err := r.List("main")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("List() = %+v, want exactly 1 row (no duplicate from the second Save)", got)
	}
	if string(got[0].State) != `{"scrollTop":42}` {
		t.Errorf("List()[0].State = %s, want the second Save's own content", got[0].State)
	}
}

// The actual point of finding 9: an unrelated tab's own row must never be deleted just because a
// *different* tab in the same Save call changed. Pre-fix, Save unconditionally ran `DELETE FROM
// tabs WHERE window_key = ?` before reinserting everything — every row for the window was deleted
// and recreated, whatever changed. An AFTER DELETE trigger recording which ids were actually
// deleted is the direct, engine-level way to observe that difference: post-fix, an id that
// survives into the next Save's own record list is never deleted at all, only ever
// inserted once (the seed) and updated in place afterward.
func TestTabsRepoSaveDoesNotDeleteAnUnrelatedTabsRow(t *testing.T) {
	r := newTabsRepo(t)

	if _, err := r.DB.Exec(`CREATE TABLE test_deleted_tab_ids (id TEXT NOT NULL)`); err != nil {
		t.Fatalf("create instrumentation table: %v", err)
	}
	if _, err := r.DB.Exec(`
		CREATE TRIGGER test_log_tab_delete AFTER DELETE ON tabs
		BEGIN
		  INSERT INTO test_deleted_tab_ids (id) VALUES (old.id);
		END
	`); err != nil {
		t.Fatalf("create instrumentation trigger: %v", err)
	}

	if err := r.Save("main", []model.TabRecord{validTabRecord("t1"), validTabRecord("t2")}); err != nil {
		t.Fatalf("Save (seed): %v", err)
	}

	// t2's own state changes; t1 is included, unchanged, in the same Save call — exactly
	// saveDebounced()'s own shape (the whole tab set is sent on every save, whatever changed).
	t2 := validTabRecord("t2")
	t2.State = json.RawMessage(`{"edited":true}`)
	if err := r.Save("main", []model.TabRecord{validTabRecord("t1"), t2}); err != nil {
		t.Fatalf("Save (t2 changes): %v", err)
	}

	var deletedCount int
	if err := r.DB.QueryRow(`SELECT COUNT(*) FROM test_deleted_tab_ids WHERE id = 't1'`).Scan(&deletedCount); err != nil {
		t.Fatalf("query instrumentation table: %v", err)
	}
	if deletedCount != 0 {
		t.Errorf("t1 was deleted %d time(s) even though it never left the tab set — Save is still clearing rows it doesn't need to", deletedCount)
	}
}

// Saving a completely empty tab set (every tab closed) must still clear the window's own rows —
// the len(keep)==0 branch the upsert/prune split above carves out specially.
func TestTabsRepoSaveWithNoRecordsClearsTheWindow(t *testing.T) {
	r := newTabsRepo(t)

	if err := r.Save("main", []model.TabRecord{validTabRecord("t1")}); err != nil {
		t.Fatalf("Save (seed): %v", err)
	}
	if err := r.Save("main", []model.TabRecord{}); err != nil {
		t.Fatalf("Save (empty): %v", err)
	}

	got, err := r.List("main")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("List() after Save([]) = %+v, want no rows", got)
	}
}
