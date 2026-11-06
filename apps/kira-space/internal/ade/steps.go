package ade

import (
	"fmt"
	"slices"
	"strings"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// steps.go is the pure half of the step machine: which branches a step runs on, a step's aggregate
// state and the next action. It mirrors frontend ade/v2/board/progress.ts, which fixes the rules.

// stepDef is one step of the workflow. A script step's Prompt is its command; a manual step's is its
// session's first message.
type stepDef struct {
	ID, Name, Kind, RunsOn, Before, Timeout, Prompt string
	Skip, Session                                   bool
	// OutsideRepo runs the step once in the task folder, on no branch.
	OutsideRepo bool
	// Results are the step's declared (or default) results with their routes.
	Results []adewire.StepResult
	// ReturnLimit is how many returns the step may make before the run stops; 0 is no limit.
	ReturnLimit  int
	AllowedTools []string
	// DbMcp gives the step's agent sessions the Kira DB tools.
	DbMcp bool
	// DisallowedTools become --disallowedTools; only the rebase step sets them.
	DisallowedTools []string
	// SmartScript names the smart script that replaces Prompt; Params are its values.
	SmartScript string
	Params      map[string][]string
	// Review makes the step a review step: no process, the run waits for the user.
	Review *adewire.ReviewStepConfig
	// Sandbox is what the step's agent may do on the machine.
	Sandbox adewire.StepSandbox
	// Settings are the stage settings per provider id.
	Settings map[string]map[string]string
	// Agent is the provider id for the step's new conversations; "" uses the Agent setting.
	Agent string
}

// manual says the step is the user's own work: no runs, no branch targets.
func (d stepDef) manual() bool { return d.Kind == adewire.KindManual }

// workflowSteps lists a workflow's steps in order.
func workflowSteps(wf adewire.Workflow) []stepDef {
	out := make([]stepDef, len(wf.Steps))
	for i, s := range wf.Steps {
		results := s.Results
		if len(results) == 0 && s.Kind != adewire.KindManual {
			results = adewire.DefaultResults()
		}
		prompt := s.Prompt
		if s.PromptExtra != "" {
			prompt += "\n\n" + s.PromptExtra
		}
		if s.Kind == adewire.KindScript {
			prompt = s.Command
		}
		out[i] = stepDef{ID: s.ID, Name: s.Name, Kind: s.Kind, RunsOn: s.RunsOn, Before: s.Before, Timeout: s.Timeout, Prompt: prompt,
			Skip: s.Skip, Session: s.Session, OutsideRepo: s.OutsideRepo, Results: results, ReturnLimit: s.MaxReturns, AllowedTools: s.AllowedTools, DbMcp: s.DbMcp,
			SmartScript: s.SmartScript, Params: s.Params, Review: s.Review, Sandbox: s.Sandbox.NormalizedFor(s.Kind), Settings: s.Settings, Agent: s.Agent}
		if s.Kind == adewire.KindScript {
			out[i].Settings, out[i].Agent, out[i].DbMcp = nil, "", false
		}
		if s.Kind == adewire.KindManual {
			out[i].Settings, out[i].Agent = nil, ""
			out[i].DbMcp = s.Session && s.DbMcp
		}
		if s.SmartScript != "" {
			out[i].DbMcp = false
		}
	}
	return out
}

// firstStep is the index of the first step a task enters: the first one not skipped.
func firstStep(wf adewire.Workflow) (int, bool) { return nextStep(wf, -1) }

// nextStep is the index of the first non-skipped step after index from.
func nextStep(wf adewire.Workflow, from int) (int, bool) {
	for i := from + 1; i < len(wf.Steps); i++ {
		if !wf.Steps[i].Skip {
			return i, true
		}
	}
	return -1, false
}

// mineBranch is a task's mine branch with its repo's display name (nickname, else name).
type mineBranch struct{ ID, Nick string }

// taskTarget is the target of a step on a task with no branch: a run with no branch, working in the
// task folder.
const taskTarget = ""

// stepTargets resolves a step's branches in task order: once = the first, each repo = all, only
// <nick> = those in that repo. A task with no mine branch runs a step on taskTarget, except a review
// step and an `only` step, which need a branch.
func stepTargets(runsOn string, review bool, mine []mineBranch) ([]string, error) {
	if len(mine) == 0 {
		if review {
			return nil, fmt.Errorf("the review step needs a branch to review")
		}
		if nick, ok := strings.CutPrefix(runsOn, "only "); ok {
			return nil, fmt.Errorf("the step runs only on %q, which has no branch in this task", nick)
		}
		return []string{taskTarget}, nil
	}
	if nick, ok := strings.CutPrefix(runsOn, "only "); ok {
		var out []string
		for _, b := range mine {
			if b.Nick == nick {
				out = append(out, b.ID)
			}
		}
		if len(out) == 0 {
			return nil, fmt.Errorf("the step runs only on %q, which has no branch in this task", nick)
		}
		return out, nil
	}
	if runsOn == "once" {
		return []string{mine[0].ID}, nil
	}
	out := make([]string, len(mine))
	for i, b := range mine {
		out[i] = b.ID
	}
	return out, nil
}

// stepView is a step with its targets and the latest run per target branch in the task's current pass.
type stepView struct {
	Def     stepDef
	Targets []string
	// TargetErr is why the step has no branches to run on; it only matters when the step starts.
	TargetErr error
	Runs      map[string]model.AdeRun
}

// state is the worst-of aggregate: stuck > failed > running/back > pending; done when all targets are done.
func (v stepView) state() string {
	if len(v.Targets) == 0 {
		return model.AdeRunPending
	}
	var stuck, failed, running, done int
	for _, id := range v.Targets {
		switch v.Runs[id].State {
		case model.AdeRunStuck:
			stuck++
		case model.AdeRunFailed:
			failed++
		case model.AdeRunRunning, model.AdeRunBack, model.AdeRunReview:
			running++
		case model.AdeRunDone:
			done++
		}
	}
	switch {
	case stuck > 0:
		return model.AdeRunStuck
	case failed > 0:
		return model.AdeRunFailed
	case running > 0:
		return model.AdeRunRunning
	case done == len(v.Targets):
		return model.AdeRunDone
	}
	return model.AdeRunPending
}

func (v stepView) started() bool { return len(v.Runs) > 0 }

type actionKind int

const (
	actIdle actionKind = iota
	actStart
	actAwaitApproval
	actWaitUser // the path ends at a manual step
	actComplete // the path ends past the last step
)

// action is nextAction's answer. Step is the index of the step it concerns.
type action struct {
	Kind actionKind
	Step int
}

// resultOf is the declared result with the id, false when the step has none such.
func (d stepDef) resultOf(id string) (adewire.StepResult, bool) {
	i := slices.IndexFunc(d.Results, func(r adewire.StepResult) bool { return r.ID == id })
	if i < 0 {
		return adewire.StepResult{}, false
	}
	return d.Results[i], true
}

// routeIndex resolves a run's stored forward route to the index of the step after steps[i]; len(steps)
// = the workflow is complete. A run with no stored route (before results) counts as `next`.
func routeIndex(steps []stepView, i int, route string) int {
	switch route {
	case "", adewire.RouteNext:
		return i + 1
	case adewire.RouteEnd:
		return len(steps)
	}
	if j := slices.IndexFunc(steps, func(v stepView) bool { return v.Def.ID == route }); j > i {
		return j
	}
	return i + 1
}

func runRoute(r model.AdeRun) string {
	if r.Outcome == nil {
		return ""
	}
	return r.Outcome.Route
}

// forwardTarget is where the path goes after the done step i: the earliest forward route among its
// runs on the given branches (the step that skips the least, D8).
func forwardTarget(steps []stepView, i int, branches []string) int {
	next := len(steps)
	for _, id := range branches {
		if r, ok := steps[i].Runs[id]; ok {
			next = min(next, routeIndex(steps, i, runRoute(r)))
		}
	}
	if next < i+1 {
		return i + 1
	}
	return next
}

// walkPath follows the workflow's routes from the entry step: a skipped step is passed over, a done
// step moves to its forward target, the first step not done (a manual step never is) ends the path.
// skipped marks the steps the route went past.
func walkPath(steps []stepView, entry int) (path []int, skipped []bool) {
	skipped = make([]bool, len(steps))
	i := max(entry, 0)
	for i < len(steps) {
		if steps[i].Def.Skip {
			skipped[i] = true
			i++
			continue
		}
		path = append(path, i)
		if steps[i].Def.manual() || steps[i].state() != model.AdeRunDone {
			break
		}
		next := forwardTarget(steps, i, steps[i].Targets)
		for j := i + 1; j < next && j < len(steps); j++ {
			skipped[j] = true
		}
		i = next
	}
	return path, skipped
}

// currentStep is the step the task is at: the end of the route path from the entry. -1 when the path
// runs past the last step.
func currentStep(steps []stepView, entry int) int {
	path, _ := walkPath(steps, entry)
	if len(path) == 0 {
		return -1
	}
	i := path[len(path)-1]
	if !steps[i].Def.manual() && steps[i].state() == model.AdeRunDone {
		return -1
	}
	return i
}

// rerun is one step run chainRerun queues again on a branch.
type rerun struct {
	Step    int
	Branch  string
	Attempt int
	Loops   int
}

// chainRerun finds, per target branch, the first step that must run again after a send-back fix.
// Walking the branch's route path, round is the highest loops among the done runs so far; a
// done or `back` run with fewer loops than the round is stale and is queued again with loops =
// round. A missing, running, pending, stuck or failed run ends the walk.
func chainRerun(steps []stepView, entry int) []rerun {
	var out []rerun
	for _, bid := range targetBranches(steps) {
		if r, ok := chainRerunBranch(steps, entry, bid); ok {
			out = append(out, r)
		}
	}
	return out
}

// targetBranches lists every target branch id in first-seen order.
func targetBranches(steps []stepView) []string {
	var branches []string
	seen := map[string]bool{}
	for _, s := range steps {
		for _, id := range s.Targets {
			if !seen[id] {
				seen[id] = true
				branches = append(branches, id)
			}
		}
	}
	return branches
}

func chainRerunBranch(steps []stepView, entry int, bid string) (rerun, bool) {
	round := 0
	for i := max(entry, 0); i < len(steps); {
		s := steps[i]
		if s.Def.Skip || !slices.Contains(s.Targets, bid) {
			i++
			continue
		}
		r, ok := s.Runs[bid]
		if !ok {
			return rerun{}, false
		}
		stale := rerun{Step: i, Branch: bid, Attempt: r.Attempt + 1, Loops: round}
		switch r.State {
		case model.AdeRunDone:
			if r.Loops < round {
				return stale, true
			}
			round = max(round, r.Loops)
			i = forwardTarget(steps, i, []string{bid})
		case model.AdeRunBack:
			return stale, r.Loops < round
		default:
			return rerun{}, false
		}
	}
	return rerun{}, false
}

// nextAction looks at the end of the route path from the entry step: the first step not done. A
// manual step waits for the user. A running, stuck or failed step waits (a failure was routed when
// it was recorded: looped, continued or stopped). A pending step with runs waits on its worktree
// gate; without runs it starts (auto) or waits for Approve, except the first of the path, which
// only StartRun starts. A path that runs past the last step = the workflow is complete.
func nextAction(steps []stepView, entry int) action {
	path, _ := walkPath(steps, entry)
	if len(path) == 0 {
		return action{Kind: actComplete, Step: len(steps)}
	}
	i := path[len(path)-1]
	s := steps[i]
	if s.Def.manual() {
		return action{Kind: actWaitUser, Step: i}
	}
	switch s.state() {
	case model.AdeRunDone:
		return action{Kind: actComplete, Step: i}
	case model.AdeRunPending:
		switch {
		case s.started() || i == path[0]:
			return action{Kind: actIdle, Step: i}
		case s.Def.Before == "approval":
			return action{Kind: actAwaitApproval, Step: i}
		}
		return action{Kind: actStart, Step: i}
	}
	return action{Kind: actIdle, Step: i}
}
