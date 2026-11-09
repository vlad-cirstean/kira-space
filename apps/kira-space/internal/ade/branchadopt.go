package ade

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitsession"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/repos"
	"github.com/kirathecat/kira-studio/internal/agentmcp"
	"github.com/kirathecat/kira-studio/internal/ids"
)

// branchadopt.go: an agent works on a branch that already exists (P583). A branch checked out nowhere is
// checked out in the task's own worktree ('checkout' item); one held by another worktree is detached there
// first, with the user's approval ('adopt' item).

const (
	branchKindCheckout = "checkout"
	branchKindAdopt    = "adopt"
)

// Holder classes: where git has a branch checked out, seen from the asking row.
const (
	holderNone    = "none"
	holderOwn     = "own"     // the asking row's own worktree
	holderTask    = "task"    // another live row's own worktree
	holderMain    = "main"    // the repo's main checkout
	holderOther   = "other"   // any other registered worktree
	holderMissing = "missing" // registered, but the folder is gone
)

type holderInfo struct {
	class string
	path  string
	row   *model.AdeTaskBranch // the live row behind holderTask
}

// holderWire is the wire form of a holder; nil for none and own.
func holderWire(h holderInfo) *adewire.BranchHolder {
	switch h.class {
	case holderTask:
		return &adewire.BranchHolder{Kind: holderTask, TaskID: h.row.TaskID, BranchID: h.row.ID, Worktree: h.path}
	case holderMain, holderOther:
		return &adewire.BranchHolder{Kind: h.class, Worktree: h.path}
	}
	return nil
}

// classifyHolder names who holds path. self is the asking row, nil while it has none.
func (b *TaskBoard) classifyHolder(rec model.CodeRepo, root string, live []model.AdeTaskBranch, self *model.AdeTaskBranch, path string) holderInfo {
	if path == "" {
		return holderInfo{class: holderNone}
	}
	clean := filepath.Clean(path)
	if self != nil {
		if own := b.ownWorktreePath(rec.Name, *self); own != "" && clean == filepath.Clean(own) {
			return holderInfo{class: holderOwn, path: path}
		}
	}
	if row, ok := b.worktreeOwners(rec.Name, live, rec.ID)[clean]; ok && (self == nil || row.ID != self.ID) {
		return holderInfo{class: holderTask, path: path, row: &row}
	}
	if clean == filepath.Clean(root) {
		return holderInfo{class: holderMain, path: path}
	}
	if _, err := os.Lstat(path); err != nil {
		return holderInfo{class: holderMissing, path: path}
	}
	return holderInfo{class: holderOther, path: path}
}

// taskTitleOf is the display title of a task, "" when it no longer exists.
func (b *TaskBoard) taskTitleOf(taskID string) string {
	t, err := b.deps.Tasks.GetTask(taskID)
	if err != nil {
		return ""
	}
	return taskTitle(t, nil)
}

// donorBusy says why Kira Space cannot detach the branch in the worktree of row now, "" when it can. It
// reads without locks: the donor task's mutex is never taken while the requester's is held.
func (b *TaskBoard) donorBusy(ctx context.Context, row model.AdeTaskBranch) (string, error) {
	why := ""
	if busy, err := b.deps.Tasks.HasActiveStepRunOn(row.ID); err != nil {
		return "", err
	} else if busy {
		why = "a run is working on it"
	}
	if why == "" {
		if r, err := b.deps.Tasks.RunningRebaseOn(row.ID); err != nil {
			return "", err
		} else if r != nil {
			why = "a rebase is running on it"
		}
	}
	if why == "" {
		if name := b.claimOn(row.ID); name != "" {
			why = fmt.Sprintf("script run %s is running in it", name)
		}
	}
	if why == "" {
		b.runMu.Lock()
		_, setting := b.setupLive[row.ID]
		b.runMu.Unlock()
		if setting {
			why = "its prepare script is still running"
		}
	}
	if why == "" && b.branchSandboxed(ctx, row) {
		why = "a sandbox runs on it"
	}
	if why == "" {
		open, err := b.runningTUI(func(s model.AdeSession) bool { return s.BranchID == row.ID })
		if err != nil {
			return "", err
		}
		if open != nil {
			why = "an agent session is open on it"
		}
	}
	if why == "" {
		return "", nil
	}
	return fmt.Sprintf("Branch %s is checked out in task %q, where %s. Try again when that ends.", row.Name, b.taskTitleOf(row.TaskID), why), nil
}

