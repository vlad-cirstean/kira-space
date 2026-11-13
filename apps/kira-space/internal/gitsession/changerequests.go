package gitsession

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/githost"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitops"
)

// Per-repo TTLs, and the LRU bound for the per-commit cache; shas are unbounded, unlike branches.
const (
	hostSnapshotTTL    = 5 * time.Minute
	hostBranchTTL      = 5 * time.Minute
	hostCommitTTL      = 10 * time.Minute
	hostCommitCacheCap = 512
	// hostStaleMax bounds how long a stale or expired entry may still be served while it refreshes
	// in the background; older entries count as a plain miss.
	hostStaleMax = time.Hour
)

type cacheState int

const (
	cacheMiss cacheState = iota
	cacheFresh
	cacheStale
)

// hostBreakerFallback is how long a rate-limited status blocks further host calls. The host calls
// request no response headers, so there is no reset time to parse.
const hostBreakerFallback = 15 * time.Minute

// ChangeRequestLookup is the shared shape of commit and branch change-request lookups; gitrpc's wire
// ChangeRequestLookup is a structural projection of it.
type ChangeRequestLookup struct {
	Kind           string // "ok" | "disabled" | "unavailable"
	ChangeRequests []githost.ChangeRequest
	Status         *githost.Status
}

func disabledResult() ChangeRequestLookup { return ChangeRequestLookup{Kind: "disabled"} }

func unavailableResult(status githost.Status) ChangeRequestLookup {
	s := status
	return ChangeRequestLookup{Kind: "unavailable", Status: &s}
}

func okResult(crs []githost.ChangeRequest) ChangeRequestLookup {
	if crs == nil {
		crs = []githost.ChangeRequest{}
	}
	return ChangeRequestLookup{Kind: "ok", ChangeRequests: crs}
}

type commitEntry struct {
	crs      []githost.ChangeRequest
	cachedAt time.Time
}

type branchEntry struct {
	cr       *githost.ChangeRequest
	cachedAt time.Time
}

// hostState is one RepoEntry's whole code-host slot: the repo-wide open snapshot, the per-branch
// cache, the per-commit LRU, this entry's resolved host, and the rate-limit breaker. One instance
// per RepoEntry, never shared across repositories or hosts.
type hostState struct {
	mu sync.Mutex

	// gen counts resetCaches calls; a fetch captures it before calling the host and drops its cache
	// write when it moved, so an answer from the previous host choice never lands in the new one.
	gen uint64

	// remoteChecked/setting/provider/repo/resolved cache the one-git-spawn host detection for this
	// entry's life, keyed by the setting it was resolved under. A refsChanged drops it (a remote
	// could have changed); a changed setting drops it and every cache below.
	//
	// refsStaleAt marks every entry cached before it as stale: still served, refreshed in the
	// background (stale-while-revalidate) instead of dropped. refreshing single-flights those
	// refreshes by key.
	remoteChecked bool
	setting       string
	provider      githost.Provider
	repo          githost.Repo
	resolved      bool
	refsStaleAt   time.Time
	refreshing    map[string]bool

	// remoteURLs caches every remote's URL, origin first (RemoteFor); dropped with the host detection.
	remoteURLs      []remoteURL
	remoteURLsValid bool

	snapshot      []githost.ChangeRequest
	snapshotAt    time.Time
	snapshotValid bool
	// snapshotFetch single-flights the snapshot fetch: non-nil while one goroutine's Open call is in
	// flight, so a concurrent cache-miss caller (the branch picker fires up to 6 branch lookups at
	// once on a cold cache) waits on the same fetch instead of starting its own.
	snapshotFetch *snapshotFetch

	branch map[string]branchEntry

	commit    map[string]commitEntry
	commitLRU []string // oldest first; hostCommitCacheCap-bounded.

	// blockedUntil/blockedStatus is the breaker, set only by a rate-limited status and deliberately
	// NOT cleared by a refsChanged.
	blockedUntil  time.Time
	blockedStatus githost.Status

	// lastEagerPurgeAt gates eagerPurgeAllowed — deliberately NOT cleared by a refsChanged: it must
	// survive the very signal that triggers the next eager pass.
	lastEagerPurgeAt time.Time
}

