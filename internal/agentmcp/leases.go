package agentmcp

import (
	"context"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kirathecat/kira-studio/internal/llmprompts"
)

// Lease states of acquire_lease.
const (
	LeaseHeld   = "held"
	LeaseQueued = "queued"
)

// AcquireLeaseArgs names the lease queue to take.
type AcquireLeaseArgs struct {
	Queue string `json:"queue"`
}

// ReleaseLeaseArgs names the queue to leave; empty leaves every queue.
type ReleaseLeaseArgs struct {
	Queue string `json:"queue,omitempty"`
}

// LeaseStatusArgs names one queue to show; empty shows every queue.
type LeaseStatusArgs struct {
	Queue string `json:"queue,omitempty"`
}

// AcquireLeaseResult is acquire_lease's answer. Position is 0 when held. Notices are lease messages
// that waited for this caller, such as a grant or a revoke.
type AcquireLeaseResult struct {
	Queue      string   `json:"queue"`
	State      string   `json:"state"`
	Position   int      `json:"position"`
	HolderTask string   `json:"holderTask"`
	TTLSeconds int      `json:"ttlSeconds"`
	Next       string   `json:"next"`
	Notices    []string `json:"notices,omitempty"`
}

// ReleaseLeaseResult is release_lease's answer: the queues left.
type ReleaseLeaseResult struct {
	Released []string `json:"released"`
	Notices  []string `json:"notices,omitempty"`
}

// LeaseEntryInfo is one holder or waiter. Position is 0 for the holder.
type LeaseEntryInfo struct {
	TaskID    string `json:"taskId"`
	TaskTitle string `json:"taskTitle"`
	Position  int    `json:"position,omitempty"`
	Mine      bool   `json:"mine"`
}

// LeaseQueueInfo is one queue with its holder and waiters in grant order.
type LeaseQueueInfo struct {
	Name    string           `json:"name"`
	Holder  *LeaseEntryInfo  `json:"holder"`
	Waiters []LeaseEntryInfo `json:"waiters"`
}

// LeaseStatusResult is lease_status' answer.
type LeaseStatusResult struct {
	Queues  []LeaseQueueInfo `json:"queues"`
	Notices []string         `json:"notices,omitempty"`
}

// LeaseTools is what the lease tools call; *ade.TaskBoard implements it. A caller is its session,
// or its headless run when it has none.
type LeaseTools interface {
	AcquireLease(ctx context.Context, call SpaceCall, args AcquireLeaseArgs) (AcquireLeaseResult, error)
	ReleaseLease(ctx context.Context, call SpaceCall, args ReleaseLeaseArgs) (ReleaseLeaseResult, error)
	LeaseStatus(ctx context.Context, call SpaceCall, args LeaseStatusArgs) (LeaseStatusResult, error)
}

func (s *Server) addLeaseTools(srv *mcp.Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "acquire_lease",
		Description: llmprompts.HeadlessToolAcquireLease.Text(s.prompts, llmprompts.NoVars{}),
		InputSchema: llmprompts.RequireNonEmpty(llmprompts.InputSchema[AcquireLeaseArgs](s.prompts, llmprompts.HeadlessToolAcquireLease.ID(), nil), "queue"),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args AcquireLeaseArgs) (*mcp.CallToolResult, AcquireLeaseResult, error) {
		call, bad := s.spaceTask(req)
		if bad != nil {
			return bad, AcquireLeaseResult{}, nil
		}
		args.Queue = strings.TrimSpace(args.Queue)
		if args.Queue == "" {
			return toolError("queue is required."), AcquireLeaseResult{}, nil
		}
		ctx, done := s.callCtx(ctx, req)
		defer done()
		res, err := s.space.AcquireLease(ctx, call, args)
		if err != nil {
			return fail("acquire_lease", err), AcquireLeaseResult{}, nil
		}
		return nil, res, nil
	})
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "release_lease",
		Description: llmprompts.HeadlessToolReleaseLease.Text(s.prompts, llmprompts.NoVars{}),
		InputSchema: llmprompts.InputSchema[ReleaseLeaseArgs](s.prompts, llmprompts.HeadlessToolReleaseLease.ID(), nil),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args ReleaseLeaseArgs) (*mcp.CallToolResult, ReleaseLeaseResult, error) {
		call, bad := s.spaceTask(req)
		if bad != nil {
			return bad, ReleaseLeaseResult{}, nil
		}
		args.Queue = strings.TrimSpace(args.Queue)
		ctx, done := s.callCtx(ctx, req)
		defer done()
		res, err := s.space.ReleaseLease(ctx, call, args)
		if err != nil {
			return fail("release_lease", err), ReleaseLeaseResult{}, nil
		}
		if res.Released == nil {
			res.Released = []string{}
		}
		return nil, res, nil
	})
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "lease_status",
		Description: llmprompts.HeadlessToolLeaseStatus.Text(s.prompts, llmprompts.NoVars{}),
		InputSchema: llmprompts.InputSchema[LeaseStatusArgs](s.prompts, llmprompts.HeadlessToolLeaseStatus.ID(), nil),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args LeaseStatusArgs) (*mcp.CallToolResult, LeaseStatusResult, error) {
		call, bad := s.spaceTask(req)
		if bad != nil {
			return bad, LeaseStatusResult{}, nil
		}
		args.Queue = strings.TrimSpace(args.Queue)
		ctx, done := s.callCtx(ctx, req)
		defer done()
		res, err := s.space.LeaseStatus(ctx, call, args)
		if err != nil {
			return fail("lease_status", err), LeaseStatusResult{}, nil
		}
		if res.Queues == nil {
			res.Queues = []LeaseQueueInfo{}
		}
		return nil, res, nil
	})
}
