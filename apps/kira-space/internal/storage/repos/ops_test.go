package repos_test

import (
	"fmt"
	"github.com/kirathecat/kira-studio/internal/kiratime"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/repos"
)

// op_log's caps now interact four ways (age cut, row cap, byte budget, per-row truncation), and
// the sweep is only *safe* because the truncation holds — "cache eviction/invalidation with
// interacting rules", CLAUDE.md's own named category, not CRUD round trips. Prune's two
// pre-existing passes are pinned here too, as regression cases, since Prune is being
// restructured for the first time.

func appendAndFinish(t *testing.T, ops *repos.DbOpsRepo, id, startedAt string, patch model.OpFinish) {
	t.Helper()
	if err := ops.Append(model.OpAppend{ID: id, Kind: "read", StartedAt: startedAt}); err != nil {
		t.Fatalf("Append(%s): %v", id, err)
	}
	if _, err := ops.Finish(id, &patch); err != nil {
		t.Fatalf("Finish(%s): %v", id, err)
	}
}

func recordFor(t *testing.T, ops *repos.DbOpsRepo, id string) model.DbOpRecord {
	t.Helper()
	recs, err := ops.Recent(1000)
	if err != nil {
		t.Fatalf("Recent: %v", err)
	}
	for _, r := range recs {
		if r.ID == id {
			return r
		}
	}
	t.Fatalf("no op_log row for %s", id)
	return model.DbOpRecord{}
}

func storedBytesFor(t *testing.T, ops *repos.DbOpsRepo, id string) int {
	t.Helper()
	var n int
	if err := ops.DB.QueryRow(`SELECT stored_bytes FROM db_op_log WHERE id = ?`, id).Scan(&n); err != nil {
		t.Fatalf("query stored_bytes(%s): %v", id, err)
	}
	return n
}

// seedRawOp inserts a terminal op_log row directly, bypassing Append/Finish's own caps —
// oplog/wire_test.go's own helper, reused here to seed a stale row for the retention-cut
// regression without needing a truthful command_truncated computation.
func seedRawOp(t *testing.T, ops *repos.DbOpsRepo, id, startedAt string) {
	t.Helper()
	if _, err := ops.DB.Exec(
		`INSERT INTO db_op_log (id, connection_id, tab_id, started_at, duration_ms, kind, status, rows, command, error)
		 VALUES (?, NULL, NULL, ?, NULL, 'read', 'ok', NULL, NULL, NULL)`,
		id, startedAt,
	); err != nil {
		t.Fatalf("seedRawOp(%s): %v", id, err)
	}
}

// ---- 1. D1(a): an oversized command is truncated to exactly the cap, flagged, and counted right ----

func TestOpsFinishTruncatesOversizedCommand(t *testing.T) {
	ops := newOpsRepo(t)
	oversized := strings.Repeat("x", 70_000) // > maxOpCommandBytes (64 KiB)

	appendAndFinish(t, ops, "op-1", kiratime.NowISO(), model.OpFinish{
		Status: "ok", DurationMs: 5, Command: &oversized,
	})

	rec := recordFor(t, ops, "op-1")
	if rec.Command == nil || len(*rec.Command) != 64*1024 {
		gotLen := -1
		if rec.Command != nil {
			gotLen = len(*rec.Command)
		}
		t.Fatalf("Command length = %d, want exactly 64 KiB (65536)", gotLen)
	}
	if !strings.HasPrefix(oversized, *rec.Command) {
		t.Fatalf("stored command is not a prefix of the original")
	}
	if !rec.CommandTruncated {
		t.Fatalf("CommandTruncated = false, want true")
	}
	if got := storedBytesFor(t, ops, "op-1"); got != 64*1024 {
		t.Fatalf("stored_bytes = %d, want %d (the truncated command's own byte length, no error)", got, 64*1024)
	}
}

// ---- 2. A command under the cap is stored verbatim, with the flag NOT spuriously set ----

func TestOpsFinishStoresShortCommandVerbatim(t *testing.T) {
	ops := newOpsRepo(t)
	short := "select 1"

	appendAndFinish(t, ops, "op-2", kiratime.NowISO(), model.OpFinish{
		Status: "ok", DurationMs: 1, Command: &short,
	})

	rec := recordFor(t, ops, "op-2")
	if rec.Command == nil || *rec.Command != short {
		t.Fatalf("Command = %v, want %q verbatim", rec.Command, short)
	}
	if rec.CommandTruncated {
		t.Fatalf("CommandTruncated = true for a command under the cap, want false")
	}
	if got := storedBytesFor(t, ops, "op-2"); got != len(short) {
		t.Fatalf("stored_bytes = %d, want %d", got, len(short))
	}
}

// ---- 3. D1(b): an oversized error is truncated, but carries no flag of its own ----

