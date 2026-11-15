package adeflow_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/agentmcp"
	"github.com/kirathecat/kira-studio/internal/approvals"
)

// P643 task files. Contract ade-details-files: tests/ui/ade-v2-details-files.spec.ts reads these and
// runs the same scenario through the UI.

const dfScenario = "ade-details-files"

var dfMask = flowharness.Mask("id", "taskId", "createdAt", "updatedAt", "sessionId", "expiresAt", "linkedAt", "addedAt")

func dfFolder(t *testing.T, app *flowharness.App, taskID string) string {
	t.Helper()
	res, err := app.W.AdeTask.DetailsFolder(ctx, adewire.DiskChangesArgs{TaskID: taskID})
	if err != nil {
		t.Fatal(err)
	}
	return res.Path
}

func dfRead(t *testing.T, folder, rel string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(folder, rel))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func dfWrite(t *testing.T, folder, rel, body string) {
	t.Helper()
	path := filepath.Join(folder, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func dfNotes(t *testing.T, folder string) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(folder, "notes", "*.md"))
	if err != nil {
		t.Fatal(err)
	}
	for i := range files {
		files[i], _ = filepath.Rel(folder, files[i])
	}
	return files
}

func dfDetailsItem(t *testing.T, app *flowharness.App, taskID string) approvals.Item {
	t.Helper()
	all, err := app.W.ApprovalsSvc.List()
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range all {
		if it.ID == approvals.KindDetailsEdit+":"+taskID {
			return it
		}
	}
	t.Fatalf("no details.edit approval for task %s in %+v", taskID, all)
	return approvals.Item{}
}

func dfAnswer(t *testing.T, app *flowharness.App, taskID string, approve bool, approved ...string) {
	t.Helper()
	id := approvals.KindDetailsEdit + ":" + taskID
	args := approvals.AnswerArgs{IDs: []string{id}, Approve: approve}
	if approve {
		raw, err := json.Marshal(map[string]any{"approved": approved})
		if err != nil {
			t.Fatal(err)
		}
		args.Options = map[string]json.RawMessage{id: raw}
	}
	res, err := app.W.ApprovalsSvc.Answer(args)
	if err != nil || res[0].Error != "" {
		t.Fatalf("answer = %+v, %v", res, err)
	}
}

// dfNote is the body of the chain's note entry.
func dfNote(t *testing.T, app *flowharness.App, taskID string) string {
	t.Helper()
	for _, h := range handovers(t, app, taskID) {
		if h.State == "note" {
			return h.Body
		}
	}
	t.Fatal("no note in the chain")
	return ""
}

func dfHandoverBodies(t *testing.T, app *flowharness.App, taskID string) []string {
	t.Helper()
	var out []string
	for _, h := range handovers(t, app, taskID) {
		out = append(out, h.State+": "+h.Body)
	}
	return out
}

func dfSubmit(t *testing.T, app *flowharness.App, taskID string, args agentmcp.SubmitArgs) string {
	t.Helper()
	reply, err := app.W.AdeBoard.Submit(ctx, agentmcp.SpaceCall{TaskID: taskID, SessionID: "session-1"}, args)
	if err != nil {
		t.Fatal(err)
	}
	return reply
}

