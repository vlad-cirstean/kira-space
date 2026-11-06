// Package adapterhost is the Go analogue of
// src/engine/{scheduler/ops.ts,control.ts,rpc.ts,data.ts}: Router serves every connection kind
// in-process against apps/kira-space/internal/adapters (the adapter contract) — there is no more
// routing decision to make, since the Node engine child this package used to forward non-native
// kinds to was deleted.
package adapterhost

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/kirathecat/kira-studio/internal/ids"
	"github.com/kirathecat/kira-studio/internal/kiratime"
	"runtime/debug"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/enginecache"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiooplog"
	"github.com/kirathecat/kira-studio/internal/notify"
)

// OpSpec is the Go analogue of scheduler/ops.ts's runOp spec parameter. ConnectionID is nil for a
// connection-less op (adapter:test probes a throwaway adapter, never a live one). OpID empty mints
// a fresh id, matching runOp's `spec.opId ?? newUuid()`.
type OpSpec struct {
	ConnectionID *string
	Kind         string // a model.OpKind value
	OpID         string
	TabID        *string
	// Incognito is P71 §4.3's own addition: forwarded onto op:start so studiooplog.Wiring can skip
	// persisting this op at all (Append/Finish) while still emitting the live Operations-panel
	// update — false for every caller but HttpService.Send/GrpcService.Call.
	Incognito bool
	// DelayMs is preconnect only: the configured wait, so the panel can count it down.
	DelayMs int
	// Adapter is the adapter running the op when it is not the connection's live one (a read
	// lane); Cancel then reaches that adapter instead.
	Adapter adapters.Adapter
	// Timeout is the statement timeout (dbmcp, Go-only): zero means none. The host stops the op
	// through the Stop path once Timeout+TimeoutGrace of run time has passed. TimeoutGrace is
	// nonzero when the engine enforces the limit too, so its own typed error wins the race.
	Timeout, TimeoutGrace time.Duration
}

// statementTimeoutGrace is how long the host backstop waits past an engine-enforced limit.
const statementTimeoutGrace = 2 * time.Second

type runningOp struct {
	cancel       context.CancelFunc
	connectionID *string
	kind         string
	source       string
	startedAt    string
	adapter      adapters.Adapter
	// listed is set once op:start is emitted: only then does the op have a panel row.
	listed bool
}

// RunningOp is one started op, as RunningOps lists it.
type RunningOp struct {
	OpID         string
	Kind         string
	Source       string
	StartedAt    string
	ConnectionID *string
}

// eventSub is the lossless delivery queue for exactly one Subscribe()r. Delivery used to be a
// non-blocking send into a fixed 32-slot buffer, dropping an event outright once it filled — on
// the stated reasoning that "dropping one event is better than blocking every other subscriber".
// oplog is the only production subscriber (main.go's `studiooplog.New(router.Host(), ...)` is the only
// Host.Subscribe call site; internal/bridge's http.go/grpc.go only ever call RunOp, never
// Subscribe), and it does a synchronous SQLite write plus a Wails emit per event — up to 64
// concurrent ops (plus tree/dbmcp/http/grpc ops) can easily outpace that, so the stated reasoning
// does not hold: there is no *other* subscriber a drop is protecting. A dropped op:end
// leaves an op_log row stuck "running" forever; a dropped op:start produces a phantom
// "test"-kind record.
//
// deliver now only ever appends to an unbounded queue (guarded by mu) and wakes drain — it never
// blocks and never drops. drain is the one dedicated goroutine that ever reads the queue or sends
// on ch, so the class of bug the previous version's own mutex existed to prevent (Emit calling a
// callback still in flight after Unsubscribe returns — notify.Emitter[T].Emit's own doc comment)
// can no longer race a send against a close at all: deliver never touches ch, and close only ever
// asks drain to finish its own queue and close ch itself, once, from that single goroutine.
type eventSub struct {
	ch   chan studiooplog.Event // consumer-facing; only drain ever sends on or closes this
	wake chan struct{}          // 1-buffered: wakes drain when the queue goes non-empty or closes

	mu     sync.Mutex
	queue  []studiooplog.Event
	closed bool
}

func newEventSub() *eventSub {
	s := &eventSub{ch: make(chan studiooplog.Event), wake: make(chan struct{}, 1)}
	go s.drain()
	return s
}

func (s *eventSub) deliver(e studiooplog.Event) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.queue = append(s.queue, e)
	s.mu.Unlock()
	s.signal()
}

