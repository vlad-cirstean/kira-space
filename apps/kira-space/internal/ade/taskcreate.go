package ade

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/agentmcp"
	"github.com/kirathecat/kira-studio/internal/approvals"
	"github.com/kirathecat/kira-studio/internal/ids"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
	"github.com/kirathecat/kira-studio/packages/theme/src/i18n"
)

// taskcreate.go: an orchestrator asks the user to approve a child task (P571). Each create_task call is
// one approvals item and blocks, without the task lock, until the user answers, the wait runs out or the
// caller is gone. Nothing caps the number of open calls: every child needs its own approval.

const (
	defaultTaskRequestWait = 30 * time.Minute

	noteTaskApproval = "waiting for task approval"
)

type taskVerdict int

const (
	taskApproved taskVerdict = iota
	taskDenied
	taskTimedOut
	taskAbandoned
)

// taskRequest is one open create_task call.
type taskRequest struct {
	ref      string
	parentID string
	item     approvals.Item
	result   chan taskVerdict // buffered 1: the one resolver never blocks
}

// taskRequests holds the open requests. Items open and close under mu, so a fast answer cannot close an
// item before it opened.
type taskRequests struct {
	mu   sync.Mutex
	byID map[string]*taskRequest
}

type taskCreateRepo struct {
	CodeRepoID string `json:"codeRepoId"`
	Name       string `json:"name"`
	Base       string `json:"base,omitempty"`
}

type taskCreateWorkflow struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type taskCreatePayload struct {
	ParentTaskID string             `json:"parentTaskId"`
	ParentTitle  string             `json:"parentTitle"`
	Title        string             `json:"title"`
	Workflow     taskCreateWorkflow `json:"workflow"`
	Repos        []taskCreateRepo   `json:"repos"`
	Brief        string             `json:"brief"`
}

// childPlan is a validated create_task call.
type childPlan struct {
	parentTitle string
	title       string
	workflow    adewire.Workflow
	repos       []taskCreateRepo
	bases       map[string]adewire.BaseChoice
	brief       string
	source      string
	step        string
}

func taskCreateItemID(ref string) string { return approvals.KindTaskCreate + ":" + ref }

// taskCreateOwner answers task.create items for the approvals queue.
type taskCreateOwner struct{ b *TaskBoard }

// Answer approves or denies the call; the item carries no options.
func (o taskCreateOwner) Answer(ref string, a approvals.Answer) error {
	v := taskDenied
	if a.Approve {
		v = taskApproved
	}
	if !o.b.resolveTaskRequest(ref, v) {
		return ipcerr.NotFoundNamed("approval")
	}
	return nil
}

func (b *TaskBoard) taskRequestWait() time.Duration {
	if b.deps.TaskRequestWait > 0 {
		return b.deps.TaskRequestWait
	}
	return defaultTaskRequestWait
}

// resolveTaskRequest withdraws the item and hands the call the verdict; false when it was already
// resolved. Only the first resolver wins.
func (b *TaskBoard) resolveTaskRequest(ref string, v taskVerdict) bool {
	q := &b.taskReqs
	q.mu.Lock()
	defer q.mu.Unlock()
	r := q.byID[ref]
	if r == nil {
		return false
	}
	delete(q.byID, ref)
	if b.deps.Approvals != nil {
		b.deps.Approvals.Remove(taskCreateItemID(ref))
	}
	r.result <- v
	return true
}

// abandonTaskRequests withdraws every open call of the task.
func (b *TaskBoard) abandonTaskRequests(taskID string) {
	q := &b.taskReqs
	q.mu.Lock()
	var refs []string
	for ref, r := range q.byID {
		if r.parentID == taskID {
			refs = append(refs, ref)
		}
	}
	q.mu.Unlock()
	for _, ref := range refs {
		b.resolveTaskRequest(ref, taskAbandoned)
	}
}

func (b *TaskBoard) abandonAllTaskRequests() {
	q := &b.taskReqs
	q.mu.Lock()
	refs := make([]string, 0, len(q.byID))
	for ref := range q.byID {
		refs = append(refs, ref)
	}
	q.mu.Unlock()
	for _, ref := range refs {
		b.resolveTaskRequest(ref, taskAbandoned)
	}
}

