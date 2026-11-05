package ade

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adeflow"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// A task that has not started follows the live workflow file. Its first run, first session or first
// step move snapshots the whole workflow; from then on it reads the snapshot, so a later edit applies
// to new work only.

// workflowHash hashes the canonical JSON of a workflow, the encoding adeflow.Reader records.
func workflowHash(js []byte) string {
	sum := sha256.Sum256(js)
	return hex.EncodeToString(sum[:])
}

// taskWorkflow returns the workflow a task runs: its snapshot when it has started, else the live file.
func (b *TaskBoard) taskWorkflow(t model.AdeTask) (adewire.Workflow, bool) {
	if t.WorkflowJSON != "" {
		var wf adewire.Workflow
		if err := json.Unmarshal([]byte(t.WorkflowJSON), &wf); err != nil {
			slog.Warn("ade: unreadable workflow snapshot", "scope", "ade", "task", t.ID, "err", err)
			return adewire.Workflow{}, false
		}
		return wf, true
	}
	if t.WorkflowID == "" || b.deps.Workflows == nil {
		return adewire.Workflow{}, false
	}
	return b.deps.Workflows.Get(t.WorkflowID)
}

// snapshotWorkflow stores the effective workflow on a task that has none yet. The task mutex is held.
func (b *TaskBoard) snapshotWorkflow(t *model.AdeTask) error {
	if t.WorkflowID == "" || t.WorkflowJSON != "" || b.deps.Workflows == nil {
		return nil
	}
	wf, ok := b.deps.Workflows.Get(t.WorkflowID)
	if !ok {
		return invalid("workflow %q is not available", t.WorkflowID)
	}
	js, err := adeflow.SnapshotJSON(wf)
	if err != nil {
		return err
	}
	hash := workflowHash(js)
	if err := b.deps.Tasks.SetWorkflowSnapshot(t.ID, string(js), hash); err != nil {
		return err
	}
	t.WorkflowJSON, t.WorkflowHash = string(js), hash
	return nil
}

// liveInfo is a workflow's live file as a task not yet started sees it: its content hash and the step a
// new task enters.
type liveInfo struct {
	hash, first string
	ok          bool
}

// decorateTask fills the wire fields that depend on the workflow. live resolves a workflow id to its
// live file, memoised by the caller. A task that has not started follows that file, so its entry is the
// file's first step.
func decorateTask(out *adewire.Task, t model.AdeTask, live func(id string) liveInfo) {
	li := live(t.WorkflowID)
	if t.WorkflowJSON == "" {
		if li.ok {
			out.EntryStepID = li.first
		}
		return
	}
	var wf adewire.Workflow
	if err := json.Unmarshal([]byte(t.WorkflowJSON), &wf); err != nil {
		return
	}
	for i := range wf.Steps {
		if wf.Steps[i].Settings == nil {
			wf.Steps[i].Settings = map[string]map[string]string{}
		}
	}
	out.Workflow = &wf
	if li.ok {
		// Compare the re-marshalled snapshot, so a wire field added since the task started does not
		// read as an edit of the live file.
		hash := t.WorkflowHash
		if js, err := adeflow.SnapshotJSON(wf); err == nil {
			hash = workflowHash(js)
		}
		out.WorkflowOutdated = li.hash != hash
	}
}

// liveWorkflows returns a memoised id -> live workflow lookup.
func (b *TaskBoard) liveWorkflows() func(id string) liveInfo {
	seen := map[string]liveInfo{}
	return func(id string) liveInfo {
		if e, hit := seen[id]; hit {
			return e
		}
		var e liveInfo
		if b.deps.Workflows != nil && id != "" {
			if wf, ok := b.deps.Workflows.Get(id); ok {
				if js, err := adeflow.SnapshotJSON(wf); err == nil {
					e = liveInfo{hash: workflowHash(js), ok: true}
					if i, has := firstStep(wf); has {
						e.first = wf.Steps[i].ID
					}
				}
			}
		}
		seen[id] = e
		return e
	}
}
