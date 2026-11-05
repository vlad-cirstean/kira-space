package ade

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"os"
	"slices"
	"sync"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitprepare"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/repos"
	"github.com/kirathecat/kira-studio/internal/agentcfg"
	"github.com/kirathecat/kira-studio/internal/agentmcp"
	"github.com/kirathecat/kira-studio/internal/agentsandbox"
	"github.com/kirathecat/kira-studio/internal/harness"
	"github.com/kirathecat/kira-studio/internal/ids"
	"github.com/kirathecat/kira-studio/internal/llmprompts"
	"github.com/kirathecat/kira-studio/internal/loginshell"
	"github.com/kirathecat/kira-studio/internal/runoutcome"
	"github.com/kirathecat/kira-studio/internal/secrets"
	"github.com/kirathecat/kira-studio/packages/theme/src/i18n"
)

// runs.go is the run engine: StartRun, the step machine's side effects (start, retry, advance),
// agent and script processes, and the log, session and step reads. Every transition of a task runs
// under that task's mutex; a process runs outside it and re-enters to record how it ended.

const (
	defaultStepTimeout = 30 * time.Minute
	noteWaitingSetup   = "waiting for worktree setup"
	noteSetupFailed    = "worktree setup failed"
	noteResumed        = "resumed after restart"
	noteRestarted      = "restarted by you"
	noteWorktreeGone   = "worktree missing"
	settingSourcesUser = "user"
	settingSourcesAll  = "user,project,local"
)

// recordFinish is the MCP server's callback; the last call of a run wins and is applied at exit.
func (b *TaskBoard) recordFinish(runID string, f agentmcp.Finish) {
	if b.tuiBound(runID) {
		b.goTracked(func() { b.applyTUIFinish(runID, f) }) // a taken-over run: applied at call time (R14)
		return
	}
	b.runMu.Lock()
	b.finishes[runID] = f
	b.runMu.Unlock()
}

func (b *TaskBoard) takeFinish(runID string) (agentmcp.Finish, bool) {
	b.runMu.Lock()
	defer b.runMu.Unlock()
	f, ok := b.finishes[runID]
	delete(b.finishes, runID)
	return f, ok
}

func (b *TaskBoard) taskMu(id string) *sync.Mutex {
	b.runMu.Lock()
	defer b.runMu.Unlock()
	m, ok := b.taskMus[id]
	if !ok {
		m = &sync.Mutex{}
		b.taskMus[id] = m
	}
	return m
}

// taskCtx is one task with its workflow, position and live branches. wf is nil when the task has no
// workflow or its file is gone and it never started.
type taskCtx struct {
	task model.AdeTask
	wf   *adewire.Workflow
	// entry is the step the task last entered ("done" when finished, "" without a workflow). A task that
	// has not started follows the live file, so its entry is that file's first step.
	entry string
	// since is the pass watermark: runs of "<step>/<branch>" at or below the attempt are history.
	since    map[string]int
	branches []model.AdeTaskBranch
	mine     []mineBranch
	nick     map[string]string // code repo id -> nickname, else name
}

func sinceKey(stepID, branchID string) string { return stepID + "/" + branchID }

// inPass says the run belongs to the task's current pass.
func (tc *taskCtx) inPass(run model.AdeRun) bool {
	return run.Attempt > tc.since[sinceKey(run.StepID, run.BranchID)]
}

// active says the task sits at a step of a workflow it can run.
func (tc *taskCtx) active() bool { return tc.wf != nil && tc.entry != "" && tc.entry != entryDone }

// entryDone is the entry step of a finished task.
const entryDone = "done"

func (tc *taskCtx) branch(id string) (model.AdeTaskBranch, bool) {
	for _, br := range tc.branches {
		if br.ID == id {
			return br, true
		}
	}
	return model.AdeTaskBranch{}, false
}

func (b *TaskBoard) loadTaskCtx(taskID string) (*taskCtx, error) {
	task, err := b.deps.Tasks.GetTask(taskID)
	if err != nil {
		return nil, err
	}
	if task.ArchivedAt != nil {
		return nil, invalid("task %s is archived", taskID)
	}
	tc := &taskCtx{task: task, nick: map[string]string{}, entry: task.EntryStepID, since: map[string]int{}}
	if wf, ok := b.taskWorkflow(task); ok {
		tc.wf = &wf
		if task.WorkflowJSON == "" {
			tc.entry = ""
			if i, ok := firstStep(wf); ok {
				tc.entry = wf.Steps[i].ID
			}
		}
	}
	if task.SinceJSON != "" {
		if err := json.Unmarshal([]byte(task.SinceJSON), &tc.since); err != nil {
			return nil, fmt.Errorf("ade: unreadable pass of task %s: %w", taskID, err)
		}
	}
	live, err := b.deps.Tasks.BranchesLive()
	if err != nil {
		return nil, err
	}
	for _, br := range live {
		if br.TaskID == taskID {
			tc.branches = append(tc.branches, br)
		}
	}
	cfgs, err := b.deps.RepoConfig.List()
	if err != nil {
		return nil, err
	}
	for _, c := range cfgs {
		tc.nick[c.CodeRepoID] = cmpNonEmpty(c.Nickname, c.Name)
	}
	for _, br := range tc.branches {
		if br.Kind == model.AdeBranchKindMine {
			tc.mine = append(tc.mine, mineBranch{ID: br.ID, Nick: tc.nick[br.CodeRepoID]})
		}
	}
	return tc, nil
}

func cmpNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// plan resolves the workflow's steps to targets and the latest runs of the current pass.
func (b *TaskBoard) plan(tc *taskCtx) ([]stepView, error) {
	if tc.wf == nil {
		return nil, invalid("the task has no workflow")
	}
	steps := workflowSteps(*tc.wf)
	latest, err := b.deps.Tasks.LatestRuns(tc.task.ID)
	if err != nil {
		return nil, err
	}
	out := make([]stepView, len(steps))
	for i, def := range steps {
		v := stepView{Def: def, Runs: map[string]model.AdeRun{}}
		switch {
		case def.OutsideRepo && !def.manual():
			v.Targets = []string{taskTarget}
		case !def.manual():
			if v.Targets, v.TargetErr = stepTargets(def.RunsOn, def.Kind == adewire.KindReview, tc.mine); v.TargetErr != nil {
				v.Targets = nil
			}
		}
		for _, r := range latest {
			if r.Purpose == "" && r.StepID == def.ID && tc.inPass(r) {
				v.Runs[r.BranchID] = r
			}
		}
		if _, ok := v.Runs[taskTarget]; ok && !def.manual() { // the step started without a branch and keeps its target
			v.Targets, v.TargetErr = []string{taskTarget}, nil
		}
		out[i] = v
	}
	return out, nil
}

// entryIndex is the index of the task's entry step in plan, -1 when the task is not at a step.
func (tc *taskCtx) entryIndex(plan []stepView) int {
	if !tc.active() {
		return -1
	}
	return stepIndex(plan, tc.entry)
}

// currentStepID is the step the task is at, derived from its runs: "done" when finished, "" when it has
// no workflow position.
func (b *TaskBoard) currentStepID(tc *taskCtx) string {
	if tc.entry == entryDone {
		return entryDone
	}
	if !tc.active() {
		return ""
	}
	plan, err := b.plan(tc)
	if err != nil {
		return tc.entry
	}
	entry := tc.entryIndex(plan)
	if entry < 0 {
		return tc.entry
	}
	cur := currentStep(plan, entry)
	if cur < 0 {
		return entryDone
	}
	return plan[cur].Def.ID
}

// staleStep refuses when the caller named the step it saw and the task is at another one now.
func (b *TaskBoard) staleStep(tc *taskCtx, from string) error {
	if from != "" && b.currentStepID(tc) != from {
		return fmt.Errorf("%w: the task is now at another step", ErrStale)
	}
	return nil
}

// stepNumber is a step's 1-based position among the workflow's automatic (non-manual, non-skipped)
// steps and their count; the prompt's Step n/m.
func stepNumber(plan []stepView, idx int) (n, of int) {
	for i, v := range plan {
		if v.Def.manual() || v.Def.Skip {
			continue
		}
		of++
		if i <= idx {
			n++
		}
	}
	return n, of
}

func stepIndex(plan []stepView, stepID string) int {
	return slices.IndexFunc(plan, func(v stepView) bool { return v.Def.ID == stepID })
}