func TestOpsFinishTruncatesOversizedErrorWithNoFlag(t *testing.T) {
	ops := newOpsRepo(t)
	oversizedErr := strings.Repeat("e", 10_000) // > maxOpErrorBytes (8 KiB)

	appendAndFinish(t, ops, "op-3", kiratime.NowISO(), model.OpFinish{
		Status: "error", DurationMs: 1, Error: &oversizedErr,
	})

	rec := recordFor(t, ops, "op-3")
	if rec.Error == nil || len(*rec.Error) != 8*1024 {
		gotLen := -1
		if rec.Error != nil {
			gotLen = len(*rec.Error)
		}
		t.Fatalf("Error length = %d, want exactly 8 KiB (8192)", gotLen)
	}
	if rec.CommandTruncated {
		t.Fatalf("CommandTruncated = true for a truncated error, want false — error truncation carries no flag")
	}
}

// ---- 4. D2: the table-wide byte budget evicts oldest-first, keeps the newest, never empties ----

func TestOpsPruneByteBudgetEvictsOldestAcrossTable(t *testing.T) {
	ops := newOpsRepo(t)
	base := time.Now()
	at := func(hoursAgo int) string { return kiratime.FormatISO(base.Add(-time.Duration(hoursAgo) * time.Hour)) }

	// old-1/old-2 are deliberately padded so they are unmistakably the two biggest rows in the
	// table; mid-1/mid-2 are ordinary small commands. Oldest-to-newest by started_at.
	oldPad := strings.Repeat("z", 40_000)
	appendAndFinish(t, ops, "old-1", at(4), model.OpFinish{Status: "ok", Command: strPtr("old-1-" + oldPad)})
	appendAndFinish(t, ops, "old-2", at(3), model.OpFinish{Status: "ok", Command: strPtr("old-2-" + oldPad)})
	appendAndFinish(t, ops, "mid-1", at(2), model.OpFinish{Status: "ok", Command: strPtr("mid-1")})
	appendAndFinish(t, ops, "mid-2", at(1), model.OpFinish{Status: "ok", Command: strPtr("mid-2")})

	midBytes := storedBytesFor(t, ops, "mid-1") + storedBytesFor(t, ops, "mid-2")

	// Shrink the budget to "both mid-* rows, plus generous headroom for one more small row" —
	// comfortably more than "newest" needs and comfortably less than either padded old-* row costs
	// on its own (response_history_test.go's own "shrink the budget for the test" shape, §6.2).
	repos.SetOpLogByteBudgetForTest(t, midBytes+600)

	appendAndFinish(t, ops, "newest", at(0), model.OpFinish{Status: "ok", Command: strPtr("newest")})

	// retentionDays huge and the row count (5) is far under hardCapRows, so only the byte-budget
	// pass under test can fire.
	if err := ops.Prune(3650); err != nil {
		t.Fatalf("Prune: %v", err)
	}

	recs, err := ops.Recent(1000)
	if err != nil {
		t.Fatalf("Recent: %v", err)
	}
	survivors := map[string]bool{}
	for _, r := range recs {
		survivors[r.ID] = true
	}
	if survivors["old-1"] || survivors["old-2"] {
		t.Errorf("old-1/old-2 survived the budget sweep, want both evicted (survivors: %v)", survivors)
	}
	if !survivors["mid-1"] || !survivors["mid-2"] {
		t.Errorf("mid-1/mid-2 did not both survive, want both kept (survivors: %v)", survivors)
	}
	if !survivors["newest"] {
		t.Fatal("the just-finished row was itself evicted — the per-entry cap invariant (D1) is broken")
	}
	if len(recs) == 0 {
		t.Fatal("Prune emptied the whole table — the byte-budget sweep's safety invariant is broken")
	}
}

func strPtr(s string) *string { return &s }

// ---- 5a. Regression: the pre-existing retention cut still behaves exactly as it does today ----

func TestOpsPruneRetentionCutStillWorks(t *testing.T) {
	ops := newOpsRepo(t)
	staleAt := kiratime.FormatISO(time.Now().Add(-48 * time.Hour))
	freshAt := kiratime.NowISO()
	seedRawOp(t, ops, "stale", staleAt)
	seedRawOp(t, ops, "fresh", freshAt)

	if err := ops.Prune(1); err != nil {
		t.Fatalf("Prune: %v", err)
	}

	recs, err := ops.Recent(1000)
	if err != nil {
		t.Fatalf("Recent: %v", err)
	}
	if len(recs) != 1 || recs[0].ID != "fresh" {
		t.Fatalf("Recent() after Prune(1) = %+v, want exactly [fresh]", recs)
	}
}

// ---- 5b. Regression: the pre-existing hard row cap still behaves exactly as it does today ----

