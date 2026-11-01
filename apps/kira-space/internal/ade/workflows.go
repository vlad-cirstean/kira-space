package ade

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adeflow"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
)

// workflowsEmitDelay coalesces an app write and the watcher's echo of it into one channel event.
const workflowsEmitDelay = 250 * time.Millisecond

// wfErr maps the writer's caller mistakes to ErrInvalidInput.
// ErrWorkflowChanged marks a workflow save built on text that has since changed; the bridge maps it
// to E_CONFLICT.
var ErrWorkflowChanged = errors.New("ade: workflow changed")

func wfErr(err error) error {
	if errors.Is(err, adeflow.ErrConflict) {
		return fmt.Errorf("%w: %s", ErrWorkflowChanged, strings.TrimPrefix(err.Error(), "configfs: conflict: "))
	}
	if errors.Is(err, adeflow.ErrInvalid) {
		return fmt.Errorf("%w: %s", ErrInvalidInput, strings.TrimPrefix(err.Error(), "configfs: invalid: "))
	}
	return err
}

func (b *TaskBoard) workflowUsage() (func(id string) int, error) {
	tasks, err := b.deps.Tasks.ListLive()
	if err != nil {
		return nil, err
	}
	used := make(map[string]int)
	for _, t := range tasks {
		if t.WorkflowID != "" {
			used[t.WorkflowID]++
		}
	}
	return func(id string) int { return used[id] }, nil
}

func (b *TaskBoard) withUsage(e adewire.WorkflowEntry) adewire.WorkflowEntry {
	if e.Workflow == nil {
		return e
	}
	if usedBy, err := b.workflowUsage(); err == nil {
		e.UsedBy = usedBy(e.Workflow.ID)
	}
	return e
}

// scheduleWorkflows emits the workflows channel once per burst of app writes and file events.
func (b *TaskBoard) scheduleWorkflows() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.wfTimer != nil {
		b.wfTimer.Stop()
	}
	b.wfTimer = time.AfterFunc(workflowsEmitDelay, func() {
		b.runTracked(func() {
			if b.deps.OnWorkflows != nil {
				b.deps.OnWorkflows()
			}
		})
	})
}

// workflowsFileChanged is the watcher callback: List records each valid file as last-valid before
// anyone reads it, then the channel fires.
func (b *TaskBoard) workflowsFileChanged() {
	b.deps.Workflows.List(nil)
	b.scheduleWorkflows()
}

// WorkflowYaml returns one workflow file's raw text.
func (b *TaskBoard) WorkflowYaml(_ context.Context, file string) (adewire.WorkflowYaml, error) {
	y, err := b.deps.Workflows.ReadYaml(file)
	return y, wfErr(err)
}

// ValidateWorkflowYaml parses text without touching disk.
func (b *TaskBoard) ValidateWorkflowYaml(_ context.Context, src string) adewire.WorkflowValidation {
	return adeflow.ValidateYaml(src)
}

// SaveWorkflow rewrites a file from the structured workflow. Running tasks keep the snapshot
// they started with; an edit applies to new work only.
func (b *TaskBoard) SaveWorkflow(_ context.Context, args adewire.SaveWorkflowArgs) (adewire.WorkflowEntry, error) {
	e, err := b.deps.Workflows.Save(args.FileName, args.BaseHash, args.Workflow)
	if err != nil {
		return adewire.WorkflowEntry{}, wfErr(err)
	}
	b.scheduleWorkflows()
	return b.withUsage(e), nil
}

// SaveWorkflowYaml writes the text as given, valid or not.
func (b *TaskBoard) SaveWorkflowYaml(_ context.Context, args adewire.SaveWorkflowYamlArgs) (adewire.WorkflowEntry, error) {
	e, err := b.deps.Workflows.SaveYaml(args.FileName, args.BaseHash, args.Yaml)
	if err != nil {
		return adewire.WorkflowEntry{}, wfErr(err)
	}
	b.scheduleWorkflows()
	return b.withUsage(e), nil
}

// ImportWorkflow copies a workflow file from path into the workflows folder.
func (b *TaskBoard) ImportWorkflow(_ context.Context, args adewire.ImportWorkflowArgs) (adewire.WorkflowEntry, error) {
	e, err := b.deps.Workflows.Import(args.Path)
	if err != nil {
		return adewire.WorkflowEntry{}, wfErr(err)
	}
	b.scheduleWorkflows()
	return b.withUsage(e), nil
}

// NewWorkflow creates a minimal valid workflow named args.Name.
func (b *TaskBoard) NewWorkflow(_ context.Context, args adewire.NewWorkflowArgs) (adewire.WorkflowEntry, error) {
	e, err := b.deps.Workflows.New(args.Name)
	if err != nil {
		return adewire.WorkflowEntry{}, wfErr(err)
	}
	b.scheduleWorkflows()
	return b.withUsage(e), nil
}

// Start begins the background work that outlives a call: the workflows folder watch and the watched
// repo folders. Close stops it.
func (b *TaskBoard) Start() {
	b.startOnce.Do(func() {
		if b.deps.Workflows != nil {
			stop, err := b.deps.Workflows.Watch(b.workflowsFileChanged)
			if err != nil {
				slog.Warn("ade workflows: watch", "scope", "ade", "err", err)
			} else {
				b.folderWMu.Lock()
				b.stopFlowW = stop
				b.folderWMu.Unlock()
			}
		}
		if len(b.recoveredRebases) > 0 {
			recovered := b.recoveredRebases
			b.goTracked(func() { b.reverifyRecovered(recovered) })
		}
		b.goTracked(b.launchAutomationHeld)
		b.goTracked(b.reopenHandoverPrompts)
		b.goTracked(func() { b.RunAllEnvScripts(b.ctx) })
		folders, err := b.deps.RepoConfig.Folders()
		if err != nil {
			slog.Warn("ade folders: list", "scope", "ade", "err", err)
			return
		}
		for _, f := range folders {
			if f.Watch {
				b.startFolderWatch(f.Path)
			}
		}
	})
}

func (b *TaskBoard) stopWatchers() {
	b.folderWMu.Lock()
	stopFlow := b.stopFlowW
	b.stopFlowW = nil
	watchers := b.folderW
	b.folderW = map[string]*folderWatcher{}
	b.folderWMu.Unlock()
	if stopFlow != nil {
		stopFlow()
	}
	for _, w := range watchers {
		w.stop()
	}
}