func (b *TaskBoard) emitRuns(runs ...model.AdeRun) {
	if len(runs) == 0 {
		return
	}
	ev := adewire.RunsChangedEvent{Runs: make([]adewire.Run, 0, len(runs))}
	batch := b.newWireBatch(runs)
	for _, r := range runs {
		if r.Purpose == model.AdeRunPurposeReview { // a round shows through its review run only
			b.emitReview(r.TaskID, r.BranchID)
			continue
		}
		w := batch.wireRun(r)
		if w.Review != nil {
			b.emitReview(r.TaskID, r.BranchID)
		}
		ev.Runs = append(ev.Runs, w)
	}
	if b.deps.OnRuns != nil && len(ev.Runs) > 0 {
		b.deps.OnRuns(ev)
	}
	b.scheduleBoard()
}

func (b *TaskBoard) notifySessions() {
	if b.deps.OnSessions != nil {
		b.deps.OnSessions()
	}
}

// --- StartRun ----------------------------------------------------------------------------------

// StartRun creates the task's missing branches and worktrees, starts their prepare scripts and
// starts the current step, which must be an automatic one waiting for Run. Later steps follow by
// themselves or wait for Approve.
func (b *TaskBoard) StartRun(ctx context.Context, args adewire.StartRunArgs) (adewire.StartRunResult, error) {
	mu := b.taskMu(args.TaskID)
	mu.Lock()
	defer mu.Unlock()
	tc, err := b.loadTaskCtx(args.TaskID)
	if err != nil {
		return adewire.StartRunResult{}, err
	}
	if err := b.staleStep(tc, args.FromStepID); err != nil {
		return adewire.StartRunResult{}, err
	}
	if !tc.active() {
		return adewire.StartRunResult{}, invalid("the task has no current step")
	}
	if err := b.snapshotWorkflow(&tc.task); err != nil {
		return adewire.StartRunResult{}, err
	}
	plan, err := b.plan(tc)
	if err != nil {
		return adewire.StartRunResult{}, err
	}
	entry := tc.entryIndex(plan)
	if entry < 0 {
		return adewire.StartRunResult{}, invalid("step %q is no longer in the workflow", tc.entry)
	}
	cur := currentStep(plan, entry)
	if cur < 0 || plan[cur].Def.manual() || plan[cur].started() || nextAction(plan, entry).Kind != actIdle {
		return adewire.StartRunResult{}, invalid("the current step does not wait for Run")
	}
	if path, _ := walkPath(plan, entry); path[0] != cur {
		return adewire.StartRunResult{}, invalid("the current step does not wait for Run")
	}
	if err := b.checkBranchNames(tc, args.BranchNames); err != nil {
		return adewire.StartRunResult{}, err
	}
	if plan[cur].TargetErr != nil {
		return adewire.StartRunResult{}, invalid("step %q: %v", plan[cur].Def.Name, plan[cur].TargetErr)
	}
	paths, err := b.prepareStepWorktrees(ctx, tc, plan[cur].Def, args.BranchNames)
	if err != nil {
		return adewire.StartRunResult{}, err
	}
	if tc, err = b.loadTaskCtx(args.TaskID); err != nil { // branch names changed
		return adewire.StartRunResult{}, err
	}
	b.setStepMessage(tc.task.ID, plan[cur].Def.ID, args.Message)
	runs, err := b.startStep(ctx, tc, plan, cur, paths)
	if err != nil {
		return adewire.StartRunResult{}, err
	}
	res := adewire.StartRunResult{RunIDs: make([]string, len(runs))}
	for i, r := range runs {
		res.RunIDs[i] = r.ID
	}
	return res, nil
}

func (b *TaskBoard) checkBranchNames(tc *taskCtx, names map[string]string) error {
	for id := range names {
		br, ok := tc.branch(id)
		if !ok || br.Kind != model.AdeBranchKindMine || br.Name != "" {
			return invalid("branchNames: %s is not a not-yet-created branch of this task", id)
		}
	}
	return nil
}

// prepareStepWorktrees prepares the worktrees a step starts in. An outside-repo step uses none, so it
// prepares only the branches the user named in the Run dialog.
func (b *TaskBoard) prepareStepWorktrees(ctx context.Context, tc *taskCtx, def stepDef, names map[string]string) (map[string]string, error) {
	if def.OutsideRepo && len(names) == 0 {
		return map[string]string{}, nil
	}
	return b.prepareWorktrees(ctx, tc, names, def.Kind == adewire.KindAgent || def.SmartScript != "")
}

// prepareWorktrees makes every mine branch's worktree exist and starts a setup in each new one. When the
// step is run by an agent, an unnamed branch the user did not name gets none: the run starts in the repo
// root and the agent asks for the branch through request_branches; Kira Space names it. Any other step
// cannot ask, so its unnamed branch takes a name derived from the task title.
func (b *TaskBoard) prepareWorktrees(ctx context.Context, tc *taskCtx, names map[string]string, agentStep bool) (map[string]string, error) {
	paths := map[string]string{}
	title := taskTitle(tc.task, tc.branches)
	for _, sb := range tc.branches {
		if sb.Kind != model.AdeBranchKindMine {
			continue
		}
		rec, err := b.deps.CodeRepos.Get(sb.CodeRepoID)
		if err != nil {
			return nil, err
		}
		if rec == nil {
			return nil, invalid("code repo %s not found", sb.CodeRepoID)
		}
		if agentStep && sb.Name == "" && names[sb.ID] == "" {
			paths[sb.ID] = rec.Root // the agent creates the branch via request_branches
			continue
		}
		res, err := b.ensureWorktree(ctx, *rec, sb, newBranchName{title: title, wanted: names[sb.ID]})
		if err != nil {
			return nil, err
		}
		paths[sb.ID] = res.Path
		if res.Fresh {
			if err := b.startSetup(*rec, sb, res.Name, res.Path, b.onSetupReady); err != nil {
				return nil, err
			}
		}
	}
	return paths, nil
}

func stepMessageKey(taskID, stepID string) string {
	return taskID + "|" + stepID
}

// setStepMessage keeps the Run dialog's edited text for the step's runs, retries included. It lives
// in memory: after a restart a retry uses the composed default.
func (b *TaskBoard) setStepMessage(taskID, stepID, message string) {
	b.runMu.Lock()
	defer b.runMu.Unlock()
	if message == "" {
		delete(b.stepMsgs, stepMessageKey(taskID, stepID))
		return
	}
	b.stepMsgs[stepMessageKey(taskID, stepID)] = message
}

func (b *TaskBoard) stepMessage(taskID, stepID string) string {
	b.runMu.Lock()
	defer b.runMu.Unlock()
	return b.stepMsgs[stepMessageKey(taskID, stepID)]
}

// startStep creates attempt-1 runs for every target of plan[idx] and launches those whose worktree
// gate is open.
func (b *TaskBoard) startStep(ctx context.Context, tc *taskCtx, plan []stepView, idx int, paths map[string]string) ([]model.AdeRun, error) {
	if err := plan[idx].TargetErr; err != nil {
		return nil, invalid("step %q: %v", plan[idx].Def.Name, err)
	}
	out := make([]model.AdeRun, 0, len(plan[idx].Targets))
	for _, bid := range plan[idx].Targets {
		run, err := b.queueRun(ctx, tc, plan, idx, bid, 1, paths[bid], runOpts{})
		if err != nil {
			return out, err
		}
		out = append(out, run)
	}
	return out, nil
}

// queueRun inserts a run attempt and launches it, or leaves it pending behind the worktree gate.
func (b *TaskBoard) queueRun(ctx context.Context, tc *taskCtx, plan []stepView, idx int, branchID string, attempt int, path string, opts runOpts) (model.AdeRun, error) {
	if err := b.checkNotArchiving(tc.task.ID); err != nil {
		return model.AdeRun{}, err
	}
	var sb model.AdeTaskBranch
	if branchID == taskTarget {
		sb.TaskID = tc.task.ID
		if path == "" {
			var err error
			if path, err = b.taskDir(tc.task.ID); err != nil {
				return model.AdeRun{}, err
			}
		}
	} else {
		var ok bool
		if sb, ok = tc.branch(branchID); !ok {
			return model.AdeRun{}, invalid("branch %s is not on this task", branchID)
		}
		if path == "" {
			var err error
			if path, err = b.worktreeOf(ctx, sb); err != nil {
				return model.AdeRun{}, err
			}
		}
	}
	setups, err := b.deps.Tasks.SetupByBranch()
	if err != nil {
		return model.AdeRun{}, err
	}
	if last, ok := b.lastRunOf(tc.task.ID, plan[idx].Def.ID, branchID); ok && attempt <= last.Attempt {
		attempt = last.Attempt + 1 // a pass after a move starts above its history, which the watermark hides
	}
	run := model.AdeRun{
		ID: ids.NewID(), TaskID: tc.task.ID, StepID: plan[idx].Def.ID, BranchID: branchID,
		Attempt: attempt, State: model.AdeRunPending, Loops: opts.Loops, Launch: opts.Launch,
	}
	open := path != "" && (branchID == taskTarget || b.setupReady(sb, path, setups))
	if !open {
		run.Note = noteWaitingSetup
		switch s, has := setups[branchID]; {
		case has && s.State == model.AdeSetupFailed:
			run.Note = noteSetupFailed
		case !has || s.State == model.AdeSetupReady:
			run.Note = noteWorktreeGone
		}
	}
	if err := b.deps.Tasks.InsertRun(run); err != nil {
		return model.AdeRun{}, err
	}
	b.syncStatus(tc.task.ID)
	if !open {
		b.emitRuns(run)
		return run, nil
	}
	return b.launch(ctx, tc, plan, idx, run, sb, path)
}

