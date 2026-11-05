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
	approved map[string]bool // item ids the user kept checked
	// renameRemote is the user's choice on a rename item: also rename its remote branch.
	renameRemote bool
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

// ApproveBranchRequest approves the checked items, a non-empty subset of the request's items; the rest
// are denied. renameRemote applies to a rename item. resolved is false when the request is already gone.
func (b *TaskBoard) ApproveBranchRequest(id string, itemIDs []string, renameRemote bool) (bool, error) {
	req, err := b.BranchRequest(id)
	if err != nil {
		return false, nil
	}
	if len(itemIDs) == 0 {
		return false, invalid("approve at least one branch")
	}
	approved := make(map[string]bool, len(itemIDs))
	for _, iid := range itemIDs {
		if !slices.ContainsFunc(req.Items, func(it adewire.BranchRequestItem) bool { return it.ID == iid }) {
			return false, invalid("item %s is not in this request", iid)
		}
		approved[iid] = true
	}
	return b.resolveBranchRequest(id, branchDecision{kind: decisionApproved, approved: approved, renameRemote: renameRemote}), nil
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

// requestSlot is one branch of a request_branches call: a final result, or an item awaiting the user.
type requestSlot struct {
	id         string
	codeRepoID string
	nick       string
	branch     string
	result     *agentmcp.RequestedBranch
	item       *adewire.BranchRequestItem
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

// RequestBranches implements agentmcp.SpaceTools. Branches whose worktree exists answer at once.
// The rest go to the user in one prompt and the call waits for the answer, unlocked. ctx ends when the
// caller is gone, which abandons the request.
func (b *TaskBoard) RequestBranches(ctx context.Context, call agentmcp.SpaceCall, specs []agentmcp.BranchSpec) (agentmcp.RequestBranchesResult, error) {
	slots, req, err := b.planBranchRequest(ctx, call, specs)
	if err != nil {
		return agentmcp.RequestBranchesResult{}, err
	}
	if req == nil {
		return b.branchResults(slots), nil
	}
	dec, err := b.awaitBranchRequest(ctx, call, req)
	if err != nil {
		return agentmcp.RequestBranchesResult{}, err
	}
	return b.finishBranchRequest(ctx, call, slots, dec)
}

// awaitBranchRequest raises the request, notes the wait on the calling run and blocks, unlocked, until the
// user decides, the wait runs out or the caller is gone.
func (b *TaskBoard) awaitBranchRequest(ctx context.Context, call agentmcp.SpaceCall, req *branchRequest) (branchDecision, error) {
	if !b.addBranchRequest(req) {
		return branchDecision{}, agentmcp.ToolError("A branch request for this task already waits for the user. Wait for its answer, then call again.")
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
	return dec, nil
}

func (b *TaskBoard) branchResults(slots []requestSlot) agentmcp.RequestBranchesResult {
	out := agentmcp.RequestBranchesResult{Branches: make([]agentmcp.RequestedBranch, 0, len(slots))}
	for _, s := range slots {
		out.Branches = append(out.Branches, *s.result)
	}
	return out
}

// planBranchRequest validates the call under the task lock. req is nil when no branch needs the user.
func (b *TaskBoard) planBranchRequest(ctx context.Context, call agentmcp.SpaceCall, specs []agentmcp.BranchSpec) ([]requestSlot, *branchRequest, error) {
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
	slots := make([]requestSlot, 0, len(specs))
	var items []adewire.BranchRequestItem
	seen := map[string]bool{}
	for i, sp := range specs {
		id, err := resolveRepoRef(sp.Repo, cands, "registered")
		if err != nil {
			return nil, nil, err
		}
		key := id + "\x00" + sp.Branch
		if seen[key] {
			continue
		}
		seen[key] = true
		slot, err := b.planItem(ctx, tc, fmt.Sprintf("i%d", i), id, sp.Branch, cfgs)
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

// taskRepoRows are the task's live rows in the repo: the one named branch, and the first row of the
// agent's own kind still waiting for a name.
func taskRepoRows(tc *taskCtx, codeRepoID, branch string) (named, draft *model.AdeTaskBranch) {
	for i, br := range tc.branches {
		if br.CodeRepoID != codeRepoID {
			continue
		}
		switch {
		case br.Name == branch:
			named = &tc.branches[i]
		case br.Name == "" && br.Kind == model.AdeBranchKindMine && draft == nil:
			draft = &tc.branches[i]
		}
	}
	return named, draft
}

const outsideTaskText = "Branch %s exists outside this task. Pick another name or ask the user to attach it."

// planItem decides one requested branch: failed, already existing, or an item for the user.
func (b *TaskBoard) planItem(ctx context.Context, tc *taskCtx, itemID, codeRepoID, branch string, cfgs []model.AdeRepoConfig) (requestSlot, error) {
	nick := tc.nick[codeRepoID]
	if nick == "" {
		for _, c := range cfgs {
			if c.CodeRepoID == codeRepoID {
				nick = cmpNonEmpty(c.Nickname, c.Name)
			}
		}
	}
	slot := requestSlot{id: itemID, codeRepoID: codeRepoID, nick: nick, branch: branch}
	rec, err := b.deps.CodeRepos.Get(codeRepoID)
	if err != nil {
		return slot, err
	}
	if rec == nil {
		slot.result = failedBranch(nick, branch, fmt.Sprintf("Repo %s is no longer registered.", nick))
		return slot, nil
	}
	named, draft := taskRepoRows(tc, codeRepoID, branch)
	item := adewire.BranchRequestItem{ID: itemID, CodeRepoID: codeRepoID, Repo: nick, Kind: branchKindWorktree, Branch: branch}
	if named != nil {
		if named.Kind != model.AdeBranchKindMine {
			slot.result = failedBranch(nick, branch, fmt.Sprintf("The branch %s of %s on this task is not yours to create.", branch, nick))
			return slot, nil
		}
		path, err := b.worktreeOf(ctx, *named)
		if err != nil {
			return slot, err
		}
		if path != "" {
			slot.result = &agentmcp.RequestedBranch{BranchInfo: b.branchInfo(*named, nick, path), Outcome: agentmcp.BranchExisting}
			return slot, nil
		}
		slot.item = &item
		return slot, nil
	}
	refusal, err := b.checkNewBranchName(ctx, *rec, branch)
	if err != nil {
		return slot, err
	}
	if refusal != "" {
		slot.result = failedBranch(nick, branch, refusal)
		return slot, nil
	}
	entry, err := b.openRepo(ctx, codeRepoID)
	if err != nil {
		return slot, err
	}
	inv, err := entry.BranchInventory(ctx)
	if err != nil {
		return slot, err
	}
	if branchExists(inv, branch) {
		slot.result = failedBranch(nick, branch, fmt.Sprintf(outsideTaskText, branch))
		return slot, nil
	}
	base := model.AdeTaskBranch{TaskID: tc.task.ID, CodeRepoID: codeRepoID, Kind: model.AdeBranchKindMine}
	if draft != nil {
		base = *draft
	}
	_, display, refusal, err := b.newBranchBase(ctx, entry, base)
	if err != nil {
		return slot, err
	}
	if refusal != "" {
		slot.result = failedBranch(nick, branch, "Cannot create a branch here: "+refusal+".")
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
			if !dec.approved[s.id] {
				s.result = outcomeBranch(s, agentmcp.BranchDenied, deniedText)
				return
			}
			if err != nil {
				s.result = failedBranch(s.nick, s.branch, toolMessage(err))
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

// createApproved re-reads the task rows, since the task may have changed during the wait, and creates the
// branch: in the repo's waiting row when it has one, else in a new row.
func (b *TaskBoard) createApproved(ctx context.Context, tc *taskCtx, s *requestSlot) *agentmcp.RequestedBranch {
	named, draft := taskRepoRows(tc, s.codeRepoID, s.branch)
	switch {
	case named != nil:
		return b.createFor(ctx, s, *named)
	case s.item.Kind == branchKindWorktree:
		return failedBranch(s.nick, s.branch, fmt.Sprintf("Branch %s is no longer on this task.", s.branch))
	case draft != nil:
		return b.createFor(ctx, s, *draft)
	}
	sb, err := b.deps.Tasks.AddBranch(model.AdeTaskBranch{
		ID: b.newID(), TaskID: tc.task.ID, CodeRepoID: s.codeRepoID, Kind: model.AdeBranchKindMine,
		AddedAt: b.deps.Now().UnixMilli(), Origin: model.AdeBranchOriginAgent,
	})
	if err != nil {
		slog.Warn("ade: add requested branch", "scope", "ade", "repo", s.codeRepoID, "err", err)
		return failedBranch(s.nick, s.branch, "Kira Space could not add this branch to the task.")
	}
	return b.createFor(ctx, s, sb)
}

func (b *TaskBoard) createFor(ctx context.Context, s *requestSlot, sb model.AdeTaskBranch) *agentmcp.RequestedBranch {
	rb, err := b.createBranch(ctx, sb, s.nick, s.branch)
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
