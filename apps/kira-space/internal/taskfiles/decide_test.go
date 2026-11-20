package taskfiles

import (
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

// TestDecide pins the author rule for agent writers: who wrote the target, its age, the op and
// collisions decide whether the user is asked.
func TestDecide(t *testing.T) {
	coll := &Collision{By: "s2", At: time.UnixMilli(5)}
	cases := []struct {
		name   string
		in     Input
		want   Outcome
		cause  Cause
		notify Notify
	}{
		{"R1 user writes anything", Input{Role: RoleUser, Kind: KindHandover, Op: OpEdit, Author: AuthorAgent}, Accept, CauseNone, NotifyNone},
		{"R1 kira writes", Input{Role: RoleKira, Kind: KindReadme, Op: OpEdit}, Accept, CauseNone, NotifyNone},
		{"R2 kira file written", Input{Role: RoleRun, Kind: KindState, Op: OpEdit}, Restore, CauseKiraReadOnly, NotifyNone},
		{"R2 readme deleted", Input{Role: RoleManual, Kind: KindReadme, Op: OpDelete}, Restore, CauseKiraReadOnly, NotifyNone},
		{"R3 draft of another step", Input{Role: RoleRun, Kind: KindDraft, Op: OpCreate, Step: "plan", CurrentStep: "implement"}, Reject, CauseWrongStage, NotifyNone},
		{"R4 draft of current step", Input{Role: RoleRun, Kind: KindDraft, Op: OpEdit, Step: "implement", CurrentStep: "implement", Author: AuthorAgent}, Accept, CauseNone, NotifyNone},
		{"R5 note created", Input{Role: RoleOrch, Kind: KindNote, Op: OpCreate}, Accept, CauseNone, NotifyNone},
		{"R5 new link", Input{Role: RoleRun, Kind: KindLinks, Op: OpCreate}, Accept, CauseNone, NotifyNone},
		{"R5 new proof", Input{Role: RoleRun, Kind: KindProof, Op: OpCreate}, Accept, CauseNone, NotifyProofReady},
		{"R5 new proof side file", Input{Role: RoleRun, Kind: KindProofImage, Op: OpCreate}, Accept, CauseNone, NotifyNone},
		{"R6 own note edited", Input{Role: RoleRun, Kind: KindNote, Op: OpEdit, Author: AuthorSelf}, Accept, CauseNone, NotifyNone},
		{"R6 own handover deleted", Input{Role: RoleRun, Kind: KindHandover, Op: OpDelete, Author: AuthorSelf}, Accept, CauseNone, NotifyNone},
		{"R6 own link removed", Input{Role: RoleRun, Kind: KindLinks, Op: OpDelete, Author: AuthorSelf}, Accept, CauseNone, NotifyNone},
		{"R6 own older proof edited", Input{Role: RoleRun, Kind: KindProof, Op: OpEdit, ProofAccepted: true, Author: AuthorSelf}, Accept, CauseNone, NotifyNone},
		{"R7 user note edited", Input{Role: RoleRun, Kind: KindNote, Op: OpEdit}, Accept, CauseNone, NotifyNone},
		{"R7 user handover deleted", Input{Role: RoleRun, Kind: KindHandover, Op: OpDelete}, Accept, CauseNone, NotifyNone},
		{"R7 user link edited", Input{Role: RoleRun, Kind: KindLinks, Op: OpEdit}, Accept, CauseNone, NotifyNone},
		{"R8 slot replaced", Input{Role: RoleRun, Kind: KindLinks, Op: OpEdit, Slot: true}, Accept, CauseNone, NotifyNone},
		{"R8 slot cleared", Input{Role: RoleRun, Kind: KindLinks, Op: OpDelete, Slot: true, Author: AuthorAgent}, Accept, CauseNone, NotifyNone},
		{"R9 latest other note edited", Input{Role: RoleRun, Kind: KindNote, Op: OpEdit, Author: AuthorAgent, Latest: true}, Accept, CauseNone, NotifyNone},
		{"R9 latest other handover edited", Input{Role: RoleRun, Kind: KindHandover, Op: OpEdit, Author: AuthorAgent, Latest: true}, Accept, CauseNone, NotifyNone},
		{"R9 latest other proof edited", Input{Role: RoleRun, Kind: KindProof, Op: OpEdit, ProofAccepted: true, Author: AuthorAgent, Latest: true}, Accept, CauseNone, NotifyNone},
		{"R9 latest other proof file created", Input{Role: RoleRun, Kind: KindProofData, Op: OpCreate, ProofAccepted: true, Author: AuthorAgent, Latest: true}, Accept, CauseNone, NotifyNone},
		{"R10 older other note edited", Input{Role: RoleRun, Kind: KindNote, Op: OpEdit, Author: AuthorAgent}, Pending, CauseNote, NotifyNone},
		{"R10 older other handover edited", Input{Role: RoleReview, Kind: KindHandover, Op: OpEdit, Author: AuthorAgent}, Pending, CauseHandover, NotifyNone},
		{"R11 older other proof edited", Input{Role: RoleRun, Kind: KindProof, Op: OpEdit, ProofAccepted: true, Author: AuthorAgent}, Pending, CauseProof, NotifyNone},
		{"R11 older other proof image edited", Input{Role: RoleRun, Kind: KindProofImage, Op: OpEdit, ProofAccepted: true, Author: AuthorAgent}, Pending, CauseProof, NotifyNone},
		{"R12 other link edited", Input{Role: RoleRun, Kind: KindLinks, Op: OpEdit, Author: AuthorAgent}, Pending, CauseLink, NotifyNone},
		{"R12 other link removed", Input{Role: RoleRun, Kind: KindLinks, Op: OpDelete, Author: AuthorAgent}, Pending, CauseLink, NotifyNone},
		{"R13 latest other note deleted", Input{Role: RoleRun, Kind: KindNote, Op: OpDelete, Author: AuthorAgent, Latest: true}, Pending, CauseNote, NotifyNone},
		{"R13 latest other handover deleted", Input{Role: RoleRun, Kind: KindHandover, Op: OpDelete, Author: AuthorAgent, Latest: true}, Pending, CauseHandover, NotifyNone},
		{"R14 other proof deleted", Input{Role: RoleRun, Kind: KindProof, Op: OpDelete, ProofAccepted: true, Author: AuthorAgent}, Pending, CauseProofDelete, NotifyNone},
		{"R14 latest other proof deleted", Input{Role: RoleRun, Kind: KindProof, Op: OpDelete, ProofAccepted: true, Author: AuthorAgent, Latest: true}, Pending, CauseProofDelete, NotifyNone},
		{"R15 collision on a free change", Input{Role: RoleRun, Kind: KindDraft, Op: OpEdit, Step: "a", CurrentStep: "a", Collision: coll}, Pending, CauseCollision, NotifyNone},
		{"R15 collision on a latest note edit", Input{Role: RoleRun, Kind: KindNote, Op: OpEdit, Author: AuthorAgent, Latest: true, Collision: coll}, Pending, CauseCollision, NotifyNone},
		{"R15 collision cause stays the author cause", Input{Role: RoleRun, Kind: KindNote, Op: OpEdit, Author: AuthorAgent, Collision: coll}, Pending, CauseNote, NotifyNone},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := Decide("f", tc.in)
			if d.Outcome != tc.want || d.Cause != tc.cause || d.Notify != tc.notify {
				t.Fatalf("got %+v, want outcome %s cause %q notify %q", d, tc.want, tc.cause, tc.notify)
			}
		})
	}
}