func (b *TaskBoard) runVarsFor(tc *taskCtx, sb model.AdeTaskBranch, path string) runVars {
	return runVars{
		Task: taskTitle(tc.task, tc.branches), Jira: tc.task.JiraKey, Repo: tc.nick[sb.CodeRepoID],
		Branch: sb.Name, Worktree: path,
	}
}

// launch marks the run running and starts its process in the background. A failure other than an
// archive or shutdown leaves the run failed with the error as note, never a note-less pending row.
func (b *TaskBoard) launch(ctx context.Context, tc *taskCtx, plan []stepView, idx int, run model.AdeRun, sb model.AdeTaskBranch, path string) (model.AdeRun, error) {
	if err := b.checkNotArchiving(run.TaskID); err != nil {
		return run, err
	}
	if r, err := b.deps.Tasks.RunningRebaseOn(sb.ID); err != nil {
		return run, err
	} else if r != nil { // a rebase is moving this worktree: the run waits for it
		note := noteWaitingRebase
		updated, err := b.deps.Tasks.UpdateRun(run.ID, model.AdeRunPatch{Note: &note})
		if err != nil {
			return run, err
		}
		b.emitRuns(updated)
		return updated, nil
	}
	if name := b.claimOn(sb.ID); name != "" { // a smart script works in this worktree
		note := noteWaitingAutomation + name
		if run.Note == note {
			return run, nil
		}
		updated, err := b.deps.Tasks.UpdateRun(run.ID, model.AdeRunPatch{Note: &note})
		if err != nil {
			return run, err
		}
		b.emitRuns(updated)
		return updated, nil
	}
	out, err := b.startRun(ctx, tc, plan, idx, run, sb, path)
	if err != nil && !errors.Is(err, errBoardClosed) {
		b.failLaunch(run, err)
	}
	return out, err
}

// failLaunch records a run that could not start as failed.
func (b *TaskBoard) failLaunch(run model.AdeRun, cause error) {
	now := b.deps.Now().UnixMilli()
	state, note := model.AdeRunFailed, "could not start: "+trimInvalid(cause)
	updated, err := b.deps.Tasks.UpdateRun(run.ID, model.AdeRunPatch{State: &state, Note: &note, FinishedAt: &now})
	if err != nil {
		slog.Warn("ade: record launch failure", "scope", "ade", "run", run.ID, "err", err)
		return
	}
	b.emitRuns(updated)
}

func (b *TaskBoard) startRun(ctx context.Context, tc *taskCtx, plan []stepView, idx int, run model.AdeRun, sb model.AdeTaskBranch, path string) (model.AdeRun, error) {
	if path == "" {
		return run, invalid("worktree of %s is missing", sb.Name)
	}
	def := plan[idx].Def
	if def.Kind == adewire.KindReview {
		return b.enterReview(ctx, tc, def, run, sb)
	}
	rec, err := b.stepRepo(sb)
	if err != nil {
		return run, err
	}
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
	spec := run.Launch
	note := spec.Note
	running := model.AdeRunRunning
	patch := model.AdeRunPatch{State: &running, StartedAt: &now, Note: &note, Launch: &model.AdeRunLaunch{}}
	vars := b.runVarsFor(tc, sb, path)
	vars.Outside = def.OutsideRepo

	var smart *smartStep
	runPath := path
	if def.Kind == adewire.KindAgent && def.SmartScript != "" {
		if smart, err = b.planSmartStep(tc, sb, path, def); err != nil {
			return run, err
		}
		runPath = smart.dir
	}
	var sessionID, prompt string
	var agent harness.Provider
	stepN, stepOf := stepNumber(plan, idx)
	if def.Kind == adewire.KindAgent {
		sessionID = ids.NewID()
		patch.SessionID = &sessionID
		agent, err = b.agentForStep(spec.ResumeID, def.Agent)
		if err != nil {
			return run, err
		}
		agentSessionID := b.proposeAgentID(agent, spec.ResumeID)
		switch {
		case spec.ResumeID != "":
			prompt = composeResumePrompt(b.prompts(), spec.Prompt, def.Results)
		case smart != nil:
			prompt = smart.prompt
		default:
			ho := b.handoverFor(tc, def.ID)
			prompt = composePrompt(b.prompts(), promptInput{
				Title: vars.Task, JiraKey: tc.task.JiraKey, JiraURL: tc.task.JiraURL, Vars: vars,
				Step: stepN, Of: stepOf, Def: def, Message: b.stepMessage(tc.task.ID, def.ID), Extra: spec.Extra,
				Context: b.rebaseContext(tc),
				Notes:   tc.task.Notes, Handover: ho.Body, HandoverFrom: ho.From,
			})
		}
		if err := b.deps.Sessions.InsertHeadless(model.AdeSession{
			ID: sessionID, Mode: model.AdeSessionModeHeadless, TaskID: tc.task.ID, BranchID: sb.ID,
			StepID: def.ID, RunID: run.ID, Resumes: spec.ResumeID, AgentSessionID: agentSessionID, Cwd: runPath, Provider: agent.ID(),
			State: model.AdeSessionStateRunning, StartedAt: now, LastActiveAt: now,
		}); err != nil {
			return run, err
		}
		b.notifySessions()
		kind := opRun
		if spec.ResumeID != "" {
			kind = opResume
		}
		op := b.startSessionOp(ctx, kind, tc.task.ID, agent.Label(), "")
		b.sessionOps.put(sessionID, op)
		b.cancelRunOp(op, run.ID)
		defer func() {
			if !started {
				b.finishRunOp(sessionID, outcome{state: model.AdeRunFailed, note: "The run did not start."})
			}
		}()
	}
	updated, err := b.deps.Tasks.UpdateRun(run.ID, patch)
	if err != nil {
		return run, err
	}
	b.emitRuns(updated)

	timeout := parseStepTimeout(def.Timeout)
	if smart != nil {
		timeout = smart.timeout
	}
	runCtx, endRun := b.beginLive(b.live, run.ID)
	started = true
	if def.Kind == adewire.KindAgent {
		go func() {
			defer b.wg.Done()
			defer endRun()
			b.superviseAgent(runCtx, updated, agentLaunch{def: def, sessionID: sessionID, resume: spec.ResumeID, path: runPath, worktree: path, prompt: prompt, timeout: timeout, smart: smart, agent: agent})
		}()
	} else {
		go func() {
			defer b.wg.Done()
			defer endRun()
			b.superviseScript(runCtx, updated, def, *rec, sb, path, substitute(def.Prompt, vars, true), timeout)
		}()
	}
	return updated, nil
}

// stepRepo is the code repo a step on sb runs in; empty for the task folder target.
func (b *TaskBoard) stepRepo(sb model.AdeTaskBranch) (*model.CodeRepo, error) {
	if sb.ID == taskTarget {
		return &model.CodeRepo{}, nil
	}
	rec, err := b.deps.CodeRepos.Get(sb.CodeRepoID)
	if err != nil {
		return nil, err
	}
	if rec == nil {
		return nil, invalid("code repo %s not found", sb.CodeRepoID)
	}
	return rec, nil
}

func parseStepTimeout(s string) time.Duration {
	if d, err := time.ParseDuration(s); err == nil && d > 0 {
		return d
	}
	return defaultStepTimeout
}

// outcome is how a run's process ended.
type outcome struct {
	state, note, summary string
	exit                 *int
	noAdvance            bool // a user stop: the engine does not move the task on
	lost                 bool // a resume whose agent conversation is gone
	out                  model.AdeRunOutcome
}

// fromOutcome builds the engine outcome for state from the shared one; the run note is its reason.
func fromOutcome(state string, o runoutcome.Outcome) outcome {
	return outcome{state: state, note: o.Reason, summary: o.Summary, exit: o.ExitCode, out: model.AdeRunOutcome{Outcome: o}}
}

// agentEnd is how an agent process ended, for the no-report outcomes.
func agentEnd(agent runoutcome.Agent, exit harness.Exit, runErr error, timeout string) runoutcome.Process {
	p := runoutcome.Process{Agent: agent, Kind: runoutcome.KindAgent, ExitCode: exit.Code, Timeout: timeout}
	switch {
	case runErr != nil:
		p.End, p.Err = runoutcome.EndStartErr, runErr
	case exit.TimedOut:
		p.End = runoutcome.EndTimeout
	}
	return p
}

