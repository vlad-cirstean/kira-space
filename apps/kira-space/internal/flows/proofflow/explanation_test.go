package proofflow

import (
	"testing"
	"unicode/utf8"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
)

func TestProofExplanation(t *testing.T) {
	f := newFixture(t)

	summary := "Checkout rejects an expired card. The orders table shows declined and no charge row."
	stepDesc := "Checkout now rejects an expired card: the orders table shows status declined for order 3 and no charge row, where before the fix it showed paid and charged twice, so no customer pays."
	if n := utf8.RuneCountInString(stepDesc); n < 160 || n > 200 {
		t.Fatalf("step description is %d runes, want 160 to 200", n)
	}
	f.summary = summary
	f.save(t, "Explained", tableStep(stepDesc))

	list := f.proofs(t)
	if len(list) != 1 {
		t.Fatalf("proofs = %+v", list)
	}
	f.app.Contract(t, "proof-explained", "AdeTaskService.TaskProofs", list, maskIDs)
	det := session(t, f.app, list[0].ProofID)
	if det.Summary != summary {
		t.Fatalf("Summary = %q, want %q", det.Summary, summary)
	}
	if got := det.Current.Steps[0].Description; got != stepDesc {
		t.Fatalf("step description = %q, want %q", got, stepDesc)
	}
	f.app.Contract(t, "proof-explained", "ProofService.Session", det, maskIDs)

	st, err := f.app.W.Proof.Step(bridge.ProofStepArgs{SessionID: list[0].ProofID, Rev: det.Current.Rev, Ordinal: 1})
	if err != nil {
		t.Fatal(err)
	}
	if st.Description != stepDesc {
		t.Fatalf("Step description = %q, want %q", st.Description, stepDesc)
	}
	f.app.Contract(t, "proof-explained", "ProofService.Step#table", st, maskIDs)
}
