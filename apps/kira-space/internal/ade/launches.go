package ade

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/oplog"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/agentmcp"
	"github.com/kirathecat/kira-studio/internal/harness"
)

// launches.go starts interactive Claude Code sessions for a task: Take over a run's session,
// the interactive manual step, and a single branch's Start. The terminal itself is mounted by the UI from
// the returned Launch; the tracker records the row when the terminal composes its command.

// tuiBinding ties a TUI record to the agent MCP grant its launch registered; runID is the taken-over
// run the record may finish (R14), "" for a Space-only grant.
type tuiBinding struct {
	runID   string
	release func()
}

// launchGate makes the branch's worktree exist and its setup finish before a session opens (R12) and
// returns the worktree path. The task mutex is held.
func (b *TaskBoard) launchGate(ctx context.Context, tc *taskCtx, sb model.AdeTaskBranch) (string, error) {
	if sb.Name == "" {
		return "", invalid("the branch of %s is not created yet", b.repoNick(tc, sb))
	}
	if err := b.checkNotArchiving(tc.task.ID); err != nil {
		return "", err
	}
	setups, err := b.deps.Tasks.SetupByBranch()
	if err != nil {
		return "", err
	}
	if s, ok := setups[sb.ID]; ok && s.State != model.AdeSetupReady {
		return "", setupGateError(sb, s)
	}
	path, err := b.worktreeOf(ctx, sb)
	if err != nil {
		return "", err
	}
	if path != "" {
		return path, nil
	}
	rec, err := b.deps.CodeRepos.Get(sb.CodeRepoID)
	if err != nil {
		return "", err
	}
	if rec == nil {
		return "", invalid("code repo %s not found", sb.CodeRepoID)
	}
	res, err := b.ensureWorktree(ctx, *rec, sb, newBranchName{title: taskTitle(tc.task, tc.branches)})
	if err != nil {
		return "", err
	}
	if res.Fresh {
		if err := b.startSetup(*rec, sb, res.Name, res.Path, b.onSetupReady); err != nil {
			return "", err
		}
		if s, err := b.deps.Tasks.GetSetup(sb.ID); err != nil {
			return "", err
		} else if s != nil && s.State != model.AdeSetupReady {
			return "", setupGateError(sb, *s)
		}
		// Ready at once (no prepare script): no onReady fires, so release runs held as "worktree missing".
		b.launchHeldLocked(sb)
	}
	return res.Path, nil
}

// ErrSetupPending marks a launch refused because a worktree's prepare script still runs; the bridge
// maps it to E_PREPARING so a dialog can wait for the setup instead of showing an error.
var ErrSetupPending = errors.New("ade: worktree setup pending")

type setupPendingError struct{ msg string }

func (e setupPendingError) Error() string        { return e.msg }
func (e setupPendingError) Is(target error) bool { return target == ErrSetupPending }

// setupGateError is the refusal for a branch whose setup is not ready.
func setupGateError(sb model.AdeTaskBranch, s model.AdeWorktreeSetup) error {
	if s.State == model.AdeSetupRunning {
		return setupPendingError{fmt.Sprintf("preparing the worktree of %s; try again when it is ready", sb.Name)}
	}
	reason := s.Note
	if reason == "" {
		reason = "see its log"
	}
	return invalid("the worktree setup of %s failed: %s", sb.Name, reason)
}

func (b *TaskBoard) repoNick(tc *taskCtx, sb model.AdeTaskBranch) string {
	return tc.nick[sb.CodeRepoID]
}

// runningTUI reports whether a task session terminal is open for the filter.
func (b *TaskBoard) runningTUI(match func(model.AdeSession) bool) (*model.AdeSession, error) {
	rows, err := b.deps.Sessions.ListRunningTUI()
	if err != nil {
		return nil, err
	}
	for i := range rows {
		if match(rows[i]) {
			return &rows[i], nil
		}
	}
	return nil, nil
}

func (b *TaskBoard) tracker() (*Tracker, error) {
	if b.deps.Tracker == nil {
		return nil, invalid("interactive sessions are not available")
	}
	return b.deps.Tracker, nil
}

func launchOf(res PrepareResult) adewire.Launch {
	return adewire.Launch{TerminalID: res.TerminalID, SessionID: res.RecordID, Command: res.Command, Cwd: res.Cwd}
}

