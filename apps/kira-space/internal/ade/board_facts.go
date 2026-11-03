package ade

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"golang.org/x/sync/errgroup"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitclient/porcelain"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitsession"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// repoResult is one repo's computed branch facts. jobs holds the rebase-conflict check of every
// visible branch (created, resolved, unmerged), keyed by branch id; the caller decides whether to
// peek, queue or await them.
type repoResult struct {
	state      adewire.RepoState
	branches   map[string]adewire.Branch
	pairs      []adewire.Pair
	hadCommits []string
	merged     map[string]int64
	jobs       map[string]rebaseJob
}

// zeroBranch is a branch with only its stored meta: not created, or no git facts available.
func zeroBranch(sb model.AdeTaskBranch, base string, setup *adewire.WorktreeSetup) adewire.Branch {
	return adewire.Branch{
		ID: sb.ID, TaskID: sb.TaskID, CodeRepoID: sb.CodeRepoID, Name: sb.Name, Kind: sb.Kind, Base: base,
		BaseBranchID: sb.BaseBranchID, BasePendingFrom: sb.BasePendingFrom,
		Setup: setup, Integration: []adewire.Integration{}, Deployments: []adewire.Deployment{},
		ConflictsIfRebased: []string{}, ConflictCheck: conflictDone, Dirty: []adewire.DirtyEntry{}, AddedAt: sb.AddedAt, Origin: sb.Origin,
	}
}

func toWireSetup(s model.AdeWorktreeSetup) *adewire.WorktreeSetup {
	return &adewire.WorktreeSetup{State: s.State, StartedAt: s.StartedAt, FinishedAt: s.FinishedAt, ExitCode: s.ExitCode, Note: s.Note}
}

// boardCtx holds one repo's git inputs shared by every branch computation.
type boardCtx struct {
	entry     *gitsession.RepoEntry
	repoID    string
	inv       []porcelain.InventoryRef
	userEmail string
	remote    string
	mainName  string
	mainRef   string
	mainFull  string
	mainTip   string
	hasMain   bool
	caches    *repoCaches
	nowMs     int64
	resolved  map[string]resolvedRef // branch id -> resolved ref
	byName    map[string]model.AdeTaskBranch
	byID      map[string]model.AdeTaskBranch
	// P145 facts: configured integration targets and environments, the stored marks and env states.
	targets  []string
	envs     []model.AdeRepoEnv
	envState map[string]model.AdeEnvState
	marks    map[string][]model.AdeBranchMark // branch id -> marks
}

// ownerOf is the display owner of a ref: empty for the user's own commits.
func (sc *boardCtx) ownerOf(row porcelain.InventoryRef) string {
	if sc.userEmail != "" && strings.EqualFold(row.AuthorEmail, sc.userEmail) {
		return ""
	}
	return row.AuthorName
}

// baseReason says why a base did not resolve.
type baseReason string

const (
	baseParentDraft baseReason = "parentDraft"
	baseMissing     baseReason = "baseMissing"
)

// baseRef is a branch's resolved latest base. ref is the spelling git takes (a short local name, or
// <remote>/<short>); row is its inventory row, empty for main.
type baseRef struct {
	tip      string
	branchID string
	owner    string
	name     string
	ref      string
	full     string
	row      porcelain.InventoryRef
	ok       bool
	reason   baseReason
}

// resolveBase is the one base rule (P241): a live planner parent by id, main for ” or the main
// short name, another live planner branch of the repo by name, else the default remote's branch,
// else the local one.
func (sc *boardCtx) resolveBase(sb model.AdeTaskBranch) baseRef {
	if sb.BaseBranchID != "" {
		if other, ok := sc.byID[sb.BaseBranchID]; ok && other.ID != sb.ID {
			if other.Name == "" {
				return baseRef{reason: baseParentDraft}
			}
			if br, ok := sc.plannerRef(other); ok {
				return br
			}
			return baseRef{name: other.Name, reason: baseMissing}
		}
	}
	if sb.Base == "" || sb.Base == sc.mainName {
		return baseRef{tip: sc.mainTip, name: sc.mainName, ref: sc.mainRef, full: sc.mainFull, ok: sc.hasMain}
	}
	if other, ok := sc.byName[sb.Base]; ok && other.ID != sb.ID {
		if br, ok := sc.plannerRef(other); ok {
			return br
		}
	}
	if row, ok := resolveBaseRow(sc.inv, sb.Base, sc.remote); ok {
		return baseRef{tip: row.Tip, owner: sc.ownerOf(row), name: sb.Base, ref: refSpelling(row), full: row.Ref, row: row, ok: true}
	}
	return baseRef{name: sb.Base, reason: baseMissing}
}

