package ade

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitclient"
)

// spawnCounter counts every git spawn.
type spawnCounter struct {
	gitclient.Runner
	spawns *atomic.Int64
}

func (c spawnCounter) Start(ctx context.Context, gitPath string, spec gitclient.Spec) (gitclient.Process, error) {
	c.spawns.Add(1)
	return c.Runner.Start(ctx, gitPath, spec)
}

// BenchmarkBoardWarm builds the board after an invalidation: one repo, 10 branches over 3 tasks,
// 5 attached worktrees, one conflicting pair. Spawns only: the harness hides its *sql.DB, so no
// query count.
func BenchmarkBoardWarm(b *testing.B) { benchBoard(b, true) }

// BenchmarkBoardHit reads the same board with no invalidation: the snapshot serves it.
func BenchmarkBoardHit(b *testing.B) { benchBoard(b, false) }

func benchBoard(b *testing.B, invalidate bool) {
	if _, err := exec.LookPath("git"); err != nil {
		b.Skip("git not on PATH")
	}
	var spawns atomic.Int64
	h := newBoardHarnessWith(b, func(r gitclient.Runner) gitclient.Runner { return spawnCounter{r, &spawns} })
	_, dir := initQueueRepo(b)
	h.addRepo("r", dir)
	commitFile(b, dir, "shared.txt", "one\n", "shared")
	runGitQueue(b, dir, "push", "-q", "origin", "main")
	wts := b.TempDir()
	var specs [3][]branchSpec
	for i := range 10 {
		name := fmt.Sprintf("feat-%d", i)
		runGitQueue(b, dir, "checkout", "-q", "-b", name, "main")
		file, content := fmt.Sprintf("f%d.txt", i), "x\n"
		if i < 2 { // feat-0 and feat-1 both rewrite shared.txt: a real pair conflict
			file, content = "shared.txt", fmt.Sprintf("side %d\n", i)
		}
		commitFile(b, dir, file, content, name)
		specs[i%3] = append(specs[i%3], branchSpec{id: "b" + name, repo: "r", name: name, kind: "mine"})
	}
	runGitQueue(b, dir, "checkout", "-q", "main")
	for i := range 5 {
		name := fmt.Sprintf("feat-%d", i+5)
		runGitQueue(b, dir, "worktree", "add", "-q", filepath.Join(wts, name), name)
	}
	for i := range specs {
		h.addTask(fmt.Sprintf("T%d", i), "task", specs[i]...)
	}
	ctx := context.Background()
	for range 200 { // settle the background conflict checks
		board, err := h.board.Board(ctx)
		if err != nil {
			b.Fatal(err)
		}
		checking := false
		for _, br := range board.Branches {
			checking = checking || br.ConflictCheck == conflictChecking
		}
		if !checking {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	spawns.Store(0)
	b.ResetTimer()
	for range b.N {
		if invalidate {
			h.board.invalidateBoard()
		}
		if _, err := h.board.Board(ctx); err != nil {
			b.Fatal(err)
		}
	}
	b.ReportMetric(float64(spawns.Load())/float64(b.N), "spawns/op")
}