// --- Take over -----------------------------------------------------------------------------------

// TakeOver opens a task session in Claude Code: a headless run's conversation continues in a TUI
// (`claude --resume`, same worktree), a stopped TUI record is resumed (R13, R14).
func (b *TaskBoard) TakeOver(ctx context.Context, args adewire.TakeOverArgs) (adewire.Launch, error) {
	tr, err := b.tracker()
	if err != nil {
		return adewire.Launch{}, err
	}
	rec, err := b.takeOverSession(tr, args)
	if err != nil {
		return adewire.Launch{}, err
	}

	mu := b.taskMu(rec.TaskID)
	mu.Lock()
	defer mu.Unlock()
	return b.takeOverLocked(ctx, tr, rec, opTakeOver)
}

// takeOverLocked opens the row's conversation in a TUI. The task lock is held.
func (b *TaskBoard) takeOverLocked(ctx context.Context, tr *Tracker, rec *model.AdeSession, kind string) (adewire.Launch, error) {
	// The check in takeOverSession ran before the lock, so a concurrent call may have opened it since.
	if err := b.ensureNotOpen(tr, rec); err != nil {
		return adewire.Launch{}, err
	}
	tc, err := b.loadTaskCtx(rec.TaskID)
	if err != nil {
		return adewire.Launch{}, err
	}
	if rec.Purpose == model.AdeSessionPurposeReview {
		l, _, _, err := b.launchReviewAgent(ctx, tc, tr, "", nil)
		return l, err
	}
	if err := b.snapshotWorkflow(&tc.task); err != nil {
		return adewire.Launch{}, err
	}
	cwd, moved, err := b.takeOverCwd(ctx, tc, rec)
	if err != nil {
		return adewire.Launch{}, err
	}
	if rec.AgentSessionID == "" {
		id, err := tr.learnSessionID(rec.ID)
		if err != nil {
			return adewire.Launch{}, err
		}
		if id == "" {
			return adewire.Launch{}, errSessionIDUnknown()
		}
		rec.AgentSessionID = id
	}

	pa := PrepareArgs{TaskID: rec.TaskID, BranchID: rec.BranchID, StepID: rec.StepID, Cwd: cwd}
	if rec.StepID != "" {
		pa.Settings = stepSettingsOf(tc, rec.StepID)
	}
	// Resume pins the recorded cwd, so a TUI record whose directory moved resumes by conversation id.
	if rec.Mode == model.AdeSessionModeTUI && !moved {
		pa.Resume = rec.ID
	} else {
		pa.Resumes = rec.AgentSessionID
	}
	pa.Provider = rec.Provider
	runID := ""
	if rec.Mode == model.AdeSessionModeHeadless {
		runID = b.finishableRun(tc, rec)
	}
	return b.prepareWithGrant(ctx, tr, tc, pa, kind, runID, rec.StepID)
}

// closeTUILocked stops and closes a running interactive session. Stop runs before the close so
// the exit's Reconcile finds nothing to do. taskMu is held by the caller.
func (b *TaskBoard) closeTUILocked(tr *Tracker, rec *model.AdeSession) error {
	if b.deps.CloseTerminal == nil {
		return invalid("interactive sessions are not available")
	}
	terminalID, ok := tr.TerminalOf(rec.ID)
	if !ok {
		return invalid("the session is not running")
	}
	if err := tr.Stop(rec.ID); err != nil {
		if errors.Is(err, ErrSessionNotRunning) {
			return invalid("the session is not running")
		}
		return err
	}
	// Blocks until the process exits, so a resumed claude never shares the conversation file.
	if err := b.deps.CloseTerminal(terminalID); err != nil {
		return fmt.Errorf("ade: close terminal: %w", err)
	}
	return nil
}

// CloseSession ends a running session and keeps its history: an interactive one stops at once
// and resumes by Take over, a background one stops its run.
func (b *TaskBoard) CloseSession(ctx context.Context, args adewire.CloseSessionArgs) error {
	rec, err := b.deps.Sessions.Get(args.SessionID)
	if err != nil {
		return err
	}
	if rec == nil || rec.TaskID == "" {
		return invalid("session %s is not a task session", args.SessionID)
	}
	if rec.State != model.AdeSessionStateRunning {
		return invalid("the session is not running")
	}
	if rec.Mode != model.AdeSessionModeTUI {
		if rec.RunID == "" {
			return invalid("the session is not running")
		}
		return b.StopRun(ctx, rec.RunID)
	}
	tr, err := b.tracker()
	if err != nil {
		return err
	}
	mu := b.taskMu(rec.TaskID)
	mu.Lock()
	defer mu.Unlock()
	if rec, err = b.deps.Sessions.Get(args.SessionID); err != nil {
		return err
	}
	if rec == nil || rec.State != model.AdeSessionStateRunning {
		return invalid("the session is not running")
	}
	return b.closeTUILocked(tr, rec)
}

