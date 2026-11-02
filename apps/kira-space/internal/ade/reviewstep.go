package ade

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitreview"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/repos"
	"github.com/kirathecat/kira-studio/internal/agentmcp"
	"github.com/kirathecat/kira-studio/internal/llmprompts"
	"github.com/kirathecat/kira-studio/internal/runoutcome"
	"github.com/kirathecat/kira-studio/packages/theme/src/i18n"
)

// reviewstep.go: the review step. A review step run has no process: it waits in state `review`
// while the user comments in the review window. "Send to agent" queues a round, a headless run
// (purpose `review`) that answers the open comments through reply_review_comment. Finish routes the
// step on a result the user picks.

const noteReviewRound = "review round"

func (b *TaskBoard) reviewStore() (*gitreview.Store, error) {
	if b.deps.Registry == nil || b.deps.Registry.Review == nil {
		return nil, invalid("review is not available")
	}
	return b.deps.Registry.Review, nil
}

func (b *TaskBoard) emitReview(taskID, branchID string) {
	if b.deps.OnReview != nil {
		b.deps.OnReview(adewire.ReviewEvent{TaskID: taskID, BranchID: branchID})
	}
}

func roundSpec(run model.AdeRun) (model.AdeReviewSpec, error) {
	var spec model.AdeReviewSpec
	if err := json.Unmarshal([]byte(run.SpecJSON), &spec); err != nil {
		return spec, fmt.Errorf("ade: review round spec of %s: %w", run.ID, err)
	}
	return spec, nil
}

// latestRoundOf is the newest round row of a review run among runs, nil when it has none.
func latestRoundOf(runs []model.AdeRun, reviewRunID string) *model.AdeRun {
	var best *model.AdeRun
	for i := range runs {
		r := runs[i]
		if r.Purpose != model.AdeRunPurposeReview {
			continue
		}
		spec, err := roundSpec(r)
		if err != nil || spec.ReviewRunID != reviewRunID {
			continue
		}
		if best == nil || r.Attempt > best.Attempt {
			best = &runs[i]
		}
	}
	return best
}

// reviewWire is the phase a review run shows on the wire.
func reviewWire(run model.AdeRun, rev model.AdeReview, round *model.AdeRun) *adewire.RunReview {
	out := &adewire.RunReview{Phase: adewire.ReviewPhaseWaiting, Round: rev.Rounds, Route: rev.Route}
	switch run.State {
	case model.AdeRunReview:
	default:
		out.Phase = adewire.ReviewPhaseDone
		return out
	}
	if round == nil {
		return out
	}
	if spec, err := roundSpec(*round); err == nil && spec.Fallback {
		out.Note = i18n.T("backend.space.ade.reviewStep.fallback.body", nil)
	}
	switch round.State {
	case model.AdeRunPending:
		out.Phase = adewire.ReviewPhaseSent
	case model.AdeRunRunning:
		out.Phase = adewire.ReviewPhaseWorking
	case model.AdeRunStuck, model.AdeRunFailed:
		out.Phase = adewire.ReviewPhaseStuck
		if out.Note == "" {
			out.Note = round.Note
		}
	}
	return out
}

// wireBatch holds what wireRun reads per task and per run, loaded once for a batch of runs.
type wireBatch struct {
	b      *TaskBoard
	badges map[string]repos.AdeRunSandbox
	runs   map[string][]model.AdeRun
	rounds map[string][]model.AdeRun
}

func (b *TaskBoard) newWireBatch(runs []model.AdeRun) *wireBatch {
	w := &wireBatch{b: b, runs: map[string][]model.AdeRun{}, rounds: map[string][]model.AdeRun{}}
	if b.deps.RunSandbox == nil {
		return w
	}
	ids := make([]string, len(runs))
	for i, r := range runs {
		ids[i] = r.ID
	}
	rows, err := b.deps.RunSandbox.Badges(ids)
	if err != nil {
		slog.Warn("ade: sandbox badges", "scope", "ade", "err", err)
		return w
	}
	w.badges = rows
	return w
}

func (w *wireBatch) taskRuns(taskID string) ([]model.AdeRun, bool) {
	if runs, ok := w.runs[taskID]; ok {
		return runs, true
	}
	runs, err := w.b.deps.Tasks.RunsOfTask(taskID)
	if err != nil {
		return nil, false
	}
	w.runs[taskID] = runs
	return runs, true
}

func (w *wireBatch) taskRounds(taskID string) ([]model.AdeRun, bool) {
	if rounds, ok := w.rounds[taskID]; ok {
		return rounds, true
	}
	rounds, err := w.b.deps.Tasks.ReviewRounds(taskID)
	if err != nil {
		return nil, false
	}
	w.rounds[taskID] = rounds
	return rounds, true
}

