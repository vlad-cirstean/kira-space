package journeyflow_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness/fakehost"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/githost"
	"github.com/kirathecat/kira-studio/internal/agentmcp"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// adeBranchRun is the newest run of the branch with purpose ("" = a step run).
func adeBranchRun(t *testing.T, app *flowharness.App, branchID, purpose string) (adewire.Run, bool) {
	t.Helper()
	var best adewire.Run
	var found bool
	for _, tk := range board(t, app).Tasks {
		for _, r := range tk.Runs {
			if r.BranchID == branchID && r.Purpose == purpose && (!found || r.Attempt >= best.Attempt) {
				best, found = r, true
			}
		}
	}
	return best, found
}

func waitBranchRun(t *testing.T, app *flowharness.App, branchID, purpose string, states ...string) adewire.Run {
	t.Helper()
	var got adewire.Run
	testx.WaitUntil(t, waitFor, func() bool {
		r, ok := adeBranchRun(t, app, branchID, purpose)
		if !ok {
			return false
		}
		got = r
		for _, s := range states {
			if r.State == s {
				return true
			}
		}
		return false
	})
	return got
}

// J-ADE-071 (J-ADE-010, J-ADE-040). One task on two repos goes from idea to merged and archived with
// one failure injected per stage: an agent that fails, a restart, a push the remote refuses, a rebase
// conflict and a rate-limited host. Each recovers through the product's own path; at the end both
// remotes hold the work on main, the task is archived, its worktrees are gone and nothing still runs.
func TestADETaskEveryFailure(t *testing.T) {
	forge := fakehost.New()
	app := flowharness.New(t, flowharness.WithGitHost(forge), flowharness.WithoutGh(), flowharness.WithTrackerGrace(time.Second))
	apiRepo, apiBare := cloneWithMain(app, filepath.Join(app.Work, "api"))
	webRepo, webBare := cloneWithMain(app, filepath.Join(app.Work, "web"))
	commit := func(file string) string {
		return "echo " + file + " > " + file + " && git add -A && git commit -q -m " + file
	}
	finish := fakeagent.MCPCall{Server: agentmcp.ServerName, Tool: "finish_step", Args: map[string]any{"status": "done"}}
	// api calls: 1 the step (fails), 2 its retry, 3 the conflicting rebase, 4 the rebase that resolves it.
	app.Scenario(fakeagent.Scenario{Claude: map[string][]fakeagent.Action{
		"api": {
			{Name: "failed"},
			{Name: "done", Sh: commit("api.txt")},
			{Name: "nofinish", Sh: "git fetch -q origin && git rebase -q origin/main || true"},
			{Name: "nofinish", Sh: "git fetch -q origin && git rebase -q -X theirs origin/main", MCP: []fakeagent.MCPCall{finish}},
		},
		"web": {{Name: "done", Sh: commit("web.txt")}},
	}})
	api, web := importRepo(t, app, apiRepo.Dir), importRepo(t, app, webRepo.Dir)
	saveWorkflow(t, app, workflowYAML)
	task, err := app.W.AdeTask.CreateTask(ctx, adewire.CreateTaskArgs{Title: "Ship it", CodeRepoIDs: []string{api.ID, web.ID}, WorkflowID: "flow"})
	if err != nil {
		t.Fatal(err)
	}
	apiBr, webBr := branchOf(t, board(t, app), task.ID, api.ID), branchOf(t, board(t, app), task.ID, web.ID)
	if _, err := app.W.AdeTask.StartRun(ctx, adewire.StartRunArgs{TaskID: task.ID, BranchNames: map[string]string{apiBr.ID: "feat/api-j", webBr.ID: "feat/web-j"}}); err != nil {
		t.Fatal(err)
	}

	// 1. The agent fails on api; web is unaffected. A retry finishes it.
	failed := waitBranchRun(t, app, apiBr.ID, "", "failed")
	waitBranchRun(t, app, webBr.ID, "", "done")
	if err := app.W.AdeTask.RetryRun(ctx, adewire.RunArgs{RunID: failed.ID}); err != nil {
		t.Fatalf("RetryRun: %v", err)
	}
	if retried := waitBranchRun(t, app, apiBr.ID, "", "done"); retried.Attempt <= failed.Attempt {
		t.Fatalf("retry attempt %d, was %d", retried.Attempt, failed.Attempt)
	}

	// 2. A restart between stages loses nothing: both runs stay done, both worktrees stay.
	app.Restart()
	apiBr, webBr = branchOf(t, board(t, app), task.ID, api.ID), branchOf(t, board(t, app), task.ID, web.ID)
	for _, br := range []adewire.Branch{apiBr, webBr} {
		if r, ok := adeBranchRun(t, app, br.ID, ""); !ok || r.State != "done" || !exists(br.Worktree) {
			t.Fatalf("after restart %s: run %+v worktree %s, want done and present", br.Name, r, br.Worktree)
		}
	}

	// 3. A teammate pushed web's branch while it was amended here: the push is refused and keeps the
	// teammate's commit; integrating their tip lets it through.
	// The branch is published from the terminal first (it tracks main until then, which a force push refuses).
	gitOut(t, webBr.Worktree, "push", "-q", "-u", "origin", "feat/web-j")
	gitOut(t, webBr.Worktree, "commit", "-q", "--amend", "-m", "web amended")
	mate := filepath.Join(app.Work, "web-mate")
	gitOut(t, app.Work, "clone", "-q", webBare.Dir, mate)
	gitOut(t, mate, "checkout", "-q", "feat/web-j")
	if err := os.WriteFile(filepath.Join(mate, "mate.txt"), []byte("mate\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitOut(t, mate, "add", "-A")
	gitOut(t, mate, "commit", "-q", "-m", "mate")
	gitOut(t, mate, "push", "-q", "origin", "feat/web-j")
	mateTip := gitOut(t, mate, "rev-parse", "HEAD")
	res, err := app.W.AdeTask.ForcePush(ctx, adewire.BranchArgs{BranchID: webBr.ID})
	if err != nil || res.Error == nil {
		t.Fatalf("ForcePush over a moved remote = %+v, %v, want a refusal", res, err)
	}
	if got := webBare.Git("rev-parse", "feat/web-j"); got != mateTip {
		t.Fatalf("remote web branch = %s, want the teammate's %s", got, mateTip)
	}
	gitOut(t, webBr.Worktree, "fetch", "-q", "origin")
	gitOut(t, webBr.Worktree, "rebase", "-q", "origin/feat/web-j")
	if res, err := app.W.AdeTask.ForcePush(ctx, adewire.BranchArgs{BranchID: webBr.ID}); err != nil || res.Error != nil {
		t.Fatalf("ForcePush after integrating = %+v %+v, %v", res, res.Error, err)
	}
	webTip := gitOut(t, webBr.Worktree, "rev-parse", "HEAD")
	if got := webBare.Git("rev-parse", "feat/web-j"); got != webTip {
		t.Fatalf("remote web branch = %s, want the pushed %s", got, webTip)
	}
	gitOut(t, webBr.Worktree, "push", "-q", "origin", "HEAD:main")

	// 4. Main moves under api with a clashing change: the rebase run conflicts and fails, the abort
	// restores the tip, and a second run resolves it.
	apiRepo.Checkout("main")
	apiRepo.Commit("main api", map[string]string{"api.txt": "from main\n"})
	apiRepo.Git("push", "-q", "origin", "main")
	before := gitOut(t, apiBr.Worktree, "rev-parse", "HEAD")
	testx.WaitUntil(t, waitFor, func() bool {
		pv, err := app.W.AdeTask.RebasePreview(ctx, adewire.OntoArgs{BranchID: apiBr.ID})
		return err == nil && !pv.NoOp
	})
	start, err := app.W.AdeTask.Rebase(ctx, adewire.RebaseArgs{BranchID: apiBr.ID})
	if err != nil {
		t.Fatalf("Rebase: %v", err)
	}
	if r := waitBranchRun(t, app, apiBr.ID, "rebase", "failed"); r.ID != start.RunID {
		t.Fatalf("rebase run = %+v, want %s failed on the conflict", r, start.RunID)
	}
	if err := app.W.AdeTask.AbortRebase(ctx, adewire.BranchArgs{BranchID: apiBr.ID}); err != nil {
		t.Fatalf("AbortRebase: %v", err)
	}
	if got := gitOut(t, apiBr.Worktree, "rev-parse", "HEAD"); got != before {
		t.Fatalf("api tip after abort = %s, want %s", got, before)
	}
	again, err := app.W.AdeTask.Rebase(ctx, adewire.RebaseArgs{BranchID: apiBr.ID})
	if err != nil {
		t.Fatalf("Rebase retry: %v", err)
	}
	if r := waitBranchRun(t, app, apiBr.ID, "rebase", "done"); r.ID != again.RunID {
		t.Fatalf("retried rebase run = %+v, want %s done", r, again.RunID)
	}
	gitOut(t, apiBr.Worktree, "push", "-q", "origin", "HEAD:main")
	gitOut(t, apiBr.Worktree, "push", "-q", "origin", "HEAD:refs/heads/feat/api-j")

	// 5. The host is rate limited: a change request read never errors, the board and the task stay usable,
	// and the request shows once the limit lifts.
	apiRepo.Git("remote", "set-url", "origin", "https://forge.test/acme/api.git")
	forge.AddChangeRequest(githost.ChangeRequest{Number: 3, Title: "Ship it", State: "open", HeadRef: "feat/api-j", HeadOwner: "acme", BaseRef: "main", URL: "https://forge.test/acme/api/-/merge_requests/3"})
	forge.SetRateLimited(time.Now().Add(time.Hour))
	if _, err := app.W.AdeTask.ChangeRequests(ctx); err != nil {
		t.Fatalf("limited ChangeRequests = %v, want a soft answer, not an error", err)
	}
	if taskOf(t, board(t, app), task.ID).ID == "" {
		t.Fatal("board lost the task while the host was limited")
	}
	forge.SetRateLimited(time.Time{})
	testx.WaitUntil(t, waitFor, func() bool {
		prs, err := app.W.AdeTask.ChangeRequests(ctx)
		return err == nil && prs.Branches[apiBr.ID].Number == 3
	})

	// Both remotes hold the work on main.
	for name, w := range map[string]struct {
		bare *flowharness.Bare
		tip  string
	}{"api": {apiBare, gitOut(t, apiBr.Worktree, "rev-parse", "HEAD")}, "web": {webBare, webTip}} {
		if out := w.bare.Git("merge-base", "--is-ancestor", w.tip, "main"); strings.Contains(out, "fatal") {
			t.Fatalf("%s: tip %s not on the remote main: %s", name, w.tip, out)
		}
	}

	// Archive: worktrees go, the task moves to history, nothing keeps running.
	if err := app.W.AdeTask.ArchiveTask(ctx, adewire.ArchiveTaskArgs{TaskID: task.ID}); err != nil {
		t.Fatalf("ArchiveTask: %v", err)
	}
	for _, br := range []adewire.Branch{apiBr, webBr} {
		if exists(br.Worktree) {
			t.Fatalf("worktree %s survived the archive", br.Worktree)
		}
	}
	b := board(t, app)
	if len(b.History) != 1 || b.History[0].TaskID != task.ID {
		t.Fatalf("history = %+v, want the archived task", b.History)
	}
	for _, tk := range b.Tasks {
		for _, r := range tk.Runs {
			if r.State == "running" || r.State == "pending" {
				t.Fatalf("run %+v still live after the archive", r)
			}
		}
	}
	sessions, err := app.W.AdeTask.Sessions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range sessions.Sessions {
		if s.State == "running" {
			t.Fatalf("session %+v still running after the archive", s)
		}
	}
}
