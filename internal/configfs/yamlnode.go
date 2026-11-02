package configfs

import (
	"encoding/json"
	"fmt"
	"strconv"

	"go.yaml.in/yaml/v3"

	"github.com/kirathecat/kira-studio/packages/theme/src/i18n"
)

// Locator maps a JSON pointer to the 1-based line and column of the field in the file; 0, 0 when unknown.
type Locator func(pointer string) (line, col int)

type nodeIndex map[string]*yaml.Node

func (n nodeIndex) locator() Locator {
	return func(pointer string) (int, int) {
		for {
			if node, ok := n[pointer]; ok {
				return node.Line, node.Column
			}
			if pointer == "" {
				return 0, 0
			}
			pointer = pointer[:lastSlash(pointer)]
		}
	}
}

func lastSlash(p string) int {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' {
			return i
		}
	}
	return 0
}

func escapePointer(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '~':
			out = append(out, '~', '0')
		case '/':
			out = append(out, '~', '1')
		default:
			out = append(out, s[i])
		}
	}
	return string(out)
}

// walker turns a yaml.Node tree into JSON-typed values and indexes each field's node by JSON pointer.
type walker struct {
	idx  nodeIndex
	prob *Problem
}

func (w *walker) fail(n *yaml.Node, pointer, message string) {
	if w.prob == nil {
		w.prob = &Problem{Line: n.Line, Col: n.Column, Path: pointer, Message: message}
	}
}

// setNode indexes a container unless its key node is already there: errors point at the key.
func (w *walker) setNode(pointer string, n *yaml.Node) {
	if _, ok := w.idx[pointer]; !ok {
		w.idx[pointer] = n
	}
}

func (w *walker) value(n *yaml.Node, pointer string) any {
	if n.Anchor != "" {
		w.fail(n, pointer, i18n.T("backend.core.config.alias.body", nil))
	}
	switch n.Kind {
	case yaml.DocumentNode:
		if len(n.Content) == 0 {
			return nil
		}
		return w.value(n.Content[0], pointer)
	case yaml.AliasNode:
		w.fail(n, pointer, i18n.T("backend.core.config.alias.body", nil))
		return nil
	case yaml.MappingNode:
		w.setNode(pointer, n)
		out := make(map[string]any, len(n.Content)/2)
		for i := 0; i+1 < len(n.Content); i += 2 {
			k, v := n.Content[i], n.Content[i+1]
			if k.ShortTag() == "!!merge" {
				w.fail(k, pointer, i18n.T("backend.core.config.alias.body", nil))
				continue
			}
			if k.Kind != yaml.ScalarNode || k.ShortTag() != "!!str" {
				w.fail(k, pointer, i18n.T("backend.core.config.keys.body", nil))
				continue
			}
			child := pointer + "/" + escapePointer(k.Value)
			w.idx[child] = k
			out[k.Value] = w.value(v, child)
		}
		return out
	case yaml.SequenceNode:
		w.setNode(pointer, n)
		out := make([]any, len(n.Content))
		for i, c := range n.Content {
			child := pointer + "/" + strconv.Itoa(i)
			w.idx[child] = c
			out[i] = w.value(c, child)
		}
		return out
	default:
		return scalar(n)
	}
}

func scalar(n *yaml.Node) any {
	switch n.ShortTag() {
	case "!!null":
		return nil
	case "!!bool":
		var b bool
		if n.Decode(&b) == nil {
			return b
		}
	case "!!int":
		var i int64
		if n.Decode(&i) == nil {
			return json.Number(strconv.FormatInt(i, 10))
		}
		var u uint64
		if n.Decode(&u) == nil {
			return json.Number(strconv.FormatUint(u, 10))
		}
	case "!!float":
		var f float64
		if n.Decode(&f) == nil && f == f && f-f == 0 {
			return json.Number(strconv.FormatFloat(f, 'g', -1, 64))
		}
	}
	return n.Value
}

// parseNode parses src and returns the document root. A syntax error carries its line.
func parseNode(src []byte) (*yaml.Node, *Problem) {
	var doc yaml.Node
	if err := yaml.Unmarshal(src, &doc); err != nil {
		line, detail := splitYAMLError(err)
		return nil, &Problem{Line: line, Message: i18n.T("backend.core.config.syntax.body", map[string]string{"detail": detail})}
	}
	return &doc, nil
}

func splitYAMLError(err error) (int, string) {
	s := err.Error()
	var line int
	var rest string
	if n, _ := fmt.Sscanf(s, "yaml: line %d: ", &line); n == 1 {
		prefix := fmt.Sprintf("yaml: line %d: ", line)
		rest = s[len(prefix):]
	} else {
		rest = s
	}
	return line, rest
}
