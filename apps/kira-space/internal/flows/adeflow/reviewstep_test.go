package adeflow_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitreview"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitrpc"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitsession"
	"github.com/kirathecat/kira-studio/internal/agentmcp"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// P286 review step. Contract ade-review-step: tests/ui/ade-v2-review-step.spec.ts reads these.

const rvScenario = "ade-review-step"

var rvMask = flowharness.Mask("id", "runId", "taskId", "branchId", "targetRunId", "defaultTargetRunId", "sessionId",
	"startedAt", "finishedAt", "exitCode", "createdAt", "repliedAt", "sentAt", "attempt")

func lineRange(n int) gitreview.LineRange { return gitreview.LineRange{Start: n, End: n} }

const rvImplement = "printf 'one\\ntwo\\nthree\\n' > r.txt && git add r.txt && git commit -q -m implement"

func rvStep(sendTo string) string {
	return "  - id: review\n    name: review\n    kind: review\n    runs_on: each repo\n    timeout: 1m\n    max_returns: 2\n" +
		"    review:\n      send_to: " + sendTo + "\n" +
		"    results:\n" +
		"      - id: approved\n        ok: true\n" +
		"      - id: rework\n        ok: false\n        next: implement\n" +
		"      - id: rejected\n        ok: false\n        next: stop\n"
}

func rvReply(id int64, text string) fakeagent.MCPCall {
	return fakeagent.MCPCall{Server: agentmcp.ServerName, Tool: "reply_review_comment",
		Args: map[string]any{"id": id, "reply": text, "resolved": true}}
}

// reviewFx is a task on implement, review, verify, with the review window's git stream.
type reviewFx struct {
	*runFixture
	br  adewire.Branch
	tgt *adewire.ReviewWindowTarget
	gs  *flowharness.GitStream
}

func newReviewFx(t *testing.T, sendTo string, actions ...fakeagent.Action) *reviewFx {
	t.Helper()
	steps := agentStep("implement", "") + rvStep(sendTo) + agentStep("verify", "")
	return &reviewFx{runFixture: newRunFixture(t, actions, steps)}
}

// toReview starts the task and waits for the review run to wait for the user.
func (f *reviewFx) toReview(t *testing.T, branch string) adewire.Run {
	t.Helper()
	f.start(t, branch)
	run := waitRun(t, f.app, f.taskID, "review", "review")
	f.br = branchOf(t, board(t, f.app), f.taskID, f.repoID)
	return run
}

// window opens the review window and the git stream its comments go through.
func (f *reviewFx) window(t *testing.T) {
	t.Helper()
	if _, err := f.app.W.AdeTask.OpenReviewWindow(ctx, adewire.BranchArgs{BranchID: f.br.ID}); err != nil {
		t.Fatal(err)
	}
	rec := f.app.WindowMgr.Opened[len(f.app.WindowMgr.Opened)-1]
	tgt, err := f.app.W.AdeTask.ReviewWindowTarget(ctx, adewire.WindowKeyArgs{WindowKey: rec.Key})
	if err != nil || tgt == nil {
		t.Fatalf("ReviewWindowTarget = %+v, %v", tgt, err)
	}
	f.tgt = tgt
	f.gs = f.app.OpenGitStream()
	var init gitrpc.AppInitResult
	f.gs.MustRequest("app.init", nil, &init)
	var opened gitrpc.RepoOpenResult
	f.gs.MustRequest("repo.open", gitrpc.RepoOpenParams{Path: filepath.Join(f.app.Work, "api")}, &opened)
	if opened.Kind != "ok" {
		t.Fatalf("repo.open = %+v", opened)
	}
}

func (f *reviewFx) comment(t *testing.T, line int, body, kind string) gitsession.CommentEntry {
	t.Helper()
	var res gitrpc.ReviewCommentAddResult
	f.gs.MustRequest("review.comment.add", gitrpc.ReviewCommentAddParams{
		RepoID: f.tgt.GitRepoID, Branch: f.tgt.Branch, Path: "r.txt", At: gitOut(t, f.br.Worktree, "rev-parse", "HEAD"),
		Range: lineRange(line), Body: body, Kind: kind,
	}, &res)
	return res.Comment
}

