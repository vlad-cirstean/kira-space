package postman

import (
	"encoding/json"
	"errors"
	"io"
	"strings"
)

// Environment is one Postman environment export: a flat name plus its values.
type Environment struct {
	Name      string
	Variables []Variable
	// Disabled counts values skipped because Postman marks them `enabled: false`.
	Disabled int
	// Empty counts imported values that are "": Postman exports only shared values.
	Empty int
	// Unnamed counts imported values whose key was missing and became unnamed_<n>.
	Unnamed int
}

// File is what ParseFile found: exactly one of Collection and Environment is set.
type File struct {
	Collection  *Tree
	Environment *Environment
}

// ParseFile classifies one export by shape and parses it. A collection has an `info` block, an
// environment a `values` array. A globals export is refused: the app has no globals scope.
func ParseFile(r io.Reader) (*File, error) {
	doc, err := readDocument(r)
	if err != nil {
		return nil, err
	}
	doc = unwrapAPIDocument(doc)
	if _, ok := doc["info"]; ok {
		tree, err := parseCollection(doc)
		if err != nil {
			return nil, err
		}
		return &File{Collection: tree}, nil
	}
	scope, _ := decodeString(doc["_postman_variable_scope"])
	if scope == "globals" {
		return nil, errors.New("postman: a globals export is not supported; export an environment instead")
	}
	if raw := doc["values"]; len(raw) > 0 && raw[0] == '[' {
		return &File{Environment: parseEnvironment(doc)}, nil
	}
	return nil, errors.New("postman: not a Postman collection or environment")
}

// unwrapAPIDocument unwraps one level of the Postman API envelope: `GET /collections/{id}` answers
// `{"collection":{info,...}}` and `GET /environments/{id}` `{"environment":{values,...}}`. Only a
// document with neither `info` nor `values` unwraps, and only when the member's own shape
// classifies; `collection` wins when both are present.
func unwrapAPIDocument(doc map[string]json.RawMessage) map[string]json.RawMessage {
	if _, ok := doc["info"]; ok {
		return doc
	}
	if _, ok := doc["values"]; ok {
		return doc
	}
	if inner := decodeObject(doc["collection"]); inner != nil && decodeObject(inner["info"]) != nil {
		return inner
	}
	if inner := decodeObject(doc["environment"]); inner != nil {
		if raw := inner["values"]; len(raw) > 0 && raw[0] == '[' {
			return inner
		}
	}
	return doc
}

func parseEnvironment(doc map[string]json.RawMessage) *Environment {
	name, _ := decodeString(doc["name"])
	if strings.TrimSpace(name) == "" {
		name = "Imported environment"
	}
	env := &Environment{Name: name, Variables: []Variable{}}
	entries := decodeArray(doc["values"])
	unnamed := newUnnamedKeys(entries)
	for _, entry := range entries {
		obj := decodeObject(entry)
		if obj == nil {
			continue
		}
		v, off := decodeVariableEntry(obj, unnamed)
		if off {
			env.Disabled++
			continue
		}
		if v.Value == "" {
			env.Empty++
		}
		env.Variables = append(env.Variables, v)
	}
	env.Unnamed = unnamed.count
	return env
}
