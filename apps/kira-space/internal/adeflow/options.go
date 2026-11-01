package adeflow

import (
	"fmt"
	"slices"
	"strings"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
)

// Shared enums: the validator checks against them, the option table and AGENTS.md print them.
var (
	kinds        = []string{adewire.KindAgent, adewire.KindScript, adewire.KindReview, adewire.KindManual}
	beforeValues = []string{"auto", "approval"}
	accessValues = []string{adewire.SandboxWrite, adewire.SandboxRead, adewire.SandboxOff}
	sendToValues = []string{adewire.ReviewSendImplementer, adewire.ReviewSendNew}
)

// option is one workflow key at one nesting level. The table below is the single list of workflow
// keys: the parser, the writer and the generated AGENTS.md all derive from it.
type option struct {
	Path       string             // dotted; [] for list items, * for any key
	Type       string             // "text", "boolean", "whole number", "list of text", "mapping", "list", "duration"
	Kinds      []adewire.StepKind // steps[] keys only; nil = every kind
	Required   bool               // always required where the key may appear
	RequiredOn []adewire.StepKind // required on these kinds only
	Default    string
	Enum       []string
	Range      string
	Legacy     bool // accepted and ignored; the writer drops it
	Doc        string
}

var (
	autoKinds   = []adewire.StepKind{adewire.KindAgent, adewire.KindScript, adewire.KindReview}
	agentReview = []adewire.StepKind{adewire.KindAgent, adewire.KindReview}
	agentOnly   = []adewire.StepKind{adewire.KindAgent}
	scriptOnly  = []adewire.StepKind{adewire.KindScript}
	reviewOnly  = []adewire.StepKind{adewire.KindReview}
	manualOnly  = []adewire.StepKind{adewire.KindManual}
	withSession = []adewire.StepKind{adewire.KindAgent, adewire.KindReview, adewire.KindManual}
)

var options = []option{
	{Path: "id", Type: "text", Required: true, Range: "pattern `" + idPattern.String() + "`", Doc: "Workflow id. Must equal the file name without `.yaml`."},
	{Path: "name", Type: "text", Required: true, Doc: "Display name."},
	{Path: "kira_space_mcp", Type: "boolean", Legacy: true, Doc: "Old switch for the Kira Space MCP server."},
	{Path: "steps", Type: "list", Required: true, Range: "at least 1 step", Doc: "Steps run top to bottom. One step must not be skipped."},
	{Path: "steps[]", Type: "mapping", Doc: "One step."},
	{Path: "steps[].id", Type: "text", Required: true, Range: fmt.Sprintf("pattern `%s`", idPattern), Doc: "Step id, unique in the workflow."},
	{Path: "steps[].name", Type: "text", Required: true, Doc: "Display name."},
	{Path: "steps[].kind", Type: "text", Required: true, Enum: kinds, Doc: "What the step does."},
	{Path: "steps[].status", Type: "text", Range: fmt.Sprintf("status id, at most %d characters", maxStatusID), Doc: "Task status while the step runs. Default: the kind's own status. Must exist in the Statuses list of the Workflows page."},
	{Path: "steps[].skip", Type: "boolean", Default: "false", Doc: "Keep the step in the file but never run it."},
	{Path: "steps[].session", Type: "boolean", Kinds: manualOnly, Default: "false", Doc: "Open an agent session for the step."},
	{Path: "steps[].runs_on", Type: "text", Kinds: autoKinds, RequiredOn: autoKinds, Range: "`once`, `each repo` or `only <repo>`", Doc: "Where the step runs."},
	{Path: "steps[].before", Type: "text", Kinds: autoKinds, Default: "auto", Enum: beforeValues, Doc: "`approval` waits for a person before the step starts."},
	{Path: "steps[].results", Type: "list", Kinds: autoKinds, Range: fmt.Sprintf("1 to %d results", maxResults), Doc: "Outcomes the step can report. Default: `done` (ok) and `failed` (not ok)."},
	{Path: "steps[].max_returns", Type: "whole number", Kinds: autoKinds, Default: fmt.Sprint(adewire.DefaultMaxReturns), Range: fmt.Sprintf("1 to %d, review steps 0 to %d (0 is no limit)", adewire.MaxMaxReturns, adewire.MaxMaxReturns), Doc: "How often results may route back to this or an earlier step. Review steps default to 0."},
	{Path: "steps[].timeout", Type: "duration", Kinds: autoKinds, RequiredOn: autoKinds, Range: fmt.Sprintf("Go duration such as `30m` or `2h`, at most %s", maxStepTimeout), Doc: "Longest the step may run."},
	{Path: "steps[].command", Type: "text", Kinds: scriptOnly, RequiredOn: scriptOnly, Doc: "Shell command. Runs in the task worktree."},
	{Path: "steps[].smart_script", Type: "text", Kinds: agentOnly, Range: fmt.Sprintf("at most %d characters", maxSmartScriptName), Doc: "Name of a smart script to run instead of a prompt."},
	{Path: "steps[].params", Type: "mapping", Kinds: agentOnly, Doc: "Smart script parameters. Only with `smart_script`."},
	{Path: "steps[].params.*", Type: "text or list of text", Range: "name pattern `" + paramNamePattern.String() + "`", Doc: "One parameter: a text value or a list of them."},
	{Path: "steps[].review", Type: "mapping", Kinds: reviewOnly, Doc: "Review settings. An empty mapping takes the defaults."},
	{Path: "steps[].review.send_to", Type: "text", Default: adewire.ReviewSendImplementer, Enum: sendToValues, Doc: "Who gets the review findings."},
	{Path: "steps[].review.implementer", Type: "text", Doc: "Id of an earlier agent step without `smart_script`. The review resumes that step's session."},
	{Path: "steps[].sandbox", Type: "mapping", Kinds: agentReview, Doc: "File and network limits. An empty mapping takes the defaults."},
	{Path: "steps[].sandbox.access", Type: "text", Default: adewire.SandboxWrite, Enum: accessValues, Doc: "`write` writes the task worktree, `read` only reads, `off` turns the sandbox off."},
	{Path: "steps[].sandbox.read", Type: "list of text", Range: fmt.Sprintf("at most %d paths", maxSandboxPaths), Doc: "Extra paths the step may read."},
	{Path: "steps[].sandbox.allow", Type: "list of text", Range: fmt.Sprintf("at most %d paths", maxSandboxPaths), Doc: "Extra paths the step may write."},
	{Path: "steps[].sandbox.network", Type: "boolean", Default: "true", Doc: "Allow network access."},
	{Path: "steps[].settings", Type: "mapping", Kinds: withSession, Doc: "Agent settings for the step, by agent id then setting key. Manual steps: only with `session: true`."},
	{Path: "steps[].prompt", Type: "text", Kinds: []adewire.StepKind{adewire.KindAgent, adewire.KindManual}, RequiredOn: agentOnly, Doc: "What the agent does. On an agent step, required unless `smart_script` is set."},
	{Path: "steps[].allowed_tools", Type: "list of text", Kinds: agentOnly, Doc: "Tools the agent may use without asking, such as `Bash(git *)` or `mcp__server__tool`. Not with `smart_script`."},
	{Path: "steps[].results[]", Type: "mapping", Doc: "One outcome."},
	{Path: "steps[].results[].id", Type: "text", Required: true, Range: fmt.Sprintf("pattern `%s`, unique in the step", idPattern), Doc: "Outcome id. `needs_input` is reserved."},
	{Path: "steps[].results[].ok", Type: "boolean", Required: true, Doc: "Whether the outcome counts as success."},
	{Path: "steps[].results[].description", Type: "text", Range: fmt.Sprintf("one line, at most %d characters", maxResultDescription), Doc: "Tells the agent when to report this outcome."},
	{Path: "steps[].results[].next", Type: "text", Default: "`next` when ok, `stop` when not ok", Range: "`next`, `end`, `stop` or a step id", Doc: "Where the workflow goes after this outcome."},
}

