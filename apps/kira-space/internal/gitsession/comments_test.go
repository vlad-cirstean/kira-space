package gitsession

import (
	"context"
	"os/exec"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitreview"
)

// buildCommentFixture builds a one-commit repo (main, a.txt = "line1\nline2\nline3\n") over a
// fresh RepoEntry whose review.db lives under t.TempDir() — D18's own fixture discipline
// (incFixtureEnv, never git config --global/--system).
func buildCommentFixture(t *testing.T) (dir, sha1 string, entry *RepoEntry) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	dir = t.TempDir()
	runInc(t, dir, "init", "-q", "-b", "main")
	writeIncFile(t, dir, "a.txt", "line1\nline2\nline3\n")
	runInc(t, dir, "add", "a.txt")
	sha1 = commitInc(t, dir, "initial commit")
	entry, _ = newIncrementalTestEntry(t, dir)
	return dir, sha1, entry
}

func addTestComment(t *testing.T, ctx context.Context, entry *RepoEntry, at string, r gitreview.LineRange) CommentEntry {
	t.Helper()
	added, err := entry.AddComment(ctx, "main", "a.txt", at, r, "note", "")
	if err != nil {
		t.Fatalf("AddComment: %v", err)
	}
	return added
}

// TestAnchorOne_ExactAfterUnrelatedCommits is D7 tier 0's own scenario: a.txt never moves, only an
// unrelated file changes — blob-oid equality answers "exact" with the stored range unchanged, no
// diff spawn.
func TestAnchorOne_ExactAfterUnrelatedCommits(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dir, sha1, entry := buildCommentFixture(t)
	added := addTestComment(t, ctx, entry, sha1, gitreview.LineRange{Start: 2, End: 2})

	writeIncFile(t, dir, "b.txt", "unrelated\n")
	runInc(t, dir, "add", "b.txt")
	commitInc(t, dir, "unrelated commit")

	result, err := entry.ListComments(ctx, "main", "")
	if err != nil {
		t.Fatalf("ListComments: %v", err)
	}
	if len(result.Comments) != 1 {
		t.Fatalf("len(Comments) = %d, want 1", len(result.Comments))
	}
	got := result.Comments[0]
	if got.ID != added.ID {
		t.Fatalf("ID = %s, want %s", got.ID, added.ID)
	}
	if got.Anchor != gitreview.AnchorExact {
		t.Fatalf("Anchor = %q, want exact", got.Anchor)
	}
	if got.Range != (gitreview.LineRange{Start: 2, End: 2}) {
		t.Fatalf("Range = %+v, want {2 2}", got.Range)
	}
}

// TestAnchorOne_ProjectedWhenLinesShiftAbove is D7 tier 1's non-empty case: five lines inserted
// above the comment shift its projected range by exactly five, through gitreview.ProjectRanges
// unchanged (D7/D18 — no new arithmetic).
func TestAnchorOne_ProjectedWhenLinesShiftAbove(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dir, sha1, entry := buildCommentFixture(t)
	addTestComment(t, ctx, entry, sha1, gitreview.LineRange{Start: 2, End: 2})

	writeIncFile(t, dir, "a.txt", "x1\nx2\nx3\nx4\nx5\nline1\nline2\nline3\n")
	runInc(t, dir, "add", "a.txt")
	commitInc(t, dir, "insert five lines above")

	result, err := entry.ListComments(ctx, "main", "")
	if err != nil {
		t.Fatalf("ListComments: %v", err)
	}
	got := result.Comments[0]
	if got.Anchor != gitreview.AnchorProjected {
		t.Fatalf("Anchor = %q, want projected", got.Anchor)
	}
	if got.Range != (gitreview.LineRange{Start: 7, End: 7}) {
		t.Fatalf("Range = %+v, want {7 7} (shifted by five)", got.Range)
	}
}

// TestAnchorOne_RemovedWhenCommentedLinesDeleted is D7 tier 1's empty case: the commented line is
// deleted outright — ProjectRanges' own nil result (probe P2) is the removed signal, with the
// stored (pre-deletion) range reported rather than a fabricated new one.
func TestAnchorOne_RemovedWhenCommentedLinesDeleted(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dir, sha1, entry := buildCommentFixture(t)
	addTestComment(t, ctx, entry, sha1, gitreview.LineRange{Start: 2, End: 2})

	writeIncFile(t, dir, "a.txt", "line1\nline3\n")
	runInc(t, dir, "add", "a.txt")
	commitInc(t, dir, "delete line 2")

	result, err := entry.ListComments(ctx, "main", "")
	if err != nil {
		t.Fatalf("ListComments: %v", err)
	}
	got := result.Comments[0]
	if got.Anchor != gitreview.AnchorRemoved {
		t.Fatalf("Anchor = %q, want removed", got.Anchor)
	}
	if got.Range != (gitreview.LineRange{Start: 2, End: 2}) {
		t.Fatalf("Range = %+v, want the stored {2 2}", got.Range)
	}
}

