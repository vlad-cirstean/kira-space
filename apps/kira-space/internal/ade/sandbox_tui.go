package ade

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/repos"
	"github.com/kirathecat/kira-studio/internal/agentsandbox"
	"github.com/kirathecat/kira-studio/internal/harness"
	"github.com/kirathecat/kira-studio/internal/llmprompts"
)

// sandbox_tui.go confines the interactive session of a manual step (P511). A session has no run row:
// its policy lives in ade_session_sandbox, keyed by the session record. Access off applies nothing
// and keeps no row.

// tuiSandboxIn is where a manual session works: the folders it writes, the read-only repo roots
// next to them, its cwd and the branch each worktree may advance.
type tuiSandboxIn struct {
	worktrees, read []string
	runDir          string
	branch          map[string]string
}

// tuiSandboxPlan is a manual step's sandbox request for one launch.
type tuiSandboxPlan struct {
	sd adewire.StepSandbox
	in tuiSandboxIn
}

// launchFiles are the Kira-written files the agent reads at start.
type launchFiles struct {
	read        []string
	isolation   string
	contextFile string
}

// liveTUISandbox is the sandbox of a running (or starting) interactive session.
type liveTUISandbox struct {
	sr      *sandboxRun
	stop    func()
	started bool
}

// manualSandboxPlan builds the plan of a manual session step from the folders its launch resolved.
// lines and cwd come from launchDirs; folder is the task folder when the session works outside the
// repos or the task has none. It returns nil when the step's sandbox is off.
func (b *TaskBoard) manualSandboxPlan(sd adewire.StepSandbox, tc *taskCtx, lines []repoLine, cwd, folder string) (*tuiSandboxPlan, error) {
	sd = sd.NormalizedFor(adewire.KindManual)
	if agentsandbox.Access(sd.Access) == agentsandbox.AccessOff {
		return nil, nil
	}
	in := tuiSandboxIn{runDir: cwd, branch: map[string]string{}}
	if folder != "" {
		in.worktrees, in.runDir = []string{folder}, folder
	}
	for _, l := range lines {
		switch {
		case l.Worktree != "":
			in.worktrees = append(in.worktrees, l.Worktree)
			if l.Branch != "" {
				in.branch[l.Worktree] = l.Branch
			}
		case l.ReadOnlyRoot != "":
			in.read = append(in.read, l.ReadOnlyRoot)
		}
	}
	if len(in.worktrees) == 0 { // only read-only repo roots: the session writes the task folder alone
		dir, err := b.taskDir(tc.task.ID)
		if err != nil {
			return nil, err
		}
		in.worktrees = []string{dir}
	}
	return &tuiSandboxPlan{sd: sd, in: in}, nil
}

// resumeSandboxPlan is manualSandboxPlan for a Take over of a manual step session: the folders are
// resolved afresh and the session keeps its cwd.
func (b *TaskBoard) resumeSandboxPlan(ctx context.Context, tc *taskCtx, rec *model.AdeSession, cwd string) (*tuiSandboxPlan, error) {
	if rec.Mode != model.AdeSessionModeTUI || rec.StepID == "" || tc.wf == nil {
		return nil, nil
	}
	var step *adewire.WorkflowStep
	for i := range tc.wf.Steps {
		if tc.wf.Steps[i].ID == rec.StepID {
			step = &tc.wf.Steps[i]
		}
	}
	if step == nil || step.Kind != adewire.KindManual || !step.Session || step.ID == reviewStepID {
		return nil, nil
	}
	if agentsandbox.Access(step.Sandbox.NormalizedFor(step.Kind).Access) == agentsandbox.AccessOff {
		return nil, nil
	}
	var lines []repoLine
	var err error
	if !step.OutsideRepo {
		lines, _, _, err = b.launchDirs(ctx, tc)
	}
	folder := ""
	if step.OutsideRepo || errors.Is(err, errNoRepo) {
		lines = nil
		if folder, err = b.taskDir(tc.task.ID); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}
	return b.manualSandboxPlan(step.Sandbox, tc, lines, cwd, folder)
}

