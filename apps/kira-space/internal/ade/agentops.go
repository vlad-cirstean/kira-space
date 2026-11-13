package ade

import (
	"context"
	"fmt"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/oplog"
)

// agentops.go: ops for the waits and slow steps an agent's tool call runs, so the UI shows what the
// agent waits on. Each op nests under the tool's mcp op, which the handler's ctx carries.

// Agent op kinds (category oplog.CategoryAgent).
const (
	opAgentApproval = "agentApproval"
	opAgentBranches = "agentBranches"
	opAgentRename   = "agentRename"
	opAgentChange   = "agentChange"
)

// startAgentOp opens an agent-category op for taskID; command is shown when non-empty. A board
// without an op log gets a nil op, which every Op method accepts.
func (b *TaskBoard) startAgentOp(ctx context.Context, kind, taskID, command string) *oplog.Op {
	op := b.deps.Ops.Start(ctx, oplog.Meta{Category: oplog.CategoryAgent, Kind: kind, Subject: b.opSubject(taskID)})
	if command != "" {
		op.SetCommand(command)
	}
	return op
}

// approvalCommand is the approval op's command: the request's title and how many branches it asks for.
func approvalCommand(title string, items int) string {
	if items == 1 {
		return title + ", 1 branch"
	}
	return fmt.Sprintf("%s, %d branches", title, items)
}

// finishApprovalOp ends an approval op from the user's decision. A partial approval is still ok.
func finishApprovalOp(op *oplog.Op, kind decisionKind) {
	switch kind {
	case decisionApproved:
		op.Finish(oplog.StatusOK, "")
	case decisionDenied:
		op.Finish(oplog.StatusCancelled, "denied")
	case decisionTimeout:
		op.Finish(oplog.StatusCancelled, "timeout")
	default:
		op.Finish(oplog.StatusCancelled, "abandoned")
	}
}

// verdictDecision maps a task request's verdict to the branch decision kinds finishApprovalOp reads.
func verdictDecision(v taskVerdict) decisionKind {
	switch v {
	case taskApproved:
		return decisionApproved
	case taskDenied:
		return decisionDenied
	case taskTimedOut:
		return decisionTimeout
	default:
		return decisionAbandoned
	}
}
