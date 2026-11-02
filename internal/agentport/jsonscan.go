package agentport

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/tailscale/hujson"
)

// topLevel streams a JSON object and returns the raw values of the wanted keys. Every other value is
// skipped token by token, never interpreted: others lists its key names only.
func topLevel(raw []byte, want ...string) (found map[string]json.RawMessage, others []string, err error) {
	std, err := hujson.Standardize(bytes.Clone(raw)) // Standardize edits its input in place
	if err != nil {
		return nil, nil, fmt.Errorf("agentport: not valid JSON: %w", err)
	}
	dec := json.NewDecoder(bytes.NewReader(std))
	if t, err := dec.Token(); err != nil || t != json.Delim('{') {
		return nil, nil, errors.New("agentport: top level is not an object")
	}
	found = map[string]json.RawMessage{}
	for dec.More() {
		kt, err := dec.Token()
		if err != nil {
			return nil, nil, err
		}
		key, _ := kt.(string)
		wanted := false
		for _, w := range want {
			wanted = wanted || w == key
		}
		if wanted {
			var rm json.RawMessage
			if err := dec.Decode(&rm); err != nil {
				return nil, nil, err
			}
			found[key] = rm
			continue
		}
		others = append(others, key)
		if err := skipValue(dec); err != nil {
			return nil, nil, err
		}
	}
	return found, others, nil
}

func skipValue(dec *json.Decoder) error {
	t, err := dec.Token()
	if err != nil {
		return err
	}
	d, ok := t.(json.Delim)
	if !ok || (d != '{' && d != '[') {
		return nil
	}
	for dec.More() {
		if d == '{' {
			if _, err := dec.Token(); err != nil {
				return err
			}
		}
		if err := skipValue(dec); err != nil {
			return err
		}
	}
	_, err = dec.Token()
	return err
}