func (f *reviewFx) comments(t *testing.T) []gitsession.CommentEntry {
	t.Helper()
	var res gitsession.CommentListResult
	f.gs.MustRequest("review.comment.list", gitrpc.ReviewCommentListParams{RepoID: f.tgt.GitRepoID, Branch: f.tgt.Branch}, &res)
	return res.Comments
}

func (f *reviewFx) setState(t *testing.T, id int64, state string) {
	t.Helper()
	var res gitrpc.ReviewCommentSetStateResult
	f.gs.MustRequest("review.comment.setState", gitrpc.ReviewCommentSetStateParams{
		RepoID: f.tgt.GitRepoID, Branch: f.tgt.Branch, ID: id, State: state,
	}, &res)
	if !res.Changed {
		t.Fatalf("setState %d %s changed nothing", id, state)
	}
}

func (f *reviewFx) step(t *testing.T) adewire.ReviewStepState {
	t.Helper()
	st, err := f.app.W.AdeTask.ReviewStep(ctx, adewire.BranchArgs{BranchID: f.br.ID})
	if err != nil {
		t.Fatalf("ReviewStep: %v", err)
	}
	return st
}

func (f *reviewFx) waitPhase(t *testing.T, phases ...string) adewire.ReviewStepState {
	t.Helper()
	var st adewire.ReviewStepState
	testx.WaitUntil(t, waitFor, func() bool {
		st = f.step(t)
		if st.Run == nil || st.Run.Review == nil {
			return false
		}
		for _, p := range phases {
			if st.Run.Review.Phase == p {
				return true
			}
		}
		return false
	})
	return st
}

func (f *reviewFx) send(route, target string) error {
	st, _ := f.app.W.AdeTask.ReviewStep(ctx, adewire.BranchArgs{BranchID: f.br.ID})
	return f.app.W.AdeTask.SendReview(ctx, adewire.SendReviewArgs{RunID: st.Run.ID, Route: route, TargetRunID: target})
}

func (f *reviewFx) finish(result, note string) error {
	st, _ := f.app.W.AdeTask.ReviewStep(ctx, adewire.BranchArgs{BranchID: f.br.ID})
	return f.app.W.AdeTask.FinishReview(ctx, adewire.FinishReviewArgs{RunID: st.Run.ID, Result: result, Note: note})
}

func (f *reviewFx) claudeID(t *testing.T, call string) string {
	t.Helper()
	args := fakeFile(t, f.app, call)
	if id := argAfter(args, "--session-id"); id != "" {
		return id
	}
	return argAfter(args, "--resume")
}

func TestReviewStepWaits(t *testing.T) {
	f := newReviewFx(t, "implementer", fakeagent.Action{Name: "done", Sh: rvImplement})
	run := f.toReview(t, "feat/api-review-waits")
	if run.Review == nil || run.Review.Phase != "waiting" || run.Review.Route != "implementer" || run.Review.Round != 0 {
		t.Fatalf("review run = %+v, want a waiting review", run)
	}
	if _, ok := latestRun(t, f.app, f.taskID, "verify"); ok {
		t.Fatal("verify started while the review waits")
	}
	for _, r := range taskOf(t, board(t, f.app), f.taskID).Runs {
		if r.Purpose != "" {
			t.Fatalf("board lists %+v: a round is no board run", r)
		}
	}

	st := f.step(t)
	if st.Run == nil || st.Run.ID != run.ID || st.StepName != "review" || len(st.Results) != 3 || st.Route != "implementer" {
		t.Fatalf("ReviewStep = %+v", st)
	}
	if len(st.Candidates) != 1 || st.Candidates[0].Step != "implement" || st.DefaultTargetRunID != st.Candidates[0].RunID {
		t.Fatalf("candidates = %+v default %q, want the implement run", st.Candidates, st.DefaultTargetRunID)
	}
	f.app.Contract(t, rvScenario, "AdeTaskService.ReviewStep#waiting", st, rvMask)

	// The review window keeps its comments: the session is pinned for as long as the task lives.
	f.window(t)
	if got := f.comment(t, 1, "keep me", ""); got.Kind != "change" || got.State != "open" {
		t.Fatalf("comment = %+v, want an open change", got)
	}
	if len(f.comments(t)) != 1 {
		t.Fatal("comment lost")
	}
}

