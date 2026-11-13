package adeflow_test

import (
	"slices"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness/fakeacli"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness/fakehost"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/githost"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/issuetracker/jira"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
)

// P512 tracked-item updates. Contracts ade-tracked-updates and settings-tracking:
// tests/ui/ade-tracked-updates.spec.ts and tests/ui/settings-updates.spec.ts read these.

const tuRepoKey = "forge|forge.test|acme/api"

var tuMask = flowharness.Mask("taskId", "itemKey", "branchId", "changedAt")

func trackedUpdates(t *testing.T, app *flowharness.App) []adewire.TrackedUpdate {
	t.Helper()
	res, err := app.W.AdeTask.TrackedUpdates(ctx)
	if err != nil {
		t.Fatalf("TrackedUpdates: %v", err)
	}
	return res.Updates
}

func updateOf(t *testing.T, ups []adewire.TrackedUpdate, kind string) adewire.TrackedUpdate {
	t.Helper()
	for _, u := range ups {
		if u.Kind == kind {
			return u
		}
	}
	t.Fatalf("no %s update in %+v", kind, ups)
	return adewire.TrackedUpdate{}
}

func refresh(t *testing.T, app *flowharness.App) {
	t.Helper()
	if _, err := app.W.AdeTask.Refresh(ctx, adewire.RefreshArgs{}); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	app.W.AdeBoard.WaitRefreshTails() // the tracked-updates pass runs after Refresh returns
}

