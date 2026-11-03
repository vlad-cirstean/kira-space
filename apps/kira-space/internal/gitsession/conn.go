package gitsession

import (
	"context"
	"errors"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/oplog"
	"sync"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitaskpass"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitclient"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitclient/porcelain"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitpreflight"
)

// ErrRepoNotHeld is returned by Conn.Walk when this connection has no open hold on repoID —
// repo.open must precede any graph.* call, exactly as it must for every other per-repo request.
var ErrRepoNotHeld = errors.New("gitsession: repository is not open on this connection")

// ConnID identifies one accepted connection — the stream's own minted session id, opaque here.
type ConnID string

// autoFetchEvent is autoFetch.changed's wire payload.
type autoFetchEvent struct {
	RepoID    string                        `json:"repoId"`
	AutoFetch *gitpreflight.AutoFetchStatus `json:"autoFetch"`
}

// hold is one (connection, repository) pairing: the ref this connection took on the entry, and the
// subscription that delivers repo.changed to it.
type hold struct {
	entry       *RepoEntry
	release     func()
	unsubscribe func()
}

// walkPair is SPEC's per-(connection, repository) walk box, read literally: "log session, commit
// store, dictionary marks, scroll/paging state, the active review-range walk if any". Two slots,
// never a map keyed by range — SPEC is explicit that the review view holds exactly one review
// session, and reviewing another branch replaces its contents rather than opening a second view.
type walkPair struct {
	graph  *Walk
	review *Walk
}

// Conn is one connection's private session state — SPEC §6's Conn box, minus Walk (G6). Emit is
// supplied by bridge so this package never imports bridge (SPEC §7's layering rule);
// it is called from a subscriber's own goroutine (D14), never from the watcher's.
type Conn struct {
	ID       ConnID
	ClientID string
	// ClientLabel is the connection's own clamped label — G5's own addition
	// (F11), threaded through so the undo slot can attribute a record to "this window" for every
	// OTHER connection's reader (D7's SnapshotFor).
	ClientLabel string
	// Module is the oplog module this connection's work is attributed to (git, vscode or ade).
	Module string

	// emitMu guards emitFn: gitrpc.Router.ForConn's own
	// repoSettings.changed forwarding goroutine starts as soon as ForConn is called — before
	// bridge's GitWindows reaches its own SetEmit call
	// — so a bare, unguarded field here was a genuine read/write data race under the Go memory
	// model (another connection's concurrent repoSettings.set could deliver its notification right
	// inside that window). The nil-check every call site used to do individually now lives once,
	// inside Emit itself. Call SetEmit to install it, Emit to call it — never read/write emitFn
	// directly from outside this file.
	emitMu sync.RWMutex
	emitFn func(method string, payload any)

	// noAutoFetch is the opt-out: the native git-graph mount's own Conn (bridge/
	// gitwindows.go's GitWindows) sets this before serving any request, so its own repo.open
	// never arms a RepoEntry's background auto-fetch timer (autofetch.go) — restoring
	// the read-only claim of docs/ARCHITECTURE.md's "Git graph in the native workspace (C10)"
	// section for that surface specifically. Set once,
	// before this Conn is handed to anything that could call Open concurrently (a plain field, not
	// mutex-guarded, on that basis) — see DisableAutoFetch's own doc comment. Every other Conn
	// leaves this false and keeps today's behaviour unchanged: RepoEntry is shared per-repository
	// across every connection regardless, so any
	// OTHER already-open connection on the same repository can still have armed its timer — this
	// only stops the native mount's OWN repo.open from being what arms it.
	noAutoFetch bool

	mu     sync.Mutex
	held   map[string]*hold     // RepoID -> hold
	walks  map[string]*walkPair // RepoID -> walkPair (D3), allocated lazily by whichever walk is created first
	closed bool                 // set by Close, inside the same critical section that swaps held/walks (D3) —
	// Requests run on their own goroutines, so a repo.open can, and reproducibly does, complete
	// after Close already ran. Open's final critical section checks this and releases rather than stores a
	// hold on a connection that is already gone (F1) — the hold would otherwise never be released by
	// anyone: Close already swapped in the maps it stored into.

	// done is G7 D20/D21's own disconnect signal — closed once, in Close, so a blocked credential
	// waiter (AskCredential's own select) can observe this connection going away without ever
	// reaching into the bridge, which this package must stay under (SPEC §7's layering
	// rule).
	done chan struct{}
	// routeCred answers every credential prompt this connection raises (RouteCredentials).
	routeCred func(ctx context.Context, req gitaskpass.Request) (string, bool)
	closeOnce sync.Once
}

