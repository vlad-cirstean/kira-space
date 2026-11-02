package agentport

import (
	"bytes"
	"errors"
	"fmt"
	"slices"
	"sort"

	"go.yaml.in/yaml/v3"
)

// settingsOrder is the top-level key order of a fresh settings.yaml.
var settingsOrder = []string{"env", "permissions", "mcp_servers", "hooks"}

// editor edits settings.yaml as a node tree: untouched nodes keep their comments.
type editor struct {
	root   *yaml.Node
	prefix []byte // the original text of a file with no mapping (comments only)
	fresh  bool
}

func newEditor(text []byte) (*editor, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(text, &doc); err != nil {
		return nil, fmt.Errorf("agentport: parse settings.yaml: %w", err)
	}
	if doc.Kind == yaml.DocumentNode && len(doc.Content) == 1 && doc.Content[0].Kind == yaml.MappingNode {
		return &editor{root: doc.Content[0]}, nil
	}
	if doc.Kind == yaml.DocumentNode && len(doc.Content) == 1 && doc.Content[0].ShortTag() != "!!null" {
		return nil, errors.New("agentport: settings.yaml is not a mapping")
	}
	e := &editor{root: &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}, fresh: true}
	if len(bytes.TrimSpace(text)) > 0 {
		e.prefix = append(bytes.TrimRight(text, "\n"), '\n', '\n')
	}
	return e, nil
}

// child returns the value node of key in mapping m, creating an empty mapping or list when absent.
func child(m *yaml.Node, key string, create *yaml.Node) *yaml.Node {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	if create == nil {
		return nil
	}
	m.Content = append(m.Content, strNode(key), create)
	return create
}

func mapping() *yaml.Node { return &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"} }

func (e *editor) apply(o op) error {
	if len(o.path) == 0 {
		return errors.New("agentport: empty edit path")
	}
	m := e.root
	for _, k := range o.path[:len(o.path)-1] {
		next := child(m, k, mapping())
		if next.Kind != yaml.MappingNode {
			return fmt.Errorf("agentport: settings.yaml key %s is not a mapping", k)
		}
		m = next
	}
	last := o.path[len(o.path)-1]
	if o.append {
		seq := child(m, last, &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"})
		if seq.Kind != yaml.SequenceNode {
			return fmt.Errorf("agentport: settings.yaml key %s is not a list", last)
		}
		for _, c := range seq.Content {
			var cur any
			if c.Decode(&cur) == nil && equalValue(cur, o.value) {
				return nil
			}
		}
		seq.Content = append(seq.Content, toNode(o.value))
		return nil
	}
	val := toNode(o.value)
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == last {
			m.Content[i+1] = val
			return nil
		}
	}
	m.Content = append(m.Content, strNode(last), val)
	return nil
}

// bytes encodes the edited document with 2-space indent.
func (e *editor) bytes() ([]byte, error) {
	if e.fresh {
		e.reorder()
	}
	out, err := marshalNode(e.root)
	if err != nil {
		return nil, err
	}
	return append(slices.Clone(e.prefix), out...), nil
}

// reorder puts the common keys first in schema order, then provider sections by name.
func (e *editor) reorder() {
	type pair struct{ k, v *yaml.Node }
	var pairs []pair
	for i := 0; i+1 < len(e.root.Content); i += 2 {
		pairs = append(pairs, pair{e.root.Content[i], e.root.Content[i+1]})
	}
	rank := func(k string) int {
		if i := slices.Index(settingsOrder, k); i >= 0 {
			return i
		}
		return len(settingsOrder)
	}
	sort.SliceStable(pairs, func(i, j int) bool {
		ri, rj := rank(pairs[i].k.Value), rank(pairs[j].k.Value)
		if ri != rj {
			return ri < rj
		}
		return pairs[i].k.Value < pairs[j].k.Value
	})
	e.root.Content = e.root.Content[:0]
	for _, p := range pairs {
		e.root.Content = append(e.root.Content, p.k, p.v)
	}
}

func marshalNode(n *yaml.Node) ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(n); err != nil {
		return nil, fmt.Errorf("agentport: encode yaml: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("agentport: encode yaml: %w", err)
	}
	return buf.Bytes(), nil
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
