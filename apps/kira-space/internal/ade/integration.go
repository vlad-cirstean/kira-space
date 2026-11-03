package ade

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitclient/porcelain"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitsession"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/packages/theme/src/i18n"
)

// patchIDScan is how many recent target commits are patch-id'd to spot a squash merge.
const patchIDScan = 500

type containState int

const (
	containNone containState = iota
	containSome
	containAll
)

// containment says how much of a branch's own commits a target holds. missing counts the commits
// the target lacks; total is the branch's own commit count above its base.
type containment struct {
	state   containState
	missing int
	total   int
}

type containKey struct{ Base, Tip, Target string }

// contains is the one evaluator behind integration and deployment facts. Own commits are
// baseTip..tip. Order: none when there are none; all when tip is in the target; then `git cherry`
// (a commit with an equivalent patch counts as held); then a squash check comparing the whole
// branch diff's patch id with the target's recent commits.
func contains(ctx context.Context, entry *gitsession.RepoEntry, caches *repoCaches, baseTip, tip, targetTip string) (containment, error) {
	key := containKey{baseTip, tip, targetTip}
	if v, ok := caches.contain.Get(key); ok {
		return v, nil
	}
	total, err := entry.CountRange(ctx, baseTip, tip)
	if err != nil {
		return containment{}, err
	}
	res := containment{total: total}
	switch {
	case total == 0:
	case targetTip == tip:
		res.state = containAll
	default:
		if res, err = containsOwn(ctx, entry, caches, baseTip, tip, targetTip, total); err != nil {
			return containment{}, err
		}
	}
	caches.contain.Add(key, res)
	return res, nil
}

func containsOwn(ctx context.Context, entry *gitsession.RepoEntry, caches *repoCaches, baseTip, tip, targetTip string, total int) (containment, error) {
	res := containment{total: total}
	in, err := entry.IsAncestor(ctx, tip, targetTip)
	if err != nil {
		return res, err
	}
	if in {
		res.state = containAll
		return res, nil
	}
	plus, _, err := entry.Cherry(ctx, targetTip, tip, baseTip)
	if err != nil {
		return res, err
	}
	switch {
	case plus == 0:
		res.state = containAll
	case plus < total:
		res.state, res.missing = containSome, plus
	default:
		res.missing = total
		squashed, err := squashMerged(ctx, entry, caches, baseTip, tip, targetTip)
		if err != nil {
			return res, err
		}
		if squashed {
			res.state, res.missing = containAll, 0
		}
	}
	return res, nil
}

func isAncestorCached(ctx context.Context, entry *gitsession.RepoEntry, caches *repoCaches, older, newer string) (bool, error) {
	key := tipPair{older, newer}
	if v, ok := caches.ancestor.Get(key); ok {
		return v, nil
	}
	v, err := entry.IsAncestor(ctx, older, newer)
	if err != nil {
		return false, err
	}
	caches.ancestor.Add(key, v)
	return v, nil
}

func countRangeCached(ctx context.Context, entry *gitsession.RepoEntry, caches *repoCaches, from, to string) (int, error) {
	key := tipPair{from, to}
	if v, ok := caches.since.Get(key); ok {
		return v, nil
	}
	v, err := entry.CountRange(ctx, from, to)
	if err != nil {
		return 0, err
	}
	caches.since.Add(key, v)
	return v, nil
}

func squashMerged(ctx context.Context, entry *gitsession.RepoEntry, caches *repoCaches, baseTip, tip, targetTip string) (bool, error) {
	id, err := entry.DiffPatchID(ctx, baseTip, tip)
	if err != nil || id == "" {
		return false, err
	}
	set, ok := caches.patchIDs.Get(targetTip)
	if !ok {
		ids, err := entry.RecentPatchIDs(ctx, targetTip, patchIDScan)
		if err != nil {
			return false, err
		}
		set = make(map[string]struct{}, len(ids))
		for _, i := range ids {
			set[i] = struct{}{}
		}
		caches.patchIDs.Add(targetTip, set)
	}
	_, found := set[id]
	return found, nil
}