// TestAnchorOne_StaleAfterAmend is D7 tier 2's exit-1 case (G11 probe P2's "present but
// unreachable"): amending the commit — with a.txt itself changing, so tier 0 cannot short-circuit
// — leaves the old anchor sha present but unreachable from the new tip.
func TestAnchorOne_StaleAfterAmend(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dir, sha1, entry := buildCommentFixture(t)
	addTestComment(t, ctx, entry, sha1, gitreview.LineRange{Start: 2, End: 2})

	writeIncFile(t, dir, "a.txt", "line1\nline2\nline3-changed\n")
	runInc(t, dir, "add", "a.txt")
	runInc(t, dir, "-c", "commit.gpgsign=false", "commit", "--amend", "--no-edit")

	result, err := entry.ListComments(ctx, "main", "")
	if err != nil {
		t.Fatalf("ListComments: %v", err)
	}
	got := result.Comments[0]
	if got.Anchor != gitreview.AnchorStale {
		t.Fatalf("Anchor = %q, want stale", got.Anchor)
	}
	if got.Range != (gitreview.LineRange{Start: 2, End: 2}) {
		t.Fatalf("Range = %+v, want the stored {2 2}", got.Range)
	}
	if got.AnchorSHA != sha1 {
		t.Fatalf("AnchorSHA = %q, want the original %q", got.AnchorSHA, sha1)
	}
}

// TestAnchorOne_StaleAfterPrune is D7 tier 2's exit-128 case: the anchor commit is genuinely
// pruned (not merely unreachable) — reflog expiry plus gc leaves merge-base --is-ancestor unable
// to resolve it at all, and the result is the same honest "stale" as the amend case.
func TestAnchorOne_StaleAfterPrune(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dir, sha1, entry := buildCommentFixture(t)
	addTestComment(t, ctx, entry, sha1, gitreview.LineRange{Start: 2, End: 2})

	writeIncFile(t, dir, "a.txt", "line1\nline2\nline3-changed\n")
	runInc(t, dir, "add", "a.txt")
	runInc(t, dir, "-c", "commit.gpgsign=false", "commit", "--amend", "--no-edit")
	runInc(t, dir, "reflog", "expire", "--expire=now", "--all")
	runInc(t, dir, "gc", "--prune=now")

	result, err := entry.ListComments(ctx, "main", "")
	if err != nil {
		t.Fatalf("ListComments: %v", err)
	}
	got := result.Comments[0]
	if got.Anchor != gitreview.AnchorStale {
		t.Fatalf("Anchor = %q, want stale", got.Anchor)
	}
}

// TestAnchorOne_ExactAtExplicitOlderRevision is D11's coordinate contract: listing at an explicit
// (older) `at` reports exact against THAT revision, not the tip — even while the tip has since
// changed the very lines the comment is about.
func TestAnchorOne_ExactAtExplicitOlderRevision(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dir, sha1, entry := buildCommentFixture(t)
	addTestComment(t, ctx, entry, sha1, gitreview.LineRange{Start: 2, End: 2})

	writeIncFile(t, dir, "a.txt", "line1\nline2-changed\nline3\n")
	runInc(t, dir, "add", "a.txt")
	commitInc(t, dir, "change line 2")
	writeIncFile(t, dir, "a.txt", "line1\nline2-changed-again\nline3\n")
	runInc(t, dir, "add", "a.txt")
	commitInc(t, dir, "change line 2 again")

	result, err := entry.ListComments(ctx, "main", sha1)
	if err != nil {
		t.Fatalf("ListComments: %v", err)
	}
	if result.At != sha1 {
		t.Fatalf("At = %q, want the requested %q, not the tip", result.At, sha1)
	}
	got := result.Comments[0]
	if got.Anchor != gitreview.AnchorExact {
		t.Fatalf("Anchor = %q, want exact", got.Anchor)
	}
	if got.Range != (gitreview.LineRange{Start: 2, End: 2}) {
		t.Fatalf("Range = %+v, want {2 2}", got.Range)
	}
}
