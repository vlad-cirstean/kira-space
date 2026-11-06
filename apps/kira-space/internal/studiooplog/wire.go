// Package oplog is the Go analogue of src/main/oplog.ts: pure orchestration over
// internal/storage/repos.OpsRepo's Append/Finish/Prune — subscribing to op:start/op:end events,
// forwarding an update for the renderer's op-log panel, and reconciling whatever was still running
// when the event source's channel closes (app shutdown).
package studiooplog

import (
	"encoding/json"
	"github.com/kirathecat/kira-studio/internal/kiratime"
	"log/slog"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/repos"
	"github.com/kirathecat/kira-studio/internal/notify"
)

// Event is the op-log's own event shape. EventSource is a consumer-declared interface, so its
// payload type belongs with the consumer, here, rather than with whichever producer happens to
// publish it first — enginehost.Event was that producer's own type, borrowed only because oplog
// was its sole consumer.
type Event struct {
	Topic   string
	Payload json.RawMessage
}

// Event topics this package's consume loop switches on, mirroring ENGINE_EVENT.opStart/opEnd
// (packages/shared/protocol/engine-ops.ts:21-25). There is no EventEngineDown any more:
// adapterhost.Host, the only producer left, never published one — that was enginehost.Host's own
// synthetic topic for a Node child exiting — and reconciliation now runs whenever the event
// channel closes for any reason, orderly shutdown included, rather than waiting for a topic that
// nothing sends.
const (
	EventOpStart = "op:start"
	EventOpEnd   = "op:end"
)

// EventSource is the slice of a producer oplog consumes — adapterhost.Host in production, a fake
// channel in a test. A one-method interface lets the reconciliation logic — this package's actual
// subject — be driven by synthetic events deterministically.
type EventSource interface {
	Subscribe() (<-chan Event, func())
}

// pruneEveryOps mirrors oplog.ts's PRUNE_EVERY_OPS (D11: bounds the table at HARD_CAP_ROWS +
// PRUNE_EVERY_OPS instead of only at the next launch).
const pruneEveryOps = 500

// opEndStatuses is opEndEventSchema's status enum — narrower than model.ValidOpStatus (which
// also accepts "running", a status op:end itself is never sent with).
var opEndStatuses = map[string]bool{"ok": true, "error": true, "cancelled": true}

var appExitedError = "app exited"

// inFlightOp is oplog.ts's InFlightOp.
type inFlightOp struct {
	connectionID *string
	tabID        *string
	kind         string
	startedAt    string
	// incognito is P71 §4.3: true for an op started with OpSpec.Incognito — handleOpStart skips
	// Append for it, handleOpEnd skips Finish, and finishInFlight skips it on shutdown. The live
	// Operations-panel update (updates.Emit) still fires in all three cases.
	incognito bool
	// delayMs is P307 Part 2: a preconnect op's configured wait (0 = none), carried onto its records.
	delayMs int
	// source is P328: "user" or "mcp", carried onto every record of the op.
	source string
	// appendFailed is set when Append failed (a reused op id): handleOpEnd and finishInFlight skip Finish,
	// so the earlier op's row cannot be overwritten. The live record still shows.
	appendFailed bool
}

// Wiring is the Go analogue of oplog.ts's wireOplog, split into New/Start/Stop (P54 D14): New
// does no I/O and subscribes nothing, so every test can attach OnUpdate before the first event.
type Wiring struct {
	src           EventSource
	ops           *repos.OpsRepo
	retentionDays int

	updates     notify.Emitter[model.OpRecord]
	unsubscribe func()
	done        chan struct{}
}

// stopWait bounds how long Stop blocks for consume to drain and run finishInFlight before giving
// up — a wedged consumer (P21 round 3 finding 3) must not hang app quit forever, but the ordinary
// case (a handful of finishInFlight writes) finishes in microseconds.
const stopWait = 2 * time.Second

func New(src EventSource, ops *repos.OpsRepo, retentionDays int) *Wiring {
	return &Wiring{src: src, ops: ops, retentionDays: retentionDays, done: make(chan struct{})}
}

// OnUpdate registers fn for every op-log row lifecycle event (running, then its terminal state).
func (w *Wiring) OnUpdate(fn func(model.OpRecord)) (unsubscribe func()) {
	return w.updates.Subscribe(fn)
}

