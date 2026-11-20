package adeflow_test

import (
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/internal/agentmcp"
	"github.com/kirathecat/kira-studio/internal/approvals"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// Task file watcher: while an agent run is live, Kira applies its saved free edits, writes the errors
// into state.yaml fileErrors and asks no approval for the latest note. An ask-first save (an older
// note) is deferred untouched: it may be the user's own edit, so it stays a disk change with no error. Contract ade-details-watch:
// tests/ui/ade-v2-details-watch.spec.ts reads these and runs the same scenario through the UI.

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

// dwNotes returns the rels of the older and the newer note, told apart by their text.
func dwNotes(t *testing.T, folder, olderBody string) (older, newer string) {
	t.Helper()
	for _, rel := range dfNotes(t, folder) {
		if dfRead(t, folder, rel) == olderBody {
			older = rel
		} else {
			newer = rel
		}
	}
	if older == "" || newer == "" {
		t.Fatalf("notes = %v, want an older and a newer one", dfNotes(t, folder))
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
	const olderBody = "Check the error states."
	for _, body := range []string{olderBody, "Newer note."} {
		if _, err := app.W.AdeTask.AddNote(ctx, adewire.AddNoteArgs{TaskID: f.task.ID, Body: body}); err != nil {
			t.Fatal(err)
		}
	}
	older, newer := dwNotes(t, folder, olderBody)

	// 1. The run saves four files with no call: the draft, the newest note, an invalid links.yaml and
	// an older note.
	claude(app, map[string][]fakeagent.Action{"*": {{
		TaskFiles: map[string]string{
			"handovers/draft-one.md": "Draft v1.",
			newer:                    "Newer note, edited.",
			"links.yaml":             "links:\n  - {kind: doc\n",
			older:                    "Older note, edited.",
		},
		WaitFile: gate, Name: "done",
	}}})
	f.startRun(t)

	// The draft and the newest note apply at once, with no approval and no disk change.
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
	// note is deferred: no error, no row change, and the banner shows it, since the save may be the user's.
	links := dwWaitError(t, f, "links.yaml")
	if links.line == 0 || links.msg == "" {
		t.Fatalf("links.yaml error = %+v, want a message with a line", links)
	}
	dwWaitDisk(t, f, older)
	if e, ok := dwErrorOf(t, f, older); ok {
		t.Fatalf("older note error = %+v, want none: a deferred save is not filed against the agent", e)
	}
	if got := dfNote(t, app, f.task.ID); got == "Older note, edited." {
		t.Fatalf("older note applied without a reason: %q", got)
	}
	if !dwNoDetailsItem(t, f) {
		t.Fatal("an approval is open for edits that needed none")
	}
	app.Contract(t, dwScenario, "AdeTaskService.Handovers#applied", adewire.HandoversResult{Handovers: handovers(t, app, f.task.ID)}, dfMask)

	// 3. The agent fixes links.yaml: the error clears and the link is filed as the run's.
	dfWrite(t, folder, "links.yaml", "links:\n  - {kind: doc, title: Design notes, ref: docs/design.md}\n")
	dwWaitNoError(t, f, "links.yaml")
	if a, ok := findArtifact(artifactsOf(t, app, f.task.ID), "doc", "Design notes"); !ok || a.Source != "agent" {
		t.Fatalf("artifacts = %+v, want the agent's doc link", artifactsOf(t, app, f.task.ID).Artifacts)
	}

	// 4. submit sends the older note to the user with its reason; the error clears.
	run := waitRun(t, app, f.task.ID, "one", "running")
	call := agentmcp.SpaceCall{TaskID: f.task.ID, RunID: run.ID}
	reply, err := app.W.AdeBoard.Submit(ctx, call, agentmcp.SubmitArgs{Paths: []string{older}, Reason: "typo fixed"})
	if err != nil || !strings.HasPrefix(reply, "accepted 0, pending user 1, invalid 0") {
		t.Fatalf("submit = %q, %v", reply, err)
	}
	item := dfDetailsItem(t, app, f.task.ID)
	app.Contract(t, dwScenario, "ApprovalsService.List#detailsEdit", []approvals.Item{item}, dfMask)

	// 5. submit with no paths or no reason is refused.
	for name, args := range map[string]agentmcp.SubmitArgs{
		"no paths":  {Reason: "typo fixed"},
		"no reason": {Paths: []string{older}, Reason: "  "},
	} {
		if _, err := app.W.AdeBoard.Submit(ctx, call, args); err == nil || !strings.Contains(err.Error(), "pass paths and reason") {
			t.Fatalf("submit with %s: err = %v, want the paths and reason refusal", name, err)
		}
	}

	// 6. The run ends: the watcher stops and fileErrors empties. A later save stays a disk change.
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

// The newest proof of a task is free to edit; an older one needs a reason.
func TestDetailsWatchLatestProof(t *testing.T) {
	gate := dwHold(t)
	f := newBRFixture(t, []fakeagent.Action{{WaitFile: gate, Name: "done"}}, onceStepYML, false)
	app := f.app
	folder := dfFolder(t, app, f.task.ID)
	f.startRun(t)
	waitRun(t, app, f.task.ID, "one", "running")

	proofs := func(n int) []adewire.TaskProof {
		var got []adewire.TaskProof
		testx.WaitUntil(t, waitFor, func() bool {
			got = taskProofs(t, app, f.task.ID)
			return len(got) == n
		})
		return got
	}
	writeProof(t, folder, "first", "First", "diagram", "table")
	proofs(1)
	writeProof(t, folder, "second", "Second", "diagram", "table")
	proofs(2)

	// The older proof: the edit is deferred, a disk change with no error; the proof stays.
	writeProof(t, folder, "first", "First", "table")
	dwWaitDisk(t, f, "proofs/first/proof.md")
	if e, ok := dwErrorOf(t, f, "proofs/first/proof.md"); ok {
		t.Fatalf("older proof error = %+v, want none", e)
	}
	// The newest: applied with no approval.
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
		t.Fatal("an approval is open for the newest proof's edit")
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
