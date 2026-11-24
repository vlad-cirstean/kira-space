package proofflow

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/proof"
	"github.com/kirathecat/kira-studio/internal/approvals"
)

var maskIDs = flowharness.Mask("id", "closedAt", "createdAt", "linkedAt", "version")

func TestProofCloseAndRead(t *testing.T) {
	f := newFixture(t)
	c := f.save(t, "Checkout fix", tableStep("Orders before"), documentStep("Doc hit"), keyValueStep("Stats"))
	if c.Name != "Checkout fix" || c.Slug != "checkout-fix" {
		t.Fatalf("saved = %+v", c)
	}

	list := f.proofs(t)
	if len(list) != 1 || len(list[0].Steps) != 3 {
		t.Fatalf("proofs = %+v", list)
	}
	f.app.Contract(t, "proof", "AdeTaskService.TaskProofs", list, maskIDs)

	det := session(t, f.app, list[0].ProofID)
	if len(det.Current.Steps) != 3 || det.Pending != nil {
		t.Fatalf("detail = %+v", det)
	}
	for i, s := range det.Current.Steps {
		if s.Ordinal != i+1 {
			t.Fatalf("step %d = %+v", i, s)
		}
	}
	f.app.Contract(t, "proof", "ProofService.Session", det, maskIDs)

	for i, key := range []string{"table", "document", "keyvalue"} {
		st, err := f.app.W.Proof.Step(bridge.ProofStepArgs{SessionID: list[0].ProofID, Rev: det.Current.Rev, Ordinal: i + 1})
		if err != nil {
			t.Fatal(err)
		}
		f.app.Contract(t, "proof", "ProofService.Step#"+key, st, maskIDs)
	}

	// Large integers survive as written.
	st, _ := f.app.W.Proof.Step(bridge.ProofStepArgs{SessionID: list[0].ProofID, Rev: det.Current.Rev, Ordinal: 1})
	if !bytes.Contains(st.Data, []byte("9007199254740993")) {
		t.Fatalf("integer precision lost: %s", st.Data)
	}
	// The proof window's cell editor shows this cell (tests/ui/proof-window.spec.ts).
	if !bytes.Contains(st.Data, []byte(`"alpha"`)) {
		t.Fatalf("first-row name missing: %s", st.Data)
	}

	if ps := proofPrompts(t, f.app); len(ps) != 1 || ps[0].Ref != list[0].ProofID {
		t.Fatalf("prompts = %+v, want one for %s", ps, list[0].ProofID)
	}
}

func TestProofNameCollision(t *testing.T) {
	f := newFixture(t)
	f.save(t, "Same Name", keyValueStep("one"))
	delete(f.last, "Same Name")
	f.slug = "other-folder"
	msg := f.saveFail(t, "same NAME", keyValueStep("two"))
	if !strings.Contains(msg, "proofs/same-name/") || !strings.Contains(msg, "Pick another name") {
		t.Fatalf("message = %q, want a name-taken error", msg)
	}
}

