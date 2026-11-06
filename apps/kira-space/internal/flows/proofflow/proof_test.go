package proofflow

import (
	"bytes"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/proof"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studioflowharness"
	"github.com/kirathecat/kira-studio/internal/approvals"
)

var maskIDs = studioflowharness.Mask("id", "closedAt", "createdAt")

func TestProofCloseAndRead(t *testing.T) {
	f := newFixture(t)
	d := f.start(t, "Checkout fix")
	f.addStep(t, d, tableStep("Orders before"))
	f.addStep(t, d, documentStep("Doc hit"))
	f.addStep(t, d, keyValueStep("Stats"))
	c := f.closeDraft(t, d)
	if c.Name != "Checkout fix" || c.Steps != 3 {
		t.Fatalf("close = %+v", c)
	}

	list, err := f.app.W.Proof.Sessions()
	if err != nil || len(list) != 1 {
		t.Fatalf("Sessions = %+v, %v", list, err)
	}
	f.app.Contract(t, "proof", "ProofService.Sessions", list, maskIDs)

	det := session(t, f.app, list[0].ID)
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
		st, err := f.app.W.Proof.Step(bridge.ProofStepArgs{SessionID: list[0].ID, Rev: det.Current.Rev, Ordinal: i + 1})
		if err != nil {
			t.Fatal(err)
		}
		f.app.Contract(t, "proof", "ProofService.Step#"+key, st, maskIDs)
	}

	// Large integers survive as written.
	st, _ := f.app.W.Proof.Step(bridge.ProofStepArgs{SessionID: list[0].ID, Rev: det.Current.Rev, Ordinal: 1})
	if !bytes.Contains(st.Data, []byte("9007199254740993")) {
		t.Fatalf("integer precision lost: %s", st.Data)
	}
	// The proof window's cell editor shows this cell (tests/ui/proof-window.spec.ts).
	if !bytes.Contains(st.Data, []byte(`"alpha"`)) {
		t.Fatalf("first-row name missing: %s", st.Data)
	}

	if ps := proofPrompts(t, f.app); len(ps) != 1 || ps[0].Ref != list[0].ID {
		t.Fatalf("prompts = %+v, want one for %s", ps, list[0].ID)
	}
}

func TestProofNameCollision(t *testing.T) {
	f := newFixture(t)
	d := f.start(t, "Same Name")
	f.addStep(t, d, keyValueStep("one"))
	f.closeDraft(t, d)
	msg := f.fail(t, "proof_start_session", map[string]any{"name": "  same NAME "})
	if !strings.Contains(msg, "already") {
		t.Fatalf("message = %q, want a name-taken error", msg)
	}
}

func TestProofEditFlow(t *testing.T) {
	f := newFixture(t)
	d := f.start(t, "Edit me")
	f.addStep(t, d, keyValueStep("keep"))
	f.addStep(t, d, keyValueStep("replace"))
	f.addStep(t, d, keyValueStep("drop"))
	f.closeDraft(t, d)
	id := sessionID(t, f.app, "Edit me")
	cur := session(t, f.app, id)

	if err := f.app.W.Proof.SetViewState(bridge.ProofSetViewStateArgs{SessionID: id, Rev: cur.Current.Rev, Ordinal: 1, State: proof.ViewState{}}); err != nil {
		t.Fatal(err)
	}

	var ed proof.StartResult
	f.call(t, "proof_edit_session", map[string]any{"name": "Edit me", "reason": "tighten"}, &ed)
	if len(ed.Steps) != 3 {
		t.Fatalf("edit draft steps = %+v", ed.Steps)
	}
	f.addStep(t, ed.DraftID, map[string]any{"keepOrdinal": 1})
	r := keyValueStep("replaced")
	r["replacesOrdinal"] = 2
	f.addStep(t, ed.DraftID, r)
	f.addStep(t, ed.DraftID, keyValueStep("brand new"))
	f.closeDraft(t, ed.DraftID)

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
	f.call(t, "proof_edit_session", map[string]any{"name": "Edit me", "reason": "again"}, &ed)
	f.addStep(t, ed.DraftID, map[string]any{"keepOrdinal": 1})
	f.closeDraft(t, ed.DraftID)
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
	f.call(t, "proof_edit_session", map[string]any{"name": "Edit me", "reason": "direct"}, &ed)
	f.addStep(t, ed.DraftID, map[string]any{"keepOrdinal": 1})
	f.closeDraft(t, ed.DraftID)
	if err := f.app.W.Proof.RejectEdit(bridge.ProofSessionArgs{SessionID: id}); err != nil {
		t.Fatal(err)
	}
	if left := reviewItems(t, f.app); len(left) != 0 {
		t.Fatalf("rejected edit still queued: %+v", left)
	}
	f.call(t, "proof_edit_session", map[string]any{"name": "Edit me", "reason": "direct accept"}, &ed)
	f.addStep(t, ed.DraftID, map[string]any{"keepOrdinal": 1})
	f.closeDraft(t, ed.DraftID)
	if err := f.app.W.Proof.AcceptEdit(bridge.ProofSessionArgs{SessionID: id}); err != nil {
		t.Fatal(err)
	}
}

