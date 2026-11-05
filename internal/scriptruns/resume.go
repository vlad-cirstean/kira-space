package scriptruns

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/kirathecat/kira-studio/internal/agentcfg"
	"github.com/kirathecat/kira-studio/internal/harness"
	"github.com/kirathecat/kira-studio/internal/ids"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
	"github.com/kirathecat/kira-studio/internal/llmprompts"
	"github.com/kirathecat/kira-studio/internal/scripts"
)

// Resume continues the Claude session of a smart run a restart ended, as a new run. The script is
// planned again from its current definition. A secret param takes the script's saved value, which
// may differ from the value typed for the original run; without one the run cannot resume.
func (s *Service) Resume(id string) (Started, error) { return s.resume(id, "", false) }

// resume is Resume; checkHash also refuses a run whose plan differs from the hash the user saw.
func (s *Service) resume(id, hash string, checkHash bool) (Started, error) {
	run, err := s.Runs.Get(id)
	if err != nil {
		return Started{}, err
	}
	notResumable := ipcerr.T(ipcerr.CodeBadRequest, "backend.core.scriptRun.resumeNotResumable.body", map[string]string{"app": s.App})
	if run.Kind != KindSmart || !run.Resumable {
		return Started{}, notResumable
	}
	prov, err := s.Agent.For(run.Provider)
	if err != nil {
		return Started{}, providerGone(err)
	}
	rec, err := s.Scripts.Get(run.ScriptID)
	if err != nil {
		return Started{}, ipcerr.InternalErr(err)
	}
	if rec == nil {
		return Started{}, ipcerr.T(ipcerr.CodeNotFound, "automations.script.gone.error", nil)
	}
	args, names := resumeArgs(run, *rec)
	p, err := s.plan(s.lifeCtx(), args, planOpts{provider: prov.ID()})
	if err != nil {
		return Started{}, err
	}
	if checkHash && p.preview.Hash != hash {
		return Started{}, ipcerr.T(ipcerr.CodeConflict, "backend.core.scriptRun.changedSincePreview.body", nil)
	}
	names = uniqueNames(names, p.preview.Missing)
	if len(names) > 0 {
		return Started{}, ipcerr.T(ipcerr.CodeBadRequest, "backend.core.scriptRun.resumeMissing.body", map[string]string{"names": strings.Join(names, ", ")})
	}
	if p.preview.Blocker != "" {
		return Started{}, ipcerr.BadRequest(p.preview.Blocker)
	}
	if p.dir.Mode == scripts.DirModeWorktree {
		if p, err = s.claimWorktree(p, true); err != nil {
			return Started{}, err
		}
	}
	sessionID, ok, err := s.Runs.TakeResume(id)
	if err != nil || !ok {
		if p.release != nil {
			p.release()
		}
		if err != nil {
			return Started{}, ipcerr.InternalErr(err)
		}
		return Started{}, notResumable
	}
	p.resume, p.resumedFrom = sessionID, id
	p.sent = llmprompts.RunResumeLine.Text(s.llmPrompts(), llmprompts.ResumeVars{App: s.App}) + "\n\n" + p.preview.Suffix
	started, err := s.startSmart(p)
	if err != nil {
		if rerr := s.Runs.RestoreResume(id, sessionID); rerr != nil {
			slog.Warn("scriptruns: restore resume", "scope", "scriptruns", "run", id, "err", rerr)
		}
		if p.release != nil {
			p.release()
		}
	}
	return started, err
}

// resumeArgs rebuilds a run's args from its stored params. A secret param is left out when the
// script has a saved value (plan fills it); it is named when the run had a value and none is saved.
func resumeArgs(run Run, rec scripts.CustomScript) (RunArgs, []string) {
	args := RunArgs{ScriptID: run.ScriptID, TaskID: run.TaskID, BranchID: run.BranchID, Params: map[string][]string{}}
	var secrets []string
	for _, rp := range run.Params {
		var def *scripts.Param
		for i := range rec.Params {
			if rec.Params[i].Name == rp.Name {
				def = &rec.Params[i]
			}
		}
		switch {
		case def == nil:
		case def.Secret && slices.Contains(rec.SavedSecrets, rp.Name):
		case def.Secret || rp.Value == scripts.SecretMask:
			secrets = append(secrets, rp.Name)
		case rp.Value == "":
		case def.Type == scripts.ParamText:
			args.Params[rp.Name] = []string{rp.Value}
		default:
			args.Params[rp.Name] = strings.Split(rp.Value, "\n")
		}
	}
	return args, secrets
}

