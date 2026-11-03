package adeflow_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/oplog"
	"github.com/kirathecat/kira-studio/internal/agentmcp"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
	"github.com/kirathecat/kira-studio/internal/shell"
	"github.com/kirathecat/kira-studio/internal/testx"
)

type rbFx struct {
	app    *flowharness.App
	repo   *flowharness.Repo
	repoID string
}

// newRbFx is an app with clone "api" holding feat/a (pushed, one commit off main) as a task branch
// with a worktree, and a main that moved on since.
func newRbFx(t *testing.T, opts ...flowharness.Opt) (*rbFx, adewire.Task, adewire.Branch) {
	t.Helper()
	app := flowharness.New(t, opts...)
	repo, _ := cloneWithMain(app, filepath.Join(app.Work, "api"))
	f := &rbFx{app: app, repo: repo, repoID: importRepo(t, app, repo.Dir).ID}
	repo.Git("checkout", "-q", "-b", "feat/a", "main")
	repo.Commit("work a", map[string]string{"a.txt": "a\n"})
	repo.Git("push", "-q", "origin", "feat/a")
	repo.Checkout("main")
	saveWorkflow(t, app, "flow", flowYAML("flow", agentStep("one", "")))
	added, err := app.W.AdeTask.AddExistingBranch(ctx, adewire.AddExistingBranchArgs{CodeRepoID: f.repoID, Name: "feat/a"})
	if err != nil {
		t.Fatalf("AddExistingBranch: %v", err)
	}
	f.advanceMain(t, "m1.txt", "m1\n")
	return f, added.Task, branchOf(t, board(t, app), added.Task.ID, f.repoID)
}

func (f *rbFx) advanceMain(t *testing.T, file, body string) {
	t.Helper()
	f.repo.Checkout("main")
	f.repo.Commit("main "+file, map[string]string{file: body})
	f.repo.Git("push", "-q", "origin", "main")
}

// waitBehind waits until the app's cached refs show main ahead of the branch; the watcher lags a push.
func waitBehind(t *testing.T, app *flowharness.App, br adewire.Branch) {
	t.Helper()
	testx.WaitUntil(t, waitFor, func() bool {
		pv, err := app.W.AdeTask.RebasePreview(ctx, adewire.OntoArgs{BranchID: br.ID})
		return err == nil && !pv.NoOp
	})
}

// waitBoardBehind waits until the board's cached refs show the branch n commits behind its base.
func (f *rbFx) waitBoardBehind(t *testing.T, taskID string, n int) {
	t.Helper()
	testx.WaitUntil(t, waitFor, func() bool { return f.branch(t, taskID).Behind == n })
}

// agent scripts the next rebase attempt: sh runs in the root branch worktree, then finish is called.
func (f *rbFx) agent(sh string, finish map[string]any) {
	a := fakeagent.Action{Name: "nofinish", Sh: sh}
	if finish != nil {
		a.MCP = []fakeagent.MCPCall{{Server: agentmcp.ServerName, Tool: "finish_step", Args: finish}}
	}
	claude(f.app, map[string][]fakeagent.Action{"*": {a}})
}

// rebaseMask hides what changes per run: times, the agent session and commit shas.
var rebaseMask = flowharness.Mask("startedAt", "finishedAt", "sessionId", "before", "after", "ontoTip", "exitCode", "tip", "createdAt", "sha", "addedAt", "lastCommitAt")

const rebaseSh = "git fetch -q origin && git rebase -q origin/main"

func done() map[string]any { return map[string]any{"status": "done", "summary": "rebased"} }

func (f *rbFx) rebase(t *testing.T, br adewire.Branch, args adewire.RebaseArgs) adewire.RebaseStart {
	t.Helper()
	args.BranchID = br.ID
	res, err := f.app.W.AdeTask.Rebase(ctx, args)
	if err != nil {
		t.Fatalf("Rebase: %v", err)
	}
	return res
}

// waitRebase waits for the run to leave pending/running and returns it.
func (f *rbFx) waitRebase(t *testing.T, taskID, runID string) adewire.Run {
	t.Helper()
	var got adewire.Run
	testx.WaitUntil(t, waitFor, func() bool {
		for _, r := range taskOf(t, board(t, f.app), taskID).Runs {
			if r.ID == runID {
				got = r
				return r.State != "pending" && r.State != "running"
			}
		}
		return false
	})
	return got
}

func (f *rbFx) branch(t *testing.T, taskID string) adewire.Branch {
	t.Helper()
	return branchOf(t, board(t, f.app), taskID, f.repoID)
}

