package ade

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/oplog"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/agentmcp"
)

// worktreeremove.go: an agent removes one of its task's worktrees (P702). It runs the UI's removeRowBranch
// path without discard: a worktree with uncommitted work is refused, and only the user can discard it.

// RemoveWorktree implements agentmcp.SpaceTools. Every refusal is a failed result with text for the
// agent; the git branch always stays.
func (b *TaskBoard) RemoveWorktree(ctx context.Context, call agentmcp.SpaceCall, args agentmcp.RemoveWorktreeArgs) (agentmcp.RemoveWorktreeResult, error) {
	mu := b.taskMu(call.TaskID)
	mu.Lock()
	defer mu.Unlock()
	tc, err := b.spaceCtx(call.TaskID)
	if err != nil {
		return agentmcp.RemoveWorktreeResult{}, err
	}
	if err := b.spaceWritable(tc); err != nil {
		return agentmcp.RemoveWorktreeResult{}, err
	}
	_, cfgs, err := b.registeredCandidates()
	if err != nil {
		return agentmcp.RemoveWorktreeResult{}, err
	}
	id, err := resolveRepoRef(args.Repo, b.taskCandidates(tc, cfgs), "task")
	if err != nil {
		return agentmcp.RemoveWorktreeResult{}, err
	}
	nick := tc.nick[id]
	failed := func(text string) (agentmcp.RemoveWorktreeResult, error) {
		return agentmcp.RemoveWorktreeResult{Repo: nick, Branch: args.Branch, Outcome: agentmcp.BranchFailed, Next: text}, nil
	}
	named, _ := taskRepoRows(tc, id, args.Branch)
	if named == nil {
		return agentmcp.RemoveWorktreeResult{}, agentmcp.ToolError(fmt.Sprintf("Branch %q is not on this task in that repo.", args.Branch))
	}
	if named.Kind != model.AdeBranchKindMine {
		return failed(fmt.Sprintf("The branch %s of %s on this task is not yours to remove.", args.Branch, nick))
	}
	lb, linked, err := b.linkedBranchOf(ctx, *named)
	if err != nil {
		return agentmcp.RemoveWorktreeResult{}, err
	}
	if !linked {
		return failed(fmt.Sprintf("Branch %s has no Kira worktree (it is checked out in the repo checkout or nowhere); nothing to remove.", args.Branch))
	}
	if inside(b.callerCwd(call), lb.path) {
		return failed("This is your working directory; you cannot remove it.")
	}
	if text := b.rowBusyRefusal(ctx, *named, "removing it waits"); text != "" {
		return failed(text)
	}
	if _, _, err := b.editableRow(ctx, call.TaskID, named.ID); err != nil {
		if errors.Is(err, ErrInvalidInput) {
			return failed(trimInvalid(err))
		}
		return agentmcp.RemoveWorktreeResult{}, err
	}
	risk, err := b.branchRisk(ctx, lb)
	if err != nil {
		return agentmcp.RemoveWorktreeResult{}, err
	}
	if risk.Blocked != "" {
		text, err := b.blockedText(ctx, lb)
		if err != nil {
			return agentmcp.RemoveWorktreeResult{}, err
		}
		return failed(text)
	}
	if n := len(risk.Dirty); n > 0 {
		return failed(fmt.Sprintf("Worktree %s has %d uncommitted changes (%s). Commit them first, or ask the user; only the user can discard them.", args.Branch, n, dirtyPaths(risk.Dirty)))
	}
	op := b.startAgentOp(ctx, opAgentWorktreeRemove, call.TaskID, nick+"/"+args.Branch)
	worktreeErr, err := b.removeRowBranch(ctx, tc, *named, "")
	b.notifyBoard()
	switch {
	case err != nil:
		op.Finish(oplog.StatusError, toolMessageOf(err))
		if errors.Is(err, ErrInvalidInput) {
			return failed(trimInvalid(err))
		}
		return agentmcp.RemoveWorktreeResult{}, err
	case worktreeErr != nil:
		op.Finish(oplog.StatusError, toolMessageOf(worktreeErr))
		return failed(fmt.Sprintf("Branch %s left the task, but git could not remove %s: %s", args.Branch, lb.path, toolMessageOf(worktreeErr)))
	}
	op.Finish(oplog.StatusOK, "")
	return agentmcp.RemoveWorktreeResult{
		Repo: nick, Branch: args.Branch, Outcome: agentmcp.BranchRemoved, Worktree: lb.path,
		Next: fmt.Sprintf("Removed worktree %s. Branch %s stays in git; check it out again with branch.request existing: true.", lb.path, args.Branch),
	}, nil
}

// callerCwd is the working directory of the session or run behind call, "" when unknown.
func (b *TaskBoard) callerCwd(call agentmcp.SpaceCall) string {
	if call.SessionID != "" {
		if s, err := b.deps.Sessions.Get(call.SessionID); err == nil && s != nil {
			return s.Cwd
		}
		return ""
	}
	if call.RunID == "" {
		return ""
	}
	rows, err := b.deps.Sessions.ListRunning(call.TaskID)
	if err != nil {
		return ""
	}
	for _, s := range rows {
		if s.RunID == call.RunID {
			return s.Cwd
		}
	}
	return ""
}

// inside reports whether dir is path or lies under it.
func inside(dir, path string) bool {
	if dir == "" || path == "" {
		return false
	}
	rel, err := filepath.Rel(filepath.Clean(path), filepath.Clean(dir))
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// dirtyPaths names the first three changed paths and counts the rest.
func dirtyPaths(entries []adewire.DirtyEntry) string {
	const shown = 3
	var names []string
	for i, e := range entries {
		if i == shown {
			names = append(names, fmt.Sprintf("and %d more", len(entries)-shown))
			break
		}
		names = append(names, e.Path)
	}
	return strings.Join(names, ", ")
}
