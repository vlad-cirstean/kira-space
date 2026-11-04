package ade

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitclient/porcelain"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitsession"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/repos"
	"github.com/kirathecat/kira-studio/internal/agentmcp"
	"github.com/kirathecat/kira-studio/internal/harness"
	"github.com/kirathecat/kira-studio/internal/llmprompts"
	"github.com/kirathecat/kira-studio/internal/runoutcome"
)

// rebase.go: a rebase is a run (purpose 'rebase') that a headless Claude carries out in the branch
// worktrees; Kira Space then checks the result in git (P241). It never enters the workflow.

const (
	rebaseStepID      = "rebase"
	noteWaitingRebase = "waiting for rebase"
	verifyTimeout     = 2 * time.Minute
)

const defaultRebaseTimeout = 20 * time.Minute

// rebaseBuiltins are the only built-ins a rebase run has: no Bash. Git goes through the validated
// git tool, which only runs in the stack's worktrees (agentmcp.CheckGitArgs).
var rebaseBuiltins = []string{"Read", "Edit", "Write", "Grep", "Glob"}

func rebaseTools(p harness.MCPHost) []string {
	out := slices.Clone(rebaseBuiltins)
	return append(out, harness.ToolNames(p, agentmcp.Git, agentmcp.FinishStep, agentmcp.RunOutcome)...)
}

// rebaseWorktrees are the stack's worktrees: where the git tool runs and what the run may edit.
func rebaseWorktrees(spec model.AdeRebaseSpec) []string {
	out := make([]string, 0, len(spec.Stack))
	for _, st := range spec.Stack {
		if st.Worktree != "" {
			out = append(out, st.Worktree)
		}
	}
	return out
}

func (b *TaskBoard) rebaseTimeout() time.Duration {
	if b.deps.RebaseTimeout > 0 {
		return b.deps.RebaseTimeout
	}
	return defaultRebaseTimeout
}

func (b *TaskBoard) rebaseDef(agent harness.Provider) stepDef {
	return stepDef{ID: rebaseStepID, Name: "Rebase", AllowedTools: rebaseTools(agent), DisallowedTools: []string{"Bash"}, Timeout: formatTimeout(b.rebaseTimeout())}
}

func formatTimeout(d time.Duration) string {
	switch {
	case d%time.Hour == 0:
		return fmt.Sprintf("%dh", d/time.Hour)
	case d%time.Minute == 0:
		return fmt.Sprintf("%dm", d/time.Minute)
	}
	return d.String()
}

func specOf(run model.AdeRun) (model.AdeRebaseSpec, error) {
	var spec model.AdeRebaseSpec
	if err := json.Unmarshal([]byte(run.SpecJSON), &spec); err != nil {
		return spec, fmt.Errorf("rebase run %s has no readable spec: %w", run.ID, err)
	}
	return spec, nil
}

// stackNode is one branch of a rebase: parent is the index of the branch it goes onto, -1 for the root.
type stackNode struct {
	sb     model.AdeTaskBranch
	parent int
}

// stackParent is the live branch br is built on: its planner base, else the branch it is queued
// after, else the branch its base name points at. "" when none.
func stackParent(sc *boardCtx, br model.AdeTaskBranch) string {
	if p, ok := sc.byID[br.BaseBranchID]; ok && p.ID != br.ID {
		return p.ID
	}
	if p, ok := sc.byID[br.QueuedAfter]; ok && p.ID != br.ID {
		return p.ID
	}
	if p, ok := sc.byName[br.Base]; ok && br.Base != "" && p.ID != br.ID {
		return p.ID
	}
	return ""
}