func TestTrackedUpdates(t *testing.T) {
	forge := fakehost.New()
	acli := fakeacli.New(itSite)
	acli.SetIssue("KIRA-12", fakeacli.IssueJSON("KIRA-12", "Add the thing", "In Review", "indeterminate", 1))
	f := newRunFixtureWith(t, []fakeagent.Action{{Name: "done"}}, agentStep("one", "")+manualStep,
		flowharness.WithGitHost(forge), flowharness.WithoutGh(), flowharness.WithTrackerGrace(time.Second),
		flowharness.WithIssueTracker(jira.Options{Locator: acli, Runner: acli}))
	app := f.app
	gitOut(t, f.repo.Dir, "remote", "set-url", "origin", "https://forge.test/acme/api.git")
	forge.AddChangeRequest(githost.ChangeRequest{Number: 7, Title: "Fix login", State: "open", HeadRef: itBranch, HeadOwner: "acme", BaseRef: "main", URL: itPR7})

	// The persisted index answers the branch's change request; each step rewrites the row as a sync would.
	store := app.W.Repos.GitHostChangeRequests
	if err := store.PutSyncState(tuRepoKey, model.GitHostSyncState{BackfillComplete: true, SchemaVersion: model.GitHostSchemaVersion}); err != nil {
		t.Fatal(err)
	}
	row := model.GitHostChangeRequest{
		Number: 7, HeadRef: itBranch, HeadOwner: "acme", BaseRef: "main", State: "open", Title: "Fix login",
		URL: itPR7, CommentCount: ptr(0), UpdatedAt: 1,
	}
	put := func(edit func(*model.GitHostChangeRequest)) {
		t.Helper()
		edit(&row)
		row.UpdatedAt++
		if _, err := store.UpsertChangeRequests(tuRepoKey, []model.GitHostChangeRequest{row}); err != nil {
			t.Fatal(err)
		}
	}
	put(func(*model.GitHostChangeRequest) {})
	setLink(t, app, f.taskID, "KIRA-12", "https://"+itSite+"/browse/KIRA-12")
	f.start(t, itBranch)
	waitRun(t, app, f.taskID, "one", "done")

	// First observation sets seen = current: nothing is an update yet.
	refresh(t, app)
	if ups := trackedUpdates(t, app); len(ups) != 0 {
		t.Fatalf("updates after the first observation = %+v, want none", ups)
	}

	// Comments, failing checks, an approval and an issue status move: one update per item.
	mark := app.Events.Mark()
	put(func(r *model.GitHostChangeRequest) {
		r.CommentCount, r.ChecksState, r.ReviewDecision = ptr(3), "FAILURE", "APPROVED"
	})
	acli.SetIssue("KIRA-12", fakeacli.IssueJSON("KIRA-12", "Add the thing", "Done", "done", 1))
	refresh(t, app)
	app.Events.WaitAfter(t, mark, adewire.ChannelUpdates, nil, waitFor)
	ups := trackedUpdates(t, app)
	if len(ups) != 2 {
		t.Fatalf("updates = %+v, want the change request and the issue", ups)
	}
	crUp, issueUp := updateOf(t, ups, "changeRequest"), updateOf(t, ups, "issue")
	for _, c := range []string{"commentAdded", "checksFailed", "approved"} {
		if !slices.Contains(crUp.Concepts, c) {
			t.Fatalf("change request concepts = %v, want %s", crUp.Concepts, c)
		}
	}
	if crUp.Counts["commentAdded"] != 3 || crUp.Label != "#7" || crUp.BranchID == "" {
		t.Fatalf("change request update = %+v, want 3 new comments on #7", crUp)
	}
	if !slices.Contains(issueUp.Concepts, "statusChanged") || issueUp.Label != "KIRA-12" || issueUp.BranchID != "" {
		t.Fatalf("issue update = %+v, want statusChanged on KIRA-12", issueUp)
	}
	app.Contract(t, "ade-tracked-updates", "AdeTaskService.TrackedUpdates#changed", adewire.TrackedUpdatesResult{Updates: ups}, tuMask)

	// Marking one item seen clears only it, and a further refresh without a change keeps it clear.
	mark = app.Events.Mark()
	if err := app.W.AdeTask.MarkTrackedSeen(ctx, adewire.MarkTrackedSeenArgs{TaskID: f.taskID, ItemKeys: []string{"issue"}}); err != nil {
		t.Fatal(err)
	}
	app.Events.WaitAfter(t, mark, adewire.ChannelUpdates, nil, waitFor)
	refresh(t, app)
	if ups := trackedUpdates(t, app); len(ups) != 1 || ups[0].Kind != "changeRequest" {
		t.Fatalf("updates after marking the issue seen = %+v, want the change request only", ups)
	}

	// A change after a mark counts against the new seen value: merged shows, the old comments do not.
	put(func(r *model.GitHostChangeRequest) { r.State = "merged" })
	refresh(t, app)
	crUp = updateOf(t, trackedUpdates(t, app), "changeRequest")
	if !slices.Contains(crUp.Concepts, "merged") {
		t.Fatalf("change request concepts = %v, want merged", crUp.Concepts)
	}

	// An empty key list marks every item of the task.
	if err := app.W.AdeTask.MarkTrackedSeen(ctx, adewire.MarkTrackedSeenArgs{TaskID: f.taskID}); err != nil {
		t.Fatal(err)
	}
	if ups := trackedUpdates(t, app); len(ups) != 0 {
		t.Fatalf("updates after marking all seen = %+v, want none", ups)
	}
	for _, keys := range [][]string{{"bad key"}, {"cr:"}, {"issue/../x"}} {
		if err := app.W.AdeTask.MarkTrackedSeen(ctx, adewire.MarkTrackedSeenArgs{TaskID: f.taskID, ItemKeys: keys}); err == nil {
			t.Fatalf("MarkTrackedSeen accepted keys %v", keys)
		}
	}
}

func TestTrackingSettings(t *testing.T) {
	app := flowharness.New(t)
	got, err := app.W.Settings.GetAll()
	if err != nil {
		t.Fatal(err)
	}
	app.Contract(t, "settings-tracking", "SettingsService.GetAll#tracking", got.Tracking)
	if !got.Tracking.ChangeRequest["merged"] || got.Tracking.ChangeRequest["checksPassed"] {
		t.Fatalf("tracking defaults = %+v, want merged on and checksPassed off", got.Tracking.ChangeRequest)
	}

	// A leaf replaces whole; a concept it omits falls back to its default on read.
	on := map[string]bool{"checksPassed": true, "merged": false}
	if _, err := app.W.Settings.Set(bridge.SettingsSetArgs{Patch: model.SettingsPatch{Tracking: &model.TrackingPatch{ChangeRequest: &on}}}); err != nil {
		t.Fatal(err)
	}
	got, err = app.W.Settings.GetAll()
	if err != nil {
		t.Fatal(err)
	}
	if cr := got.Tracking.ChangeRequest; !cr["checksPassed"] || cr["merged"] || !cr["approved"] {
		t.Fatalf("tracking after Set = %+v, want checksPassed on, merged off, approved default on", cr)
	}

	bogus := map[string]bool{"bogus": true}
	if _, err := app.W.Settings.Set(bridge.SettingsSetArgs{Patch: model.SettingsPatch{Tracking: &model.TrackingPatch{Issue: &bogus}}}); err == nil {
		t.Fatal("Set accepted an unknown concept")
	}
}