func TestProofRetentionAndDelete(t *testing.T) {
	f := newFixture(t)
	for i := 0; i < 31; i++ {
		d := f.start(t, "s"+string(rune('A'+i/26))+string(rune('a'+i%26)))
		f.addStep(t, d, keyValueStep("x"))
		f.closeDraft(t, d)
	}
	list, err := f.app.W.Proof.Sessions()
	if err != nil || len(list) != 30 {
		t.Fatalf("kept %d sessions (%v), want 30", len(list), err)
	}
	for _, s := range list {
		if s.Name == "sAa" {
			t.Fatal("oldest session survived")
		}
	}
	if err := f.app.W.Proof.Delete(bridge.ProofSessionArgs{SessionID: list[0].ID}); err != nil {
		t.Fatal(err)
	}
	f.start(t, list[0].Name)
}

func TestProofParallelAddStep(t *testing.T) {
	f := newFixture(t)
	d := f.start(t, "Parallel")
	const n = 12
	got := make([]int, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got[i] = f.addStep(t, d, keyValueStep("p"))
		}()
	}
	wg.Wait()
	seen := map[int]bool{}
	for _, o := range got {
		seen[o] = true
	}
	for o := 1; o <= n; o++ {
		if !seen[o] {
			t.Fatalf("ordinals = %v, want 1..%d with no gap", got, n)
		}
	}
}

func TestProofLimits(t *testing.T) {
	f := newFixture(t)
	d := f.start(t, "Limits")
	long := strings.Repeat("x", (64<<10)+1)
	for name, args := range map[string]map[string]any{
		"oversize cell": {"description": "d", "view": "table", "data": map[string]any{
			"columns": []any{map[string]any{"name": "a"}}, "rows": []any{[]any{long}}}},
		"short row": {"description": "d", "view": "table", "data": map[string]any{
			"columns": []any{map[string]any{"name": "a"}, map[string]any{"name": "b"}}, "rows": []any{[]any{1}}}},
		"unknown view": {"description": "d", "view": "heatmap", "data": map[string]any{}},
	} {
		args["draftId"] = d
		if msg := f.fail(t, "proof_add_step", args); msg == "" {
			t.Fatalf("%s: empty error", name)
		}
	}
	for i := 0; i < 100; i++ {
		f.addStep(t, d, keyValueStep("s"))
	}
	if msg := f.fail(t, "proof_add_step", withDraft(keyValueStep("one too many"), d)); !strings.Contains(msg, "100") {
		t.Fatalf("message = %q, want the step limit", msg)
	}
}

func withDraft(a map[string]any, d string) map[string]any { a["draftId"] = d; return a }

func TestProofWindowAndSavePng(t *testing.T) {
	f := newFixture(t)
	d := f.start(t, "Window")
	f.addStep(t, d, keyValueStep("x"))
	f.closeDraft(t, d)
	id := sessionID(t, f.app, "Window")

	res, err := f.app.W.Proof.OpenWindow(bridge.ProofOpenArgs{SessionID: id})
	if err != nil || !res.Opened || !strings.HasPrefix(res.Key, proof.WindowKeyPrefix) {
		t.Fatalf("OpenWindow = %+v, %v", res, err)
	}
	tg, err := f.app.W.Proof.WindowTarget(bridge.ProofWindowKeyArgs{Key: res.Key})
	if err != nil || tg.SessionID == nil || *tg.SessionID != id {
		t.Fatalf("WindowTarget = %+v, %v", tg, err)
	}
	again, err := f.app.W.Proof.OpenWindow(bridge.ProofOpenArgs{SessionID: id})
	if err != nil || again.Opened || again.Key != res.Key {
		t.Fatalf("second OpenWindow = %+v, %v; want the same window raised", again, err)
	}
	if err := f.app.W.Proof.MarkSeen(bridge.ProofSessionArgs{SessionID: id}); err != nil {
		t.Fatal(err)
	}

	png := append([]byte("\x89PNG\r\n\x1a\n"), 1, 2, 3)
	out := filepath.Join(t.TempDir(), "shot.png")
	f.app.Dialogs.QueueSaveFile(out)
	sv, err := f.app.W.Proof.SavePng(bridge.ProofSavePngArgs{Filename: "../a b.png", PngBase64: base64.StdEncoding.EncodeToString(png)})
	if err != nil || !sv.Saved {
		t.Fatalf("SavePng = %+v, %v", sv, err)
	}
	if b, err := os.ReadFile(out); err != nil || !bytes.Equal(b, png) {
		t.Fatalf("saved file = %v, %v", b, err)
	}
	if _, err := f.app.W.Proof.SavePng(bridge.ProofSavePngArgs{Filename: "x.png", PngBase64: base64.StdEncoding.EncodeToString([]byte("nope"))}); err == nil {
		t.Fatal("non-PNG accepted")
	}
}
