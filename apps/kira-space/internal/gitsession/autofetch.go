package gitsession

import (
	"context"
	"errors"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/oplog"
	"strings"
	"sync"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitops"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitpreflight"
	"github.com/kirathecat/kira-studio/internal/kiratime"
	"github.com/kirathecat/kira-studio/internal/notify"
)

// AutoFetchChange is the event payload for a stop (non-nil) or a re-arm (nil).
type AutoFetchChange struct{ AutoFetch *gitpreflight.AutoFetchStatus }

type autoFetchVerdict int

const (
	verdictBusy autoFetchVerdict = iota
	verdictTransient
	verdictPermanent
)

const (
	autoFetchBackoffCap = 60 * time.Minute
	// maxBackoffShift keeps interval<<failures from overflowing int64 (interval <= 1440 min).
	maxBackoffShift = 16
)

// autoFetchOutcome classifies a failed background fetch's error kind. Unknown kinds stop: the app
// cannot tell, and silent endless retry would hide a real problem.
func autoFetchOutcome(kind string) autoFetchVerdict {
	switch kind {
	case "OperationInProgress", "Cancelled":
		return verdictBusy
	case "NetworkFailed", "LockHeld":
		return verdictTransient
	}
	return verdictPermanent
}

// nextAutoFetchDelay doubles the interval per consecutive transient failure, capped at an hour (or
// the interval itself when that is longer). No attempt limit: an offline night must not stop it.
func nextAutoFetchDelay(interval time.Duration, failures int) time.Duration {
	if failures <= 0 {
		return interval
	}
	ceiling := max(interval, autoFetchBackoffCap)
	d := interval << min(failures, maxBackoffShift)
	if d <= 0 || d > ceiling {
		return ceiling
	}
	return d
}

// autoFetchState is one RepoEntry's own background-fetch timer (D23) — one per repository,
// regardless of how many windows have it open, silent, credential-free and self-stopping.
type autoFetchState struct {
	mu    sync.Mutex
	timer *time.Timer
	// disabled is teardown's permanent flag; stopped is a failure stop that a successful explicit
	// fetch or pull clears (rearmAutoFetch).
	disabled bool
	stopped  *gitpreflight.AutoFetchStatus
	failures int
	changed  notify.Emitter[AutoFetchChange]
	// everAcquiredNonQuiet is C14-3's own gate: true once at least one non-quiet (real, e.g. a
	// paired external client) acquirer has held this entry — set by markAcquiredNonQuiet, read by
	// EnsureAutoFetch. See markAcquiredNonQuiet's own comment for why this exists.
	everAcquiredNonQuiet bool
}

// startAutoFetch arms the timer if minutes > 0 — called once, by newRepoEntry, with whatever the
// server-owned interval reads as at entry-creation time. The first tick always waits a full
// interval (D23: a tick racing a window's cold start would compete with the initial graph load).
func (e *RepoEntry) startAutoFetch(minutes int) {
	if minutes <= 0 {
		return
	}
	e.autoFetch.mu.Lock()
	defer e.autoFetch.mu.Unlock()
	if e.autoFetch.disabled || e.autoFetch.stopped != nil || e.autoFetch.timer != nil {
		return
	}
	e.autoFetch.timer = time.AfterFunc(time.Duration(minutes)*time.Minute, e.autoFetchTick)
}

// EnsureAutoFetch re-reads this entry's current settings and arms the timer if the interval is now
// non-zero — called from Conn.Open on the path that stores a new hold, and from
// Registry.ReconcileAutoFetch, the OTHER off→on path: a user flipping fetch.autoInterval from 0
// back to a positive value while the repository is already open, with no repo.open in between to
// reach this any other way (ReconcileAutoFetch's own comment traces the rest of that call chain up
// to bridge/settings.go).
// startAutoFetch already no-ops when a timer is already running, the entry is `disabled` (torn
// down) or `stopped` (permanent failure, cleared only by rearmAutoFetch), so calling this
// redundantly (both an open AND a settings change, or several windows) arms exactly one timer.
// Exported for registry.go's own cross-file call; unexported callers within this package (conn.go)
// use it exactly the same way.
//
// Also no-ops entirely when this entry has never had a non-quiet acquirer (see
// markAcquiredNonQuiet) — otherwise ReconcileAutoFetch, which arms every constructed entry with no
// memory of how each was acquired, could arm a repository the native git-graph/review UI alone has
// ever opened (AcquireQuiet), defeating that surface's own "provably read-only" guarantee
// the moment the user set a positive fetch interval in Settings.
func (e *RepoEntry) EnsureAutoFetch() {
	if !e.hasNonQuietAcquirer() {
		return
	}
	_, minutes, _ := e.settings()
	if minutes > 0 {
		e.startAutoFetch(minutes)
	}
}

