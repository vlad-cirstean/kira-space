package ade

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitsession"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/oplog"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/agentmcp"
	"github.com/kirathecat/kira-studio/internal/approvals"
	"github.com/kirathecat/kira-studio/internal/ids"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
	"github.com/kirathecat/kira-studio/packages/theme/src/i18n"
	"golang.org/x/sync/errgroup"
)

// branchrequest.go: an agent asks the user to approve branches and worktrees (P408). The call blocks
// without holding the task lock. A task has one open request, one approvals item; concurrent calls for the
// task merge their items into it and one answer settles every call (P520).

const (
	defaultBranchRequestWait = 30 * time.Minute

	branchKindBranch   = "branch"
	branchKindWorktree = "worktree"
	branchKindSwitch   = "switch"

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
	// remote holds the rename items whose same-named remote branch the user also wants renamed.
	remote map[string]bool
}

// branchCall is one request_branches or rename_branch call merged into a request. Its items carry the
// id prefix.
type branchCall struct {
	prefix   string
	deadline time.Time           // the call's own wait ends here; the request never outlives its oldest live call
	result   chan branchDecision // buffered 1: the one resolver never blocks
}

type branchRequest struct {
	view  adewire.BranchRequest
	calls []*branchCall
	next  int // call counter; item ids are never renumbered
}

// branchRequests holds the open requests, at most one per task. Items open and close under mu, so a
// fast answer cannot close an item before it opened.
type branchRequests struct {
	mu     sync.Mutex
	byID   map[string]*branchRequest
	byTask map[string]string
}

// requestDraft is what one call asks of the user, before it joins the task's request.
type requestDraft struct {
	title, source, step, session string
	items                        []adewire.BranchRequestItem // ids are call-local, "i<k>"
}

func branchItemID(requestID string) string { return approvals.KindBranchRequest + ":" + requestID }

type branchPayload struct {
	TaskID    string                      `json:"taskId"`
	TaskTitle string                      `json:"taskTitle"`
	Items     []adewire.BranchRequestItem `json:"items"`
}

// itemOfBranchRequest projects the request into its approvals item.
func itemOfBranchRequest(v adewire.BranchRequest) approvals.Item {
	class := "request"
	kinds := map[string]bool{}
	for _, it := range v.Items {
		kinds[it.Kind] = true
	}
	switch {
	case len(kinds) == 1 && kinds[branchKindRename]:
		class = branchKindRename
	case len(kinds) == 1 && kinds[branchKindSwitch]:
		class = "change"
	case len(kinds) > 0 && !kinds[branchKindRename]:
		class = "create"
	}
	raw, err := json.Marshal(branchPayload{TaskID: v.TaskID, TaskTitle: v.TaskTitle, Items: v.Items})
	if err != nil {
		// Plain strings and slices only; a failure is a programming error.
		panic("ade: branch request payload: " + err.Error())
	}
	it := approvals.Item{
		ID: branchItemID(v.ID), Kind: approvals.KindBranchRequest, CreatedAt: v.CreatedAt, ExpiresAt: v.ExpiresAt,
		Origin:    approvals.Origin{Task: v.TaskTitle},
		Target:    approvals.Target{Name: v.TaskTitle, Kind: "task"},
		Operation: approvals.Operation{Class: class, Tone: "caution"},
		Reason:    "request", Payload: raw,
		Title: i18n.T("backend.prompt.branchRequest.title", map[string]string{"task": v.TaskTitle}),
	}
	if len(v.Items) > 0 {
		it.Origin.Source, it.Origin.Step = v.Items[0].Source, v.Items[0].StepName
	}
	return it
}

// branchOwner answers branch.request items for the approvals queue.
type branchOwner struct{ b *TaskBoard }