// RestartSession closes a running session and continues its conversation: an interactive one
// resumes as Take over does for a stopped one, a background step run queues its next attempt.
func (b *TaskBoard) RestartSession(ctx context.Context, args adewire.RestartSessionArgs) (adewire.Launch, error) {
	tr, err := b.tracker()
	if err != nil {
		return adewire.Launch{}, err
	}
	rec, err := b.deps.Sessions.Get(args.SessionID)
	if err != nil {
		return adewire.Launch{}, err
	}
	if rec == nil || rec.TaskID == "" {
		return adewire.Launch{}, invalid("session %s is not a task session", args.SessionID)
	}
	// StopRun must not run under the task lock.
	if rec.Mode != model.AdeSessionModeTUI {
		return b.restartHeadless(ctx, rec)
	}
	// Held across Stop, close and resume: a concurrent Take over would otherwise resume the
	// conversation while the old process still writes it.
	mu := b.taskMu(rec.TaskID)
	mu.Lock()
	defer mu.Unlock()
	if rec, err = b.deps.Sessions.Get(args.SessionID); err != nil {
		return adewire.Launch{}, err
	}
	if rec == nil || rec.TaskID == "" {
		return adewire.Launch{}, invalid("session %s is not a task session", args.SessionID)
	}
	if rec.Mode != model.AdeSessionModeTUI {
		return adewire.Launch{}, invalid("the session is not running")
	}
	if rec.State != model.AdeSessionStateRunning {
		return adewire.Launch{}, invalid("the session is not running")
	}
	if err := b.closeTUILocked(tr, rec); err != nil {
		return adewire.Launch{}, err
	}
	if rec, err = b.deps.Sessions.Get(rec.ID); err != nil {
		return adewire.Launch{}, err
	}
	return b.takeOverLocked(ctx, tr, rec, opRestart)
}

// ensureNotOpen rejects a session whose conversation runs in a TUI or has one prepared and not yet
// composed.
func (b *TaskBoard) ensureNotOpen(tr *Tracker, rec *model.AdeSession) error {
	if rec.Mode == model.AdeSessionModeTUI && rec.State == model.AdeSessionStateRunning {
		return invalid("the session is already open")
	}
	if rec.AgentSessionID == "" {
		return nil // no id yet, so nothing else can share the conversation
	}
	open, err := b.runningTUI(func(s model.AdeSession) bool { return s.AgentSessionID == rec.AgentSessionID })
	if err != nil {
		return err
	}
	if open != nil {
		return invalid("the conversation is already open")
	}
	if tr.hasPending(func(p pendingIntent) bool { return p.AgentSessionID == rec.AgentSessionID }) {
		return invalid("a launch of the conversation is already starting")
	}
	return nil
}

// takeOverSession loads the row and rejects one that is not takeable; a running headless run is
// stopped when args allow.
func (b *TaskBoard) takeOverSession(tr *Tracker, args adewire.TakeOverArgs) (*model.AdeSession, error) {
	rec, err := b.deps.Sessions.Get(args.SessionID)
	if err != nil {
		return nil, err
	}
	if rec == nil || rec.TaskID == "" {
		return nil, invalid("session %s is not a task session", args.SessionID)
	}
	if err := b.ensureNotOpen(tr, rec); err != nil {
		return nil, err
	}
	if rec.Mode == model.AdeSessionModeHeadless {
		if err := b.stopForTakeOver(rec, args.StopIfRunning); err != nil {
			return nil, err
		}
	}
	return rec, nil
}

