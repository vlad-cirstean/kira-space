package ade

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitsession"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/repos"
	"github.com/kirathecat/kira-studio/internal/agentmcp"
	"github.com/kirathecat/kira-studio/internal/agentsandbox"
	"github.com/kirathecat/kira-studio/internal/harness"
	"github.com/kirathecat/kira-studio/internal/llmprompts"
)

// sandbox.go confines headless agents (P287): it resolves each run's policy, applies the prepared
// sandbox to the process spec, records the denials and serves the sandbox git tools.

// sandboxRun is the live sandbox of one run.
type sandboxRun struct {
	access  agentsandbox.Access
	prep    agentsandbox.Prepared
	policy  agentsandbox.Policy
	tracker agentsandbox.Tracker
	agent   agentsandbox.Namer
	grant   *agentmcp.SandboxGrant
	scratch string
	notes   []string
	release func()
}

// sandboxTarget is what the sandbox git tools act on: the run's own repo, branch and worktree.
type sandboxTarget struct {
	codeRepoID, branch, worktree string
}

// sandboxGrantFor is the tool grant of a run: nil when no sandbox git tool applies (rebase runs keep
// their `git` tool, access off runs are unconfined).
func sandboxGrantFor(rebase bool, access agentsandbox.Access) *agentmcp.SandboxGrant {
	if rebase || access == agentsandbox.AccessOff {
		return nil
	}
	return &agentmcp.SandboxGrant{Push: access == agentsandbox.AccessWrite}
}

// sandboxInput builds the policy input of a launch.
func (b *TaskBoard) sandboxInput(run model.AdeRun, l agentLaunch, sd adewire.StepSandbox) (agentsandbox.Input, error) {
	in := agentsandbox.Input{
		RunID: run.ID, Access: agentsandbox.Access(sd.Access), Read: sd.Read, Allow: sd.Allow, Network: sd.Network,
		Scratch: filepath.Join(b.deps.AgentDir, "sandbox", run.ID), Cache: filepath.Join(b.deps.AgentDir, "sandbox-cache"),
		ConfigDir: b.deps.AgentDir,
		Ports:     []int{b.agent.Port()}, RunDir: l.path,
	}
	if l.contextFile != "" {
		in.ContextDir = filepath.Dir(l.contextFile)
	}
	if run.Purpose == model.AdeRunPurposeRebase {
		spec, err := specOf(run)
		if err != nil {
			return in, err
		}
		in.Access, in.Network = agentsandbox.AccessWrite, true
		in.Worktrees = rebaseWorktrees(spec)
		in.Branch = map[string]string{}
		for _, st := range spec.Stack {
			if st.Worktree != "" {
				in.Branch[st.Worktree] = st.Name
			}
		}
		return in, nil
	}
	var sb model.AdeTaskBranch
	if run.BranchID != taskTarget {
		var err error
		if sb, err = b.deps.Tasks.GetBranch(run.BranchID); err != nil {
			return in, err
		}
		in.Branch = map[string]string{l.worktree: sb.Name}
	}
	if run.BranchID != taskTarget && sb.Name == "" {
		allow, err := b.requestedWorktreeDirs(sb.TaskID)
		if err != nil {
			return in, err
		}
		in.Allow = append(slices.Clone(in.Allow), allow...)
	}
	switch {
	case l.path == l.worktree:
		in.Worktrees = []string{l.worktree}
	case in.Access == agentsandbox.AccessWrite: // a smart step writes its own folder and reads the task worktree
		in.Worktrees, in.ReadWorktrees = []string{l.path}, []string{l.worktree}
	default:
		in.Worktrees, in.Read = []string{l.worktree}, append(slices.Clone(in.Read), l.path)
	}
	return in, nil
}

// requestedWorktreeDirs are the per-repo folders new worktrees land in, for every unnamed branch of the
// task. A run that starts at a repo root asks for its branches later through request_branches and must
// reach the worktrees it gets; the folders are created so the policy can resolve them.
func (b *TaskBoard) requestedWorktreeDirs(taskID string) ([]string, error) {
	rows, err := b.deps.Tasks.BranchesOfTask(taskID)
	if err != nil {
		return nil, err
	}
	var dirs []string
	for _, row := range rows {
		if row.Kind != model.AdeBranchKindMine || row.Name != "" {
			continue
		}
		rec, err := b.deps.CodeRepos.Get(row.CodeRepoID)
		if err != nil {
			return nil, err
		}
		if rec == nil {
			continue
		}
		dir := filepath.Join(b.deps.HomeDir, "wt", safeSegment(rec.Name))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
		if !slices.Contains(dirs, dir) {
			dirs = append(dirs, dir)
		}
	}
	return dirs, nil
}

