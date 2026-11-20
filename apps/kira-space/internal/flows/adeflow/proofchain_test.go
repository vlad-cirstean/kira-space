package adeflow_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
)

// P543 proof chain, P643 files. Contract ade-proof-chain: tests/ui/ade-v2-proof-chain.spec.ts reads these.

const proofChainScenario = "ade-proof-chain"

var proofChainMask = flowharness.Mask("id", "proofId", "taskId", "sessionId", "createdAt", "updatedAt", "closedAt", "linkedAt")

const proofTable = `{"columns":[{"name":"id","type":"number"}],"rows":[[1]]}`

// writeProof writes proofs/<slug>/proof.md with one table step per name in views and a diagram for "diagram".
func writeProof(t *testing.T, folder, slug, name string, views ...string) {
	t.Helper()
	var b strings.Builder
	b.WriteString("---\nname: " + name + "\nsummary: Checks " + name + "\nsteps:\n")
	for i, v := range views {
		id := "s" + string(rune('1'+i))
		if v == "diagram" {
			b.WriteString("  - id: " + id + "\n    description: " + name + " flow\n    view: diagram\n    source: mermaid\n    inline: |\n      flowchart TD\n        Form --> Check\n        Check --> Saved\n")
			continue
		}
		b.WriteString("  - id: " + id + "\n    description: " + name + " rows\n    view: table\n    language: sql\n    source: postgres\n    query: select id from users\n    inline: " + proofTable + "\n")
	}
	b.WriteString("---\n")
	dfWrite(t, folder, "proofs/"+slug+"/proof.md", b.String())
}

func taskProofs(t *testing.T, app *flowharness.App, taskID string) []adewire.TaskProof {
	t.Helper()
	res, err := app.W.AdeTask.TaskProofs(ctx, adewire.TaskArgs{TaskID: taskID})
	if err != nil {
		t.Fatalf("TaskProofs: %v", err)
	}
	return res.Proofs
}

func TestProofChain(t *testing.T) {
	app := flowharness.New(t)
	repo, _ := cloneWithMain(app, filepath.Join(app.Work, "api"))
	rec := importRepo(t, app, repo.Dir)
	saveWorkflow(t, app, "flow", flowYAML("flow", "  - id: design\n    name: Design\n    kind: manual\n    session: true\n"))
	login := createTask(t, app, "Login form", "flow", rec.ID)
	other := createTask(t, app, "Other", "flow", rec.ID)
	folder := dfFolder(t, app, login.ID)

	writeProof(t, folder, "login-checks", "Login checks", "diagram", "table")
	if reply := dfSync(t, app, login.ID); !strings.Contains(reply, "proofs/login-checks/") || !strings.Contains(reply, "accepted") {
		t.Fatalf("submit reply = %q", reply)
	}
	got := taskProofs(t, app, login.ID)
	if len(got) != 1 {
		t.Fatalf("proofs = %+v, want one", got)
	}
	p := got[0]
	if p.Name != "Login checks" || p.Slug != "login-checks" || p.ClosedAt == 0 || p.Rev != 1 || p.Pending {
		t.Fatalf("proof = %+v", p)
	}
	if len(p.Steps) != 2 || p.Steps[0].ID != "s1" || p.Steps[0].View != "diagram" || p.Steps[0].Source != "mermaid" || p.Steps[1].View != "table" || p.Steps[1].Ordinal != 2 {
		t.Fatalf("steps = %+v", p.Steps)
	}
	if n := len(taskProofs(t, app, other.ID)); n != 0 {
		t.Fatalf("other task holds %d proofs, want 0", n)
	}

	// A note newer than the proof makes a mixed chain for the UI.
	if _, err := app.W.AdeTask.AddNote(ctx, adewire.AddNoteArgs{TaskID: login.ID, Body: "Check the error states."}); err != nil {
		t.Fatal(err)
	}
	app.Contract(t, proofChainScenario, "AdeTaskService.TaskProofs#chain", adewire.TaskProofsResult{Proofs: taskProofs(t, app, login.ID)}, proofChainMask)
	app.Contract(t, proofChainScenario, "AdeTaskService.Handovers#chain", adewire.HandoversResult{Handovers: handovers(t, app, login.ID)}, notesMask)
	st, err := app.W.Proof.Step(bridge.ProofStepArgs{SessionID: p.ProofID, Rev: 1, Ordinal: 1})
	if err != nil {
		t.Fatal(err)
	}
	app.Contract(t, proofChainScenario, "ProofService.Step#diagram", st, proofChainMask)

	// Archived: reads work.
	if err := app.W.AdeTask.ArchiveTask(ctx, adewire.TaskArgs{TaskID: login.ID}); err != nil {
		t.Fatal(err)
	}
	if ps := taskProofs(t, app, login.ID); len(ps) != 1 || ps[0].Name != "Login checks" {
		t.Fatalf("archived task proofs = %+v, want still readable", ps)
	}
	// Deleting the proof empties the entry and removes its folder.
	if err := app.W.Proof.Delete(bridge.ProofSessionArgs{SessionID: p.ProofID}); err != nil {
		t.Fatal(err)
	}
	if ps := taskProofs(t, app, login.ID); len(ps) != 0 {
		t.Fatalf("proofs after delete = %+v, want none", ps)
	}
	if _, err := os.Stat(filepath.Join(folder, "proofs", "login-checks")); !os.IsNotExist(err) {
		t.Fatalf("proof folder after delete: %v, want gone", err)
	}
}

// J-PRF-020: a proof of a task survives a stage move and an archive/restore: the rows and the folder stay.
func TestProofSurvivesStageMoveAndArchive(t *testing.T) {
	app := flowharness.New(t)
	repo, _ := cloneWithMain(app, filepath.Join(app.Work, "api"))
	rec := importRepo(t, app, repo.Dir)
	saveWorkflow(t, app, "flow", flowYAML("flow", "  - id: a\n    name: A\n    kind: manual\n"+"  - id: b\n    name: B\n    kind: manual\n"))
	task := createTask(t, app, "Proofed", "flow", rec.ID)
	folder := dfFolder(t, app, task.ID)
	writeProof(t, folder, "checks", "Checks", "diagram", "table")
	dfSync(t, app, task.ID)
	proofID := taskProofs(t, app, task.ID)[0].ProofID
	check := func(label string) {
		t.Helper()
		ps := taskProofs(t, app, task.ID)
		if len(ps) != 1 || ps[0].Name != "Checks" || ps[0].ProofID != proofID {
			t.Fatalf("%s: task proofs = %+v, want the proof", label, ps)
		}
		var steps int
		if err := app.DB().QueryRow(`SELECT count(*) FROM proof_steps WHERE session_id = ?`, proofID).Scan(&steps); err != nil || steps != 2 {
			t.Fatalf("%s: proof steps = %d, %v, want 2", label, steps, err)
		}
		if _, err := os.Stat(filepath.Join(folder, "proofs", "checks", "proof.md")); err != nil {
			t.Fatalf("%s: proof.md: %v", label, err)
		}
	}
	check("before")
	if _, err := app.W.AdeTask.StepDone(ctx, adewire.StepDoneArgs{TaskID: task.ID}); err != nil {
		t.Fatal(err)
	}
	check("after the stage move")
	if err := app.W.AdeTask.ArchiveTask(ctx, adewire.TaskArgs{TaskID: task.ID}); err != nil {
		t.Fatal(err)
	}
	check("archived")
	if _, err := app.W.AdeTask.RestoreTask(ctx, adewire.TaskArgs{TaskID: task.ID}); err != nil {
		t.Fatal(err)
	}
	check("restored")
}
