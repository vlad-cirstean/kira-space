package ade

import (
	"cmp"
	"context"
	"log/slog"
	"slices"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/repos"
	"github.com/kirathecat/kira-studio/internal/ids"
)

// taskbranches.go edits the repos and branches of a task row by row (P397). A task holds one row per
// repo; a row with no name is a repo without a branch. Detaching a row removes the task link and the
// ADE linked worktree, never the git branch.

// editableRow loads the row's task and refuses a change while anything works on the row. The caller
// holds the task mutex. A running session is refused, not closed, so no terminal sits in a removed
// worktree.
func (b *TaskBoard) editableRow(ctx context.Context, taskID, branchID string) (*taskCtx, model.AdeTaskBranch, error) {
	tc, err := b.loadTaskCtx(taskID)
	if err != nil {
		return nil, model.AdeTaskBranch{}, err
	}
	if err := b.checkNotArchiving(taskID); err != nil {
		return nil, model.AdeTaskBranch{}, err
	}
	if tc.task.Kind == model.AdeTaskKindReview {
		return nil, model.AdeTaskBranch{}, invalid("the repos and branches of a review task cannot change")
	}
	row, ok := tc.branch(branchID)
	if !ok {
		return nil, model.AdeTaskBranch{}, repos.ErrBranchMissing
	}
	label := cmp.Or(row.Name, tc.nick[row.CodeRepoID])
	b.runMu.Lock()
	_, setting := b.setupLive[row.ID]
	b.runMu.Unlock()
	if setting {
		return nil, model.AdeTaskBranch{}, invalid("a prepare script is running on %s", label)
	}
	// A step session carries no branch id; its cwd is the worktree.
	wt := b.worktreePath(ctx, row)
	open, err := b.runningTUI(func(s model.AdeSession) bool {
		return s.BranchID == row.ID || (wt != "" && s.TaskID == taskID && s.Cwd == wt)
	})
	if err != nil {
		return nil, model.AdeTaskBranch{}, err
	}
	if open != nil {
		return nil, model.AdeTaskBranch{}, invalid("a session is open on %s: close it first", label)
	}
	if tr := b.deps.Tracker; tr != nil && tr.hasPending(func(p pendingIntent) bool { return p.BranchID == row.ID }) {
		return nil, model.AdeTaskBranch{}, invalid("a session is starting on %s", label)
	}
	if err := b.checkNoBackgroundRun(row); err != nil {
		return nil, model.AdeTaskBranch{}, err
	}
	return tc, row, nil
}

// ownKind is the kind a row keeps when its branch is replaced by a branch this app creates: a created
// branch is never a review branch.
func ownKind(kind string) string {
	if kind == model.AdeBranchKindReview {
		return model.AdeBranchKindMine
	}
	return kind
}

// detachRow takes the row off the task, putting replacement (when given) in its place, then removes
// the row's linked worktree. A worktree with uncommitted changes needs discard. The caller holds the
// task mutex and has run editableRow.
func (b *TaskBoard) detachRow(ctx context.Context, old model.AdeTaskBranch, replacement *model.AdeTaskBranch, discard bool) error {
	lb, linked, err := b.linkedBranchOf(ctx, old)
	if err != nil {
		return err
	}
	if linked {
		risk, err := b.branchRisk(ctx, lb)
		if err != nil {
			return err
		}
		if risk.Blocked != "" {
			return invalid("the worktree of %s cannot be removed (%s)", old.Name, risk.Blocked)
		}
		if len(risk.Dirty) > 0 && !discard {
			return invalid("the worktree of %s has uncommitted changes: confirm to discard them", old.Name)
		}
	}
	// Row first: a failed transaction leaves the worktree intact.
	if err := b.deps.Tasks.DetachBranch(old.ID, replacement, b.deps.Now().UnixMilli()); err != nil {
		return err
	}
	if old.Name != "" {
		b.snapshotChangeRequests(ctx, old.TaskID, []model.AdeTaskBranch{old})
		b.snapshotReviewComments(ctx, []model.AdeTaskBranch{old})
	}
	if linked {
		if err := b.removeWorktree(ctx, lb); err != nil {
			slog.Warn("ade: remove detached worktree", "scope", "ade", "branch", old.ID, "path", lb.path, "err", err)
		}
	}
	return nil
}

