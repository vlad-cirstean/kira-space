package bridge

import (
	"context"
	"slices"
	"strings"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitreview"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitsession"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
)

// adetask_review_working.go: review of the uncommitted work in a branch's worktree (P418).

const (
	adeMaxWorkingMarkPaths  = 1000
	adeMaxWorkingMarkRanges = 256
	adeMaxStampPaths        = 5000
)

func validateWorkingRange(r gitreview.LineRange) error {
	if r.Start < 1 || r.End < r.Start {
		return adeTaskInvalid("range must satisfy 1 <= start <= end")
	}
	return nil
}

// ReviewWorkingFile reads one uncommitted file's diff and its reviewed ranges.
func (s *AdeTaskService) ReviewWorkingFile(ctx context.Context, args adewire.ReviewWorkingDiffArgs) (adewire.ReviewWorkingFileReview, error) {
	ctx = adeUserCtx(ctx)
	if err := validateAdeItemID(args.BranchID, "branchId"); err != nil {
		return adewire.ReviewWorkingFileReview{}, err
	}
	if args.Path == "" {
		return adewire.ReviewWorkingFileReview{}, adeTaskInvalid("path is required")
	}
	r, err := s.Engine.ReviewWorkingFile(ctx, args.BranchID, args.Path)
	return r, adeTaskError(err)
}

// ReviewWorkingMark marks or unmarks uncommitted files as reviewed, whole or by line ranges.
func (s *AdeTaskService) ReviewWorkingMark(ctx context.Context, args adewire.ReviewWorkingMarkArgs) (adewire.ReviewWorkingMarkResult, error) {
	ctx = adeUserCtx(ctx)
	if err := validateAdeItemID(args.BranchID, "branchId"); err != nil {
		return adewire.ReviewWorkingMarkResult{}, err
	}
	if len(args.Paths) == 0 {
		return adewire.ReviewWorkingMarkResult{}, adeTaskInvalid("paths is required")
	}
	if len(args.Paths) > adeMaxWorkingMarkPaths {
		return adewire.ReviewWorkingMarkResult{}, adeTaskInvalid("too many paths")
	}
	for _, p := range args.Paths {
		if p == "" {
			return adewire.ReviewWorkingMarkResult{}, adeTaskInvalid("path is required")
		}
	}
	for p := range args.ContentHashes {
		if !slices.Contains(args.Paths, p) {
			return adewire.ReviewWorkingMarkResult{}, adeTaskInvalid("contentHashes names a path outside paths")
		}
	}
	if args.Ranges != nil {
		if len(args.Paths) != 1 {
			return adewire.ReviewWorkingMarkResult{}, adeTaskInvalid("ranges need exactly one path")
		}
		if len(args.Ranges) > adeMaxWorkingMarkRanges {
			return adewire.ReviewWorkingMarkResult{}, adeTaskInvalid("too many ranges")
		}
		for _, r := range args.Ranges {
			if err := validateWorkingRange(r); err != nil {
				return adewire.ReviewWorkingMarkResult{}, err
			}
		}
	}
	r, err := s.Engine.ReviewWorkingMark(ctx, args)
	return r, adeTaskError(err)
}

// ReviewWorkingStamp digests a stat of the listed worktree files, for change polling.
func (s *AdeTaskService) ReviewWorkingStamp(ctx context.Context, args adewire.ReviewWorkingStampArgs) (adewire.ReviewWorkingStamp, error) {
	ctx = adeUserCtx(ctx)
	if err := validateAdeItemID(args.BranchID, "branchId"); err != nil {
		return adewire.ReviewWorkingStamp{}, err
	}
	if len(args.Paths) > adeMaxStampPaths {
		return adewire.ReviewWorkingStamp{}, adeTaskInvalid("too many paths")
	}
	if slices.Contains(args.Paths, "") {
		return adewire.ReviewWorkingStamp{}, adeTaskInvalid("path is required")
	}
	stamp, err := s.Engine.ReviewWorkingStamp(ctx, args.BranchID, args.Paths)
	return adewire.ReviewWorkingStamp{Stamp: stamp}, adeTaskError(err)
}

// ReviewWorkingComments lists the branch's uncommitted-origin review comments.
func (s *AdeTaskService) ReviewWorkingComments(ctx context.Context, args adewire.BranchArgs) (adewire.ReviewWorkingCommentsResult, error) {
	ctx = adeUserCtx(ctx)
	if err := validateAdeItemID(args.BranchID, "branchId"); err != nil {
		return adewire.ReviewWorkingCommentsResult{}, err
	}
	r, err := s.Engine.ReviewWorkingComments(ctx, args.BranchID)
	return r, adeTaskError(err)
}

// ReviewWorkingCommentAdd adds a review comment on uncommitted text.
func (s *AdeTaskService) ReviewWorkingCommentAdd(ctx context.Context, args adewire.ReviewWorkingCommentAddArgs) (gitsession.CommentEntry, error) {
	ctx = adeUserCtx(ctx)
	if err := validateAdeItemID(args.BranchID, "branchId"); err != nil {
		return gitsession.CommentEntry{}, err
	}
	if args.Path == "" {
		return gitsession.CommentEntry{}, adeTaskInvalid("path is required")
	}
	if err := validateWorkingRange(args.Range); err != nil {
		return gitsession.CommentEntry{}, err
	}
	if args.Kind != gitreview.CommentKindChange && args.Kind != gitreview.CommentKindQuestion {
		return gitsession.CommentEntry{}, adeTaskInvalid("kind must be change or question")
	}
	body := strings.ReplaceAll(strings.ReplaceAll(args.Body, "\r\n", "\n"), "\r", "\n")
	args.Body = strings.TrimSpace(body)
	if args.Body == "" {
		return gitsession.CommentEntry{}, ipcerr.T(ipcerr.CodeBadRequest, "backend.space.gitrpc.commentEmpty.body", nil)
	}
	if len(args.Body) > gitreview.MaxCommentBytes {
		return gitsession.CommentEntry{}, ipcerr.T(ipcerr.CodeBadRequest, "backend.space.gitrpc.commentTooLong.body", nil)
	}
	c, err := s.Engine.ReviewWorkingCommentAdd(ctx, args)
	return c, adeTaskError(err)
}
