package githostflow_test

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
)

// Contract github-pr-open: tests/ui/github-pr-open.spec.ts runs the same scenario against the view.
// The detail rule matches the query's own aliases, so a query GitHub would reject as invalid (the
// field-merge conflicts the aliases avoid) finds no rule and the flow fails.
func TestOpenGitHubPullRequest(t *testing.T) {
	const scenario = "github-pr-open"
	fixture, err := filepath.Abs("../../githost/github/testdata/pr_detail.json")
	if err != nil {
		t.Fatal(err)
	}
	app := flowharness.New(t)
	app.Scenario(fakeagent.Scenario{Gh: []fakeagent.GhRule{
		{Match: "--version", Emit: writeFile(t, filepath.Join(app.Work, "version.txt"), "gh version 2.42.0 (2024-01-08)\n")},
		{Match: "auth status", Emit: writeFile(t, filepath.Join(app.Work, "auth.txt"), "github.com\n  Logged in to github.com account octocat (keyring)\n")},
		{Match: "teamName: name", Emit: fixture},
	}})

	opened, err := app.W.HostView.Open(ctx, bridge.HostViewOpenArgs{URL: "https://github.com/acme/api/pull/7"})
	if err != nil || !opened.Opened || app.WindowMgr.Titles[opened.Key] != "PR #7 · acme/api" {
		t.Fatalf("Open = %+v, %v; titles %v", opened, err, app.WindowMgr.Titles)
	}

	target, err := app.W.HostView.WindowTarget(ctx, bridge.HostViewWindowArgs{WindowKey: opened.Key})
	if err != nil || target == nil || target.Provider != "github" || target.Number != 7 || target.Repo != "acme/api" {
		t.Fatalf("WindowTarget = %+v, %v", target, err)
	}
	app.Contract(t, scenario, "HostViewService.WindowTarget", target)

	got := hvDetail(t, app, target.URL)
	if got.Kind != bridge.HostViewOK || got.Detail == nil || len(got.Detail.Threads) == 0 {
		t.Fatalf("ChangeRequest = %+v, want ok with threads", got)
	}
	var team bool
	for _, r := range got.Detail.Reviewers {
		if r.Team && r.Person.Login == "platform" && r.Person.Name == "Platform" {
			team = true
		}
	}
	if !team {
		t.Fatalf("reviewers = %+v, want team platform/Platform", got.Detail.Reviewers)
	}
	app.Contract(t, scenario, "HostViewService.ChangeRequest", got)

	// The window's "Open on GitHub".
	args := bridge.OpenChangeRequestURLArgs{URL: target.URL}
	if err := app.W.GitHost.OpenChangeRequestURL(args); err != nil {
		t.Fatal(err)
	}
	if b := app.Browser.Opened(); !slices.Equal(b, []string{target.URL}) {
		t.Fatalf("browser opened %v, want [%s]", b, target.URL)
	}
	app.Contract(t, scenario, "args:GitHostService.OpenChangeRequestURL", args)
}
