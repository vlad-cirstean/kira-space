package ade

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/repos"
	"github.com/kirathecat/kira-studio/internal/agentmcp"
)

// branchchange.go: an agent asks the user to point one of its task's branch rows at another existing
// local branch. The row keeps its id and worktree folder (and so its sessions); the folder switches to
// the branch when it holds something else. Unlike the user's change-branch action it never removes the folder.

// changePlan is a validated change: the row, the draft waiting for the user and the folder facts.
type changePlan struct {
	sb      model.AdeTaskBranch
	nick    string
	own     string // the row's own folder
	head    string // local branch checked out in own now, "" = none or detached
	draft   *requestDraft
	refusal string
}

func changeResult(args agentmcp.ChangeBranchArgs, nick, outcome, next string) agentmcp.ChangeBranchResult {
	return agentmcp.ChangeBranchResult{Repo: nick, From: args.From, To: args.To, Outcome: outcome, Next: next}
}

// ChangeBranch implements agentmcp.SpaceTools. A refused change answers at once; otherwise the user
// decides in one prompt and the call waits, unlocked.
func (b *TaskBoard) ChangeBranch(ctx context.Context, call agentmcp.SpaceCall, args agentmcp.ChangeBranchArgs) (agentmcp.ChangeBranchResult, error) {
	plan, err := b.planChange(ctx, call, args)
	if err != nil {
		return agentmcp.ChangeBranchResult{}, err
	}
	if plan.refusal != "" {
		return changeResult(args, plan.nick, agentmcp.BranchFailed, plan.refusal), nil
	}
	dec, prefix := b.awaitBranchRequest(ctx, call, plan.draft)
	itemID := prefix + "i0"
	if err := ctx.Err(); err != nil {
		return agentmcp.ChangeBranchResult{}, err // the caller is gone: nothing changes for it
	}
	switch {
	case dec.kind == decisionAbandoned:
		return agentmcp.ChangeBranchResult{}, agentmcp.ToolError("The branch change request was cancelled.")
	case dec.kind == decisionTimeout:
		next := fmt.Sprintf("No answer from the user in %s; nothing changed. Call change_branch again later or ask the user.", waitText(b.branchRequestWait()))
		return changeResult(args, plan.nick, agentmcp.BranchTimeout, next), nil
	case dec.kind == decisionDenied || !dec.approved[itemID]:
		return changeResult(args, plan.nick, agentmcp.BranchDenied, "The user declined this change. Do not switch branches with git in the task folder; keep working on the task branch or ask the user."), nil
	}
	return b.applyChange(ctx, call, args)
}

// planChange validates the change under the task lock and builds the request.
func (b *TaskBoard) planChange(ctx context.Context, call agentmcp.SpaceCall, args agentmcp.ChangeBranchArgs) (changePlan, error) {
	mu := b.taskMu(call.TaskID)
	mu.Lock()
	defer mu.Unlock()
	plan, tc, err := b.changeTarget(ctx, call, args)
	if err != nil || plan.refusal != "" {
		return plan, err
	}
	item := adewire.BranchRequestItem{
		ID: "i0", CodeRepoID: plan.sb.CodeRepoID, Repo: plan.nick, Kind: branchKindSwitch,
		Branch: args.To, From: args.From, Stacked: []string{},
	}
	plan.draft = b.requestDraftOf(tc, call, []adewire.BranchRequestItem{item})
	return plan, nil
}

