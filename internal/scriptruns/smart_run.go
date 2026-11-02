package scriptruns

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/kirathecat/kira-studio/internal/agentmcp"
	"github.com/kirathecat/kira-studio/internal/harness"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
	"github.com/kirathecat/kira-studio/internal/loginshell"
	"github.com/kirathecat/kira-studio/internal/runoutcome"
	"github.com/kirathecat/kira-studio/internal/scripts"
)

// How a background (smart or headless) run was ended from outside, set before its context is cancelled.
const (
	endNone = iota
	endUser
	endQuit
)

type bgRun struct {
	cancel context.CancelFunc
	mu     sync.Mutex
	end    int
}

func (r *bgRun) stop(end int) {
	r.mu.Lock()
	if r.end == endNone {
		r.end = end
	}
	r.mu.Unlock()
	r.cancel()
}

func (r *bgRun) endedBy() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.end
}

// mcpServer returns the loopback report server, started on first use.
func (s *Service) mcpServer() *agentmcp.Server {
	if s.server == nil {
		opts := agentmcp.Options{Dir: filepath.Join(s.Home, "automations-mcp"), OnFinish: s.recordFinish, Prompts: s.llmPrompts()}
		if s.ADE != nil {
			opts.Space, opts.Outcomes = s.ADE.Tools()
			opts.SandboxGit = s.ADE.SandboxGit()
		}
		s.server = agentmcp.NewServer(opts)
	}
	return s.server
}

func (s *Service) recordFinish(runID string, f agentmcp.Finish) {
	s.mu.Lock()
	if s.finishes == nil {
		s.finishes = map[string]agentmcp.Finish{}
	}
	s.finishes[runID] = f
	s.mu.Unlock()
}

func (s *Service) takeFinish(runID string) (agentmcp.Finish, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, ok := s.finishes[runID]
	delete(s.finishes, runID)
	return f, ok
}

func (s *Service) startSmart(p *planned) (Started, error) {
	return s.launchSmart(p, s.newRun(p, nil), false)
}