func newHostState() *hostState {
	s := &hostState{}
	s.resetCaches()
	return s
}

// resetCaches empties every cached fact and the breaker. Caller holds s.mu, or owns s alone.
func (s *hostState) resetCaches() {
	s.gen++
	s.snapshotFetch = nil
	s.remoteChecked = false
	s.remoteURLs, s.remoteURLsValid = nil, false
	s.setting = ""
	s.provider = nil
	s.repo = githost.Repo{}
	s.resolved = false
	s.refsStaleAt = time.Time{}
	s.refreshing = make(map[string]bool)
	s.snapshot = nil
	s.snapshotValid = false
	s.branch = make(map[string]branchEntry)
	s.commit = make(map[string]commitEntry)
	s.commitLRU = nil
	s.blockedUntil = time.Time{}
	s.blockedStatus = githost.Status{}
}

// snapshotFetch is one in-flight snapshot call's shared outcome: the leader fills in crs/status and
// closes done; every follower that arrived meanwhile waits on done and reads the same result.
type snapshotFetch struct {
	done   chan struct{}
	crs    []githost.ChangeRequest
	status githost.Status
}

// claimSnapshotFetch returns the fetch to wait on. leader is true for the one caller that runs the
// fetch and calls finishSnapshotFetch.
func (s *hostState) claimSnapshotFetch() (fetch *snapshotFetch, leader bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.snapshotFetch != nil {
		return s.snapshotFetch, false
	}
	f := &snapshotFetch{done: make(chan struct{})}
	s.snapshotFetch = f
	return f, true
}

// finishSnapshotFetch records the leader's result on fetch, wakes every follower, and clears
// s.snapshotFetch so the next cache miss starts a fresh fetch.
func (s *hostState) finishSnapshotFetch(fetch *snapshotFetch, crs []githost.ChangeRequest, status githost.Status) {
	fetch.crs = crs
	fetch.status = status
	s.mu.Lock()
	if s.snapshotFetch == fetch {
		s.snapshotFetch = nil
	}
	s.mu.Unlock()
	close(fetch.done)
}

// markStale is the refsChanged handler's call: the host detection is dropped (a remote could have
// changed) and every cached fact is marked stale, not dropped — it keeps serving while a background
// refresh replaces it. The breaker is untouched.
func (s *hostState) markStale() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.remoteChecked = false
	s.remoteURLsValid = false
	s.refsStaleAt = time.Now()
}

// classify reports how an entry cached at cachedAt may be used: fresh, served-while-refreshing, or
// a miss. Caller holds s.mu.
func (s *hostState) classify(cachedAt time.Time, ttl time.Duration) cacheState {
	if state := classifyAge(cachedAt, ttl); state != cacheFresh {
		return state
	}
	if cachedAt.Before(s.refsStaleAt) {
		return cacheStale
	}
	return cacheFresh
}

func classifyAge(cachedAt time.Time, ttl time.Duration) cacheState {
	age := time.Since(cachedAt)
	switch {
	case age >= hostStaleMax:
		return cacheMiss
	case age >= ttl:
		return cacheStale
	}
	return cacheFresh
}

// claimRefresh reports whether the caller owns the background refresh for key.
func (s *hostState) claimRefresh(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.refreshing[key] {
		return false
	}
	s.refreshing[key] = true
	return true
}

func (s *hostState) releaseRefresh(key string) {
	s.mu.Lock()
	delete(s.refreshing, key)
	s.mu.Unlock()
}

// breakerUntil is when the armed breaker lapses.
func (s *hostState) breakerUntil() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.blockedUntil
}

