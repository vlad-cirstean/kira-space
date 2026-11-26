package adeflow_test

import (
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness/fakehost"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/githost"
	"github.com/kirathecat/kira-studio/internal/flowtest"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// P758: opening task Details never waits on the host. Artifacts answers from caches, the stored rows
// and the board snapshot, then one background job asks the host and pushes the task when the answer
// would differ. Contract ade-artifacts-open: tests/ui/ade-v2-artifacts-open.spec.ts reads these.

const (
	aoScenario = "ade-artifacts-open"
	aoBranch   = "feat/api-work"
	aoPRURL    = "https://forge.test/acme/api/-/merge_requests/7"
)

var aoMask = flowharness.Mask("id", "createdAt", "ref", "taskId", "sessionId", "foundIn")

// aoRefsTask matches the confirm job's push: refs changed, naming exactly the task.
func aoRefsTask(t *testing.T, taskID string) func(flowtest.Event) bool {
	return func(ev flowtest.Event) bool {
		var p adewire.BoardChangedEvent
		ev.Decode(t, &p)
		return p.Refs && slices.Equal(p.TaskIDs, []string{taskID})
	}
}

// aoCall runs Artifacts and fails when it does not return within 3s, below the source timeout.
func aoCall(t *testing.T, app *flowharness.App, taskID string) adewire.ArtifactsResult {
	t.Helper()
	type out struct {
		res adewire.ArtifactsResult
		err error
	}
	ch := make(chan out, 1)
	go func() {
		res, err := app.W.AdeTask.Artifacts(ctx, adewire.TaskArgs{TaskID: taskID})
		ch <- out{res, err}
	}()
	select {
	case o := <-ch:
		if o.err != nil {
			t.Fatalf("Artifacts: %v", o.err)
		}
		return o.res
	case <-time.After(3 * time.Second):
		t.Fatal("Artifacts waited on the held host")
	}
	return adewire.ArtifactsResult{}
}

func TestArtifactsOpen(t *testing.T) {
	forge := fakehost.New()
	forge.AddChangeRequest(githost.ChangeRequest{
		Number: 7, Title: "Fix login", State: "open", HeadRef: aoBranch, HeadOwner: "acme", BaseRef: "main", URL: aoPRURL,
	})
	app := flowharness.New(t, flowharness.WithGitHost(forge), flowharness.WithoutGh())
	repo, _ := cloneWithMain(app, filepath.Join(app.Work, "api"))
	rec := importRepo(t, app, repo.Dir)
	gitOut(t, repo.Dir, "remote", "set-url", "origin", "https://forge.test/acme/api.git")
	gitOut(t, repo.Dir, "branch", aoBranch)
	task := createTask(t, app, "Fix login", "", rec.ID)
	if _, err := app.W.AdeTask.AddExistingBranch(ctx, adewire.AddExistingBranchArgs{CodeRepoID: rec.ID, Name: aoBranch, TaskID: task.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.W.AdeTask.AddLink(ctx, adewire.AddLinkArgs{TaskID: task.ID, Text: "Runbook https://wiki.example/runbook"}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.W.AdeTask.AddNote(ctx, adewire.AddNoteArgs{TaskID: task.ID, Body: "See https://notes.example/design for the plan."}); err != nil {
		t.Fatal(err)
	}
	opts := []flowharness.ContractOption{aoMask, flowharness.Replace(task.ID, "T_alerts")}
	refsTask := aoRefsTask(t, task.ID)

	// 1. Cold host cache, host held: the call returns with the stored, notes and branch rows, no PR, no gap.
	app.Restart()
	release := forge.Hold()
	mark := app.Events.Mark()
	cold := aoCall(t, app, task.ID)
	if _, ok := findArtifact(cold, "link", "Runbook"); !ok {
		t.Fatalf("cold answer lacks the stored link: %+v", cold.Artifacts)
	}
	if a, ok := findArtifact(cold, "link", "notes.example/design"); !ok || a.Source != "notes" {
		t.Fatalf("cold answer lacks the notes link: %+v", cold.Artifacts)
	}
	if a, ok := findArtifact(cold, "branch", aoBranch); !ok || a.Source != "git" {
		t.Fatalf("cold answer lacks the branch: %+v", cold.Artifacts)
	}
	if _, ok := findArtifact(cold, "changeRequest", "Fix login"); ok {
		t.Fatalf("cold answer lists the PR before the host answered: %+v", cold.Artifacts)
	}
	if len(cold.Gaps) != 0 {
		t.Fatalf("cold gaps = %+v, want none", cold.Gaps)
	}
	app.Contract(t, aoScenario, "AdeTaskService.Artifacts#cold", cold, opts...)

	// 2. Released: the job finds the PR and pushes the task with refs, once.
	release()
	ev := app.Events.WaitAfter(t, mark, adewire.ChannelBoard, refsTask, waitFor)
	app.Contract(t, aoScenario, "event:kira:adetask:board#hostRefresh", ev.Data, opts...)
	warm := aoCall(t, app, task.ID)
	if a, ok := findArtifact(warm, "changeRequest", "Fix login"); !ok || a.Status != "open" || a.URL != aoPRURL || a.Source != "host" {
		t.Fatalf("warm answer lacks the PR: %+v", warm.Artifacts)
	}
	app.Contract(t, aoScenario, "AdeTaskService.Artifacts#warm", warm, opts...)

	// 3. Nothing changed: a further call pushes nothing for the task.
	time.Sleep(300 * time.Millisecond) // the push of step 2 and the first confirm settle
	mark = app.Events.Mark()
	again := aoCall(t, app, task.ID)
	if len(again.Artifacts) != len(warm.Artifacts) {
		t.Fatalf("second answer = %+v, want %+v", again.Artifacts, warm.Artifacts)
	}
	time.Sleep(10 * 30 * time.Millisecond) // well past the push window
	for _, ev := range app.Events.Since(mark, adewire.ChannelBoard) {
		if refsTask(ev) {
			t.Fatalf("an unchanged answer pushed %s", ev.Data)
		}
	}

	// 4. The host signed out: the first answer shows no gap, then a scoped push, then the gap.
	forge.SetOffline(true)
	app.Restart()
	mark = app.Events.Mark()
	first := aoCall(t, app, task.ID)
	if len(first.Gaps) != 0 {
		t.Fatalf("first answer gaps = %+v, want none: the host kind is not known yet", first.Gaps)
	}
	app.Events.WaitAfter(t, mark, adewire.ChannelBoard, refsTask, waitFor)
	testx.WaitUntil(t, waitFor, func() bool {
		gaps := aoCall(t, app, task.ID).Gaps
		return len(gaps) == 1 && gaps[0].Source == "host" && gaps[0].Repo == "api"
	})

	// 5. A call that arrives while the task's confirm job waits on the host is not dropped: the job
	// confirms once more against the latest share served, and the task still gets its push.
	forge.SetOffline(false)
	app.Restart()
	release = forge.Hold()
	mark = app.Events.Mark()
	aoCall(t, app, task.ID)
	aoCall(t, app, task.ID)
	release()
	app.Events.WaitAfter(t, mark, adewire.ChannelBoard, refsTask, waitFor)
	testx.WaitUntil(t, waitFor, func() bool {
		_, ok := findArtifact(aoCall(t, app, task.ID), "changeRequest", "Fix login")
		return ok
	})
}
