package gitsession

import (
	"context"
	"strings"
	"sync"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitclient/porcelain"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitreview"
)

// reviewRangeCountSlot is a one-entry per-repo slot: a rev-list --count remembered for
// exactly one <base>..<branch> pair, computed by ResolveReviewBase and peeked (never blocking) by
// the ranged graph.* handlers. A miss -- an empty slot, or one for a different range -- is a
// correct optimisation fallback, never a correctness dependency: logsession runs its own
// rev-list --count when this misses. One slot, not a map, matching upstream's
// lastReviewResolution.
type reviewRangeCountSlot struct {
	mu           sync.Mutex
	base, branch string
	count        int
	valid        bool
}

func (s *reviewRangeCountSlot) remember(base, branch string, n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.base, s.branch, s.count, s.valid = base, branch, n, true
}

// take is a non-blocking peek -- it does not consume the slot. (Walk.resetLocked is what consumes
// its own copy, once, after threading it into logsession.Options -- D9's "cleared after the
// walk's first open".)
func (s *reviewRangeCountSlot) take(base, branch string) *int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.valid || s.base != base || s.branch != branch {
		return nil
	}
	n := s.count
	return &n
}

func (s *reviewRangeCountSlot) drop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.valid = false
}

// RememberRangeCount records n as the range-count slot's own value for base..branch (D9) — written
// on a "ready" outcome.
func (e *RepoEntry) RememberRangeCount(base, branch string, n int) {
	e.rangeCount.remember(base, branch, n)
}

// TakeRangeCount is a non-blocking peek at the range-count slot (D9) — nil on a miss (an empty
// slot, or one for a different range), which the caller (gitrpc's ranged graph.* handlers) treats
// as a correct fallback, never an error: logsession runs its own rev-list --count when this
// misses.
func (e *RepoEntry) TakeRangeCount(base, branch string) *int {
	return e.rangeCount.take(base, branch)
}

// originHead is `symbolic-ref --short refs/remotes/origin/HEAD` (D7c step 2's one spawn) -- exit
// 128 (no origin, origin/HEAD unset, or dangling -- probe P1, all indistinguishable and all "not
// detected") folds into "", never an error the UI would have to render. ctx.Err() is checked
// before that fold, so a genuinely cancelled read is never mistaken for "no default branch
// detected" (gitclient.Classify's own discipline, restated here because runAllowingExit's ok-code
// match happens after Classify would have already caught an in-flight cancellation, not a
// same-instant race against a process that happened to exit 128 on its own).
func (e *RepoEntry) originHead(ctx context.Context) (string, error) {
	return metaGet(e.meta, "originHead", func() (string, error) { return e.readOriginHead(ctx) })
}

func (e *RepoEntry) readOriginHead(ctx context.Context) (string, error) {
	res, err := e.runAllowingExit(ctx, porcelain.OriginHeadArgs(), 0, 128)
	if err != nil {
		return "", err
	}
	if res.ExitCode != 0 {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		return "", nil
	}
	return strings.TrimSpace(string(res.Stdout)), nil
}

// sharesHistory is `merge-base <base> <branch>`: exit 0 means the two share
// history, exit 1 means they do not (BaseResolution's "unrelated" outcome) -- anything else is a
// classified error, never a silent "unrelated". A thin wrapper over the mergeBase (incremental.go)
// — one helper, two callers, one spawn: mergeBase reads the sha this call used to throw away,
// and this collapses into mergeBase's own `ok` return.
func (e *RepoEntry) sharesHistory(ctx context.Context, base, branch string) (bool, error) {
	_, ok, err := e.mergeBase(ctx, base, branch)
	return ok, err
}

// countRange is `rev-list --count <base>..<branch>` (D7c step 6) -- a bad base/branch is exit 128
// and classifies normally through runOne (probe P3).
func (e *RepoEntry) countRange(ctx context.Context, base, branch string) (int, error) {
	out, err := e.runOne(ctx, porcelain.CountRangeArgs(base, branch))
	if err != nil {
		return 0, err
	}
	return porcelain.ParseCount(out)
}

// findBranchRef resolves branch's short name against the CACHED refs snapshot's branches, then
// remote branches -- ResolveReviewBase's own first step.
func findBranchRef(snapshot RefsResult, shortName string) (porcelain.RefRow, bool) {
	for _, r := range snapshot.Branches {
		if r.ShortName == shortName {
			return r, true
		}
	}
	for _, r := range snapshot.RemoteBranches {
		if r.ShortName == shortName {
			return r, true
		}
	}
	return porcelain.RefRow{}, false
}

