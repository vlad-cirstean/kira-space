package ade

import (
	"context"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
)

type diskAnswer struct {
	res []adewire.DiskChange
	err error
}

func askDisk(b *TaskBoard, id string) <-chan diskAnswer {
	ch := make(chan diskAnswer, 1)
	go func() {
		res, err := b.DiskChanges(context.Background(), id)
		ch <- diskAnswer{res, err}
	}()
	return ch
}

// DiskChanges answers its last result while a task op holds the task lock, blocks when a write
// dropped that result, and never stores a result computed across a write.
func TestDiskChangesDuringTaskOp(t *testing.T) {
	h, _ := stateBoard(t)
	b, df := h.board, h.board.files

	if _, err := b.DiskChanges(context.Background(), "t1"); err != nil {
		t.Fatal(err)
	}
	if _, ok := df.lastDisk("t1"); !ok {
		t.Fatal("a computed call kept no result")
	}

	// A marker only the cache holds proves the answer is the cached one, not a recompute.
	df.storeDisk("t1", []adewire.DiskChange{{Path: "cached.md"}}, df.diskGeneration("t1"))
	mu := b.taskMu("t1")
	mu.Lock()
	select {
	case a := <-askDisk(b, "t1"):
		if a.err != nil || len(a.res) != 1 || a.res[0].Path != "cached.md" {
			t.Fatalf("answer under the lock = %+v, want the cached list", a)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("DiskChanges waited on the task lock with a cached result")
	}

	// After a write dropped the cache, the call has nothing to serve and waits for the lock.
	df.dropDisk("t1")
	pending := askDisk(b, "t1")
	select {
	case a := <-pending:
		t.Fatalf("DiskChanges answered %+v under the lock with no cached result", a)
	case <-time.After(200 * time.Millisecond):
	}
	mu.Unlock()
	select {
	case a := <-pending:
		if a.err != nil {
			t.Fatal(a.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("DiskChanges still waiting after the lock was released")
	}

	// A result computed before a write is not kept.
	gen := df.diskGeneration("t1")
	df.dropDisk("t1")
	df.storeDisk("t1", []adewire.DiskChange{{Path: "stale.md"}}, gen)
	if last, ok := df.lastDisk("t1"); ok && len(last) == 1 && last[0].Path == "stale.md" {
		t.Fatal("a result computed across a write was stored")
	}
}
