package gitsession

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"maps"
	"slices"
	"strings"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitclient/catfile"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitclient/porcelain"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitreview"
)

// removed.go: reviewed lines that left the current content. Capture happens before a re-mark
// overwrites a snapshot (markFileAt, markWorkingLocked) and at list time for records whose
// current side changed since the last check.

// maxRemovedTextBytes bounds one entry's text on the wire; the full text stays stored.
const maxRemovedTextBytes = 64 << 10

// absentKey is the removal_checked token for a current side that does not exist, distinct from the
// "" of a record never checked.
const absentKey = "absent"

// ReviewRemovedEntry is one removed block on the wire. Range is nil and Text nil for a non-text
// whole-file entry.
type ReviewRemovedEntry struct {
	ID            int64                `json:"id"`
	Origin        string               `json:"origin"`
	Path          string               `json:"path"`
	ReviewedAt    int64                `json:"reviewedAt"`
	ReviewedAtSHA string               `json:"reviewedAtSha"`
	ContentKind   string               `json:"contentKind"`
	Range         *gitreview.LineRange `json:"range"`
	Text          *string              `json:"text"`
	Truncated     bool                 `json:"truncated"`
	RemovedAt     int64                `json:"removedAt"`
}

// ReviewRemovedResult is review.removed.list's payload; Entries is never nil.
type ReviewRemovedResult struct {
	Entries []ReviewRemovedEntry `json:"entries"`
}

func commitSource(rec gitreview.FileRecord) gitreview.RemovedSource {
	return gitreview.RemovedSource{
		Origin: gitreview.OriginCommit, Path: rec.Path, ReviewedAt: rec.ReviewedAt,
		ReviewedAtSHA: rec.ReviewedAtSHA, SnapshotKey: rec.BlobOID, ContentKind: rec.ContentKind,
	}
}

func workingSource(rec gitreview.WorkingRecord) gitreview.RemovedSource {
	return gitreview.RemovedSource{
		Origin: gitreview.OriginWorking, Path: rec.Path, ReviewedAt: rec.ReviewedAt,
		ReviewedAtSHA: rec.HeadSHA, SnapshotKey: rec.ContentHash, ContentKind: rec.ContentKind,
	}
}

// captureCommitted stores the reviewed lines of rec that hunks (snapshot to current) delete, or a
// whole-file row for a fully reviewed non-text record whose file is gone. unparsed marks a delta
// without usable hunks (patch over the cap, binary current side): every reviewed line counts as
// removed, as when the file is gone.
func (e *RepoEntry) captureCommitted(ctx context.Context, branch string, rec gitreview.FileRecord, snapshot []byte, hunks []porcelain.DiffHunk, currentAbsent, unparsed bool) error {
	repo := e.Summary.RepoID
	switch rec.ContentKind {
	case gitreview.ContentText:
		ranges := recordRanges(rec)
		if !currentAbsent && !unparsed {
			ranges = gitreview.DeletedRanges(ranges, hunks)
		}
		if len(ranges) == 0 {
			return nil
		}
		_, err := e.review.CaptureRemoved(ctx, repo, branch, commitSource(rec), ranges, snapshot)
		return err
	case gitreview.ContentBinary, gitreview.ContentTooLarge:
		if rec.State == "full" && currentAbsent {
			_, err := e.review.CaptureRemovedFile(ctx, repo, branch, commitSource(rec))
			return err
		}
	}
	return nil
}

// captureWorking is captureCommitted for an uncommitted record against cur. hunks is the
// snapshot-to-cur delta, nil when unchanged or not computed; unparsed marks a text delta without
// usable hunks (patch over the cap).
func (e *RepoEntry) captureWorking(ctx context.Context, branch string, rec gitreview.WorkingRecord, snapshot []byte, cur WorkingContent, hunks []porcelain.DiffHunk, unparsed bool) error {
	repo := e.Summary.RepoID
	switch rec.ContentKind {
	case gitreview.ContentText:
		reviewed := workingRecordRanges(rec)
		ranges := reviewed
		if cur.Kind == gitreview.ContentText && !unparsed {
			ranges = gitreview.DeletedRanges(reviewed, hunks)
		}
		if len(ranges) == 0 {
			return nil
		}
		_, err := e.review.CaptureRemoved(ctx, repo, branch, workingSource(rec), ranges, snapshot)
		return err
	case gitreview.ContentBinary, gitreview.ContentTooLarge:
		if rec.State == "full" && cur.Kind == gitreview.ContentAbsent {
			_, err := e.review.CaptureRemovedFile(ctx, repo, branch, workingSource(rec))
			return err
		}
	}
	return nil
}

