package bridge

import (
	"context"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
)

// The step library methods of AdeTaskService: standalone steps that workflows reference.

func (s *AdeTaskService) StepYaml(ctx context.Context, args adewire.FileNameArgs) (adewire.StepYaml, error) {
	ctx = adeUserCtx(ctx)
	if err := validateAdeBranchName(args.FileName, "fileName"); err != nil {
		return adewire.StepYaml{}, err
	}
	r, err := s.Engine.StepYaml(ctx, args.FileName)
	return r, adeTaskError(err)
}

func (s *AdeTaskService) ValidateStepYaml(ctx context.Context, args adewire.ValidateStepYamlArgs) (adewire.StepValidation, error) {
	ctx = adeUserCtx(ctx)
	if err := validateAdeYaml(args.Yaml); err != nil {
		return adewire.StepValidation{}, err
	}
	return s.Engine.ValidateStepYaml(ctx, args.Yaml), nil
}

func (s *AdeTaskService) SaveStep(ctx context.Context, args adewire.SaveStepArgs) (adewire.StepEntry, error) {
	ctx = adeUserCtx(ctx)
	if err := validateAdeBranchName(args.FileName, "fileName"); err != nil {
		return adewire.StepEntry{}, err
	}
	r, err := s.Engine.SaveStep(ctx, args)
	return r, adeTaskError(err)
}

func (s *AdeTaskService) SaveStepYaml(ctx context.Context, args adewire.SaveStepYamlArgs) (adewire.StepEntry, error) {
	ctx = adeUserCtx(ctx)
	if err := validateAdeBranchName(args.FileName, "fileName"); err != nil {
		return adewire.StepEntry{}, err
	}
	if err := validateAdeYaml(args.Yaml); err != nil {
		return adewire.StepEntry{}, err
	}
	r, err := s.Engine.SaveStepYaml(ctx, args)
	return r, adeTaskError(err)
}

// NewStep creates a library step, or saves args.From (an inline workflow step) to the library.
func (s *AdeTaskService) NewStep(ctx context.Context, args adewire.NewStepArgs) (adewire.StepEntry, error) {
	ctx = adeUserCtx(ctx)
	if err := validateAdeName(args.Name, "name"); err != nil {
		return adewire.StepEntry{}, err
	}
	r, err := s.Engine.NewStep(ctx, args)
	return r, adeTaskError(err)
}

// DeleteStep removes a library step that no workflow references.
func (s *AdeTaskService) DeleteStep(ctx context.Context, args adewire.DeleteStepArgs) error {
	ctx = adeUserCtx(ctx)
	if err := validateAdeBranchName(args.FileName, "fileName"); err != nil {
		return err
	}
	return adeTaskError(s.Engine.DeleteStep(ctx, args))
}