// prepareTUISandbox resolves and prepares the sandbox of a manual session. It returns nil when the
// board has no enforcer or the access is off: before any policy, wrapper or row exists.
func (b *TaskBoard) prepareTUISandbox(ctx context.Context, recordID string, agent harness.Provider, plan *tuiSandboxPlan, files launchFiles) (*sandboxRun, error) {
	if b.deps.Sandbox == nil || plan == nil {
		return nil, nil
	}
	sd := plan.sd.NormalizedFor(adewire.KindManual)
	if agentsandbox.Access(sd.Access) == agentsandbox.AccessOff {
		return nil, nil
	}
	in := agentsandbox.Input{
		RunID: recordID, Access: agentsandbox.Access(sd.Access), Read: sd.Read, Allow: slices.Clone(sd.Allow), Network: sd.Network,
		Worktrees: plan.in.worktrees, ReadWorktrees: plan.in.read, RunDir: plan.in.runDir, Branch: plan.in.branch,
		Scratch: filepath.Join(b.deps.AgentDir, "sandbox", recordID), Cache: filepath.Join(b.deps.AgentDir, "sandbox-cache"),
		Ports: []int{b.agent.Port()}, ReadFiles: files.read,
	}
	if files.isolation != "" {
		in.Allow = append(in.Allow, files.isolation)
	}
	if files.contextFile != "" {
		in.ContextDir = filepath.Dir(files.contextFile)
	}
	rt := agentsandbox.DiscoverRuntime(ctx, agent, b.deps.AgentBin)
	pol, notes, err := agentsandbox.ResolvePolicy(in, rt)
	if err != nil {
		return nil, err
	}
	prep, err := b.deps.Sandbox.Prepare(pol, rt)
	if err != nil {
		_ = os.RemoveAll(pol.Scratch)
		return nil, err
	}
	sr := &sandboxRun{access: agentsandbox.Access(sd.Access), prep: prep, policy: pol, notes: notes, agent: agent, scratch: pol.Scratch}
	sr.release = func() { b.deps.Sandbox.Release(recordID) }
	return sr, nil
}

// applyTUISandbox puts the prepared sandbox on the launch: wrapper, permission flags, env where no
// wrapper carries it, and the note on a new session's first message.
func (b *TaskBoard) applyTUISandbox(pa *PrepareArgs, sr *sandboxRun, sd adewire.StepSandbox) {
	pa.Sandbox = sr.prep.Exec
	if sr.prep.Exec == nil {
		pa.Env = append(pa.Env, sr.prep.Env...)
	}
	for _, d := range sr.prep.AddDirs {
		if !slices.Contains(pa.AddDirs, d) {
			pa.AddDirs = append(pa.AddDirs, d)
		}
	}
	pa.DisallowedTools = append(slices.Clone(pa.DisallowedTools), sr.prep.Disallowed...)
	if pa.Resume == "" && pa.Resumes == "" && pa.Message != "" {
		note := llmprompts.AdeManualSandboxNote.Text(b.prompts(), llmprompts.SandboxNoteVars{
			Access: string(sr.access), Worktree: strings.Join(sr.policy.Write, ", "), Read: sd.Read, Network: sr.policy.Network,
			Enforced: b.deps.Sandbox.Support().Enforced,
		})
		pa.Message = strings.TrimRight(pa.Message, "\n") + "\n\n" + note
	}
}

// holdTUISandbox registers the live sandbox of a record and returns its release.
func (b *TaskBoard) holdTUISandbox(recordID string, sr *sandboxRun) func() {
	live := &liveTUISandbox{sr: sr}
	b.runMu.Lock()
	prev := b.tuiSandbox[recordID]
	b.tuiSandbox[recordID] = live
	b.runMu.Unlock()
	if prev != nil {
		b.endTUISandbox(recordID, prev)
	}
	return func() { b.endTUISandbox(recordID, live) }
}

// endTUISandbox stops the watch and drops the sandbox of a record, unless a newer one replaced it.
func (b *TaskBoard) endTUISandbox(recordID string, live *liveTUISandbox) {
	b.runMu.Lock()
	if b.tuiSandbox[recordID] == live {
		delete(b.tuiSandbox, recordID)
	}
	stop := live.stop
	live.stop = nil
	b.runMu.Unlock()
	if stop != nil {
		stop()
	}
	live.sr.cleanup()
}

// OnTUIAgentStarted is Tracker's agent-started target: the session row exists and the shell runs,
// so the sandbox row is stored and the denial watch follows the shell's process tree.
func (b *TaskBoard) OnTUIAgentStarted(recordID string, pid int) {
	b.runMu.Lock()
	live := b.tuiSandbox[recordID]
	if live == nil || live.started {
		b.runMu.Unlock()
		return
	}
	live.started = true
	b.runMu.Unlock()
	sup := b.deps.Sandbox.Support()
	if err := b.storeSessionSandbox(recordID, live.sr, sup.Enforced, sup.Reason); err != nil {
		slog.Warn("ade: store session sandbox", "scope", "ade", "session", recordID, "err", err)
		return
	}
	b.notifySessions()
	stop := b.watchDenials(recordID, live.sr, pid,
		func(n int, listJSON string) { b.countSessionDenials(recordID, n, listJSON) },
		func(agentsandbox.Denial) {},
		b.notifySessions)
	b.runMu.Lock()
	if b.tuiSandbox[recordID] == live {
		live.stop = stop
		stop = nil
	}
	b.runMu.Unlock()
	if stop != nil { // the session ended while the watch started
		stop()
	}
}