// takeOverCwd gates the session's branch and returns its cwd, falling back to the gate path when
// the recorded directory is gone (moved).
func (b *TaskBoard) takeOverCwd(ctx context.Context, tc *taskCtx, rec *model.AdeSession) (cwd string, moved bool, err error) {
	gatePath := ""
	if rec.BranchID != "" {
		sb, ok := tc.branch(rec.BranchID)
		if !ok {
			return "", false, invalid("the session's branch is no longer on the task")
		}
		if gatePath, err = b.launchGate(ctx, tc, sb); err != nil {
			return "", false, err
		}
	}
	if info, statErr := os.Stat(rec.Cwd); statErr != nil || !info.IsDir() {
		if gatePath == "" {
			return "", false, invalid("%s no longer exists", rec.Cwd)
		}
		return gatePath, true, nil
	}
	return rec.Cwd, false, nil
}

// stopForTakeOver stops the headless run behind the row when it still runs (D3).
func (b *TaskBoard) stopForTakeOver(rec *model.AdeSession, stop bool) error {
	run, err := b.deps.Tasks.GetRun(rec.RunID)
	if err != nil || run.State != model.AdeRunRunning {
		return nil //nolint:nilerr // a run deleted by a workflow switch leaves its conversation takeable
	}
	if !stop {
		return invalid("the run is still running; stop it first")
	}
	agent, err := b.agentFor(rec.Provider)
	if err != nil {
		return err
	}
	done := b.cancelLive(b.live, run.ID, takenOver{product: agent.ProductLabel()})
	return b.awaitEnd(done)
}

// finishableRun is the taken-over run when it is the latest attempt of its step in the current pass and waits for
// a decision (R14), else "": its TUI can then report through finish_step.
func (b *TaskBoard) finishableRun(tc *taskCtx, rec *model.AdeSession) string {
	run, err := b.deps.Tasks.GetRun(rec.RunID)
	if err != nil || run.State != model.AdeRunStuck && run.State != model.AdeRunFailed {
		return ""
	}
	if run.Purpose == model.AdeRunPurposeRebase {
		latest, err := b.deps.Tasks.LatestRebaseRun(run.BranchID)
		if err != nil || latest == nil || latest.ID != run.ID {
			return ""
		}
		return run.ID
	}
	if !b.isLatestInPass(tc, run) {
		return ""
	}
	return run.ID
}

// prepareWithGrant registers the agent MCP grant every task TUI launch carries (handover, run_outcome;
// finish_step for a taken-over run; the Space tools) and prepares the
// launch with its flags. grantStep is the stage the session opens for, "" when it has none. The grant
// lives until the record stops or its launch expires.
func (b *TaskBoard) prepareWithGrant(ctx context.Context, tr *Tracker, tc *taskCtx, pa PrepareArgs, kind, runID, grantStep string) (_ adewire.Launch, err error) {
	agent, err := b.agentFor(pa.Provider)
	if err != nil {
		return adewire.Launch{}, err
	}
	pa.Provider = agent.ID()
	pa.Op = b.startSessionOp(ctx, kind, tc.task.ID, agent.Label(), "")
	defer func() {
		if err != nil {
			pa.Op.Finish(oplog.StatusError, err.Error())
		}
	}()
	grant := agentmcp.Grant{RunID: runID, TaskID: tc.task.ID, Space: true, Agent: agent}
	var ids []harness.MCPTool
	if runID != "" {
		grant.Results = resultSpecs(stepResultsOf(tc, pa.StepID))
		ids = append(ids, agentmcp.FinishStep)
	} else {
		grant.StepID = grantStep
	}
	cfg, release, err := b.agent.Register(grant)
	if err != nil {
		return adewire.Launch{}, err
	}
	ids = append(ids, agentmcp.RunOutcome)
	if runID == "" {
		ids = append(ids, agentmcp.Handover)
	}
	ids = append(ids, agentmcp.SpaceToolIDs...)
	pa.MCPConfig, pa.AllowedTools = cfg, harness.ToolNames(agent, ids...)
	res, err := tr.Prepare(pa)
	if err != nil {
		release()
		return adewire.Launch{}, err
	}
	pa.Op.SetCommand(res.Command)
	b.bindTUI(res.RecordID, runID, release)
	return launchOf(res), nil
}

// stepResultsOf are the results of a step of the task's workflow; none when it has no such step.
func stepResultsOf(tc *taskCtx, stepID string) []adewire.StepResult {
	if tc.wf == nil {
		return nil
	}
	for _, d := range workflowSteps(*tc.wf) {
		if d.ID == stepID {
			return d.Results
		}
	}
	return nil
}

