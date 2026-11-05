// Package pairing is the approval broker shared by every Kira Space pairing flow (git extension,
// mobile web): a FIFO of pending requests, a per-client cooldown after an explicit deny, an
// injected clock and a token minted at approval. M is the caller's own request metadata.
package pairing

import (
	"log/slog"
	"sync"
	"time"

	"github.com/kirathecat/kira-studio/internal/ids"
	"github.com/kirathecat/kira-studio/internal/notify"
	"github.com/kirathecat/kira-studio/internal/prompts"
	"github.com/kirathecat/kira-studio/internal/tokenauth"
)

// Outcome is Broker.Request's own vocabulary: a pairing decision is a value, never a Go error.
type Outcome int

const (
	Approved Outcome = iota
	Denied
	TimedOut
	// Aborted is a request resolved by something that is not a user decision: shutdown, a full
	// queue, Request after Shutdown, a requester that went away, or a sibling approval. Distinct
	// from Denied, which is always a real user Deny (and starts the cooldown).
	Aborted
)

// ActionResult is what Approve/Deny report.
type ActionResult int

const (
	Resolved ActionResult = iota
	AlreadyResolved
	Expired
)

// Config tunes one Broker. Timeout runs from enqueue, not from being presented; Cooldown starts
// on an explicit Deny only. Now defaults to time.Now.
type Config struct {
	Timeout  time.Duration
	Cooldown time.Duration
	MaxQueue int
	Now      func() time.Time
	// Prompts routes each queued request to a window and an OS notification as Kind (P246); nil routes none.
	Prompts prompts.Sink
	Kind    prompts.Kind
}

// Request is one queued request, what a window's prompt renders.
type Request[M any] struct {
	RequestID  string
	ClientID   string
	Meta       M
	EnqueuedAt time.Time
	ExpiresAt  time.Time
}

// Snapshot is emitted on every queue change: the head request (nil when empty) plus how many are
// queued in total.
type Snapshot[M any] struct {
	Pending *Request[M]
	Queued  int
}

// ApprovedToken is the token an approval mints for the approved request alone. Read via
// TakeApprovedToken, keyed by RequestID.
type ApprovedToken struct {
	Plain string
	Hash  []byte
	Salt  []byte
}

type pendingEntry[M any] struct {
	req    Request[M]
	result chan Outcome // buffered 1; exactly one send over the entry's lifetime.
}

// Broker is a FIFO of pending requests (the head is the one presented), a per-clientID cooldown
// map and a notify.OrderedEmitter fanning out every change. It knows nothing about whether a
// window is open: a request simply sits at the head until someone renders it or it expires.
type Broker[M any] struct {
	cfg Config
	// Title is the routed popup's title for a request; it must not carry the pairing code.
	Title func(Request[M]) string

	mu       sync.Mutex
	queue    *notify.PendingQueue[*pendingEntry[M]]
	cooldown map[string]time.Time
	// closed refuses new requests after Shutdown: nothing would ever resolve them.
	closed bool
	// tokens holds each unclaimed approval token, keyed by the RequestID it was minted for.
	tokens map[string]ApprovedToken

	emitter notify.OrderedEmitter[Snapshot[M]]
}

func NewBroker[M any](cfg Config) *Broker[M] {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Broker[M]{
		cfg: cfg, queue: notify.NewPendingQueue[*pendingEntry[M]](),
		cooldown: map[string]time.Time{}, tokens: map[string]ApprovedToken{},
	}
}

// Timeout is the window a request waits for a decision.
func (b *Broker[M]) Timeout() time.Duration { return b.cfg.Timeout }

func (b *Broker[M]) Subscribe(fn func(Snapshot[M])) (unsubscribe func()) {
	return b.emitter.Subscribe(fn)
}

// Pending is the snapshot a newly opened window fetches on mount.
func (b *Broker[M]) Pending() Snapshot[M] {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.snapshotLocked()
}

func (b *Broker[M]) snapshotLocked() Snapshot[M] {
	items := b.queue.Snapshot()
	if len(items) == 0 {
		return Snapshot[M]{}
	}
	head := items[0].req
	return Snapshot[M]{Pending: &head, Queued: len(items)}
}

