package gitsession

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitclient"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitclient/catfile"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/githost"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitpreflight"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitreview"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/oplog"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// ErrRepoTornDown is returned by whatever still reaches a RepoEntry after its own teardown() ran
// — Registry.Close() tears entries down regardless of refcount, so a request still
// in flight at process shutdown can observe this instead of a nil-map panic or a double-close.
// gitrpc's mapGitError maps it to E_BAD_REQUEST, same as ErrRepoNotHeld; the response is
// delivered nowhere anyway, since the socket is already closing.
var ErrRepoTornDown = errors.New("gitsession: repository entry has been torn down")

// Watcher is the minimal seam RepoEntry needs from a repo watcher — gitclient.RepoWatcher
// satisfies it structurally. Declared here (not imported as a concrete type) and exported so both
// registry_test.go (same package) and the flow harness (a different package,
// injecting a counting fake through Registry.NewWatcher) can drive refcount/linger logic with no
// filesystem and no real git.
type Watcher interface {
	Signals() <-chan gitclient.Signal
	Close() error
}

// Event is repo.changed's payload, SPEC §6/D20's shape verbatim — crossed as-is by
// a connection's emit through json.Marshal.
type Event struct {
	RepoID string `json:"repoId"`
	Kind   string `json:"kind"` // "refsChanged" | "worktreeChanged" | "changeRequestsChanged"
}

// subscriptionID is Subscribe's own per-call token — unique for the life of a RepoEntry
// (RepoEntry.nextSubID only ever increments, under mu, and is never reused), unlike the ConnID it
// replaced as e.subs' key: two concurrent Subscribe calls for the very same conn (a real race —
// conn.Open's own two-repo.open-requests case) must never collide on one map slot.
type subscriptionID uint64