// launchSmart runs a smart script headless. existing means run is a waiting row to turn running.
func (s *Service) launchSmart(p *planned, run Run, existing bool) (Started, error) {
	runID, sessionID := run.ID, run.SessionID
	ctx, cancel := context.WithCancel(context.Background())
	sr := &bgRun{cancel: cancel}

	s.mu.Lock()
	switch {
	case s.closed:
		s.mu.Unlock()
		cancel()
		return Started{}, ipcerr.T(ipcerr.CodeUnavailable, "backend.core.scriptRun.closing.body", map[string]string{"app": s.App})
	case len(s.smart) >= maxSmartRuns:
		s.mu.Unlock()
		cancel()
		return Started{}, ipcerr.T(ipcerr.CodeConflict, "backend.core.scriptRun.smartCap.body", map[string]string{"max": strconv.Itoa(maxSmartRuns)})
	}
	if s.smart == nil {
		s.smart = map[string]*bgRun{}
	}
	s.smart[runID] = sr
	s.smartWait.Add(1)
	s.mu.Unlock()
	abandon := func() {
		cancel()
		s.mu.Lock()
		delete(s.smart, runID)
		s.mu.Unlock()
		s.smartWait.Done()
	}

	pv := p.preview
	if err := scripts.PrepareDir(p.dir); err != nil {
		abandon()
		if !existing {
			now := s.now()
			o := runoutcome.ForProcess(runoutcome.Process{End: runoutcome.EndStartErr, Err: err})
			run.State, run.Outcome, run.FinishedAt = string(o.Status), &o, &now
			s.record(run)
		}
		return Started{}, ipcerr.BadRequest(err.Error())
	}
	s.mu.Lock()
	srv := s.mcpServer()
	s.mu.Unlock()
	contextFile, err := s.contextFile()
	if err != nil {
		abandon()
		return Started{}, ipcerr.InternalErr(err)
	}
	sbx, err := s.prepareSandbox(ctx, p, runID, srv.Port(), contextFile)
	if err != nil {
		abandon()
		return Started{}, ipcerr.InternalErr(err)
	}
	grant := agentmcp.Grant{RunID: runID, TaskID: run.TaskID, Space: run.TaskID != "", Agent: p.agent}
	releaseTarget := func() {}
	if sbx != nil && p.preview.ADE != nil {
		if rel, err := s.ADE.SandboxTarget(runID, p.preview.ADE.BranchID, p.dir.Path); err == nil {
			grant.Sandbox, releaseTarget = &agentmcp.SandboxGrant{Push: true}, rel
		}
	}
	cfg, releaseGrant, err := srv.Register(grant)
	if err != nil {
		releaseTarget()
		sbx.cleanup()
		abandon()
		return Started{}, ipcerr.InternalErr(err)
	}
	if err := s.begin(run, existing); err != nil {
		releaseGrant()
		releaseTarget()
		sbx.cleanup()
		abandon()
		return Started{}, err
	}

	timeout := p.smart.TimeoutDuration()
	timeoutText := p.smart.Timeout
	if s.SmartTimeout > 0 {
		timeout, timeoutText = s.SmartTimeout, s.SmartTimeout.String()
	}
	agent := p.agent
	env := p.headless.ScrubEnv(loginshell.ScrubGitEnv(os.Environ()))
	env = append(env, "GIT_TERMINAL_PROMPT=0")
	env = append(env, p.envList...)
	spec := harness.HeadlessSpec{
		Dir: p.dir.Path, Prompt: p.sent, SessionID: sessionID, Resume: p.resume, MCPConfigs: []string{cfg},
		Model: p.smart.Model, MaxBudgetUSD: p.smart.MaxBudgetUSD, Tools: pv.Tools, AllowedTools: pv.Allowed,
		Isolated: true, UserMCP: p.mcp, GrantMCP: []string{agentmcp.ServerName}, Timeout: timeout, Env: env,
		ContextFile: contextFile,
	}
	spec = s.applySandbox(spec, sbx, agent)
	go func() {
		defer s.smartWait.Done()
		sink := newLogSink(s, runID, p.mask)
		s.noticeSandbox(sink, sbx)
		var res *harness.Result
		var stopWatch func()
		exit, runErr := p.headless.RunHeadless(ctx, spec, harness.Handler{
			OnLine:   func(l harness.Line) { sink.add(l.Stream, l.Text) },
			OnResult: func(r harness.Result) { res = &r },
			OnStart:  func(pid int) { stopWatch = s.watchSandbox(sbx, runID, sink, pid) },
			OnSessionID: func(id string) {
				if err := s.Runs.SetSessionID(runID, id); err != nil {
					slog.Warn("scriptruns: store session id", "scope", "scriptruns", "run", runID, "err", err)
					return
				}
				if r, err := s.Runs.Get(runID); err == nil {
					s.emit(r)
				}
			},
		})
		if stopWatch != nil {
			stopWatch()
		}
		sink.flush()
		releaseGrant()
		releaseTarget()
		sbx.cleanup()
		out := decide(decision{
			finish: s.takeFinishPtr(runID), res: res, exit: exit, runErr: runErr, end: sr.endedBy(),
			agent: runoutcome.AgentOf(agent), timeout: timeoutText, budget: p.smart.MaxBudgetUSD, app: s.App, lastErr: sink.lastStderr(), resumeLost: p.resume != "" && p.headless.ResumeLost(sink.lastStderr()),
		})
		out = sbx.outcome(out)
		cancel()
		done, ferr := s.Runs.FinishByRun(runID, maskOutcome(out, p.mask), s.now())
		if p.release != nil {
			p.release()
		}
		s.mu.Lock()
		delete(s.smart, runID)
		delete(s.finishes, runID)
		s.mu.Unlock()
		s.finish(done, ferr)
	}()
	return Started{RunID: runID}, nil
}

