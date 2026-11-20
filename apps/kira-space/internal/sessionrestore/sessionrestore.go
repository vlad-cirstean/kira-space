// Package sessionrestore joins the three restart offers (script runs, Docker containers, ADE
// agents) behind one dialog at launch. Each module keeps its own offer and banner; this package
// reads them together, restores a selection across them and hands out the one auto-open claim.
package sessionrestore

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/ade"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/internal/docker"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
	"github.com/kirathecat/kira-studio/internal/scriptruns"
)

// DefaultOfferWindow is how long after start the dialog may still open itself, so a Docker engine
// that comes up after Kira (and is then seen restarting) still gets offered.
const DefaultOfferWindow = 2 * time.Minute

// Scripts is the script runs offer.
type Scripts interface {
	RestoreState() (scriptruns.RestoreState, error)
	Restore(scriptruns.RestoreArgs) (scriptruns.RestoreResult, error)
}

// Docker is the Docker restore tracker. Current never blocks on the engine.
type Docker interface {
	Current() docker.RestoreState
	Restore(context.Context, docker.RestoreArgs) (docker.RestoreResult, error)
}

// Agents is the ADE agent offer.
type Agents interface {
	AgentRestoreState() []ade.AgentItem
	RestoreAgent(context.Context, ade.AgentChoice) ade.AgentOutcome
	CloseAgentOffer()
}

// Options tunes a Service; the zero value is production's.
type Options struct {
	// OfferWindow replaces DefaultOfferWindow.
	OfferWindow time.Duration
	// Now replaces time.Now.
	Now func() time.Time
	// MapError turns an agent restore failure into the error a person reads; nil keeps ipcerr.Wrap.
	MapError func(error) error
}

// Service is the aggregator.
type Service struct {
	scripts Scripts
	docker  Docker
	agents  Agents
	opts    Options
	started time.Time

	mu      sync.Mutex
	claimed bool
}

// New starts the offer window now: build it at process start, after the modules recovered.
func New(scripts Scripts, dock Docker, agents Agents, opts Options) *Service {
	if opts.OfferWindow == 0 {
		opts.OfferWindow = DefaultOfferWindow
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.MapError == nil {
		opts.MapError = ipcerr.Wrap
	}
	return &Service{scripts: scripts, docker: dock, agents: agents, opts: opts, started: opts.Now()}
}

// State is everything the dialog can offer. Open says the dialog may still open itself.
type State struct {
	Open    bool                     `json:"open"`
	Scripts []scriptruns.RestoreItem `json:"scripts"`
	Docker  docker.RestoreState      `json:"docker"`
	Agents  []ade.AgentItem          `json:"agents"`
}

func (s *Service) open() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return !s.claimed && s.opts.Now().Sub(s.started) < s.opts.OfferWindow
}

// State reads the three offers. Docker comes from the tracker's last tick, never from a live probe.
func (s *Service) State() (State, error) {
	scripts, err := s.scripts.RestoreState()
	if err != nil {
		return State{}, err
	}
	st := State{Open: s.open(), Scripts: scripts.Items, Docker: s.docker.Current(), Agents: s.agents.AgentRestoreState()}
	if st.Scripts == nil {
		st.Scripts = []scriptruns.RestoreItem{}
	}
	if st.Docker.Items == nil {
		st.Docker.Items = []docker.RestoreItem{}
	}
	if st.Agents == nil {
		st.Agents = []ade.AgentItem{}
	}
	return st, nil
}

// Claim answers true once per process, and only while the dialog may still open itself: the window
// that wins shows it, every other window stays quiet.
func (s *Service) Claim() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.claimed || s.opts.Now().Sub(s.started) >= s.opts.OfferWindow {
		return false
	}
	s.claimed = true
	return true
}

// Close ends the auto-open and drops the agent offer. The script and Docker offers stay for their banners.
func (s *Service) Close() {
	s.mu.Lock()
	s.claimed = true
	s.mu.Unlock()
	s.agents.CloseAgentOffer()
}

// DockerChoice names the containers of the pending engine to start.
type DockerChoice struct {
	EngineID string   `json:"engineId"`
	IDs      []string `json:"ids"`
}

// Args is the selection: script runs with their plan hashes, Docker containers, agents with Continue.
type Args struct {
	Scripts []scriptruns.RestoreChoice `json:"scripts"`
	Docker  *DockerChoice              `json:"docker"`
	Agents  []ade.AgentChoice          `json:"agents"`
}

// AgentResult is one agent's outcome: a TUI launch to open, or the run queued in the background.
type AgentResult struct {
	ID     string          `json:"id"`
	Launch *adewire.Launch `json:"launch"`
	RunID  string          `json:"runId"`
	Code   string          `json:"code"`
	Error  string          `json:"error"`
}