// hermetic is the agent.hermetic setting for one launch; on without a reader.
func (b *TaskBoard) hermetic() bool { return b.deps.Hermetic == nil || b.deps.Hermetic() }

func (b *TaskBoard) configScope() harness.ConfigScope {
	if b.deps.HeadlessSettingSources != nil && b.deps.HeadlessSettingSources() == "user" {
		return harness.ConfigUser
	}
	return harness.ConfigAll
}

func allowedTools(p harness.MCPHost, step []string, space bool) []string {
	out := slices.Clone(step)
	want := harness.ToolNames(p, agentmcp.FinishStep, agentmcp.RunOutcome)
	if space {
		want = append(want, harness.ToolNames(p, agentmcp.SpaceToolIDs...)...)
	}
	for _, t := range want {
		if !slices.Contains(out, t) {
			out = append(out, t)
		}
	}
	return out
}

// proposeAgentID is the conversation id a headless row starts with: the resumed one, a fresh one for
// an agent that takes the id Kira proposes, "" for one that assigns its own (OnSessionID fills it).
func (b *TaskBoard) proposeAgentID(agent harness.Provider, resume string) string {
	if resume != "" {
		return resume
	}
	if _, ok := agent.(harness.SessionLocator); ok {
		return ""
	}
	return ids.NewID()
}

// agentLaunch is what superviseAgent runs: a prompt, or a smart script's step (smart != nil).
type agentLaunch struct {
	def               stepDef
	sessionID, resume string
	path, prompt      string
	// worktree is the task branch's worktree; path differs for a smart step that runs in its own folder.
	worktree string
	timeout  time.Duration
	smart    *smartStep
	// git, when set, makes the run a sandboxed git agent: only the listed built-ins, no settings files,
	// the worktrees added as directories and the git tool granted.
	git *agentmcp.GitGrant
	// review, when set, makes the run a review round: it may answer those comments.
	review *agentmcp.ReviewGrant
	// agent is the provider the conversation runs under.
	agent harness.Provider
	// contextFile is the agent context path of this run, "" for none; set by superviseAgent.
	contextFile string
	// onSessionID, when set, also receives the id a self-assigning agent reports.
	onSessionID func(id string)
}

// agentFor is the provider a launch runs under: the one that ran the conversation it continues
// (id), or the active one for a new conversation (id "").
func (b *TaskBoard) agentFor(id string) (harness.Provider, error) {
	if id == "" {
		return b.deps.Agent.Active(), nil
	}
	return b.deps.Agent.For(id)
}

// agentForStep is the provider of a headless launch: the one of the conversation it continues, else
// the step's agent, else the active one.
func (b *TaskBoard) agentForStep(resumeID, stepAgent string) (harness.Provider, error) {
	if resumeID == "" && stepAgent != "" {
		return b.agentFor(stepAgent)
	}
	return b.agentForResume(resumeID)
}

// agentForResume is agentFor over the provider recorded with the session a resume continues.
func (b *TaskBoard) agentForResume(resumeID string) (harness.Provider, error) {
	if resumeID == "" {
		return b.deps.Agent.Active(), nil
	}
	id, err := b.deps.Sessions.ProviderOfAgentSession(resumeID)
	if err != nil {
		return nil, err
	}
	return b.agentFor(id)
}

// grantResults are the results finish_step takes for a run: a rebase run or review round keeps the implicit pair.
func grantResults(implicit bool, def stepDef) []agentmcp.ResultSpec {
	if implicit {
		return nil
	}
	return resultSpecs(def.Results)
}

func (b *TaskBoard) superviseAgent(ctx context.Context, run model.AdeRun, l agentLaunch) {
	def, sessionID := l.def, l.sessionID
	sink := b.newLogSink(repos.AdeLogRun, run.ID, run.TaskID)
	rebase := run.Purpose == model.AdeRunPurposeRebase
	round := run.Purpose == model.AdeRunPurposeReview
	space := !rebase && !round
	complete := func(out outcome) {
		if round {
			b.completeReviewRound(run, sessionID, out)
			return
		}
		b.completeRun(run, sessionID, out)
	}
	fail := func(err error) {
		sink.add(logStderr, "could not start: "+err.Error())
		sink.flush()
		end := fromOutcome(model.AdeRunFailed, runoutcome.ForProcess(runoutcome.Process{Kind: runoutcome.KindAgent, End: runoutcome.EndStartErr, Err: err}))
		if rebase {
			b.completeRebase(run, sessionID, nil, end)
			return
		}
		complete(end)
	}
	headless, ok := harness.HeadlessOf(l.agent)
	if !ok {
		fail(&harness.MissingRole{Agent: l.agent.Label(), Role: "headless runs"})
		return
	}
	if l.smart != nil {
		if err := l.smart.checkMCP(ctx, b.deps.Servers, l.agent, b.deps.ScriptsHome); err != nil {
			fail(err)
			return
		}
	}
	if b.deps.ContextFile != nil {
		var err error
		if l.contextFile, err = b.deps.ContextFile(); err != nil {
			fail(err)
			return
		}
	}
	sbx, err := b.prepareSandbox(ctx, run, l)
	if err != nil {
		fail(err)
		return
	}
	defer b.endSandbox(run.ID, sbx)
	var grantSandbox *agentmcp.SandboxGrant
	if sbx != nil {
		grantSandbox = sbx.grant
	}
	cfg, release, err := b.agent.Register(agentmcp.Grant{RunID: run.ID, TaskID: run.TaskID, SessionID: l.sessionID, Space: space, Git: l.git, Review: l.review, Sandbox: grantSandbox, Agent: l.agent, Results: grantResults(rebase || round, def)})
	if err != nil {
		fail(err)
		return
	}
	prep, err := b.headlessApp(ctx, l, run.ID)
	if err != nil {
		release()
		fail(err)
		return
	}
	defer prep.Release()
	ticked, err := b.wireTicked(ctx, l, run.ID)
	if err != nil {
		release()
		fail(err)
		return
	}
	defer ticked.Release()
	masked := append(slices.Clone(prep.Secrets), ticked.Secrets...)
	if l.smart != nil {
		masked = append(masked, l.smart.secrets...)
	}
	sink.setMask(secrets.NewMasker(masked...))
	agent := l.agent
	spec := harness.HeadlessSpec{
		Bin: b.deps.AgentBin, Dir: l.path, Prompt: l.prompt, SessionID: b.sessionAgentID(sessionID),
		Resume: l.resume, MCPConfigs: []string{cfg}, Config: b.configScope(), AllowedTools: roundTools(agent, allowedTools(agent, def.AllowedTools, space), round), DisallowedTools: def.DisallowedTools,
		Timeout: l.timeout, Stage: stageValues(def.Settings, agent), ContextFile: l.contextFile, App: prep.App,
		Hermetic: b.hermetic() && len(ticked.UserMCP) == 0,
	}
	spec.Env = gitprepare.BuildEnv(withAppEnv(os.Environ(), prep.App), gitprepare.Vars{WorktreePath: l.path})
	if rebase {
		spec.Env = append(spec.Env, "GIT_EDITOR=true")
	}
	if l.git != nil {
		spec.Isolated, spec.Tools, spec.AddDirs = true, rebaseBuiltins, l.git.Worktrees
	}
	if l.smart != nil {
		spec = withTicked(l.smart.spec(agent, spec, space), ticked)
	}
	spec = b.applySandbox(spec, sbx, l)
	b.noticeSandbox(sink, sbx)
	var res *harness.Result
	var stopWatch func()
	onLimits := b.deps.OnRateLimits
	if !harness.ReportsUsage(agent) {
		onLimits = nil
	}
	exit, runErr := headless.RunHeadless(ctx, spec, harness.Handler{
		OnLine:       func(l harness.Line) { sink.add(l.Stream, l.Text) },
		OnRateLimits: onLimits,
		OnResult:     func(r harness.Result) { res = &r },
		OnStart: func(pid int) {
			if sbx.on() {
				stopWatch = b.watchSandbox(run, sbx, sink, pid)
			}
		},
		OnSessionID: func(id string) { b.learnAgentSessionID(sessionID, id, l.onSessionID) },
	})
	if stopWatch != nil {
		stopWatch()
	}
	sink.flush()
	release()
	if b.ctx.Err() != nil {
		return // app quit: rows stay running (R18)
	}
	b.endAgentRun(ctx, run, l, runEnd{exit: exit, err: runErr, res: res, sink: sink, sbx: sbx, complete: complete, headless: headless})
}

