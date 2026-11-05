package configfs

import (
	"bytes"
	"errors"
	"fmt"

	"go.yaml.in/yaml/v3"
)

// ErrNotMapping marks an edit whose path crosses a value that is not a mapping.
var ErrNotMapping = errors.New("configfs: the path crosses a value that is not a mapping")

// NodeSet is one leaf edit: Value, rendered like a file Kira writes whole, goes at the key Path.
// Unset removes the key instead (Value is ignored): a parent mapping left empty and without
// comments goes too, and a missing key is a no-op.
type NodeSet struct {
	Path  []string
	Value any
	Unset bool
}

// EditYAML applies sets to a YAML mapping document in order. A missing parent mapping is created, an
// existing value is replaced in place. Comments and key order outside the edited values stay.
func EditYAML(src []byte, sets []NodeSet) ([]byte, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(src, &doc); err != nil {
		return nil, fmt.Errorf("configfs: parse: %w", err)
	}
	if len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, ErrNotMapping
	}
	for _, s := range sets {
		if len(s.Path) == 0 {
			return nil, fmt.Errorf("configfs: edit: empty path")
		}
		if s.Unset {
			unsetNode(doc.Content[0], s.Path)
			continue
		}
		val, err := valueNode(s.Value)
		if err != nil {
			return nil, err
		}
		if err := setNode(doc.Content[0], s.Path, val); err != nil {
			return nil, err
		}
	}
	var out bytes.Buffer
	enc := yaml.NewEncoder(&out)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		return nil, fmt.Errorf("configfs: encode: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("configfs: encode: %w", err)
	}
	return out.Bytes(), nil
}

func setNode(m *yaml.Node, path []string, val *yaml.Node) error {
	key := path[0]
	at := -1
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			at = i + 1
			break
		}
	}
	if len(path) == 1 {
		if at < 0 {
			m.Content = append(m.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, val)
			return nil
		}
		old := m.Content[at]
		val.HeadComment, val.LineComment, val.FootComment = old.HeadComment, old.LineComment, old.FootComment
		m.Content[at] = val
		return nil
	}
	if at < 0 {
		child := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		m.Content = append(m.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, child)
		return setNode(child, path[1:], val)
	}
	child := m.Content[at]
	if child.Kind != yaml.MappingNode {
		return ErrNotMapping
	}
	child.Style = 0 // a flow mapping `{}` grows block keys
	return setNode(child, path[1:], val)
}

// unsetNode removes the key at path and prunes parents it leaves empty and uncommented.
func unsetNode(m *yaml.Node, path []string) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value != path[0] {
			continue
		}
		if len(path) > 1 {
			child := m.Content[i+1]
			if child.Kind != yaml.MappingNode {
				return
			}
			unsetNode(child, path[1:])
			if len(child.Content) > 0 || hasComment(m.Content[i]) || hasComment(child) {
				return
			}
		}
		// A comment above the first key is the file's header: it moves to the next key.
		if head := m.Content[i].HeadComment; i == 0 && head != "" && len(m.Content) > 2 {
			next := m.Content[2]
			if next.HeadComment != "" {
				head += "\n" + next.HeadComment
			}
			next.HeadComment = head
		}
		m.Content = append(m.Content[:i], m.Content[i+2:]...)
		return
	}
}

func hasComment(n *yaml.Node) bool {
	return n.HeadComment != "" || n.LineComment != "" || n.FootComment != ""
}

// valueNode renders v as a YAML node through RenderYAML, so quoting and block scalars match a whole file.
func valueNode(v any) (*yaml.Node, error) {
	text, err := RenderYAML(v, "")
	if err != nil {
		return nil, err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(text, &doc); err != nil || len(doc.Content) != 1 {
		return nil, fmt.Errorf("configfs: render node: %v", err)
	}
	return doc.Content[0], nil
}
