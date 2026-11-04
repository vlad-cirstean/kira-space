package ade

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/agentcfg"
	"github.com/kirathecat/kira-studio/internal/agentmcp"
	"github.com/kirathecat/kira-studio/internal/harness"
	"github.com/kirathecat/kira-studio/internal/runoutcome"
	"github.com/kirathecat/kira-studio/internal/scripts"
)

// smartStep is a workflow step that runs a smart script: the script's prompt, tools, model and budget
// replace the step prompt, and the run is isolated from the user's Claude settings (D8, D9).
type smartStep struct {
	script  scripts.CustomScript
	prompt  string
	env     []string
	dir     string
	timeout time.Duration
	// secrets are the values of the script's secret params, for the step's log mask.
	secrets []string
}

// planSmartStep resolves def's smart script for a run on sb. It has no side effect apart from
// creating the script's own folder when the script does not run in the worktree.
func (b *TaskBoard) planSmartStep(tc *taskCtx, sb model.AdeTaskBranch, path string, def stepDef) (*smartStep, error) {
	fail := func(format string, a ...any) (*smartStep, error) {
		return nil, invalid("smart script %q: %s", def.SmartScript, fmt.Sprintf(format, a...))
	}
	all, err := b.listScripts()
	if err != nil {
		return nil, err
	}
	res, err := b.resolveScriptRef(all, workflowUse, def.SmartScript, def.Params, b.adeVars(tc, sb, path), path)
	if err != nil {
		var r refReason
		if errors.As(err, &r) {
			return fail("%s", r)
		}
		return nil, err
	}
	script, rendered := res.script, res.rendered
	step := &smartStep{script: script, env: res.env, dir: res.dir, timeout: script.Smart.TimeoutDuration(), secrets: res.secrets}
	if def.Timeout != "" {
		step.timeout = parseStepTimeout(def.Timeout)
	}
	body := scripts.PlainText(scripts.Compose(script.Command, rendered))
	step.prompt = body + "\n\n" + agentmcp.SpaceNote(b.prompts()) + "\n\n" + agentmcp.FinishInstruction(b.prompts(), resultSpecs(def.Results))
	return step, nil
}

// secretParams is the step's param values with each saved secret param filled from the script's sealed
// value, as a run does. A workflow file never carries a secret value: a `params` entry for a secret
// param is refused, since the file is plain text.
func (b *TaskBoard) secretParams(script scripts.CustomScript, given map[string][]string, file string) (map[string][]string, error) {
	out := make(map[string][]string, len(given))
	maps.Copy(out, given)
	for _, p := range script.Params {
		if p.Secret && len(given[p.Name]) == 0 {
			delete(out, p.Name)
		}
	}
	var want []string
	for _, p := range script.Params {
		if !p.Secret {
			continue
		}
		if len(given[p.Name]) > 0 {
			return nil, fmt.Errorf("%s is a secret param: save its value in the script, not in the %s", p.Name, file)
		}
		if slices.Contains(script.SavedSecrets, p.Name) {
			want = append(want, p.Name)
		}
	}
	if len(want) == 0 {
		return out, nil
	}
	vals, unreadable, err := b.deps.CustomScripts.SecretValues(script.ID)
	if err != nil {
		return nil, err
	}
	for _, n := range want {
		if v, ok := vals[n]; ok {
			out[n] = []string{v}
		} else if e, bad := unreadable[n]; bad {
			return nil, fmt.Errorf("the saved value of %s cannot be read, set it again in the script: %w", n, e)
		}
	}
	return out, nil
}

// mcpNames are the user MCP servers the script ticked.
func (s *smartStep) mcpNames() []string {
	names := make([]string, 0, len(s.script.Smart.MCP))
	for _, c := range s.script.Smart.MCP {
		names = append(names, c.Server)
	}
	return names
}

// checkMCP fails with harness.ErrServerGone when a ticked server is missing from its source: the
// app's agent settings when hermetic, else the agent's list for the scripts home (the picker's
// folder, never a project's `.mcp.json`). A removed server stops the step instead of running it
// without its tools.
func (s *smartStep) checkMCP(ctx context.Context, src agentcfg.ServerSource, agent harness.Provider, home string) error {
	names := s.mcpNames()
	if len(names) == 0 {
		return nil
	}
	missing, _, err := src.Missing(ctx, agent, home, names, true)
	if err != nil {
		return err
	}
	if missing != "" {
		return src.Gone(agent, missing)
	}
	return nil
}

// spec is the process spec of a smart step run; the caller adds the MCP config and user servers.
func (s *smartStep) spec(agent harness.Provider, base harness.HeadlessSpec, space bool) harness.HeadlessSpec {
	extra := harness.ToolNames(agent, agentmcp.FinishStep, agentmcp.RunOutcome)
	if space {
		extra = append(extra, harness.ToolNames(agent, agentmcp.SpaceToolIDs...)...)
	}
	base.Dir, base.Model, base.MaxBudgetUSD, base.Isolated = s.dir, s.script.Smart.Model, s.script.Smart.MaxBudgetUSD, true
	base.Tools, base.AllowedTools = agent.ToolArgs(s.script.Smart.ToolPolicy(extra))
	base.Config = harness.ConfigNone
	base.Env = append(base.Env, s.env...)
	base.Timeout = s.timeout
	return base
}

// smartOutcome is a smart step's outcome: a finish_step call wins, then the result event (budget,
// turn limit, error), then how the process ended. The cost and denials of the result event are added.
func (b *TaskBoard) smartOutcome(an runoutcome.Agent, runID string, exit harness.Exit, runErr error, res *harness.Result, step *smartStep, results []adewire.StepResult, lastErr string, resumeLost bool) outcome {
	var out outcome
	if f, finished := b.takeFinish(runID); finished {
		code := exit.Code
		out = fromFinish(f, &code, results)
	} else if lost, ok := lostOutcome(an, resumeLost, lastErr); ok {
		out = lost
	} else if o, ok := agentmcp.ResultOutcome(an, res, exit.Code, step.script.Smart.MaxBudgetUSD); ok {
		out = fromOutcome(model.AdeRunFailed, o.WithLastError(lastErr))
	} else {
		o := runoutcome.ForProcess(agentEnd(an, exit, runErr, formatTimeout(step.timeout))).WithLastError(lastErr)
		out = fromOutcome(model.AdeRunFailed, o)
	}
	out.out.Outcome = agentmcp.ApplyResult(out.out.Outcome, res)
	out.note = out.out.Reason
	return out
}

// workflowSecretParam names the first secret param a smart step of wf carries a value for, or "". A
// workflow file is plain text, so Save and validation refuse it before a run does.
func (b *TaskBoard) workflowSecretParam(wf *adewire.Workflow) (step, param string) {
	if wf == nil || b.deps.CustomScripts == nil {
		return "", ""
	}
	var all []scripts.CustomScript
	listed := false
	for _, st := range wf.Steps {
		if st.SmartScript == "" || len(st.Params) == 0 {
			continue
		}
		if !listed {
			all, _ = b.deps.CustomScripts.List()
			listed = true
		}
		for _, sc := range all {
			if sc.Kind != scripts.KindSmart || sc.Name != st.SmartScript {
				continue
			}
			for _, p := range sc.Params {
				if p.Secret && len(st.Params[p.Name]) > 0 {
					return st.Name, p.Name
				}
			}
		}
	}
	return "", ""
}