// withTicked adds the ticked servers' wiring to a smart step's spec.
func withTicked(spec harness.HeadlessSpec, ticked agentcfg.Ticked) harness.HeadlessSpec {
	if len(ticked.UserMCP) > 0 {
		spec.UserMCP, spec.GrantMCP = ticked.UserMCP, []string{agentmcp.ServerName}
	}
	if ticked.MCPConfig != "" {
		spec.MCPConfigs = append(spec.MCPConfigs, ticked.MCPConfig)
	}
	return spec
}

// wireTicked wires the servers a smart step ticked; none for any other run.
func (b *TaskBoard) wireTicked(ctx context.Context, l agentLaunch, runID string) (agentcfg.Ticked, error) {
	if l.smart == nil {
		return agentcfg.Ticked{Release: func() {}}, nil
	}
	return b.deps.Servers.Wire(ctx, agentcfg.Launch{Provider: l.agent, Kind: harness.LaunchHeadless, Dir: b.deps.AgentDir, Stem: runID}, l.smart.mcpNames())
}

// headlessApp prepares the app's agent settings for a run; an isolated run (rebase, smart step) takes
// none, its tools and MCP servers are explicit.
func (b *TaskBoard) headlessApp(ctx context.Context, l agentLaunch, runID string) (agentcfg.Prepared, error) {
	if l.git != nil || l.smart != nil {
		return agentcfg.Prepared{Release: func() {}}, nil
	}
	return b.prepareApp(ctx, agentcfg.Launch{Provider: l.agent, Kind: harness.LaunchHeadless, Dir: b.deps.AgentDir, Stem: runID})
}

// prepareApp prepares the app's agent settings for one launch; none without a preparer.
func (b *TaskBoard) prepareApp(ctx context.Context, l agentcfg.Launch) (agentcfg.Prepared, error) {
	if b.deps.AgentConfig == nil {
		return agentcfg.Prepared{Release: func() {}}, nil
	}
	return b.deps.AgentConfig(ctx, l)
}

// withAppEnv puts the app settings' env after the process environment, so it overrides it and the
// variables Kira adds afterwards override it.
func withAppEnv(environ []string, app *harness.AppConfig) []string {
	if app == nil {
		return environ
	}
	return append(slices.Clone(environ), app.Env...)
}

// runEnd is what a headless agent run left behind.
type runEnd struct {
	exit     harness.Exit
	err      error
	res      *harness.Result
	sink     *logSink
	sbx      *sandboxRun
	complete func(outcome)
	headless harness.Headless
}

// endAgentRun maps how an agent process ended to the run's outcome and completes the run.
func (b *TaskBoard) endAgentRun(ctx context.Context, run model.AdeRun, l agentLaunch, e runEnd) {
	def, sessionID, agent := l.def, l.sessionID, l.agent
	exit, runErr, res, sink, sbx, complete := e.exit, e.err, e.res, e.sink, e.sbx, e.complete
	rebase := run.Purpose == model.AdeRunPurposeRebase
	if out, ok := stopOutcome(ctx); ok {
		b.takeFinish(run.ID)
		code := exit.Code
		out.exit, out.out.ExitCode = &code, &code
		if rebase {
			b.completeRebase(run, sessionID, nil, out)
			return
		}
		complete(out)
		return
	}
	if rebase {
		an := runoutcome.AgentOf(agent)
		code := exit.Code
		o := runoutcome.ForProcess(agentEnd(an, exit, runErr, def.Timeout)).WithLastError(sink.lastStderr())
		end := sbx.sandboxOutcome(fromOutcome(model.AdeRunFailed, o))
		end.out.ExitCode = &code
		f, ok := b.takeFinish(run.ID)
		var report *agentmcp.Finish
		if ok {
			report = &f
		}
		b.completeRebase(run, sessionID, report, end)
		return
	}
	resumeLost := l.resume != "" && e.headless.ResumeLost(sink.lastStderr())
	an := runoutcome.AgentOf(agent)
	if l.smart != nil {
		b.completeRun(run, sessionID, sbx.sandboxOutcome(b.smartOutcome(an, run.ID, exit, runErr, res, l.smart, def.Results, sink.lastStderr(), resumeLost)))
		return
	}
	complete(sbx.sandboxOutcome(b.agentOutcome(an, run.ID, exit, runErr, def, sink.lastStderr(), resumeLost)))
}

// roundTools adds the reply tool to a review round's allowed tools.
func roundTools(p harness.MCPHost, tools []string, round bool) []string {
	reply := p.MCPToolName(agentmcp.ReviewReply)
	if round && !slices.Contains(tools, reply) {
		return append(tools, reply)
	}
	return tools
}

// learnAgentSessionID stores the id a self-assigning agent reported for the headless row.
func (b *TaskBoard) learnAgentSessionID(sessionID, id string, also func(string)) {
	if err := b.deps.Sessions.SetAgentSessionID(sessionID, id); err != nil {
		slog.Warn("ade: store agent session id", "scope", "ade", "session", sessionID, "err", err)
		return
	}
	b.notifySessions()
	if also != nil {
		also(id)
	}
}

// sessionAgentID reads the agent session id stored with the ade_sessions row; "" for an agent that
// assigns its own.
func (b *TaskBoard) sessionAgentID(sessionID string) string {
	rec, err := b.deps.Sessions.Get(sessionID)
	if err != nil || rec == nil {
		return ids.NewID()
	}
	return rec.AgentSessionID
}

// lostOutcome is the outcome of a resume whose agent conversation is gone.
func lostOutcome(agent runoutcome.Agent, lost bool, lastErr string) (outcome, bool) {
	if !lost {
		return outcome{}, false
	}
	o := runoutcome.ForProcess(runoutcome.Process{Agent: agent, Kind: runoutcome.KindAgent, End: runoutcome.EndResumeLost}).WithLastError(lastErr)
	out := fromOutcome(model.AdeRunFailed, o)
	out.lost = true
	return out, true
}

func (b *TaskBoard) agentOutcome(an runoutcome.Agent, runID string, exit harness.Exit, runErr error, def stepDef, lastErr string, resumeLost bool) outcome {
	code := exit.Code
	f, finished := b.takeFinish(runID)
	if !finished {
		if out, ok := lostOutcome(an, resumeLost, lastErr); ok {
			return out
		}
		o := runoutcome.ForProcess(agentEnd(an, exit, runErr, def.Timeout)).WithLastError(lastErr)
		return fromOutcome(model.AdeRunFailed, o)
	}
	return fromFinish(f, &code, def.Results)
}

// fromFinish is the outcome of a run whose agent called finish_step.
func fromFinish(f agentmcp.Finish, exit *int, results []adewire.StepResult) outcome {
	o := runoutcome.Outcome{Source: runoutcome.SourceAgent, Reported: true, Summary: f.Summary, ExitCode: exit}
	state := finishState(&o, f, results)
	out := fromOutcome(state, o)
	out.out.Report = reportOf(f)
	return out
}

// finishState fills o from a finish_step call and returns the run state: done for an ok result,
// failed for a not-ok one. results are the step's; none = the implicit done and failed.
func finishState(o *runoutcome.Outcome, f agentmcp.Finish, results []adewire.StepResult) string {
	if f.Status == "needs_input" {
		o.Status, o.Reason = runoutcome.StatusBlocked, cmpNonEmpty(f.Reason, f.Summary)
		return model.AdeRunStuck
	}
	o.Result = f.Status
	ok := f.Status == adewire.ResultDone
	if i := slices.IndexFunc(results, func(r adewire.StepResult) bool { return r.ID == f.Status }); i >= 0 {
		ok = results[i].OK
	}
	if ok {
		o.Status = runoutcome.StatusDone
		return model.AdeRunDone
	}
	o.Status, o.Reason = runoutcome.StatusFailed, cmpNonEmpty(f.Reason, cmpNonEmpty(f.Summary, "the agent reported failure without a reason"))
	return model.AdeRunFailed
}

// resultSpecs are a step's results as the MCP grant takes them.
func resultSpecs(results []adewire.StepResult) []agentmcp.ResultSpec {
	out := make([]agentmcp.ResultSpec, len(results))
	for i, r := range results {
		out[i] = agentmcp.ResultSpec{ID: r.ID, OK: r.OK, Description: r.Description}
	}
	return out
}

// reportOf is the detail a finish_step call carried beyond status and summary, nil when none.
func reportOf(f agentmcp.Finish) *model.AgentReport {
	if len(f.ConflictedFiles) == 0 && f.LastGitError == "" && f.Tried == "" {
		return nil
	}
	return &model.AgentReport{ConflictedFiles: f.ConflictedFiles, LastGitError: f.LastGitError, Tried: f.Tried}
}