// openTaskRequest puts the call's item on the approvals queue.
func (b *TaskBoard) openTaskRequest(parentID string, p childPlan, expires time.Time) *taskRequest {
	ref := ids.NewID()
	pl := taskCreatePayload{
		ParentTaskID: parentID, ParentTitle: p.parentTitle, Title: p.title,
		Workflow: taskCreateWorkflow{ID: p.workflow.ID, Name: cmpNonEmpty(p.workflow.Name, p.workflow.ID)},
		Repos:    p.repos, Brief: p.brief,
	}
	if pl.Repos == nil {
		pl.Repos = []taskCreateRepo{}
	}
	raw, err := json.Marshal(pl)
	if err != nil {
		// Plain strings and slices only; a failure is a programming error.
		panic("ade: task request payload: " + err.Error())
	}
	r := &taskRequest{ref: ref, parentID: parentID, result: make(chan taskVerdict, 1)}
	r.item = approvals.Item{
		ID: taskCreateItemID(ref), Kind: approvals.KindTaskCreate, CreatedAt: b.deps.Now().UnixMilli(), ExpiresAt: expires.UnixMilli(),
		Origin:    approvals.Origin{Task: p.parentTitle, Step: p.step, Source: p.source},
		Target:    approvals.Target{Name: p.title, Kind: "task"},
		Operation: approvals.Operation{Class: "create", Tone: "caution"},
		Reason:    "request", Payload: raw,
		Title: i18n.T("backend.prompt.taskCreate.title", map[string]string{"task": p.parentTitle}),
	}
	q := &b.taskReqs
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.byID == nil {
		q.byID = map[string]*taskRequest{}
	}
	q.byID[ref] = r
	if b.deps.Approvals != nil {
		b.deps.Approvals.Put(r.item)
	}
	return r
}

// SpawnTask implements agentmcp.OrchestratorTools. Bad input fails before any prompt. The user's approval
// creates the child; the call waits for it without holding the task lock.
func (b *TaskBoard) SpawnTask(ctx context.Context, call agentmcp.SpaceCall, args agentmcp.CreateTaskArgs) (agentmcp.CreateTaskResult, error) {
	plan, err := b.planChildTask(ctx, call, args)
	if err != nil {
		return agentmcp.CreateTaskResult{}, err
	}
	wait := b.taskRequestWait()
	req := b.openTaskRequest(call.TaskID, plan, b.deps.Now().Add(wait))
	noted := b.holdRunNote(call.RunID, noteTaskApproval)
	b.notifyBoard()

	timer := time.NewTimer(wait)
	defer timer.Stop()
	var verdict taskVerdict
	select {
	case verdict = <-req.result:
	case <-timer.C:
		verdict = b.finishWait(req, taskTimedOut)
	case <-ctx.Done():
		verdict = b.finishWait(req, taskAbandoned)
	}
	if noted {
		b.releaseRunNote(call.TaskID, call.RunID)
	}
	res := agentmcp.CreateTaskResult{Title: plan.title}
	switch verdict {
	case taskApproved:
		return b.createChild(call, plan, res)
	case taskDenied:
		res.Outcome, res.Next = agentmcp.TaskDenied, "The user denied this task. Do not retry it unchanged: continue without it or change the request."
	case taskTimedOut:
		res.Outcome, res.Next = agentmcp.TaskTimeout, fmt.Sprintf("The user did not answer within %s. Call create_task again if the work is still needed.", waitText(wait))
	default:
		res.Outcome, res.Next = agentmcp.TaskFailed, "The request was withdrawn before the user answered."
	}
	return res, nil
}

// finishWait withdraws a call whose wait ended; the verdict is the answer that raced in, if any.
func (b *TaskBoard) finishWait(r *taskRequest, v taskVerdict) taskVerdict {
	b.resolveTaskRequest(r.ref, v)
	return <-r.result
}

// planChildTask validates a call under the parent's task lock.
func (b *TaskBoard) planChildTask(ctx context.Context, call agentmcp.SpaceCall, args agentmcp.CreateTaskArgs) (childPlan, error) {
	mu := b.taskMu(call.TaskID)
	mu.Lock()
	defer mu.Unlock()
	tc, err := b.spaceCtx(call.TaskID)
	if err != nil {
		return childPlan{}, err
	}
	if tc.task.Kind != model.AdeTaskKindOrchestrator {
		return childPlan{}, agentmcp.ToolError("Only an orchestrator task creates tasks.")
	}
	wf, err := b.resolveWorkflowRef(args.Workflow)
	if err != nil {
		return childPlan{}, err
	}
	p := childPlan{
		parentTitle: taskTitle(tc.task, tc.branches), title: args.Title, workflow: wf, brief: args.Brief,
		bases: map[string]adewire.BaseChoice{},
	}
	p.source, p.step = b.callSource(tc, call)
	cands, _, err := b.registeredCandidates()
	if err != nil {
		return childPlan{}, err
	}
	seen := map[string]bool{}
	for _, r := range args.Repos {
		id, err := resolveRepoRef(r.Repo, cands, "registered")
		if err != nil {
			return childPlan{}, err
		}
		if seen[id] {
			return childPlan{}, agentmcp.ToolError(fmt.Sprintf("Repo %s is listed twice.", r.Repo))
		}
		seen[id] = true
		nick := b.repoNickOf(id)
		if r.Base != "" {
			if _, _, err := b.storedBase(ctx, id, nick, adewire.BaseChoice{Ref: r.Base}); err != nil {
				return childPlan{}, toolErr(err)
			}
			p.bases[id] = adewire.BaseChoice{Ref: r.Base}
		}
		p.repos = append(p.repos, taskCreateRepo{CodeRepoID: id, Name: nick, Base: r.Base})
	}
	return p, nil
}

