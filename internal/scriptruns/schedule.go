package scriptruns

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
	"github.com/kirathecat/kira-studio/internal/prompts"
	"github.com/kirathecat/kira-studio/internal/runoutcome"
	"github.com/kirathecat/kira-studio/internal/scripts"
	"github.com/kirathecat/kira-studio/packages/theme/src/i18n"
)

// scheduleArgs is the run a script's schedule describes. secrets are typed in the confirm popup and
// fill secret params only; a schedule holds none, plan fills the rest from the saved values.
func scheduleArgs(rec scripts.CustomScript, secrets map[string][]string) RunArgs {
	params := map[string][]string{}
	for k, v := range rec.Schedule.Params {
		params[k] = v
	}
	for _, p := range rec.Params {
		if v, ok := secrets[p.Name]; ok && p.Secret {
			params[p.Name] = v
		}
	}
	return RunArgs{ScriptID: rec.ID, Params: params, TaskID: rec.Schedule.TaskID, BranchID: rec.Schedule.BranchID}
}

// errText is an error's message without the wire envelope.
func errText(err error) string {
	var ie *ipcerr.Error
	if errors.As(err, &ie) {
		return ie.Message
	}
	return err.Error()
}

// scheduled loads a script that has a schedule.
func (s *Service) scheduled(scriptID string) (*scripts.CustomScript, error) {
	rec, err := s.Scripts.Get(scriptID)
	if err != nil {
		return nil, ipcerr.InternalErr(err)
	}
	if rec == nil {
		return nil, ipcerr.T(ipcerr.CodeNotFound, "automations.script.gone.error", nil)
	}
	if rec.Schedule == nil {
		return nil, ipcerr.T(ipcerr.CodeConflict, "backend.core.scriptRun.noSchedule.body", nil)
	}
	return rec, nil
}

// SchedulePreview resolves the run a script's schedule would start, as the confirm popup shows it.
func (s *Service) SchedulePreview(scriptID string, secrets map[string][]string) (Preview, error) {
	rec, err := s.scheduled(scriptID)
	if err != nil {
		return Preview{}, err
	}
	p, err := s.plan(s.lifeCtx(), scheduleArgs(*rec, secrets), planOpts{headless: true, informational: true})
	if err != nil {
		return Preview{}, err
	}
	return p.preview, nil
}

// startScheduled starts a planned scheduled run. existing is the waiting row the user accepted.
// A worktree run holds the board's claim until it ends, for either kind.
func (s *Service) startScheduled(p *planned, existing *Run) (Started, error) {
	p.trigger = TriggerScheduled
	if p.dir.Mode == scripts.DirModeWorktree {
		np, err := s.claimWorktree(p, true)
		if err != nil {
			return Started{}, err
		}
		p = np
	}
	p.timeoutText = scripts.DefaultScheduleTimeout
	p.timeout = scripts.Schedule{}.TimeoutDuration()
	if sch := p.script.Schedule; sch != nil && sch.Timeout != "" {
		p.timeout, p.timeoutText = sch.TimeoutDuration(), sch.Timeout
	}
	run := s.newRun(p, existing)
	var started Started
	var err error
	if p.script.Kind == scripts.KindSmart {
		started, err = s.launchSmart(p, run, existing != nil)
	} else {
		started, err = s.startHeadless(p, run, existing != nil)
	}
	if err != nil && p.release != nil {
		p.release()
	}
	return started, err
}

// checkStart refuses a plan that cannot start: a changed script, a blocker or missing values.
func checkStart(p *planned, hash string) error {
	if p.preview.Hash != hash {
		return ipcerr.T(ipcerr.CodeConflict, "backend.core.scriptRun.changedSincePreview.body", nil)
	}
	if p.preview.Blocker != "" {
		return ipcerr.BadRequest(p.preview.Blocker)
	}
	if len(p.preview.Missing) > 0 {
		return ipcerr.T(ipcerr.CodeBadRequest, "backend.core.scriptRun.fillIn.body", map[string]string{"names": strings.Join(p.preview.Missing, ", ")})
	}
	return nil
}

// clockText is a moment as the run reasons print it, in the schedule's zone.
func clockText(ms int64, sch *scripts.Schedule) string {
	loc := time.UTC
	if sch != nil {
		if l, err := time.LoadLocation(sch.Timezone); err == nil {
			loc = l
		}
	}
	return time.UnixMilli(ms).In(loc).Format("15:04")
}

// overlapText says why a scheduled run cannot start beside the script's active run.
func overlapText(rec scripts.CustomScript, active Run) string {
	if active.State == StateWaiting {
		return fmt.Sprintf("the previous run is still waiting for your answer (due %s)", clockText(active.CreatedAt, rec.Schedule))
	}
	started := active.CreatedAt
	if active.StartedAt != nil {
		started = *active.StartedAt
	}
	return fmt.Sprintf("the previous run is still running (started %s)", clockText(started, rec.Schedule))
}

// skip records a scheduled run that did not start, due at due.
func (s *Service) skip(rec scripts.CustomScript, due time.Time, reason string) {
	o := runoutcome.Skipped(reason, runoutcome.SourceSchedule)
	now := s.now()
	s.record(Run{
		ID: uuid.NewString(), ScriptID: rec.ID, ScriptName: rec.Name, Color: rec.Color, Kind: rec.Kind,
		Trigger: TriggerScheduled, State: string(o.Status), CreatedAt: due.UnixMilli(), FinishedAt: &now, Outcome: &o,
	})
}

// skipWaiting ends the waiting runs matched by by/value as skipped.
func (s *Service) skipWaiting(by, value, reason string, src runoutcome.Source) {
	runs, err := s.Runs.SkipWaiting(by, value, reason, src, s.now())
	if err != nil {
		slog.Warn("scriptruns: skip waiting", "scope", "scriptruns", "err", err)
		return
	}
	for _, r := range runs {
		s.emit(r)
		s.closePrompt(r.ID)
	}
	if len(runs) > 0 {
		s.purge()
	}
}

func (s *Service) openPrompt(r Run) {
	if s.Prompts != nil {
		s.Prompts.Open(prompts.Prompt{
			ID: prompts.ID(prompts.KindSchedule, r.ID), Kind: prompts.KindSchedule, Ref: r.ID,
			Title: i18n.T("backend.prompt.schedule.title", map[string]string{"name": r.ScriptName}), CreatedAt: r.CreatedAt,
		})
	}
}

func (s *Service) closePrompt(runID string) {
	if s.Prompts != nil {
		s.Prompts.Close(prompts.ID(prompts.KindSchedule, runID))
	}
}

// retire ends a script's waiting runs because its schedule or the script is gone.
func (s *Service) retire(scriptID, reason string) {
	s.skipWaiting(skipByScript, scriptID, reason, runoutcome.SourceSchedule)
}