func dfDisk(t *testing.T, app *flowharness.App, taskID string) []adewire.DiskChange {
	t.Helper()
	res, err := app.W.AdeTask.DetailsDiskChanges(ctx, adewire.DiskChangesArgs{TaskID: taskID})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestDetailsFiles(t *testing.T) {
	app := flowharness.New(t)
	repo, _ := cloneWithMain(app, filepath.Join(app.Work, "api"))
	rec := importRepo(t, app, repo.Dir)
	saveWorkflow(t, app, "flow", flowYAML("flow", "  - id: work\n    name: Work\n    kind: manual\n    session: true\n"))
	task := createTask(t, app, "Login form", "flow", rec.ID)
	folder := dfFolder(t, app, task.ID)

	// The folder buttons hand the task folder to the file manager.
	if err := app.W.AdeTask.OpenDetailsFolder(ctx, adewire.DiskChangesArgs{TaskID: task.ID}); err != nil {
		t.Fatal(err)
	}
	if err := app.W.AdeTask.RevealDetailsFolder(ctx, adewire.DiskChangesArgs{TaskID: task.ID}); err != nil {
		t.Fatal(err)
	}
	if o, r := app.FileOpener.OpenedFiles(), app.FileOpener.RevealedFiles(); len(o) != 1 || o[0] != folder || len(r) != 1 || r[0] != folder {
		t.Fatalf("opened %v, revealed %v, want the task folder %s", o, r, folder)
	}

	// 1. A UI note is a file of the task folder.
	const first = "Check the error states."
	addNote := adewire.AddNoteArgs{TaskID: task.ID, Body: first}
	app.Contract(t, dfScenario, "args:AdeTaskService.AddNote", addNote, dfMask)
	note, err := app.W.AdeTask.AddNote(ctx, addNote)
	if err != nil {
		t.Fatal(err)
	}
	app.Contract(t, dfScenario, "AdeTaskService.AddNote", note, dfMask)
	files := dfNotes(t, folder)
	if len(files) != 1 || dfRead(t, folder, files[0]) != first {
		t.Fatalf("note files = %v, want one holding %q", files, first)
	}
	noteRel := files[0]
	if !strings.HasPrefix(dfRead(t, folder, "README.md"), "<!--") && !strings.Contains(dfRead(t, folder, "README.md"), "Login form") {
		t.Fatalf("README = %q, want the task title", dfRead(t, folder, "README.md"))
	}
	if d := dfDisk(t, app, task.ID); len(d) != 0 {
		t.Fatalf("disk changes after the app's own write = %+v, want none", d)
	}

	// 2. An agent adds a link by editing links.yaml: accepted at once, no approval.
	dfWrite(t, folder, "links.yaml", "links:\n  - {kind: doc, title: Design notes, ref: docs/design.md}\n")
	reply := dfSubmit(t, app, task.ID, agentmcp.SubmitArgs{})
	if !strings.HasPrefix(reply, "accepted 1, pending user 0, invalid 0\n") {
		t.Fatalf("link submit reply = %q", reply)
	}
	arts := artifactsOf(t, app, task.ID)
	if a, ok := findArtifact(arts, "doc", "Design notes"); !ok || a.Source != "agent" || a.Ref != "docs/design.md" {
		t.Fatalf("artifacts = %+v, want the agent's doc", arts.Artifacts)
	}
	app.Contract(t, dfScenario, "AdeTaskService.Artifacts#link", arts, dfMask)

	// 3. The agent writes the stage handover and says it is done: a draft row, the move prompt.
	dfWrite(t, folder, "handovers/draft-work.md", "Form wired; validation missing.")
	reply = dfSubmit(t, app, task.ID, agentmcp.SubmitArgs{})
	if !strings.HasPrefix(reply, "accepted 1, pending user 0, invalid 0") {
		t.Fatalf("draft submit reply = %q", reply)
	}
	if note, err := app.W.AdeBoard.FinishSession(ctx, agentmcp.SpaceCall{TaskID: task.ID, SessionID: "session-1"}); err != nil || !strings.Contains(note, "The user was asked to move the task on") {
		t.Fatalf("session step.finish = %q, %v", note, err)
	}
	if got := dfHandoverBodies(t, app, task.ID); len(got) != 2 || got[0] != "draft: Form wired; validation missing." {
		t.Fatalf("chain = %q, want the draft, then the note", got)
	}
	if d := taskOf(t, board(t, app), task.ID).HandoverDrafts; len(d) != 1 || !d[0].Finished || d[0].Source != "agent" {
		t.Fatalf("drafts = %+v, want one finished agent draft", d)
	}
	app.Contract(t, dfScenario, "AdeTaskService.Handovers#draft", adewire.HandoversResult{Handovers: handovers(t, app, task.ID)}, dfMask)

	// 4. Editing the user's note needs a reason; with one it waits for the user.
	const changed = "Check the error states and the empty form."
	dfWrite(t, folder, noteRel, changed)
	reply = dfSubmit(t, app, task.ID, agentmcp.SubmitArgs{Paths: []string{noteRel}})
	if !strings.HasPrefix(reply, "accepted 0, pending user 0, invalid 1\n") || !strings.Contains(reply, "reason") {
		t.Fatalf("note submit without reason = %q", reply)
	}
	reply = dfSubmit(t, app, task.ID, agentmcp.SubmitArgs{Paths: []string{noteRel}, Reason: "the empty form case is missing"})
	if !strings.HasPrefix(reply, "accepted 0, pending user 1, invalid 0\n") {
		t.Fatalf("note submit with reason = %q", reply)
	}
	item := dfDetailsItem(t, app, task.ID)
	app.Contract(t, dfScenario, "ApprovalsService.List#detailsEdit", []approvals.Item{item}, dfMask)
	if got := dfNote(t, app, task.ID); got != first {
		t.Fatalf("note before the answer = %q, want the held text", got)
	}
	app.Contract(t, dfScenario, "args:ApprovalsService.Answer#approve", map[string]any{"approved": []string{noteRel}})
	dfAnswer(t, app, task.ID, true, noteRel)
	if got := dfNote(t, app, task.ID); got != changed {
		t.Fatalf("note after approve = %q, want the agent's text", got)
	}
	if got := dfRead(t, folder, noteRel); got != changed {
		t.Fatalf("note file after approve = %q", got)
	}

	// 5. A denied edit restores the file.
	dfWrite(t, folder, noteRel, "Drop the second check.")
	dfSubmit(t, app, task.ID, agentmcp.SubmitArgs{Paths: []string{noteRel}, Reason: "the second check is a duplicate"})
	dfAnswer(t, app, task.ID, false)
	if got := dfRead(t, folder, noteRel); got != changed {
		t.Fatalf("note file after deny = %q, want it restored to %q", got, changed)
	}
	if got := dfNote(t, app, task.ID); got != changed {
		t.Fatalf("note after deny = %q", got)
	}

	// 6. A hand edit outside any submit shows as a disk change; Accept keeps it, Discard restores the held copy.
	dfWrite(t, folder, noteRel, "Edited by hand.")
	changes := dfDisk(t, app, task.ID)
	if len(changes) != 1 || changes[0].Path != noteRel || changes[0].State != "edited" || changes[0].Pending || changes[0].Diff == "" {
		t.Fatalf("disk changes = %+v, want the hand-edited note", changes)
	}
	app.Contract(t, dfScenario, "AdeTaskService.DetailsDiskChanges", changes, dfMask)
	accept := adewire.DiskResolveArgs{TaskID: task.ID, Paths: []string{noteRel}}
	app.Contract(t, dfScenario, "args:AdeTaskService.DetailsAcceptDisk", accept, dfMask)
	if err := app.W.AdeTask.DetailsAcceptDisk(ctx, accept); err != nil {
		t.Fatal(err)
	}
	if got := dfNote(t, app, task.ID); got != "Edited by hand." {
		t.Fatalf("note after accept = %q", got)
	}
	if d := dfDisk(t, app, task.ID); len(d) != 0 {
		t.Fatalf("disk changes after accept = %+v", d)
	}
	dfWrite(t, folder, noteRel, "Typo.")
	discard := adewire.DiskResolveArgs{TaskID: task.ID, Paths: []string{}}
	app.Contract(t, dfScenario, "args:AdeTaskService.DetailsDiscardDisk", discard, dfMask)
	if err := app.W.AdeTask.DetailsDiscardDisk(ctx, discard); err != nil {
		t.Fatal(err)
	}
	if got := dfRead(t, folder, noteRel); got != "Edited by hand." {
		t.Fatalf("note file after discard = %q, want it restored", got)
	}
	app.Contract(t, dfScenario, "AdeTaskService.Handovers#final", adewire.HandoversResult{Handovers: handovers(t, app, task.ID)}, dfMask)
}
