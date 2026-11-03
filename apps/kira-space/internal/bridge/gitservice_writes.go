package bridge

import (
	"context"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitpreflight"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitrpc"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitsession"
)

// GitPreflightCheckoutArgs is PreflightCheckout's request.
type GitPreflightCheckoutArgs struct {
	WindowKey string                         `json:"windowKey"`
	Params    gitrpc.PreflightCheckoutParams `json:"params"`
}

// GitPreflightRevertArgs is PreflightRevert's request.
type GitPreflightRevertArgs struct {
	WindowKey string                       `json:"windowKey"`
	Params    gitrpc.PreflightRevertParams `json:"params"`
}

// GitPreflightResetArgs is PreflightReset's request.
type GitPreflightResetArgs struct {
	WindowKey string                      `json:"windowKey"`
	Params    gitrpc.PreflightResetParams `json:"params"`
}

// GitPreflightCherryPickArgs is PreflightCherryPick's request.
type GitPreflightCherryPickArgs struct {
	WindowKey string                           `json:"windowKey"`
	Params    gitrpc.PreflightCherryPickParams `json:"params"`
}

// GitStashListArgs is StashList's request.
type GitStashListArgs struct {
	WindowKey string                 `json:"windowKey"`
	Params    gitrpc.StashListParams `json:"params"`
}

// GitStashShowArgs is StashShow's request.
type GitStashShowArgs struct {
	WindowKey string                 `json:"windowKey"`
	Params    gitrpc.StashShowParams `json:"params"`
}

// GitPreflightStashPopArgs is PreflightStashPop's request.
type GitPreflightStashPopArgs struct {
	WindowKey string                         `json:"windowKey"`
	Params    gitrpc.PreflightStashPopParams `json:"params"`
}

// GitPreflightStashBranchArgs is PreflightStashBranch's request.
type GitPreflightStashBranchArgs struct {
	WindowKey string                            `json:"windowKey"`
	Params    gitrpc.PreflightStashBranchParams `json:"params"`
}

// GitGlobalStashListArgs is GlobalStashList's request.
type GitGlobalStashListArgs struct {
	WindowKey string                       `json:"windowKey"`
	Params    gitrpc.GlobalStashListParams `json:"params"`
}

// GitOpRunArgs is OpRun's request.
type GitOpRunArgs struct {
	WindowKey string             `json:"windowKey"`
	Params    gitrpc.OpRunParams `json:"params"`
}

// GitUndoPeekArgs is UndoPeek's request.
type GitUndoPeekArgs struct {
	WindowKey string                `json:"windowKey"`
	Params    gitrpc.UndoPeekParams `json:"params"`
}

// GitUndoRunArgs is UndoRun's request.
type GitUndoRunArgs struct {
	WindowKey string               `json:"windowKey"`
	Params    gitrpc.UndoRunParams `json:"params"`
}

// GitRemotePullPreflightArgs is RemotePullPreflight's request.
type GitRemotePullPreflightArgs struct {
	WindowKey string                           `json:"windowKey"`
	Params    gitrpc.RemotePullPreflightParams `json:"params"`
}

// GitRemotePushPreflightArgs is RemotePushPreflight's request.
type GitRemotePushPreflightArgs struct {
	WindowKey string                           `json:"windowKey"`
	Params    gitrpc.RemotePushPreflightParams `json:"params"`
}

// GitRemoteRunArgs is RemoteRun's request.
type GitRemoteRunArgs struct {
	WindowKey string                 `json:"windowKey"`
	Params    gitrpc.RemoteRunParams `json:"params"`
}

// GitRemoteCancelArgs is RemoteCancel's request.
type GitRemoteCancelArgs struct {
	WindowKey string                    `json:"windowKey"`
	Params    gitrpc.RemoteCancelParams `json:"params"`
}

// GitWorktreeListArgs is WorktreeList's request.
type GitWorktreeListArgs struct {
	WindowKey string                    `json:"windowKey"`
	Params    gitrpc.WorktreeListParams `json:"params"`
}

// GitPreflightWorktreeAddArgs is PreflightWorktreeAdd's request.
type GitPreflightWorktreeAddArgs struct {
	WindowKey string                            `json:"windowKey"`
	Params    gitrpc.PreflightWorktreeAddParams `json:"params"`
}

