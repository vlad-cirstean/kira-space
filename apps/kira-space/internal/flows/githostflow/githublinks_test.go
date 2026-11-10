package githostflow_test

import (
	"path/filepath"
	"strconv"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/githost"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
)

// P594 GitHub PR links open in the app. Contract github-links-in-app:
// tests/ui/github-links-in-app.spec.ts reads these.
const glScenario = "github-links-in-app"

var glMask = flowharness.Mask("id", "createdAt", "updatedAt", "taskId")

func TestGitHubLinksInApp(t *testing.T) {
	app := flowharness.New(t)
	app.Scenario(fakeagent.Scenario{Gh: []fakeagent.GhRule{
		{Match: "--version", Emit: writeFile(t, filepath.Join(app.Work, "version.txt"), "gh version 2.42.0 (2024-01-08)\n")},
		{Match: "auth status", Emit: writeFile(t, filepath.Join(app.Work, "auth.txt"), "github.com\n  Logged in to github.com account octocat (keyring)\n")},
	}})
	task, err := app.W.AdeTask.CreateTask(ctx, adewire.CreateTaskArgs{Title: "GitHub links"})
	if err != nil {
		t.Fatal(err)
	}
	links := []struct {
		number int
		url    string
		filed  string
	}{
		{12, "https://github.com/acme/api/pull/12/changes", "host"},
		{13, "https://github.com/acme/api/pull/13/files/abc..def", "stored"},
		{14, "https://github.com/acme/api/pull/14/commits/abc123", "stored"},
		{15, "https://github.com/acme/api/pull/15/", "stored"},
	}
	for _, l := range links {
		res, err := app.W.AdeTask.AddLink(ctx, adewire.AddLinkArgs{TaskID: task.ID, Text: l.url})
		if err != nil || res.Filed != l.filed || res.Kind != "changeRequest" {
			t.Fatalf("AddLink(%s) = %+v, %v; want %s changeRequest", l.url, res, err, l.filed)
		}
	}
	b, err := app.W.AdeTask.Board(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var tk adewire.Task
	for _, x := range b.Tasks {
		if x.ID == task.ID {
			tk = x
		}
	}
	app.Contract(t, glScenario, "Task#links", struct {
		HostURL  string        `json:"hostUrl"`
		HostLink *githost.Link `json:"hostLink"`
	}{tk.HostURL, tk.HostLink}, glMask)
	arts, err := app.W.AdeTask.Artifacts(ctx, adewire.TaskArgs{TaskID: task.ID})
	if err != nil {
		t.Fatal(err)
	}
	app.Contract(t, glScenario, "AdeTaskService.Artifacts", arts, glMask)

	var first string
	for _, l := range links {
		args := bridge.HostViewOpenArgs{URL: l.url}
		opened, err := app.W.HostView.Open(ctx, args)
		if err != nil || !opened.Opened {
			t.Fatalf("Open(%s) = %+v, %v", l.url, opened, err)
		}
		if title := app.WindowMgr.Titles[opened.Key]; title != "PR #"+itoa(l.number)+" · acme/api" {
			t.Fatalf("Open(%s) title = %q", l.url, title)
		}
		target, err := app.W.HostView.WindowTarget(ctx, bridge.HostViewWindowArgs{WindowKey: opened.Key})
		if err != nil || target == nil {
			t.Fatalf("WindowTarget(%s) = %+v, %v", opened.Key, target, err)
		}
		if want := "https://github.com/acme/api/pull/" + itoa(l.number); target.URL != want {
			t.Fatalf("target of %s = %q, want %s", l.url, target.URL, want)
		}
		app.Contract(t, glScenario, "args:HostViewService.Open#"+itoa(l.number), args)
		app.Contract(t, glScenario, "HostViewService.WindowTarget#"+itoa(l.number), target)
		if first == "" {
			first = opened.Key
		}
	}

	again, err := app.W.HostView.Open(ctx, bridge.HostViewOpenArgs{URL: "https://github.com/acme/api/pull/12"})
	if err != nil || again.Opened || again.Key != first {
		t.Fatalf("Open(pull/12) = %+v, %v; want the focused window %s", again, err, first)
	}
	hvRefused(t, app, "https://github.com/acme/api/pull/12/foo", "GitHub PR link")
	hvRefused(t, app, "https://github.com/acme/api/issues/12", "GitHub PR link")
}

func itoa(n int) string { return strconv.Itoa(n) }
