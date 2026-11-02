package ade

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/agentmcp"
	"github.com/kirathecat/kira-studio/internal/prompts"
	"github.com/kirathecat/kira-studio/packages/theme/src/i18n"
)

// branchrequest.go: an agent asks the user to approve branches and worktrees (P408). The call blocks
// without holding the task lock; the user answers through a routed prompt.

const (
	defaultBranchRequestWait = 30 * time.Minute

	branchKindBranch   = "branch"
	branchKindWorktree = "worktree"

	sourceRun        = "run"
	sourceSession    = "session"
	sourceAutomation = "automation"

	noteBranchApproval = "waiting for branch approval"
)

// ErrBranchRequestGone is the answer for a request already resolved, abandoned or never open.
var ErrBranchRequestGone = errors.New("ade: branch request is gone")

type decisionKind int

const (
	decisionApproved decisionKind = iota
	decisionDenied
	decisionTimeout
	decisionAbandoned
)

type branchDecision struct {
	kind     decisionKind
	approved map[string]bool // code repo ids the user kept checked
}

type branchRequest struct {
	view   adewire.BranchRequest
	result chan branchDecision // buffered 1: the one resolver never blocks
}

// branchRequests holds the open requests, at most one per task. Prompts open and close under mu, so a
// fast answer cannot close a prompt before it opened.
type branchRequests struct {
	mu     sync.Mutex
	byID   map[string]*branchRequest
	byTask map[string]string
}

func branchRequestPromptID(id string) string { return prompts.ID(prompts.KindBranchRequest, id) }

// add stores r and raises its prompt; false when the task already has an open request.
func (b *TaskBoard) addBranchRequest(r *branchRequest) bool {
	q := &b.branchReqs
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.byID == nil {
		q.byID, q.byTask = map[string]*branchRequest{}, map[string]string{}
	}
	if _, busy := q.byTask[r.view.TaskID]; busy {
		return false
	}
	q.byID[r.view.ID] = r
	q.byTask[r.view.TaskID] = r.view.ID
	if b.deps.PromptSink != nil {
		b.deps.PromptSink.Open(prompts.Prompt{
			ID: branchRequestPromptID(r.view.ID), Kind: prompts.KindBranchRequest, Ref: r.view.ID,
			Title: i18n.T("backend.prompt.branchRequest.title", map[string]string{"task": r.view.TaskTitle}),
		})
	}
	return true
}

// resolveBranchRequest removes the request, closes its prompt and hands it the decision; false when it was
// already resolved. Only the first resolver wins.
func (b *TaskBoard) resolveBranchRequest(id string, d branchDecision) bool {
	q := &b.branchReqs
	q.mu.Lock()
	defer q.mu.Unlock()
	r := q.byID[id]
	if r == nil {
		return false
	}
	delete(q.byID, id)
	delete(q.byTask, r.view.TaskID)
	if b.deps.PromptSink != nil {
		b.deps.PromptSink.Close(branchRequestPromptID(id))
	}
	r.result <- d
	return true
}

func (b *TaskBoard) abandonBranchRequests(taskID string) {
	q := &b.branchReqs
	q.mu.Lock()
	id := q.byTask[taskID]
	q.mu.Unlock()
	if id != "" {
		b.resolveBranchRequest(id, branchDecision{kind: decisionAbandoned})
	}
}

func (b *TaskBoard) abandonAllBranchRequests() {
	q := &b.branchReqs
	q.mu.Lock()
	ids := make([]string, 0, len(q.byID))
	for id := range q.byID {
		ids = append(ids, id)
	}
	q.mu.Unlock()
	for _, id := range ids {
		b.resolveBranchRequest(id, branchDecision{kind: decisionAbandoned})
	}
}

// branchRequestOf is the id of the task's open request, "" when none.
func (b *TaskBoard) branchRequestOf(taskID string) string {
	q := &b.branchReqs
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.byTask[taskID]
}

// BranchRequest returns an open request; ErrBranchRequestGone once it is resolved.
func (b *TaskBoard) BranchRequest(id string) (adewire.BranchRequest, error) {
	q := &b.branchReqs
	q.mu.Lock()
	defer q.mu.Unlock()
	r := q.byID[id]
	if r == nil {
		return adewire.BranchRequest{}, ErrBranchRequestGone
	}
	v := r.view
	v.Items = slices.Clone(v.Items)
	return v, nil
}