var headKeys = []string{"id", "name", "kind", "status", "skip"}

// childKeys lists the keys directly under parent in table order ("" is the top level).
func childKeys(parent string, include func(option) bool) []string {
	var out []string
	for _, o := range options {
		rest, ok := strings.CutPrefix(o.Path, parent+".")
		if parent == "" {
			rest, ok = o.Path, !strings.HasSuffix(o.Path, "[]")
		}
		if !ok || strings.ContainsAny(rest, ".*") || strings.HasSuffix(rest, "[]") || (include != nil && !include(o)) {
			continue
		}
		out = append(out, rest)
	}
	return out
}

func notLegacy(o option) bool { return !o.Legacy }

// Key sets derived from the table; fields() refuses any key outside them.
var (
	topKeys     = childKeys("", nil)
	topOrder    = childKeys("", notLegacy)
	stepOrder   = childKeys("steps[]", nil)
	resultKeys  = childKeys("steps[].results[]", nil)
	resultOrder = resultKeys
	sandboxKeys = childKeys("steps[].sandbox", nil)
	reviewKeys  = childKeys("steps[].review", nil)
	// bodyKeys are the step keys beyond the head; each kind allows a subset (stepKeys).
	bodyKeys = slices.DeleteFunc(slices.Clone(stepOrder), func(k string) bool { return slices.Contains(headKeys, k) })
	stepKeys = func() map[string][]string {
		out := map[string][]string{}
		for _, kind := range kinds {
			out[kind] = slices.DeleteFunc(slices.Clone(bodyKeys), func(k string) bool {
				o := optionFor("steps[]." + k)
				return o.Kinds != nil && !slices.Contains(o.Kinds, kind)
			})
		}
		return out
	}()
)

func optionFor(path string) option {
	for _, o := range options {
		if o.Path == path {
			return o
		}
	}
	panic("adeflow: no option " + path)
}

// oneOf renders vals as "a, b or c".
func oneOf(vals []string) string {
	if len(vals) < 2 {
		return strings.Join(vals, "")
	}
	return strings.Join(vals[:len(vals)-1], ", ") + " or " + vals[len(vals)-1]
}