func (s *eventSub) signal() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// drain pops and sends one event at a time, blocking on ch<-e for as long as the consumer takes —
// exactly what makes delivery lossless: nothing here ever drops an event to avoid blocking,
// because nothing else is waiting on this subscriber's own queue while it does. Once close has
// been called, drain finishes whatever is still queued before closing ch, so an event accepted
// before Stop was called is never lost at shutdown either.
func (s *eventSub) drain() {
	for {
		s.mu.Lock()
		if len(s.queue) == 0 {
			closed := s.closed
			s.mu.Unlock()
			if closed {
				close(s.ch)
				return
			}
			<-s.wake
			continue
		}
		e := s.queue[0]
		s.queue = s.queue[1:]
		s.mu.Unlock()
		s.ch <- e
	}
}

func (s *eventSub) close() {
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
	s.signal()
}

// Host is the Go analogue of scheduler/ops.ts's module-level running map plus control.ts's
// handler bodies for the five adapter:* ops (§4.8's RunOp) — the scheduler and the panic boundary,
// not the per-kind routing decision (router.go).
type Host struct {
	deps  adapters.Deps
	cache *enginecache.Cache

	events notify.Emitter[studiooplog.Event]

	mu      sync.Mutex
	running map[string]runningOp
	// earlyCancels remembers an op id whose CancelOp arrived before RunOp registered it (the
	// renderer sends Run and Stop as separate bound calls), keyed to its expiry.
	earlyCancels map[string]time.Time
	earlyTTL     time.Duration

	// throttleWait bounds one op's wait for its connection's slot and token (throttleMaxWait).
	throttleWait time.Duration
	throttles    *throttleRegistry
}

const (
	earlyCancelTTL = 5 * time.Second
	// maxEarlyCancels bounds earlyCancels so a misbehaving renderer cannot grow it.
	maxEarlyCancels = 1024
)

// NewHost constructs a Host. cache is used by the native Connect/Disconnect handlers
// (disconnecting releases the connection's cached pages — see router.go).
//
// deps.Log is a bare func field with no constructor enforcing it
// (adapters.Deps has none), and this package treated it as optional in some call sites
// (`if h.deps.Log != nil`, dataframe.go's panic recovery) and mandatory in others
// (dataframe.go's response-path logging) — latent in production (main.go always sets one) but a
// nil-func panic waiting for a test harness or a future caller that doesn't. Normalised once here,
// the same way enginecache.NewCache substitutes a no-op for a nil log, so every ad-hoc nil check
// downstream can be — and has been — dropped.
func NewHost(deps adapters.Deps, cache *enginecache.Cache) *Host {
	deps = withDefaultLog(deps)
	return &Host{deps: deps, cache: cache, running: make(map[string]runningOp),
		earlyCancels: make(map[string]time.Time), earlyTTL: earlyCancelTTL, throttleWait: throttleMaxWait, throttles: newThrottleRegistry()}
}

// withDefaultLog substitutes a no-op for a nil deps.Log, so nothing downstream needs to nil-check
// it again.
func withDefaultLog(deps adapters.Deps) adapters.Deps {
	if deps.Log == nil {
		deps.Log = func(string, string) {}
	}
	return deps
}

// SetThrottle installs (perSec > 0) or clears (perSec <= 0) connectionID's command rate limit —
// P28 §5.5's plumbing target, called by Router.SetThrottle.
func (h *Host) SetThrottle(connectionID string, perSec float64) {
	h.throttles.set(connectionID, perSec)
}

type opStartPayload struct {
	OpID         string  `json:"opId"`
	ConnectionID *string `json:"connectionId"`
	TabID        *string `json:"tabId"`
	Kind         string  `json:"kind"`
	StartedAt    string  `json:"startedAt"`
	Incognito    bool    `json:"incognito"`
	DelayMs      int     `json:"delayMs,omitempty"`
	Source       string  `json:"source"`
}

type opEndPayload struct {
	OpID       string  `json:"opId"`
	Status     string  `json:"status"`
	DurationMs int     `json:"durationMs"`
	Rows       *int    `json:"rows"`
	Command    *string `json:"command"`
	Error      *string `json:"error"`
	// Path (P108 Part 11 F5): see model.DbOpRecord.Path.
	Path *string `json:"path"`
}

