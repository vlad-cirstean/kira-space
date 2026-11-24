package githostflow_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness/fakeaz"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/githost"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/githost/azure"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
)

// P751 PR, merge request and Jira links open in the system browser. Contract github-links-browser:
// tests/ui/github-links-browser.spec.ts reads these.
const glScenario = "github-links-browser"

var glMask = flowharness.Mask("id", "createdAt", "updatedAt", "taskId")

func TestGitHubLinksOpenInBrowser(t *testing.T) {
	az := fakeaz.New()
	az.SetDefaults("https://dev.azure.com/acme", "Web App")
	app := flowharness.New(t, flowharness.WithAzure(azure.Options{Locator: az, Runner: az}))
	app.Scenario(fakeagent.Scenario{Gh: []fakeagent.GhRule{
		{Match: "--version", Emit: writeFile(t, filepath.Join(app.Work, "version.txt"), "gh version 2.42.0 (2024-01-08)\n")},
		{Match: "auth status", Emit: writeFile(t, filepath.Join(app.Work, "auth.txt"), "github.com\n  Logged in to github.com account octocat (keyring)\n")},
	}})
	task, err := app.W.AdeTask.CreateTask(ctx, adewire.CreateTaskArgs{Title: "GitHub links"})
	if err != nil {
		t.Fatal(err)
	}
	links := []struct {
		url   string
		kind  string
		filed string
	}{
		{"https://github.com/acme/api/pull/12/changes", "changeRequest", "host"},
		{"https://github.com/acme/api/pull/13/files/abc..def", "changeRequest", "stored"},
		{"https://github.com/acme/api/pull/14/commits/abc123", "changeRequest", "stored"},
		{"https://github.com/acme/api/pull/15/", "changeRequest", "stored"},
		{"https://dev.azure.com/acme/Web%20App/_git/api/pullrequest/7", "changeRequest", "stored"},
		{"https://jira.example.com/browse/PAY-9", "issue", "jira"},
	}
	for _, l := range links {
		res, err := app.W.AdeTask.AddLink(ctx, adewire.AddLinkArgs{TaskID: task.ID, Text: l.url})
		if err != nil || res.Filed != l.filed || res.Kind != l.kind {
			t.Fatalf("AddLink(%s) = %+v, %v; want %s %s", l.url, res, err, l.filed, l.kind)
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
		Jira     *adewire.Jira `json:"jira"`
		HostURL  string        `json:"hostUrl"`
		HostLink *githost.Link `json:"hostLink"`
	}{tk.Jira, tk.HostURL, tk.HostLink}, glMask)
	arts, err := app.W.AdeTask.Artifacts(ctx, adewire.TaskArgs{TaskID: task.ID})
	if err != nil {
		t.Fatal(err)
	}
	app.Contract(t, glScenario, "AdeTaskService.Artifacts", arts, glMask)

	// Each card opens its stored URL, the normalised form of what was pasted; the Jira pill opens the
	// slot's URL.
	stored := map[string]string{}
	for _, art := range arts.Artifacts {
		stored[art.Ref] = art.URL
	}
	if tk.Jira == nil || tk.Jira.URL == "" || tk.HostURL == "" {
		t.Fatalf("slots missing in %+v", tk)
	}
	opens := []struct{ name, url string }{
		{"pr12", tk.HostURL},
		{"pr13", stored["api#13"]},
		{"pr14", stored["api#14"]},
		{"pr15", stored["api#15"]},
		{"azure", stored["api!7"]},
		{"jira", tk.Jira.URL},
	}
	var urls []string
	for _, o := range opens {
		if o.url == "" {
			t.Fatalf("no stored url for %s in %+v", o.name, arts.Artifacts)
		}
		args := bridge.LinkOpenExternalArgs{URL: o.url}
		app.Contract(t, glScenario, "args:LinkService.OpenExternal#"+o.name, args)
		if err := app.W.Link.OpenExternal(args); err != nil {
			t.Fatalf("OpenExternal(%s) = %v", o.url, err)
		}
		urls = append(urls, o.url)
	}
	if got := app.Browser.Opened(); !slices.Equal(got, urls) {
		t.Fatalf("browser opened %v, want %v", got, urls)
	}
	if len(app.WindowMgr.Opened) != 0 {
		t.Fatalf("windows opened %+v, want none", app.WindowMgr.Opened)
	}
}

func writeFile(t *testing.T, path, content string) string {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