// rebaseStack is root plus the created branches of mine stacked on it, depth first.
func rebaseStack(sc *boardCtx, root model.AdeTaskBranch) []stackNode {
	kids := map[string][]model.AdeTaskBranch{}
	for _, br := range sc.byID {
		if br.Kind != model.AdeBranchKindMine || br.Name == "" || br.MergedAt != nil || br.ID == root.ID {
			continue
		}
		if p := stackParent(sc, br); p != "" {
			kids[p] = append(kids[p], br)
		}
	}
	for _, list := range kids {
		sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })
	}
	out := []stackNode{{sb: root, parent: -1}}
	seen := map[string]bool{root.ID: true}
	var walk func(at int)
	walk = func(at int) {
		for _, kid := range kids[out[at].sb.ID] {
			if seen[kid.ID] {
				continue
			}
			seen[kid.ID] = true
			out = append(out, stackNode{sb: kid, parent: at})
			walk(len(out) - 1)
		}
	}
	walk(0)
	return out
}

// rebasePlan is everything Rebase and RebasePreview derive from the current tree.
type rebasePlan struct {
	tc        *taskCtx
	root      model.AdeTaskBranch
	sc        *boardCtx
	nick      string
	spec      model.AdeRebaseSpec
	blockers  []adewire.RebaseBlocker
	noOp      bool
	resolved  bool
	target    baseTarget
	changeBs  bool // Onto set: the stored base changes
	review    model.AdeTaskBranch
	predicted map[string]string // branch id -> worktree path a missing worktree is predicted to get
	stack     []stackNode
}

func (p *rebasePlan) block(branchID, kind, format string, a ...any) {
	p.blockers = append(p.blockers, adewire.RebaseBlocker{BranchID: branchID, Kind: kind, Text: fmt.Sprintf(format, a...)})
}

// refusal is the first blocker that stops a real rebase, nil when it can start.
func (p *rebasePlan) refusal(autostash bool) error {
	for _, bl := range p.blockers {
		if bl.Kind == "dirty" && autostash {
			continue
		}
		return invalid("%s", bl.Text)
	}
	return nil
}

func hasTrackedChanges(entries []porcelain.StatusEntry) bool {
	for _, e := range entries {
		if e.Kind != "ignored" && e.Kind != "untracked" {
			return true
		}
	}
	return false
}

// planRebase resolves the target, the stack and the blockers for args. With commit it also makes each
// stack worktree of the task exist (the launch gate). The root task's mutex is held.
func (b *TaskBoard) planRebase(ctx context.Context, tc *taskCtx, args adewire.OntoArgs, commit bool) (*rebasePlan, error) {
	root, ok := tc.branch(args.BranchID)
	switch {
	case !ok:
		return nil, invalid("branch %s is not on a live task", args.BranchID)
	case tc.task.Kind == model.AdeTaskKindReview:
		return nil, invalid("a review task has no branch to rebase")
	case root.Kind != model.AdeBranchKindMine:
		return nil, invalid("only your own branch can be rebased")
	case root.Name == "":
		return nil, invalid("the branch of %s is not created yet", b.repoNick(tc, root))
	case root.MergedAt != nil:
		return nil, invalid("%s is merged", root.Name)
	}
	if err := b.checkNotArchiving(tc.task.ID); err != nil {
		return nil, err
	}
	rec, err := b.deps.CodeRepos.Get(root.CodeRepoID)
	if err != nil {
		return nil, err
	}
	if rec == nil {
		return nil, invalid("code repo %s not found", root.CodeRepoID)
	}
	entry, err := b.openRepo(ctx, root.CodeRepoID)
	if err != nil {
		return nil, err
	}
	sc, err := b.baseCtxFor(ctx, entry, root.CodeRepoID)
	if err != nil {
		return nil, err
	}
	p := &rebasePlan{tc: tc, root: root, nick: tc.nick[root.CodeRepoID], predicted: map[string]string{}}
	p.stack = rebaseStack(sc, root)

	// Worktrees: the task's own branches go through the launch gate on a real rebase.
	paths := make([]string, len(p.stack))
	for i, n := range p.stack {
		if row, ok := localBranch(sc.inv, n.sb.Name); ok {
			paths[i] = row.WorktreePath
		}
		if paths[i] == "" {
			if p.predicted[n.sb.ID], err = b.worktreeDirFor(n.sb, rec.Name, ""); err != nil {
				return nil, err
			}
		}
	}
	if commit {
		for i, n := range p.stack {
			if n.sb.TaskID == tc.task.ID {
				paths[i], err = b.launchGate(ctx, tc, n.sb)
			} else {
				paths[i], err = b.launchGateOf(ctx, n.sb)
			}
			if err != nil {
				return nil, err
			}
		}
		if sc, err = b.baseCtxFor(ctx, entry, root.CodeRepoID); err != nil { // the gate may have created branches
			return nil, err
		}
	}
	p.sc = sc

	if err := b.resolveRebaseTarget(p, args); err != nil {
		return nil, err
	}
	if !p.resolved {
		return p, nil
	}
	b.fillRebaseSpec(p, paths, args)
	b.rebaseBlockers(ctx, p, entry, paths)
	p.noOp = p.blockersClear() && b.rebaseIsNoOp(ctx, entry, p)
	return p, nil
}