func (b *TaskBoard) superviseScript(ctx context.Context, run model.AdeRun, def stepDef, rec model.CodeRepo, sb model.AdeTaskBranch, path, command string, timeout time.Duration) {
	sink := b.newLogSink(repos.AdeLogRun, run.ID, run.TaskID)
	sbx, err := b.prepareScriptSandbox(ctx, run, def, sb.Name, path)
	if err != nil {
		sink.add(logStderr, "could not start: "+err.Error())
		sink.flush()
		b.completeRun(run, "", fromOutcome(model.AdeRunFailed, runoutcome.ForProcess(runoutcome.Process{Kind: runoutcome.KindScript, End: runoutcome.EndStartErr, Err: err})))
		return
	}
	defer b.endSandbox(run.ID, sbx)
	shell, login := loginshell.ResolveShell(os.Getenv, loginshell.IsExecutableFile)
	env := gitprepare.BuildEnv(os.Environ(), gitprepare.Vars{WorktreePath: path, WorktreeBranch: sb.Name, RepoRoot: rec.Root})
	spec := gitprepare.Spec{
		Shell: shell, LoginShell: login, Script: command, Dir: path, Timeout: timeout, Env: env,
		OnBatch: func(lines []gitprepare.Line) {
			for _, l := range lines {
				sink.add(l.Stream, l.Text)
			}
		},
	}
	var stopWatch func()
	if sbx.on() {
		spec.Env = agentsandbox.MergeEnv(env, sbx.prep.Env)
		if sbx.prep.Exec != nil {
			spec.Wrap = agentsandbox.ExecArgv(sbx.prep.Exec)
		}
		b.noticeSandbox(sink, sbx)
		spec.OnStart = func(pid int) { stopWatch = b.watchSandbox(run, sbx, sink, pid) }
	}
	res, err := b.scriptRunner().Run(ctx, spec)
	if stopWatch != nil {
		stopWatch()
	}
	sink.flush()
	if b.ctx.Err() != nil {
		return // app quit: rows stay running (R18)
	}
	code := res.ExitCode
	if out, ok := stopOutcome(ctx); ok {
		out.exit, out.out.ExitCode = &code, &code
		b.completeRun(run, "", out)
		return
	}
	p := runoutcome.Process{Kind: runoutcome.KindScript, ExitCode: res.ExitCode, Timeout: def.Timeout}
	switch {
	case err != nil:
		p.End, p.Err = runoutcome.EndStartErr, err
	case res.TimedOut:
		p.End = runoutcome.EndTimeout
	}
	o := runoutcome.ForProcess(p)
	if o.Status == runoutcome.StatusFailed {
		o = o.WithLastError(sink.lastStderr())
	}
	state := model.AdeRunDone
	if o.Status != runoutcome.StatusDone {
		state = model.AdeRunFailed
	}
	b.completeRun(run, "", sbx.sandboxOutcome(fromOutcome(state, o)))
}

// completeRun records how a process ended, then moves the task on.
func (b *TaskBoard) completeRun(run model.AdeRun, sessionID string, out outcome) {
	mu := b.taskMu(run.TaskID)
	mu.Lock()
	defer mu.Unlock()
	b.recordOutcomeLocked(run, sessionID, out)
}

// recordOutcomeLocked stores a run's outcome and acts on it: a finished step is routed on its result
// (route.go), any other outcome moves the task on unless it was a user stop. The task mutex is held.
func (b *TaskBoard) recordOutcomeLocked(run model.AdeRun, sessionID string, out outcome) {
	now := b.deps.Now().UnixMilli()
	b.finishRunOp(sessionID, out)
	if sessionID != "" {
		if err := b.deps.Sessions.MarkStopped(sessionID, now); err != nil {
			slog.Warn("ade: stop headless session", "scope", "ade", "session", sessionID, "err", err)
		}
		b.notifySessions()
	}
	var loop *pendingLoop
	if (out.state == model.AdeRunFailed || out.state == model.AdeRunDone) && !out.noAdvance {
		loop, out = b.decideRoute(run, out)
	}
	updated, err := b.deps.Tasks.UpdateRun(run.ID, model.AdeRunPatch{
		State: &out.state, Note: &out.note, Summary: &out.summary, ExitCode: out.exit, FinishedAt: &now,
		Outcome: storedOutcome(out.out),
	})
	if err != nil {
		slog.Warn("ade: record run outcome", "scope", "ade", "run", run.ID, "err", err)
		return
	}
	b.emitRuns(updated)
	b.recordRunHandoverLocked(updated)
	switch {
	case loop != nil:
		loop.queue(b)
	case !out.noAdvance:
		b.advanceLocked(b.ctx, run.TaskID)
	}
}

// advanceLocked applies the step machine's next action; the task mutex is held.
func (b *TaskBoard) advanceLocked(ctx context.Context, taskID string) {
	defer b.syncStatus(taskID)
	tc, err := b.loadTaskCtx(taskID)
	if err != nil || !tc.active() {
		return
	}
	plan, err := b.plan(tc)
	if err != nil {
		slog.Warn("ade: plan step", "scope", "ade", "task", taskID, "err", err)
		return
	}
	entry := tc.entryIndex(plan)
	if entry < 0 {
		return
	}
	if rr := chainRerun(plan, entry); len(rr) > 0 {
		for _, r := range rr {
			if _, err := b.queueRun(ctx, tc, plan, r.Step, r.Branch, r.Attempt, "", runOpts{Loops: r.Loops}); err != nil {
				slog.Warn("ade: rerun after send-back", "scope", "ade", "branch", r.Branch, "err", err)
			}
		}
		return
	}
	switch act := nextAction(plan, entry); act.Kind {
	case actStart:
		def := plan[act.Step].Def
		paths, err := b.prepareStepWorktrees(ctx, tc, def, nil)
		if err != nil {
			slog.Warn("ade: prepare worktrees", "scope", "ade", "task", taskID, "err", err)
			return
		}
		if tc, err = b.loadTaskCtx(taskID); err != nil { // branch names changed
			return
		}
		if _, err := b.startStep(ctx, tc, plan, act.Step, paths); err != nil {
			slog.Warn("ade: start step", "scope", "ade", "task", taskID, "err", err)
		}
	case actComplete:
		if err := b.deps.Tasks.SetEntryStep(taskID, entryDone, nil, nil); err != nil {
			slog.Warn("ade: finish workflow", "scope", "ade", "task", taskID, "err", err)
			return
		}
		b.notifyBoard()
	}
}

// onSetupReady launches the runs a finished prepare script was holding back.
func (b *TaskBoard) onSetupReady(branchID string) {
	sb, err := b.deps.Tasks.GetBranch(branchID)
	if err != nil {
		return
	}
	mu := b.taskMu(sb.TaskID)
	mu.Lock()
	defer mu.Unlock()
	b.launchHeldLocked(sb)
}

// launchHeldLocked launches the pending runs of a branch whose setup finished; the task mutex is held.
func (b *TaskBoard) launchHeldLocked(sb model.AdeTaskBranch) {
	branchID := sb.ID
	tc, err := b.loadTaskCtx(sb.TaskID)
	if err != nil || !tc.active() {
		return
	}
	plan, err := b.plan(tc)
	if err != nil {
		return
	}
	path, err := b.worktreeOf(b.ctx, sb)
	if err != nil {
		slog.Warn("ade: worktree of ready branch", "scope", "ade", "branch", branchID, "err", err)
		return
	}
	if path == "" {
		b.setPendingNote(branchID, noteWorktreeGone)
		return
	}
	for idx, v := range plan {
		r, ok := v.Runs[branchID]
		if !ok || r.State != model.AdeRunPending {
			continue
		}
		if _, err := b.launch(b.ctx, tc, plan, idx, r, sb, path); err != nil {
			slog.Warn("ade: launch held run", "scope", "ade", "run", r.ID, "err", err)
		}
	}
	b.releaseHeldRounds(tc, sb, path)
}

// --- Approve, RetryRun, StepDone --------------------------------------------------------------

// Approve starts a step that waits for approval.
func (b *TaskBoard) Approve(ctx context.Context, args adewire.StepArgs) error {
	mu := b.taskMu(args.TaskID)
	mu.Lock()
	defer mu.Unlock()
	tc, err := b.loadTaskCtx(args.TaskID)
	if err != nil {
		return err
	}
	plan, err := b.plan(tc)
	if err != nil {
		return err
	}
	idx := stepIndex(plan, args.StepID)
	if idx < 0 {
		return invalid("step %s is not in the workflow", args.StepID)
	}
	entry := tc.entryIndex(plan)
	if entry < 0 {
		return invalid("the task has no current step")
	}
	if act := nextAction(plan, entry); act.Kind != actAwaitApproval || act.Step != idx {
		return invalid("step %q is not waiting for approval", plan[idx].Def.Name)
	}
	_, err = b.startStep(ctx, tc, plan, idx, nil)
	return err
}

