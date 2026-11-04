package gitsession

import (
	"cmp"
	"context"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v4"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitclient"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/githost"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// SignalChangeRequestsChanged is repo.changed's kind for a sync run that changed the persisted
// change request index. It carries no rows: the client re-reads the index.
const SignalChangeRequestsChanged gitclient.Signal = "changeRequestsChanged"

// ChangeRequestStore is the persisted change request index (storage/repos.GitHostChangeRequestsRepo).
type ChangeRequestStore interface {
	UpsertChangeRequests(repoKey string, rows []model.GitHostChangeRequest) (changed int, err error)
	DeleteChangeRequest(repoKey string, number int) (bool, error)
	NewestPerBranch(repoKey, headOwner string) ([]model.GitHostChangeRequest, error)
	ChangeRequestsForBranch(repoKey, headOwner, headRef string) ([]model.GitHostChangeRequest, error)
	OpenChangeRequests(repoKey string) ([]model.GitHostChangeRequest, error)
	SyncState(repoKey string) (model.GitHostSyncState, bool, error)
	PutSyncState(repoKey string, s model.GitHostSyncState) error
	DropRepo(repoKey string) error
	PruneIdle(before int64) error
}

// CRSyncConfig tunes the change request sync; a zero field takes its default.
type CRSyncConfig struct {
	// Disabled turns every automatic trigger off (startup, timer, refs change); an explicit
	// SyncChangeRequests still runs.
	Disabled bool
	// StartDelay is how long a new entry waits before its first run, so git work goes first.
	StartDelay time.Duration
	// MinGap is the least time between two runs started by a trigger.
	MinGap time.Duration
	// FocusedInterval and BlurredInterval are the periodic cadence by window focus.
	FocusedInterval time.Duration
	BlurredInterval time.Duration
	// ChecksInterval spaces refreshes of pending checks while focused; SweepInterval spaces
	// refreshes of every open change request.
	ChecksInterval time.Duration
	SweepInterval  time.Duration
	// Skew is how far before the watermark a delta keeps reading: updatedAt is not strictly
	// monotonic across writes landing in one second.
	Skew time.Duration
	// PageCap bounds the pages one run reads; a longer walk resumes on a later run.
	PageCap int
	// ContinueDelay is the wait before the next run of an unfinished walk.
	ContinueDelay time.Duration
	// BackoffInitial and BackoffMax bound the exponential wait after a failed run.
	BackoffInitial time.Duration
	BackoffMax     time.Duration
	// LowBudget is the remaining GraphQL budget under which a run stops until the reset.
	LowBudget int
	// IdleRetention is how long a repository's rows outlive its last sync.
	IdleRetention time.Duration
}

func (c CRSyncConfig) withDefaults() CRSyncConfig {
	def := func(v *time.Duration, d time.Duration) {
		if *v <= 0 {
			*v = d
		}
	}
	def(&c.StartDelay, 2*time.Second)
	def(&c.MinGap, 30*time.Second)
	def(&c.FocusedInterval, 5*time.Minute)
	def(&c.BlurredInterval, 15*time.Minute)
	def(&c.ChecksInterval, 2*time.Minute)
	def(&c.SweepInterval, 15*time.Minute)
	def(&c.Skew, 2*time.Minute)
	def(&c.ContinueDelay, 5*time.Second)
	def(&c.BackoffInitial, 30*time.Second)
	def(&c.BackoffMax, 15*time.Minute)
	def(&c.IdleRetention, 30*24*time.Hour)
	if c.PageCap <= 0 {
		c.PageCap = 10
	}
	if c.LowBudget <= 0 {
		c.LowBudget = 200
	}
	return c
}

const crPageSize = 100

// maxBranchChangeRequests is how many of one branch's requests the index lists, as ForBranch does.
const maxBranchChangeRequests = 5

// crSyncer keeps one entry's persisted change request index in step with the host. One run at a
// time; a trigger during a run asks for one more run after it.
type crSyncer struct {
	e     *RepoEntry
	store ChangeRequestStore
	cfg   CRSyncConfig
	share *crShared

	mu      sync.Mutex
	running bool
	again   bool
	stopped bool
	done    chan struct{}
	timer   *time.Timer
	focused bool
	// lastStart gates MinGap. notBefore is the backoff wait after a failure; rateUntil is the
	// host's rate limit reset, which even an explicit request waits for.
	lastStart time.Time
	notBefore time.Time
	rateUntil time.Time
	bo        *backoff.ExponentialBackOff
	// key is the repo key of the last run, to drop its rows if the host is switched off.
	key string
	// more is set by a run that stopped at its page cap, to run again sooner than the interval.
	more bool
}

func newCRSyncer(e *RepoEntry, store ChangeRequestStore, cfg CRSyncConfig, share *crShared) *crSyncer {
	cfg = cfg.withDefaults()
	bo := backoff.NewExponentialBackOff()
	bo.InitialInterval, bo.MaxInterval, bo.MaxElapsedTime = cfg.BackoffInitial, cfg.BackoffMax, 0
	bo.Reset()
	return &crSyncer{e: e, store: store, cfg: cfg, share: share, focused: true, bo: bo}
}

// start arms the first run.
func (s *crSyncer) start() {
	if s.cfg.Disabled {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.armLocked(s.cfg.StartDelay)
}

func (s *crSyncer) stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopped = true
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
	if s.key != "" {
		s.share.detach(s.key, s.e)
	}
}

func (s *crSyncer) interval() time.Duration {
	if s.focused {
		return s.cfg.FocusedInterval
	}
	return s.cfg.BlurredInterval
}

// armLocked replaces the timer. Caller holds s.mu.
func (s *crSyncer) armLocked(d time.Duration) {
	if s.stopped || s.cfg.Disabled {
		return
	}
	if s.timer != nil {
		s.timer.Stop()
	}
	s.timer = time.AfterFunc(d, s.tick)
}

func (s *crSyncer) tick() {
	if _, started := s.trigger(false); started {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.running {
		s.armLocked(s.waitLocked(time.Now()))
	}
}

// waitLocked is how long the timer should wait from now: the interval, or the gate if later.
func (s *crSyncer) waitLocked(now time.Time) time.Duration {
	d := s.interval()
	if gate := max(s.notBefore.Sub(now), s.rateUntil.Sub(now)); gate > d {
		d = gate
	}
	return d
}

// continueWaitLocked is the wait before a backfill continuation: past the min gap measured from
// the run's start, or trigger would refuse it and fall back to the full interval.
func (s *crSyncer) continueWaitLocked(now time.Time) time.Duration {
	return max(s.cfg.ContinueDelay, s.cfg.MinGap-now.Sub(s.lastStart), s.notBefore.Sub(now), s.rateUntil.Sub(now))
}

var closedChan = func() chan struct{} { c := make(chan struct{}); close(c); return c }()

// trigger starts a run, or joins the one in flight. started is false when nothing will run (a
// gate, the min gap, or a stopped syncer); done is then already closed. An explicit trigger skips
// the failure backoff but never the min gap or a host rate limit reset.
func (s *crSyncer) trigger(explicit bool) (done <-chan struct{}, started bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	if s.stopped || now.Before(s.rateUntil) {
		return closedChan, false
	}
	if s.running {
		s.again = true
		return s.done, true
	}
	if (!explicit && now.Before(s.notBefore)) || (!s.lastStart.IsZero() && now.Sub(s.lastStart) < s.cfg.MinGap) {
		return closedChan, false
	}
	s.running, s.lastStart, s.done = true, now, make(chan struct{})
	go s.loop(s.done)
	return s.done, true
}

func (s *crSyncer) loop(done chan struct{}) {
	for {
		s.runOnce()
		s.mu.Lock()
		now := time.Now()
		if s.again && !s.stopped && !now.Before(s.rateUntil) {
			s.again, s.lastStart = false, now
			s.mu.Unlock()
			continue
		}
		s.again, s.running = false, false
		wait := s.waitLocked(now)
		if s.more {
			wait = s.continueWaitLocked(now)
		}
		s.armLocked(wait)
		s.mu.Unlock()
		close(done)
		return
	}
}

// onRefsChanged runs a delta after a ref change, within the min gap.
func (s *crSyncer) onRefsChanged() {
	if s.cfg.Disabled {
		return
	}
	s.trigger(false)
}

// sync is the explicit request. focused nil or true syncs now; false only records the blur.
func (s *crSyncer) sync(ctx context.Context, focused *bool, wait bool) {
	s.mu.Lock()
	if focused != nil {
		s.focused = *focused
	}
	s.mu.Unlock()
	if focused != nil && !*focused {
		return
	}
	done, started := s.trigger(true)
	if !wait || !started {
		return
	}
	select {
	case <-done:
	case <-ctx.Done():
	}
}

// crRun is the state of one run.
type crRun struct {
	s       *crSyncer
	k       *crKey
	ctx     context.Context
	sp      githost.ChangeRequestSync
	p       githost.Provider
	repo    githost.Repo
	key     string
	st      model.GitHostSyncState
	nowMs   int64
	changed bool
}

func (s *crSyncer) runOnce() {
	e := s.e
	ctx := e.life
	if ctx.Err() != nil {
		return
	}
	s.mu.Lock()
	s.more = false
	s.mu.Unlock()

	p, repo, ok := e.hostRepo(ctx)
	if !ok {
		s.dropForgotten()
		return
	}
	sp, ok := p.(githost.ChangeRequestSync)
	if !ok {
		return
	}
	if _, armed := e.host.breakerStatus(); armed {
		s.holdUntil(e.host.breakerUntil())
		return
	}
	if status := p.Status(ctx, repo); !status.OK() {
		s.failed(status)
		return
	}

	key := crRepoKey(p, repo)
	s.mu.Lock()
	old := s.key
	s.key = key
	s.mu.Unlock()
	if old != "" && old != key {
		s.share.detach(old, e)
	}
	s.share.attach(key, e)
	// Held from the state read to the state write: a peer's run on this key cannot interleave.
	k, release, ok := s.share.acquire(ctx, key)
	if !ok {
		return
	}
	defer release()
	r := &crRun{s: s, k: k, ctx: ctx, sp: sp, p: p, repo: repo, key: key, nowMs: time.Now().UnixMilli()}
	if r.prepare() && r.sync() {
		s.succeeded()
	}
	if r.changed {
		s.noteKey(key)
	}
}

// noteKey tells every window on the repository that the index changed.
func (s *crSyncer) noteKey(key string) {
	for _, peer := range s.share.peers(key) {
		peer.noteChangeRequests()
	}
}

// crRepoKey names a repository across clones: provider|host|owner/name, lower-cased.
func crRepoKey(p githost.Provider, repo githost.Repo) string {
	return strings.ToLower(p.ID() + "|" + repo.Host + "|" + repo.Owner + "/" + repo.Name)
}

// dropForgotten removes the last key's rows when the host was switched off or no longer matches.
func (s *crSyncer) dropForgotten() {
	// Only a host the user switched off drops the index; a failed remote read must not.
	if s.e.life.Err() != nil || s.e.hostSetting() != githost.SettingOff {
		return
	}
	s.mu.Lock()
	key := s.key
	s.key = ""
	s.mu.Unlock()
	if key == "" {
		return
	}
	if err := s.store.DropRepo(key); err != nil {
		slog.Warn("githost: drop change request index", "scope", "gitsession", "repo", s.e.Summary.RepoID, "err", err)
		return
	}
	s.noteKey(key)
	s.share.detach(key, s.e)
}

func (s *crSyncer) failed(status githost.Status) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	s.notBefore = now.Add(s.bo.NextBackOff())
	if status.RateLimited {
		reset := status.ResetAt
		if reset.IsZero() {
			reset = now.Add(hostBreakerFallback)
		}
		s.rateUntil = reset
		s.notBefore = later(s.notBefore, reset)
	}
	slog.Warn("githost: change request sync", "scope", "gitsession", "repo", s.e.Summary.RepoID, "kind", status.Kind, "reason", status.Reason)
}

func (s *crSyncer) succeeded() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.bo.Reset()
	s.notBefore = time.Time{}
}

