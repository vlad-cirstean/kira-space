package adeflow_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/agentmcp"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
	"github.com/kirathecat/kira-studio/internal/shell"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// P545 details list. Contract ade-details-list: tests/ui/ade-v2-details-list.spec.ts reads these.
// The list sorts every source by one time; the contract masks times, the spec injects them.

const detailsListScenario = "ade-details-list"

var detailsListMask = flowharness.Mask("id", "proofId", "taskId", "sessionId", "runId", "branchId", "createdAt", "updatedAt", "closedAt", "linkedAt", "addedAt", "ref")

func TestDetailsList(t *testing.T) {
	dir := t.TempDir()
	hold := filepath.Join(dir, "hold")
	app := flowharness.New(t, flowharness.WithTrackerGrace(time.Second))
	claude(app, map[string][]fakeagent.Action{"*": {
		{MCP: []fakeagent.MCPCall{handoverCall("Form wired.", false)}, WaitFile: hold},
	}})
	repo, _ := cloneWithMain(app, filepath.Join(app.Work, "api"))
	rec := importRepo(t, app, repo.Dir)
	saveWorkflow(t, app, "flow", flowYAML("flow", "  - id: work\n    name: Work\n    kind: manual\n    status: In progress\n    session: true\n"))
	app.WindowMgr.OpenWindow(shell.WindowRecord{Key: sessionWindow})
	t.Cleanup(func() { _ = os.WriteFile(hold, nil, 0o644) })

	task := createTask(t, app, "Login form", "flow", rec.ID)
	if _, err := app.W.AdeTask.AddLink(ctx, adewire.AddLinkArgs{TaskID: task.ID, Text: "Runbook " + tpRunbook}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.W.AdeTask.AddNote(ctx, adewire.AddNoteArgs{TaskID: task.ID, Body: "Check the error states."}); err != nil {
		t.Fatal(err)
	}
	launch, err := app.W.AdeTask.LaunchStep(ctx, adewire.LaunchStepArgs{TaskID: task.ID})
	if err != nil {
		t.Fatal(err)
	}
	openLaunch(t, app, launch)
	testx.WaitUntil(t, waitFor, func() bool {
		_, ok := draftOf(handovers(t, app, task.ID))
		return ok
	})
	proofID := closedProof(t, app.W.ProofStore, "Login checks", "diagram", "table")
	if _, err := app.W.AdeBoard.AttachProof(ctx, agentmcp.SpaceCall{TaskID: task.ID}, "Login checks"); err != nil {
		t.Fatal(err)
	}

	b := board(t, app)
	tk := taskOf(t, b, task.ID)
	br := branchOf(t, b, task.ID, rec.ID)
	arts := artifactsOf(t, app, task.ID)
	hs := handovers(t, app, task.ID)
	ps := taskProofs(t, app, task.ID)

	var stored *adewire.Artifact
	for i := range arts.Artifacts {
		if arts.Artifacts[i].ID != "" && arts.Artifacts[i].URL == tpRunbook {
			stored = &arts.Artifacts[i]
		}
	}
	draft, hasDraft := draftOf(hs)
	var note *adewire.Handover
	for i := range hs {
		if hs[i].State == "note" {
			note = &hs[i]
		}
	}
	if stored == nil || !hasDraft || note == nil || len(ps) != 1 || ps[0].ProofID != proofID {
		t.Fatalf("sources: artifact %v, draft %v, note %v, proofs %+v", stored, hasDraft, note, ps)
	}
	if draft.State == "note" {
		t.Fatalf("draft = %+v, want an agent draft, not a note", draft)
	}
	times := []struct {
		name string
		at   int64
	}{
		{"task createdAt", tk.CreatedAt},
		{"branch addedAt", br.AddedAt},
		{"artifact createdAt", stored.CreatedAt},
		{"note createdAt", note.CreatedAt},
		{"draft updatedAt", draft.UpdatedAt},
		{"proof closedAt", ps[0].ClosedAt},
	}
	// The branch row lands with the task, so it may share the task's millisecond but never precede it.
	for i, x := range times {
		if x.at == 0 {
			t.Fatalf("%s is zero", x.name)
		}
		if i > 0 && x.at < times[i-1].at {
			t.Fatalf("%s %d precedes %s %d", x.name, x.at, times[i-1].name, times[i-1].at)
		}
	}

	app.Contract(t, detailsListScenario, "AdeTaskService.Board#list", b, detailsListMask)
	app.Contract(t, detailsListScenario, "AdeTaskService.Artifacts#list", arts, detailsListMask)
	app.Contract(t, detailsListScenario, "AdeTaskService.Handovers#list", adewire.HandoversResult{Handovers: hs}, detailsListMask)
	app.Contract(t, detailsListScenario, "AdeTaskService.TaskProofs#list", adewire.TaskProofsResult{Proofs: ps}, detailsListMask)
}