// RepoEntry is one repository's SHARED state — everything true of the repository rather than of
// one viewer. It has the identity, the reader/writer gate (unchanged from gitclient, now shared
// across connections instead of within one), the watcher, and the subscriber fan-out. It has the
// cat-file session and the two detail/diff caches. It has the live head, the refs cache and the
// per-repo undo slot — still to come: stash shapes and active remote op.
type RepoEntry struct {
	Summary gitclient.RepoSummary
	Repo    *gitclient.Repo

	watcher Watcher

	mu sync.Mutex
	// subs is keyed by subscriptionID, not ConnID: two concurrent
	// Subscribe calls for the SAME conn (a real case — conn.Open's own two-repo.open-requests
	// race in conn.go) used to collide on a shared ConnID key, so the second call's
	// entry silently overwrote the first's, orphaning its goroutine; whichever call's own
	// unsubscribe fired first then deleted-and-closed whatever currently occupied that slot —
	// the OTHER call's subscriber — leaving the map with zero entries for this repo/conn pair
	// until the next repo.open, with no repo.changed delivery in between. subscriptionID is
	// unique per Subscribe call and never reused, so each call's own unsubscribe can only ever
	// find, remove and close its own entry.
	subs      map[subscriptionID]*subscriber
	nextSubID subscriptionID
	life      context.Context // cancelled first in teardown; bounds background passes
	lifeStop  context.CancelFunc
	tornDown  bool // set once, under mu, by teardown() (D4) — makes Subscribe/CatFile/teardown itself
	// safe against a concurrent subscribe or a second teardown call (F2/F3), guarded by the SAME
	// mu that already serialises subs.

	catfileMu sync.Mutex
	catfile   *catfile.Session

	detail *detailCache
	diff   *diffCache
	refs   *refsCache
	// stack is G26 D3/D16's own stack.list cache — one value per repository, dropped on
	// refsChanged and by invalidateAfterWrite exactly like refs.
	stack *stackCache
	// mergeBases is the cache — RangeFiles's two callers (review.files, review.fileDiff) re-spawned
	// merge-base uncached on every request; dropped on refsChanged and by invalidateAfterWrite
	// exactly like refs/stack (cache.go's own doc comment on mergeBaseCache).
	mergeBases *mergeBaseCache

	// cacheGen is F10's own generation counter (P108 Part 16 review): bumped by invalidateAfterWrite
	// and note's own refsChanged branch, alongside (and for the same reason as) every cache drop
	// those two already make. Nothing stops a read that started BEFORE a ref change from calling its
	// own cache.set()/setHead() AFTER that change already cleared the cache for it — serving
	// pre-change data even to the client's own refetch after the repo.changed event this same change
	// triggers, until the next ref change happens to clear it again. A reader captures
	// cacheGeneration() before spawning its own work and only writes back if it still matches
	// afterward — see CommitDetail/Stacks/mergeBase/Refs/statusAndInProgress's own call sites.
	cacheGen atomic.Uint64

	// headMu guards head/headStale, separate from mu (subs' own lock): every read/status/pre-flight
	// spawn touches head far more often than it touches the subscriber set.
	headMu    sync.Mutex
	head      gitclient.HeadState
	headStale bool

	// undo is SPEC §6's undo slot — one per repo, not per connection (D7). Its own mutex, following
	// this file's own cache pattern.
	undo *gitpreflight.UndoSlot

	// rangeCount is G6 D9's one-entry range-count slot (review.go) — a fact about the repository,
	// shared the same way detail/diff/refs are.
	rangeCount reviewRangeCountSlot

	// remoteOp is SPEC §6's own "active remote op (≤1)" box (G7 D9/D11/D20/D21).
	remoteOp opSlot
	// autoFetch is G7 D23's own background-fetch timer — one per repository.
	autoFetch autoFetchState
	// askPassMu/askPassChecked/askPassValue cache `git config --get core.askPass` for this entry's
	// whole life (G7 D10) — read once, lazily, on the first remote op.
	askPassMu      sync.Mutex
	askPassChecked bool
	askPassValue   string
	// settings is the server-owned settings accessor (protected-branch patterns, auto-fetch
	// minutes, and gitPath) — a plain func, not an interface, so this package
	// keeps importing only gitclient/gitaskpass/stdlib; threaded in by Registry.Acquire from its
	// own Registry.Settings field.
	settings func() (protectedBranches []string, autoFetchMinutes int, gitPath string)

	// repoSettingsGet is the per-repo settings accessor (the seven keys moved into
	// their own table) — threaded in by Registry.Acquire from Registry.RepoSettingsGet, read
	// through RepoSettings() below rather than directly, so a nil closure or a storage error both
	// fall back to the schema's own defaults instead of every caller re-deriving that fallback.
	repoSettingsGet func(repoID string) (model.GitRepoSettings, error)

	// review is G11's own review.db handle (D3/D14) — shared off the Registry, not per-connection
	// (D12: "I have reviewed X" is a fact about the repository and the person, not the window).
	review *gitreview.Store

	// host is the three-cache-plus-breaker slot (changerequests.go); hosts is the process-wide
	// selector threaded in by Registry.Acquire from Registry.Hosts, mirroring review's "shared off
	// the Registry" shape — one provider set, not one per repository.
	host  *hostState
	hosts githost.Selector

	// crSync keeps the persisted change request index in step with the host; nil without a store.
	crSync *crSyncer

	// isOpen is the cross-window query, threaded in from Registry.IsOpen — a plain func,
	// the same "this package imports only gitclient/gitreview/stdlib, so a cross-entry fact reaches
	// here as a closure, never as a *Registry field" discipline settings/repoSettingsGet already
	// follow. nil-safe: every call site checks for nil before calling (a RepoEntry built directly
	// by an older test never worries about worktree cross-window checks in the first place).
	isOpen func(repoID string) bool

	// restack is G26 D6/D9's own "≤1 restack run per repository" box — teardown force-cancels it
	// exactly like remoteOp.
	restack opSlot

	// opLog is the app-wide op log; nil turns logging off.
	opLog *oplog.Log

	done chan struct{}
}