// Result mirrors Args: one outcome per chosen item.
type Result struct {
	Scripts []scriptruns.RestoreOutcome `json:"scripts"`
	Docker  *docker.RestoreResult       `json:"docker"`
	Agents  []AgentResult               `json:"agents"`
}

func codeOf(err error) (code, message string) {
	var ie *ipcerr.Error
	if errors.As(ipcerr.Wrap(err), &ie) {
		return ie.Code, ie.Message
	}
	return string(ipcerr.CodeInternal), err.Error()
}

// Restore starts the selection. Scripts go first and synchronously, so smart runs and headless
// scripts are running before an agent starts; Docker and agents then run side by side, so an agent
// never waits on a Compose health check. Each item succeeds or fails alone.
func (s *Service) Restore(ctx context.Context, args Args) (Result, error) {
	res := Result{Scripts: []scriptruns.RestoreOutcome{}, Agents: []AgentResult{}}
	if err := s.restoreScripts(args.Scripts, &res); err != nil {
		return Result{}, err
	}
	var wg sync.WaitGroup
	if args.Docker != nil && len(args.Docker.IDs) > 0 {
		wg.Go(func() { res.Docker = s.restoreDocker(ctx, *args.Docker) })
	}
	res.Agents = s.restoreAgents(ctx, args.Agents)
	wg.Wait()
	return res, nil
}

func (s *Service) restoreScripts(choices []scriptruns.RestoreChoice, res *Result) error {
	if len(choices) == 0 {
		return nil
	}
	state, err := s.scripts.RestoreState()
	if err != nil {
		return err
	}
	offered := map[string]bool{}
	for _, it := range state.Items {
		offered[it.RunID] = true
	}
	// A stale id fails its own item, not the call (scriptruns refuses the whole call for one).
	var valid []scriptruns.RestoreChoice
	stale := map[string]scriptruns.RestoreOutcome{}
	for _, c := range choices {
		if offered[c.RunID] {
			valid = append(valid, c)
			continue
		}
		code, msg := codeOf(ipcerr.T(ipcerr.CodeBadRequest, "backend.core.scriptRun.restoreNotOffered.body", nil))
		stale[c.RunID] = scriptruns.RestoreOutcome{RunID: c.RunID, Code: code, Error: msg}
	}
	done := map[string]scriptruns.RestoreOutcome{}
	if len(valid) > 0 {
		out, err := s.scripts.Restore(scriptruns.RestoreArgs{Items: valid})
		if err != nil {
			return err
		}
		for _, o := range out.Items {
			done[o.RunID] = o
		}
	}
	for _, c := range choices {
		if o, ok := done[c.RunID]; ok {
			res.Scripts = append(res.Scripts, o)
		} else if o, ok := stale[c.RunID]; ok {
			res.Scripts = append(res.Scripts, o)
		}
	}
	return nil
}

func (s *Service) restoreDocker(ctx context.Context, c DockerChoice) *docker.RestoreResult {
	out, err := s.docker.Restore(ctx, docker.RestoreArgs{EngineID: c.EngineID, IDs: c.IDs})
	if err != nil {
		_, msg := codeOf(err)
		failed := docker.RestoreResult{Items: make([]docker.RestoreResultItem, 0, len(c.IDs))}
		for _, id := range c.IDs {
			failed.Items = append(failed.Items, docker.RestoreResultItem{ID: id, Error: msg})
		}
		return &failed
	}
	return &out
}

func (s *Service) restoreAgents(ctx context.Context, choices []ade.AgentChoice) []AgentResult {
	out := make([]AgentResult, 0, len(choices))
	if len(choices) == 0 {
		return out
	}
	offered := map[string]ade.AgentItem{}
	for _, it := range s.agents.AgentRestoreState() {
		offered[it.ID] = it
	}
	for _, c := range choices {
		r := AgentResult{ID: c.ID}
		it, ok := offered[c.ID]
		switch {
		case !ok:
			r.Code, r.Error = codeOf(ipcerr.T(ipcerr.CodeBadRequest, "backend.space.ade.restore.notOffered.body", nil))
		case it.State == ade.AgentUnavailable:
			r.Code, r.Error = string(ipcerr.CodeBadRequest), it.Reason
		default:
			o := s.agents.RestoreAgent(ctx, c)
			r.Launch, r.RunID = o.Launch, o.RunID
			if o.Err != nil {
				r.Code, r.Error = codeOf(s.opts.MapError(o.Err))
			}
		}
		out = append(out, r)
	}
	return out
}

type dockerAdapter struct{ b *docker.BoundService }

// DockerOf adapts the bound Docker service: Current reads the tracker without ticking.
func DockerOf(b *docker.BoundService) Docker { return dockerAdapter{b} }

func (d dockerAdapter) Current() docker.RestoreState { return docker.RestoreCurrentBound(d.b) }

func (d dockerAdapter) Restore(_ context.Context, args docker.RestoreArgs) (docker.RestoreResult, error) {
	return d.b.RestoreContainers(args)
}
