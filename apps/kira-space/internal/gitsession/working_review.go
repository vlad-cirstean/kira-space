package gitsession

import (
	"bytes"
	"context"
	"errors"
	"maps"
	"slices"
	"strconv"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitclient/catfile"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitclient/porcelain"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitreview"
)

// working_review.go: review of uncommitted worktree content (P418). Marks and comments live in
// review.db beside the committed ones, in the coordinates of a content snapshot, and project
// through `diff --no-index` as the worktree file changes.

// WorkingContent is one side of an uncommitted file as the caller read it: Text is set only for
// ContentText; Bytes is the on-disk size. Hash identifies the content (sha256 of text; a size
// identity for binary and tooLarge, whose bytes are never read; "" for absent).
type WorkingContent struct {
	Kind  gitreview.ContentKind
	Text  []byte
	Bytes int
	Hash  string
}

// NewWorkingContent builds a WorkingContent, computing its Hash.
func NewWorkingContent(kind gitreview.ContentKind, text []byte, size int) WorkingContent {
	c := WorkingContent{Kind: kind, Bytes: size}
	switch kind {
	case gitreview.ContentText:
		c.Text = text
		c.Bytes = len(text)
		c.Hash = gitreview.ContentHash(text)
	case gitreview.ContentAbsent:
		c.Bytes = 0
	default:
		c.Hash = gitreview.ContentHash([]byte(string(kind) + ":" + strconv.Itoa(size)))
	}
	return c
}

func (c WorkingContent) lineCount() int {
	if c.Kind != gitreview.ContentText {
		return 0
	}
	return countLines(c.Text)
}

// WorkingReviewResult is the uncommitted counterpart of review.fileDiff's result.
type WorkingReviewResult struct {
	Path           string                 `json:"path"`
	Body           porcelain.FileDiffBody `json:"body"`
	ReviewedRanges []gitreview.LineRange  `json:"reviewedRanges"`
	LineCount      int                    `json:"lineCount"`
	ContentKind    string                 `json:"contentKind"`
}

// workingStatus folds a working record into the wire status, as reviewFileStatus does for a
// committed one. changed is true when the file differs from the content the record was taken on.
func workingStatus(rec gitreview.WorkingRecord, found, changed bool) ReviewFileStatus {
	if !found {
		return ReviewFileStatus{Kind: "none"}
	}
	kind := "partial"
	switch {
	case rec.State == "full":
		kind = "full"
	case len(rec.Ranges) == 0:
		kind = "none"
	}
	if kind == "none" {
		changed = false
	}
	millis := rec.ReviewedAt.UnixMilli()
	sha := rec.HeadSHA
	return ReviewFileStatus{Kind: kind, ChangedSinceReview: changed, ReviewedAt: &millis, ReviewedAtSHA: &sha}
}

func workingRecordRanges(rec gitreview.WorkingRecord) []gitreview.LineRange {
	if rec.State == "full" {
		return gitreview.Expand(rec.LineCount)
	}
	return rec.Ranges
}

// projectWorkingRanges maps a record's ranges (snapshot coordinates) onto cur. Non-text records and
// non-text cur carry no line ranges.
// It also returns the snapshot-to-cur hunks, nil when none were computed, and whether the delta
// carried no parsed hunks.
func (e *RepoEntry) projectWorkingRanges(ctx context.Context, rec gitreview.WorkingRecord, snapshot []byte, cur WorkingContent) ([]gitreview.LineRange, []porcelain.DiffHunk, bool, error) {
	if rec.ContentKind != gitreview.ContentText || cur.Kind != gitreview.ContentText {
		return nil, nil, false, nil
	}
	ranges := workingRecordRanges(rec)
	if len(ranges) == 0 {
		return nil, nil, false, nil
	}
	if rec.ContentHash == cur.Hash {
		return gitreview.ProjectRanges(ranges, nil, cur.lineCount()), nil, false, nil
	}
	hunks, body, _, err := e.contentDelta(ctx, snapshot, cur.Text)
	if err != nil {
		return nil, nil, false, err
	}
	return gitreview.ProjectRanges(ranges, hunks, cur.lineCount()), hunks, unparsedBody(body), nil
}

