package adeflow_test

import (
	"strconv"
	"sync"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
)

// J-ADE-044: two windows edit one task at once, each field through its own call. Every field ends at its
// last write, nothing unrelated is lost, and both windows read the same board.
func TestTwoWindowsEditOneTask(t *testing.T) {
	app := flowharness.New(t)
	task, err := app.W.AdeTask.CreateTask(ctx, adewire.CreateTaskArgs{Title: "Shared"})
	if err != nil {
		t.Fatal(err)
	}
	other, err := app.W.AdeTask.CreateTask(ctx, adewire.CreateTaskArgs{Title: "Untouched"})
	if err != nil {
		t.Fatal(err)
	}
	const rounds = 8
	days := []string{"2030-03-04", "2030-03-05"}
	var wg sync.WaitGroup
	errs := make(chan error, 3*rounds)
	run := func(f func(i int) error) {
		wg.Go(func() {
			for i := range rounds {
				if err := f(i); err != nil {
					errs <- err
					return
				}
			}
		})
	}
	// Window A: title and notes. Window B: estimate, colour and plan day.
	run(func(i int) error {
		title := "Title " + strconv.Itoa(i)
		_, err := app.W.AdeTask.UpdateTask(ctx, adewire.UpdateTaskArgs{TaskID: task.ID, Patch: adewire.TaskPatch{Title: &title}})
		return err
	})
	run(func(i int) error {
		_, err := app.W.AdeTask.AddNote(ctx, adewire.AddNoteArgs{TaskID: task.ID, Body: "note " + strconv.Itoa(i)})
		return err
	})
	run(func(i int) error {
		est, color := strconv.Itoa(i+1)+"h", i
		if _, err := app.W.AdeTask.UpdateTask(ctx, adewire.UpdateTaskArgs{TaskID: task.ID, Patch: adewire.TaskPatch{Est: &est, Color: &color}}); err != nil {
			return err
		}
		day := days[i%2]
		return app.W.AdeTask.SetPlan(ctx, adewire.SetPlanArgs{Order: []string{task.ID, other.ID}, Days: map[string]*string{task.ID: &day}})
	})
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}

	first, second := board(t, app), board(t, app)
	got := taskOf(t, first, task.ID)
	if got.Title != "Title 7" || got.Est != "8h" || got.Color != 7 {
		t.Fatalf("task = title %q est %q color %d, want the last write of each field", got.Title, got.Est, got.Color)
	}
	if first.Plan.Day[task.ID] != days[(rounds-1)%2] {
		t.Fatalf("plan day = %q, want %q", first.Plan.Day[task.ID], days[(rounds-1)%2])
	}
	if o := taskOf(t, first, other.ID); o.Title != "Untouched" || o.Est != "" {
		t.Fatalf("unrelated task changed: %+v", o)
	}
	notes := 0
	for _, h := range handovers(t, app, task.ID) {
		if h.State == "note" {
			notes++
		}
	}
	if notes != rounds {
		t.Fatalf("notes = %d, want all %d", notes, rounds)
	}
	if taskOf(t, second, task.ID).Title != got.Title || second.Plan.Day[task.ID] != first.Plan.Day[task.ID] {
		t.Fatal("two reads of the board disagree")
	}
	var title string
	if err := app.DB().QueryRow(`SELECT title FROM ade_tasks WHERE id = ?`, task.ID).Scan(&title); err != nil || title != got.Title {
		t.Fatalf("DB title = %q, %v, want %q", title, err, got.Title)
	}
}
