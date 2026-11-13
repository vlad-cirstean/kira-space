package ade

import (
	"context"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitsession"
)

// remoteSlot is the per-repo queue of ADE remote operations. gitsession refuses a second remote op
// on an entry with OperationInProgress instead of queueing it, and ADE used to queue by holding
// repoMutex across the network step. The queue keeps that wait without the repo lock: a sandbox
// push no longer blocks adopt, detach or setup on the repo, yet two pushes (or a push and the
// refresh fetch) on one repo still run one after the other. A holder outside ADE (the Git panel,
// auto-fetch) still gets the slot refusal, as before.
//
// Lock order: repoMutex (when held at all) before the remote slot; nothing takes repoMutex while
// holding the slot.
func (b *TaskBoard) remoteSlot(codeRepoID string) chan struct{} {
	b.mu.Lock()
	defer b.mu.Unlock()
	s, ok := b.remoteSlots[codeRepoID]
	if !ok {
		s = make(chan struct{}, 1)
		b.remoteSlots[codeRepoID] = s
	}
	return s
}

// runRemote runs one remote op once the repo's ADE remote slot is free; ctx ends the wait.
func (b *TaskBoard) runRemote(ctx context.Context, codeRepoID string, entry *gitsession.RepoEntry, params gitsession.RemoteOpParams) (gitsession.RemoteOpResult, error) {
	s := b.remoteSlot(codeRepoID)
	select {
	case s <- struct{}{}:
		defer func() { <-s }()
	case <-ctx.Done():
		return gitsession.RemoteOpResult{}, ctx.Err()
	}
	return entry.RunRemote(ctx, b.conn, params, gitsession.RemoteDeps{Askpass: b.deps.Askpass})
}