func (b *TaskBoard) storeSessionSandbox(sessionID string, sr *sandboxRun, enforced bool, reason string) error {
	if b.deps.SessionSandbox == nil {
		return nil
	}
	raw, err := json.Marshal(sr.policy)
	if err != nil {
		return err
	}
	return b.deps.SessionSandbox.Upsert(repos.AdeSessionSandbox{
		SessionID: sessionID, Access: string(sr.access), Enforced: enforced, Reason: reason,
		PolicyJSON: string(raw), Profile: sr.prep.Profile,
	})
}

func (b *TaskBoard) countSessionDenials(sessionID string, delta int, listJSON string) {
	if b.deps.SessionSandbox == nil || delta == 0 {
		return
	}
	if err := b.deps.SessionSandbox.AddDenials(sessionID, delta, listJSON); err != nil {
		slog.Warn("ade: count session sandbox denial", "scope", "ade", "session", sessionID, "err", err)
	}
}

// badgeSessions sets the sandbox badge of every session in out with one query.
func (b *TaskBoard) badgeSessions(out []adewire.Session) {
	if b.deps.SessionSandbox == nil {
		return
	}
	var ids []string
	for _, s := range out {
		if s.Mode == model.AdeSessionModeTUI && s.StepID != "" {
			ids = append(ids, s.ID)
		}
	}
	rows, err := b.deps.SessionSandbox.Badges(ids)
	if err != nil {
		slog.Warn("ade: session sandbox badges", "scope", "ade", "err", err)
		return
	}
	for i := range out {
		if row, ok := rows[out[i].ID]; ok {
			out[i].Sandbox = badgeFrom(row.Access, row.Enforced, row.DenialCount, row.DenialsJSON)
		}
	}
}

// SessionSandbox returns the stored sandbox of a manual step session: its policy, profile and denials.
func (b *TaskBoard) SessionSandbox(_ context.Context, sessionID string) (adewire.RunSandboxDetail, error) {
	if b.deps.SessionSandbox == nil {
		return adewire.RunSandboxDetail{}, invalid("session %s has no sandbox", sessionID)
	}
	row, err := b.deps.SessionSandbox.Get(sessionID)
	if err != nil {
		return adewire.RunSandboxDetail{}, err
	}
	if row == nil {
		return adewire.RunSandboxDetail{}, invalid("session %s has no sandbox", sessionID)
	}
	reason := ""
	if row.Reason != "" {
		rec, err := b.deps.Sessions.Get(sessionID)
		if err != nil {
			return adewire.RunSandboxDetail{}, err
		}
		provider := b.deps.Agent.Active()
		if rec != nil {
			if p, err := b.deps.Agent.For(rec.Provider); err == nil {
				provider = p
			}
		}
		reason = agentsandbox.NotEnforcedText(provider)
	}
	return sandboxDetail(row.Access, row.Enforced, reason, row.Profile, row.PolicyJSON, row.DenialsJSON, row.DenialCount), nil
}

// sandboxTUI confines a manual session launch when its plan asks for it: it prepares the sandbox from
// the launch's own files and applies it to pa. A resume that now runs unconfined drops its old row.
func (b *TaskBoard) sandboxTUI(ctx context.Context, pa *PrepareArgs, recordID string, agent harness.Provider, plan *tuiSandboxPlan, app *harness.AppConfig, mcpConfig string) (*sandboxRun, error) {
	files := launchFiles{read: []string{mcpConfig}, isolation: pa.IsolationDir}
	if app != nil {
		for _, f := range app.Files {
			files.read = append(files.read, f)
		}
	}
	if plan != nil && b.deps.ContextFile != nil {
		var err error
		if files.contextFile, err = b.deps.ContextFile(); err != nil {
			return nil, err
		}
		pa.ContextFile = files.contextFile
	}
	sr, err := b.prepareTUISandbox(ctx, recordID, agent, plan, files)
	if err != nil {
		return nil, err
	}
	if sr == nil {
		if pa.Resume != "" && b.deps.SessionSandbox != nil {
			if err := b.deps.SessionSandbox.Delete(recordID); err != nil {
				return nil, err
			}
		}
		return nil, nil
	}
	b.applyTUISandbox(pa, sr, plan.sd)
	return sr, nil
}