func (e *RepoEntry) workingBody(ctx context.Context, head, cur WorkingContent) (porcelain.FileDiffBody, error) {
	textual := func(c WorkingContent) bool {
		return c.Kind == gitreview.ContentText || c.Kind == gitreview.ContentAbsent
	}
	switch {
	case textual(head) && textual(cur):
		_, body, _, err := e.contentDelta(ctx, head.Text, cur.Text)
		return body, err
	case head.Kind == gitreview.ContentTooLarge || cur.Kind == gitreview.ContentTooLarge:
		return porcelain.FileDiffBody{
			Kind: porcelain.BodyTooLarge, Bytes: int64(max(head.Bytes, cur.Bytes)), LimitBytes: MaxPatchBytes,
		}, nil
	}
	oldBytes, newBytes := int64(head.Bytes), int64(cur.Bytes)
	return porcelain.FileDiffBody{Kind: porcelain.BodyBinary, OldBytes: &oldBytes, NewBytes: &newBytes}, nil
}

// WorkingFileReview is the diff of one uncommitted file (head side against the worktree) with the
// ranges the user already marked, projected onto the current content.
func (e *RepoEntry) WorkingFileReview(ctx context.Context, branch, path string, head, cur WorkingContent) (WorkingReviewResult, error) {
	body, err := e.workingBody(ctx, head, cur)
	if err != nil {
		return WorkingReviewResult{}, err
	}
	rec, snapshot, found, err := e.review.WorkingRecord(ctx, e.Summary.RepoID, branch, path)
	if err != nil {
		return WorkingReviewResult{}, err
	}
	reviewed := []gitreview.LineRange{}
	if found {
		projected, _, _, perr := e.projectWorkingRanges(ctx, rec, snapshot, cur)
		if perr != nil {
			return WorkingReviewResult{}, perr
		}
		reviewed = nonNil(projected)
	}
	if err := e.review.Touch(ctx, e.Summary.RepoID, branch); err != nil {
		return WorkingReviewResult{}, err
	}
	return WorkingReviewResult{
		Path: path, Body: body, ReviewedRanges: reviewed, LineCount: cur.lineCount(), ContentKind: string(cur.Kind),
	}, nil
}

// MarkWorkingFile marks or unmarks an uncommitted file under the path's review lock. nil ranges
// means the whole file; the union, subtract and full-state rules match markFileAt. Ranges are in
// cur's coordinates.
func (e *RepoEntry) MarkWorkingFile(ctx context.Context, branch, path string, cur WorkingContent, headSHA string, reviewed bool, ranges []gitreview.LineRange) (ReviewFileStatus, error) {
	unlock := e.review.Lock(e.Summary.RepoID, branch, path)
	defer unlock()
	return e.markWorkingLocked(ctx, branch, path, cur, headSHA, reviewed, ranges)
}

func (e *RepoEntry) markWorkingLocked(ctx context.Context, branch, path string, cur WorkingContent, headSHA string, reviewed bool, ranges []gitreview.LineRange) (ReviewFileStatus, error) {
	if ranges != nil && cur.Kind != gitreview.ContentText {
		return ReviewFileStatus{}, ErrRangedMarkOnNonText
	}
	rec, snapshot, found, err := e.review.WorkingRecord(ctx, e.Summary.RepoID, branch, path)
	if err != nil {
		return ReviewFileStatus{}, err
	}
	lineCount := cur.lineCount()
	var existing []gitreview.LineRange
	if found {
		var hunks []porcelain.DiffHunk
		var unparsed bool
		if existing, hunks, unparsed, err = e.projectWorkingRanges(ctx, rec, snapshot, cur); err != nil {
			return ReviewFileStatus{}, err
		}
		// Capture before PutWorking overwrites the snapshot.
		if err := e.captureWorking(ctx, branch, rec, snapshot, cur, hunks, unparsed); err != nil {
			return ReviewFileStatus{}, err
		}
	}

	var state string
	var stored []gitreview.LineRange
	if ranges == nil {
		state = "partial"
		if reviewed {
			state = "full"
		}
	} else {
		given := gitreview.ProjectRanges(ranges, nil, lineCount)
		var next []gitreview.LineRange
		if reviewed {
			next = gitreview.Union(existing, given)
		} else {
			next = gitreview.Subtract(existing, given)
		}
		state, stored = normalizeState(next, lineCount)
	}

	out := gitreview.WorkingRecord{
		Path: path, State: state, HeadSHA: headSHA, ReviewedAt: time.Now(), ContentHash: cur.Hash,
		ContentKind: cur.Kind, ContentBytes: cur.Bytes, LineCount: lineCount, Ranges: stored,
	}
	if err := e.review.PutWorking(ctx, e.Summary.RepoID, branch, out, cur.Text); err != nil {
		return ReviewFileStatus{}, err
	}
	return workingStatus(out, true, false), nil
}

