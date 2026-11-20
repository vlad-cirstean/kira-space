package gitsession

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/sync/errgroup"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitclient/porcelain"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitops"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitpreflight"
	"github.com/kirathecat/kira-studio/internal/fileservice"
)

// rewrittenPaths reads T — the paths target's checkout would rewrite (gitops.RewrittenPathsArgs).
//
// target reaches RewrittenPathsArgs as a bare argv token — a leading "-"
// would be read by `git diff`/`git diff-index` as a flag of its own (e.g. `--output=<path>`, an
// arbitrary-file-write primitive) rather than the ref it is meant to be. validOpArg is the same
// guard op.run's prepare* functions already use at their own point of use.
func (e *RepoEntry) rewrittenPaths(ctx context.Context, target string) ([]string, error) {
	if err := validOpArg("target", target); err != nil {
		return nil, err
	}
	recs, err := e.runRecords(ctx, gitops.RewrittenPathsArgs(target))
	if err != nil {
		return nil, err
	}
	out := make([]string, len(recs))
	for i, r := range recs {
		out[i] = string(r)
	}
	return out, nil
}

// PreflightCheckout is preflight.checkout's own orchestration: gather the reads in parallel,
// resolve the wire's bare target against a FRESH ref snapshot (never the cache), call the pure
// classifier. StashAvailable: false; flipping it to true unconditionally, which alone is what turns
// ClassifyCheckout's own "stashAndCarry" route on.
func (e *RepoEntry) PreflightCheckout(ctx context.Context, target, mode string) (gitpreflight.CheckoutPreflight, error) {
	snapshot, err := e.refsSnapshot(ctx)
	if err != nil {
		return gitpreflight.CheckoutPreflight{}, err
	}
	resolved := resolveCheckoutTarget(snapshot, target)

	var statusResult porcelain.StatusResult
	var inProgress *gitpreflight.InProgressOperation
	var rewritten []string
	var statusErr, rewrittenErr error
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		statusResult, inProgress, statusErr = e.statusAndInProgress(ctx)
	}()
	go func() {
		defer wg.Done()
		rewritten, rewrittenErr = e.rewrittenPaths(ctx, resolved.Name)
	}()
	wg.Wait()
	if statusErr != nil {
		return gitpreflight.CheckoutPreflight{}, statusErr
	}
	if rewrittenErr != nil {
		return gitpreflight.CheckoutPreflight{}, rewrittenErr
	}

	return gitpreflight.ClassifyCheckout(gitpreflight.ClassifyCheckoutInput{
		Target:         gitpreflight.CheckoutTarget{Kind: resolved.Kind, Name: resolved.Name},
		Mode:           mode,
		Dirty:          gitpreflight.DirtyPaths(statusResult),
		Rewritten:      rewritten,
		InProgress:     inProgress,
		CheckedOutIn:   resolved.CheckedOutIn,
		StashAvailable: true,
	}), nil
}

// revertMergeParents looks up, for every MERGE commit among shas, its parents' shas and subjects:
// one `show -s` per requested sha to learn its parent count/shas, then one more per
// DISTINCT parent sha across every merge found (deduplicated) to learn that parent's subject.
// Sequential, not concurrent (the deviation from upstream's own Promise.all): the read pool is
// four wide and shared with every other connection on the repository, so a large multi-select
// revert firing dozens of concurrent reads would starve the graph stream of another window.
// Every sha reaches ShowMetadataArgs (`git show -s ...`) as a bare argv
// token, reached by both PreflightRevert and PreflightCherryPick — validated once here, at the one
// choke point both callers share, rather than duplicated in each.
func (e *RepoEntry) revertMergeParents(ctx context.Context, shas []string) (map[string][]gitpreflight.RevertParentChoice, error) {
	for _, sha := range shas {
		if err := validOpArg("sha", sha); err != nil {
			return nil, err
		}
	}
	type meta struct {
		sha     string
		parents []string
	}
	metas := make([]meta, 0, len(shas))
	for _, sha := range shas {
		raw, err := e.runOne(ctx, porcelain.ShowMetadataArgs(sha))
		if err != nil {
			return nil, err
		}
		// F3: ShowMetadataArgs' LogFormat is NUL-delimited field to field now — ParseLogRecordFromRaw
		// does its own fixed-field-count split rather than oneRecord's single-delimiter one.
		parsed, err := porcelain.ParseLogRecordFromRaw(raw)
		if err != nil {
			return nil, err
		}
		metas = append(metas, meta{sha: sha, parents: parsed.Parents})
	}

	subjects := make(map[string]string)
	var parentOrder []string
	for _, m := range metas {
		if len(m.parents) <= 1 {
			continue
		}
		for _, p := range m.parents {
			if _, ok := subjects[p]; !ok {
				subjects[p] = ""
				parentOrder = append(parentOrder, p)
			}
		}
	}
	for _, p := range parentOrder {
		raw, err := e.runOne(ctx, porcelain.ShowMetadataArgs(p))
		if err != nil {
			return nil, err
		}
		parsed, err := porcelain.ParseLogRecordFromRaw(raw)
		if err != nil {
			return nil, err
		}
		subjects[p] = parsed.Subject
	}

	result := make(map[string][]gitpreflight.RevertParentChoice)
	for _, m := range metas {
		if len(m.parents) <= 1 {
			continue
		}
		choices := make([]gitpreflight.RevertParentChoice, len(m.parents))
		for i, p := range m.parents {
			choices[i] = gitpreflight.RevertParentChoice{ParentNumber: i + 1, Sha: p, Subject: subjects[p]}
		}
		result[m.sha] = choices
	}
	return result, nil
}

