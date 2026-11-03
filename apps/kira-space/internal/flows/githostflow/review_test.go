package githostflow_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness/fakehost"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/githost"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitreview"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitrpc"
	"github.com/kirathecat/kira-studio/internal/terminal"
	"github.com/kirathecat/kira-studio/internal/testx"
)

func TestChangeRequestReviewActions(t *testing.T) {
	forge := fakehost.New()
	r := newRig(t, forge)
	sha := flowharness.Replace(r.tip, strings.Repeat("a", 40))
	const scenario = "ade-review-actions"

	state := func() gitrpc.ChangeRequestReviewStateResult {
		return call[gitrpc.ChangeRequestReviewStateResult](t, r.gs, "changeRequest.reviewState",
			gitrpc.ChangeRequestReviewStateParams{RepoID: r.gitID, Branch: branch})
	}
	approve := func() gitrpc.ChangeRequestPostResult {
		return call[gitrpc.ChangeRequestPostResult](t, r.gs, "changeRequest.submitReview",
			gitrpc.ChangeRequestSubmitReviewParams{RepoID: r.gitID, Number: 7, Event: githost.ReviewApprove, Body: "LGTM\r\n", HeadSha: r.tip})
	}

	forge.SetAuthor(7, "alice")
	got := state()
	if got.Kind != "ok" || got.ChangeRequestReview == nil {
		t.Fatalf("reviewState = %+v", got)
	}
	rv := got.ChangeRequestReview
	if rv.Number != 7 || rv.Host.ID != "forge" || rv.State != "open" || rv.HeadSha != r.tip || rv.LocalHeadSha != r.tip ||
		rv.Author != "alice" || rv.Viewer != "forger" || rv.Own {
		t.Fatalf("reviewState ok payload = %+v", rv)
	}
	r.app.Contract(t, scenario, "git:changeRequest.reviewState", got, sha)

	posted := approve()
	if posted.Kind != "ok" {
		t.Fatalf("submitReview = %+v", posted)
	}
	want := []githost.Review{{Event: githost.ReviewApprove, Body: "LGTM", CommitSHA: r.tip}}
	if reviews := forge.Reviews(7); !reflect.DeepEqual(reviews, want) {
		t.Fatalf("Reviews = %+v, want %+v", reviews, want)
	}
	r.app.Contract(t, scenario, "git:changeRequest.submitReview", posted)

	commented := call[gitrpc.ChangeRequestPostResult](t, r.gs, "changeRequest.comment",
		gitrpc.ChangeRequestCommentParams{RepoID: r.gitID, Number: 7, Body: "@alice thanks"})
	if commented.Kind != "ok" {
		t.Fatalf("comment = %+v", commented)
	}
	if comments := forge.Comments(7); !reflect.DeepEqual(comments, []string{"@alice thanks"}) {
		t.Fatalf("Comments = %v", comments)
	}
	r.app.Contract(t, scenario, "git:changeRequest.comment", commented)

	lineParams := gitrpc.ChangeRequestLineCommentParams{
		RepoID: r.gitID, Number: 7, Path: "a.txt", Range: gitreview.LineRange{Start: 1, End: 2}, HeadSha: r.tip, Body: "tighten this",
	}
	r.app.Contract(t, scenario, "args:changeRequest.lineComment", lineParams, sha)
	lined := call[gitrpc.ChangeRequestLineCommentResult](t, r.gs, "changeRequest.lineComment", lineParams)
	if lined.Kind != "ok" {
		t.Fatalf("lineComment = %+v", lined)
	}
	wantLine := []githost.LineComment{{Path: "a.txt", Start: 1, End: 2, EndColumn: 2, CommitSHA: r.tip, Body: "tighten this"}}
	if got := forge.LineComments(7); !reflect.DeepEqual(got, wantLine) {
		t.Fatalf("LineComments = %+v, want %+v", got, wantLine)
	}
	r.app.Contract(t, scenario, "git:changeRequest.lineComment", lined)

	pastEnd := lineParams
	pastEnd.Range.End = 3
	if err := r.gs.Request("changeRequest.lineComment", pastEnd, nil); flowharness.ErrorCode(err) == "" {
		t.Fatalf("lineComment past the end of the file = %v, want a bad request", err)
	}
	if n := len(forge.LineComments(7)); n != 1 {
		t.Fatalf("LineComments after refusal = %d, want 1", n)
	}

	forge.SetAuthor(7, "forger")
	own := state()
	if own.Kind != "ok" || own.ChangeRequestReview == nil || !own.Own {
		t.Fatalf("own reviewState = %+v", own)
	}
	r.app.Contract(t, scenario, "git:changeRequest.reviewState#own", own, sha)
	refused := approve()
	if refused.Kind != "unavailable" || refused.Status == nil || refused.Status.Reason != "you cannot approve your own merge request" {
		t.Fatalf("own submitReview = %+v", refused)
	}
	r.app.Contract(t, scenario, "git:changeRequest.submitReview#own", refused)
	if n := len(forge.Reviews(7)); n != 1 {
		t.Fatalf("Reviews after refusal = %d, want 1", n)
	}

	forge.SetAuthor(7, "alice")
	hostHead := strings.Repeat("b", 40)
	forge.SetHead(7, hostHead)
	stale := state()
	if stale.Kind != "ok" || stale.ChangeRequestReview == nil {
		t.Fatalf("stale reviewState = %+v", stale)
	}
	if sv := stale.ChangeRequestReview; sv.HeadSha != hostHead || sv.LocalHeadSha != r.tip || sv.Own {
		t.Fatalf("stale payload = %+v", sv)
	}
	r.app.Contract(t, scenario, "git:changeRequest.reviewState#stale", stale, sha)

	moved := call[gitrpc.ChangeRequestLineCommentResult](t, r.gs, "changeRequest.lineComment", lineParams)
	if moved.Kind != "headMoved" || moved.HeadSha != hostHead {
		t.Fatalf("lineComment on a moved head = %+v, want headMoved at %s", moved, hostHead)
	}
	if n := len(forge.LineComments(7)); n != 1 {
		t.Fatalf("LineComments after headMoved = %d, want 1", n)
	}
	r.app.Contract(t, scenario, "git:changeRequest.lineComment#headMoved", moved)
}