func (p *rebasePlan) blockersClear() bool { return len(p.blockers) == 0 }

// launchGateOf opens the launch gate of a branch on another task, under that task's mutex.
func (b *TaskBoard) launchGateOf(ctx context.Context, sb model.AdeTaskBranch) (string, error) {
	mu := b.taskMu(sb.TaskID)
	mu.Lock()
	defer mu.Unlock()
	tc, err := b.loadTaskCtx(sb.TaskID)
	if err != nil {
		return "", err
	}
	return b.launchGate(ctx, tc, sb)
}

// resolveRebaseTarget picks the ref the root goes onto: the review branch to queue after, a chosen
// base (Change base), or the branch's current base. An unresolved base is a blocker, not an error.
func (b *TaskBoard) resolveRebaseTarget(p *rebasePlan, args adewire.OntoArgs) error {
	sc, root := p.sc, p.root
	switch {
	case args.QueueWith != "":
		other, ok := sc.byID[args.QueueWith]
		if !ok || other.ID == root.ID {
			return invalid("the branch to queue after must be another branch of the same repo")
		}
		t, err := b.plannerChoice(sc, &root, other)
		if err != nil {
			return err
		}
		p.review = other
		p.target = t
	case args.Onto != nil:
		t, err := b.resolveChoice(sc, &root, p.nick, *args.Onto)
		if err != nil {
			return err
		}
		p.target, p.changeBs = t, true
	default:
		br := sc.resolveBase(root)
		p.target = baseTarget{base: root.Base, baseBranchID: root.BaseBranchID, name: br.name, ref: br.ref, tip: br.tip, ok: br.ok, reason: br.reason}
	}
	p.resolved = p.target.ok
	switch p.target.reason {
	case baseParentDraft:
		p.block(root.ID, "parentDraft", "the base of %s is not created yet", root.Name)
	case baseMissing:
		p.block(root.ID, "baseMissing", "base %s no longer exists", cmpNonEmpty(p.target.name, root.Base))
	default:
		if !p.target.ok {
			p.block(root.ID, "baseMissing", "the repo has no main branch to rebase onto")
		}
	}
	return nil
}

func (b *TaskBoard) fillRebaseSpec(p *rebasePlan, paths []string, args adewire.OntoArgs) {
	sc := p.sc
	spec := model.AdeRebaseSpec{
		CodeRepoID: p.root.CodeRepoID, Repo: p.nick, Remote: sc.remote,
		OntoRef: p.target.ref, OntoName: p.target.display(sc.mainName), OntoTipBefore: p.target.tip,
		ChangeBase: p.changeBs, Push: args.Push, Autostash: args.Autostash, Review: p.review.Name,
	}
	for i, n := range p.stack {
		st := model.AdeRebaseStack{BranchID: n.sb.ID, Name: n.sb.Name, Worktree: cmpNonEmpty(paths[i], p.predicted[n.sb.ID])}
		if row, ok := localBranch(sc.inv, n.sb.Name); ok {
			st.Before = row.Tip
		}
		if n.parent < 0 {
			st.ParentRef, st.ParentName = p.target.ref, spec.OntoName
		} else {
			par := spec.Stack[n.parent]
			st.ParentRef, st.ParentName, st.ParentBefore = par.Name, par.Name, par.Before
		}
		spec.Stack = append(spec.Stack, st)
	}
	p.spec = spec
}

