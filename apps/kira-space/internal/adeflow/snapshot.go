package adeflow

import (
	"encoding/json"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
)

// SnapshotJSON is the canonical JSON of wf as hashed and stored on a task: the library reference is
// cleared, so moving a step into the library changes nothing a started task compares.
func SnapshotJSON(wf adewire.Workflow) ([]byte, error) {
	steps := make([]adewire.WorkflowStep, len(wf.Steps))
	for i, s := range wf.Steps {
		s.Use = ""
		steps[i] = s
	}
	wf.Steps = steps
	return json.Marshal(wf)
}