// predictRevert runs the merge-tree prediction for firstSha, scoped exactly to shas[0]
// (predictedFor says so). A merge commit with no mainline chosen yet gets {kind:"unknown"} rather
// than a guess ("rather than guessing -m 1"); exit 0/1 are both real predictions,
// gated by runAllowingExit rather than gitclient.Classify.
func (e *RepoEntry) predictRevert(
	ctx context.Context, firstSha string, mergeParents map[string][]gitpreflight.RevertParentChoice, mainline *int,
) gitpreflight.RevertPrediction {
	if firstSha == "" {
		return gitpreflight.RevertPrediction{Kind: "unknown", Reason: "no commit selected"}
	}
	_, isMerge := mergeParents[firstSha]
	if isMerge && mainline == nil {
		return gitpreflight.RevertPrediction{
			Kind: "unknown", Reason: "a mainline parent must be chosen before predicting this merge commit's revert",
		}
	}
	effectiveMainline := 1
	if isMerge {
		effectiveMainline = *mainline
	}
	other := fmt.Sprintf("%s^%d", firstSha, effectiveMainline)

	res, err := e.runAllowingExit(ctx, porcelain.MergeTreeArgs("HEAD", other, firstSha), 0, 1)
	if err != nil {
		return gitpreflight.RevertPrediction{Kind: "unknown", Reason: err.Error()}
	}
	pred, err := porcelain.ParseMergeTreeOutput(res.Stdout, res.ExitCode)
	if err != nil {
		return gitpreflight.RevertPrediction{Kind: "unknown", Reason: err.Error()}
	}
	if pred.Kind == "clean" {
		return gitpreflight.RevertPrediction{Kind: "clean"}
	}
	return gitpreflight.RevertPrediction{Kind: "conflicts", Paths: pred.Paths}
}

// PreflightRevert is preflight.revert's own orchestration (D13): status/in-progress and the
// merge-parent lookups run in parallel; the prediction and the classification both follow.
func (e *RepoEntry) PreflightRevert(ctx context.Context, shas []string, mainline *int) (gitpreflight.RevertPreflight, error) {
	var statusResult porcelain.StatusResult
	var inProgress *gitpreflight.InProgressOperation
	var mergeParents map[string][]gitpreflight.RevertParentChoice
	var statusErr, mergeErr error
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		statusResult, inProgress, statusErr = e.statusAndInProgress(ctx)
	}()
	go func() {
		defer wg.Done()
		mergeParents, mergeErr = e.revertMergeParents(ctx, shas)
	}()
	wg.Wait()
	if statusErr != nil {
		return gitpreflight.RevertPreflight{}, statusErr
	}
	if mergeErr != nil {
		return gitpreflight.RevertPreflight{}, mergeErr
	}

	var firstSha string
	if len(shas) > 0 {
		firstSha = shas[0]
	}
	prediction := e.predictRevert(ctx, firstSha, mergeParents, mainline)

	dirty := gitpreflight.DirtyPaths(statusResult)
	dirtyPaths := make([]string, len(dirty))
	for i, d := range dirty {
		dirtyPaths[i] = d.Path
	}

	head := headStateFromStatusBranch(statusResult.Branch)

	return gitpreflight.ClassifyRevert(gitpreflight.ClassifyRevertInput{
		Shas: shas, MergeParents: mergeParents, Mainline: mainline,
		DirtyPaths: dirtyPaths, InProgress: inProgress,
		DetachedHead: head.Kind == "detached",
		Prediction:   prediction,
	}), nil
}

