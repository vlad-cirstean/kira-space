package gitsession

import (
	"context"
	"errors"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitclient/porcelain"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitreview"
)

// ErrCommentNotText is review.comment.add's own refusal for a path that is not text at `at` —
// binary/tooLarge/absent, reusing readSnapshotSource's own classification (F11).
var ErrCommentNotText = errors.New("gitsession: a comment requires a text file")

// ErrCommentRangeOutOfFile is review.comment.add's own refusal for a range past `at`'s own line
// count — validated against the file rather than trusted, so a caller is told rather than served a
// comment that silently drifts once projected (D15).
var ErrCommentRangeOutOfFile = errors.New("gitsession: comment range is past the end of the file")

// CommentEntry mirrors @kira/git-ipc's own ReviewComment (D11).
type CommentEntry struct {
	ID        string                  `json:"id"`
	Path      string                  `json:"path"`
	Range     gitreview.LineRange     `json:"range"`
	Body      string                  `json:"body"`
	Anchor    gitreview.CommentAnchor `json:"anchor"`
	AnchorSHA string                  `json:"anchorSha"`
	CreatedAt int64                   `json:"createdAt"`
	Kind      string                  `json:"kind"`
	State     string                  `json:"state"`
	Reply     string                  `json:"reply"`
	RepliedAt *int64                  `json:"repliedAt"`
	SentAt    *int64                  `json:"sentAt"`
	Origin    string                  `json:"origin"`
}

// CommentListResult mirrors @kira/git-ipc's own review.comment.list result.
type CommentListResult struct {
	At       string         `json:"at"`
	Comments []CommentEntry `json:"comments"`
}

func toCommentEntry(ac gitreview.AnchoredComment) CommentEntry {
	origin := ac.Origin
	if origin == "" {
		origin = gitreview.OriginCommit
	}
	return CommentEntry{
		ID: ac.ID, Origin: origin, Path: ac.Path, Range: ac.Range, Body: ac.Body,
		Anchor: ac.Anchor, AnchorSHA: ac.AnchorSHA, CreatedAt: ac.CreatedAt.UnixMilli(),
		Kind: ac.Kind, State: ac.State, Reply: ac.Reply,
		RepliedAt: millisPtr(ac.RepliedAt), SentAt: millisPtr(ac.SentAt),
	}
}

func millisPtr(t *time.Time) *int64 {
	if t == nil {
		return nil
	}
	ms := t.UnixMilli()
	return &ms
}

func toCommentEntries(acs []gitreview.AnchoredComment) []CommentEntry {
	out := make([]CommentEntry, 0, len(acs))
	for _, ac := range acs {
		out = append(out, toCommentEntry(ac))
	}
	return out
}

// AddComment is review.comment.add's whole orchestration: readSnapshotSource(ctx, at,
// path) is the identical call MarkFile makes — refuse non-text, bound the range by its line
// count at `at` — then blobOID(at, path) captures tier 0's own comparand and the row is written
// anchored to `at`. `at` is the revision the caller says it was reading; there is no default
// (the refusal to guess).
func (e *RepoEntry) AddComment(ctx context.Context, branch, path, at string, r gitreview.LineRange, body, kind string) (CommentEntry, error) {
	// G31 round-2 performance review, finding #5 (same fix as MarkFile's): blobOID comes straight
	// off readSnapshotSource's own return now, not a second blobOID(ctx, at, path) round trip for
	// the identical rev — valid here since a non-ContentText kind already returns above.
	contentKind, _, lineCount, blobOID, err := e.readSnapshotSource(ctx, at, path)
	if err != nil {
		return CommentEntry{}, err
	}
	if contentKind != gitreview.ContentText {
		return CommentEntry{}, ErrCommentNotText
	}
	if r.End > lineCount {
		return CommentEntry{}, ErrCommentRangeOutOfFile
	}

	stored, err := e.review.AddComment(ctx, e.Summary.RepoID, branch, gitreview.Comment{
		Path: path, Range: r, Body: body, AnchorSHA: at, AnchorBlobOID: blobOID, CreatedAt: time.Now(), Kind: kind,
	})
	if err != nil {
		return CommentEntry{}, err
	}
	// The row was just anchored to `at` itself, so tier 0's own comparand trivially holds — no
	// second round trip through anchorOne to learn what its own write already knows.
	return toCommentEntry(gitreview.AnchoredComment{
		Comment: stored, Anchor: gitreview.AnchorExact, Range: stored.Range,
	}), nil
}

