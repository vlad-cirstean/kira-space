package gitsession

import (
	"context"
	"reflect"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitreview"
)

const workingBase = "a\nb\nc\nd\ne\n"

func textContent(s string) WorkingContent {
	return NewWorkingContent(gitreview.ContentText, []byte(s), len(s))
}

func headOf(t *testing.T, dir string) string {
	t.Helper()
	return trimTrailingNL(runInc(t, dir, "rev-parse", "HEAD"))
}

func lr(s, e int) gitreview.LineRange { return gitreview.LineRange{Start: s, End: e} }

// workingFixture is a repo with a.txt committed as workingBase.
func workingFixture(t *testing.T) (dir, base string, entry *RepoEntry) {
	t.Helper()
	dir, _ = buildFileDeltaFixture(t)
	writeIncFile(t, dir, "a.txt", workingBase)
	runInc(t, dir, "add", "a.txt")
	base = commitInc(t, dir, "base content")
	entry, _ = newIncrementalTestEntry(t, dir)
	return dir, base, entry
}

func reviewedOf(t *testing.T, entry *RepoEntry, head, cur WorkingContent) []gitreview.LineRange {
	t.Helper()
	res, err := entry.WorkingFileReview(context.Background(), "main", "a.txt", head, cur)
	if err != nil {
		t.Fatalf("WorkingFileReview: %v", err)
	}
	return res.ReviewedRanges
}

func TestWorkingMarkProjection(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	_, base, entry := workingFixture(t)
	head := textContent(workingBase)
	v1 := textContent("a\nb\nC!\nd\ne\n")
	if _, err := entry.MarkWorkingFile(ctx, "main", "a.txt", v1, base, true, []gitreview.LineRange{lr(3, 3)}); err != nil {
		t.Fatalf("mark: %v", err)
	}
	if got := reviewedOf(t, entry, head, v1); !reflect.DeepEqual(got, []gitreview.LineRange{lr(3, 3)}) {
		t.Fatalf("unchanged: %v", got)
	}
	shifted := textContent("top\na\nb\nC!\nd\ne\n")
	if got := reviewedOf(t, entry, head, shifted); !reflect.DeepEqual(got, []gitreview.LineRange{lr(4, 4)}) {
		t.Fatalf("shifted: %v", got)
	}
	edited := textContent("a\nb\nC!!\nd\ne\n")
	if got := reviewedOf(t, entry, head, edited); len(got) != 0 {
		t.Fatalf("edit inside a marked line must reopen it: %v", got)
	}
}

func commitWorktree(t *testing.T, entry *RepoEntry, dir, content string) string {
	t.Helper()
	writeIncFile(t, dir, "a.txt", content)
	runInc(t, dir, "add", "a.txt")
	sha := commitInc(t, dir, "commit work")
	waitForRefsChangedInc(t, entry)
	return sha
}

func TestPromoteExactCommitMovesRangesAndDeletesRecord(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dir, base, entry := workingFixture(t)
	work := "a\nb\nC!\nd\ne\n"
	if _, err := entry.MarkWorkingFile(ctx, "main", "a.txt", textContent(work), base, true, nil); err != nil {
		t.Fatalf("mark: %v", err)
	}
	if paths, err := entry.PromoteWorkingReview(ctx, "main", headOf(t, dir)); err != nil || len(paths) != 0 {
		t.Fatalf("promote before a commit = %v, %v; want nothing", paths, err)
	}
	commitWorktree(t, entry, dir, work)
	paths, err := entry.PromoteWorkingReview(ctx, "main", headOf(t, dir))
	if err != nil || !reflect.DeepEqual(paths, []string{"a.txt"}) {
		t.Fatalf("promote = %v, %v", paths, err)
	}
	res, err := entry.ReviewFileDiff(ctx, base, "main", "a.txt", "delta")
	if err != nil {
		t.Fatalf("ReviewFileDiff: %v", err)
	}
	if !reflect.DeepEqual(res.ReviewedRanges, []gitreview.LineRange{lr(3, 3)}) {
		t.Fatalf("committed ranges = %v; only the changed line carries over", res.ReviewedRanges)
	}
	recs, _ := entry.review.WorkingRecords(ctx, entry.Summary.RepoID, "main")
	if len(recs) != 0 {
		t.Fatalf("working record survived promotion: %v", recs)
	}

	// An unmark in the committed view must not come back on the next promotion.
	if _, err := entry.MarkFile(ctx, "main", "a.txt", false, []gitreview.LineRange{lr(3, 3)}); err != nil {
		t.Fatalf("unmark: %v", err)
	}
	if paths, err := entry.PromoteWorkingReview(ctx, "main", headOf(t, dir)); err != nil || len(paths) != 0 {
		t.Fatalf("second promote = %v, %v", paths, err)
	}
	res, _ = entry.ReviewFileDiff(ctx, base, "main", "a.txt", "delta")
	if len(res.ReviewedRanges) != 0 {
		t.Fatalf("unmark did not stick: %v", res.ReviewedRanges)
	}
}

