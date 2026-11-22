package adeflow_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness/fakehost"
)

// P731 links found in notes and handovers. Contract ade-found-links: tests/ui/ade-v2-found-links.spec.ts
// reads these.

const flScenario = "ade-found-links"

var flMask = flowharness.Mask("id", "createdAt", "updatedAt", "taskId", "sessionId", "handoverId")

const (
	flD0     = "https://docs.google.com/document/d/d0/edit"
	flDesign = "https://docs.google.com/document/d/f1/edit"
	flPR     = "https://forge.test/acme/api/-/merge_requests/21"
	flA      = "https://ex.test/a"
	flB      = "https://ex.test/b"
)

func found(res adewire.ArtifactsResult) map[string]adewire.Artifact {
	out := map[string]adewire.Artifact{}
	for _, a := range res.Artifacts {
		if a.Source == "notes" {
			out[a.URL] = a
		}
	}
	return out
}

func foundURLs(res adewire.ArtifactsResult) []string {
	var urls []string
	for url := range found(res) {
		urls = append(urls, url)
	}
	slices.Sort(urls)
	return urls
}

func countURL(res adewire.ArtifactsResult, url string) int {
	n := 0
	for _, a := range res.Artifacts {
		if a.URL == url {
			n++
		}
	}
	return n
}

// mainOf returns the non-found artifact of url, if any.
func mainOf(res adewire.ArtifactsResult, url string) (adewire.Artifact, bool) {
	for _, a := range res.Artifacts {
		if a.URL == url && a.Source != "notes" {
			return a, true
		}
	}
	return adewire.Artifact{}, false
}

