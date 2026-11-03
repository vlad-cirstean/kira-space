package flowharness

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitrpc"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
)

// boundFn calls one GitService method for window key with wire-shaped params.
type boundFn func(ctx context.Context, key string, raw json.RawMessage) (any, error)

// boundCall is the table of methods GitService carries; a method outside it goes over the stream.
func (s *GitStream) boundCall(method string) (boundFn, bool) {
	svc := s.app.W.GitSvc
	table := map[string]boundFn{
		"app.init": func(ctx context.Context, key string, _ json.RawMessage) (any, error) {
			return svc.AppInit(ctx, bridge.GitAppInitArgs{WindowKey: key})
		},
		"repo.open": bind(func(ctx context.Context, key string, p gitrpc.RepoOpenParams) (any, error) {
			return svc.RepoOpen(ctx, bridge.GitRepoOpenArgs{WindowKey: key, Params: p})
		}),
		"repo.close": bind(func(ctx context.Context, key string, p gitrpc.RepoCloseParams) (any, error) {
			return svc.RepoClose(ctx, bridge.GitRepoCloseArgs{WindowKey: key, Params: p})
		}),
		"status.get": bind(func(ctx context.Context, key string, p gitrpc.StatusGetParams) (any, error) {
			return svc.StatusGet(ctx, bridge.GitStatusGetArgs{WindowKey: key, Params: p})
		}),
		"refs.list": bind(func(ctx context.Context, key string, p gitrpc.RefsListParams) (any, error) {
			return svc.RefsList(ctx, bridge.GitRefsListArgs{WindowKey: key, Params: p})
		}),
		"commit.detail": bind(func(ctx context.Context, key string, p gitrpc.CommitDetailParams) (any, error) {
			return svc.CommitDetail(ctx, bridge.GitCommitDetailArgs{WindowKey: key, Params: p})
		}),
		"commit.fileDiff": bind(func(ctx context.Context, key string, p gitrpc.CommitFileDiffParams) (any, error) {
			return svc.CommitFileDiff(ctx, bridge.GitCommitFileDiffArgs{WindowKey: key, Params: p})
		}),
		"working.detail": bind(func(ctx context.Context, key string, p gitrpc.WorkingDetailParams) (any, error) {
			return svc.WorkingDetail(ctx, bridge.GitWorkingDetailArgs{WindowKey: key, Params: p})
		}),
		"file.read": bind(func(ctx context.Context, key string, p gitrpc.FileReadParams) (any, error) {
			return svc.FileRead(ctx, bridge.GitFileReadArgs{WindowKey: key, Params: p})
		}),
		"file.goToTarget": bind(func(ctx context.Context, key string, p gitrpc.FileGoToTargetParams) (any, error) {
			return svc.FileGoToTarget(ctx, bridge.GitFileGoToTargetArgs{WindowKey: key, Params: p})
		}),
		"blame.line": bind(func(ctx context.Context, key string, p gitrpc.BlameLineParams) (any, error) {
			return svc.BlameLine(ctx, bridge.GitBlameLineArgs{WindowKey: key, Params: p})
		}),
		"search.run": bind(func(ctx context.Context, key string, p gitrpc.SearchRunParams) (any, error) {
			return svc.SearchRun(ctx, bridge.GitSearchRunArgs{WindowKey: key, Params: p})
		}),
		"repoSettings.get": bind(func(ctx context.Context, key string, p gitrpc.RepoSettingsGetParams) (any, error) {
			return svc.RepoSettingsGet(ctx, bridge.GitRepoSettingsGetArgs{WindowKey: key, Params: p})
		}),
		"repoSettings.set": bind(func(ctx context.Context, key string, p gitrpc.RepoSettingsSetParams) (any, error) {
			return svc.RepoSettingsSet(ctx, bridge.GitRepoSettingsSetArgs{WindowKey: key, Params: p})
		}),
		"preflight.checkout": bind(func(ctx context.Context, key string, p gitrpc.PreflightCheckoutParams) (any, error) {
			return svc.PreflightCheckout(ctx, bridge.GitPreflightCheckoutArgs{WindowKey: key, Params: p})
		}),
		"preflight.revert": bind(func(ctx context.Context, key string, p gitrpc.PreflightRevertParams) (any, error) {
			return svc.PreflightRevert(ctx, bridge.GitPreflightRevertArgs{WindowKey: key, Params: p})
		}),
		"preflight.reset": bind(func(ctx context.Context, key string, p gitrpc.PreflightResetParams) (any, error) {
			return svc.PreflightReset(ctx, bridge.GitPreflightResetArgs{WindowKey: key, Params: p})
		}),
		"preflight.cherryPick": bind(func(ctx context.Context, key string, p gitrpc.PreflightCherryPickParams) (any, error) {
			return svc.PreflightCherryPick(ctx, bridge.GitPreflightCherryPickArgs{WindowKey: key, Params: p})
		}),
		"stash.list": bind(func(ctx context.Context, key string, p gitrpc.StashListParams) (any, error) {
			return svc.StashList(ctx, bridge.GitStashListArgs{WindowKey: key, Params: p})
		}),
		"stash.show": bind(func(ctx context.Context, key string, p gitrpc.StashShowParams) (any, error) {
			return svc.StashShow(ctx, bridge.GitStashShowArgs{WindowKey: key, Params: p})
		}),
		"preflight.stashPop": bind(func(ctx context.Context, key string, p gitrpc.PreflightStashPopParams) (any, error) {
			return svc.PreflightStashPop(ctx, bridge.GitPreflightStashPopArgs{WindowKey: key, Params: p})
		}),
		"preflight.stashBranch": bind(func(ctx context.Context, key string, p gitrpc.PreflightStashBranchParams) (any, error) {
			return svc.PreflightStashBranch(ctx, bridge.GitPreflightStashBranchArgs{WindowKey: key, Params: p})
		}),
		"globalStash.list": bind(func(ctx context.Context, key string, p gitrpc.GlobalStashListParams) (any, error) {
			return svc.GlobalStashList(ctx, bridge.GitGlobalStashListArgs{WindowKey: key, Params: p})
		}),
		"op.run": bind(func(ctx context.Context, key string, p gitrpc.OpRunParams) (any, error) {
			return svc.OpRun(ctx, bridge.GitOpRunArgs{WindowKey: key, Params: p})
		}),
		"undo.peek": bind(func(ctx context.Context, key string, p gitrpc.UndoPeekParams) (any, error) {
			return svc.UndoPeek(ctx, bridge.GitUndoPeekArgs{WindowKey: key, Params: p})
		}),
		"undo.run": bind(func(ctx context.Context, key string, p gitrpc.UndoRunParams) (any, error) {
			return svc.UndoRun(ctx, bridge.GitUndoRunArgs{WindowKey: key, Params: p})
		}),
		"remote.pullPreflight": bind(func(ctx context.Context, key string, p gitrpc.RemotePullPreflightParams) (any, error) {
			return svc.RemotePullPreflight(ctx, bridge.GitRemotePullPreflightArgs{WindowKey: key, Params: p})
		}),
		"remote.pushPreflight": bind(func(ctx context.Context, key string, p gitrpc.RemotePushPreflightParams) (any, error) {
			return svc.RemotePushPreflight(ctx, bridge.GitRemotePushPreflightArgs{WindowKey: key, Params: p})
		}),
		"remote.run": bind(func(ctx context.Context, key string, p gitrpc.RemoteRunParams) (any, error) {
			return svc.RemoteRun(ctx, bridge.GitRemoteRunArgs{WindowKey: key, Params: p})
		}),
		"remote.cancel": bind(func(ctx context.Context, key string, p gitrpc.RemoteCancelParams) (any, error) {
			return svc.RemoteCancel(ctx, bridge.GitRemoteCancelArgs{WindowKey: key, Params: p})
		}),
		"worktree.list": bind(func(ctx context.Context, key string, p gitrpc.WorktreeListParams) (any, error) {
			return svc.WorktreeList(ctx, bridge.GitWorktreeListArgs{WindowKey: key, Params: p})
		}),
		"preflight.worktreeAdd": bind(func(ctx context.Context, key string, p gitrpc.PreflightWorktreeAddParams) (any, error) {
			return svc.PreflightWorktreeAdd(ctx, bridge.GitPreflightWorktreeAddArgs{WindowKey: key, Params: p})
		}),
		"preflight.worktreeRemove": bind(func(ctx context.Context, key string, p gitrpc.PreflightWorktreeRemoveParams) (any, error) {
			return svc.PreflightWorktreeRemove(ctx, bridge.GitPreflightWorktreeRemoveArgs{WindowKey: key, Params: p})
		}),
		"stack.list": bind(func(ctx context.Context, key string, p gitrpc.StackListParams) (any, error) {
			return svc.StackList(ctx, bridge.GitStackListArgs{WindowKey: key, Params: p})
		}),
		"preflight.restack": bind(func(ctx context.Context, key string, p gitrpc.PreflightRestackParams) (any, error) {
			return svc.PreflightRestack(ctx, bridge.GitPreflightRestackArgs{WindowKey: key, Params: p})
		}),
		"stack.restack": bind(func(ctx context.Context, key string, p gitrpc.StackRestackParams) (any, error) {
			return svc.StackRestack(ctx, bridge.GitStackRestackArgs{WindowKey: key, Params: p})
		}),
		"stack.cancelRestack": bind(func(ctx context.Context, key string, p gitrpc.StackCancelRestackParams) (any, error) {
			return svc.StackCancelRestack(ctx, bridge.GitStackCancelRestackArgs{WindowKey: key, Params: p})
		}),
	}
	fn, ok := table[method]
	return fn, ok
}

// bind decodes wire params into the method's typed params; a malformed body is the contract error
// the handler would have raised.
func bind[P any](call func(ctx context.Context, key string, p P) (any, error)) boundFn {
	return func(ctx context.Context, key string, raw json.RawMessage) (any, error) {
		var p P
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, ipcerr.Contract("gitrpc: invalid params")
		}
		return call(ctx, key, p)
	}
}

// requestBound runs a bound call and decodes the result into out, errors as the wire carries them.
func (s *GitStream) requestBound(call boundFn, method string, params, out any) error {
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	res, err := call(ctx, s.key, rawParams(s.t, params))
	if err != nil {
		var ie *ipcerr.Error
		if errors.As(err, &ie) {
			return &WireError{Code: ie.Code, Message: ie.Message}
		}
		return &WireError{Code: string(ipcerr.CodeInternal), Message: err.Error()}
	}
	if out == nil {
		return nil
	}
	raw, err := json.Marshal(res)
	if err != nil {
		return fmt.Errorf("%s: encode result: %w", method, err)
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("%s: decode result %s: %w", method, raw, err)
	}
	return nil
}
