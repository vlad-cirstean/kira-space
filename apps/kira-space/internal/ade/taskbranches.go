package ade

import (
	"cmp"
	"context"
	"log/slog"
	"slices"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitsession"
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
	// A step session carries no branch id; its cwd is the worktree, whichever task it belongs to.
	wt := b.worktreePath(ctx, row)
	open, err := b.runningTUI(func(s model.AdeSession) bool {
		return s.BranchID == row.ID || (wt != "" && s.Cwd == wt)
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
// the row's linked worktree. A worktree with uncommitted changes needs discardToken, the BranchRisk
// token of the set the user saw; it is checked again when the worktree is removed. The caller holds the
// task mutex and has run editableRow. err is a refusal or fault before the row changed; worktreeErr is a
// failed removal after it: the row change stands and the folder stays.
func (b *TaskBoard) detachRow(ctx context.Context, old model.AdeTaskBranch, replacement *model.AdeTaskBranch, discardToken string) (worktreeErr, err error) {
	lb, linked, err := b.linkedBranchOf(ctx, old)
	if err != nil {
		return nil, err
	}
	if linked {
		worktreeErr, err = b.detachLinked(ctx, lb, old, replacement, discardToken)
	} else {
		err = b.deps.Tasks.DetachBranch(old.ID, replacement, b.deps.Now().UnixMilli())
	}
	if err != nil {
		return nil, err
	}
	if old.Name != "" {
		b.snapshotChangeRequests(ctx, old.TaskID, []model.AdeTaskBranch{old})
		b.snapshotReviewComments(ctx, []model.AdeTaskBranch{old})
	}
	return worktreeErr, nil
}

// detachLinked checks, detaches the row and removes its worktree under the repo lock, so the check
// that clears the removal sits right before the row change: a tree that turned dirty is refused while
// the row is still on the task. Row first: a failed transaction leaves the worktree intact.
func (b *TaskBoard) detachLinked(ctx context.Context, lb linkedBranch, old model.AdeTaskBranch, replacement *model.AdeTaskBranch, discardToken string) (worktreeErr, err error) {
	mu := b.repoMutex(lb.sb.CodeRepoID)
	mu.Lock()
	defer mu.Unlock()
	risk, err := b.branchRisk(ctx, lb)
	if err != nil {
		return nil, err
	}
	if risk.Blocked != "" {
		text, err := b.blockedText(ctx, lb)
		if err != nil {
			return nil, err
		}
		return nil, invalid("the worktree of %s cannot be removed: %s", old.Name, text)
	}
	if len(risk.Dirty) > 0 {
		if err := b.refuseDiscard(old.Name, len(risk.Dirty), risk.DirtyToken, discardToken); err != nil {
			return nil, err
		}
	}
	b.taskFiles.Close(lb.sb.ID)
	if err := b.deps.Tasks.DetachBranch(old.ID, replacement, b.deps.Now().UnixMilli()); err != nil {
		return nil, err
	}
	if err := b.removeWorktreeLocked(ctx, lb, discardToken); err != nil {
		slog.Warn("ade: remove detached worktree", "scope", "ade", "branch", old.ID, "path", lb.path, "err", err)
		return err, nil
	}
	return nil, nil
}

// discardRefusal is nil when token confirms the uncommitted changes now in the worktree (current is
// their fingerprint). A missing token means no discard was asked; a different one means the set changed
// after the user saw it. Either way nothing is deleted.
func discardRefusal(name string, count int, current, token string) error {
	switch {
	case token == "" && count == 0:
		return invalid("the worktree of %s has uncommitted changes: commit them, or use Discard changes and remove", name)
	case token == "":
		return invalid("the worktree of %s has %d uncommitted changes: commit them, or use Discard changes and remove", name, count)
	case token != current:
		return invalid("the uncommitted changes of %s changed since you reviewed them: review them again before discarding", name)
	}
	return nil
}

// refuseDiscard is discardRefusal that also pushes the board when a token was refused as stale, so the
// client reloads the current list and token instead of resending the old one.
func (b *TaskBoard) refuseDiscard(name string, count int, current, token string) error {
	err := discardRefusal(name, count, current, token)
	if err != nil && token != "" {
		b.notifyBoard()
	}
	return err
}

// blockedText is the human reason the preflight blocks removing lb's worktree.
func (b *TaskBoard) blockedText(ctx context.Context, lb linkedBranch) (string, error) {
	pf, err := lb.entry.WorktreeRemovePreflight(ctx, lb.path)
	if err != nil {
		return "", err
	}
	return gitsession.WorktreeRemoveBlockedMessage(pf), nil
}

// removeRowBranch takes a created row's branch off the task and removes its worktree, the path the
// Remove branch menu and the agent's worktree.remove share. The repo keeps a row without a branch, unless
// the task has another live row in that repo: then the row goes away whole. The git branch stays. The
// caller holds the task mutex and has run editableRow for old.
func (b *TaskBoard) removeRowBranch(ctx context.Context, tc *taskCtx, old model.AdeTaskBranch, discardToken string) (worktreeErr, err error) {
	var draft *model.AdeTaskBranch
	if !slices.ContainsFunc(tc.branches, func(br model.AdeTaskBranch) bool { return br.CodeRepoID == old.CodeRepoID && br.ID != old.ID }) {
		draft = &model.AdeTaskBranch{
			ID: ids.NewID(), TaskID: old.TaskID, CodeRepoID: old.CodeRepoID, Kind: ownKind(old.Kind), AddedAt: b.deps.Now().UnixMilli(),
			Base: old.Base, BaseBranchID: old.BaseBranchID, QueuedAfter: old.QueuedAfter,
		}
	}
	return b.detachRow(ctx, old, draft, discardToken)
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
	return joinOp(ctx, b, opKey("setBranch", args), func(ctx context.Context) (adewire.Branch, error) { return b.setTaskBranch(ctx, args) })
}

func (b *TaskBoard) setTaskBranch(ctx context.Context, args adewire.SetTaskBranchArgs) (adewire.Branch, error) {
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
	if _, err := b.detachRow(ctx, old, &next, args.DiscardToken); err != nil {
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

// RemoveTaskBranch detaches a created row's branch through removeRowBranch.
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
	if _, err := b.removeRowBranch(ctx, tc, old, args.DiscardToken); err != nil {
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
	if _, err := b.detachRow(ctx, old, nil, args.DiscardToken); err != nil {
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
	if _, err := b.detachRow(ctx, old, &next, ""); err != nil {
		return adewire.Branch{}, err
	}
	b.notifyBoard()
	return zeroBranch(next, b.mainShortName(ctx, next.CodeRepoID), nil), nil
}