// wireRun is the run as it goes on the wire, with the review state of a review step run.
func (w *wireBatch) wireRun(r model.AdeRun) adewire.Run {
	out := toWireRun(r)
	if row, ok := w.badges[r.ID]; ok {
		out.Sandbox = badgeOf(row)
	}
	if r.Purpose != "" {
		return out
	}
	if runs, ok := w.taskRuns(r.TaskID); ok {
		out.Returns = countReturns(runs, r.StepID, r.BranchID, r.Attempt)
	}
	rev, err := w.b.deps.Tasks.GetReview(r.ID)
	if err != nil || rev == nil {
		return out
	}
	rounds, ok := w.taskRounds(r.TaskID)
	if !ok {
		return out
	}
	out.Review = reviewWire(r, *rev, latestRoundOf(rounds, r.ID))
	return out
}

// --- entering the step ---------------------------------------------------------------------------

// enterReview starts a review step run: no process, the run waits for the user. The branch's review
// session is pinned so its comments survive until the task is archived.
func (b *TaskBoard) enterReview(ctx context.Context, tc *taskCtx, def stepDef, run model.AdeRun, sb model.AdeTaskBranch) (model.AdeRun, error) {
	store, err := b.reviewStore()
	if err != nil {
		return run, err
	}
	tgt, err := b.reviewTarget(ctx, tc, sb)
	if err != nil {
		return run, err
	}
	if err := store.SetPinned(ctx, tgt.GitRepoID, tgt.Branch, true); err != nil {
		return run, err
	}
	if err := b.deps.Tasks.UpsertReview(model.AdeReview{RunID: run.ID, Route: def.Review.SendTo}); err != nil {
		return run, err
	}
	now := b.deps.Now().UnixMilli()
	state, note := model.AdeRunReview, ""
	updated, err := b.deps.Tasks.UpdateRun(run.ID, model.AdeRunPatch{State: &state, StartedAt: &now, Note: &note, Launch: &model.AdeRunLaunch{}})
	if err != nil {
		return run, err
	}
	b.emitRuns(updated)
	return updated, nil
}

// --- candidates ----------------------------------------------------------------------------------

type reviewCandidate struct {
	run     model.AdeRun
	step    string
	allowed []string
}

// reviewCandidates lists the agent runs on the review run's branch that can take the comments:
// earlier agent steps of the current pass, nearest first.
func (b *TaskBoard) reviewCandidates(tc *taskCtx, review model.AdeRun) ([]reviewCandidate, error) {
	if tc.wf == nil {
		return nil, nil
	}
	latest, err := b.deps.Tasks.LatestRuns(tc.task.ID)
	if err != nil {
		return nil, err
	}
	byStep := map[string]model.AdeRun{}
	for _, r := range latest {
		if r.Purpose == "" && r.BranchID == review.BranchID && tc.inPass(r) {
			byStep[r.StepID] = r
		}
	}
	var out []reviewCandidate
	steps := tc.wf.Steps
	at := slices.IndexFunc(steps, func(s adewire.WorkflowStep) bool { return s.ID == review.StepID })
	for i := at - 1; i >= 0; i-- {
		s := steps[i]
		if s.Kind != adewire.KindAgent || s.SmartScript != "" {
			continue
		}
		r, ok := byStep[s.ID]
		if !ok || r.State == model.AdeRunRunning || r.State == model.AdeRunPending || b.agentIDOfSession(r.SessionID) == "" {
			continue
		}
		out = append(out, reviewCandidate{run: r, step: s.Name, allowed: s.AllowedTools})
	}
	return out, nil
}

// defaultCandidate is the step the review block names, else the nearest candidate; -1 when none.
func defaultCandidate(cands []reviewCandidate, def stepDef) int {
	if def.Review != nil && def.Review.Implementer != "" {
		if i := slices.IndexFunc(cands, func(c reviewCandidate) bool {
			return c.run.StepID == def.Review.Implementer
		}); i >= 0 {
			return i
		}
	}
	if len(cands) > 0 {
		return 0
	}
	return -1
}

// --- context -------------------------------------------------------------------------------------

// reviewCtx is a review step run with what a send or finish needs.
type reviewCtx struct {
	tc   *taskCtx
	plan []stepView
	idx  int
	sb   model.AdeTaskBranch
	run  model.AdeRun
	rev  model.AdeReview
	def  stepDef
}