// WorkingReviewStates reports the working review status of each given path. read supplies the
// current content, and is called only for paths with a record; the rest are "none".
func (e *RepoEntry) WorkingReviewStates(ctx context.Context, branch string, paths []string, read func(path string) (WorkingContent, error)) (map[string]ReviewFileStatus, error) {
	records, err := e.review.WorkingRecords(ctx, e.Summary.RepoID, branch)
	if err != nil {
		return nil, err
	}
	out := make(map[string]ReviewFileStatus, len(paths))
	for _, path := range paths {
		rec, found := records[path]
		if !found {
			out[path] = workingStatus(rec, false, false)
			continue
		}
		cur, err := read(path)
		if err != nil {
			return nil, err
		}
		out[path] = workingStatus(rec, true, rec.ContentHash != cur.Hash)
	}
	return out, nil
}

// AddWorkingComment stores a comment on uncommitted text, anchored to cur itself.
func (e *RepoEntry) AddWorkingComment(ctx context.Context, branch, path string, cur WorkingContent, headSHA string, r gitreview.LineRange, body, kind string) (CommentEntry, error) {
	if cur.Kind != gitreview.ContentText {
		return CommentEntry{}, ErrCommentNotText
	}
	if r.End > cur.lineCount() {
		return CommentEntry{}, ErrCommentRangeOutOfFile
	}
	stored, err := e.review.AddComment(ctx, e.Summary.RepoID, branch, gitreview.Comment{
		Path: path, Range: r, Body: body, AnchorSHA: headSHA, AnchorBlobOID: cur.Hash, CreatedAt: time.Now(),
		Kind: kind, Origin: gitreview.OriginWorking, AnchorContent: cur.Text,
	})
	if err != nil {
		return CommentEntry{}, err
	}
	return toCommentEntry(gitreview.AnchoredComment{Comment: stored, Anchor: gitreview.AnchorExact, Range: stored.Range}), nil
}

// ListWorkingComments anchors every working-origin comment against the current worktree content
// the contents callback returns for its path.
func (e *RepoEntry) ListWorkingComments(ctx context.Context, branch string, contents func(path string) (WorkingContent, error)) ([]CommentEntry, error) {
	stored, err := e.review.Comments(ctx, e.Summary.RepoID, branch)
	if err != nil {
		return nil, err
	}
	curByPath := map[string]WorkingContent{}
	hunksByKey := map[string][]porcelain.DiffHunk{}
	var anchored []gitreview.AnchoredComment
	for _, c := range stored {
		if c.Origin != gitreview.OriginWorking {
			continue
		}
		cur, ok := curByPath[c.Path]
		if !ok {
			if cur, err = contents(c.Path); err != nil {
				return nil, err
			}
			curByPath[c.Path] = cur
		}
		ac, err := e.anchorWorking(ctx, c, cur, hunksByKey)
		if err != nil {
			return nil, err
		}
		anchored = append(anchored, ac)
	}
	gitreview.SortAnchored(anchored)
	if err := e.review.Touch(ctx, e.Summary.RepoID, branch); err != nil {
		return nil, err
	}
	return toCommentEntries(anchored), nil
}