// stashPopPrediction runs §7.6's merge-tree pop prediction — ALWAYS with --merge-base=<the stash's
// own base> (probe 2: omitting it makes a genuinely conflicting pop report clean) — against target.
// A small, deliberate duplicate of predictRevert's own merge-tree-plus-parse shape above (queries.go
// is not touched by this phase, §3.14) rather than a shared refactor of an existing function.
func (e *RepoEntry) stashPopPrediction(ctx context.Context, target, stashSha, baseSha string) gitpreflight.RevertPrediction {
	res, err := e.runAllowingExit(ctx, porcelain.MergeTreeArgs(target, stashSha, baseSha), 0, 1)
	if err != nil {
		return gitpreflight.RevertPrediction{Kind: "unknown", Reason: err.Error()}
	}
	pred, err := porcelain.ParseMergeTreeOutput(res.Stdout, res.ExitCode)
	if err != nil {
		return gitpreflight.RevertPrediction{Kind: "unknown", Reason: err.Error()}
	}
	if pred.Kind == "clean" {
		return gitpreflight.RevertPrediction{Kind: "clean"}
	}
	return gitpreflight.RevertPrediction{Kind: "conflicts", Paths: pred.Paths}
}

// resolvedCommit is resolveCommit's own return shape — a commit's canonical sha and subject.
type resolvedCommit struct {
	Sha     string
	Subject string
}

// resolveCommit resolves ref (anything `git show` can resolve — a branch, tag, or bare sha) to its
// canonical sha and subject, or (nil, nil) when ref does not resolve at all — the bad-target
// guard, shared by preflightReset (target existence + display subject) and prepareReset's own
// re-check host-side immediately before the write. Exit 128 is a resolution failure, never a spawn
// error to propagate: `show -s` on an unresolvable ref exits non-zero with real stderr, cleanly
// distinguishable from a genuine spawn failure.
//
// ref reaches ShowMetadataArgs as a bare
// argv token — a leading "-" would let it be read by `git show` as a flag of its own (e.g.
// --output=<path>, an arbitrary-file-write primitive) rather than the rev it is meant to be, and
// none of resolveCommit's four callers (PreflightReset, PreflightCherryPick,
// WorktreeAddPreflight's startPointRef, prepareReset's own re-check) ever ran it through anything
// resembling gitrpc's own validRefArg first. Every one of them already treats "does not resolve" as
// an ordinary, expected outcome — so the minimal fix is to answer exactly that way here too (nil,
// nil), rather than inventing a new error path each of the four would have to separately learn.
func (e *RepoEntry) resolveCommit(ctx context.Context, ref string) (*resolvedCommit, error) {
	if ref == "" || strings.HasPrefix(ref, "-") {
		return nil, nil
	}
	res, err := e.runAllowingExit(ctx, porcelain.ShowMetadataArgs(ref), 0, 128)
	if err != nil {
		return nil, err
	}
	if res.ExitCode == 128 {
		return nil, nil
	}
	parsed, err := porcelain.ParseLogRecordFromRaw(res.Stdout)
	if err != nil {
		return nil, err
	}
	return &resolvedCommit{Sha: parsed.SHA, Subject: parsed.Subject}, nil
}

