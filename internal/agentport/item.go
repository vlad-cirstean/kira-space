package agentport

import (
	"encoding/json"
	"reflect"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Item states.
const (
	StateNew      = "new"
	StateSame     = "same"
	StateConflict = "conflict"
	StateSkipped  = "skipped"
)

// Item is one thing an import would bring in. It never carries a credential value: Text and Current
// show secrets as ${secret:name} references.
type Item struct {
	// ID is "mcp/gh", "env/JIRA_TOKEN", "allow/Bash(git status)", "deny/...", "hook/<name>",
	// "claude.hook/<name>", "claude/model", "claude/permission_mode", "claude/status_line",
	// "opencode/model", "opencode/plugin/<url>", "context" or "skip/<key>".
	ID   string `json:"id"`
	Kind string `json:"kind"`
	// Source is the display path of the file the item came from.
	Source string `json:"source"`
	State  string `json:"state"`
	Reason string `json:"reason"`
	// Modes lists the apply modes of a conflicting context item: replace, append.
	Modes   []string     `json:"modes"`
	Text    string       `json:"text"`
	Current string       `json:"current"`
	Secrets []SecretSlot `json:"secrets"`

	ops     []op
	pending []pendingSecret
	same    bool // an equal item exists under another name
	ctxText string
}

// SecretSlot is one credential an item seals into the secret store.
type SecretSlot struct {
	Name string `json:"name"`
	// Field is the settings.yaml path holding the reference.
	Field string `json:"field"`
	// Reuse is true when an equal value is already saved under Name.
	Reuse bool `json:"reuse"`
}

type pendingSecret struct{ name, value string }

// op is one settings.yaml edit: set a key, or append to a list unless an equal entry exists.
type op struct {
	path   []string
	append bool
	value  any
}

// omap is an ordered mapping, so a written block keeps the order a reader expects.
type omap []kv

type kv struct {
	k string
	v any
}

// plain converts v to JSON-shaped values with maps in place of omaps and float64 numbers.
func plain(v any) any {
	switch t := v.(type) {
	case omap:
		m := make(map[string]any, len(t))
		for _, e := range t {
			m[e.k] = plain(e.v)
		}
		return m
	case map[string]any:
		m := make(map[string]any, len(t))
		for k, e := range t {
			m[k] = plain(e)
		}
		return m
	case []any:
		out := make([]any, len(t))
		for i, e := range t {
			out[i] = plain(e)
		}
		return out
	case []string:
		out := make([]any, len(t))
		for i, e := range t {
			out[i] = e
		}
		return out
	case int:
		return float64(t)
	case json.Number:
		f, _ := t.Float64()
		return f
	}
	return v
}

func equalValue(a, b any) bool { return reflect.DeepEqual(plain(a), plain(b)) }

// toNode encodes v as a YAML node.
func toNode(v any) *yaml.Node {
	switch t := v.(type) {
	case omap:
		n := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		for _, e := range t {
			n.Content = append(n.Content, strNode(e.k), toNode(e.v))
		}
		return n
	case map[string]any:
		n := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		for _, k := range sortedKeys(t) {
			n.Content = append(n.Content, strNode(k), toNode(t[k]))
		}
		return n
	case []any:
		n := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		for _, e := range t {
			n.Content = append(n.Content, toNode(e))
		}
		return n
	case []string:
		n := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		for _, e := range t {
			n.Content = append(n.Content, strNode(e))
		}
		return n
	case string:
		return strNode(t)
	case bool:
		n := &yaml.Node{}
		_ = n.Encode(t)
		return n
	case int:
		n := &yaml.Node{}
		_ = n.Encode(t)
		return n
	case float64:
		n := &yaml.Node{}
		if t == float64(int64(t)) {
			_ = n.Encode(int64(t))
		} else {
			_ = n.Encode(t)
		}
		return n
	}
	n := &yaml.Node{}
	_ = n.Encode(v)
	return n
}

func strNode(s string) *yaml.Node { return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: s} }

// snippet renders value at path as the YAML text it has in settings.yaml; append wraps it in a list.
func snippet(path []string, value any, asList bool) string {
	var node *yaml.Node
	if asList {
		node = toNode([]any{value})
	} else {
		node = toNode(value)
	}
	for i := len(path) - 1; i >= 0; i-- {
		node = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Content: []*yaml.Node{strNode(path[i]), node}}
	}
	out, err := marshalNode(node)
	if err != nil {
		return ""
	}
	return strings.TrimRight(string(out), "\n")
}

// lookup follows path through decoded settings.
func lookup(doc map[string]any, path []string) (any, bool) {
	var cur any = doc
	for _, k := range path {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		if cur, ok = m[k]; !ok {
			return nil, false
		}
	}
	return cur, true
}

// resolveState sets the item's state and conflict text against the current settings.
func (it *Item) resolveState(cur map[string]any) {
	if it.State == StateSkipped {
		return
	}
	if it.same {
		it.State = StateSame
		return
	}
	it.State = StateSame
	for _, o := range it.ops {
		existing, ok := lookup(cur, o.path)
		switch {
		case o.append:
			list, _ := existing.([]any)
			found := false
			for _, e := range list {
				if equalValue(e, o.value) {
					found = true
				}
			}
			if !found {
				it.State = worse(it.State, StateNew)
			}
		case !ok:
			it.State = worse(it.State, StateNew)
		case !equalValue(existing, o.value):
			it.State = StateConflict
			it.Current = snippet(o.path, existing, false)
		}
	}
}

func worse(a, b string) string {
	rank := map[string]int{StateSame: 0, StateNew: 1, StateConflict: 2}
	if rank[b] > rank[a] {
		return b
	}
	return a
}
