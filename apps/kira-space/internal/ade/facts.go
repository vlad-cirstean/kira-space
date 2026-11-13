package ade

import "sort"

// facts.go holds the pure fact rules the v2 board shares: pairFacts (pair conflicts), mergedRule and
// atRisk. Git-backed assembly lives in gitfacts.go and board_facts.go.

// ---------------------------------------------------------------------------------------
// pairFacts (§0.7, §4.3)
// ---------------------------------------------------------------------------------------

// pairItem is pairFacts' own per-item input.
type pairItem struct {
	Item, Tip string
	Kind      string // "mine" | "review" | "parked"
	Merged    bool
	// Files is this item's own changed-file set (§4.2's own "files"), keyed by path.
	Files map[string]bool
}

// pair is pairFacts' own output row — AdePair's own domain shape, pre-sort.
type pair struct {
	A, B      string
	Shared    []string
	Conflicts []string
	// Checking is set while the pair's merge-tree check runs: Conflicts is empty until it lands.
	Checking bool
	// Failed is set when the pair's check failed for good (retries exhausted): Conflicts is empty
	// but unknown.
	Failed bool
}

// pairFacts computes §0.7's own conflict/share table over items: for each unordered pair with `a`
// mine and `b` mine-or-review, neither parked nor merged, neither an ancestor of the other, and a
// non-empty own-file intersection, it calls mergeTree(tipA, tipB) — ordered so a repeat call with
// the same two tips is a cache hit at the CALLER's own layer (§4.2's tip-keyed LRU; this function
// itself holds no cache, it only ever asks once per passing pair per call). mergeTree reports
// pending while the check still runs (the pair shows Checking) and an error when it failed for good
// (the pair shows Failed; the checker already logged it). isAncestor(x, y) reports whether x or y is a
// (queued) ancestor of the other, in either order. Output sorted by (a, b).
func pairFacts(items []pairItem, isAncestor func(x, y string) bool, mergeTree func(tipA, tipB string) (paths []string, pending bool, err error)) []pair {
	var pairs []pair
	for i := 0; i < len(items); i++ {
		for j := i + 1; j < len(items); j++ {
			a, b, ok := orderPair(items[i], items[j])
			if !ok {
				continue
			}
			if a.Merged || b.Merged || a.Kind == "parked" || b.Kind == "parked" {
				continue
			}
			if isAncestor(a.Item, b.Item) {
				continue
			}
			shared := sharedFiles(a.Files, b.Files)
			if len(shared) == 0 {
				continue
			}
			conflictedPaths, pending, err := mergeTree(a.Tip, b.Tip)
			if err != nil {
				conflictedPaths = nil
			}
			pairs = append(pairs, pair{A: a.Item, B: b.Item, Shared: shared, Conflicts: intersectSorted(conflictedPaths, shared), Checking: pending && err == nil, Failed: err != nil})
		}
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].A != pairs[j].A {
			return pairs[i].A < pairs[j].A
		}
		return pairs[i].B < pairs[j].B
	})
	return pairs
}

// orderPair applies §0.7's own kind filter and ordering: `a` mine x `b` mine-or-review only: a
// mine×review pair always orders `a` mine; a mine×mine pair orders `a`/`b` by name. Any other kind
// combination (review×review, either parked) is not a pair at all.
func orderPair(x, y pairItem) (a, b pairItem, ok bool) {
	switch {
	case x.Kind == "mine" && y.Kind == "mine":
		if x.Item <= y.Item {
			return x, y, true
		}
		return y, x, true
	case x.Kind == "mine" && y.Kind == "review":
		return x, y, true
	case y.Kind == "mine" && x.Kind == "review":
		return y, x, true
	default:
		return pairItem{}, pairItem{}, false
	}
}

func sharedFiles(a, b map[string]bool) []string {
	var out []string
	for f := range a {
		if b[f] {
			out = append(out, f)
		}
	}
	sort.Strings(out)
	return out
}

func intersectSorted(paths, shared []string) []string {
	sharedSet := make(map[string]bool, len(shared))
	for _, s := range shared {
		sharedSet[s] = true
	}
	var out []string
	for _, p := range paths {
		if sharedSet[p] {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
}

// ---------------------------------------------------------------------------------------
// mergedRule (§0.8)
// ---------------------------------------------------------------------------------------

// mergedRule implements §0.8: merged iff (the tip is reachable from main AND had_commits) or the
// raw PR state is exactly "merged" (covers squash/rebase merges, whose tip never becomes an
// ancestor of main). prState is ResolveBranchPr's own raw State when a PR is known, "" otherwise.
func mergedRule(tipReachableFromMain, hadCommits bool, prState string) bool {
	if tipReachableFromMain && hadCommits {
		return true
	}
	return prState == "merged"
}

// ---------------------------------------------------------------------------------------
// atRisk (§0.9)
// ---------------------------------------------------------------------------------------

// atRiskInput is atRisk's own per-item input.
type atRiskInput struct {
	IsNewWork         bool
	Kind              string // "mine" | "review" | "parked" — ignored when IsNewWork
	Merged            bool
	HasLinkedWorktree bool
	// DirtyCount is the linked (or main) worktree's own dirty-path count; meaningless (and unread)
	// when HasLinkedWorktree is false.
	DirtyCount int
	// UnmergedAheadOfMain is count(mainRef..tip); meaningless (and unread) for review or a merged
	// branch.
	UnmergedAheadOfMain int
}

// atRiskResult is atRisk's own output.
type atRiskResult struct {
	Dirty    int
	Unmerged int
}

// atRisk implements the deliberate deviation from the design: dirty paths count for ANY
// branch with a linked worktree, review included (removing a dirty worktree discards them
// regardless of whose branch it is) — never for new work, which has no worktree of its own to lose
// anything from. Unmerged commits ahead of main count only for mine/parked, and only while unmerged
// (a merged branch's own history is safe in main already).
func atRisk(in atRiskInput) atRiskResult {
	if in.IsNewWork {
		return atRiskResult{}
	}
	var out atRiskResult
	if in.HasLinkedWorktree {
		out.Dirty = in.DirtyCount
	}
	if (in.Kind == "mine" || in.Kind == "parked") && !in.Merged {
		out.Unmerged = in.UnmergedAheadOfMain
	}
	return out
}