func TestReviewSendImplementer(t *testing.T) {
	gate := filepath.Join(t.TempDir(), "gate")
	f := newReviewFx(t, "implementer",
		fakeagent.Action{Name: "done", Sh: rvImplement},
		fakeagent.Action{
			Sh:       "echo fixed >> r.txt && git commit -q -am fix",
			MCP:      []fakeagent.MCPCall{rvReply(1, "renamed"), rvReply(2, "because the API retries")},
			WaitFile: gate, Name: "done",
		})
	f.toReview(t, "feat/api-review-impl")
	f.window(t)
	change := f.comment(t, 1, "rename this", "")
	question := f.comment(t, 3, "why twice?", "question")
	if change.ID != 1 || question.ID != 2 {
		t.Fatalf("comment ids = %d, %d, want 1, 2 (the fake agent replies to them)", change.ID, question.ID)
	}
	implementID := f.claudeID(t, "api-1.args")

	if err := f.send("", ""); err != nil {
		t.Fatalf("SendReview: %v", err)
	}
	working := f.waitPhase(t, "working")
	if working.Round == nil || working.Round.N != 1 || working.Round.Route != "implementer" || working.Round.Fallback {
		t.Fatalf("round = %+v", working.Round)
	}
	f.app.Contract(t, rvScenario, "AdeTaskService.ReviewStep#working", working, rvMask)
	if err := f.send("", ""); err == nil {
		t.Fatal("SendReview accepted while a round runs")
	}
	if err := os.WriteFile(gate, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	answered := f.waitPhase(t, "waiting")
	if answered.Round == nil || answered.Round.Run.State != "done" || answered.Round.Commits == nil || *answered.Round.Commits != 1 {
		t.Fatalf("round after the answer = %+v, want done with one commit", answered.Round)
	}
	f.app.Contract(t, rvScenario, "AdeTaskService.ReviewStep#answered", answered, rvMask)

	args := fakeFile(t, f.app, "api-2.args")
	if got := argAfter(args, "--resume"); got != implementID {
		t.Fatalf("round --resume = %q, want the implementer session %q", got, implementID)
	}
	if !strings.Contains(args, toolName(agentmcp.ReviewReply)) {
		t.Fatalf("round args lack %s:\n%s", toolName(agentmcp.ReviewReply), args)
	}
	prompt := fakeFile(t, f.app, "api-2.prompt")
	for _, want := range []string{"#1 change r.txt:1", "rename this", "#2 question r.txt:3", "why twice?", "reply_review_comment", "finish_step"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("round prompt lacks %q:\n%s", want, prompt)
		}
	}
	for _, c := range f.comments(t) {
		if c.State != "addressed" || c.Reply == "" || c.RepliedAt == nil || c.SentAt == nil {
			t.Fatalf("comment after the round = %+v, want addressed with a reply", c)
		}
	}
	if run, _ := latestRun(t, f.app, f.taskID, "review"); run.State != "review" {
		t.Fatalf("review run = %+v, want it still waiting", run)
	}
}