// anchoredComments resolves `at` (branch tip when empty), loads every stored comment, anchors
// each against `at` and applies the total order — the one computation ListComments and
// ExportComments share, so the two can never disagree about what "ordered by file then line"
// means. Touches the session (list/export are reads that keep it alive, never create one).
func (e *RepoEntry) anchoredComments(ctx context.Context, branch, at string) (string, []gitreview.AnchoredComment, error) {
	if at == "" {
		tip, err := e.branchTip(ctx, branch)
		if err != nil {
			return "", nil, err
		}
		at = tip
	}

	stored, err := e.review.Comments(ctx, e.Summary.RepoID, branch)
	if err != nil {
		return "", nil, err
	}
	anchored, err := e.anchorAll(ctx, at, stored)
	if err != nil {
		return "", nil, err
	}
	gitreview.SortAnchored(anchored)

	if err := e.review.Touch(ctx, e.Summary.RepoID, branch); err != nil {
		return "", nil, err
	}
	return at, anchored, nil
}

// ListComments is review.comment.list's whole orchestration.
func (e *RepoEntry) ListComments(ctx context.Context, branch, at string) (CommentListResult, error) {
	resolvedAt, anchored, err := e.anchoredComments(ctx, branch, at)
	if err != nil {
		return CommentListResult{}, err
	}
	return CommentListResult{At: resolvedAt, Comments: toCommentEntries(anchored)}, nil
}

// ExportComments is review.comment.export's whole orchestration — the same anchored slice
// ListComments computes, rendered by gitreview.FormatComments instead of marshaled onto the wire.
func (e *RepoEntry) ExportComments(ctx context.Context, branch, at string) (string, string, error) {
	resolvedAt, anchored, err := e.anchoredComments(ctx, branch, at)
	if err != nil {
		return "", "", err
	}
	return resolvedAt, gitreview.FormatComments(branch, anchored), nil
}

// RemoveComment is review.comment.remove's whole orchestration — a thin pass-through plus Touch
// (D16); the store's own statement is what scopes the delete to this session and makes it
// idempotent (D11).
func (e *RepoEntry) RemoveComment(ctx context.Context, branch string, id string) (bool, error) {
	removed, err := e.review.RemoveComment(ctx, e.Summary.RepoID, branch, id)
	if err != nil {
		return false, err
	}
	if err := e.review.Touch(ctx, e.Summary.RepoID, branch); err != nil {
		return false, err
	}
	return removed, nil
}

// SetCommentState is review.comment.setState's whole orchestration.
func (e *RepoEntry) SetCommentState(ctx context.Context, branch string, id string, state string) (bool, error) {
	changed, err := e.review.SetCommentState(ctx, e.Summary.RepoID, branch, id, state)
	if err != nil {
		return false, err
	}
	if err := e.review.Touch(ctx, e.Summary.RepoID, branch); err != nil {
		return false, err
	}
	return changed, nil
}

// EditComment is review.comment.edit's whole orchestration.
func (e *RepoEntry) EditComment(ctx context.Context, branch string, id string, body, kind string) (bool, error) {
	changed, err := e.review.EditComment(ctx, e.Summary.RepoID, branch, id, body, kind)
	if err != nil {
		return false, err
	}
	if err := e.review.Touch(ctx, e.Summary.RepoID, branch); err != nil {
		return false, err
	}
	return changed, nil
}

// ClearComments is review.comment.clear's whole orchestration — a thin pass-through plus Touch.
func (e *RepoEntry) ClearComments(ctx context.Context, branch string) (int, error) {
	n, err := e.review.ClearComments(ctx, e.Summary.RepoID, branch)
	if err != nil {
		return 0, err
	}
	if err := e.review.Touch(ctx, e.Summary.RepoID, branch); err != nil {
		return 0, err
	}
	return n, nil
}

// lineCountAt memoises readSnapshotSource's own line count per path for one anchorAll call — a
// file usually carries several comments, and this turns N comments into at most one snapshot read
// per distinct path.
func (e *RepoEntry) lineCountAt(ctx context.Context, at, path string, cache map[string]int) (int, error) {
	if n, ok := cache[path]; ok {
		return n, nil
	}
	_, _, n, _, err := e.readSnapshotSource(ctx, at, path)
	if err != nil {
		return 0, err
	}
	cache[path] = n
	return n, nil
}

