package proofflow

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/appshell"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/approvals"
)

// P643 Part 4 proofs as files. Contract proof-files: tests/ui/ade-v2-proof-chain.spec.ts reads these.

const filesScenario = "proof-files"

var filesMask = flowharness.Mask("id", "proofId", "taskId", "sessionId", "createdAt", "updatedAt", "closedAt", "linkedAt", "version")

func (f *fixture) put(t *testing.T, rel string, data []byte) {
	t.Helper()
	p := filepath.Join(f.folder, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func (f *fixture) read(t *testing.T, rel string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(f.folder, rel))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func (f *fixture) exists(rel string) bool {
	_, err := os.Stat(filepath.Join(f.folder, rel))
	return err == nil
}

func mustHave(t *testing.T, what, got string, want ...string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(got, w) {
			t.Fatalf("%s lacks %q:\n%s", what, w, got)
		}
	}
}

const filesProofMD = `---
name: Checkout fix
summary: Checkout rejects an expired card.
steps:
  - id: orders
    description: Orders table shows declined for order 3
    view: table
    language: sql
    source: postgres
    query: select id, status from orders
    inline:
      columns:
        - {name: id, type: number}
        - {name: status, type: text}
      rows:
        - [3, declined]
  - id: shot
    description: The error banner on the payment page
    view: image
    data: shot.jpg
  - id: audit
    description: Audit rows, kept in a side file
    view: table
    data: audit.json
---
`

func TestProofFiles(t *testing.T) {
	f := newFixture(t)
	jpg := jpegBytes(t, 64, 48)
	const audit = `{"columns":[{"name":"n","type":"number"}],"rows":[[1],[2]]}`

	// 1. A new proof folder plus submit: accepted, linked, the ready prompt raised, the reply names it.
	f.put(t, "proofs/checkout-fix/proof.md", []byte(filesProofMD))
	f.put(t, "proofs/checkout-fix/shot.jpg", jpg)
	f.put(t, "proofs/checkout-fix/audit.json", []byte(audit))
	reply := f.submit(t)
	mustHave(t, "first submit reply", reply, "accepted 1", `new proof "Checkout fix"`, "orders (table)", "shot (image)", "audit (table)", "3 steps")
	list := f.proofs(t)
	if len(list) != 1 || list[0].Slug != "checkout-fix" || len(list[0].Steps) != 3 || list[0].Steps[1].ID != "shot" {
		t.Fatalf("proofs = %+v", list)
	}
	f.app.Contract(t, filesScenario, "AdeTaskService.TaskProofs", f.proofs(t), filesMask)
	f.app.Contract(t, filesScenario, "reply#new", reply)
	id := list[0].ProofID
	for i, key := range []string{"table", "image", "data-file"} {
		st, err := f.app.W.Proof.Step(bridge.ProofStepArgs{SessionID: id, Rev: 1, Ordinal: i + 1})
		if err != nil {
			t.Fatal(err)
		}
		f.app.Contract(t, filesScenario, "ProofService.Step#"+key, st, filesMask)
	}

	// Open and reveal go to the proof's folder inside the task folder.
	if err := f.app.W.Proof.OpenFolder(bridge.ProofSessionArgs{SessionID: id}); err != nil {
		t.Fatal(err)
	}
	if err := f.app.W.Proof.RevealFolder(bridge.ProofSessionArgs{SessionID: id}); err != nil {
		t.Fatal(err)
	}
	for name, got := range map[string][]string{"open": f.app.FileOpener.OpenedFiles(), "reveal": f.app.FileOpener.RevealedFiles()} {
		if len(got) != 1 || !strings.HasSuffix(got[0], "/proofs/checkout-fix") {
			t.Fatalf("%s paths = %v, want the proof folder", name, got)
		}
	}

	// 2. Nothing changed: unchanged.
	mustHave(t, "second submit", f.submit(t), "accepted 0", "invalid 0")

	// 3. README and prompt name the proof; the README holds the example.
	readme := string(f.read(t, "README.md"))
	mustHave(t, "README", readme, `Proofs: checkout-fix "Checkout fix"`, "inline:")

	// 4. Over-cap inline data is refused with the fix.
	big := "---\nname: Big\nsummary: x\nsteps:\n  - id: rows\n    description: Many rows\n    view: table\n    inline:\n      columns:\n        - {name: a, type: text}\n      rows:\n" +
		strings.Repeat("        - [\""+strings.Repeat("x", 100)+"\"]\n", 700) + "---\n"
	f.put(t, "proofs/big/proof.md", []byte(big))
	msg := f.submit(t)
	mustHave(t, "over-cap reply", msg, "proofs/big/proof.md", "steps[rows].inline is 71 KiB", "Move it to <this folder>/rows.json")
	f.app.Contract(t, filesScenario, "reply#overcap", strings.ReplaceAll(msg, "\n", " | "))
	_ = os.RemoveAll(filepath.Join(f.folder, "proofs", "big"))
	f.submit(t)

	// 5. proof.md names a missing image: the whole unit is invalid and no file is touched; the image
	// written later plus submit accepts the proof as new.
	md := "---\nname: Login page\nsummary: x\nsteps:\n  - id: shot\n    description: The login page\n    view: image\n    data: shot.png\n---\n"
	f.put(t, "proofs/login-page/proof.md", []byte(md))
	msg = f.submit(t)
	mustHave(t, "missing image reply", msg, "proofs/login-page/proof.md:8", "shot.png not found in this folder")
	if string(f.read(t, "proofs/login-page/proof.md")) != md || len(f.proofs(t)) != 1 {
		t.Fatalf("an invalid unit changed files or rows: %+v", f.proofs(t))
	}
	f.put(t, "proofs/login-page/shot.png", pngBytes(t))
	mustHave(t, "image written later", f.submit(t), `new proof "Login page"`)

	// 6. A side file nothing names is invalid.
	f.put(t, "proofs/login-page/extra.jpg", jpg)
	mustHave(t, "unreferenced file", f.submit(t), "proofs/login-page/extra.jpg", "not used by proof.md")
	_ = os.Remove(filepath.Join(f.folder, "proofs/login-page/extra.jpg"))
	f.submit(t)

	// 7. A folder name that breaks the slug rule says the slug to use.
	f.put(t, "proofs/Bad Name/proof.md", []byte(md))
	mustHave(t, "bad folder", f.submit(t), "proofs/Bad Name/", "proofs/bad-name/")
	_ = os.RemoveAll(filepath.Join(f.folder, "proofs", "Bad Name"))
	f.submit(t)

	// 8. A second folder with a taken name names the other folder.
	f.put(t, "proofs/dup/proof.md", []byte(strings.Replace(md, "Login page", "login PAGE", 1)))
	f.put(t, "proofs/dup/shot.png", pngBytes(t))
	mustHave(t, "duplicate name", f.submit(t), "proofs/dup/proof.md", "proofs/login-page/", "Pick another name")
	_ = os.RemoveAll(filepath.Join(f.folder, "proofs", "dup"))
	f.submit(t)

	// 9. A YAML error answers file:line and the fix.
	f.put(t, "proofs/tabs/proof.md", []byte("---\nname: Tabs\nsummary: x\nsteps:\n\t- id: a\n---\n"))
	msg = f.submit(t)
	mustHave(t, "yaml error", msg, "proofs/tabs/proof.md:5", "tab")
	f.app.Contract(t, filesScenario, "reply#yaml", strings.ReplaceAll(msg, "\n", " | "))
	_ = os.RemoveAll(filepath.Join(f.folder, "proofs", "tabs"))
	f.submit(t)

	// 10. An edit by step id from another agent (session-2) of an older proof waits for review.
	edited := strings.Replace(filesProofMD, "select id, status from orders", "select id, status from orders where id = 3", 1)
	f.put(t, "proofs/checkout-fix/proof.md", []byte(edited))
	if err := f.app.W.Proof.SetViewState(bridge.ProofSetViewStateArgs{SessionID: id, Rev: 1, Ordinal: 3}); err != nil {
		t.Fatal(err)
	}
	reply = f.submitAs(t, "session-2")
	mustHave(t, "edit by another agent", reply, "pending user 1", "orders")
	for _, p := range f.proofs(t) {
		if p.ProofID == id && !p.Pending {
			t.Fatalf("proof = %+v, want the edit pending", p)
		}
	}
	items := reviewItems(t, f.app)
	if len(items) != 1 || items[0].ID != "proof.review:"+id {
		t.Fatalf("review items = %+v", items)
	}
	f.app.Contract(t, filesScenario, "reply#pending", reply)

	// 11. A resave of the pending edit replaces its approval item.
	f.submitAs(t, "session-2")
	if again := reviewItems(t, f.app); len(again) != 1 {
		t.Fatalf("review items after resubmit = %+v, want one", again)
	}

	// 12. Deny restores the files byte for byte, the image included.
	if res, err := f.app.W.ApprovalsSvc.Answer(approvals.AnswerArgs{IDs: []string{"proof.review:" + id}, Approve: false}); err != nil || res[0].Error != "" {
		t.Fatalf("deny = %+v, %v", res, err)
	}
	if string(f.read(t, "proofs/checkout-fix/proof.md")) != filesProofMD || !bytes.Equal(f.read(t, "proofs/checkout-fix/shot.jpg"), jpg) {
		t.Fatal("deny did not restore the accepted files")
	}

	// 13. Approve keeps the view state by step id.
	f.put(t, "proofs/checkout-fix/proof.md", []byte(edited))
	f.submitAs(t, "session-2")
	if res, err := f.app.W.ApprovalsSvc.Answer(approvals.AnswerArgs{IDs: []string{"proof.review:" + id}, Approve: true}); err != nil || res[0].Error != "" {
		t.Fatalf("approve = %+v, %v", res, err)
	}
	det := session(t, f.app, id)
	vs, err := f.app.W.Proof.ViewState(bridge.ProofRevArgs{SessionID: id, Rev: det.Current.Rev})
	if err != nil || det.Pending != nil || len(vs) != 1 || vs[0].Ordinal != 3 {
		t.Fatalf("after approve: pending %v, view state %+v, %v; want step audit's kept", det.Pending, vs, err)
	}
	f.app.Contract(t, filesScenario, "ProofService.Session#edited", det, filesMask)

	// 14. The user deletes the proof: the folder and the rows go.
	if err := f.app.W.Proof.Delete(bridge.ProofSessionArgs{SessionID: id}); err != nil {
		t.Fatal(err)
	}
	if f.exists("proofs/checkout-fix/proof.md") || f.exists("proofs/checkout-fix/shot.jpg") {
		t.Fatal("files left after the user's delete")
	}
	for _, p := range f.proofs(t) {
		if p.ProofID == id {
			t.Fatalf("rows left after the user's delete: %+v", p)
		}
	}
}

// The proof tool and every way in besides the folder are gone.
func TestProofRemovals(t *testing.T) {
	f := newFixture(t)
	for _, sec := range appshell.BuildTemplate("Kira Space", false) {
		for _, it := range sec.Items {
			if strings.Contains(strings.ToLower(it.Label+it.Action+it.Channel), "validation proof") {
				t.Fatalf("menu item %+v still opens proofs", it)
			}
		}
	}
	if _, err := f.app.W.Proof.OpenWindow(bridge.ProofOpenArgs{}); err == nil {
		t.Fatal("OpenWindow without a task and slug succeeded")
	}
	if _, err := f.app.W.Proof.OpenWindow(bridge.ProofOpenArgs{TaskID: f.taskID, Slug: "nope"}); err == nil {
		t.Fatal("OpenWindow of an unknown proof succeeded")
	}
	raw, _ := json.Marshal(f.app.W.DbMcp.Status())
	if strings.Contains(string(raw), "proof_") {
		t.Fatalf("db MCP status mentions a proof tool: %s", raw)
	}
}
