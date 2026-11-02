package githostflow_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness/fakeacli"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness/fakeaz"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness/fakehost"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/githost"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/githost/azure"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/issuetracker/jira"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/oplog"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeclock"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
)

// Contract host-view-comments: tests/ui/host-view-comments.spec.ts reads these.
const hcScenario = "host-view-comments"

func hcOpen(t *testing.T, app *flowharness.App, url string) string {
	t.Helper()
	o, err := app.W.HostView.Open(ctx, bridge.HostViewOpenArgs{URL: url})
	if err != nil || !o.Opened {
		t.Fatalf("Open(%s) = %+v, %v", url, o, err)
	}
	return o.Key
}

// hcBadRequest fails unless err is a bad request containing want.
func hcBadRequest(t *testing.T, err error, label, want string) {
	t.Helper()
	var ie *ipcerr.Error
	if err == nil || !errors.As(err, &ie) || ie.Code != string(ipcerr.CodeBadRequest) || !strings.Contains(err.Error(), want) {
		t.Errorf("%s: err = %v, want a bad request containing %q", label, err, want)
	}
}

func hcPost(t *testing.T, app *flowharness.App, a bridge.HostViewPostArgs) bridge.HostViewPosted {
	t.Helper()
	got, err := app.W.HostView.PostComment(ctx, a)
	if err != nil {
		t.Fatalf("PostComment(%+v) = %v", a, err)
	}
	return got
}

// Posting to the PR view over the in-process fake host: compose caps, a post, a reply, the refetch, the breaker and the window close. Backend half of
// host-view-comments.spec.ts.
func TestHostViewCommentsForge(t *testing.T) {
	forge := fakehost.New()
	forge.AddChangeRequest(githost.ChangeRequest{
		Number: 12, Title: "Rework auth", State: "open", HeadRef: "feat/auth", BaseRef: "main", HeadOwner: "acme",
		URL: "https://forge.test/acme/api/-/merge_requests/12", UpdatedAt: 1_700_000_300_000,
	})
	forge.SetAuthor(12, "ann")
	forge.SetDetail(12, hvDetailFixture())
	clock := fakeclock.New(time.Now())
	app := flowharness.New(t, flowharness.WithGitHost(forge), flowharness.WithoutGh(), flowharness.WithClock(clock))
	const link = "https://forge.test/acme/api/-/merge_requests/12"
	key := hcOpen(t, app, link)

	// 1. The read result says what the composer may do.
	got := hvDetail(t, app, link)
	if got.Kind != bridge.HostViewOK || got.Compose == nil || !slices.Equal(got.Compose.ReplyKinds, []string{"comment", "review", "code"}) {
		t.Fatalf("ChangeRequest compose = %+v", got.Compose)
	}
	app.Contract(t, hcScenario, "HostViewService.ChangeRequest#composeForge", got)

	// 2. A post reaches the host as body and reply target only.
	pArgs := bridge.HostViewPostArgs{WindowKey: key, Body: "Looks good"}
	app.Contract(t, hcScenario, "args:HostViewService.PostComment", pArgs, flowharness.Mask("windowKey"))
	res := hcPost(t, app, pArgs)
	if res.Kind != bridge.HostViewOK {
		t.Fatalf("PostComment = %+v", res)
	}
	app.Contract(t, hcScenario, "HostViewService.PostComment#ok", res)
	posted := forge.Posted(12)
	if len(posted) != 1 || posted[0] != (fakehost.PostedComment{Body: "Looks good"}) {
		t.Fatalf("posted = %+v", posted)
	}

	// 3. A reply targets a thread; an unknown thread is data.
	if res := hcPost(t, app, bridge.HostViewPostArgs{WindowKey: key, Body: "Thanks", ReplyTo: "t1"}); res.Kind != bridge.HostViewOK {
		t.Fatalf("reply = %+v", res)
	}
	if p := forge.Posted(12); len(p) != 2 || p[1].ReplyTo != "t1" {
		t.Fatalf("posted after reply = %+v", p)
	}
	bad := hcPost(t, app, bridge.HostViewPostArgs{WindowKey: key, Body: "Lost", ReplyTo: "nope"})
	if bad.Kind != bridge.HostViewUnavailable || bad.Status == nil || bad.Status.Reason != "no such thread" {
		t.Fatalf("reply to an unknown thread = %+v", bad)
	}
	app.Contract(t, hcScenario, "HostViewService.PostComment#unavailable", bad)

	// 4. A refetch shows both comments.
	after := hvDetail(t, app, link)
	var ids []string
	for _, th := range after.Detail.Threads {
		for _, c := range th.Comments {
			ids = append(ids, th.ID+"/"+c.ID)
		}
	}
	if !slices.Contains(ids, "posted-1/posted-1") || !slices.Contains(ids, "t1/posted-2") {
		t.Fatalf("thread comments after the posts = %v", ids)
	}

	// 5. A rate limit arms the breaker: the next post does not reach the host.
	forge.SetRateLimited(time.Now().Add(time.Hour))
	forge.ResetCalls()
	for range 2 {
		if r := hcPost(t, app, bridge.HostViewPostArgs{WindowKey: key, Body: "again"}); r.Kind != bridge.HostViewUnavailable || r.Status == nil || !r.Status.RateLimited {
			t.Fatalf("limited post = %+v", r)
		}
	}
	if n := forge.Count("post:"); n != 1 {
		t.Fatalf("host reached %d times while limited, want once", n)
	}
	clock.Advance(2 * time.Hour)
	forge.SetRateLimited(time.Time{})

	// 6. Guards.
	_, err := app.W.HostView.PostComment(ctx, bridge.HostViewPostArgs{WindowKey: key, Body: "  \n"})
	hcBadRequest(t, err, "empty body", "Write a comment")
	_, err = app.W.HostView.PostComment(ctx, bridge.HostViewPostArgs{WindowKey: key, Body: strings.Repeat("a", githost.MaxHostCommentBytes+1)})
	hcBadRequest(t, err, "long body", "at most")
	_, err = app.W.HostView.PostComment(ctx, bridge.HostViewPostArgs{WindowKey: key, Body: "x", ReplyTo: strings.Repeat("a", 257)})
	hcBadRequest(t, err, "long reply target", "reply target")
	_, err = app.W.HostView.PostComment(ctx, bridge.HostViewPostArgs{WindowKey: "no-such-window", Body: "x"})
	hcBadRequest(t, err, "unknown window", "can’t post")
}