// Answer approves or denies the request. Options are {itemIds, remoteItemIds, shownItemIds}; none approves
// every item with the remote rename off. A call with an item outside shownItemIds joined after the user's
// view rendered; it stays pending.
func (o branchOwner) Answer(ref string, a approvals.Answer) error {
	var opts struct {
		ItemIDs       []string `json:"itemIds"`
		RemoteItemIDs []string `json:"remoteItemIds"`
		ShownItemIDs  []string `json:"shownItemIds"`
	}
	if len(a.Options) > 0 {
		if err := json.Unmarshal(a.Options, &opts); err != nil {
			return ipcerr.Contract("branch request options: %v", err)
		}
	}
	if !a.Approve {
		if !o.b.DenyBranchRequest(ref, opts.ShownItemIDs) {
			return ipcerr.NotFoundNamed("approval")
		}
		return nil
	}
	resolved, err := o.b.ApproveBranchRequest(ref, opts.ItemIDs, opts.RemoteItemIDs, opts.ShownItemIDs)
	switch {
	case errors.Is(err, ErrInvalidInput):
		return ipcerr.Contract("%s", trimInvalid(err))
	case err != nil:
		return err
	case !resolved:
		return ipcerr.NotFoundNamed("approval")
	}
	return nil
}

func (b *TaskBoard) putBranchRequestLocked(r *branchRequest) {
	if b.deps.Approvals != nil {
		b.deps.Approvals.Put(itemOfBranchRequest(r.view))
	}
}

func (b *TaskBoard) dropBranchRequestLocked(id string) {
	if b.deps.Approvals != nil {
		b.deps.Approvals.Remove(branchItemID(id))
	}
}

// joinBranchRequest adds a call's items to the task's open request, or opens one. Each call keeps its own
// deadline; the request's expiry is the oldest live call's, and the item is re-put so the countdown follows.
func (b *TaskBoard) joinBranchRequest(taskID string, d *requestDraft) (reqID string, bc *branchCall) {
	q := &b.branchReqs
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.byID == nil {
		q.byID, q.byTask = map[string]*branchRequest{}, map[string]string{}
	}
	now := b.deps.Now()
	r := q.byID[q.byTask[taskID]]
	if r == nil {
		r = &branchRequest{view: adewire.BranchRequest{ID: ids.NewID(), TaskID: taskID, TaskTitle: d.title, CreatedAt: now.UnixMilli()}}
		q.byID[r.view.ID], q.byTask[taskID] = r, r.view.ID
	}
	bc = &branchCall{prefix: fmt.Sprintf("c%d", r.next), deadline: now.Add(b.branchRequestWait()), result: make(chan branchDecision, 1)}
	r.next++
	for _, it := range d.items {
		it.ID = bc.prefix + it.ID
		it.Source, it.StepName, it.Session = d.source, d.step, d.session
		r.view.Items = append(r.view.Items, it)
	}
	r.calls = append(r.calls, bc)
	refreshBranchExpiry(r)
	b.putBranchRequestLocked(r)
	return r.view.ID, bc
}

// refreshBranchExpiry sets the request's expiry to its earliest live call deadline.
func refreshBranchExpiry(r *branchRequest) {
	for i, c := range r.calls {
		if ms := c.deadline.UnixMilli(); i == 0 || ms < r.view.ExpiresAt {
			r.view.ExpiresAt = ms
		}
	}
}

// resolveBranchRequest removes the request, withdraws its item and hands every merged call the decision;
// false when it was already resolved. Only the first resolver wins.
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
	b.dropBranchRequestLocked(id)
	for _, c := range r.calls {
		c.result <- d
	}
	return true
}

// withdrawBranchCall ends one call with kind (abandoned or timeout): its items go, the others keep waiting,
// and the request closes with its last call. A call already answered keeps its answer.
func (b *TaskBoard) withdrawBranchCall(id string, bc *branchCall, kind decisionKind) {
	q := &b.branchReqs
	q.mu.Lock()
	defer q.mu.Unlock()
	r := q.byID[id]
	if r == nil || !slices.Contains(r.calls, bc) {
		return
	}
	b.removeCallsLocked(r, branchDecision{kind: kind}, func(c *branchCall) bool { return c == bc })
}

