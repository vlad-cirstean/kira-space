package ade

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	lru "github.com/hashicorp/golang-lru/v2"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitclient"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitsession"
)

// rebasecheck.go is "would this branch conflict if rebased onto its latest base". git is the
// only merge engine: gitsession.RepoEntry.MergeTreeConflicts runs `git merge-tree --write-tree`
// (git >= 2.38), which never touches the worktree, index, HEAD or refs, so it is safe next to
// running agents. Results are cached per repo by (baseTip, tip) object ids, so a moved base or tip
// invalidates itself. Failures are never cached as a clean result. A pair check (retries > 0)
// that fails is retried with backoff; meanwhile it reads as checking, and after the last attempt it
// stays failed until Refresh, so a real conflict never hides behind a transient failure.

const (
	rebaseCacheCap    = 512
	rebaseConcurrency = 4
	// rebaseRetryBase is the first retry delay of a failed pair check; each retry waits 4x longer.
	rebaseRetryBase = 5 * time.Second
	// pairCheckRetries is how many times a failed pair check is retried.
	pairCheckRetries = 3

	conflictChecking = "checking"
	conflictDone     = "done"
	conflictFailed   = "failed"
)

type rebaseKey struct{ BaseTip, Tip string }

type rebaseCache = lru.Cache[rebaseKey, []string]

func newRebaseCache() *rebaseCache {
	c, _ := lru.New[rebaseKey, []string](rebaseCacheCap)
	return c
}

// rebaseState is one branch's check state as the board wire reports it.
type rebaseState struct {
	Check  string // conflictChecking | conflictDone | conflictFailed
	Paths  []string
	Reason string // set only when Check == conflictFailed
}

// rebaseJob is one (repo, baseTip, tip) check. run is the merge-tree call; label fields feed the log.
type rebaseJob struct {
	repoID string
	key    rebaseKey
	cache  *rebaseCache
	run    func(ctx context.Context, baseTip, tip string) ([]string, error)
	branch string
	base   string
	// retries is how many times a failed check is retried automatically; 0 reports at once.
	retries int
}

// checkFailure is a recorded failed job. A non-zero retryAt means a retry is pending: the job
// reads as checking until then and as unknown (so the next request retries) after.
type checkFailure struct {
	reason   string
	at       time.Time
	attempts int
	retryAt  time.Time
}

type jobID struct {
	repoID string
	key    rebaseKey
}

// rebaseChecker runs checks with one board-wide semaphore and dedupes in-flight work by key.
type rebaseChecker struct {
	ctx       context.Context
	gitStatus func(context.Context) gitclient.GitStatus
	onDone    func(repoID string)
	sem       chan struct{}

	mu       sync.Mutex
	inflight map[jobID]chan struct{}
	failed   map[jobID]checkFailure
	closed   bool
	// retryBase overrides rebaseRetryBase in tests.
	retryBase time.Duration
	wg        sync.WaitGroup // running checks; close waits for them
}

func newRebaseChecker(ctx context.Context, gitStatus func(context.Context) gitclient.GitStatus, onDone func(repoID string)) *rebaseChecker {
	return &rebaseChecker{
		ctx: ctx, gitStatus: gitStatus, onDone: onDone,
		sem:      make(chan struct{}, rebaseConcurrency),
		inflight: map[jobID]chan struct{}{},
		failed:   map[jobID]checkFailure{},
	}
}

// peek reports the current state without starting work: ok is false when nothing is cached, running
// or recorded as failed (the caller then calls request).
func (c *rebaseChecker) peek(repoID string, cache *rebaseCache, key rebaseKey) (rebaseState, bool) {
	if paths, ok := cache.Get(key); ok {
		return rebaseState{Check: conflictDone, Paths: paths}, true
	}
	id := jobID{repoID, key}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, running := c.inflight[id]; running {
		return rebaseState{Check: conflictChecking, Paths: []string{}}, true
	}
	if f, bad := c.failed[id]; bad {
		if !f.retryAt.IsZero() {
			if time.Now().Before(f.retryAt) {
				return rebaseState{Check: conflictChecking, Paths: []string{}}, true
			}
			return rebaseState{}, false // retry due: the caller requests again
		}
		return rebaseState{Check: conflictFailed, Paths: []string{}, Reason: f.reason}, true
	}
	return rebaseState{}, false
}

// forgetFailures lets the next request retry every failed check of a repo (Refresh calls it).
func (c *rebaseChecker) forgetFailures(repoID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for id := range c.failed {
		if id.repoID == repoID {
			delete(c.failed, id)
		}
	}
}

