package gitrpc

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitclient/porcelain"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitpath"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitsession"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
)

// mapDetailError maps gitsession's own closed error vocabulary for the four queries into
// ipcerr — every one of these is a caller mistake the webview cannot have made from data it read
// out of a previous result (the validation rules), so all three are E_BAD_REQUEST; any
// other error falls through to mapGitError, same as every other repo-touching handler.
func mapDetailError(err error) error {
	switch {
	case errors.Is(err, gitsession.ErrParentIndexOutOfRange):
		return ipcerr.T(ipcerr.CodeBadRequest, "backend.space.gitrpc.parentOutOfRange.body", nil)
	case errors.Is(err, gitsession.ErrFileNotInCommit):
		return ipcerr.T(ipcerr.CodeBadRequest, "backend.space.gitrpc.fileNotInCommit.body", nil)
	case errors.Is(err, gitsession.ErrPathEscapesRoot):
		return ipcerr.T(ipcerr.CodeBadRequest, "backend.space.gitrpc.pathEscapesRoot.body", nil)
	case errors.Is(err, gitsession.ErrBranchNotFound):
		return ipcerr.T(ipcerr.CodeBadRequest, "backend.space.ade.branchMissing.body", nil)
	case errors.Is(err, gitsession.ErrUnrelatedHistories):
		return ipcerr.T(ipcerr.CodeBadRequest, "backend.space.gitrpc.noSharedHistory.body", nil)
	case errors.Is(err, gitsession.ErrRangedMarkOnNonText):
		return ipcerr.T(ipcerr.CodeBadRequest, "backend.space.gitrpc.rangedMarkNeedsText.body", nil)
	case errors.Is(err, gitsession.ErrCommentNotText):
		return ipcerr.T(ipcerr.CodeBadRequest, "backend.space.gitrpc.commentNeedsText.body", nil)
	case errors.Is(err, gitsession.ErrCommentRangeOutOfFile):
		return ipcerr.T(ipcerr.CodeBadRequest, "backend.space.gitrpc.commentRangePastEnd.body", nil)
	default:
		return mapGitError(err)
	}
}

// entryFor resolves c's held RepoEntry for repoID, or ErrRepoNotHeld mapped the same way graph.*
// already maps it — repo.open must precede every one of these four methods, exactly as it
// must for every other per-repo request. repoID is normalized to NFC here, at the layer
// that owns wire ingestion — Registry.Entry stays a pure map lookup with no opinion about string
// forms, and a persisted client's un-normalized repoId resolves against the registry rather
// than erroring.
func entryFor(c *gitsession.Conn, repoID string) (*gitsession.RepoEntry, error) {
	entry, ok := c.Entry(gitpath.CleanNFC(repoID)) // G27 D6
	if !ok {
		return nil, mapGitError(gitsession.ErrRepoNotHeld)
	}
	return entry, nil
}

func parentIndexOrDefault(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

func stringOrEmpty(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func (r *Router) handleCommitDetail(ctx context.Context, c *gitsession.Conn, params json.RawMessage) (any, error) {
	return handleRepoCall(ctx, c, "commit.detail", params,
		func(p CommitDetailParams) (string, error) {
			if p.RepoID == "" || p.SHA == "" {
				return "", ipcerr.Contract("gitrpc: commit.detail: repoId and sha are required")
			}
			if err := validRefArg("sha", p.SHA); err != nil {
				return "", err
			}
			return p.RepoID, nil
		},
		func(ctx context.Context, entry *gitsession.RepoEntry, p CommitDetailParams) (porcelain.CommitDetail, error) {
			detail, err := entry.CommitDetail(ctx, p.SHA, parentIndexOrDefault(p.ParentIndex))
			if err != nil {
				return porcelain.CommitDetail{}, mapDetailError(err)
			}
			return detail, nil
		},
	)
}

// handleCommitFileDiff is one of the two size-sensitive handlers: it marshals its own result
// once and, only if the *encoded* result exceeds MaxResultBytes, replaces the body with
// FileDiffBody's own "tooLarge" arm (carrying the real raw patch size, gitsession.FileDiffResult's
// own RawPatchBytes — never re-derived from the encoded bytes) and marshals again. Returned as
// json.RawMessage, which the bound call's json.Marshal passes through unchanged.
func (r *Router) handleCommitFileDiff(ctx context.Context, c *gitsession.Conn, params json.RawMessage) (any, error) {
	var p CommitFileDiffParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, ipcerr.Contract("gitrpc: commit.fileDiff: invalid params")
	}
	if p.RepoID == "" || p.SHA == "" || p.Path == "" {
		return nil, ipcerr.Contract("gitrpc: commit.fileDiff: repoId, sha and path are required")
	}
	if err := validRefArg("sha", p.SHA); err != nil {
		return nil, err
	}
	entry, err := entryFor(c, p.RepoID)
	if err != nil {
		return nil, err
	}
	result, err := entry.FileDiff(ctx, p.SHA, p.Path, stringOrEmpty(p.OriginalPath), parentIndexOrDefault(p.ParentIndex))
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
}

// handleFileRead is D2(b)'s other size-sensitive handler — the same marshal-once, cap-check,
// re-marshal-if-needed shape as commit.fileDiff, over file.read's own result union.
func (r *Router) handleFileRead(ctx context.Context, c *gitsession.Conn, params json.RawMessage) (any, error) {
	var p FileReadParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, ipcerr.Contract("gitrpc: file.read: invalid params")
	}
	if p.RepoID == "" || p.Rev == "" || p.Path == "" {
		return nil, ipcerr.Contract("gitrpc: file.read: repoId, rev and path are required")
	}
	// D8 (see validRefArg, review.go): Rev reaches `git show <rev>:<path>` as a bare argv token
	// (gitclient/catfile) — unguarded, a rev beginning with "-" would be read as an option.
	if err := validRefArg("rev", p.Rev); err != nil {
		return nil, err
	}
	entry, err := entryFor(c, p.RepoID)
	if err != nil {
		return nil, err
	}
	result, err := entry.Blob(ctx, p.Rev, p.Path)
	if err != nil {
		return nil, mapDetailError(err)
	}

	raw, merr := json.Marshal(result)
	if merr != nil {
		return nil, ipcerr.Internal(merr.Error())
	}
	if len(raw) > MaxResultBytes {
		result = gitsession.BlobResult{Kind: "tooLarge", Bytes: int64(len(result.Content)), LimitBytes: MaxResultBytes}
		raw, merr = json.Marshal(result)
		if merr != nil {
			return nil, ipcerr.Internal(merr.Error())
		}
	}
	return json.RawMessage(raw), nil
}

