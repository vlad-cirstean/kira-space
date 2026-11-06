package adeflow_test

import (
	"time"

	"path/filepath"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness/fakehost"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/githost"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitrpc"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// P399. Contract ade-archived: tests/ui/ade-v2-archived.spec.ts reads these.

const archivedBranch = "feat/api-keep"

func TestArchivedTaskKeepsData(t *testing.T) {
	forge := fakehost.New()
	forge.AddChangeRequest(githost.ChangeRequest{
		Number: 7, Title: "Fix login", State: "open", HeadRef: archivedBranch, HeadOwner: "acme", BaseRef: "main",
		URL: "https://forge.test/acme/api/-/merge_requests/7",
	})
	f := newRunFixtureWith(t, []fakeagent.Action{{Name: "done", Sh: rvImplement}}, agentStep("one", ""),
		flowharness.WithGitHost(forge), flowharness.WithoutGh())
	app := f.app
	gitOut(t, f.repo.Dir, "remote", "set-url", "origin", "https://forge.test/acme/api.git")

	notes, est, hostURL := "<p>remember the cache</p>", "3h", "https://forge.test/acme/api/-/merge_requests/9"
	jira := &adewire.Jira{Key: "KIRA-12", URL: "https://jira.test/browse/KIRA-12"}
	if _, err := app.W.AdeTask.UpdateTask(ctx, adewire.UpdateTaskArgs{TaskID: f.taskID, Patch: adewire.TaskPatch{
		Est: &est, HostURL: &hostURL, Jira: jira,
	}}); err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}
	if _, err := app.W.AdeTask.AddNote(ctx, adewire.AddNoteArgs{TaskID: f.taskID, Body: notes}); err != nil {
		t.Fatalf("AddNote: %v", err)
	}
	f.start(t, archivedBranch)
	run := waitRun(t, app, f.taskID, "one", "done")
	br := branchOf(t, board(t, app), f.taskID, f.repoID)
	before := taskOf(t, board(t, app), f.taskID)
	if before.HostLink == nil || before.Jira == nil {
		t.Fatalf("task before archive = %+v, want jira and host link", before)
	}
	runLog := logText(t, app, "run", run.ID)
	if runLog == "" {
		t.Fatal("run wrote no log")
	}

	// A review comment through the review window's git stream; opening the window pins the session.
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
	var added gitrpc.ReviewCommentAddResult
	gs.MustRequest("review.comment.add", gitrpc.ReviewCommentAddParams{
		RepoID: tgt.GitRepoID, Branch: tgt.Branch, Path: "r.txt", At: gitOut(t, br.Worktree, "rev-parse", "HEAD"),
		Range: lineRange(2), Body: "rename this", Kind: "change",
	}, &added)

	if err := app.W.AdeTask.ArchiveTask(ctx, adewire.TaskArgs{TaskID: f.taskID}); err != nil {
		t.Fatalf("ArchiveTask: %v", err)
	}
	// Review sessions are keyed by repo and branch name: archive moves the comments onto the row.
	liveComments := func() int {
		var res struct {
			Comments []struct{ Body string } `json:"comments"`
		}
		stream := app.OpenGitStream()
		stream.MustRequest("repo.open", gitrpc.RepoOpenParams{Path: filepath.Join(app.Work, "api")}, &opened)
		stream.MustRequest("review.comment.list", gitrpc.ReviewCommentListParams{RepoID: tgt.GitRepoID, Branch: tgt.Branch}, &res)
		return len(res.Comments)
	}
	if n := liveComments(); n != 0 {
		t.Fatalf("live comments after archive = %d, want 0", n)
	}
	args := adewire.TaskArgs{TaskID: f.taskID}
	app.Contract(t, "ade-archived", "args:AdeTaskService.ArchivedTask", args)
	app.Contract(t, "ade-archived", "args:AdeTaskService.RestoreTask", args)

	got, err := app.W.AdeTask.ArchivedTask(ctx, args)
	if err != nil {
		t.Fatalf("ArchivedTask: %v", err)
	}
	wt := got.Task
	if wt.Notes != notes || wt.Est != est || wt.Jira == nil || *wt.Jira != *jira || wt.HostLink == nil || *wt.HostLink != *before.HostLink ||
		wt.StatusID != before.StatusID || wt.Title != before.Title {
		t.Fatalf("archived task = %+v, want the pre-archive notes, jira, link, estimate and status of %+v", wt, before)
	}
	if len(got.Branches) != 1 || len(got.Statuses) == 0 || len(got.Repos) != 1 {
		t.Fatalf("archived branches %d, statuses %d, repos %d", len(got.Branches), len(got.Statuses), len(got.Repos))
	}
	ab := got.Branches[0]
	if ab.Name != archivedBranch || ab.Removed || ab.ChangeRequest == nil || ab.ChangeRequest.Number != 7 || ab.ChangeRequest.Title != "Fix login" ||
		ab.PRSource != "live" || len(ab.Comments) != 1 || ab.Comments[0].Body != "rename this" || ab.Comments[0].Path != "r.txt" {
		t.Fatalf("archived branch = %+v, want the named branch, live PR #7 and the review comment", ab)
	}
	if len(wt.Runs) != 1 || wt.Runs[0].ID != run.ID {
		t.Fatalf("archived runs = %+v, want %s", wt.Runs, run.ID)
	}
	app.Contract(t, "ade-archived", "AdeTaskService.ArchivedTask#live", got, flowharness.Mask(
		"id", "taskId", "branchId", "codeRepoId", "runId", "sessionId", "createdAt", "addedAt", "archivedAt", "startedAt", "finishedAt",
		"exitCode", "since", "color", "mergedAt"))

	// The host stops answering after a restart (cold caches): the snapshot taken at archive serves.
	app.Restart()
	forge.SetOffline(true)
	snap, err := app.W.AdeTask.ArchivedTask(ctx, args)
	if err != nil {
		t.Fatalf("ArchivedTask offline: %v", err)
	}
	sb := snap.Branches[0]
	if sb.ChangeRequest == nil || sb.ChangeRequest.Number != 7 || sb.PRSource != "archived" || len(sb.Comments) != 1 {
		t.Fatalf("offline archived branch = %+v, want the archive-time PR #7 and the comment", sb)
	}
	app.Contract(t, "ade-archived", "AdeTaskService.ArchivedTask#snapshot", snap, flowharness.Mask(
		"id", "taskId", "branchId", "codeRepoId", "runId", "sessionId", "createdAt", "addedAt", "archivedAt", "startedAt", "finishedAt",
		"exitCode", "since", "color", "mergedAt"))

	// Nothing is purged with time: run log and review comments stay.
	if again := logText(t, app, "run", run.ID); again != runLog {
		t.Fatalf("run log after archive = %q, want %q", again, runLog)
	}

	restored, err := app.W.AdeTask.RestoreTask(ctx, args)
	if err != nil || restored.ID != f.taskID || len(restored.BranchIDs) != 1 {
		t.Fatalf("RestoreTask = %+v, %v", restored, err)
	}
	b := board(t, app)
	if len(b.History) != 0 || len(b.Plan.Order) != 1 || b.Plan.Order[0] != f.taskID || b.Plan.Day[f.taskID] != "" {
		t.Fatalf("board after restore: history %+v, plan %+v, want the task in Later", b.History, b.Plan)
	}
	rb := branchOf(t, b, f.taskID, f.repoID)
	if rb.Name != archivedBranch || rb.Worktree != "" || rb.Setup != nil {
		t.Fatalf("restored branch = %+v, want the name, no worktree and no stale setup", rb)
	}
	if _, err := app.W.AdeTask.ArchivedTask(ctx, args); errCode(err) != "E_BAD_REQUEST" {
		t.Fatalf("ArchivedTask of a live task = %v, want E_BAD_REQUEST", err)
	}

	if n := liveComments(); n != 1 {
		t.Fatalf("live comments after restore = %d, want the 1 archived comment back", n)
	}

	// Refusal: the branch name is live on another task.
	if err := app.W.AdeTask.ArchiveTask(ctx, args); err != nil {
		t.Fatalf("second archive: %v", err)
	}
	other := createTask(t, app, "Other", "flow", f.repoID)
	row := branchOf(t, board(t, app), other.ID, f.repoID)
	if _, err := app.W.AdeTask.SetTaskBranch(ctx, adewire.SetTaskBranchArgs{BranchID: row.ID, Name: archivedBranch, Existing: true}); err != nil {
		t.Fatalf("SetTaskBranch: %v", err)
	}
	// The other task reuses the name: it starts a fresh session and the archived view keeps its own comment.
	if n := liveComments(); n != 0 {
		t.Fatalf("live comments on the reused name = %d, want 0", n)
	}
	if again, err := app.W.AdeTask.ArchivedTask(ctx, args); err != nil || len(again.Branches[0].Comments) != 1 {
		t.Fatalf("archived comments after reuse = %+v, %v, want the 1 snapshot", again.Branches, err)
	}
	_, err = app.W.AdeTask.RestoreTask(ctx, args)
	if errCode(err) != "E_BAD_REQUEST" || !strings.Contains(err.Error(), archivedBranch) {
		t.Fatalf("RestoreTask over a live name = %v, want E_BAD_REQUEST naming %s", err, archivedBranch)
	}
	app.Contract(t, "ade-archived", "error:AdeTaskService.RestoreTask#branch-live", err.Error())
	if _, err := app.W.AdeTask.ArchivedTask(ctx, args); err != nil {
		t.Fatalf("task left the archive after a refused restore: %v", err)
	}
}

