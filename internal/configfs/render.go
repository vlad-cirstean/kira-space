package configfs

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"go.yaml.in/yaml/v3"
)

// RenderYAML renders v, through its JSON form and field order, as a YAML file with header as leading
// comment lines. A string a YAML reader would re-type (yes, 1.0, ~) is quoted and a multi-line string
// is a literal block, so reading the file back yields the same values.
func RenderYAML(v any, header string) ([]byte, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("configfs: render: %w", err)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	node, err := jsonNode(dec)
	if err != nil {
		return nil, fmt.Errorf("configfs: render: %w", err)
	}
	var out bytes.Buffer
	if header != "" {
		for _, line := range strings.Split(strings.TrimRight(header, "\n"), "\n") {
			out.WriteString("# " + line + "\n")
		}
	}
	enc := yaml.NewEncoder(&out)
	enc.SetIndent(2)
	if err := enc.Encode(node); err != nil {
		return nil, fmt.Errorf("configfs: render: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("configfs: render: %w", err)
	}
	return out.Bytes(), nil
}

// jsonNode reads one JSON value into a yaml.Node, keeping object key order.
func jsonNode(dec *json.Decoder) (*yaml.Node, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	switch t := tok.(type) {
	case json.Delim:
		if t == '{' {
			n := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
			for dec.More() {
				k, err := dec.Token()
				if err != nil {
					return nil, err
				}
				val, err := jsonNode(dec)
				if err != nil {
					return nil, err
				}
				n.Content = append(n.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: k.(string)}, val)
			}
			_, err := dec.Token()
			return n, err
		}
		n := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		for dec.More() {
			el, err := jsonNode(dec)
			if err != nil {
				return nil, err
			}
			n.Content = append(n.Content, el)
		}
		_, err := dec.Token()
		return n, err
	case string:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: t}, nil
	case bool:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: fmt.Sprint(t)}, nil
	case json.Number:
		tag := "!!int"
		if strings.ContainsAny(t.String(), ".eE") {
			tag = "!!float"
		}
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: tag, Value: t.String()}, nil
	case nil:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!null", Value: "null"}, nil
	}
	return nil, io.ErrUnexpectedEOF
}