func (r *Router) handleFileGoToTarget(ctx context.Context, c *gitsession.Conn, params json.RawMessage) (any, error) {
	var p FileGoToTargetParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, ipcerr.Contract("gitrpc: file.goToTarget: invalid params")
	}
	if p.RepoID == "" || p.Rev == "" || p.Path == "" {
		return nil, ipcerr.Contract("gitrpc: file.goToTarget: repoId, rev and path are required")
	}
	// D8 (see validRefArg, review.go): Rev reaches `git diff <rev> -- <path>` as a bare argv token
	// in an option-consuming position (gitclient/porcelain.WorktreeDiffArgs) — unguarded, a rev of
	// "--output=<path>" makes `git diff` write to an arbitrary file instead of erroring.
	if err := validRefArg("rev", p.Rev); err != nil {
		return nil, err
	}
	entry, err := entryFor(c, p.RepoID)
	if err != nil {
		return nil, err
	}
	result, err := entry.GoToTarget(ctx, p.Rev, p.Path)
	if err != nil {
		return nil, mapDetailError(err)
	}
	return result, nil
}

// handleBlameLine is P5's own handler — no size guard needed (unlike commit.fileDiff/file.read):
// a BlameLine result is a handful of short fields, nowhere near MaxResultBytes.
func (r *Router) handleBlameLine(ctx context.Context, c *gitsession.Conn, params json.RawMessage) (any, error) {
	return handleRepoCall(ctx, c, "blame.line", params,
		func(p BlameLineParams) (string, error) {
			if p.RepoID == "" || p.Path == "" || p.Line < 1 {
				return "", ipcerr.Contract("gitrpc: blame.line: repoId, path and a 1-based line are required")
			}
			return p.RepoID, nil
		},
		func(ctx context.Context, entry *gitsession.RepoEntry, p BlameLineParams) (porcelain.BlameLine, error) {
			line, err := entry.BlameLine(ctx, p.Path, p.Line)
			if err != nil {
				// ErrPathEscapesRoot is already mapped by mapDetailError; everything else (an
				// untracked path, a line past EOF) falls to mapGitError's default arm.
				return porcelain.BlameLine{}, mapDetailError(err)
			}
			return line, nil
		},
	)
}

// handleWorkingDetail is working.detail's own handler (P7, item 2) — no size guard needed
// (unlike commit.fileDiff/file.read): a FileChange list is bounded by the working tree's own dirty
// set, the same order of magnitude status.get already returns uncapped-and-unguarded today.
func (r *Router) handleWorkingDetail(ctx context.Context, c *gitsession.Conn, params json.RawMessage) (any, error) {
	return handleRepoCall(ctx, c, "working.detail", params,
		func(p WorkingDetailParams) (string, error) {
			if p.RepoID == "" {
				return "", ipcerr.Contract("gitrpc: working.detail: repoId is required")
			}
			return p.RepoID, nil
		},
		func(ctx context.Context, entry *gitsession.RepoEntry, _ WorkingDetailParams) (WorkingDetailResult, error) {
			files, err := entry.WorkingDetail(ctx)
			if err != nil {
				return WorkingDetailResult{}, mapDetailError(err)
			}
			return WorkingDetailResult{Files: files}, nil
		},
	)
}
