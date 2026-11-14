package adeflow_test

import (
	"slices"
	"strconv"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
)

// J-ADE-070: 300 tasks. A reordered plan persists across restart, a second read sees it, and a 50-task
// day move through SetPlan lands all or nothing.
func TestBoardThreeHundredTasks(t *testing.T) {
	app := flowharness.New(t)
	ids := make([]string, 300)
	for i := range ids {
		tk, err := app.W.AdeTask.CreateTask(ctx, adewire.CreateTaskArgs{Title: "T" + strconv.Itoa(i)})
		if err != nil {
			t.Fatal(err)
		}
		ids[i] = tk.ID
	}
	order := slices.Clone(ids)
	slices.Reverse(order)
	if err := app.W.AdeTask.SetPlan(ctx, adewire.SetPlanArgs{Order: order}); err != nil {
		t.Fatal(err)
	}
	later := func(b adewire.Board) []string {
		var got []string
		for _, id := range b.Plan.Order {
			if slices.Contains(ids, id) {
				got = append(got, id)
			}
		}
		return got
	}
	if got := later(board(t, app)); !slices.Equal(got, order) {
		t.Fatalf("order after SetPlan differs from the request (%d ids)", len(got))
	}

	// Failure injection: one unknown id in 50 moves nothing.
	day := "2030-06-01"
	days := map[string]*string{}
	for _, id := range order[:50] {
		days[id] = &day
	}
	days["no-such-task"] = &day
	if err := app.W.AdeTask.SetPlan(ctx, adewire.SetPlanArgs{Order: order, Days: days}); err == nil {
		t.Fatal("SetPlan with an unknown task id succeeded")
	}
	for _, id := range order[:50] {
		if d := board(t, app).Plan.Day[id]; d != "" {
			t.Fatalf("task %s moved to %q despite the failed call", id, d)
		}
	}
	delete(days, "no-such-task")
	if err := app.W.AdeTask.SetPlan(ctx, adewire.SetPlanArgs{Order: order, Days: days}); err != nil {
		t.Fatal(err)
	}
	app.Restart()
	after := board(t, app)
	for _, id := range order[:50] {
		if after.Plan.Day[id] != day {
			t.Fatalf("task %s day = %q after restart, want %q", id, after.Plan.Day[id], day)
		}
	}
	if d := after.Plan.Day[order[50]]; d != "" {
		t.Fatalf("task outside the move got day %q", d)
	}
	if got := later(after); !slices.Equal(got, order) {
		t.Fatal("order changed across restart")
	}
	if got := later(board(t, app)); !slices.Equal(got, order) {
		t.Fatal("second window sees another order")
	}
}