// Posting over az: text only, through the CLI; az is never asked for a token. Backend half of host-view-comments.spec.ts.
func TestHostViewCommentsAzure(t *testing.T) {
	az := fakeaz.New()
	az.SetDefaults("https://dev.azure.com/acme", "Web App")
	az.AddPR(azKey, fakeaz.PR{
		ID: 12, Title: "PROJ-7 Add login", Status: "active", Src: "feature/login", Dst: "main", Head: strings.Repeat("a", 40),
		Created: day(3), Author: "ann@example.com", AuthorName: "Ann Lee",
		Threads: []fakeaz.Thread{{ID: 1, Status: "active", Comments: []fakeaz.ThreadComment{
			{ID: 1, Author: "Bob Ray", Login: "bob@example.com", Body: "Add tests.", Published: "2026-05-03T11:00:00Z", LastUpdate: "2026-05-03T11:00:00Z"}}}},
	})
	clock := &azClock{now: time.Now()}
	app := flowharness.New(t, flowharness.WithoutGh(),
		flowharness.WithAzure(azure.Options{Locator: az, Runner: az, Clock: clock}))
	key := hcOpen(t, app, azPR12)

	// 1. A post is a thread through the CLI.
	got := hvDetail(t, app, azPR12)
	if got.Compose == nil || !slices.Equal(got.Compose.ReplyKinds, []string{"comment", "code"}) {
		t.Fatalf("compose = %+v", got.Compose)
	}
	az.ResetCalls()
	res := hcPost(t, app, bridge.HostViewPostArgs{WindowKey: key, Body: "see this"})
	if res.Kind != bridge.HostViewOK {
		t.Fatalf("PostComment = %+v", res)
	}
	app.Contract(t, hcScenario, "HostViewService.PostComment#azure", res)
	if az.Count("invoke:pullRequestThreads:"+azKey) != 1 {
		t.Fatalf("calls = %v", az.Calls())
	}
	if threads := az.Threads(12); len(threads) != 1 || threads[0] != "see this" {
		t.Fatalf("threads = %q", threads)
	}

	// 2. A reply goes into the thread under its first comment.
	if r := hcPost(t, app, bridge.HostViewPostArgs{WindowKey: key, Body: "Done", ReplyTo: "1"}); r.Kind != bridge.HostViewOK {
		t.Fatalf("reply = %+v", r)
	}
	if rep := az.Replies(12); len(rep) != 1 || rep[0] != (fakeaz.Reply{ThreadID: 1, ParentCommentID: 1, Content: "Done"}) {
		t.Fatalf("replies = %+v", rep)
	}
	if az.Count("invoke:pullRequestThreadComments:"+azKey+":1") != 1 {
		t.Fatalf("calls = %v", az.Calls())
	}

	// 3. A token request is an unsupported call, so Violations catches it.
	if v := az.Violations(); len(v) != 0 {
		t.Fatalf("az contract violations: %v", v)
	}
}

