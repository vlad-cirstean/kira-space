package adeflow_test

import (
	"strconv"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
)

// P764: task colours have 10 slots. The first 10 live tasks take 0-9 without a repeat, the 11th reuses a
// slot in range, and a patch outside 0-9 is refused.
func TestTaskColorSlotsTen(t *testing.T) {
	app := flowharness.New(t)
	seen := map[int]bool{}
	var last adewire.Task
	for i := range 11 {
		tk, err := app.W.AdeTask.CreateTask(ctx, adewire.CreateTaskArgs{Title: "Task " + strconv.Itoa(i)})
		if err != nil {
			t.Fatal(err)
		}
		if tk.Color < 0 || tk.Color > 9 {
			t.Fatalf("task %d colour = %d, want 0-9", i, tk.Color)
		}
		if i < 10 && seen[tk.Color] {
			t.Fatalf("task %d repeats colour %d before all 10 are used", i, tk.Color)
		}
		seen[tk.Color] = true
		last = tk
	}
	bad := 10
	if _, err := app.W.AdeTask.UpdateTask(ctx, adewire.UpdateTaskArgs{TaskID: last.ID, Patch: adewire.TaskPatch{Color: &bad}}); err == nil {
		t.Fatal("patch to colour 10 succeeded, want an error")
	}
	if got := taskOf(t, board(t, app), last.ID).Color; got == bad {
		t.Fatalf("colour = %d after refused patch", got)
	}
}
