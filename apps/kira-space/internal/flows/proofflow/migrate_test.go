package proofflow

import (
	"bytes"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/internal/approvals"
	"github.com/kirathecat/kira-studio/internal/testx"
)

type legacyStep struct {
	view, desc, payload string
}

// legacyProof inserts a closed proof session as the tool era stored it: linked to a task, no folder.
func legacyProof(t *testing.T, f *fixture, id, name string, taskID string, steps []legacyStep, pending []legacyStep) {
	t.Helper()
	db := f.app.DB()
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	pend := any(nil)
	if pending != nil {
		pend = 2
	}
	exec(`INSERT INTO proof_sessions (id, state, current_rev, pending_rev, created_at, closed_at, updated_at, name, name_key, summary) VALUES (?, 'closed', 1, ?, 1000, 2000, 2000, ?, ?, 'old summary')`,
		id, pend, name, strings.ToLower(name))
	revs := map[int][]legacyStep{1: steps}
	if pending != nil {
		revs[2] = pending
	}
	for rev, list := range revs {
		state := "current"
		if rev == 2 {
			state = "pending"
		}
		exec(`INSERT INTO proof_revisions (session_id, rev, state, created_at, closed_at, reason) VALUES (?, ?, ?, 1000, 2000, ?)`, id, rev, state, map[bool]string{true: "tighten", false: ""}[rev == 2])
		for i, s := range list {
			exec(`INSERT INTO proof_steps (session_id, rev, ordinal, view, diff, payload_bytes, origin_ordinal, description, query, language, source, payload) VALUES (?, ?, ?, ?, 'added', ?, ?, ?, '', 'text', '', ?)`,
				id, rev, i+1, s.view, len(s.payload), map[bool]any{true: i + 1, false: nil}[rev == 2 && i == 0], s.desc, []byte(s.payload))
		}
	}
	if taskID != "" {
		exec(`INSERT INTO proof_task_links (session_id, task_id, source, linked_at) VALUES (?, ?, 'agent', 3000)`, id, taskID)
	}
}

// Linked sessions become folders with their images as .jpg files; a pending edit lands on disk with its
// review pending; taskless sessions are deleted.
func TestProofMigration(t *testing.T) {
	f := newFixture(t)
	jpg := jpegBytes(t, 32, 24)
	img, _ := json.Marshal(map[string]any{"width": 32, "height": 24, "bytes": len(jpg), "base64": base64.StdEncoding.EncodeToString(jpg)})
	kv := `{"entries":[{"field":"rows","value":"1"}]}`
	legacyProof(t, f, "legacy-a", "Old proof", f.taskID,
		[]legacyStep{{"keyvalue", "Stats", kv}, {"image", "Screenshot", string(img)}}, nil)
	legacyProof(t, f, "legacy-b", "Edited proof", f.taskID,
		[]legacyStep{{"keyvalue", "Keep", kv}},
		[]legacyStep{{"keyvalue", "Keep", kv}, {"keyvalue", "New step", `{"entries":[{"field":"x","value":"2"}]}`}})
	legacyProof(t, f, "legacy-c", "Nobody's proof", "", []legacyStep{{"keyvalue", "Stats", kv}}, nil)

	// The first boot's migration may have marked the task done before the legacy rows existed; shutdown
	// ends it, then the markers go so the next boot migrates the planted rows.
	f.app.RestartAfter(func(db *sql.DB) {
		if _, err := db.Exec(`DELETE FROM task_files_migration`); err != nil {
			t.Fatal(err)
		}
	})
	// The migration runs in the background at start.
	testx.WaitUntil(t, 10*time.Second, func() bool {
		var n int
		return f.app.DB().QueryRow(`SELECT count(*) FROM task_files_migration WHERE part = 'proofs' AND state = 'done'`).Scan(&n) == nil && n == 1
	})

	byName := map[string]string{}
	for _, p := range f.proofs(t) {
		byName[p.Name] = p.Slug
	}
	if byName["Old proof"] != "old-proof" || byName["Edited proof"] != "edited-proof" || len(byName) != 2 {
		t.Fatalf("proofs = %+v, want the two linked sessions with slugs", byName)
	}
	md := string(f.read(t, "proofs/old-proof/proof.md"))
	mustHave(t, "old-proof/proof.md", md, "name: Old proof", "id: s1", "id: s2", "data: s2.jpg")
	if !bytes.Equal(f.read(t, "proofs/old-proof/s2.jpg"), jpg) {
		t.Fatal("the image file differs from the stored bytes")
	}
	id := f.idOf(t, "Old proof")
	st, err := f.app.W.Proof.Step(bridge.ProofStepArgs{SessionID: id, Rev: 1, Ordinal: 2})
	if err != nil {
		t.Fatal(err)
	}
	mustHave(t, "image step", string(st.Data), `"format":"jpeg"`, `"file":"s2.jpg"`)

	// The pending edit is the disk version and waits for review.
	pend := string(f.read(t, "proofs/edited-proof/proof.md"))
	mustHave(t, "edited-proof/proof.md", pend, "New step")
	items := reviewItems(t, f.app)
	if len(items) != 1 || items[0].ID != approvals.KindProofReview+":legacy-b" {
		t.Fatalf("review items = %+v, want the migrated pending edit", items)
	}
	if res, err := f.app.W.ApprovalsSvc.Answer(approvals.AnswerArgs{IDs: []string{items[0].ID}, Approve: false}); err != nil || res[0].Error != "" {
		t.Fatalf("deny = %+v, %v", res, err)
	}
	if strings.Contains(string(f.read(t, "proofs/edited-proof/proof.md")), "New step") {
		t.Fatal("deny did not restore the accepted version")
	}

	// Taskless sessions are gone.
	var n int
	if err := f.app.DB().QueryRow(`SELECT count(*) FROM proof_sessions WHERE id = 'legacy-c'`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("taskless session rows = %d, %v, want 0", n, err)
	}
}