// Start reconciles whatever op_log left 'running' after a hard kill (consume's own finishInFlight
// only runs on an orderly channel close, never after SIGKILL/OOM/a panic), prunes once
// (oplog.ts:28), and then consumes events on one goroutine until Stop is called or the event
// source's channel closes on its own (adapterhost.Host's, once app teardown unsubscribes every
// subscriber). Reconciliation runs first, before anything else in this app
// touches op_log, so a restart after a crash never hydrates the Operations panel with a phantom
// still-running op.
func (w *Wiring) Start() {
	if n, err := w.ops.ReconcileInterrupted(); err != nil {
		slog.Warn("reconcile interrupted ops failed", "scope", "oplog", "err", err)
	} else if n > 0 {
		slog.Info("reconciled op_log rows left running by an unclean exit", "scope", "oplog", "count", n)
	}
	w.prune()
	events, unsubscribe := w.src.Subscribe()
	w.unsubscribe = unsubscribe
	go w.consume(events)
}

// Stop ends the consumer goroutine (main's before-quit; a test's own cleanup) and waits for it to
// finish before returning, up to stopWait. Without this,
// unsubscribe only closes the event channel; consume still has to drain, exit its range loop and
// run finishInFlight (one repos.OpsRepo.Finish write per still-running op), and nothing previously
// synchronised that with the caller. main.go's teardown calls repositories.Close()/db.Close()
// immediately after Stop(), so a race here meant "app exited" rows for in-flight ops were either
// never written (permanent phantom "running" rows in the Operations panel) or attempted against an
// already-closed statement/database, depending on the race's outcome. Bounded rather than
// unconditional so a wedged consumer (should the OpsRepo call itself somehow block) cannot hang
// quit forever — Ticker.Stop's shape is the same idea without the timeout, which it can afford
// because it has no I/O in its stop path.
//
// Idempotent only in the sense that the event source's own unsubscribe already tolerates a second
// call — Wiring itself is only ever Stopped once in practice.
func (w *Wiring) Stop() {
	if w.unsubscribe != nil {
		w.unsubscribe()
	}
	select {
	case <-w.done:
	case <-time.After(stopWait):
		slog.Warn("consumer did not finish within stopWait; shutdown finish for any still-running ops may race", "scope", "oplog")
	}
}

func (w *Wiring) prune() {
	if err := w.ops.Prune(w.retentionDays); err != nil {
		slog.Warn("prune failed", "scope", "oplog", "err", err)
	}
}

// consume is the only reader and writer of inFlight, so that map needs no mutex — nobody should
// add one. The `for evt := range events` loop ends when the event source's channel closes — Stop's
// unsubscribe (main's before-quit) causes that in production — at which point finishInFlight
// reconciles whatever never reached a terminal state.
func (w *Wiring) consume(events <-chan Event) {
	defer close(w.done)

	inFlight := map[string]inFlightOp{}
	completedSincePrune := 0

	for evt := range events {
		switch evt.Topic {
		case EventOpStart:
			w.handleOpStart(evt.Payload, inFlight)
		case EventOpEnd:
			completedSincePrune = w.handleOpEnd(evt.Payload, inFlight, completedSincePrune)
		}
	}
	w.finishInFlight(inFlight, appExitedError)
}

func (w *Wiring) handleOpStart(payload json.RawMessage, inFlight map[string]inFlightOp) {
	var evt struct {
		OpID         string  `json:"opId"`
		ConnectionID *string `json:"connectionId"`
		TabID        *string `json:"tabId"`
		Kind         string  `json:"kind"`
		StartedAt    string  `json:"startedAt"`
		Incognito    bool    `json:"incognito"`
		DelayMs      int     `json:"delayMs"`
		Source       string  `json:"source"`
	}
	if err := json.Unmarshal(payload, &evt); err != nil || !model.ValidOpKind(evt.Kind) {
		return
	}
	if !model.ValidOpSource(evt.Source) {
		evt.Source = "user"
	}

	rec := inFlightOp{
		connectionID: evt.ConnectionID, tabID: evt.TabID, kind: evt.Kind, startedAt: evt.StartedAt,
		incognito: evt.Incognito, delayMs: evt.DelayMs, source: evt.Source,
	}

	// P71 §4.3: an incognito op is never Appended — nothing about it reaches op_log — but the
	// live Operations-panel update below still fires, so a running incognito op is visible while
	// it runs and simply gone (never having existed on disk) once it completes.
	if !evt.Incognito {
		if err := w.ops.Append(model.OpAppend{
			ID: evt.OpID, ConnectionID: rec.connectionID, TabID: rec.tabID, Kind: rec.kind, StartedAt: rec.startedAt, Source: rec.source,
		}); err != nil {
			slog.Warn("append failed", "scope", "oplog", "opId", evt.OpID, "err", err)
			rec.appendFailed = true
		}
	}
	inFlight[evt.OpID] = rec

	w.updates.Emit(model.OpRecord{
		ID: evt.OpID, ConnectionID: rec.connectionID, TabID: rec.tabID, StartedAt: rec.startedAt,
		DurationMs: nil, Kind: rec.kind, Status: "running", Rows: nil, Command: nil, Error: nil,
		DelayMs: delayPtr(rec.delayMs), Source: rec.source,
	})
}