// PreflightReset is preflight.reset's own orchestration: target may be anything `git
// show` can resolve — a branch, tag, or bare sha — so the first read is resolveCommit, which
// doubles as probe 3's bad-target guard and the source of the canonical sha leaving/gaining/
// leavingCommits are all computed against (never the wire's own possibly-abbreviated target
// string). An unresolved target short-circuits the range reads entirely: there is no commit to
// diff, count, or confirm against, and ClassifyReset's own unknownTarget blocker is what the
// dialog renders instead. Target is passed to ClassifyReset verbatim — never resolved.sha.
func (e *RepoEntry) PreflightReset(ctx context.Context, target, mode string) (gitpreflight.ResetPreflight, error) {
	var statusResult porcelain.StatusResult
	var inProgress *gitpreflight.InProgressOperation
	var resolved *resolvedCommit
	var statusErr, resolveErr error
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		statusResult, inProgress, statusErr = e.statusAndInProgress(ctx)
	}()
	go func() {
		defer wg.Done()
		resolved, resolveErr = e.resolveCommit(ctx, target)
	}()
	wg.Wait()
	if statusErr != nil {
		return gitpreflight.ResetPreflight{}, statusErr
	}
	if resolveErr != nil {
		return gitpreflight.ResetPreflight{}, resolveErr
	}

	head := headStateFromStatusBranch(statusResult.Branch)
	var branch *string
	if head.Kind == "branch" {
		name := head.Name
		branch = &name
	}
	dirty := gitpreflight.DirtySplit(statusResult)
	stagedNew := gitpreflight.StagedNewPaths(statusResult)

	if resolved == nil {
		return gitpreflight.ClassifyReset(gitpreflight.ClassifyResetInput{
			Target: target, TargetSubject: "", Mode: mode,
			CurrentHead: statusResult.Branch.OID, Branch: branch,
			Leaving: 0, Gaining: 0, LeavingCommits: []gitpreflight.ResetLeavingCommit{}, LeavingTruncated: false,
			Dirty: dirty, StagedNew: stagedNew, InProgress: inProgress, TargetResolves: false,
		}), nil
	}

	// Probe 4: `<target>...HEAD` — right is only-reachable-from-HEAD (what resetting to target
	// leaves behind), left is only-reachable-from-target (what it would additionally gain on a
	// diverged target).
	countRaw, err := e.runOne(ctx, porcelain.LeftRightCountArgs(resolved.Sha, "HEAD"))
	if err != nil {
		return gitpreflight.ResetPreflight{}, err
	}
	gaining, leaving, err := porcelain.ParseLeftRightCount(countRaw)
	if err != nil {
		return gitpreflight.ResetPreflight{}, err
	}

	leavingCommits := []gitpreflight.ResetLeavingCommit{}
	leavingTruncated := false
	if leaving > 0 {
		subjectsRaw, err := e.runOne(ctx, porcelain.RangeSubjectsArgs(resolved.Sha, "HEAD", porcelain.ResetLeavingCommitsCap))
		if err != nil {
			return gitpreflight.ResetPreflight{}, err
		}
		commits, truncated, err := porcelain.ParseRangeSubjects(subjectsRaw, porcelain.ResetLeavingCommitsCap)
		if err != nil {
			return gitpreflight.ResetPreflight{}, err
		}
		for _, c := range commits {
			leavingCommits = append(leavingCommits, gitpreflight.ResetLeavingCommit{Sha: c.Sha, Subject: c.Subject})
		}
		leavingTruncated = truncated
	}

	return gitpreflight.ClassifyReset(gitpreflight.ClassifyResetInput{
		Target: target, TargetSubject: resolved.Subject, Mode: mode,
		CurrentHead: statusResult.Branch.OID, Branch: branch,
		Leaving: leaving, Gaining: gaining,
		LeavingCommits: leavingCommits, LeavingTruncated: leavingTruncated,
		Dirty: dirty, StagedNew: stagedNew, InProgress: inProgress, TargetResolves: true,
	}), nil
}

// predictCherryPick runs the merge-tree prediction for sha — the exact INVERSE of
// predictRevert's own arrangement: HEAD is the base, sha is the other side, --merge-base=<sha>^
// <mainline> — because a pick applies its diff forwards and a revert backwards. Left to itself git
// would pick merge-base(HEAD, sha), against which a commit that undoes an earlier one on this same
// branch reads as no change at all — reporting a genuinely conflicting pick as clean.
// nil effectiveMainline (a merge with none chosen) predicts nothing: there is no single "other"
// tree to diff against yet, and mainlineRequired already names why.
func (e *RepoEntry) predictCherryPick(ctx context.Context, sha string, effectiveMainline *int) gitpreflight.RevertPrediction {
	if effectiveMainline == nil {
		return gitpreflight.RevertPrediction{Kind: "unknown", Reason: "Pick a mainline parent first."}
	}
	mergeBase := fmt.Sprintf("%s^%d", sha, *effectiveMainline)

	res, err := e.runAllowingExit(ctx, porcelain.MergeTreeArgs("HEAD", sha, mergeBase), 0, 1)
	if err != nil {
		return gitpreflight.RevertPrediction{Kind: "unknown", Reason: err.Error()}
	}
	pred, err := porcelain.ParseMergeTreeOutput(res.Stdout, res.ExitCode)
	if err != nil {
		return gitpreflight.RevertPrediction{Kind: "unknown", Reason: err.Error()}
	}
	if pred.Kind == "clean" {
		return gitpreflight.RevertPrediction{Kind: "clean"}
	}
	return gitpreflight.RevertPrediction{Kind: "conflicts", Paths: pred.Paths}
}