// rebaseBlockers reads each stack branch for the reasons a rebase cannot start now.
func (b *TaskBoard) rebaseBlockers(ctx context.Context, p *rebasePlan, entry *gitsession.RepoEntry, paths []string) {
	setups, _ := b.deps.Tasks.SetupByBranch()
	for i, n := range p.stack {
		sb := n.sb
		if busy, err := b.deps.Tasks.HasActiveStepRunOn(sb.ID); err == nil && busy {
			p.block(sb.ID, "running", "a background run is working on %s", sb.Name)
		} else if r, err := b.deps.Tasks.RunningRebaseOn(sb.ID); err == nil && r != nil {
			p.block(sb.ID, "running", "a rebase is already running on %s", sb.Name)
		} else if name := b.claimOn(sb.ID); name != "" {
			p.block(sb.ID, "running", "script run %s is running in %s", name, sb.Name)
		}
		if s, ok := setups[sb.ID]; ok && s.State != model.AdeSetupReady {
			p.block(sb.ID, "setup", "the worktree of %s is still being prepared", sb.Name)
		}
		if paths[i] == "" {
			p.block(sb.ID, "noWorktree", "%s has no worktree yet: one is created", sb.Name)
			continue
		}
		status, err := entry.WorktreeStatus(ctx, paths[i])
		if err != nil {
			slog.Warn("ade: rebase status", "scope", "ade", "branch", sb.ID, "err", err)
			continue
		}
		if op := entry.WorktreeInProgress(paths[i], status); op != nil {
			p.block(sb.ID, "inProgress", "a %s is in progress in %s: abort it first", opName(op), paths[i])
		} else if hasTrackedChanges(status) {
			p.block(sb.ID, "dirty", "%s has uncommitted changes", sb.Name)
		}
	}
}

// rebaseIsNoOp reports whether every branch already sits on its base, so no run is needed.
func (b *TaskBoard) rebaseIsNoOp(ctx context.Context, entry *gitsession.RepoEntry, p *rebasePlan) bool {
	tips := make([]string, len(p.stack))
	for i, st := range p.spec.Stack {
		tips[i] = st.Before
	}
	for i, n := range p.stack {
		parentTip := p.target.tip
		if n.parent >= 0 {
			parentTip = tips[n.parent]
		}
		if parentTip == "" || tips[i] == "" {
			return false
		}
		if on, err := entry.IsAncestor(ctx, parentTip, tips[i]); err != nil || !on {
			return false
		}
	}
	return true
}

// RebasePreview returns the prompt a Rebase would send, its stack and what blocks it.
func (b *TaskBoard) RebasePreview(ctx context.Context, args adewire.OntoArgs) (adewire.RebasePreview, error) {
	sb, err := b.deps.Tasks.GetBranch(args.BranchID)
	if err != nil {
		return adewire.RebasePreview{}, err
	}
	mu := b.taskMu(sb.TaskID)
	mu.Lock()
	defer mu.Unlock()
	tc, err := b.loadTaskCtx(sb.TaskID)
	if err != nil {
		return adewire.RebasePreview{}, err
	}
	p, err := b.planRebase(ctx, tc, args, false)
	if err != nil {
		return adewire.RebasePreview{}, err
	}
	out := adewire.RebasePreview{Suffix: agentmcp.RebaseReport(b.prompts()), Stack: []adewire.RebaseStackItem{}, Blockers: p.blockers, NoOp: p.noOp}
	if out.Blockers == nil {
		out.Blockers = []adewire.RebaseBlocker{}
	}
	if p.resolved {
		out.Prompt = rebasePromptText(b.prompts(), p.spec)
		for _, st := range p.spec.Stack {
			out.Stack = append(out.Stack, adewire.RebaseStackItem{BranchID: st.BranchID, Name: st.Name, Worktree: st.Worktree, OntoRef: st.ParentRef})
		}
	}
	return out, nil
}

