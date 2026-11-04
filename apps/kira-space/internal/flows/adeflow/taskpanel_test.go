package adeflow_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness/fakehost"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/githost"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
	"github.com/kirathecat/kira-studio/internal/shell"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// P501 task panel. Contract ade-task-panel: tests/ui/ade-v2-task-panel.spec.ts reads these. Session ids
// stay visible in the contract as stable <id:N> placeholders.

const (
	tpScenario = "ade-task-panel"
	tpRunbook  = "https://wiki.example/runbook"
	tpOther    = "https://wiki.example/other"
	tpMR       = "https://forge.test/acme/api/-/merge_requests/8"
)

var tpMask = flowharness.Mask("id", "createdAt", "updatedAt", "taskId", "ref")

func draftOf(hs []adewire.Handover) (adewire.Handover, bool) {
	for _, h := range hs {
		if h.State == "draft" {
			return h, true
		}
	}
	return adewire.Handover{}, false
}

func TestTaskPanelOrigins(t *testing.T) {
	dir := t.TempDir()
	flag := func(n string) string { return filepath.Join(dir, n) }
	forge := fakehost.New()
	forge.AddChangeRequest(githost.ChangeRequest{
		Number: 7, Title: "Fix login", State: "open", HeadRef: "feat/api-work", HeadOwner: "acme", BaseRef: "main", URL: artPRURL,
	})
	forge.SetChecksState(7, "FAILURE")
	app := flowharness.New(t, flowharness.WithGitHost(forge), flowharness.WithoutGh(), flowharness.WithTrackerGrace(time.Second))
	// Call 1 is run `one`; 2 the first work session; 3 its resume.
	claude(app, map[string][]fakeagent.Action{"*": {
		{Name: "done"},
		{MCP: []fakeagent.MCPCall{
			handoverCall("Form wired.", false),
			reportCall(artItem("link", "Runbook", tpRunbook, "")),
		}, WaitFile: flag("hold1")},
		{MCP: []fakeagent.MCPCall{handoverCall("Form wired; tests missing.", false)}, WaitFile: flag("hold2")},
	}})
	repo, _ := cloneWithMain(app, filepath.Join(app.Work, "api"))
	gitOut(t, repo.Dir, "remote", "set-url", "origin", "https://forge.test/acme/api.git")
	rec := importRepo(t, app, repo.Dir)
	saveWorkflow(t, app, "flow", flowYAML("flow", agentStep("one", "")+
		"  - id: work\n    name: Work\n    kind: manual\n    status: In progress\n    session: true\n"))
	task := createTask(t, app, "Fix login", "flow", rec.ID)
	br := branchOf(t, board(t, app), task.ID, rec.ID)
	if _, err := app.W.AdeTask.StartRun(ctx, adewire.StartRunArgs{TaskID: task.ID, BranchNames: map[string]string{br.ID: "feat/api-work"}}); err != nil {
		t.Fatal(err)
	}
	run := waitRun(t, app, task.ID, "one", "done")
	app.WindowMgr.OpenWindow(shell.WindowRecord{Key: sessionWindow})

	// A finished run's handover names the run's session.
	hs := handovers(t, app, task.ID)
	if len(hs) != 1 || hs[0].Source != "run" || run.SessionID == "" || hs[0].SessionID != run.SessionID {
		t.Fatalf("run handover = %+v, want sessionId %q", hs, run.SessionID)
	}

	// A TUI session's handover draft and reported artifact name that session.
	first, err := app.W.AdeTask.LaunchStep(ctx, adewire.LaunchStepArgs{TaskID: task.ID})
	if err != nil {
		t.Fatal(err)
	}
	openLaunch(t, app, first)
	var draft adewire.Handover
	testx.WaitUntil(t, waitFor, func() bool {
		var ok bool
		draft, ok = draftOf(handovers(t, app, task.ID))
		return ok
	})
	if draft.SessionID != first.SessionID {
		t.Fatalf("draft sessionId = %q, want the launch session %q", draft.SessionID, first.SessionID)
	}
	var runbook adewire.Artifact
	testx.WaitUntil(t, waitFor, func() bool {
		runbook, _ = findArtifact(artifactsOf(t, app, task.ID), "link", "Runbook")
		return runbook.ID != ""
	})
	if runbook.SessionID != first.SessionID || runbook.Source != "agent" {
		t.Fatalf("reported artifact = %+v, want sessionId %q", runbook, first.SessionID)
	}

	// A resumed session keeps its id, so its next draft still names it.
	if err := os.WriteFile(flag("hold1"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	waitSession(t, app, first.SessionID, "stopped")
	again, err := app.W.AdeTask.TakeOver(ctx, adewire.TakeOverArgs{SessionID: first.SessionID})
	if err != nil {
		t.Fatal(err)
	}
	if again.SessionID != first.SessionID {
		t.Fatalf("resume session = %q, want %q", again.SessionID, first.SessionID)
	}
	openLaunch(t, app, again)
	testx.WaitUntil(t, waitFor, func() bool {
		d, ok := draftOf(handovers(t, app, task.ID))
		draft = d
		return ok && d.Body == "Form wired; tests missing."
	})
	if draft.SessionID != first.SessionID {
		t.Fatalf("resumed draft sessionId = %q, want %q", draft.SessionID, first.SessionID)
	}
	hs = handovers(t, app, task.ID)
	app.Contract(t, tpScenario, "AdeTaskService.Handovers", adewire.HandoversResult{Handovers: hs}, tpMask)

	// A user add of a reported URL keeps the agent origin; another URL has none.
	kept, err := app.W.AdeTask.AddArtifact(ctx, adewire.AddArtifactArgs{TaskID: task.ID, Kind: "link", Title: "Runbook", URL: tpRunbook})
	if err != nil || kept.SessionID != first.SessionID {
		t.Fatalf("re-add = %+v, %v, want sessionId %q kept", kept, err, first.SessionID)
	}
	other, err := app.W.AdeTask.AddArtifact(ctx, adewire.AddArtifactArgs{TaskID: task.ID, Kind: "link", Title: "Other", URL: tpOther})
	if err != nil || other.SessionID != "" || other.Source != "user" {
		t.Fatalf("user add = %+v, %v, want no session", other, err)
	}

	// An empty title takes the host link ref, and the kind follows the link.
	mr, err := app.W.AdeTask.AddArtifact(ctx, adewire.AddArtifactArgs{TaskID: task.ID, Kind: "link", Title: "", URL: tpMR})
	if err != nil || mr.Kind != "changeRequest" || mr.Title == "" || mr.Title != mr.Ref {
		t.Fatalf("empty-title add = %+v, %v, want the ref as title and kind changeRequest", mr, err)
	}
	plain, err := app.W.AdeTask.AddArtifact(ctx, adewire.AddArtifactArgs{TaskID: task.ID, Kind: "link", Title: "", URL: "https://docs.example/guide/start/"})
	if err != nil || plain.Kind != "link" || plain.Title != "docs.example/guide/start" {
		t.Fatalf("plain empty-title add = %+v, %v, want the host and path as title", plain, err)
	}

	// Rename a stored row; a detected id and an empty title refuse.
	renamed, err := app.W.AdeTask.UpdateArtifact(ctx, adewire.UpdateArtifactArgs{TaskID: task.ID, ID: other.ID, Title: "  Other docs  "})
	if err != nil || renamed.Title != "Other docs" || renamed.ID != other.ID {
		t.Fatalf("UpdateArtifact = %+v, %v", renamed, err)
	}
	if _, err := app.W.AdeTask.UpdateArtifact(ctx, adewire.UpdateArtifactArgs{TaskID: task.ID, ID: br.ID, Title: "x"}); err == nil {
		t.Fatal("UpdateArtifact accepted a detected id")
	}
	if _, err := app.W.AdeTask.UpdateArtifact(ctx, adewire.UpdateArtifactArgs{TaskID: task.ID, ID: other.ID, Title: "  "}); err == nil {
		t.Fatal("UpdateArtifact accepted an empty title")
	}
	app.Contract(t, tpScenario, "AdeTaskService.Artifacts", artifactsOf(t, app, task.ID), tpMask)

	// The branch's change request carries its target and CI state.
	var found adewire.ChangeRequest
	testx.WaitUntil(t, waitFor, func() bool {
		prs, err := app.W.AdeTask.ChangeRequests(ctx)
		found = prs.Branches[br.ID]
		return err == nil && found.Number == 7
	})
	if found.BaseRef != "main" || found.ChecksState != "FAILURE" {
		t.Fatalf("change request = %+v, want baseRef main and checksState FAILURE", found)
	}
	prs, err := app.W.AdeTask.ChangeRequests(ctx)
	if err != nil {
		t.Fatal(err)
	}
	app.Contract(t, tpScenario, "AdeTaskService.ChangeRequests", prs, tpMask)

	if err := os.WriteFile(flag("hold2"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
}
