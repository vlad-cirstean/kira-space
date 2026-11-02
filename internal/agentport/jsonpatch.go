package agentport

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"

	"github.com/tailscale/hujson"
)

// patcher edits a JSON(C) document through RFC 6902 operations, so comments, key order and every key
// Kira does not manage stay byte for byte.
type patcher struct {
	ops      []patchOp
	have     map[string]bool // pointers known to exist
	existing map[string]any  // decoded values of the managed subtrees
}

type patchOp struct {
	Op    string          `json:"op"`
	Path  string          `json:"path"`
	Value json.RawMessage `json:"value,omitempty"`
}

// newPatcher knows the document's top-level keys (others) and the managed subtrees (found).
func newPatcher(found map[string]json.RawMessage, others []string) *patcher {
	p := &patcher{have: map[string]bool{}, existing: map[string]any{}}
	for _, k := range others {
		p.have[ptr(k)] = true
	}
	for k, raw := range found {
		var v any
		if json.Unmarshal(raw, &v) == nil {
			p.mark(ptr(k), v, 0)
		}
	}
	return p
}

func ptr(parts ...string) string {
	var b strings.Builder
	for _, s := range parts {
		b.WriteString("/" + strings.ReplaceAll(strings.ReplaceAll(s, "~", "~0"), "/", "~1"))
	}
	return b.String()
}

func (p *patcher) mark(at string, v any, depth int) {
	p.have[at] = true
	p.existing[at] = v
	if m, ok := v.(map[string]any); ok && depth < 3 {
		for k, e := range m {
			p.mark(at+ptr(k), e, depth+1)
		}
	}
}

func (p *patcher) ensure(parts []string, container string) {
	for i := 1; i < len(parts); i++ {
		at := ptr(parts[:i]...)
		if !p.have[at] {
			p.ops = append(p.ops, patchOp{Op: "add", Path: at, Value: json.RawMessage("{}")})
			p.have[at] = true
		}
	}
}

// set adds or replaces the value at path unless it already equals v.
func (p *patcher) set(path []string, v any) {
	raw, _ := json.Marshal(v)
	at := ptr(path...)
	if cur, ok := p.existing[at]; ok {
		var want any
		_ = json.Unmarshal(raw, &want)
		if reflect.DeepEqual(cur, want) {
			return
		}
	}
	op := "add"
	if p.have[at] {
		op = "replace"
	} else {
		p.ensure(path, "")
	}
	p.ops = append(p.ops, patchOp{Op: op, Path: at, Value: raw})
	p.have[at] = true
}

// appendTo adds v to the list at path, creating the list, unless an equal entry exists.
func (p *patcher) appendTo(path []string, v any) {
	raw, _ := json.Marshal(v)
	at := ptr(path...)
	var want any
	_ = json.Unmarshal(raw, &want)
	if list, ok := p.existing[at].([]any); ok {
		for _, e := range list {
			if reflect.DeepEqual(e, want) {
				return
			}
		}
		p.existing[at] = append(list, want)
	}
	if !p.have[at] {
		p.ensure(path, "")
		p.ops = append(p.ops, patchOp{Op: "add", Path: at, Value: json.RawMessage("[]")})
		p.have[at] = true
		p.existing[at] = []any{want}
	}
	p.ops = append(p.ops, patchOp{Op: "add", Path: at + "/-", Value: raw})
}

// apply patches base, a document or empty text for a new one; a new document is formatted.
func (p *patcher) apply(base []byte) ([]byte, error) {
	fresh := len(bytes.TrimSpace(base)) == 0
	if fresh {
		base = []byte("{}")
	}
	if len(p.ops) == 0 && !fresh {
		return base, nil
	}
	// A file already in canonical layout stays canonical; any other keeps the user's layout.
	canon, cerr := hujson.Format(bytes.Clone(base))
	isCanon := cerr == nil && bytes.Equal(canon, base)
	v, err := hujson.Parse(base)
	if err != nil {
		return nil, err
	}
	patch, err := json.Marshal(p.ops)
	if err != nil {
		return nil, err
	}
	if err := v.Patch(patch); err != nil {
		return nil, err
	}
	if fresh {
		v.Format()
		return append(bytes.TrimRight(v.Pack(), "\n"), '\n'), nil
	}
	if isCanon {
		v.Format()
	}
	return v.Pack(), nil
}