func (s *hostState) armBreaker(status githost.Status) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.blockedUntil = time.Now().Add(hostBreakerFallback)
	s.blockedStatus = status
}

// breakerStatus returns the cached status and true while the breaker is armed — every call inside
// that window is served from here with no host call.
func (s *hostState) breakerStatus() (githost.Status, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.blockedUntil.IsZero() || time.Now().After(s.blockedUntil) {
		return githost.Status{}, false
	}
	return s.blockedStatus, true
}

// eagerPurgeMinGap bounds how often eagerResolveClosedBranches may actually do its own work,
// independent of maxEagerPurgeBranches' per-pass branch cap: the refsChanged handler schedules a
// pass on every signal, and an interactive rebase or `git fetch --prune` sends bursts.
const eagerPurgeMinGap = 1 * time.Second

// eagerPurgeAllowed reports whether enough time has passed since the last eager pass that actually
// ran, and — if so — atomically claims this call as the one that runs.
func (s *hostState) eagerPurgeAllowed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.lastEagerPurgeAt.IsZero() && time.Since(s.lastEagerPurgeAt) < eagerPurgeMinGap {
		return false
	}
	s.lastEagerPurgeAt = time.Now()
	return true
}

func (s *hostState) commitCacheGet(sha string) ([]githost.ChangeRequest, cacheState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.commit[sha]
	if !ok {
		return nil, cacheMiss
	}
	return entry.crs, s.classify(entry.cachedAt, hostCommitTTL)
}

func (s *hostState) commitCachePut(gen uint64, sha string, crs []githost.ChangeRequest) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.gen != gen {
		return
	}
	if _, exists := s.commit[sha]; !exists {
		s.commitLRU = append(s.commitLRU, sha)
		for len(s.commitLRU) > hostCommitCacheCap {
			oldest := s.commitLRU[0]
			s.commitLRU = s.commitLRU[1:]
			delete(s.commit, oldest)
		}
	}
	s.commit[sha] = commitEntry{crs: crs, cachedAt: time.Now()}
}

func (s *hostState) branchCacheGet(branch string) (*githost.ChangeRequest, cacheState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.branch[branch]
	if !ok {
		return nil, cacheMiss
	}
	if entry.cr == nil {
		// A refs change cannot open a change request; only the snapshot refresh can reveal one.
		return nil, classifyAge(entry.cachedAt, hostBranchTTL)
	}
	return entry.cr, s.classify(entry.cachedAt, hostBranchTTL)
}

func (s *hostState) branchCachePut(gen uint64, branch string, cr *githost.ChangeRequest) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.gen != gen {
		return
	}
	s.branch[branch] = branchEntry{cr: cr, cachedAt: time.Now()}
}

func (s *hostState) snapshotGet() ([]githost.ChangeRequest, cacheState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.snapshotValid {
		return nil, cacheMiss
	}
	return s.snapshot, s.classify(s.snapshotAt, hostSnapshotTTL)
}

func (s *hostState) generation() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.gen
}

func (s *hostState) snapshotPut(gen uint64, crs []githost.ChangeRequest) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.gen != gen {
		return
	}
	s.snapshot = crs
	s.snapshotAt = time.Now()
	s.snapshotValid = true
}

// hostSetting reads this entry's host setting; a live change must take effect on the very next
// lookup, so there is no caching of its own.
func (e *RepoEntry) hostSetting() string {
	return e.RepoSettings().GitHostProvider
}

// hostRepo resolves the host and repository identity of this entry's "origin" remote under the
// current setting. It costs one git spawn (`git remote get-url origin`) per refsChanged, and none
// when the setting is off, so a repo with no matching host never triggers a host call. A changed
// setting, host or repository drops every cached fact so nothing crosses hosts.
func (e *RepoEntry) hostRepo(ctx context.Context) (githost.Provider, githost.Repo, bool) {
	p, repo, ok, _ := e.hostRepoGen(ctx)
	return p, repo, ok
}

