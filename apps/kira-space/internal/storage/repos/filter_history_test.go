package repos_test

import (
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// TestFilterHistoryReRecordMovesNotDuplicates pins the dedupe delete's use of SQLite's NULL-safe
// IS operator: `where_text = NULL` never matches anything, so the obvious `=` form would let an
// entry with a NULL where_text be re-inserted on every use and fill the history with copies of
// itself. Re-recording an identical entry must move it, not duplicate it.
func TestFilterHistoryReRecordMovesNotDuplicates(t *testing.T) {
	r := newFilterHistoryRepo(t)
	seedConnection(t, r.DB, "c1")

	// where=nil, orderBy=nil is a no-op per Record's own rule, so exercise the NULL where_text
	// path with an orderBy-only entry re-recorded identically — the IS-operator dedupe's actual
	// target.
	spec := &model.SortSpec{Kind: "text", Text: "x"}
	if err := r.Record("c1", "p", nil, spec); err != nil {
		t.Fatalf("Record 1: %v", err)
	}
	if err := r.Record("c1", "p", nil, spec); err != nil {
		t.Fatalf("Record 2 (identical): %v", err)
	}
	entries, err := r.List("c1", "p", 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("List() after re-recording identical entry = %+v, want exactly 1 (moved, not duplicated)", entries)
	}
}

// P23 §4.2 case 3: filter_history's per-path cap (regression — Record had no test at all before
// this phase) still trims to 20 rows on its own path once the per-connection cap (D4) is nowhere
// near being reached.
func TestFilterHistoryPerPathCapStillWorks(t *testing.T) {
	r := newFilterHistoryRepo(t)
	seedConnection(t, r.DB, "c1")

	for i := 0; i < 21; i++ {
		w := fmt.Sprintf("where-%d", i)
		if err := r.Record("c1", "p", &w, nil); err != nil {
			t.Fatalf("Record(%d): %v", i, err)
		}
	}

	entries, err := r.List("c1", "p", 100)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 20 {
		t.Fatalf("List() after 21 recordings on one path = %d entries, want 20", len(entries))
	}
}

// Past the per-connection cap (4,000 = metadata_cache's 200 paths/connection x filter_history's own
// 20 rows/path), the least-recently-used path's rows are evicted and the most-recently-used path's
// are not — the property a flat per-path cap alone cannot give. 4,000 background rows are seeded
// directly (one bulk transaction, bypassing Record's own three statements per call) so the
// boundary is exercised with a single real Record call rather than 4,000 of them, which is both
// true to what the cap bounds and fast.
func TestFilterHistoryPerConnectionCapEvictsLeastRecentlyUsedPath(t *testing.T) {
	r := newFilterHistoryRepo(t)
	seedConnection(t, r.DB, "c1")

	tx, err := r.DB.Begin()
	if err != nil {
		t.Fatalf("begin seed tx: %v", err)
	}
	stmt, err := tx.Prepare(`INSERT INTO connection_filter_history (id, connection_id, path, where_text, order_by_json, used_at)
		VALUES (?, 'c1', ?, 'seed', NULL, ?)`)
	if err != nil {
		t.Fatalf("prepare seed insert: %v", err)
	}
	base := time.Now().Add(-24 * time.Hour)
	const total = 4000 // historyPerConnectionLimit, unexported — mirrored here as a literal the
	// same way other repo tests mirror a private cap rather than exporting it.
	for i := 0; i < total; i++ {
		id := fmt.Sprintf("fh-%05d", i)
		path := fmt.Sprintf("p%05d", i) // one row per path: the per-path cap (20) never fires here.
		usedAt := base.Add(time.Duration(i) * time.Millisecond).UnixMilli()
		if _, err := stmt.Exec(id, path, usedAt); err != nil {
			t.Fatalf("seed row %d: %v", i, err)
		}
	}
	if err := stmt.Close(); err != nil {
		t.Fatalf("close seed stmt: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit seed tx: %v", err)
	}

	// One real Record call, on a brand-new path — this is what must trigger the per-connection
	// sweep and push the connection from exactly at cap to one over.
	w := "newest"
	if err := r.Record("c1", "new-path", &w, nil); err != nil {
		t.Fatalf("Record: %v", err)
	}

	var count int
	if err := r.DB.QueryRow(`SELECT COUNT(*) FROM connection_filter_history WHERE connection_id = 'c1'`).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != total {
		t.Fatalf("filter_history row count for c1 = %d, want exactly %d (the per-connection cap)", count, total)
	}

	// p00000 has the oldest used_at of the whole seeded set — the least-recently-used path.
	var lruSurvived int
	err = r.DB.QueryRow(`SELECT 1 FROM connection_filter_history WHERE path = 'p00000'`).Scan(&lruSurvived)
	if err == nil {
		t.Error("the least-recently-used path's row survived the per-connection sweep, want it evicted")
	}

	newest, err := r.List("c1", "new-path", 10)
	if err != nil {
		t.Fatalf("List(new-path): %v", err)
	}
	if len(newest) != 1 {
		t.Fatal("the just-recorded row was itself evicted — the per-entry cap invariant is broken")
	}
}

// P23 §4.2 case 3: a where_text over the per-row cap (4 KiB) is stored truncated, silently.
func TestFilterHistoryWhereTextOverCapIsTruncated(t *testing.T) {
	r := newFilterHistoryRepo(t)
	seedConnection(t, r.DB, "c1")

	oversized := strings.Repeat("w", 5000)
	if err := r.Record("c1", "p", &oversized, nil); err != nil {
		t.Fatalf("Record: %v", err)
	}

	entries, err := r.List("c1", "p", 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("List() = %d entries, want 1", len(entries))
	}
	if entries[0].Where == nil || len(*entries[0].Where) != 4*1024 {
		gotLen := -1
		if entries[0].Where != nil {
			gotLen = len(*entries[0].Where)
		}
		t.Fatalf("Where length = %d, want exactly 4 KiB (4096)", gotLen)
	}
	if !strings.HasPrefix(oversized, *entries[0].Where) {
		t.Fatal("stored where_text is not a prefix of the original")
	}
}

// TestFilterHistoryWhereTextOverCapTruncatesOnRuneBoundary: a raw byte cut can
// land mid-rune for a where_text made of multi-byte UTF-8 characters, producing an invalid string
// nothing can safely display. Built from a 2-byte rune ("é") repeated so its 4 KiB cut point falls
// exactly mid-character under a naive byte truncation (4096 is even, so a 2-byte rune repeated
// would land clean — 3-byte runes don't divide it evenly, which is the point).
func TestFilterHistoryWhereTextOverCapTruncatesOnRuneBoundary(t *testing.T) {
	r := newFilterHistoryRepo(t)
	seedConnection(t, r.DB, "c1")

	const multiByteRune = "€" // U+20AC, 3 bytes in UTF-8 — 4096 is not a multiple of 3.
	oversized := strings.Repeat(multiByteRune, 2000)
	if err := r.Record("c1", "p", &oversized, nil); err != nil {
		t.Fatalf("Record: %v", err)
	}

	entries, err := r.List("c1", "p", 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 1 || entries[0].Where == nil {
		t.Fatalf("List() = %+v, want exactly 1 entry with a non-nil Where", entries)
	}
	got := *entries[0].Where
	if len(got) > 4*1024 {
		t.Fatalf("stored where_text is %d bytes, want at most 4096", len(got))
	}
	if !strings.HasPrefix(oversized, got) {
		t.Fatal("stored where_text is not a prefix of the original")
	}
	if !utf8.ValidString(got) {
		t.Fatalf("stored where_text %q is not valid UTF-8 — truncated mid-rune", got)
	}
}

// TestFilterHistoryOversizedOrderByIsDroppedNotCorrupted: order_by_json is an
// *encoded JSON* string — truncating it at any byte offset, rune-boundary-safe or not, still
// leaves invalid JSON, which List's own decode guard then drops the *whole row* over (including a
// perfectly good where_text sitting right next to it). An oversized orderBy must be dropped
// instead, keeping the row and its where_text intact.
func TestFilterHistoryOversizedOrderByIsDroppedNotCorrupted(t *testing.T) {
	r := newFilterHistoryRepo(t)
	seedConnection(t, r.DB, "c1")

	where := "status = 'active'"
	oversizedSpec := &model.SortSpec{Kind: "text", Text: strings.Repeat("x", 5000)}
	if err := r.Record("c1", "p", &where, oversizedSpec); err != nil {
		t.Fatalf("Record: %v", err)
	}

	entries, err := r.List("c1", "p", 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("List() = %+v, want exactly 1 entry (the row must survive, not be dropped as a bad order_by_json row)", entries)
	}
	if entries[0].Where == nil || *entries[0].Where != where {
		t.Fatalf("Where = %v, want %q — a good where_text must survive an oversized sibling orderBy", entries[0].Where, where)
	}
	if entries[0].OrderBy != nil {
		t.Fatalf("OrderBy = %+v, want nil — an oversized sort spec must be dropped, not stored truncated/corrupt", entries[0].OrderBy)
	}
}

// TestFilterHistoryRecordsNothingWhenOnlyAnOversizedOrderByIsGiven is Record's own "nothing worth
// recording" rule (D4's pre-existing where==nil&&orderBy==nil early return), reached from a second
// direction now that capOrderByJSON can itself turn a non-nil orderBy into nil (F9).
func TestFilterHistoryRecordsNothingWhenOnlyAnOversizedOrderByIsGiven(t *testing.T) {
	r := newFilterHistoryRepo(t)
	seedConnection(t, r.DB, "c1")

	oversizedSpec := &model.SortSpec{Kind: "text", Text: strings.Repeat("x", 5000)}
	if err := r.Record("c1", "p", nil, oversizedSpec); err != nil {
		t.Fatalf("Record: %v", err)
	}

	entries, err := r.List("c1", "p", 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("List() = %+v, want empty — an oversized orderBy with no where_text carries nothing worth recording", entries)
	}
}