// plannerRef resolves another planner branch by its own name (local first).
func (sc *boardCtx) plannerRef(other model.AdeTaskBranch) (baseRef, bool) {
	row, found := resolveQueuedRef(sc.inv, other.Name, sc.remote)
	if !found {
		return baseRef{}, false
	}
	owner := ""
	if other.Kind != model.AdeBranchKindMine {
		owner = sc.ownerOf(row)
	}
	return baseRef{tip: row.Tip, branchID: other.ID, owner: owner, name: other.Name, ref: refSpelling(row), full: row.Ref, row: row, ok: true}, true
}

type branchOut struct {
	wb        adewire.Branch
	pi        *pairItem
	tip, ref  string
	hadNew    bool
	mergedNew bool
	job       *rebaseJob
}

// repoFacts computes one repo's branch facts. A repo that cannot be read degrades to stored-meta
// branches (logged), never failing the whole board.
func (b *TaskBoard) repoFacts(ctx context.Context, codeRepoID string, branches []model.AdeTaskBranch, setups map[string]model.AdeWorktreeSetup) *repoResult {
	res := &repoResult{
		state:    adewire.RepoState{CodeRepoID: codeRepoID},
		branches: make(map[string]adewire.Branch, len(branches)), pairs: []adewire.Pair{},
		merged: map[string]int64{}, jobs: map[string]rebaseJob{},
	}
	setupOf := func(id string) *adewire.WorktreeSetup {
		if s, ok := setups[id]; ok {
			return toWireSetup(s)
		}
		return nil
	}
	fallback := func() {
		for _, sb := range branches {
			res.branches[sb.ID] = zeroBranch(sb, sb.Base, setupOf(sb.ID))
		}
	}
	entry, err := b.openRepo(ctx, codeRepoID)
	if err != nil {
		slog.Warn("ade board: open repo", "scope", "ade", "repo", codeRepoID, "err", err)
		fallback()
		return res
	}
	remote, _ := entry.DefaultRemote(ctx)
	res.state.Remote, res.state.LastFetchAt = remote, lastFetchAt(entry)
	if err := b.collectRepo(ctx, entry, codeRepoID, branches, setupOf, res); err != nil {
		slog.Warn("ade board: repo facts", "scope", "ade", "repo", codeRepoID, "err", err)
		res.hadCommits, res.merged, res.jobs, res.pairs = nil, map[string]int64{}, map[string]rebaseJob{}, []adewire.Pair{}
		fallback()
	}
	return res
}

