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
// Lock order: the remote slot before repoMutex. A caller that needs both takes the slot first
// (holdRemoteSlot); a caller that only runs the network step takes the slot alone and locks
// repoMutex after releasing it. Nothing waits for the slot while holding repoMutex.
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

// slotHeld marks a ctx whose caller already holds the repo's remote slot; runRemote then skips the wait.
type slotHeld struct{ codeRepoID string }

// holdRemoteSlot waits for the repo's ADE remote slot (ctx ends the wait) and returns a ctx that
// marks it held plus its release. A caller that needs repoMutex and a remote op takes the slot
// first, so a queued caller waits without the repo lock.
func (b *TaskBoard) holdRemoteSlot(ctx context.Context, codeRepoID string) (context.Context, func(), error) {
	s := b.remoteSlot(codeRepoID)
	select {
	case s <- struct{}{}:
		return context.WithValue(ctx, slotHeld{codeRepoID}, true), func() { <-s }, nil
	case <-ctx.Done():
		return ctx, func() {}, ctx.Err()
	}
}

// runRemote runs one remote op once the repo's ADE remote slot is free; ctx ends the wait.
func (b *TaskBoard) runRemote(ctx context.Context, codeRepoID string, entry *gitsession.RepoEntry, params gitsession.RemoteOpParams) (gitsession.RemoteOpResult, error) {
	if ctx.Value(slotHeld{codeRepoID}) == nil {
		var release func()
		var err error
		if ctx, release, err = b.holdRemoteSlot(ctx, codeRepoID); err != nil {
			return gitsession.RemoteOpResult{}, err
		}
		defer release()
	}
	return entry.RunRemote(ctx, b.conn, params, gitsession.RemoteDeps{Askpass: b.deps.Askpass})
}