func (s *Service) takeFinishPtr(runID string) *agentmcp.Finish {
	if f, ok := s.takeFinish(runID); ok {
		return &f
	}
	return nil
}

func (s *Service) stopSmart(id string) {
	s.mu.Lock()
	sr := s.smart[id]
	s.mu.Unlock()
	if sr != nil {
		sr.stop(endUser)
	}
}

func (s *Service) stopHeadless(id string) {
	s.mu.Lock()
	br := s.bg[id]
	s.mu.Unlock()
	if br != nil {
		br.stop(endUser)
	}
}

// Live counts the smart and headless runs a quit would end.
func (s *Service) Live() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.smart) + len(s.bg)
}

// Close ends every smart and headless run as interrupted by the quit and waits until each is recorded.
func (s *Service) Close() {
	s.mu.Lock()
	s.closed = true
	if s.endLife != nil {
		s.endLife()
	}
	runs := make([]*bgRun, 0, len(s.smart)+len(s.bg))
	for _, r := range s.smart {
		runs = append(runs, r)
	}
	for _, r := range s.bg {
		runs = append(runs, r)
	}
	srv := s.server
	s.mu.Unlock()
	s.releaseAllAgents()
	for _, r := range runs {
		r.stop(endQuit)
	}
	s.smartWait.Wait()
	s.bgWait.Wait()
	if srv != nil {
		if err := srv.Close(); err != nil {
			slog.Warn("scriptruns: close report server", "scope", "scriptruns", "err", err)
		}
	}
}

type decision struct {
	finish  *agentmcp.Finish
	res     *harness.Result
	exit    harness.Exit
	runErr  error
	end     int
	timeout string
	budget  float64
	app     string
	agent   runoutcome.Agent
	lastErr string
	// resumeLost says the run resumed an earlier agent session that no longer exists.
	resumeLost bool
}

// decide maps how a smart run ended to its outcome: the first matching row wins.
func decide(d decision) runoutcome.Outcome {
	code := d.exit.Code
	var out runoutcome.Outcome
	switch {
	case d.end == endUser:
		out = runoutcome.ForProcess(runoutcome.Process{End: runoutcome.EndUser})
	case d.end == endQuit:
		out = runoutcome.ForProcess(runoutcome.Process{End: runoutcome.EndQuit, App: d.app})
	case d.finish != nil:
		out = runoutcome.Outcome{Source: runoutcome.SourceAgent, Reported: true, Summary: d.finish.Summary, ExitCode: &code}
		switch d.finish.Status {
		case "done":
			out.Status = runoutcome.StatusDone
		case "needs_input":
			out.Status, out.Reason = runoutcome.StatusBlocked, firstNonEmpty(d.finish.Reason, d.finish.Summary)
		default:
			out.Status = runoutcome.StatusFailed
			out.Reason = firstNonEmpty(d.finish.Reason, d.finish.Summary, "the agent reported failure without a reason")
		}
	case d.resumeLost:
		out = runoutcome.ForProcess(runoutcome.Process{Agent: d.agent, End: runoutcome.EndResumeLost})
	case resultEnds(d):
		out, _ = agentmcp.ResultOutcome(d.agent, d.res, code, d.budget)
	default:
		p := runoutcome.Process{Agent: d.agent, Kind: runoutcome.KindAgent, ExitCode: code, Timeout: d.timeout}
		switch {
		case d.runErr != nil:
			p.End, p.Err = runoutcome.EndStartErr, d.runErr
		case d.exit.TimedOut:
			p.End = runoutcome.EndTimeout
		}
		out = runoutcome.ForProcess(p)
	}
	out = agentmcp.ApplyResult(out, d.res)
	return out.WithLastError(d.lastErr)
}

func resultEnds(d decision) bool {
	_, ok := agentmcp.ResultOutcome(d.agent, d.res, d.exit.Code, d.budget)
	return ok
}

func firstNonEmpty(vals ...string) string { return agentmcp.FirstNonEmpty(vals...) }