func TestPromotePartialCommitKeepsUncommittedRanges(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dir, base, entry := workingFixture(t)
	both := "A!\nb\nc\nd\nE!\n"
	if _, err := entry.MarkWorkingFile(ctx, "main", "a.txt", textContent(both), base, true, []gitreview.LineRange{lr(1, 1), lr(5, 5)}); err != nil {
		t.Fatalf("mark: %v", err)
	}
	commitWorktree(t, entry, dir, "A!\nb\nc\nd\ne\n")
	writeIncFile(t, dir, "a.txt", both)
	paths, err := entry.PromoteWorkingReview(ctx, "main", headOf(t, dir))
	if err != nil || !reflect.DeepEqual(paths, []string{"a.txt"}) {
		t.Fatalf("promote = %v, %v", paths, err)
	}
	rec, _, found, _ := entry.review.WorkingRecord(ctx, entry.Summary.RepoID, "main", "a.txt")
	if !found || !reflect.DeepEqual(rec.Ranges, []gitreview.LineRange{lr(5, 5)}) {
		t.Fatalf("working record = %+v found=%v; want only line 5", rec, found)
	}
	res, _ := entry.ReviewFileDiff(ctx, base, "main", "a.txt", "delta")
	if !reflect.DeepEqual(res.ReviewedRanges, []gitreview.LineRange{lr(1, 1)}) {
		t.Fatalf("committed ranges = %v", res.ReviewedRanges)
	}
}

func TestPromoteWorkingComments(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dir, base, entry := workingFixture(t)
	work := "a\nb\nC!\nd\nE!\n"
	cur := textContent(work)
	landed, err := entry.AddWorkingComment(ctx, "main", "a.txt", cur, base, lr(3, 3), "why", "change")
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	open, err := entry.AddWorkingComment(ctx, "main", "a.txt", cur, base, lr(5, 5), "and this", "question")
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	commitWorktree(t, entry, dir, "a\nb\nC!\nd\ne\n")
	writeIncFile(t, dir, "a.txt", work)
	if _, err := entry.PromoteWorkingReview(ctx, "main", headOf(t, dir)); err != nil {
		t.Fatalf("promote: %v", err)
	}
	list, err := entry.ListComments(ctx, "main", "")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	got := map[string]CommentEntry{}
	for _, c := range list.Comments {
		got[c.ID] = c
	}
	if c := got[landed.ID]; c.Origin != "commit" || c.Anchor != gitreview.AnchorExact || c.Range != lr(3, 3) {
		t.Fatalf("landed comment = %+v", c)
	}
	if c := got[open.ID]; c.Origin != "working" || c.Anchor != gitreview.AnchorUncommitted || c.Range != lr(5, 5) {
		t.Fatalf("uncommitted comment = %+v", c)
	}
	wl, err := entry.ListWorkingComments(ctx, "main", func(string) (WorkingContent, error) { return textContent(work), nil })
	if err != nil || len(wl) != 1 || wl[0].ID != open.ID || wl[0].Anchor != gitreview.AnchorExact {
		t.Fatalf("working list = %+v, %v", wl, err)
	}
}