// markFor returns the stored mark of kind/name on a branch.
func (sc *boardCtx) markFor(branchID, kind, name string) (model.AdeBranchMark, bool) {
	for _, m := range sc.marks[branchID] {
		if m.Kind == kind && m.Name == name {
			return m, true
		}
	}
	return model.AdeBranchMark{}, false
}

// refTip resolves a short branch name to a tip: the default remote's branch first, else local.
func (sc *boardCtx) refTip(short string) (string, bool) {
	if sc.remote != "" {
		if row, ok := findRemoteRow(sc.inv, sc.remote, short); ok {
			return row.Tip, true
		}
	}
	for _, r := range sc.inv {
		if r.Remote == "" && r.Short == short {
			return r.Tip, true
		}
	}
	return "", false
}

func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", one)
	}
	return fmt.Sprintf("%d %s", n, many)
}

// integrationFacts is one `mine` branch's merged/stale/not merged row per configured target.
func (b *TaskBoard) integrationFacts(ctx context.Context, sc *boardCtx, sb model.AdeTaskBranch, baseTip, tip string) []adewire.Integration {
	out := make([]adewire.Integration, 0, len(sc.targets))
	for _, target := range sc.targets {
		row, err := b.integrationRow(ctx, sc, sb, baseTip, tip, target)
		if err != nil {
			slog.Warn("ade board: integration", "scope", "ade", "repo", sc.repoID, "branch", sb.Name, "target", target, "err", err)
			row = adewire.Integration{Target: target, Status: "not merged", Note: i18n.T("backend.space.ade.unchecked.body", nil)}
		}
		out = append(out, row)
	}
	return out
}

func (b *TaskBoard) integrationRow(ctx context.Context, sc *boardCtx, sb model.AdeTaskBranch, baseTip, tip, target string) (adewire.Integration, error) {
	mark, hasMark := sc.markFor(sb.ID, "target", target)
	row := adewire.Integration{Target: target, Status: "not merged", Recorded: hasMark && mark.Recorded}
	targetTip, found := sc.refTip(target)
	if !found {
		row.Note = target + " not found"
		return row, nil
	}
	if hasMark && mark.MergedTip != tip {
		descends, err := isAncestorCached(ctx, sc.entry, sc.caches, mark.MergedTip, tip)
		if err != nil {
			return row, err
		}
		inTarget, err := isAncestorCached(ctx, sc.entry, sc.caches, tip, targetTip)
		if err != nil {
			return row, err
		}
		if !descends && !inTarget {
			row.Status, row.Note = "stale", "rebased since it was merged"
			return row, nil
		}
	}
	c, err := contains(ctx, sc.entry, sc.caches, baseTip, tip, targetTip)
	if err != nil {
		return row, err
	}
	switch {
	case c.total == 0:
		return row, nil
	case c.state == containAll:
		row.Status, row.Note = "merged", "up to date"
		if !hasMark || mark.MergedTip != tip {
			b.saveMark(sc, sb.ID, "target", target, tip)
		}
	case hasMark:
		row.Status = "stale"
		since, err := countRangeCached(ctx, sc.entry, sc.caches, mark.MergedTip, tip)
		if err != nil {
			return row, err
		}
		if since > 0 {
			row.Note = plural(since, "commit", "commits") + " since it was merged"
		} else {
			row.Note = "no longer in " + target
		}
	case c.state == containSome:
		row.Status, row.Note = "stale", plural(c.missing, "commit", "commits")+" since it was merged"
	}
	return row, nil
}

// saveMark remembers that tip was fully contained; recorded flags are left to RecordMerge.
func (b *TaskBoard) saveMark(sc *boardCtx, branchID, kind, name, tip string) {
	if b.deps.Facts == nil {
		return
	}
	err := b.deps.Facts.UpsertMark(model.AdeBranchMark{
		BranchID: branchID, Kind: kind, Name: name, MergedTip: tip, UpdatedAt: sc.nowMs,
	})
	if err != nil {
		slog.Warn("ade board: save mark", "scope", "ade", "repo", sc.repoID, "branch", branchID, "err", err)
	}
}