// RunOp is scheduler/ops.ts's runOp: mint or accept an op id, refuse a duplicate (a duplicate id
// would corrupt the op log's primary key and let the stop button cancel the wrong query), derive a
// cancellable context, emit op:start, run fn behind a recover(), emit op:end with a
// status/duration/rows/command/error decided the same way runOp decides them, and remove the op
// from the running map in a defer. ctx is the caller's own context (e.g. app shutdown); CancelOp is
// the explicit, opID-addressed cancellation path — both cancel the same derived context.
func (h *Host) RunOp(ctx context.Context, spec OpSpec, fn func(context.Context, *adapters.OpCtx) (any, error)) (string, any, error) {
	opID := spec.OpID
	if opID == "" {
		opID = ids.NewID()
	} else if !ids.IsUUID(opID) {
		return "", nil, errInvalidOpID()
	}

	h.mu.Lock()
	if _, exists := h.running[opID]; exists {
		h.mu.Unlock()
		return "", nil, adapters.New(adapters.CodeQuery, "duplicate operation id: "+opID, nil)
	}
	if expiry, ok := h.earlyCancels[opID]; ok {
		delete(h.earlyCancels, opID)
		if time.Now().Before(expiry) {
			h.mu.Unlock()
			return "", nil, adapters.T(adapters.CodeCancelled, "backend.studio.adapter.cancelledBeforeStart.body", nil, nil)
		}
	}
	derived, cancel := context.WithCancel(ctx)
	h.running[opID] = runningOp{cancel: cancel, connectionID: spec.ConnectionID, kind: spec.Kind, source: OpSourceOf(ctx), adapter: spec.Adapter}
	h.mu.Unlock()
	defer func() {
		h.mu.Lock()
		delete(h.running, opID)
		h.mu.Unlock()
		cancel()
	}()

	// The throttle gate sits here — after registration (so a queued op is cancellable
	// via CancelOp/the deferred cancel() above) but before op:start (so startedAt/DurationMs stay
	// the adapter's own numbers, never counting client-side queue time). Exempt kinds
	// (throttleExemptKinds) skip it, so a misconfigured throttle can never lock a user out of
	// fixing it. An op cancelled or timed out while queued returns here with neither op:start nor
	// op:end ever emitted — it never touched the database, so it is not a database operation.
	releaseSlot := func() {}
	if spec.ConnectionID != nil && !throttleExemptKinds[spec.Kind] {
		gated, release, err := h.admit(derived, *spec.ConnectionID)
		if err != nil {
			return "", nil, err
		}
		derived, releaseSlot = gated, release
	}
	defer releaseSlot()

	startedAt := kiratime.NowISO()
	h.mu.Lock()
	if cur, ok := h.running[opID]; ok {
		cur.startedAt, cur.listed = startedAt, true
		h.running[opID] = cur
	}
	h.mu.Unlock()
	h.emitJSON(studiooplog.EventOpStart, opStartPayload{
		OpID: opID, ConnectionID: spec.ConnectionID, TabID: spec.TabID, Kind: spec.Kind, StartedAt: startedAt,
		Incognito: spec.Incognito, DelayMs: spec.DelayMs, Source: OpSourceOf(ctx),
	})

	op := adapters.NewOpCtx(opID)
	start := time.Now()
	// Armed here, after admit and op:start, so throttle queue wait never counts against the limit.
	var timedOut atomic.Bool
	var timer *time.Timer
	if spec.Timeout > 0 {
		timer = time.AfterFunc(spec.Timeout+spec.TimeoutGrace, func() {
			h.mu.Lock()
			cur, ok := h.running[opID]
			h.mu.Unlock()
			if ok {
				timedOut.Store(true)
				h.stopOp(context.Background(), opID, cur)
			}
		})
	}
	value, err := h.safeRun(derived, op, fn)
	if timer != nil {
		timer.Stop()
	}
	elapsed := time.Since(start)
	durationMs := int(elapsed.Milliseconds())
	releaseSlot() // before op:end so the next queued op is not held by event emission

	var detail *adapters.TimeoutDetail
	switch {
	case err == nil:
	case errors.As(err, &detail):
		if detail.Elapsed == 0 {
			detail.Elapsed = elapsed
		}
	case timedOut.Load():
		err = adapters.StatementTimeout(spec.Timeout, elapsed, err)
		errors.As(err, &detail)
	}

	status := "ok"
	var errMsg *string
	if err != nil {
		status = "error"
		if derived.Err() == context.Canceled && detail == nil {
			status = "cancelled"
		}
		m := err.Error()
		errMsg = &m
	}
	var command *string
	if c := op.Command(); c != "" {
		command = &c
	}
	h.emitJSON(studiooplog.EventOpEnd, opEndPayload{
		OpID: opID, Status: status, DurationMs: durationMs, Rows: op.Rows(), Command: command, Error: errMsg,
		Path: op.Path(),
	})

	return opID, value, err
}