// NewConn constructs a Conn with an empty hold set.
// Origin is the op-log attribution for work this connection's user asked for.
func (c *Conn) Origin() oplog.Origin {
	m := c.Module
	if m == "" {
		m = oplog.ModuleApp
	}
	return oplog.Origin{Module: m, Trigger: oplog.TriggerUser, Label: c.ClientLabel}
}

func NewConn(id ConnID, clientID, clientLabel string, emit func(method string, payload any)) *Conn {
	return &Conn{
		ID: id, ClientID: clientID, ClientLabel: clientLabel, emitFn: emit,
		held: make(map[string]*hold), walks: make(map[string]*walkPair),
		done: make(chan struct{}),
	}
}

// SetEmit installs (or replaces) this connection's Emit function — bridge's GitWindows calls this
// once, right after the Conn exists (F9's own doc comment on emitMu).
func (c *Conn) SetEmit(fn func(method string, payload any)) {
	c.emitMu.Lock()
	c.emitFn = fn
	c.emitMu.Unlock()
}

// Emit calls this connection's currently installed Emit function, silently doing nothing if none
// is installed yet — every call site used to repeat that "check for nil, then call" pattern
// itself, directly against the bare field this method replaces.
func (c *Conn) Emit(method string, payload any) {
	c.emitMu.RLock()
	fn := c.emitFn
	c.emitMu.RUnlock()
	if fn != nil {
		fn(method, payload)
	}
}

// Done reports this connection's own disconnect signal (G7 D20) — closed exactly once, by Close.
func (c *Conn) Done() <-chan struct{} { return c.done }

// DisableAutoFetch opts this Conn's own Open calls out of arming a RepoEntry's background
// auto-fetch timer. Call it once, right after NewConn, before this Conn is handed to
// anything that could start dispatching requests on it (GitWindows does so before the Conn is
// handed out) — the resulting happens-before edge (the "go" statement that
// starts each request-handling goroutine) is what makes this safe with no lock of its own.
func (c *Conn) DisableAutoFetch() { c.noAutoFetch = true }

// RouteCredentials sets where every credential prompt this connection raises goes. Call it once,
// before this Conn serves — the same happens-before rule as DisableAutoFetch. fn gets a ctx that
// also ends when this connection closes.
func (c *Conn) RouteCredentials(fn func(ctx context.Context, req gitaskpass.Request) (string, bool)) {
	c.routeCred = fn
}

// AskCredential is this connection's own gitaskpass.Prompter implementation: it hands the prompt to
// the routed relay and ends the wait when ctx (the op's own cancellation) or this connection ends.
// A connection with no route answers nothing.
func (c *Conn) AskCredential(ctx context.Context, req gitaskpass.Request) (string, bool) {
	if c.routeCred == nil {
		return "", false
	}
	routed, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		select {
		case <-c.done:
			cancel()
		case <-routed.Done():
		}
	}()
	return c.routeCred(routed, req)
}