// markAcquiredNonQuiet records that this entry has now had at least one non-quiet (real) acquirer
// — called by Registry.acquire whenever a call reaches it through Acquire, never AcquireQuiet,
// whether that construction is brand-new or a reuse of an entry AcquireQuiet built. Idempotent and
// one-directional: once set, never cleared — a repository that has ever had a real acquirer stays
// eligible for auto-fetch arming for the rest of this entry's life, even if every current
// connection happens to be quiet right now.
func (e *RepoEntry) markAcquiredNonQuiet() {
	e.autoFetch.mu.Lock()
	e.autoFetch.everAcquiredNonQuiet = true
	e.autoFetch.mu.Unlock()
}

func (e *RepoEntry) hasNonQuietAcquirer() bool {
	e.autoFetch.mu.Lock()
	defer e.autoFetch.mu.Unlock()
	return e.autoFetch.everAcquiredNonQuiet
}

// stopAutoFetch is teardown's own call — permanent, the entry is going away.
func (e *RepoEntry) stopAutoFetch() {
	e.autoFetch.mu.Lock()
	defer e.autoFetch.mu.Unlock()
	if e.autoFetch.timer != nil {
		e.autoFetch.timer.Stop()
		e.autoFetch.timer = nil
	}
	e.autoFetch.disabled = true
}

func (e *RepoEntry) rescheduleAutoFetch(delay time.Duration) {
	e.autoFetch.mu.Lock()
	defer e.autoFetch.mu.Unlock()
	if e.autoFetch.disabled || e.autoFetch.stopped != nil {
		return
	}
	e.autoFetch.timer = time.AfterFunc(delay, e.autoFetchTick)
}

func (e *RepoEntry) disableAutoFetch() {
	e.autoFetch.mu.Lock()
	e.autoFetch.disabled = true
	e.autoFetch.timer = nil
	e.autoFetch.mu.Unlock()
}

// pauseAutoFetch stops the ticking loop for a user-set interval of zero. Clearing only `timer`
// (never `disabled` or `stopped`) lets the next EnsureAutoFetch arm a fresh timer once the interval
// reads positive again.
func (e *RepoEntry) pauseAutoFetch() {
	e.autoFetch.mu.Lock()
	e.autoFetch.timer = nil
	e.autoFetch.mu.Unlock()
}

// autoFetchTick re-reads the server-owned interval fresh (a setting change takes effect within one
// interval) and, when nothing else is using the repository, runs one silent fetch through the SAME
// RunRemote path an explicit fetch takes — with conn == nil, which makes it credential-free
// structurally: no askpass env, no progress emission, never the undo slot. Busy right now
// reschedules; settleAutoFetch decides what each failure kind means.
func (e *RepoEntry) autoFetchTick() {
	if e.autoFetchDisabled() {
		return
	}

	_, minutes, _ := e.settings()
	if minutes <= 0 {
		e.pauseAutoFetch()
		return
	}
	interval := time.Duration(minutes) * time.Minute
	if e.Repo.Writing() {
		e.rescheduleAutoFetch(interval)
		return
	}

	remote, ok := e.pickAutoFetchRemote(autoFetchCtx)
	if !ok {
		e.rescheduleAutoFetch(interval)
		return
	}

	if e.autoFetchDisabled() { // teardown may have stopped auto-fetch while the remote was picked.
		return
	}
	result, err := e.RunRemote(context.WithoutCancel(autoFetchCtx), nil, RemoteOpParams{
		Kind: "fetch", Remote: remote, Prune: true,
	}, RemoteDeps{})
	e.settleAutoFetch(interval, remote, result, err)
}