// prepareSandbox resolves and prepares the run's sandbox and stores its row. It returns nil when the
// board has no enforcer. Access off stores a row and applies nothing.
func (b *TaskBoard) prepareSandbox(ctx context.Context, run model.AdeRun, l agentLaunch) (*sandboxRun, error) {
	if b.deps.Sandbox == nil {
		return nil, nil
	}
	sd := l.def.Sandbox.Normalized()
	rebase := run.Purpose == model.AdeRunPurposeRebase
	if rebase {
		sd = adewire.DefaultSandbox()
	}
	agent := l.agent
	sr := &sandboxRun{access: agentsandbox.Access(sd.Access), agent: agent}
	if sr.access == agentsandbox.AccessOff {
		return sr, b.storeSandbox(run.ID, sr, false, "")
	}
	in, err := b.sandboxInput(run, l, sd)
	if err != nil {
		return nil, err
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
	sr.policy, sr.prep, sr.notes, sr.scratch = pol, prep, notes, pol.Scratch
	sr.release = func() { b.deps.Sandbox.Release(run.ID) }
	sr.grant = sandboxGrantFor(rebase, sr.access)
	if sr.grant != nil && run.BranchID != taskTarget { // no repo to act on
		sb, err := b.deps.Tasks.GetBranch(run.BranchID)
		if err != nil {
			sr.cleanup()
			return nil, err
		}
		b.sandboxTargets.Store(run.ID, sandboxTarget{codeRepoID: sb.CodeRepoID, branch: sb.Name, worktree: l.worktree})
	}
	sup := b.deps.Sandbox.Support()
	if err := b.storeSandbox(run.ID, sr, sup.Enforced, sup.Reason); err != nil {
		sr.cleanup()
		return nil, err
	}
	return sr, nil
}

func (b *TaskBoard) storeSandbox(runID string, sr *sandboxRun, enforced bool, reason string) error {
	if b.deps.RunSandbox == nil {
		return nil
	}
	policy, profile := "{}", ""
	if sr.access != agentsandbox.AccessOff {
		raw, err := json.Marshal(sr.policy)
		if err != nil {
			return err
		}
		policy, profile = string(raw), sr.prep.Profile
	}
	return b.deps.RunSandbox.Insert(repos.AdeRunSandbox{
		RunID: runID, Access: string(sr.access), Enforced: enforced && sr.access != agentsandbox.AccessOff,
		Reason: reason, PolicyJSON: policy, Profile: profile,
	})
}

func (sr *sandboxRun) cleanup() {
	if sr == nil {
		return
	}
	if sr.release != nil {
		sr.release()
	}
	if sr.scratch != "" {
		_ = os.RemoveAll(sr.scratch)
	}
}

// endSandbox releases a run's sandbox: its tool target and scratch directory.
func (b *TaskBoard) endSandbox(runID string, sr *sandboxRun) {
	b.sandboxTargets.Delete(runID)
	sr.cleanup()
}

// on reports whether the run is confined.
func (sr *sandboxRun) on() bool { return sr != nil && sr.access != agentsandbox.AccessOff }

// apply adds the sandbox to the process spec: launch wrap, environment, permission flags, the tool
// names and the prompt note.
func (b *TaskBoard) applySandbox(spec harness.HeadlessSpec, sr *sandboxRun, l agentLaunch) harness.HeadlessSpec {
	if !sr.on() {
		return spec
	}
	spec.Sandbox = sr.prep.Exec
	spec.Env = agentsandbox.MergeEnv(spec.Env, sr.prep.Env)
	for _, d := range sr.prep.AddDirs {
		if !slices.Contains(spec.AddDirs, d) {
			spec.AddDirs = append(spec.AddDirs, d)
		}
	}
	spec.DisallowedTools = append(slices.Clone(spec.DisallowedTools), sr.prep.Disallowed...)
	if sr.grant != nil {
		spec.AllowedTools = append(slices.Clone(spec.AllowedTools), harness.ToolNames(l.agent, agentmcp.SandboxToolIDs...)...)
	}
	sd := l.def.Sandbox.Normalized()
	note := llmprompts.HeadlessSandboxNote.Text(b.prompts(), llmprompts.SandboxNoteVars{
		Access: string(sr.access), Worktree: sr.policy.RunDir, Read: sd.Read, Network: sr.policy.Network,
		Enforced: b.deps.Sandbox.Support().Enforced, Tools: sr.grant != nil,
	})
	spec.Prompt = strings.TrimRight(spec.Prompt, "\n") + "\n\n" + note
	return spec
}

// denialFlushEvery bounds how often repeated denials write the run row.
const denialFlushEvery = time.Second

// watchSandbox starts the denial watch for pid and returns the stop function. A repeated denial
// only counts in memory; the count reaches the row at most once per denialFlushEvery, via a
// trailing timer, and at stop.
func (b *TaskBoard) watchSandbox(run model.AdeRun, sr *sandboxRun, sink *logSink, pid int) func() {
	var (
		mu      sync.Mutex
		pending int
		flushed time.Time
		timer   *time.Timer
	)
	flush := func(listJSON string) {
		mu.Lock()
		n := pending
		pending, flushed = 0, time.Now()
		if timer != nil {
			timer.Stop()
			timer = nil
		}
		mu.Unlock()
		b.countDenials(run.ID, n, listJSON)
	}
	stop := b.deps.Sandbox.Watch(pid, run.ID, func(d agentsandbox.Denial) {
		if !sr.tracker.Add(d) {
			mu.Lock()
			pending++
			wait := denialFlushEvery - time.Since(flushed)
			if wait > 0 && timer == nil {
				timer = time.AfterFunc(wait, func() { flush("") })
			}
			mu.Unlock()
			if wait <= 0 {
				flush("")
			}
			return
		}
		mu.Lock()
		pending++
		mu.Unlock()
		sink.add(logStdout, fmt.Sprintf("sandbox denied %s %s (%s)", d.Op, d.Target, d.Process))
		list, _ := sr.tracker.Snapshot()
		raw, err := json.Marshal(list)
		if err != nil {
			slog.Warn("ade: encode sandbox denials", "scope", "ade", "run", run.ID, "err", err)
			return
		}
		flush(string(raw))
		if cur, err := b.deps.Tasks.GetRun(run.ID); err == nil {
			b.emitRuns(cur)
		}
	})
	return func() {
		stop()
		flush("")
	}
}

func (b *TaskBoard) countDenials(runID string, delta int, listJSON string) {
	if b.deps.RunSandbox == nil || delta == 0 {
		return
	}
	if err := b.deps.RunSandbox.AddDenials(runID, delta, listJSON); err != nil {
		slog.Warn("ade: count sandbox denial", "scope", "ade", "run", runID, "err", err)
	}
}

// sandboxOutcome maps a failed run to its first sandbox denial.
func (sr *sandboxRun) sandboxOutcome(out outcome) outcome {
	if !sr.on() || out.state != model.AdeRunFailed {
		return out
	}
	reason := sr.tracker.BlockedReason(out.out.Reason)
	out.note, out.out.Reason = reason, reason
	return out
}

// --- sandbox git tools ------------------------------------------------------------------------------

func (b *TaskBoard) sandboxTargetOf(runID string) (sandboxTarget, error) {
	v, ok := b.sandboxTargets.Load(runID)
	if !ok {
		return sandboxTarget{}, errors.New("this run has no sandbox git access")
	}
	return v.(sandboxTarget), nil
}

// FetchRemote implements agentmcp.SandboxGit: it fetches one remote of the run's repo in the app.
func (b *TaskBoard) FetchRemote(ctx context.Context, runID, remote string) (agentmcp.RemoteResult, error) {
	t, err := b.sandboxTargetOf(runID)
	if err != nil {
		return agentmcp.RemoteResult{}, err
	}
	mu := b.repoMutex(t.codeRepoID)
	mu.Lock()
	defer mu.Unlock()
	entry, err := b.openRepo(ctx, t.codeRepoID)
	if err != nil {
		return agentmcp.RemoteResult{}, err
	}
	names, err := entry.Remotes(ctx)
	if err != nil {
		return agentmcp.RemoteResult{}, err
	}
	if !slices.Contains(names, remote) {
		return agentmcp.RemoteResult{}, agentmcp.ToolError("remote must be one of: " + strings.Join(names, ", "))
	}
	res, err := entry.RunRemote(ctx, b.conn, gitsession.RemoteOpParams{Kind: "fetch", Remote: remote}, gitsession.RemoteDeps{Askpass: b.deps.Askpass})
	if err != nil {
		return agentmcp.RemoteResult{}, err
	}
	return remoteResult(remote, t.branch, res)
}

// PushBranch implements agentmcp.SandboxGit: it pushes the run's own task branch.
func (b *TaskBoard) PushBranch(ctx context.Context, runID string, force bool) (agentmcp.RemoteResult, error) {
	t, err := b.sandboxTargetOf(runID)
	if err != nil {
		return agentmcp.RemoteResult{}, err
	}
	mu := b.repoMutex(t.codeRepoID)
	mu.Lock()
	defer mu.Unlock()
	entry, err := b.openRepo(ctx, t.codeRepoID)
	if err != nil {
		return agentmcp.RemoteResult{}, err
	}
	head, err := entry.WorktreeHeadRef(ctx, t.worktree)
	if err != nil {
		return agentmcp.RemoteResult{}, err
	}
	if head != "refs/heads/"+t.branch {
		return agentmcp.RemoteResult{}, agentmcp.ToolError("your worktree is not on " + t.branch)
	}
	remote := forcePushRemote(ctx, entry, t.branch)
	if remote == "" {
		return agentmcp.RemoteResult{}, agentmcp.ToolError("the repo has no remote to push to")
	}
	pf, err := entry.PushPreflightTo(ctx, remote, t.branch, t.branch)
	if err != nil {
		return agentmcp.RemoteResult{}, err
	}
	if pf.ProtectedBy != nil {
		return agentmcp.RemoteResult{}, agentmcp.ToolError(t.branch + " is a protected branch")
	}
	params := gitsession.RemoteOpParams{Kind: "push", Remote: remote, Branch: t.branch, RemoteBranch: t.branch, SetUpstream: pf.Upstream == nil}
	if force {
		params = gitsession.RemoteOpParams{Kind: "forcePush", Remote: remote, Branch: t.branch, RemoteBranch: t.branch, ExpectedRemoteTip: pf.RemoteTip}
	}
	res, err := entry.RunRemote(ctx, b.conn, params, gitsession.RemoteDeps{Askpass: b.deps.Askpass})
	if err != nil {
		return agentmcp.RemoteResult{}, err
	}
	out, err := remoteResult(remote, t.branch, res)
	b.notifyBoard()
	return out, err
}

// remoteResult turns a remote op result into the tool result; a failed op is an error with its message.
func remoteResult(remote, branch string, res gitsession.RemoteOpResult) (agentmcp.RemoteResult, error) {
	if res.Error != nil {
		return agentmcp.RemoteResult{}, agentmcp.ToolError(res.Error.Message)
	}
	var lines []string
	for _, u := range res.Updates {
		from, to := "none", "none"
		if u.From != nil {
			from = short(*u.From)
		}
		if u.To != nil {
			to = short(*u.To)
		}
		lines = append(lines, fmt.Sprintf("%s %s..%s", u.Ref, from, to))
	}
	if len(lines) == 0 {
		lines = append(lines, "up to date")
	}
	return agentmcp.RemoteResult{Remote: remote, Branch: branch, Output: strings.Join(lines, "\n")}, nil
}

func short(sha string) string {
	if len(sha) > 8 {
		return sha[:8]
	}
	return sha
}

// noticeSandbox writes the sandbox's first log lines: the not-enforced notice and skipped paths.
func (b *TaskBoard) noticeSandbox(sink *logSink, sr *sandboxRun) {
	if !sr.on() {
		return
	}
	if sup := b.deps.Sandbox.Support(); !sup.Enforced {
		sink.add(logStdout, agentsandbox.NotEnforcedText(sr.agent))
	}
	for _, n := range sr.notes {
		sink.add(logStdout, n)
	}
}

// --- wire ---------------------------------------------------------------------------------------------

// sandboxSupport is whether this OS enforces step sandboxes; Reason is the editor notice when not.
func (b *TaskBoard) sandboxSupport() adewire.SandboxSupport {
	if b.deps.Sandbox != nil {
		if sup := b.deps.Sandbox.Support(); sup.Enforced {
			return adewire.SandboxSupport{Enforced: true}
		} else {
			return adewire.SandboxSupport{Reason: agentsandbox.NotEnforcedText(b.deps.Agent.Active())}
		}
	}
	return adewire.SandboxSupport{Reason: agentsandbox.NotEnforcedText(b.deps.Agent.Active())}
}

func badgeOf(s repos.AdeRunSandbox) *adewire.RunSandboxBadge {
	badge := &adewire.RunSandboxBadge{Access: s.Access, Enforced: s.Enforced, Denials: s.DenialCount}
	var list []agentsandbox.Denial
	if json.Unmarshal([]byte(s.DenialsJSON), &list) == nil && len(list) > 0 {
		last := list[len(list)-1]
		badge.Last = last.Op + " " + last.Target
	}
	return badge
}

// badgeTasks sets the sandbox badge of every headless run in tasks with one query.
func (b *TaskBoard) badgeTasks(tasks []adewire.Task) {
	if b.deps.RunSandbox == nil {
		return
	}
	var ids []string
	for _, t := range tasks {
		for _, r := range t.Runs {
			ids = append(ids, r.ID)
		}
	}
	rows, err := b.deps.RunSandbox.Badges(ids)
	if err != nil {
		slog.Warn("ade: sandbox badges", "scope", "ade", "err", err)
		return
	}
	for _, t := range tasks {
		for i := range t.Runs {
			if row, ok := rows[t.Runs[i].ID]; ok {
				t.Runs[i].Sandbox = badgeOf(row)
			}
		}
	}
}

// runProvider is the provider a stored run ran under, from its session row; the active one when the
// run has no session.
func (b *TaskBoard) runProvider(runID string) harness.Provider {
	if run, err := b.deps.Tasks.GetRun(runID); err == nil && run.SessionID != "" {
		if rec, err := b.deps.Sessions.Get(run.SessionID); err == nil && rec != nil {
			if p, err := b.deps.Agent.For(rec.Provider); err == nil {
				return p
			}
		}
	}
	return b.deps.Agent.Active()
}

// RunSandbox returns the stored sandbox of a run: its policy, profile and denials.
func (b *TaskBoard) RunSandbox(_ context.Context, runID string) (adewire.RunSandboxDetail, error) {
	if b.deps.RunSandbox == nil {
		return adewire.RunSandboxDetail{}, invalid("run %s has no sandbox", runID)
	}
	row, err := b.deps.RunSandbox.Get(runID)
	if err != nil {
		return adewire.RunSandboxDetail{}, err
	}
	if row == nil {
		return adewire.RunSandboxDetail{}, invalid("run %s has no sandbox", runID)
	}
	out := adewire.RunSandboxDetail{
		Access: row.Access, Enforced: row.Enforced, Profile: row.Profile, DenialCount: row.DenialCount,
		Write: []string{}, Read: []string{}, Allow: []string{}, Denials: []adewire.SandboxDenial{},
	}
	if row.Reason != "" {
		out.Reason = agentsandbox.NotEnforcedText(b.runProvider(runID))
	}
	var pol agentsandbox.Policy
	if err := json.Unmarshal([]byte(row.PolicyJSON), &pol); err == nil {
		out.Write, out.Read, out.Allow = nonNil(pol.Write), nonNil(pol.Read), nonNil(pol.Allow)
		out.Network = pol.Network
	}
	var list []agentsandbox.Denial
	if err := json.Unmarshal([]byte(row.DenialsJSON), &list); err == nil {
		for _, d := range list {
			out.Denials = append(out.Denials, adewire.SandboxDenial{At: d.At.UnixMilli(), Op: d.Op, Target: d.Target, Process: d.Process})
		}
	}
	return out, nil
}
