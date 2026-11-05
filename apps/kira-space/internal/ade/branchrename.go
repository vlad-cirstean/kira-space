package ade

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitclient/porcelain"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitsession"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/repos"
	"github.com/kirathecat/kira-studio/internal/agentmcp"
)

// branchrename.go: an agent asks the user to rename one of its task's branches. The worktree folder is
// named by an id, so it stays; the rename moves the git branch, the task row and every row stacked on it.

const branchKindRename = "rename"

// renameFacts are what a rename checks found about the branch's remote tracking.
type renameFacts struct {
	remote string // remote the branch tracks by its own name, "" when it does not
	// trackedOther is true when the branch tracks a differently named remote branch, kept as is.
	trackedOther bool
}

// renamePlan is a validated rename: the row, the request waiting for the user and the remote facts.
type renamePlan struct {
	sb      model.AdeTaskBranch
	nick    string
	facts   renameFacts
	req     *branchRequest
	refusal string
}

func renameResult(args agentmcp.RenameBranchArgs, nick, outcome, next string) agentmcp.RenameBranchResult {
	return agentmcp.RenameBranchResult{
		Repo: nick, From: args.From, To: args.To, Outcome: outcome, Remote: agentmcp.RemoteKept, Next: next,
	}
}

// RenameBranch implements agentmcp.SpaceTools. A refused rename answers at once; otherwise the user
// decides in one prompt and the call waits, unlocked.
func (b *TaskBoard) RenameBranch(ctx context.Context, call agentmcp.SpaceCall, args agentmcp.RenameBranchArgs) (agentmcp.RenameBranchResult, error) {
	plan, err := b.planRename(ctx, call, args)
	if err != nil {
		return agentmcp.RenameBranchResult{}, err
	}
	if plan.refusal != "" {
		return renameResult(args, plan.nick, agentmcp.BranchFailed, plan.refusal), nil
	}
	dec, err := b.awaitBranchRequest(ctx, call, plan.req)
	if err != nil {
		return agentmcp.RenameBranchResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return agentmcp.RenameBranchResult{}, err // the caller is gone: nothing is renamed for it
	}
	switch {
	case dec.kind == decisionAbandoned:
		return agentmcp.RenameBranchResult{}, agentmcp.ToolError("The rename request was cancelled.")
	case dec.kind == decisionTimeout:
		next := fmt.Sprintf("No answer from the user in %s; nothing was renamed. Call rename_branch again later or ask the user.", waitText(b.branchRequestWait()))
		return renameResult(args, plan.nick, agentmcp.BranchTimeout, next), nil
	case dec.kind == decisionDenied || !dec.approved[plan.req.view.Items[0].ID]:
		return renameResult(args, plan.nick, agentmcp.BranchDenied, "The user declined this rename. Do not rename it with git; keep the branch name or ask the user."), nil
	}
	return b.applyRename(ctx, call, args, dec.renameRemote)
}

// planRename validates the rename under the task lock and builds the request.
func (b *TaskBoard) planRename(ctx context.Context, call agentmcp.SpaceCall, args agentmcp.RenameBranchArgs) (renamePlan, error) {
	mu := b.taskMu(call.TaskID)
	mu.Lock()
	defer mu.Unlock()
	plan, tc, err := b.renameTarget(ctx, call.TaskID, args)
	if err != nil || plan.refusal != "" {
		return plan, err
	}
	stacked := stackedOn(tc, plan.sb)
	if stacked == nil {
		stacked = []string{}
	}
	item := adewire.BranchRequestItem{
		ID: "i0", CodeRepoID: plan.sb.CodeRepoID, Repo: plan.nick, Kind: branchKindRename,
		Branch: args.To, From: args.From, Stacked: stacked,
	}
	if plan.facts.remote != "" {
		item.Upstream = plan.facts.remote + "/" + args.From
	}
	source, stepName := b.callSource(tc, call)
	now := b.deps.Now()
	plan.req = &branchRequest{
		view: adewire.BranchRequest{
			ID: b.newID(), TaskID: call.TaskID, TaskTitle: taskTitle(tc.task, tc.branches), Source: source, StepName: stepName,
			Items: []adewire.BranchRequestItem{item}, CreatedAt: now.UnixMilli(), ExpiresAt: now.Add(b.branchRequestWait()).UnixMilli(),
		},
		result: make(chan branchDecision, 1),
	}
	return plan, nil
}

