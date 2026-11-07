package adeflow_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/proof"
	"github.com/kirathecat/kira-studio/internal/agentmcp"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
	"github.com/kirathecat/kira-studio/internal/shell"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// P543 proof chain. Contract ade-proof-chain: tests/ui/ade-v2-proof-chain.spec.ts reads these.

const proofChainScenario = "ade-proof-chain"

var proofChainMask = flowharness.Mask("id", "proofId", "taskId", "sessionId", "createdAt", "updatedAt", "closedAt", "linkedAt")

func attachCall(name string) fakeagent.MCPCall {
	return fakeagent.MCPCall{Server: agentmcp.ServerName, Tool: "report_artifacts", Tolerate: true, Args: map[string]any{"artifacts": []map[string]any{{"kind": "proof", "ref": name}}}}
}

// closedProof builds a closed proof session of the given steps; a "diagram" step is a flowchart, any other a one-row table.
func closedProof(t *testing.T, svc *proof.Service, name string, views ...string) string {
	t.Helper()
	d := openProofDraft(t, svc, name, views...)
	if _, err := svc.CloseSession(d); err != nil {
		t.Fatalf("CloseSession %s: %v", name, err)
	}
	for _, s := range mustSessions(t, svc) {
		if s.Name == name {
			return s.ID
		}
	}
	t.Fatalf("session %s not listed", name)
	return ""
}

func openProofDraft(t *testing.T, svc *proof.Service, name string, views ...string) string {
	t.Helper()
	start, err := svc.StartSession(name, "")
	if err != nil {
		t.Fatalf("StartSession %s: %v", name, err)
	}
	for i, v := range views {
		in := proof.StepInput{DraftID: start.DraftID, Description: fmt.Sprintf("%s step %d", name, i+1), View: v}
		if v == "diagram" {
			in.Source = "mermaid"
			in.Data = json.RawMessage(`{"source":"flowchart TD\n  Form --> Check\n  Check --> Saved"}`)
		} else {
			in.View, in.Language, in.Source, in.Query = "table", "sql", "postgres", "select id from users"
			in.Data = json.RawMessage(`{"columns":[{"name":"id","type":"number"}],"rows":[[1]]}`)
		}
		if _, err := svc.AddStep(in); err != nil {
			t.Fatalf("AddStep %s: %v", name, err)
		}
	}
	return start.DraftID
}

func mustSessions(t *testing.T, svc *proof.Service) []proof.Summary {
	t.Helper()
	list, err := svc.Sessions()
	if err != nil {
		t.Fatal(err)
	}
	return list
}

func taskProofs(t *testing.T, app *flowharness.App, taskID string) []adewire.TaskProof {
	t.Helper()
	res, err := app.W.AdeTask.TaskProofs(ctx, adewire.TaskArgs{TaskID: taskID})
	if err != nil {
		t.Fatalf("TaskProofs: %v", err)
	}
	return res.Proofs
}

func proofChoices(t *testing.T, app *flowharness.App) []adewire.ProofChoice {
	t.Helper()
	res, err := app.W.AdeTask.ProofChoices(ctx)
	if err != nil {
		t.Fatalf("ProofChoices: %v", err)
	}
	return res.Proofs
}

