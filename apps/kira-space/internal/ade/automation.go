package ade

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/repos"
	"github.com/kirathecat/kira-studio/internal/agentmcp"
	"github.com/kirathecat/kira-studio/internal/scriptruns"
)

// automation.go is the board's side of scripts in ADE (P242 Part 3): it implements
// scriptruns.ADE, so a script run can resolve a task's variables, pick its branch, and hold a smart
// run's worktree against the pipeline.

var _ scriptruns.ADE = (*TaskBoard)(nil)

// noteWaitingAutomation holds a pending step run behind a smart script working in its worktree; the
// script's name follows.
const noteWaitingAutomation = "waiting for script run "

// noteWaitingAutomationOld is the pre-rename note; a run held by an older version still releases.
const noteWaitingAutomationOld = "waiting for automation "

// claimOn is the name of the automation holding the branch's worktree, "" when none.
func (b *TaskBoard) claimOn(branchID string) string {
	b.runMu.Lock()
	defer b.runMu.Unlock()
	return b.claims[branchID]
}

// Tasks implements scriptruns.ADE: the live tasks in board order.
func (b *TaskBoard) Tasks() ([]scriptruns.TaskChoice, error) {
	tasks, err := b.deps.Tasks.ListLive()
	if err != nil {
		return nil, err
	}
	branches, err := b.deps.Tasks.BranchesLive()
	if err != nil {
		return nil, err
	}
	byTask := map[string][]model.AdeTaskBranch{}
	for _, br := range branches {
		byTask[br.TaskID] = append(byTask[br.TaskID], br)
	}
	out := make([]scriptruns.TaskChoice, 0, len(tasks))
	for _, t := range tasks {
		out = append(out, scriptruns.TaskChoice{ID: t.ID, Title: taskTitle(t, byTask[t.ID])})
	}
	return out, nil
}

// archived reports whether the task is archived: a run for it sees the same as a deleted task.
func (b *TaskBoard) archived(taskID string) bool {
	task, err := b.deps.Tasks.GetTask(taskID)
	return err == nil && task.ArchivedAt != nil
}

// Context implements scriptruns.ADE.
func (b *TaskBoard) Context(taskID, branchID string) (scriptruns.ADEContext, error) {
	tc, err := b.loadTaskCtx(taskID)
	if errors.Is(err, repos.ErrTaskNotFound) || b.archived(taskID) {
		return scriptruns.ADEContext{}, errors.New("the task no longer exists")
	}
	if err != nil {
		return scriptruns.ADEContext{}, errors.New(trimInvalid(err))
	}
	title := taskTitle(tc.task, tc.branches)
	c := scriptruns.ADEContext{
		TaskID: taskID, TaskTitle: title, Branches: []scriptruns.BranchChoice{},
		Vars: map[string]string{"task": title, "jira": tc.task.JiraKey},
	}
	chosen := map[string]model.AdeTaskBranch{}
	var enabled []string
	for _, br := range tc.branches {
		if br.Kind != model.AdeBranchKindMine && br.Kind != model.AdeBranchKindReview {
			continue
		}
		ch := scriptruns.BranchChoice{ID: br.ID, Label: tc.nick[br.CodeRepoID] + " · " + br.Name}
		if br.Name == "" {
			ch.Label, ch.Disabled, ch.Why = tc.nick[br.CodeRepoID], true, "not created yet"
		} else {
			enabled = append(enabled, br.ID)
		}
		c.Branches = append(c.Branches, ch)
		chosen[br.ID] = br
	}
	pick := branchID
	if pick == "" && len(enabled) == 1 {
		pick = enabled[0]
	}
	for i := range c.Branches {
		if c.Branches[i].ID == pick && !c.Branches[i].Disabled {
			c.Branch = &c.Branches[i]
		}
	}
	if c.Branch == nil {
		return c, nil
	}
	return c, b.branchContext(tc, chosen[pick], &c)
}

// branchContext adds a chosen branch's variables, worktree and setup state to c.
func (b *TaskBoard) branchContext(tc *taskCtx, sb model.AdeTaskBranch, c *scriptruns.ADEContext) error {
	path, err := b.worktreeOf(b.ctx, sb)
	if err != nil {
		return err
	}
	if path == "" {
		rec, err := b.deps.CodeRepos.Get(sb.CodeRepoID)
		if err != nil {
			return err
		}
		if rec == nil {
			return fmt.Errorf("code repo %s is no longer registered", sb.CodeRepoID)
		}
		path, c.Pending = b.freePath(rec.Name, sb.Name), true
	}
	c.Worktree = path
	for name, v := range b.adeVars(tc, sb, path) {
		c.Vars[name] = v
	}
	setups, err := b.deps.Tasks.SetupByBranch()
	if err != nil {
		return err
	}
	if s, ok := setups[sb.ID]; ok && s.State != model.AdeSetupReady {
		c.Preparing = fmt.Sprintf("the worktree of %s is still being prepared", sb.Name)
		if s.State != model.AdeSetupRunning {
			c.Preparing = setupGateError(sb, s).Error()
		}
	}
	return nil
}