// GitPreflightWorktreeRemoveArgs is PreflightWorktreeRemove's request.
type GitPreflightWorktreeRemoveArgs struct {
	WindowKey string                               `json:"windowKey"`
	Params    gitrpc.PreflightWorktreeRemoveParams `json:"params"`
}

// GitStackListArgs is StackList's request.
type GitStackListArgs struct {
	WindowKey string                 `json:"windowKey"`
	Params    gitrpc.StackListParams `json:"params"`
}

// GitPreflightRestackArgs is PreflightRestack's request.
type GitPreflightRestackArgs struct {
	WindowKey string                        `json:"windowKey"`
	Params    gitrpc.PreflightRestackParams `json:"params"`
}

// GitStackRestackArgs is StackRestack's request.
type GitStackRestackArgs struct {
	WindowKey string                    `json:"windowKey"`
	Params    gitrpc.StackRestackParams `json:"params"`
}

// GitStackCancelRestackArgs is StackCancelRestack's request.
type GitStackCancelRestackArgs struct {
	WindowKey string                          `json:"windowKey"`
	Params    gitrpc.StackCancelRestackParams `json:"params"`
}

// PreflightCheckout answers preflight.checkout.
func (s *GitService) PreflightCheckout(ctx context.Context, a GitPreflightCheckoutArgs) (gitpreflight.CheckoutPreflight, error) {
	return callGit[gitpreflight.CheckoutPreflight](ctx, s.windows, a.WindowKey, "preflight.checkout", a.Params)
}

// PreflightRevert answers preflight.revert.
func (s *GitService) PreflightRevert(ctx context.Context, a GitPreflightRevertArgs) (gitpreflight.RevertPreflight, error) {
	return callGit[gitpreflight.RevertPreflight](ctx, s.windows, a.WindowKey, "preflight.revert", a.Params)
}

// PreflightReset answers preflight.reset.
func (s *GitService) PreflightReset(ctx context.Context, a GitPreflightResetArgs) (gitpreflight.ResetPreflight, error) {
	return callGit[gitpreflight.ResetPreflight](ctx, s.windows, a.WindowKey, "preflight.reset", a.Params)
}

// PreflightCherryPick answers preflight.cherryPick.
func (s *GitService) PreflightCherryPick(ctx context.Context, a GitPreflightCherryPickArgs) (gitpreflight.CherryPickPreflight, error) {
	return callGit[gitpreflight.CherryPickPreflight](ctx, s.windows, a.WindowKey, "preflight.cherryPick", a.Params)
}

// StashList answers stash.list.
func (s *GitService) StashList(ctx context.Context, a GitStashListArgs) (gitrpc.StashListResult, error) {
	return callGit[gitrpc.StashListResult](ctx, s.windows, a.WindowKey, "stash.list", a.Params)
}

// StashShow answers stash.show.
func (s *GitService) StashShow(ctx context.Context, a GitStashShowArgs) (gitsession.StashShowResult, error) {
	return callGit[gitsession.StashShowResult](ctx, s.windows, a.WindowKey, "stash.show", a.Params)
}

// PreflightStashPop answers preflight.stashPop.
func (s *GitService) PreflightStashPop(ctx context.Context, a GitPreflightStashPopArgs) (gitpreflight.StashPopPreflight, error) {
	return callGit[gitpreflight.StashPopPreflight](ctx, s.windows, a.WindowKey, "preflight.stashPop", a.Params)
}

// PreflightStashBranch answers preflight.stashBranch.
func (s *GitService) PreflightStashBranch(ctx context.Context, a GitPreflightStashBranchArgs) (gitpreflight.StashBranchPreflight, error) {
	return callGit[gitpreflight.StashBranchPreflight](ctx, s.windows, a.WindowKey, "preflight.stashBranch", a.Params)
}

// GlobalStashList answers globalStash.list.
func (s *GitService) GlobalStashList(ctx context.Context, a GitGlobalStashListArgs) (gitrpc.StashListResult, error) {
	return callGit[gitrpc.StashListResult](ctx, s.windows, a.WindowKey, "globalStash.list", a.Params)
}

// OpRun answers op.run. Detached: finishes after the caller cancels or the window closes.
func (s *GitService) OpRun(ctx context.Context, a GitOpRunArgs) (gitsession.OpResult, error) {
	return callGit[gitsession.OpResult](ctx, s.windows, a.WindowKey, "op.run", a.Params)
}

