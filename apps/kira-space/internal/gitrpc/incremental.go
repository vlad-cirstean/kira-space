package gitrpc

import (
	"context"
	"encoding/json"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitclient/porcelain"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitsession"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
)

// Thin dispatch, three handlers — decode, validate (repoId/branch/path non-empty, validRefArg
// on branch/base, the same guard review.resolveBase already uses at the same entrance), entryFor,
// one RepoEntry call, marshal (with the size guard on review.fileDiff). No handler here computes
// anything itself.

// maxRemovedDismissIDs bounds one review.removed.dismiss call.
const maxRemovedDismissIDs = 1000

func (r *Router) handleReviewRemovedList(ctx context.Context, c *gitsession.Conn, params json.RawMessage) (any, error) {
	return handleRepoCall(ctx, c, "review.removed.list", params,
		func(p ReviewRemovedListParams) (string, error) {
			if err := requireNonEmpty("review.removed.list", "repoId", p.RepoID); err != nil {
				return "", err
			}
			if err := validRefArg("branch", p.Branch); err != nil {
				return "", err
			}
			if err := validRefArg("base", p.Base); err != nil {
				return "", err
			}
			return p.RepoID, nil
		},
		func(ctx context.Context, entry *gitsession.RepoEntry, p ReviewRemovedListParams) (gitsession.ReviewRemovedResult, error) {
			result, err := entry.RemovedSinceReview(ctx, p.Base, p.Branch)
			if err != nil {
				return gitsession.ReviewRemovedResult{}, mapDetailError(err)
			}
			return result, nil
		},
	)
}

func (r *Router) handleReviewRemovedDismiss(ctx context.Context, c *gitsession.Conn, params json.RawMessage) (any, error) {
	return handleRepoCall(ctx, c, "review.removed.dismiss", params,
		func(p ReviewRemovedDismissParams) (string, error) {
			if err := requireNonEmpty("review.removed.dismiss", "repoId", p.RepoID); err != nil {
				return "", err
			}
			if err := validRefArg("branch", p.Branch); err != nil {
				return "", err
			}
			if len(p.IDs) == 0 || len(p.IDs) > maxRemovedDismissIDs {
				return "", ipcerr.Contract("gitrpc: review.removed.dismiss: ids must hold 1 to 1000 entries")
			}
			for _, id := range p.IDs {
				if id <= 0 {
					return "", ipcerr.Contract("gitrpc: review.removed.dismiss: every id must be positive")
				}
			}
			return p.RepoID, nil
		},
		func(ctx context.Context, entry *gitsession.RepoEntry, p ReviewRemovedDismissParams) (ReviewRemovedDismissResult, error) {
			n, err := entry.DismissRemoved(ctx, p.Branch, p.IDs)
			if err != nil {
				return ReviewRemovedDismissResult{}, mapDetailError(err)
			}
			return ReviewRemovedDismissResult{Dismissed: n}, nil
		},
	)
}

func (r *Router) handleReviewFiles(ctx context.Context, c *gitsession.Conn, params json.RawMessage) (any, error) {
	return handleRepoCall(ctx, c, "review.files", params,
		func(p ReviewFilesParams) (string, error) {
			if err := requireNonEmpty("review.files", "repoId", p.RepoID); err != nil {
				return "", err
			}
			if err := validRefArg("branch", p.Branch); err != nil {
				return "", err
			}
			if err := validRefArg("base", p.Base); err != nil {
				return "", err
			}
			return p.RepoID, nil
		},
		func(ctx context.Context, entry *gitsession.RepoEntry, p ReviewFilesParams) (json.RawMessage, error) {
			result, err := entry.RangeFiles(ctx, p.Base, p.Branch)
			if err != nil {
				return nil, mapDetailError(err)
			}

			// D15: a file list has no `tooLarge` arm, so an over-cap list is an E_TOO_LARGE-shaped
			// ipcerr naming the file count — unreachable in practice (a FileChange +
			// ReviewFileStatus is well under 300 bytes), but "unreachable" is not "unhandled".
			raw, merr := json.Marshal(result)
			if merr != nil {
				return nil, ipcerr.Internal(merr.Error())
			}
			if len(raw) > MaxResultBytes {
				return nil, ipcerr.T(ipcerr.CodeTooLarge, "backend.space.gitrpc.resultTooLarge.body", nil)
			}
			return json.RawMessage(raw), nil
		},
	)
}

// handleReviewFileDiff is the other size-sensitive handler here: the same marshal-once /
// check MaxResultBytes / replace-body-with-tooLarge / re-marshal shape commit.fileDiff already
// uses — the body is the same FileDiffBody union, so the same {kind: "tooLarge", ...} arm applies
// with no new type.
func (r *Router) handleReviewFileDiff(ctx context.Context, c *gitsession.Conn, params json.RawMessage) (any, error) {
	return handleRepoCall(ctx, c, "review.fileDiff", params,
		func(p ReviewFileDiffParams) (string, error) {
			if err := requireNonEmpty("review.fileDiff", "repoId", p.RepoID, "path", p.Path); err != nil {
				return "", err
			}
			if err := validRefArg("branch", p.Branch); err != nil {
				return "", err
			}
			if err := validRefArg("base", p.Base); err != nil {
				return "", err
			}
			if p.Mode != "range" && p.Mode != "sinceReview" {
				return "", ipcerr.Contract("gitrpc: review.fileDiff: mode must be \"range\" or \"sinceReview\"")
			}
			return p.RepoID, nil
		},
		func(ctx context.Context, entry *gitsession.RepoEntry, p ReviewFileDiffParams) (json.RawMessage, error) {
			result, err := entry.ReviewFileDiff(ctx, p.Base, p.Branch, p.Path, p.Mode)
			if err != nil {
				return nil, mapDetailError(err)
			}

			raw, merr := json.Marshal(result)
			if merr != nil {
				return nil, ipcerr.Internal(merr.Error())
			}
			if len(raw) > MaxResultBytes {
				result.Body = porcelain.FileDiffBody{Kind: porcelain.BodyTooLarge, Bytes: result.RawPatchBytes, LimitBytes: MaxResultBytes}
				raw, merr = json.Marshal(result)
				if merr != nil {
					return nil, ipcerr.Internal(merr.Error())
				}
			}
			return json.RawMessage(raw), nil
		},
	)
}