func (b *TaskBoard) isLatestInPass(tc *taskCtx, run model.AdeRun) bool {
	if tc.wf == nil || !tc.inPass(run) {
		return false
	}
	plan, err := b.plan(tc)
	if err != nil {
		return false
	}
	idx := stepIndex(plan, run.StepID)
	if idx < 0 {
		return false
	}
	latest, ok := plan[idx].Runs[run.BranchID]
	return ok && latest.ID == run.ID
}

func (b *TaskBoard) bindTUI(record, runID string, release func()) {
	b.runMu.Lock()
	prev := b.tuiGrants[record]
	b.tuiGrants[record] = tuiBinding{runID: runID, release: release}
	b.runMu.Unlock()
	if prev.release != nil {
		prev.release()
	}
}

// OnTUIStopped is Tracker.OnStopped's target: it releases the agent MCP grant of the record's launch.
func (b *TaskBoard) OnTUIStopped(recordID string) {
	b.runMu.Lock()
	bind, ok := b.tuiGrants[recordID]
	delete(b.tuiGrants, recordID)
	b.runMu.Unlock()
	if ok {
		bind.release()
	}
}

func (b *TaskBoard) tuiBound(runID string) bool {
	b.runMu.Lock()
	defer b.runMu.Unlock()
	for _, bind := range b.tuiGrants {
		if bind.runID == runID {
			return true
		}
	}
	return false
}

// applyTUIFinish applies a finish_step call of a taken-over run at call time (R14).
func (b *TaskBoard) applyTUIFinish(runID string, f agentmcp.Finish) {
	if b.ctx.Err() != nil {
		return
	}
	run, err := b.deps.Tasks.GetRun(runID)
	if err != nil {
		slog.Warn("ade: taken-over finish", "scope", "ade", "run", runID, "err", err)
		return
	}
	mu := b.taskMu(run.TaskID)
	mu.Lock()
	defer mu.Unlock()
	if run, err = b.deps.Tasks.GetRun(runID); err != nil || (run.State != model.AdeRunStuck && run.State != model.AdeRunFailed) {
		return
	}
	if run.Purpose == model.AdeRunPurposeRebase {
		b.applyRebaseFinish(run, f)
		return
	}
	tc, err := b.loadTaskCtx(run.TaskID)
	if err != nil || !b.isLatestInPass(tc, run) {
		return
	}
	switch f.Status {
	case "needs_input": // the run keeps waiting for a decision
		stuck := model.AdeRunStuck
		summary := cmpNonEmpty(f.Reason, f.Summary)
		updated, err := b.deps.Tasks.UpdateRun(runID, model.AdeRunPatch{State: &stuck, Note: &summary, Summary: &summary})
		if err != nil {
			slog.Warn("ade: taken-over finish", "scope", "ade", "run", runID, "err", err)
			return
		}
		b.emitRuns(updated)
	default:
		b.recordOutcomeLocked(run, "", fromFinish(f, nil, stepResultsOf(tc, run.StepID)))
	}
}

// --- LaunchStep, StartBranch ---------------------------------------------------------------------