// cherryPickCommitPaths is the path-scoped blockers' own input: the picked commit's own
// changed paths split "added" (an untracked file at one of these is refused) from
// everything else (an unstaged change overlapping one of these is refused) — reuses
// CommitDetail's own metadata/diff-tree plumbing against whichever parent effectiveMainline names,
// rather than a third parser for the same two spawns. A renamed/copied path counts as "touched"
// under both its new and its original name. nil effectiveMainline (a merge with no mainline chosen
// yet) returns both empty, mirroring predictCherryPick's own reason.
func (e *RepoEntry) cherryPickCommitPaths(ctx context.Context, sha string, effectiveMainline *int) (gitpreflight.CherryPickCommitPaths, error) {
	if effectiveMainline == nil {
		return gitpreflight.CherryPickCommitPaths{Touched: []string{}, Added: []string{}}, nil
	}
	detail, err := e.CommitDetail(ctx, sha, *effectiveMainline-1)
	if err != nil {
		return gitpreflight.CherryPickCommitPaths{}, err
	}
	touched := []string{}
	added := []string{}
	for _, f := range detail.Files {
		if f.Kind == porcelain.FileAdded {
			added = append(added, f.Path)
			continue
		}
		touched = append(touched, f.Path)
		if f.OriginalPath != nil {
			touched = append(touched, *f.OriginalPath)
		}
	}
	return gitpreflight.CherryPickCommitPaths{Touched: touched, Added: added}, nil
}

// isAncestorOrNot runs `merge-base --is-ancestor a b` — exit 0 yes, exit 1 or 128 (a genuinely
// unresolvable ref, which cannot reach preflightCherryPick's own alreadyApplied read from a normal
// graph-row sha, but this read must not itself become a hard failure over one) no.
func (e *RepoEntry) isAncestorOrNot(ctx context.Context, a, b string) (bool, error) {
	res, err := e.runAllowingExit(ctx, []string{"merge-base", "--is-ancestor", a, b}, 0, 1, 128)
	if err != nil {
		return false, err
	}
	return res.ExitCode == 0, nil
}

