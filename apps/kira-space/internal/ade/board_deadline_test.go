package ade

import (
	"context"
	"os/exec"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/internal/testx"
)

func repoUpdating(t *testing.T, h *boardHarness, id string) (bool, string) {
	t.Helper()
	board, err := h.board.Board(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range board.Repos {
		if r.CodeRepoID == id {
			return r.Updating, boardBranch(t, board, "b1").Name
		}
	}
	t.Fatalf("repo %s missing", id)
	return false, ""
}

// boardQuiet reports that no debounced invalidation or push is pending: the repo-changed debounce
// (adeRepoChangedDebounce) and the push coalescer each bump the gen when they fire.
func boardQuiet(b *TaskBoard) bool {
	b.mu.Lock()
	sched := b.boardSched.any
	b.mu.Unlock()
	b.bcache.pushMu.Lock()
	push := b.bcache.push.any
	b.bcache.pushMu.Unlock()
	return !sched && !push
}

// settleBoard waits until background sources (deferred mark writes, conflict checks and their
// debounced invalidations) stop moving the board gen, so a later gen bump in the test is its own.
func settleBoard(t *testing.T, h *boardHarness) {
	t.Helper()
	testx.WaitUntil(t, 10*time.Second, func() bool {
		for stable := 0; stable < 3; {
			g := h.board.bcache.gen.Load()
			repoUpdating(t, h, "r")
			time.Sleep(50 * time.Millisecond)
			if h.board.bcache.gen.Load() == g && boardQuiet(h.board) {
				stable++
			} else {
				return false
			}
		}
		return true
	})
}

// A facts computation held past the deadline shows the last good facts with updating set; its
// landing pushes once more and clears the flag. A second build of the same gen joins it.
func TestBoardFacts_deadlineShowsLastGoodThenLands(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	h := newBoardHarness(t)
	_, dir := initQueueRepo(t)
	h.addRepo("r", dir)
	runGitQueue(t, dir, "branch", "feat-a", "main")
	h.addTask("T1", "task", branchSpec{id: "b1", repo: "r", name: "feat-a", kind: "mine"})
	if updating, _ := repoUpdating(t, h, "r"); updating {
		t.Fatal("first load must wait, not report updating")
	}

	// Settle the first build's deferred mark writes and conflict checks: they drop the snapshot when
	// they land. Rebuild once so the checks of the held build are cached.
	h.board.invalidateBoard()
	settleBoard(t, h)

	var computations atomic.Int32
	release := make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	h.board.facts.hook = func(string) {
		computations.Add(1)
		<-release
	}
	pushes := len(h.pushed())
	h.board.invalidateBoard()
	start := time.Now()
	updating, name := repoUpdating(t, h, "r")
	if !updating || name != "feat-a" {
		t.Fatalf("want last-good facts with updating, got updating=%v name=%q", updating, name)
	}
	if took := time.Since(start); took > 3*repoFactsDeadline {
		t.Fatalf("board waited %v past the deadline", took)
	}
	if _, err := h.board.buildBoard(context.Background()); err != nil {
		t.Fatal(err)
	}
	if n := computations.Load(); n != 1 {
		t.Fatalf("second build of the same gen started a computation: %d", n)
	}

	unblock()
	testx.WaitUntil(t, 5*time.Second, func() bool { return len(h.pushed()) > pushes })
	testx.WaitUntil(t, 5*time.Second, func() bool {
		u, _ := repoUpdating(t, h, "r")
		return !u
	})
	if n := computations.Load(); n != 1 {
		t.Fatalf("late result was recomputed: %d computations", n)
	}
}

// A flight that a build of a newer gen joined predates a change: its late result is overlay-only
// and the next build starts a fresh computation instead of taking it as the answer.
func TestBoardFacts_staleFlightResultIsNotReused(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	h := newBoardHarness(t)
	_, dir := initQueueRepo(t)
	h.addRepo("r", dir)
	runGitQueue(t, dir, "branch", "feat-a", "main")
	h.addTask("T1", "task", branchSpec{id: "b1", repo: "r", name: "feat-a", kind: "mine"})
	repoUpdating(t, h, "r")
	h.board.invalidateBoard()
	settleBoard(t, h)

	h.board.facts.deadline = 100 * time.Millisecond
	var computations atomic.Int32
	release := make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	h.board.facts.hook = func(string) {
		if computations.Add(1) == 1 {
			<-release
		}
	}
	h.board.invalidateBoard()
	if u, _ := repoUpdating(t, h, "r"); !u {
		t.Fatal("want updating while the flight is held")
	}
	time.Sleep(150 * time.Millisecond) // flight now older than the deadline
	h.board.invalidateBoard()          // a change after the flight read its tips
	if u, _ := repoUpdating(t, h, "r"); !u {
		t.Fatal("newer gen must overlay on the old flight")
	}
	if n := computations.Load(); n != 1 {
		t.Fatalf("newer gen started a computation instead of joining: %d", n)
	}

	unblock()
	testx.WaitUntil(t, 5*time.Second, func() bool {
		u, _ := repoUpdating(t, h, "r")
		return !u
	})
	if n := computations.Load(); n != 2 {
		t.Fatalf("want one fresh computation after the stale landing, got %d", n)
	}
}