// stackedOn names the live branches of the same repository built on sb, by planner link or base name.
func stackedOn(tc *taskCtx, sb model.AdeTaskBranch) []string {
	out := []string{}
	for _, br := range tc.branches {
		if br.ID == sb.ID || br.CodeRepoID != sb.CodeRepoID || br.Name == "" {
			continue
		}
		if br.BaseBranchID == sb.ID || (br.BaseBranchID == "" && br.Base == sb.Name) {
			out = append(out, br.Name)
		}
	}
	return out
}

// renameTarget finds the task's row for args and runs every refusal; plan.refusal holds the agent-facing
// text of the first that applies. The caller holds the task lock.
func (b *TaskBoard) renameTarget(ctx context.Context, taskID string, args agentmcp.RenameBranchArgs) (renamePlan, *taskCtx, error) {
	tc, err := b.spaceCtx(taskID)
	if err != nil {
		return renamePlan{}, nil, err
	}
	if err := b.spaceWritable(tc); err != nil {
		return renamePlan{}, nil, err
	}
	_, cfgs, err := b.registeredCandidates()
	if err != nil {
		return renamePlan{}, nil, err
	}
	id, err := resolveRepoRef(args.Repo, b.taskCandidates(tc, cfgs), "task")
	if err != nil {
		return renamePlan{}, nil, err
	}
	plan := renamePlan{nick: tc.nick[id]}
	named, _ := taskRepoRows(tc, id, args.From)
	if named == nil {
		return plan, nil, agentmcp.ToolError(fmt.Sprintf("Branch %q is not on this task in that repo.", args.From))
	}
	plan.sb = *named
	if named.Kind != model.AdeBranchKindMine {
		plan.refusal = fmt.Sprintf("The branch %s of %s on this task is not yours to rename.", args.From, plan.nick)
		return plan, tc, nil
	}
	plan.refusal, plan.facts, err = b.renameRefusal(ctx, plan.sb, args.To)
	return plan, tc, err
}

// renameRefusal returns the text for the first reason sb cannot take the name to, "" when it can.
func (b *TaskBoard) renameRefusal(ctx context.Context, sb model.AdeTaskBranch, to string) (string, renameFacts, error) {
	rec, err := b.deps.CodeRepos.Get(sb.CodeRepoID)
	if err != nil {
		return "", renameFacts{}, err
	}
	if rec == nil {
		return "Repo is no longer registered.", renameFacts{}, nil
	}
	if text, err := b.nameRefusal(ctx, *rec, sb.Name, to); text != "" || err != nil {
		return text, renameFacts{}, err
	}
	if text := b.busyRefusal(ctx, sb); text != "" {
		return text, renameFacts{}, nil
	}
	entry, err := b.openRepo(ctx, sb.CodeRepoID)
	if err != nil {
		return "", renameFacts{}, err
	}
	inv, err := entry.BranchInventory(ctx)
	if err != nil {
		return "", renameFacts{}, err
	}
	if branchExists(inv, to) {
		return fmt.Sprintf("Branch %s already exists. Pick another name.", to), renameFacts{}, nil
	}
	facts, ok := trackingOf(inv, sb.Name)
	if !ok {
		return fmt.Sprintf("Branch %s no longer exists in the repo.", sb.Name), renameFacts{}, nil
	}
	text, err := b.remoteRefusal(ctx, entry, sb.Name, to, facts)
	return text, facts, err
}

// nameRefusal checks both names: git validity and reserved names for to, reserved names for from.
func (b *TaskBoard) nameRefusal(ctx context.Context, rec model.CodeRepo, from, to string) (string, error) {
	if text, err := b.checkNewBranchName(ctx, rec, to); text != "" || err != nil {
		return text, err
	}
	text, err := b.reservedBranchText(ctx, rec.ID, from)
	if text != "" {
		return fmt.Sprintf("%q cannot be renamed: it belongs to the repo.", from), nil
	}
	return "", err
}

// busyRefusal refuses while Kira Space itself works on the branch: a sandbox, a rebase or a prepare script.
func (b *TaskBoard) busyRefusal(ctx context.Context, sb model.AdeTaskBranch) string {
	if b.branchSandboxed(ctx, sb) {
		return fmt.Sprintf("Branch %s runs in a sandbox; its name is fixed until that ends.", sb.Name)
	}
	if r, err := b.deps.Tasks.RunningRebaseOn(sb.ID); err != nil || r != nil {
		return fmt.Sprintf("A rebase runs on %s. Try again when it ends.", sb.Name)
	}
	b.runMu.Lock()
	_, setting := b.setupLive[sb.ID]
	b.runMu.Unlock()
	if setting {
		return fmt.Sprintf("The prepare script is still running on %s. Try again when it ends.", sb.Name)
	}
	return ""
}