func (e *RepoEntry) anchorWorking(ctx context.Context, c gitreview.Comment, cur WorkingContent, memo map[string][]porcelain.DiffHunk) (gitreview.AnchoredComment, error) {
	if cur.Kind == gitreview.ContentText && c.AnchorBlobOID == cur.Hash {
		return gitreview.AnchoredComment{Comment: c, Anchor: gitreview.AnchorExact, Range: c.Range}, nil
	}
	removed := gitreview.AnchoredComment{Comment: c, Anchor: gitreview.AnchorRemoved, Range: c.Range}
	if cur.Kind != gitreview.ContentText || len(c.AnchorContent) == 0 {
		return removed, nil
	}
	key := c.Path + "\x00" + c.AnchorBlobOID
	hunks, ok := memo[key]
	if !ok {
		var err error
		if hunks, _, _, err = e.contentDelta(ctx, c.AnchorContent, cur.Text); err != nil {
			return gitreview.AnchoredComment{}, err
		}
		memo[key] = hunks
	}
	projected := gitreview.ProjectRanges([]gitreview.LineRange{c.Range}, hunks, cur.lineCount())
	if len(projected) == 0 {
		return removed, nil
	}
	return gitreview.AnchoredComment{Comment: c, Anchor: gitreview.AnchorProjected, Range: unionRange(projected)}, nil
}

// tipWorkingContent reads rev:path as WorkingContent, classified like a worktree read. A blob over
// the cat-file gate reports a zero hash that matches nothing.
func (e *RepoEntry) tipWorkingContent(ctx context.Context, rev, path string) (WorkingContent, string, error) {
	oid, content, err := e.readCurrentContent(ctx, rev, path)
	switch {
	case errors.Is(err, catfile.ErrMissing):
		return NewWorkingContent(gitreview.ContentAbsent, nil, 0), "", nil
	case errors.Is(err, catfile.ErrTooLarge):
		return WorkingContent{Kind: gitreview.ContentTooLarge}, oid, nil
	case err != nil:
		return WorkingContent{}, "", err
	case len(content) > gitreview.MaxSnapshotBytes:
		return NewWorkingContent(gitreview.ContentTooLarge, nil, len(content)), oid, nil
	case looksBinary(content):
		return NewWorkingContent(gitreview.ContentBinary, nil, len(content)), oid, nil
	}
	return NewWorkingContent(gitreview.ContentText, content, len(content)), oid, nil
}

// addedLines is the set of new-side lines the hunks add.
func addedLines(hunks []porcelain.DiffHunk) []gitreview.LineRange {
	var out []gitreview.LineRange
	for _, h := range hunks {
		for _, l := range h.Lines {
			if l.Kind == porcelain.LineAdd && l.NewLine != nil {
				out = append(out, gitreview.LineRange{Start: *l.NewLine, End: *l.NewLine})
			}
		}
	}
	return gitreview.Normalize(out)
}

// PromoteWorkingReview carries uncommitted review state onto commits made since it was taken, and
// returns the paths it changed. A path is touched only when the branch tip's blob differs from the
// blob at the record's head; ranges move only for lines the work added. Idempotent. tip is the
// worktree's own HEAD, read fresh by the caller: the cached ref snapshot can lag a commit.
func (e *RepoEntry) PromoteWorkingReview(ctx context.Context, branch, tip string) ([]string, error) {
	if tip == "" {
		return nil, nil
	}
	records, err := e.review.WorkingRecords(ctx, e.Summary.RepoID, branch)
	if err != nil {
		return nil, err
	}
	stored, err := e.review.Comments(ctx, e.Summary.RepoID, branch)
	if err != nil {
		return nil, err
	}
	paths := map[string]struct{}{}
	for p := range records {
		paths[p] = struct{}{}
	}
	// Loaded once: each load inflates every snapshot blob.
	commentsByPath := map[string][]gitreview.Comment{}
	for _, c := range stored {
		if c.Origin == gitreview.OriginWorking {
			paths[c.Path] = struct{}{}
			commentsByPath[c.Path] = append(commentsByPath[c.Path], c)
		}
	}
	var promoted []string
	for _, path := range slices.Sorted(maps.Keys(paths)) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		changed, err := e.promoteWorkingPath(ctx, tip, branch, path, commentsByPath[path])
		if err != nil {
			return nil, err
		}
		if changed {
			promoted = append(promoted, path)
		}
	}
	return promoted, nil
}