// Open acquires path's repository and subscribes this connection to it — idempotent per (Conn,
// RepoID): a second Open for a repo this connection already holds releases the ref it just took
// and returns the existing hold's summary. RepoID is only known after Identify
// runs inside Acquire, so this is acquire-then-release rather than check-then-acquire — identifying
// under this connection's own lock would serialise every Open this connection makes.
func (c *Conn) Open(ctx context.Context, reg *Registry, gitPath, path string) (gitclient.RepoSummary, error) {
	// C13-10: a noAutoFetch Conn (the native git-graph mount) must never be what arms a BRAND-NEW
	// entry's auto-fetch timer either — AcquireQuiet is Acquire with exactly that one difference.
	var entry *RepoEntry
	var release func()
	var err error
	if c.noAutoFetch {
		entry, release, err = reg.AcquireQuiet(ctx, gitPath, path)
	} else {
		entry, release, err = reg.Acquire(ctx, gitPath, path)
	}
	if err != nil {
		return gitclient.RepoSummary{}, err
	}
	repoID := entry.Summary.RepoID

	if existing, ok := c.alreadyHeld(repoID); ok {
		release()
		return existing.Summary, nil
	}

	unsubscribeRepo := entry.Subscribe(c.ID, func(ev Event) {
		// Mark before emitting: a client that reacts to repo.changed by re-opening its
		// stream must never be able to observe a walk that has not yet been told refs moved.
		// Marks BOTH the graph and review walk — a deliberate departure from upstream, whose
		// review walk dies on hide and so never needs this: ours persists across hide/show, and
		// without this the client's own "the comparison has changed" banner would replay a stale
		// store instead of re-walking.
		if ev.Kind == string(gitclient.SignalRefsChanged) {
			c.markWalksStale(ev.RepoID)
		}
		c.Emit("repo.changed", ev)
	})
	unsubscribeAutoFetch := entry.autoFetch.changed.Subscribe(func(ch AutoFetchChange) {
		c.Emit("autoFetch.changed", autoFetchEvent{RepoID: repoID, AutoFetch: ch.AutoFetch})
	})
	unsubscribe := func() {
		unsubscribeRepo()
		unsubscribeAutoFetch()
	}

	c.mu.Lock()
	if c.closed {
		// This connection's Close already ran (and swapped in the fresh maps below) while this
		// Open was still identifying the repository or subscribing. The repository was identified —
		// the caller gets a correct summary back — but nothing must be stored: storing a hold here
		// would leak it forever.
		c.mu.Unlock()
		unsubscribe()
		release()
		return entry.Summary, nil
	}
	if existing, ok := c.held[repoID]; ok {
		// Lost a race against a concurrent Open for the same repo on this connection (two
		// repo.open requests dispatched on their own goroutines) — clean up the
		// work just done rather than leaking it.
		c.mu.Unlock()
		unsubscribe()
		release()
		return existing.entry.Summary, nil
	}
	c.held[repoID] = &hold{entry: entry, release: release, unsubscribe: unsubscribe}
	c.mu.Unlock()
	// Arms auto-fetch for the off→on direction: newRepoEntry only ever sees the interval
	// AT construction time, so a repository already open when the setting flips on would otherwise
	// never get a timer. ensureAutoFetch is a no-op whenever one is already running or the entry is
	// disabled, so N windows opening this repository arm exactly one.
	//
	// Skipped for a Conn that opted out (the native git-graph mount) — its own repo.open
	// must never be what arms a timer. This is per-Conn only: RepoEntry itself is still shared
	// across every connection on this repository, so another, ordinary connection opening the same
	// repository can still arm it regardless of what this Conn does.
	if !c.noAutoFetch {
		entry.EnsureAutoFetch()
	}
	return entry.Summary, nil
}

func (c *Conn) alreadyHeld(repoID string) (*RepoEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	h, ok := c.held[repoID]
	if !ok {
		return nil, false
	}
	return h.entry, true
}

// RecordFailure writes one Operations-log record for a failure no running op covers (a graph load
// that died). It does nothing when this connection does not hold repoID.
func (c *Conn) RecordFailure(repoID, kind, message string) {
	if entry, ok := c.alreadyHeld(repoID); ok {
		entry.recordFailure(oplog.WithOrigin(context.Background(), c.Origin()), kind, c.ClientLabel, message)
	}
}

// Entry returns the RepoEntry this connection holds for repoID — the seam every per-repo request
// that is not a graph.* walk needs: a commit's detail, a file's patch and a blob's bytes
// are facts about the repository, not about one viewer's walk. Reports
// false under the same condition ErrRepoNotHeld names — repo.open must precede this call, exactly
// as it must for every other per-repo request.
func (c *Conn) Entry(repoID string) (*RepoEntry, bool) {
	return c.alreadyHeld(repoID)
}

// CloseRepo releases this connection's hold on repoID, if any, reporting whether one was present —
// the RPC ignores the bool and answers {} either way, preserving repo.close's idempotency. Any
// walk over repoID is disposed (its git log process killed) before the ref is released, so
// the registry's refcount can never reach zero while a walk still holds a process against it.
func (c *Conn) CloseRepo(repoID string) bool {
	c.mu.Lock()
	h, ok := c.held[repoID]
	if ok {
		delete(c.held, repoID)
	}
	pair, hasWalk := c.walks[repoID]
	if hasWalk {
		delete(c.walks, repoID)
	}
	c.mu.Unlock()

	if hasWalk {
		disposePair(pair)
	}
	if !ok {
		return false
	}
	h.unsubscribe()
	h.release()
	return true
}

// Close releases every hold this connection has — bridge's disconnect teardown (SPEC §6:
// "release its RepoEntry refcounts"). Every walk (both slots of every pair) is disposed first
// (D13), same ordering as CloseRepo.
func (c *Conn) Close() {
	c.closeOnce.Do(func() { close(c.done) })

	c.mu.Lock()
	c.closed = true
	holds := c.held
	c.held = make(map[string]*hold)
	walks := c.walks
	c.walks = make(map[string]*walkPair)
	c.mu.Unlock()

	for _, pair := range walks {
		disposePair(pair)
	}
	for _, h := range holds {
		h.unsubscribe()
		h.release()
	}
}

// disposePair disposes both slots of pair, whichever are non-nil.
func disposePair(pair *walkPair) {
	if pair.graph != nil {
		pair.graph.dispose()
	}
	if pair.review != nil {
		pair.review.dispose()
	}
}

