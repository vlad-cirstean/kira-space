package proof

import (
	"bytes"
	"encoding/json"
	"strconv"

	"go.yaml.in/yaml/v3"
)

// InlineToJSON converts the YAML node of an inline step value to the raw JSON ValidateStep takes.
// It walks the node instead of decoding into any, so integers keep every digit, floats keep their
// written text and mapping keys keep their file order. Aliases, anchors, merge keys and non-string
// keys are refused; errors carry the node's line.
func InlineToJSON(n *yaml.Node) (json.RawMessage, error) {
	var buf bytes.Buffer
	if err := writeJSON(&buf, n); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func inlineErr(n *yaml.Node, msg string) error {
	return invalid("line %d: %s", n.Line, msg)
}

func writeJSON(buf *bytes.Buffer, n *yaml.Node) error {
	if n.Anchor != "" {
		return inlineErr(n, "anchors are not allowed")
	}
	switch n.Kind {
	case yaml.DocumentNode:
		if len(n.Content) == 0 {
			buf.WriteString("null")
			return nil
		}
		return writeJSON(buf, n.Content[0])
	case yaml.AliasNode:
		return inlineErr(n, "aliases are not allowed")
	case yaml.MappingNode:
		return writeMapping(buf, n)
	case yaml.SequenceNode:
		buf.WriteByte('[')
		for i, c := range n.Content {
			if i > 0 {
				buf.WriteByte(',')
			}
			if err := writeJSON(buf, c); err != nil {
				return err
			}
		}
		buf.WriteByte(']')
		return nil
	}
	return writeScalar(buf, n)
}

func writeMapping(buf *bytes.Buffer, n *yaml.Node) error {
	buf.WriteByte('{')
	seen := map[string]bool{}
	for i := 0; i+1 < len(n.Content); i += 2 {
		k, v := n.Content[i], n.Content[i+1]
		if k.ShortTag() == "!!merge" {
			return inlineErr(k, "merge keys are not allowed")
		}
		if k.Kind != yaml.ScalarNode || k.ShortTag() != "!!str" {
			return inlineErr(k, "mapping keys must be strings")
		}
		if seen[k.Value] {
			return inlineErr(k, "duplicate key "+strconv.Quote(k.Value))
		}
		seen[k.Value] = true
		if i > 0 {
			buf.WriteByte(',')
		}
		key, _ := json.Marshal(k.Value)
		buf.Write(key)
		buf.WriteByte(':')
		if err := writeJSON(buf, v); err != nil {
			return err
		}
	}
	buf.WriteByte('}')
	return nil
}

func writeScalar(buf *bytes.Buffer, n *yaml.Node) error {
	switch n.ShortTag() {
	case "!!null":
		buf.WriteString("null")
	case "!!bool":
		var b bool
		if err := n.Decode(&b); err != nil {
			return inlineErr(n, "not a boolean")
		}
		buf.WriteString(strconv.FormatBool(b))
	case "!!int":
		if json.Valid([]byte(n.Value)) { // plain decimal text; hex, octal and underscores fall through
			buf.WriteString(n.Value)
			return nil
		}
		var i int64
		if n.Decode(&i) == nil {
			buf.WriteString(strconv.FormatInt(i, 10))
			return nil
		}
		var u uint64
		if err := n.Decode(&u); err != nil {
			return inlineErr(n, "integer out of range")
		}
		buf.WriteString(strconv.FormatUint(u, 10))
	case "!!float":
		var f float64
		if err := n.Decode(&f); err != nil || f != f || f-f != 0 {
			return inlineErr(n, "number is not finite")
		}
		if json.Valid([]byte(n.Value)) {
			buf.WriteString(n.Value)
			return nil
		}
		buf.WriteString(strconv.FormatFloat(f, 'g', -1, 64))
	default:
		s, _ := json.Marshal(n.Value)
		buf.Write(s)
	}
	return nil
}
