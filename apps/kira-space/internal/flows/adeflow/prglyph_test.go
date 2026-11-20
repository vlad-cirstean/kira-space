package adeflow_test

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness/fakehost"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/githost"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// P706 pull request glyph. Contract ade-pr-glyph: tests/ui/ade-v2-pr-glyph.spec.ts serves it as the
// change-request read. Keyed by branch name, which the spec maps onto its board fixture.

// TestChangeRequestGlyphStatus: the board's change-request read carries review, mergeable and
// auto-merge beside state and checks, for an approved auto-merging PR, a pending draft, a conflicted
// PR with changes requested, and a merged one.
func TestChangeRequestGlyphStatus(t *testing.T) {
	forge := fakehost.New()
	want := map[string]githost.ChangeRequest{
		"feat/webhooks": {
			Number: 212, Title: "Outgoing webhooks", State: "open", ReviewDecision: "APPROVED", ChecksState: "SUCCESS",
			Mergeable: githost.MergeableYes, AutoMerge: true,
		},
		"feat/oauth-login-ui": {Number: 1425, Title: "Login screen UI", State: "draft", ChecksState: "PENDING"},
		"feat/usage-billing": {
			Number: 1427, Title: "Usage-based billing", State: "open", ReviewDecision: "CHANGES_REQUESTED",
			ChecksState: "FAILURE", Mergeable: githost.MergeableConflict,
		},
		"fix/cart-flaky": {Number: 1431, Title: "Stabilise cart total test", State: "merged"},
	}
	names := []string{"feat/webhooks", "feat/oauth-login-ui", "feat/usage-billing", "fix/cart-flaky"}
	for i, name := range names {
		cr := want[name]
		cr.HeadRef, cr.HeadOwner, cr.BaseRef = name, "acme", "main"
		cr.URL = fmt.Sprintf("https://forge.test/acme/api/pull/%d", cr.Number)
		cr.UpdatedAt = time.Date(2026, 9, 20, 10, i, 0, 0, time.UTC).UnixMilli()
		forge.AddChangeRequest(cr)
	}
	app := flowharness.New(t, flowharness.WithGitHost(forge), flowharness.WithoutGh(), flowharness.WithTrackerGrace(time.Second))
	repo, _ := cloneWithMain(app, filepath.Join(app.Work, "api"))
	gitOut(t, repo.Dir, "remote", "set-url", "origin", "https://forge.test/acme/api.git")
	for _, n := range names {
		gitOut(t, repo.Dir, "branch", n)
	}
	rec := importRepo(t, app, repo.Dir)
	task := createTask(t, app, "Glyph status", "", rec.ID)
	for _, n := range names {
		if _, err := app.W.AdeTask.AddExistingBranch(ctx, adewire.AddExistingBranchArgs{CodeRepoID: rec.ID, Name: n, TaskID: task.ID}); err != nil {
			t.Fatalf("AddExistingBranch %s: %v", n, err)
		}
	}
	byName := map[string]string{}
	for _, b := range branchesOf(t, app, task.ID) {
		byName[b.Name] = b.ID
	}

	var got map[string]adewire.ChangeRequest
	testx.WaitUntil(t, waitFor, func() bool {
		prs, err := app.W.AdeTask.ChangeRequests(ctx)
		if err != nil {
			return false
		}
		got = map[string]adewire.ChangeRequest{}
		for _, n := range names {
			if cr, ok := prs.Branches[byName[n]]; ok {
				got[n] = cr
			}
		}
		return len(got) == len(names)
	})
	for name, w := range want {
		g := got[name]
		wantState := w.State
		if g.State != wantState || g.ChecksState != w.ChecksState || g.ReviewDecision != w.ReviewDecision ||
			g.Mergeable != w.Mergeable || g.AutoMerge != w.AutoMerge {
			t.Errorf("%s = %+v, want state %q checks %q review %q mergeable %q autoMerge %v",
				name, g, wantState, w.ChecksState, w.ReviewDecision, w.Mergeable, w.AutoMerge)
		}
	}
	app.Contract(t, "ade-pr-glyph", "AdeTaskService.ChangeRequests", adewire.ChangeRequestsResult{Branches: got}, tpMask)
}