// anchorAll is the tier machine, run once per comment with two memos scoped to this one call:
// lineCounts keyed by path, patches keyed by "path\x00anchorSHA" — a file usually carries
// several comments and often a single anchor sha, so this turns N comments into ~1 diff per
// (path, sha) pair rather than one per comment.
func (e *RepoEntry) anchorAll(ctx context.Context, at string, cs []gitreview.Comment) ([]gitreview.AnchoredComment, error) {
	lineCounts := make(map[string]int)
	patches := make(map[string][]porcelain.DiffHunk)

	out := make([]gitreview.AnchoredComment, 0, len(cs))
	for _, c := range cs {
		ac, err := e.anchorOne(ctx, at, c, lineCounts, patches)
		if err != nil {
			return nil, err
		}
		out = append(out, ac)
	}
	return out, nil
}

// anchorOne is the three-tier resolution for one comment, verbatim:
//
//  0. blobOID(at, c.Path) == c.AnchorBlobOID — byte-identical content, no diff at all: exact.
//  1. merge-base --is-ancestor c.AnchorSHA at, exit 0 — an ordinary git diff maps the range
//     forward through gitreview.ProjectRanges, unchanged (no new arithmetic): a non-empty
//     result is projected (unioned, since a projection can split a range across an insertion), an
//     empty one is removed (the lines no longer exist).
//  2. exit 1 (unreachable but present, the common rewrite case) or exit 128 (genuinely pruned) —
//     both take stale: there is no honest mapping to compute, so the original range and sha are
//     reported rather than guessed. Any other exit is the classified error.
//
// A working-origin comment (P418) skips the tiers: anchor "uncommitted", stored range.
func (e *RepoEntry) anchorOne(
	ctx context.Context, at string, c gitreview.Comment,
	lineCounts map[string]int, patches map[string][]porcelain.DiffHunk,
) (gitreview.AnchoredComment, error) {
	if c.Origin == gitreview.OriginWorking {
		// Its range is in worktree coordinates: a commit diff would mis-project it.
		return gitreview.AnchoredComment{Comment: c, Anchor: gitreview.AnchorUncommitted, Range: c.Range}, nil
	}
	currentOID, err := e.blobOID(ctx, at, c.Path)
	if err != nil {
		return gitreview.AnchoredComment{}, err
	}
	if currentOID == c.AnchorBlobOID {
		return gitreview.AnchoredComment{Comment: c, Anchor: gitreview.AnchorExact, Range: c.Range}, nil
	}

	ancestorRes, err := e.runAllowingExit(ctx, porcelain.IsAncestorArgs(c.AnchorSHA, at), 0, 1, 128)
	if err != nil {
		return gitreview.AnchoredComment{}, err
	}
	if ancestorRes.ExitCode != 0 {
		return gitreview.AnchoredComment{Comment: c, Anchor: gitreview.AnchorStale, Range: c.Range}, nil
	}

	patchKey := c.Path + "\x00" + c.AnchorSHA
	hunks, ok := patches[patchKey]
	if !ok {
		raw, rerr := e.runOne(ctx, porcelain.FileDiffArgs(&c.AnchorSHA, at, c.Path, nil))
		if rerr != nil {
			return gitreview.AnchoredComment{}, rerr
		}
		parsed, _, perr := e.parseAndResolve(ctx, raw)
		if perr != nil {
			return gitreview.AnchoredComment{}, perr
		}
		hunks = parsed.Hunks
		patches[patchKey] = hunks
	}

	lineCount, err := e.lineCountAt(ctx, at, c.Path, lineCounts)
	if err != nil {
		return gitreview.AnchoredComment{}, err
	}
	projected := gitreview.ProjectRanges([]gitreview.LineRange{c.Range}, hunks, lineCount)
	if len(projected) == 0 {
		return gitreview.AnchoredComment{Comment: c, Anchor: gitreview.AnchorRemoved, Range: c.Range}, nil
	}
	return gitreview.AnchoredComment{Comment: c, Anchor: gitreview.AnchorProjected, Range: unionRange(projected)}, nil
}

// unionRange envelopes a (possibly split, D7's own "∪ if it split") projection back into the one
// LineRange a flat comment carries — min start to max end across every surviving piece.
func unionRange(rs []gitreview.LineRange) gitreview.LineRange {
	out := rs[0]
	for _, r := range rs[1:] {
		if r.Start < out.Start {
			out.Start = r.Start
		}
		if r.End > out.End {
			out.End = r.End
		}
	}
	return out
}
