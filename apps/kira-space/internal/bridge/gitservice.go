package bridge

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitclient/porcelain"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitpreflight"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitrpc"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitsession"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
)

// GitService is the bound git surface. Every method runs the matching gitrpc handler on the
// window's shared Conn (GitWindows), so results, errors and size caps match the git stream.
type GitService struct {
	windows *GitWindows
}

// NewGitService returns the service over windows.
func NewGitService(windows *GitWindows) *GitService { return &GitService{windows: windows} }

// callGit runs one gitrpc request on the window's Conn and types the result. Handlers answer T directly;
// the review comment exports answer an encoded json.RawMessage.
func callGit[T any](ctx context.Context, windows *GitWindows, key, method string, params any) (T, error) {
	var zero T
	raw, err := json.Marshal(params)
	if err != nil {
		return zero, ipcerr.Wrap(fmt.Errorf("gitservice: %s: %w", method, err))
	}
	w, err := windows.get(key)
	if err != nil {
		return zero, err
	}
	res, err := w.handlers.Request(ctx, method, raw)
	if err != nil {
		return zero, ipcerr.Wrap(err)
	}
	switch v := res.(type) {
	case json.RawMessage:
		var out T
		if err := json.Unmarshal(v, &out); err != nil {
			return zero, ipcerr.Wrap(fmt.Errorf("gitservice: %s: %w", method, err))
		}
		return out, nil
	case T:
		return v, nil
	}
	return zero, ipcerr.Wrap(fmt.Errorf("gitservice: %s: unexpected result type", method))
}

// GitAppInitArgs is AppInit's request.
type GitAppInitArgs struct {
	WindowKey string `json:"windowKey"`
}

// GitRepoOpenArgs is RepoOpen's request.
type GitRepoOpenArgs struct {
	WindowKey string                `json:"windowKey"`
	Params    gitrpc.RepoOpenParams `json:"params"`
}

// GitRepoCloseArgs is RepoClose's request.
type GitRepoCloseArgs struct {
	WindowKey string                 `json:"windowKey"`
	Params    gitrpc.RepoCloseParams `json:"params"`
}

// GitStatusGetArgs is StatusGet's request.
type GitStatusGetArgs struct {
	WindowKey string                 `json:"windowKey"`
	Params    gitrpc.StatusGetParams `json:"params"`
}

// GitRefsListArgs is RefsList's request.
type GitRefsListArgs struct {
	WindowKey string                `json:"windowKey"`
	Params    gitrpc.RefsListParams `json:"params"`
}

// GitCommitDetailArgs is CommitDetail's request.
type GitCommitDetailArgs struct {
	WindowKey string                    `json:"windowKey"`
	Params    gitrpc.CommitDetailParams `json:"params"`
}

// GitCommitFileDiffArgs is CommitFileDiff's request.
type GitCommitFileDiffArgs struct {
	WindowKey string                      `json:"windowKey"`
	Params    gitrpc.CommitFileDiffParams `json:"params"`
}

// GitWorkingDetailArgs is WorkingDetail's request.
type GitWorkingDetailArgs struct {
	WindowKey string                     `json:"windowKey"`
	Params    gitrpc.WorkingDetailParams `json:"params"`
}

// GitFileReadArgs is FileRead's request.
type GitFileReadArgs struct {
	WindowKey string                `json:"windowKey"`
	Params    gitrpc.FileReadParams `json:"params"`
}

// GitFileGoToTargetArgs is FileGoToTarget's request.
type GitFileGoToTargetArgs struct {
	WindowKey string                      `json:"windowKey"`
	Params    gitrpc.FileGoToTargetParams `json:"params"`
}

// GitBlameLineArgs is BlameLine's request.
type GitBlameLineArgs struct {
	WindowKey string                 `json:"windowKey"`
	Params    gitrpc.BlameLineParams `json:"params"`
}

// GitSearchRunArgs is SearchRun's request.
type GitSearchRunArgs struct {
	WindowKey string                 `json:"windowKey"`
	Params    gitrpc.SearchRunParams `json:"params"`
}

// GitRepoSettingsGetArgs is RepoSettingsGet's request.
type GitRepoSettingsGetArgs struct {
	WindowKey string                       `json:"windowKey"`
	Params    gitrpc.RepoSettingsGetParams `json:"params"`
}

// GitRepoSettingsSetArgs is RepoSettingsSet's request.
type GitRepoSettingsSetArgs struct {
	WindowKey string                       `json:"windowKey"`
	Params    gitrpc.RepoSettingsSetParams `json:"params"`
}

