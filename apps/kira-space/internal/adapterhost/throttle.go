package adapterhost

import (
	"context"
	"fmt"
	"math"
	"slices"
	"sync"
	"time"

	"golang.org/x/sync/semaphore"
	"golang.org/x/time/rate"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
)

// throttleMaxWait bounds how long admit will wait for a slot and a token before giving up with E_TIMEOUT
// (P28 §5.4) — a pathological rate/queue-depth combination (e.g. 0.1/s with 50 queued reads) must
// not look like a hung app forever.
const throttleMaxWait = 30 * time.Second

// throttleExemptKinds is every op kind the gate skips: a throttle must never lock a user out of
// connecting to, disconnecting from or testing the connection whose throttle is misconfigured.
// Everything else with a connection id is gated, so a future kind fails closed (P108 Part 6 F3
// hit the old allow-list failing open). Cancel is not an op at all: Host.stopOp calls
// adapter.Cancel directly, which must reach an op holding the slot or serial mode would deadlock Stop.
var throttleExemptKinds = map[string]bool{
	"connect": true, "disconnect": true, "test": true, "preconnect": true,
}

// connGate is one connection's gate: ops run one at a time (slot, FIFO) and are paced (limiter).
type connGate struct {
	limiter *rate.Limiter
	slot    *semaphore.Weighted
}

// throttleRegistry maps a connection id to its live gate. A dedicated mutex, not Host.mu, so the
// hot running map (touched on every op) never contends with a config write from Service.Update or
// a disconnect's clear.
type throttleRegistry struct {
	mu   sync.RWMutex
	byID map[string]*connGate
}

func newThrottleRegistry() *throttleRegistry {
	return &throttleRegistry{byID: make(map[string]*connGate)}
}

// set installs (perSec > 0) or clears (perSec <= 0) connectionID's gate. Burst is
// max(1, min(10, round(perSec))) (§5.3): a strict burst of 1 would delay the first click after an
// idle period, making a correctly-configured limit feel broken; a small burst lets interactive use
// through immediately and paces only sustained traffic, which is what a server-side limit actually
// cares about. A live edit updates the limiter in place and keeps the slot, so the op holding it
// and the ops queued behind it stay in order.
func (t *throttleRegistry) set(connectionID string, perSec float64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if perSec <= 0 {
		delete(t.byID, connectionID)
		return
	}
	burst := min(10, max(1, int(math.Round(perSec))))
	if g, ok := t.byID[connectionID]; ok {
		g.limiter.SetLimit(rate.Limit(perSec))
		g.limiter.SetBurst(burst)
		return
	}
	t.byID[connectionID] = &connGate{limiter: rate.NewLimiter(rate.Limit(perSec), burst), slot: semaphore.NewWeighted(1)}
}

// gateFor returns the live gate for connectionID, or nil when the connection is unthrottled.
func (t *throttleRegistry) gateFor(connectionID string) *connGate {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.byID[connectionID]
}

type heldSlotsKey struct{}

// holdsSlot reports whether ctx already runs inside connectionID's slot.
func holdsSlot(ctx context.Context, connectionID string) bool {
	held, _ := ctx.Value(heldSlotsKey{}).([]string)
	return slices.Contains(held, connectionID)
}

func withHeldSlot(ctx context.Context, connectionID string) context.Context {
	held, _ := ctx.Value(heldSlotsKey{}).([]string)
	return context.WithValue(ctx, heldSlotsKey{}, append(slices.Clone(held), connectionID))
}

// admit takes connectionID's turn: its slot, then a token. Slot first, so only the op about to run
// spends a token and the admitted rate holds. The returned release frees the slot (idempotent)
// and the returned ctx marks the slot held, so a nested call under it skips the slot instead of
// deadlocking. An unthrottled connection returns a no-op release at once. One timeout bounds both
// waits; a cancelled ctx gives E_CANCELLED, an expired wait E_TIMEOUT.
func (h *Host) admit(ctx context.Context, connectionID string) (context.Context, func(), error) {
	g := h.throttles.gateFor(connectionID)
	if g == nil {
		return ctx, func() {}, nil
	}
	reentrant := holdsSlot(ctx, connectionID)
	waitCtx, cancel := context.WithTimeout(ctx, h.throttleWait)
	defer cancel()

	var stageErr error
	slotTaken := false
	if !reentrant {
		if stageErr = g.slot.Acquire(waitCtx, 1); stageErr != nil {
			return ctx, nil, waitFailure(ctx, "backend.studio.adapter.throttleBusyTimeout.body", h.throttleWait, 0, stageErr)
		}
		slotTaken = true
	}
	if stageErr = g.limiter.Wait(waitCtx); stageErr != nil {
		if slotTaken {
			g.slot.Release(1)
		}
		return ctx, nil, waitFailure(ctx, "backend.studio.adapter.throttleRateTimeout.body", h.throttleWait, float64(g.limiter.Limit()), stageErr)
	}
	if reentrant {
		return ctx, func() {}, nil
	}
	var once sync.Once
	return withHeldSlot(ctx, connectionID), func() { once.Do(func() { g.slot.Release(1) }) }, nil
}

// waitFailure classifies a failed wait: the caller's cancel is E_CANCELLED, anything else
// (deadline, or limiter.Wait refusing a wait that cannot fit the deadline) is E_TIMEOUT.
func waitFailure(ctx context.Context, timeoutKey string, wait time.Duration, perSec float64, cause error) error {
	if ctx.Err() == context.Canceled {
		return adapters.T(adapters.CodeCancelled, "backend.studio.adapter.cancelledWaitingForRateLimit.body", nil, cause)
	}
	params := map[string]string{"seconds": fmt.Sprintf("%.4g", wait.Seconds())}
	if perSec > 0 {
		params["rate"] = fmt.Sprintf("%.4g", perSec)
	}
	return adapters.T(adapters.CodeTimeout, timeoutKey, params, cause)
}