// PreflightCherryPick is preflight.cherryPick's own orchestration — PreflightRevert's
// direct structural sibling. mergeParents reuses the same revertMergeParents lookup (empty unless
// sha is a merge); alreadyApplied is isAncestor(sha, HEAD) (an advisory note, never a blocker);
// commitPaths and the prediction are only ever computed once a mainline is known — with none
// chosen for a merge commit there is no single parent to diff against, and
// mainlineRequired already blocks the pick regardless of what either would say.
func (e *RepoEntry) PreflightCherryPick(ctx context.Context, sha string, mainline *int) (gitpreflight.CherryPickPreflight, error) {
	// F1 (P108 Part 16 review): sha reaches isAncestorOrNot/MergeTreeArgs/CommitDetail as a bare
	// argv token directly (not just through revertMergeParents' own guard) — validated once here,
	// at this entry point, before any of those spawns.
	if err := validOpArg("sha", sha); err != nil {
		return gitpreflight.CherryPickPreflight{}, err
	}
	var statusResult porcelain.StatusResult
	var inProgress *gitpreflight.InProgressOperation
	var mergeParentsBySha map[string][]gitpreflight.RevertParentChoice
	var alreadyApplied bool
	var resolved *resolvedCommit
	var errs [4]error
	var wg sync.WaitGroup
	wg.Add(4)
	go func() {
		defer wg.Done()
		statusResult, inProgress, errs[0] = e.statusAndInProgress(ctx)
	}()
	go func() {
		defer wg.Done()
		mergeParentsBySha, errs[1] = e.revertMergeParents(ctx, []string{sha})
	}()
	go func() {
		defer wg.Done()
		alreadyApplied, errs[2] = e.isAncestorOrNot(ctx, sha, "HEAD")
	}()
	go func() {
		defer wg.Done()
		resolved, errs[3] = e.resolveCommit(ctx, sha)
	}()
	wg.Wait()
	for _, spawnErr := range errs {
		if spawnErr != nil {
			return gitpreflight.CherryPickPreflight{}, spawnErr
		}
	}

	mergeParents := mergeParentsBySha[sha]
	if mergeParents == nil {
		mergeParents = []gitpreflight.RevertParentChoice{}
	}
	var effectiveMainline *int
	switch {
	case mainline != nil:
		effectiveMainline = mainline
	case len(mergeParents) == 0:
		one := 1
		effectiveMainline = &one
	default:
		effectiveMainline = nil
	}

	var prediction gitpreflight.RevertPrediction
	var commitPaths gitpreflight.CherryPickCommitPaths
	var pathsErr error
	var wg2 sync.WaitGroup
	wg2.Add(2)
	go func() {
		defer wg2.Done()
		prediction = e.predictCherryPick(ctx, sha, effectiveMainline)
	}()
	go func() {
		defer wg2.Done()
		commitPaths, pathsErr = e.cherryPickCommitPaths(ctx, sha, effectiveMainline)
	}()
	wg2.Wait()
	if pathsErr != nil {
		return gitpreflight.CherryPickPreflight{}, pathsErr
	}

	subject := ""
	if resolved != nil {
		subject = resolved.Subject
	}
	head := headStateFromStatusBranch(statusResult.Branch)

	return gitpreflight.ClassifyCherryPick(gitpreflight.ClassifyCherryPickInput{
		Sha: sha, Subject: subject, MergeParents: mergeParents, Mainline: mainline,
		CommitPaths: commitPaths, Dirty: gitpreflight.DirtySplit(statusResult),
		Prediction: prediction, AlreadyApplied: alreadyApplied, InProgress: inProgress,
		DetachedHead: head.Kind == "detached",
	}), nil
}

// PreflightStashPop is preflight.stashPop's own orchestration: re-resolves the stash
// entry fresh (resolveStashEntryScoped's own doc comment), gathers stashPaths (stash.show's own
// name-status), stashUntrackedPaths (an ls-tree over the untracked helper commit, when one exists),
// dirty/inProgress and the filesystem-stat existingUntrackedPaths, plus the merge-tree
// prediction against targetSha (defaulting to HEAD — the `stashAndCarry` route's own use of a
// non-HEAD target, per the contract's own doc comment on preflight.stashPop's targetSha param) —
// then calls the pure classifier. Scope ("" or "stack" for the ordinary stack, "global"
// for the bucket) only changes how entry is resolved — everything downstream (ClassifyStashPop,
// stashPopPrediction, the untracked-collision os.Stat loop) takes a porcelain.StashEntry and does
// not care where it came from.
// resolveStashPopTarget resolves PreflightStashPop's own targetSha param: a client-supplied value
// (validated — it reaches MergeTreeArgs as a bare argv token) or, absent
// one, a fresh `rev-parse HEAD`. Split out of PreflightStashPop itself to keep that function's own
// cognitive complexity under the linter's threshold.
func (e *RepoEntry) resolveStashPopTarget(ctx context.Context, targetSha *string) (string, error) {
	if targetSha == nil {
		raw, err := e.runOne(ctx, []string{"rev-parse", "HEAD"})
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(raw)), nil
	}
	if err := validOpArg("targetSha", *targetSha); err != nil {
		return "", err
	}
	return *targetSha, nil
}