// reviewRun loads a review step run that is the latest of the current pass. The task mutex is held.
func (b *TaskBoard) reviewRun(runID string) (*reviewCtx, error) {
	run, err := b.deps.Tasks.GetRun(runID)
	if err != nil {
		return nil, err
	}
	rev, err := b.deps.Tasks.GetReview(runID)
	if err != nil {
		return nil, err
	}
	if rev == nil || run.Purpose != "" {
		return nil, invalid("%s", i18n.T("backend.space.ade.reviewStep.notReview.body", nil))
	}
	tc, err := b.loadTaskCtx(run.TaskID)
	if err != nil {
		return nil, err
	}
	if tc.wf == nil || !tc.inPass(run) {
		return nil, invalid("the run is no longer in the current pass")
	}
	plan, err := b.plan(tc)
	if err != nil {
		return nil, err
	}
	idx := stepIndex(plan, run.StepID)
	if idx < 0 {
		return nil, invalid("the run's step is no longer in the workflow")
	}
	if latest, ok := plan[idx].Runs[run.BranchID]; !ok || latest.ID != run.ID {
		return nil, invalid("run %s is not the latest attempt", runID)
	}
	sb, ok := tc.branch(run.BranchID)
	if !ok {
		return nil, invalid("branch %s is not on this task", run.BranchID)
	}
	return &reviewCtx{tc: tc, plan: plan, idx: idx, sb: sb, run: run, rev: *rev, def: plan[idx].Def}, nil
}

func (b *TaskBoard) liveRound(taskID, reviewRunID string) (*model.AdeRun, error) {
	rounds, err := b.deps.Tasks.ReviewRounds(taskID)
	if err != nil {
		return nil, err
	}
	r := latestRoundOf(rounds, reviewRunID)
	if r != nil && (r.State == model.AdeRunPending || r.State == model.AdeRunRunning) {
		return r, nil
	}
	return nil, nil
}

// --- ReviewStep ----------------------------------------------------------------------------------

// ReviewStep describes the review step of a branch for the review window; Run is nil when the
// current pass has no review step run there.
func (b *TaskBoard) ReviewStep(_ context.Context, branchID string) (adewire.ReviewStepState, error) {
	out := adewire.ReviewStepState{Results: []adewire.StepResult{}, Candidates: []adewire.ReviewCandidate{}}
	sb, err := b.deps.Tasks.GetBranch(branchID)
	if err != nil {
		return out, err
	}
	tc, err := b.loadTaskCtx(sb.TaskID)
	if err != nil {
		return out, err
	}
	if tc.wf == nil {
		return out, nil
	}
	plan, err := b.plan(tc)
	if err != nil {
		return out, err
	}
	idx := -1
	for i, v := range plan {
		r, ok := v.Runs[branchID]
		if v.Def.Review == nil || !ok {
			continue
		}
		if idx < 0 || r.State == model.AdeRunReview {
			idx = i
		}
	}
	if idx < 0 {
		return out, nil
	}
	def, run := plan[idx].Def, plan[idx].Runs[branchID]
	rev, err := b.deps.Tasks.GetReview(run.ID)
	if err != nil {
		return out, err
	}
	if rev == nil {
		return out, nil
	}
	rounds, err := b.deps.Tasks.ReviewRounds(tc.task.ID)
	if err != nil {
		return out, err
	}
	round := latestRoundOf(rounds, run.ID)
	w := toWireRun(run)
	w.Review = reviewWire(run, *rev, round)
	out.Run, out.StepName, out.Results = &w, def.Name, def.Results
	out.Route, out.TargetRunID = rev.Route, rev.TargetRunID
	cands, err := b.reviewCandidates(tc, run)
	if err != nil {
		return out, err
	}
	for _, c := range cands {
		out.Candidates = append(out.Candidates, adewire.ReviewCandidate{
			RunID: c.run.ID, Step: c.step, Attempt: c.run.Attempt, FinishedAt: c.run.FinishedAt,
		})
	}
	if i := defaultCandidate(cands, def); i >= 0 {
		out.DefaultTargetRunID = cands[i].run.ID
	}
	if round != nil {
		spec, err := roundSpec(*round)
		if err != nil {
			return out, err
		}
		rr := &adewire.ReviewRound{Run: toWireRun(*round), N: spec.Round, Route: spec.Route, Fallback: spec.Fallback}
		if round.State == model.AdeRunDone && spec.HeadBefore != "" && spec.HeadAfter != "" {
			rr.Commits = b.roundCommits(tc, sb, spec)
		}
		out.Round = rr
	}
	return out, nil
}

// roundCommits counts the commits a round added; nil when git cannot say.
func (b *TaskBoard) roundCommits(tc *taskCtx, sb model.AdeTaskBranch, spec model.AdeReviewSpec) *int {
	entry, err := b.openRepo(b.ctx, sb.CodeRepoID)
	if err != nil {
		return nil
	}
	n, err := entry.CountRange(b.ctx, spec.HeadBefore, spec.HeadAfter)
	if err != nil {
		slog.Warn("ade: review round commits", "scope", "ade", "task", tc.task.ID, "err", err)
		return nil
	}
	return &n
}

