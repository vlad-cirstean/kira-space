package scriptruns

import (
	"crypto/rand"
	"encoding/base64"
	"time"

	"github.com/kirathecat/kira-studio/internal/ipcerr"
	"github.com/kirathecat/kira-studio/internal/scripts"
	"github.com/kirathecat/kira-studio/internal/secrets"
)

// LaunchTTL is how long a normal script's launch token stays valid; a var so a flow test can shorten it.
var LaunchTTL = 60 * time.Second

// launch is what Start resolved for a normal script, waiting for its terminal tab to open.
type launch struct {
	scriptID string
	dir      scripts.Dir
	command  string
	env      []string
	params   []RunParam
	created  time.Time
	trigger  Trigger
	ade      *RunADE
	mask     *secrets.Masker
}

func (s *Service) startTerminal(p *planned) (Started, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return Started{}, ipcerr.InternalErr(err)
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	if err := scripts.PrepareDir(p.dir); err != nil {
		return Started{}, ipcerr.BadRequest(err.Error())
	}
	now := time.UnixMilli(s.now())
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return Started{}, ipcerr.T(ipcerr.CodeUnavailable, "backend.core.scriptRun.closing.body", map[string]string{"app": s.App})
	}
	if s.launches == nil {
		s.launches = map[string]launch{}
	}
	for t, l := range s.launches {
		if now.Sub(l.created) > LaunchTTL {
			delete(s.launches, t)
		}
	}
	s.launches[token] = launch{
		scriptID: p.script.ID, dir: p.dir, command: p.script.Command, env: p.envList, params: p.params, created: now,
		trigger: runTrigger(p), ade: p.preview.ADE, mask: p.mask,
	}
	return Started{Terminal: &TerminalStart{Token: token, Cwd: p.dir.Path}}, nil
}

// takeLaunch consumes a token: it works once, for the script it was made for, within LaunchTTL.
func (s *Service) takeLaunch(token, scriptID string) (launch, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, ok := s.launches[token]
	delete(s.launches, token)
	if !ok || l.scriptID != scriptID || time.UnixMilli(s.now()).Sub(l.created) > LaunchTTL {
		return launch{}, false
	}
	return l, true
}

// runTrigger is what started the run: the caller's choice, else ade for a task, else manual.
func runTrigger(p *planned) Trigger {
	if p.trigger != "" {
		return p.trigger
	}
	if p.preview.ADE != nil {
		return TriggerADE
	}
	return TriggerManual
}

// agentLaunch is the env of a Continue in terminal launch, waiting for its terminal tab to open.
type agentLaunch struct {
	command string
	env     []string
	created time.Time
}

// newAgentLaunch stores env under a single-use token valid for LaunchTTL, for exactly command.
func (s *Service) newAgentLaunch(command string, env []string) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", ipcerr.InternalErr(err)
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	now := time.UnixMilli(s.now())
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.agentLaunches == nil {
		s.agentLaunches = map[string]agentLaunch{}
	}
	for t, l := range s.agentLaunches {
		if now.Sub(l.created) > LaunchTTL {
			delete(s.agentLaunches, t)
		}
	}
	s.agentLaunches[token] = agentLaunch{command: command, env: env, created: now}
	return token, nil
}

// takeAgentLaunch consumes a token: it works once, for the command it was made for, within LaunchTTL.
func (s *Service) takeAgentLaunch(token, command string) ([]string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, ok := s.agentLaunches[token]
	delete(s.agentLaunches, token)
	if !ok || l.command != command || time.UnixMilli(s.now()).Sub(l.created) > LaunchTTL {
		return nil, false
	}
	return l.env, true
}