// InCooldown reports whether clientID is inside its post-denial window.
func (b *Broker[M]) InCooldown(clientID string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	until, ok := b.cooldown[clientID]
	return ok && b.cfg.Now().Before(until)
}

// Request enqueues (clientID, meta) and blocks the calling goroutine until it is approved,
// denied, or its deadline elapses. onEnqueued runs synchronously, before Request blocks, with the
// request as minted (its RequestID and deadline).
//
// The cooldown check short-circuits with neither enqueueing nor emitting: a client mid-cooldown
// gets an immediate, silent Denied.
func (b *Broker[M]) Request(clientID string, meta M, onEnqueued func(Request[M])) Outcome {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return Aborted
	}
	if until, ok := b.cooldown[clientID]; ok && b.cfg.Now().Before(until) {
		b.mu.Unlock()
		return Denied
	}
	// A full queue is capacity, not a decision about this client: Aborted.
	if b.queue.Len() >= b.cfg.MaxQueue {
		b.mu.Unlock()
		return Aborted
	}
	now := b.cfg.Now()
	entry := &pendingEntry[M]{
		req: Request[M]{
			RequestID: ids.NewID(), ClientID: clientID, Meta: meta,
			EnqueuedAt: now, ExpiresAt: now.Add(b.cfg.Timeout),
		},
		result: make(chan Outcome, 1),
	}
	b.queue.Add(entry.req.RequestID, entry)
	// Under mu so a fast answer cannot Close before this Open.
	b.openPromptLocked(entry.req)
	snap := b.snapshotLocked()
	seq := b.emitter.NextSeq()
	b.mu.Unlock()

	if onEnqueued != nil {
		onEnqueued(entry.req)
	}
	// Every enqueue changes Queued, so every enqueue emits; consumers dedupe on RequestID when
	// only the count behind the head changed.
	b.emitter.Emit(seq, snap)
	return <-entry.result
}

// Approve resolves requestID as approved and mints one token for that request only. Every other
// request already queued from the same clientID resolves Aborted: one click admits exactly one
// connection. Deny resolves requestID as denied, starts the client's cooldown and purges every
// sibling from the same clientID as denied. A deadline already passed when the decision arrives
// reports Expired, exactly like a timeout, with neither a cooldown nor a token.
func (b *Broker[M]) Approve(requestID string) ActionResult {
	return b.answer(requestID, Approved, false)
}

func (b *Broker[M]) Deny(requestID string) ActionResult {
	return b.answer(requestID, Denied, true)
}

func (b *Broker[M]) answer(requestID string, outcome Outcome, isDeny bool) ActionResult {
	b.mu.Lock()
	entry, ok := b.queue.Remove(requestID)
	if !ok {
		b.mu.Unlock()
		return AlreadyResolved
	}
	b.closePromptLocked(requestID)
	expired := !b.cfg.Now().Before(entry.req.ExpiresAt)
	var others []*pendingEntry[M]
	var mintFailed bool
	if !expired {
		others = b.removeAllForClientLocked(entry.req.ClientID)
		if isDeny {
			b.cooldown[entry.req.ClientID] = b.cfg.Now().Add(b.cfg.Cooldown)
		} else {
			plain, hash, salt, err := tokenauth.Mint()
			if err != nil {
				mintFailed = true
			} else {
				b.tokens[entry.req.RequestID] = ApprovedToken{Plain: plain, Hash: hash, Salt: salt}
			}
		}
	}
	snap := b.snapshotLocked()
	seq := b.emitter.NextSeq()
	b.mu.Unlock()

	if mintFailed {
		slog.Warn("pairing: mint token for approval", "scope", "pairing", "client", entry.req.ClientID)
	}

	// A mint failure degrades the decision to denied (fail closed) rather than approving with no
	// token to hand over.
	headOutcome, siblingOutcome := outcome, Denied
	if !isDeny {
		siblingOutcome = Aborted
		if mintFailed {
			headOutcome = Denied
		}
	}

	if expired {
		entry.result <- TimedOut
	} else {
		entry.result <- headOutcome
	}
	for _, other := range others {
		other.result <- siblingOutcome
	}
	b.emitter.Emit(seq, snap)
	if expired {
		return Expired
	}
	return Resolved
}