func TestReviewSendNew(t *testing.T) {
	f := newReviewFx(t, "implementer",
		fakeagent.Action{Name: "done", Sh: rvImplement},
		fakeagent.Action{MCP: []fakeagent.MCPCall{rvReply(1, "looks wrong, fixed")}, Name: "done"},
		fakeagent.Action{MCP: []fakeagent.MCPCall{rvReply(1, "fixed again")}, Name: "done"})
	f.toReview(t, "feat/api-review-new")
	f.window(t)
	f.comment(t, 1, "rename this", "")

	if err := f.send("new", ""); err != nil {
		t.Fatalf("SendReview new: %v", err)
	}
	st := f.waitPhase(t, "waiting")
	if st.Route != "new" || st.Round == nil || st.Round.N != 1 {
		t.Fatalf("after round 1 = route %q round %+v", st.Route, st.Round)
	}
	first := fakeFile(t, f.app, "api-2.args")
	dedicated := argAfter(first, "--session-id")
	if dedicated == "" || strings.Contains(first, "--resume") {
		t.Fatalf("round 1 args want a fresh --session-id:\n%s", first)
	}
	if p := fakeFile(t, f.app, "api-2.prompt"); !strings.Contains(p, "Read the branch diff against the base first") || !strings.Contains(p, "#1 change r.txt:1") {
		t.Fatalf("round 1 prompt is not the dedicated agent's message:\n%s", p)
	}
	if got := f.comments(t); len(got) != 1 || got[0].State != "addressed" {
		t.Fatalf("comments after round 1 = %+v", got)
	}

	f.setState(t, 1, "open")
	if err := f.send("", ""); err != nil {
		t.Fatalf("SendReview again: %v", err)
	}
	f.waitPhase(t, "waiting")
	testx.WaitUntil(t, waitFor, func() bool { _, err := os.Stat(filepath.Join(f.app.FakeDir, "api-3.args")); return err == nil })
	second := fakeFile(t, f.app, "api-3.args")
	if got := argAfter(second, "--resume"); got != dedicated {
		t.Fatalf("round 2 --resume = %q, want the dedicated session %q", got, dedicated)
	}
	if p := fakeFile(t, f.app, "api-3.prompt"); !strings.Contains(p, "Review round 2") {
		t.Fatalf("round 2 prompt is not the follow-up message:\n%s", p)
	}
	if st := f.step(t); st.Round == nil || st.Round.N != 2 || st.Route != "new" {
		t.Fatalf("after round 2 = %+v", st)
	}
}

func TestReviewFinish(t *testing.T) {
	t.Run("approved", func(t *testing.T) {
		f := newReviewFx(t, "implementer", fakeagent.Action{Name: "done", Sh: rvImplement}, fakeagent.Action{Name: "done"})
		f.toReview(t, "feat/api-review-ok")
		if err := f.finish("nope", ""); err == nil {
			t.Fatal("FinishReview accepted an unknown result")
		}
		if err := f.finish("approved", "lgtm"); err != nil {
			t.Fatalf("FinishReview: %v", err)
		}
		waitRun(t, f.app, f.taskID, "verify", "done")
		run, _ := latestRun(t, f.app, f.taskID, "review")
		if run.State != "done" || run.Outcome == nil || run.Outcome.Result != "approved" || run.Summary != "lgtm" {
			t.Fatalf("review run = %+v, want done on approved", run)
		}
		st := f.step(t)
		if st.Run.Review.Phase != "done" {
			t.Fatalf("phase = %q, want done", st.Run.Review.Phase)
		}
		f.app.Contract(t, rvScenario, "AdeTaskService.ReviewStep#done", st, rvMask)
	})

	t.Run("rework", func(t *testing.T) {
		f := newReviewFx(t, "implementer",
			fakeagent.Action{Name: "done", Sh: rvImplement},
			fakeagent.Action{Name: "done", Sh: "echo more >> r.txt && git commit -q -am more"},
			fakeagent.Action{Name: "done"})
		first := f.toReview(t, "feat/api-review-rework")
		f.window(t)
		f.comment(t, 1, "keep me", "")
		implementID := f.claudeID(t, "api-1.args")

		if err := f.finish("rework", "rename foo"); err != nil {
			t.Fatalf("FinishReview rework: %v", err)
		}
		testx.WaitUntil(t, waitFor, func() bool {
			r, _ := latestRun(t, f.app, f.taskID, "review")
			return r.ID != first.ID && r.State == "review"
		})
		impl, _ := latestRun(t, f.app, f.taskID, "implement")
		if impl.Attempt != 2 {
			t.Fatalf("implement = %+v, want attempt 2 after the send-back", impl)
		}
		args := fakeFile(t, f.app, "api-2.args")
		if got := argAfter(args, "--resume"); got != implementID {
			t.Fatalf("send-back --resume = %q, want %q", got, implementID)
		}
		if p := fakeFile(t, f.app, "api-2.prompt"); !strings.Contains(p, "rename foo") {
			t.Fatalf("send-back prompt lacks the note:\n%s", p)
		}
		if len(f.comments(t)) != 1 {
			t.Fatal("the second review lost the comments")
		}
		if err := f.finish("approved", ""); err != nil {
			t.Fatal(err)
		}
		waitRun(t, f.app, f.taskID, "verify", "done")
	})
}