// adeVars are the built-in variables of a run on sb: the base is the branch's own, else the repo's main branch.
func (b *TaskBoard) adeVars(tc *taskCtx, sb model.AdeTaskBranch, path string) map[string]string {
	base := sb.Base
	if base == "" && sb.CodeRepoID != "" {
		base = b.mainShortName(b.ctx, sb.CodeRepoID)
	}
	return map[string]string{
		"task": taskTitle(tc.task, tc.branches), "jira": tc.task.JiraKey, "repo": tc.nick[sb.CodeRepoID],
		"branch": sb.Name, "base": base, "worktree": path,
	}
}

// busyLocked says why a smart run cannot take the branch now; the task mutex is held.
func (b *TaskBoard) busyLocked(sb model.AdeTaskBranch) (string, error) {
	if busy, err := b.deps.Tasks.HasActiveStepRunOn(sb.ID); err != nil {
		return "", err
	} else if busy {
		return "a run is working on " + sb.Name, nil
	}
	if r, err := b.deps.Tasks.RunningRebaseOn(sb.ID); err != nil {
		return "", err
	} else if r != nil {
		return "a rebase is running on " + sb.Name, nil
	}
	if name := b.claimOn(sb.ID); name != "" {
		return fmt.Sprintf("script run %s is running in this worktree", name), nil
	}
	return "", nil
}

// Busy implements scriptruns.ADE.
func (b *TaskBoard) Busy(branchID string) string {
	sb, err := b.deps.Tasks.GetBranch(branchID)
	if err != nil {
		return "that branch no longer exists"
	}
	mu := b.taskMu(sb.TaskID)
	mu.Lock()
	defer mu.Unlock()
	why, err := b.busyLocked(sb)
	if err != nil {
		return err.Error()
	}
	return why
}

// ClaimWorktree implements scriptruns.ADE. It creates a missing worktree, then marks the branch as
// held by script: step runs and rebases on it wait until release.
func (b *TaskBoard) ClaimWorktree(ctx context.Context, branchID, script string) (func(), error) {
	sb, err := b.deps.Tasks.GetBranch(branchID)
	if err != nil {
		return nil, errors.New("that branch no longer exists")
	}
	mu := b.taskMu(sb.TaskID)
	mu.Lock()
	defer mu.Unlock()
	if why, err := b.busyLocked(sb); err != nil {
		return nil, err
	} else if why != "" {
		return nil, errors.New(why)
	}
	tc, err := b.loadTaskCtx(sb.TaskID)
	if err != nil {
		return nil, errors.New(trimInvalid(err))
	}
	if _, err := b.launchGate(ctx, tc, sb); err != nil {
		return nil, errors.New(trimInvalid(err))
	}
	b.runMu.Lock()
	b.claims[branchID] = script
	b.runMu.Unlock()
	var once sync.Once
	return func() { once.Do(func() { b.releaseClaim(sb) }) }, nil
}

// releaseClaim drops the claim and launches the step runs that waited for it.
func (b *TaskBoard) releaseClaim(sb model.AdeTaskBranch) {
	mu := b.taskMu(sb.TaskID)
	mu.Lock()
	defer mu.Unlock()
	b.runMu.Lock()
	delete(b.claims, sb.ID)
	b.runMu.Unlock()
	if b.ctx.Err() != nil {
		return
	}
	b.launchHeldLocked(sb)
}

// Tools implements scriptruns.ADE.
func (b *TaskBoard) Tools() (agentmcp.SpaceTools, agentmcp.Outcomes) { return b, b }

// launchAutomationHeld launches the step runs a smart script held when the app last quit: no claim
// survives a restart.
func (b *TaskBoard) launchAutomationHeld() {
	byTask, err := b.deps.Tasks.RunsByLiveTask()
	if err != nil {
		return
	}
	released := map[string]bool{}
	for _, runs := range byTask {
		for _, r := range runs {
			if r.State != model.AdeRunPending || !(strings.HasPrefix(r.Note, noteWaitingAutomation) || strings.HasPrefix(r.Note, noteWaitingAutomationOld)) || released[r.BranchID] {
				continue
			}
			released[r.BranchID] = true
			sb, err := b.deps.Tasks.GetBranch(r.BranchID)
			if err != nil {
				continue
			}
			mu := b.taskMu(sb.TaskID)
			mu.Lock()
			b.launchHeldLocked(sb)
			mu.Unlock()
		}
	}
}

// SandboxGit serves fetch_remote and push_branch to a sandboxed smart run (scriptruns.ADE).
func (b *TaskBoard) SandboxGit() agentmcp.SandboxGit { return b }

// SandboxTarget points the sandbox git tools at a branch's worktree for one smart run.
func (b *TaskBoard) SandboxTarget(runID, branchID, worktree string) (func(), error) {
	sb, err := b.deps.Tasks.GetBranch(branchID)
	if err != nil {
		return nil, err
	}
	b.sandboxTargets.Store(runID, sandboxTarget{codeRepoID: sb.CodeRepoID, branch: sb.Name, worktree: worktree})
	return func() { b.sandboxTargets.Delete(runID) }, nil
}
