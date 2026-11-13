package ade

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
)

// Two identical concurrent StartRun calls join: one run per repo, same result, no "already
// started" error from a second call. A different task's call runs on its own.
func TestStartRun_identicalCallsJoin(t *testing.T) {
	e := newEngine(t, map[string][]string{"*": {"done"}})
	e.repo("api")
	e.workflow(flowYAML(agentStep("one", "") + manualStep))
	id, other := e.task("api"), e.task("api")

	tc, err := e.board.loadTaskCtx(id)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]string{}
	for _, br := range tc.branches {
		names[br.ID] = "feat/joined"
	}
	args := adewire.StartRunArgs{TaskID: id, BranchNames: names}

	// Holding the task lock keeps the leader inside the call until every caller has arrived.
	mu := e.board.taskMu(id)
	mu.Lock()
	const callers = 4
	results := make([]adewire.StartRunResult, callers)
	errs := make([]error, callers)
	var started, done sync.WaitGroup
	started.Add(callers)
	done.Add(callers)
	for i := range callers {
		go func() {
			defer done.Done()
			started.Done()
			results[i], errs[i] = e.board.StartRun(context.Background(), args)
		}()
	}
	started.Wait()
	time.Sleep(100 * time.Millisecond) // let every caller reach the join; they cannot be observed
	mu.Unlock()
	done.Wait()

	for i := range callers {
		if errs[i] != nil {
			t.Fatalf("caller %d: %v", i, errs[i])
		}
		if !slices.Equal(results[i].RunIDs, results[0].RunIDs) {
			t.Fatalf("caller %d run ids %v, want %v", i, results[i].RunIDs, results[0].RunIDs)
		}
	}
	if len(results[0].RunIDs) != 1 || len(e.runs(id)) != 1 {
		t.Fatalf("run ids %v, runs %v: want exactly one run row", results[0].RunIDs, e.runs(id))
	}
	if ids := e.start(other); len(ids) != 1 {
		t.Fatalf("other task run ids %v, want its own run", ids)
	}
}