func TestReviewLostFallback(t *testing.T) {
	f := newReviewFx(t, "implementer",
		fakeagent.Action{Name: "done", Sh: rvImplement},
		fakeagent.Action{Name: "lost"},
		fakeagent.Action{MCP: []fakeagent.MCPCall{rvReply(1, "done anyway")}, Name: "done"})
	f.toReview(t, "feat/api-review-lost")
	f.window(t)
	f.comment(t, 1, "rename this", "")

	if err := f.send("", ""); err != nil {
		t.Fatalf("SendReview: %v", err)
	}
	st := f.waitPhase(t, "waiting")
	testx.WaitUntil(t, waitFor, func() bool {
		st = f.step(t)
		return st.Round != nil && st.Round.Fallback && st.Round.Run.State == "done"
	})
	if st.Route != "new" {
		t.Fatalf("route after the fallback = %q, want new", st.Route)
	}
	if !strings.Contains(fakeFile(t, f.app, "api-2.args"), "--resume") {
		t.Fatal("the first round did not try to resume the implementer")
	}
	fallback := fakeFile(t, f.app, "api-3.args")
	if !strings.Contains(fallback, "--session-id") || strings.Contains(fallback, "--resume") {
		t.Fatalf("fallback args want a fresh session:\n%s", fallback)
	}
	if got := f.comments(t); len(got) != 1 || got[0].State != "addressed" || got[0].Reply != "done anyway" {
		t.Fatalf("comments after the fallback = %+v", got)
	}
	if st.Run.Review.Note == "" {
		t.Fatal("the review shows no fallback note")
	}
	f.app.Contract(t, rvScenario, "AdeTaskService.ReviewStep#fallback", st, rvMask)
}

