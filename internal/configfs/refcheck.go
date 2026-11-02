package configfs

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"

	"github.com/kirathecat/kira-studio/packages/theme/src/i18n"
)

// refChecker enforces the x-kira-ref and x-kira-secret schema keywords. Both are unknown to the
// validator and read here: x-kira-ref marks a string that may hold references, x-kira-secret one
// that must be exactly one secret reference, never a literal.
type refChecker struct{ root map[string]any }

func newRefChecker(schemaJSON []byte) (*refChecker, error) {
	var root map[string]any
	if err := json.Unmarshal(schemaJSON, &root); err != nil {
		return nil, err
	}
	return &refChecker{root: root}, nil
}

func (c *refChecker) resolve(s map[string]any) map[string]any {
	for i := 0; i < 8; i++ {
		ref, _ := s["$ref"].(string)
		name, ok := strings.CutPrefix(ref, "#/$defs/")
		if !ok {
			return s
		}
		defs, _ := c.root["$defs"].(map[string]any)
		next, ok := defs[name].(map[string]any)
		if !ok {
			return s
		}
		s = next
	}
	return s
}

func (c *refChecker) check(schema map[string]any, val any, pointer string, at Locator) *Problem {
	schema = c.resolve(schema)
	switch v := val.(type) {
	case string:
		return c.checkString(schema, v, pointer, at)
	case map[string]any:
		props, _ := schema["properties"].(map[string]any)
		extra, _ := schema["additionalProperties"].(map[string]any)
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			sub, ok := props[k].(map[string]any)
			if !ok {
				sub = extra
			}
			if sub == nil {
				continue
			}
			if p := c.check(sub, v[k], pointer+"/"+escapePointer(k), at); p != nil {
				return p
			}
		}
	case []any:
		items, _ := schema["items"].(map[string]any)
		if items == nil {
			return nil
		}
		for i, e := range v {
			if p := c.check(items, e, pointer+"/"+strconv.Itoa(i), at); p != nil {
				return p
			}
		}
	}
	return nil
}

func (c *refChecker) checkString(schema map[string]any, text, pointer string, at Locator) *Problem {
	secret, _ := schema["x-kira-secret"].(bool)
	ref, _ := schema["x-kira-ref"].(bool)
	var p *Problem
	switch {
	case secret:
		if !wholeSecret(text) {
			p = &Problem{Message: i18n.T("backend.core.config.refSecretLiteral.body", map[string]string{"field": fieldName(pointer), "syntax": secretSyntax})}
		}
	case ref:
		_, p = Refs(text)
	}
	if p != nil {
		p.Path = pointer
		p.Line, p.Col = at(pointer)
	}
	return p
}
