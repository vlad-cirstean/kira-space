package githostflow_test

import (
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness/fakeacli"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness/fakehost"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/issuetracker/jira"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeclock"
)

// Contract issue-view-fetch: tests/ui/issue-view-fetch.spec.ts reads these.
const ivfScenario = "issue-view-fetch"

func ivIssueArgs(t *testing.T, app *flowharness.App, fresh bool) bridge.HostViewIssue {
	t.Helper()
	got, err := app.W.HostView.Issue(ctx, bridge.HostViewIssueArgs{Ref: ivLink, Fresh: fresh})
	if err != nil {
		t.Fatalf("Issue(fresh=%v) = %v", fresh, err)
	}
	return got
}

// The issue read path: the window open prefetches, one acli call serves a read (the Development field
// rides in it), a saved copy answers without acli and goes stale, a fresh read replaces it, a post
// drops it, and an overflow reads the comment list beside the view. Backend half of
// issue-view-fetch.spec.ts.
func TestIssueViewFetch(t *testing.T) {
	acli := fakeacli.New(ivSite)
	acli.SetIssue("PROJ-12", fakeacli.IssueJSON("PROJ-12", "Export report as CSV", "In Progress", "indeterminate", 2))
	detail := ivTestdata(t, "issue-detail.json")
	acli.SetIssueDetail("PROJ-12", detail)
	clock := fakeclock.New(time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC))
	app := flowharness.New(t, flowharness.WithGitHost(fakehost.New()), flowharness.WithoutGh(), flowharness.WithClock(clock),
		flowharness.WithIssueTracker(jira.Options{Locator: acli, Runner: acli, Clock: clock}))

	// 1. The link opens a window and its read starts before any Issue call.
	opened, err := app.W.HostView.Open(ctx, bridge.HostViewOpenArgs{URL: ivLink})
	if err != nil || !opened.Opened {
		t.Fatalf("Open = %+v, %v", opened, err)
	}
	ivPrefetched(t, acli, 1)

	// 2. The window's own read joins or finds it: no second detail read. The site's first read also
	// discovers the Development field with one summary view.
	app.Contract(t, ivfScenario, "args:HostViewService.Issue", bridge.HostViewIssueArgs{Ref: ivLink, Fresh: true})
	got := ivIssueArgs(t, app, false)
	if got.Kind != bridge.HostViewOK || got.Detail == nil || got.Detail.Stale || acli.Count("detail:PROJ-12") != 1 || acli.Count("view:PROJ-12") != 1 {
		t.Fatalf("first Issue = %+v, detail reads %d, summary views %d", got, acli.Count("detail:PROJ-12"), acli.Count("view:PROJ-12"))
	}
	app.Contract(t, ivfScenario, "HostViewService.Issue#first", got)

	// 3. A fresh read is one acli call: the Development field is in the detail field list.
	got = ivIssueArgs(t, app, true)
	if got.Detail == nil || !got.Detail.Summary.Known || got.Detail.Summary.Count != 2 || acli.Count("detail:PROJ-12") != 2 || acli.Count("view:PROJ-12") != 1 {
		t.Fatalf("fresh Issue = %+v, detail reads %d, summary views %d", got, acli.Count("detail:PROJ-12"), acli.Count("view:PROJ-12"))
	}

	// 4. Past the fresh window the saved copy answers without acli, marked stale; fresh=true replaces it.
	clock.Advance(2 * time.Minute)
	got = ivIssueArgs(t, app, false)
	if got.Detail == nil || !got.Detail.Stale || acli.Count("detail:PROJ-12") != 2 || got.Detail.Title != "Export report as CSV" {
		t.Fatalf("stale Issue = %+v, detail reads %d", got, acli.Count("detail:PROJ-12"))
	}
	app.Contract(t, ivfScenario, "HostViewService.Issue#stale", got)
	acli.SetIssueDetail("PROJ-12", strings.Replace(detail, `"summary": "Export report as CSV"`, `"summary": "Export report as CSV v2"`, 1))
	got = ivIssueArgs(t, app, true)
	if got.Detail == nil || got.Detail.Stale || got.Detail.Title != "Export report as CSV v2" || acli.Count("detail:PROJ-12") != 3 {
		t.Fatalf("revalidated Issue = %+v, detail reads %d", got, acli.Count("detail:PROJ-12"))
	}
	app.Contract(t, ivfScenario, "HostViewService.Issue#fresh", got)

	// 5. A posted comment drops the saved copy, so the next plain read goes to acli.
	res := hcPost(t, app, bridge.HostViewPostArgs{WindowKey: opened.Key, Body: "Thanks"})
	if res.Kind != bridge.HostViewOK {
		t.Fatalf("post = %+v", res)
	}
	got = ivIssueArgs(t, app, false)
	if got.Detail == nil || got.Detail.Stale || acli.Count("detail:PROJ-12") != 4 {
		t.Fatalf("Issue after post = %+v, detail reads %d", got, acli.Count("detail:PROJ-12"))
	}

	// 6. Comments over the output cap come from `comment list`, read beside the view.
	acli.SetTooLarge("PROJ-12", true)
	acli.SetComments("PROJ-12", ivTestdata(t, "comment-list.json"))
	got = ivIssueArgs(t, app, true)
	if got.Detail == nil || len(got.Detail.Comments) != 2 || got.Detail.CommentsOmitted || acli.Count("comments:") != 1 {
		t.Fatalf("overflow Issue = %+v, comment lists %d", got, acli.Count("comments:"))
	}
}