// Walk returns this connection's Walk for repoID — created lazily on first call, rebuilt (the old
// one disposed) whenever spec no longer matches what it was built with (a scope or range change,
// the #ensureReviewWalk shape). Errors with ErrRepoNotHeld if this connection has not
// opened repoID.
//
// spec.Range == nil selects the graph slot; spec.Range != nil selects the review slot — both
// slots live in one walkPair per repoID, so a ranged request can never disturb the graph's own
// walk and vice versa. Reviewing a second branch replaces the first review walk; there is never
// more than one per (connection, repository).
//
// pageSize (the graph.pageSize) only takes effect on first construction of a given slot — the
// underlying log session's own page size is fixed at Open time and a later call with a different
// value does not rebuild an otherwise-matching walk; it is not part of matchesSpec.
//
// precomputedTotal is threaded into a freshly-built walk only — an already-matching walk
// reused here keeps whatever total it already has (a peeked count is a hint for the walk's own
// FIRST open, never a value that overwrites a walk already under way).
func (c *Conn) Walk(repoID string, gitPath string, spec porcelain.WalkSpec, pageSize int, precomputedTotal *int) (*Walk, error) {
	c.mu.Lock()

	if c.closed {
		c.mu.Unlock()
		return nil, ErrRepoNotHeld
	}
	h, ok := c.held[repoID]
	if !ok {
		c.mu.Unlock()
		return nil, ErrRepoNotHeld
	}

	pair, ok := c.walks[repoID]
	if !ok {
		pair = &walkPair{}
		c.walks[repoID] = pair
	}

	slot := &pair.graph
	if spec.Range != nil {
		slot = &pair.review
	}

	if *slot != nil && (*slot).matchesSpec(spec) {
		w := *slot
		c.mu.Unlock()
		return w, nil
	}

	// Swap the slot under c.mu, then dispose the OLD walk only after
	// unlocking — the same shape CloseRepo/Close already use in this file. dispose() takes the
	// walk's own lock, which a running Stream holds across a git page read and its own emit
	// backpressure; disposing while still holding c.mu would stall every other c.mu user on this
	// connection (Entry, alreadyHeld, the subscriber goroutine's markWalksStale) behind that same
	// page read — breaking the documented "subscriber never blocks behind a page read" rule.
	old := *slot
	w := newWalk(h.entry, gitPath, spec, pageSize, precomputedTotal)
	*slot = w
	c.mu.Unlock()

	if old != nil {
		old.dispose()
	}
	return w, nil
}

// walkForSlot is WalkFor/ReviewWalkFor's shared body (I2-47): locate repoID's walk pair, then read
// one of its two slots via slot, without creating either.
func (c *Conn) walkForSlot(repoID string, slot func(*walkPair) *Walk) (*Walk, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	pair, ok := c.walks[repoID]
	if !ok {
		return nil, false
	}
	if w := slot(pair); w != nil {
		return w, true
	}
	return nil, false
}

// WalkFor returns this connection's existing GRAPH walk for repoID, if any, without creating one.
func (c *Conn) WalkFor(repoID string) (*Walk, bool) {
	return c.walkForSlot(repoID, func(p *walkPair) *Walk { return p.graph })
}

// ReviewWalkFor returns this connection's existing REVIEW (ranged) walk for repoID, if any,
// without creating one.
func (c *Conn) ReviewWalkFor(repoID string) (*Walk, bool) {
	return c.walkForSlot(repoID, func(p *walkPair) *Walk { return p.review })
}

// markWalksStale marks BOTH slots of repoID's walk pair stale — called by Open's own
// subscriber on refsChanged, in place of reaching into a single walk. Never takes a walk's own mu
// (MarkStale's own doc): the subscriber's goroutine must not block behind a page read.
//
// pair.graph/pair.review themselves — the
// walkPair's own *Walk fields, not the walks it points to — are c.mu-protected state (Walk's own
// `*slot = w` / `(*slot).dispose(); *slot = nil` above only ever runs under c.mu). This used to
// read them AFTER releasing c.mu, racing against a concurrent Walk() call rebuilding either slot —
// exactly the unsynchronized concurrent read/write go test -race exists to catch. Both fields are
// snapshotted into locals while c.mu is still held; MarkStale itself (an atomic store, walk.go)
// still runs outside the lock, preserving the original "never block behind a page read" property.
func (c *Conn) markWalksStale(repoID string) {
	c.mu.Lock()
	pair, ok := c.walks[repoID]
	var graph, review *Walk
	if ok {
		graph, review = pair.graph, pair.review
	}
	c.mu.Unlock()
	if !ok {
		return
	}
	if graph != nil {
		graph.MarkStale()
	}
	if review != nil {
		review.MarkStale()
	}
}