func (r *Router) handleReviewMark(ctx context.Context, c *gitsession.Conn, params json.RawMessage) (any, error) {
	return handleRepoCall(ctx, c, "review.mark", params,
		func(p ReviewMarkParams) (string, error) {
			if err := requireNonEmpty("review.mark", "repoId", p.RepoID, "path", p.Path); err != nil {
				return "", err
			}
			if err := validRefArg("branch", p.Branch); err != nil {
				return "", err
			}
			// gitreview.Normalize only drops a range where End < Start,
			// and clampToLineCount only clamps the UPPER bound — so a range like {Start:-9, End:0}
			// on a 10-line file was accepted and stored, and CountLines then equalled the file's
			// own lineCount, reading as "fully reviewed" with zero actual lines marked. Every
			// element is checked, not just the first — comments.go's own review.comment.add
			// validation right above enforces the identical rule.
			for _, rng := range p.Ranges {
				if rng.Start < 1 || rng.End < rng.Start {
					return "", ipcerr.Contract("gitrpc: review.mark: range must satisfy 1 <= start <= end")
				}
			}
			return p.RepoID, nil
		},
		func(ctx context.Context, entry *gitsession.RepoEntry, p ReviewMarkParams) (ReviewMarkResult, error) {
			rec, err := entry.MarkFile(ctx, p.Branch, p.Path, p.Reviewed, p.Ranges)
			if err != nil {
				return ReviewMarkResult{}, mapDetailError(err)
			}
			return ReviewMarkResult{Review: gitsession.ReviewFileStatusFor(rec)}, nil
		},
	)
}

func (r *Router) handleReviewMarkMany(ctx context.Context, c *gitsession.Conn, params json.RawMessage) (any, error) {
	return handleRepoCall(ctx, c, "review.markMany", params,
		func(p ReviewMarkManyParams) (string, error) {
			if err := requireNonEmpty("review.markMany", "repoId", p.RepoID); err != nil {
				return "", err
			}
			if err := validRefArg("branch", p.Branch); err != nil {
				return "", err
			}
			if len(p.Paths) == 0 {
				return "", ipcerr.Contract("gitrpc: review.markMany: paths must not be empty")
			}
			for _, path := range p.Paths {
				if path == "" {
					return "", ipcerr.Contract("gitrpc: review.markMany: path must not be empty")
				}
			}
			return p.RepoID, nil
		},
		func(ctx context.Context, entry *gitsession.RepoEntry, p ReviewMarkManyParams) (ReviewMarkManyResult, error) {
			paths := dedupePaths(p.Paths)
			// Each file's record is independent: a mid-batch error keeps earlier writes.
			recs, err := entry.MarkFiles(ctx, p.Branch, paths, p.Reviewed)
			if err != nil {
				return ReviewMarkManyResult{}, mapDetailError(err)
			}
			out := make([]ReviewMarkManyEntry, len(recs))
			for i, rec := range recs {
				out[i] = ReviewMarkManyEntry{Path: paths[i], Review: gitsession.ReviewFileStatusFor(rec)}
			}
			return ReviewMarkManyResult{Results: out}, nil
		},
	)
}

func dedupePaths(paths []string) []string {
	seen := make(map[string]struct{}, len(paths))
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	return out
}

func (r *Router) handleReviewSnapshot(ctx context.Context, c *gitsession.Conn, params json.RawMessage) (any, error) {
	return handleRepoCall(ctx, c, "review.snapshot", params,
		func(p ReviewSnapshotParams) (string, error) {
			if err := requireNonEmpty("review.snapshot", "repoId", p.RepoID, "path", p.Path); err != nil {
				return "", err
			}
			if err := validRefArg("branch", p.Branch); err != nil {
				return "", err
			}
			return p.RepoID, nil
		},
		func(ctx context.Context, entry *gitsession.RepoEntry, p ReviewSnapshotParams) (json.RawMessage, error) {
			res, err := entry.ReviewSnapshot(ctx, p.Branch, p.Path)
			if err != nil {
				return nil, mapDetailError(err)
			}
			raw, merr := json.Marshal(res)
			if merr != nil {
				return nil, ipcerr.Internal(merr.Error())
			}
			if len(raw) > MaxResultBytes {
				raw, merr = json.Marshal(ReviewSnapshotResult{Kind: "tooLarge", ReviewedAtSHA: res.ReviewedAtSHA})
				if merr != nil {
					return nil, ipcerr.Internal(merr.Error())
				}
			}
			return json.RawMessage(raw), nil
		},
	)
}