// errInvalidOpID never echoes the id: it is client text that reaches logs and engines.
func errInvalidOpID() error {
	return adapters.T(adapters.CodeQuery, "backend.studio.adapter.invalidOperationId.body", nil, nil)
}

// Cancellable registers opID so CancelOp cancels the returned context, without emitting op events
// (no Operations panel row, no op log). release must be called when the work ends. A duplicate id
// is refused; an earlier Stop for opID returns an already-cancelled context.
func (h *Host) Cancellable(ctx context.Context, opID string) (context.Context, func(), error) {
	if opID != "" && !ids.IsUUID(opID) {
		return nil, nil, errInvalidOpID()
	}
	h.mu.Lock()
	if _, exists := h.running[opID]; exists {
		h.mu.Unlock()
		return nil, nil, adapters.New(adapters.CodeQuery, "duplicate operation id: "+opID, nil)
	}
	derived, cancel := context.WithCancel(ctx)
	if expiry, ok := h.earlyCancels[opID]; ok {
		delete(h.earlyCancels, opID)
		if time.Now().Before(expiry) {
			cancel()
		}
	}
	h.running[opID] = runningOp{cancel: cancel}
	h.mu.Unlock()
	return derived, func() {
		h.mu.Lock()
		delete(h.running, opID)
		h.mu.Unlock()
		cancel()
	}, nil
}

// safeRun is the recover() boundary: a panic inside fn becomes a failed op instead of
// crashing the app. E_INTERNAL deliberately is not one of adapters' eight closed error codes —
// the renderer's classify() already handles an unrecognized code correctly, and adding it
// to the closed set would be a claim that ordinary adapter code can produce it on purpose.
func (h *Host) safeRun(ctx context.Context, op *adapters.OpCtx, fn func(context.Context, *adapters.OpCtx) (any, error)) (value any, err error) {
	defer func() {
		if r := recover(); r != nil {
			h.deps.Log("error", fmt.Sprintf("adapter panic: %v\n%s", r, debug.Stack()))
			// quoteIdent's own NUL-byte guard (postgres/mysqlfamily/sqlite/clickhouse read.go)
			// panics with an *adapters.Error carrying a real code (E_QUERY — a user-input problem,
			// not an internal fault). Unwrapping it here, rather than always folding into
			// E_INTERNAL, keeps that code intact instead of presenting a query error as an
			// internal one.
			if adapterErr, ok := r.(*adapters.Error); ok {
				err = adapterErr
				return
			}
			err = adapters.Internal(fmt.Sprintf("internal error: %v", r), nil)
		}
	}()
	return fn(ctx, op)
}

func (h *Host) emitJSON(topic string, payload any) {
	body, err := json.Marshal(payload)
	if err != nil {
		return
	}
	h.events.Emit(studiooplog.Event{Topic: topic, Payload: body})
}

// rememberEarlyCancelLocked records an unknown id for RunOp to refuse; h.mu must be held.
func (h *Host) rememberEarlyCancelLocked(opID string) {
	now := time.Now()
	for id, expiry := range h.earlyCancels {
		if !now.Before(expiry) {
			delete(h.earlyCancels, id)
		}
	}
	if len(h.earlyCancels) >= maxEarlyCancels {
		var oldest string
		var oldestExpiry time.Time
		for id, expiry := range h.earlyCancels {
			if oldest == "" || expiry.Before(oldestExpiry) {
				oldest, oldestExpiry = id, expiry
			}
		}
		delete(h.earlyCancels, oldest)
	}
	h.earlyCancels[opID] = now.Add(h.earlyTTL)
}