// RetryRun starts a new attempt of a failed or stuck run.
func (b *TaskBoard) RetryRun(ctx context.Context, runID string) error {
	run, err := b.deps.Tasks.GetRun(runID)
	if err != nil {
		return err
	}
	mu := b.taskMu(run.TaskID)
	mu.Lock()
	defer mu.Unlock()
	if run.Purpose == model.AdeRunPurposeRebase {
		return invalid("retry from the Rebase button: it shows the prompt first")
	}
	if run.Purpose == model.AdeRunPurposeReview {
		return invalid("%s", i18n.T("backend.space.ade.reviewStep.retryRound.body", nil))
	}
	if run.State == model.AdeRunReview {
		return errReviewWaits()
	}
	tc, err := b.loadTaskCtx(run.TaskID)
	if err != nil {
		return err
	}
	plan, err := b.plan(tc)
	if err != nil {
		return err
	}
	idx := stepIndex(plan, run.StepID)
	if idx < 0 {
		return invalid("the run's step is no longer in the workflow")
	}
	latest, ok := plan[idx].Runs[run.BranchID]
	if !ok || latest.ID != run.ID {
		return invalid("run %s is not the latest attempt", runID)
	}
	if run.State != model.AdeRunFailed && run.State != model.AdeRunStuck {
		return invalid("only a failed or stuck run can be retried")
	}
	opts := runOpts{Loops: run.Loops}
	if run.StartedAt == nil { // stopped before launch: the retry keeps the held resume spec
		opts.Launch = run.Launch
	}
	_, err = b.queueRun(ctx, tc, plan, idx, run.BranchID, run.Attempt+1, "", opts)
	return err
}

// runResumable says a restart stopped the run mid-session, so its agent conversation can continue.
func runResumable(run model.AdeRun) bool {
	return run.Purpose != model.AdeRunPurposeRebase && run.State == model.AdeRunStuck && run.Launch.ResumeID != "" && run.StartedAt != nil
}

// ResumeRun continues the agent session of a run a restart stopped, as a new attempt.
func (b *TaskBoard) ResumeRun(ctx context.Context, runID string) error {
	run, err := b.deps.Tasks.GetRun(runID)
	if err != nil {
		return err
	}
	mu := b.taskMu(run.TaskID)
	mu.Lock()
	defer mu.Unlock()
	run, err = b.deps.Tasks.GetRun(runID)
	if err != nil {
		return err
	}
	if !runResumable(run) {
		return invalid("%s", i18n.T("backend.space.ade.resumeNotResumable.body", nil))
	}
	if run.Purpose == model.AdeRunPurposeReview {
		return b.resumeReviewRound(ctx, run)
	}
	line := llmprompts.RunResumeLine.Text(b.prompts(), llmprompts.ResumeVars{App: appName})
	spec := model.AdeRunLaunch{ResumeID: run.Launch.ResumeID, Prompt: line, Note: noteResumed}
	if _, err := b.queueResumeLocked(ctx, run, spec); err != nil {
		return err
	}
	// Clear only after queueing: a failed queue keeps the stuck run resumable.
	cleared, err := b.deps.Tasks.UpdateRun(run.ID, model.AdeRunPatch{Launch: &model.AdeRunLaunch{}})
	if err != nil {
		return err
	}
	b.emitRuns(cleared)
	return nil
}

// queueResumeLocked queues the next attempt of run's step, continuing spec.ResumeID. taskMu is
// held by the caller.
func (b *TaskBoard) queueResumeLocked(ctx context.Context, run model.AdeRun, spec model.AdeRunLaunch) (model.AdeRun, error) {
	tc, err := b.loadTaskCtx(run.TaskID)
	if err != nil {
		return model.AdeRun{}, err
	}
	plan, err := b.plan(tc)
	if err != nil {
		return model.AdeRun{}, err
	}
	idx := stepIndex(plan, run.StepID)
	if idx < 0 {
		return model.AdeRun{}, invalid("the run's step is no longer in the workflow")
	}
	latest, ok := plan[idx].Runs[run.BranchID]
	if !ok || latest.ID != run.ID {
		return model.AdeRun{}, invalid("run %s is not the latest attempt", run.ID)
	}
	return b.queueRun(ctx, tc, plan, idx, run.BranchID, run.Attempt+1, "", runOpts{Loops: run.Loops, Launch: spec})
}

// restartHeadless stops a running step run and queues its next attempt on the same conversation.
func (b *TaskBoard) restartHeadless(ctx context.Context, rec *model.AdeSession) (adewire.Launch, error) {
	if rec.State != model.AdeSessionStateRunning || rec.RunID == "" {
		return adewire.Launch{}, invalid("the session is not running")
	}
	if rec.AgentSessionID == "" {
		return adewire.Launch{}, invalid("the session has no conversation id yet")
	}
	run, err := b.deps.Tasks.GetRun(rec.RunID)
	if err != nil {
		return adewire.Launch{}, err
	}
	if run.Purpose != "" {
		return adewire.Launch{}, invalid("this run restarts from its own panel")
	}
	if run.State != model.AdeRunRunning {
		return adewire.Launch{}, invalid("the run is not running")
	}
	if err := b.StopRun(ctx, run.ID); err != nil {
		return adewire.Launch{}, err
	}
	mu := b.taskMu(run.TaskID)
	mu.Lock()
	defer mu.Unlock()
	if run, err = b.deps.Tasks.GetRun(run.ID); err != nil {
		return adewire.Launch{}, err
	}
	if run.State != model.AdeRunStuck {
		return adewire.Launch{}, invalid("the run is not running")
	}
	line := llmprompts.RunResumeLine.Text(b.prompts(), llmprompts.ResumeVars{App: appName})
	next, err := b.queueResumeLocked(ctx, run, model.AdeRunLaunch{ResumeID: rec.AgentSessionID, Prompt: line, Note: noteRestarted})
	if err != nil {
		return adewire.Launch{}, err
	}
	return adewire.Launch{SessionID: next.SessionID, Cwd: rec.Cwd}, nil
}

// StepDone ends the current manual step: the task enters the next step, or finishes after the last.
// closeSessions closes the stage sessions of the move first.
func (b *TaskBoard) StepDone(_ context.Context, taskID string, notes *string, closeSessions bool) (adewire.Task, error) {
	if err := checkNotes(notes); err != nil {
		return adewire.Task{}, err
	}
	mu := b.taskMu(taskID)
	mu.Lock()
	defer mu.Unlock()
	tc, err := b.loadTaskCtx(taskID)
	if err != nil {
		return adewire.Task{}, err
	}
	if !tc.active() {
		return adewire.Task{}, invalid("the task has no current step")
	}
	plan, err := b.plan(tc)
	if err != nil {
		return adewire.Task{}, err
	}
	entry := tc.entryIndex(plan)
	if entry < 0 {
		return adewire.Task{}, invalid("step %q is no longer in the workflow", tc.entry)
	}
	cur := currentStep(plan, entry)
	if cur < 0 || !plan[cur].Def.manual() {
		return adewire.Task{}, invalid("the current step is not a manual step")
	}
	if err := b.snapshotWorkflow(&tc.task); err != nil {
		return adewire.Task{}, err
	}
	next := entryDone
	if i, more := nextStep(*tc.wf, cur); more {
		next = tc.wf.Steps[i].ID
	}
	if closeSessions {
		if err := b.closeStageSessionsLocked(taskID, plan[cur].Def.ID, next); err != nil {
			return adewire.Task{}, err
		}
	}
	if err := b.deps.Tasks.SetEntryStep(taskID, next, nil, b.handoffFor(tc, plan[cur].Def.ID, next, notes)); err != nil {
		return adewire.Task{}, err
	}
	b.closeHandoverPrompt(taskID)
	b.syncStatus(taskID)
	b.notifyBoard()
	task, err := b.deps.Tasks.GetTask(taskID)
	if err != nil {
		return adewire.Task{}, err
	}
	return b.wireTask(task)
}