func (b *TaskBoard) collectRepo(ctx context.Context, entry *gitsession.RepoEntry, codeRepoID string, branches []model.AdeTaskBranch, setupOf func(string) *adewire.WorktreeSetup, res *repoResult) error {
	inv, err := entry.BranchInventory(ctx)
	if err != nil {
		return err
	}
	mainRefName, mainTip, hasMain, err := entry.MainRef(ctx)
	if err != nil {
		return err
	}
	userEmail, err := entry.ConfigValue(ctx, "user.email")
	if err != nil {
		return err
	}
	remote := res.state.Remote
	caches, rebaseC := b.cachesFor(codeRepoID)
	sc := &boardCtx{
		entry: entry, repoID: codeRepoID, inv: inv, userEmail: userEmail, remote: remote, mainTip: mainTip,
		hasMain: hasMain, caches: caches, nowMs: b.deps.Now().UnixMilli(),
		resolved: map[string]resolvedRef{}, byName: map[string]model.AdeTaskBranch{}, byID: map[string]model.AdeTaskBranch{},
	}
	if hasMain {
		sc.mainName, sc.mainRef = mainDisplay(mainRefName)
		sc.mainFull = mainRefName
	}
	res.state.MainName = sc.mainName
	if err := b.loadFacts(sc, branches); err != nil {
		return err
	}
	for _, sb := range branches {
		sc.byID[sb.ID] = sb
		if sb.Name == "" {
			continue
		}
		sc.byName[sb.Name] = sb
		row, found := resolveQueuedRef(inv, sb.Name, remote)
		sc.resolved[sb.ID] = resolvedRef{row: row, found: found}
	}

	outs := make([]branchOut, len(branches))
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(boardFanOut)
	for i, sb := range branches {
		g.Go(func() error {
			out, err := b.computeBranch(gctx, sc, sb, rebaseC, setupOf(sb.ID))
			outs[i] = out
			return err
		})
	}
	if err := g.Wait(); err != nil {
		return err
	}

	var items []pairItem
	tips, refs := map[string]string{}, map[string]string{}
	var existing []string
	for i, sb := range branches {
		o := outs[i]
		res.branches[sb.ID] = o.wb
		if o.hadNew {
			res.hadCommits = append(res.hadCommits, sb.ID)
		}
		if o.mergedNew {
			res.merged[sb.ID] = sc.nowMs
		}
		if o.job != nil {
			res.jobs[sb.ID] = *o.job
		}
		if o.tip != "" {
			existing = append(existing, sb.ID)
			tips[sb.ID], refs[sb.ID] = o.tip, o.ref
		}
		if o.pi != nil {
			items = append(items, *o.pi)
		}
	}
	if len(items) < 2 {
		return nil
	}
	ancestry, err := computeAncestry(ctx, entry, existing, tips, refs)
	if err != nil {
		return err
	}
	pairs, err := computePairFacts(ctx, entry, caches, ancestry, items)
	if err != nil {
		return err
	}
	for _, p := range pairs {
		res.pairs = append(res.pairs, adewire.Pair{A: p.A, B: p.B, Shared: nonNil(p.Shared), Conflicts: nonNil(p.Conflicts)})
	}
	return nil
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// computeBranch assembles one branch's wire facts.
func (b *TaskBoard) computeBranch(ctx context.Context, sc *boardCtx, sb model.AdeTaskBranch, rebaseC *rebaseCache, setup *adewire.WorktreeSetup) (branchOut, error) {
	baseName := sb.Base
	if baseName == "" {
		baseName = sc.mainName
	}
	out := branchOut{wb: zeroBranch(sb, baseName, setup)}
	rr, resolved := sc.resolved[sb.ID]
	if sb.Name == "" || !resolved || !rr.found {
		return out, nil
	}
	row := rr.row
	wb := &out.wb
	out.tip, out.ref = row.Tip, row.Ref
	wb.Tip, wb.Owner = row.Tip, sc.ownerOf(row)
	lastCommit := row.CommitterUnix * 1000
	wb.LastCommitAt = &lastCommit
	wb.Worktree = row.WorktreePath
	if wb.Worktree == "" && wb.Setup != nil && wb.Setup.State == "ready" {
		wb.Setup = nil // worktree gone (archive, restore): a ready setup row is stale
	}
	if err := sc.fillUpstream(ctx, wb, sb, row); err != nil {
		return out, err
	}

	base := sc.resolveBase(sb)
	wb.BaseBranchID, wb.BaseOwner = base.branchID, base.owner
	wb.BaseMissing = base.reason == baseMissing
	if base.name != "" {
		baseName, wb.Base = base.name, base.name
	}
	var files []porcelain.FileChange
	if base.ok {
		rf, err := rangeFacts(ctx, sc.entry, sc.caches, base.tip, row.Tip)
		if err != nil {
			return out, err
		}
		wb.Ahead, wb.Behind = rf.Ahead, rf.Behind
		files = rf.Files
	}
	depth, err := sc.mainDepth(ctx, sb, row, wb.Ahead)
	if err != nil {
		return out, err
	}

	if wb.Worktree != "" {
		status, err := sc.entry.WorktreeStatus(ctx, wb.Worktree)
		if err != nil {
			return out, err
		}
		for _, d := range toDirtyEntries(status) {
			wb.Dirty = append(wb.Dirty, adewire.DirtyEntry{Code: d.Code, Path: d.Path})
		}
		wb.RebaseInProgress = sc.entry.WorktreeInProgress(wb.Worktree, status) != nil
	}

	if sb.Kind == model.AdeBranchKindMine && base.ok {
		wb.Integration = b.integrationFacts(ctx, sc, sb, base.tip, row.Tip)
		wb.Deployments = b.deploymentFacts(ctx, sc, sb, base.tip, row.Tip)
	}

	hadCommits := sb.HadCommits || depth > 0
	out.hadNew = hadCommits && !sb.HadCommits
	byAncestor := mergedRule(sc.hasMain && depth == 0, hadCommits, "")
	wb.MergedIntoMain = byAncestor || sb.MergedAt != nil
	switch {
	case sb.MergedAt != nil:
		wb.MergedAt = sb.MergedAt
	case byAncestor:
		now := sc.nowMs
		wb.MergedAt = &now
		out.mergedNew = true
	}

	if sb.Kind == model.AdeBranchKindMine || sb.Kind == model.AdeBranchKindReview {
		out.pi = &pairItem{Item: sb.ID, Tip: row.Tip, Kind: sb.Kind, Merged: wb.MergedIntoMain, Files: filesSet(files)}
	}
	switch {
	case wb.MergedIntoMain:
	case base.reason == baseParentDraft:
		wb.ConflictCheck, wb.ConflictCheckReason = conflictFailed, "base branch is not created yet"
	case !base.ok:
		wb.ConflictCheck, wb.ConflictCheckReason = conflictFailed, fmt.Sprintf("base %s not found", baseName)
	default:
		out.job = &rebaseJob{
			repoID: sc.repoID, key: rebaseKey{BaseTip: base.tip, Tip: row.Tip}, cache: rebaseC,
			run: mergeTreeRun(sc.entry), branch: sb.Name, base: baseName,
		}
	}
	return out, nil
}

// fillUpstream sets the tracked upstream, falling back to a same-named remote branch.
func (sc *boardCtx) fillUpstream(ctx context.Context, wb *adewire.Branch, sb model.AdeTaskBranch, row porcelain.InventoryRef) error {
	wb.Upstream, wb.UpstreamAhead, wb.UpstreamBehind = row.Upstream, row.UpstreamAhead, row.UpstreamBehind
	if row.Upstream != "" || sc.remote == "" {
		return nil
	}
	remoteRow, found := findRemoteRow(sc.inv, sc.remote, sb.Name)
	if !found {
		return nil
	}
	ahead, behind, err := sc.entry.AheadBehind(ctx, row.Tip, remoteRow.Tip)
	if err != nil {
		return err
	}
	wb.Upstream, wb.UpstreamAhead, wb.UpstreamBehind = remoteRow.Ref, ahead, behind
	return nil
}

// mainDepth is the branch's commit count over main; baseAhead is reused when main is the base.
func (sc *boardCtx) mainDepth(ctx context.Context, sb model.AdeTaskBranch, row porcelain.InventoryRef, baseAhead int) (int, error) {
	if !sc.hasMain {
		return 0, nil
	}
	if sb.BaseBranchID == "" && (sb.Base == "" || sb.Base == sc.mainName) {
		return baseAhead, nil
	}
	ahead, _, err := sc.entry.AheadBehind(ctx, row.Tip, sc.mainTip)
	return ahead, err
}

// loadFacts reads the configured integration targets, environments, stored marks and env states of
// one repo into sc.
func (b *TaskBoard) loadFacts(sc *boardCtx, branches []model.AdeTaskBranch) error {
	configs, err := b.deps.RepoConfig.List()
	if err != nil {
		return err
	}
	for _, c := range configs {
		if c.CodeRepoID == sc.repoID {
			sc.targets, sc.envs = withoutMain(c.IntegrationBranches, sc.mainName), c.Environments
		}
	}
	if b.deps.Facts == nil {
		return nil
	}
	ids := make([]string, 0, len(branches))
	for _, sb := range branches {
		if sb.Kind == model.AdeBranchKindMine && sb.Name != "" {
			ids = append(ids, sb.ID)
		}
	}
	if sc.marks, err = b.deps.Facts.Marks(ids); err != nil {
		return err
	}
	sc.envState, err = b.deps.Facts.EnvStates(sc.repoID)
	return err
}