// CancelOp is scheduler/ops.ts's cancelOp, in the same order and for the same reason: the local
// abort unblocks the running RunOp call immediately; the adapter call is what kills the
// server-side work (cancellation is always forwarded). Both steps are best-effort — an
// unknown op id (remembered briefly, so a Stop that beats its own Run still cancels it), or an
// adapter that has already gone, is not an error, matching cancel()'s own
// "never throws for already finished" contract.
func (h *Host) CancelOp(ctx context.Context, opID string) (bool, error) {
	if !ids.IsUUID(opID) {
		return false, nil
	}
	h.mu.Lock()
	op, ok := h.running[opID]
	if !ok {
		h.rememberEarlyCancelLocked(opID)
	}
	h.mu.Unlock()
	if !ok {
		return false, nil
	}
	h.stopOp(ctx, opID, op)
	return true, nil
}

// stopOp is the shared tail of CancelOp and CancelRunning.
func (h *Host) stopOp(ctx context.Context, opID string, op runningOp) {
	op.cancel()
	adapter := op.adapter
	if adapter == nil && op.connectionID != nil {
		adapter, _ = adapters.GetLiveAdapter(*op.connectionID)
	}
	if adapter != nil {
		// F1 (P168 Part 5): a stalled engine must not park the Cancel call forever.
		cancelCtx, stop := context.WithTimeout(ctx, disconnectTimeout)
		_, _ = adapter.Cancel(cancelCtx, opID)
		stop()
	}
}

// RunningOps snapshots ops that have a panel row (op:start emitted), oldest first.
func (h *Host) RunningOps() []RunningOp {
	h.mu.Lock()
	out := make([]RunningOp, 0, len(h.running))
	for id, op := range h.running {
		if op.listed {
			out = append(out, RunningOp{OpID: id, Kind: op.kind, Source: op.source, StartedAt: op.startedAt, ConnectionID: op.connectionID})
		}
	}
	h.mu.Unlock()
	sort.Slice(out, func(i, j int) bool {
		if out[i].StartedAt != out[j].StartedAt {
			return out[i].StartedAt < out[j].StartedAt
		}
		return out[i].OpID < out[j].OpID
	})
	return out
}

// CancelRunning cancels a listed op. Unlike CancelOp, a miss leaves no early-cancel entry, so a
// guessed or stale id cannot poison a later op.
func (h *Host) CancelRunning(ctx context.Context, opID string) (bool, error) {
	h.mu.Lock()
	op, ok := h.running[opID]
	h.mu.Unlock()
	if !ok || !op.listed {
		return false, nil
	}
	h.stopOp(ctx, opID, op)
	return true, nil
}

// CancelOpsForConnection locally cancels every currently-running op registered against
// connectionID, except exceptOpID (pass "" when there is none to spare). Used
// before tearing an old adapter down (a reconnect's own teardown in Router.Connect, or an explicit
// Router.Disconnect) so each cancelled op's own driver ctx watcher can unblock whatever the old
// adapter's real Disconnect would otherwise wait on: QueryTracker.Drain's inFlight.Wait has no
// bound of its own under context.Background, and ConnSet.CloseAll's per-entry Close takes the
// entry mutex an in-flight op still holds. Only the local abort runs here — same as CancelOp, the
// adapter-side Cancel (if any of these ops still need it) is left to CancelOp's own path.
func (h *Host) CancelOpsForConnection(connectionID, exceptOpID string) {
	h.mu.Lock()
	var toCancel []context.CancelFunc
	for opID, op := range h.running {
		if opID == exceptOpID || op.connectionID == nil || *op.connectionID != connectionID {
			continue
		}
		toCancel = append(toCancel, op.cancel)
	}
	h.mu.Unlock()
	for _, cancel := range toCancel {
		cancel()
	}
}

// CancelOpsOnAdapter locally cancels every running op that runs on adapter (a read lane).
func (h *Host) CancelOpsOnAdapter(adapter adapters.Adapter) {
	h.mu.Lock()
	var toCancel []context.CancelFunc
	for _, op := range h.running {
		if op.adapter == adapter {
			toCancel = append(toCancel, op.cancel)
		}
	}
	h.mu.Unlock()
	for _, cancel := range toCancel {
		cancel()
	}
}

// Subscribe returns this subscriber's own channel of every op:start/op:end event Host emits, and
// an unsubscribe func — the exact shape studiooplog.EventSource wants, since this Host is oplog's only
// producer now (P58f D9; it used to be fanned together with enginehost.Host's own events).
func (h *Host) Subscribe() (<-chan studiooplog.Event, func()) {
	sub := newEventSub()
	unsubscribe := h.events.Subscribe(sub.deliver)
	return sub.ch, func() {
		unsubscribe()
		sub.close()
	}
}
