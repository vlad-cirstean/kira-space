package adeflow_test

import (
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/internal/approvals"
)

// Edit approvals by author: an agent's save asks the user only for another agent's content. Contract
// ade-details-rules: tests/ui/ade-v2-details-rules.spec.ts reads these and runs the same scenario
// through the UI. The decision table itself (R1-R15) is covered by taskfiles TestDecide; these flows
// run the rows that reach the user through the real stores.

const drScenario = "ade-details-rules"

const drLinks = "links:\n  - {kind: doc, title: Design notes, ref: docs/design.md}\n"

func TestDetailsEditRules(t *testing.T) {
	f := newBRFixture(t, nil, onceStepYML, false)
	app := f.app
	id := f.task.ID
	folder := dfFolder(t, app, id)
	const olderBody = "Agent note."
	older, newer := dwNotes(t, app, id, folder, olderBody)

	t.Run("R1 user note edit applies", func(t *testing.T) {
		dfWrite(t, folder, newer, "Newer note, tuned.")
		if reply := dfSync(t, app, id, newer); !strings.HasPrefix(reply, "accepted 1, pending user 0") {
			t.Fatalf("reply = %q", reply)
		}
		if !dfNoItem(t, app, id) {
			t.Fatal("details approval open for a user note")
		}
	})

	t.Run("R2 another agent's older note waits", func(t *testing.T) {
		dfWrite(t, folder, older, "Agent note, edited.")
		reply := dfSync(t, app, id, older)
		if !strings.Contains(reply, "pending user 1") || !strings.Contains(reply, "another agent's note") {
			t.Fatalf("reply = %q, want pending on another agent's note", reply)
		}
		item := dfDetailsItem(t, app, id)
		app.Contract(t, drScenario, "ApprovalsService.List#otherAgentNote", []approvals.Item{item}, dfMask)
		dfAnswer(t, app, id, false)
		if got := dfRead(t, folder, older); got != olderBody {
			t.Fatalf("note after deny = %q, want it restored", got)
		}
	})

	t.Run("R3 own latest note edit applies", func(t *testing.T) {
		own := "notes/20261102T100000-s1.md"
		dfWrite(t, folder, own, "Mine.")
		if reply := dfSync(t, app, id, own); !strings.HasPrefix(reply, "accepted 1, pending user 0") {
			t.Fatalf("create reply = %q", reply)
		}
		dfWrite(t, folder, own, "Mine, edited.")
		if reply := dfSync(t, app, id, own); !strings.HasPrefix(reply, "accepted 1, pending user 0") {
			t.Fatalf("own edit reply = %q", reply)
		}
		if !dfNoItem(t, app, id) {
			t.Fatal("details approval open for the agent's own note")
		}
		app.Contract(t, drScenario, "AdeTaskService.Handovers#ownApplied", adewire.HandoversResult{Handovers: handovers(t, app, id)}, dfMask)
	})

	t.Run("R4 another agent's link edit waits", func(t *testing.T) {
		dfWrite(t, folder, "links.yaml", drLinks)
		if reply := dfSyncAs(t, app, id, "session-2", "links.yaml"); !strings.HasPrefix(reply, "accepted 1, pending user 0") {
			t.Fatalf("session-2 link reply = %q", reply)
		}
		dfWrite(t, folder, "links.yaml", "links:\n  - {kind: doc, title: Design notes v2, ref: docs/design.md}\n")
		reply := dfSync(t, app, id, "links.yaml")
		if !strings.Contains(reply, "pending user 1") || !strings.Contains(reply, "another agent's") {
			t.Fatalf("reply = %q, want pending on another agent's link", reply)
		}
		item := dfDetailsItem(t, app, id)
		app.Contract(t, drScenario, "ApprovalsService.List#otherAgentLink", []approvals.Item{item}, dfMask)
		dfAnswer(t, app, id, false)
		if a, ok := findArtifact(artifactsOf(t, app, id), "doc", "Design notes"); !ok || a.Source != "agent" {
			t.Fatalf("artifacts = %+v, want the original link kept", artifactsOf(t, app, id).Artifacts)
		}
	})

	t.Run("R5 outside save with no live agent is a banner only", func(t *testing.T) {
		dfWrite(t, folder, newer, "Edited in an outside editor.")
		time.Sleep(3 * 300 * time.Millisecond)
		var found bool
		for _, c := range dfDisk(t, app, id) {
			if c.Path == newer && c.State == "edited" && !c.Pending {
				found = true
			}
		}
		if !found {
			t.Fatalf("disk changes = %+v, want the outside save listed", dfDisk(t, app, id))
		}
		if !dfNoItem(t, app, id) {
			t.Fatal("details approval raised with no agent to credit the save to")
		}
		app.Contract(t, drScenario, "AdeTaskService.DetailsDiskChanges#outside", dfDisk(t, app, id), dfMask)
	})
}
