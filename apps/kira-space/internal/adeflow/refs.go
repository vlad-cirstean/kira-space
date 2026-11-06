package adeflow

import (
	"fmt"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
)

// refOnlyKeys are the keys a workflow item that references a library step takes besides the step keys.
var refOnlyKeys = []string{"step", "routes", "prompt_extra"}

var (
	refKeys  = append(slices.Clone(stepOrder), refOnlyKeys...)
	refOrder = append([]string{"step"}, append(slices.Clone(stepOrder), "routes", "prompt_extra")...)
)

func hasKey(m *yaml.Node, key string) bool {
	i, _ := find(m, key)
	return i >= 0
}

// item reads one workflow step: a reference to a library step (`step:` key) or an inline step.
func (v *validator) item(sn *yaml.Node, sp string, earlier, candidates map[string]bool) (adewire.WorkflowStep, *resultNodes) {
	if sn.Kind == yaml.MappingNode && hasKey(sn, "step") {
		return v.reference(sn, sp, earlier, candidates)
	}
	return v.step(sn, sp, earlier, candidates)
}

// reference merges a library step with the item's overrides into one mapping and validates that.
func (v *validator) reference(sn *yaml.Node, sp string, earlier, candidates map[string]bool) (adewire.WorkflowStep, *resultNodes) {
	none := adewire.WorkflowStep{Results: []adewire.StepResult{}}
	fm := v.fields(sn, sp, refKeys...)
	libID := v.str(fm, sp, "step", true)
	if v.err == nil && !idPattern.MatchString(libID) {
		v.fail(fm["step"], sp, "step must name a library step id")
	}
	if n, has := fm["kind"]; has && v.err == nil {
		v.fail(n, sp, "kind comes from library step %q and cannot be overridden", libID)
	}
	if v.err != nil {
		return none, nil
	}
	var lib *yaml.Node
	if v.lookup != nil {
		var msg string
		if lib, msg = v.lookup(libID); lib == nil {
			v.fail(fm["step"], sp, "library step %q %s", libID, msg)
			return none, nil
		}
	} else {
		v.fail(fm["step"], sp, "library step %q does not exist", libID)
		return none, nil
	}
	merged := cloneNoLines(lib)
	v.overrides(merged, sn, fm, sp)
	if v.err != nil {
		return none, nil
	}
	extra := v.str(fm, sp, "prompt_extra", false)
	step, rn := v.step(merged, sp, earlier, candidates)
	if v.err != nil {
		v.markLibrary(sp, libID)
		return step, rn
	}
	if extra != "" && !(step.Kind == adewire.KindAgent && step.SmartScript == "" || step.Kind == adewire.KindManual && step.Session) {
		v.fail(fm["prompt_extra"], sp, "prompt_extra needs an agent step with a prompt or a manual step with a session")
		return step, rn
	}
	step.Use, step.PromptExtra = libID, extra
	return step, rn
}

// markLibrary names the library step in an error whose node came from the library file (line 0).
func (v *validator) markLibrary(sp, libID string) {
	if v.err.Line == 0 && len(v.err.Message) >= len(sp) && v.err.Message[:len(sp)] == sp {
		v.err.Message = sp + fmt.Sprintf("library step %q: ", libID) + v.err.Message[len(sp):]
	}
}

// overrides applies the item's keys onto the cloned library mapping.
func (v *validator) overrides(merged, item *yaml.Node, fm map[string]*yaml.Node, sp string) {
	idNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: strings.TrimSpace(fm["step"].Value), Line: fm["step"].Line}
	if n, has := fm["id"]; has {
		idNode = n
	}
	put(merged, "id", idNode, stepOrder)
	for i := 0; i+1 < len(item.Content); i += 2 {
		key, val := item.Content[i].Value, item.Content[i+1]
		switch {
		case slices.Contains(refOnlyKeys, key) || key == "id":
		case key == "params" || key == "review":
			mergeMap(merged, key, val, 1)
		case key == "settings":
			mergeMap(merged, key, val, 2)
		default:
			put(merged, key, val, stepOrder)
		}
	}
	reconcileBody(merged, fm)
	if rn, has := fm["routes"]; has {
		if _, both := fm["results"]; both {
			v.fail(rn, sp, "routes and results cannot both be set")
			return
		}
		v.applyRoutes(merged, rn, sp)
	}
}

// reconcileBody drops the library keys an override makes invalid: runs_on under outside_repo, and the
// prompt keys against the smart script keys.
func reconcileBody(merged *yaml.Node, fm map[string]*yaml.Node) {
	if n, has := fm["outside_repo"]; has && n.Value == "true" && fm["runs_on"] == nil {
		dropKeys(merged, "runs_on")
	}
	if n, has := fm["session"]; has && n.Value != "true" && fm["db_mcp"] == nil {
		dropKeys(merged, "db_mcp")
	}
	if fm["smart_script"] != nil {
		if fm["prompt"] == nil {
			dropKeys(merged, "prompt")
		}
		if fm["allowed_tools"] == nil {
			dropKeys(merged, "allowed_tools")
		}
		if fm["db_mcp"] == nil {
			dropKeys(merged, "db_mcp")
		}
	} else if fm["prompt"] != nil || fm["allowed_tools"] != nil {
		dropKeys(merged, "smart_script", "params")
	}
}

