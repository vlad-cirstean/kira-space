package proofflow

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
)

func TestProofExplanation(t *testing.T) {
	f := newFixture(t)

	tools, err := f.sess.ListTools(ctx, &mcp.ListToolsParams{})
	if err != nil {
		t.Fatal(err)
	}
	var saveTool *mcp.Tool
	for _, tl := range tools.Tools {
		if tl.Name == "proof_save" {
			saveTool = tl
		}
	}
	if saveTool == nil {
		t.Fatal("proof_save not listed")
	}
	if !strings.Contains(saveTool.Description, "behavior, not code") {
		t.Fatalf("proof_save description lacks the behavior framing: %q", saveTool.Description)
	}
	schema, ok := saveTool.InputSchema.(map[string]any)
	if !ok {
		t.Fatalf("input schema = %T, want map", saveTool.InputSchema)
	}
	steps := schema["properties"].(map[string]any)["steps"].(map[string]any)
	desc := steps["items"].(map[string]any)["properties"].(map[string]any)["description"].(map[string]any)["description"].(string)
	if !strings.Contains(desc, "terse") {
		t.Fatalf("steps.description guidance lacks terse: %q", desc)
	}

	summary := "Checkout rejects an expired card. The orders table shows declined and no charge row."
	stepDesc := "Checkout now rejects an expired card: the orders table shows status declined for order 3 and no charge row, where before the fix it showed paid and charged twice, so no customer pays."
	if n := utf8.RuneCountInString(stepDesc); n < 160 || n > 200 {
		t.Fatalf("step description is %d runes, want 160 to 200", n)
	}
	args := saveArgs("Explained", []map[string]any{tableStep(stepDesc)})
	args["summary"] = summary
	var r struct{ Name string }
	f.call(t, "proof_save", args, &r)

	list, err := f.app.W.Proof.Sessions()
	if err != nil || len(list) != 1 {
		t.Fatalf("Sessions = %+v, %v", list, err)
	}
	f.app.Contract(t, "proof-explained", "ProofService.Sessions", list, maskIDs)
	det := session(t, f.app, list[0].ID)
	if det.Summary != summary {
		t.Fatalf("Summary = %q, want %q", det.Summary, summary)
	}
	if got := det.Current.Steps[0].Description; got != stepDesc {
		t.Fatalf("step description = %q, want %q", got, stepDesc)
	}
	f.app.Contract(t, "proof-explained", "ProofService.Session", det, maskIDs)

	st, err := f.app.W.Proof.Step(bridge.ProofStepArgs{SessionID: list[0].ID, Rev: det.Current.Rev, Ordinal: 1})
	if err != nil {
		t.Fatal(err)
	}
	if st.Description != stepDesc {
		t.Fatalf("Step description = %q, want %q", st.Description, stepDesc)
	}
	f.app.Contract(t, "proof-explained", "ProofService.Step#table", st, maskIDs)
}
