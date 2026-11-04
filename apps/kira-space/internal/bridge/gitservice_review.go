package bridge

import (
	"context"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitreview"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitrpc"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitsession"
)

// GitReviewResolveBaseArgs is ReviewResolveBase's request.
type GitReviewResolveBaseArgs struct {
	WindowKey string                         `json:"windowKey"`
	Params    gitrpc.ReviewResolveBaseParams `json:"params"`
}

// ReviewResolveBase answers review.resolveBase.
func (s *GitService) ReviewResolveBase(ctx context.Context, a GitReviewResolveBaseArgs) (gitreview.BaseResolution, error) {
	return callGit[gitreview.BaseResolution](ctx, s.windows, a.WindowKey, "review.resolveBase", a.Params)
}

// GitReviewFilesArgs is ReviewFiles's request.
type GitReviewFilesArgs struct {
	WindowKey string                   `json:"windowKey"`
	Params    gitrpc.ReviewFilesParams `json:"params"`
}

// ReviewFiles answers review.files.
func (s *GitService) ReviewFiles(ctx context.Context, a GitReviewFilesArgs) (gitsession.RangeFilesResult, error) {
	return callGit[gitsession.RangeFilesResult](ctx, s.windows, a.WindowKey, "review.files", a.Params)
}

// GitReviewRemovedListArgs is ReviewRemovedList's request.
type GitReviewRemovedListArgs struct {
	WindowKey string                         `json:"windowKey"`
	Params    gitrpc.ReviewRemovedListParams `json:"params"`
}

// ReviewRemovedList answers review.removed.list.
func (s *GitService) ReviewRemovedList(ctx context.Context, a GitReviewRemovedListArgs) (gitsession.ReviewRemovedResult, error) {
	return callGit[gitsession.ReviewRemovedResult](ctx, s.windows, a.WindowKey, "review.removed.list", a.Params)
}

// GitReviewRemovedDismissArgs is ReviewRemovedDismiss's request.
type GitReviewRemovedDismissArgs struct {
	WindowKey string                            `json:"windowKey"`
	Params    gitrpc.ReviewRemovedDismissParams `json:"params"`
}

// ReviewRemovedDismiss answers review.removed.dismiss.
func (s *GitService) ReviewRemovedDismiss(ctx context.Context, a GitReviewRemovedDismissArgs) (gitrpc.ReviewRemovedDismissResult, error) {
	return callGit[gitrpc.ReviewRemovedDismissResult](ctx, s.windows, a.WindowKey, "review.removed.dismiss", a.Params)
}

// GitReviewFileDiffArgs is ReviewFileDiff's request.
type GitReviewFileDiffArgs struct {
	WindowKey string                      `json:"windowKey"`
	Params    gitrpc.ReviewFileDiffParams `json:"params"`
}

// ReviewFileDiff answers review.fileDiff.
func (s *GitService) ReviewFileDiff(ctx context.Context, a GitReviewFileDiffArgs) (gitsession.ReviewFileDiffResult, error) {
	return callGit[gitsession.ReviewFileDiffResult](ctx, s.windows, a.WindowKey, "review.fileDiff", a.Params)
}

// GitReviewSnapshotArgs is ReviewSnapshot's request.
type GitReviewSnapshotArgs struct {
	WindowKey string                      `json:"windowKey"`
	Params    gitrpc.ReviewSnapshotParams `json:"params"`
}

// ReviewSnapshot answers review.snapshot.
func (s *GitService) ReviewSnapshot(ctx context.Context, a GitReviewSnapshotArgs) (gitrpc.ReviewSnapshotResult, error) {
	return callGit[gitrpc.ReviewSnapshotResult](ctx, s.windows, a.WindowKey, "review.snapshot", a.Params)
}

// GitReviewMarkArgs is ReviewMark's request.
type GitReviewMarkArgs struct {
	WindowKey string                  `json:"windowKey"`
	Params    gitrpc.ReviewMarkParams `json:"params"`
}