// uniqueNames joins name lists, keeping first occurrences.
func uniqueNames(lists ...[]string) []string {
	var out []string
	for _, l := range lists {
		for _, n := range l {
			if !slices.Contains(out, n) {
				out = append(out, n)
			}
		}
	}
	return out
}

// ResumeLaunch is how to continue a smart run's saved conversation in a terminal. Token is "" when
// the launch needs no env; else it is the single-use token the terminal open sends back.
type ResumeLaunch struct {
	Command string `json:"command"`
	Token   string `json:"token"`
}

// ResumeCommand is the terminal launch that continues a smart run's saved conversation.
func (s *Service) ResumeCommand(id string) (ResumeLaunch, error) {
	run, err := s.Runs.Get(id)
	if err != nil {
		return ResumeLaunch{}, err
	}
	if run.Kind != KindSmart {
		return ResumeLaunch{}, ipcerr.T(ipcerr.CodeBadRequest, "backend.core.scriptRun.resumeCommandNone.body", nil)
	}
	if run.SessionID == "" {
		return ResumeLaunch{}, ipcerr.T(ipcerr.CodeBadRequest, "backend.core.scriptRun.sessionIdUnknown.body", nil)
	}
	prov, err := s.Agent.For(run.Provider)
	if err != nil {
		return ResumeLaunch{}, providerGone(err)
	}
	contextFile, err := s.contextFile()
	if err != nil {
		return ResumeLaunch{}, ipcerr.InternalErr(err)
	}
	stem := ids.New(ids.TUI)
	prep, err := s.prepareApp(prov, stem)
	if err != nil {
		return ResumeLaunch{}, ipcerr.BadRequest(err.Error())
	}
	spec := harness.InteractiveSpec{SessionID: run.SessionID, Resume: true, ContextFile: contextFile, App: prep.App, Hermetic: s.hermetic()}
	release := prep.Release
	if spec.Hermetic {
		var releaseIso func()
		spec.IsolationDir, releaseIso, err = harness.PrepareIsolation(prov, filepath.Join(s.Home, "automations-mcp"), stem, os.Environ())
		if err != nil {
			prep.Release()
			return ResumeLaunch{}, ipcerr.InternalErr(err)
		}
		release = func() { prep.Release(); releaseIso() }
	}
	l := prov.InteractiveCommand(spec)
	out := ResumeLaunch{Command: l.Command}
	if len(l.Env) > 0 || prep.App != nil || spec.IsolationDir != "" {
		if out.Token, err = s.newAgentLaunch(l.Command, l.Env, release); err != nil {
			release()
			return ResumeLaunch{}, err
		}
	} else {
		release()
	}
	return out, nil
}

// prepareApp prepares the app's agent settings for a Continue in terminal launch; none without a preparer.
// A failure names the reference that did not resolve, never a value.
func (s *Service) prepareApp(prov harness.Provider, stem string) (agentcfg.Prepared, error) {
	if s.AgentConfig == nil {
		return agentcfg.Prepared{Release: func() {}}, nil
	}
	return s.AgentConfig(context.Background(), agentcfg.Launch{
		Provider: prov, Kind: harness.LaunchInteractive, Dir: filepath.Join(s.Home, "automations-mcp"), Stem: stem,
	})
}

// providerGone maps an unregistered stored provider to its user-facing error.
func providerGone(err error) error {
	if errors.Is(err, harness.ErrProviderGone) {
		return ipcerr.T(ipcerr.CodeConflict, "backend.core.agent.providerGone.body", nil)
	}
	return ipcerr.InternalErr(err)
}

// agentFor is the provider a plan runs under: the one named by id, else the active one.
func (s *Service) agentFor(id string) (harness.Provider, error) {
	if id == "" {
		return s.Agent.Active(), nil
	}
	return s.Agent.For(id)
}

// contextFile is the agent context path of a launch, "" for none.
func (s *Service) contextFile() (string, error) {
	if s.ContextFile == nil {
		return "", nil
	}
	return s.ContextFile()
}