// biDetail is the detail view JSON of an issue with the fields the watcher's change check reads.
func biDetail(key, title, status, updated string) string {
	return `{"key":"` + key + `","fields":{"summary":"` + title + `","status":{"name":"` + status + `","statusCategory":{"key":"indeterminate"}},` +
		`"priority":{"name":"High"},"updated":"` + updated + `","comment":{"total":0,"comments":[]}}}`
}

// P592 batch issue reads: one workitem search per pass, a detail read only where updated moved.
// Backend half only: no UI behavior changes.
func TestTrackedIssueBatch(t *testing.T) {
	acli := fakeacli.New(itSite)
	const stamp = "2026-10-01T10:00:00.000+0000"
	set := func(key, status, updated string) {
		acli.SetIssue(key, fakeacli.IssueJSON(key, "Issue "+key, status, "indeterminate", 1))
		acli.SetIssueDetail(key, biDetail(key, "Issue "+key, status, updated))
	}
	app := flowharness.New(t, flowharness.WithIssueTracker(jira.Options{Locator: acli, Runner: acli}))
	saveWorkflow(t, app, "flow", flowYAML("flow", agentStep("one", "")))
	tasks := map[string]string{}
	for _, key := range []string{"KIRA-1", "KIRA-2", "KIRA-3"} {
		set(key, "In Progress", stamp)
		task := createTask(t, app, key, "flow")
		setLink(t, app, task.ID, key, "")
		tasks[key] = task.ID
	}

	// 1. First pass: one search for all three, a detail read each (no snapshots yet).
	refresh(t, app)
	if acli.Count("search:") != 1 || acli.Count("detail:") != 3 {
		t.Fatalf("calls %v, want 1 search and 3 details", acli.Calls())
	}

	// 2. Nothing changed: one more search, no detail read.
	refresh(t, app)
	if acli.Count("search:") != 2 || acli.Count("detail:") != 3 {
		t.Fatalf("calls %v, want 2 searches and still 3 details", acli.Calls())
	}

	// 3. The search warmed every issue line: no view.
	views := acli.Count("view:")
	for _, id := range tasks {
		if ti := issueOf(t, app, id); ti.Reason != "" || ti.Status != "In Progress" {
			t.Fatalf("TaskIssue = %+v", ti)
		}
	}
	if acli.Count("view:") != views {
		t.Fatalf("TaskIssue spawned %d views, want none", acli.Count("view:")-views)
	}

	// 4. Only KIRA-2 moved: one detail read, one update.
	set("KIRA-2", "Done", "2026-10-02T10:00:00.000+0000")
	refresh(t, app)
	if acli.Count("detail:") != 4 || acli.Count("detail:KIRA-2") != 2 {
		t.Fatalf("calls %v, want one more detail, of KIRA-2", acli.Calls())
	}
	ups := trackedUpdates(t, app)
	if len(ups) != 1 || ups[0].TaskID != tasks["KIRA-2"] || !slices.Contains(ups[0].Concepts, "statusChanged") {
		t.Fatalf("updates = %+v, want statusChanged on KIRA-2 only", ups)
	}

	// 5. A key Jira does not know fails the whole search; the others are read per key.
	ghost := createTask(t, app, "Ghost", "flow")
	setLink(t, app, ghost.ID, "KIRA-404", "")
	details := acli.Count("detail:")
	refresh(t, app)
	if acli.Count("detail:") != details+3 {
		t.Fatalf("calls %v, want a detail read for each of KIRA-1..3", acli.Calls())
	}

	// 6. A search acli refuses: reads go per issue, and the next pass does not try it again.
	acli.SetSearchBroken(true)
	searches, details := acli.Count("search:"), acli.Count("detail:")
	refresh(t, app)
	if acli.Count("search:") != searches+1 || acli.Count("detail:") != details+3 {
		t.Fatalf("calls %v, want one refused search and 3 detail reads", acli.Calls())
	}
	refresh(t, app)
	if acli.Count("search:") != searches+1 {
		t.Fatalf("search tried again inside the backoff: %v", acli.Calls())
	}
	if acli.Count("detail:") != details+6 {
		t.Fatalf("calls %v, want 3 more detail reads", acli.Calls())
	}
}