// resolveWorkflowRef finds a workflow by id, then by exact name; it must have a step to start on.
func (b *TaskBoard) resolveWorkflowRef(ref string) (adewire.Workflow, error) {
	if b.deps.Workflows == nil {
		return adewire.Workflow{}, agentmcp.ToolError("No workflows are configured.")
	}
	wf, ok := b.deps.Workflows.Get(ref)
	if !ok {
		var hits []adewire.Workflow
		for _, e := range b.deps.Workflows.List(nil).Workflows {
			if e.Workflow != nil && e.Workflow.Name == ref {
				hits = append(hits, *e.Workflow)
			}
		}
		switch len(hits) {
		case 0:
			return adewire.Workflow{}, agentmcp.ToolError(fmt.Sprintf("Workflow %q not found. Use a workflow id or name.", ref))
		case 1:
			wf = hits[0]
		default:
			return adewire.Workflow{}, agentmcp.ToolError(fmt.Sprintf("Workflow name %q matches several workflows. Use the id.", ref))
		}
	}
	if firstStepID(wf) == "" {
		return adewire.Workflow{}, agentmcp.ToolError(fmt.Sprintf("Workflow %q has no step to start on.", cmpNonEmpty(wf.Name, wf.ID)))
	}
	return wf, nil
}

// createChild creates the approved child under the parent's task lock, re-checked since the wait. The child
// joins the parent's plan day; a backlog parent puts it in the backlog, a parent off the plan puts it today.
func (b *TaskBoard) createChild(call agentmcp.SpaceCall, p childPlan, res agentmcp.CreateTaskResult) (agentmcp.CreateTaskResult, error) {
	fail := func(text string) (agentmcp.CreateTaskResult, error) {
		res.Outcome, res.Next = agentmcp.TaskFailed, text
		return res, nil
	}
	mu := b.taskMu(call.TaskID)
	mu.Lock()
	defer mu.Unlock()
	ctx := b.ctx
	tc, err := b.loadTaskCtx(call.TaskID)
	if err != nil || tc.task.Kind != model.AdeTaskKindOrchestrator {
		return fail("The orchestrator task is archived or no longer an orchestrator, so the task was not created.")
	}
	rows, err := b.deps.Tasks.PlanRows()
	if err != nil {
		return agentmcp.CreateTaskResult{}, err
	}
	var day *string
	backlog := false
	today := b.deps.Now().Format("2006-01-02")
	day = &today
	for _, row := range rows {
		if row.TaskID == call.TaskID {
			day, backlog = row.Day, row.Backlog
		}
	}
	args := adewire.CreateTaskArgs{Title: p.title, WorkflowID: p.workflow.ID, Bases: p.bases}
	for _, r := range p.repos {
		args.CodeRepoIDs = append(args.CodeRepoIDs, r.CodeRepoID)
	}
	nt, err := b.buildNewTask(ctx, args, model.AdeTaskKindTask, call.TaskID)
	if err != nil {
		if errors.Is(err, ErrInvalidInput) {
			return fail(trimInvalid(err))
		}
		return agentmcp.CreateTaskResult{}, err
	}
	brief := model.AdeHandover{
		ID: ids.NewID(), TaskID: nt.task.ID, FromStepName: p.parentTitle, FromTaskID: call.TaskID,
		ToStepID: nt.task.EntryStepID, ToStepName: nt.entryName, Body: p.brief, CreatedAt: nt.task.CreatedAt,
	}
	child, err := b.deps.Tasks.CreateChildTask(nt.task, nt.branches, day, brief)
	if err != nil {
		return agentmcp.CreateTaskResult{}, err
	}
	if backlog {
		if err := b.deps.Tasks.Shelve(child.ID, true, ""); err != nil {
			slog.Warn("ade: shelve child task", "scope", "ade", "task", child.ID, "err", err)
		}
	}
	b.notifyBoard()
	res.Outcome, res.TaskID = agentmcp.TaskCreated, child.ID
	res.Next = "Task created. The user starts it. Its final handover is copied into your chain: read it with task_chain, or list the children with list_tasks."
	return res, nil
}

// compile-time check that the board serves the orchestrator tools.
var _ agentmcp.OrchestratorTools = (*TaskBoard)(nil)