func TestReviewQuitResume(t *testing.T) {
	hold := filepath.Join(t.TempDir(), "hold")
	f := newReviewFx(t, "implementer",
		fakeagent.Action{Name: "done", Sh: rvImplement},
		fakeagent.Action{WaitFile: hold},
		fakeagent.Action{MCP: []fakeagent.MCPCall{rvReply(1, "renamed")}, Name: "done"})
	review := f.toReview(t, "feat/api-review-quit")
	f.window(t)
	f.comment(t, 1, "rename this", "")
	implementID := f.claudeID(t, "api-1.args")
	if err := f.send("", ""); err != nil {
		t.Fatal(err)
	}
	f.waitPhase(t, "working")
	testx.WaitUntil(t, waitFor, func() bool { _, err := os.Stat(filepath.Join(f.app.FakeDir, "api-2.args")); return err == nil })

	f.app.Restart()
	st := f.waitPhase(t, "stuck")
	if st.Run.ID != review.ID || st.Run.State != "review" || st.Round == nil || !st.Round.Run.Resumable {
		t.Fatalf("after the restart = %+v, want the review waiting and the round resumable", st)
	}
	f.app.Contract(t, rvScenario, "AdeTaskService.ReviewStep#stuck", st, rvMask)

	if err := f.app.W.AdeTask.ResumeRun(ctx, adewire.RunArgs{RunID: st.Round.Run.ID}); err != nil {
		t.Fatalf("ResumeRun: %v", err)
	}
	st = f.waitPhase(t, "waiting")
	testx.WaitUntil(t, waitFor, func() bool { st = f.step(t); return st.Round != nil && st.Round.Run.State == "done" })
	args := fakeFile(t, f.app, "api-3.args")
	if got := argAfter(args, "--resume"); got != implementID {
		t.Fatalf("resumed round --resume = %q, want %q", got, implementID)
	}
	prompt := fakeFile(t, f.app, "api-3.prompt")
	if !strings.Contains(prompt, "Kira Space closed while you worked") || !strings.Contains(prompt, "reply_review_comment") {
		t.Fatalf("resumed round prompt lacks the resume line or the reply instruction:\n%s", prompt)
	}
	if err := f.app.W.AdeTask.ResumeRun(ctx, adewire.RunArgs{RunID: st.Round.Run.ID}); err == nil {
		t.Fatal("ResumeRun accepted a finished round")
	}
}