func hcGhCalls(app *flowharness.App) int {
	files, _ := filepath.Glob(filepath.Join(app.FakeDir, "gh-*.args"))
	return len(files)
}

func hcLastGhArgs(t *testing.T, app *flowharness.App) []string {
	t.Helper()
	files, _ := filepath.Glob(filepath.Join(app.FakeDir, "gh-*.args"))
	n := func(p string) int {
		v, _ := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(filepath.Base(p), "gh-"), ".args"))
		return v
	}
	sort.Slice(files, func(i, j int) bool { return n(files[i]) < n(files[j]) })
	if len(files) == 0 {
		t.Fatal("gh was never called")
	}
	raw, err := os.ReadFile(files[len(files)-1])
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(string(raw), "\n")
}

// Posting over gh: REST for a comment, GraphQL for a reply on a code thread.
func TestHostViewCommentsGitHub(t *testing.T) {
	fixture, err := filepath.Abs("../../githost/github/testdata/pr_detail.json")
	if err != nil {
		t.Fatal(err)
	}
	app := flowharness.New(t)
	emit := func(name, body string) string { return writeFile(t, filepath.Join(app.Work, name), body) }
	app.Scenario(fakeagent.Scenario{Gh: []fakeagent.GhRule{
		{Match: "--version", Emit: emit("version.txt", "gh version 2.42.0 (2024-01-08)\n")},
		{Match: "auth status", Emit: emit("auth.txt", "github.com\n  Logged in to github.com account octocat (keyring)\n")},
		{Match: "latestOpinionatedReviews", Emit: fixture},
		{Match: "issues/7/comments", Emit: emit("comment.json", `{"id":1}`)},
		{Match: "addPullRequestReviewThreadReply", Emit: emit("reply.json", `{"data":{"addPullRequestReviewThreadReply":{"comment":{"id":"C_1"}}}}`)},
	}})
	const link = "https://github.com/acme/api/pull/7"
	key := hcOpen(t, app, link)

	// 1. Compose: reply kinds only.
	got := hvDetail(t, app, link)
	if got.Compose == nil || !slices.Equal(got.Compose.ReplyKinds, []string{"code"}) {
		t.Fatalf("compose = %+v", got.Compose)
	}
	app.Contract(t, hcScenario, "HostViewService.ChangeRequest#composeGitHub", got)

	// 2. A comment is a REST call; the body travels via -f.
	if r := hcPost(t, app, bridge.HostViewPostArgs{WindowKey: key, Body: "hi @alice"}); r.Kind != bridge.HostViewOK {
		t.Fatalf("post = %+v", r)
	}
	args := hcLastGhArgs(t, app)
	if !slices.Contains(args, "repos/acme/api/issues/7/comments") {
		t.Fatalf("gh args = %q", args)
	}
	i := slices.Index(args, "-f")
	if i < 0 || args[i+1] != "body=hi @alice" || slices.Contains(args, "-F") {
		t.Fatalf("gh args = %q", args)
	}

	// 3. A reply on a code thread is GraphQL.
	if r := hcPost(t, app, bridge.HostViewPostArgs{WindowKey: key, Body: "ok then", ReplyTo: "PRRT_abc"}); r.Kind != bridge.HostViewOK {
		t.Fatalf("reply = %+v", r)
	}
	args = hcLastGhArgs(t, app)
	joined := strings.Join(args, "\n")
	if !strings.Contains(joined, "addPullRequestReviewThreadReply") || !strings.Contains(joined, "\nthreadId=PRRT_abc") ||
		!strings.Contains(joined, "\nbody=ok then") || slices.Contains(args, "-F") {
		t.Fatalf("gh args = %q", args)
	}

	// 4. A thread id that is not a review thread never spawns gh.
	calls := hcGhCalls(app)
	bad := hcPost(t, app, bridge.HostViewPostArgs{WindowKey: key, Body: "x", ReplyTo: "IC_kwDOabc"})
	if bad.Kind != bridge.HostViewUnavailable || bad.Status == nil || bad.Status.Reason != "no such thread" || hcGhCalls(app) != calls {
		t.Fatalf("reply to a comment thread = %+v, gh calls %d -> %d", bad, calls, hcGhCalls(app))
	}
}