// holderBlock says why the holder's worktree cannot be detached now ("" when it can) and how many
// uncommitted changes would stay there.
func (b *TaskBoard) holderBlock(ctx context.Context, entry *gitsession.RepoEntry, branch string, h holderInfo) (text string, dirty int, err error) {
	if h.class == holderTask {
		if text, err = b.donorBusy(ctx, *h.row); text != "" || err != nil {
			return text, 0, err
		}
	}
	status, err := entry.WorktreeStatus(ctx, h.path)
	if err != nil {
		return "", 0, err
	}
	if op := entry.WorktreeInProgress(h.path, status); op != nil {
		return fmt.Sprintf("A %s is in progress in %s, where %s is checked out. The user must finish or abort it first.", opName(op), h.path, branch), 0, nil
	}
	for _, e := range status {
		if e.Kind != "ignored" {
			dirty++
		}
	}
	return "", dirty, nil
}

const reservedExistingText = "Branch %s belongs to the repo itself (its main or an integration branch). A task cannot check it out."

// planTake decides a branch an agent wants to work on that already exists: self is the task's row for it,
// nil while it has none. Refusals come before any prompt.
func (b *TaskBoard) planTake(ctx context.Context, slot requestSlot, rec model.CodeRepo, self *model.AdeTaskBranch) (requestSlot, error) {
	nick, branch := slot.nick, slot.branch
	item := adewire.BranchRequestItem{ID: slot.id, CodeRepoID: slot.codeRepoID, Repo: nick, Kind: branchKindCheckout, Branch: branch}
	if text, err := b.reservedBranchText(ctx, slot.codeRepoID, branch); err != nil {
		return slot, err
	} else if text != "" {
		slot.result = failedBranch(nick, branch, "", fmt.Sprintf(reservedExistingText, branch))
		return slot, nil
	}
	entry, err := b.openRepo(ctx, slot.codeRepoID)
	if err != nil {
		return slot, err
	}
	inv, err := entry.BranchInventory(ctx)
	if err != nil {
		return slot, err
	}
	if !branchExists(inv, branch) {
		slot.result = failedBranch(nick, branch, "", fmt.Sprintf("Branch %s not found. Omit existing to create it.", branch))
		return slot, nil
	}
	path := ""
	if row, ok := localBranch(inv, branch); ok {
		path = row.WorktreePath
	}
	live, err := b.deps.Tasks.BranchesLive()
	if err != nil {
		return slot, err
	}
	h := b.classifyHolder(rec, entry.Summary.Root, live, self, path)
	switch h.class {
	case holderMissing:
		slot.result = failedBranch(nick, branch, "", fmt.Sprintf("Branch %s is registered to a missing worktree at %s. Ask the user to run git worktree prune.", branch, h.path))
		return slot, nil
	case holderTask, holderMain, holderOther:
		text, dirty, err := b.holderBlock(ctx, entry, branch, h)
		if err != nil {
			return slot, err
		}
		if text != "" {
			slot.result = failedBranch(nick, branch, "", text)
			return slot, nil
		}
		item.Kind, item.Holder, item.Dirty = branchKindAdopt, holderWire(h), dirty
	}
	slot.item = &item
	return slot, nil
}

// planExisting decides existing: true for a branch the task has no row for.
func (b *TaskBoard) planExisting(ctx context.Context, slot requestSlot, rec model.CodeRepo) (requestSlot, error) {
	return b.planTake(ctx, slot, rec, nil)
}

