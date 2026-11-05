package adeflow

import (
	"reflect"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
)

// override is one key a workflow item sets on top of its library step.
type override struct {
	key  string
	node *yaml.Node
}

// referencedSteps resolves the library step of every reference in wf; nil entries are inline steps.
func referencedSteps(wf adewire.Workflow, lib *StepLibrary) ([]*adewire.WorkflowStep, error) {
	out := make([]*adewire.WorkflowStep, len(wf.Steps))
	for i, st := range wf.Steps {
		if st.Use == "" {
			continue
		}
		ls, ok := lib.Get(st.Use)
		if !ok {
			return nil, invalidf("step %d: library step %q does not exist", i+1, st.Use)
		}
		if ls.Kind != st.Kind {
			return nil, invalidf("step %d: the kind of library step %q cannot be changed", i+1, st.Use)
		}
		out[i] = &ls
	}
	return out, nil
}

// applyRef writes a reference item: `step`, a differing id, and only the keys whose value differs from
// the library step. Keys already equal in the file stay untouched, comments included.
func applyRef(m *yaml.Node, s, lib adewire.WorkflowStep) {
	setScalar(m, "step", kText, s.Use, true, refOrder)
	if s.ID != s.Use {
		setScalar(m, "id", kText, s.ID, true, refOrder)
	} else {
		dropKeys(m, "id")
	}
	want := refOverrides(s, lib)
	keep := map[string]bool{"step": true, "id": true}
	for _, w := range want {
		keep[w.key] = true
	}
	dropOthers(m, keep)
	for _, w := range want {
		if _, cur := find(m, w.key); cur != nil && sameNode(cur, w.node) {
			continue
		}
		put(m, w.key, w.node, refOrder)
	}
}

// refOverrides lists the keys of s that differ from lib, in file order.
func refOverrides(s, lib adewire.WorkflowStep) []override {
	ms, ml := encodedStep(s), encodedStep(lib)
	var out []override
	for _, k := range stepOrder {
		switch k {
		case "id", "kind", "skip", "results", "review":
			continue
		}
		_, ns := find(ms, k)
		_, nl := find(ml, k)
		switch {
		case ns != nil && (nl == nil || !sameNode(ns, nl)):
			out = append(out, override{k, ns})
		case ns == nil && nl != nil:
			if off := explicitOff(k, s); off != nil {
				out = append(out, override{k, off})
			}
		}
	}
	if s.Skip {
		out = append(out, override{"skip", newScalar(kBool, "true")})
	}
	if rv := reviewOverride(s, lib); rv != nil {
		out = append(out, override{"review", rv})
	}
	out = append(out, routeOverride(s, lib)...)
	if s.PromptExtra != "" {
		out = append(out, override{"prompt_extra", newScalar(kBlock, s.PromptExtra)})
	}
	return sortOverrides(out)
}

// sortOverrides puts overrides in refOrder.
func sortOverrides(in []override) []override {
	out := make([]override, 0, len(in))
	for _, k := range refOrder {
		for _, o := range in {
			if o.key == k {
				out = append(out, o)
			}
		}
	}
	return out
}

func encodedStep(s adewire.WorkflowStep) *yaml.Node {
	m := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	applyStep(m, s)
	return m
}

// explicitOff is the node that restores a library value s no longer spells out: a default the writer
// otherwise omits. nil when the key cannot be unset.
func explicitOff(key string, s adewire.WorkflowStep) *yaml.Node {
	switch key {
	case "before":
		return newScalar(kText, "auto")
	case "max_returns":
		return newScalar(kInt, strconv.Itoa(s.MaxReturns))
	case "outside_repo", "session":
		return newScalar(kBool, "false")
	case "allowed_tools":
		return scalarList(nil)
	case "sandbox":
		m := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		put(m, "access", newScalar(kText, s.Sandbox.NormalizedFor(s.Kind).Access), nil)
		return m
	}
	return nil
}

// reviewOverride is the review block keys that differ from the library step; nil when none.
func reviewOverride(s, lib adewire.WorkflowStep) *yaml.Node {
	if s.Kind != adewire.KindReview {
		return nil
	}
	rs, rl := defaultReview(s.Review), defaultReview(lib.Review)
	m := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	if rs.SendTo != rl.SendTo {
		put(m, "send_to", newScalar(kText, defaultStr(rs.SendTo, adewire.ReviewSendImplementer)), nil)
	}
	if rs.Implementer != "" {
		put(m, "implementer", newScalar(kText, rs.Implementer), nil)
	}
	if len(m.Content) == 0 {
		return nil
	}
	return m
}

// routeOverride is `routes` when only result targets differ from the library step, `results` when more does.
func routeOverride(s, lib adewire.WorkflowStep) []override {
	if s.Kind == adewire.KindManual {
		return nil
	}
	rs, rl := resultsOrDefault(s.Results), resultsOrDefault(lib.Results)
	if sameResults(rs, rl) {
		return nil
	}
	if routes := routesOnly(rs, rl); routes != nil {
		return []override{{"routes", routes}}
	}
	return []override{{"results", resultsList(rs)}}
}

func resultsOrDefault(r []adewire.StepResult) []adewire.StepResult {
	if len(r) == 0 {
		return adewire.DefaultResults()
	}
	return r
}

// routesOnly maps result id to target when rs equals rl in everything but the targets; nil otherwise.
func routesOnly(rs, rl []adewire.StepResult) *yaml.Node {
	if len(rs) != len(rl) {
		return nil
	}
	m := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	for i := range rs {
		a, b := rs[i], rl[i]
		if a.ID != b.ID || a.OK != b.OK || a.Description != b.Description {
			return nil
		}
		if next := defaultStr(a.Next, adewire.DefaultRoute(a.OK)); next != defaultStr(b.Next, adewire.DefaultRoute(b.OK)) {
			m.Content = append(m.Content, newScalar(kText, a.ID), newScalar(kText, next))
		}
	}
	return m
}

// sameNode compares two nodes by decoded value; a trailing newline of a text value does not count.
func sameNode(a, b *yaml.Node) bool {
	var x, y any
	if a.Decode(&x) != nil || b.Decode(&y) != nil {
		return false
	}
	return reflect.DeepEqual(normalize(x), normalize(y))
}

func normalize(v any) any {
	switch t := v.(type) {
	case string:
		return strings.TrimRight(t, "\n")
	case []any:
		out := make([]any, len(t))
		for i, e := range t {
			out[i] = normalize(e)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, e := range t {
			out[k] = normalize(e)
		}
		return out
	}
	return v
}