// changeTarget finds the task's row for args and runs every refusal; plan.refusal holds the agent-facing
// text of the first that applies. The caller holds the task lock.
func (b *TaskBoard) changeTarget(ctx context.Context, call agentmcp.SpaceCall, args agentmcp.ChangeBranchArgs) (changePlan, *taskCtx, error) {
	tc, err := b.spaceCtx(call.TaskID)
	if err != nil {
		return changePlan{}, nil, err
	}
	if err := b.spaceWritable(tc); err != nil {
		return changePlan{}, nil, err
	}
	_, cfgs, err := b.registeredCandidates()
	if err != nil {
		return changePlan{}, nil, err
	}
	id, err := resolveRepoRef(args.Repo, b.taskCandidates(tc, cfgs), "task")
	if err != nil {
		return changePlan{}, nil, err
	}
	plan := changePlan{nick: tc.nick[id]}
	named, _ := taskRepoRows(tc, id, args.From)
	if named == nil {
		return plan, nil, agentmcp.ToolError(fmt.Sprintf("Branch %q is not on this task in that repo.", args.From))
	}
	plan.sb = *named
	if named.Kind != model.AdeBranchKindMine {
		plan.refusal = fmt.Sprintf("The branch %s of %s on this task is not yours to change.", args.From, plan.nick)
		return plan, tc, nil
	}
	err = b.changeRefusal(ctx, call, &plan, args.To)
	return plan, tc, err
}

// changeRefusal sets plan.refusal to the first reason the row cannot take branch to, and fills the
// folder facts when it can.
func (b *TaskBoard) changeRefusal(ctx context.Context, call agentmcp.SpaceCall, plan *changePlan, to string) error {
	sb := plan.sb
	rec, err := b.deps.CodeRepos.Get(sb.CodeRepoID)
	if err != nil {
		return err
	}
	if rec == nil {
		plan.refusal = "Repo is no longer registered."
		return nil
	}
	if text, err := b.reservedBranchText(ctx, rec.ID, to); err != nil {
		return err
	} else if text != "" {
		plan.refusal = fmt.Sprintf(reservedExistingText, to)
		return nil
	}
	if text, err := b.reservedBranchText(ctx, rec.ID, sb.Name); err != nil {
		return err
	} else if text != "" {
		plan.refusal = fmt.Sprintf("%q belongs to the repo; it cannot be changed.", sb.Name)
		return nil
	}
	if text := b.rowBusyRefusal(ctx, sb, "its branch is fixed"); text != "" {
		plan.refusal = text
		return nil
	}
	if text, err := b.otherWorkOn(call, sb); err != nil {
		return err
	} else if text != "" {
		plan.refusal = text
		return nil
	}
	entry, err := b.openRepo(ctx, sb.CodeRepoID)
	if err != nil {
		return err
	}
	inv, err := entry.BranchInventory(ctx)
	if err != nil {
		return err
	}
	target, ok := localBranch(inv, to)
	if !ok {
		plan.refusal = fmt.Sprintf("Branch %s does not exist locally. Use request_branches to create or check out a branch.", to)
		return nil
	}
	live, err := b.deps.Tasks.BranchesLive()
	if err != nil {
		return err
	}
	for _, row := range live {
		if row.ID != sb.ID && row.CodeRepoID == sb.CodeRepoID && row.Name == to {
			plan.refusal = fmt.Sprintf("Branch %s is already on task %q.", to, b.taskTitleOf(row.TaskID))
			return nil
		}
	}
	own := b.ownWorktreePath(rec.Name, sb)
	if own == "" {
		plan.refusal = fmt.Sprintf("Branch %s has no worktree folder yet.", sb.Name)
		return nil
	}
	if _, err := os.Lstat(own); err != nil {
		plan.refusal = fmt.Sprintf("The worktree folder %s of %s is missing.", own, sb.Name)
		return nil
	}
	plan.own, plan.head = own, headIn(inv, own)
	h := b.classifyHolder(*rec, entry.Summary.Root, live, &sb, target.WorktreePath)
	if h.class != holderNone && h.class != holderOwn {
		plan.refusal = fmt.Sprintf("Branch %s is checked out in %s. Free it there first or pick another branch.", to, h.path)
		return nil
	}
	if plan.head == to {
		return nil
	}
	text, dirty, err := b.holderBlock(ctx, entry, to, holderInfo{class: holderOwn, path: own})
	switch {
	case err != nil:
		return err
	case text != "":
		plan.refusal = text
	case dirty > 0:
		plan.refusal = fmt.Sprintf("The worktree %s has %d uncommitted changes. Commit or stash them, then call change_branch again.", own, dirty)
	}
	return nil
}