func TestOpsPruneHardCapRowsStillWorks(t *testing.T) {
	ops := newOpsRepo(t)

	// 20,001 raw rows, one over hardCapRows (20,000, unexported — mirrored here as a literal the
	// same way response_history_test.go mirrors historyPerScopeLimit rather than exporting it).
	// One transaction, one prepared statement: real rows, not a shortcut around what Prune scans.
	tx, err := ops.DB.Begin()
	if err != nil {
		t.Fatalf("begin seed tx: %v", err)
	}
	stmt, err := tx.Prepare(`INSERT INTO db_op_log (id, connection_id, tab_id, started_at, duration_ms, kind, status, rows, command, error)
		VALUES (?, NULL, NULL, ?, NULL, 'read', 'ok', NULL, NULL, NULL)`)
	if err != nil {
		t.Fatalf("prepare seed insert: %v", err)
	}
	base := time.Now().Add(-24 * time.Hour) // well inside the huge retention window Prune is called with below
	const total = 20_001
	for i := 0; i < total; i++ {
		id := fmt.Sprintf("op-%05d", i)
		startedAt := kiratime.FormatISO(base.Add(time.Duration(i) * time.Millisecond))
		if _, err := stmt.Exec(id, startedAt); err != nil {
			t.Fatalf("seed row %d: %v", i, err)
		}
	}
	if err := stmt.Close(); err != nil {
		t.Fatalf("close seed stmt: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit seed tx: %v", err)
	}

	if err := ops.Prune(3650); err != nil {
		t.Fatalf("Prune: %v", err)
	}

	var count int
	if err := ops.DB.QueryRow(`SELECT COUNT(*) FROM db_op_log`).Scan(&count); err != nil {
		t.Fatalf("count op_log: %v", err)
	}
	if count != 20_000 {
		t.Fatalf("op_log row count after Prune = %d, want exactly 20,000 (hardCapRows)", count)
	}

	// The oldest row (op-00000) is the one that must have been evicted, not an arbitrary one.
	var exists int
	err = ops.DB.QueryRow(`SELECT 1 FROM db_op_log WHERE id = 'op-00000'`).Scan(&exists)
	if err == nil {
		t.Fatal("op-00000 (the oldest seeded row) survived the hard cap, want it evicted")
	}
	var stillExists int
	if err := ops.DB.QueryRow(`SELECT 1 FROM db_op_log WHERE id = 'op-20000'`).Scan(&stillExists); err != nil {
		t.Fatalf("op-20000 (the newest seeded row) did not survive the hard cap: %v", err)
	}
}

// TestReconcileInterruptedFlipsRunningRowsToError: a hard kill (SIGKILL, OOM,
// a panic outside Host.safeRun) skips oplog.Wiring's own finishInFlight entirely — it only runs on
// an orderly channel close — leaving 'running' op_log rows that would otherwise persist forever.
// ReconcileInterrupted, called at the next startup, must flip every one of them to 'error' with a
// clear message, and must leave an already-terminal row alone.
func TestReconcileInterruptedFlipsRunningRowsToError(t *testing.T) {
	ops := newOpsRepo(t)

	// A genuinely still-'running' row — Append is the real path that leaves one in that state.
	if err := ops.Append(model.OpAppend{ID: "op-running", Kind: "read", StartedAt: kiratime.NowISO()}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	// A second one, to confirm the UPDATE is not somehow limited to a single row.
	if err := ops.Append(model.OpAppend{ID: "op-running-2", Kind: "read", StartedAt: kiratime.NowISO()}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	// An already-finished row must be left alone.
	appendAndFinish(t, ops, "op-already-done", kiratime.NowISO(), model.OpFinish{Status: "ok", DurationMs: 5})

	n, err := ops.ReconcileInterrupted()
	if err != nil {
		t.Fatalf("ReconcileInterrupted: %v", err)
	}
	if n != 2 {
		t.Errorf("ReconcileInterrupted returned %d, want 2", n)
	}

	for _, id := range []string{"op-running", "op-running-2"} {
		rec := recordFor(t, ops, id)
		if rec.Status != "error" {
			t.Errorf("%s: Status = %q, want %q", id, rec.Status, "error")
		}
		if rec.Error == nil || *rec.Error != repos.InterruptedOpError {
			t.Errorf("%s: Error = %v, want %q", id, rec.Error, repos.InterruptedOpError)
		}
	}

	done := recordFor(t, ops, "op-already-done")
	if done.Status != "ok" {
		t.Errorf("op-already-done: Status = %q, want unchanged %q", done.Status, "ok")
	}
	if done.Error != nil {
		t.Errorf("op-already-done: Error = %v, want unchanged nil", done.Error)
	}

	// Calling it again with nothing left running must be a true no-op.
	n2, err := ops.ReconcileInterrupted()
	if err != nil {
		t.Fatalf("ReconcileInterrupted (second call): %v", err)
	}
	if n2 != 0 {
		t.Errorf("ReconcileInterrupted (second call) returned %d, want 0", n2)
	}
}
