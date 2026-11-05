package ade

import (
	"context"
	"log/slog"
	"sort"
	"strings"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adeflow"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
)

// steps is the library the workflows reader resolves against.
func (b *TaskBoard) steps() (*adeflow.StepLibrary, error) {
	if b.deps.Workflows == nil || b.deps.Workflows.Steps == nil {
		return nil, invalid("the step library is not available")
	}
	return b.deps.Workflows.Steps, nil
}

// startStepLibrary writes the library's AGENTS.md and watches its folder.
func (b *TaskBoard) startStepLibrary() {
	lib := b.deps.Workflows.Steps
	if lib == nil {
		return
	}
	if err := lib.WriteDocs(); err != nil {
		slog.Warn("ade steps: write AGENTS.md", "scope", "ade", "err", err)
	}
	stop, err := lib.Watch(b.stepsFileChanged)
	if err != nil {
		slog.Warn("ade steps: watch", "scope", "ade", "err", err)
		return
	}
	b.folderWMu.Lock()
	b.stopStepsW = stop
	b.folderWMu.Unlock()
}

// stepUsers returns the sorted ids of the workflows that reference each library step.
func (b *TaskBoard) stepUsers() map[string][]string {
	users := b.deps.Workflows.StepUsers()
	for _, ids := range users {
		sort.Strings(ids)
	}
	return users
}

// stepList lists the library with the workflows that reference each step.
func (b *TaskBoard) stepList() []adewire.StepEntry {
	if b.deps.Workflows == nil || b.deps.Workflows.Steps == nil {
		return []adewire.StepEntry{}
	}
	users := b.stepUsers()
	return b.deps.Workflows.Steps.List(func(id string) []string { return users[id] })
}

func (b *TaskBoard) stepSecretParam(step adewire.WorkflowStep) error {
	if name, param := b.workflowSecretParam(&adewire.Workflow{Steps: []adewire.WorkflowStep{step}}); param != "" {
		return invalid("%s", secretParamMsg(name, param))
	}
	return nil
}

func (b *TaskBoard) withStepUsage(e adewire.StepEntry) adewire.StepEntry {
	if users := b.stepUsers()[strings.TrimSuffix(e.FileName, ".yaml")]; users != nil {
		e.UsedBy = users
	}
	return e
}

// StepYaml returns one library file's raw text.
func (b *TaskBoard) StepYaml(_ context.Context, file string) (adewire.StepYaml, error) {
	lib, err := b.steps()
	if err != nil {
		return adewire.StepYaml{}, err
	}
	y, err := lib.ReadYaml(file)
	return y, wfErr(err)
}

// ValidateStepYaml parses text without touching disk.
func (b *TaskBoard) ValidateStepYaml(_ context.Context, src string) adewire.StepValidation {
	v := adeflow.ValidateStepYaml(src)
	if v.Step != nil {
		if err := b.stepSecretParam(*v.Step); err != nil {
			return adewire.StepValidation{Error: &adewire.WorkflowError{Line: 1, Message: trimInvalid(err)}}
		}
	}
	return v
}

// SaveStep rewrites a library file from the structured step. Started tasks keep their snapshot.
func (b *TaskBoard) SaveStep(_ context.Context, args adewire.SaveStepArgs) (adewire.StepEntry, error) {
	lib, err := b.steps()
	if err != nil {
		return adewire.StepEntry{}, err
	}
	if err := b.stepSecretParam(args.Step); err != nil {
		return adewire.StepEntry{}, err
	}
	e, err := lib.Save(args.FileName, args.BaseHash, args.Step)
	if err != nil {
		return adewire.StepEntry{}, wfErr(err)
	}
	b.scheduleWorkflows()
	return b.withStepUsage(e), nil
}

// SaveStepYaml writes the text as given, valid or not.
func (b *TaskBoard) SaveStepYaml(_ context.Context, args adewire.SaveStepYamlArgs) (adewire.StepEntry, error) {
	lib, err := b.steps()
	if err != nil {
		return adewire.StepEntry{}, err
	}
	e, err := lib.SaveYaml(args.FileName, args.BaseHash, args.Yaml)
	if err != nil {
		return adewire.StepEntry{}, wfErr(err)
	}
	b.scheduleWorkflows()
	return b.withStepUsage(e), nil
}

// NewStep creates a library step, or saves args.From to the library.
func (b *TaskBoard) NewStep(_ context.Context, args adewire.NewStepArgs) (adewire.StepEntry, error) {
	lib, err := b.steps()
	if err != nil {
		return adewire.StepEntry{}, err
	}
	if args.From != nil {
		if err := b.stepSecretParam(*args.From); err != nil {
			return adewire.StepEntry{}, err
		}
	}
	e, err := lib.New(args.Name, args.Kind, args.From)
	if err != nil {
		return adewire.StepEntry{}, wfErr(err)
	}
	b.scheduleWorkflows()
	return b.withStepUsage(e), nil
}

// DeleteStep removes a library file that no workflow references.
func (b *TaskBoard) DeleteStep(_ context.Context, args adewire.DeleteStepArgs) error {
	lib, err := b.steps()
	if err != nil {
		return err
	}
	if users := b.stepUsers()[strings.TrimSuffix(args.FileName, ".yaml")]; len(users) > 0 {
		return invalid("used by workflows: %s", strings.Join(users, ", "))
	}
	if err := lib.Delete(args.FileName); err != nil {
		return wfErr(err)
	}
	b.scheduleWorkflows()
	return nil
}
