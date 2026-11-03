package gitflow_test

import (
	"context"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness/fakehost"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/githost"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitreview"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitrpc"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
)

// GitService carries the review and change-request methods with the stream's results: byte-equal
// answers, the stream's refusals, and a bound hold that keeps the window's connection past its last
// stream.
func TestGitServiceBoundReview(t *testing.T) {
	const scenario = "git-bound-review"
	forge := fakehost.New()
	forge.AddChangeRequest(githost.ChangeRequest{
		Number: 7, Title: "Fix login", State: "open", HeadRef: "feat", HeadOwner: "acme", BaseRef: "main",
		URL: "https://forge.test/acme/r/-/merge_requests/7",
	})
	app := flowharness.New(t, flowharness.WithGitHost(forge), flowharness.WithoutGh())
	svc := app.W.GitSvc
	ctx := context.Background()
	mask := flowharness.Mask("sha", "parents", "timestamp", "authorTimeSeconds", "committerDate", "objectId", "id", "at", "createdAt", "anchorSha", "baseSha", "headSha", "localHeadSha", "syncedAt", "updatedAt")
	chk := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}

	repo := app.NewRepo("proj")
	repo.Commit("base", map[string]string{"a.txt": "one\ntwo\nthree\n", "b.txt": "b\n"})
	repo.Git("remote", "add", "origin", "https://forge.test/acme/r.git")
	repo.Git("checkout", "-q", "-b", "feat")
	tip := repo.Commit("feat work", map[string]string{"a.txt": "one\ntwo\nthree\nfour\n"})
	forge.SetHead(7, tip)
	forge.SetAuthor(7, "alice")

	open, err := svc.RepoOpen(ctx, bridge.GitRepoOpenArgs{WindowKey: "w1", Params: gitrpc.RepoOpenParams{Path: repo.Dir}})
	chk(err)
	if open.Kind != "ok" || open.Repo == nil {
		t.Fatalf("RepoOpen = %+v", open)
	}
	id := open.Repo.RepoID

	// Coverage.
	base, err := svc.ReviewResolveBase(ctx, bridge.GitReviewResolveBaseArgs{WindowKey: "w1", Params: gitrpc.ReviewResolveBaseParams{RepoID: id, Branch: "feat"}})
	chk(err)
	if base.Base == nil {
		t.Fatalf("ReviewResolveBase = %+v", base)
	}
	files, err := svc.ReviewFiles(ctx, bridge.GitReviewFilesArgs{WindowKey: "w1", Params: gitrpc.ReviewFilesParams{RepoID: id, Branch: "feat", Base: "main"}})
	chk(err)
	if len(files.Files) != 1 {
		t.Fatalf("ReviewFiles = %+v", files)
	}
	app.Contract(t, scenario, "git:review.files#bound", files, mask)
	diffParams := gitrpc.ReviewFileDiffParams{RepoID: id, Branch: "feat", Base: "main", Path: "a.txt", Mode: "range"}
	diff, err := svc.ReviewFileDiff(ctx, bridge.GitReviewFileDiffArgs{WindowKey: "w1", Params: diffParams})
	chk(err)
	app.Contract(t, scenario, "git:review.fileDiff#bound", diff, mask)
	_, err = svc.ReviewSnapshot(ctx, bridge.GitReviewSnapshotArgs{WindowKey: "w1", Params: gitrpc.ReviewSnapshotParams{RepoID: id, Branch: "feat", Path: "a.txt"}})
	chk(err)
	marked, err := svc.ReviewMark(ctx, bridge.GitReviewMarkArgs{WindowKey: "w1", Params: gitrpc.ReviewMarkParams{RepoID: id, Branch: "feat", Path: "a.txt", Reviewed: true}})
	chk(err)
	if marked.Review.Kind != "full" {
		t.Fatalf("ReviewMark = %+v", marked)
	}
	_, err = svc.ReviewMarkMany(ctx, bridge.GitReviewMarkManyArgs{WindowKey: "w1", Params: gitrpc.ReviewMarkManyParams{RepoID: id, Branch: "feat", Paths: []string{"a.txt"}, Reviewed: false}})
	chk(err)

	rng := gitreview.LineRange{Start: 4, End: 4}
	added, err := svc.ReviewCommentAdd(ctx, bridge.GitReviewCommentAddArgs{WindowKey: "w1", Params: gitrpc.ReviewCommentAddParams{RepoID: id, Branch: "feat", Path: "a.txt", At: tip, Range: rng, Body: "tighten"}})
	chk(err)
	cid := added.Comment.ID
	_, err = svc.ReviewCommentEdit(ctx, bridge.GitReviewCommentEditArgs{WindowKey: "w1", Params: gitrpc.ReviewCommentEditParams{RepoID: id, Branch: "feat", ID: cid, Body: "tighten more", Kind: "change"}})
	chk(err)
	_, err = svc.ReviewCommentSetState(ctx, bridge.GitReviewCommentSetStateArgs{WindowKey: "w1", Params: gitrpc.ReviewCommentSetStateParams{RepoID: id, Branch: "feat", ID: cid, State: "addressed"}})
	chk(err)
	listParams := gitrpc.ReviewCommentListParams{RepoID: id, Branch: "feat"}
	list, err := svc.ReviewCommentList(ctx, bridge.GitReviewCommentListArgs{WindowKey: "w1", Params: listParams})
	chk(err)
	if len(list.Comments) != 1 {
		t.Fatalf("ReviewCommentList = %+v", list)
	}
	app.Contract(t, scenario, "git:review.comment.list#bound", list, mask)
	exportParams := gitrpc.ReviewCommentExportParams{RepoID: id, Branch: "feat"}
	exported, err := svc.ReviewCommentExport(ctx, bridge.GitReviewCommentExportArgs{WindowKey: "w1", Params: exportParams})
	chk(err)
	if exported.Text == "" {
		t.Fatalf("ReviewCommentExport = %+v", exported)
	}
	removed, err := svc.ReviewCommentRemove(ctx, bridge.GitReviewCommentRemoveArgs{WindowKey: "w1", Params: gitrpc.ReviewCommentRemoveParams{RepoID: id, Branch: "feat", ID: cid}})
	chk(err)
	if !removed.Removed {
		t.Fatalf("ReviewCommentRemove = %+v", removed)
	}
	_, err = svc.ReviewCommentAdd(ctx, bridge.GitReviewCommentAddArgs{WindowKey: "w1", Params: gitrpc.ReviewCommentAddParams{RepoID: id, Branch: "feat", Path: "a.txt", At: tip, Range: rng, Body: "again"}})
	chk(err)
	cleared, err := svc.ReviewCommentClear(ctx, bridge.GitReviewCommentClearArgs{WindowKey: "w1", Params: gitrpc.ReviewCommentClearParams{RepoID: id, Branch: "feat"}})
	chk(err)
	if cleared.Removed != 1 {
		t.Fatalf("ReviewCommentClear = %+v", cleared)
	}

	// Change requests against the fake host.
	_, err = svc.BranchSyncChangeRequests(ctx, bridge.GitBranchSyncChangeRequestsArgs{WindowKey: "w1", Params: gitrpc.BranchSyncChangeRequestsParams{RepoID: id, Wait: true}})
	chk(err)
	index, err := svc.BranchChangeRequestIndex(ctx, bridge.GitBranchChangeRequestIndexArgs{WindowKey: "w1", Params: gitrpc.BranchChangeRequestIndexParams{RepoID: id}})
	chk(err)
	app.Contract(t, scenario, "git:branch.changeRequestIndex#bound", index, mask)
	byBranch, err := svc.BranchResolveChangeRequest(ctx, bridge.GitBranchResolveChangeRequestArgs{WindowKey: "w1", Params: gitrpc.BranchResolveChangeRequestParams{RepoID: id, Branch: "feat"}})
	chk(err)
	if byBranch.Kind != "ok" || len(byBranch.ChangeRequests) != 1 {
		t.Fatalf("BranchResolveChangeRequest = %+v", byBranch)
	}
	byCommit, err := svc.CommitResolveChangeRequests(ctx, bridge.GitCommitResolveChangeRequestsArgs{WindowKey: "w1", Params: gitrpc.CommitResolveChangeRequestsParams{RepoID: id, SHA: tip}})
	chk(err)
	if byCommit.Kind != "ok" {
		t.Fatalf("CommitResolveChangeRequests = %+v", byCommit)
	}
	info, err := svc.GitHostInfo(ctx, bridge.GitGitHostInfoArgs{WindowKey: "w1", Params: gitrpc.GitHostInfoParams{RepoID: id}})
	chk(err)
	if info.Host == nil {
		t.Fatalf("GitHostInfo = %+v", info)
	}
	app.Contract(t, scenario, "git:gitHost.info#bound", info)
	url, err := svc.ChangeRequestBrowserUrl(ctx, bridge.GitChangeRequestBrowserUrlArgs{WindowKey: "w1", Params: gitrpc.ChangeRequestBrowserUrlParams{RepoID: id, Number: 7}})
	chk(err)
	if url.URL == nil {
		t.Fatalf("ChangeRequestBrowserUrl = %+v", url)
	}
	stateParams := gitrpc.ChangeRequestReviewStateParams{RepoID: id, Branch: "feat"}
	state, err := svc.ChangeRequestReviewState(ctx, bridge.GitChangeRequestReviewStateArgs{WindowKey: "w1", Params: stateParams})
	chk(err)
	if state.Kind != "ok" {
		t.Fatalf("ChangeRequestReviewState = %+v", state)
	}
	app.Contract(t, scenario, "git:changeRequest.reviewState#bound", state, mask)
	posted, err := svc.ChangeRequestSubmitReview(ctx, bridge.GitChangeRequestSubmitReviewArgs{WindowKey: "w1", Params: gitrpc.ChangeRequestSubmitReviewParams{RepoID: id, Number: 7, Event: githost.ReviewApprove, Body: "LGTM", HeadSha: tip}})
	chk(err)
	if posted.Kind != "ok" {
		t.Fatalf("ChangeRequestSubmitReview = %+v", posted)
	}
	commented, err := svc.ChangeRequestComment(ctx, bridge.GitChangeRequestCommentArgs{WindowKey: "w1", Params: gitrpc.ChangeRequestCommentParams{RepoID: id, Number: 7, Body: "thanks"}})
	chk(err)
	if commented.Kind != "ok" {
		t.Fatalf("ChangeRequestComment = %+v", commented)
	}
	lined, err := svc.ChangeRequestLineComment(ctx, bridge.GitChangeRequestLineCommentArgs{WindowKey: "w1", Params: gitrpc.ChangeRequestLineCommentParams{RepoID: id, Number: 7, Path: "a.txt", Range: rng, HeadSha: tip, Body: "tighten"}})
	chk(err)
	if lined.Kind != "ok" {
		t.Fatalf("ChangeRequestLineComment = %+v", lined)
	}
	if len(forge.Reviews(7)) != 1 || len(forge.Comments(7)) != 1 || len(forge.LineComments(7)) != 1 {
		t.Fatalf("host saw reviews %d, comments %d, line comments %d", len(forge.Reviews(7)), len(forge.Comments(7)), len(forge.LineComments(7)))
	}

	_, err = svc.ReviewSnapshot(ctx, bridge.GitReviewSnapshotArgs{WindowKey: "w1", Params: gitrpc.ReviewSnapshotParams{RepoID: id, Branch: "feat", Path: "a.txt"}})
	chk(err)

	// Refusals.
	_, err = svc.ReviewCommentAdd(ctx, bridge.GitReviewCommentAddArgs{WindowKey: "w1", Params: gitrpc.ReviewCommentAddParams{RepoID: id, Branch: "feat", Path: "a.txt", At: tip, Range: rng, Body: " "}})
	if code, msg := boundCode(t, err); code != string(ipcerr.CodeBadRequest) {
		t.Fatalf("empty comment = %s %s, want E_BAD_REQUEST", code, msg)
	}
	_, err = svc.ReviewFiles(ctx, bridge.GitReviewFilesArgs{WindowKey: "w9", Params: gitrpc.ReviewFilesParams{RepoID: id, Branch: "feat", Base: "main"}})
	if code, _ := boundCode(t, err); code != string(ipcerr.CodeConflict) {
		t.Fatalf("ReviewFiles on an unheld repo = %s, want E_CONFLICT", code)
	}

	// gitHost.providers is the existing GitHostService answer.
	if got := app.W.GitHost.Providers(); len(got.Providers) == 0 {
		t.Fatalf("GitHost.Providers = %+v", got)
	}
	app.Contract(t, scenario, "git:gitHost.providers#bound", app.W.GitHost.Providers())
}
