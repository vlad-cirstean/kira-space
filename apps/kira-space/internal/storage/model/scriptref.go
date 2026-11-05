package model

import (
	"bytes"
	"encoding/json"
	"maps"
	"slices"
)

// MaxScriptRefName is the longest Run script name a repo file may reference.
const MaxScriptRefName = 200

// ParamValues is one param's values in a repo file: a text or a list of texts.
type ParamValues []string

// UnmarshalJSON accepts a JSON string or an array of strings.
func (p *ParamValues) UnmarshalJSON(b []byte) error {
	if bytes.HasPrefix(bytes.TrimSpace(b), []byte("[")) {
		var l []string
		if err := json.Unmarshal(b, &l); err != nil {
			return err
		}
		*p = l
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	*p = []string{s}
	return nil
}

// MarshalJSON writes one value as a string, else an array. Mirrors UnmarshalJSON so a decoded file
// round-trips in its own shape.
func (p ParamValues) MarshalJSON() ([]byte, error) {
	if len(p) == 1 {
		return json.Marshal(p[0])
	}
	return json.Marshal([]string(p))
}

// ScriptRef names a Run-module script a repo file runs in place of inline shell text. It mirrors a
// workflow step's `smart_script` plus `params`.
type ScriptRef struct {
	Script string                 `json:"script"`
	Params map[string]ParamValues `json:"params,omitempty"`
}

// Clone is a deep copy; nil stays nil.
func (r *ScriptRef) Clone() *ScriptRef {
	if r == nil {
		return nil
	}
	c := &ScriptRef{Script: r.Script}
	if r.Params != nil {
		c.Params = make(map[string]ParamValues, len(r.Params))
		for k, v := range r.Params {
			c.Params[k] = slices.Clone(v)
		}
	}
	return c
}

// Equal reports whether r and o name the same script with the same params.
func (r *ScriptRef) Equal(o *ScriptRef) bool {
	if r == nil || o == nil {
		return r == o
	}
	return r.Script == o.Script && maps.EqualFunc(r.Params, o.Params, slices.Equal[ParamValues])
}

// Values is the params as the resolver takes them.
func (r *ScriptRef) Values() map[string][]string {
	out := make(map[string][]string, len(r.Params))
	for k, v := range r.Params {
		out[k] = []string(v)
	}
	return out
}
