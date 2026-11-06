package studiooplog_test

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiooplog"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/repos"
	"github.com/kirathecat/kira-studio/internal/kiratime"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// fakeSource is an EventSource a test drives directly, without a real engine child.
type fakeSource struct {
	ch chan studiooplog.Event
}

func newFakeSource() *fakeSource {
	return &fakeSource{ch: make(chan studiooplog.Event, 1024)}
}

func (f *fakeSource) Subscribe() (<-chan studiooplog.Event, func()) {
	return f.ch, func() { close(f.ch) }
}

type harness struct {
	wiring *studiooplog.Wiring
	src    *fakeSource
	ops    *repos.OpsRepo
}

func newHarness(t *testing.T, retentionDays int) *harness {
	t.Helper()
	t.Setenv("KIRA_SPACE_HOME", t.TempDir())
	db, err := studiostorage.Open()
	if err != nil {
		t.Fatalf("storage.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	r, err := repos.New(db.DB)
	if err != nil {
		t.Fatalf("repos.New: %v", err)
	}
	t.Cleanup(func() { _ = r.Close() })

	src := newFakeSource()
	w := studiooplog.New(src, r.Ops, retentionDays)
	return &harness{wiring: w, src: src, ops: r.Ops}
}

func opStartPayload(opID string, connectionID, tabID *string, kind, startedAt string) json.RawMessage {
	b, _ := json.Marshal(map[string]any{
		"opId": opID, "connectionId": connectionID, "tabId": tabID, "kind": kind, "startedAt": startedAt,
	})
	return b
}

// opStartPayloadIncognito is opStartPayload's own P71 sibling — the one field distinguishing an
// incognito op's op:start event.
func opStartPayloadIncognito(opID, startedAt string) json.RawMessage {
	b, _ := json.Marshal(map[string]any{
		"opId": opID, "connectionId": nil, "tabId": nil, "kind": "http", "startedAt": startedAt, "incognito": true,
	})
	return b
}

func opEndPayload(opID, status string) json.RawMessage {
	b, _ := json.Marshal(map[string]any{
		"opId": opID, "status": status, "durationMs": 1, "rows": nil, "command": nil, "error": nil,
	})
	return b
}

func seedRawOp(t *testing.T, ops *repos.OpsRepo, id, startedAt string) {
	t.Helper()
	if _, err := ops.DB.Exec(
		`INSERT INTO op_log (id, connection_id, tab_id, started_at, duration_ms, kind, status, rows, command, error)
		 VALUES (?, NULL, NULL, ?, NULL, 'read', 'running', NULL, NULL, NULL)`,
		id, startedAt,
	); err != nil {
		t.Fatalf("seed op %s: %v", id, err)
	}
}

func rowExists(t *testing.T, ops *repos.OpsRepo, id string) bool {
	t.Helper()
	var exists int
	err := ops.DB.QueryRow(`SELECT 1 FROM op_log WHERE id = ?`, id).Scan(&exists)
	if err == sql.ErrNoRows {
		return false
	}
	if err != nil {
		t.Fatalf("query %s: %v", id, err)
	}
	return true
}

func fetchOp(t *testing.T, ops *repos.OpsRepo, id string) model.OpRecord {
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
	return model.OpRecord{}
}

// waitUntil is testx.WaitUntil (P107 I2-28).
var waitUntil = testx.WaitUntil

// updateCollector is an OnUpdate sink safe for concurrent use.
type updateCollector struct {
	mu   sync.Mutex
	recs []model.OpRecord
}

func (c *updateCollector) handle(r model.OpRecord) {
	c.mu.Lock()
	c.recs = append(c.recs, r)
	c.mu.Unlock()
}

func (c *updateCollector) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.recs)
}

// TestPruneRunsAtStartAndEvery500 pins the prune cadence's counter arithmetic: once at Start, then
// on exactly the 500th completed op and not again until the 1000th. An off-by-one here either
// prunes on every op (a full table scan per query) or never prunes at all after startup.
// consume's own finishInFlight
// (TestShutdownReconcilesInFlight, below) only ever runs on an orderly channel close — a hard kill
// (SIGKILL, OOM, a panic outside Host.safeRun) skips it entirely, leaving a 'running' row seeded by
// a *previous* process's Append with nothing in this process's own in-flight map to reconcile it.
// Start must flip that leftover row to 'error' via OpsRepo.ReconcileInterrupted before it ever
// subscribes, so the very next launch after a crash never hydrates the Operations panel with a
// phantom still-running op.
func TestStartReconcilesInterruptedRunningRows(t *testing.T) {
	h := newHarness(t, 30)
	seedRawOp(t, h.ops, "left-running-by-a-crash", kiratime.NowISO())

	h.wiring.Start()
	t.Cleanup(h.wiring.Stop)

	waitUntil(t, time.Second, func() bool { return fetchOp(t, h.ops, "left-running-by-a-crash").Status == "error" })
	row := fetchOp(t, h.ops, "left-running-by-a-crash")
	if row.Error == nil || *row.Error != repos.InterruptedOpError {
		t.Errorf("Error = %v, want %q", row.Error, repos.InterruptedOpError)
	}
}