// naturalResolution is upstream's own #naturalResolution: a git-free probe with OriginHead: ""
// first, and only when that falls through (Reason != "upstream") does the real, spawn-bearing
// resolution run -- "one spawn, only when rule 1 falls through" (D7c).
func (e *RepoEntry) naturalResolution(ctx context.Context, branchRef porcelain.RefRow, snapshot RefsResult, candidates []string) (gitreview.Core, error) {
	probe := gitreview.ResolveBase(gitreview.Input{
		Branch: branchRef, Branches: snapshot.Branches, RemoteBranches: snapshot.RemoteBranches,
	})
	originHead := ""
	if probe.Reason != gitreview.ReasonUpstream {
		oh, err := e.originHead(ctx)
		if err != nil {
			return gitreview.Core{}, err
		}
		originHead = oh
	}
	return gitreview.ResolveBase(gitreview.Input{
		Branch: branchRef, Branches: snapshot.Branches, RemoteBranches: snapshot.RemoteBranches,
		OriginHead: originHead, Candidates: candidates,
	}), nil
}

// ResolveReviewBase is review.resolveBase's own orchestration -- upstream's
// resolveReviewBase, computed fresh on every call before any row is painted. The candidates are
// this repo's own stored kiraSpace.review.baseCandidates, falling back to
// gitreview.DefaultBaseCandidates when none are stored.
func (e *RepoEntry) ResolveReviewBase(ctx context.Context, branch string, base *string) (gitreview.BaseResolution, error) {
	candidates := e.RepoSettings().ReviewBaseCandidates
	if len(candidates) == 0 {
		candidates = gitreview.DefaultBaseCandidates
	}

	gen := e.cacheGeneration()
	snapshot, err := e.Refs(ctx)
	if err != nil {
		return gitreview.BaseResolution{}, err
	}

	branchRef, ok := findBranchRef(snapshot, branch)
	if !ok {
		return gitreview.BaseResolution{
			Branch: branch, Base: nil, Reason: gitreview.ReasonNone,
			Range: gitreview.RangeState{Kind: "ask"}, Candidates: []gitreview.Candidate{},
		}, nil
	}

	natural, err := e.naturalResolution(ctx, branchRef, snapshot, candidates)
	if err != nil {
		return gitreview.BaseResolution{}, err
	}

	// candidates always reflects the NATURAL resolution, even on the override path (upstream's own
	// behaviour) -- so the header picker's shortlist stays stable while the user cycles bases.
	resolvedBase, reason := natural.Base, natural.Reason
	if base != nil {
		b := *base
		resolvedBase, reason = &b, gitreview.ReasonOverride
	}

	if resolvedBase == nil {
		return gitreview.BaseResolution{
			Branch: branch, Base: nil, Reason: reason,
			Range: gitreview.RangeState{Kind: "ask"}, Candidates: natural.Candidates,
		}, nil
	}

	// merge-base runs BEFORE rev-list --count, sequential with short-circuit (D7c): an unrelated
	// pair never runs the count at all (probe P4).
	shares, err := e.sharesHistory(ctx, *resolvedBase, branch)
	if err != nil {
		return gitreview.BaseResolution{}, err
	}
	if !shares {
		return gitreview.BaseResolution{
			Branch: branch, Base: resolvedBase, Reason: reason,
			Range: gitreview.RangeState{Kind: "unrelated"}, Candidates: natural.Candidates,
		}, nil
	}

	count, err := e.countRange(ctx, *resolvedBase, branch)
	if err != nil {
		return gitreview.BaseResolution{}, err
	}
	if count == 0 {
		return gitreview.BaseResolution{
			Branch: branch, Base: resolvedBase, Reason: reason,
			Range: gitreview.RangeState{Kind: "empty"}, Candidates: natural.Candidates,
		}, nil
	}

	if e.cacheGeneration() == gen {
		e.RememberRangeCount(*resolvedBase, branch, count)
	}
	n := count
	return gitreview.BaseResolution{
		Branch: branch, Base: resolvedBase, Reason: reason,
		Range:      gitreview.RangeState{Kind: "ready", CommitCount: &n},
		Candidates: natural.Candidates,
	}, nil
}