// ReviewMark answers review.mark.
func (s *GitService) ReviewMark(ctx context.Context, a GitReviewMarkArgs) (gitrpc.ReviewMarkResult, error) {
	return callGit[gitrpc.ReviewMarkResult](ctx, s.windows, a.WindowKey, "review.mark", a.Params)
}

// GitReviewMarkManyArgs is ReviewMarkMany's request.
type GitReviewMarkManyArgs struct {
	WindowKey string                      `json:"windowKey"`
	Params    gitrpc.ReviewMarkManyParams `json:"params"`
}

// ReviewMarkMany answers review.markMany.
func (s *GitService) ReviewMarkMany(ctx context.Context, a GitReviewMarkManyArgs) (gitrpc.ReviewMarkManyResult, error) {
	return callGit[gitrpc.ReviewMarkManyResult](ctx, s.windows, a.WindowKey, "review.markMany", a.Params)
}

// GitReviewCommentAddArgs is ReviewCommentAdd's request.
type GitReviewCommentAddArgs struct {
	WindowKey string                        `json:"windowKey"`
	Params    gitrpc.ReviewCommentAddParams `json:"params"`
}

// ReviewCommentAdd answers review.comment.add.
func (s *GitService) ReviewCommentAdd(ctx context.Context, a GitReviewCommentAddArgs) (gitrpc.ReviewCommentAddResult, error) {
	return callGit[gitrpc.ReviewCommentAddResult](ctx, s.windows, a.WindowKey, "review.comment.add", a.Params)
}

// GitReviewCommentListArgs is ReviewCommentList's request.
type GitReviewCommentListArgs struct {
	WindowKey string                         `json:"windowKey"`
	Params    gitrpc.ReviewCommentListParams `json:"params"`
}

// ReviewCommentList answers review.comment.list.
func (s *GitService) ReviewCommentList(ctx context.Context, a GitReviewCommentListArgs) (gitsession.CommentListResult, error) {
	return callGit[gitsession.CommentListResult](ctx, s.windows, a.WindowKey, "review.comment.list", a.Params)
}

// GitReviewCommentRemoveArgs is ReviewCommentRemove's request.
type GitReviewCommentRemoveArgs struct {
	WindowKey string                           `json:"windowKey"`
	Params    gitrpc.ReviewCommentRemoveParams `json:"params"`
}

// ReviewCommentRemove answers review.comment.remove.
func (s *GitService) ReviewCommentRemove(ctx context.Context, a GitReviewCommentRemoveArgs) (gitrpc.ReviewCommentRemoveResult, error) {
	return callGit[gitrpc.ReviewCommentRemoveResult](ctx, s.windows, a.WindowKey, "review.comment.remove", a.Params)
}

// GitReviewCommentSetStateArgs is ReviewCommentSetState's request.
type GitReviewCommentSetStateArgs struct {
	WindowKey string                             `json:"windowKey"`
	Params    gitrpc.ReviewCommentSetStateParams `json:"params"`
}

// ReviewCommentSetState answers review.comment.setState.
func (s *GitService) ReviewCommentSetState(ctx context.Context, a GitReviewCommentSetStateArgs) (gitrpc.ReviewCommentSetStateResult, error) {
	return callGit[gitrpc.ReviewCommentSetStateResult](ctx, s.windows, a.WindowKey, "review.comment.setState", a.Params)
}

// GitReviewCommentEditArgs is ReviewCommentEdit's request.
type GitReviewCommentEditArgs struct {
	WindowKey string                         `json:"windowKey"`
	Params    gitrpc.ReviewCommentEditParams `json:"params"`
}

// ReviewCommentEdit answers review.comment.edit.
func (s *GitService) ReviewCommentEdit(ctx context.Context, a GitReviewCommentEditArgs) (gitrpc.ReviewCommentEditResult, error) {
	return callGit[gitrpc.ReviewCommentEditResult](ctx, s.windows, a.WindowKey, "review.comment.edit", a.Params)
}