func TestRebaseRun(t *testing.T) {
	t.Run("clean rebase is verified against git and clears the pending base", func(t *testing.T) {
		f, task, br := newRbFx(t)
		before := gitOut(t, br.Worktree, "rev-parse", "HEAD")
		f.agent(rebaseSh, done())
		start := f.rebase(t, br, adewire.RebaseArgs{})
		if start.RunID == "" || start.NoOp {
			t.Fatalf("start = %+v, want a run", start)
		}
		run := f.waitRebase(t, task.ID, start.RunID)
		if run.State != "done" || run.Purpose != "rebase" || run.Outcome == nil || run.Outcome.Rebase == nil {
			t.Fatalf("run = %+v, want done rebase with facts", run)
		}
		f.app.Contract(t, "ade-base", "AdeTaskService.Run#rebase-done", run, rebaseMask)
		facts := run.Outcome.Rebase
		after := gitOut(t, br.Worktree, "rev-parse", "HEAD")
		if !facts.Verified || len(facts.Branches) != 1 || facts.Branches[0].Before != before || facts.Branches[0].After != after || !facts.Branches[0].OnBase {
			t.Fatalf("facts = %+v, want verified %s -> %s", facts, before, after)
		}
		if before == after {
			t.Fatal("branch did not move")
		}
	})

	t.Run("agent claims done but git disagrees", func(t *testing.T) {
		f, task, br := newRbFx(t)
		f.agent("true", done())
		run := f.waitRebase(t, task.ID, f.rebase(t, br, adewire.RebaseArgs{}).RunID)
		o := run.Outcome
		if run.State != "failed" || o == nil || o.Source != "verify" || !strings.Contains(o.Reason, "verification failed") || o.Rebase == nil || o.Rebase.Verified {
			t.Fatalf("run = %+v outcome %+v, want failed by verify", run, o)
		}
	})

	t.Run("conflict left in progress is reported then aborted", func(t *testing.T) {
		f, task, br := newRbFx(t)
		commitIn(t, br.Worktree, "clash.txt")
		gitOut(t, br.Worktree, "push", "-q", "origin", "feat/a")
		f.advanceMain(t, "clash.txt", "clash.txt\n\ntheirs\n")
		f.waitBoardBehind(t, task.ID, 2) // the watcher lags a push; a stale base made behind 1 and no conflict
		f.agent(rebaseSh+" || true", map[string]any{
			"status": "failed", "summary": "conflict", "reason": "clash.txt conflicts",
			"conflictedFiles": []string{"clash.txt"}, "lastGitError": "CONFLICT (add/add)", "tried": "plain rebase",
		})
		run := f.waitRebase(t, task.ID, f.rebase(t, br, adewire.RebaseArgs{}).RunID)
		o := run.Outcome
		if run.State != "failed" || o == nil || o.Rebase == nil || !o.Rebase.InProgress || len(o.Rebase.ConflictedFiles) == 0 {
			t.Fatalf("run = %+v outcome %+v, want failed with a rebase in progress", run, o)
		}
		if o.Report == nil || o.Report.LastGitError != "CONFLICT (add/add)" || o.Report.Tried != "plain rebase" || o.Reason != "clash.txt conflicts" {
			t.Fatalf("report = %+v reason %q, want the agent's detail", o.Report, o.Reason)
		}
		if cur := f.branch(t, task.ID); !cur.RebaseInProgress || cur.Worktree != br.Worktree {
			t.Fatalf("board branch = worktree %q rebaseInProgress %v, want %q and true", cur.Worktree, cur.RebaseInProgress, br.Worktree)
		}
		f.app.Contract(t, "ade-base", "AdeTaskService.Run#rebase-conflict", run, rebaseMask)
		f.app.Contract(t, "ade-base", "AdeTaskService.Branch#rebase-conflict", f.branch(t, task.ID), rebaseMask)
		if err := f.app.W.AdeTask.AbortRebase(ctx, adewire.BranchArgs{BranchID: br.ID}); err != nil {
			t.Fatalf("AbortRebase: %v", err)
		}
		run, _ = latestRebase(t, f.app, task.ID)
		if run.Outcome.Rebase == nil || !run.Outcome.Rebase.Aborted || run.Outcome.Rebase.InProgress {
			t.Fatalf("after abort facts = %+v, want aborted and not in progress", run.Outcome.Rebase)
		}
		f.app.Contract(t, "ade-base", "AdeTaskService.Run#rebase-aborted", run, rebaseMask)
		if err := f.app.W.AdeTask.AbortRebase(ctx, adewire.BranchArgs{BranchID: br.ID}); err == nil {
			t.Fatal("second AbortRebase succeeded with nothing in progress")
		}
	})

	t.Run("no finish_step still records what git shows", func(t *testing.T) {
		f, task, br := newRbFx(t)
		f.agent(rebaseSh, nil)
		run := f.waitRebase(t, task.ID, f.rebase(t, br, adewire.RebaseArgs{}).RunID)
		if run.Outcome == nil || run.Outcome.Reported || run.Outcome.Rebase == nil || !run.Outcome.Rebase.Verified {
			t.Fatalf("outcome = %+v, want unreported with verified facts", run.Outcome)
		}
	})

	t.Run("already on the base is a no-op without a run", func(t *testing.T) {
		f, task, br := newRbFx(t)
		f.agent(rebaseSh, done())
		f.waitRebase(t, task.ID, f.rebase(t, br, adewire.RebaseArgs{}).RunID)
		start := f.rebase(t, f.branch(t, task.ID), adewire.RebaseArgs{})
		if !start.NoOp || start.RunID != "" {
			t.Fatalf("second rebase = %+v, want a no-op", start)
		}
		pv, err := f.app.W.AdeTask.RebasePreview(ctx, adewire.OntoArgs{BranchID: br.ID})
		if err != nil || !pv.NoOp {
			t.Fatalf("preview = %+v err %v, want noOp", pv, err)
		}
	})

	t.Run("a dirty worktree blocks the rebase", func(t *testing.T) {
		f, _, br := newRbFx(t)
		commitIn(t, br.Worktree, "tracked.txt")
		gitOut(t, br.Worktree, "reset", "-q", "--soft", "HEAD~1")
		gitOut(t, br.Worktree, "add", "-A")
		pv, err := f.app.W.AdeTask.RebasePreview(ctx, adewire.OntoArgs{BranchID: br.ID})
		if err != nil {
			t.Fatal(err)
		}
		if len(pv.Blockers) != 1 || pv.Blockers[0].Kind != "dirty" || pv.Prompt == "" {
			t.Fatalf("preview = %+v, want one dirty blocker and a prompt", pv)
		}
		if _, err := f.app.W.AdeTask.Rebase(ctx, adewire.RebaseArgs{BranchID: br.ID}); err == nil {
			t.Fatal("Rebase ran on a dirty worktree")
		}
	})

	t.Run("an agent that outlives the rebase timeout is stopped", func(t *testing.T) {
		f, task, br := newRbFx(t, flowharness.WithRebaseTimeout(2*time.Second))
		claude(f.app, map[string][]fakeagent.Action{"*": {{Name: "sleep"}}})
		run := f.waitRebase(t, task.ID, f.rebase(t, br, adewire.RebaseArgs{}).RunID)
		if run.State != "failed" || run.Outcome == nil || run.Outcome.Source != "timeout" {
			t.Fatalf("run = %+v outcome %+v, want failed by timeout", run, run.Outcome)
		}
	})

	t.Run("stopping a rebase run ends it as the user's", func(t *testing.T) {
		f, task, br := newRbFx(t)
		claude(f.app, map[string][]fakeagent.Action{"*": {{Name: "sleep"}}})
		start := f.rebase(t, br, adewire.RebaseArgs{})
		testx.WaitUntil(t, waitFor, func() bool {
			r, ok := latestRebase(t, f.app, task.ID)
			return ok && r.State == "running"
		})
		if err := f.app.W.AdeTask.StopRun(ctx, adewire.RunArgs{RunID: start.RunID}); err != nil {
			t.Fatal(err)
		}
		run := f.waitRebase(t, task.ID, start.RunID)
		if run.Outcome == nil || run.Outcome.Source != "user" {
			t.Fatalf("run = %+v outcome %+v, want stopped by the user", run, run.Outcome)
		}
	})

	t.Run("retry of a rebase run is refused", func(t *testing.T) {
		f, task, br := newRbFx(t)
		f.agent("true", done())
		run := f.waitRebase(t, task.ID, f.rebase(t, br, adewire.RebaseArgs{}).RunID)
		if err := f.app.W.AdeTask.RetryRun(ctx, adewire.RunArgs{RunID: run.ID}); err == nil || !strings.Contains(err.Error(), "Rebase button") {
			t.Fatalf("RetryRun err = %v, want a pointer to the Rebase button", err)
		}
	})

	t.Run("push option is verified against the upstream", func(t *testing.T) {
		f, task, br := newRbFx(t)
		f.agent(rebaseSh+" && git push -q -u --force-with-lease origin feat/a", done())
		run := f.waitRebase(t, task.ID, f.rebase(t, br, adewire.RebaseArgs{Push: true}).RunID)
		p := run.Outcome.Rebase.Pushed
		if run.State != "done" || p == nil || !*p {
			t.Fatalf("run = %s facts %+v, want done and pushed", run.State, run.Outcome.Rebase)
		}
	})

	t.Run("push option fails when the agent did not push", func(t *testing.T) {
		f, task, br := newRbFx(t)
		f.agent(rebaseSh, done())
		run := f.waitRebase(t, task.ID, f.rebase(t, br, adewire.RebaseArgs{Push: true}).RunID)
		p := run.Outcome.Rebase.Pushed
		if run.State != "failed" || p == nil || *p {
			t.Fatalf("run = %s facts %+v, want failed and not pushed", run.State, run.Outcome.Rebase)
		}
	})

	t.Run("a stacked child rebases with its parent", func(t *testing.T) {
		f, task, br := newRbFx(t)
		oldTip := gitOut(t, br.Worktree, "rev-parse", "HEAD")
		child, err := f.app.W.AdeTask.CreateTask(ctx, adewire.CreateTaskArgs{
			Title: "child", CodeRepoIDs: []string{f.repoID}, WorkflowID: "flow",
			Bases: map[string]adewire.BaseChoice{f.repoID: {BranchID: br.ID}},
		})
		if err != nil {
			t.Fatal(err)
		}
		cb := f.branch(t, child.ID)
		claude(f.app, map[string][]fakeagent.Action{"*": {{Name: "done", Sh: "git commit -q --allow-empty -m child"}}})
		if _, err := f.app.W.AdeTask.StartRun(ctx, adewire.StartRunArgs{TaskID: child.ID, BranchNames: map[string]string{cb.ID: "feat/child"}}); err != nil {
			t.Fatal(err)
		}
		waitRun(t, f.app, child.ID, "one", "done")
		cb = f.branch(t, child.ID)
		pv, err := f.app.W.AdeTask.RebasePreview(ctx, adewire.OntoArgs{BranchID: br.ID})
		if err != nil {
			t.Fatal(err)
		}
		if len(pv.Stack) != 2 || pv.Stack[0].BranchID != br.ID || pv.Stack[1].BranchID != cb.ID {
			t.Fatalf("stack = %+v, want parent then child", pv.Stack)
		}
		gitCall := func(wt string, args ...string) fakeagent.MCPCall {
			return fakeagent.MCPCall{Server: agentmcp.ServerName, Tool: "git", Args: map[string]any{"worktree": wt, "args": args}}
		}
		claude(f.app, map[string][]fakeagent.Action{"*": {{Name: "nofinish", MCP: []fakeagent.MCPCall{
			gitCall(br.Worktree, "fetch", "-q", "origin"),
			gitCall(br.Worktree, "rebase", "-q", "origin/main"),
			gitCall(cb.Worktree, "rebase", "-q", "--onto", "feat/a", oldTip),
			{Server: agentmcp.ServerName, Tool: "finish_step", Args: done()},
		}}}})
		run := f.waitRebase(t, task.ID, f.rebase(t, br, adewire.RebaseArgs{}).RunID)
		if run.State != "done" || run.Outcome.Rebase == nil || len(run.Outcome.Rebase.Branches) != 2 || !run.Outcome.Rebase.Verified {
			t.Fatalf("run = %s facts %+v, want both branches verified", run.State, run.Outcome.Rebase)
		}
		matches, _ := filepath.Glob(filepath.Join(f.app.FakeDir, "*.args"))
		var args string
		for _, m := range matches {
			if raw, err := os.ReadFile(m); err == nil && strings.Contains(string(raw), toolName(agentmcp.Git)) {
				args = string(raw)
			}
		}
		for _, want := range []string{"--add-dir\n" + br.Worktree, cb.Worktree, "--tools\nRead,Edit,Write,Grep,Glob", "--strict-mcp-config", toolName(agentmcp.Git)} {
			if !strings.Contains(args, want) {
				t.Errorf("rebase args lack %q:\n%s", want, args)
			}
		}
		if strings.Contains(args, "Bash") && !strings.Contains(args, "--disallowedTools\nBash") {
			t.Errorf("rebase args grant Bash:\n%s", args)
		}
		var gitCalls, refChecks int
		for _, r := range opsNow(t, f.app) {
			if r.Category == oplog.CategoryMCP && r.Kind == "git" {
				gitCalls++
				if len(r.Steps) != 1 || !strings.HasPrefix(r.Steps[0].Argv, "git --no-pager ") || r.Status != oplog.StatusOK {
					t.Errorf("git tool op = %+v, want one step on the shared runner's argv", r)
				}
			}
			for _, st := range r.Steps {
				if strings.Contains(st.Argv, "check-ref-format") {
					refChecks++
				}
			}
			if strings.Contains(derefCmd(r), "check-ref-format") && r.Status != oplog.StatusOK {
				t.Errorf("check-ref-format op %+v is not ok", r)
			}
		}
		if gitCalls != 3 {
			t.Errorf("git tool ops = %d, want 3", gitCalls)
		}
		t.Logf("check-ref-format steps: %d", refChecks)
	})

	t.Run("run_outcome lists the failed rebase to a later step run of the same task only", func(t *testing.T) {
		f := newBaseFx(t)
		saveWorkflow(t, f.app, "flow", flowYAML("flow", agentStep("one", "")+agentStep("two", "    before: approval\n")))
		task, br := f.task(t, "Fix", adewire.BaseChoice{})
		claude(f.app, map[string][]fakeagent.Action{"*": {
			{Name: "done"},
			{Name: "nofinish", MCP: []fakeagent.MCPCall{{Server: agentmcp.ServerName, Tool: "finish_step", Args: map[string]any{
				"status": "failed", "summary": "conflict", "reason": "clash", "conflictedFiles": []string{"x.txt"},
			}}}},
			{Name: "done", MCP: []fakeagent.MCPCall{{Server: agentmcp.ServerName, Tool: "run_outcome", Args: map[string]any{"kind": "rebase"}}}},
		}})
		if err := f.startRun(t, task, br, "feat/api-work"); err != nil {
			t.Fatal(err)
		}
		waitRun(t, f.app, task.ID, "one", "done")
		f.repo.Checkout("main")
		f.repo.Commit("main moves", map[string]string{"m.txt": "m\n"})
		f.repo.Git("push", "-q", "origin", "main")
		br = branchOf(t, board(t, f.app), task.ID, f.repoID)
		waitBehind(t, f.app, br)
		res, err := f.app.W.AdeTask.Rebase(ctx, adewire.RebaseArgs{BranchID: br.ID})
		if err != nil {
			t.Fatal(err)
		}
		testx.WaitUntil(t, waitFor, func() bool {
			r, ok := latestRebase(t, f.app, task.ID)
			return ok && r.ID == res.RunID && r.State == "failed"
		})
		if err := f.app.W.AdeTask.Approve(ctx, adewire.StepArgs{TaskID: task.ID, StepID: "two"}); err != nil {
			t.Fatal(err)
		}
		waitRun(t, f.app, task.ID, "two", "done")
		prompt := fakeFile(t, f.app, "api-3.prompt")
		if !strings.Contains(prompt, "Last rebase of feat/api-work failed") {
			t.Fatalf("step prompt lacks the failed-rebase line:\n%s", prompt)
		}
		out := fakeFile(t, f.app, "mcp-run_outcome-1.json")
		if !strings.Contains(out, "clash") || !strings.Contains(out, "x.txt") {
			t.Fatalf("run_outcome result = %s, want the rebase run's reason and conflicted files", out)
		}
	})

	t.Run("an approved step waits for a running rebase, then launches", func(t *testing.T) {
		f := newBaseFx(t)
		saveWorkflow(t, f.app, "flow", flowYAML("flow", agentStep("one", "")+agentStep("two", "    before: approval\n")))
		task, br := f.task(t, "Fix", adewire.BaseChoice{})
		gate := filepath.Join(f.app.Work, "release-rebase")
		claude(f.app, map[string][]fakeagent.Action{"*": {
			{Name: "done"},
			{Name: "nofinish", Sh: rebaseSh, WaitFile: gate, MCP: []fakeagent.MCPCall{{Server: agentmcp.ServerName, Tool: "finish_step", Args: done()}}},
			{Name: "done"},
		}})
		if err := f.startRun(t, task, br, "feat/api-work"); err != nil {
			t.Fatal(err)
		}
		waitRun(t, f.app, task.ID, "one", "done")
		f.repo.Checkout("main")
		f.repo.Commit("main moves", map[string]string{"m.txt": "m\n"})
		f.repo.Git("push", "-q", "origin", "main")
		br = branchOf(t, board(t, f.app), task.ID, f.repoID)
		waitBehind(t, f.app, br)
		res, err := f.app.W.AdeTask.Rebase(ctx, adewire.RebaseArgs{BranchID: br.ID})
		if err != nil {
			t.Fatal(err)
		}
		if err := f.app.W.AdeTask.Approve(ctx, adewire.StepArgs{TaskID: task.ID, StepID: "two"}); err != nil {
			t.Fatal(err)
		}
		testx.WaitUntil(t, waitFor, func() bool {
			r, ok := latestRun(t, f.app, task.ID, "two")
			return ok && r.State == "pending" && strings.Contains(r.Note, "waiting for rebase")
		})
		if err := os.WriteFile(gate, nil, 0o644); err != nil {
			t.Fatal(err)
		}
		if run := (&rbFx{app: f.app}).waitRebase(t, task.ID, res.RunID); run.State != "done" {
			t.Fatalf("rebase = %+v, want done", run)
		}
		waitRun(t, f.app, task.ID, "two", "done")
	})

	t.Run("taking over a failed rebase and finishing it settles the run from git", func(t *testing.T) {
		f, task, br := newRbFx(t, flowharness.WithTrackerGrace(time.Second))
		commitIn(t, br.Worktree, "clash.txt")
		f.advanceMain(t, "clash.txt", "clash.txt\n\ntheirs\n")
		claude(f.app, map[string][]fakeagent.Action{"*": {
			{Name: "nofinish", Sh: rebaseSh + " || true", MCP: []fakeagent.MCPCall{{Server: agentmcp.ServerName, Tool: "finish_step", Args: map[string]any{
				"status": "failed", "summary": "conflict", "reason": "clash.txt conflicts",
			}}}},
			{Name: "nofinish", Sh: "printf 'merged\\n' > clash.txt && git add clash.txt && GIT_EDITOR=true git rebase --continue", MCP: []fakeagent.MCPCall{{
				Server: agentmcp.ServerName, Tool: "finish_step", Args: done(),
			}}},
		}})
		run := f.waitRebase(t, task.ID, f.rebase(t, br, adewire.RebaseArgs{}).RunID)
		if run.State != "failed" || run.SessionID == "" {
			t.Fatalf("run = %+v, want failed with a session", run)
		}
		f.app.WindowMgr.OpenWindow(shell.WindowRecord{Key: sessionWindow})
		launch, err := f.app.W.AdeTask.TakeOver(ctx, adewire.TakeOverArgs{SessionID: run.SessionID})
		if err != nil {
			t.Fatal(err)
		}
		openLaunch(t, f.app, launch)
		testx.WaitUntil(t, waitFor, func() bool {
			r, _ := latestRebase(t, f.app, task.ID)
			return r.State == "done" && r.Outcome != nil && r.Outcome.Rebase != nil && r.Outcome.Rebase.Verified
		})
	})

	t.Run("a step run reports the reason and git error through finish_step", func(t *testing.T) {
		f := newBaseFx(t)
		task, br := f.task(t, "Fix", adewire.BaseChoice{})
		claude(f.app, map[string][]fakeagent.Action{"*": {{Name: "nofinish", MCP: []fakeagent.MCPCall{{
			Server: agentmcp.ServerName, Tool: "finish_step",
			Args: map[string]any{"status": "failed", "summary": "stuck", "reason": "push rejected", "lastGitError": "non-fast-forward"},
		}}}}})
		if err := f.startRun(t, task, br, "feat/api-fix"); err != nil {
			t.Fatal(err)
		}
		run := waitRun(t, f.app, task.ID, "one", "failed")
		if run.Outcome == nil || run.Outcome.Reason != "push rejected" || run.Outcome.Report == nil || run.Outcome.Report.LastGitError != "non-fast-forward" {
			t.Fatalf("outcome = %+v, want the agent's reason and git error", run.Outcome)
		}
	})
}

func latestRebase(t *testing.T, app *flowharness.App, taskID string) (adewire.Run, bool) {
	t.Helper()
	var best adewire.Run
	found := false
	for _, r := range taskOf(t, board(t, app), taskID).Runs {
		if r.Purpose == "rebase" && (!found || r.Attempt >= best.Attempt) {
			best, found = r, true
		}
	}
	return best, found
}

func derefCmd(r oplog.Record) string {
	if r.Command == nil {
		return ""
	}
	return *r.Command
}