// holdUntil gates every call until reset.
func (s *crSyncer) holdUntil(reset time.Time) {
	s.mu.Lock()
	s.rateUntil = reset
	s.mu.Unlock()
}

func later(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}

// fail records a failed host call and ends the run.
func (r *crRun) fail(status githost.Status) bool {
	if status.RateLimited {
		r.s.e.host.armBreaker(status)
	}
	if status.Gone {
		r.dropRepo()
	}
	r.s.failed(status)
	return false
}

func (r *crRun) dropRepo() {
	if err := r.s.store.DropRepo(r.key); err != nil {
		slog.Warn("githost: drop change request index", "scope", "gitsession", "repo", r.s.e.Summary.RepoID, "err", err)
		return
	}
	r.changed = true
}

// storeErr logs a store failure and ends the run; the host side is fine, so no backoff.
func (r *crRun) storeErr(err error) bool {
	slog.Warn("githost: change request store", "scope", "gitsession", "repo", r.s.e.Summary.RepoID, "err", err)
	return false
}

// prepare loads the sync state and drops rows read as another account or an older layout.
func (r *crRun) prepare() bool {
	s := r.s
	account := r.p.Account(r.ctx, r.repo)
	st, have, err := s.store.SyncState(r.key)
	if err != nil {
		return r.storeErr(err)
	}
	if have && (st.Account != account || st.SchemaVersion < model.GitHostSchemaVersion) {
		r.dropRepo()
		have = false
	}
	if !have {
		st = model.GitHostSyncState{Account: account, SchemaVersion: model.GitHostSchemaVersion}
	}
	r.st = st
	return true
}