// TaskBranchRisk reports what removing the row's branch would discard.
func (b *TaskBoard) TaskBranchRisk(ctx context.Context, branchID string) (adewire.BranchRisk, error) {
	sb, err := b.deps.Tasks.GetBranch(branchID)
	if err != nil {
		return adewire.BranchRisk{}, err
	}
	tc, err := b.loadTaskCtx(sb.TaskID)
	if err != nil {
		return adewire.BranchRisk{}, err
	}
	row, ok := tc.branch(branchID)
	if !ok {
		return adewire.BranchRisk{}, repos.ErrBranchMissing
	}
	lb, linked, err := b.linkedBranchOf(ctx, row)
	if err != nil {
		return adewire.BranchRisk{}, err
	}
	if !linked {
		return adewire.BranchRisk{BranchID: branchID, Dirty: []adewire.DirtyEntry{}}, nil
	}
	return b.branchRisk(ctx, lb)
}

// checkFreshName refuses a new branch name git, the repo or the task list would reject, so a refusal
// changes nothing.
func (b *TaskBoard) checkFreshName(ctx context.Context, repoID, name string) error {
	rec, err := b.deps.CodeRepos.Get(repoID)
	if err != nil {
		return err
	}
	if rec == nil {
		return invalid("code repo %s not found", repoID)
	}
	msg, err := b.checkNewBranchName(ctx, *rec, name)
	if err != nil {
		return err
	}
	if msg != "" {
		return invalid("%s", msg)
	}
	mu := b.repoMutex(repoID)
	mu.Lock()
	defer mu.Unlock()
	entry, err := b.openRepo(ctx, repoID)
	if err != nil {
		return err
	}
	inv, err := entry.BranchInventory(ctx)
	if err != nil {
		return err
	}
	if branchExists(inv, name) {
		return invalid("branch %s already exists", name)
	}
	live, err := b.deps.Tasks.BranchesLive()
	if err != nil {
		return err
	}
	if slices.ContainsFunc(live, func(br model.AdeTaskBranch) bool { return br.CodeRepoID == repoID && br.Name == name }) {
		return invalid("branch %s is already on a task", name)
	}
	return nil
}

// SetTaskBranch gives a row its branch: it fills a repo that has none or replaces the branch of a
// created row, with an existing git branch or a new one.
func (b *TaskBoard) SetTaskBranch(ctx context.Context, args adewire.SetTaskBranchArgs) (adewire.Branch, error) {
	cur, err := b.deps.Tasks.GetBranch(args.BranchID)
	if err != nil {
		return adewire.Branch{}, err
	}
	mu := b.taskMu(cur.TaskID)
	mu.Lock()
	defer mu.Unlock()
	tc, old, err := b.editableRow(ctx, cur.TaskID, args.BranchID)
	if err != nil {
		return adewire.Branch{}, err
	}
	rec, err := b.deps.CodeRepos.Get(old.CodeRepoID)
	if err != nil {
		return adewire.Branch{}, err
	}
	if rec == nil {
		return adewire.Branch{}, invalid("code repo %s not found", old.CodeRepoID)
	}
	next := model.AdeTaskBranch{ID: ids.NewID(), TaskID: old.TaskID, CodeRepoID: old.CodeRepoID, Kind: ownKind(old.Kind), AddedAt: b.deps.Now().UnixMilli()}
	if args.Existing {
		rmu := b.repoMutex(old.CodeRepoID)
		rmu.Lock()
		target, err := b.resolveExisting(ctx, old.CodeRepoID, args.Name, tc.task.Kind, false)
		rmu.Unlock()
		if err != nil {
			return adewire.Branch{}, err
		}
		next.Name, next.Kind = args.Name, target.kind
	} else {
		if err := b.checkFreshName(ctx, old.CodeRepoID, args.Name); err != nil {
			return adewire.Branch{}, err
		}
		next.Base, next.BaseBranchID, next.QueuedAfter = old.Base, old.BaseBranchID, old.QueuedAfter
	}
	if err := b.detachRow(ctx, old, &next, args.DiscardChanges); err != nil {
		return adewire.Branch{}, err
	}
	b.notifyBoard()
	if args.Existing {
		if next.Kind != model.AdeBranchKindReview {
			b.prepareAttached(ctx, next)
		}
	} else if err := b.createNamed(ctx, tc, *rec, &next, args.Name); err != nil {
		return adewire.Branch{}, err
	}
	return b.wireRow(ctx, next.ID)
}

// createNamed creates the row's new branch and worktree. A git failure leaves the row not created.
func (b *TaskBoard) createNamed(ctx context.Context, tc *taskCtx, rec model.CodeRepo, row *model.AdeTaskBranch, name string) error {
	res, err := b.ensureWorktree(ctx, rec, *row, newBranchName{title: taskTitle(tc.task, tc.branches), wanted: name})
	if err != nil {
		b.notifyBoard()
		return err
	}
	row.Name = res.Name
	if res.Fresh {
		if err := b.startSetup(rec, *row, res.Name, res.Path, b.onSetupReady); err != nil {
			return err
		}
	}
	b.notifyBoard()
	return nil
}