// --- SendReview ----------------------------------------------------------------------------------

func commentVars(comments []gitreview.Comment) []llmprompts.ReviewCommentVar {
	out := make([]llmprompts.ReviewCommentVar, len(comments))
	for i, c := range comments {
		lines := fmt.Sprint(c.Range.Start)
		if c.Range.End > c.Range.Start {
			lines = fmt.Sprintf("%d-%d", c.Range.Start, c.Range.End)
		}
		out[i] = llmprompts.ReviewCommentVar{ID: c.ID, Kind: c.Kind, Path: c.Path, Lines: lines, Body: c.Body,
			Uncommitted: c.Origin == gitreview.OriginWorking}
	}
	return out
}

// roundMessage is the first message of a dedicated review round, with the branch's change request link.
func (b *TaskBoard) roundMessage(ctx context.Context, rc *reviewCtx, tgt adewire.ReviewWindowTarget, round int, comments []gitreview.Comment) string {
	v := b.roundVars(rc, tgt, round, comments)
	v.ChangeRequestURL = b.changeRequestURL(ctx, rc.sb, rc.tc.task.HostURL)
	return llmprompts.AdeReviewRoundMessage.Text(b.prompts(), v)
}

func (b *TaskBoard) roundVars(rc *reviewCtx, tgt adewire.ReviewWindowTarget, round int, comments []gitreview.Comment) llmprompts.ReviewRoundVars {
	return llmprompts.ReviewRoundVars{
		Title: taskTitle(rc.tc.task, rc.tc.branches), JiraKey: rc.tc.task.JiraKey, JiraURL: rc.tc.task.JiraURL,
		Repo: rc.tc.nick[rc.sb.CodeRepoID], Branch: rc.sb.Name, Base: tgt.Base, Worktree: tgt.Worktree,
		Round: round, Comments: commentVars(comments),
	}
}

// SendReview sends the open comments of a waiting review run to an agent as a new round.
func (b *TaskBoard) SendReview(ctx context.Context, args adewire.SendReviewArgs) error {
	run, err := b.deps.Tasks.GetRun(args.RunID)
	if err != nil {
		return err
	}
	mu := b.taskMu(run.TaskID)
	mu.Lock()
	defer mu.Unlock()
	rc, err := b.reviewRun(args.RunID)
	if err != nil {
		return err
	}
	if rc.run.State != model.AdeRunReview {
		return invalid("%s", i18n.T("backend.space.ade.reviewStep.notWaiting.body", nil))
	}
	if live, err := b.liveRound(rc.run.TaskID, rc.run.ID); err != nil {
		return err
	} else if live != nil {
		return invalid("%s", i18n.T("backend.space.ade.reviewStep.roundLive.body", nil))
	}
	store, err := b.reviewStore()
	if err != nil {
		return err
	}
	tgt, err := b.reviewTarget(ctx, rc.tc, rc.sb)
	if err != nil {
		return err
	}
	open, err := store.OpenComments(ctx, tgt.GitRepoID, tgt.Branch)
	if err != nil {
		return err
	}
	if len(open) == 0 {
		return invalid("%s", i18n.T("backend.space.ade.reviewStep.noOpen.body", nil))
	}
	route := cmpNonEmpty(args.Route, rc.rev.Route)
	if route != adewire.ReviewSendImplementer && route != adewire.ReviewSendNew {
		return invalid("route must be implementer or new")
	}
	spec := model.AdeReviewSpec{
		ReviewRunID: rc.run.ID, Route: route, GitRepoID: tgt.GitRepoID, Branch: tgt.Branch,
		CommentIDs: make([]int64, len(open)),
	}
	for i, c := range open {
		spec.CommentIDs[i] = c.ID
	}
	round := rc.rev.Rounds + 1
	spec.Round = round
	launch := model.AdeRunLaunch{Note: noteReviewRound}
	targetID := ""
	switch {
	case route == adewire.ReviewSendImplementer:
		cands, err := b.reviewCandidates(rc.tc, rc.run)
		if err != nil {
			return err
		}
		pick := defaultCandidate(cands, rc.def)
		if args.TargetRunID != "" {
			pick = slices.IndexFunc(cands, func(c reviewCandidate) bool { return c.run.ID == args.TargetRunID })
		}
		if pick < 0 {
			return invalid("%s", i18n.T("backend.space.ade.reviewStep.badTarget.body", nil))
		}
		target := cands[pick]
		agentSessionID := b.agentIDOfSession(target.run.SessionID)
		if agentSessionID == "" {
			return invalid("%s", i18n.T("backend.space.ade.sessionIdUnknown.body", nil))
		}
		if busy, err := b.agentBusy(agentSessionID); err != nil {
			return err
		} else if busy {
			return invalid("%s", i18n.T("backend.space.ade.reviewStep.targetBusy.body", nil))
		}
		targetID = target.run.ID
		spec.AllowedTools = target.allowed
		launch.ResumeID = agentSessionID
		launch.Prompt = llmprompts.AdeReviewFollowupMessage.Text(b.prompts(), b.roundVars(rc, tgt, round, open))
	case rc.rev.DedicatedClaudeID != "":
		launch.ResumeID = rc.rev.DedicatedClaudeID
		launch.Prompt = llmprompts.AdeReviewFollowupMessage.Text(b.prompts(), b.roundVars(rc, tgt, round, open))
	default:
		launch.Prompt = b.roundMessage(ctx, rc, tgt, round, open)
	}
	if entry, err := b.openRepo(ctx, rc.sb.CodeRepoID); err == nil {
		if _, tip, err := entry.WorktreeHead(ctx, tgt.Worktree); err == nil {
			spec.HeadBefore = tip
		}
	}
	if err := b.deps.Tasks.UpsertReview(model.AdeReview{RunID: rc.run.ID, Route: route, TargetRunID: targetID}); err != nil {
		return err
	}
	if _, err := b.deps.Tasks.BumpReviewRound(rc.run.ID); err != nil {
		return err
	}
	if err := store.MarkSent(ctx, tgt.GitRepoID, tgt.Branch, spec.CommentIDs, b.deps.Now()); err != nil {
		return err
	}
	_, err = b.queueRound(ctx, rc.tc, rc.sb, tgt.Worktree, spec, launch)
	b.emitReview(rc.tc.task.ID, rc.sb.ID)
	return err
}