func TestProofEditFlow(t *testing.T) {
	f := newFixture(t)
	f.save(t, "Edit me", keyValueStep("keep"), keyValueStep("replace"), keyValueStep("drop"))
	id := f.idOf(t, "Edit me")
	cur := session(t, f.app, id)

	if err := f.app.W.Proof.SetViewState(bridge.ProofSetViewStateArgs{SessionID: id, Rev: cur.Current.Rev, Ordinal: 1, State: proof.ViewState{}}); err != nil {
		t.Fatal(err)
	}

	f.newest(t)
	if _, err := f.app.W.Proof.OpenWindow(bridge.ProofOpenArgs{TaskID: f.taskID, Slug: "newest"}); err != nil {
		t.Fatal(err)
	}
	r := keyValueStep("replaced")
	r["replacesOrdinal"] = 2
	if c := f.edit(t, "Edit me", map[string]any{"keepOrdinal": 1}, r, keyValueStep("brand new")); !c.Pending {
		t.Fatalf("edit reply = %q, want pending", c.Reply)
	}

	det := session(t, f.app, id)
	if det.Pending == nil {
		t.Fatal("no pending revision")
	}
	var diffs []string
	for _, s := range det.Pending.Steps {
		diffs = append(diffs, s.Diff)
	}
	if strings.Join(diffs, ",") != "unchanged,changed,added" || len(det.Removed) != 1 || det.Removed[0].Description != "drop" {
		t.Fatalf("diffs = %v removed = %+v", diffs, det.Removed)
	}
	f.app.Contract(t, "proof-pending", "ProofService.Session", det, maskIDs)

	// The closed edit waits in the approvals queue; approving it accepts the revision.
	review := reviewItems(t, f.app)
	if len(review) != 1 || review[0].ID != "proof.review:"+id || review[0].Target.Name != "Edit me" {
		t.Fatalf("review items = %+v, want the pending edit", review)
	}
	// Opening the session withdraws the ready popup, never the review of the pending edit.
	if ps := proofPrompts(t, f.app); len(ps) != 1 {
		t.Fatalf("prompts before open = %+v, want the ready popup", ps)
	}
	if _, err := f.app.W.Proof.OpenWindow(bridge.ProofOpenArgs{TaskID: f.taskID, Slug: "edit-me"}); err != nil {
		t.Fatal(err)
	}
	if ps := proofPrompts(t, f.app); len(ps) != 0 {
		t.Fatalf("prompts after open = %+v, want none", ps)
	}
	if left := reviewItems(t, f.app); len(left) != 1 || left[0].ID != "proof.review:"+id {
		t.Fatalf("review items after open = %+v, want the pending edit kept", left)
	}
	if res, err := f.app.W.ApprovalsSvc.Answer(approvals.AnswerArgs{IDs: []string{review[0].ID}, Approve: true}); err != nil || len(res) != 1 || res[0].Error != "" {
		t.Fatalf("approve edit = %+v (%v)", res, err)
	}
	if left := reviewItems(t, f.app); len(left) != 0 {
		t.Fatalf("answered edit still queued: %+v", left)
	}
	det = session(t, f.app, id)
	if det.Pending != nil || len(det.Current.Steps) != 3 {
		t.Fatalf("after accept = %+v", det)
	}
	vs, err := f.app.W.Proof.ViewState(bridge.ProofRevArgs{SessionID: id, Rev: det.Current.Rev})
	if err != nil || len(vs) != 1 || vs[0].Ordinal != 1 {
		t.Fatalf("view state after accept = %+v, %v; want the unchanged step's", vs, err)
	}

	// A rejected edit leaves the current revision alone.
	f.edit(t, "Edit me", map[string]any{"keepOrdinal": 1})
	if len(reviewItems(t, f.app)) != 1 {
		t.Fatal("second edit not queued for review")
	}
	if res, err := f.app.W.ApprovalsSvc.Answer(approvals.AnswerArgs{IDs: []string{"proof.review:" + id}, Approve: false}); err != nil || len(res) != 1 || res[0].Error != "" {
		t.Fatalf("deny edit = %+v (%v)", res, err)
	}
	if det = session(t, f.app, id); det.Pending != nil || len(det.Current.Steps) != 3 {
		t.Fatalf("after reject = %+v", det)
	}

	// The bound methods still work on their own.
	f.edit(t, "Edit me", map[string]any{"keepOrdinal": 1})
	if err := f.app.W.Proof.RejectEdit(bridge.ProofSessionArgs{SessionID: id}); err != nil {
		t.Fatal(err)
	}
	if left := reviewItems(t, f.app); len(left) != 0 {
		t.Fatalf("rejected edit still queued: %+v", left)
	}
	f.edit(t, "Edit me", map[string]any{"keepOrdinal": 1})
	if err := f.app.W.Proof.AcceptEdit(bridge.ProofSessionArgs{SessionID: id}); err != nil {
		t.Fatal(err)
	}
}

func TestProofDelete(t *testing.T) {
	f := newFixture(t)
	f.save(t, "Delete me", keyValueStep("x"))
	id := f.idOf(t, "Delete me")
	if err := f.app.W.Proof.Delete(bridge.ProofSessionArgs{SessionID: id}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(f.folder, "proofs", "delete-me", "proof.md")); !os.IsNotExist(err) {
		t.Fatalf("folder after delete: %v, want gone", err)
	}
	if list := f.proofs(t); len(list) != 0 {
		t.Fatalf("proofs after delete = %+v", list)
	}
	f.save(t, "Delete me", keyValueStep("again"))
}

func TestProofWindow(t *testing.T) {
	f := newFixture(t)
	f.save(t, "Window", keyValueStep("x"))
	id := f.idOf(t, "Window")

	if ps := proofPrompts(t, f.app); len(ps) != 1 {
		t.Fatalf("prompts before open = %+v, want one", ps)
	}
	res, err := f.app.W.Proof.OpenWindow(bridge.ProofOpenArgs{TaskID: f.taskID, Slug: "window"})
	if ps := proofPrompts(t, f.app); err == nil && len(ps) != 0 {
		t.Fatalf("prompts after open = %+v, want the ready popup withdrawn", ps)
	}
	if err != nil || !res.Opened || !strings.HasPrefix(res.Key, proof.WindowKeyPrefix) {
		t.Fatalf("OpenWindow = %+v, %v", res, err)
	}
	f.app.Contract(t, "proof-window", "ProofService.OpenWindow", res, maskIDs)
	tg, err := f.app.W.Proof.WindowTarget(bridge.ProofWindowKeyArgs{Key: res.Key})
	if err != nil || tg.SessionID == nil || *tg.SessionID != id {
		t.Fatalf("WindowTarget = %+v, %v", tg, err)
	}
	again, err := f.app.W.Proof.OpenWindow(bridge.ProofOpenArgs{TaskID: f.taskID, Slug: "window"})
	if err != nil || again.Opened || again.Key != res.Key {
		t.Fatalf("second OpenWindow = %+v, %v; want the same window raised", again, err)
	}
	if err := f.app.W.Proof.MarkSeen(bridge.ProofSessionArgs{SessionID: id}); err != nil {
		t.Fatal(err)
	}
}
