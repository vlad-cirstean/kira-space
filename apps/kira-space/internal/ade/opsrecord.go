package ade

import (
	"context"
	"sync"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/oplog"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/agentmcp"
)

// Session op kinds.
const (
	opStart       = "start"
	opAdHoc       = "adHoc"
	opTakeOver    = "takeOver"
	opRestart     = "restart"
	opManual      = "manual"
	opReviewAgent = "reviewAgent"
	opRun         = "run"
	opResume      = "resume"
	opRebase      = "rebase"
	opReview      = "review"
	opSetup       = "setup"
	opStop        = "stop"
	// opRefreshTail is the oplog kind of a failed Refresh tail (category git).
	opRefreshTail  = "refreshTail"
	opTerminalLost = "The terminal never started the session."
)

// sessionOps holds the op of each live agent session until its outcome lands, keyed by the
// session record id.
type sessionOps struct {
	mu sync.Mutex
	m  map[string]*oplog.Op
}

func (s *sessionOps) put(id string, op *oplog.Op) {
	if op == nil || id == "" {
		return
	}
	s.mu.Lock()
	if s.m == nil {
		s.m = map[string]*oplog.Op{}
	}
	s.m[id] = op
	s.mu.Unlock()
}

func (s *sessionOps) peek(id string) *oplog.Op {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.m[id]
}

func (s *sessionOps) take(id string) *oplog.Op {
	s.mu.Lock()
	defer s.mu.Unlock()
	op := s.m[id]
	delete(s.m, id)
	return op
}

// opSubject is the task's display title, "" when it has none.
func (b *TaskBoard) opSubject(taskID string) string {
	if taskID == "" {
		return ""
	}
	t, err := b.deps.Tasks.GetTask(taskID)
	if err != nil {
		return ""
	}
	switch {
	case t.Title != "":
		return t.Title
	case t.JiraKey != "":
		return t.JiraKey
	}
	return ""
}

// startSessionOp opens a session-category op for a task; origin (user from the UI, auto on the board context) comes from ctx.
func (b *TaskBoard) startSessionOp(ctx context.Context, kind, taskID, provider, command string) *oplog.Op {
	op := b.deps.Ops.Start(oplog.WithOrigin(ctx, originOf(ctx, taskID)), oplog.Meta{
		Category: oplog.CategorySession, Kind: kind, Source: provider, Subject: b.opSubject(taskID),
	})
	if command != "" {
		op.SetCommand(command)
	}
	return op
}

// originOf keeps the caller's origin and adds the task.
func originOf(ctx context.Context, taskID string) oplog.Origin {
	o, _ := oplog.OriginFrom(ctx)
	o.TaskID = taskID
	o.Background = false
	return o
}

// finishRunOp ends the op of an agent session from a run outcome.
func (b *TaskBoard) finishRunOp(sessionID string, out outcome) {
	op := b.sessionOps.take(sessionID)
	if op == nil {
		return
	}
	switch {
	case out.noAdvance:
		op.Finish(oplog.StatusCancelled, "")
	case out.state == model.AdeRunDone || out.state == model.AdeRunBack:
		op.Finish(oplog.StatusOK, "")
	default:
		op.Finish(oplog.StatusError, out.note)
	}
}

// cancelRunOp registers a Cancel on the op that stops the run. The func only signals: Log.Cancel holds the log lock.
func (b *TaskBoard) cancelRunOp(op *oplog.Op, runID string) {
	op.SetCancel(func() bool { return b.cancelLive(b.live, runID, errStopped) != nil })
}

// mcpRecorder is agentmcp.Options.OnToolCall: it records each tools/call as an mcp op and hands the
// handler a ctx carrying it, so nested git work lands under it.
func (b *TaskBoard) mcpRecorder(ctx context.Context, c agentmcp.ToolCall) (context.Context, func(isError bool, text string, err error)) {
	b.lease.book.Touch(leaseKeyOf(c.SessionID, c.RunID)) // any tool call is a lease heartbeat
	o := oplog.Origin{Module: oplog.ModuleADE, Trigger: oplog.TriggerMCP, Label: "MCP " + c.Tool, TaskID: c.TaskID}
	ctx = oplog.WithOrigin(ctx, o)
	op := b.deps.Ops.Start(ctx, oplog.Meta{
		Category: oplog.CategoryMCP, Kind: c.Tool, Source: "MCP " + c.Tool, Subject: b.opSubject(c.TaskID),
	})
	op.SetCommand(c.Tool + " " + oplog.RedactJSON(c.Args, 2<<10))
	return oplog.WithOp(ctx, op), func(isError bool, text string, err error) { oplog.FinishCall(op, isError, text, err) }
}

// recordSessionEnd logs one finished stop of an interactive session: a user stop, a terminal that
// exited (auto) or a launch whose terminal never opened (error).
func (t *Tracker) recordSessionEnd(recordID, trigger, status, msg string) {
	if t.deps.Ops == nil {
		return
	}
	rec, err := t.deps.Store.Get(recordID)
	if err != nil || rec == nil {
		return
	}
	subject := ""
	if t.deps.OpSubject != nil {
		subject = t.deps.OpSubject(rec.TaskID)
	}
	ctx := oplog.WithOrigin(context.Background(), oplog.Origin{Module: oplog.ModuleADE, Trigger: trigger, TaskID: rec.TaskID})
	t.deps.Ops.Record(ctx, oplog.Meta{Category: oplog.CategorySession, Kind: opStop, Source: rec.Provider, Subject: subject}, status, msg)
}