// sync runs the backfill while the index is incomplete, else a delta, then the checks refresh.
func (r *crRun) sync() bool {
	complete := r.st.BackfillComplete
	var ok bool
	if complete {
		ok = r.delta()
	} else {
		ok = r.backfill()
	}
	if !ok {
		return false
	}
	if r.st.BackfillComplete && !r.s.isStopped() && !r.lowBudgetStop() {
		if !r.refreshChecks() {
			return false
		}
	}
	if err := r.s.store.PutSyncState(r.key, r.st); err != nil {
		return r.storeErr(err)
	}
	return true
}

func (s *crSyncer) isStopped() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stopped
}

// lowBudgetStop reports whether a low-budget stop already gates the syncer.
func (r *crRun) lowBudgetStop() bool {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	return time.Now().Before(r.s.rateUntil)
}

// noteBudget stops the syncer until the reset when the remaining budget runs low.
func (r *crRun) noteBudget(rl githost.RateLimit) {
	if rl.ResetAt.IsZero() || rl.Remaining >= r.s.cfg.LowBudget {
		return
	}
	r.s.holdUntil(rl.ResetAt)
}

func (r *crRun) store(items []githost.ChangeRequest) bool {
	rows := make([]model.GitHostChangeRequest, 0, len(items))
	for _, cr := range items {
		rows = append(rows, crRow(cr, r.nowMs))
	}
	n, err := r.s.store.UpsertChangeRequests(r.key, rows)
	if err != nil {
		return r.storeErr(err)
	}
	if n > 0 {
		r.changed = true
	}
	return true
}

