package claude

import (
	"encoding/json"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
)

// proofRun runs one agent step with prompt, which must end in finish_step, and returns the task and its
// proofs once the run is done. The agent learns the proof format from the README and the prompt text.
func proofRun(t *testing.T, prompt string) (*adeFixture, adewire.Task, []adewire.TaskProof) {
	t.Helper()
	f := newAdeFixture(t, nil)
	step := "  - id: one\n    name: one\n    kind: agent\n    runs_on: each repo\n    timeout: 4m\n    prompt: |\n      " + prompt + "\n"
	f.saveWorkflow(t, "flow", step+manualStepYAML)
	task := f.createTask(t, "flow")
	run := f.startRun(t, task, "feat/proof")
	if run.State != "done" {
		t.Fatalf("run = %+v, want done; log:\n%s", run, tail(f.runLog(t, run.ID), 1500))
	}
	res, err := f.W.AdeTask.TaskProofs(ctx, adewire.TaskArgs{TaskID: task.ID})
	if err != nil {
		t.Fatalf("TaskProofs: %v", err)
	}
	return f, task, res.Proofs
}

// TestProofMcpThroughClaude has the real claude write a validation proof as proofs/<slug>/proof.md in
// the task folder and submit it, then checks the stored proof.
func TestProofMcpThroughClaude(t *testing.T) {
	f, _, proofs := proofRun(t, `Write a validation proof named "Claude proof" in the task folder with two steps: `+
		`first a table step (description "Two rows", columns id and label, rows 1 a and 2 b), `+
		`then a document step (description "One doc", one document with body ok true). `+
		`Call submit, then call finish_step with status done.`)
	if len(proofs) != 1 || proofs[0].Name != "Claude proof" || proofs[0].Slug != "claude-proof" {
		t.Fatalf("proofs = %+v, want one named %q in folder claude-proof", proofs, "Claude proof")
	}
	steps := proofs[0].Steps
	if len(steps) != 2 || steps[0].View != "table" || steps[0].Description != "Two rows" || steps[1].View != "document" || steps[1].Description != "One doc" {
		t.Fatalf("steps = %+v, want table then document in order", steps)
	}
	f.requireCostUnder(t)
}

// TestProofChartThroughClaude has the real claude write a bar chart step from the README text alone and
// checks the stored payload.
func TestProofChartThroughClaude(t *testing.T) {
	f, _, proofs := proofRun(t, `Write a validation proof named "Chart proof" in the task folder with one step: `+
		`a chart step with description "Errors by service" showing errors per service as a bar chart: api 3, web 5, worker 0. `+
		`Call submit, then call finish_step with status done.`)
	if len(proofs) != 1 || proofs[0].Name != "Chart proof" || len(proofs[0].Steps) != 1 || proofs[0].Steps[0].View != "chart" || proofs[0].Steps[0].Description != "Errors by service" {
		t.Fatalf("proofs = %+v, want one chart step %q", proofs, "Errors by service")
	}
	step, err := f.W.Proof.Step(bridge.ProofStepArgs{SessionID: proofs[0].ProofID, Rev: proofs[0].Rev, Ordinal: proofs[0].Steps[0].Ordinal})
	if err != nil {
		t.Fatal(err)
	}
	var data struct {
		Type string `json:"type"`
		X    struct {
			Type string `json:"type"`
		} `json:"x"`
		Series []struct {
			Points [][]any `json:"points"`
		} `json:"series"`
	}
	if err := json.Unmarshal(step.Data, &data); err != nil {
		t.Fatal(err)
	}
	if data.Type != "bar" || data.X.Type != "category" || len(data.Series) != 1 {
		t.Fatalf("data = %s, want one bar series on a category axis", step.Data)
	}
	want := [][]any{{"api", 3.0}, {"web", 5.0}, {"worker", 0.0}}
	got := data.Series[0].Points
	if len(got) != len(want) {
		t.Fatalf("points = %v, want %v", got, want)
	}
	for i := range want {
		if len(got[i]) != 2 || got[i][0] != want[i][0] || got[i][1] != want[i][1] {
			t.Fatalf("points = %v, want %v", got, want)
		}
	}
	f.requireCostUnder(t)
}
