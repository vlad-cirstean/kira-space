package gitsession

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitclient/porcelain"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitreview"
)

// incFixtureEnv isolates every fixture repo's own git config from the machine's (G5 D19's
// fixtureEnv() convention) — never git config --global/--system.
func incFixtureEnv() []string {
	return append(os.Environ(),
		"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.com",
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
	)
}

func runInc(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = incFixtureEnv()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func writeIncFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func commitInc(t *testing.T, dir, msg string) string {
	t.Helper()
	runInc(t, dir, "-c", "commit.gpgsign=false", "commit", "-q", "-m", msg)
	return trimTrailingNL(runInc(t, dir, "rev-parse", "HEAD"))
}

// newIncrementalTestEntry opens repoDir over a fresh Registry whose Review store lives under
// t.TempDir() (never a real app home) and returns the RepoEntry plus that store, so a test can
// seed a FileRecord directly with Store.Put before driving FileDelta/ReviewFileDiff.
func newIncrementalTestEntry(t *testing.T, repoDir string) (*RepoEntry, *gitreview.Store) {
	t.Helper()
	store := gitreview.NewStore(filepath.Join(t.TempDir(), "review.db"))
	_, entry := newTestEntry(t, ConnID("incremental-test-conn"), repoDir, testEntryOpts{store: store})
	return entry, store
}

func blobOIDInc(t *testing.T, dir, rev, path string) string {
	t.Helper()
	return trimTrailingNL(runInc(t, dir, "rev-parse", rev+":"+path))
}

// buildFileDeltaFixture builds a one-commit repo (main, a.txt = "line1\nline2\nline3\n") and
// returns its dir and that first commit's sha, ready for each scenario to mutate from there.
func buildFileDeltaFixture(t *testing.T) (dir, sha1 string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	dir = t.TempDir()
	runInc(t, dir, "init", "-q", "-b", "main")
	writeIncFile(t, dir, "a.txt", "line1\nline2\nline3\n")
	runInc(t, dir, "add", "a.txt")
	sha1 = commitInc(t, dir, "initial commit")
	return dir, sha1
}

// TestFileDelta_UnchangedAfterUnrelatedCommits is D18's tier-0 scenario: a.txt's own content never
// moves, only an unrelated file changes — blob-oid equality answers "unchanged" with no diff spawn.
func TestFileDelta_UnchangedAfterUnrelatedCommits(t *testing.T) {
	t.Parallel()
	dir, sha1 := buildFileDeltaFixture(t)
	entry, store := newIncrementalTestEntry(t, dir)
	ctx := context.Background()

	content := "line1\nline2\nline3\n"
	oid1 := blobOIDInc(t, dir, sha1, "a.txt")
	rec := gitreview.FileRecord{
		Path: "a.txt", State: "full", ReviewedAtSHA: sha1, ReviewedAt: time.UnixMilli(1000),
		BlobOID: oid1, ContentKind: gitreview.ContentText, ContentBytes: len(content), LineCount: 3,
	}
	if err := store.Put(ctx, entry.Summary.RepoID, "main", rec, []byte(content)); err != nil {
		t.Fatalf("Put: %v", err)
	}
	storedRec, snapshot, found, err := store.Record(ctx, entry.Summary.RepoID, "main", "a.txt")
	if err != nil || !found {
		t.Fatalf("Record: found=%v err=%v", found, err)
	}

	writeIncFile(t, dir, "b.txt", "unrelated\n")
	runInc(t, dir, "add", "b.txt")
	commitInc(t, dir, "unrelated commit")

	delta, err := entry.FileDelta(ctx, "main", "a.txt", storedRec, snapshot, "main")
	if err != nil {
		t.Fatalf("FileDelta: %v", err)
	}
	if delta.Source != "unchanged" {
		t.Fatalf("Source = %q, want unchanged", delta.Source)
	}
	if delta.CurrentLineCount != 3 {
		t.Fatalf("CurrentLineCount = %d, want 3", delta.CurrentLineCount)
	}
}

// TestFileDelta_FastAfterANormalEdit is D18's tier-1 scenario: an ordinary commit edits the
// reviewed file, and the snapshot commit is still an ancestor of the branch.
func TestFileDelta_FastAfterANormalEdit(t *testing.T) {
	t.Parallel()
	dir, sha1 := buildFileDeltaFixture(t)
	entry, store := newIncrementalTestEntry(t, dir)
	ctx := context.Background()

	content := "line1\nline2\nline3\n"
	oid1 := blobOIDInc(t, dir, sha1, "a.txt")
	rec := gitreview.FileRecord{
		Path: "a.txt", State: "full", ReviewedAtSHA: sha1, ReviewedAt: time.UnixMilli(1000),
		BlobOID: oid1, ContentKind: gitreview.ContentText, ContentBytes: len(content), LineCount: 3,
	}
	if err := store.Put(ctx, entry.Summary.RepoID, "main", rec, []byte(content)); err != nil {
		t.Fatalf("Put: %v", err)
	}
	storedRec, snapshot, found, err := store.Record(ctx, entry.Summary.RepoID, "main", "a.txt")
	if err != nil || !found {
		t.Fatalf("Record: found=%v err=%v", found, err)
	}

	writeIncFile(t, dir, "a.txt", "line1\nline2\nline3\nline4\n")
	runInc(t, dir, "add", "a.txt")
	commitInc(t, dir, "edit a.txt")

	delta, err := entry.FileDelta(ctx, "main", "a.txt", storedRec, snapshot, "main")
	if err != nil {
		t.Fatalf("FileDelta: %v", err)
	}
	if delta.Source != "fast" {
		t.Fatalf("Source = %q, want fast", delta.Source)
	}
	if delta.Body.Kind != "text" || len(delta.Hunks) == 0 {
		t.Fatalf("Body/Hunks = %+v/%v, want a real text diff", delta.Body, delta.Hunks)
	}
	if delta.CurrentLineCount != 4 {
		t.Fatalf("CurrentLineCount = %d, want 4", delta.CurrentLineCount)
	}
}

// TestFileDelta_FastPathTooLargePatch_StillReportsTheRealCurrentLineCount: when the fast (tier-1)
// path's own raw patch exceeds MaxPatchBytes, parseAndResolve returns a zero-value ParsedBody
// (no hunks) — CurrentLineCount used to be computed as `rec.LineCount + sumHunkDelta(nil)`,
// silently equal to rec.LineCount (the SNAPSHOT's own line count),
// even though ancestorRes.ExitCode == 0 already proved the file changed.
// Rewrites a.txt from 3 lines to 9000 DIFFERENT lines (a diff comfortably over the 1 MiB cap) —
// CurrentLineCount must report 9000, not the stale snapshot's 3.
func TestFileDelta_FastPathTooLargePatch_StillReportsTheRealCurrentLineCount(t *testing.T) {
	t.Parallel()
	dir, sha1 := buildFileDeltaFixture(t)
	entry, store := newIncrementalTestEntry(t, dir)
	ctx := context.Background()

	content := "line1\nline2\nline3\n"
	oid1 := blobOIDInc(t, dir, sha1, "a.txt")
	rec := gitreview.FileRecord{
		Path: "a.txt", State: "full", ReviewedAtSHA: sha1, ReviewedAt: time.UnixMilli(1000),
		BlobOID: oid1, ContentKind: gitreview.ContentText, ContentBytes: len(content), LineCount: 3,
	}
	if err := store.Put(ctx, entry.Summary.RepoID, "main", rec, []byte(content)); err != nil {
		t.Fatalf("Put: %v", err)
	}
	storedRec, snapshot, found, err := store.Record(ctx, entry.Summary.RepoID, "main", "a.txt")
	if err != nil || !found {
		t.Fatalf("Record: found=%v err=%v", found, err)
	}

	const wantLines = 15000
	var b strings.Builder
	for i := 0; i < wantLines; i++ {
		fmt.Fprintf(&b, "a completely different line number %d with enough padding to push the raw diff comfortably past the one-mebibyte MaxPatchBytes cap regardless of git's own diff header overhead\n", i)
	}
	writeIncFile(t, dir, "a.txt", b.String())
	runInc(t, dir, "add", "a.txt")
	commitInc(t, dir, "rewrite a.txt entirely")

	delta, err := entry.FileDelta(ctx, "main", "a.txt", storedRec, snapshot, "main")
	if err != nil {
		t.Fatalf("FileDelta: %v", err)
	}
	if delta.Source != "fast" {
		t.Fatalf("Source = %q, want fast", delta.Source)
	}
	if delta.Body.Kind != porcelain.BodyTooLarge {
		t.Fatalf("Body.Kind = %q, want %q (this test's own fixture is meant to exceed MaxPatchBytes)", delta.Body.Kind, porcelain.BodyTooLarge)
	}
	if delta.CurrentLineCount != wantLines {
		t.Fatalf("CurrentLineCount = %d, want %d (the file's real current line count, not rec.LineCount=%d)", delta.CurrentLineCount, wantLines, rec.LineCount)
	}
}

// TestFileDelta_SlowAfterAmend is D18's tier-2 scenario, and F4's own headline finding: after
// `git commit --amend`, the snapshot commit is unreachable but still present (exit 1, not 128) —
// the overwhelmingly common rewrite case, and this phase's whole reason to exist.
func TestFileDelta_SlowAfterAmend(t *testing.T) {
	t.Parallel()
	dir, sha1 := buildFileDeltaFixture(t)
	entry, store := newIncrementalTestEntry(t, dir)
	ctx := context.Background()

	content := "line1\nline2\nline3\n"
	oid1 := blobOIDInc(t, dir, sha1, "a.txt")
	rec := gitreview.FileRecord{
		Path: "a.txt", State: "full", ReviewedAtSHA: sha1, ReviewedAt: time.UnixMilli(1000),
		BlobOID: oid1, ContentKind: gitreview.ContentText, ContentBytes: len(content), LineCount: 3,
	}
	if err := store.Put(ctx, entry.Summary.RepoID, "main", rec, []byte(content)); err != nil {
		t.Fatalf("Put: %v", err)
	}
	storedRec, snapshot, found, err := store.Record(ctx, entry.Summary.RepoID, "main", "a.txt")
	if err != nil || !found {
		t.Fatalf("Record: found=%v err=%v", found, err)
	}

	writeIncFile(t, dir, "a.txt", "line1\nline2\nline3\nline4\n")
	runInc(t, dir, "add", "a.txt")
	runInc(t, dir, "-c", "commit.gpgsign=false", "commit", "--amend", "-q", "-m", "amended")

	// sha1 is no longer reachable from main, but git has not pruned it — probe P2's own claim.
	if out := runInc(t, dir, "cat-file", "-e", sha1); out != "" {
		t.Fatalf("cat-file -e unexpectedly printed output: %q", out)
	}

	delta, err := entry.FileDelta(ctx, "main", "a.txt", storedRec, snapshot, "main")
	if err != nil {
		t.Fatalf("FileDelta: %v", err)
	}
	if delta.Source != "slow" {
		t.Fatalf("Source = %q, want slow", delta.Source)
	}
	if delta.Body.Kind != "text" || len(delta.Hunks) == 0 {
		t.Fatalf("Body/Hunks = %+v/%v, want a real text diff", delta.Body, delta.Hunks)
	}
	if delta.CurrentLineCount != 4 {
		t.Fatalf("CurrentLineCount = %d, want 4", delta.CurrentLineCount)
	}
}

// TestFileDelta_SlowAfterPrune is D18's own "genuinely pruned" scenario: the snapshot commit is
// not merely unreachable but actually gone (exit 128) — still the slow path, still correct.
func TestFileDelta_SlowAfterPrune(t *testing.T) {
	t.Parallel()
	dir, sha1 := buildFileDeltaFixture(t)
	entry, store := newIncrementalTestEntry(t, dir)
	ctx := context.Background()

	content := "line1\nline2\nline3\n"
	oid1 := blobOIDInc(t, dir, sha1, "a.txt")
	rec := gitreview.FileRecord{
		Path: "a.txt", State: "full", ReviewedAtSHA: sha1, ReviewedAt: time.UnixMilli(1000),
		BlobOID: oid1, ContentKind: gitreview.ContentText, ContentBytes: len(content), LineCount: 3,
	}
	if err := store.Put(ctx, entry.Summary.RepoID, "main", rec, []byte(content)); err != nil {
		t.Fatalf("Put: %v", err)
	}
	storedRec, snapshot, found, err := store.Record(ctx, entry.Summary.RepoID, "main", "a.txt")
	if err != nil || !found {
		t.Fatalf("Record: found=%v err=%v", found, err)
	}

	writeIncFile(t, dir, "a.txt", "line1\nline2\nline3\nline4\n")
	runInc(t, dir, "add", "a.txt")
	runInc(t, dir, "-c", "commit.gpgsign=false", "commit", "--amend", "-q", "-m", "amended")
	runInc(t, dir, "reflog", "expire", "--expire=now", "--all")
	runInc(t, dir, "gc", "--prune=now", "-q")

	// sha1 is now genuinely gone — cat-file -e must fail (a non-zero exit, CombinedOutput would
	// otherwise Fatalf via runInc, so probe this one with exec directly).
	cmd := exec.Command("git", "cat-file", "-e", sha1)
	cmd.Dir = dir
	if err := cmd.Run(); err == nil {
		t.Fatal("sha1 unexpectedly still resolves after reflog expire + gc --prune=now")
	}

	delta, err := entry.FileDelta(ctx, "main", "a.txt", storedRec, snapshot, "main")
	if err != nil {
		t.Fatalf("FileDelta: %v", err)
	}
	if delta.Source != "slow" {
		t.Fatalf("Source = %q, want slow", delta.Source)
	}
	if delta.Body.Kind != "text" || len(delta.Hunks) == 0 {
		t.Fatalf("Body/Hunks = %+v/%v, want a real text diff", delta.Body, delta.Hunks)
	}
}

// TestFileDelta_SnapshotUnavailableForRewrittenBinary is D18's own binary-degradation scenario: a
// binary file's snapshot stores no content at all (D9), so once its sha is rewritten there is
// nothing left to diff against — reported honestly, never silently as "unchanged" or an error.
func TestFileDelta_SnapshotUnavailableForRewrittenBinary(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	dir := t.TempDir()
	runInc(t, dir, "init", "-q", "-b", "main")
	binary := []byte{0x89, 'P', 'N', 'G', 0x00, 0x01, 0x02, 0x03}
	if err := os.WriteFile(filepath.Join(dir, "img.bin"), binary, 0o644); err != nil {
		t.Fatalf("write img.bin: %v", err)
	}
	runInc(t, dir, "add", "img.bin")
	sha1 := commitInc(t, dir, "add binary")

	entry, store := newIncrementalTestEntry(t, dir)
	ctx := context.Background()

	oid1 := blobOIDInc(t, dir, sha1, "img.bin")
	rec := gitreview.FileRecord{
		Path: "img.bin", State: "full", ReviewedAtSHA: sha1, ReviewedAt: time.UnixMilli(1000),
		BlobOID: oid1, ContentKind: gitreview.ContentBinary, ContentBytes: 0, LineCount: 0,
	}
	if err := store.Put(ctx, entry.Summary.RepoID, "main", rec, nil); err != nil {
		t.Fatalf("Put: %v", err)
	}
	storedRec, snapshot, found, err := store.Record(ctx, entry.Summary.RepoID, "main", "img.bin")
	if err != nil || !found {
		t.Fatalf("Record: found=%v err=%v", found, err)
	}

	binary2 := []byte{0x89, 'P', 'N', 'G', 0xFF, 0xFE, 0xFD, 0xFC}
	if err := os.WriteFile(filepath.Join(dir, "img.bin"), binary2, 0o644); err != nil {
		t.Fatalf("rewrite img.bin: %v", err)
	}
	runInc(t, dir, "add", "img.bin")
	runInc(t, dir, "-c", "commit.gpgsign=false", "commit", "--amend", "-q", "-m", "amended binary")

	delta, err := entry.FileDelta(ctx, "main", "img.bin", storedRec, snapshot, "main")
	if err != nil {
		t.Fatalf("FileDelta: %v", err)
	}
	if delta.Source != "snapshotUnavailable" {
		t.Fatalf("Source = %q, want snapshotUnavailable", delta.Source)
	}
}

// TestReviewFileDiff_NoSnapshotForAFileWithNoRecord is D18's own "noSnapshot for a file with no
// record" scenario — decided one level up from FileDelta (which always assumes a record exists),
// in ReviewFileDiff itself: the delta IS the whole range diff.
func TestReviewFileDiff_NoSnapshotForAFileWithNoRecord(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	dir := t.TempDir()
	runInc(t, dir, "init", "-q", "-b", "main")
	writeIncFile(t, dir, "a.txt", "line1\nline2\n")
	runInc(t, dir, "add", "a.txt")
	commitInc(t, dir, "base commit")

	runInc(t, dir, "checkout", "-q", "-b", "feature")
	writeIncFile(t, dir, "a.txt", "line1\nline2\nline3\n")
	runInc(t, dir, "add", "a.txt")
	commitInc(t, dir, "feature commit")

	entry, _ := newIncrementalTestEntry(t, dir)
	ctx := context.Background()

	result, err := entry.ReviewFileDiff(ctx, "main", "feature", "a.txt", "sinceReview")
	if err != nil {
		t.Fatalf("ReviewFileDiff: %v", err)
	}
	if result.DeltaSource != "noSnapshot" {
		t.Fatalf("DeltaSource = %q, want noSnapshot", result.DeltaSource)
	}
	if len(result.ReviewedRanges) != 0 {
		t.Fatalf("ReviewedRanges = %v, want empty", result.ReviewedRanges)
	}
	if result.ReviewedAtSHA != nil {
		t.Fatalf("ReviewedAtSHA = %v, want nil", result.ReviewedAtSHA)
	}
	if result.Body.Kind != "text" || len(result.Body.Hunks) == 0 {
		t.Fatalf("Body = %+v, want the whole range diff, not an empty body", result.Body)
	}
}

// TestRangeFiles_MergeBaseIsCachedAcrossRequests is the proof: RangeFiles (review.files' own
// orchestration) used to spawn `git merge-base` uncached on every single call, even for the
// identical (base, branch) pair a review session's own review.files/review.fileDiff round trips
// repeat over and over while the session sits open.
func TestRangeFiles_MergeBaseIsCachedAcrossRequests(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	dir := t.TempDir()
	runInc(t, dir, "init", "-q", "-b", "main")
	writeIncFile(t, dir, "a.txt", "line1\n")
	runInc(t, dir, "add", "a.txt")
	commitInc(t, dir, "base commit")

	runInc(t, dir, "checkout", "-q", "-b", "feature")
	writeIncFile(t, dir, "a.txt", "line1\nline2\n")
	runInc(t, dir, "add", "a.txt")
	commitInc(t, dir, "feature commit")

	runner := newArgSpawnCountingRunner("merge-base")
	registry := NewRegistry(runner)
	store := gitreview.NewStore(filepath.Join(t.TempDir(), "review.db"))
	registry.Review = store
	t.Cleanup(registry.Close)

	conn := NewConn(ConnID("merge-base-cache-test-conn"), "test-client", "test-client-label", nil)
	summary, err := conn.Open(context.Background(), registry, "git", dir)
	if err != nil {
		t.Fatalf("conn.Open: %v", err)
	}
	t.Cleanup(conn.Close)
	entry, ok := conn.Entry(summary.RepoID)
	if !ok {
		t.Fatal("conn.Entry: not held after Open")
	}

	ctx := context.Background()
	first, err := entry.RangeFiles(ctx, "main", "feature")
	if err != nil {
		t.Fatalf("RangeFiles (first): %v", err)
	}
	if got := runner.count("merge-base"); got != 1 {
		t.Fatalf("merge-base spawns after the first RangeFiles = %d, want 1", got)
	}

	second, err := entry.RangeFiles(ctx, "main", "feature")
	if err != nil {
		t.Fatalf("RangeFiles (second): %v", err)
	}
	if got := runner.count("merge-base"); got != 1 {
		t.Fatalf("merge-base spawns after a second, identical RangeFiles = %d, want still 1 (cached)", got)
	}
	if second.MergeBase != first.MergeBase {
		t.Fatalf("MergeBase = %q, want %q (the cached value must match the freshly spawned one)", second.MergeBase, first.MergeBase)
	}

	// A refsChanged signal (a real commit landing on the branch) must invalidate the cache —
	// the next RangeFiles call re-spawns rather than serving a now-stale merge-base.
	writeIncFile(t, dir, "a.txt", "line1\nline2\nline3\n")
	runInc(t, dir, "add", "a.txt")
	commitInc(t, dir, "second feature commit")
	waitForRefsChangedInc(t, entry)

	if _, err := entry.RangeFiles(ctx, "main", "feature"); err != nil {
		t.Fatalf("RangeFiles (after a real ref move): %v", err)
	}
	if got := runner.count("merge-base"); got != 2 {
		t.Fatalf("merge-base spawns after a real ref move = %d, want 2 (the cache must drop on refsChanged)", got)
	}
}

// TestMarkFile_StoredBlobOIDMatchesGit is the regression coverage: MarkFile's own currentOID used
// to come from a SECOND, separately-spawned blobOID(ctx, tip, path) call, after readSnapshotSource
// had already resolved (and discarded) the identical rev's OID as part of its own Read. MarkFile
// now takes currentOID straight off readSnapshotSource's own return (readCurrentContent's own doc
// comment) — this proves that plumbing is correct, not just "compiles": the stored record's
// BlobOID must still equal what an independent `git rev-parse <tip>:<path>` reports.
func TestMarkFile_StoredBlobOIDMatchesGit(t *testing.T) {
	t.Parallel()
	dir, sha1 := buildFileDeltaFixture(t)
	entry, store := newIncrementalTestEntry(t, dir)
	ctx := context.Background()

	rec, err := entry.MarkFile(ctx, "main", "a.txt", true, nil)
	if err != nil {
		t.Fatalf("MarkFile: %v", err)
	}
	wantOID := blobOIDInc(t, dir, sha1, "a.txt")
	if rec.BlobOID != wantOID {
		t.Fatalf("MarkFile's stored BlobOID = %q, want %q (git rev-parse %s:a.txt)", rec.BlobOID, wantOID, sha1)
	}

	storedRec, _, found, err := store.Record(ctx, entry.Summary.RepoID, "main", "a.txt")
	if err != nil || !found {
		t.Fatalf("Record: found=%v err=%v", found, err)
	}
	if storedRec.BlobOID != wantOID {
		t.Fatalf("stored record's own BlobOID = %q, want %q", storedRec.BlobOID, wantOID)
	}
}

// waitForRefsChangedInc polls entry.refs until it reports invalid (dropped by note() on
// refsChanged) or the timeout expires — the watcher's own signal is asynchronous (fsnotify), so a
// test proving cache invalidation on a real ref move cannot call RangeFiles immediately
// after the commit above.
func waitForRefsChangedInc(t *testing.T, entry *RepoEntry) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, valid := entry.refs.get(); !valid {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("timed out waiting for the watcher's own refsChanged signal to invalidate refs")
}

// A renamed file with identical content keeps its review under the new path; an edited rename does not (P150 Q5).
func TestRangeFiles_RenamedUnchangedKeepsReview(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	dir := t.TempDir()
	runInc(t, dir, "init", "-q", "-b", "main")
	writeIncFile(t, dir, "keep.txt", "one\ntwo\nthree\nfour\nfive\n")
	writeIncFile(t, dir, "edit.txt", "alpha\nbeta\ngamma\ndelta\nepsilon\n")
	runInc(t, dir, "add", ".")
	commitInc(t, dir, "base")
	runInc(t, dir, "checkout", "-q", "-b", "feature")
	writeIncFile(t, dir, "other.txt", "x\n")
	runInc(t, dir, "add", ".")
	commitInc(t, dir, "other")

	entry, _ := newIncrementalTestEntry(t, dir)
	ctx := context.Background()
	for _, p := range []string{"keep.txt", "edit.txt"} {
		if _, err := entry.MarkFile(ctx, "feature", p, true, nil); err != nil {
			t.Fatalf("MarkFile %s: %v", p, err)
		}
	}
	runInc(t, dir, "mv", "keep.txt", "kept.txt")
	runInc(t, dir, "mv", "edit.txt", "edited.txt")
	writeIncFile(t, dir, "edited.txt", "alpha\nbeta\ngamma\ndelta\nepsilon\nzeta\n")
	runInc(t, dir, "add", ".")
	commitInc(t, dir, "renames")
	waitForRefsChangedInc(t, entry)

	res, err := entry.RangeFiles(ctx, "main", "feature")
	if err != nil {
		t.Fatalf("RangeFiles: %v", err)
	}
	got := map[string]ReviewFileStatus{}
	for _, f := range res.Files {
		got[f.Change.Path] = f.Review
	}
	if k := got["kept.txt"]; k.Kind != "full" || k.ChangedSinceReview {
		t.Errorf("kept.txt = %+v, want full and unchanged", k)
	}
	if e := got["edited.txt"]; e.Kind != "none" {
		t.Errorf("edited.txt = %+v, want none (content differs)", e)
	}
}

// A file marked while over MaxSnapshotBytes stores LineCount 0; after it shrinks below the cap the
// delta must measure the real count, not derive one from that zero (it went negative before).
func TestMarkFile_RangedMarkAfterTooLargeFileShrinks(t *testing.T) {
	t.Parallel()
	dir, _ := buildFileDeltaFixture(t)
	entry, _ := newIncrementalTestEntry(t, dir)
	ctx := context.Background()

	lines := func(n int) string {
		var b strings.Builder
		for i := 0; i < n; i++ {
			fmt.Fprintf(&b, "line %06d padded to exactly one hundred bytes %s\n", i, strings.Repeat("x", 42))
		}
		return b.String()
	}
	big := lines(12000)
	if len(big) <= gitreview.MaxSnapshotBytes {
		t.Fatalf("fixture is %d bytes, want over %d", len(big), gitreview.MaxSnapshotBytes)
	}
	writeIncFile(t, dir, "a.txt", big)
	runInc(t, dir, "add", "a.txt")
	commitInc(t, dir, "grow a.txt past the cap")
	rec, err := entry.MarkFile(ctx, "main", "a.txt", true, nil)
	if err != nil || rec.ContentKind != gitreview.ContentTooLarge {
		t.Fatalf("MarkFile = %+v, %v, want a tooLarge record", rec, err)
	}

	const shrunk = 8000
	writeIncFile(t, dir, "a.txt", lines(shrunk))
	runInc(t, dir, "add", "a.txt")
	commitInc(t, dir, "shrink a.txt below the cap")
	waitForRefsChangedInc(t, entry)

	want := []gitreview.LineRange{{Start: 1, End: 10}}
	rec, err = entry.MarkFile(ctx, "main", "a.txt", true, want)
	if err != nil {
		t.Fatalf("ranged MarkFile: %v", err)
	}
	if rec.LineCount != shrunk || rec.State != "partial" || len(rec.Ranges) != 1 || rec.Ranges[0] != want[0] {
		t.Fatalf("record = %+v, want LineCount %d and partial range %v", rec, shrunk, want)
	}
}