func (e *RepoEntry) promoteWorkingPath(ctx context.Context, tip, branch, path string, comments []gitreview.Comment) (bool, error) {
	unlock := e.review.Lock(e.Summary.RepoID, branch, path)
	defer unlock()

	tipContent, tipOID, err := e.tipWorkingContent(ctx, tip, path)
	if err != nil {
		return false, err
	}
	changed, err := e.promoteWorkingMarks(ctx, tip, branch, path, tipContent)
	if err != nil {
		return false, err
	}
	commentsChanged, err := e.promoteWorkingComments(ctx, tip, branch, comments, tipContent, tipOID)
	if err != nil {
		return false, err
	}
	return changed || commentsChanged, nil
}

// committedSince is true when path differs between anchorSHA and tip.
func (e *RepoEntry) committedSince(ctx context.Context, anchorSHA, tip, tipOID, path string) (bool, error) {
	if anchorSHA == "" {
		return true, nil
	}
	if anchorSHA == tip {
		return false, nil
	}
	oid, err := e.blobOID(ctx, anchorSHA, path)
	if err != nil {
		return false, err
	}
	return oid != tipOID, nil
}

func (e *RepoEntry) promoteWorkingMarks(ctx context.Context, tip, branch, path string, tipContent WorkingContent) (bool, error) {
	rec, snapshot, found, err := e.review.WorkingRecord(ctx, e.Summary.RepoID, branch, path)
	if err != nil || !found {
		return false, err
	}
	tipOID := ""
	if tipContent.Kind != gitreview.ContentAbsent {
		if tipOID, err = e.blobOID(ctx, tip, path); err != nil {
			return false, err
		}
	}
	moved, err := e.committedSince(ctx, rec.HeadSHA, tip, tipOID, path)
	if err != nil || !moved {
		return false, err
	}

	if rec.ContentKind != gitreview.ContentText {
		return e.promoteNonTextMark(ctx, tip, branch, path, rec, snapshot, tipContent)
	}

	landed, exact, err := e.landedWorkingRanges(ctx, rec, snapshot, tipContent, path)
	if err != nil {
		return false, err
	}
	if len(landed) > 0 {
		if _, err := e.markFileAt(ctx, tip, branch, path, true, landed); err != nil {
			return false, err
		}
	}
	if exact {
		return len(landed) > 0, e.review.DeleteWorking(ctx, e.Summary.RepoID, branch, path)
	}
	return len(landed) > 0, e.keepUnlandedWorking(ctx, tip, branch, rec, snapshot, tipContent, landed)
}

// promoteNonTextMark moves a whole-file mark of a binary, oversized or absent file once the
// commit holds the same content.
func (e *RepoEntry) promoteNonTextMark(ctx context.Context, tip, branch, path string, rec gitreview.WorkingRecord, snapshot []byte, tipContent WorkingContent) (bool, error) {
	if tipContent.Hash != rec.ContentHash || tipContent.Kind != rec.ContentKind {
		return false, e.rebaseWorkingRecord(ctx, branch, rec, snapshot, tip)
	}
	if rec.State == "full" {
		if _, err := e.markFileAt(ctx, tip, branch, path, true, nil); err != nil {
			return false, err
		}
	}
	return true, e.review.DeleteWorking(ctx, e.Summary.RepoID, branch, path)
}