// takeState is what applyExisting changed, for undo.
type takeState struct {
	sb       model.AdeTaskBranch
	added    bool   // the row was inserted for this call
	named    bool   // a draft row got its name for this call
	detached string // worktree path where the branch was detached
}

// applyExisting checks the approved branch out: it re-reads the holder, makes the row, detaches the
// holder when there is one and creates the task's worktree. The task lock is held.
func (b *TaskBoard) applyExisting(ctx context.Context, tc *taskCtx, s *requestSlot) *agentmcp.RequestedBranch {
	fail := func(text string) *agentmcp.RequestedBranch { return failedBranch(s.nick, s.branch, "", text) }
	rec, err := b.deps.CodeRepos.Get(s.codeRepoID)
	if err != nil || rec == nil {
		return fail(fmt.Sprintf("Repo %s is no longer registered.", s.nick))
	}
	named, draft := taskRepoRows(tc, s.codeRepoID, s.branch)
	st, entry, text, err := b.takeLocked(ctx, tc, s, *rec, named, draft)
	if err != nil {
		slog.Warn("ade: check out existing branch", "scope", "ade", "repo", s.codeRepoID, "err", err)
		return fail(cmpNonEmpty(text, "Kira Space could not check out this branch."))
	}
	if text != "" {
		return fail(text)
	}
	res, err := b.ensureWorktree(ctx, *rec, st.sb, newBranchName{})
	if err != nil {
		b.undoTake(ctx, entry, st, s.branch)
		return fail(cmpNonEmpty(toolMessageOf(err), "Kira Space could not create the worktree."))
	}
	if res.Fresh {
		if err := b.startSetup(*rec, st.sb, res.Name, res.Path, b.onSetupReady); err != nil {
			slog.Warn("ade: start setup of checked out branch", "scope", "ade", "branch", st.sb.ID, "err", err)
			return fail("Kira Space could not prepare the worktree of this branch.")
		}
	}
	rb := agentmcp.RequestedBranch{BranchInfo: b.branchInfo(st.sb, s.nick, res.Path), Outcome: agentmcp.BranchCheckedOut}
	if st.detached != "" {
		rb.Next = fmt.Sprintf("Detached it in %s first. ", st.detached) + rb.Next
	}
	return &rb
}

// toolMessageOf is the agent-facing text of err: a caller mistake or git's own message.
func toolMessageOf(err error) string {
	var te agentmcp.ToolError
	if errors.As(toolErr(err), &te) {
		return te.Error()
	}
	return err.Error()
}

// takeLocked runs the re-check, row and detach steps of applyExisting under the repo mutex. text is a
// refusal for the agent; err is a fault.
func (b *TaskBoard) takeLocked(ctx context.Context, tc *taskCtx, s *requestSlot, rec model.CodeRepo, named, draft *model.AdeTaskBranch) (st takeState, entry *gitsession.RepoEntry, text string, err error) {
	mu := b.repoMutex(s.codeRepoID)
	mu.Lock()
	defer mu.Unlock()
	if t, err := b.reservedBranchText(ctx, s.codeRepoID, s.branch); err != nil {
		return st, nil, "", err
	} else if t != "" {
		return st, nil, fmt.Sprintf(reservedExistingText, s.branch), nil
	}
	if entry, err = b.openRepo(ctx, s.codeRepoID); err != nil {
		return st, nil, "", err
	}
	inv, err := entry.BranchInventory(ctx)
	if err != nil {
		return st, nil, "", err
	}
	if !branchExists(inv, s.branch) {
		return st, nil, fmt.Sprintf("Branch %s not found. Omit existing to create it.", s.branch), nil
	}
	path := ""
	if row, ok := localBranch(inv, s.branch); ok {
		path = row.WorktreePath
	}
	live, err := b.deps.Tasks.BranchesLive()
	if err != nil {
		return st, nil, "", err
	}
	h := b.classifyHolder(rec, entry.Summary.Root, live, named, path)
	switch h.class {
	case holderMissing:
		return st, nil, fmt.Sprintf("Branch %s is registered to a missing worktree at %s. Ask the user to run git worktree prune.", s.branch, h.path), nil
	case holderTask, holderMain, holderOther:
		asked := ""
		if s.item.Holder != nil {
			asked = s.item.Holder.Worktree
		}
		if asked == "" || filepath.Clean(asked) != filepath.Clean(h.path) {
			return st, nil, fmt.Sprintf("Branch %s moved to %s while waiting. Call request_branches again.", s.branch, h.path), nil
		}
		if t, _, err := b.holderBlock(ctx, entry, s.branch, h); t != "" || err != nil {
			return st, nil, t, err
		}
	}
	if st.sb, st.added, st.named, err = b.takeRow(tc, s, named, draft, live); err != nil {
		b.undoTake(ctx, entry, st, s.branch)
		if errors.Is(err, repos.ErrBranchOnTask) || errors.Is(err, repos.ErrBranchNameTaken) {
			return takeState{}, nil, fmt.Sprintf("Branch %s is already on a task. Call request_branches again.", s.branch), nil
		}
		return takeState{}, nil, "", err
	}
	if h.class == holderTask || h.class == holderMain || h.class == holderOther {
		if err := entry.DetachWorktree(ctx, h.path); err != nil {
			b.undoTake(ctx, entry, st, s.branch)
			return takeState{}, nil, toolMessageOf(err), nil
		}
		st.detached = h.path
	}
	return st, entry, "", nil
}