// otherWorkOn says why the row's folder is in use by someone other than the caller, "" when it is
// not: a switch would swap files under that work. A headless caller is itself the row's step run.
func (b *TaskBoard) otherWorkOn(call agentmcp.SpaceCall, sb model.AdeTaskBranch) (string, error) {
	why := ""
	if name := b.claimOn(sb.ID); name != "" {
		why = fmt.Sprintf("script run %s is running in its worktree", name)
	}
	if why == "" && call.RunID == "" {
		if busy, err := b.deps.Tasks.HasActiveStepRunOn(sb.ID); err != nil {
			return "", err
		} else if busy {
			why = "a run is working on it"
		}
	}
	if why == "" {
		// A task-level session (step, review agent) has no BranchID and works in every own worktree.
		other, err := b.runningTUI(func(s model.AdeSession) bool {
			return s.ID != call.SessionID && (s.BranchID == sb.ID || (s.BranchID == "" && s.TaskID == sb.TaskID))
		})
		if err != nil {
			return "", err
		}
		if other != nil {
			why = "another agent session is open on it"
		}
	}
	if why == "" {
		return "", nil
	}
	return fmt.Sprintf("Branch %s cannot change now: %s. Try again when that ends.", sb.Name, why), nil
}

// applyChange switches the folder and re-points the row after approval, under the task lock, with every
// refusal checked again since the wait.
func (b *TaskBoard) applyChange(ctx context.Context, call agentmcp.SpaceCall, args agentmcp.ChangeBranchArgs) (agentmcp.ChangeBranchResult, error) {
	mu := b.taskMu(call.TaskID)
	mu.Lock()
	defer mu.Unlock()
	plan, _, err := b.changeTarget(ctx, call, args)
	if err != nil {
		return agentmcp.ChangeBranchResult{}, err
	}
	if plan.refusal != "" {
		return changeResult(args, plan.nick, agentmcp.BranchFailed, plan.refusal), nil
	}
	rmu := b.repoMutex(plan.sb.CodeRepoID)
	rmu.Lock()
	defer rmu.Unlock()
	entry, err := b.openRepo(ctx, plan.sb.CodeRepoID)
	if err != nil {
		return agentmcp.ChangeBranchResult{}, err
	}
	switched := plan.head != args.To
	if switched {
		if err := entry.SwitchWorktree(ctx, plan.own, args.To); err != nil {
			return changeResult(args, plan.nick, agentmcp.BranchFailed, "git could not switch the worktree: "+err.Error()), nil
		}
	}
	if err := b.deps.Tasks.ChangeBranch(plan.sb.ID, args.From, args.To); err != nil {
		slog.Warn("ade: change branch row", "scope", "ade", "branch", plan.sb.ID, "err", err)
		if switched && plan.head != "" {
			if rerr := entry.SwitchWorktree(ctx, plan.own, plan.head); rerr != nil {
				slog.Error("ade: change branch rollback failed", "scope", "ade", "branch", plan.sb.ID, "err", rerr)
			}
		}
		if errors.Is(err, repos.ErrBranchNameTaken) {
			return changeResult(args, plan.nick, agentmcp.BranchFailed, fmt.Sprintf("Another branch of this repo on a task already uses %s.", args.To)), nil
		}
		return changeResult(args, plan.nick, agentmcp.BranchFailed, "Kira Space could not change this branch."), nil
	}
	// The old branch's change request no longer belongs to the row.
	if err := b.deps.Tasks.SetBranchChangeRequests([]string{plan.sb.ID}, nil); err != nil {
		slog.Warn("ade: change branch clear change request", "scope", "ade", "branch", plan.sb.ID, "err", err)
	}
	res := changeResult(args, plan.nick, agentmcp.BranchChanged, "")
	res.Worktree = plan.own
	res.Next = fmt.Sprintf("The task branch is now %s in %s. Work there.", args.To, plan.own)
	b.notifyBoard()
	return res, nil
}