// backfill pages the whole history, newest update first, persisting the cursor after every page so
// a restart resumes. It reads at most PageCap pages, then yields.
func (r *crRun) backfill() bool {
	fresh := r.st.BackfillCursor == ""
	if fresh {
		_, etag, status := r.sp.Probe(r.ctx, r.repo, "")
		if !status.OK() {
			return r.fail(status)
		}
		r.st.ETag = etag
	}
	for range r.s.cfg.PageCap {
		page, status := r.sp.Page(r.ctx, r.repo, r.st.BackfillCursor, crPageSize)
		if !status.OK() {
			return r.fail(status)
		}
		items, reached := page.Items, false
		if r.st.DeltaFloor > 0 {
			var keep int
			keep, reached = deltaStop(items, r.st.DeltaFloor, r.s.cfg.Skew)
			items = items[:keep]
		}
		if !r.store(items) {
			return false
		}
		for _, cr := range items {
			r.st.WatermarkUpdatedAt = max(r.st.WatermarkUpdatedAt, cr.UpdatedAt)
		}
		if reached || !page.HasNext || page.EndCursor == "" {
			r.st.BackfillComplete, r.st.BackfillCursor, r.st.DeltaFloor = true, "", 0
			r.st.LastFullSyncAt, r.st.LastChecksAt = r.nowMs, r.nowMs
			r.k.lastPending = time.Now()
			break
		}
		r.st.BackfillCursor = page.EndCursor
		if err := r.s.store.PutSyncState(r.key, r.st); err != nil {
			return r.storeErr(err)
		}
		r.noteBudget(page.RateLimit)
		if r.lowBudgetStop() {
			return true
		}
	}
	if !r.st.BackfillComplete {
		r.s.mu.Lock()
		r.s.more = true
		r.s.mu.Unlock()
	}
	return true
}