// Rebase stores the base change, then starts the headless run that carries the rebase out.
func (b *TaskBoard) Rebase(ctx context.Context, args adewire.RebaseArgs) (adewire.RebaseStart, error) {
	onto := adewire.OntoArgs{BranchID: args.BranchID, Onto: args.Onto, QueueWith: args.QueueWith, Push: args.Push, Autostash: args.Autostash}
	root, err := b.deps.Tasks.GetBranch(args.BranchID)
	if err != nil {
		return adewire.RebaseStart{}, err
	}
	mu := b.taskMu(root.TaskID)
	mu.Lock()
	defer mu.Unlock()
	tc, err := b.loadTaskCtx(root.TaskID)
	if err != nil {
		return adewire.RebaseStart{}, err
	}
	p, err := b.planRebase(ctx, tc, onto, true)
	if err != nil {
		return adewire.RebaseStart{}, err
	}
	if err := p.refusal(args.Autostash); err != nil {
		return adewire.RebaseStart{}, err
	}
	if err := b.storeRebaseBase(p, p.noOp); err != nil {
		return adewire.RebaseStart{}, err
	}
	if p.noOp {
		b.notifyBoard()
		return adewire.RebaseStart{NoOp: true}, nil
	}
	message := args.Message
	if message == "" {
		message = rebasePromptText(b.prompts(), p.spec)
	} else {
		for _, st := range p.spec.Stack { // the preview predicted a path for a worktree that did not exist yet
			if pred := p.predicted[st.BranchID]; pred != "" && pred != st.Worktree {
				message = strings.ReplaceAll(message, pred, st.Worktree)
			}
		}
	}
	raw, err := json.Marshal(p.spec)
	if err != nil {
		return adewire.RebaseStart{}, err
	}
	n, err := b.deps.Tasks.CountRebaseRuns(p.root.ID)
	if err != nil {
		return adewire.RebaseStart{}, err
	}
	run := model.AdeRun{
		ID: b.newID(), TaskID: tc.task.ID, StepID: rebaseStepID, BranchID: p.root.ID, Attempt: n + 1,
		State: model.AdeRunPending, Purpose: model.AdeRunPurposeRebase, SpecJSON: string(raw),
	}
	if err := b.deps.Tasks.InsertRun(run); err != nil {
		return adewire.RebaseStart{}, err
	}
	run, err = b.startRebase(run, p.spec, rebaseRunPrompt(b.prompts(), message))
	if err != nil {
		if !errors.Is(err, errBoardClosed) {
			b.failLaunch(run, err)
		}
		return adewire.RebaseStart{}, err
	}
	b.notifyBoard()
	return adewire.RebaseStart{RunID: run.ID}, nil
}

// storeRebaseBase writes the base change before the run starts (a failed rebase keeps it). noOp
// means the branches already sit on it, so nothing stays pending.
func (b *TaskBoard) storeRebaseBase(p *rebasePlan, noOp bool) error {
	root := p.root
	if p.review.ID != "" {
		if err := b.deps.Tasks.SetQueuedAfter(root.ID, p.review.ID); err != nil {
			return err
		}
	}
	if p.changeBs {
		pending := root.BasePendingFrom
		changed := p.target.base != root.Base || p.target.baseBranchID != root.BaseBranchID
		if pending == "" && changed {
			old := p.sc.resolveBase(root)
			pending = cmpNonEmpty(old.name, cmpNonEmpty(root.Base, p.sc.mainName))
		}
		if noOp {
			pending = ""
		}
		if err := b.deps.Tasks.SetBranchBase(root.ID, p.target.base, p.target.baseBranchID, pending); err != nil {
			return err
		}
	}
	if noOp {
		ids := make([]string, len(p.stack))
		for i, n := range p.stack {
			ids[i] = n.sb.ID
		}
		return b.deps.Tasks.ClearBasePending(ids)
	}
	return nil
}