// takeRow returns the task's row for the branch: the named row, the repo's waiting draft given the name,
// or a new row. A row whose name another live row has already is marked adopted.
func (b *TaskBoard) takeRow(tc *taskCtx, s *requestSlot, named, draft *model.AdeTaskBranch, live []model.AdeTaskBranch) (sb model.AdeTaskBranch, added, nameSet bool, err error) {
	if named != nil {
		return *named, false, false, nil
	}
	shared := false
	for _, r := range live {
		if r.CodeRepoID == s.codeRepoID && r.Name == s.branch {
			shared = true
		}
	}
	kind := model.AdeBranchKindMine
	if tc.task.Kind == model.AdeTaskKindParked {
		kind = model.AdeBranchKindParked
	}
	if draft != nil {
		if err := b.deps.Tasks.SetBranchNameShared(draft.ID, s.branch, shared); err != nil {
			return sb, false, false, err
		}
		if err := b.deps.Tasks.SetBranchOrigin(draft.ID, model.AdeBranchOriginAgent); err != nil {
			return sb, false, true, err
		}
		sb = *draft
		sb.Name, sb.Adopted, sb.Origin = s.branch, shared, model.AdeBranchOriginAgent
		return sb, false, true, nil
	}
	sb, err = b.deps.Tasks.AddBranch(model.AdeTaskBranch{
		ID: ids.NewID(), TaskID: tc.task.ID, CodeRepoID: s.codeRepoID, Kind: kind, Name: s.branch,
		AddedAt: b.deps.Now().UnixMilli(), Origin: model.AdeBranchOriginAgent, Adopted: shared,
	})
	return sb, err == nil, false, err
}

// undoTake reverts takeLocked's changes after a later step failed: it gives the donor its branch back and
// removes the row it made.
func (b *TaskBoard) undoTake(ctx context.Context, entry *gitsession.RepoEntry, st takeState, branch string) {
	if st.detached != "" {
		if err := entry.SwitchWorktree(ctx, st.detached, branch); err != nil {
			slog.Warn("ade: restore detached worktree", "scope", "ade", "path", st.detached, "err", err)
		}
	}
	var err error
	switch {
	case st.added:
		err = b.deps.Tasks.DeleteFreshBranch(st.sb.ID)
	case st.named:
		err = b.deps.Tasks.SetBranchNameShared(st.sb.ID, "", false)
	}
	if err != nil {
		slog.Warn("ade: undo checked out branch row", "scope", "ade", "branch", st.sb.ID, "err", err)
	}
}