func TestReviewRefusals(t *testing.T) {
	gate := filepath.Join(t.TempDir(), "gate")
	f := newReviewFx(t, "implementer",
		fakeagent.Action{Name: "done", Sh: rvImplement},
		fakeagent.Action{WaitFile: gate, Name: "done"},
		fakeagent.Action{MCP: []fakeagent.MCPCall{rvReply(99, "not mine")}, Name: "done"})
	f.toReview(t, "feat/api-review-refuse")
	f.window(t)

	wantErr := func(what string, err error, sub string) {
		t.Helper()
		if err == nil || !strings.Contains(err.Error(), sub) {
			t.Fatalf("%s: err = %v, want it to contain %q", what, err, sub)
		}
	}
	wantErr("send without comments", f.send("", ""), "no open comments")
	f.comment(t, 1, "rename this", "")
	wantErr("send with a bad target", f.send("implementer", "nope"), "take the review")
	wantErr("send with a bad route", f.send("sideways", ""), "route")
	if err := f.send("", ""); err != nil {
		t.Fatal(err)
	}
	st := f.waitPhase(t, "working")
	wantErr("send during a round", f.send("", ""), "already running")
	wantErr("finish during a round", f.finish("approved", ""), "already running")
	wantErr("retry a round", f.app.W.AdeTask.RetryRun(ctx, adewire.RunArgs{RunID: st.Round.Run.ID}), "Send the review again")

	if err := os.WriteFile(gate, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	f.waitPhase(t, "waiting")
	testx.WaitUntil(t, waitFor, func() bool { st = f.step(t); return st.Round != nil && st.Round.Run.State == "done" })
	wantErr("finish with an unknown result", f.finish("nope", ""), "Choose one")
	wantErr("retry a waiting review", f.app.W.AdeTask.RetryRun(ctx, adewire.RunArgs{RunID: st.Run.ID}), "waits for you")
	wantErr("stop a waiting review", f.app.W.AdeTask.StopRun(ctx, adewire.RunArgs{RunID: st.Run.ID}), "waits for you")
	if got := f.comments(t); len(got) != 1 || got[0].State != "open" || got[0].Reply != "" {
		t.Fatalf("comments after a round that replied to nothing = %+v, want the comment still open", got)
	}

	// A reply to an id the round was not sent comes back as a tool error and stores nothing.
	if err := f.send("", ""); err != nil {
		t.Fatal(err)
	}
	st = f.waitPhase(t, "stuck")
	if log := logText(t, f.app, "run", st.Round.Run.ID); !strings.Contains(log, "tool error") {
		t.Fatalf("round log lacks the refused reply:\n%s", log)
	}
	if got := f.comments(t); len(got) != 1 || got[0].State != "open" || got[0].Reply != "" {
		t.Fatalf("comments after the refused reply = %+v", got)
	}
}

// P354: the interactive review agent takes the user's open comments through one button. Contract
// ade-review-forward: tests/ui/ade-v2-review.spec.ts reads these.
func TestReviewForwardComments(t *testing.T) {
	hold := filepath.Join(t.TempDir(), "hold")
	f := newReviewFx(t, "implementer",
		fakeagent.Action{Name: "done", Sh: rvImplement},
		fakeagent.Action{WaitFile: hold, Name: "done"})
	f.toReview(t, "feat/api-review-forward")
	f.window(t)
	f.comment(t, 1, "rename this", "")
	f.comment(t, 3, "why twice?", "question")
	done := f.comment(t, 2, "already handled", "")
	f.setState(t, done.ID, "addressed")

	args := adewire.BranchArgs{BranchID: f.br.ID}
	f.app.Contract(t, "ade-review-forward", "args:AdeTaskService.ForwardReviewComments", args)
	if _, err := f.app.W.AdeTask.ForwardReviewComments(ctx, args); err == nil {
		t.Fatal("ForwardReviewComments succeeded before the review agent runs")
	}

	copied, err := f.app.W.AdeTask.ReviewForwardText(ctx, args)
	if err != nil || !strings.Contains(copied.Text, "rename this") || !strings.Contains(copied.Text, "why twice?") ||
		strings.Contains(copied.Text, "already handled") {
		t.Fatalf("ReviewForwardText = %q, %v, want the two open comments with no agent running", copied.Text, err)
	}
	f.app.Contract(t, "ade-review-forward", "AdeTaskService.ReviewForwardText", copied)
	for _, c := range f.comments(t) {
		if c.SentAt != nil {
			t.Fatalf("ReviewForwardText stamped comment %d sent", c.ID)
		}
	}

	launch, err := f.app.W.AdeTask.LaunchReviewAgent(ctx, adewire.TaskArgs{TaskID: f.taskID})
	if err != nil {
		t.Fatal(err)
	}
	openLaunch(t, f.app, launch.Launch)
	waitSession(t, f.app, launch.Launch.SessionID, "running")

	res, err := f.app.W.AdeTask.ForwardReviewComments(ctx, args)
	if err != nil || res.Count != 2 {
		t.Fatalf("ForwardReviewComments = %+v, %v, want 2 comments", res, err)
	}
	f.app.Contract(t, "ade-review-forward", "AdeTaskService.ForwardReviewComments", res)
	testx.WaitUntil(t, waitFor, func() bool {
		raw, _ := os.ReadFile(filepath.Join(f.app.FakeDir, "api-2.prompt"))
		p := string(raw)
		return strings.Contains(p, "rename this") && strings.Contains(p, "why twice?")
	})
	if p := fakeFile(t, f.app, "api-2.prompt"); !strings.Contains(p, strings.TrimSpace(copied.Text)) {
		t.Fatalf("forwarded prompt differs from the copy text:\n%s\nvs\n%s", p, copied.Text)
	}
	if p := fakeFile(t, f.app, "api-2.prompt"); strings.Contains(p, "already handled") || strings.Contains(p, "reply_review_comment") {
		t.Fatalf("forwarded prompt carries an addressed comment or the round tool:\n%s", p)
	}
	left := f.comments(t)
	if len(left) != 1 || left[0].ID != done.ID {
		t.Fatalf("comments after the forward = %+v, want only the addressed one", left)
	}
	f.app.Contract(t, "ade-review-forward", "git:review.comment.list#forwarded", gitsession.CommentListResult{Comments: left},
		flowharness.Mask("at", "createdAt", "anchorSha"))
	if _, err := f.app.W.AdeTask.ForwardReviewComments(ctx, args); err == nil {
		t.Fatal("ForwardReviewComments succeeded with no open comment")
	}
}