// startRebase launches the run's headless session. The task mutex is held.
func (b *TaskBoard) startRebase(run model.AdeRun, spec model.AdeRebaseSpec, prompt string) (model.AdeRun, error) {
	root := spec.Stack[0]
	if err := b.deps.Logs.Reset(repos.AdeLogRun, run.ID, run.TaskID); err != nil {
		return run, err
	}
	if !b.track() {
		return run, errBoardClosed
	}
	started := false
	defer func() {
		if !started {
			b.wg.Done()
		}
	}()
	now := b.deps.Now().UnixMilli()
	sessionID, agentSessionID := b.newID(), b.newID()
	agent := b.deps.Agent.Active()
	if err := b.deps.Sessions.InsertHeadless(model.AdeSession{
		ID: sessionID, Mode: model.AdeSessionModeHeadless, TaskID: run.TaskID, BranchID: run.BranchID, StepID: rebaseStepID,
		RunID: run.ID, AgentSessionID: agentSessionID, Cwd: root.Worktree, Provider: agent.ID(), State: model.AdeSessionStateRunning, StartedAt: now, LastActiveAt: now,
	}); err != nil {
		return run, err
	}
	b.notifySessions()
	b.sessionOps.put(sessionID, b.startSessionOp(b.ctx, opRebase, run.TaskID, agent.Label(), ""))
	b.cancelRunOp(b.sessionOps.peek(sessionID), run.ID)
	defer func() {
		if !started {
			b.finishRunOp(sessionID, outcome{state: model.AdeRunFailed, note: "The rebase did not start."})
		}
	}()
	running := model.AdeRunRunning
	updated, err := b.deps.Tasks.UpdateRun(run.ID, model.AdeRunPatch{State: &running, StartedAt: &now, SessionID: &sessionID})
	if err != nil {
		return run, err
	}
	b.emitRuns(updated)
	runCtx, endRun := b.beginLive(b.live, run.ID)
	started = true
	go func() {
		defer b.wg.Done()
		defer endRun()
		b.superviseAgent(runCtx, updated, agentLaunch{
			def: b.rebaseDef(agent), agent: agent, sessionID: sessionID, path: root.Worktree, prompt: prompt, timeout: b.rebaseTimeout(),
			git: &agentmcp.GitGrant{Worktrees: rebaseWorktrees(spec), Push: spec.Push},
		})
	}()
	return updated, nil
}

// completeRebase verifies the result in git, then settles the run. f is the agent's last finish_step
// call, nil when it made none; end says how the process ended.
func (b *TaskBoard) completeRebase(run model.AdeRun, sessionID string, f *agentmcp.Finish, end outcome) {
	spec, err := specOf(run)
	if err != nil {
		slog.Warn("ade: rebase spec", "scope", "ade", "run", run.ID, "err", err)
		return
	}
	vctx, cancel := context.WithTimeout(b.ctx, verifyTimeout)
	defer cancel()
	chk := b.verifyRebase(vctx, spec)
	out := decideRebaseOutcome(f, chk, end)
	mu := b.taskMu(run.TaskID)
	mu.Lock()
	defer mu.Unlock()
	b.settleRebase(run.ID, sessionID, spec, out, false)
}

