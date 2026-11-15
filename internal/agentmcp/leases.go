package agentmcp

import (
	"context"
	"time"
)

// Lease states of lease.acquire.
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

// AcquireLeaseResult is lease.acquire's answer. Position is 0 when held. Notices are lease messages
// that waited for this caller, such as a grant or a revoke.
type AcquireLeaseResult struct {
	Queue      string `json:"queue"`
	State      string `json:"state"`
	Position   int    `json:"position"`
	HolderTask string `json:"holderTask"`
	TTLSeconds int    `json:"ttlSeconds"`
	Next       string `json:"next"`
	// Waited is how long the call waited for the grant; set when it ended queued.
	Waited  time.Duration `json:"-"`
	Notices []string      `json:"notices,omitempty"`
}

// ReleaseLeaseResult is lease.release's answer: the queues left.
type ReleaseLeaseResult struct {
	Released []string `json:"released"`
	Notices  []string `json:"notices,omitempty"`
}

// LeaseTools is what the lease tools call; *ade.TaskBoard implements it. A caller is its session,
// or its headless run when it has none.
type LeaseTools interface {
	AcquireLease(ctx context.Context, call SpaceCall, args AcquireLeaseArgs) (AcquireLeaseResult, error)
	ReleaseLease(ctx context.Context, call SpaceCall, args ReleaseLeaseArgs) (ReleaseLeaseResult, error)
}