// skipInitialAutoFetch is the construction-time opt-out: true only for
// Registry.AcquireQuiet, whose one caller (Conn.Open, for a Conn with noAutoFetch set — the native
// git-graph mount) must never be what arms a brand-new entry's auto-fetch timer. It affects ONLY
// this first-construction moment: a later, ordinary Open of the SAME (already-existing) entry still
// arms it normally through EnsureAutoFetch's own off→on path — this parameter never
// suppresses that, since it is read exactly once, here, and never stored on the entry itself.
func newRepoEntry(
	summary gitclient.RepoSummary, repo *gitclient.Repo, w Watcher,
	settings func() ([]string, int, string), repoSettingsGet func(string) (model.GitRepoSettings, error),
	review *gitreview.Store, hosts githost.Selector, isOpen func(string) bool,
	skipInitialAutoFetch bool, opLog *oplog.Log, crStore ChangeRequestStore, crCfg CRSyncConfig, crShare *crShared,
) *RepoEntry {
	life, lifeStop := context.WithCancel(oplog.WithOrigin(context.Background(), oplog.Origin{Module: oplog.ModuleGit, Trigger: oplog.TriggerAuto}))
	e := &RepoEntry{
		life:            life,
		lifeStop:        lifeStop,
		Summary:         summary,
		Repo:            repo,
		watcher:         w,
		subs:            make(map[subscriptionID]*subscriber),
		detail:          newDetailCache(),
		diff:            newDiffCache(diffCacheCapBytes),
		refs:            newRefsCache(),
		stack:           newStackCache(),
		mergeBases:      newMergeBaseCache(),
		head:            summary.Head,
		undo:            &gitpreflight.UndoSlot{},
		settings:        settings,
		repoSettingsGet: repoSettingsGet,
		review:          review,
		host:            newHostState(),
		hosts:           hosts,
		isOpen:          isOpen,
		opLog:           opLog,
		done:            make(chan struct{}),
	}
	// Seeds hasNonQuietAcquirer's own answer — a construction reached via Acquire starts
	// eligible for auto-fetch arming immediately; one reached via AcquireQuiet starts NOT eligible,
	// until some later real acquirer calls markAcquiredNonQuiet (Registry.acquire's reuse branch).
	// No lock needed here: e is not yet reachable from anywhere else.
	e.autoFetch.everAcquiredNonQuiet = !skipInitialAutoFetch
	if _, minutes, _ := settings(); minutes > 0 && !skipInitialAutoFetch {
		e.startAutoFetch(minutes)
	}
	if crStore != nil {
		e.crSync = newCRSyncer(e, crStore, crCfg, crShare)
		e.crSync.start()
	}
	// Last: pump reads e.crSync, so it must not start before the field is set.
	go e.pump()
	return e
}

// RepoSettings is the resolution point: entry.go's callers (graph.go/review.go/remote.go)
// use this instead of a hardcoded constant when a request's own optional field is empty. Falls
// back to the schema's own defaults (never errors, never panics) when repoSettingsGet is nil (a
// RepoEntry constructed directly by a test) or the storage read itself fails — the same "fail
// closed to a known-good value" discipline storage/repos.leaf already applies one layer down.
func (e *RepoEntry) RepoSettings() model.GitRepoSettings {
	if e.repoSettingsGet == nil {
		return model.DefaultGitRepoSettings()
	}
	s, err := e.repoSettingsGet(e.Summary.RepoID)
	if err != nil {
		return model.DefaultGitRepoSettings()
	}
	return s
}

// pump is the entry's watcher-draining goroutine: one signal in, fanned out to every current
// subscriber. It exits when the watcher's Signals channel closes (teardown calls watcher.Close,
// which is what closes it).
func (e *RepoEntry) pump() {
	defer close(e.done)
	for sig := range e.watcher.Signals() {
		e.note(sig)
	}
}

func (e *RepoEntry) note(sig gitclient.Signal) {
	// D7/D10/D16: dropped before the fan-out, exactly the ordering G3 D13 established for marking
	// a Walk stale — a client that reacts to repo.changed by re-requesting a detail, a ref list or
	// a fresh head must never be served the pre-change decoration.
	if sig == gitclient.SignalRefsChanged {
		// F10 (P108 Part 16 review): bumped alongside the drops below — see cacheGeneration's own
		// doc comment.
		e.cacheGen.Add(1)
		e.detail.dropAll()
		e.refs.drop()
		e.stack.drop()
		e.mergeBases.dropAll()
		e.rangeCount.drop()
		e.headMu.Lock()
		e.headStale = true
		e.headMu.Unlock()
		// The snapshot, per-branch and per-commit host caches turn stale (served while a background
		// refresh runs) and the host detection is dropped; the breaker is not. The bounded, gated
		// eager re-resolve pass is scheduled AFTER the mark, in its own goroutine, so it never
		// delays this signal's fan-out to subscribers.
		e.host.markStale()
		go e.eagerResolveClosedBranches()
		if e.crSync != nil {
			e.crSync.onRefsChanged()
		}
	}
	e.mu.Lock()
	subs := make([]*subscriber, 0, len(e.subs))
	for _, s := range e.subs {
		subs = append(subs, s)
	}
	e.mu.Unlock()
	for _, s := range subs {
		s.note(sig)
	}
}