// Cancel removes requestID from the queue, if still there, and resolves it Aborted: the requester
// itself went away while its decision was pending. Without it an Approve could still mint a trust
// grant for a connection nobody holds. A no-op if requestID was already resolved.
func (b *Broker[M]) Cancel(requestID string) {
	b.mu.Lock()
	entry, ok := b.queue.Remove(requestID)
	if !ok {
		b.mu.Unlock()
		return
	}
	b.closePromptLocked(requestID)
	snap := b.snapshotLocked()
	seq := b.emitter.NextSeq()
	b.mu.Unlock()

	entry.result <- Aborted
	b.emitter.Emit(seq, snap)
}

// TakeApprovedToken returns (and forgets) the token Approve minted for requestID. ok is false when
// none was stored: a Denied or TimedOut outcome, or an approval degraded by a mint failure.
func (b *Broker[M]) TakeApprovedToken(requestID string) (ApprovedToken, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	tok, ok := b.tokens[requestID]
	if ok {
		delete(b.tokens, requestID)
	}
	return tok, ok
}

// ExpireOverdue resolves every queued request whose deadline has passed as TimedOut, with no
// cooldown: a timeout expresses no opinion. RunExpiry calls this on a real ticker; tests call it
// directly after advancing an injected clock.
func (b *Broker[M]) ExpireOverdue() {
	b.mu.Lock()
	now := b.cfg.Now()
	var overdue []*pendingEntry[M]
	for _, entry := range b.queue.Snapshot() {
		if !now.Before(entry.req.ExpiresAt) {
			overdue = append(overdue, entry)
		}
	}
	for _, entry := range overdue {
		b.queue.Remove(entry.req.RequestID)
		b.closePromptLocked(entry.req.RequestID)
	}
	snap := b.snapshotLocked()
	seq := b.emitter.NextSeq()
	b.mu.Unlock()

	for _, entry := range overdue {
		entry.result <- TimedOut
	}
	if len(overdue) > 0 {
		b.emitter.Emit(seq, snap)
	}
}

// RunExpiry drives ExpireOverdue every interval until stop closes. A deadline measured from
// enqueue can otherwise only be noticed by whoever next calls Approve/Deny, which may be nobody
// for a lone, unattended request.
func (b *Broker[M]) RunExpiry(stop <-chan struct{}, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			b.ExpireOverdue()
		case <-stop:
			return
		}
	}
}

// Shutdown resolves every queued request as Aborted, unblocking any Request still waiting. A
// parked Request blocks on a plain channel receive that closing its connection does not unblock,
// so the owner must call this before waiting on its connection goroutines. No cooldown starts:
// the owner going away is not a decision about the client.
func (b *Broker[M]) Shutdown() {
	b.mu.Lock()
	b.closed = true
	all := b.queue.Clear()
	for _, entry := range all {
		b.closePromptLocked(entry.req.RequestID)
	}
	snap := b.snapshotLocked()
	seq := b.emitter.NextSeq()
	b.mu.Unlock()

	for _, entry := range all {
		entry.result <- Aborted
	}
	if len(all) > 0 {
		b.emitter.Emit(seq, snap)
	}
}

// removeAllForClientLocked drops every remaining queued entry of clientID and returns them so the
// caller can resolve their result channels outside the lock. Caller holds b.mu.
func (b *Broker[M]) removeAllForClientLocked(clientID string) []*pendingEntry[M] {
	var removed []*pendingEntry[M]
	for _, e := range b.queue.Snapshot() {
		if e.req.ClientID == clientID {
			b.queue.Remove(e.req.RequestID)
			b.closePromptLocked(e.req.RequestID)
			removed = append(removed, e)
		}
	}
	return removed
}

func (b *Broker[M]) openPromptLocked(req Request[M]) {
	if b.cfg.Prompts == nil {
		return
	}
	title := string(b.cfg.Kind)
	if b.Title != nil {
		title = b.Title(req)
	}
	b.cfg.Prompts.Open(prompts.Prompt{
		ID: prompts.ID(b.cfg.Kind, req.RequestID), Kind: b.cfg.Kind, Ref: req.RequestID,
		Title: title, CreatedAt: req.EnqueuedAt.UnixMilli(),
	})
}

func (b *Broker[M]) closePromptLocked(requestID string) {
	if b.cfg.Prompts != nil {
		b.cfg.Prompts.Close(prompts.ID(b.cfg.Kind, requestID))
	}
}
