package adeflow_test

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitreview"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitrpc"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitsession"
)

// Backend half of the uncommitted entries in the review window's "Removed since review" section.
// Frontend half: tests/ui/ade-v2-review-removed.spec.ts reads the git-review-removed-working contract.
func TestReviewWorkingRemoved(t *testing.T) {
	f, br := doneFixture(t, "feat/api-working-removed")
	app, wt := f.app, br.Worktree
	writeIn(t, wt, "d.txt", "d1\nd2\n")
	gitOut(t, wt, "add", "-A")
	gitOut(t, wt, "commit", "-q", "-m", "add d.txt")
	writeIn(t, wt, "c.txt", "c1\nc2\nc3\nc4\n")
	writeIn(t, wt, "d.txt", "d1\nd2\nd3\n")

	if _, err := app.W.AdeTask.OpenReviewWindow(ctx, adewire.BranchArgs{BranchID: br.ID}); err != nil {
		t.Fatal(err)
	}
	rec := app.WindowMgr.Opened[len(app.WindowMgr.Opened)-1]
	tgt, err := app.W.AdeTask.ReviewWindowTarget(ctx, adewire.WindowKeyArgs{WindowKey: rec.Key})
	if err != nil || tgt == nil {
		t.Fatalf("ReviewWindowTarget = %+v, %v", tgt, err)
	}
	gs := app.OpenGitStream()
	var opened gitrpc.RepoOpenResult
	gs.MustRequest("repo.open", gitrpc.RepoOpenParams{Path: filepath.Join(app.Work, "api")}, &opened)
	list := gitrpc.ReviewRemovedListParams{RepoID: tgt.GitRepoID, Branch: tgt.Branch, Base: tgt.Base}
	listRemoved := func() gitsession.ReviewRemovedResult {
		t.Helper()
		var out gitsession.ReviewRemovedResult
		gs.MustRequest("review.removed.list", list, &out)
		return out
	}
	markWhole := func(paths ...string) {
		t.Helper()
		working, err := app.W.AdeTask.ReviewWorking(ctx, adewire.BranchArgs{BranchID: br.ID})
		if err != nil {
			t.Fatal(err)
		}
		hashes := map[string]string{}
		for _, p := range paths {
			hashes[p] = working.ContentHashes[p]
		}
		if _, err := app.W.AdeTask.ReviewWorkingMark(ctx, adewire.ReviewWorkingMarkArgs{
			BranchID: br.ID, Paths: paths, Reviewed: true, ContentHashes: hashes,
		}); err != nil {
			t.Fatal(err)
		}
	}
	byPath := func(res gitsession.ReviewRemovedResult) map[string][]gitsession.ReviewRemovedEntry {
		out := map[string][]gitsession.ReviewRemovedEntry{}
		for _, e := range res.Entries {
			if e.Origin != "working" {
				t.Fatalf("entry %+v, want working origin", e)
			}
			out[e.Path] = append(out[e.Path], e)
		}
		return out
	}

	markWhole("c.txt", "d.txt")
	writeIn(t, wt, "c.txt", "c1\nc4\n")
	gitOut(t, wt, "checkout", "--", "d.txt") // reverted to HEAD: no longer dirty

	if _, err := app.W.AdeTask.ReviewWorking(ctx, adewire.BranchArgs{BranchID: br.ID}); err != nil {
		t.Fatal(err)
	}
	got := listRemoved()
	entries := byPath(got)
	c, d := entries["c.txt"], entries["d.txt"]
	if len(entries) != 2 || len(c) != 1 || len(d) != 1 {
		t.Fatalf("entries = %+v, want one for c.txt and one for d.txt", got.Entries)
	}
	if *c[0].Range != (gitreview.LineRange{Start: 2, End: 3}) || *c[0].Text != "c2\nc3\n" {
		t.Fatalf("c.txt entry = %+v, want lines 2-3", c[0])
	}
	if *d[0].Range != (gitreview.LineRange{Start: 3, End: 3}) || *d[0].Text != "d3\n" {
		t.Fatalf("d.txt entry = %+v, want line 3", d[0])
	}

	// Listing again adds no duplicates.
	if _, err := app.W.AdeTask.ReviewWorking(ctx, adewire.BranchArgs{BranchID: br.ID}); err != nil {
		t.Fatal(err)
	}
	if again := listRemoved(); !reflect.DeepEqual(again, got) {
		t.Fatalf("second list = %+v, want %+v", again, got)
	}

	// A re-mark overwrites the snapshot; lines deleted since are captured first.
	markWhole("c.txt")
	writeIn(t, wt, "c.txt", "c1\n")
	markWhole("c.txt")
	c = byPath(listRemoved())["c.txt"]
	if len(c) != 2 || *c[1].Text != "c4\n" || *c[1].Range != (gitreview.LineRange{Start: 2, End: 2}) {
		t.Fatalf("c.txt entries after re-mark = %+v, want 2-3 and 2", c)
	}

	app.Contract(t, "git-review-removed-working", "args:git:review.removed.list", list,
		flowharness.Replace(tgt.GitRepoID, "<repo>"), flowharness.Replace(tgt.Branch, "<branch>"))
	app.Contract(t, "git-review-removed-working", "git:review.removed.list#working", listRemoved(),
		flowharness.Mask("reviewedAt", "reviewedAtSha", "removedAt"))
}
