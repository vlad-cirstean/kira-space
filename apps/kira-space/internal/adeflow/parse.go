// Package adeflow reads and writes ADE v2 workflow YAML files in <KiraSpaceHome>/workflows. It
// validates strictly (an unknown key is an error) and reports node line numbers. The Reader lists;
// writer.go edits files in place, preserving comments and key order.
package adeflow

import (
	"fmt"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"go.yaml.in/yaml/v3"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/internal/harness/providers"
	"github.com/kirathecat/kira-studio/packages/theme/src/i18n"
)

const maxSmartScriptName = 200

// maxStatusID is the longest status id.
const maxStatusID = 40

const syntaxMessage = "unexpected content here (check the indentation)"

var (
	idPattern        = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)
	paramNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)
	toolPattern      = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]*(\(.+\))?$`)
	mcpToolPattern   = regexp.MustCompile(`^mcp__[A-Za-z0-9_-]+__[A-Za-z0-9_-]+$`)
	yamlErrLine      = regexp.MustCompile(`line (\d+)`)
	maxStepTimeout   = 24 * time.Hour
)

// stagesRemoved is the error for a file in the pre-P293 format.
const stagesRemoved = `"stages" is no longer supported: list every step under "steps" (see the Agents docs)`

// Parse decodes and validates one workflow file. src is the whole file; the file stem is checked
// separately by the Reader (Parse has no file name).
func Parse(src []byte) (adewire.Workflow, *adewire.WorkflowError) {
	var doc yaml.Node
	if err := yaml.Unmarshal(src, &doc); err != nil {
		line := 0
		if m := yamlErrLine.FindStringSubmatch(err.Error()); m != nil {
			line, _ = strconv.Atoi(m[1])
		}
		return adewire.Workflow{}, &adewire.WorkflowError{Line: line, Message: syntaxMessage}
	}
	if doc.Kind == 0 || len(doc.Content) == 0 {
		return adewire.Workflow{}, &adewire.WorkflowError{Message: i18n.T("backend.space.ade.workflowEmpty.body", nil)}
	}
	v := &validator{}
	wf := v.workflow(doc.Content[0])
	if v.err != nil {
		return adewire.Workflow{}, v.err
	}
	return wf, nil
}

// validator keeps the first error; every rule short-circuits on it.
type validator struct {
	err *adewire.WorkflowError
}

func (v *validator) fail(n *yaml.Node, prefix, format string, args ...any) {
	if v.err != nil {
		return
	}
	line := 0
	if n != nil {
		line = n.Line
	}
	v.err = &adewire.WorkflowError{Line: line, Message: prefix + fmt.Sprintf(format, args...)}
}

// fields maps a mapping node's keys to their value nodes, rejecting duplicates, non-scalar keys and
// keys outside allowed.
func (v *validator) fields(n *yaml.Node, prefix string, allowed ...string) map[string]*yaml.Node {
	out := make(map[string]*yaml.Node)
	if n.Kind != yaml.MappingNode {
		v.fail(n, prefix, "expected a mapping of keys")
		return out
	}
	ok := make(map[string]bool, len(allowed))
	for _, a := range allowed {
		ok[a] = true
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		k, val := n.Content[i], n.Content[i+1]
		if k.Kind != yaml.ScalarNode {
			v.fail(k, prefix, "keys must be plain words")
			return out
		}
		if !ok[k.Value] {
			v.fail(k, prefix, "unknown key %q", k.Value)
			return out
		}
		if _, dup := out[k.Value]; dup {
			v.fail(k, prefix, "duplicate key %q", k.Value)
			return out
		}
		out[k.Value] = val
	}
	return out
}

// str returns a scalar string field; required = fail when missing or empty.
func (v *validator) str(m map[string]*yaml.Node, prefix, key string, required bool) string {
	n, ok := m[key]
	if !ok {
		if required {
			v.failMissing(prefix, key)
		}
		return ""
	}
	if n.Kind != yaml.ScalarNode || n.Tag == "!!null" {
		v.fail(n, prefix, "%s must be text", key)
		return ""
	}
	val := n.Value
	if key == "prompt" || key == "command" {
		val = strings.TrimRight(val, "\n")
	} else {
		val = strings.TrimSpace(val)
	}
	if required && strings.TrimSpace(val) == "" {
		v.fail(n, prefix, "%s must not be empty", key)
	}
	return val
}

// failMissing reports a missing required key with line 0 (no node to point at).
func (v *validator) failMissing(prefix, key string) {
	v.fail(nil, prefix, "%s is required", key)
}

func (v *validator) boolean(m map[string]*yaml.Node, prefix, key string) bool {
	n, ok := m[key]
	if !ok {
		return false
	}
	if n.Kind != yaml.ScalarNode || n.Tag != "!!bool" {
		v.fail(n, prefix, "%s must be true or false", key)
		return false
	}
	return n.Value == "true"
}

func (v *validator) workflow(root *yaml.Node) adewire.Workflow {
	wf := adewire.Workflow{Steps: make([]adewire.WorkflowStep, 0)}
	if root.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(root.Content); i += 2 {
			if k := root.Content[i]; k.Kind == yaml.ScalarNode && k.Value == "stages" {
				v.fail(k, "", "%s", stagesRemoved)
				return wf
			}
		}
	}
	m := v.fields(root, "", topKeys...)
	if v.err != nil {
		return wf
	}
	wf.ID = v.str(m, "", "id", true)
	if v.err == nil && !idPattern.MatchString(wf.ID) {
		v.fail(m["id"], "", "id must be lowercase letters, digits, - or _")
	}
	wf.Name = v.str(m, "", "name", true)
	v.boolean(m, "", "kira_space_mcp") // legacy switch (P408): still valid, ignored, dropped on save
	if v.err != nil {
		return wf
	}
	list, ok := m["steps"]
	if !ok {
		v.failMissing("", "steps")
		return wf
	}
	if list.Kind != yaml.SequenceNode || len(list.Content) == 0 {
		v.fail(list, "", "steps must be a non-empty list")
		return wf
	}
	earlier := make(map[string]bool)
	candidates := make(map[string]bool) // earlier steps a review may resume: agent prompt steps only
	var nodes []*resultNodes
	var prefixes []string
	for i, sn := range list.Content {
		sp := fmt.Sprintf("step %d: ", i+1)
		step, rn := v.step(sn, sp, earlier, candidates)
		if v.err != nil {
			return wf
		}
		earlier[step.ID] = true
		if step.Kind == adewire.KindAgent && step.SmartScript == "" {
			candidates[step.ID] = true
		}
		wf.Steps = append(wf.Steps, step)
		nodes = append(nodes, rn)
		prefixes = append(prefixes, sp)
	}
	if !slices.ContainsFunc(wf.Steps, func(s adewire.WorkflowStep) bool { return !s.Skip }) {
		v.fail(list, "", "at least one step must not be skipped")
		return wf
	}
	v.resolveRoutes(wf.Steps, nodes, prefixes)
	return wf
}

// step reads one step. It returns the step and its declared result nodes (nil when it declares none).
func (v *validator) step(sn *yaml.Node, sp string, earlier, candidates map[string]bool) (adewire.WorkflowStep, *resultNodes) {
	step := adewire.WorkflowStep{Results: []adewire.StepResult{}, AllowedTools: make([]string, 0), Params: map[string][]string{}, Sandbox: adewire.DefaultSandbox(), Settings: map[string]map[string]string{}}
	sm := v.fields(sn, sp, stepOrder...)
	if v.err != nil {
		return step, nil
	}
	if !v.stepHead(&step, sm, sp, earlier) {
		return step, nil
	}
	step.Status = v.status(sm, sp)
	step.Skip = v.boolean(sm, sp, "skip")
	if v.err != nil {
		return step, nil
	}
	if step.Kind == adewire.KindManual {
		v.manualStep(&step, sm, sp)
		return step, nil
	}
	step.RunsOn = v.runsOn(sm, sp)
	step.Before = v.before(sm, sp)
	var rn *resultNodes
	if n, has := sm["results"]; has {
		rn = v.results(n, sp)
		if v.err == nil && step.Kind == adewire.KindScript {
			v.scriptResults(rn, sp)
		}
		step.Results = rn.list
	} else {
		step.Results = adewire.DefaultResults()
	}
	step.Timeout = v.timeout(sm, sp)
	switch step.Kind {
	case adewire.KindScript:
		step.Command = v.str(sm, sp, "command", true)
	case adewire.KindReview:
		if rv, has := sm["review"]; has {
			step.Review = v.reviewConfig(rv, sp, candidates)
		} else {
			step.Review = &adewire.ReviewStepConfig{SendTo: adewire.ReviewSendImplementer}
		}
	default:
		v.agentBody(sm, sp, &step)
	}
	v.maxReturns(sm, sp, &step)
	if n, has := sm["sandbox"]; has && v.err == nil {
		step.Sandbox = v.sandbox(n, sp)
	}
	if n, has := sm["settings"]; has && v.err == nil {
		step.Settings = v.settings(n, sp)
	}
	return step, rn
}

// stepHead reads id, name and kind and rejects keys the kind disallows. It reports whether the step may go on.
func (v *validator) stepHead(step *adewire.WorkflowStep, sm map[string]*yaml.Node, sp string, earlier map[string]bool) bool {
	step.ID = v.str(sm, sp, "id", true)
	if v.err == nil && !idPattern.MatchString(step.ID) {
		v.fail(sm["id"], sp, "id must be lowercase letters, digits, - or _")
	}
	if v.err == nil && earlier[step.ID] {
		v.fail(sm["id"], sp, "id %q is already used by another step", step.ID)
	}
	step.Name = v.str(sm, sp, "name", true)
	step.Kind = v.str(sm, sp, "kind", true)
	if v.err != nil {
		return false
	}
	allowed, known := stepKeys[step.Kind]
	if !known {
		v.fail(sm["kind"], sp, "kind must be %s", oneOf(kinds))
		return false
	}
	for _, k := range bodyKeys {
		if n, has := sm[k]; has && !slices.Contains(allowed, k) {
			v.fail(n, sp, "%s is not allowed on %s steps", k, step.Kind)
			return false
		}
	}
	return true
}

// manualStep reads the session, prompt and settings of a manual step.
func (v *validator) manualStep(step *adewire.WorkflowStep, sm map[string]*yaml.Node, sp string) {
	step.Session = v.boolean(sm, sp, "session")
	if v.err != nil {
		return
	}
	if step.Session {
		step.Prompt = v.str(sm, sp, "prompt", false)
	} else if n, has := sm["prompt"]; has {
		v.fail(n, sp, "prompt is not allowed on %s steps without a session", step.Kind)
	}
	if n, has := sm["settings"]; has && v.err == nil {
		if step.Session {
			step.Settings = v.settings(n, sp)
		} else {
			v.fail(n, sp, "settings is not allowed on %s steps without a session", step.Kind)
		}
	}
}

// legacyStatuses maps the names files used before statuses became ids to the ids the app first shipped;
// a file-format compatibility shim, not a status rule.
var legacyStatuses = map[string]string{"To do": "todo", "In progress": "in-progress", "In review": "in-review", "Done": "done"}

// status reads a step's task status id; absent, it is "" and the default of the step's kind applies.
// Membership in the user's set is checked at run time, so a file loads without the DB.
func (v *validator) status(sm map[string]*yaml.Node, sp string) string {
	if _, has := sm["status"]; !has {
		return ""
	}
	val := v.str(sm, sp, "status", true)
	if id, legacy := legacyStatuses[val]; legacy {
		return id
	}
	if v.err == nil && (!idPattern.MatchString(val) || len(val) > maxStatusID) {
		v.fail(sm["status"], sp, "status must be a status id")
	}
	return val
}

// scriptResults refuses script results other than done (ok) and failed (not ok).
func (v *validator) scriptResults(rn *resultNodes, sp string) {
	if len(rn.list) == 2 {
		d, f := rn.list[0], rn.list[1]
		if d.ID == adewire.ResultDone && d.OK && f.ID == adewire.ResultFailed && !f.OK {
			return
		}
	}
	v.fail(rn.node, sp, "results of a script step must be done (ok: true) and failed (ok: false), in that order")
}

// maxReturns reads a step's return limit into step.MaxReturns. Absent, it is the default for the kind.
func (v *validator) maxReturns(sm map[string]*yaml.Node, sp string, step *adewire.WorkflowStep) {
	review := step.Kind == adewire.KindReview
	n, has := sm["max_returns"]
	if !has {
		step.MaxReturns = adewire.DefaultLimit(review)
		return
	}
	lo, rule := 1, "from 1 to %d"
	if review {
		lo, rule = 0, "from 0 to %d on a review step"
	}
	val, err := strconv.Atoi(n.Value)
	if n.Kind != yaml.ScalarNode || err != nil || val < lo || val > adewire.MaxMaxReturns {
		v.fail(n, sp, "max_returns must be a whole number "+rule, adewire.MaxMaxReturns)
		return
	}
	step.MaxReturns = val
}

// before reads a step's approval gate; the default is auto.
func (v *validator) before(sm map[string]*yaml.Node, sp string) string {
	if _, has := sm["before"]; !has {
		return beforeValues[0]
	}
	b := v.str(sm, sp, "before", true)
	if v.err == nil && !slices.Contains(beforeValues, b) {
		v.fail(sm["before"], sp, "before must be %s", oneOf(beforeValues))
	}
	return b
}

// agentBody reads what an agent step runs: a prompt with its allowed tools, or a smart script with its params.
func (v *validator) agentBody(sm map[string]*yaml.Node, sp string, step *adewire.WorkflowStep) {
	if _, smart := sm["smart_script"]; !smart {
		if n, has := sm["params"]; has {
			v.fail(n, sp, "params is only allowed with smart_script")
			return
		}
		step.Prompt = v.str(sm, sp, "prompt", true)
		step.AllowedTools = v.allowedTools(sm, sp)
		return
	}
	for _, key := range []string{"prompt", "allowed_tools"} {
		if n, has := sm[key]; has {
			v.fail(n, sp, "%s is not allowed with smart_script", key)
			return
		}
	}
	step.SmartScript = v.str(sm, sp, "smart_script", true)
	if v.err == nil && utf8.RuneCountInString(step.SmartScript) > maxSmartScriptName {
		v.fail(sm["smart_script"], sp, "smart_script must be at most %d characters", maxSmartScriptName)
	}
	step.Params = v.stepParams(sm, sp)
}

const (
	maxSandboxPaths = 32
	maxSandboxPath  = 4096
)

// sandbox reads a step's sandbox block. An empty mapping is the default.
func (v *validator) sandbox(n *yaml.Node, sp string) adewire.StepSandbox {
	sb := adewire.DefaultSandbox()
	if n.Kind != yaml.MappingNode {
		v.fail(n, sp, "sandbox must be a mapping")
		return sb
	}
	m := v.fields(n, sp, sandboxKeys...)
	if v.err != nil {
		return sb
	}
	if _, has := m["access"]; has {
		sb.Access = v.str(m, sp, "access", true)
		if v.err == nil && !slices.Contains(accessValues, sb.Access) {
			v.fail(m["access"], sp, "access must be %s", oneOf(accessValues))
		}
	}
	sb.Read = v.sandboxPaths(m, sp, "read")
	sb.Allow = v.sandboxPaths(m, sp, "allow")
	if _, has := m["network"]; has {
		sb.Network = v.boolean(m, sp, "network")
	}
	if v.err == nil && sb.Access == adewire.SandboxOff && (len(sb.Read) > 0 || len(sb.Allow) > 0 || m["network"] != nil) {
		v.fail(n, sp, "sandbox off takes no other keys")
	}
	return sb
}

// stageCatalog is every registered agent's stage settings. The set is static, so parsing needs no registry.
var stageCatalog = providers.StageCatalog()

// settings reads a step's settings block: agent id, then setting key, then value. An empty block reads as none.
func (v *validator) settings(n *yaml.Node, sp string) map[string]map[string]string {
	out := map[string]map[string]string{}
	if n.Kind != yaml.MappingNode {
		v.fail(n, sp, "settings must be a mapping")
		return out
	}
	ids := make([]string, len(stageCatalog))
	for i, set := range stageCatalog {
		ids[i] = set.ID
	}
	if k := unknownKey(n, ids); k != nil {
		v.fail(k, sp, "settings: unknown agent %q (known: %s)", k.Value, strings.Join(ids, ", "))
		return out
	}
	agents := v.fields(n, sp, ids...)
	if v.err != nil {
		return out
	}
	for _, set := range stageCatalog {
		an, has := agents[set.ID]
		if !has {
			continue
		}
		if an.Kind != yaml.MappingNode {
			v.fail(an, sp, "settings.%s must be a mapping", set.ID)
			return out
		}
		keys := make([]string, len(set.Settings))
		for i, s := range set.Settings {
			keys[i] = s.Key
		}
		if k := unknownKey(an, keys); k != nil {
			v.fail(k, sp, "settings.%s: unknown setting %q", set.ID, k.Value)
			return out
		}
		vals := v.fields(an, sp, keys...)
		if v.err != nil {
			return out
		}
		for _, s := range set.Settings {
			vn, has := vals[s.Key]
			if !has {
				continue
			}
			val, ok := "", false
			if vn.Kind == yaml.ScalarNode {
				val, ok = s.Parse(vn.Value)
			}
			if !ok {
				v.fail(vn, sp, "settings.%s.%s must be a whole number from %d to %d", set.ID, s.Key, s.Min, s.Max)
				return out
			}
			if out[set.ID] == nil {
				out[set.ID] = map[string]string{}
			}
			out[set.ID][s.Key] = val
		}
	}
	return out
}

// unknownKey returns the first plain-word key of mapping n outside allowed, nil when none.
func unknownKey(n *yaml.Node, allowed []string) *yaml.Node {
	for i := 0; i+1 < len(n.Content); i += 2 {
		if k := n.Content[i]; k.Kind == yaml.ScalarNode && !slices.Contains(allowed, k.Value) {
			return k
		}
	}
	return nil
}

// sandboxPaths reads a list of absolute or ~/ paths.
func (v *validator) sandboxPaths(m map[string]*yaml.Node, sp, key string) []string {
	out := make([]string, 0)
	n, has := m[key]
	if !has {
		return out
	}
	if n.Kind != yaml.SequenceNode {
		v.fail(n, sp, "%s must be a list of paths", key)
		return out
	}
	if len(n.Content) > maxSandboxPaths {
		v.fail(n, sp, "%s has too many entries: at most %d", key, maxSandboxPaths)
		return out
	}
	for _, e := range n.Content {
		if e.Kind != yaml.ScalarNode || e.Tag == "!!null" {
			v.fail(e, sp, "%s entries must be text", key)
			return out
		}
		switch p := e.Value; {
		case len(p) > maxSandboxPath || strings.ContainsAny(p, "\x00\r\n"):
			v.fail(e, sp, "path %q is not allowed", truncate(p, 80))
		case p != "~" && !strings.HasPrefix(p, "~/") && !strings.HasPrefix(p, "/"):
			v.fail(e, sp, "path %q must be absolute or start with ~/", p)
		case protectedSandboxPath(p):
			v.fail(e, sp, "path %q is not allowed", p)
		default:
			out = append(out, p)
		}
		if v.err != nil {
			return out
		}
	}
	return out
}

// protectedSandboxPath reports whether p is the root, the home folder or an ancestor of it, a
// folder holding an agent's config or the keychain directory, or inside the keychain directory.
func protectedSandboxPath(p string) bool {
	c := path.Clean(p)
	switch c {
	case "/", "~", "~/Library", ".":
		return true
	}
	if slices.Contains(agentConfigDirs, strings.TrimPrefix(c, "~/")) && strings.HasPrefix(c, "~/") {
		return true
	}
	return strings.HasPrefix(c, "..") || strings.HasPrefix(c+"/", "~/Library/Keychains/") || protectedAbsoluteHome(c)
}

// agentConfigDirs are the home config folders of the registered agents.
var agentConfigDirs = providers.ConfigDirs()

// protectedAbsoluteHome catches the absolute spelling of the protected paths: /Users or /home, a
// home folder under it, its Library and agent config folders, and the keychain directory.
func protectedAbsoluteHome(c string) bool {
	seg := strings.Split(strings.TrimPrefix(c, "/"), "/")
	if !strings.HasPrefix(c, "/") || (seg[0] != "Users" && seg[0] != "home") {
		return false
	}
	switch {
	case len(seg) <= 2:
		return true
	case len(seg) == 3:
		return seg[2] == "Library" || slices.Contains(agentConfigDirs, seg[2])
	}
	return seg[2] == "Library" && seg[3] == "Keychains"
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// reviewConfig reads a review step's block. An empty mapping takes the defaults.
func (v *validator) reviewConfig(n *yaml.Node, sp string, candidates map[string]bool) *adewire.ReviewStepConfig {
	if n.Kind != yaml.MappingNode {
		v.fail(n, sp, "review must be a mapping")
		return nil
	}
	m := v.fields(n, sp, reviewKeys...)
	if v.err != nil {
		return nil
	}
	cfg := &adewire.ReviewStepConfig{SendTo: adewire.ReviewSendImplementer}
	if _, has := m["send_to"]; has {
		cfg.SendTo = v.str(m, sp, "send_to", true)
		if v.err == nil && !slices.Contains(sendToValues, cfg.SendTo) {
			v.fail(m["send_to"], sp, "send_to must be %s", oneOf(sendToValues))
		}
	}
	if _, has := m["implementer"]; has {
		cfg.Implementer = v.str(m, sp, "implementer", true)
		if v.err == nil && !candidates[cfg.Implementer] {
			v.fail(m["implementer"], sp, "implementer must be an earlier agent step")
		}
	}
	return cfg
}

// stepParams reads params: a mapping from a param name to one text value or a list of them.
func (v *validator) stepParams(sm map[string]*yaml.Node, sp string) map[string][]string {
	out := map[string][]string{}
	n, has := sm["params"]
	if !has {
		return out
	}
	if n.Kind != yaml.MappingNode {
		v.fail(n, sp, "params must be a mapping of names to values")
		return out
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		k, val := n.Content[i], n.Content[i+1]
		if k.Kind != yaml.ScalarNode || !paramNamePattern.MatchString(k.Value) {
			v.fail(k, sp, "param name %q must be lowercase letters, digits or _, starting with a letter", k.Value)
			return out
		}
		if _, dup := out[k.Value]; dup {
			v.fail(k, sp, "duplicate param %q", k.Value)
			return out
		}
		vals, ok := scalarValues(val)
		if !ok {
			v.fail(val, sp, "param %q must be text or a list of text", k.Value)
			return out
		}
		out[k.Value] = vals
	}
	return out
}

// scalarValues is a scalar as one value, or a sequence of scalars; ok is false for anything else.
func scalarValues(n *yaml.Node) ([]string, bool) {
	isText := func(c *yaml.Node) bool { return c.Kind == yaml.ScalarNode && c.Tag != "!!null" }
	switch {
	case isText(n):
		return []string{n.Value}, true
	case n.Kind == yaml.SequenceNode:
		out := make([]string, 0, len(n.Content))
		for _, c := range n.Content {
			if !isText(c) {
				return nil, false
			}
			out = append(out, c.Value)
		}
		return out, true
	}
	return nil, false
}

// runsOn: once | each repo | only <repo> (syntax only; repos are checked at run time).
func (v *validator) runsOn(m map[string]*yaml.Node, prefix string) string {
	val := v.str(m, prefix, "runs_on", true)
	if v.err != nil {
		return val
	}
	switch {
	case val == "once", val == "each repo":
	case strings.HasPrefix(val, "only ") && strings.TrimSpace(strings.TrimPrefix(val, "only ")) != "":
		val = "only " + strings.TrimSpace(strings.TrimPrefix(val, "only "))
	default:
		v.fail(m["runs_on"], prefix, "runs_on must be once, each repo or only <repo>")
	}
	return val
}

func (v *validator) timeout(m map[string]*yaml.Node, prefix string) string {
	val := v.str(m, prefix, "timeout", true)
	if v.err != nil {
		return val
	}
	d, err := time.ParseDuration(val)
	if err != nil || d <= 0 || d > maxStepTimeout {
		v.fail(m["timeout"], prefix, "timeout must be a duration such as 30m or 2h (at most 24h)")
	}
	return val
}

func (v *validator) allowedTools(m map[string]*yaml.Node, prefix string) []string {
	out := make([]string, 0)
	n, has := m["allowed_tools"]
	if !has {
		return out
	}
	if n.Kind != yaml.SequenceNode {
		v.fail(n, prefix, "allowed_tools must be a list")
		return out
	}
	seen := make(map[string]bool)
	for _, e := range n.Content {
		if e.Kind != yaml.ScalarNode || strings.ContainsAny(e.Value, "\r\n") || !(toolPattern.MatchString(e.Value) || mcpToolPattern.MatchString(e.Value)) {
			v.fail(e, prefix, "allowed_tools entry %q is not a tool pattern such as Bash(git *)", e.Value)
			return out
		}
		if seen[e.Value] {
			v.fail(e, prefix, "allowed_tools entry %q is listed twice", e.Value)
			return out
		}
		seen[e.Value] = true
		out = append(out, e.Value)
	}
	return out
}

const (
	maxResults           = 12
	maxResultDescription = 200
)

// resultNodes keeps the YAML nodes of a step's declared results so routes can be checked once every
// step id of the workflow is known.
type resultNodes struct {
	list []adewire.StepResult
	next []*yaml.Node
	node *yaml.Node
}

// results reads a step's results list; routes are resolved later by resolveRoutes.
func (v *validator) results(n *yaml.Node, sp string) *resultNodes {
	rn := &resultNodes{node: n, list: make([]adewire.StepResult, 0)}
	if n.Kind != yaml.SequenceNode || len(n.Content) == 0 || len(n.Content) > maxResults {
		v.fail(n, sp, "results must be a list of 1 to %d results", maxResults)
		return rn
	}
	seen := make(map[string]bool)
	for i, item := range n.Content {
		rp := fmt.Sprintf("%sresult %d: ", sp, i+1)
		m := v.fields(item, rp, resultKeys...)
		if v.err != nil {
			return rn
		}
		r := adewire.StepResult{ID: v.str(m, rp, "id", true)}
		if v.err == nil {
			switch {
			case !idPattern.MatchString(r.ID):
				v.fail(m["id"], rp, "id must be lowercase letters, digits, - or _")
			case r.ID == "needs_input":
				v.fail(m["id"], rp, "id needs_input is reserved")
			case seen[r.ID]:
				v.fail(m["id"], rp, "id %q is already used by another result of this step", r.ID)
			}
		}
		seen[r.ID] = true
		if _, has := m["ok"]; !has {
			v.failMissing(rp, "ok")
		}
		r.OK = v.boolean(m, rp, "ok")
		r.Description = v.str(m, rp, "description", false)
		if v.err == nil && (strings.ContainsAny(r.Description, "\r\n") || utf8.RuneCountInString(r.Description) > maxResultDescription) {
			v.fail(m["description"], rp, "description must be one line of at most %d characters", maxResultDescription)
		}
		r.Next = adewire.DefaultRoute(r.OK)
		if _, has := m["next"]; has {
			r.Next = v.str(m, rp, "next", true)
		}
		if v.err != nil {
			return rn
		}
		rn.list = append(rn.list, r)
		rn.next = append(rn.next, m["next"])
	}
	return rn
}

// resolveRoutes checks every declared result's route against the workflow's step ids and marks returns.
func (v *validator) resolveRoutes(steps []adewire.WorkflowStep, nodes []*resultNodes, prefixes []string) {
	if v.err != nil {
		return
	}
	index := make(map[string]int, len(steps))
	for i, s := range steps {
		index[s.ID] = i
	}
	for si, rn := range nodes {
		if rn == nil {
			continue
		}
		if !v.resolveStepRoutes(steps, index, si, rn, prefixes[si]) {
			return
		}
	}
}

// resolveStepRoutes resolves one step's results; false means a violation was recorded.
func (v *validator) resolveStepRoutes(steps []adewire.WorkflowStep, index map[string]int, si int, rn *resultNodes, sp string) bool {
	forward := false
	for ri := range rn.list {
		r := &rn.list[ri]
		rp := fmt.Sprintf("%sresult %d: ", sp, ri+1)
		node := cmpNode(rn.next[ri], rn.node)
		loop := false
		switch r.Next {
		case adewire.RouteNext, adewire.RouteEnd:
		case adewire.RouteStop:
			if r.OK {
				v.fail(node, rp, "stop is only allowed on a result that is not ok")
				return false
			}
		default:
			target, known := index[r.Next]
			if !known {
				v.fail(node, rp, "next must be next, end, stop or a step id of this workflow")
				return false
			}
			loop = target <= si
			if loop && (steps[target].Kind == adewire.KindManual || steps[target].Skip) {
				v.fail(node, rp, "next cannot return to a manual or skipped step, which never runs")
				return false
			}
		}
		forward = forward || !loop
		r.Loop = loop
	}
	if !forward {
		v.fail(rn.node, sp, "at least one result must lead forward, or the step can never finish")
		return false
	}
	steps[si].Results = rn.list
	return true
}

func cmpNode(n, fallback *yaml.Node) *yaml.Node {
	if n != nil {
		return n
	}
	return fallback
}
