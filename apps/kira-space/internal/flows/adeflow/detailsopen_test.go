package adeflow_test

import (
	"slices"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness/fakehost"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/githost"
)

// P586 Details links open in the system browser. Contract ade-details-open:
// tests/ui/ade-v2-details-open.spec.ts reads these.

const doScenario = "ade-details-open"

func TestDetailsOpenExternal(t *testing.T) {
	app := flowharness.New(t, flowharness.WithGitHost(fakehost.New()), flowharness.WithoutGh())
	task, err := app.W.AdeTask.CreateTask(ctx, adewire.CreateTaskArgs{Title: "Open links"})
	if err != nil {
		t.Fatal(err)
	}
	add := func(text string) adewire.LinkResult {
		t.Helper()
		res, err := app.W.AdeTask.AddLink(ctx, adewire.AddLinkArgs{TaskID: task.ID, Text: text})
		if err != nil {
			t.Fatalf("AddLink(%q) = %v", text, err)
		}
		return res
	}
	const (
		jiraURL = "https://jira.example.com/browse/PAY-9"
		hostURL = "https://forge.test/acme/api/-/merge_requests/8"
		crURL   = "https://forge.test/acme/api/-/merge_requests/9"
		docURL  = "https://docs.google.com/document/d/abc/edit"
	)
	if res := add(jiraURL); res.Filed != "jira" {
		t.Fatalf("jira link = %+v, want the Jira slot", res)
	}
	if res := add(hostURL); res.Filed != "host" || res.Kind != "changeRequest" {
		t.Fatalf("host link = %+v, want the host slot", res)
	}
	if res := add(crURL); res.Filed != "stored" || res.Kind != "changeRequest" {
		t.Fatalf("change request = %+v, want stored", res)
	}
	if res := add("Design notes " + docURL); res.Filed != "stored" || res.Kind != "doc" {
		t.Fatalf("doc = %+v, want stored doc", res)
	}
	if res := add("PAY-20"); res.Filed != "stored" || res.Kind != "issue" || res.Artifact == nil || res.Artifact.URL != "" {
		t.Fatalf("bare key = %+v, want a stored issue without url", res)
	}

	tk := taskOf(t, board(t, app), task.ID)
	if tk.Jira == nil || tk.Jira.URL != jiraURL || tk.HostURL != hostURL {
		t.Fatalf("slots = jira %+v host %q, want %s and %s", tk.Jira, tk.HostURL, jiraURL, hostURL)
	}
	app.Contract(t, doScenario, "Task#links", struct {
		Jira     *adewire.Jira `json:"jira"`
		HostURL  string        `json:"hostUrl"`
		HostLink *githost.Link `json:"hostLink"`
	}{tk.Jira, tk.HostURL, tk.HostLink}, dlMask)
	arts := artifactsOf(t, app, task.ID)
	app.Contract(t, doScenario, "AdeTaskService.Artifacts", arts, dlMask)

	urlOf := func(kind string) string {
		for _, a := range arts.Artifacts {
			if a.Kind == kind && a.Source == "user" && a.URL != "" {
				return a.URL
			}
		}
		t.Fatalf("no stored %s artifact with a url in %+v", kind, arts.Artifacts)
		return ""
	}
	open := []struct{ key, url string }{
		{"jira", tk.Jira.URL},
		{"host", tk.HostURL},
		{"changeRequest", urlOf("changeRequest")},
		{"doc", urlOf("doc")},
	}
	want := make([]string, 0, len(open))
	for _, o := range open {
		args := bridge.LinkOpenExternalArgs{URL: o.url}
		app.Contract(t, doScenario, "args:LinkService.OpenExternal#"+o.key, args)
		if err := app.W.Link.OpenExternal(args); err != nil {
			t.Fatalf("OpenExternal(%s) = %v", o.url, err)
		}
		want = append(want, o.url)
	}
	if got := app.Browser.Opened(); !slices.Equal(got, want) {
		t.Fatalf("browser opened %v, want %v", got, want)
	}
}