func (e *RepoEntry) settleAutoFetch(interval time.Duration, remote string, result RemoteOpResult, err error) {
	switch {
	case errors.Is(err, ErrRepoTornDown):
		e.disableAutoFetch()
		return
	case err != nil:
		e.stopAutoFetchFor(remote, "Unknown", err.Error())
		return
	case result.OK:
		e.autoFetch.mu.Lock()
		e.autoFetch.failures = 0
		e.autoFetch.mu.Unlock()
		e.rescheduleAutoFetch(interval)
		return
	}
	kind, msg := "Unknown", ""
	if result.Error != nil {
		kind, msg = result.Error.Kind, result.Error.Message
	}
	switch autoFetchOutcome(kind) {
	case verdictBusy:
		e.rescheduleAutoFetch(interval)
	case verdictTransient:
		e.autoFetch.mu.Lock()
		e.autoFetch.failures++
		delay := nextAutoFetchDelay(interval, e.autoFetch.failures)
		e.autoFetch.mu.Unlock()
		e.rescheduleAutoFetch(delay)
	default:
		e.stopAutoFetchFor(remote, kind, msg)
	}
}

// stopAutoFetchFor stops the timer on a permanent failure, records the marker, and logs the stop
// once (never each retry).
func (e *RepoEntry) stopAutoFetchFor(remote, kind, message string) {
	line, _, _ := strings.Cut(strings.TrimSpace(message), "\n")
	status := &gitpreflight.AutoFetchStatus{State: "stopped", Kind: kind, Message: line, At: kiratime.NowISO()}
	e.autoFetch.mu.Lock()
	if e.autoFetch.disabled {
		e.autoFetch.mu.Unlock()
		return
	}
	e.autoFetch.stopped = status
	e.autoFetch.timer = nil
	e.autoFetch.failures = 0
	e.autoFetch.mu.Unlock()

	logMsg := remote + ": " + kind
	if line != "" {
		logMsg += " — " + line
	}
	e.recordFailure(e.life, "autoFetch", "Auto-fetch", logMsg)
	e.autoFetch.changed.Emit(AutoFetchChange{AutoFetch: status})
}

// rearmAutoFetch clears a failure stop after a successful explicit fetch or pull, then re-arms
// through EnsureAutoFetch (keeps the non-quiet gate).
func (e *RepoEntry) rearmAutoFetch() {
	e.autoFetch.mu.Lock()
	wasStopped := e.autoFetch.stopped != nil
	e.autoFetch.stopped = nil
	e.autoFetch.failures = 0
	e.autoFetch.mu.Unlock()
	if wasStopped {
		e.autoFetch.changed.Emit(AutoFetchChange{})
	}
	e.EnsureAutoFetch()
}

// autoFetchStatus is the current stop marker, nil while running, paused or never armed.
func (e *RepoEntry) autoFetchStatus() *gitpreflight.AutoFetchStatus {
	e.autoFetch.mu.Lock()
	defer e.autoFetch.mu.Unlock()
	return e.autoFetch.stopped
}

func (e *RepoEntry) autoFetchDisabled() bool {
	e.autoFetch.mu.Lock()
	defer e.autoFetch.mu.Unlock()
	return e.autoFetch.disabled
}

// pickAutoFetchRemote is D23's own rule: "origin" if it exists, else the sole remote if there is
// exactly one, else the tick is skipped (no guessing among several).
func (e *RepoEntry) pickAutoFetchRemote(ctx context.Context) (string, bool) {
	remotes, err := metaGet(e.meta, "remotes", func() ([]string, error) {
		raw, err := e.runOne(ctx, gitops.RemotesArgs())
		if err != nil {
			return nil, err
		}
		var out []string
		for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
			if line != "" {
				out = append(out, line)
			}
		}
		return out, nil
	})
	if err != nil {
		return "", false
	}
	for _, r := range remotes {
		if r == "origin" {
			return "origin", true
		}
	}
	if len(remotes) == 1 {
		return remotes[0], true
	}
	return "", false
}

// autoFetchCtx stamps every spawn of an auto-fetch pass as Kira's own background work.
var autoFetchCtx = oplog.WithOrigin(context.Background(), oplog.Origin{
	Module: oplog.ModuleGit, Trigger: oplog.TriggerAuto, Label: "Auto-fetch", Background: true,
})
