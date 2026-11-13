package ade

import (
	"errors"
	"testing"
)

// facts_test.go covers the pure functions in facts.go (white-box, package ade).

// ---------------------------------------------------------------------------------------
// pairFacts (§0.7, §4.3)
// ---------------------------------------------------------------------------------------

func noAncestor(x, y string) bool { return false }

func mergeTreeReturning(paths ...string) func(string, string) ([]string, bool, error) {
	return func(string, string) ([]string, bool, error) { return paths, false, nil }
}

func TestPairFacts_MineReviewConflict(t *testing.T) {
	t.Parallel()
	items := []pairItem{
		{Item: "mine1", Tip: "t1", Kind: "mine", Files: map[string]bool{"a.go": true}},
		{Item: "rev1", Tip: "t2", Kind: "review", Files: map[string]bool{"a.go": true}},
	}
	pairs := pairFacts(items, noAncestor, mergeTreeReturning("a.go"))
	if len(pairs) != 1 {
		t.Fatalf("got %d pairs, want 1: %+v", len(pairs), pairs)
	}
	if pairs[0].A != "mine1" || pairs[0].B != "rev1" {
		t.Fatalf("pair = %+v, want A=mine1 B=rev1 (mine always ordered a)", pairs[0])
	}
	if len(pairs[0].Conflicts) != 1 || pairs[0].Conflicts[0] != "a.go" {
		t.Fatalf("Conflicts = %v, want [a.go]", pairs[0].Conflicts)
	}
}

func TestPairFacts_MineMineShare(t *testing.T) {
	t.Parallel()
	items := []pairItem{
		{Item: "b", Tip: "t1", Kind: "mine", Files: map[string]bool{"a.go": true}},
		{Item: "a", Tip: "t2", Kind: "mine", Files: map[string]bool{"a.go": true}},
	}
	pairs := pairFacts(items, noAncestor, mergeTreeReturning())
	if len(pairs) != 1 {
		t.Fatalf("got %d pairs, want 1: %+v", len(pairs), pairs)
	}
	if pairs[0].A != "a" || pairs[0].B != "b" {
		t.Fatalf("pair = %+v, want A=a B=b (mine x mine ordered by name)", pairs[0])
	}
	if len(pairs[0].Shared) != 1 || pairs[0].Shared[0] != "a.go" {
		t.Fatalf("Shared = %v, want [a.go]", pairs[0].Shared)
	}
	if len(pairs[0].Conflicts) != 0 {
		t.Fatalf("Conflicts = %v, want none (clean merge)", pairs[0].Conflicts)
	}
}

func TestPairFacts_ParkedExcluded(t *testing.T) {
	t.Parallel()
	items := []pairItem{
		{Item: "mine1", Tip: "t1", Kind: "mine", Files: map[string]bool{"a.go": true}},
		{Item: "parked1", Tip: "t2", Kind: "parked", Files: map[string]bool{"a.go": true}},
	}
	pairs := pairFacts(items, noAncestor, mergeTreeReturning("a.go"))
	if len(pairs) != 0 {
		t.Fatalf("got %d pairs, want 0 (parked excluded)", len(pairs))
	}
}

func TestPairFacts_MergedExcluded(t *testing.T) {
	t.Parallel()
	items := []pairItem{
		{Item: "mine1", Tip: "t1", Kind: "mine", Files: map[string]bool{"a.go": true}},
		{Item: "mine2", Tip: "t2", Kind: "mine", Merged: true, Files: map[string]bool{"a.go": true}},
	}
	pairs := pairFacts(items, noAncestor, mergeTreeReturning("a.go"))
	if len(pairs) != 0 {
		t.Fatalf("got %d pairs, want 0 (merged excluded)", len(pairs))
	}
}

func TestPairFacts_AncestorPairExcluded(t *testing.T) {
	t.Parallel()
	items := []pairItem{
		{Item: "mine1", Tip: "t1", Kind: "mine", Files: map[string]bool{"a.go": true}},
		{Item: "mine2", Tip: "t2", Kind: "mine", Files: map[string]bool{"a.go": true}},
	}
	isAncestor := func(x, y string) bool { return true }
	pairs := pairFacts(items, isAncestor, mergeTreeReturning("a.go"))
	if len(pairs) != 0 {
		t.Fatalf("got %d pairs, want 0 (ancestor pair excluded)", len(pairs))
	}
}

func TestPairFacts_ReviewReviewExcluded(t *testing.T) {
	t.Parallel()
	items := []pairItem{
		{Item: "rev1", Tip: "t1", Kind: "review", Files: map[string]bool{"a.go": true}},
		{Item: "rev2", Tip: "t2", Kind: "review", Files: map[string]bool{"a.go": true}},
	}
	pairs := pairFacts(items, noAncestor, mergeTreeReturning("a.go"))
	if len(pairs) != 0 {
		t.Fatalf("got %d pairs, want 0 (review x review is never a pair)", len(pairs))
	}
}