// reviewAgentPrompt launches the task's review agent and returns its argv, which carries the first message.
func (r *rig) reviewAgentPrompt() string {
	r.t.Helper()
	res, err := r.app.W.AdeTask.LaunchReviewAgent(ctx, adewire.TaskArgs{TaskID: r.taskID()})
	if err != nil {
		r.t.Fatalf("LaunchReviewAgent: %v", err)
	}
	l := res.Launch
	if _, err := r.app.W.Terminal.Open(terminal.OpenArgs{
		TerminalID: l.TerminalID, Cwd: l.Cwd, Cols: 100, Rows: 30, WindowKey: "review", Command: l.Command, LaunchKind: terminal.LaunchKindAgent,
	}); err != nil {
		r.t.Fatalf("Terminal.Open: %v", err)
	}
	var prompt string
	testx.WaitUntil(r.t, waitFor, func() bool {
		files, _ := filepath.Glob(filepath.Join(r.app.FakeDir, "*.args"))
		for _, f := range files {
			if raw, _ := os.ReadFile(f); strings.Contains(string(raw), "I review these branches") {
				prompt = string(raw)
				return true
			}
		}
		return false
	})
	return prompt
}

func TestReviewAgentLinksChangeRequest(t *testing.T) {
	const want = "https://forge.test/acme/r/-/merge_requests/7"
	line := "worktree"

	t.Run("branch lookup", func(t *testing.T) {
		r := newRig(t, fakehost.New())
		if p := r.reviewAgentPrompt(); !strings.Contains(p, ", PR "+want+"\n") {
			t.Fatalf("prompt lacks the PR link after the repo line:\n%s", p)
		}
	})

	t.Run("no host, launch still works", func(t *testing.T) {
		r := newRig(t, fakehost.New())
		r.setProvider("off")
		p := r.reviewAgentPrompt()
		if strings.Contains(p, "PR ") || !strings.Contains(p, line) {
			t.Fatalf("prompt = %s, want the repo line without a PR link", p)
		}
	})
}