// hostRepoGen is hostRepo plus the cache generation the answer belongs to, read under the same lock
// hold that reads or writes the resolution. A fetch passes it to its cache puts, so a put from a
// provider resolved before a reset never lands after it.
func (e *RepoEntry) hostRepoGen(ctx context.Context) (githost.Provider, githost.Repo, bool, uint64) {
	setting := e.hostSetting()
	e.host.mu.Lock()
	if e.host.setting != "" && e.host.setting != setting {
		e.host.resetCaches()
	}
	gen := e.host.gen
	if e.host.remoteChecked {
		p, repo, ok := e.host.provider, e.host.repo, e.host.resolved
		e.host.mu.Unlock()
		return p, repo, ok, gen
	}
	e.host.mu.Unlock()

	remoteURL := ""
	if setting != githost.SettingOff {
		res, err := e.runAllowingExit(ctx, gitops.RemoteGetURLArgs("origin"), 0, 128)
		if err == nil && res.ExitCode == 0 {
			remoteURL = strings.TrimSpace(string(res.Stdout))
		}
		if ctx.Err() != nil {
			// A cancelled spawn says nothing about the remote; caching "no host" would stick.
			return nil, githost.Repo{}, false, gen
		}
	}
	p, repo, ok := e.hosts.Resolve(ctx, setting, remoteURL)

	e.host.mu.Lock()
	defer e.host.mu.Unlock()
	if e.host.gen != gen {
		// A reset landed while resolving: this answer belongs to the old choice. Serve it under the
		// old generation so its puts drop, and leave the slot for the next lookup.
		return p, repo, ok, gen
	}
	if e.host.resolved && (!ok || e.host.provider.ID() != p.ID() || e.host.repo != repo) {
		e.host.resetCaches()
		gen = e.host.gen
	}
	e.host.remoteChecked = true
	e.host.setting = setting
	e.host.provider, e.host.repo, e.host.resolved = p, repo, ok
	return p, repo, ok, gen
}

// ensureSnapshot is the repo-wide open snapshot, fetched lazily on first need and cached for
// hostSnapshotTTL — the one bulk read this feature makes, serving the graph indicator's pre-seed,
// the branch-picker badges and search's synchronous match at once. Never called at repo open.
//
// A cold cache (repo open, or every hostSnapshotTTL expiry) is exactly when the client's fan-out
// fires up to 6 concurrent branch lookups to warm the branch picker. The single-flight below makes
// only the first caller (the leader) fetch; every other concurrent caller waits on that fetch and
// shares its result.
func (e *RepoEntry) ensureSnapshot(ctx context.Context) ([]githost.ChangeRequest, githost.Status) {
	// A complete persisted index is the snapshot: no host call, and it survives a restart.
	if p, repo, ok := e.hostRepo(ctx); ok {
		if crs, ok := e.indexedOpen(p, repo); ok {
			return crs, githost.Status{Kind: githost.KindOK}
		}
	}
	crs, state := e.host.snapshotGet()
	switch state {
	case cacheFresh:
		return crs, githost.Status{Kind: githost.KindOK}
	case cacheStale:
		e.refreshInBackground("snapshot", func(ctx context.Context) {
			if _, st := e.host.snapshotGet(); st == cacheStale {
				e.fetchSnapshot(ctx)
			}
		})
		return crs, githost.Status{Kind: githost.KindOK}
	}
	return e.fetchSnapshot(ctx)
}

// refreshInBackground runs fn once per key in the entry's own lifetime context; concurrent callers
// for the same key skip.
func (e *RepoEntry) refreshInBackground(key string, fn func(ctx context.Context)) {
	if !e.host.claimRefresh(key) {
		return
	}
	go func() {
		defer e.host.releaseRefresh(key)
		if e.life.Err() != nil {
			return
		}
		fn(e.life)
	}()
}

