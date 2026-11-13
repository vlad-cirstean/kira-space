package adeflow_test

import (
	"slices"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/testx"
)

func TestQueueMergeTree(t *testing.T) {
	app := flowharness.New(t)
	repo := app.NewRepo("api")
	repo.Commit("base", map[string]string{"shared.txt": "base\n", "other.txt": "base\n"})
	for _, b := range []struct{ name, file, body string }{
		{"feat-a", "shared.txt", "from a\n"}, {"feat-b", "shared.txt", "from b\n"}, {"feat-c", "other.txt", "from c\n"},
	} {
		repo.Git("checkout", "-q", "-b", b.name, "main")
		repo.Commit("change "+b.file+" on "+b.name, map[string]string{b.file: b.body})
	}
	repo.Checkout("main")
	rec := importRepo(t, app, repo.Dir)

	tasks := map[string]adewire.Task{}
	branches := map[string]adewire.Branch{}
	for _, name := range []string{"feat-a", "feat-b", "feat-c"} {
		res, err := app.W.AdeTask.AddExistingBranch(ctx, adewire.AddExistingBranchArgs{CodeRepoID: rec.ID, Name: name})
		if err != nil {
			t.Fatal(err)
		}
		tasks[name] = res.Task
		branches[name] = res.Branch
	}

	if _, err := app.W.AdeTask.Refresh(ctx, adewire.RefreshArgs{CodeRepoIDs: []string{rec.ID}}); err != nil {
		t.Fatal(err)
	}
	// Pair conflict checks run in the background: poll until none is checking.
	var b adewire.Board
	pairs := map[[2]string]adewire.Pair{}
	testx.WaitUntil(t, waitFor, func() bool {
		b = board(t, app)
		pairs = map[[2]string]adewire.Pair{}
		for _, p := range b.Pairs {
			if p.Checking {
				return false
			}
			pairs[[2]string{p.A, p.B}] = p
		}
		return len(pairs) > 0
	})
	ab, ok := pairs[[2]string{branches["feat-a"].ID, branches["feat-b"].ID}]
	if !ok {
		ab, ok = pairs[[2]string{branches["feat-b"].ID, branches["feat-a"].ID}]
	}
	if !ok || !slices.Equal(ab.Shared, []string{"shared.txt"}) || !slices.Equal(ab.Conflicts, []string{"shared.txt"}) {
		t.Fatalf("pairs = %+v, want feat-a and feat-b sharing and conflicting on shared.txt", b.Pairs)
	}
	for k, p := range pairs {
		if (k[0] == branches["feat-c"].ID || k[1] == branches["feat-c"].ID) && len(p.Conflicts) > 0 {
			t.Fatalf("feat-c pair %+v conflicts, but it only touches other.txt", p)
		}
	}

	// Main moves under feat-a: git merge-tree says the rebase would conflict.
	repo.Commit("main edits shared", map[string]string{"shared.txt": "main\n"})
	if _, err := app.W.AdeTask.Refresh(ctx, adewire.RefreshArgs{CodeRepoIDs: []string{rec.ID}}); err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"feat-a": true, "feat-b": true, "feat-c": false}
	testx.WaitUntil(t, waitFor, func() bool {
		b = board(t, app)
		for name, wantConflict := range want {
			br := branchOf(t, b, tasks[name].ID, rec.ID)
			if br.ConflictCheck != "done" || (len(br.ConflictsIfRebased) > 0) != wantConflict {
				return false
			}
		}
		return true
	})

	// Queue links: feat-b after feat-a; a cycle, a self link and a stranger are refused.
	a, bb := branches["feat-a"].ID, branches["feat-b"].ID
	if err := app.W.AdeTask.SetQueuedAfter(ctx, adewire.SetQueuedAfterArgs{BranchID: bb, AfterBranchID: a}); err != nil {
		t.Fatal(err)
	}
	if got := board(t, app).Plan.QueuedAfter; got[bb] != a || len(got) != 1 {
		t.Fatalf("queuedAfter = %v, want feat-b after feat-a", got)
	}
	for _, bad := range []adewire.SetQueuedAfterArgs{{BranchID: a, AfterBranchID: bb}, {BranchID: a, AfterBranchID: a}, {BranchID: a, AfterBranchID: "nope"}} {
		if err := app.W.AdeTask.SetQueuedAfter(ctx, bad); err == nil {
			t.Fatalf("SetQueuedAfter(%+v) accepted", bad)
		}
	}
	if err := app.W.AdeTask.SetQueuedAfter(ctx, adewire.SetQueuedAfterArgs{BranchID: bb}); err != nil {
		t.Fatal(err)
	}
	if got := board(t, app).Plan.QueuedAfter; len(got) != 0 {
		t.Fatalf("queuedAfter after clearing = %v", got)
	}

	// SetPlan reorders the tasks and sets a day.
	order := []string{tasks["feat-c"].ID, tasks["feat-a"].ID, tasks["feat-b"].ID}
	day := "2026-10-12"
	if err := app.W.AdeTask.SetPlan(ctx, adewire.SetPlanArgs{Order: order, Days: map[string]*string{tasks["feat-a"].ID: &day}}); err != nil {
		t.Fatal(err)
	}
	plan := board(t, app).Plan
	if !slices.Equal(plan.Order, order) || plan.Day[tasks["feat-a"].ID] != day {
		t.Fatalf("plan = %+v, want order %v and feat-a on %s", plan, order, day)
	}
	if err := app.W.AdeTask.SetPlan(ctx, adewire.SetPlanArgs{Order: []string{"nope"}}); err == nil {
		t.Fatal("SetPlan accepted an unknown task")
	}
}
