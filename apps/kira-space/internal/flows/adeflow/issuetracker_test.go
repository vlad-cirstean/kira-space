package adeflow_test

import (
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness/fakeacli"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness/fakehost"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/githost"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/issuetracker/jira"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/oplog"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/agentmcp"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// P417 Jira through acli. Contract ade-issue-tracker: tests/ui/ade-v2-issue-tracker.spec.ts reads these.

const (
	itSite   = "acme.atlassian.net"
	itBranch = "feat/KIRA-12-login"
	itPR7    = "https://forge.test/acme/api/-/merge_requests/7"
)

func setLink(t *testing.T, app *flowharness.App, taskID, key, url string) {
	t.Helper()
	if _, err := app.W.AdeTask.UpdateTask(ctx, adewire.UpdateTaskArgs{TaskID: taskID, Patch: adewire.TaskPatch{
		Jira: &adewire.Jira{Key: key, URL: url},
	}}); err != nil {
		t.Fatal(err)
	}
}

func issueOf(t *testing.T, app *flowharness.App, taskID string) adewire.TaskIssue {
	t.Helper()
	res, err := app.W.IssueTracker.TaskIssue(ctx, adewire.TaskArgs{TaskID: taskID})
	if err != nil {
		t.Fatalf("TaskIssue: %v", err)
	}
	return res
}

func gapOf(res adewire.ArtifactsResult, source string) (adewire.ArtifactGap, bool) {
	for _, g := range res.Gaps {
		if g.Source == source && g.Repo == "" {
			return g, true
		}
	}
	return adewire.ArtifactGap{}, false
}

func TestTaskIssueTracker(t *testing.T) {
	forge := fakehost.New()
	acli := fakeacli.New(itSite)
	acli.SetIssue("KIRA-12", fakeacli.IssueJSON("KIRA-12", "Add the thing", "In Review", "indeterminate", 4))
	finish := fakeagent.MCPCall{Server: agentmcp.ServerName, Tool: "finish_step", Args: map[string]any{"status": "done", "summary": "all done"}}
	f := newRunFixtureWith(t, []fakeagent.Action{{Sh: rvImplement, MCP: []fakeagent.MCPCall{finish}, Name: "nofinish"}},
		agentStep("one", "")+manualStep,
		flowharness.WithGitHost(forge), flowharness.WithoutGh(), flowharness.WithTrackerGrace(time.Second),
		flowharness.WithIssueTracker(jira.Options{Locator: acli, Runner: acli}))
	app := f.app
	gitOut(t, f.repo.Dir, "remote", "set-url", "origin", "https://forge.test/acme/api.git")

	// The task's own PR (branch carries the key, title does not), a PR in another repo whose title
	// carries the key (not the task's: it must not show), and a PR whose title names a longer key.
	forge.AddChangeRequest(githost.ChangeRequest{Number: 7, Title: "Fix login", State: "open", HeadRef: itBranch, HeadOwner: "acme", BaseRef: "main", URL: itPR7})
	forge.AddChangeRequest(githost.ChangeRequest{Number: 20, Title: "KIRA-12 groundwork", State: "merged", URL: "https://forge.test/acme/lib/-/merge_requests/20"})
	forge.AddChangeRequest(githost.ChangeRequest{Number: 30, Title: "KIRA-123 unrelated", State: "open", URL: "https://forge.test/acme/lib/-/merge_requests/30"})
	if _, err := app.W.Repos.GitHostChangeRequests.UpsertChangeRequests("forge|forge.test|acme/api", []model.GitHostChangeRequest{
		{Number: 7, HeadRef: itBranch, HeadOwner: "acme", State: "open", Title: "Fix login", URL: itPR7, UpdatedAt: 3},
	}); err != nil {
		t.Fatal(err)
	}
	setLink(t, app, f.taskID, "KIRA-12", "https://"+itSite+"/browse/KIRA-12")
	f.start(t, itBranch)
	waitRun(t, app, f.taskID, "one", "done")

	// 1. Issue row; the only PR is the task's own, from the host. A title match elsewhere does not show.
	res := artifactsOf(t, app, f.taskID)
	issue, ok := findArtifact(res, "issue", "Add the thing")
	if !ok || issue.Source != "jira" || issue.Status != "In Review" || issue.Ref != "KIRA-12" || issue.URL != "https://"+itSite+"/browse/KIRA-12" {
		t.Fatalf("issue = %+v in %+v", issue, res.Artifacts)
	}
	prs := map[string]adewire.Artifact{}
	for _, a := range res.Artifacts {
		if a.Kind == "changeRequest" {
			if _, dup := prs[a.URL]; dup {
				t.Fatalf("PR %s listed twice", a.URL)
			}
			prs[a.URL] = a
		}
	}
	if len(prs) != 1 || prs[itPR7].Source != "host" {
		t.Fatalf("PRs = %+v, want only the task's own", prs)
	}
	if _, ok := prs["https://forge.test/acme/lib/-/merge_requests/20"]; ok {
		t.Fatalf("title match outside the task listed: %+v", prs)
	}
	if g, ok := gapOf(res, "jira"); ok {
		t.Fatalf("jira gap = %+v, want none", g)
	}
	app.Contract(t, "ade-issue-tracker", "AdeTaskService.Artifacts", res, artMask)

	// 2. The task tab line.
	args := adewire.TaskArgs{TaskID: f.taskID}
	app.Contract(t, "ade-issue-tracker", "args:IssueTrackerService.TaskIssue", args)
	ti := issueOf(t, app, f.taskID)
	if ti.Reason != "" || ti.Title != "Add the thing" || ti.Status != "In Review" || ti.StatusCategory != "indeterminate" ||
		ti.Label != "Jira" || !ti.Summary.Known || ti.Summary.Count != 4 {
		t.Fatalf("TaskIssue = %+v", ti)
	}
	app.Contract(t, "ade-issue-tracker", "IssueTrackerService.TaskIssue", ti)

	// 3. A second listing inside the cache window does not call acli again.
	views := acli.Count("view:")
	artifactsOf(t, app, f.taskID)
	issueOf(t, app, f.taskID)
	if acli.Count("view:") != views || views != 1 {
		t.Fatalf("acli views = %d (was %d), want one cached read", acli.Count("view:"), views)
	}

	// 4. The finishing reply lists no jira gap.
	_, list := mcpReply(t, app, "mcp-finish_step-1.json")
	for _, g := range list.Gaps {
		if g.Source == "jira" {
			t.Fatalf("finish_step gaps = %+v, want no jira gap", list.Gaps)
		}
	}

	// 5. A slow acli leaves the static row and a timeout gap inside the 5 s budget; the detached fetch
	// finishes and the next listing has the title.
	slow := createTask(t, app, "Slow", "flow")
	setLink(t, app, slow.ID, "KIRA-60", "")
	acli.SetIssue("KIRA-60", fakeacli.IssueJSON("KIRA-60", "Slow thing", "To Do", "new", 0))
	release := acli.Hold()
	t.Cleanup(release)
	began := time.Now()
	res = artifactsOf(t, app, slow.ID)
	if took := time.Since(began); took > 8*time.Second {
		t.Fatalf("listing took %v, want the 5 s source budget", took)
	}
	if a, ok := findArtifact(res, "issue", "KIRA-60"); !ok || a.Source != "jira" || a.Status != "" {
		t.Fatalf("static row = %+v in %+v", a, res.Artifacts)
	}
	if g, ok := gapOf(res, "jira"); !ok || g.Reason != "timeout" {
		t.Fatalf("gaps = %+v, want timeout", res.Gaps)
	}
	release()
	testx.WaitUntil(t, waitFor, func() bool {
		_, ok := findArtifact(artifactsOf(t, app, slow.ID), "issue", "Slow thing")
		return ok
	})
	if acli.Count("view:KIRA-60") != 1 {
		t.Fatalf("slow issue read %d times, want once", acli.Count("view:KIRA-60"))
	}
	if g, ok := gapOf(artifactsOf(t, app, slow.ID), "jira"); ok {
		t.Fatalf("gap %+v after a count of 0", g)
	}

	// 6. Reasons: signed out, not installed, no such issue, another site.
	tools := func() { // drop acli's cached probe, as Settings' Refresh does
		if _, err := app.W.IssueTracker.Tools(ctx, bridge.ToolsArgs{Refresh: true}); err != nil {
			t.Fatal(err)
		}
	}
	gone := createTask(t, app, "Gone", "flow")
	setLink(t, app, gone.ID, "KIRA-71", "")
	other := createTask(t, app, "Other", "flow")
	setLink(t, app, other.ID, "KIRA-70", "https://other.atlassian.net/browse/KIRA-70")
	for _, c := range []struct {
		task, reason string
	}{{gone.ID, "notFound"}, {other.ID, "otherSite"}} {
		if g, ok := gapOf(artifactsOf(t, app, c.task), "jira"); !ok || g.Reason != c.reason {
			t.Fatalf("task %s gap = %+v, want %s", c.task, g, c.reason)
		}
		if r := issueOf(t, app, c.task).Reason; r != c.reason {
			t.Fatalf("TaskIssue reason = %q, want %s", r, c.reason)
		}
	}
	signedOut := createTask(t, app, "Signed out", "flow")
	setLink(t, app, signedOut.ID, "KIRA-72", "")
	acli.SetLoggedIn(false)
	tools()
	if g, ok := gapOf(artifactsOf(t, app, signedOut.ID), "jira"); !ok || g.Reason != "unauthenticated" {
		t.Fatalf("signed out gap = %+v", g)
	}
	if ti := issueOf(t, app, signedOut.ID); ti.Reason != "unauthenticated" || ti.Key != "KIRA-72" {
		t.Fatalf("signed out TaskIssue = %+v", ti)
	}
	app.Contract(t, "ade-issue-tracker", "IssueTrackerService.TaskIssue#unauthenticated", issueOf(t, app, signedOut.ID))
	acli.SetLoggedIn(true)
	acli.SetInstalled(false)
	tools()
	if ti := issueOf(t, app, signedOut.ID); ti.Reason != "unavailable" {
		t.Fatalf("not installed TaskIssue = %+v", ti)
	}
	acli.SetInstalled(true)
	tools()

	// 7. A task without a link has no tracker.
	if ti := issueOf(t, app, createTask(t, app, "Plain", "flow").ID); ti.Reason != "noTracker" {
		t.Fatalf("TaskIssue without a link = %+v", ti)
	}

	// 8. Every acli call is a background row of the op log; the failed sign-in probe of step 6 shows as an error.
	rows, err := app.W.Ops.Recent(bridge.OpsRecentArgs{Limit: 200})
	if err != nil {
		t.Fatal(err)
	}
	ops := map[string]string{}
	for _, r := range rows {
		if r.Category != oplog.CategoryTracker {
			continue
		}
		if !r.Background || r.Source != "Jira" || r.Command == nil || !strings.HasPrefix(*r.Command, "acli ") {
			t.Fatalf("tracker row = %+v, command %v", r, *r.Command)
		}
		if r.Status == oplog.StatusError {
			ops[r.Kind+"!"] = *r.Command
		}
		ops[r.Kind] = *r.Command
	}
	for _, kind := range []string{"version", "auth status", "workitem view", "auth status!"} {
		if ops[kind] == "" {
			t.Fatalf("no %q tracker row in %+v", kind, ops)
		}
	}
	if !strings.Contains(ops["workitem view"], "workitem view KIRA-") {
		t.Fatalf("view command = %q", ops["workitem view"])
	}
}
