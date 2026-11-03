package apifiles

import (
	"bytes"
	"errors"
	"fmt"

	"go.yaml.in/yaml/v3"

	"github.com/kirathecat/kira-studio/internal/configfs"
)

// Edit errors, matched with errors.Is.
var (
	ErrItemGone  = errors.New("apifiles: the item is not in the file")
	ErrNotFolder = errors.New("apifiles: the parent is not a folder")
	ErrProtocol  = errors.New("apifiles: the item holds the other protocol")
)

// valueNode renders v as a YAML node, through the foundation's renderer, so quoting and block scalars
// match a file Kira writes whole.
func valueNode(v any) (*yaml.Node, error) {
	text, err := configfs.RenderYAML(v, "")
	if err != nil {
		return nil, err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(text, &doc); err != nil || len(doc.Content) != 1 {
		return nil, fmt.Errorf("apifiles: render node: %v", err)
	}
	return doc.Content[0], nil
}

func parseDoc(src []byte) (*yaml.Node, *yaml.Node, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(src, &doc); err != nil {
		return nil, nil, fmt.Errorf("apifiles: parse: %w", err)
	}
	if len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, nil, errors.New("apifiles: the file is not a mapping")
	}
	return &doc, doc.Content[0], nil
}

func encodeDoc(doc *yaml.Node) ([]byte, error) {
	var out bytes.Buffer
	enc := yaml.NewEncoder(&out)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		return nil, fmt.Errorf("apifiles: encode: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("apifiles: encode: %w", err)
	}
	return out.Bytes(), nil
}

// mapIndex returns the index of key's value in a mapping's Content, or -1.
func mapIndex(m *yaml.Node, key string) int {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return i + 1
		}
	}
	return -1
}

// findItem returns the mapping of the item with the given id, searching folders depth first.
func findItem(seq *yaml.Node, id string) *yaml.Node {
	if seq == nil || seq.Kind != yaml.SequenceNode {
		return nil
	}
	for _, it := range seq.Content {
		if it.Kind != yaml.MappingNode {
			continue
		}
		if i := mapIndex(it, "id"); i >= 0 && it.Content[i].Value == id {
			return it
		}
		if i := mapIndex(it, "items"); i >= 0 {
			if found := findItem(it.Content[i], id); found != nil {
				return found
			}
		}
	}
	return nil
}

func rootItems(root *yaml.Node) *yaml.Node {
	if i := mapIndex(root, "items"); i >= 0 {
		return root.Content[i]
	}
	return nil
}

// replaceRequest sets the item's name and swaps its http or grpc node. Comments outside that node stay.
func replaceRequest(src []byte, id, name string, http *HTTP, grpc *GRPC) ([]byte, error) {
	doc, root, err := parseDoc(src)
	if err != nil {
		return nil, err
	}
	item := findItem(rootItems(root), id)
	if item == nil {
		return nil, ErrItemGone
	}
	key, value := "http", any(http)
	if grpc != nil {
		key, value = "grpc", grpc
	}
	at := mapIndex(item, key)
	if at < 0 {
		return nil, ErrProtocol
	}
	node, err := valueNode(value)
	if err != nil {
		return nil, err
	}
	node.HeadComment = item.Content[at].HeadComment
	item.Content[at] = node
	if n := mapIndex(item, "name"); n >= 0 {
		item.Content[n].Value, item.Content[n].Tag, item.Content[n].Style = name, "!!str", 0
	}
	return encodeDoc(doc)
}

// appendItem adds item as the last child of the folder parentID, or of the collection when parentID is "".
func appendItem(src []byte, parentID string, item Item) ([]byte, error) {
	doc, root, err := parseDoc(src)
	if err != nil {
		return nil, err
	}
	var seq *yaml.Node
	if parentID == "" {
		if seq = rootItems(root); seq == nil {
			seq = &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
			root.Content = append(root.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "items"}, seq)
		}
	} else {
		parent := findItem(rootItems(root), parentID)
		if parent == nil {
			return nil, ErrItemGone
		}
		i := mapIndex(parent, "items")
		if i < 0 {
			return nil, ErrNotFolder
		}
		seq = parent.Content[i]
	}
	if seq.Kind != yaml.SequenceNode {
		return nil, ErrNotFolder
	}
	node, err := valueNode(item)
	if err != nil {
		return nil, err
	}
	seq.Style = 0
	seq.Content = append(seq.Content, node)
	return encodeDoc(doc)
}