// agentBusy says a running session already uses the agent conversation.
func (b *TaskBoard) agentBusy(agentSessionID string) (bool, error) {
	rows, err := b.deps.Sessions.ListTask()
	if err != nil {
		return false, err
	}
	return slices.ContainsFunc(rows, func(s model.AdeSession) bool {
		return s.AgentSessionID == agentSessionID && s.State == model.AdeSessionStateRunning
	}), nil
}

// queueRound inserts a round run and launches it, or leaves it pending behind a gate. The task
// mutex is held.
func (b *TaskBoard) queueRound(ctx context.Context, tc *taskCtx, sb model.AdeTaskBranch, path string, spec model.AdeReviewSpec, launch model.AdeRunLaunch) (model.AdeRun, error) {
	if err := b.checkNotArchiving(tc.task.ID); err != nil {
		return model.AdeRun{}, err
	}
	rounds, err := b.deps.Tasks.ReviewRounds(tc.task.ID)
	if err != nil {
		return model.AdeRun{}, err
	}
	attempt := 1
	if last := latestRoundOf(rounds, spec.ReviewRunID); last != nil {
		attempt = last.Attempt + 1
	}
	raw, err := json.Marshal(spec)
	if err != nil {
		return model.AdeRun{}, err
	}
	rv, err := b.deps.Tasks.GetRun(spec.ReviewRunID)
	if err != nil {
		return model.AdeRun{}, err
	}
	run := model.AdeRun{
		ID: b.newID(), TaskID: tc.task.ID, StepID: rv.StepID, BranchID: rv.BranchID, Attempt: attempt,
		State: model.AdeRunPending, Purpose: model.AdeRunPurposeReview, SpecJSON: string(raw), Launch: launch,
	}
	if err := b.deps.Tasks.InsertRun(run); err != nil {
		return model.AdeRun{}, err
	}
	return b.launchRound(ctx, tc, run, sb, path)
}

