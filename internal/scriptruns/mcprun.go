package scriptruns

import (
	"context"
	"maps"
	"slices"

	"github.com/kirathecat/kira-studio/internal/ipcerr"
	"github.com/kirathecat/kira-studio/internal/scripts"
)

// agentArgs refuses what an agent cannot run: a smart script, and a value for a secret param.
func (s *Service) agentArgs(args RunArgs) error {
	rec, err := s.Scripts.Get(args.ScriptID)
	if err != nil {
		return ipcerr.InternalErr(err)
	}
	if rec == nil {
		return ipcerr.T(ipcerr.CodeNotFound, "automations.script.gone.error", nil)
	}
	if rec.Kind == scripts.KindSmart {
		return ipcerr.T(ipcerr.CodeBadRequest, "backend.core.scriptRun.mcpSmart.body", nil)
	}
	for _, p := range rec.Params {
		if _, given := args.Params[p.Name]; given && p.Secret {
			return ipcerr.T(ipcerr.CodeBadRequest, "backend.core.scriptRun.mcpSecret.body", map[string]string{"name": p.Name})
		}
	}
	return nil
}

// PlanMCP resolves the headless run an agent asked for, without starting it. A secret param takes
// only its saved value.
func (s *Service) PlanMCP(args RunArgs) (Preview, error) {
	if err := s.agentArgs(args); err != nil {
		return Preview{}, err
	}
	p, err := s.plan(s.lifeCtx(), args, planOpts{headless: true, informational: true})
	if err != nil {
		return Preview{}, err
	}
	return p.preview, nil
}

// mcpTimeout sets a planned run's limit from the script's mcp block.
func mcpTimeout(p *planned) {
	p.timeoutText = scripts.DefaultScheduleTimeout
	p.timeout = scripts.MCPAccess{}.TimeoutDuration()
	if m := p.script.MCP; m != nil && m.Timeout != "" {
		p.timeout, p.timeoutText = m.TimeoutDuration(), m.Timeout
	}
}

// StartMCP starts the run PlanMCP showed as a headless run with trigger mcp. A script that changed
// since the preview, or one that already runs, is refused.
func (s *Service) StartMCP(args StartArgs) (Started, error) {
	if err := s.agentArgs(args.RunArgs); err != nil {
		return Started{}, err
	}
	s.startMu.Lock()
	defer s.startMu.Unlock()
	p, err := s.plan(s.lifeCtx(), args.RunArgs, planOpts{headless: true})
	if err != nil {
		return Started{}, err
	}
	if live, err := s.Runs.Running(p.script.ID, "", ""); err != nil {
		return Started{}, ipcerr.InternalErr(err)
	} else if live != nil {
		return Started{}, ipcerr.T(ipcerr.CodeConflict, "backend.core.scriptRun.alreadyRunning.body", map[string]string{"name": p.script.Name})
	}
	if err := checkStart(p, args.Hash); err != nil {
		return Started{}, err
	}
	p.trigger = TriggerMCP
	mcpTimeout(p)
	return s.startHeadless(p, s.newRun(p, nil), false)
}

// Wait returns the run once it is no longer running or waiting, or its current row when ctx ends
// first. An unknown run is an error.
func (s *Service) Wait(ctx context.Context, runID string) (Run, error) {
	ch := s.watch(runID)
	defer s.unwatch(runID, ch)
	for {
		run, err := s.Runs.Get(runID)
		if err != nil {
			return Run{}, err
		}
		if run.State != StateRunning && run.State != StateWaiting {
			return run, nil
		}
		select {
		case <-ch:
		case <-ctx.Done():
			return run, nil
		}
	}
}

// watch registers a waiter before the caller reads the row, so a finish between the two is not missed.
func (s *Service) watch(runID string) chan struct{} {
	ch := make(chan struct{}, 1)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.waiters == nil {
		s.waiters = map[string]map[chan struct{}]struct{}{}
	}
	if s.waiters[runID] == nil {
		s.waiters[runID] = map[chan struct{}]struct{}{}
	}
	s.waiters[runID][ch] = struct{}{}
	return ch
}

func (s *Service) unwatch(runID string, ch chan struct{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.waiters[runID], ch)
	if len(s.waiters[runID]) == 0 {
		delete(s.waiters, runID)
	}
}

// wake releases every Wait on a run that just ended.
func (s *Service) wake(runID string) {
	s.mu.Lock()
	chans := slices.Collect(maps.Keys(s.waiters[runID]))
	s.mu.Unlock()
	for _, ch := range chans {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}