// Resume of an archived task's session: Restore brings the task back, then Take over continues the
// conversation in a recreated worktree. Contract ade-archived: tests/ui/ade-v2-archived.spec.ts.
func TestResumeArchivedSession(t *testing.T) {
	f := newRunFixtureWith(t, []fakeagent.Action{{Name: "done"}}, agentStep("one", ""),
		flowharness.WithTrackerGrace(time.Second))
	app := f.app
	f.start(t, archivedBranch)
	run := waitRun(t, app, f.taskID, "one", "done")
	if run.SessionID == "" {
		t.Fatalf("run %+v has no session", run)
	}
	args := adewire.TaskArgs{TaskID: f.taskID}
	if err := app.W.AdeTask.ArchiveTask(ctx, args); err != nil {
		t.Fatal(err)
	}
	if _, err := app.W.AdeTask.TakeOver(ctx, adewire.TakeOverArgs{SessionID: run.SessionID}); err == nil {
		t.Fatal("TakeOver of an archived task's session succeeded")
	}
	if _, err := app.W.AdeTask.RestoreTask(ctx, args); err != nil {
		t.Fatal(err)
	}
	takeArgs := adewire.TakeOverArgs{SessionID: run.SessionID, StopIfRunning: true}
	app.Contract(t, "ade-archived", "args:AdeTaskService.TakeOver", takeArgs)
	var launch adewire.Launch
	testx.WaitUntil(t, waitFor, func() bool {
		l, err := app.W.AdeTask.TakeOver(ctx, takeArgs)
		launch = l
		return err == nil
	})
	app.Contract(t, "ade-archived", "AdeTaskService.TakeOver#archived-resume", launch, flowharness.Mask("sessionId", "terminalId", "cwd", "command"))
	br := branchOf(t, board(t, app), f.taskID, f.repoID)
	if br.Worktree == "" || launch.Cwd != br.Worktree || !strings.Contains(launch.Command, "--resume") {
		t.Fatalf("resume launch = %+v, want a --resume in the restored worktree %q", launch, br.Worktree)
	}
	if got := sessionByID(t, app, run.SessionID); got.TaskID != f.taskID || got.Cwd != br.Worktree {
		t.Fatalf("session after restore = %+v, want task %s in %q", got, f.taskID, br.Worktree)
	}
}