func TestPruneRunsAtStartAndEvery500(t *testing.T) {
	h := newHarness(t, 1)
	staleAt := kiratime.FormatISO(time.Now().Add(-48 * time.Hour))
	seedRawOp(t, h.ops, "stale-at-start", staleAt)

	h.wiring.Start()
	t.Cleanup(h.wiring.Stop)
	waitUntil(t, time.Second, func() bool { return !rowExists(t, h.ops, "stale-at-start") })

	var updates updateCollector
	h.wiring.OnUpdate(updates.handle)

	seedRawOp(t, h.ops, "stale-at-500", staleAt)
	for i := 0; i < 500; i++ {
		h.src.ch <- studiooplog.Event{Topic: studiooplog.EventOpEnd, Payload: opEndPayload(fmt.Sprintf("op%d", i), "ok")}
	}
	waitUntil(t, 2*time.Second, func() bool { return updates.count() == 500 })
	waitUntil(t, time.Second, func() bool { return !rowExists(t, h.ops, "stale-at-500") })

	seedRawOp(t, h.ops, "stale-at-501", staleAt)
	h.src.ch <- studiooplog.Event{Topic: studiooplog.EventOpEnd, Payload: opEndPayload("op500", "ok")}
	waitUntil(t, 2*time.Second, func() bool { return updates.count() == 501 })
	time.Sleep(100 * time.Millisecond) // give a hypothetical (buggy) third prune a moment to run
	if !rowExists(t, h.ops, "stale-at-501") {
		t.Errorf("a prune ran at op 501, want the next one only at op 1000")
	}
}

// TestShutdownReconcilesInFlight covers this package's actual subject: everything still tracked as
// running when the event channel closes must be finished with the shutdown error and dropped from
// the in-flight map, while an op that already reached a terminal state is left alone. Closing the
// channel is what Stop (main's before-quit, via fakeSource's unsubscribe) does in production too —
// there is no separate synthetic "engine died" event to send any more.
func TestShutdownReconcilesInFlight(t *testing.T) {
	h := newHarness(t, 30)
	var updates updateCollector
	h.wiring.OnUpdate(updates.handle)
	h.wiring.Start()

	startedAt := kiratime.NowISO()
	for _, id := range []string{"op1", "op2", "op3"} {
		h.src.ch <- studiooplog.Event{Topic: studiooplog.EventOpStart, Payload: opStartPayload(id, nil, nil, "read", startedAt)}
	}
	h.src.ch <- studiooplog.Event{Topic: studiooplog.EventOpStart, Payload: opStartPayload("op4", nil, nil, "read", startedAt)}
	h.src.ch <- studiooplog.Event{Topic: studiooplog.EventOpEnd, Payload: opEndPayload("op4", "ok")}
	waitUntil(t, time.Second, func() bool { return rowExists(t, h.ops, "op4") && fetchOp(t, h.ops, "op4").Status == "ok" })

	// P71 §4.3: an incognito op still running at shutdown was never Appended (handleOpStart's own
	// skip) — finishInFlight must skip it too, rather than writing a fresh "app exited" row for an
	// op that never reached op_log at all.
	h.src.ch <- studiooplog.Event{Topic: studiooplog.EventOpStart, Payload: opStartPayloadIncognito("op-incog-shutdown", startedAt)}
	waitUntil(t, time.Second, func() bool { return updates.count() >= 6 })

	// Stop's unsubscribe closes the channel; consume's range loop returns and finishInFlight runs.
	// Must not panic or hang.
	h.wiring.Stop()

	if rowExists(t, h.ops, "op-incog-shutdown") {
		t.Errorf("an incognito op left running at shutdown was persisted")
	}

	for _, id := range []string{"op1", "op2", "op3"} {
		waitUntil(t, time.Second, func() bool { return fetchOp(t, h.ops, id).Status == "error" })
		row := fetchOp(t, h.ops, id)
		if row.Error == nil || *row.Error != "app exited" {
			t.Errorf("%s.Error = %v, want \"app exited\"", id, row.Error)
		}
		if row.DurationMs == nil || *row.DurationMs < 0 {
			t.Errorf("%s.DurationMs = %v, want a non-negative value", id, row.DurationMs)
		}
	}
	if row := fetchOp(t, h.ops, "op4"); row.Status != "ok" {
		t.Errorf("op4 (already finished before shutdown) status = %q, want untouched ok", row.Status)
	}
}