// LaunchStep opens the interactive session of the task's current manual step (R15).
func (b *TaskBoard) LaunchStep(ctx context.Context, args adewire.LaunchStepArgs) (adewire.Launch, error) {
	tr, err := b.tracker()
	if err != nil {
		return adewire.Launch{}, err
	}
	mu := b.taskMu(args.TaskID)
	mu.Lock()
	defer mu.Unlock()
	tc, err := b.loadTaskCtx(args.TaskID)
	if err != nil {
		return adewire.Launch{}, err
	}
	if err := b.staleStep(tc, args.FromStepID); err != nil {
		return adewire.Launch{}, err
	}
	plan, err := b.plan(tc)
	if err != nil {
		return adewire.Launch{}, err
	}
	entry := tc.entryIndex(plan)
	cur := -1
	if entry >= 0 {
		cur = currentStep(plan, entry)
	}
	if cur < 0 || !plan[cur].Def.manual() || !plan[cur].Def.Session {
		return adewire.Launch{}, invalid("the current step has no interactive session")
	}
	step := tc.wf.Steps[cur]
	if step.ID == reviewStepID {
		l, _, _, err := b.launchReviewAgent(ctx, tc, tr, args.Message, &step)
		return l, err
	}
	if open, err := b.runningTUI(func(s model.AdeSession) bool { return s.TaskID == tc.task.ID && s.StepID == step.ID }); err != nil {
		return adewire.Launch{}, err
	} else if open != nil {
		return adewire.Launch{}, invalid("a %s session is already running", step.Name)
	}
	if tr.hasPending(func(p pendingIntent) bool { return p.TaskID == tc.task.ID && p.StepID == step.ID && p.Purpose == "" }) {
		return adewire.Launch{}, invalid("a %s launch is already starting", step.Name)
	}

	if err := b.snapshotWorkflow(&tc.task); err != nil {
		return adewire.Launch{}, err
	}
	lines, cwd, extra, err := b.launchDirs(ctx, tc)
	if err != nil {
		return adewire.Launch{}, err
	}
	message := args.Message
	if message == "" {
		message = composeManualMessage(b.prompts(), step, taskTitle(tc.task, tc.branches), tc.task.JiraKey, tc.task.JiraURL, tc.task.Notes, lines, b.handoverFor(tc, step.ID))
	}
	pa := PrepareArgs{TaskID: tc.task.ID, StepID: step.ID, Cwd: cwd, Message: message, Settings: step.Settings}
	if len(extra) > 0 {
		pa.AddDirs = extra
	}
	return b.prepareWithGrant(ctx, tr, tc, pa, opManual, "", step.ID)
}

// launchDirs gathers the repos of a task-level session: the first writable branch's worktree is the
// cwd (else the first repo root), every other worktree and read-only repo root joins via --add-dir.
func (b *TaskBoard) launchDirs(ctx context.Context, tc *taskCtx) (lines []repoLine, cwd string, extra []string, err error) {
	var dirs []string
	firstOwn := -1
	for _, sb := range tc.branches {
		nick := b.repoNick(tc, sb)
		if sb.Kind == model.AdeBranchKindMine && sb.Name != "" {
			path, err := b.launchGate(ctx, tc, sb)
			if err != nil {
				return nil, "", nil, err
			}
			lines = append(lines, repoLine{Nick: nick, Branch: sb.Name, Base: sb.Base, Worktree: path, BranchID: sb.ID})
			if firstOwn < 0 {
				firstOwn = len(dirs)
			}
			dirs = append(dirs, path)
			continue
		}
		rec, err := b.deps.CodeRepos.Get(sb.CodeRepoID)
		if err != nil {
			return nil, "", nil, err
		}
		if rec == nil {
			return nil, "", nil, invalid("code repo %s not found", sb.CodeRepoID)
		}
		lines = append(lines, repoLine{Nick: nick, ReadOnlyRoot: rec.Root, BranchID: sb.ID})
		dirs = append(dirs, rec.Root)
	}
	if len(dirs) == 0 {
		return nil, "", nil, invalid("the task has no repo")
	}
	if firstOwn < 0 {
		firstOwn = 0
	}
	cwd = dirs[firstOwn]
	for i, d := range dirs {
		if i != firstOwn && filepath.Clean(d) != filepath.Clean(cwd) {
			extra = append(extra, d)
		}
	}
	return lines, cwd, extra, nil
}

// StartBranch opens an interactive session on one branch's worktree (R16).
func (b *TaskBoard) StartBranch(ctx context.Context, args adewire.StartBranchArgs) (adewire.Launch, error) {
	return b.startBranch(ctx, args.BranchID, opStart, func(tc *taskCtx, sb model.AdeTaskBranch, path string) string {
		if args.Message != "" {
			return args.Message
		}
		return composeStartMessage(b.prompts(), taskTitle(tc.task, tc.branches), tc.task.JiraKey, tc.task.JiraURL,
			repoLine{Nick: b.repoNick(tc, sb), Branch: sb.Name, Worktree: path}, b.firstOpenStep(tc), b.rebaseContext(tc), b.handoverFor(tc, b.currentStepID(tc)))
	})
}

// StartAdHoc opens an agent session on the branch's worktree with no first message.
func (b *TaskBoard) StartAdHoc(ctx context.Context, args adewire.BranchArgs) (adewire.Launch, error) {
	return b.startBranch(ctx, args.BranchID, opAdHoc, func(*taskCtx, model.AdeTaskBranch, string) string { return "" })
}