// Subscribe registers deliver for every future signal on this entry, wrapped in the coalescing
// subscriber so a slow deliver can never stall another subscriber or the watcher itself. The
// returned func unsubscribes and stops the subscriber's own goroutine; safe to call once. Returns
// a no-op unsubscribe, constructing no subscriber, when this entry has already been torn down
// (teardown may run concurrently with Open racing Registry.Close during app quit) — the
// caller (Conn.Open) then holds a ref on a dead entry for a moment and releases it normally.
func (e *RepoEntry) Subscribe(_ ConnID, deliver func(Event)) func() {
	e.mu.Lock()
	if e.tornDown {
		e.mu.Unlock()
		return func() {}
	}
	e.nextSubID++
	subID := e.nextSubID
	s := newSubscriber(e.Summary.RepoID, deliver)
	e.subs[subID] = s
	e.mu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			e.mu.Lock()
			// subID is unique to this one Subscribe call — never shared
			// with any other call, even a concurrent one for the same conn — so this identity check
			// can only ever match this call's own entry, never one a colliding call raced in over
			// it (the old ConnID-keyed bug) nor one teardown already removed and closed.
			present := e.subs[subID] == s
			if present {
				delete(e.subs, subID)
			}
			e.mu.Unlock()
			// Close only the subscriber this call actually removed from the map — teardown (F2b)
			// may have already deleted it and closed it itself, and closing an already-closed
			// subscriber's channel a second time is the double-close this guard exists to avoid.
			if present {
				s.close()
			}
		})
	}
}

// Head returns the entry's live head, re-resolving through gitclient.ResolveHead first if a
// refsChanged signal marked it stale — a lazy refresh, never eager: the extra two spawns
// this costs only happen when a ref changed AND the next reader is refs.list rather than
// status.get/RunOp, both of which set the head for free from their own spawn's output (setHead).
func (e *RepoEntry) Head(ctx context.Context) (gitclient.HeadState, error) {
	e.headMu.Lock()
	if !e.headStale {
		h := e.head
		e.headMu.Unlock()
		return h, nil
	}
	e.headMu.Unlock()

	gen := e.cacheGeneration()
	var h gitclient.HeadState
	err := e.Repo.Read(ctx, func(ctx context.Context) error {
		var rerr error
		h, rerr = gitclient.ResolveHead(ctx, e.Repo.Runner(), e.Repo.GitPath(), repoWorkingDir(e.Summary))
		return rerr
	})
	if err != nil {
		return gitclient.HeadState{}, err
	}
	if e.cacheGeneration() == gen {
		e.setHead(h)
	}
	return h, nil
}

// setHead writes a freshly-resolved head (from a status --branch header, or ResolveHead above) and
// clears the stale flag — called by statusAndInProgress and RunOp's own read-back, both of which
// already have a fresh head for free from their own spawn's output (D16).
func (e *RepoEntry) setHead(h gitclient.HeadState) {
	e.headMu.Lock()
	e.head = h
	e.headStale = false
	e.headMu.Unlock()
}

// invalidateAfterWrite drops exactly what a completed local write can have invalidated, without
// waiting for the watcher's own debounced signal: the same four drops note() makes on
// refsChanged. It is a second line of defence for OUR OWN writes, for the case a watch was lost
// (fsnotify's Add can fail with EMFILE, logs-and-skips; ErrEventOverflow is documented) — the
// watcher remains the ONLY thing that notices a write made outside this app, and the only thing
// that fans repo.changed out to subscribers. This emits no event (a second emitter would double
// every event a client sees) and marks no Walk stale (a Walk is per-connection; reaching from an
// entry into every connection's walks would invert the dependency the subscriber already owns).
// cacheGeneration returns the entry's current cache generation — see cacheGen's own doc
// comment on the RepoEntry struct.
func (e *RepoEntry) cacheGeneration() uint64 {
	return e.cacheGen.Load()
}