// TestIncognitoOpSkipsPersistence is the guard on handleOpEnd's reordering: an op
// started with incognito must reach neither Append (handleOpStart) nor Finish (handleOpEnd) — the
// live Operations-panel view (updates.Emit) still fires for both, so a running incognito op stays
// visible while it runs and never existed in op_log once it completes.
func TestIncognitoOpSkipsPersistence(t *testing.T) {
	h := newHarness(t, 30)
	var updates updateCollector
	h.wiring.OnUpdate(updates.handle)
	h.wiring.Start()
	t.Cleanup(h.wiring.Stop)

	startedAt := kiratime.NowISO()
	h.src.ch <- studiooplog.Event{Topic: studiooplog.EventOpStart, Payload: opStartPayloadIncognito("op-incog", startedAt)}
	waitUntil(t, time.Second, func() bool { return updates.count() == 1 })
	if rowExists(t, h.ops, "op-incog") {
		t.Fatalf("op:start with incognito=true wrote a row")
	}

	h.src.ch <- studiooplog.Event{Topic: studiooplog.EventOpEnd, Payload: opEndPayload("op-incog", "ok")}
	waitUntil(t, time.Second, func() bool { return updates.count() == 2 })
	if rowExists(t, h.ops, "op-incog") {
		t.Fatalf("op:end for an incognito op wrote a row")
	}
}

// TestStopWaitsForConsumerBeforeReturning: Stop() must not return until
// the consumer goroutine has run finishInFlight, because main.go's teardown calls
// repositories.Close()/db.Close() immediately after Stop() — if Stop only unsubscribed (closing
// the channel) without waiting for consume to drain, finishInFlight's writes would race the very
// next statements in teardown, and "app exited" rows for still-running ops would be lost (or hit
// "sql: statement is closed") depending on which side of the race won. Unlike
// TestShutdownReconcilesInFlight (which uses a generous waitUntil grace period that would hide
// exactly this race), this test asserts the reconciled state is already correct the instant Stop()
// returns — no polling.
func TestStopWaitsForConsumerBeforeReturning(t *testing.T) {
	h := newHarness(t, 30)
	h.wiring.Start()

	startedAt := kiratime.NowISO()
	for _, id := range []string{"op1", "op2", "op3"} {
		h.src.ch <- studiooplog.Event{Topic: studiooplog.EventOpStart, Payload: opStartPayload(id, nil, nil, "read", startedAt)}
	}
	// No synchronisation before Stop other than a short, bounded wait for the Append writes to
	// land — Stop's own job is to wait out whatever is still in flight beyond that.
	for _, id := range []string{"op1", "op2", "op3"} {
		waitUntil(t, time.Second, func() bool { return rowExists(t, h.ops, id) })
	}

	h.wiring.Stop()

	// Immediately after Stop returns — simulating main.go's teardown, which closes the repo and
	// the database right after oplogWiring.Stop() — every still-running op must already be
	// reconciled. A pre-fix Stop (unsubscribe only, no wait) fails this non-deterministically but
	// reliably enough to catch in one run: the consumer has not yet had a chance to run
	// finishInFlight when Stop returns.
	for _, id := range []string{"op1", "op2", "op3"} {
		row := fetchOp(t, h.ops, id)
		if row.Status != "error" {
			t.Fatalf("%s.Status immediately after Stop() = %q, want \"error\" (finishInFlight must have already run)", id, row.Status)
		}
		if row.Error == nil || *row.Error != "app exited" {
			t.Errorf("%s.Error = %v, want \"app exited\"", id, row.Error)
		}
	}

	// Mirrors main.go's teardown ordering: closing the repo/db right after Stop() must not race a
	// still-running finishInFlight write. If Stop returned early, this Close could interleave with
	// finishInFlight's tx.Exec calls (the storage layer runs on a single *sql.DB with
	// MaxOpenConns(1), so the two would be strictly ordered by the driver — but a premature Close
	// would still surface as "sql: statement is closed" in finishInFlight's own Warn log, which
	// TestShutdownReconcilesInFlight's grace period would never observe in time).
	if err := h.ops.DB.Close(); err != nil {
		t.Fatalf("close ops db: %v", err)
	}
}