func delayPtr(ms int) *int {
	if ms <= 0 {
		return nil
	}
	return &ms
}

func (w *Wiring) handleOpEnd(payload json.RawMessage, inFlight map[string]inFlightOp, completedSincePrune int) int {
	var evt struct {
		OpID       string  `json:"opId"`
		Status     string  `json:"status"`
		DurationMs int     `json:"durationMs"`
		Rows       *int    `json:"rows"`
		Command    *string `json:"command"`
		Error      *string `json:"error"`
		Path       *string `json:"path"`
	}
	if err := json.Unmarshal(payload, &evt); err != nil || !opEndStatuses[evt.Status] {
		return completedSincePrune
	}

	patch := model.OpFinish{
		Status: evt.Status, DurationMs: evt.DurationMs, Rows: evt.Rows, Command: evt.Command, Error: evt.Error,
		Path: evt.Path,
	}

	// The inFlight lookup moves above Finish (oplog.ts:80-84's exact fallbacks for an
	// op:end with no matching op:start still apply below) so an incognito op's own skip decision
	// is known before deciding whether to write anything at all — nothing was ever Appended for
	// it (handleOpStart's own skip), so Finish would either no-op or fail against a missing row.
	started, ok := inFlight[evt.OpID]
	delete(inFlight, evt.OpID)

	var commandTruncated bool
	if ok && (started.incognito || started.appendFailed) {
		// patch.Command/patch.Error stay exactly as the event carried them — Finish is the only
		// thing that truncates them in place, and this op's live record is in-memory only, never
		// written to op_log, so there is nothing to truncate for.
	} else {
		// Finish truncates patch.Command/patch.Error in place, so the record built
		// below (and pushed live to the renderer) reflects exactly what was stored, never the
		// pre-truncation event payload — a live update must not disagree with a subsequent
		// Recent() reload about whether Re-run should be disabled.
		var err error
		commandTruncated, err = w.ops.Finish(evt.OpID, &patch)
		if err != nil {
			slog.Warn("finish failed", "scope", "oplog", "opId", evt.OpID, "err", err)
			return completedSincePrune
		}
	}

	record := model.OpRecord{
		ID: evt.OpID, DurationMs: &evt.DurationMs, Status: evt.Status, Rows: evt.Rows,
		Command: patch.Command, Error: patch.Error, CommandTruncated: commandTruncated, Path: patch.Path,
	}
	if ok {
		record.ConnectionID, record.TabID, record.StartedAt, record.Kind = started.connectionID, started.tabID, started.startedAt, started.kind
		record.DelayMs = delayPtr(started.delayMs)
		record.Source = started.source
	} else {
		record.StartedAt, record.Kind, record.Source = kiratime.NowISO(), "test", "user"
	}
	w.updates.Emit(record)

	completedSincePrune++
	if completedSincePrune >= pruneEveryOps {
		completedSincePrune = 0
		w.prune()
	}
	return completedSincePrune
}

// finishInFlight mirrors oplog.ts's engine:down handler, generalised from "the engine child died"
// to "the event source is done": whatever op-log rows were still 'running' when consume's
// range loop ended is finished with message, and dropped from inFlight so a crash mid-session
// cannot grow the map without bound. A hard kill (SIGKILL, OOM, a panic outside Host.safeRun) still
// leaves 'running' rows — this only runs on an orderly channel close.
func (w *Wiring) finishInFlight(inFlight map[string]inFlightOp, message string) {
	now := time.Now()
	for opID, rec := range inFlight {
		delete(inFlight, opID)
		// P71 §4.3: an incognito op still running at shutdown was never Appended, so there is no
		// row to Finish — skip it rather than writing a fresh "app exited" row for an op that
		// never reached op_log at all.
		if rec.incognito || rec.appendFailed {
			continue
		}

		durationMs := 0 // a startedAt that will not parse yields 0 — JS gives NaN there, which
		// is not a number SQLite should store, and 0 is the honest value for "we cannot tell".
		if started, err := kiratime.ParseISO(rec.startedAt); err == nil {
			if d := int(now.Sub(started).Milliseconds()); d > 0 {
				durationMs = d
			}
		}

		patch := model.OpFinish{Status: "error", DurationMs: durationMs, Rows: nil, Command: nil, Error: &message}
		if _, err := w.ops.Finish(opID, &patch); err != nil {
			slog.Warn("shutdown finish failed", "scope", "oplog", "opId", opID, "err", err)
		}
	}
}
