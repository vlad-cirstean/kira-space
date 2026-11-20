package adeflow_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
)

// shelfTitles lists the titles of ids in order.
func shelfTitles(t *testing.T, b adewire.Board, ids []string) []string {
	t.Helper()
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = taskOf(t, b, id).Title
	}
	return out
}

// laterIDs is the Later shelf of b: undated, non-Backlog, non-review tasks in plan order.
func laterIDs(b adewire.Board) []string {
	var out []string
	for _, id := range b.Plan.Order {
		var kind string
		for _, tk := range b.Tasks {
			if tk.ID == id {
				kind = tk.Kind
			}
		}
		if _, dated := b.Plan.Day[id]; !dated && kind != "review" && !slices.Contains(b.Plan.Backlog, id) {
			out = append(out, id)
		}
	}
	return out
}

func TestShelves(t *testing.T) {
	app := flowharness.New(t)
	svc := app.W.AdeTask
	repo := app.NewRepo("api")
	repo.Commit("base", map[string]string{"a.txt": "base\n"})
	rec := importRepo(t, app, repo.Dir)

	capture := func(title string) adewire.Task {
		t.Helper()
		task, err := svc.CaptureTask(ctx, adewire.CaptureTaskArgs{Title: title})
		if err != nil {
			t.Fatal(err)
		}
		return task
	}
	shelve := func(task adewire.Task, shelf, before string) error {
		return svc.ShelveTask(ctx, adewire.ShelveTaskArgs{TaskID: task.ID, Shelf: shelf, BeforeID: before})
	}
	wantShelves := func(step string, later, backlog []string) {
		t.Helper()
		b := board(t, app)
		if got := shelfTitles(t, b, laterIDs(b)); !slices.Equal(got, later) {
			t.Fatalf("%s: later = %v, want %v", step, got, later)
		}
		if got := shelfTitles(t, b, b.Plan.Backlog); !slices.Equal(got, backlog) {
			t.Fatalf("%s: backlog = %v, want %v", step, got, backlog)
		}
	}

	one, two := capture("one"), capture("two")
	wantShelves("capture lands on top of Later", []string{"two", "one"}, []string{})
	if _, err := svc.CaptureTask(ctx, adewire.CaptureTaskArgs{Title: "  "}); err == nil {
		t.Fatal("capturing a blank title succeeded")
	}

	today := "2026-10-12"
	planned, err := svc.CreateTask(ctx, adewire.CreateTaskArgs{Title: "planned", CodeRepoIDs: []string{rec.ID}, Day: &today})
	if err != nil {
		t.Fatal(err)
	}
	branches := taskOf(t, board(t, app), planned.ID).BranchIDs
	if err := shelve(planned, adewire.ShelfBacklog, ""); err != nil {
		t.Fatal(err)
	}
	b := board(t, app)
	if !slices.Equal(b.Plan.Backlog, []string{planned.ID}) {
		t.Fatalf("backlog = %v, want [%s]", b.Plan.Backlog, planned.ID)
	}
	if day, ok := b.Plan.Day[planned.ID]; ok {
		t.Fatalf("shelved task keeps day %q", day)
	}
	if got := taskOf(t, b, planned.ID).BranchIDs; !slices.Equal(got, branches) {
		t.Fatalf("shelving changed branches %v to %v", branches, got)
	}

	if err := shelve(one, adewire.ShelfLater, two.ID); err != nil {
		t.Fatal(err)
	}
	wantShelves("reorder Later before a task", []string{"one", "two"}, []string{"planned"})
	if err := shelve(one, adewire.ShelfLater, ""); err != nil {
		t.Fatal(err)
	}
	wantShelves("no beforeId lands at the end of Later", []string{"two", "one"}, []string{"planned"})

	if err := shelve(two, adewire.ShelfBacklog, planned.ID); err != nil {
		t.Fatal(err)
	}
	wantShelves("Later to Backlog before a task", []string{"one"}, []string{"two", "planned"})
	if err := shelve(planned, adewire.ShelfLater, one.ID); err != nil {
		t.Fatal(err)
	}
	wantShelves("Backlog to Later before a task", []string{"planned", "one"}, []string{"two"})

	if err := svc.SetPlan(ctx, adewire.SetPlanArgs{Order: board(t, app).Plan.Order, Days: map[string]*string{two.ID: &today}}); err != nil {
		t.Fatal(err)
	}
	b = board(t, app)
	if b.Plan.Day[two.ID] != today || slices.Contains(b.Plan.Backlog, two.ID) {
		t.Fatalf("dating a Backlog task left day %q, backlog %v", b.Plan.Day[two.ID], b.Plan.Backlog)
	}
	if err := shelve(two, adewire.ShelfBacklog, ""); err != nil {
		t.Fatal(err)
	}
	if err := svc.SetPlan(ctx, adewire.SetPlanArgs{Order: board(t, app).Plan.Order, Days: map[string]*string{two.ID: nil}}); err != nil {
		t.Fatal(err)
	}
	wantShelves("Plan > Later takes a Backlog task off Backlog", []string{"planned", "one", "two"}, []string{})
	if err := shelve(two, adewire.ShelfBacklog, ""); err != nil {
		t.Fatal(err)
	}
	app.Contract(t, "ade-shelf", "AdeTaskService.Board#shelves", board(t, app), flowharness.Mask(
		"createdAt", "addedAt", "since", "color", "startedAt", "finishedAt", "mergedAt", "exitCode"))

	if err := shelve(one, adewire.ShelfBacklog, one.ID); err == nil {
		t.Fatal("a task before itself succeeded")
	}
	err = shelve(one, adewire.ShelfBacklog, planned.ID)
	if err == nil || !strings.Contains(err.Error(), "shelf") {
		t.Fatalf("beforeId on the other shelf = %v, want a shelf refusal", err)
	}
	if err := svc.ShelveTask(ctx, adewire.ShelveTaskArgs{TaskID: one.ID, Shelf: "plan"}); err == nil {
		t.Fatal("unknown shelf succeeded")
	}

	revRepo := app.NewRepo("rev")
	revRepo.Commit("base", map[string]string{"a.txt": "base\n"})
	revRepo.Git("checkout", "-q", "-b", "sara/topic", "main")
	revRepo.Commit("topic", map[string]string{"b.txt": "topic\n"})
	revRepo.Checkout("main")
	revRepo.Git("config", "user.email", "me@example.com")
	revRec := importRepo(t, app, revRepo.Dir)
	res, err := svc.AddExistingBranch(ctx, adewire.AddExistingBranchArgs{CodeRepoID: revRec.ID, Name: "sara/topic"})
	if err != nil {
		t.Fatal(err)
	}
	if err := shelve(res.Task, adewire.ShelfBacklog, ""); err == nil {
		t.Fatal("shelving a review item succeeded")
	}

	if err := svc.ArchiveTask(ctx, adewire.ArchiveTaskArgs{TaskID: one.ID}); err != nil {
		t.Fatal(err)
	}
	if err := shelve(one, adewire.ShelfLater, ""); err == nil {
		t.Fatal("shelving an archived task succeeded")
	}
}
