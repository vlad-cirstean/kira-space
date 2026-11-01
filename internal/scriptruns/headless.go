package scriptruns

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
	"github.com/kirathecat/kira-studio/internal/linewriter"
	"github.com/kirathecat/kira-studio/internal/loginshell"
	"github.com/kirathecat/kira-studio/internal/procgroup"
	"github.com/kirathecat/kira-studio/internal/runoutcome"
	"github.com/kirathecat/kira-studio/internal/scripts"
)

// headlessGrace is SIGTERM's grace window before SIGKILL.
const headlessGrace = 2 * time.Second

// newRun builds the run row of a planned run. existing is the waiting row it fills, if any: the
// row keeps its id and due time.
func (s *Service) newRun(p *planned, existing *Run) Run {
	now := s.now()
	run := Run{
		ID: uuid.NewString(), ScriptID: p.script.ID, ScriptName: p.script.Name, Color: p.script.Color, Kind: p.script.Kind,
		Trigger: runTrigger(p), State: StateRunning, Cwd: p.dir.Path, CreatedAt: now, StartedAt: &now, Params: p.params,
	}
	if existing != nil {
		run.ID, run.CreatedAt = existing.ID, existing.CreatedAt
	}
	if p.script.Kind == scripts.KindSmart {
		pv := p.preview
		run.Model, run.SessionID, run.Prompt = pv.Model, firstNonEmpty(p.resume, uuid.NewString()), p.sent
		run.ResumedFrom = p.resumedFrom
		run.Provider = p.provider
		run.Tools = RunTools{Tools: pv.Tools, AllowedTools: pv.Allowed, McpServers: pv.MCP}
	} else {
		run.Command = p.script.Command
	}
	if a := p.preview.ADE; a != nil {
		run.TaskID, run.TaskTitle, run.BranchID, run.BranchLabel = a.TaskID, a.TaskTitle, a.BranchID, a.BranchLabel
	}
	return run
}

// begin stores the run as running: a new row, or the waiting row turned running. A waiting row
// already answered elsewhere fails the start.
func (s *Service) begin(run Run, existing bool) error {
	if !existing {
		if err := s.Runs.Insert(run); err != nil {
			return ipcerr.InternalErr(err)
		}
		s.emit(run)
		return nil
	}
	ok, err := s.Runs.BeginWaiting(run.ID, run)
	if err != nil {
		return ipcerr.InternalErr(err)
	}
	if !ok {
		return errAnswered()
	}
	s.emit(run)
	s.closePrompt(run.ID)
	return nil
}

// startHeadless runs a normal script under the login shell with no terminal tab and no stdin;
// output goes to the run log. existing means run is a waiting row to turn running.
func (s *Service) startHeadless(p *planned, run Run, existing bool) (Started, error) {
	ctx, cancel := context.WithCancel(context.Background())
	br := &bgRun{cancel: cancel}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		cancel()
		return Started{}, ipcerr.T(ipcerr.CodeUnavailable, "backend.core.scriptRun.closing.body", map[string]string{"app": s.App})
	}
	if s.bg == nil {
		s.bg = map[string]*bgRun{}
	}
	s.bg[run.ID] = br
	s.bgWait.Add(1)
	s.mu.Unlock()
	abandon := func() {
		cancel()
		s.mu.Lock()
		delete(s.bg, run.ID)
		s.mu.Unlock()
		s.bgWait.Done()
	}
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
	if err := s.begin(run, existing); err != nil {
		abandon()
		return Started{}, err
	}
	timeout, timeoutText := p.timeout, p.timeoutText
	if s.ScheduleTimeout > 0 {
		timeout, timeoutText = s.ScheduleTimeout, s.ScheduleTimeout.String()
	}
	go func() {
		defer s.bgWait.Done()
		out, sink := s.runHeadless(ctx, br, p, run.ID, timeout, timeoutText)
		cancel()
		out = out.WithLastError(sink.lastStderr())
		if out.Status == runoutcome.StatusFailed {
			out = out.WithOutputTail(sink.outputTail())
		}
		done, ferr := s.Runs.FinishByRun(run.ID, maskOutcome(out, p.mask), s.now())
		if p.release != nil {
			p.release()
		}
		s.mu.Lock()
		delete(s.bg, run.ID)
		s.mu.Unlock()
		s.finish(done, ferr)
	}()
	return Started{RunID: run.ID}, nil
}

// runHeadless runs the command to its end and maps how it ended to an outcome.
func (s *Service) runHeadless(ctx context.Context, br *bgRun, p *planned, runID string, timeout time.Duration, timeoutText string) (runoutcome.Outcome, *logSink) {
	runCtx, stopTimer := context.WithTimeout(ctx, timeout)
	defer stopTimer()
	sink := newLogSink(s, runID, p.mask)
	shell, login := loginshell.ResolveShell(s.getenv, loginshell.IsExecutableFile)
	argv := loginshell.BuildArgv(shell, login, p.script.Command)
	cmd := exec.CommandContext(runCtx, argv[0], argv[1:]...)
	cmd.Dir = p.dir.Path
	cmd.Env = append(append(loginshell.ScrubGitEnv(os.Environ()), "GIT_TERMINAL_PROMPT=0"), p.envList...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	stopEscalate := procgroup.GracefulCancel(cmd, headlessGrace, procgroup.Kill)
	stdout := linewriter.New(func(l string) { sink.add("stdout", l) })
	stderr := linewriter.New(func(l string) { sink.add("stderr", l) })
	cmd.Stdout, cmd.Stderr = stdout, stderr

	proc := runoutcome.Process{Kind: runoutcome.KindScript, App: s.App, Timeout: timeoutText}
	if err := cmd.Start(); err != nil {
		sink.flush()
		proc.End, proc.Err = runoutcome.EndStartErr, err
		// A stop that landed before the process started cancelled the context, so Start fails.
		switch br.endedBy() {
		case endUser:
			proc.End, proc.Err = runoutcome.EndUser, nil
		case endQuit:
			proc.End, proc.Err = runoutcome.EndQuit, nil
		}
		return runoutcome.ForProcess(proc), sink
	}
	waitErr := cmd.Wait()
	stopEscalate()
	stdout.Flush()
	stderr.Flush()
	sink.flush()
	timedOut := errors.Is(runCtx.Err(), context.DeadlineExceeded) && ctx.Err() == nil
	if (timedOut || ctx.Err() != nil) && cmd.Process != nil {
		_ = procgroup.Kill(cmd.Process.Pid, syscall.SIGKILL)
	}
	switch br.endedBy() {
	case endUser:
		proc.End = runoutcome.EndUser
	case endQuit:
		proc.End = runoutcome.EndQuit
	case endNone:
		switch {
		case timedOut:
			proc.End = runoutcome.EndTimeout
		case cmd.ProcessState != nil:
			proc.ExitCode = cmd.ProcessState.ExitCode()
		case waitErr != nil:
			proc.End, proc.Err = runoutcome.EndStartErr, waitErr
		}
	}
	return runoutcome.ForProcess(proc), sink
}