// fetchSnapshot always asks the host (single-flighted) and caches a successful answer.
func (e *RepoEntry) fetchSnapshot(ctx context.Context) ([]githost.ChangeRequest, githost.Status) {
	if status, armed := e.host.breakerStatus(); armed {
		return nil, status
	}
	p, repo, ok, gen := e.hostRepoGen(ctx)
	if !ok {
		return nil, githost.Status{Kind: "disabled"}
	}

	fetch, leader := e.host.claimSnapshotFetch()
	if !leader {
		<-fetch.done
		return fetch.crs, fetch.status
	}

	crs, status := p.Open(ctx, repo)
	if status.OK() {
		e.host.snapshotPut(gen, crs)
	} else if status.RateLimited {
		e.host.armBreaker(status)
	}
	e.host.finishSnapshotFetch(fetch, crs, status)
	if !status.OK() {
		return nil, status
	}
	return crs, status
}

// ResolveCommitChangeRequests is commit.resolveChangeRequests' orchestration: setting off or no
// matching host -> {kind:'disabled'}; a cached commit record -> served with no host call; the
// breaker armed -> the cached status with no host call; otherwise ForCommit, cached on a successful
// answer. Never returns a Go error: every outcome is a ChangeRequestLookup the RPC layer answers
// with directly, so a host hiccup never blocks or fails the request.
func (e *RepoEntry) ResolveCommitChangeRequests(ctx context.Context, sha string) ChangeRequestLookup {
	p, repo, ok, gen := e.hostRepoGen(ctx)
	if !ok {
		return disabledResult()
	}
	crs, state := e.host.commitCacheGet(sha)
	switch state {
	case cacheFresh:
		return okResult(crs)
	case cacheStale:
		e.refreshInBackground("commit:"+sha, func(ctx context.Context) {
			if _, st := e.host.commitCacheGet(sha); st == cacheStale {
				e.fetchCommitChangeRequests(ctx, p, repo, sha, gen)
			}
		})
		return okResult(crs)
	}
	return e.fetchCommitChangeRequests(ctx, p, repo, sha, gen)
}

func (e *RepoEntry) fetchCommitChangeRequests(ctx context.Context, p githost.Provider, repo githost.Repo, sha string, gen uint64) ChangeRequestLookup {
	if status, armed := e.host.breakerStatus(); armed {
		return unavailableResult(status)
	}

	crs, status := p.ForCommit(ctx, repo, sha)
	if !status.OK() {
		if status.RateLimited {
			e.host.armBreaker(status)
		}
		return unavailableResult(status)
	}
	e.host.commitCachePut(gen, sha, crs)
	return okResult(crs)
}

// ResolveBranchChangeRequest is branch.resolveChangeRequest's orchestration: a snapshot hit on
// head ref == branch answers with no call at all; a miss falls through to the per-branch query that
// includes closed ones — the only call that can report a change request that has since closed. A
// resolved closed or merged one purges that branch's review session through the review store's
// Purge seam; a purge failure is logged and never turned into an RPC error.
func (e *RepoEntry) ResolveBranchChangeRequest(ctx context.Context, branch string) ChangeRequestLookup {
	return e.resolveBranchChangeRequest(ctx, branch, false)
}