// branchSandboxed reports whether a live sandbox is pinned to the branch or to its worktree.
func (b *TaskBoard) branchSandboxed(ctx context.Context, sb model.AdeTaskBranch) bool {
	found := false
	b.sandboxTargets.Range(func(_, v any) bool {
		t, ok := v.(sandboxTarget)
		found = ok && t.codeRepoID == sb.CodeRepoID && t.branch == sb.Name
		return !found
	})
	if found {
		return true
	}
	wt := b.worktreePath(ctx, sb)
	b.runMu.Lock()
	defer b.runMu.Unlock()
	for _, live := range b.tuiSandbox {
		if live.sr == nil || live.done {
			continue
		}
		if name, ok := live.sr.policy.Branch[wt]; ok && wt != "" && name == sb.Name {
			return true
		}
	}
	return false
}

// trackingOf reads which remote branch the local branch tracks; ok is false when the branch is gone.
func trackingOf(inv []porcelain.InventoryRef, name string) (renameFacts, bool) {
	for _, r := range inv {
		if r.Remote != "" || r.Short != name {
			continue
		}
		rest := strings.TrimPrefix(r.Upstream, "refs/remotes/")
		remote, branch, ok := strings.Cut(rest, "/")
		switch {
		case r.Upstream == "" || !ok:
			return renameFacts{}, true
		case branch == name:
			return renameFacts{remote: remote}, true
		default:
			return renameFacts{trackedOther: true}, true
		}
	}
	return renameFacts{}, false
}

// remoteRefusal refuses a rename touching a protected branch name or one with an open change request.
func (b *TaskBoard) remoteRefusal(ctx context.Context, entry *gitsession.RepoEntry, from, to string, facts renameFacts) (string, error) {
	if res := entry.ResolveBranchChangeRequest(ctx, from); res.Kind == "ok" {
		for _, cr := range res.ChangeRequests {
			if cr.State == "open" {
				return fmt.Sprintf("Branch %s has an open change request (#%d). Renaming it would orphan the request.", from, cr.Number), nil
			}
		}
	}
	remote := facts.remote
	if remote == "" {
		remote, _ = entry.DefaultRemote(ctx)
	}
	if remote == "" {
		return "", nil
	}
	for _, name := range []string{from, to} {
		pf, err := entry.PushPreflightTo(ctx, remote, name, name)
		if err != nil {
			slog.Warn("ade: rename protected check", "scope", "ade", "branch", name, "err", err)
			continue
		}
		if pf.ProtectedBy != nil {
			return fmt.Sprintf("%s is a protected branch name.", name), nil
		}
	}
	return "", nil
}

// applyRename renames after approval, under the task lock, with every refusal checked again since the wait.
func (b *TaskBoard) applyRename(ctx context.Context, call agentmcp.SpaceCall, args agentmcp.RenameBranchArgs, renameRemote bool) (agentmcp.RenameBranchResult, error) {
	mu := b.taskMu(call.TaskID)
	mu.Lock()
	defer mu.Unlock()
	plan, tc, err := b.renameTarget(ctx, call.TaskID, args)
	if err != nil {
		return agentmcp.RenameBranchResult{}, err
	}
	if plan.refusal != "" {
		return renameResult(args, plan.nick, agentmcp.BranchFailed, plan.refusal), nil
	}
	rmu := b.repoMutex(plan.sb.CodeRepoID)
	rmu.Lock()
	defer rmu.Unlock()
	entry, err := b.openRepo(ctx, plan.sb.CodeRepoID)
	if err != nil {
		return agentmcp.RenameBranchResult{}, err
	}
	oldURL, _ := entry.BranchURL(ctx, args.From)
	if msg := b.renameLocal(ctx, entry, plan, args); msg != "" {
		return renameResult(args, plan.nick, agentmcp.BranchFailed, msg), nil
	}
	res := renameResult(args, plan.nick, agentmcp.BranchRenamed, "")
	if row, err := b.deps.Tasks.GetBranch(plan.sb.ID); err == nil {
		res.Worktree = b.worktreePath(ctx, row)
	}
	switch {
	case plan.facts.remote == "":
		res.Remote = agentmcp.RemoteUntracked
		if plan.facts.trackedOther {
			res.Remote = agentmcp.RemoteKept
		}
	case renameRemote:
		res.Remote = b.renameRemoteBranch(ctx, entry, plan.facts.remote, args)
	}
	if res.Remote == agentmcp.RemoteRenamed {
		b.followTaskLink(ctx, tc, entry, oldURL, args.To)
	}
	res.Next = renameNext(res)
	b.notifyBoard()
	return res, nil
}

