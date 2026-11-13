package ade

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitclient"
)

func okGit(context.Context) gitclient.GitStatus { return gitclient.GitStatus{Kind: "ok"} }

func waitFor(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatal("rebase check did not finish")
	}
}

func TestRebaseChecker_cacheKeyFailureAndVersionGate(t *testing.T) {
	var spawns atomic.Int32
	var failNext atomic.Bool
	run := func(_ context.Context, base, tip string) ([]string, error) {
		spawns.Add(1)
		if failNext.Load() {
			return nil, errors.New("fatal: bad object\nmore")
		}
		return []string{"a.txt"}, nil
	}
	status := gitclient.GitStatus{Kind: "ok"}
	c := newRebaseChecker(context.Background(), func(context.Context) gitclient.GitStatus { return status }, nil)
	cache := newRebaseCache()
	job := func(base, tip string) rebaseJob {
		return rebaseJob{repoID: "r", key: rebaseKey{base, tip}, cache: cache, run: run, branch: "b", base: "main"}
	}

	if _, known := c.peek("r", cache, rebaseKey{"b1", "t1"}); known {
		t.Fatal("peek of an unseen key must be unknown")
	}
	waitFor(t, c.request(job("b1", "t1")))
	waitFor(t, c.request(job("b1", "t1")))
	if spawns.Load() != 1 {
		t.Fatalf("same (baseTip, tip) spawned %d times, want 1", spawns.Load())
	}
	if st, _ := c.peek("r", cache, rebaseKey{"b1", "t1"}); st.Check != conflictDone || !slices.Equal(st.Paths, []string{"a.txt"}) {
		t.Fatalf("state %+v", st)
	}
	waitFor(t, c.request(job("b1", "t2")))
	if spawns.Load() != 2 {
		t.Fatalf("moved tip must re-check, spawns = %d", spawns.Load())
	}

	failNext.Store(true)
	waitFor(t, c.request(job("b2", "t1")))
	st, known := c.peek("r", cache, rebaseKey{"b2", "t1"})
	if !known || st.Check != conflictFailed || st.Reason != "fatal: bad object" || len(st.Paths) != 0 {
		t.Fatalf("failure state %+v", st)
	}
	if _, cached := cache.Get(rebaseKey{"b2", "t1"}); cached {
		t.Fatal("failure was cached")
	}
	failNext.Store(false)
	c.forgetFailures("r")
	waitFor(t, c.request(job("b2", "t1")))
	if st, _ := c.peek("r", cache, rebaseKey{"b2", "t1"}); st.Check != conflictDone {
		t.Fatalf("retry after forgetFailures: %+v", st)
	}

	before := spawns.Load()
	status = gitclient.GitStatus{Kind: "tooOld", Detected: "2.30.0", Required: "2.38.0"}
	waitFor(t, c.request(job("b3", "t1")))
	st, _ = c.peek("r", cache, rebaseKey{"b3", "t1"})
	if st.Check != conflictFailed || !strings.Contains(st.Reason, "git 2.30.0 is older than 2.38.0") || spawns.Load() != before {
		t.Fatalf("tooOld: %+v spawns %d->%d", st, before, spawns.Load())
	}
}

func TestRebaseChecker_realRepos(t *testing.T) {
	skipWithoutGitQueue(t)
	h := newBoardHarness(t)
	_, dir := initQueueRepo(t)
	h.addRepo("r", dir)
	entry, err := h.board.openRepo(context.Background(), "r")
	if err != nil {
		t.Fatal(err)
	}
	writeQueueFile(t, dir, "shared.txt", "one\n")
	runGitQueue(t, dir, "add", "shared.txt")
	runGitQueue(t, dir, "commit", "-q", "-m", "shared")
	fork := runGitQueue(t, dir, "rev-parse", "HEAD")
	runGitQueue(t, dir, "checkout", "-q", "-b", "clean", fork)
	writeQueueFile(t, dir, "own.txt", "own\n")
	runGitQueue(t, dir, "add", "own.txt")
	runGitQueue(t, dir, "commit", "-q", "-m", "own")
	cleanTip := runGitQueue(t, dir, "rev-parse", "HEAD")
	runGitQueue(t, dir, "checkout", "-q", "-b", "clash", fork)
	writeQueueFile(t, dir, "shared.txt", "branch side\n")
	runGitQueue(t, dir, "commit", "-q", "-am", "clash")
	clashTip := runGitQueue(t, dir, "rev-parse", "HEAD")
	runGitQueue(t, dir, "checkout", "-q", "main")
	writeQueueFile(t, dir, "shared.txt", "main side\n")
	runGitQueue(t, dir, "commit", "-q", "-am", "main moves")
	mainTip := runGitQueue(t, dir, "rev-parse", "HEAD")

	c := newRebaseChecker(context.Background(), okGit, nil)
	cache := newRebaseCache()
	run := mergeTreeRun(entry)
	for tip, want := range map[string][]string{cleanTip: {}, clashTip: {"shared.txt"}} {
		waitFor(t, c.request(rebaseJob{repoID: "r", key: rebaseKey{mainTip, tip}, cache: cache, run: run, branch: "x", base: "main"}))
		st, _ := c.peek("r", cache, rebaseKey{mainTip, tip})
		if st.Check != conflictDone || !slices.Equal(st.Paths, want) {
			t.Fatalf("tip %s: %+v, want paths %v", tip, st, want)
		}
	}
}

// A failed pair check reads as checking while a retry is pending, retries after the backoff and
// settles as failed only once the retries ran out.
func TestRebaseChecker_pairRetryThenFailed(t *testing.T) {
	var spawns atomic.Int32
	var fails atomic.Int32 // failures left to hand out
	fails.Store(2)
	run := func(context.Context, string, string) ([]string, error) {
		spawns.Add(1)
		if fails.Add(-1) >= 0 {
			return nil, errors.New("timeout")
		}
		return []string{"a.txt"}, nil
	}
	dones := make(chan struct{}, 16)
	c := newRebaseChecker(context.Background(), okGit, func(string) { dones <- struct{}{} })
	c.retryBase = 5 * time.Millisecond
	cache := newRebaseCache()
	key := rebaseKey{"b", "t"}
	job := rebaseJob{repoID: "pair:r", key: key, cache: cache, run: run, branch: "pair", retries: 2}
	settle := func() rebaseState {
		t.Helper()
		waitFor(t, c.request(job))
		st, _ := c.peek("pair:r", cache, key)
		return st
	}
	if st := settle(); st.Check != conflictChecking {
		t.Fatalf("first failure with a retry pending must read checking, got %+v", st)
	}
	time.Sleep(60 * time.Millisecond) // retry due: peek reports unknown so the caller requests again
	if _, known := c.peek("pair:r", cache, key); known {
		t.Fatal("due retry must read unknown")
	}
	if st := settle(); st.Check != conflictChecking {
		t.Fatalf("second failure: %+v", st)
	}
	time.Sleep(120 * time.Millisecond)
	if st := settle(); st.Check != conflictDone || spawns.Load() != 3 {
		t.Fatalf("third attempt should succeed: %+v spawns %d", st, spawns.Load())
	}

	// retries exhausted: failed until forgetFailures
	fails.Store(100)
	job.key = rebaseKey{"b", "u"}
	for i := 0; i < 3; i++ {
		settle()
		time.Sleep(200 * time.Millisecond)
	}
	if st, _ := c.peek("pair:r", cache, job.key); st.Check != conflictFailed || st.Reason != "timeout" {
		t.Fatalf("after retries: %+v", st)
	}
}
