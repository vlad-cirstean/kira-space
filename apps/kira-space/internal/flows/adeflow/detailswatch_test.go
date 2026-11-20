package adeflow_test

import (
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/approvals"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// Task file watcher: while an agent run is live, Kira applies its saved free edits, writes the errors
// into state.yaml fileErrors and asks no approval for the user's notes or the latest note. A save that
// edits another agent's older note raises the details.edit approval at once, with no error filed
// against the agent. Contract ade-details-watch: tests/ui/ade-v2-details-watch.spec.ts reads these and
// runs the same scenario through the UI.

const dwScenario = "ade-details-watch"

type fileErr struct {
	path, msg string
	line      int
}

// dwErrors is the fileErrors section of state.yaml.
func dwErrors(t *testing.T, f *brFixture) []fileErr {
	t.Helper()
	doc := stateYAML(t, f.app, f.task.ID, nil)
	list, _ := doc["fileErrors"].([]any)
	var out []fileErr
	for _, e := range list {
		m := e.(map[string]any)
		fe := fileErr{path: m["path"].(string), msg: m["error"].(string)}
		fe.line, _ = m["line"].(int)
		out = append(out, fe)
	}
	return out
}

func dwErrorOf(t *testing.T, f *brFixture, path string) (fileErr, bool) {
	t.Helper()
	for _, e := range dwErrors(t, f) {
		if e.path == path {
			return e, true
		}
	}
	return fileErr{}, false
}

func dwWaitError(t *testing.T, f *brFixture, path string) fileErr {
	t.Helper()
	var got fileErr
	testx.WaitUntil(t, waitFor, func() bool {
		e, ok := dwErrorOf(t, f, path)
		got = e
		return ok
	})
	return got
}

func dwWaitNoError(t *testing.T, f *brFixture, path string) {
	t.Helper()
	testx.WaitUntil(t, waitFor, func() bool {
		_, ok := dwErrorOf(t, f, path)
		return !ok
	})
}

// dwWaitDisk waits until the disk banner lists exactly the given paths.
func dwWaitDisk(t *testing.T, f *brFixture, paths ...string) {
	t.Helper()
	testx.WaitUntil(t, waitFor, func() bool {
		var got []string
		for _, c := range dfDisk(t, f.app, f.task.ID) {
			got = append(got, c.Path)
		}
		sort.Strings(got)
		return slices.Equal(got, paths)
	})
}

func dwNoDetailsItem(t *testing.T, f *brFixture) bool {
	t.Helper()
	all, err := f.app.W.ApprovalsSvc.List()
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range all {
		if it.ID == approvals.KindDetailsEdit+":"+f.task.ID {
			return false
		}
	}
	return true
}

// dwNotes writes another agent's note (session-2), then adds a newer user note, and returns the rels of
// the older agent note and the newer note.
func dwNotes(t *testing.T, app *flowharness.App, taskID, folder, agentBody string) (older, newer string) {
	t.Helper()
	older = "notes/20261101T100000-s2.md"
	dfWrite(t, folder, older, agentBody)
	if reply := dfSyncAs(t, app, taskID, "session-2", older); !strings.HasPrefix(reply, "accepted 1, pending user 0, invalid 0") {
		t.Fatalf("session-2 note reply = %q", reply)
	}
	const newBody = "Newer note."
	if _, err := app.W.AdeTask.AddNote(ctx, adewire.AddNoteArgs{TaskID: taskID, Body: newBody}); err != nil {
		t.Fatal(err)
	}
	for _, rel := range dfNotes(t, folder) {
		if dfRead(t, folder, rel) == newBody {
			newer = rel
		}
	}
	if newer == "" {
		t.Fatalf("notes = %v, want the newer note", dfNotes(t, folder))
	}
	return older, newer
}

func dwHold(t *testing.T) string {
	t.Helper()
	gate := filepath.Join(t.TempDir(), "gate")
	t.Cleanup(func() { _ = os.WriteFile(gate, nil, 0o644) })
	return gate
}

func TestDetailsWatch(t *testing.T) {
	gate := dwHold(t)
	hold := []fakeagent.Action{{WaitFile: gate, Name: "done"}}
	f := newBRFixture(t, hold, onceStepYML, false)
	app := f.app
	folder := dfFolder(t, app, f.task.ID)
	const olderBody = "Agent note."
	older, newer := dwNotes(t, app, f.task.ID, folder, olderBody)

	// 1. The run saves four files with no call: the draft, the newest note, an invalid links.yaml and
	// another agent's older note.
	claude(app, map[string][]fakeagent.Action{"*": {{
		TaskFiles: map[string]string{
			"handovers/draft-one.md": "Draft v1.",
			newer:                    "Newer note, edited.",
			"links.yaml":             "links:\n  - {kind: doc\n",
			older:                    "Agent note, edited.",
		},
		WaitFile: gate, Name: "done",
	}}})
	f.startRun(t)

	// The draft and the newest note apply at once, with no approval.
	testx.WaitUntil(t, waitFor, func() bool {
		var body string
		for _, h := range handovers(t, app, f.task.ID) {
			if h.State == "draft" && h.Body == "Draft v1." {
				body = h.Body
			}
		}
		return body != "" && dfRead(t, folder, newer) == "Newer note, edited."
	})
	testx.WaitUntil(t, waitFor, func() bool {
		for _, h := range dfHandoverBodies(t, app, f.task.ID) {
			if h == "note: Newer note, edited." {
				return true
			}
		}
		return false
	})
	// 2. The invalid file is an error in state.yaml and the agent is mid-fix: no banner for it. The older
	// note of another agent raises the approval at once: no error, the held text stays, and the banner
	// and state.yaml pending list it.
	links := dwWaitError(t, f, "links.yaml")
	if links.line == 0 || links.msg == "" {
		t.Fatalf("links.yaml error = %+v, want a message with a line", links)
	}
	testx.WaitUntil(t, waitFor, func() bool { return !dwNoDetailsItem(t, f) })
	if e, ok := dwErrorOf(t, f, older); ok {
		t.Fatalf("older note error = %+v, want none: a held edit is not an error", e)
	}
	if !slices.Contains(dfHandoverBodies(t, app, f.task.ID), "note: "+olderBody) {
		t.Fatalf("chain = %q, want the older note held", dfHandoverBodies(t, app, f.task.ID))
	}
	dwWaitDisk(t, f, older)
	stateYAML(t, app, f.task.ID, func(d map[string]any) bool {
		pending, _ := d["pending"].([]any)
		for _, p := range pending {
			if m, _ := p.(map[string]any); m["kind"] == "details.edit" && m["subject"] == older {
				return true
			}
		}
		return false
	})
	item := dfDetailsItem(t, app, f.task.ID)
	app.Contract(t, dwScenario, "ApprovalsService.List#detailsEdit", []approvals.Item{item}, dfMask)
	app.Contract(t, dwScenario, "AdeTaskService.Handovers#applied", adewire.HandoversResult{Handovers: handovers(t, app, f.task.ID)}, dfMask)

	// 3. The agent fixes links.yaml: the error clears and the link is filed as the run's.
	dfWrite(t, folder, "links.yaml", "links:\n  - {kind: doc, title: Design notes, ref: docs/design.md}\n")
	dwWaitNoError(t, f, "links.yaml")
	if a, ok := findArtifact(artifactsOf(t, app, f.task.ID), "doc", "Design notes"); !ok || a.Source != "agent" {
		t.Fatalf("artifacts = %+v, want the agent's doc link", artifactsOf(t, app, f.task.ID).Artifacts)
	}

	// 4. The run ends: the watcher stops and fileErrors empties. A later save stays a disk change.
	_ = os.WriteFile(gate, nil, 0o644)
	waitRun(t, app, f.task.ID, "one", "done")
	stateYAML(t, app, f.task.ID, func(d map[string]any) bool {
		errs, ok := d["fileErrors"].([]any)
		return ok && len(errs) == 0
	})
	dfWrite(t, folder, newer, "Saved after the run.")
	time.Sleep(3 * 300 * time.Millisecond)
	if got := dfRead(t, folder, newer); got != "Saved after the run." {
		t.Fatalf("note file = %q", got)
	}
	var late, held bool
	for _, c := range dfDisk(t, app, f.task.ID) {
		late = late || c.Path == newer && c.State == "edited" && !c.Pending
		held = held || c.Path == older && c.Pending
	}
	if !late || !held {
		t.Fatalf("disk changes after the run = %+v, want the late save and the older note waiting", dfDisk(t, app, f.task.ID))
	}
}

// The newest proof of a task is free to edit; another agent's older one waits for the user.
func TestDetailsWatchLatestProof(t *testing.T) {
	gate := dwHold(t)
	f := newBRFixture(t, []fakeagent.Action{{WaitFile: gate, Name: "done"}}, onceStepYML, false)
	app := f.app
	folder := dfFolder(t, app, f.task.ID)
	proofs := func(n int) []adewire.TaskProof {
		var got []adewire.TaskProof
		testx.WaitUntil(t, waitFor, func() bool {
			got = taskProofs(t, app, f.task.ID)
			return len(got) == n
		})
		return got
	}
	// Another agent (session-2) made the first proof.
	writeProof(t, folder, "first", "First", "diagram", "table")
	if reply := dfSyncAs(t, app, f.task.ID, "session-2", "proofs/first/proof.md"); !strings.HasPrefix(reply, "accepted 1") {
		t.Fatalf("session-2 proof reply = %q", reply)
	}
	proofs(1)
	f.startRun(t)
	waitRun(t, app, f.task.ID, "one", "running")
	writeProof(t, folder, "second", "Second", "diagram", "table")
	proofs(2)

	// The older proof is another agent's: the edit waits for the user with no error; the accepted proof stays.
	writeProof(t, folder, "first", "First", "table")
	testx.WaitUntil(t, waitFor, func() bool {
		for _, p := range taskProofs(t, app, f.task.ID) {
			if p.Slug == "first" && p.Pending {
				return true
			}
		}
		return false
	})
	if e, ok := dwErrorOf(t, f, "proofs/first/proof.md"); ok {
		t.Fatalf("older proof error = %+v, want none", e)
	}
	// The newest is the run's own: applied with no approval.
	writeProof(t, folder, "second", "Second", "table")
	testx.WaitUntil(t, waitFor, func() bool {
		for _, p := range taskProofs(t, app, f.task.ID) {
			if p.Slug == "second" && len(p.Steps) == 1 && !p.Pending {
				return true
			}
		}
		return false
	})
	if _, ok := dwErrorOf(t, f, "proofs/second/proof.md"); ok {
		t.Fatal("the newest proof reports an error")
	}
	if !dwNoDetailsItem(t, f) {
		t.Fatal("a details approval is open for proof edits, which review as revisions")
	}
	for _, p := range taskProofs(t, app, f.task.ID) {
		if p.Slug == "first" && len(p.Steps) != 2 {
			t.Fatalf("older proof = %+v, want its accepted two steps", p)
		}
	}
	_ = os.WriteFile(gate, nil, 0o644)
	waitRun(t, app, f.task.ID, "one", "done")
}

// A fileErrors entry under a path the agent then removes clears with the next pass.
func TestDetailsWatchGonePathClears(t *testing.T) {
	gate := dwHold(t)
	f := newBRFixture(t, []fakeagent.Action{{WaitFile: gate, Name: "done"}}, onceStepYML, false)
	folder := dfFolder(t, f.app, f.task.ID)
	f.startRun(t)
	waitRun(t, f.app, f.task.ID, "one", "running")

	dfWrite(t, folder, "proofs/Orders_1/proof.md", "x")
	e := dwWaitError(t, f, "proofs/Orders_1/")
	if !strings.Contains(e.msg, "rename it") {
		t.Fatalf("hint error = %q", e.msg)
	}
	if err := os.RemoveAll(filepath.Join(folder, "proofs", "Orders_1")); err != nil {
		t.Fatal(err)
	}
	dwWaitNoError(t, f, "proofs/Orders_1/")
	_ = os.WriteFile(gate, nil, 0o644)
	waitRun(t, f.app, f.task.ID, "one", "done")
}

// Nine live task watchers apply their saves: no cap on the number of watched tasks (macOS had one of 8).
func TestDetailsWatchBeyondEight(t *testing.T) {
	const tasks = 9
	gate := dwHold(t)
	hold := []fakeagent.Action{{WaitFile: gate, Name: "done"}}
	f := newBRFixture(t, hold, onceStepYML, false)
	app := f.app
	claude(app, map[string][]fakeagent.Action{"*": {{
		TaskFiles: map[string]string{"handovers/draft-one.md": "Draft v1."},
		WaitFile:  gate, Name: "done",
	}}})
	ids := []string{f.task.ID}
	for i := 1; i < tasks; i++ {
		ids = append(ids, createTask(t, app, "Fix login "+strconv.Itoa(i), "flow", f.api).ID)
	}
	for _, id := range ids {
		if _, err := app.W.AdeTask.StartRun(ctx, adewire.StartRunArgs{TaskID: id}); err != nil {
			t.Fatalf("StartRun %s: %v", id, err)
		}
	}
	for _, id := range ids {
		testx.WaitUntil(t, waitFor, func() bool {
			for _, h := range handovers(t, app, id) {
				if h.State == "draft" && h.Body == "Draft v1." {
					return true
				}
			}
			return false
		})
	}
}