// removeCallsLocked answers the calls pick selects with d and drops their items. The request closes when no
// call is left; otherwise its expiry follows the remaining calls.
func (b *TaskBoard) removeCallsLocked(r *branchRequest, d branchDecision, pick func(*branchCall) bool) {
	q := &b.branchReqs
	var kept []*branchCall
	for _, c := range r.calls {
		if !pick(c) {
			kept = append(kept, c)
			continue
		}
		prefix := c.prefix + "i"
		r.view.Items = slices.DeleteFunc(r.view.Items, func(it adewire.BranchRequestItem) bool { return strings.HasPrefix(it.ID, prefix) })
		c.result <- d
	}
	r.calls = kept
	if len(kept) == 0 {
		delete(q.byID, r.view.ID)
		delete(q.byTask, r.view.TaskID)
		b.dropBranchRequestLocked(r.view.ID)
		return
	}
	refreshBranchExpiry(r)
	b.putBranchRequestLocked(r)
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

// ApproveBranchRequest approves the listed items, a non-empty subset of the request's items; nil approves
// every shown item. The rest of the shown items are denied. remoteIDs name rename items whose remote branch
// moves too. shownIDs are the item ids the user's view held; nil means the items open now. A call with an
// item outside that set joined after the view rendered and stays pending. resolved is false when the
// request is already gone.
func (b *TaskBoard) ApproveBranchRequest(id string, itemIDs, remoteIDs, shownIDs []string) (bool, error) {
	q := &b.branchReqs
	q.mu.Lock()
	defer q.mu.Unlock()
	r := q.byID[id]
	if r == nil {
		return false, nil
	}
	known := func(iid string) bool {
		return slices.ContainsFunc(r.view.Items, func(it adewire.BranchRequestItem) bool { return it.ID == iid })
	}
	shown := shownSet(r, shownIDs)
	if itemIDs == nil {
		for _, it := range r.view.Items {
			if shown[it.ID] {
				itemIDs = append(itemIDs, it.ID)
			}
		}
	}
	if len(itemIDs) == 0 {
		return false, invalid("approve at least one branch")
	}
	dec := branchDecision{kind: decisionApproved, approved: make(map[string]bool, len(itemIDs)), remote: map[string]bool{}}
	for _, iid := range itemIDs {
		if !known(iid) {
			return false, invalid("item %s is not in this request", iid)
		}
		dec.approved[iid] = true
	}
	for _, iid := range remoteIDs {
		if !known(iid) {
			return false, invalid("item %s is not in this request", iid)
		}
		dec.remote[iid] = true
	}
	b.removeCallsLocked(r, dec, seenCalls(r, shown))
	return true, nil
}

// shownSet is the item ids the user's view held; nil means the items open now.
func shownSet(r *branchRequest, shownIDs []string) map[string]bool {
	shown := make(map[string]bool, len(r.view.Items))
	if shownIDs == nil {
		for _, it := range r.view.Items {
			shown[it.ID] = true
		}
		return shown
	}
	for _, iid := range shownIDs {
		shown[iid] = true
	}
	return shown
}

// seenCalls selects the calls whose items the user's view all held; a call that joined later stays pending.
func seenCalls(r *branchRequest, shown map[string]bool) func(*branchCall) bool {
	return func(c *branchCall) bool {
		prefix := c.prefix + "i"
		for _, it := range r.view.Items {
			if strings.HasPrefix(it.ID, prefix) && !shown[it.ID] {
				return false
			}
		}
		return true
	}
}

// DenyBranchRequest denies the calls whose items the user's view held (shownIDs; nil means the items open
// now); resolved is false when the request is already gone.
func (b *TaskBoard) DenyBranchRequest(id string, shownIDs []string) bool {
	q := &b.branchReqs
	q.mu.Lock()
	defer q.mu.Unlock()
	r := q.byID[id]
	if r == nil {
		return false
	}
	b.removeCallsLocked(r, branchDecision{kind: decisionDenied}, seenCalls(r, shownSet(r, shownIDs)))
	return true
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
	asked      string // base the agent asked for, trimmed; "" for none
	note       string // prefix for Next when the branch exists with another base
	result     *agentmcp.RequestedBranch
	item       *adewire.BranchRequestItem
}

func noBranchInfo(nick, branch, base, next string) agentmcp.BranchInfo {
	return agentmcp.BranchInfo{Repo: nick, Branch: branch, Base: base, Setup: agentmcp.SetupInfo{State: "none"}, Next: next}
}

func failedBranch(nick, branch, base, text string) *agentmcp.RequestedBranch {
	return &agentmcp.RequestedBranch{BranchInfo: noBranchInfo(nick, branch, base, text), Outcome: agentmcp.BranchFailed}
}

func waitText(d time.Duration) string {
	if d >= time.Minute {
		return fmt.Sprintf("%d minutes", int(d/time.Minute))
	}
	return fmt.Sprintf("%d seconds", int(d/time.Second))
}

// RequestBranches implements agentmcp.SpaceTools. Branches whose worktree exists answer at once.
// The rest go to the user in the task's one request and the call waits for the answer, unlocked. ctx ends
// when the caller is gone, which withdraws this call's items.
func (b *TaskBoard) RequestBranches(ctx context.Context, call agentmcp.SpaceCall, specs []agentmcp.BranchSpec) (agentmcp.RequestBranchesResult, error) {
	slots, draft, err := b.planBranchRequest(ctx, call, specs)
	if err != nil {
		return agentmcp.RequestBranchesResult{}, err
	}
	if draft == nil {
		return b.branchResults(slots), nil
	}
	dec, prefix := b.awaitBranchRequest(ctx, call, draft)
	for i := range slots {
		if slots[i].item != nil {
			slots[i].id = prefix + slots[i].id
		}
	}
	return b.finishBranchRequest(ctx, call, slots, dec)
}

// awaitBranchRequest joins the task's request, notes the wait on the calling run and blocks, unlocked,
// until the user decides, the wait runs out or the caller is gone. prefix is the call's item id prefix.
func (b *TaskBoard) awaitBranchRequest(ctx context.Context, call agentmcp.SpaceCall, d *requestDraft) (dec branchDecision, prefix string) {
	reqID, bc := b.joinBranchRequest(call.TaskID, d)
	op := b.startAgentOp(ctx, opAgentApproval, call.TaskID, approvalCommand(d.title, len(d.items)))
	noted := b.holdRunNote(call.RunID, noteBranchApproval)
	b.notifyBoard()

	timer := time.NewTimer(b.branchRequestWait())
	defer timer.Stop()
wait:
	for {
		select {
		case dec = <-bc.result:
			break wait
		case <-timer.C:
			b.withdrawBranchCall(reqID, bc, decisionTimeout)
			dec = <-bc.result
			break wait
		case <-ctx.Done():
			b.withdrawBranchCall(reqID, bc, decisionAbandoned)
			dec = <-bc.result
			break wait
		}
	}
	finishApprovalOp(op, dec.kind)
	if noted {
		b.releaseRunNote(call.TaskID, call.RunID)
	}
	return dec, bc.prefix
}

func (b *TaskBoard) branchResults(slots []requestSlot) agentmcp.RequestBranchesResult {
	out := agentmcp.RequestBranchesResult{Branches: make([]agentmcp.RequestedBranch, 0, len(slots))}
	for _, s := range slots {
		out.Branches = append(out.Branches, *s.result)
	}
	return out
}

// planBranchRequest validates the call under the task lock. req is nil when no branch needs the user.
func (b *TaskBoard) planBranchRequest(ctx context.Context, call agentmcp.SpaceCall, specs []agentmcp.BranchSpec) ([]requestSlot, *requestDraft, error) {
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
	type asked struct {
		base     string
		existing bool
	}
	seen := map[string]asked{} // repo and branch to the first entry's base and kind
	for i, sp := range specs {
		id, err := resolveRepoRef(sp.Repo, cands, "registered")
		if err != nil {
			return nil, nil, err
		}
		key := id + "\x00" + sp.Branch
		if first, dup := seen[key]; dup {
			if first.base != sp.Base || first.existing != sp.Existing {
				return nil, nil, agentmcp.ToolError(fmt.Sprintf("Branch %s of %s is asked twice with different bases or existing flags.", sp.Branch, sp.Repo))
			}
			continue
		}
		seen[key] = asked{base: sp.Base, existing: sp.Existing}
		slot, err := b.planItem(ctx, tc, fmt.Sprintf("i%d", i), id, sp.Branch, sp.Base, sp.Existing, cfgs)
		if err != nil {
			return nil, nil, err
		}
		slots = append(slots, slot)
		if slot.item != nil {
			slot.item.Stacked = []string{}
			items = append(items, *slot.item)
		}
	}
	if len(items) == 0 {
		return slots, nil, nil
	}
	return slots, b.requestDraftOf(tc, call, items), nil
}

// requestDraftOf names who asks for items.
func (b *TaskBoard) requestDraftOf(tc *taskCtx, call agentmcp.SpaceCall, items []adewire.BranchRequestItem) *requestDraft {
	source, stepName := b.callSource(tc, call)
	return &requestDraft{title: taskTitle(tc.task, tc.branches), source: source, step: stepName, session: call.SessionID, items: items}
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

const outsideTaskText = "Branch %s exists outside this task. Pick another name, or pass existing: true to work on it."

// planItem decides one requested branch: failed, already existing, or an item for the user. asked is the
// base the agent chose, "" for none.
func (b *TaskBoard) planItem(ctx context.Context, tc *taskCtx, itemID, codeRepoID, branch, asked string, existing bool, cfgs []model.AdeRepoConfig) (requestSlot, error) {
	nick := tc.nick[codeRepoID]
	if nick == "" {
		for _, c := range cfgs {
			if c.CodeRepoID == codeRepoID {
				nick = cmpNonEmpty(c.Nickname, c.Name)
			}
		}
	}
	slot := requestSlot{id: itemID, codeRepoID: codeRepoID, nick: nick, branch: branch, asked: asked}
	rec, err := b.deps.CodeRepos.Get(codeRepoID)
	if err != nil {
		return slot, err
	}
	if rec == nil {
		slot.result = failedBranch(nick, branch, asked, fmt.Sprintf("Repo %s is no longer registered.", nick))
		return slot, nil
	}
	if existing && asked != "" {
		slot.result = failedBranch(nick, branch, asked, "Cannot use base with existing: true; base only applies to a new branch.")
		return slot, nil
	}
	named, draft := taskRepoRows(tc, codeRepoID, branch)
	switch {
	case named != nil:
		return b.planNamed(ctx, slot, itemID, *named)
	case existing:
		return b.planExisting(ctx, slot, *rec)
	}
	return b.planNew(ctx, tc, slot, *rec, draft)
}

// planNamed decides a branch the task already has a row for. Its base stays as it is; a different asked
// base is reported in Next, never refused, so a retry of the same call stays idempotent.
func (b *TaskBoard) planNamed(ctx context.Context, slot requestSlot, itemID string, named model.AdeTaskBranch) (requestSlot, error) {
	item := adewire.BranchRequestItem{ID: itemID, CodeRepoID: slot.codeRepoID, Repo: slot.nick, Kind: branchKindWorktree, Branch: slot.branch}
	if named.Kind != model.AdeBranchKindMine {
		slot.result = failedBranch(slot.nick, slot.branch, slot.asked, fmt.Sprintf("The branch %s of %s on this task is not yours to create.", slot.branch, slot.nick))
		return slot, nil
	}
	if slot.asked != "" {
		note, err := b.keptBaseNote(ctx, named, slot.asked)
		if err != nil {
			return slot, err
		}
		slot.note = note
	}
	at, err := b.rowWorktree(ctx, named)
	if err != nil {
		return slot, err
	}
	if at.attached != "" {
		slot.result = &agentmcp.RequestedBranch{BranchInfo: b.branchInfo(named, slot.nick, at.attached), Outcome: agentmcp.BranchExisting}
		slot.result.Next = slot.note + slot.result.Next
		return slot, nil
	}
	if at.owner != nil {
		// Another task's worktree holds the branch: taking it back detaches it there.
		rec, err := b.deps.CodeRepos.Get(slot.codeRepoID)
		if err != nil || rec == nil {
			return slot, err
		}
		return b.planTake(ctx, slot, *rec, &named)
	}
	slot.item = &item
	return slot, nil
}

// keptBaseNote is the Next prefix for an existing branch whose base differs from asked, "" when they match.
func (b *TaskBoard) keptBaseNote(ctx context.Context, named model.AdeTaskBranch, asked string) (string, error) {
	entry, err := b.openRepo(ctx, named.CodeRepoID)
	if err != nil {
		return "", err
	}
	sc, err := b.baseCtxFor(ctx, entry, named.CodeRepoID)
	if err != nil {
		return "", err
	}
	current := sc.resolveBase(named).name
	if current == asked {
		return "", nil
	}
	return fmt.Sprintf("This branch exists and keeps its base %s; base %s was not applied. Ask the user to change it if needed. ", current, asked), nil
}

// planNew decides a branch with no row yet: name and base checks, then an item for the user.
func (b *TaskBoard) planNew(ctx context.Context, tc *taskCtx, slot requestSlot, rec model.CodeRepo, draft *model.AdeTaskBranch) (requestSlot, error) {
	nick, branch, asked := slot.nick, slot.branch, slot.asked
	item := adewire.BranchRequestItem{ID: slot.id, CodeRepoID: slot.codeRepoID, Repo: nick, Kind: branchKindWorktree, Branch: branch}
	refusal, err := b.checkNewBranchName(ctx, rec, branch)
	if err != nil {
		return slot, err
	}
	if refusal != "" {
		slot.result = failedBranch(nick, branch, asked, refusal)
		return slot, nil
	}
	entry, err := b.openRepo(ctx, slot.codeRepoID)
	if err != nil {
		return slot, err
	}
	inv, err := entry.BranchInventory(ctx)
	if err != nil {
		return slot, err
	}
	if branchExists(inv, branch) {
		slot.result = failedBranch(nick, branch, asked, fmt.Sprintf(outsideTaskText, branch))
		return slot, nil
	}
	base := model.AdeTaskBranch{TaskID: tc.task.ID, CodeRepoID: slot.codeRepoID, Kind: model.AdeBranchKindMine}
	if draft != nil {
		base = *draft
	}
	if asked != "" {
		refusal, err := b.baseNameRefusal(ctx, rec, asked)
		if err != nil {
			return slot, err
		}
		if refusal == "" {
			var t baseTarget
			if t, refusal, err = b.agentBase(ctx, entry, slot.codeRepoID, nick, draft, asked); err != nil {
				return slot, err
			}
			base.Base, base.BaseBranchID = t.base, t.baseBranchID
		}
		if refusal != "" {
			slot.result = failedBranch(nick, branch, asked, refusal)
			return slot, nil
		}
	}
	_, display, refusal, err := b.newBranchBase(ctx, entry, base)
	if err != nil {
		return slot, err
	}
	if refusal != "" {
		slot.result = failedBranch(nick, branch, asked, "Cannot create a branch here: "+refusal+".")
		return slot, nil
	}
	item.Kind, item.Base = branchKindBranch, display
	slot.item = &item
	return slot, nil
}

// baseNameRefusal says why asked cannot be a git branch name, "" when it can.
func (b *TaskBoard) baseNameRefusal(ctx context.Context, rec model.CodeRepo, asked string) (string, error) {
	refusal, err := b.branchNameRefusal(ctx, rec, asked)
	if refusal == "" || err != nil {
		return "", err
	}
	return fmt.Sprintf("Cannot use base %s: %s", asked, refusal), nil
}

// agentBase resolves the base an agent asked for as the base of the repo's waiting row draft (nil when the
// task has none). refusal is the text for the agent; err is a git or DB fault.
func (b *TaskBoard) agentBase(ctx context.Context, entry *gitsession.RepoEntry, codeRepoID, nick string, draft *model.AdeTaskBranch, asked string) (t baseTarget, refusal string, err error) {
	sc, err := b.baseCtxFor(ctx, entry, codeRepoID)
	if err != nil {
		return baseTarget{}, "", err
	}
	t, err = b.resolveChoice(sc, draft, nick, adewire.BaseChoice{Ref: asked})
	switch {
	case errors.Is(err, ErrInvalidInput):
		text := fmt.Sprintf("Cannot use base %s: %s.", asked, trimInvalid(err))
		if strings.Contains(text, " not found in ") {
			text += " Give an existing branch; a branch asked for in this same call must be created first."
		}
		return baseTarget{}, text, nil
	case err != nil:
		return baseTarget{}, "", err
	case t.reason == baseParentDraft:
		return baseTarget{}, fmt.Sprintf("Cannot use base %s: it is not created yet.", asked), nil
	}
	if draft != nil {
		set := draft.Base
		if set == sc.mainName && draft.BaseBranchID == "" {
			set = ""
		}
		if (set != "" || draft.BaseBranchID != "") && (set != t.base || draft.BaseBranchID != t.baseBranchID) {
			return baseTarget{}, fmt.Sprintf("The task's %s branch starts from %s, set by the user. Omit base, or use %s.", nick, set, set), nil
		}
	}
	return t, "", nil
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

// finishBranchRequest maps the user's decision to a result per repo; approved repos are created under
// the task lock, re-checked since the wait.
func (b *TaskBoard) finishBranchRequest(ctx context.Context, call agentmcp.SpaceCall, slots []requestSlot, dec branchDecision) (agentmcp.RequestBranchesResult, error) {
	if err := ctx.Err(); err != nil {
		return agentmcp.RequestBranchesResult{}, err // the caller is gone: nothing is created for it
	}
	if dec.kind == decisionAbandoned {
		return agentmcp.RequestBranchesResult{}, agentmcp.ToolError("The branch request was cancelled.")
	}
	switch dec.kind {
	case decisionTimeout:
		next := fmt.Sprintf("No answer from the user in %s; nothing was created. Call request_branches again later or ask the user.", waitText(b.branchRequestWait()))
		for i := range slots {
			if slots[i].item != nil {
				slots[i].result = outcomeBranch(&slots[i], agentmcp.BranchTimeout, next)
			}
		}
	case decisionDenied:
		for i := range slots {
			if slots[i].item != nil {
				slots[i].result = outcomeBranch(&slots[i], agentmcp.BranchDenied, deniedText)
			}
		}
	default:
		mu := b.taskMu(call.TaskID)
		mu.Lock()
		defer mu.Unlock()
		b.createApprovedSlots(ctx, call, slots, dec)
	}
	b.notifyBoard()
	return b.branchResults(slots), nil
}

// createApprovedSlots answers every slot that awaited the user, under the task lock the caller holds.
// Approved slots of different repos are created in parallel: each repo's worktree add is git work on
// its own repo, serialised per repo by repoMutex. Slots of one repo stay in order, since
// createApproved may name the repo's draft row. The caller's goroutine keeps the task lock until all
// workers return; no worker takes it.
func (b *TaskBoard) createApprovedSlots(ctx context.Context, call agentmcp.SpaceCall, slots []requestSlot, dec branchDecision) {
	tc, err := b.spaceCtx(call.TaskID)
	var order []string
	groups := map[string][]*requestSlot{}
	for i := range slots {
		s := &slots[i]
		switch {
		case s.item == nil:
		case !dec.approved[s.id]:
			s.result = outcomeBranch(s, agentmcp.BranchDenied, deniedText)
		case err != nil:
			s.result = failedBranch(s.nick, s.branch, s.item.Base, toolMessage(err))
		default:
			if groups[s.codeRepoID] == nil {
				order = append(order, s.codeRepoID)
			}
			groups[s.codeRepoID] = append(groups[s.codeRepoID], s)
		}
	}
	if len(order) == 0 {
		return
	}
	var nicks []string
	for i := range slots {
		if s := &slots[i]; groups[s.codeRepoID] != nil && !slices.Contains(nicks, s.nick) && slices.Contains(groups[s.codeRepoID], s) {
			nicks = append(nicks, s.nick)
		}
	}
	op := b.startAgentOp(ctx, opAgentBranches, call.TaskID, strings.Join(nicks, ", "))
	var g errgroup.Group
	g.SetLimit(boardFanOut)
	for _, id := range order {
		g.Go(func() error {
			// createApproved writes a repo's draft row in tc.branches; a clone keeps that off the other
			// workers' reads of the slice.
			own := *tc
			own.branches = slices.Clone(tc.branches)
			for _, s := range groups[id] {
				s.result = b.createApproved(ctx, &own, s)
				if s.result.Outcome == agentmcp.BranchCreated || s.result.Outcome == agentmcp.BranchExisting || s.result.Outcome == agentmcp.BranchCheckedOut {
					s.result.Next = s.note + s.result.Next
				}
			}
			return nil
		})
	}
	_ = g.Wait() // results land in the slots
	var failed []string
	for _, id := range order {
		for _, s := range groups[id] {
			switch s.result.Outcome {
			case agentmcp.BranchCreated, agentmcp.BranchExisting, agentmcp.BranchCheckedOut:
			case agentmcp.BranchFailed:
				if !slices.Contains(failed, s.nick) {
					failed = append(failed, s.nick)
				}
			}
		}
	}
	if len(failed) > 0 {
		op.Finish(oplog.StatusError, "failed: "+strings.Join(failed, ", "))
		return
	}
	op.Finish(oplog.StatusOK, "")
}

const deniedText = "The user declined this branch. Do not create it with git; continue without it or ask the user."

func outcomeBranch(s *requestSlot, outcome, next string) *agentmcp.RequestedBranch {
	return &agentmcp.RequestedBranch{BranchInfo: noBranchInfo(s.nick, s.item.Branch, s.item.Base, next), Outcome: outcome}
}

func toolMessage(err error) string {
	var te agentmcp.ToolError
	if errors.As(toolErr(err), &te) {
		return te.Error()
	}
	return "Kira Space could not create this branch."
}

// createApproved re-reads the task rows, since the task may have changed during the wait, and creates the
// branch: in the repo's waiting row when it has one, else in a new row. A base the agent asked for is
// resolved again and stored on the row first; the user approved it, so a failed start leaves it there.
func (b *TaskBoard) createApproved(ctx context.Context, tc *taskCtx, s *requestSlot) *agentmcp.RequestedBranch {
	if s.item.Kind == branchKindCheckout || s.item.Kind == branchKindAdopt {
		return b.applyExisting(ctx, tc, s)
	}
	named, draft := taskRepoRows(tc, s.codeRepoID, s.branch)
	switch {
	case named != nil:
		return b.createFor(ctx, s, *named)
	case s.item.Kind == branchKindWorktree:
		return failedBranch(s.nick, s.branch, s.item.Base, fmt.Sprintf("Branch %s is no longer on this task.", s.branch))
	}
	var t baseTarget
	if s.asked != "" {
		entry, err := b.openRepo(ctx, s.codeRepoID)
		if err == nil {
			var refusal string
			if t, refusal, err = b.agentBase(ctx, entry, s.codeRepoID, s.nick, draft, s.asked); refusal != "" {
				return failedBranch(s.nick, s.branch, s.item.Base, refusal)
			}
		}
		if err != nil {
			slog.Warn("ade: resolve requested base", "scope", "ade", "repo", s.codeRepoID, "err", err)
			return failedBranch(s.nick, s.branch, s.item.Base, "Kira Space could not resolve the base of this branch.")
		}
	}
	if draft != nil {
		if s.asked != "" {
			if err := b.deps.Tasks.SetBranchBase(draft.ID, t.base, t.baseBranchID, ""); err != nil {
				slog.Warn("ade: store requested base", "scope", "ade", "repo", s.codeRepoID, "err", err)
				return failedBranch(s.nick, s.branch, s.item.Base, "Kira Space could not store the base of this branch.")
			}
			draft.Base, draft.BaseBranchID = t.base, t.baseBranchID
		}
		return b.createFor(ctx, s, *draft)
	}
	sb, err := b.deps.Tasks.AddBranch(model.AdeTaskBranch{
		ID: ids.NewID(), TaskID: tc.task.ID, CodeRepoID: s.codeRepoID, Kind: model.AdeBranchKindMine,
		Base: t.base, BaseBranchID: t.baseBranchID,
		AddedAt: b.deps.Now().UnixMilli(), Origin: model.AdeBranchOriginAgent,
	})
	if err != nil {
		slog.Warn("ade: add requested branch", "scope", "ade", "repo", s.codeRepoID, "err", err)
		return failedBranch(s.nick, s.branch, s.item.Base, "Kira Space could not add this branch to the task.")
	}
	return b.createFor(ctx, s, sb)
}

func (b *TaskBoard) createFor(ctx context.Context, s *requestSlot, sb model.AdeTaskBranch) *agentmcp.RequestedBranch {
	rb, err := b.createBranch(ctx, sb, s.nick, s.branch)
	if err != nil {
		var te agentmcp.ToolError
		if errors.As(err, &te) {
			return failedBranch(s.nick, sb.Name, s.item.Base, te.Error())
		}
		slog.Warn("ade: create requested branch", "scope", "ade", "repo", s.codeRepoID, "err", err)
		return failedBranch(s.nick, sb.Name, s.item.Base, "Kira Space could not create this branch.")
	}
	return &rb
}