// delta reads what changed since the last run: a conditional probe, then pages from the newest
// until the watermark. A walk that hits the page cap first falls back to a resumable backfill that
// stops at the pre-delta watermark (DeltaFloor).
func (r *crRun) delta() bool {
	changed, etag, status := r.sp.Probe(r.ctx, r.repo, r.st.ETag)
	if !status.OK() {
		return r.fail(status)
	}
	r.st.LastDeltaAt = r.nowMs
	if !changed {
		return true
	}
	walk := walkDelta(func(cursor string) (githost.ChangeRequestPage, githost.Status) {
		return r.sp.Page(r.ctx, r.repo, cursor, crPageSize)
	}, r.st.WatermarkUpdatedAt, r.s.cfg.Skew, r.s.cfg.PageCap)
	if !walk.status.OK() {
		return r.fail(walk.status)
	}
	if !r.store(walk.items) {
		return false
	}
	floor := r.st.WatermarkUpdatedAt
	for _, cr := range walk.items {
		r.st.WatermarkUpdatedAt = max(r.st.WatermarkUpdatedAt, cr.UpdatedAt)
	}
	r.st.ETag = etag
	if !walk.reached {
		r.st.BackfillComplete, r.st.BackfillCursor, r.st.DeltaFloor = false, walk.cursor, floor
		r.s.mu.Lock()
		r.s.more = true
		r.s.mu.Unlock()
	}
	r.noteBudget(walk.rate)
	return true
}

// deltaWalk is the outcome of walkDelta.
type deltaWalk struct {
	items []githost.ChangeRequest
	// reached reports whether the walk met an item older than the watermark (or the end).
	reached bool
	// cursor is where an unreached walk stopped.
	cursor string
	rate   githost.RateLimit
	status githost.Status
}

// walkDelta pages newest first until deltaStop says the rest is already ingested, or pageCap pages
// were read.
func walkDelta(page func(cursor string) (githost.ChangeRequestPage, githost.Status), watermark int64, skew time.Duration, pageCap int) deltaWalk {
	var w deltaWalk
	for range pageCap {
		pg, status := page(w.cursor)
		if !status.OK() {
			w.status = status
			return w
		}
		keep, done := deltaStop(pg.Items, watermark, skew)
		w.items = append(w.items, pg.Items[:keep]...)
		w.rate = pg.RateLimit
		if done || !pg.HasNext || pg.EndCursor == "" {
			w.reached, w.status = true, githost.Status{Kind: githost.KindOK}
			return w
		}
		w.cursor = pg.EndCursor
	}
	w.status = githost.Status{Kind: githost.KindOK}
	return w
}

// deltaStop cuts a newest-first page at the watermark: keep is how many leading items to ingest,
// done whether an older item ended the walk. An item exactly skew before the watermark is kept.
func deltaStop(items []githost.ChangeRequest, watermark int64, skew time.Duration) (keep int, done bool) {
	cutoff := watermark - skew.Milliseconds()
	for i, cr := range items {
		if cr.UpdatedAt < cutoff {
			return i, true
		}
	}
	return len(items), false
}

// refreshChecks re-reads review and checks state, which a change request's updatedAt does not
// reflect: pending ones while the window is focused, every open one on the sweep interval. An id the
// host no longer returns is a deleted or hidden change request, and its row goes.
func (r *crRun) refreshChecks() bool {
	s := r.s
	now := time.Now()
	s.mu.Lock()
	focused := s.focused
	s.mu.Unlock()
	pendingDue := focused && now.Sub(r.k.lastPending) >= s.cfg.ChecksInterval
	sweepDue := r.nowMs-r.st.LastChecksAt >= s.cfg.SweepInterval.Milliseconds()
	if !pendingDue && !sweepDue {
		return true
	}
	open, err := s.store.OpenChangeRequests(r.key)
	if err != nil {
		return r.storeErr(err)
	}
	var ids []string
	byID := map[string]int{}
	for _, row := range open {
		if row.NodeID == "" {
			continue
		}
		if sweepDue || (!row.Draft && (row.ChecksState == "PENDING" || row.ChecksState == "EXPECTED")) {
			ids = append(ids, row.NodeID)
			byID[row.NodeID] = row.Number
		}
	}
	if len(ids) > 0 {
		found, status := r.sp.Checks(r.ctx, r.repo, ids)
		if !status.OK() {
			return r.fail(status)
		}
		if !r.store(found) {
			return false
		}
		for _, cr := range found {
			delete(byID, cr.NodeID)
		}
		for _, number := range byID {
			gone, err := s.store.DeleteChangeRequest(r.key, number)
			if err != nil {
				return r.storeErr(err)
			}
			r.changed = r.changed || gone
		}
	}
	r.k.lastPending = now
	if sweepDue {
		r.st.LastChecksAt = r.nowMs
	}
	return true
}