// Posting to the issue view over acli: Markdown becomes ADF in a temp file, never argv.
func TestIssueComments(t *testing.T) {
	acli := fakeacli.New(ivSite)
	acli.SetIssue("PROJ-12", fakeacli.IssueJSON("PROJ-12", "Export report as CSV", "In Progress", "indeterminate", 0))
	acli.SetIssueDetail("PROJ-12", ivTestdata(t, "issue-detail.json"))
	clock := fakeclock.New(time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC))
	app := flowharness.New(t, flowharness.WithGitHost(fakehost.New()), flowharness.WithoutGh(), flowharness.WithClock(clock),
		flowharness.WithIssueTracker(jira.Options{Locator: acli, Runner: acli, Clock: clock}))
	key := hcOpen(t, app, ivLink)
	ivPrefetched(t, acli, 1)

	// 1. Compose: no replies.
	got := ivIssue(t, app, ivLink)
	if got.Compose == nil || len(got.Compose.ReplyKinds) != 0 || got.Compose.MaxBodyBytes != 32<<10 {
		t.Fatalf("compose = %+v", got.Compose)
	}
	app.Contract(t, hcScenario, "HostViewService.Issue#compose", got)

	// 2. The body lands as ADF in a file that is gone again.
	res := hcPost(t, app, bridge.HostViewPostArgs{WindowKey: key, Body: "**Done** see https://x.test"})
	if res.Kind != bridge.HostViewOK {
		t.Fatalf("post = %+v", res)
	}
	app.Contract(t, hcScenario, "HostViewService.PostComment#issue", res)
	posted := acli.Posted("PROJ-12")
	if len(posted) != 1 {
		t.Fatalf("posted = %+v", posted)
	}
	var doc struct {
		Type    string `json:"type"`
		Version int    `json:"version"`
	}
	body := posted[0].Body
	if json.Unmarshal([]byte(body), &doc) != nil || doc.Type != "doc" || doc.Version != 1 ||
		!strings.Contains(body, `{"type":"strong"}`) || !strings.Contains(body, `"href":"https://x.test"`) {
		t.Fatalf("ADF body = %s", body)
	}
	if _, err := os.Stat(posted[0].Path); !os.IsNotExist(err) {
		t.Fatalf("temp body file still there: %v", err)
	}

	// 3. The op log row holds the file flag, not the text.
	rows, err := app.W.Ops.Recent(bridge.OpsRecentArgs{Limit: 200})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range rows {
		if r.Category == oplog.CategoryTracker && r.Command != nil && strings.Contains(*r.Command, "workitem comment") {
			found = true
			if !strings.Contains(*r.Command, "--body-file") || strings.Contains(*r.Command, "Done") {
				t.Fatalf("comment command = %q", *r.Command)
			}
		}
	}
	if !found {
		t.Fatalf("no workitem comment row in %d rows", len(rows))
	}

	// 4. Replies are refused.
	_, err = app.W.HostView.PostComment(ctx, bridge.HostViewPostArgs{WindowKey: key, Body: "x", ReplyTo: "1"})
	hcBadRequest(t, err, "reply", "no replies")

	// 5. Signed out is data; an unknown key is gone.
	acli.SetLoggedIn(false)
	clock.Advance(6 * time.Minute)
	if r := hcPost(t, app, bridge.HostViewPostArgs{WindowKey: key, Body: "x"}); r.Kind != bridge.HostViewUnavailable || r.Reason != "unauthenticated" {
		t.Fatalf("signed out post = %+v", r)
	}
	acli.SetLoggedIn(true)
	clock.Advance(6 * time.Minute)
	gone := hcOpen(t, app, "https://"+ivSite+"/browse/PROJ-99")
	if r := hcPost(t, app, bridge.HostViewPostArgs{WindowKey: gone, Body: "x"}); r.Kind != bridge.HostViewGone || r.Reason != "notFound" {
		t.Fatalf("unknown key post = %+v", r)
	}
}