// DismissRemoved hides removed entries of branch for good.
func (e *RepoEntry) DismissRemoved(ctx context.Context, branch string, ids []int64) (int, error) {
	return e.review.DismissRemoved(ctx, e.Summary.RepoID, branch, ids)
}

func hasCommittedReview(rec gitreview.FileRecord) bool {
	switch rec.ContentKind {
	case gitreview.ContentText:
		return len(recordRanges(rec)) > 0
	case gitreview.ContentBinary, gitreview.ContentTooLarge:
		return rec.State == "full"
	}
	return false
}

func hasWorkingReview(rec gitreview.WorkingRecord) bool {
	switch rec.ContentKind {
	case gitreview.ContentText:
		return len(workingRecordRanges(rec)) > 0
	case gitreview.ContentBinary, gitreview.ContentTooLarge:
		return rec.State == "full"
	}
	return false
}

// unparsedBody reports a diff body that carries no hunks to read deletions from.
func unparsedBody(b porcelain.FileDiffBody) bool {
	return b.Kind == porcelain.BodyTooLarge || b.Kind == porcelain.BodyBinary || b.Kind == porcelain.BodyLFSPointer
}

func oidKey(oid string) string {
	if oid == "" {
		return absentKey
	}
	return oid
}

// RemovedSinceReview captures reviewed lines gone from the branch tip, then lists every stored
// undismissed block of the branch (both origins). A failed capture is logged and the stored blocks
// still list; a canceled context fails the call.
func (e *RepoEntry) RemovedSinceReview(ctx context.Context, base, branch string) (ReviewRemovedResult, error) {
	if err := e.captureCommittedRemoved(ctx, base, branch); err != nil {
		if ctx.Err() != nil {
			return ReviewRemovedResult{}, err
		}
		slog.Warn("gitsession: capture removed lines", "scope", "gitsession", "repo", e.Summary.RepoID, "branch", branch, "err", err)
	}
	stored, err := e.review.Removed(ctx, e.Summary.RepoID, branch)
	if err != nil {
		return ReviewRemovedResult{}, err
	}
	return ReviewRemovedResult{Entries: toRemovedEntries(stored)}, nil
}

func (e *RepoEntry) captureCommittedRemoved(ctx context.Context, base, branch string) error {
	res, _, err := e.rangeFilesRecords(ctx, base, branch)
	if err != nil {
		return err
	}
	records, err := e.review.Records(ctx, e.Summary.RepoID, branch)
	if err != nil {
		return err
	}

	renamed := map[string]string{} // old path -> new path
	for _, f := range res.Files {
		if src := renameSource(records, f.Change); src != "" {
			renamed[src] = f.Change.Path
		}
	}

	var recs []gitreview.FileRecord
	var current []string
	for _, path := range slices.Sorted(maps.Keys(records)) { // id order must not depend on map order
		rec := records[path]
		if !hasCommittedReview(rec) {
			continue
		}
		recs = append(recs, rec)
		if np, ok := renamed[rec.Path]; ok {
			current = append(current, np)
		} else {
			current = append(current, rec.Path)
		}
	}
	oids, err := e.blobOIDs(ctx, res.BranchTip, current)
	if err != nil {
		return err
	}
	for i, rec := range recs {
		oid := oids[i]
		if oid == rec.BlobOID || oidKey(oid) == rec.RemovalChecked {
			continue
		}
		if err := e.captureCommittedRecord(ctx, branch, res.BranchTip, rec, current[i], oid); err != nil {
			return err
		}
	}
	return nil
}