// launchRound starts a round, or holds it while a rebase or smart script works in the worktree.
func (b *TaskBoard) launchRound(ctx context.Context, tc *taskCtx, run model.AdeRun, sb model.AdeTaskBranch, path string) (model.AdeRun, error) {
	if err := b.checkNotArchiving(run.TaskID); err != nil {
		return run, err
	}
	hold := func(note string) (model.AdeRun, error) {
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
	if r, err := b.deps.Tasks.RunningRebaseOn(sb.ID); err != nil {
		return run, err
	} else if r != nil {
		return hold(noteWaitingRebase)
	}
	if name := b.claimOn(sb.ID); name != "" {
		return hold(noteWaitingAutomation + name)
	}
	out, err := b.startRound(ctx, tc, run, sb, path)
	if err != nil && !errors.Is(err, errBoardClosed) {
		b.failLaunch(run, err)
	}
	return out, err
}

func (b *TaskBoard) startRound(ctx context.Context, tc *taskCtx, run model.AdeRun, sb model.AdeTaskBranch, path string) (model.AdeRun, error) {
	if path == "" {
		return run, invalid("worktree of %s is missing", sb.Name)
	}
	spec, err := roundSpec(run)
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
	launch := run.Launch
	sessionID := b.newID()
	agent, err := b.agentForResume(launch.ResumeID)
	if err != nil {
		return run, err
	}
	agentSessionID := b.proposeAgentID(agent, launch.ResumeID)
	if err := b.deps.Sessions.InsertHeadless(model.AdeSession{
		ID: sessionID, Mode: model.AdeSessionModeHeadless, TaskID: tc.task.ID, BranchID: sb.ID,
		StepID: run.StepID, RunID: run.ID, Resumes: launch.ResumeID, AgentSessionID: agentSessionID, Cwd: path, Provider: agent.ID(),
		State: model.AdeSessionStateRunning, StartedAt: now, LastActiveAt: now,
	}); err != nil {
		return run, err
	}
	b.notifySessions()
	b.sessionOps.put(sessionID, b.startSessionOp(ctx, opReview, tc.task.ID, agent.Label(), ""))
	b.cancelRunOp(b.sessionOps.peek(sessionID), run.ID)
	defer func() {
		if !started {
			b.finishRunOp(sessionID, outcome{state: model.AdeRunFailed, note: "The review round did not start."})
		}
	}()
	var learned func(string)
	if spec.Route == adewire.ReviewSendNew {
		dedicate := func(id string) {
			if err := b.deps.Tasks.SetReviewDedicated(spec.ReviewRunID, id); err != nil {
				slog.Warn("ade: store review conversation", "scope", "ade", "run", spec.ReviewRunID, "err", err)
			}
		}
		if agentSessionID != "" {
			dedicate(agentSessionID)
		} else {
			learned = dedicate
		}
	}
	running, note := model.AdeRunRunning, launch.Note
	updated, err := b.deps.Tasks.UpdateRun(run.ID, model.AdeRunPatch{
		State: &running, StartedAt: &now, SessionID: &sessionID, Note: &note, Launch: &model.AdeRunLaunch{},
	})
	if err != nil {
		return run, err
	}
	b.emitRuns(updated)
	def := stepDef{ID: run.StepID, Timeout: "", AllowedTools: spec.AllowedTools}
	for _, s := range workflowSteps(*tc.wf) {
		if s.ID == run.StepID {
			def.Name, def.Timeout, def.Sandbox, def.Settings = s.Name, s.Timeout, s.Sandbox, s.Settings
		}
	}
	prompt := composeResumePrompt(b.prompts(), launch.Prompt, nil)
	runCtx, endRun := b.beginLive(b.live, run.ID)
	started = true
	go func() {
		defer b.wg.Done()
		defer endRun()
		b.superviseAgent(runCtx, updated, agentLaunch{
			def: def, agent: agent, sessionID: sessionID, resume: launch.ResumeID, path: path, worktree: path, prompt: prompt,
			timeout: parseStepTimeout(def.Timeout), review: &agentmcp.ReviewGrant{CommentIDs: spec.CommentIDs}, onSessionID: learned,
		})
	}()
	return updated, nil
}

// --- round end -----------------------------------------------------------------------------------

// completeReviewRound records how a round ended; the review run stays waiting.
func (b *TaskBoard) completeReviewRound(run model.AdeRun, sessionID string, out outcome) {
	mu := b.taskMu(run.TaskID)
	mu.Lock()
	defer mu.Unlock()
	now := b.deps.Now().UnixMilli()
	b.finishRunOp(sessionID, out)
	if sessionID != "" {
		if err := b.deps.Sessions.MarkStopped(sessionID, now); err != nil {
			slog.Warn("ade: stop headless session", "scope", "ade", "session", sessionID, "err", err)
		}
		b.notifySessions()
	}
	spec, err := roundSpec(run)
	if err != nil {
		slog.Warn("ade: review round", "scope", "ade", "run", run.ID, "err", err)
		return
	}
	patch := model.AdeRunPatch{
		State: &out.state, Note: &out.note, Summary: &out.summary, ExitCode: out.exit, FinishedAt: &now, Outcome: storedOutcome(out.out),
	}
	if out.state == model.AdeRunDone {
		if sb, err := b.deps.Tasks.GetBranch(run.BranchID); err == nil {
			spec.HeadAfter = b.worktreeTip(sb)
			if raw, err := json.Marshal(spec); err == nil {
				s := string(raw)
				patch.SpecJSON = &s
			}
		}
	}
	updated, err := b.deps.Tasks.UpdateRun(run.ID, patch)
	if err != nil {
		slog.Warn("ade: record review round", "scope", "ade", "run", run.ID, "err", err)
		return
	}
	b.emitRuns(updated)
	if out.lost && !spec.Fallback {
		b.queueFallback(updated, spec)
	}
}

// worktreeTip is the HEAD of the branch's worktree, "" when git cannot say.
func (b *TaskBoard) worktreeTip(sb model.AdeTaskBranch) string {
	path, err := b.worktreeOf(b.ctx, sb)
	if err != nil || path == "" {
		return ""
	}
	entry, err := b.openRepo(b.ctx, sb.CodeRepoID)
	if err != nil {
		return ""
	}
	_, tip, err := entry.WorktreeHead(b.ctx, path)
	if err != nil {
		return ""
	}
	return tip
}

// queueFallback sends the same comments to a fresh review agent after the target conversation was
// lost. One fallback per send. The task mutex is held.
func (b *TaskBoard) queueFallback(lost model.AdeRun, spec model.AdeReviewSpec) {
	tc, err := b.loadTaskCtx(lost.TaskID)
	if err != nil || tc.wf == nil || !tc.inPass(lost) {
		return
	}
	rv, err := b.deps.Tasks.GetRun(spec.ReviewRunID)
	if err != nil || rv.State != model.AdeRunReview {
		return
	}
	sb, ok := tc.branch(lost.BranchID)
	if !ok {
		return
	}
	if spec.Route == adewire.ReviewSendNew {
		if err := b.deps.Tasks.SetReviewDedicated(rv.ID, ""); err != nil {
			slog.Warn("ade: forget lost review session", "scope", "ade", "run", rv.ID, "err", err)
		}
	}
	rev, err := b.deps.Tasks.GetReview(rv.ID)
	if err != nil || rev == nil {
		return
	}
	if err := b.deps.Tasks.UpsertReview(model.AdeReview{RunID: rv.ID, Route: adewire.ReviewSendNew, TargetRunID: rev.TargetRunID}); err != nil {
		slog.Warn("ade: review fallback route", "scope", "ade", "run", rv.ID, "err", err)
		return
	}
	store, err := b.reviewStore()
	if err != nil {
		return
	}
	all, err := store.Comments(b.ctx, spec.GitRepoID, spec.Branch)
	if err != nil {
		slog.Warn("ade: review fallback comments", "scope", "ade", "run", rv.ID, "err", err)
		return
	}
	var comments []gitreview.Comment
	for _, c := range all {
		if slices.Contains(spec.CommentIDs, c.ID) {
			comments = append(comments, c)
		}
	}
	tgt, err := b.reviewTarget(b.ctx, tc, sb)
	if err != nil {
		slog.Warn("ade: review fallback target", "scope", "ade", "run", rv.ID, "err", err)
		return
	}
	next := spec
	next.Route, next.Fallback, next.HeadAfter, next.AllowedTools = adewire.ReviewSendNew, true, "", nil
	launch := model.AdeRunLaunch{Note: i18n.T("backend.space.ade.reviewStep.fallback.body", nil)}
	launch.Prompt = b.roundMessage(b.ctx, &reviewCtx{tc: tc, sb: sb}, tgt, spec.Round, comments)
	if _, err := b.queueRound(b.ctx, tc, sb, tgt.Worktree, next, launch); err != nil && !errors.Is(err, errBoardClosed) {
		slog.Warn("ade: review fallback round", "scope", "ade", "run", rv.ID, "err", err)
	}
	b.emitReview(tc.task.ID, sb.ID)
}

// resumeReviewRound continues the conversation of a round a restart stopped, as a new attempt. The
// task mutex is held.
func (b *TaskBoard) resumeReviewRound(ctx context.Context, run model.AdeRun) error {
	spec, err := roundSpec(run)
	if err != nil {
		return err
	}
	tc, err := b.loadTaskCtx(run.TaskID)
	if err != nil {
		return err
	}
	if tc.wf == nil || !tc.inPass(run) {
		return invalid("the run is no longer in the current pass")
	}
	rv, err := b.deps.Tasks.GetRun(spec.ReviewRunID)
	if err != nil {
		return err
	}
	if rv.State != model.AdeRunReview {
		return invalid("the review is no longer waiting")
	}
	rounds, err := b.deps.Tasks.ReviewRounds(run.TaskID)
	if err != nil {
		return err
	}
	if last := latestRoundOf(rounds, rv.ID); last == nil || last.ID != run.ID {
		return invalid("run %s is not the latest attempt", run.ID)
	}
	sb, ok := tc.branch(run.BranchID)
	if !ok {
		return invalid("branch %s is not on this task", run.BranchID)
	}
	tgt, err := b.reviewTarget(ctx, tc, sb)
	if err != nil {
		return err
	}
	store, err := b.reviewStore()
	if err != nil {
		return err
	}
	all, err := store.Comments(ctx, spec.GitRepoID, spec.Branch)
	if err != nil {
		return err
	}
	var open []gitreview.Comment
	for _, c := range all {
		if slices.Contains(spec.CommentIDs, c.ID) && c.State == gitreview.CommentStateOpen {
			open = append(open, c)
		}
	}
	line := llmprompts.RunResumeLine.Text(b.prompts(), llmprompts.ResumeVars{App: appName})
	if len(open) > 0 {
		line += "\n\n" + llmprompts.AdeReviewFollowupMessage.Text(b.prompts(), b.roundVars(&reviewCtx{tc: tc, sb: sb}, tgt, spec.Round, open))
	}
	launch := model.AdeRunLaunch{ResumeID: run.Launch.ResumeID, Prompt: line, Note: noteResumed}
	next := spec
	next.HeadAfter = ""
	if _, err := b.queueRound(ctx, tc, sb, tgt.Worktree, next, launch); err != nil {
		return err
	}
	cleared, err := b.deps.Tasks.UpdateRun(run.ID, model.AdeRunPatch{Launch: &model.AdeRunLaunch{}})
	if err != nil {
		return err
	}
	b.emitRuns(cleared)
	return nil
}

// --- reply and finish ----------------------------------------------------------------------------

// Reply stores an agent's answer to one comment of its round (agentmcp.ReviewReplier).
func (b *TaskBoard) Reply(runID string, id int64, reply string, resolved bool) error {
	run, err := b.deps.Tasks.GetRun(runID)
	if err != nil {
		return err
	}
	if run.Purpose != model.AdeRunPurposeReview {
		return fmt.Errorf("run %s is not a review round", runID)
	}
	spec, err := roundSpec(run)
	if err != nil {
		return err
	}
	if !slices.Contains(spec.CommentIDs, id) {
		return fmt.Errorf("comment %d was not sent to you", id)
	}
	store, err := b.reviewStore()
	if err != nil {
		return err
	}
	ok, err := store.ReplyComment(b.ctx, spec.GitRepoID, spec.Branch, id, reply, resolved, b.deps.Now())
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("comment %d no longer exists", id)
	}
	b.emitReview(run.TaskID, run.BranchID)
	return nil
}