func TestProofChain(t *testing.T) {
	dir := t.TempDir()
	hold := filepath.Join(dir, "hold")
	app := flowharness.New(t, flowharness.WithTrackerGrace(time.Second))
	// Task folder sessions: the first launch belongs to "Login form", the second to "Other".
	claude(app, map[string][]fakeagent.Action{"work": {
		{MCP: []fakeagent.MCPCall{attachCall("Login checks"), attachCall("login checks"), attachCall("Draft proof"), attachCall("No such proof")}, WaitFile: hold},
		{MCP: []fakeagent.MCPCall{attachCall("Login checks")}, WaitFile: hold},
	}})
	t.Cleanup(func() { _ = os.WriteFile(hold, nil, 0o644) })
	repo, _ := cloneWithMain(app, filepath.Join(app.Work, "api"))
	rec := importRepo(t, app, repo.Dir)
	saveWorkflow(t, app, "flow", flowYAML("flow", "  - id: design\n    name: Design\n    kind: manual\n    session: true\n"+"  - id: ship\n    name: Ship\n    kind: manual\n"))
	app.WindowMgr.OpenWindow(shell.WindowRecord{Key: sessionWindow})
	svc := app.W.ProofStore

	login, err := app.W.AdeTask.CreateTask(ctx, adewire.CreateTaskArgs{Title: "Login form", CodeRepoIDs: []string{rec.ID}, WorkflowID: "flow"})
	if err != nil {
		t.Fatal(err)
	}
	other, err := app.W.AdeTask.CreateTask(ctx, adewire.CreateTaskArgs{Title: "Other", CodeRepoIDs: []string{rec.ID}, WorkflowID: "flow"})
	if err != nil {
		t.Fatal(err)
	}
	loginID := closedProof(t, svc, "Login checks", "diagram", "table")
	openProofDraft(t, svc, "Draft proof", "table")
	legacyID := closedProof(t, svc, "Legacy", "table")

	// The agent attaches through the MCP server; the repeat, the open draft and the unknown name leave one link.
	launch, err := app.W.AdeTask.LaunchStep(ctx, adewire.LaunchStepArgs{TaskID: login.ID})
	if err != nil {
		t.Fatal(err)
	}
	openLaunch(t, app, launch)
	testx.WaitUntil(t, waitFor, func() bool { return len(taskProofs(t, app, login.ID)) == 1 })
	if err := os.WriteFile(hold, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	waitSession(t, app, launch.SessionID, "stopped")
	got := taskProofs(t, app, login.ID)
	if len(got) != 1 {
		t.Fatalf("proofs = %+v, want one", got)
	}
	p := got[0]
	if p.ProofID != loginID || p.Name != "Login checks" || p.Source != "agent" || p.StepID != "design" || p.StepName != "Design" ||
		p.SessionID != launch.SessionID || p.ClosedAt == 0 || p.LinkedAt < p.ClosedAt || p.Rev != 1 || p.Pending {
		t.Fatalf("proof = %+v", p)
	}
	if len(p.Steps) != 2 || p.Steps[0].View != "diagram" || p.Steps[0].Source != "mermaid" || p.Steps[1].View != "table" || p.Steps[1].Ordinal != 2 {
		t.Fatalf("steps = %+v", p.Steps)
	}

	// Refusals through the engine, as the agent sees them.
	call := agentmcp.SpaceCall{TaskID: other.ID}
	for name, want := range map[string]string{
		"Login checks":  `proof session "Login checks" belongs to another task`,
		"Draft proof":   "the proof is still being saved; retry in a moment",
		"No such proof": "no proof session with that name; call proof_list",
	} {
		_, err := app.W.AdeBoard.AttachProof(ctx, call, name)
		var te agentmcp.ToolError
		if err == nil || !strings.Contains(err.Error(), want) || !errors.As(err, &te) {
			t.Fatalf("AttachProof(%q) = %v, want tool error %q", name, err, want)
		}
	}
	if again, err := app.W.AdeBoard.AttachProof(ctx, agentmcp.SpaceCall{TaskID: login.ID}, "LOGIN CHECKS"); err != nil || again.Status != "attached" || again.Steps != 2 {
		t.Fatalf("repeat attach = %+v, %v", again, err)
	}
	// A second task's session cannot take it either.
	second, err := app.W.AdeTask.LaunchStep(ctx, adewire.LaunchStepArgs{TaskID: other.ID})
	if err != nil {
		t.Fatal(err)
	}
	openLaunch(t, app, second)
	waitSession(t, app, second.SessionID, "stopped")
	if n := len(taskProofs(t, app, other.ID)); n != 0 {
		t.Fatalf("other task holds %d proofs, want 0", n)
	}

	// A note newer than the proof makes a mixed chain for the UI.
	if _, err := app.W.AdeTask.AddNote(ctx, adewire.AddNoteArgs{TaskID: login.ID, Body: "Check the error states."}); err != nil {
		t.Fatal(err)
	}
	chain := adewire.TaskProofsResult{Proofs: taskProofs(t, app, login.ID)}
	app.Contract(t, proofChainScenario, "AdeTaskService.TaskProofs#chain", chain, proofChainMask)
	app.Contract(t, proofChainScenario, "AdeTaskService.Handovers#chain", adewire.HandoversResult{Handovers: handovers(t, app, login.ID)}, notesMask)
	choices := proofChoices(t, app)
	app.Contract(t, proofChainScenario, "AdeTaskService.ProofChoices", adewire.ProofChoicesResult{Proofs: choices}, proofChainMask)
	if len(choices) != 1 || choices[0].Name != "Legacy" || choices[0].ProofID != legacyID || choices[0].Steps != 1 {
		t.Fatalf("choices = %+v, want Legacy only (the draft is open, Login checks is attached)", choices)
	}
	st, err := app.W.Proof.Step(bridge.ProofStepArgs{SessionID: loginID, Rev: 1, Ordinal: 1})
	if err != nil {
		t.Fatal(err)
	}
	app.Contract(t, proofChainScenario, "ProofService.Step#diagram", st, proofChainMask)

	// The user attaches Legacy; a task already holding it elsewhere refuses.
	link := adewire.LinkProofArgs{TaskID: login.ID, ProofID: legacyID}
	if err := app.W.AdeTask.LinkProof(ctx, link); err != nil {
		t.Fatalf("LinkProof: %v", err)
	}
	if err := app.W.AdeTask.LinkProof(ctx, link); err != nil {
		t.Fatalf("repeat LinkProof: %v", err)
	}
	if err := app.W.AdeTask.LinkProof(ctx, adewire.LinkProofArgs{TaskID: other.ID, ProofID: legacyID}); err == nil || !strings.Contains(err.Error(), "belongs to another task") {
		t.Fatalf("LinkProof to a second task = %v, want refusal", err)
	}
	if ps := taskProofs(t, app, login.ID); len(ps) != 2 || ps[0].Name != "Legacy" || ps[0].Source != "user" || ps[0].SessionID != "" || ps[0].StepName != "Design" {
		t.Fatalf("proofs after user attach = %+v", ps)
	}
	if len(proofChoices(t, app)) != 0 {
		t.Fatal("an attached session must leave the choices")
	}

	// Retention: 31 more unattached sessions push out the older unattached ones, never an attached one.
	for i := range 31 {
		closedProof(t, svc, fmt.Sprintf("Filler %02d", i), "table")
	}
	closed := mustSessions(t, svc)
	if len(closed) != 32 || len(taskProofs(t, app, login.ID)) != 2 {
		t.Fatalf("closed sessions = %d, task proofs = %d, want 32 and 2", len(closed), len(taskProofs(t, app, login.ID)))
	}
	if n := len(proofChoices(t, app)); n != 30 {
		t.Fatalf("choices = %d, want the newest 30 unattached", n)
	}

	// Unlink returns the session to the choices; it then counts as unattached for pruning.
	if err := app.W.AdeTask.UnlinkProof(ctx, link); err != nil {
		t.Fatalf("UnlinkProof: %v", err)
	}
	if err := app.W.AdeTask.UnlinkProof(ctx, link); err == nil {
		t.Fatal("unlinking twice must fail")
	}
	if ch := proofChoices(t, app); len(ch) != 31 || ch[len(ch)-1].Name != "Legacy" {
		t.Fatalf("choices after unlink: %d, last %+v, want 31 ending in Legacy", len(ch), ch[len(ch)-1])
	}

	// Archived: writes refuse, reads work; deleting the session empties the entry.
	if err := app.W.AdeTask.ArchiveTask(ctx, adewire.TaskArgs{TaskID: login.ID}); err != nil {
		t.Fatal(err)
	}
	if err := app.W.AdeTask.LinkProof(ctx, link); err == nil || !strings.Contains(err.Error(), "archived") {
		t.Fatalf("LinkProof on an archived task = %v, want refusal", err)
	}
	if _, err := app.W.AdeBoard.AttachProof(ctx, agentmcp.SpaceCall{TaskID: login.ID}, "Login checks"); err == nil || !strings.Contains(err.Error(), "archived") {
		t.Fatalf("AttachProof on an archived task = %v, want refusal", err)
	}
	if ps := taskProofs(t, app, login.ID); len(ps) != 1 || ps[0].Name != "Login checks" {
		t.Fatalf("archived task proofs = %+v, want still readable", ps)
	}
	if err := app.W.Proof.Delete(bridge.ProofSessionArgs{SessionID: loginID}); err != nil {
		t.Fatal(err)
	}
	if ps := taskProofs(t, app, login.ID); len(ps) != 0 {
		t.Fatalf("proofs after delete = %+v, want none", ps)
	}
}