// request starts the check unless it is cached or already running. The returned channel closes
// when the result (or failure) is recorded; it is already closed on a cache hit.
func (c *rebaseChecker) request(j rebaseJob) <-chan struct{} {
	start := time.Now()
	if paths, ok := j.cache.Get(j.key); ok {
		logRebase(j, "cached", len(paths), "", true, start)
		done := make(chan struct{})
		close(done)
		return done
	}
	id := jobID{j.repoID, j.key}
	c.mu.Lock()
	if done, running := c.inflight[id]; running {
		c.mu.Unlock()
		return done
	}
	done := make(chan struct{})
	if c.closed {
		c.mu.Unlock()
		close(done)
		return done
	}
	c.inflight[id] = done
	prior := c.failed[id].attempts
	delete(c.failed, id)
	c.wg.Add(1)
	c.mu.Unlock()

	go func() {
		defer c.wg.Done()
		paths, err := c.execute(j)
		c.mu.Lock()
		delete(c.inflight, id)
		if err != nil {
			c.recordFailure(id, j, err, prior+1)
		} else {
			j.cache.Add(j.key, paths)
		}
		c.mu.Unlock()
		if err != nil {
			logRebase(j, "failed", 0, err.Error(), false, start)
		} else {
			logRebase(j, "ok", len(paths), "", false, start)
		}
		close(done)
		if c.onDone != nil {
			c.onDone(id.repoID)
		}
	}()
	return done
}

// recordFailure stores a failed job, schedules its retry when the job allows one, and keeps the
// map bounded by dropping the oldest entries. The caller holds c.mu.
func (c *rebaseChecker) recordFailure(id jobID, j rebaseJob, err error, attempts int) {
	f := checkFailure{reason: err.Error(), at: time.Now(), attempts: attempts}
	if attempts <= j.retries && !c.closed {
		base := c.retryBase
		if base <= 0 {
			base = rebaseRetryBase
		}
		delay := base << (2 * (attempts - 1))
		f.retryAt = f.at.Add(delay)
		time.AfterFunc(delay, func() {
			c.mu.Lock()
			closed := c.closed
			c.mu.Unlock()
			if !closed && c.onDone != nil {
				c.onDone(id.repoID) // rebuild the board so its next pair pass retries
			}
		})
	}
	c.failed[id] = f
	for len(c.failed) > rebaseCacheCap {
		var oldest jobID
		var at time.Time
		for k, v := range c.failed {
			if at.IsZero() || v.at.Before(at) {
				oldest, at = k, v.at
			}
		}
		delete(c.failed, oldest)
	}
}

// close refuses new checks and waits for the running ones; cancel the checker's ctx first.
func (c *rebaseChecker) close() {
	c.mu.Lock()
	c.closed = true
	c.mu.Unlock()
	c.wg.Wait()
}

// mergeTreeRun binds a job's run func to the repo entry: the one `git merge-tree` caller.
func mergeTreeRun(entry *gitsession.RepoEntry) func(ctx context.Context, baseTip, tip string) ([]string, error) {
	return func(ctx context.Context, baseTip, tip string) ([]string, error) {
		pred, err := entry.MergeTreeConflicts(ctx, baseTip, tip)
		if err != nil {
			return nil, err
		}
		return pred.Paths, nil
	}
}

// execute gates on the git version, then runs merge-tree under the semaphore.
func (c *rebaseChecker) execute(j rebaseJob) ([]string, error) {
	select {
	case c.sem <- struct{}{}:
		defer func() { <-c.sem }()
	case <-c.ctx.Done():
		return nil, c.ctx.Err()
	}
	if st := c.gitStatus(c.ctx); st.Kind != "ok" {
		return nil, fmt.Errorf("%s", gitUnusableReason(st))
	}
	paths, err := j.run(c.ctx, j.key.BaseTip, j.key.Tip)
	if err != nil {
		return nil, fmt.Errorf("%s", firstLine(err.Error()))
	}
	if paths == nil {
		paths = []string{}
	}
	return paths, nil
}

func gitUnusableReason(st gitclient.GitStatus) string {
	if st.Kind == "tooOld" {
		return fmt.Sprintf("git %s is older than %s; merge-tree --write-tree needs %s", st.Detected, st.Required, st.Required)
	}
	if st.Reason != "" {
		return st.Reason
	}
	return "git not found"
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

func logRebase(j rebaseJob, outcome string, conflicts int, reason string, cached bool, start time.Time) {
	result := "clean"
	switch {
	case outcome == "failed":
		result = "failed"
	case conflicts > 0:
		result = fmt.Sprintf("conflicts(%d)", conflicts)
	}
	attrs := []any{"repo", j.repoID, "branch", j.branch, "base", j.base, "baseTip", gitclient.ShortSHA(j.key.BaseTip),
		"tip", gitclient.ShortSHA(j.key.Tip), "result", result, "cached", cached, "ms", time.Since(start).Milliseconds()}
	rebaseLog := slog.With("scope", "ade")
	if outcome == "failed" {
		rebaseLog.Warn("ade rebase check", append(attrs, "reason", reason)...)
		return
	}
	rebaseLog.Info("ade rebase check", attrs...)
}
