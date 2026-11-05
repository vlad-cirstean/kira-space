package adeflow_test

import (
	"testing"

	"github.com/kirathecat/kira-studio/internal/ids"
)

// Task, branch and run keys are UUIDs.
func TestAdeIDs(t *testing.T) {
	f, br := doneFixture(t, "feat/ids")
	run, ok := latestRun(t, f.app, f.taskID, "one")
	if !ok {
		t.Fatal("no run")
	}
	for name, id := range map[string]string{"task": f.taskID, "branch": br.ID, "run": run.ID} {
		if !ids.IsUUID(id) {
			t.Errorf("%s id %q is not a UUID", name, id)
		}
	}
}