func (e *RepoEntry) captureCommittedRecord(ctx context.Context, branch, tip string, listed gitreview.FileRecord, currentPath, oid string) error {
	repo := e.Summary.RepoID
	unlock := e.review.Lock(repo, branch, listed.Path)
	defer unlock()
	rec, snapshot, found, err := e.review.Record(ctx, repo, branch, listed.Path)
	if err != nil {
		return err
	}
	if !found || !rec.ReviewedAt.Equal(listed.ReviewedAt) {
		return nil
	}
	if currentPath == rec.Path {
		delta, err := e.FileDelta(ctx, branch, rec.Path, rec, snapshot, tip)
		if err != nil {
			return err
		}
		if err := e.captureCommitted(ctx, branch, rec, snapshot, delta.Hunks, delta.CurrentOID == "", unparsedBody(delta.Body)); err != nil {
			return err
		}
	} else if rec.ContentKind == gitreview.ContentText {
		_, content, err := e.readCurrentContent(ctx, tip, currentPath)
		if err != nil && !errors.Is(err, catfile.ErrMissing) && !errors.Is(err, catfile.ErrTooLarge) {
			return err
		}
		var hunks []porcelain.DiffHunk
		unparsed := err != nil || looksBinary(content) || len(content) > gitreview.MaxSnapshotBytes
		if !unparsed {
			var body porcelain.FileDiffBody
			if hunks, body, _, err = e.contentDelta(ctx, snapshot, content); err != nil {
				return err
			}
			unparsed = unparsedBody(body)
		}
		if err := e.captureCommitted(ctx, branch, rec, snapshot, hunks, false, unparsed); err != nil {
			return err
		}
	}
	return e.review.SetRemovalChecked(ctx, repo, branch, gitreview.OriginCommit, rec.Path, rec.ReviewedAt, oidKey(oid))
}

// CaptureWorkingRemoved captures reviewed uncommitted lines that left the worktree, for every
// working record, not only dirty paths. read supplies the current worktree content.
func (e *RepoEntry) CaptureWorkingRemoved(ctx context.Context, branch string, read func(path string) (WorkingContent, error)) error {
	repo := e.Summary.RepoID
	records, err := e.review.WorkingRecords(ctx, repo, branch)
	if err != nil {
		return err
	}
	for _, path := range slices.Sorted(maps.Keys(records)) { // id order must not depend on map order
		listed := records[path]
		if !hasWorkingReview(listed) {
			continue
		}
		cur, err := read(listed.Path)
		if err != nil {
			return err
		}
		if cur.Hash == listed.ContentHash {
			continue
		}
		key := cur.Hash
		if cur.Kind == gitreview.ContentAbsent {
			key = absentKey
		}
		if key == listed.RemovalChecked {
			continue
		}
		if err := e.captureWorkingRecord(ctx, branch, listed, cur, key); err != nil {
			return err
		}
	}
	return nil
}

func (e *RepoEntry) captureWorkingRecord(ctx context.Context, branch string, listed gitreview.WorkingRecord, cur WorkingContent, key string) error {
	repo := e.Summary.RepoID
	unlock := e.review.Lock(repo, branch, listed.Path)
	defer unlock()
	rec, snapshot, found, err := e.review.WorkingRecord(ctx, repo, branch, listed.Path)
	if err != nil {
		return err
	}
	if !found || !rec.ReviewedAt.Equal(listed.ReviewedAt) {
		return nil
	}
	var hunks []porcelain.DiffHunk
	var unparsed bool
	if rec.ContentKind == gitreview.ContentText && cur.Kind == gitreview.ContentText {
		var body porcelain.FileDiffBody
		if hunks, body, _, err = e.contentDelta(ctx, snapshot, cur.Text); err != nil {
			return err
		}
		unparsed = unparsedBody(body)
	}
	if err := e.captureWorking(ctx, branch, rec, snapshot, cur, hunks, unparsed); err != nil {
		return err
	}
	return e.review.SetRemovalChecked(ctx, repo, branch, gitreview.OriginWorking, rec.Path, rec.ReviewedAt, key)
}

func toRemovedEntries(stored []gitreview.Removed) []ReviewRemovedEntry {
	out := make([]ReviewRemovedEntry, 0, len(stored))
	for _, r := range stored {
		entry := ReviewRemovedEntry{
			ID: r.ID, Origin: r.Origin, Path: r.Path, ReviewedAt: r.ReviewedAt.UnixMilli(),
			ReviewedAtSHA: r.ReviewedAtSHA, ContentKind: string(r.ContentKind), Range: r.Range,
			RemovedAt: r.RemovedAt.UnixMilli(),
		}
		if r.Text != nil {
			text, truncated := capRemovedText(r.Text)
			entry.Text, entry.Truncated = &text, truncated
		}
		out = append(out, entry)
	}
	return out
}

// capRemovedText cuts text at a line boundary within maxRemovedTextBytes.
func capRemovedText(text []byte) (string, bool) {
	if len(text) <= maxRemovedTextBytes {
		return string(text), false
	}
	head := text[:maxRemovedTextBytes]
	if i := bytes.LastIndexByte(head, '\n'); i >= 0 {
		head = head[:i+1]
	}
	return strings.ToValidUTF8(string(head), ""), true
}