// ApproveBranchRequest approves the checked repos, a non-empty subset of the request's items; the rest
// are denied. resolved is false when the request is already gone.
func (b *TaskBoard) ApproveBranchRequest(id string, codeRepoIDs []string) (bool, error) {
	req, err := b.BranchRequest(id)
	if err != nil {
		return false, nil
	}
	if len(codeRepoIDs) == 0 {
		return false, invalid("approve at least one repo")
	}
	approved := make(map[string]bool, len(codeRepoIDs))
	for _, cid := range codeRepoIDs {
		if !slices.ContainsFunc(req.Items, func(it adewire.BranchRequestItem) bool { return it.CodeRepoID == cid }) {
			return false, invalid("repo %s is not in this request", cid)
		}
		approved[cid] = true
	}
	return b.resolveBranchRequest(id, branchDecision{kind: decisionApproved, approved: approved}), nil
}

// DenyBranchRequest denies every item; resolved is false when the request is already gone.
func (b *TaskBoard) DenyBranchRequest(id string) bool {
	return b.resolveBranchRequest(id, branchDecision{kind: decisionDenied})
}

func (b *TaskBoard) branchRequestWait() time.Duration {
	if b.deps.BranchRequestWait > 0 {
		return b.deps.BranchRequestWait
	}
	return defaultBranchRequestWait
}

// requestSlot is one repo of a request_branches call: a final result, or an item awaiting the user.
type requestSlot struct {
	codeRepoID string
	nick       string
	result     *agentmcp.RequestedBranch
	item       *adewire.BranchRequestItem
	addRow     bool // the repo is registered but not on the task: approval adds it
}

func noBranchInfo(nick, branch, next string) agentmcp.BranchInfo {
	return agentmcp.BranchInfo{Repo: nick, Branch: branch, Setup: agentmcp.SetupInfo{State: "none"}, Next: next}
}

func failedBranch(nick, branch, text string) *agentmcp.RequestedBranch {
	return &agentmcp.RequestedBranch{BranchInfo: noBranchInfo(nick, branch, text), Outcome: agentmcp.BranchFailed}
}

func waitText(d time.Duration) string {
	if d >= time.Minute {
		return fmt.Sprintf("%d minutes", int(d/time.Minute))
	}
	return fmt.Sprintf("%d seconds", int(d/time.Second))
}

// RequestBranches implements agentmcp.SpaceTools. Task or registered repos whose branch and worktree exist answer at once.
// The rest go to the user in one prompt and the call waits for the answer, unlocked. ctx ends when the
// caller is gone, which abandons the request.
func (b *TaskBoard) RequestBranches(ctx context.Context, call agentmcp.SpaceCall, refs []string) (agentmcp.RequestBranchesResult, error) {
	slots, req, err := b.planBranchRequest(ctx, call, refs)
	if err != nil {
		return agentmcp.RequestBranchesResult{}, err
	}
	if req == nil {
		return b.branchResults(slots), nil
	}
	if !b.addBranchRequest(req) {
		return agentmcp.RequestBranchesResult{}, agentmcp.ToolError("A branch request for this task already waits for the user. Wait for its answer, then call again.")
	}
	prevNote, noted := b.setRunNote(call.RunID, noteBranchApproval)
	b.notifyBoard()

	timer := time.NewTimer(b.branchRequestWait())
	defer timer.Stop()
	var dec branchDecision
	select {
	case dec = <-req.result:
	case <-timer.C:
		b.resolveBranchRequest(req.view.ID, branchDecision{kind: decisionTimeout})
		dec = <-req.result
	case <-ctx.Done():
		b.resolveBranchRequest(req.view.ID, branchDecision{kind: decisionAbandoned})
		dec = <-req.result
	}
	if noted {
		b.restoreRunNote(call.TaskID, call.RunID, prevNote)
	}
	return b.finishBranchRequest(ctx, call, slots, dec)
}

func (b *TaskBoard) branchResults(slots []requestSlot) agentmcp.RequestBranchesResult {
	out := agentmcp.RequestBranchesResult{Branches: make([]agentmcp.RequestedBranch, 0, len(slots))}
	for _, s := range slots {
		out.Branches = append(out.Branches, *s.result)
	}
	return out
}

