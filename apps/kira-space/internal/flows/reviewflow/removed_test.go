package reviewflow_test

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitreview"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitrpc"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitsession"
)

// Backend half of the review window's "Removed since review" section: reviewed lines that left the
// range diff stay listed until dismissed. Frontend half: tests/ui/ade-v2-review-removed.spec.ts
// reads the git-review-removed contract.
func TestReviewRemovedSinceReview(t *testing.T) {
	app := flowharness.New(t)
	gs := app.OpenGitStream()
	repo := app.NewRepo("proj")
	repo.Commit("base", map[string]string{"a.txt": "a1\na2\na3\na4\na5\n", "b.txt": "b1\n"})
	repo.Git("checkout", "-q", "-b", "feat")
	repo.Commit("work", map[string]string{
		"new.txt": "n1\nn2\nn3\n",
		"a.txt":   "a1\na2\na3\na4\na5\na6\na7\na8\na9\n",
		"b.txt":   "b1\nb2\n",
	})
	repo.Checkout("main")
	id := openRepo(t, gs, repo.Dir)
	mark := func(path string, ranges ...gitreview.LineRange) int64 {
		res := call[gitrpc.ReviewMarkResult](t, gs, "review.mark", gitrpc.ReviewMarkParams{
			RepoID: id, Branch: "feat", Path: path, Reviewed: true, Ranges: ranges,
		})
		if res.Review.ReviewedAt == nil {
			t.Fatalf("mark %s = %+v, want reviewedAt", path, res)
		}
		return *res.Review.ReviewedAt
	}
	newAt := mark("new.txt")
	aAt := mark("a.txt", gitreview.LineRange{Start: 6, End: 8})

	repo.Checkout("feat")
	repo.Git("rm", "-q", "new.txt")
	repo.Commit("trim", map[string]string{"a.txt": "a1\na2\na3\na4\na5\na9\n"})
	repo.Commit("revert b", map[string]string{"b.txt": "b1\n"})
	repo.Checkout("main")
	waitTip(t, gs, id, repo.Git("rev-parse", "feat"))

	// Re-marking overwrites the snapshot; the removed lines are captured before that.
	mark("a.txt")

	list := gitrpc.ReviewRemovedListParams{RepoID: id, Branch: "feat", Base: "main"}
	got := call[gitsession.ReviewRemovedResult](t, gs, "review.removed.list", list)
	if len(got.Entries) != 2 {
		t.Fatalf("entries = %+v, want a.txt and new.txt", got.Entries)
	}
	a, n := got.Entries[0], got.Entries[1]
	if a.Path != "a.txt" || a.Range == nil || *a.Range != (gitreview.LineRange{Start: 6, End: 8}) ||
		a.Text == nil || *a.Text != "a6\na7\na8\n" || a.ReviewedAt != aAt || a.Origin != "commit" {
		t.Fatalf("a.txt entry = %+v", a)
	}
	if n.Path != "new.txt" || n.Range == nil || *n.Range != (gitreview.LineRange{Start: 1, End: 3}) ||
		n.Text == nil || *n.Text != "n1\nn2\nn3\n" || n.ReviewedAt != newAt {
		t.Fatalf("new.txt entry = %+v", n)
	}
	mask := flowharness.Mask("reviewedAt", "reviewedAtSha", "removedAt")
	app.Contract(t, "git-review-removed", "args:git:review.removed.list", list)
	app.Contract(t, "git-review-removed", "git:review.removed.list#removed", got, mask)

	again := call[gitsession.ReviewRemovedResult](t, gs, "review.removed.list", list)
	if !reflect.DeepEqual(again, got) {
		t.Fatalf("second list = %+v, want the same entries %+v", again, got)
	}

	dismiss := gitrpc.ReviewRemovedDismissParams{RepoID: id, Branch: "feat", IDs: []string{a.ID, n.ID}}
	done := call[gitrpc.ReviewRemovedDismissResult](t, gs, "review.removed.dismiss", dismiss)
	if done.Dismissed != 2 {
		t.Fatalf("dismissed = %d, want 2", done.Dismissed)
	}
	app.Contract(t, "git-review-removed", "args:git:review.removed.dismiss", dismiss)
	app.Contract(t, "git-review-removed", "git:review.removed.dismiss", done)
	empty := call[gitsession.ReviewRemovedResult](t, gs, "review.removed.list", list)
	if len(empty.Entries) != 0 {
		t.Fatalf("after dismiss = %+v, want none", empty.Entries)
	}
	app.Contract(t, "git-review-removed", "git:review.removed.list#dismissed", empty)

	// A dismissed block never comes back, even when the same file is re-marked.
	mark("a.txt")
	if again = call[gitsession.ReviewRemovedResult](t, gs, "review.removed.list", list); len(again.Entries) != 0 {
		t.Fatalf("after re-mark = %+v, want none", again.Entries)
	}

	if code := wireErrCode(gs, "review.removed.dismiss", gitrpc.ReviewRemovedDismissParams{RepoID: id, Branch: "feat"}); code != "E_CONTRACT" {
		t.Fatalf("empty ids = %q, want E_CONTRACT", code)
	}
	if code := wireErrCode(gs, "review.removed.dismiss", gitrpc.ReviewRemovedDismissParams{RepoID: id, Branch: "feat", IDs: []string{"0"}}); code != "E_CONTRACT" {
		t.Fatalf("zero id = %q, want E_CONTRACT", code)
	}
}