// resolveBranchChangeRequest serves stale cached facts while refreshing them, unless fresh is set
// (the eager purge pass needs the current state to see a closed change request).
func (e *RepoEntry) resolveBranchChangeRequest(ctx context.Context, branch string, fresh bool) ChangeRequestLookup {
	p, repo, ok, gen := e.hostRepoGen(ctx)
	if !ok {
		return disabledResult()
	}

	// A complete persisted index answers a branch it holds with no host call. The eager purge pass
	// (fresh) skips it: that pass needs the host's current state.
	if !fresh {
		if cr, ok := e.indexedBranch(p, repo, branch); ok {
			e.maybePurgeClosed(ctx, branch, &cr)
			return okResult([]githost.ChangeRequest{cr})
		}
	}

	// Reading the snapshot cache alone never populates it; ensureSnapshot does. On a cold cache the
	// first lookup pays one bulk Open call and warms the snapshot for every other branch with an
	// open change request; only a since-closed one, or the breaker or a fetch failure, falls
	// through to the per-branch query below.
	snapshotFn := e.ensureSnapshot
	if fresh {
		snapshotFn = e.fetchSnapshot
	}
	if snapshot, status := snapshotFn(ctx); status.OK() {
		for _, cr := range snapshot {
			// HeadRef alone is not enough: a fork's change request from a commonly-named branch
			// ("main", "master", "patch-1") would otherwise badge onto an unrelated local branch of
			// the same name. ForBranch filters by owner server-side; the repo-wide snapshot has no
			// per-branch owner to ask for, so the loop filters client-side.
			if cr.HeadRef == branch && cr.HeadOwner == repo.Owner {
				e.maybePurgeClosed(ctx, branch, &cr)
				return okResult([]githost.ChangeRequest{cr})
			}
		}
		// No open change request in the snapshot is not yet authoritative for "none at all": the
		// snapshot is open-only, so this still falls through to the per-branch query.
	}

	cached, state := e.host.branchCacheGet(branch)
	if state == cacheStale && fresh {
		state = cacheMiss
	}
	switch state {
	case cacheFresh:
		return branchResult(cached)
	case cacheStale:
		e.refreshInBackground("branch:"+branch, func(ctx context.Context) {
			if _, st := e.host.branchCacheGet(branch); st == cacheStale {
				e.fetchBranchChangeRequest(ctx, p, repo, branch, gen)
			}
		})
		return branchResult(cached)
	}
	return e.fetchBranchChangeRequest(ctx, p, repo, branch, gen)
}

// CachedBranchChangeRequest is the change request this entry already knows for branch, of any age:
// the persisted index, the repo-wide open snapshot or the per-branch cache. It never calls the host
// and never refreshes, so a cold cache answers false. A closed or merged one shows only while cached.
func (e *RepoEntry) CachedBranchChangeRequest(ctx context.Context, branch string) (githost.ChangeRequest, bool) {
	p, repo, ok := e.hostRepo(ctx)
	if !ok {
		return githost.ChangeRequest{}, false
	}
	if cr, ok := e.indexedBranch(p, repo, branch); ok {
		return cr, true
	}
	if snapshot, state := e.host.snapshotGet(); state != cacheMiss {
		for _, cr := range snapshot {
			if cr.HeadRef == branch && cr.HeadOwner == repo.Owner {
				return cr, true
			}
		}
	}
	if cr, state := e.host.branchCacheGet(branch); state != cacheMiss && cr != nil {
		return *cr, true
	}
	return githost.ChangeRequest{}, false
}

func branchResult(cr *githost.ChangeRequest) ChangeRequestLookup {
	if cr == nil {
		return okResult(nil)
	}
	return okResult([]githost.ChangeRequest{*cr})
}

func (e *RepoEntry) fetchBranchChangeRequest(ctx context.Context, p githost.Provider, repo githost.Repo, branch string, gen uint64) ChangeRequestLookup {
	if status, armed := e.host.breakerStatus(); armed {
		return unavailableResult(status)
	}

	crs, status := p.ForBranch(ctx, repo, branch)
	if !status.OK() {
		if status.RateLimited {
			e.host.armBreaker(status)
		}
		return unavailableResult(status)
	}

	var latest *githost.ChangeRequest
	if len(crs) > 0 {
		c := crs[0]
		latest = &c
	}
	e.host.branchCachePut(gen, branch, latest)
	e.reconcileBranch(p, repo, branch, crs)
	e.maybePurgeClosed(ctx, branch, latest)
	return branchResult(latest)
}

