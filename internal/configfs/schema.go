package configfs

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"

	"github.com/kirathecat/kira-studio/packages/theme/src/i18n"
)

// Rule is a cross-field or id-reference check run after schema validation. Doc is mandatory and
// renders into AGENTS.md, so a rule cannot exist without its doc line. A returned Problem without a
// Line gets the line of its Path.
type Rule[T any] struct {
	Doc   string
	Check func(v T, at Locator) *Problem
}

// YAMLSchema returns the decode function of a YAML kind: parse, reject anchors and merge keys,
// validate against the JSON Schema (draft 2020-12), decode into T (unknown fields refused), run rules.
// The DocModel carries the options and rule docs for AGENTS.md. Every Problem points at the file line of the offending field and never echoes the offending value.
func YAMLSchema[T any](schemaJSON []byte, rules ...Rule[T]) (DecodeFunc[T], DocModel, error) {
	for _, r := range rules {
		if r.Doc == "" || r.Check == nil {
			return nil, DocModel{}, fmt.Errorf("configfs: rule needs Doc and Check")
		}
	}
	sch, err := compileSchema(schemaJSON)
	if err != nil {
		return nil, DocModel{}, err
	}
	rc, err := newRefChecker(schemaJSON)
	if err != nil {
		return nil, DocModel{}, fmt.Errorf("configfs: schema: %w", err)
	}
	docs := make([]string, len(rules))
	for i, r := range rules {
		docs[i] = r.Doc
	}
	model, err := DocFromSchema(schemaJSON, docs)
	if err != nil {
		return nil, DocModel{}, err
	}
	return func(_ string, src []byte) (T, *Problem) {
		var zero T
		doc, prob := parseNode(src)
		if prob != nil {
			return zero, prob
		}
		w := &walker{idx: nodeIndex{}}
		val := w.value(doc, "")
		if w.prob != nil {
			return zero, w.prob
		}
		at := w.idx.locator()
		if prob := validate(sch, val, at); prob != nil {
			return zero, prob
		}
		if prob := rc.check(rc.root, val, "", at); prob != nil {
			return zero, prob
		}
		var out T
		raw, err := json.Marshal(val)
		if err == nil {
			dec := json.NewDecoder(bytes.NewReader(raw))
			dec.DisallowUnknownFields()
			err = dec.Decode(&out)
		}
		if err != nil {
			return zero, &Problem{Message: i18n.T("backend.core.config.invalid.body", map[string]string{"field": fieldName("")})}
		}
		for _, r := range rules {
			if p := r.Check(out, at); p != nil {
				if p.Line == 0 {
					p.Line, p.Col = at(p.Path)
				}
				return zero, p
			}
		}
		return out, nil
	}, model, nil
}

func compileSchema(schemaJSON []byte) (*jsonschema.Schema, error) {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(schemaJSON))
	if err != nil {
		return nil, fmt.Errorf("configfs: schema: %w", err)
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource("schema.json", doc); err != nil {
		return nil, fmt.Errorf("configfs: schema: %w", err)
	}
	sch, err := c.Compile("schema.json")
	if err != nil {
		return nil, fmt.Errorf("configfs: schema: %w", err)
	}
	return sch, nil
}

func validate(sch *jsonschema.Schema, val any, at Locator) *Problem {
	err := sch.Validate(val)
	if err == nil {
		return nil
	}
	verr, ok := err.(*jsonschema.ValidationError)
	if !ok {
		return &Problem{Message: i18n.T("backend.core.config.invalid.body", map[string]string{"field": fieldName("")})}
	}
	for len(verr.Causes) > 0 {
		verr = verr.Causes[0]
	}
	return problemFor(verr, at)
}

func pointerOf(tokens []string) string {
	var b strings.Builder
	for _, t := range tokens {
		b.WriteByte('/')
		b.WriteString(escapePointer(t))
	}
	return b.String()
}

// fieldName renders a JSON pointer as a dotted path for messages.
func fieldName(pointer string) string {
	if pointer == "" {
		return i18n.T("backend.core.config.root.label", nil)
	}
	parts := strings.Split(pointer[1:], "/")
	for i, p := range parts {
		p = strings.ReplaceAll(p, "~1", "/")
		parts[i] = strings.ReplaceAll(p, "~0", "~")
	}
	return strings.Join(parts, ".")
}

func ratString(r *big.Rat) string {
	if r.IsInt() {
		return r.Num().String()
	}
	return strings.TrimRight(strings.TrimRight(r.FloatString(4), "0"), ".")
}

func problemFor(verr *jsonschema.ValidationError, at Locator) *Problem {
	ptr := pointerOf(verr.InstanceLocation)
	at0 := func(pointer string) *Problem {
		line, col := at(pointer)
		return &Problem{Line: line, Col: col, Path: pointer}
	}
	switch k := verr.ErrorKind.(type) {
	case *kind.Required:
		p := at0(ptr)
		field := ptr + "/" + escapePointer(k.Missing[0])
		p.Path = field
		p.Message = i18n.T("backend.core.config.required.body", map[string]string{"field": fieldName(field)})
		return p
	case *kind.AdditionalProperties:
		field := ptr + "/" + escapePointer(k.Properties[0])
		p := at0(field)
		p.Message = i18n.T("backend.core.config.additional.body", map[string]string{"field": fieldName(field)})
		return p
	case *kind.Type:
		p := at0(ptr)
		p.Message = i18n.T("backend.core.config.type.body", map[string]string{"field": fieldName(ptr), "want": strings.Join(k.Want, " or ")})
		return p
	case *kind.Enum:
		allowed := make([]string, len(k.Want))
		for i, w := range k.Want {
			b, _ := json.Marshal(w)
			allowed[i] = string(b)
		}
		p := at0(ptr)
		p.Message = i18n.T("backend.core.config.enum.body", map[string]string{"field": fieldName(ptr), "allowed": strings.Join(allowed, ", ")})
		return p
	case *kind.Minimum:
		p := at0(ptr)
		p.Message = i18n.T("backend.core.config.min.body", map[string]string{"field": fieldName(ptr), "limit": ratString(k.Want)})
		return p
	case *kind.Maximum:
		p := at0(ptr)
		p.Message = i18n.T("backend.core.config.max.body", map[string]string{"field": fieldName(ptr), "limit": ratString(k.Want)})
		return p
	case *kind.MinLength:
		p := at0(ptr)
		p.Message = i18n.T("backend.core.config.minLength.body", map[string]string{"field": fieldName(ptr), "limit": fmt.Sprint(k.Want)})
		return p
	case *kind.MaxLength:
		p := at0(ptr)
		p.Message = i18n.T("backend.core.config.maxLength.body", map[string]string{"field": fieldName(ptr), "limit": fmt.Sprint(k.Want)})
		return p
	case *kind.MinItems:
		p := at0(ptr)
		p.Message = i18n.T("backend.core.config.minItems.body", map[string]string{"field": fieldName(ptr), "limit": fmt.Sprint(k.Want)})
		return p
	case *kind.MaxItems:
		p := at0(ptr)
		p.Message = i18n.T("backend.core.config.maxItems.body", map[string]string{"field": fieldName(ptr), "limit": fmt.Sprint(k.Want)})
		return p
	case *kind.Pattern:
		p := at0(ptr)
		p.Message = i18n.T("backend.core.config.pattern.body", map[string]string{"field": fieldName(ptr)})
		return p
	default:
		p := at0(ptr)
		p.Message = i18n.T("backend.core.config.invalid.body", map[string]string{"field": fieldName(ptr)})
		return p
	}
}