// RecordMerge remembers that the app's merge dialog merged a branch into target. It is the only
// writer of recorded=1; the mark keeps the branch tip at that moment.
func (b *TaskBoard) RecordMerge(ctx context.Context, branchID, target string) error {
	branches, err := b.deps.Tasks.BranchesLive()
	if err != nil {
		return err
	}
	var sb *model.AdeTaskBranch
	for i := range branches {
		if branches[i].ID == branchID {
			sb = &branches[i]
		}
	}
	if sb == nil {
		return invalid("branch %s not found", branchID)
	}
	if sb.Kind != model.AdeBranchKindMine || sb.Name == "" {
		return invalid("only a created branch of mine can record a merge")
	}
	cfg, err := b.repoConfig(sb.CodeRepoID)
	if err != nil {
		return err
	}
	configured := false
	for _, t := range withoutMain(cfg.IntegrationBranches, b.mainShortName(ctx, sb.CodeRepoID)) {
		configured = configured || t == target
	}
	if !configured {
		return invalid("%q is not an integration branch of this repo", target)
	}
	entry, err := b.openRepo(ctx, sb.CodeRepoID)
	if err != nil {
		return err
	}
	inv, err := entry.BranchInventory(ctx)
	if err != nil {
		return err
	}
	remote, _ := entry.DefaultRemote(ctx)
	row, ok := resolveQueuedRef(inv, sb.Name, remote)
	if !ok {
		return invalid("branch %s does not exist in the repo", sb.Name)
	}
	if b.deps.Facts == nil {
		return invalid("facts store is not available")
	}
	held, err := tipInTarget(ctx, entry, inv, remote, row.Tip, target)
	if err != nil {
		return err
	}
	if !held {
		return invalid("%s is not in %s yet; nothing recorded", sb.Name, target)
	}
	err = b.deps.Facts.UpsertMark(model.AdeBranchMark{
		BranchID: sb.ID, Kind: "target", Name: target, MergedTip: row.Tip, Recorded: true, UpdatedAt: b.deps.Now().UnixMilli(),
	})
	if err != nil {
		return err
	}
	b.notifyBoard()
	return nil
}

// tipInTarget reports whether tip is reachable from the local or the default remote's target branch.
func tipInTarget(ctx context.Context, entry *gitsession.RepoEntry, inv []porcelain.InventoryRef, remote, tip, target string) (bool, error) {
	for _, r := range inv {
		if r.Short != target || (r.Remote != "" && r.Remote != remote) {
			continue
		}
		in, err := entry.IsAncestor(ctx, tip, r.Tip)
		if err != nil {
			return false, err
		}
		if in {
			return true, nil
		}
	}
	return false, nil
}

// marksOfRepo reads the stored marks of one repo's live branches, keyed branch id + target name.
func (b *TaskBoard) marksOfRepo(codeRepoID string) (map[string]model.AdeBranchMark, error) {
	out := map[string]model.AdeBranchMark{}
	if b.deps.Facts == nil {
		return out, nil
	}
	live, err := b.deps.Tasks.BranchesLive()
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, br := range live {
		if br.CodeRepoID == codeRepoID && br.Kind == model.AdeBranchKindMine && br.Name != "" {
			ids = append(ids, br.ID)
		}
	}
	marks, err := b.deps.Facts.Marks(ids)
	if err != nil {
		return nil, err
	}
	for id, list := range marks {
		for _, m := range list {
			if m.Kind == "target" {
				out[id+"\x00"+m.Name] = m
			}
		}
	}
	return out, nil
}

// newlyMerged lists the targets a branch is merged into now that it was not merged into at its
// current tip before the fetch (no mark, or a mark at an older tip).
func newlyMerged(branches []model.AdeTaskBranch, facts *repoResult, before map[string]model.AdeBranchMark) []adewire.MergedInto {
	out := []adewire.MergedInto{}
	for _, sb := range branches {
		wb, ok := facts.branches[sb.ID]
		if !ok {
			continue
		}
		for _, in := range wb.Integration {
			if in.Status != "merged" {
				continue
			}
			if m, had := before[sb.ID+"\x00"+in.Target]; !had || m.MergedTip != wb.Tip {
				out = append(out, adewire.MergedInto{BranchID: sb.ID, Target: in.Target})
			}
		}
	}
	return out
}