func TestPairFacts_NoSharedPathsNoMergeTreeCall(t *testing.T) {
	t.Parallel()
	items := []pairItem{
		{Item: "mine1", Tip: "t1", Kind: "mine", Files: map[string]bool{"a.go": true}},
		{Item: "mine2", Tip: "t2", Kind: "mine", Files: map[string]bool{"b.go": true}},
	}
	calls := 0
	mergeTree := func(a, b string) ([]string, bool, error) {
		calls++
		return nil, false, nil
	}
	pairs := pairFacts(items, noAncestor, mergeTree)
	if len(pairs) != 0 {
		t.Fatalf("got %d pairs, want 0 (no shared paths)", len(pairs))
	}
	if calls != 0 {
		t.Fatalf("mergeTree called %d times, want 0 (no candidate pair to check)", calls)
	}
}

func TestPairFacts_ConflictedPathOutsideSharedDropped(t *testing.T) {
	t.Parallel()
	items := []pairItem{
		{Item: "mine1", Tip: "t1", Kind: "mine", Files: map[string]bool{"a.go": true}},
		{Item: "mine2", Tip: "t2", Kind: "mine", Files: map[string]bool{"a.go": true}},
	}
	// merge-tree reports a conflict in c.go too, but c.go is not one of the two items' own shared
	// changed files (e.g. it's a file only one of them ever touched, base picked up elsewhere) —
	// Conflicts must stay confined to shared.
	pairs := pairFacts(items, noAncestor, mergeTreeReturning("a.go", "c.go"))
	if len(pairs) != 1 {
		t.Fatalf("got %d pairs, want 1", len(pairs))
	}
	if len(pairs[0].Conflicts) != 1 || pairs[0].Conflicts[0] != "a.go" {
		t.Fatalf("Conflicts = %v, want [a.go] (c.go dropped, outside shared)", pairs[0].Conflicts)
	}
}

func TestPairFacts_PendingAndFailedChecks(t *testing.T) {
	t.Parallel()
	items := []pairItem{
		{Item: "mine1", Tip: "t1", Kind: "mine", Files: map[string]bool{"a.go": true}},
		{Item: "mine2", Tip: "t2", Kind: "mine", Files: map[string]bool{"a.go": true}},
	}
	pending := pairFacts(items, noAncestor, func(a, b string) ([]string, bool, error) { return nil, true, nil })
	if len(pending) != 1 || !pending[0].Checking || len(pending[0].Conflicts) != 0 {
		t.Fatalf("pending pair = %+v, want Checking with no conflicts", pending)
	}
	failed := pairFacts(items, noAncestor, func(a, b string) ([]string, bool, error) {
		return []string{"a.go"}, true, errors.New("boom")
	})
	if len(failed) != 1 || failed[0].Checking || len(failed[0].Conflicts) != 0 {
		t.Fatalf("failed pair = %+v, want no conflicts and not Checking", failed)
	}
}

// ---------------------------------------------------------------------------------------
// mergedRule (§0.8)
// ---------------------------------------------------------------------------------------

func TestMergedRule(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name                  string
		tipReachable, commits bool
		prState               string
		want                  bool
	}{
		{"ancestor without had_commits not merged", true, false, "", false},
		{"ancestor with had_commits merged", true, true, "", true},
		{"PR merged wins", false, false, "merged", true},
		{"PR closed not merged", false, false, "closed", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := mergedRule(c.tipReachable, c.commits, c.prState); got != c.want {
				t.Fatalf("mergedRule(%v, %v, %q) = %v, want %v", c.tipReachable, c.commits, c.prState, got, c.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------------------
// atRisk (§0.9)
// ---------------------------------------------------------------------------------------

func TestAtRisk_ReviewDirtyCountedUnmergedNot(t *testing.T) {
	t.Parallel()
	got := atRisk(atRiskInput{Kind: "review", HasLinkedWorktree: true, DirtyCount: 4, UnmergedAheadOfMain: 9})
	if got.Dirty != 4 {
		t.Fatalf("Dirty = %d, want 4 (review with a linked worktree still counts dirty)", got.Dirty)
	}
	if got.Unmerged != 0 {
		t.Fatalf("Unmerged = %d, want 0 (review never counts unmerged)", got.Unmerged)
	}
}

func TestAtRisk_MergedMineUnmergedZero(t *testing.T) {
	t.Parallel()
	got := atRisk(atRiskInput{Kind: "mine", Merged: true, HasLinkedWorktree: true, DirtyCount: 2, UnmergedAheadOfMain: 9})
	if got.Unmerged != 0 {
		t.Fatalf("Unmerged = %d, want 0 (merged branch's history is already safe in main)", got.Unmerged)
	}
	if got.Dirty != 2 {
		t.Fatalf("Dirty = %d, want 2 (dirty still counts regardless of merged)", got.Dirty)
	}
}

func TestAtRisk_NewWorkNone(t *testing.T) {
	t.Parallel()
	got := atRisk(atRiskInput{IsNewWork: true, HasLinkedWorktree: true, DirtyCount: 9, UnmergedAheadOfMain: 9})
	if got != (atRiskResult{}) {
		t.Fatalf("got %+v, want zero value (new work has no worktree of its own to lose anything from)", got)
	}
}