// wireRow is the board facts of one stored row.
func (b *TaskBoard) wireRow(ctx context.Context, branchID string) (adewire.Branch, error) {
	sb, err := b.deps.Tasks.GetBranch(branchID)
	if err != nil {
		return adewire.Branch{}, err
	}
	facts := b.repoFacts(ctx, sb.CodeRepoID, []model.AdeTaskBranch{sb}, nil)
	b.applyChecks(sb.CodeRepoID, facts)
	return facts.branches[sb.ID], nil
}

// RemoveTaskBranch detaches a created row's branch. The repo stays as a row without a branch, unless
// the task has another live row in that repo: then the row goes away whole.
func (b *TaskBoard) RemoveTaskBranch(ctx context.Context, args adewire.RemoveTaskRowArgs) error {
	branchID := args.BranchID
	cur, err := b.deps.Tasks.GetBranch(branchID)
	if err != nil {
		return err
	}
	mu := b.taskMu(cur.TaskID)
	mu.Lock()
	defer mu.Unlock()
	tc, old, err := b.editableRow(ctx, cur.TaskID, branchID)
	if err != nil {
		return err
	}
	if old.Name == "" {
		return invalid("the repo has no branch to remove")
	}
	var draft *model.AdeTaskBranch
	if !slices.ContainsFunc(tc.branches, func(br model.AdeTaskBranch) bool { return br.CodeRepoID == old.CodeRepoID && br.ID != old.ID }) {
		draft = &model.AdeTaskBranch{
			ID: ids.NewID(), TaskID: old.TaskID, CodeRepoID: old.CodeRepoID, Kind: ownKind(old.Kind), AddedAt: b.deps.Now().UnixMilli(),
			Base: old.Base, BaseBranchID: old.BaseBranchID, QueuedAfter: old.QueuedAfter,
		}
	}
	if err := b.detachRow(ctx, old, draft, args.DiscardChanges); err != nil {
		return err
	}
	b.notifyBoard()
	return nil
}

// RemoveTaskRepo detaches the row, its branch and its worktree included.
func (b *TaskBoard) RemoveTaskRepo(ctx context.Context, args adewire.RemoveTaskRowArgs) error {
	branchID := args.BranchID
	cur, err := b.deps.Tasks.GetBranch(branchID)
	if err != nil {
		return err
	}
	mu := b.taskMu(cur.TaskID)
	mu.Lock()
	defer mu.Unlock()
	_, old, err := b.editableRow(ctx, cur.TaskID, branchID)
	if err != nil {
		return err
	}
	if err := b.detachRow(ctx, old, nil, args.DiscardChanges); err != nil {
		return err
	}
	b.notifyBoard()
	return nil
}

// UpdateTaskRepo swaps the repo of a row whose branch is not created. Its base resets to the new
// repo's main.
func (b *TaskBoard) UpdateTaskRepo(ctx context.Context, args adewire.UpdateTaskRepoArgs) (adewire.Branch, error) {
	cur, err := b.deps.Tasks.GetBranch(args.BranchID)
	if err != nil {
		return adewire.Branch{}, err
	}
	mu := b.taskMu(cur.TaskID)
	mu.Lock()
	defer mu.Unlock()
	tc, old, err := b.editableRow(ctx, cur.TaskID, args.BranchID)
	if err != nil {
		return adewire.Branch{}, err
	}
	if old.Name != "" {
		return adewire.Branch{}, invalid("the branch exists: remove it before changing the repo")
	}
	if err := b.requireRepos([]string{args.CodeRepoID}); err != nil {
		return adewire.Branch{}, err
	}
	if slices.ContainsFunc(tc.branches, func(br model.AdeTaskBranch) bool {
		return br.CodeRepoID == args.CodeRepoID && br.Name == "" && br.ID != old.ID
	}) {
		return adewire.Branch{}, repos.ErrDraftInRepo
	}
	next := model.AdeTaskBranch{ID: ids.NewID(), TaskID: old.TaskID, CodeRepoID: args.CodeRepoID, Kind: old.Kind, AddedAt: b.deps.Now().UnixMilli()}
	if err := b.detachRow(ctx, old, &next, false); err != nil {
		return adewire.Branch{}, err
	}
	b.notifyBoard()
	return zeroBranch(next, b.mainShortName(ctx, next.CodeRepoID), nil), nil
}
