package bridge

import (
	"slices"

	"github.com/kirathecat/kira-studio/internal/agentcfg"
	"github.com/kirathecat/kira-studio/internal/scriptruns"
	"github.com/kirathecat/kira-studio/internal/scripts"
)

// ScriptRunsService is this app's binding-name shim over scriptruns.Bound. It routes over both
// homes' stacks until the home fold: list calls merge, a call naming a run or script goes to the
// stack that owns it.
type ScriptRunsService struct {
	Space  *scriptruns.Bound
	Studio *scriptruns.Bound
	// Scripts decides which stack owns a script id.
	Scripts *CustomScriptsService
}

// ChannelScriptRunsChanged pushes one changed script run to every window.
const ChannelScriptRunsChanged = "kira:scriptRuns:changed"

// ChannelScriptRunLog pushes the log lines a smart run just stored: {runId, chunks}.
const ChannelScriptRunLog = "kira:scriptRunLog:appended"

func (s *ScriptRunsService) byScript(id string) *scriptruns.Bound {
	if s.Scripts.owner(id) == s.Scripts.Studio {
		return s.Studio
	}
	return s.Space
}

func (s *ScriptRunsService) byRun(id string) *scriptruns.Bound {
	if _, err := s.Space.Svc.Get(id); err != nil {
		if _, err := s.Studio.Svc.Get(id); err == nil {
			return s.Studio
		}
	}
	return s.Space
}

// List merges both stacks' runs, newest first, cut to args.Limit.
func (s *ScriptRunsService) List(args scriptruns.ListArgs) ([]scriptruns.Run, error) {
	space, err := s.Space.List(args)
	if err != nil {
		return nil, err
	}
	studio, err := s.Studio.List(args)
	if err != nil {
		return nil, err
	}
	out := slices.Concat(space, studio)
	slices.SortStableFunc(out, func(a, b scriptruns.Run) int { return int(b.CreatedAt - a.CreatedAt) })
	if args.Limit > 0 && len(out) > args.Limit {
		out = out[:args.Limit]
	}
	return out, nil
}

func (s *ScriptRunsService) Get(args scriptruns.IDArgs) (scriptruns.Run, error) {
	return s.byRun(args.ID).Get(args)
}

func (s *ScriptRunsService) Stop(args scriptruns.IDArgs) error {
	return s.byRun(args.ID).Stop(args)
}

func (s *ScriptRunsService) ResolveDir(args scriptruns.ResolveDirArgs) (scripts.Dir, error) {
	return s.byScript(args.ScriptID).ResolveDir(args)
}

func (s *ScriptRunsService) Preview(args scriptruns.RunArgs) (scriptruns.Preview, error) {
	return s.byScript(args.ScriptID).Preview(args)
}

func (s *ScriptRunsService) Start(args scriptruns.StartArgs) (scriptruns.Started, error) {
	return s.byScript(args.ScriptID).Start(args)
}

func (s *ScriptRunsService) Resume(args scriptruns.IDArgs) (scriptruns.Started, error) {
	return s.byRun(args.ID).Resume(args)
}

func (s *ScriptRunsService) ResumeCommand(args scriptruns.ResumeCommandArgs) (scriptruns.ResumeLaunch, error) {
	return s.byRun(args.RunID).ResumeCommand(args)
}

func (s *ScriptRunsService) ReadLog(args scriptruns.ReadLogArgs) (scriptruns.LogPage, error) {
	return s.byRun(args.ID).ReadLog(args)
}

func (s *ScriptRunsService) McpServers() (agentcfg.ServerList, error) {
	return s.Space.McpServers()
}

func (s *ScriptRunsService) NextFires(args scriptruns.NextFiresArgs) ([]int64, error) {
	return s.Space.NextFires(args)
}

func (s *ScriptRunsService) SchedulePreview(args scriptruns.ScheduleArgs) (scriptruns.Preview, error) {
	return s.byScript(args.ScriptID).SchedulePreview(args)
}

func (s *ScriptRunsService) RunScheduleNow(args scriptruns.ScheduleStartArgs) (scriptruns.Started, error) {
	return s.byScript(args.ScriptID).RunScheduleNow(args)
}

func (s *ScriptRunsService) ConfirmAccept(args scriptruns.ConfirmArgs) (scriptruns.Started, error) {
	return s.byRun(args.RunID).ConfirmAccept(args)
}

func (s *ScriptRunsService) ConfirmDecline(args scriptruns.IDArgs) error {
	return s.byRun(args.ID).ConfirmDecline(args)
}

// RestoreState merges both stacks' offers.
func (s *ScriptRunsService) RestoreState() (scriptruns.RestoreState, error) {
	space, err := s.Space.RestoreState()
	if err != nil {
		return scriptruns.RestoreState{}, err
	}
	studio, err := s.Studio.RestoreState()
	if err != nil {
		return scriptruns.RestoreState{}, err
	}
	return scriptruns.RestoreState{Items: append(space.Items, studio.Items...)}, nil
}

// Restore sends each confirmed item to the stack that offered it.
func (s *ScriptRunsService) Restore(args scriptruns.RestoreArgs) (scriptruns.RestoreResult, error) {
	var space, studio scriptruns.RestoreArgs
	for _, c := range args.Items {
		if s.byRun(c.RunID) == s.Studio {
			studio.Items = append(studio.Items, c)
		} else {
			space.Items = append(space.Items, c)
		}
	}
	var out scriptruns.RestoreResult
	for _, part := range []struct {
		b    *scriptruns.Bound
		args scriptruns.RestoreArgs
	}{{s.Space, space}, {s.Studio, studio}} {
		if len(part.args.Items) == 0 {
			continue
		}
		res, err := part.b.Restore(part.args)
		if err != nil {
			return scriptruns.RestoreResult{}, err
		}
		out.Items = append(out.Items, res.Items...)
	}
	return out, nil
}

// RestoreDismiss forgets both offers.
func (s *ScriptRunsService) RestoreDismiss() error {
	if err := s.Space.RestoreDismiss(); err != nil {
		return err
	}
	return s.Studio.RestoreDismiss()
}