// SetTaskStep moves the task to any non-skipped step of its workflow, or to "done". The target and every
// later step run fresh: their earlier runs stay as history. Refused while a run of the task is live.
// closeSessions closes the stage sessions of the move first.
func (b *TaskBoard) SetTaskStep(_ context.Context, taskID, stepID, fromStepID string, notes *string, closeSessions bool) (adewire.Task, error) {
	if err := checkNotes(notes); err != nil {
		return adewire.Task{}, err
	}
	mu := b.taskMu(taskID)
	mu.Lock()
	defer mu.Unlock()
	tc, err := b.loadTaskCtx(taskID)
	if err != nil {
		return adewire.Task{}, err
	}
	if err := b.staleStep(tc, fromStepID); err != nil {
		return adewire.Task{}, err
	}
	if tc.task.WorkflowID == "" {
		return adewire.Task{}, invalid("the task has no workflow")
	}
	if tc.wf == nil {
		return adewire.Task{}, invalid("workflow %q is not available", tc.task.WorkflowID)
	}
	from := b.currentStepID(tc)
	if stepID == from {
		return b.wireTask(tc.task)
	}
	running, err := b.deps.Tasks.HasRunning(taskID)
	if err != nil {
		return adewire.Task{}, err
	}
	if running {
		return adewire.Task{}, invalid("stop its running agents first")
	}
	target := -1
	if stepID != entryDone {
		if target = slices.IndexFunc(tc.wf.Steps, func(st adewire.WorkflowStep) bool { return st.ID == stepID }); target < 0 {
			return adewire.Task{}, invalid("workflow %q has no step %q", tc.wf.Name, stepID)
		}
		if tc.wf.Steps[target].Skip {
			return adewire.Task{}, invalid("step %q is skipped in workflow %q", stepID, tc.wf.Name)
		}
	}
	if closeSessions {
		if err := b.closeStageSessionsLocked(taskID, from, stepID); err != nil {
			return adewire.Task{}, err
		}
	}
	if err := b.snapshotWorkflow(&tc.task); err != nil {
		return adewire.Task{}, err
	}
	var since map[string]int
	if target >= 0 {
		runs, err := b.deps.Tasks.LatestRuns(taskID)
		if err != nil {
			return adewire.Task{}, err
		}
		since = maps.Clone(tc.since)
		for _, r := range runs {
			if i := slices.IndexFunc(tc.wf.Steps, func(st adewire.WorkflowStep) bool { return st.ID == r.StepID }); r.Purpose == "" && i >= target {
				k := sinceKey(r.StepID, r.BranchID)
				since[k] = max(since[k], r.Attempt)
			}
		}
	}
	if err := b.deps.Tasks.SetEntryStep(taskID, stepID, since, b.handoffFor(tc, from, stepID, notes)); err != nil {
		return adewire.Task{}, err
	}
	b.closeHandoverPrompt(taskID)
	b.syncStatus(taskID)
	b.clearStepMessages(taskID)
	b.notifyBoard()
	task, err := b.deps.Tasks.GetTask(taskID)
	if err != nil {
		return adewire.Task{}, err
	}
	return b.wireTask(task)
}

// RetrySetup reruns a failed prepare script.
func (b *TaskBoard) RetrySetup(ctx context.Context, branchID string) error {
	sb, err := b.deps.Tasks.GetBranch(branchID)
	if err != nil {
		return err
	}
	mu := b.taskMu(sb.TaskID)
	mu.Lock()
	defer mu.Unlock()
	setup, err := b.deps.Tasks.GetSetup(branchID)
	if err != nil {
		return err
	}
	if setup == nil || setup.State != model.AdeSetupFailed {
		return invalid("only a failed worktree setup can be retried")
	}
	if sb.Name == "" {
		return invalid("the branch is not created yet")
	}
	rec, err := b.deps.CodeRepos.Get(sb.CodeRepoID)
	if err != nil {
		return err
	}
	if rec == nil {
		return invalid("code repo %s not found", sb.CodeRepoID)
	}
	path, err := b.worktreeOf(ctx, sb)
	if err != nil {
		return err
	}
	if path == "" {
		res, err := b.ensureWorktree(ctx, *rec, sb, newBranchName{})
		if err != nil {
			return err
		}
		path = res.Path
	}
	if err := b.startSetup(*rec, sb, sb.Name, path, b.onSetupReady); err != nil {
		return err
	}
	if setup, err = b.deps.Tasks.GetSetup(branchID); err == nil && setup != nil && setup.State == model.AdeSetupReady {
		b.launchHeldLocked(sb) // no prepare script: ready at once, nobody else will launch the held runs
		return nil
	}
	if setup != nil && setup.State == model.AdeSetupRunning {
		b.setPendingNote(branchID, noteWaitingSetup)
	}
	return nil
}

// setPendingNote rewrites the note of a branch's pending runs (they wait behind its setup).
func (b *TaskBoard) setPendingNote(branchID, note string) {
	runs, err := b.deps.Tasks.SetPendingNote(branchID, note)
	if err != nil {
		slog.Warn("ade: pending run note", "scope", "ade", "branch", branchID, "err", err)
		return
	}
	b.emitRuns(runs...)
}

// StopRun stops a running run (it becomes stuck, "stopped by you") or a pending one.
func (b *TaskBoard) StopRun(_ context.Context, runID string) error {
	run, err := b.deps.Tasks.GetRun(runID)
	if err != nil {
		return err
	}
	switch run.State {
	case model.AdeRunRunning:
		done := b.cancelLive(b.live, runID, errStopped)
		if done == nil {
			return invalid("run %s has no live process", runID)
		}
		return b.awaitEnd(done)
	case model.AdeRunPending:
		mu := b.taskMu(run.TaskID)
		mu.Lock()
		defer mu.Unlock()
		if run, err = b.deps.Tasks.GetRun(runID); err != nil {
			return err
		}
		if run.State != model.AdeRunPending {
			return invalid("run %s is no longer pending", runID)
		}
		return b.markStopped(run)
	case model.AdeRunReview:
		return errReviewWaits()
	}
	return invalid("only a running or pending run can be stopped")
}

// markStopped makes a run that has no process stuck.
func (b *TaskBoard) markStopped(run model.AdeRun) error {
	now := b.deps.Now().UnixMilli()
	stuck, note := model.AdeRunStuck, errStopped.Error()
	updated, err := b.deps.Tasks.UpdateRun(run.ID, model.AdeRunPatch{State: &stuck, Note: &note, FinishedAt: &now})
	if err != nil {
		return err
	}
	b.emitRuns(updated)
	return nil
}

// --- reads -------------------------------------------------------------------------------------

// ReadLog returns a page of a run or setup log, readable while it is still being written.
func (b *TaskBoard) ReadLog(_ context.Context, args adewire.ReadLogArgs) (adewire.LogPage, error) {
	if args.Kind != repos.AdeLogRun && args.Kind != repos.AdeLogSetup {
		return adewire.LogPage{}, invalid("kind must be run or setup")
	}
	page, err := b.deps.Logs.Page(args.Kind, args.ID, args.AfterSeq)
	if err != nil {
		return adewire.LogPage{}, err
	}
	out := adewire.LogPage{Kind: args.Kind, ID: args.ID, Chunks: toWireChunks(page.Chunks), NextSeq: page.NextSeq, Truncated: page.Truncated}
	if args.Kind == repos.AdeLogRun {
		run, err := b.deps.Tasks.GetRun(args.ID)
		if err != nil {
			return adewire.LogPage{}, err
		}
		out.Done = run.State != model.AdeRunRunning && run.State != model.AdeRunPending
		return out, nil
	}
	setup, err := b.deps.Tasks.GetSetup(args.ID)
	if err != nil {
		return adewire.LogPage{}, err
	}
	out.Done = setup == nil || setup.State != model.AdeSetupRunning
	return out, nil
}

// Sessions lists the v2 task sessions, running first.
func (b *TaskBoard) Sessions(_ context.Context) (adewire.SessionsResult, error) {
	rows, err := b.deps.Sessions.ListTask()
	if err != nil {
		return adewire.SessionsResult{}, err
	}
	out := adewire.SessionsResult{Sessions: make([]adewire.Session, len(rows))}
	for i, s := range rows {
		out.Sessions[i] = toWireSession(s)
	}
	b.badgeSessions(out.Sessions)
	return out, nil
}

func toWireSession(s model.AdeSession) adewire.Session {
	w := adewire.Session{
		ID: s.ID, AgentSessionID: s.AgentSessionID, Provider: s.Provider, Mode: s.Mode, State: s.State, TerminalID: s.TerminalID,
		TaskID: s.TaskID, BranchID: s.BranchID, StepID: s.StepID, RunID: s.RunID, Resumes: s.Resumes, Purpose: s.Purpose,
		Cwd: s.Cwd, StartedAt: s.StartedAt, LastActiveAt: s.LastActiveAt,
	}
	if s.Mode == model.AdeSessionModeHeadless {
		w.Activity = "idle"
		if s.State == model.AdeSessionStateRunning {
			w.Activity = "working"
		}
	}
	if s.State == model.AdeSessionStateStopped {
		_, err := os.Stat(s.Cwd)
		w.CwdMissing = err != nil
	}
	return w
}

func storedOutcome(o model.AdeRunOutcome) *model.AdeRunOutcome {
	if !o.Ended() {
		return nil
	}
	return &o
}