// UndoPeek answers undo.peek.
func (s *GitService) UndoPeek(ctx context.Context, a GitUndoPeekArgs) (gitrpc.UndoPeekResult, error) {
	return callGit[gitrpc.UndoPeekResult](ctx, s.windows, a.WindowKey, "undo.peek", a.Params)
}

// UndoRun answers undo.run. Detached: finishes after the caller cancels or the window closes.
func (s *GitService) UndoRun(ctx context.Context, a GitUndoRunArgs) (gitsession.OpResult, error) {
	return callGit[gitsession.OpResult](ctx, s.windows, a.WindowKey, "undo.run", a.Params)
}

// RemotePullPreflight answers remote.pullPreflight.
func (s *GitService) RemotePullPreflight(ctx context.Context, a GitRemotePullPreflightArgs) (gitpreflight.PullPreflight, error) {
	return callGit[gitpreflight.PullPreflight](ctx, s.windows, a.WindowKey, "remote.pullPreflight", a.Params)
}

// RemotePushPreflight answers remote.pushPreflight.
func (s *GitService) RemotePushPreflight(ctx context.Context, a GitRemotePushPreflightArgs) (gitpreflight.PushPreflight, error) {
	return callGit[gitpreflight.PushPreflight](ctx, s.windows, a.WindowKey, "remote.pushPreflight", a.Params)
}

// RemoteRun answers remote.run. Detached: finishes after the caller cancels or the window closes.
func (s *GitService) RemoteRun(ctx context.Context, a GitRemoteRunArgs) (gitsession.RemoteOpResult, error) {
	return callGit[gitsession.RemoteOpResult](ctx, s.windows, a.WindowKey, "remote.run", a.Params)
}

// RemoteCancel answers remote.cancel.
func (s *GitService) RemoteCancel(ctx context.Context, a GitRemoteCancelArgs) (gitrpc.RemoteCancelResult, error) {
	return callGit[gitrpc.RemoteCancelResult](ctx, s.windows, a.WindowKey, "remote.cancel", a.Params)
}

// WorktreeList answers worktree.list.
func (s *GitService) WorktreeList(ctx context.Context, a GitWorktreeListArgs) (gitrpc.WorktreeListResult, error) {
	return callGit[gitrpc.WorktreeListResult](ctx, s.windows, a.WindowKey, "worktree.list", a.Params)
}

// PreflightWorktreeAdd answers preflight.worktreeAdd.
func (s *GitService) PreflightWorktreeAdd(ctx context.Context, a GitPreflightWorktreeAddArgs) (gitpreflight.WorktreeAddPreflight, error) {
	return callGit[gitpreflight.WorktreeAddPreflight](ctx, s.windows, a.WindowKey, "preflight.worktreeAdd", a.Params)
}

// PreflightWorktreeRemove answers preflight.worktreeRemove.
func (s *GitService) PreflightWorktreeRemove(ctx context.Context, a GitPreflightWorktreeRemoveArgs) (gitpreflight.WorktreeRemovePreflight, error) {
	return callGit[gitpreflight.WorktreeRemovePreflight](ctx, s.windows, a.WindowKey, "preflight.worktreeRemove", a.Params)
}

// StackList answers stack.list.
func (s *GitService) StackList(ctx context.Context, a GitStackListArgs) (gitpreflight.StackListResult, error) {
	return callGit[gitpreflight.StackListResult](ctx, s.windows, a.WindowKey, "stack.list", a.Params)
}

// PreflightRestack answers preflight.restack.
func (s *GitService) PreflightRestack(ctx context.Context, a GitPreflightRestackArgs) (gitpreflight.RestackPreflight, error) {
	return callGit[gitpreflight.RestackPreflight](ctx, s.windows, a.WindowKey, "preflight.restack", a.Params)
}

// StackRestack answers stack.restack. Detached: finishes after the caller cancels or the window closes.
func (s *GitService) StackRestack(ctx context.Context, a GitStackRestackArgs) (gitsession.RestackResult, error) {
	return callGit[gitsession.RestackResult](ctx, s.windows, a.WindowKey, "stack.restack", a.Params)
}

// StackCancelRestack answers stack.cancelRestack.
func (s *GitService) StackCancelRestack(ctx context.Context, a GitStackCancelRestackArgs) (gitrpc.StackCancelRestackResult, error) {
	return callGit[gitrpc.StackCancelRestackResult](ctx, s.windows, a.WindowKey, "stack.cancelRestack", a.Params)
}