// planBranchRequest validates the call under the task lock. req is nil when no repo needs the user.
func (b *TaskBoard) planBranchRequest(ctx context.Context, call agentmcp.SpaceCall, refs []string) ([]requestSlot, *branchRequest, error) {
	mu := b.taskMu(call.TaskID)
	mu.Lock()
	defer mu.Unlock()
	tc, err := b.spaceCtx(call.TaskID)
	if err != nil {
		return nil, nil, err
	}
	if err := b.spaceWritable(tc); err != nil {
		return nil, nil, err
	}
	cands, cfgs, err := b.registeredCandidates()
	if err != nil {
		return nil, nil, err
	}
	var ids []string
	for _, ref := range refs {
		id, err := resolveRepoRef(ref, cands, "registered")
		if err != nil {
			return nil, nil, err
		}
		if !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}
	var slots []requestSlot
	var items []adewire.BranchRequestItem
	for _, id := range ids {
		slot, err := b.planRepo(ctx, tc, id, cfgs)
		if err != nil {
			return nil, nil, err
		}
		slots = append(slots, slot)
		if slot.item != nil {
			items = append(items, *slot.item)
		}
	}
	if len(items) == 0 {
		return slots, nil, nil
	}
	source, stepName := b.callSource(tc, call)
	now := b.deps.Now()
	return slots, &branchRequest{
		view: adewire.BranchRequest{
			ID: b.newID(), TaskID: call.TaskID, TaskTitle: taskTitle(tc.task, tc.branches), Source: source, StepName: stepName,
			Items: items, CreatedAt: now.UnixMilli(), ExpiresAt: now.Add(b.branchRequestWait()).UnixMilli(),
		},
		result: make(chan branchDecision, 1),
	}, nil
}

// planRepo decides one repo: failed, already existing, or an item for the user.
func (b *TaskBoard) planRepo(ctx context.Context, tc *taskCtx, codeRepoID string, cfgs []model.AdeRepoConfig) (requestSlot, error) {
	var sb model.AdeTaskBranch
	found := false
	for _, br := range tc.branches {
		if br.CodeRepoID == codeRepoID {
			sb, found = br, true
		}
	}
	nick := tc.nick[codeRepoID]
	slot := requestSlot{codeRepoID: codeRepoID, nick: nick}
	if !found {
		for _, c := range cfgs {
			if c.CodeRepoID == codeRepoID {
				slot.nick = cmpNonEmpty(c.Nickname, c.Name)
			}
		}
		nick = slot.nick
		sb = model.AdeTaskBranch{TaskID: tc.task.ID, CodeRepoID: codeRepoID, Kind: model.AdeBranchKindMine}
		slot.addRow = true
	}
	if sb.Kind != model.AdeBranchKindMine {
		slot.result = failedBranch(nick, sb.Name, fmt.Sprintf("The branch of %s on this task is not yours to create.", nick))
		return slot, nil
	}
	rec, err := b.deps.CodeRepos.Get(codeRepoID)
	if err != nil {
		return slot, err
	}
	if rec == nil {
		slot.result = failedBranch(nick, sb.Name, fmt.Sprintf("Repo %s is no longer registered.", nick))
		return slot, nil
	}
	item := adewire.BranchRequestItem{CodeRepoID: codeRepoID, Repo: nick, Kind: branchKindWorktree, Branch: sb.Name}
	if sb.Name != "" {
		path, err := b.worktreeOf(ctx, sb)
		if err != nil {
			return slot, err
		}
		if path != "" {
			slot.result = &agentmcp.RequestedBranch{BranchInfo: b.branchInfo(sb, nick, path), Outcome: agentmcp.BranchExisting}
			return slot, nil
		}
		slot.item = &item
		return slot, nil
	}
	entry, err := b.openRepo(ctx, codeRepoID)
	if err != nil {
		return slot, err
	}
	_, display, refusal, err := b.newBranchBase(ctx, entry, sb)
	if err != nil {
		return slot, err
	}
	if refusal != "" {
		slot.result = failedBranch(nick, "", "Cannot create a branch here: "+refusal+".")
		return slot, nil
	}
	item.Kind, item.Base = branchKindBranch, display
	slot.item = &item
	return slot, nil
}

// callSource says who asks: a step run, a session, or an automation.
func (b *TaskBoard) callSource(tc *taskCtx, call agentmcp.SpaceCall) (source, step string) {
	if call.RunID != "" {
		if run, err := b.deps.Tasks.GetRun(call.RunID); err == nil {
			return sourceRun, stepName(tc, run.StepID)
		}
		return sourceAutomation, ""
	}
	return sourceSession, stepName(tc, call.StepID)
}

// setRunNote sets the note of an ADE run and returns the old one; ok is false when runID is no ADE run.
func (b *TaskBoard) setRunNote(runID, note string) (prev string, ok bool) {
	if runID == "" {
		return "", false
	}
	run, err := b.deps.Tasks.GetRun(runID)
	if err != nil {
		return "", false
	}
	updated, err := b.deps.Tasks.UpdateRun(runID, model.AdeRunPatch{Note: &note})
	if err != nil {
		slog.Warn("ade: branch request run note", "scope", "ade", "run", runID, "err", err)
		return "", false
	}
	b.emitRuns(updated)
	return run.Note, true
}

