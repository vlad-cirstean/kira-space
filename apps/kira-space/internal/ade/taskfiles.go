package ade

import (
	"context"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitsession"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// taskWorktree loads a task branch, its repo entry and worktree path ("" when it has none). The
// path comes from the branch row, never from the caller. gate, when set, runs on the loaded task
// before the repo opens.
func (b *TaskBoard) taskWorktree(ctx context.Context, branchID string, gate func(tc *taskCtx, sb model.AdeTaskBranch) error) (*gitsession.RepoEntry, model.AdeTaskBranch, string, error) {
	sb, err := b.deps.Tasks.GetBranch(branchID)
	if err != nil {
		return nil, sb, "", err
	}
	tc, err := b.loadTaskCtx(sb.TaskID)
	if err != nil {
		return nil, sb, "", err
	}
	if gate != nil {
		if err := gate(tc, sb); err != nil {
			return nil, sb, "", err
		}
	}
	entry, err := b.openRepo(ctx, sb.CodeRepoID)
	if err != nil {
		return nil, sb, "", err
	}
	worktree, err := b.worktreeOf(ctx, sb)
	return entry, sb, worktree, err
}
