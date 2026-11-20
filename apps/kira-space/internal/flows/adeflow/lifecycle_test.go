package adeflow_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// doneFixture runs the task's single step to done, so its worktree exists.
func doneFixture(t *testing.T, branch string) (*runFixture, adewire.Branch) {
	t.Helper()
	f := newRunFixture(t, []fakeagent.Action{{Name: "done"}}, agentStep("one", ""))
	f.start(t, branch)
	waitRun(t, f.app, f.taskID, "one", "done")
	return f, branchOf(t, board(t, f.app), f.taskID, f.repoID)
}

func TestArchiveRisk(t *testing.T) {
	f, br := doneFixture(t, "feat/api-risk")
	app := f.app
	risk, err := app.W.AdeTask.ArchiveRisk(ctx, adewire.TaskArgs{TaskID: f.taskID})
	if err != nil || len(risk.Branches) != 1 || risk.Branches[0].Unmerged != 0 || len(risk.Branches[0].Dirty) != 0 {
		t.Fatalf("clean worktree risk = %+v, %v, want nothing at risk", risk, err)
	}
	// Contract ade-task: tests/ui/ade-v2-archive.spec.ts "contract: ..." reads these.
	app.Contract(t, "ade-task", "AdeTaskService.ArchiveRisk#clean", risk)

	commitIn(t, br.Worktree, "unpushed.txt")
	if err := os.WriteFile(filepath.Join(br.Worktree, "wip.txt"), []byte("wip\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	risk, err = app.W.AdeTask.ArchiveRisk(ctx, adewire.TaskArgs{TaskID: f.taskID})
	if err != nil {
		t.Fatal(err)
	}
	app.Contract(t, "ade-task", "AdeTaskService.ArchiveRisk#at-risk", risk, flowharness.Mask("dirtyToken"))
	app.Contract(t, "ade-task", "args:AdeTaskService.ArchiveTask", adewire.ArchiveTaskArgs{TaskID: f.taskID, DiscardTokens: map[string]string{}})
	got := risk.Branches[0]
	if got.DirtyToken == "" {
		t.Fatal("at-risk branch has no dirtyToken")
	}
	if got.BranchID != br.ID || got.Worktree != br.Worktree || got.Unmerged != 1 || len(got.Dirty) != 1 || got.Dirty[0].Path != "wip.txt" {
		t.Fatalf("risk = %+v, want 1 unmerged commit and dirty wip.txt in %s", got, br.Worktree)
	}

	opened, err := app.W.AdeTask.OpenReviewWindow(ctx, adewire.BranchArgs{BranchID: br.ID})
	if err != nil || !opened {
		t.Fatalf("OpenReviewWindow = %v, %v", opened, err)
	}
	key := app.WindowMgr.Opened[len(app.WindowMgr.Opened)-1].Key

	liveTitle := taskOf(t, board(t, app), f.taskID).Title
	if err := app.W.AdeTask.ArchiveTask(ctx, adewire.ArchiveTaskArgs{TaskID: f.taskID, DiscardTokens: map[string]string{br.ID: got.DirtyToken}}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(br.Worktree); !os.IsNotExist(err) {
		t.Fatalf("worktree %s survives the archive: %v", br.Worktree, err)
	}
	if out := gitOut(t, f.repo.Dir, "branch", "--list", "feat/api-risk"); out == "" {
		t.Fatal("archive deleted the branch")
	}
	b := board(t, app)
	if len(b.Tasks) != 0 || len(b.History) != 1 || b.History[0].TaskID != f.taskID {
		t.Fatalf("board after archive: tasks %d, history %+v", len(b.Tasks), b.History)
	}
	if got := b.History[0].Title; got != liveTitle {
		t.Fatalf("history title = %q, want the live title %q", got, liveTitle)
	}
	if tgt, err := app.W.AdeTask.ReviewWindowTarget(ctx, adewire.WindowKeyArgs{WindowKey: key}); err != nil || tgt != nil {
		t.Fatalf("review target after archive = %+v, %v, want nil", tgt, err)
	}
	if closed := app.WindowMgr.Closed; !slices.Contains(closed, key) {
		t.Fatalf("window manager closed %v, want the review window %s", closed, key)
	}
}

func TestReviewWindowAndAgent(t *testing.T) {
	f, br := doneFixture(t, "feat/api-review")
	app := f.app
	opened, err := app.W.AdeTask.OpenReviewWindow(ctx, adewire.BranchArgs{BranchID: br.ID})
	if err != nil || !opened {
		t.Fatalf("OpenReviewWindow = %v, %v, want a new window", opened, err)
	}
	rec := app.WindowMgr.Opened[len(app.WindowMgr.Opened)-1]
	if title := app.WindowMgr.Titles[rec.Key]; title != "Review · api · feat/api-review" {
		t.Fatalf("window title = %q", title)
	}
	if again, err := app.W.AdeTask.OpenReviewWindow(ctx, adewire.BranchArgs{BranchID: br.ID}); err != nil || again {
		t.Fatalf("second OpenReviewWindow = %v, %v, want a focus of the open window", again, err)
	}
	if got := app.WindowMgr.Focused; len(got) == 0 || got[len(got)-1] != rec.Key {
		t.Fatalf("focused %v, want %s", got, rec.Key)
	}
	if n := len(app.WindowMgr.Opened); n != 1 {
		t.Fatalf("windows opened = %d after the second press, want 1", n)
	}
	app.Contract(t, "ade-review-open", "AdeTaskService.OpenReviewWindow#existing", false)
	tgt, err := app.W.AdeTask.ReviewWindowTarget(ctx, adewire.WindowKeyArgs{WindowKey: rec.Key})
	if err != nil || tgt == nil {
		t.Fatalf("ReviewWindowTarget = %+v, %v", tgt, err)
	}
	if tgt.TaskID != f.taskID || tgt.BranchID != br.ID || tgt.Branch != "feat/api-review" || tgt.Base != "origin/main" && tgt.Base != "main" ||
		tgt.Worktree != br.Worktree {
		t.Fatalf("review target = %+v", tgt)
	}
	if other, err := app.W.AdeTask.ReviewWindowTarget(ctx, adewire.WindowKeyArgs{WindowKey: "not-a-review-window"}); err != nil || other != nil {
		t.Fatalf("target of a plain window = %+v, %v, want nil", other, err)
	}

	if st, err := app.W.AdeTask.ReviewAgent(ctx, adewire.TaskArgs{TaskID: f.taskID}); err != nil || st.Session != nil {
		t.Fatalf("review agent before launch = %+v, %v, want none", st, err)
	}
	launch, err := app.W.AdeTask.LaunchReviewAgent(ctx, adewire.TaskArgs{TaskID: f.taskID})
	if err != nil || launch.Resumed {
		t.Fatalf("LaunchReviewAgent = %+v, %v, want a new conversation", launch, err)
	}
	if launch.Launch.Cwd != br.Worktree {
		t.Fatalf("review agent cwd = %q, want %q", launch.Launch.Cwd, br.Worktree)
	}
	claude(app, map[string][]fakeagent.Action{"*": {{Name: "done"}, {WaitFile: filepath.Join(t.TempDir(), "never")}}})
	app.WindowMgr.OpenWindow(rec)
	openLaunch(t, app, launch.Launch)
	sess := waitSession(t, app, launch.Launch.SessionID, "running")
	if sess.Purpose != "review" || sess.Cwd != br.Worktree {
		t.Fatalf("review session = %+v", sess)
	}
	st, err := app.W.AdeTask.ReviewAgent(ctx, adewire.TaskArgs{TaskID: f.taskID})
	if err != nil || st.Session == nil || st.Session.ID != sess.ID || st.HostWindowKey != sessionWindow {
		t.Fatalf("running review agent = %+v, %v, want session %s hosted by %s", st, err, sess.ID, sessionWindow)
	}
	if _, err := app.W.AdeTask.LaunchReviewAgent(ctx, adewire.TaskArgs{TaskID: f.taskID}); err == nil {
		t.Fatal("second review agent launched while one runs")
	}
}

func TestRestartRecovery(t *testing.T) {
	hold := filepath.Join(t.TempDir(), "hold")
	f := newRunFixture(t, []fakeagent.Action{{WaitFile: hold}, {Name: "done"}}, agentStep("one", ""))
	app := f.app
	f.start(t, "feat/api-restart")
	run := waitRun(t, app, f.taskID, "one", "running")
	br := branchOf(t, board(t, app), f.taskID, f.repoID)
	if run.SessionID == "" || br.Worktree == "" {
		t.Fatalf("run %+v on branch %+v is not underway", run, br)
	}

	testx.WaitUntil(t, waitFor, func() bool { _, err := os.Stat(filepath.Join(app.FakeDir, "api-1.args")); return err == nil })

	app.Restart()
	if err := os.WriteFile(hold, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	after, ok := latestRun(t, app, f.taskID, "one")
	if !ok || after.State == "running" {
		t.Fatalf("run after restart = %+v, want it no longer running", after)
	}
	res, err := app.W.AdeTask.Sessions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range res.Sessions {
		if s.State == "running" {
			t.Fatalf("session %+v still running after restart", s)
		}
	}
	if _, err := os.Stat(br.Worktree); err != nil {
		t.Fatalf("worktree lost across restart: %v", err)
	}
	if gitOut(t, br.Worktree, "rev-parse", "--abbrev-ref", "HEAD") != "feat/api-restart" {
		t.Fatal("worktree left its branch")
	}

	if err := app.W.AdeTask.RetryRun(ctx, adewire.RunArgs{RunID: after.ID}); err != nil {
		t.Fatalf("RetryRun after restart: %v", err)
	}
	retried := waitRun(t, app, f.taskID, "one", "done")
	if retried.Attempt <= after.Attempt {
		t.Fatalf("retry reused attempt %d, was %d", retried.Attempt, after.Attempt)
	}
}

// githubClone is a clone whose origin is a github.com URL, so the board treats it as a GitHub repo.
func githubClone(t *testing.T, app *flowharness.App) (adewire.Branch, string) {
	t.Helper()
	repo, _ := cloneWithMain(app, filepath.Join(app.Work, "api"))
	gitOut(t, repo.Dir, "remote", "set-url", "origin", "https://github.com/acme/api.git")
	rec := importRepo(t, app, repo.Dir)
	saveWorkflow(t, app, "flow", flowYAML("flow", agentStep("one", "")))
	task := createTask(t, app, "Fix login", "flow", rec.ID)
	br := branchOf(t, board(t, app), task.ID, rec.ID)
	if _, err := app.W.AdeTask.StartRun(ctx, adewire.StartRunArgs{TaskID: task.ID, BranchNames: map[string]string{br.ID: "feat/api-gh"}}); err != nil {
		t.Fatal(err)
	}
	waitRun(t, app, task.ID, "one", "done")
	return branchOf(t, board(t, app), task.ID, rec.ID), rec.ID
}

func TestGitHubWithoutGh(t *testing.T) {
	app := flowharness.New(t, flowharness.WithoutGh())
	claude(app, map[string][]fakeagent.Action{"*": {{Name: "done"}}})
	_, repoID := githubClone(t, app)

	var prs adewire.ChangeRequestsResult
	testx.WaitUntil(t, waitFor, func() bool {
		var err error
		prs, err = app.W.AdeTask.ChangeRequests(ctx)
		return err == nil && len(prs.Repos) == 1 && prs.Repos[0].CodeRepoID == repoID && prs.Repos[0].Kind != "ok"
	})
	if prs.Repos[0].Kind != "unavailable" || len(prs.Branches) != 0 {
		t.Fatalf("Prs = %+v, want unavailable and no PRs", prs)
	}
}

func TestGitHubChangeRequestsWithFakeGh(t *testing.T) {
	flowharness.Complete(t)
	app := flowharness.New(t)
	gh := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(gh, name)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	pulls := write("pulls.json", `[{"number":7,"title":"Fix login","html_url":"https://github.com/acme/api/pull/7","state":"open","draft":false,`+
		`"merged_at":null,"head":{"ref":"feat/api-gh","sha":"x","repo":{"owner":{"login":"acme"}}},"base":{"ref":"main"},"updated_at":"2026-03-01T00:00:00Z"}]`)
	app.Scenario(fakeagent.Scenario{
		Claude: map[string][]fakeagent.Action{"*": {{Name: "done"}}},
		Gh: []fakeagent.GhRule{
			{Match: "--version", Emit: write("version.txt", "gh version 2.42.0 (2024-01-08)\n")},
			{Match: "auth status", Emit: write("auth.txt", "github.com\n  Logged in to github.com account octocat (keyring)\n")},
			{Match: "/pulls?", Emit: pulls},
		},
	})
	br, repoID := githubClone(t, app)

	prs, err := app.W.AdeTask.ChangeRequests(ctx)
	if err != nil || len(prs.Repos) != 1 || prs.Repos[0].CodeRepoID != repoID || prs.Repos[0].Kind != "ok" {
		t.Fatalf("Prs = %+v, %v, want ok", prs, err)
	}
	if pr := prs.Branches[br.ID]; pr.Number != 7 || pr.State != "open" || pr.URL != "https://github.com/acme/api/pull/7" {
		t.Fatalf("branch PR = %+v, want #7 open", pr)
	}
}