// settleRebase stores a rebase run's outcome, clears the pending-base mark of a verified rebase and
// releases the step runs held behind it. Unless force, a run no longer running is left alone. The
// task mutex is held.
func (b *TaskBoard) settleRebase(runID, sessionID string, spec model.AdeRebaseSpec, out outcome, force bool) {
	cur, err := b.deps.Tasks.GetRun(runID)
	if err != nil || (!force && cur.State != model.AdeRunRunning) {
		return
	}
	now := b.deps.Now().UnixMilli()
	b.finishRunOp(sessionID, out)
	if sessionID != "" {
		if err := b.deps.Sessions.MarkStopped(sessionID, now); err != nil {
			slog.Warn("ade: stop headless session", "scope", "ade", "session", sessionID, "err", err)
		}
		b.notifySessions()
	}
	updated, err := b.deps.Tasks.UpdateRun(runID, model.AdeRunPatch{
		State: &out.state, Note: &out.note, Summary: &out.summary, ExitCode: out.exit, FinishedAt: &now,
		Outcome: storedOutcome(out.out),
	})
	if err != nil {
		slog.Warn("ade: record rebase outcome", "scope", "ade", "run", runID, "err", err)
		return
	}
	b.emitRuns(updated)
	if out.state == model.AdeRunDone && out.out.Rebase != nil && out.out.Rebase.Verified {
		ids := make([]string, len(spec.Stack))
		for i, st := range spec.Stack {
			ids[i] = st.BranchID
		}
		if err := b.deps.Tasks.ClearBasePending(ids); err != nil {
			slog.Warn("ade: clear pending base", "scope", "ade", "run", runID, "err", err)
		}
	}
	b.releaseRebaseGate(cur.TaskID, spec)
	b.notifyBoard()
}

// releaseRebaseGate launches the step runs that waited for the rebase.
func (b *TaskBoard) releaseRebaseGate(taskID string, spec model.AdeRebaseSpec) {
	for _, st := range spec.Stack {
		sb, err := b.deps.Tasks.GetBranch(st.BranchID)
		if err != nil {
			continue
		}
		if sb.TaskID == taskID {
			b.launchHeldLocked(sb)
			continue
		}
		b.goTracked(func() {
			mu := b.taskMu(sb.TaskID)
			mu.Lock()
			defer mu.Unlock()
			b.launchHeldLocked(sb)
		})
	}
}

// applyRebaseFinish applies a finish_step call of a taken-over rebase run: git decides again. The
// task mutex is held.
func (b *TaskBoard) applyRebaseFinish(run model.AdeRun, f agentmcp.Finish) {
	spec, err := specOf(run)
	if err != nil {
		slog.Warn("ade: rebase spec", "scope", "ade", "run", run.ID, "err", err)
		return
	}
	if latest, err := b.deps.Tasks.LatestRebaseRun(run.BranchID); err != nil || latest == nil || latest.ID != run.ID {
		return
	}
	vctx, cancel := context.WithTimeout(b.ctx, verifyTimeout)
	defer cancel()
	chk := b.verifyRebase(vctx, spec)
	end := outcome{}
	if run.Outcome != nil {
		end.out = *run.Outcome
	}
	b.settleRebase(run.ID, "", spec, decideRebaseOutcome(&f, chk, end), true)
}

// AbortRebase aborts the operation left in progress in a branch worktree. The base stays changed:
// the branch keeps showing the pending rebase until a rebase completes or the base is changed back.
func (b *TaskBoard) AbortRebase(ctx context.Context, args adewire.BranchArgs) error {
	sb, err := b.deps.Tasks.GetBranch(args.BranchID)
	if err != nil {
		return err
	}
	mu := b.taskMu(sb.TaskID)
	mu.Lock()
	defer mu.Unlock()
	if sb.Name == "" {
		return invalid("the branch is not created yet")
	}
	if r, err := b.deps.Tasks.RunningRebaseOn(sb.ID); err != nil {
		return err
	} else if running, err := b.deps.Tasks.HasRunningOn(sb.ID); err != nil {
		return err
	} else if r != nil || running {
		return invalid("stop the run first")
	}
	wt, err := b.worktreeOf(ctx, sb)
	if err != nil {
		return err
	}
	if wt == "" {
		return invalid("no rebase is in progress in %s: it has no worktree", sb.Name)
	}
	entry, err := b.openRepo(ctx, sb.CodeRepoID)
	if err != nil {
		return err
	}
	status, err := entry.WorktreeStatus(ctx, wt)
	if err != nil {
		return err
	}
	if entry.WorktreeInProgress(wt, status) == nil {
		return invalid("no rebase is in progress in %s", wt)
	}
	if err := entry.WorktreeAbort(ctx, wt); err != nil {
		return invalid("could not abort: %s", err)
	}
	b.recordAbort(ctx, sb)
	b.notifyBoard()
	return nil
}