func renameNext(res agentmcp.RenameBranchResult) string {
	text := fmt.Sprintf("Renamed %s to %s; the worktree %s is unchanged.", res.From, res.To, res.Worktree)
	if res.Remote == agentmcp.RemoteFailed {
		text += " The remote branch was not renamed: push " + res.To + " yourself if it should exist there."
	}
	return text
}

// renameLocal renames the git branch, then the task rows; a failed row update puts the git name back.
// It returns the failure text, "" on success.
func (b *TaskBoard) renameLocal(ctx context.Context, entry *gitsession.RepoEntry, plan renamePlan, args agentmcp.RenameBranchArgs) string {
	op := gitsession.OpRequest{Kind: "branchRename", From: args.From, To: args.To, UnsetUpstream: plan.facts.remote != ""}
	if text := b.runBranchOp(ctx, entry, op); text != "" {
		return text
	}
	if _, err := b.deps.Tasks.RenameBranch(plan.sb.ID, args.From, args.To); err != nil {
		slog.Warn("ade: rename branch row", "scope", "ade", "branch", plan.sb.ID, "err", err)
		if text := b.runBranchOp(ctx, entry, gitsession.OpRequest{Kind: "branchRename", From: args.To, To: args.From}); text != "" {
			slog.Error("ade: rename rollback failed", "scope", "ade", "branch", plan.sb.ID, "err", text)
		}
		if errors.Is(err, repos.ErrBranchNameTaken) {
			return fmt.Sprintf("Another branch of this repo on a task already uses %s.", args.To)
		}
		return "Kira Space could not rename this branch."
	}
	return ""
}

// runBranchOp runs a git op and returns its failure text, "" on success.
func (b *TaskBoard) runBranchOp(ctx context.Context, entry *gitsession.RepoEntry, op gitsession.OpRequest) string {
	res, err := entry.RunOp(ctx, b.conn.ID, boardConnLabel, op)
	switch {
	case err != nil:
		return "git could not rename the branch: " + err.Error()
	case !res.OK && res.Error != nil:
		return res.Error.Message
	case !res.OK:
		return "git could not rename the branch."
	}
	return ""
}

// renameRemoteBranch pushes the new name, tracking it, then deletes the old remote branch. A failure of
// either leaves the local rename standing and is reported as failed.
func (b *TaskBoard) renameRemoteBranch(ctx context.Context, entry *gitsession.RepoEntry, remote string, args agentmcp.RenameBranchArgs) string {
	deps := gitsession.RemoteDeps{Askpass: b.deps.Askpass}
	push := gitsession.RemoteOpParams{Kind: "push", Remote: remote, Branch: args.To, RemoteBranch: args.To, SetUpstream: true}
	del := gitsession.RemoteOpParams{Kind: "deleteRemoteBranch", Remote: remote, Branch: args.From}
	for _, params := range []gitsession.RemoteOpParams{push, del} {
		res, err := entry.RunRemote(ctx, b.conn, params, deps)
		if err != nil || !res.OK {
			slog.Warn("ade: rename remote branch", "scope", "ade", "kind", params.Kind, "remote", remote, "err", err, "result", res.Error)
			return agentmcp.RemoteFailed
		}
	}
	return agentmcp.RemoteRenamed
}

// followTaskLink moves the task's link along when it pointed at the old branch page.
func (b *TaskBoard) followTaskLink(ctx context.Context, tc *taskCtx, entry *gitsession.RepoEntry, oldURL, to string) {
	if oldURL == "" || tc.task.HostURL != oldURL {
		return
	}
	next, ok := entry.BranchURL(ctx, to)
	if !ok {
		return
	}
	if _, err := b.deps.Tasks.UpdateTask(tc.task.ID, model.AdeTaskPatch{HostURL: &next}); err != nil {
		slog.Warn("ade: rename task link", "scope", "ade", "task", tc.task.ID, "err", err)
	}
}