func crRow(cr githost.ChangeRequest, nowMs int64) model.GitHostChangeRequest {
	state, draft := cr.State, cr.Draft
	if state == "draft" {
		state, draft = "open", true
	}
	return model.GitHostChangeRequest{
		Number: cr.Number, NodeID: cr.NodeID, HeadRef: cr.HeadRef, HeadOwner: cr.HeadOwner, HeadSha: cr.HeadSha,
		BaseRef: cr.BaseRef, State: state, Draft: draft, Title: cr.Title, URL: cr.URL,
		ReviewDecision: cr.ReviewDecision, ChecksState: cr.ChecksState, CommentCount: cr.CommentCount, Assignees: cr.Assignees,
		UpdatedAt: cr.UpdatedAt, SyncedAt: nowMs,
	}
}

func crFromRow(row model.GitHostChangeRequest) githost.ChangeRequest {
	state := row.State
	if state == "open" && row.Draft {
		state = "draft"
	}
	return githost.ChangeRequest{
		Number: row.Number, Title: row.Title, URL: row.URL, State: state, HeadRef: row.HeadRef, HeadSha: row.HeadSha,
		HeadOwner: row.HeadOwner, BaseRef: row.BaseRef, UpdatedAt: row.UpdatedAt, NodeID: row.NodeID,
		Draft: row.Draft, ReviewDecision: row.ReviewDecision, ChecksState: row.ChecksState,
		CommentCount: row.CommentCount, Assignees: row.Assignees,
	}
}

// BranchChangeRequest is one head branch's newest cached change request.
type BranchChangeRequest struct {
	Name          string
	ChangeRequest githost.ChangeRequest
}

// ChangeRequestIndex is the persisted index of a repository's change requests whose head lives in
// the repository itself (a fork's request from a commonly named branch is not this repo's branch),
// narrowed to branches that exist in refs plus open requests: a deleted merged branch has no tip.
type ChangeRequestIndex struct {
	// Complete reports the full history was ingested, so a branch missing from Branches has none.
	Complete bool
	// SyncedAt is the last full or delta sync, Unix milliseconds; 0 when never synced.
	SyncedAt int64
	Branches []BranchChangeRequest
}

// ChangeRequestIndex answers from SQLite at once, whatever the host is doing. A repository with no
// sync support, no host or no store answers an incomplete, empty index.
func (e *RepoEntry) ChangeRequestIndex(ctx context.Context) ChangeRequestIndex {
	store := e.crStore()
	if store == nil {
		return ChangeRequestIndex{}
	}
	p, repo, ok := e.hostRepo(ctx)
	if !ok {
		return ChangeRequestIndex{}
	}
	if _, ok := p.(githost.ChangeRequestSync); !ok {
		return ChangeRequestIndex{}
	}
	key := crRepoKey(p, repo)
	st, _, err := store.SyncState(key)
	if err != nil {
		slog.Warn("githost: read change request sync state", "scope", "gitsession", "repo", e.Summary.RepoID, "err", err)
		return ChangeRequestIndex{}
	}
	rows, err := store.NewestPerBranch(key, repo.Owner)
	if err != nil {
		slog.Warn("githost: read change request index", "scope", "gitsession", "repo", e.Summary.RepoID, "err", err)
		return ChangeRequestIndex{}
	}
	out := ChangeRequestIndex{Complete: st.BackfillComplete, SyncedAt: max(st.LastFullSyncAt, st.LastDeltaAt)}
	live := map[string]bool{}
	if refs, err := e.Refs(ctx); err == nil {
		for _, b := range refs.Branches {
			live[b.ShortName] = true
		}
		for _, b := range refs.RemoteBranches {
			if _, name, ok := strings.Cut(b.ShortName, "/"); ok {
				live[name] = true
			}
		}
	} else {
		// Without refs the merged and closed rows cannot be narrowed, so the list is partial.
		out.Complete = false
	}
	for _, row := range rows {
		if cr := crFromRow(row); live[row.HeadRef] || cr.State == "open" || cr.State == "draft" {
			out.Branches = append(out.Branches, BranchChangeRequest{Name: row.HeadRef, ChangeRequest: cr})
		}
	}
	return out
}