// GitReviewCommentClearArgs is ReviewCommentClear's request.
type GitReviewCommentClearArgs struct {
	WindowKey string                          `json:"windowKey"`
	Params    gitrpc.ReviewCommentClearParams `json:"params"`
}

// ReviewCommentClear answers review.comment.clear.
func (s *GitService) ReviewCommentClear(ctx context.Context, a GitReviewCommentClearArgs) (gitrpc.ReviewCommentClearResult, error) {
	return callGit[gitrpc.ReviewCommentClearResult](ctx, s.windows, a.WindowKey, "review.comment.clear", a.Params)
}

// GitReviewCommentExportArgs is ReviewCommentExport's request.
type GitReviewCommentExportArgs struct {
	WindowKey string                           `json:"windowKey"`
	Params    gitrpc.ReviewCommentExportParams `json:"params"`
}

// ReviewCommentExport answers review.comment.export.
func (s *GitService) ReviewCommentExport(ctx context.Context, a GitReviewCommentExportArgs) (gitrpc.ReviewCommentExportResult, error) {
	return callGit[gitrpc.ReviewCommentExportResult](ctx, s.windows, a.WindowKey, "review.comment.export", a.Params)
}

// GitCommitResolveChangeRequestsArgs is CommitResolveChangeRequests's request.
type GitCommitResolveChangeRequestsArgs struct {
	WindowKey string                                   `json:"windowKey"`
	Params    gitrpc.CommitResolveChangeRequestsParams `json:"params"`
}

// CommitResolveChangeRequests answers commit.resolveChangeRequests.
func (s *GitService) CommitResolveChangeRequests(ctx context.Context, a GitCommitResolveChangeRequestsArgs) (gitrpc.ChangeRequestLookup, error) {
	return callGit[gitrpc.ChangeRequestLookup](ctx, s.windows, a.WindowKey, "commit.resolveChangeRequests", a.Params)
}

// GitBranchResolveChangeRequestArgs is BranchResolveChangeRequest's request.
type GitBranchResolveChangeRequestArgs struct {
	WindowKey string                                  `json:"windowKey"`
	Params    gitrpc.BranchResolveChangeRequestParams `json:"params"`
}

// BranchResolveChangeRequest answers branch.resolveChangeRequest; calls the code host.
func (s *GitService) BranchResolveChangeRequest(ctx context.Context, a GitBranchResolveChangeRequestArgs) (gitrpc.ChangeRequestLookup, error) {
	return callGit[gitrpc.ChangeRequestLookup](ctx, s.windows, a.WindowKey, "branch.resolveChangeRequest", a.Params)
}

// GitBranchChangeRequestIndexArgs is BranchChangeRequestIndex's request.
type GitBranchChangeRequestIndexArgs struct {
	WindowKey string                                `json:"windowKey"`
	Params    gitrpc.BranchChangeRequestIndexParams `json:"params"`
}

// BranchChangeRequestIndex answers branch.changeRequestIndex.
func (s *GitService) BranchChangeRequestIndex(ctx context.Context, a GitBranchChangeRequestIndexArgs) (gitrpc.BranchChangeRequestIndexResult, error) {
	return callGit[gitrpc.BranchChangeRequestIndexResult](ctx, s.windows, a.WindowKey, "branch.changeRequestIndex", a.Params)
}

// GitBranchSyncChangeRequestsArgs is BranchSyncChangeRequests's request.
type GitBranchSyncChangeRequestsArgs struct {
	WindowKey string                                `json:"windowKey"`
	Params    gitrpc.BranchSyncChangeRequestsParams `json:"params"`
}

// BranchSyncChangeRequests answers branch.syncChangeRequests; calls the code host.
func (s *GitService) BranchSyncChangeRequests(ctx context.Context, a GitBranchSyncChangeRequestsArgs) (struct{}, error) {
	return callGit[struct{}](ctx, s.windows, a.WindowKey, "branch.syncChangeRequests", a.Params)
}