func TestFoundLinks(t *testing.T) {
	app := flowharness.New(t, flowharness.WithGitHost(fakehost.New()), flowharness.WithoutGh())
	saveWorkflow(t, app, "flow", flowYAML("flow", "  - id: work\n    name: Work\n    kind: manual\n    session: true\n  - id: ship\n    name: Ship\n    kind: manual\n"))
	task := createTask(t, app, "Found links", "flow")

	// 1. D0 is a main link the user filed.
	if res, err := app.W.AdeTask.AddLink(ctx, adewire.AddLinkArgs{TaskID: task.ID, Text: "Spec " + flD0}); err != nil || res.Filed != "stored" {
		t.Fatalf("AddLink D0 = %+v, %v", res, err)
	}

	// 2. A note and a stage-move handover hold links.
	noteBody := "See [Design](" + flDesign + "), the PR " + flPR + ". Also javascript:alert(1), <" + flA + ">,\n" +
		"`https://ex.test/code` and " + flD0 + " again."
	note, err := app.W.AdeTask.AddNote(ctx, adewire.AddNoteArgs{TaskID: task.ID, Body: noteBody})
	if err != nil {
		t.Fatal(err)
	}
	moveNotes := "Check " + flB + " and " + flA + "."
	if _, err := app.W.AdeTask.SetTaskStep(ctx, adewire.SetTaskStepArgs{TaskID: task.ID, StepID: "ship", Notes: &moveNotes}); err != nil {
		t.Fatal(err)
	}

	res := artifactsOf(t, app, task.ID)
	app.Contract(t, flScenario, "AdeTaskService.Artifacts#initial", res, flMask)
	if got, want := foundURLs(res), []string{flDesign, flA, flB, flPR}; !slices.Equal(got, want) {
		t.Fatalf("found = %v, want %v", got, want)
	}
	f := found(res)
	if d := f[flDesign]; d.Kind != "doc" || d.Title != "Design" || d.ID != "" || len(d.FoundIn) != 1 || d.FoundIn[0].State != "note" {
		t.Fatalf("Design = %+v", d)
	}
	if p := f[flPR]; p.Kind != "changeRequest" || p.Ref == "" || p.ID != "" {
		t.Fatalf("PR = %+v, want a change request", p)
	}
	if a := f[flA]; len(a.FoundIn) != 2 || a.FoundIn[0].State != "handed" || a.FoundIn[0].ToStep != "Ship" || a.FoundIn[1].State != "note" {
		t.Fatalf("a = %+v, want the handover then the note", a)
	}
	if d0, ok := mainOf(res, flD0); !ok || d0.Source != "user" {
		t.Fatalf("D0 = %+v, want one main user row", d0)
	}
	if n := countURL(res, flD0); n != 1 {
		t.Fatalf("D0 appears %d times, want 1", n)
	}

	// 3. The user promotes Design: a stored user row, no found copy left.
	promote := adewire.AddLinkArgs{TaskID: task.ID, Text: "Design " + flDesign}
	app.Contract(t, flScenario, "args:AdeTaskService.AddLink", promote, flMask)
	if r, err := app.W.AdeTask.AddLink(ctx, promote); err != nil || r.Filed != "stored" || r.Artifact == nil || r.Artifact.Source != "user" {
		t.Fatalf("promote = %+v, %v", r, err)
	}

	// 4. The agent promotes b by listing it in links.yaml: applied at once, no approval.
	folder := dfFolder(t, app, task.ID)
	yaml := dfRead(t, folder, "links.yaml")
	dfWrite(t, folder, "links.yaml", strings.TrimRight(yaml, "\n")+"\n  - {kind: link, title: \"B\", url: \""+flB+"\"}\n")
	if reply := dfSync(t, app, task.ID, "links.yaml"); !strings.HasPrefix(reply, "accepted 1, pending user 0, invalid 0") {
		t.Fatalf("links.yaml sync = %q", reply)
	}
	res = artifactsOf(t, app, task.ID)
	app.Contract(t, flScenario, "AdeTaskService.Artifacts#promoted", res, flMask)
	if got, want := foundURLs(res), []string{flA, flPR}; !slices.Equal(got, want) {
		t.Fatalf("found after promotions = %v, want %v", got, want)
	}
	if a, ok := mainOf(res, flDesign); !ok || a.Source != "user" {
		t.Fatalf("Design = %+v, want main user", a)
	}
	b, ok := mainOf(res, flB)
	if !ok || b.Source != "agent" {
		t.Fatalf("b = %+v, want main agent", b)
	}

	// 5. Dismiss the PR: gone now and on every later read.
	dismiss := adewire.DismissFoundLinkArgs{TaskID: task.ID, URL: flPR}
	app.Contract(t, flScenario, "args:AdeTaskService.DismissFoundLink", dismiss, flMask)
	if err := app.W.AdeTask.DismissFoundLink(ctx, dismiss); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		res = artifactsOf(t, app, task.ID)
		if got, want := foundURLs(res), []string{flA}; !slices.Equal(got, want) {
			t.Fatalf("found after dismiss = %v, want %v", got, want)
		}
	}
	app.Contract(t, flScenario, "AdeTaskService.Artifacts#dismissed", res, flMask)
	other := createTask(t, app, "Archived", "flow")
	if err := app.W.AdeTask.ArchiveTask(ctx, adewire.ArchiveTaskArgs{TaskID: other.ID}); err != nil {
		t.Fatal(err)
	}
	if err := app.W.AdeTask.DismissFoundLink(ctx, adewire.DismissFoundLinkArgs{TaskID: other.ID, URL: flPR}); err == nil {
		t.Fatal("DismissFoundLink on an archived task succeeded")
	}
	if err := app.W.AdeTask.DismissFoundLink(ctx, adewire.DismissFoundLinkArgs{TaskID: task.ID, URL: "ftp://ex.test/x"}); err == nil {
		t.Fatal("DismissFoundLink accepted a non-http url")
	}

	// 6. Edit the note to drop a: a keeps the handover only. Delete the note: nothing of it is left.
	var noteID string
	for _, h := range handovers(t, app, task.ID) {
		if h.State == "note" {
			noteID = h.ID
		}
	}
	if noteID == "" || noteID != note.ID {
		t.Fatalf("note id = %q, want %q", noteID, note.ID)
	}
	if err := app.W.AdeTask.UpdateHandover(ctx, adewire.UpdateHandoverArgs{ID: noteID, Body: "See [Design](" + flDesign + ")."}); err != nil {
		t.Fatal(err)
	}
	if a := found(artifactsOf(t, app, task.ID))[flA]; len(a.FoundIn) != 1 || a.FoundIn[0].State != "handed" {
		t.Fatalf("a after the note edit = %+v, want the handover only", a)
	}
	if err := app.W.AdeTask.UpdateHandover(ctx, adewire.UpdateHandoverArgs{ID: noteID, Body: ""}); err != nil {
		t.Fatal(err)
	}
	res = artifactsOf(t, app, task.ID)
	if got, want := foundURLs(res), []string{flA}; !slices.Equal(got, want) {
		t.Fatalf("found after the note delete = %v, want %v", got, want)
	}
	if _, ok := mainOf(res, flDesign); !ok {
		t.Fatal("Design left the main links with the note")
	}

	// 7. Removing the agent's b row brings it back as a found link: nothing promotes by itself.
	if err := app.W.AdeTask.RemoveArtifact(ctx, adewire.RemoveArtifactArgs{TaskID: task.ID, ID: b.ID}); err != nil {
		t.Fatal(err)
	}
	res = artifactsOf(t, app, task.ID)
	if got, want := foundURLs(res), []string{flA, flB}; !slices.Equal(got, want) {
		t.Fatalf("found after removing b = %v, want %v", got, want)
	}
	if _, ok := mainOf(res, flB); ok {
		t.Fatal("b stayed main after its row was removed")
	}
}