// startBranch holds every Start rule; message supplies the first prompt once the worktree is ready.
func (b *TaskBoard) startBranch(ctx context.Context, branchID, kind string, message func(tc *taskCtx, sb model.AdeTaskBranch, path string) string) (adewire.Launch, error) {
	tr, err := b.tracker()
	if err != nil {
		return adewire.Launch{}, err
	}
	sb, err := b.deps.Tasks.GetBranch(branchID)
	if err != nil {
		return adewire.Launch{}, err
	}
	mu := b.taskMu(sb.TaskID)
	mu.Lock()
	defer mu.Unlock()
	tc, err := b.loadTaskCtx(sb.TaskID)
	if err != nil {
		return adewire.Launch{}, err
	}
	if sb, _ = tc.branch(branchID); sb.ID == "" {
		return adewire.Launch{}, invalid("branch %s is not on a live task", branchID)
	}
	if sb.Name == "" {
		return adewire.Launch{}, invalid("the branch is not created yet")
	}
	if open, err := b.runningTUI(func(s model.AdeSession) bool { return s.BranchID == sb.ID }); err != nil {
		return adewire.Launch{}, err
	} else if open != nil {
		return adewire.Launch{}, invalid("a session is already open on %s", sb.Name)
	}
	if tr.hasPending(func(p pendingIntent) bool { return p.BranchID == sb.ID }) {
		return adewire.Launch{}, invalid("a launch is already starting on %s", sb.Name)
	}
	if err := b.checkNoBackgroundRun(sb); err != nil {
		return adewire.Launch{}, err
	}
	if err := b.snapshotWorkflow(&tc.task); err != nil {
		return adewire.Launch{}, err
	}
	path, err := b.launchGate(ctx, tc, sb)
	if err != nil {
		return adewire.Launch{}, err
	}
	// The gate may have released a held run onto this worktree.
	if err := b.checkNoBackgroundRun(sb); err != nil {
		return adewire.Launch{}, err
	}
	return b.prepareWithGrant(ctx, tr, tc, PrepareArgs{TaskID: tc.task.ID, BranchID: sb.ID, Cwd: path, Message: message(tc, sb, path)}, kind, "", "")
}

// checkNoBackgroundRun refuses a session on a worktree a run or a rebase is working in.
func (b *TaskBoard) checkNoBackgroundRun(sb model.AdeTaskBranch) error {
	has, err := b.deps.Tasks.HasRunningOn(sb.ID)
	if err != nil {
		return err
	}
	if r, err := b.deps.Tasks.RunningRebaseOn(sb.ID); err != nil {
		return err
	} else if has || r != nil {
		return invalid("a background run is working on %s", sb.Name)
	}
	if name := b.claimOn(sb.ID); name != "" {
		return invalid("script run %s is running in %s", name, sb.Name)
	}
	return nil
}

// firstOpenStep names the current step when it is an automatic one, "" otherwise.
func (b *TaskBoard) firstOpenStep(tc *taskCtx) string {
	plan, err := b.plan(tc)
	if err != nil {
		return ""
	}
	entry := tc.entryIndex(plan)
	if entry < 0 {
		return ""
	}
	if cur := currentStep(plan, entry); cur >= 0 && !plan[cur].Def.manual() {
		return plan[cur].Def.Name
	}
	return ""
}

// --- Send, session lookup ------------------------------------------------------------------------

// Send types a message into a running TUI session's terminal.
func (b *TaskBoard) Send(_ context.Context, args adewire.SendArgs) error {
	tr, err := b.tracker()
	if err != nil {
		return err
	}
	rec, err := b.deps.Sessions.Get(args.SessionID)
	if err != nil {
		return err
	}
	if rec == nil || rec.TaskID == "" {
		return ErrSessionNotFound
	}
	if rec.Mode == model.AdeSessionModeHeadless {
		return invalid("a background run takes no input; use Take over")
	}
	return tr.Send(args.SessionID, args.Message)
}

// TUISession returns a task session row, nil when it does not exist or is not a task session.
func (b *TaskBoard) TUISession(sessionID string) (*model.AdeSession, error) {
	rec, err := b.deps.Sessions.Get(sessionID)
	if err != nil || rec == nil || rec.TaskID == "" {
		return nil, err
	}
	return rec, nil
}
