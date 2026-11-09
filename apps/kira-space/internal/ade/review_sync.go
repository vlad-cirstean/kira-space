package ade

import (
	"context"
	"slices"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitclient/porcelain"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitsession"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// review_sync.go: how fresh a review's branch and base are, and the two actions that refresh them
// (P581). Facts compare local refs only, so they are as current as the last fetch.

const (
	reviewSideBranch = "branch"
	reviewSideBase   = "base"
)

// syncSide is one resolved side of a review: its row, the twin when one exists and the counts.
type syncSide struct {
	row           porcelain.InventoryRef
	twin          porcelain.InventoryRef
	hasTwin       bool
	ahead, behind int
	missing       bool   // base not found: only the stored name is known
	name          string // spelling the review uses
}

func (b *TaskBoard) syncSideOf(ctx context.Context, entry *gitsession.RepoEntry, refs reviewRefs, side string) (syncSide, error) {
	if side == reviewSideBranch {
		return b.syncSideFor(ctx, entry, refs, refs.branch, refSpelling(refs.branch))
	}
	if !refs.base.ok {
		return syncSide{missing: true, name: refs.baseSpelling}, nil
	}
	// The row is empty for main; its full refname is in the inventory.
	for _, r := range refs.inv {
		if r.Ref == refs.base.full {
			return b.syncSideFor(ctx, entry, refs, r, refs.base.ref)
		}
	}
	return syncSide{missing: true, name: refs.baseSpelling}, nil
}

func (b *TaskBoard) syncSideFor(ctx context.Context, entry *gitsession.RepoEntry, refs reviewRefs, row porcelain.InventoryRef, name string) (syncSide, error) {
	out := syncSide{row: row, name: name}
	if row.Remote != "" {
		return out, nil
	}
	twin, ahead, behind, ok, err := remoteTwin(ctx, entry, refs.inv, row, refs.remote)
	if err != nil {
		return syncSide{}, err
	}
	out.twin, out.ahead, out.behind, out.hasTwin = twin, ahead, behind, ok
	return out, nil
}

func (s syncSide) wire() adewire.RefSync {
	if s.missing {
		return adewire.RefSync{Name: s.name, Short: s.name, State: "noRemote"}
	}
	out := adewire.RefSync{Name: s.name, Short: s.row.Short, Local: s.row.Remote == "", Worktree: s.row.WorktreePath}
	switch {
	case !out.Local:
		out.State = "remote"
	case !s.hasTwin:
		out.State = "noRemote"
	default:
		out.Twin, out.Ahead, out.Behind = refSpelling(s.twin), s.ahead, s.behind
		switch {
		case s.ahead == 0 && s.behind == 0:
			out.State = "current"
		case s.behind == 0:
			out.State = "ahead"
		case s.ahead == 0:
			out.State = "behind"
		default:
			out.State = "diverged"
		}
	}
	return out
}

func (b *TaskBoard) reviewSyncFacts(ctx context.Context, entry *gitsession.RepoEntry, sb model.AdeTaskBranch) (adewire.ReviewSync, reviewRefs, error) {
	refs, err := b.reviewRefs(ctx, entry, sb)
	if err != nil {
		return adewire.ReviewSync{}, reviewRefs{}, err
	}
	out := adewire.ReviewSync{Remote: refs.remote, LastFetchAt: lastFetchAt(entry)}
	for _, side := range []string{reviewSideBranch, reviewSideBase} {
		s, err := b.syncSideOf(ctx, entry, refs, side)
		if err != nil {
			return adewire.ReviewSync{}, reviewRefs{}, err
		}
		if side == reviewSideBranch {
			out.Branch = s.wire()
		} else {
			out.Base = s.wire()
		}
	}
	return out, refs, nil
}

// ReviewSync reports the branch's and base's freshness against their remote twins. Read only.
func (b *TaskBoard) ReviewSync(ctx context.Context, branchID string) (adewire.ReviewSync, error) {
	entry, sb, _, err := b.reviewWorktree(ctx, branchID)
	if err != nil {
		return adewire.ReviewSync{}, err
	}
	out, _, err := b.reviewSyncFacts(ctx, entry, sb)
	return out, err
}

// ReviewFetch fetches every remote the review depends on: the default remote plus the remotes of
// the branch and base twins. Whole remotes with prune, as the repo chip does, so a deleted remote
// branch disappears. git owns credentials; only the askpass prompt is interposed.
func (b *TaskBoard) ReviewFetch(ctx context.Context, branchID string) (adewire.ReviewFetchResult, error) {
	entry, sb, _, err := b.reviewWorktree(ctx, branchID)
	if err != nil {
		return adewire.ReviewFetchResult{}, err
	}
	mu := b.repoMutex(sb.CodeRepoID)
	mu.Lock()
	defer mu.Unlock()
	refs, err := b.reviewRefs(ctx, entry, sb)
	if err != nil {
		return adewire.ReviewFetchResult{}, err
	}
	var remotes []string
	add := func(r string) {
		if r != "" && !slices.Contains(remotes, r) {
			remotes = append(remotes, r)
		}
	}
	add(refs.remote)
	for _, side := range []string{reviewSideBranch, reviewSideBase} {
		s, err := b.syncSideOf(ctx, entry, refs, side)
		if err != nil {
			return adewire.ReviewFetchResult{}, err
		}
		if s.missing {
			continue
		}
		add(s.row.Remote)
		if s.hasTwin {
			add(s.twin.Remote)
		}
	}
	if len(remotes) == 0 {
		return adewire.ReviewFetchResult{}, invalid("the repository has no remote")
	}
	defer b.notifyBoard()
	for _, remote := range remotes {
		res, err := entry.RunRemote(ctx, b.conn, gitsession.RemoteOpParams{Kind: "fetch", Remote: remote, Prune: true}, gitsession.RemoteDeps{Askpass: b.deps.Askpass})
		if err != nil {
			return adewire.ReviewFetchResult{Error: &adewire.RemoteOpError{Kind: "Unknown", Message: err.Error()}}, nil
		}
		if !res.OK {
			return adewire.ReviewFetchResult{Error: remoteOpError(res.Error)}, nil
		}
	}
	return adewire.ReviewFetchResult{}, nil
}

// ReviewUpdateLocal fast-forwards the local branch or base to its remote twin, never anything else:
// diverged, ahead and remote spellings are refused, and git refuses a merge that would overwrite
// local changes.
func (b *TaskBoard) ReviewUpdateLocal(ctx context.Context, branchID, side string) (adewire.ReviewSync, error) {
	if side != reviewSideBranch && side != reviewSideBase {
		return adewire.ReviewSync{}, invalid("side must be branch or base")
	}
	entry, sb, _, err := b.reviewWorktree(ctx, branchID)
	if err != nil {
		return adewire.ReviewSync{}, err
	}
	mu := b.repoMutex(sb.CodeRepoID)
	mu.Lock()
	defer mu.Unlock()
	refs, err := b.reviewRefs(ctx, entry, sb)
	if err != nil {
		return adewire.ReviewSync{}, err
	}
	s, err := b.syncSideOf(ctx, entry, refs, side)
	if err != nil {
		return adewire.ReviewSync{}, err
	}
	if err := b.checkFastForward(ctx, entry, s); err != nil {
		return adewire.ReviewSync{}, err
	}
	if s.row.WorktreePath != "" {
		err = entry.FastForwardWorktree(ctx, s.row.WorktreePath, s.twin.Ref)
	} else {
		err = entry.FastForwardRef(ctx, s.row.Ref, s.twin.Tip, s.row.Tip)
	}
	b.notifyBoard()
	if err != nil {
		return adewire.ReviewSync{}, err
	}
	out, _, err := b.reviewSyncFacts(ctx, entry, sb)
	return out, err
}

// checkFastForward refuses unless s is a local ref strictly behind its twin and, when checked
// out, its worktree is idle with the ref as HEAD.
func (b *TaskBoard) checkFastForward(ctx context.Context, entry *gitsession.RepoEntry, s syncSide) error {
	switch {
	case s.missing:
		return invalid("%s was not found", s.name)
	case s.row.Remote != "":
		return invalid("%s is a remote branch; fetch updates it", s.name)
	case !s.hasTwin:
		return invalid("%s has no remote branch to update from", s.name)
	}
	behind, err := entry.IsAncestor(ctx, s.row.Tip, s.twin.Tip)
	if err != nil {
		return err
	}
	if !behind || s.row.Tip == s.twin.Tip {
		return invalid("%s is not behind %s; the review only fast-forwards", s.name, refSpelling(s.twin))
	}
	if s.row.WorktreePath == "" {
		return nil
	}
	status, err := entry.WorktreeStatus(ctx, s.row.WorktreePath)
	if err != nil {
		return err
	}
	if op := entry.WorktreeInProgress(s.row.WorktreePath, status); op != nil {
		return invalid("a %s is in progress in %s", op.Kind, s.row.WorktreePath)
	}
	head, _, err := entry.WorktreeHead(ctx, s.row.WorktreePath)
	if err != nil {
		return err
	}
	if head != s.row.Ref {
		return invalid("%s is not the checked-out branch of %s", s.name, s.row.WorktreePath)
	}
	return nil
}