// landedWorkingRanges returns the marked lines the commit holds, in tip coordinates. Only lines
// the work changed relative to the head the record was taken on count; context stays out of the
// committed review. exact reports the commit holds the reviewed snapshot byte for byte.
func (e *RepoEntry) landedWorkingRanges(ctx context.Context, rec gitreview.WorkingRecord, snapshot []byte, tipContent WorkingContent, path string) (landed []gitreview.LineRange, exact bool, err error) {
	var headText []byte
	if rec.HeadSHA != "" {
		headContent, _, err := e.tipWorkingContent(ctx, rec.HeadSHA, path)
		if err != nil {
			return nil, false, err
		}
		headText = headContent.Text
	}
	addHunks, _, _, err := e.contentDelta(ctx, headText, snapshot)
	if err != nil {
		return nil, false, err
	}
	marked := gitreview.Intersect(workingRecordRanges(rec), addedLines(addHunks))

	exact = tipContent.Kind == gitreview.ContentText && bytes.Equal(tipContent.Text, snapshot)
	switch {
	case len(marked) == 0 || tipContent.Kind != gitreview.ContentText:
		return nil, exact, nil
	case exact:
		return marked, true, nil
	}
	hunks, _, _, err := e.contentDelta(ctx, snapshot, tipContent.Text)
	if err != nil {
		return nil, false, err
	}
	return gitreview.ProjectRanges(marked, hunks, tipContent.lineCount()), false, nil
}

// keepUnlandedWorking rewrites the record to the marks the commit did not take, on the new head,
// or deletes it when none remain.
func (e *RepoEntry) keepUnlandedWorking(ctx context.Context, tip, branch string, rec gitreview.WorkingRecord, snapshot []byte, tipContent WorkingContent, landed []gitreview.LineRange) error {
	remaining := workingRecordRanges(rec)
	if len(landed) > 0 {
		hunks, _, _, err := e.contentDelta(ctx, tipContent.Text, snapshot)
		if err != nil {
			return err
		}
		back := gitreview.ProjectRanges(landed, hunks, rec.LineCount)
		remaining = gitreview.Subtract(remaining, back)
	}
	if len(remaining) == 0 {
		return e.review.DeleteWorking(ctx, e.Summary.RepoID, branch, rec.Path)
	}
	state, ranges := normalizeState(remaining, rec.LineCount)
	rec.State, rec.Ranges, rec.HeadSHA = state, ranges, tip
	return e.review.PutWorking(ctx, e.Summary.RepoID, branch, rec, snapshot)
}

// rebaseWorkingRecord moves a record whose content did not reach the tip onto the new head.
func (e *RepoEntry) rebaseWorkingRecord(ctx context.Context, branch string, rec gitreview.WorkingRecord, snapshot []byte, tip string) error {
	rec.HeadSHA = tip
	return e.review.PutWorking(ctx, e.Summary.RepoID, branch, rec, snapshot)
}

func (e *RepoEntry) promoteWorkingComments(ctx context.Context, tip, branch string, comments []gitreview.Comment, tipContent WorkingContent, tipOID string) (bool, error) {
	if tipContent.Kind != gitreview.ContentText {
		return false, nil
	}
	hunksByKey := map[string][]porcelain.DiffHunk{}
	promoted := false
	for _, c := range comments {
		if len(c.AnchorContent) == 0 {
			continue
		}
		moved, err := e.committedSince(ctx, c.AnchorSHA, tip, tipOID, c.Path)
		if err != nil {
			return false, err
		}
		if !moved {
			continue
		}
		rng := c.Range
		if !bytes.Equal(c.AnchorContent, tipContent.Text) {
			key := c.AnchorBlobOID
			hunks, ok := hunksByKey[key]
			if !ok {
				if hunks, _, _, err = e.contentDelta(ctx, c.AnchorContent, tipContent.Text); err != nil {
					return false, err
				}
				hunksByKey[key] = hunks
			}
			projected := gitreview.ProjectRanges([]gitreview.LineRange{c.Range}, hunks, tipContent.lineCount())
			if len(projected) == 0 {
				continue
			}
			rng = unionRange(projected)
		}
		ok, err := e.review.PromoteComment(ctx, e.Summary.RepoID, branch, c.ID, tip, tipOID, rng)
		if err != nil {
			return false, err
		}
		promoted = promoted || ok
	}
	return promoted, nil
}