func (e *RepoEntry) PreflightStashPop(ctx context.Context, sha string, targetSha *string, scope string) (gitpreflight.StashPopPreflight, error) {
	entry, err := e.resolveStashEntryScoped(ctx, sha, scope)
	if err != nil {
		return gitpreflight.StashPopPreflight{}, err
	}

	target, err := e.resolveStashPopTarget(ctx, targetSha)
	if err != nil {
		return gitpreflight.StashPopPreflight{}, err
	}

	var (
		statusResult   porcelain.StatusResult
		inProgress     *gitpreflight.InProgressOperation
		stashPaths     []string
		untrackedPaths []string
		g              errgroup.Group
	)
	g.Go(func() error {
		var serr error
		statusResult, inProgress, serr = e.statusAndInProgress(ctx)
		return serr
	})
	g.Go(func() error {
		changes, rerr := e.fileChanges(ctx, porcelain.NameStatusArgs(&entry.BaseSha, entry.Sha))
		if rerr != nil {
			return rerr
		}
		stashPaths = make([]string, len(changes))
		for i, c := range changes {
			stashPaths[i] = c.Path
		}
		return nil
	})
	if entry.UntrackedSha != nil {
		g.Go(func() error {
			var uerr error
			untrackedPaths, uerr = e.stashUntrackedPaths(ctx, *entry.UntrackedSha)
			return uerr
		})
	}
	if err := g.Wait(); err != nil {
		return gitpreflight.StashPopPreflight{}, err
	}

	// D4: a real filesystem stat per untracked path — never a git spawn (probe 3: an ignored file,
	// or one status elides, still collides; default `status` output omits ignored paths by design).
	existingUntrackedPaths := []string{}
	root := repoWorkingDir(e.Summary)
	for _, p := range untrackedPaths {
		if _, statErr := fileservice.Host().Stat(filepath.Join(root, p)); statErr == nil {
			existingUntrackedPaths = append(existingUntrackedPaths, p)
		}
	}

	prediction := e.stashPopPrediction(ctx, target, entry.Sha, entry.BaseSha)

	if stashPaths == nil {
		stashPaths = []string{}
	}
	if untrackedPaths == nil {
		untrackedPaths = []string{}
	}

	return gitpreflight.ClassifyStashPop(gitpreflight.ClassifyStashPopInput{
		Stash: entry, TargetSha: target, Prediction: prediction,
		StashPaths: stashPaths, StashUntrackedPaths: untrackedPaths,
		Dirty: gitpreflight.DirtyPaths(statusResult), ExistingUntrackedPaths: existingUntrackedPaths,
		InProgress: inProgress,
	}), nil
}

// PreflightStashBranch is preflight.stashBranch's own orchestration: re-resolves the
// stash entry fresh, then reuses the PreflightCheckout-shaped classification path at the stash's
// own base as the target ("no pop prediction — clean by construction", so this reuses
// ClassifyCheckout, never ClassifyStashPop), plus the branch-name validation ClassifyStashBranch
// itself performs. Scope-aware via resolveStashEntryScoped, same one-line change as
// PreflightStashPop above.
func (e *RepoEntry) PreflightStashBranch(ctx context.Context, sha, branch, scope string) (gitpreflight.StashBranchPreflight, error) {
	entry, err := e.resolveStashEntryScoped(ctx, sha, scope)
	if err != nil {
		return gitpreflight.StashBranchPreflight{}, err
	}

	snapshot, err := e.refsSnapshot(ctx)
	if err != nil {
		return gitpreflight.StashBranchPreflight{}, err
	}
	existingBranchNames := make(map[string]bool, len(snapshot.Branches))
	for _, b := range snapshot.Branches {
		existingBranchNames[b.ShortName] = true
	}

	var statusResult porcelain.StatusResult
	var inProgress *gitpreflight.InProgressOperation
	var rewritten []string
	var statusErr, rewrittenErr error
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		statusResult, inProgress, statusErr = e.statusAndInProgress(ctx)
	}()
	go func() {
		defer wg.Done()
		rewritten, rewrittenErr = e.rewrittenPaths(ctx, entry.BaseSha)
	}()
	wg.Wait()
	if statusErr != nil {
		return gitpreflight.StashBranchPreflight{}, statusErr
	}
	if rewrittenErr != nil {
		return gitpreflight.StashBranchPreflight{}, rewrittenErr
	}

	checkout := gitpreflight.ClassifyCheckout(gitpreflight.ClassifyCheckoutInput{
		Target:         gitpreflight.CheckoutTarget{Kind: "sha", Name: entry.BaseSha},
		Mode:           "switch",
		Dirty:          gitpreflight.DirtyPaths(statusResult),
		Rewritten:      rewritten,
		InProgress:     inProgress,
		StashAvailable: true,
	})

	return gitpreflight.ClassifyStashBranch(gitpreflight.ClassifyStashBranchInput{
		Name: branch, ExistingBranchNames: existingBranchNames, Checkout: checkout,
	}), nil
}
