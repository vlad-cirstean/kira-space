package ade

import (
	"context"
	"fmt"
	"log/slog"
	"slices"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// resolveStepStatus is the status a step applies: its own when the set still holds it, else the
// default of its kind; "" keeps the current status.
func resolveStepStatus(set []model.AdeTaskStatus, defaults model.AdeStatusDefaults, step adewire.WorkflowStep) string {
	if slices.ContainsFunc(set, func(s model.AdeTaskStatus) bool { return s.ID == step.Status }) {
		return step.Status
	}
	if step.Kind == adewire.KindReview {
		return defaults.Review
	}
	return defaults.Step
}

// notStarted says the task sits at its first non-skipped step with nothing run in the pass.
func notStarted(tc *taskCtx, plan []stepView) bool {
	entry := tc.entryIndex(plan)
	first := slices.IndexFunc(plan, func(v stepView) bool { return !v.Def.Skip })
	if entry < 0 || first < 0 || entry != first || currentStep(plan, entry) != first {
		return false
	}
	return !slices.ContainsFunc(plan, func(v stepView) bool { return v.started() })
}

// statusKey names the workflow position whose status the task shows: "done" when finished, "" while
// not started, else the current step id. ok is false when the position cannot be read.
func (b *TaskBoard) statusKey(tc *taskCtx) (key string, ok bool) {
	if tc.entry == entryDone {
		return entryDone, true
	}
	if !tc.active() {
		return "", true
	}
	plan, err := b.plan(tc)
	if err != nil {
		return "", false
	}
	entry := tc.entryIndex(plan)
	if entry < 0 {
		return "", false
	}
	cur := currentStep(plan, entry)
	if cur < 0 {
		return entryDone, true
	}
	if notStarted(tc, plan) {
		sessions, err := b.deps.Sessions.HasTask(tc.task.ID)
		if err != nil {
			return "", false
		}
		if !sessions {
			return "", true
		}
	}
	return plan[cur].Def.ID, true
}

// syncStatus applies the status of the task's workflow position when that position changed since the
// last sync; the task mutex is held. A manual move leaves the position alone, so it holds until the
// task moves on. Only work tasks sync.
func (b *TaskBoard) syncStatus(taskID string) { b.syncStatusWith(taskID, nil) }

// statusConfig is the status set and defaults a status sync reads.
type statusConfig struct {
	set      []model.AdeTaskStatus
	defaults model.AdeStatusDefaults
}

func (b *TaskBoard) loadStatusConfig() (statusConfig, error) {
	set, err := b.deps.Statuses.List()
	if err != nil {
		return statusConfig{}, fmt.Errorf("read task statuses: %w", err)
	}
	defaults, err := b.deps.Statuses.Defaults()
	if err != nil {
		return statusConfig{}, fmt.Errorf("read task status defaults: %w", err)
	}
	return statusConfig{set, defaults}, nil
}

// syncStatusWith is syncStatus with the status config already loaded; nil loads it when a sync needs it.
func (b *TaskBoard) syncStatusWith(taskID string, cfg *statusConfig) {
	tc, err := b.loadTaskCtx(taskID)
	if err != nil || !model.IsWorkKind(tc.task.Kind) {
		return
	}
	key, ok := b.statusKey(tc)
	if !ok || key == tc.task.StatusStep {
		return
	}
	if cfg == nil {
		loaded, err := b.loadStatusConfig()
		if err != nil {
			slog.Warn("ade: sync task status", "scope", "ade", "task", taskID, "err", err)
			return
		}
		cfg = &loaded
	}
	set, defaults := cfg.set, cfg.defaults
	var target string
	switch key {
	case "":
		target = defaults.New
	case entryDone:
		target = defaults.Finished
	default:
		i := slices.IndexFunc(tc.wf.Steps, func(st adewire.WorkflowStep) bool { return st.ID == key })
		if i < 0 {
			return
		}
		target = resolveStepStatus(set, defaults, tc.wf.Steps[i])
	}
	if target == "" {
		return
	}
	if err := b.deps.Tasks.SetAutoStatus(taskID, target, key); err != nil {
		slog.Warn("ade: sync task status", "scope", "ade", "task", taskID, "err", err)
		return
	}
	b.notifyBoardWith(taskChanged(taskID))
}

// OnTUIStarted is Tracker.OnStarted's target: a first session of a task counts as started.
func (b *TaskBoard) OnTUIStarted(taskID string) {
	mu := b.taskMu(taskID)
	mu.Lock()
	defer mu.Unlock()
	b.syncStatus(taskID)
}

// SaveStatuses rewrites the status set and defaults; tasks on a removed status move in the same transaction.
func (b *TaskBoard) SaveStatuses(_ context.Context, args adewire.SaveStatusesArgs) (adewire.SaveStatusesResult, error) {
	set := make([]model.AdeTaskStatus, len(args.Statuses))
	for i, s := range args.Statuses {
		set[i] = model.AdeTaskStatus{ID: s.ID, Name: s.Name, Color: s.Color}
	}
	d := model.AdeStatusDefaults{New: args.Defaults.New, Step: args.Defaults.Step, Review: args.Defaults.Review, Finished: args.Defaults.Finished}
	saved, savedDefaults, err := b.deps.Statuses.Save(set, d, args.Reassign)
	if err != nil {
		return adewire.SaveStatusesResult{}, err
	}
	b.notifyBoard()
	return adewire.SaveStatusesResult{Statuses: toWireStatuses(saved), Defaults: toWireDefaults(savedDefaults)}, nil
}

func toWireStatuses(set []model.AdeTaskStatus) []adewire.TaskStatusDef {
	out := make([]adewire.TaskStatusDef, len(set))
	for i, s := range set {
		out[i] = adewire.TaskStatusDef{ID: s.ID, Name: s.Name, Color: s.Color}
	}
	return out
}

func toWireDefaults(d model.AdeStatusDefaults) adewire.StatusDefaults {
	return adewire.StatusDefaults{New: d.New, Step: d.Step, Review: d.Review, Finished: d.Finished}
}