// FinishReview ends a waiting review run on a result the user picks; the step then routes on it.
func (b *TaskBoard) FinishReview(_ context.Context, args adewire.FinishReviewArgs) error {
	run, err := b.deps.Tasks.GetRun(args.RunID)
	if err != nil {
		return err
	}
	mu := b.taskMu(run.TaskID)
	mu.Lock()
	defer mu.Unlock()
	rc, err := b.reviewRun(args.RunID)
	if err != nil {
		return err
	}
	if rc.run.State != model.AdeRunReview {
		return invalid("%s", i18n.T("backend.space.ade.reviewStep.notWaiting.body", nil))
	}
	if live, err := b.liveRound(rc.run.TaskID, rc.run.ID); err != nil {
		return err
	} else if live != nil {
		return invalid("%s", i18n.T("backend.space.ade.reviewStep.roundLive.body", nil))
	}
	res, ok := rc.def.resultOf(args.Result)
	if !ok {
		return invalid("%s", i18n.T("backend.space.ade.reviewStep.unknownResult.body", nil))
	}
	o := runoutcome.Outcome{Source: runoutcome.SourceUser, Reported: true, Summary: args.Note, Result: res.ID, Status: runoutcome.StatusDone}
	state := model.AdeRunDone
	if !res.OK {
		state, o.Status = model.AdeRunFailed, runoutcome.StatusFailed
		o.Reason = cmpNonEmpty(args.Note, cmpNonEmpty(res.Description, res.ID))
	}
	b.recordOutcomeLocked(rc.run, "", fromOutcome(state, o))
	b.emitReview(rc.tc.task.ID, rc.sb.ID)
	return nil
}

// releaseHeldRounds launches the round runs a branch's gate was holding. The task mutex is held.
func (b *TaskBoard) releaseHeldRounds(tc *taskCtx, sb model.AdeTaskBranch, path string) {
	held, err := b.deps.Tasks.PendingOnBranch(sb.ID, model.AdeRunPurposeReview)
	if err != nil {
		slog.Warn("ade: held review rounds", "scope", "ade", "branch", sb.ID, "err", err)
		return
	}
	for _, r := range held {
		if _, err := b.launchRound(b.ctx, tc, r, sb, path); err != nil {
			slog.Warn("ade: launch held review round", "scope", "ade", "run", r.ID, "err", err)
		}
	}
}

// errReviewWaits is the refusal for an action a waiting review run does not take.
func errReviewWaits() error {
	return invalid("%s", i18n.T("backend.space.ade.reviewStep.waits.body", nil))
}
