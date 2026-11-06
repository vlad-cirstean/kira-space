package claude

import (
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitreview"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitrpc"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitsession"
	"github.com/kirathecat/kira-studio/internal/testx"
)

const reviewStepsYAML = "  - id: implement\n    name: implement\n    kind: agent\n    runs_on: each repo\n    timeout: 3m\n" +
	"    allowed_tools: [\"Bash(sh:*)\", \"Bash(git:*)\", \"Edit\"]\n" +
	"    prompt: \"Run exactly one Bash command: sh -c \\\"printf 'one\\\\ntwo\\\\nthree\\\\n' > notes.txt && git add notes.txt && git commit -q -m add-notes\\\" . Then call finish_step with status done and summary ok.\"\n" +
	"  - id: review\n    name: review\n    kind: review\n    runs_on: each repo\n    timeout: 5m\n" +
	"    review:\n      send_to: implementer\n" +
	"    results:\n      - id: approved\n        ok: true\n"

type reviewRig struct {
	*adeFixture
	task adewire.Task
	br   adewire.Branch
	gs   interface {
		MustRequest(method string, params, out any)
	}
	target *adewire.ReviewWindowTarget
}

// startReview runs the implement step with the real claude and opens the review window.
func startReview(t *testing.T) *reviewRig {
	t.Helper()
	f := newAdeFixture(t, nil)
	f.saveWorkflow(t, "flow", reviewStepsYAML)
	task := f.createTask(t, "flow")
	b, err := f.W.AdeTask.Board(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var br adewire.Branch
	for _, x := range b.Branches {
		if x.TaskID == task.ID && x.CodeRepoID == f.repoID {
			br = x
		}
	}
	if _, err := f.W.AdeTask.StartRun(ctx, adewire.StartRunArgs{TaskID: task.ID, BranchNames: map[string]string{br.ID: "feat/real-review"}}); err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	r := &reviewRig{adeFixture: f, task: task, br: br}
	testx.WaitUntil(t, testTimeout, func() bool {
		st := r.step(t)
		return st.Run != nil && st.Run.Review != nil && st.Run.Review.Phase == "waiting"
	})
	b, _ = f.W.AdeTask.Board(ctx)
	for _, x := range b.Branches {
		if x.TaskID == task.ID && x.CodeRepoID == f.repoID {
			r.br = x
		}
	}
	if _, err := f.W.AdeTask.OpenReviewWindow(ctx, adewire.BranchArgs{BranchID: r.br.ID}); err != nil {
		t.Fatal(err)
	}
	rec := f.WindowMgr.Opened[len(f.WindowMgr.Opened)-1]
	r.target, err = f.W.AdeTask.ReviewWindowTarget(ctx, adewire.WindowKeyArgs{WindowKey: rec.Key})
	if err != nil || r.target == nil {
		t.Fatalf("ReviewWindowTarget = %+v, %v", r.target, err)
	}
	gs := f.OpenGitStream()
	var init gitrpc.AppInitResult
	gs.MustRequest("app.init", nil, &init)
	var opened gitrpc.RepoOpenResult
	gs.MustRequest("repo.open", gitrpc.RepoOpenParams{Path: f.repo.Dir}, &opened)
	r.gs = gs
	return r
}

func (r *reviewRig) step(t *testing.T) adewire.ReviewStepState {
	t.Helper()
	st, err := r.W.AdeTask.ReviewStep(ctx, adewire.BranchArgs{BranchID: r.br.ID})
	if err != nil {
		t.Fatalf("ReviewStep: %v", err)
	}
	return st
}

func (r *reviewRig) comment(t *testing.T, line int, body, kind string) gitsession.CommentEntry {
	t.Helper()
	head, err := exec.Command("git", "-C", r.br.Worktree, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	var res gitrpc.ReviewCommentAddResult
	r.gs.MustRequest("review.comment.add", gitrpc.ReviewCommentAddParams{
		RepoID: r.target.GitRepoID, Branch: r.target.Branch, Path: "notes.txt", At: strings.TrimSpace(string(head)),
		Range: gitreview.LineRange{Start: line, End: line}, Body: body, Kind: kind,
	}, &res)
	return res.Comment
}

func (r *reviewRig) comments(t *testing.T) []gitsession.CommentEntry {
	t.Helper()
	var res gitsession.CommentListResult
	r.gs.MustRequest("review.comment.list", gitrpc.ReviewCommentListParams{RepoID: r.target.GitRepoID, Branch: r.target.Branch}, &res)
	return res.Comments
}

// sendAndWait sends a round and waits until it finished and the review waits again.
func (r *reviewRig) sendAndWait(t *testing.T, route string) adewire.ReviewRound {
	t.Helper()
	st := r.step(t)
	if err := r.W.AdeTask.SendReview(ctx, adewire.SendReviewArgs{RunID: st.Run.ID, Route: route}); err != nil {
		t.Fatalf("SendReview: %v", err)
	}
	var done adewire.ReviewStepState
	ok := false
	for deadline := time.Now().Add(testTimeout); time.Now().Before(deadline); time.Sleep(500 * time.Millisecond) {
		done = r.step(t)
		if done.Run != nil && done.Run.Review != nil && done.Run.Review.Phase == "waiting" && done.Round != nil && done.Round.Run.State != "running" {
			ok = true
			break
		}
	}
	if !ok {
		b, _ := json.Marshal(done)
		log := ""
		if done.Round != nil {
			log = r.roundLog(t, *done.Round)
		}
		t.Fatalf("round did not finish: %s\nlog:\n%s", b, log)
	}
	return *done.Round
}

func (r *reviewRig) roundLog(t *testing.T, round adewire.ReviewRound) string {
	return tail(r.runLog(t, round.Run.ID), 1500)
}

// TestAdeReviewStep sends review comments to the implementer's resumed session and then to a
// dedicated agent: the real claude must answer both comments through reply_review_comment.
func TestAdeReviewStep(t *testing.T) {
	r := startReview(t)
	change := r.comment(t, 1, "Rewrite this line as ONE in capitals and commit the change.", "change")
	question := r.comment(t, 2, "Why does this file have three lines? Answer in one sentence; edit nothing.", "question")

	impl := r.step(t).Candidates
	if len(impl) != 1 {
		t.Fatalf("candidates = %+v, want the implement run", impl)
	}
	round := r.sendAndWait(t, "implementer")
	if round.Run.State != "done" || round.Route != "implementer" || round.Fallback {
		t.Fatalf("round 1 = %+v; log:\n%s", round, r.roundLog(t, round))
	}
	for _, c := range r.comments(t) {
		if c.Reply == "" {
			t.Fatalf("comment %s has no reply after round 1: %+v; log:\n%s", c.ID, c, r.roundLog(t, round))
		}
	}
	got := map[string]gitsession.CommentEntry{}
	for _, c := range r.comments(t) {
		got[c.ID] = c
	}
	if got[change.ID].State != "addressed" {
		t.Logf("change comment not marked addressed (model choice): %+v", got[change.ID])
	}
	if got[question.ID].Reply == "" {
		t.Fatal("question unanswered")
	}
	implSession := r.claudeSession(t, round.Run.SessionID)

	var res gitrpc.ReviewCommentSetStateResult
	r.gs.MustRequest("review.comment.setState", gitrpc.ReviewCommentSetStateParams{
		RepoID: r.target.GitRepoID, Branch: r.target.Branch, ID: question.ID, State: "open",
	}, &res)
	second := r.sendAndWait(t, "new")
	if second.Run.State != "done" || second.Route != "new" || second.N != 2 {
		t.Fatalf("round 2 = %+v; log:\n%s", second, r.roundLog(t, second))
	}
	if s := r.claudeSession(t, second.Run.SessionID); s == "" || s == implSession {
		t.Fatalf("round 2 on claude session %q, want a dedicated one other than %q", s, implSession)
	}
	r.requireCostUnder(t)
}

// TestAdeReviewLost deletes the implementer's transcript: the resumed round fails to resume and the
// engine falls back to a dedicated agent that still answers.
func TestAdeReviewLost(t *testing.T) {
	r := startReview(t)
	st := r.step(t)
	impl := st.Candidates[0]
	b, _ := r.W.AdeTask.Board(ctx)
	var sess string
	for _, tk := range b.Tasks {
		for _, run := range tk.Runs {
			if run.ID == impl.RunID {
				sess = r.claudeSession(t, run.SessionID)
			}
		}
	}
	removeTranscript(t, r.Home, sess)
	c := r.comment(t, 1, "Is this line fine? Answer in one sentence; edit nothing.", "question")
	round := r.sendAndWait(t, "implementer")
	if !round.Fallback || round.Route != "new" || round.Run.State != "done" {
		t.Fatalf("round = %+v, want a done fallback on route new; log:\n%s", round, r.roundLog(t, round))
	}
	for _, x := range r.comments(t) {
		if x.ID == c.ID && x.Reply == "" {
			t.Fatalf("fallback round left the comment unanswered; log:\n%s", r.roundLog(t, round))
		}
	}
	r.requireCostUnder(t)
}