// A reviewed file rewritten past the patch cap has no parsed hunks; every reviewed line still counts
// as removed. A range error (unrelated base) keeps the stored blocks listed.
func TestReviewRemovedUnparsedDelta(t *testing.T) {
	app := flowharness.New(t)
	gs := app.OpenGitStream()
	repo := app.NewRepo("proj")
	lines := func(prefix string) string {
		var b strings.Builder
		for i := range 6000 {
			fmt.Fprintf(&b, "%s%04d %s\n", prefix, i, strings.Repeat("x", 90))
		}
		return b.String()
	}
	repo.Commit("base", map[string]string{"seed.txt": "s\n"})
	repo.Git("checkout", "-q", "-b", "feat")
	repo.Commit("work", map[string]string{"big.txt": lines("old")})
	repo.Checkout("main")
	id := openRepo(t, gs, repo.Dir)
	call[gitrpc.ReviewMarkResult](t, gs, "review.mark", gitrpc.ReviewMarkParams{
		RepoID: id, Branch: "feat", Path: "big.txt", Reviewed: true,
	})

	repo.Checkout("feat")
	repo.Commit("rewrite", map[string]string{"big.txt": lines("new")})
	repo.Checkout("main")
	waitTip(t, gs, id, repo.Git("rev-parse", "feat"))

	list := gitrpc.ReviewRemovedListParams{RepoID: id, Branch: "feat", Base: "main"}
	got := call[gitsession.ReviewRemovedResult](t, gs, "review.removed.list", list)
	if len(got.Entries) != 1 || got.Entries[0].Path != "big.txt" || got.Entries[0].Range == nil ||
		*got.Entries[0].Range != (gitreview.LineRange{Start: 1, End: 6000}) {
		t.Fatalf("entries = %+v, want big.txt 1-6000", got.Entries)
	}

	repo.Git("checkout", "-q", "--orphan", "lone")
	repo.Commit("lone", map[string]string{"lone.txt": "l\n"})
	repo.Checkout("main")
	list.Base = "lone"
	kept := call[gitsession.ReviewRemovedResult](t, gs, "review.removed.list", list)
	if len(kept.Entries) != 1 {
		t.Fatalf("entries on range error = %+v, want the stored block", kept.Entries)
	}
}

// waitTip blocks until the session's cached refs show want as feat's tip.
func waitTip(t *testing.T, gs *flowharness.GitStream, repoID, want string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		files := call[gitsession.RangeFilesResult](t, gs, "review.files", gitrpc.ReviewFilesParams{RepoID: repoID, Branch: "feat", Base: "main"})
		if files.BranchTip == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("branch tip = %s, want %s", files.BranchTip, want)
		}
		time.Sleep(50 * time.Millisecond)
	}
}