// restoreRunNote puts the pre-request note back only while the run still runs and still shows the
// approval note; a run that ended during the wait keeps the note its end path wrote.
func (b *TaskBoard) restoreRunNote(taskID, runID, note string) {
	mu := b.taskMu(taskID)
	mu.Lock()
	defer mu.Unlock()
	run, err := b.deps.Tasks.GetRun(runID)
	if err != nil || run.State != model.AdeRunRunning || run.Note != noteBranchApproval {
		return
	}
	updated, err := b.deps.Tasks.UpdateRun(runID, model.AdeRunPatch{Note: &note})
	if err != nil {
		slog.Warn("ade: branch request run note", "scope", "ade", "run", runID, "err", err)
		return
	}
	b.emitRuns(updated)
}

// finishBranchRequest maps the user's decision to a result per repo; approved repos are created under
// the task lock, re-checked since the wait.
func (b *TaskBoard) finishBranchRequest(ctx context.Context, call agentmcp.SpaceCall, slots []requestSlot, dec branchDecision) (agentmcp.RequestBranchesResult, error) {
	if err := ctx.Err(); err != nil {
		return agentmcp.RequestBranchesResult{}, err // the caller is gone: nothing is created for it
	}
	if dec.kind == decisionAbandoned {
		return agentmcp.RequestBranchesResult{}, agentmcp.ToolError("The branch request was cancelled.")
	}
	var apply func(*requestSlot)
	switch dec.kind {
	case decisionTimeout:
		next := fmt.Sprintf("No answer from the user in %s; nothing was created. Call request_branches again later or ask the user.", waitText(b.branchRequestWait()))
		apply = func(s *requestSlot) { s.result = outcomeBranch(s, agentmcp.BranchTimeout, next) }
	case decisionDenied:
		apply = func(s *requestSlot) { s.result = outcomeBranch(s, agentmcp.BranchDenied, deniedText) }
	default:
		mu := b.taskMu(call.TaskID)
		mu.Lock()
		defer mu.Unlock()
		tc, err := b.spaceCtx(call.TaskID)
		apply = func(s *requestSlot) {
			if !dec.approved[s.codeRepoID] {
				s.result = outcomeBranch(s, agentmcp.BranchDenied, deniedText)
				return
			}
			if err != nil {
				s.result = failedBranch(s.nick, "", toolMessage(err))
				return
			}
			s.result = b.createApproved(ctx, tc, s)
		}
	}
	for i := range slots {
		if slots[i].item != nil {
			apply(&slots[i])
		}
	}
	b.notifyBoard()
	return b.branchResults(slots), nil
}

const deniedText = "The user declined this branch. Do not create it with git; continue without it or ask the user."

func outcomeBranch(s *requestSlot, outcome, next string) *agentmcp.RequestedBranch {
	return &agentmcp.RequestedBranch{BranchInfo: noBranchInfo(s.nick, s.item.Branch, next), Outcome: outcome}
}

func toolMessage(err error) string {
	var te agentmcp.ToolError
	if errors.As(toolErr(err), &te) {
		return te.Error()
	}
	return "Kira Space could not create this branch."
}

// createApproved re-reads the branch row, since the task may have changed during the wait, and creates it.
func (b *TaskBoard) createApproved(ctx context.Context, tc *taskCtx, s *requestSlot) *agentmcp.RequestedBranch {
	var sb model.AdeTaskBranch
	found := false
	for _, br := range tc.branches {
		if br.CodeRepoID == s.codeRepoID {
			sb, found = br, true
		}
	}
	if !found && s.addRow {
		sb, err := b.deps.Tasks.AddBranch(model.AdeTaskBranch{
			ID: b.newID(), TaskID: tc.task.ID, CodeRepoID: s.codeRepoID, Kind: model.AdeBranchKindMine,
			AddedAt: b.deps.Now().UnixMilli(), Origin: model.AdeBranchOriginAgent,
		})
		if err != nil {
			slog.Warn("ade: add requested repo", "scope", "ade", "repo", s.codeRepoID, "err", err)
			return failedBranch(s.nick, "", "Kira Space could not add this repo to the task.")
		}
		return b.createFor(ctx, s, sb)
	}
	if !found {
		return failedBranch(s.nick, "", fmt.Sprintf("Repo %s is no longer on this task.", s.nick))
	}
	return b.createFor(ctx, s, sb)
}

func (b *TaskBoard) createFor(ctx context.Context, s *requestSlot, sb model.AdeTaskBranch) *agentmcp.RequestedBranch {
	rb, err := b.createBranch(ctx, sb, s.nick)
	if err != nil {
		var te agentmcp.ToolError
		if errors.As(err, &te) {
			return failedBranch(s.nick, sb.Name, te.Error())
		}
		slog.Warn("ade: create requested branch", "scope", "ade", "repo", s.codeRepoID, "err", err)
		return failedBranch(s.nick, sb.Name, "Kira Space could not create this branch.")
	}
	return &rb
}
