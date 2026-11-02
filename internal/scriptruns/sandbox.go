package scriptruns

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/kirathecat/kira-studio/internal/agentmcp"
	"github.com/kirathecat/kira-studio/internal/agentsandbox"
	"github.com/kirathecat/kira-studio/internal/harness"
	"github.com/kirathecat/kira-studio/internal/llmprompts"
	"github.com/kirathecat/kira-studio/internal/runoutcome"
	"github.com/kirathecat/kira-studio/internal/scripts"
)

// sandbox.go confines a smart run in dir mode worktree: the default write policy on its worktree.
// Studio and the other folder modes leave Service.Sandbox nil or are not confined.

type smartSandbox struct {
	policy  agentsandbox.Policy
	prep    agentsandbox.Prepared
	tracker agentsandbox.Tracker
	tools   bool
	release func()
	agent   agentsandbox.Namer
}

// prepareSandbox returns nil when the run is not confined.
func (s *Service) prepareSandbox(ctx context.Context, p *planned, runID string, port int, contextFile string) (*smartSandbox, error) {
	if s.Sandbox == nil || p.dir.Mode != scripts.DirModeWorktree {
		return nil, nil
	}
	in := agentsandbox.Input{
		RunID: runID, Access: agentsandbox.AccessWrite, Worktrees: []string{p.dir.Path}, Network: true,
		Scratch: filepath.Join(s.Home, "sandbox", runID), Cache: filepath.Join(s.Home, "sandbox-cache"),
		ConfigDir: filepath.Join(s.Home, "automations-mcp"), Ports: []int{port},
	}
	if contextFile != "" {
		in.ContextDir = filepath.Dir(contextFile)
	}
	agent := p.agent
	rt := agentsandbox.DiscoverRuntime(ctx, agent, "")
	pol, _, err := agentsandbox.ResolvePolicy(in, rt)
	if err != nil {
		return nil, err
	}
	prep, err := s.Sandbox.Prepare(pol, rt)
	if err != nil {
		_ = os.RemoveAll(pol.Scratch)
		return nil, err
	}
	return &smartSandbox{policy: pol, prep: prep, tools: p.preview.ADE != nil, agent: agent, release: func() { s.Sandbox.Release(runID) }}, nil
}

func (sb *smartSandbox) cleanup() {
	if sb != nil {
		sb.release()
		_ = os.RemoveAll(sb.policy.Scratch)
	}
}

// applySandbox adds the sandbox to the process spec.
func (s *Service) applySandbox(spec harness.HeadlessSpec, sb *smartSandbox, agent harness.Provider) harness.HeadlessSpec {
	if sb == nil {
		return spec
	}
	spec.Sandbox = sb.prep.Exec
	spec.Env = agentsandbox.MergeEnv(spec.Env, sb.prep.Env)
	for _, d := range sb.prep.AddDirs {
		if !slices.Contains(spec.AddDirs, d) {
			spec.AddDirs = append(spec.AddDirs, d)
		}
	}
	spec.DisallowedTools = append(slices.Clone(spec.DisallowedTools), sb.prep.Disallowed...)
	if sb.tools {
		spec.AllowedTools = append(slices.Clone(spec.AllowedTools), harness.ToolNames(agent, agentmcp.SandboxToolIDs...)...)
	}
	note := llmprompts.HeadlessSandboxNote.Text(s.llmPrompts(), llmprompts.SandboxNoteVars{
		Access: string(agentsandbox.AccessWrite), Worktree: sb.policy.RunDir, Network: true,
		Enforced: s.Sandbox.Support().Enforced, Tools: sb.tools,
	})
	spec.Prompt = strings.TrimRight(spec.Prompt, "\n") + "\n\n" + note
	return spec
}

// noticeSandbox writes the not-enforced line first in the run log.
func (s *Service) noticeSandbox(sink *logSink, sb *smartSandbox) {
	if sb == nil {
		return
	}
	if sup := s.Sandbox.Support(); !sup.Enforced {
		sink.add("stdout", agentsandbox.NotEnforcedText(sb.agent))
	}
}

// watchSandbox logs each new denial as a run log line.
func (s *Service) watchSandbox(sb *smartSandbox, runID string, sink *logSink, pid int) func() {
	if sb == nil {
		return func() {}
	}
	return s.Sandbox.Watch(pid, runID, func(d agentsandbox.Denial) {
		if sb.tracker.Add(d) {
			sink.add("stdout", fmt.Sprintf("sandbox denied %s %s (%s)", d.Op, d.Target, d.Process))
		}
	})
}

// outcome maps a failed run to its sandbox cause (plan 6.3, 6.4).
func (sb *smartSandbox) outcome(out runoutcome.Outcome) runoutcome.Outcome {
	if sb == nil || out.Status != runoutcome.StatusFailed {
		return out
	}
	out.Reason = sb.tracker.BlockedReason(out.Reason)
	return out
}