// mergeMap merges val into the mapping at key by key, depth levels deep; anything else replaces.
func mergeMap(m *yaml.Node, key string, val *yaml.Node, depth int) {
	_, cur := find(m, key)
	if cur == nil || cur.Kind != yaml.MappingNode || val.Kind != yaml.MappingNode {
		put(m, key, val, stepOrder)
		return
	}
	mergeInto(cur, val, depth)
}

func mergeInto(dst, src *yaml.Node, depth int) {
	for i := 0; i+1 < len(src.Content); i += 2 {
		k, v := src.Content[i], src.Content[i+1]
		_, cur := find(dst, k.Value)
		switch {
		case cur == nil:
			dst.Content = append(dst.Content, k, v)
		case depth > 1 && cur.Kind == yaml.MappingNode && v.Kind == yaml.MappingNode:
			mergeInto(cur, v, depth-1)
		default:
			put(dst, k.Value, v, nil)
		}
	}
}

// applyRoutes re-points the next of the library step's results. Missing results materialize the defaults.
func (v *validator) applyRoutes(merged, routes *yaml.Node, sp string) {
	if routes.Kind != yaml.MappingNode {
		v.fail(routes, sp, "routes must map result ids to next, end, stop or a step id")
		return
	}
	_, list := find(merged, "results")
	if list == nil {
		list = resultsList(adewire.DefaultResults())
		put(merged, "results", list, stepOrder)
	}
	for i := 0; i+1 < len(routes.Content); i += 2 {
		k, target := routes.Content[i], routes.Content[i+1]
		if target.Kind != yaml.ScalarNode || target.Value == "" {
			v.fail(target, sp, "route of %q must be next, end, stop or a step id", k.Value)
			return
		}
		var res *yaml.Node
		for _, item := range list.Content {
			if itemID(item) == k.Value {
				res = item
			}
		}
		if res == nil {
			v.fail(k, sp, "routes: the library step has no result %q", k.Value)
			return
		}
		put(res, "next", target, resultOrder)
	}
}

// resultsList builds a results sequence node from results.
func resultsList(results []adewire.StepResult) *yaml.Node {
	seq := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	for _, r := range results {
		item := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		applyResult(item, r)
		seq.Content = append(seq.Content, item)
	}
	return seq
}

// cloneNoLines deep-copies n with positions cleared, so errors in library content carry no line of the
// workflow file.
func cloneNoLines(n *yaml.Node) *yaml.Node {
	c := *n
	c.Line, c.Column = 0, 0
	c.Content = make([]*yaml.Node, len(n.Content))
	for i, ch := range n.Content {
		c.Content[i] = cloneNoLines(ch)
	}
	return &c
}

// --- library mode -------------------------------------------------------------------------------

// libraryKeys refuses the keys that belong to a workflow.
func (v *validator) libraryKeys(sm map[string]*yaml.Node, sp string) {
	if n, has := sm["skip"]; has {
		v.fail(n, sp, "skip is set in a workflow, not in a library step")
	}
}

// libraryRoutes refuses a result route to a step id: ids exist only inside a workflow.
func (v *validator) libraryRoutes(rn *resultNodes, sp string) {
	for i, r := range rn.list {
		rp := fmt.Sprintf("%sresult %d: ", sp, i+1)
		node := cmpNode(rn.next[i], rn.node)
		switch r.Next {
		case adewire.RouteNext, adewire.RouteEnd:
		case adewire.RouteStop:
			if r.OK {
				v.fail(node, rp, "stop is only allowed on a result that is not ok")
				return
			}
		default:
			v.fail(node, rp, "next must be next, end or stop in a library step; a workflow sets other routes with routes")
			return
		}
	}
}

// ParseStep decodes and validates one library step file. It returns the step and its mapping node.
func ParseStep(src []byte) (adewire.WorkflowStep, *yaml.Node, *adewire.WorkflowError) {
	var doc yaml.Node
	if err := yaml.Unmarshal(src, &doc); err != nil {
		return adewire.WorkflowStep{}, nil, &adewire.WorkflowError{Line: yamlErrLineOf(err), Message: syntaxMessage}
	}
	if doc.Kind == 0 || len(doc.Content) == 0 {
		return adewire.WorkflowStep{}, nil, &adewire.WorkflowError{Message: "the step file is empty"}
	}
	v := &validator{library: true}
	step, _ := v.step(doc.Content[0], "", map[string]bool{}, map[string]bool{})
	if v.err != nil {
		return adewire.WorkflowStep{}, nil, v.err
	}
	return step, doc.Content[0], nil
}