func (e *RepoEntry) invalidateAfterWrite() {
	// F10 (P108 Part 16 review): bumped alongside the drops below — see cacheGeneration's own doc
	// comment.
	e.cacheGen.Add(1)
	e.detail.dropAll()
	e.refs.drop()
	e.stack.drop()
	e.mergeBases.dropAll()
	e.rangeCount.drop()
	e.headMu.Lock()
	e.headStale = true
	e.headMu.Unlock()
}

// CatFile returns nil once this entry has been torn down (F3) — a lazy, unguarded construction
// here is exactly what let auto-fetch (and any other reader racing teardown) start a fresh
// `cat-file --batch` pair nothing would ever close. Callers treat nil as ErrRepoTornDown.
func (e *RepoEntry) CatFile() *catfile.Session {
	e.mu.Lock()
	tornDown := e.tornDown
	e.mu.Unlock()
	if tornDown {
		return nil
	}

	e.catfileMu.Lock()
	defer e.catfileMu.Unlock()
	if e.catfile == nil {
		dir := e.Summary.Root
		if e.Summary.IsBare {
			dir = e.Summary.GitDir
		}
		e.catfile = catfile.NewSession(catfile.Deps{
			Runner: e.Repo.Runner(), GitPath: e.Repo.GitPath(), Dir: dir,
		}, 0)
	}
	return e.catfile
}

// detachCatFile forgets the memoised cat-file session and returns it for the caller to Close
// outside any lock. Registry.release calls it under reg.mu so a re-acquirer always builds a fresh
// session rather than receiving one about to be closed.
func (e *RepoEntry) detachCatFile() *catfile.Session {
	e.catfileMu.Lock()
	defer e.catfileMu.Unlock()
	s := e.catfile
	e.catfile = nil
	return s
}

// closeCatFile closes and forgets the memoised cat-file session, if one exists — called by
// teardown. It restarts lazily on the next use.
func (e *RepoEntry) closeCatFile() {
	if s := e.detachCatFile(); s != nil {
		s.Close()
	}
}

// errIfTornDown is the check a slot claimer runs after claiming: teardown's forceCancel finds
// nothing claimed later, so a claim made after teardown would run uncancellable on a dead entry.
func (e *RepoEntry) errIfTornDown() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.tornDown {
		return ErrRepoTornDown
	}
	return nil
}

// teardown stops the watcher, waits for pump to drain, stops every remaining subscriber, and
// closes the cat-file session if one was ever started — called by Registry once refcount and
// linger both say the entry is done, or unconditionally by Registry.Close() at shutdown
// regardless of refcount. Idempotent: Registry.Close no longer needs to be the only
// caller, since a concurrent Subscribe/CatFile now sees tornDown rather than racing the
// map/session this function clears.
func (e *RepoEntry) teardown() {
	e.mu.Lock()
	if e.tornDown {
		e.mu.Unlock()
		return
	}
	e.tornDown = true
	subs := e.subs
	e.subs = make(map[subscriptionID]*subscriber) // never nil (F2a) — a Subscribe losing this race must
	// find a real, writable-looking map rather than panic; it is refused by the tornDown check
	// above before it would ever write into it.
	e.mu.Unlock()
	e.lifeStop()

	e.stopAutoFetch()
	if e.crSync != nil {
		e.crSync.stop()
	}
	e.remoteOp.forceCancel()
	e.restack.forceCancel() // G26 D6/D9: a restack in flight is killed, not left orphaned.
	_ = e.watcher.Close()
	<-e.done

	e.closeCatFile()

	e.detail.dropAll()
	e.diff.clear()
	e.refs.drop()
	e.stack.drop()
	e.mergeBases.dropAll()
	e.rangeCount.drop()
	e.undo.Set(nil)

	for _, s := range subs {
		s.close()
	}
}