// GitGitHostInfoArgs is GitHostInfo's request.
type GitGitHostInfoArgs struct {
	WindowKey string                   `json:"windowKey"`
	Params    gitrpc.GitHostInfoParams `json:"params"`
}

// GitHostInfo answers gitHost.info.
func (s *GitService) GitHostInfo(ctx context.Context, a GitGitHostInfoArgs) (gitrpc.GitHostInfoResult, error) {
	return callGit[gitrpc.GitHostInfoResult](ctx, s.windows, a.WindowKey, "gitHost.info", a.Params)
}

// GitChangeRequestBrowserUrlArgs is ChangeRequestBrowserUrl's request.
type GitChangeRequestBrowserUrlArgs struct {
	WindowKey string                               `json:"windowKey"`
	Params    gitrpc.ChangeRequestBrowserUrlParams `json:"params"`
}

// ChangeRequestBrowserUrl answers changeRequest.browserUrl.
func (s *GitService) ChangeRequestBrowserUrl(ctx context.Context, a GitChangeRequestBrowserUrlArgs) (gitrpc.ChangeRequestBrowserUrlResult, error) {
	return callGit[gitrpc.ChangeRequestBrowserUrlResult](ctx, s.windows, a.WindowKey, "changeRequest.browserUrl", a.Params)
}

// GitChangeRequestReviewStateArgs is ChangeRequestReviewState's request.
type GitChangeRequestReviewStateArgs struct {
	WindowKey string                                `json:"windowKey"`
	Params    gitrpc.ChangeRequestReviewStateParams `json:"params"`
}

// ChangeRequestReviewState answers changeRequest.reviewState; calls the code host.
func (s *GitService) ChangeRequestReviewState(ctx context.Context, a GitChangeRequestReviewStateArgs) (gitrpc.ChangeRequestReviewStateResult, error) {
	return callGit[gitrpc.ChangeRequestReviewStateResult](ctx, s.windows, a.WindowKey, "changeRequest.reviewState", a.Params)
}

// GitChangeRequestSubmitReviewArgs is ChangeRequestSubmitReview's request.
type GitChangeRequestSubmitReviewArgs struct {
	WindowKey string                                 `json:"windowKey"`
	Params    gitrpc.ChangeRequestSubmitReviewParams `json:"params"`
}

// ChangeRequestSubmitReview answers changeRequest.submitReview; calls the code host.
func (s *GitService) ChangeRequestSubmitReview(ctx context.Context, a GitChangeRequestSubmitReviewArgs) (gitrpc.ChangeRequestPostResult, error) {
	return callGit[gitrpc.ChangeRequestPostResult](ctx, s.windows, a.WindowKey, "changeRequest.submitReview", a.Params)
}

// GitChangeRequestCommentArgs is ChangeRequestComment's request.
type GitChangeRequestCommentArgs struct {
	WindowKey string                            `json:"windowKey"`
	Params    gitrpc.ChangeRequestCommentParams `json:"params"`
}

// ChangeRequestComment answers changeRequest.comment; calls the code host.
func (s *GitService) ChangeRequestComment(ctx context.Context, a GitChangeRequestCommentArgs) (gitrpc.ChangeRequestPostResult, error) {
	return callGit[gitrpc.ChangeRequestPostResult](ctx, s.windows, a.WindowKey, "changeRequest.comment", a.Params)
}

// GitChangeRequestLineCommentArgs is ChangeRequestLineComment's request.
type GitChangeRequestLineCommentArgs struct {
	WindowKey string                                `json:"windowKey"`
	Params    gitrpc.ChangeRequestLineCommentParams `json:"params"`
}

// ChangeRequestLineComment answers changeRequest.lineComment; calls the code host.
func (s *GitService) ChangeRequestLineComment(ctx context.Context, a GitChangeRequestLineCommentArgs) (gitrpc.ChangeRequestLineCommentResult, error) {
	return callGit[gitrpc.ChangeRequestLineCommentResult](ctx, s.windows, a.WindowKey, "changeRequest.lineComment", a.Params)
}
