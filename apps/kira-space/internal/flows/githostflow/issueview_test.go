package githostflow_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness/fakeacli"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness/fakehost"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/issuetracker/jira"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeclock"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
)

// Contract issue-view: tests/ui/issue-view.spec.ts reads these.
const ivScenario = "issue-view"

const (
	ivSite = "acme.atlassian.net"
	ivLink = "https://acme.atlassian.net/browse/PROJ-12"
)

func ivTestdata(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "issuetracker", "jira", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func ivIssue(t *testing.T, app *flowharness.App, ref string) bridge.HostViewIssue {
	t.Helper()
	got, err := app.W.HostView.Issue(ctx, bridge.HostViewIssueArgs{Ref: ref, Fresh: true})
	if err != nil {
		t.Fatalf("Issue(%s) = %v", ref, err)
	}
	return got
}

// ivPrefetched waits until the window open's background read has reached acli, so later steps do not
// race it.
func ivPrefetched(t *testing.T, acli *fakeacli.Acli, want int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for acli.Count("detail:PROJ-12") < want {
		if time.Now().After(deadline) {
			t.Fatalf("detail reads = %d, want %d", acli.Count("detail:PROJ-12"), want)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func ivBadRequest(t *testing.T, err error, label string) {
	t.Helper()
	var ie *ipcerr.Error
	if err == nil || !strings.Contains(err.Error(), label) || !errors.As(err, &ie) || ie.Code != string(ipcerr.CodeBadRequest) {
		t.Errorf("err = %v, want a bad request naming %q", err, label)
	}
}

const ivDescription = "## Goal\n\n" +
	"Users export the **monthly report** as CSV. See [the spec](https://example.com/spec) and run `kira export`. \\<script\\>alert(1)\\</script\\>\n\n" +
	"- Columns\n  - date\n  - total\n- Header row\n\n" +
	"```sh\necho `date`\nkira export --csv\n```\n\n" +
	"| Field | Type |\n| --- | --- |\n| a\\|b | text |\n\n" +
	"[@Dana Dev](#kira-mention) please check [https://example.com/design](https://example.com/design)\n\n" +
	"Review state: [IN REVIEW](#kira-status-blue)\n\n" +
	"> [!NOTE]\n> Export runs nightly.\n\n" +
	"![mockup.png](https://acme.atlassian.net/rest/api/3/attachment/content/10100)"

// The issue view over the in-process fake acli: open or focus the window, read the detail (comments
// inline, from the comment list, or omitted), and report every tracker failure as data. Backend half of
// issue-view.spec.ts.
func TestIssueView(t *testing.T) {
	forge := fakehost.New()
	acli := fakeacli.New(ivSite)
	acli.SetIssue("PROJ-12", fakeacli.IssueJSON("PROJ-12", "Export report as CSV", "In Progress", "indeterminate", 2))
	acli.SetIssueDetail("PROJ-12", ivTestdata(t, "issue-detail.json"))
	clock := fakeclock.New(time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC))
	app := flowharness.New(t, flowharness.WithGitHost(forge), flowharness.WithoutGh(), flowharness.WithClock(clock),
		flowharness.WithIssueTracker(jira.Options{Locator: acli, Runner: acli, Clock: clock}))
	wm := app.WindowMgr

	// 1-2. A link opens a window titled by key; the bare key finds the same one through the signed-in site.
	opened, err := app.W.HostView.Open(ctx, bridge.HostViewOpenArgs{URL: ivLink + "?focusedCommentId=10001"})
	if err != nil || !opened.Opened || !strings.HasPrefix(opened.Key, bridge.HostWindowKeyPrefix) || wm.Titles[opened.Key] != "PROJ-12 · Jira" {
		t.Fatalf("Open = %+v, %v; titles %v", opened, err, wm.Titles)
	}
	wm.Known[opened.Key] = true
	ivPrefetched(t, acli, 1)
	again, err := app.W.HostView.Open(ctx, bridge.HostViewOpenArgs{URL: "PROJ-12"})
	if err != nil || again.Opened || again.Key != opened.Key || !slices.Equal(wm.Focused, []string{opened.Key}) {
		t.Fatalf("Open(key) = %+v, %v; focused %v", again, err, wm.Focused)
	}

	// 3. The window target.
	target, err := app.W.HostView.WindowTarget(ctx, bridge.HostViewWindowArgs{WindowKey: opened.Key})
	if err != nil || target == nil || target.Kind != bridge.HostViewKindIssue || target.Key != "PROJ-12" || target.URL != ivLink ||
		target.Tracker == nil || target.Tracker.Label != "Jira" || target.Provider != "jira" || target.Host != nil {
		t.Fatalf("WindowTarget = %+v, %v", target, err)
	}
	app.Contract(t, ivScenario, "HostViewService.WindowTarget#issue", target)

	// 4. The detail: fields, the ADF description as Markdown, three comments of five, the PR count.
	app.Contract(t, ivScenario, "args:HostViewService.Issue", bridge.HostViewIssueArgs{Ref: target.URL})
	got := ivIssue(t, app, target.URL)
	d := got.Detail
	if got.Kind != bridge.HostViewOK || d == nil || d.Title != "Export report as CSV" || d.Status != "In Progress" ||
		d.StatusCategory != "indeterminate" || d.Type != "Story" || d.Priority != "High" || d.Assignee != nil ||
		d.Reporter == nil || d.Reporter.Name != "Rita Reporter" || d.Parent == nil || d.Parent.Key != "PROJ-1" ||
		!slices.Equal(d.Labels, []string{"backend", "export"}) || !slices.Equal(d.Components, []string{"Reports", "API"}) {
		t.Fatalf("Issue = %+v", got)
	}
	if d.Description != ivDescription {
		t.Fatalf("description:\n%s\nwant:\n%s", d.Description, ivDescription)
	}
	if len(d.Comments) != 3 || d.Comments[0].ID != "10001" || d.Comments[2].ID != "10003" || d.CommentsTotal != 5 || !d.CommentsTruncated {
		t.Fatalf("comments = %+v total %d truncated %v", d.Comments, d.CommentsTotal, d.CommentsTruncated)
	}
	if !d.Summary.Known || d.Summary.Count != 2 {
		t.Fatalf("summary = %+v", d.Summary)
	}
	if len(d.Attachments) != 2 || d.Attachments[0].Filename != "spec.pdf" || d.Attachments[1].ID != "10100" ||
		!strings.HasPrefix(d.Attachments[1].URL, "https://") || d.Stale || d.FetchedAt != clock.Now().UnixMilli() {
		t.Fatalf("attachments = %+v stale %v fetchedAt %d", d.Attachments, d.Stale, d.FetchedAt)
	}
	app.Contract(t, ivScenario, "HostViewService.Issue#ok", got)

	// 5. Comments over the output cap come from `comment list`; when that fails too they are omitted.
	acli.SetTooLarge("PROJ-12", true)
	acli.SetComments("PROJ-12", ivTestdata(t, "comment-list.json"))
	got = ivIssue(t, app, target.URL)
	if got.Kind != bridge.HostViewOK || got.Detail == nil || len(got.Detail.Comments) != 2 || got.Detail.Comments[1].ID != "10009" ||
		got.Detail.CommentsOmitted || acli.Count("comments:") != 1 {
		t.Fatalf("fallback Issue = %+v, comment lists %d", got, acli.Count("comments:"))
	}
	app.Contract(t, ivScenario, "HostViewService.Issue#commentsFallback", got)
	acli.SetComments("PROJ-12", "")
	got = ivIssue(t, app, target.URL)
	if got.Kind != bridge.HostViewOK || got.Detail == nil || !got.Detail.CommentsOmitted || len(got.Detail.Comments) != 0 {
		t.Fatalf("omitted Issue = %+v", got)
	}
	app.Contract(t, ivScenario, "HostViewService.Issue#commentsOmitted", got)
	acli.SetTooLarge("PROJ-12", false)

	// 6. An issue the site does not know.
	got = ivIssue(t, app, "https://"+ivSite+"/browse/PROJ-99")
	if got.Kind != bridge.HostViewGone || got.Detail != nil || got.Reason != "notFound" {
		t.Fatalf("missing Issue = %+v", got)
	}
	app.Contract(t, ivScenario, "HostViewService.Issue#gone", got)

	// 7. Another site, a signed-out CLI, then no CLI.
	got = ivIssue(t, app, "https://other.atlassian.net/browse/PROJ-12")
	if got.Kind != bridge.HostViewUnavailable || got.Reason != "otherSite" {
		t.Fatalf("other site Issue = %+v", got)
	}
	acli.SetLoggedIn(false)
	clock.Advance(6 * time.Minute)
	got = ivIssue(t, app, ivLink)
	if got.Kind != bridge.HostViewUnavailable || got.Reason != "unauthenticated" || got.Detail != nil {
		t.Fatalf("signed out Issue = %+v", got)
	}
	app.Contract(t, ivScenario, "HostViewService.Issue#unauthenticated", got)
	acli.SetInstalled(false)
	clock.Advance(6 * time.Minute)
	if got = ivIssue(t, app, ivLink); got.Kind != bridge.HostViewUnavailable || got.Reason != "unavailable" {
		t.Fatalf("no CLI Issue = %+v", got)
	}

	// 8. Refusals name both things the view opens.
	_, err = app.W.HostView.Open(ctx, bridge.HostViewOpenArgs{URL: "hello"})
	ivBadRequest(t, err, "Forge MR link")
	ivBadRequest(t, err, "Jira work item key or link")
	_, err = app.W.HostView.Issue(ctx, bridge.HostViewIssueArgs{Ref: "https://forge.test/acme/api/-/merge_requests/12"})
	ivBadRequest(t, err, "Jira work item key or link")
	_, err = app.W.HostView.Issue(ctx, bridge.HostViewIssueArgs{Ref: ivLink + "?x=" + strings.Repeat("a", 2100)})
	ivBadRequest(t, err, "Jira work item key or link")
	_, err = app.W.HostView.Open(ctx, bridge.HostViewOpenArgs{URL: ivLink + "?x=" + strings.Repeat("a", 2100)})
	ivBadRequest(t, err, "Forge MR link")

	// 9. A change request link still opens a change request window.
	cr, err := app.W.HostView.Open(ctx, bridge.HostViewOpenArgs{URL: "https://forge.test/acme/api/-/merge_requests/12"})
	if err != nil || !cr.Opened {
		t.Fatalf("Open(MR) = %+v, %v", cr, err)
	}
	ct, err := app.W.HostView.WindowTarget(ctx, bridge.HostViewWindowArgs{WindowKey: cr.Key})
	if err != nil || ct == nil || ct.Kind != bridge.HostViewKindChangeRequest || ct.Host == nil || ct.Host.Label != "Forge" || ct.Tracker != nil {
		t.Fatalf("MR WindowTarget = %+v, %v", ct, err)
	}
}