// recordAbort notes the abort on the branch's latest rebase run and refreshes its git facts.
func (b *TaskBoard) recordAbort(ctx context.Context, sb model.AdeTaskBranch) {
	run, err := b.deps.Tasks.LatestRebaseRun(sb.ID)
	if err != nil || run == nil || run.Outcome == nil {
		return
	}
	sink := b.newLogSink(repos.AdeLogRun, run.ID, run.TaskID)
	sink.add(logEvent, "aborted by you")
	sink.flush()
	out := *run.Outcome
	facts := model.RebaseFacts{}
	if out.Rebase != nil {
		facts = *out.Rebase
	}
	if spec, err := specOf(*run); err == nil {
		vctx, cancel := context.WithTimeout(ctx, verifyTimeout)
		defer cancel()
		facts = b.verifyRebase(vctx, spec).facts
	}
	facts.Aborted, facts.InProgress = true, false
	out.Rebase = &facts
	updated, err := b.deps.Tasks.UpdateRun(run.ID, model.AdeRunPatch{Outcome: &out})
	if err != nil {
		slog.Warn("ade: record abort", "scope", "ade", "run", run.ID, "err", err)
		return
	}
	b.emitRuns(updated)
}

// reverifyRecovered reads git for the rebase runs a restart interrupted, so their facts show what
// was left behind, and releases the step runs held behind them.
func (b *TaskBoard) reverifyRecovered(runs []model.AdeRun) {
	for _, run := range runs {
		if b.ctx.Err() != nil {
			return
		}
		spec, err := specOf(run)
		if err != nil {
			continue
		}
		vctx, cancel := context.WithTimeout(b.ctx, verifyTimeout)
		chk := b.verifyRebase(vctx, spec)
		cancel()
		mu := b.taskMu(run.TaskID)
		mu.Lock()
		if cur, err := b.deps.Tasks.GetRun(run.ID); err == nil && cur.Outcome != nil {
			out := *cur.Outcome
			out.Rebase = &chk.facts
			if updated, err := b.deps.Tasks.UpdateRun(run.ID, model.AdeRunPatch{Outcome: &out}); err == nil {
				b.emitRuns(updated)
			}
		}
		b.releaseRebaseGate(run.TaskID, spec)
		mu.Unlock()
		b.notifyBoard()
	}
}

// rebaseContext is one prompt line per branch of the task whose latest rebase did not finish.
func (b *TaskBoard) rebaseContext(tc *taskCtx) []string {
	var lines []string
	for _, sb := range tc.branches {
		run, err := b.deps.Tasks.LatestRebaseRun(sb.ID)
		if err != nil || run == nil || run.Outcome == nil {
			continue
		}
		var what string
		switch run.Outcome.Status {
		case runoutcome.StatusFailed:
			what = "failed"
		case runoutcome.StatusBlocked:
			what = "needs input"
		case runoutcome.StatusCancelled:
			what = "was stopped"
		default:
			continue
		}
		lines = append(lines, llmprompts.AdeRebaseContextLine.Text(b.prompts(), llmprompts.RebaseContextVars{
			Branch: sb.Name, Outcome: what, Reason: run.Outcome.Reason, RunID: run.ID,
		}))
	}
	return lines
}