// ChangeRequestURL composes a change request's browser URL server-side, never trusting a URL the
// renderer might supply. false when the setting is off or no host matches the remote — the
// renderer then shows the number as plain text.
func (e *RepoEntry) ChangeRequestURL(ctx context.Context, number int) (string, bool) {
	p, repo, ok := e.hostRepo(ctx)
	if !ok {
		return "", false
	}
	return p.ChangeRequestURL(repo, number), true
}

// RepoURL is the Branch link's web root — same gates as ChangeRequestURL.
func (e *RepoEntry) RepoURL(ctx context.Context) (string, bool) {
	p, repo, ok := e.hostRepo(ctx)
	if !ok {
		return "", false
	}
	return p.RepoURL(repo), true
}

// BranchURL is the web page of one branch — same gates as ChangeRequestURL.
func (e *RepoEntry) BranchURL(ctx context.Context, branch string) (string, bool) {
	p, repo, ok := e.hostRepo(ctx)
	if !ok {
		return "", false
	}
	return p.BranchURL(repo, branch), true
}

// HostRepo is the provider and host repository of this entry's origin remote.
func (e *RepoEntry) HostRepo(ctx context.Context) (githost.Provider, githost.Repo, bool) {
	return e.hostRepo(ctx)
}

// HostInfo describes the host this entry resolves to; false when off or no host matches.
func (e *RepoEntry) HostInfo(ctx context.Context) (githost.Info, bool) {
	p, _, ok := e.hostRepo(ctx)
	if !ok {
		return githost.Info{}, false
	}
	return githost.Describe(p), true
}

// maxEagerPurgeBranches bounds the eager post-fetch re-resolve pass, triggered by refsChanged and
// nothing else (never a timer) — a repository with many stale review sessions never turns one
// signal into an unbounded burst of host calls.
const maxEagerPurgeBranches = 8

// eagerResolveClosedBranches is the eager purge, run in its own goroutine by the refsChanged
// handler AFTER the cache drop: skipped when the setting is off, no host matches, or the breaker is
// armed. It resolves at most maxEagerPurgeBranches branches that have a stored review
// session (a read, never a second lifecycle), reusing the snapshot exactly like
// ResolveBranchChangeRequest, and purges each closed or merged one through maybePurgeClosed. It is
// a background best-effort pass, not a request with a caller waiting on it.
func (e *RepoEntry) eagerResolveClosedBranches() {
	if !e.host.eagerPurgeAllowed() {
		return
	}
	if _, armed := e.host.breakerStatus(); armed {
		return
	}
	ctx := e.life
	if ctx.Err() != nil {
		return
	}
	if _, _, ok := e.hostRepo(ctx); !ok {
		return
	}
	branches, err := e.review.Branches(ctx, e.Summary.RepoID)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		slog.Warn("githost: list review branches for eager re-resolve", "scope", "gitsession", "repo", e.Summary.RepoID, "err", err)
		return
	}
	if len(branches) > maxEagerPurgeBranches {
		branches = branches[:maxEagerPurgeBranches]
	}
	for _, branch := range branches {
		if ctx.Err() != nil {
			return
		}
		e.resolveBranchChangeRequest(ctx, branch, true)
	}
}

// maybePurgeClosed is the eager-purge call site shared by both lookup paths: a resolved change
// request whose state is "closed" or "merged" purges that branch's review session via the review
// store's Purge — nothing else. A purge failure is a warning, never an RPC error.
func (e *RepoEntry) maybePurgeClosed(ctx context.Context, branch string, cr *githost.ChangeRequest) {
	if cr == nil || (cr.State != "closed" && cr.State != "merged") {
		return
	}
	if err := e.review.Purge(ctx, e.Summary.RepoID, branch); err != nil {
		slog.Warn("githost: purge review session", "scope", "gitsession", "repo", e.Summary.RepoID, "branch", branch, "err", err)
	}
}