// SyncChangeRequests is branch.syncChangeRequests: sync now (within the min gap and rate limit
// gates), or with focused false only note that the window blurred. wait blocks until the run ends.
func (e *RepoEntry) SyncChangeRequests(ctx context.Context, focused *bool, wait bool) {
	if e.crSync != nil {
		e.crSync.sync(ctx, focused, wait)
	}
}

func (e *RepoEntry) crStore() ChangeRequestStore {
	if e.crSync == nil {
		return nil
	}
	return e.crSync.store
}

// noteChangeRequests fans changeRequestsChanged out to every subscriber.
func (e *RepoEntry) noteChangeRequests() {
	e.mu.Lock()
	subs := make([]*subscriber, 0, len(e.subs))
	for _, s := range e.subs {
		subs = append(subs, s)
	}
	e.mu.Unlock()
	for _, s := range subs {
		s.note(SignalChangeRequestsChanged)
	}
}

// indexedBranch answers a branch's change requests from the persisted index, when the sync
// is complete. ok is false when the index cannot answer: no sync, incomplete, or no row for the
// branch (a request opened since the last sync is then looked up on the host).
func (e *RepoEntry) indexedBranch(p githost.Provider, repo githost.Repo, branch string) (githost.ChangeRequest, bool) {
	store := e.crStore()
	if store == nil {
		return githost.ChangeRequest{}, false
	}
	if _, ok := p.(githost.ChangeRequestSync); !ok {
		return githost.ChangeRequest{}, false
	}
	key := crRepoKey(p, repo)
	if st, have, err := store.SyncState(key); err != nil || !have || !st.BackfillComplete {
		return githost.ChangeRequest{}, false
	}
	rows, err := store.ChangeRequestsForBranch(key, repo.Owner, branch)
	if err != nil || len(rows) == 0 {
		return githost.ChangeRequest{}, false
	}
	return crFromRow(rows[0]), true
}

// indexedOpen is the repo-wide open snapshot read from the persisted index, when the sync is
// complete.
func (e *RepoEntry) indexedOpen(p githost.Provider, repo githost.Repo) ([]githost.ChangeRequest, bool) {
	store := e.crStore()
	if store == nil {
		return nil, false
	}
	if _, ok := p.(githost.ChangeRequestSync); !ok {
		return nil, false
	}
	key := crRepoKey(p, repo)
	if st, have, err := store.SyncState(key); err != nil || !have || !st.BackfillComplete {
		return nil, false
	}
	rows, err := store.OpenChangeRequests(key)
	if err != nil {
		return nil, false
	}
	out := make([]githost.ChangeRequest, 0, len(rows))
	for _, row := range rows {
		out = append(out, crFromRow(row))
	}
	return out, true
}

// reconcileBranch drops stored rows a branch lookup proves gone. The lookup lists at most
// maxBranchChangeRequests, so with a full window only rows as new as its oldest entry count.
func (e *RepoEntry) reconcileBranch(p githost.Provider, repo githost.Repo, branch string, found []githost.ChangeRequest) {
	store := e.crStore()
	if store == nil {
		return
	}
	if _, ok := p.(githost.ChangeRequestSync); !ok {
		return
	}
	key := crRepoKey(p, repo)
	rows, err := store.ChangeRequestsForBranch(key, repo.Owner, branch)
	if err != nil || len(rows) == 0 {
		return
	}
	floor := int64(0)
	if len(found) >= maxBranchChangeRequests {
		floor = slices.MinFunc(found, func(a, b githost.ChangeRequest) int { return cmp.Compare(a.UpdatedAt, b.UpdatedAt) }).UpdatedAt
	}
	dropped := false
	for _, row := range rows {
		listed := slices.ContainsFunc(found, func(cr githost.ChangeRequest) bool { return cr.Number == row.Number })
		if listed || row.UpdatedAt < floor {
			continue
		}
		if gone, err := store.DeleteChangeRequest(key, row.Number); err == nil && gone {
			dropped = true
		}
	}
	if dropped {
		e.noteChangeRequests()
	}
}
