package adeflow_test

import (
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness/fakehost"
)

// P509 Details links. Contract ade-details-links: tests/ui/ade-v2-details-links.spec.ts reads these.

const dlScenario = "ade-details-links"

var dlMask = flowharness.Mask("id", "createdAt", "updatedAt", "taskId")

func TestDetailsLinks(t *testing.T) {
	app := flowharness.New(t, flowharness.WithGitHost(fakehost.New()), flowharness.WithoutGh())
	task, err := app.W.AdeTask.CreateTask(ctx, adewire.CreateTaskArgs{Title: "Links"})
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
	stored := func(text, kind, ref, title string) adewire.Artifact {
		t.Helper()
		res := add(text)
		a := res.Artifact
		if res.Filed != "stored" || a == nil || a.Kind != kind || a.Ref != ref || a.Title != title || a.Source != "user" {
			t.Fatalf("AddLink(%q) = %+v (%+v), want stored %s ref %q title %q", text, res, a, kind, ref, title)
		}
		return *a
	}
	storedRows := func() int {
		n := 0
		for _, a := range artifactsOf(t, app, task.ID).Artifacts {
			if a.ID != "" {
				n++
			}
		}
		return n
	}

	if res := add("PAY-9"); res.Filed != "jira" || taskOf(t, board(t, app), task.ID).Jira.Key != "PAY-9" {
		t.Fatalf("PAY-9 = %+v, want the Jira slot", res)
	}
	if res := add("https://forge.test/acme/api/-/merge_requests/8"); res.Filed != "host" || res.Kind != "changeRequest" {
		t.Fatalf("host link = %+v, want the host slot", res)
	}
	stored("https://forge.test/acme/api/-/merge_requests/9", "changeRequest", "api!9", "api!9")
	stored("https://github.com/acme/api/tree/feat/x", "branch", "feat/x", "feat/x")
	docText := "Design notes https://docs.google.com/document/d/abc/edit"
	app.Contract(t, dlScenario, "args:AdeTaskService.AddLink", adewire.AddLinkArgs{TaskID: task.ID, Text: docText}, dlMask)
	docRes := add(docText)
	if docRes.Filed != "stored" || docRes.Artifact == nil || docRes.Kind != "doc" || docRes.Artifact.Title != "Design notes" {
		t.Fatalf("doc = %+v", docRes)
	}
	app.Contract(t, dlScenario, "AdeTaskService.AddLink", docRes, dlMask)
	doc := *docRes.Artifact
	runbook := stored("[Runbook](https://wiki.example/runbook)", "link", "", "Runbook")
	stored("https://jira.example.com/browse/PAY-12", "issue", "PAY-12", "PAY-12")

	n := storedRows()
	if res := add("PAY-9"); res.Filed != "known" || storedRows() != n {
		t.Fatalf("repeat key = %+v, rows %d -> %d, want known and no write", res, n, storedRows())
	}
	for _, bad := range []string{"just some words", "https://a.example https://b.example"} {
		if _, err := app.W.AdeTask.AddLink(ctx, adewire.AddLinkArgs{TaskID: task.ID, Text: bad}); errCode(err) != "E_BAD_REQUEST" {
			t.Fatalf("AddLink(%q) = %v, want E_BAD_REQUEST", bad, err)
		}
	}
	if storedRows() != n {
		t.Fatal("a refused line wrote a row")
	}

	edit := func(args adewire.EditLinkArgs) adewire.LinkResult {
		t.Helper()
		args.TaskID = task.ID
		res, err := app.W.AdeTask.EditLink(ctx, args)
		if err != nil {
			t.Fatalf("EditLink(%+v) = %v", args, err)
		}
		return res
	}
	res := edit(adewire.EditLinkArgs{Slot: "stored", ID: doc.ID, Text: "Design spec https://docs.google.com/document/d/abc/edit"})
	if res.Artifact == nil || res.Artifact.Title != "Design spec" || res.Artifact.URL != doc.URL {
		t.Fatalf("retitle = %+v", res)
	}
	runbookEdit := adewire.EditLinkArgs{TaskID: task.ID, Slot: "stored", ID: runbook.ID, Text: "https://wiki.example/runbook-v2"}
	app.Contract(t, dlScenario, "args:AdeTaskService.EditLink", runbookEdit, dlMask)
	res = edit(runbookEdit)
	if res.Artifact == nil || res.Artifact.URL != "https://wiki.example/runbook-v2" || res.Artifact.Title != "Runbook" {
		t.Fatalf("new url = %+v, want the title kept", res)
	}
	app.Contract(t, dlScenario, "AdeTaskService.EditLink", res, dlMask)
	if res = edit(adewire.EditLinkArgs{Slot: "stored", ID: runbook.ID, Kind: "doc"}); res.Artifact == nil || res.Artifact.Kind != "doc" {
		t.Fatalf("recategorise = %+v", res)
	}
	if _, err := app.W.AdeTask.EditLink(ctx, adewire.EditLinkArgs{TaskID: task.ID, Slot: "stored", ID: runbook.ID, Text: doc.URL}); errCode(err) != "E_BAD_REQUEST" {
		t.Fatalf("duplicate url = %v, want E_BAD_REQUEST", err)
	}

	edit(adewire.EditLinkArgs{Slot: "jira", Text: "PAY-10"})
	if got := taskOf(t, board(t, app), task.ID).Jira.Key; got != "PAY-10" {
		t.Fatalf("jira slot = %q, want PAY-10", got)
	}
	before := storedRows()
	if res = edit(adewire.EditLinkArgs{Slot: "host", Text: "https://wiki.example/x"}); res.Filed != "stored" || res.Artifact == nil || res.Artifact.Kind != "link" || res.Artifact.Title != "wiki.example/x" {
		t.Fatalf("host refile = %+v, want a stored link titled by host and path", res)
	}
	if tk := taskOf(t, board(t, app), task.ID); tk.HostURL != "" || storedRows() != before+1 {
		t.Fatalf("host slot %q, rows %d -> %d, want the slot cleared and one row added", tk.HostURL, before, storedRows())
	}

	cr, ok := findArtifact(artifactsOf(t, app, task.ID), "changeRequest", "api!9")
	if !ok {
		t.Fatal("stored change request missing")
	}
	if _, err := app.W.AdeTask.EditLink(ctx, adewire.EditLinkArgs{TaskID: task.ID, Slot: "stored", ID: "not-a-row", Text: "x " + cr.URL}); errCode(err) != "E_BAD_REQUEST" {
		t.Fatalf("unknown id = %v, want E_BAD_REQUEST", err)
	}
	app.Contract(t, dlScenario, "AdeTaskService.Artifacts", artifactsOf(t, app, task.ID), dlMask)

	// Edits to what the Jira or host slot already shows are refused; a repeated bare key is known.
	if res := add("https://forge.test/acme/api/-/merge_requests/8"); res.Filed != "host" {
		t.Fatalf("host slot refill = %+v", res)
	}
	for _, text := range []string{"PAY-10", "https://jira.example.com/browse/PAY-10", "https://forge.test/acme/api/-/merge_requests/8"} {
		if _, err := app.W.AdeTask.EditLink(ctx, adewire.EditLinkArgs{TaskID: task.ID, Slot: "stored", ID: cr.ID, Text: text}); errCode(err) != "E_BAD_REQUEST" {
			t.Fatalf("edit to slot link %q = %v, want E_BAD_REQUEST", text, err)
		}
	}
	if res := add("PAY-20"); res.Filed != "stored" {
		t.Fatalf("bare key = %+v, want stored", res)
	}
	n = storedRows()
	if res := add("PAY-20"); res.Filed != "known" || storedRows() != n {
		t.Fatalf("repeat bare key = %+v, rows %d -> %d, want known and no write", res, n, storedRows())
	}
}
