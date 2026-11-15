package taskfiles

import (
	"strings"
	"testing"
	"time"
)

func TestClassify(t *testing.T) {
	cases := map[string]Class{
		"README.md": {Kind: KindReadme}, "task.yaml": {Kind: KindTask}, "state.yaml": {Kind: KindState}, "links.yaml": {Kind: KindLinks},
		"notes/20261101T090000-a1.md": {Kind: KindNote}, "handovers/draft-implement.md": {Kind: KindDraft, Step: "implement"},
		"handovers/0003-review-to-implement.md": {Kind: KindHandover},
		"proofs/orders/proof.md":                {Kind: KindProof, Slug: "orders"}, "proofs/orders/totals.json": {Kind: KindProofData, Slug: "orders"},
		"proofs/orders/shot.PNG": {Kind: KindProofImage, Slug: "orders"}, "proofs/orders/shot.svg": {Kind: KindProofImage, Slug: "orders"},
		"proofs/orders/shot.gif": {}, "proofs/orders/sub/x.png": {}, "proofs/Orders/proof.md": {}, "notes/.hidden.md": {},
		".git/config": {}, "notes/deep/a.md": {}, "other.txt": {}, "handovers/1-a-to-b.md": {}, "../x": {}, "": {},
	}
	for rel, want := range cases {
		if got := Classify(rel); got != want {
			t.Errorf("Classify(%q) = %+v, want %+v", rel, got, want)
		}
	}
}

// TestDecide pins the matrix of plan section 7.3 for agent writers: writer, kind, owner, collision
// and whether the change needs a reason.
func TestDecide(t *testing.T) {
	coll := &Collision{By: "user", At: time.UnixMilli(5)}
	cases := []struct {
		name   string
		in     Input
		want   Outcome
		cause  Cause
		notify Notify
	}{
		{"user writes anything", Input{Role: RoleUser, Kind: KindHandover, Op: OpEdit}, Accept, CauseNone, NotifyNone},
		{"kira writes", Input{Role: RoleKira, Kind: KindReadme, Op: OpEdit}, Accept, CauseNone, NotifyNone},
		{"draft of current step", Input{Role: RoleRun, Kind: KindDraft, Op: OpEdit, Step: "implement", CurrentStep: "implement"}, Accept, CauseNone, NotifyNone},
		{"draft created", Input{Role: RoleManual, Kind: KindDraft, Op: OpCreate, Step: "plan", CurrentStep: "plan"}, Accept, CauseNone, NotifyNone},
		{"draft of another step", Input{Role: RoleRun, Kind: KindDraft, Op: OpCreate, Step: "plan", CurrentStep: "implement"}, Reject, CauseWrongStage, NotifyNone},
		{"draft collision, no reason", Input{Role: RoleRun, Kind: KindDraft, Op: OpEdit, Step: "a", CurrentStep: "a", Collision: coll}, Invalid, CauseCollision, NotifyNone},
		{"draft collision, reason", Input{Role: RoleRun, Kind: KindDraft, Op: OpEdit, Step: "a", CurrentStep: "a", Collision: coll, Reason: "mine wins"}, Pending, CauseCollision, NotifyNone},
		{"handed handover edit, no reason", Input{Role: RoleReview, Kind: KindHandover, Op: OpEdit}, Invalid, CauseHandover, NotifyNone},
		{"handed handover delete, reason", Input{Role: RoleReview, Kind: KindHandover, Op: OpDelete, Reason: "dup"}, Pending, CauseHandover, NotifyNone},
		{"note created, reason", Input{Role: RoleOrch, Kind: KindNote, Op: OpCreate, Reason: "add"}, Pending, CauseNote, NotifyNone},
		{"note edited, no reason", Input{Role: RoleRun, Kind: KindNote, Op: OpEdit}, Invalid, CauseNote, NotifyNone},
		{"new link", Input{Role: RoleRun, Kind: KindLinks, Op: OpCreate}, Accept, CauseNone, NotifyNone},
		{"own link edited", Input{Role: RoleRun, Kind: KindLinks, Op: OpEdit, Own: true}, Accept, CauseNone, NotifyNone},
		{"own link removed", Input{Role: RoleRun, Kind: KindLinks, Op: OpDelete, Own: true}, Accept, CauseNone, NotifyNone},
		{"other's link edited, no reason", Input{Role: RoleRun, Kind: KindLinks, Op: OpEdit}, Invalid, CauseLink, NotifyNone},
		{"other's link removed, reason", Input{Role: RoleRun, Kind: KindLinks, Op: OpDelete, Reason: "gone"}, Pending, CauseLink, NotifyNone},
		{"empty slot filled", Input{Role: RoleRun, Kind: KindLinks, Op: OpCreate, Slot: true}, Accept, CauseNone, NotifyNone},
		{"slot replaced, reason", Input{Role: RoleRun, Kind: KindLinks, Op: OpEdit, Slot: true, Reason: "new PR"}, Pending, CauseSlot, NotifyNone},
		{"slot cleared, no reason", Input{Role: RoleRun, Kind: KindLinks, Op: OpDelete, Slot: true}, Invalid, CauseSlot, NotifyNone},
		{"new proof", Input{Role: RoleRun, Kind: KindProof, Op: OpCreate}, Accept, CauseNone, NotifyProofReady},
		{"new proof side file", Input{Role: RoleRun, Kind: KindProofImage, Op: OpCreate}, Accept, CauseNone, NotifyNone},
		{"accepted proof changed, no reason", Input{Role: RoleRun, Kind: KindProof, Op: OpEdit, ProofAccepted: true}, Invalid, CauseProof, NotifyNone},
		{"accepted proof image changed, reason", Input{Role: RoleRun, Kind: KindProofImage, Op: OpEdit, ProofAccepted: true, Reason: "fix"}, Pending, CauseProof, NotifyNone},
		{"accepted proof deleted, reason", Input{Role: RoleRun, Kind: KindProof, Op: OpDelete, ProofAccepted: true, Reason: "dup"}, Pending, CauseProofDelete, NotifyNone},
		{"kira file written", Input{Role: RoleRun, Kind: KindState, Op: OpEdit}, Restore, CauseKiraReadOnly, NotifyNone},
		{"readme deleted", Input{Role: RoleManual, Kind: KindReadme, Op: OpDelete}, Restore, CauseKiraReadOnly, NotifyNone},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := Decide("f", tc.in)
			if d.Outcome != tc.want || d.Cause != tc.cause || d.Notify != tc.notify {
				t.Fatalf("got %+v, want outcome %s cause %q notify %q", d, tc.want, tc.cause, tc.notify)
			}
			if d.Outcome == Invalid && !strings.Contains(d.Message, `needs reason`) {
				t.Fatalf("invalid message = %q", d.Message)
			}
		})
	}
}

func TestReasonError(t *testing.T) {
	got := ReasonError("proofs/orders/proof.md", CauseProof)
	want := `proofs/orders/proof.md: needs reason (the user approves changes to an accepted proof). Call submit again with {"reason": "order 12 corrected"}.`
	if got != want {
		t.Fatalf("got %q\nwant %q", got, want)
	}
}

func TestCleanReason(t *testing.T) {
	if r, err := CleanReason("  fix  "); err != nil || r != "fix" {
		t.Fatalf("%q %v", r, err)
	}
	if r, err := CleanReason("   "); err != nil || r != "" {
		t.Fatalf("blank = %q %v", r, err)
	}
	if _, err := CleanReason(strings.Repeat("x", 501)); err == nil {
		t.Fatal("501 runes accepted")
	}
}