// AppInit answers app.init: contract version, git discovery status, date format.
func (s *GitService) AppInit(ctx context.Context, a GitAppInitArgs) (gitrpc.AppInitResult, error) {
	return callGit[gitrpc.AppInitResult](ctx, s.windows, a.WindowKey, "app.init", struct{}{})
}

// RepoOpen holds a repository for the window.
func (s *GitService) RepoOpen(ctx context.Context, a GitRepoOpenArgs) (gitrpc.RepoOpenResult, error) {
	return callGit[gitrpc.RepoOpenResult](ctx, s.windows, a.WindowKey, "repo.open", a.Params)
}

// RepoClose releases the window's hold on a repository.
func (s *GitService) RepoClose(ctx context.Context, a GitRepoCloseArgs) (struct{}, error) {
	return callGit[struct{}](ctx, s.windows, a.WindowKey, "repo.close", a.Params)
}

// StatusGet answers status.get.
func (s *GitService) StatusGet(ctx context.Context, a GitStatusGetArgs) (gitpreflight.StatusSummary, error) {
	return callGit[gitpreflight.StatusSummary](ctx, s.windows, a.WindowKey, "status.get", a.Params)
}

// RefsList answers refs.list.
func (s *GitService) RefsList(ctx context.Context, a GitRefsListArgs) (gitsession.RefsResult, error) {
	return callGit[gitsession.RefsResult](ctx, s.windows, a.WindowKey, "refs.list", a.Params)
}

// CommitDetail answers commit.detail.
func (s *GitService) CommitDetail(ctx context.Context, a GitCommitDetailArgs) (porcelain.CommitDetail, error) {
	return callGit[porcelain.CommitDetail](ctx, s.windows, a.WindowKey, "commit.detail", a.Params)
}

// CommitFileDiff answers commit.fileDiff.
func (s *GitService) CommitFileDiff(ctx context.Context, a GitCommitFileDiffArgs) (gitsession.FileDiffResult, error) {
	return callGit[gitsession.FileDiffResult](ctx, s.windows, a.WindowKey, "commit.fileDiff", a.Params)
}

// WorkingDetail answers working.detail.
func (s *GitService) WorkingDetail(ctx context.Context, a GitWorkingDetailArgs) (gitrpc.WorkingDetailResult, error) {
	return callGit[gitrpc.WorkingDetailResult](ctx, s.windows, a.WindowKey, "working.detail", a.Params)
}

// FileRead answers file.read.
func (s *GitService) FileRead(ctx context.Context, a GitFileReadArgs) (gitsession.BlobResult, error) {
	return callGit[gitsession.BlobResult](ctx, s.windows, a.WindowKey, "file.read", a.Params)
}

// FileGoToTarget answers file.goToTarget.
func (s *GitService) FileGoToTarget(ctx context.Context, a GitFileGoToTargetArgs) (gitsession.GoToTarget, error) {
	return callGit[gitsession.GoToTarget](ctx, s.windows, a.WindowKey, "file.goToTarget", a.Params)
}

// BlameLine answers blame.line.
func (s *GitService) BlameLine(ctx context.Context, a GitBlameLineArgs) (porcelain.BlameLine, error) {
	return callGit[porcelain.BlameLine](ctx, s.windows, a.WindowKey, "blame.line", a.Params)
}

// SearchRun answers search.run.
func (s *GitService) SearchRun(ctx context.Context, a GitSearchRunArgs) (gitrpc.SearchRunResult, error) {
	return callGit[gitrpc.SearchRunResult](ctx, s.windows, a.WindowKey, "search.run", a.Params)
}

// RepoSettingsGet answers repoSettings.get.
func (s *GitService) RepoSettingsGet(ctx context.Context, a GitRepoSettingsGetArgs) (gitrpc.RepoSettingsSnapshot, error) {
	return callGit[gitrpc.RepoSettingsSnapshot](ctx, s.windows, a.WindowKey, "repoSettings.get", a.Params)
}

// RepoSettingsSet answers repoSettings.set; a patch touching a field only Kira Space writes is
// refused before the handler runs.
func (s *GitService) RepoSettingsSet(ctx context.Context, a GitRepoSettingsSetArgs) (gitrpc.RepoSettingsSnapshot, error) {
	if patchTouchesRestrictedField(a.Params.Patch) {
		return gitrpc.RepoSettingsSnapshot{}, errRestrictedSetting()
	}
	return callGit[gitrpc.RepoSettingsSnapshot](ctx, s.windows, a.WindowKey, "repoSettings.set", a.Params)
}
