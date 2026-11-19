package jsengine

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/big"
	"strconv"
	"strings"

	"github.com/dop251/goja"
)

const maxSafeInt = 1<<53 - 1

// nestingExceeds reports whether JSON text nests deeper than limit. It does not validate; strings
// are skipped so brackets inside them do not count.
func nestingExceeds(b []byte, limit int) bool {
	depth := 0
	inStr := false
	for i := 0; i < len(b); i++ {
		c := b[i]
		if inStr {
			if c == '\\' {
				i++
			} else if c == '"' {
				inStr = false
			}
			continue
		}
		switch c {
		case '"':
			inStr = true
		case '[', '{':
			if depth++; depth > limit {
				return true
			}
		case ']', '}':
			if depth > 0 {
				depth--
			}
		}
	}
	return false
}

var errJSONDepth = fmt.Errorf("JSON nesting deeper than %d", MaxJSONDepth)

// fromJSON builds a goja value from JSON text. Integer literals beyond 2^53-1 become BigInt, so
// they stay exact; decimals and exponents are float64. Keys keep source order.
func (st *runState) fromJSON(raw []byte) (goja.Value, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	v, err := st.parseJSONValue(dec, 0)
	if err != nil {
		return nil, err
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, errors.New("unexpected data after JSON value")
	}
	return v, nil
}

func (st *runState) parseJSONValue(dec *json.Decoder, depth int) (goja.Value, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	switch t := tok.(type) {
	case json.Delim:
		if depth+1 > MaxJSONDepth {
			return nil, errJSONDepth
		}
		if t == '{' {
			obj := st.vm.NewObject()
			for dec.More() {
				kt, err := dec.Token()
				if err != nil {
					return nil, err
				}
				v, err := st.parseJSONValue(dec, depth+1)
				if err != nil {
					return nil, err
				}
				// Define, never Set: a "__proto__" key is data, as in JSON.parse.
				if err := obj.DefineDataProperty(kt.(string), v, goja.FLAG_TRUE, goja.FLAG_TRUE, goja.FLAG_TRUE); err != nil {
					return nil, err
				}
			}
			_, err := dec.Token()
			return obj, err
		}
		var items []any
		for dec.More() {
			v, err := st.parseJSONValue(dec, depth+1)
			if err != nil {
				return nil, err
			}
			items = append(items, v)
		}
		_, err := dec.Token()
		return st.vm.NewArray(items...), err
	case string:
		return st.vm.ToValue(t), nil
	case bool:
		return st.vm.ToValue(t), nil
	case nil:
		return goja.Null(), nil
	case json.Number:
		return st.number(t), nil
	}
	return nil, fmt.Errorf("unexpected JSON token %v", tok)
}

func (st *runState) number(n json.Number) goja.Value {
	s := string(n)
	if !strings.ContainsAny(s, ".eE") {
		if i, err := strconv.ParseInt(s, 10, 64); err == nil && i >= -maxSafeInt && i <= maxSafeInt {
			if i == 0 && s[0] == '-' {
				return st.vm.ToValue(math.Copysign(0, -1))
			}
			return st.vm.ToValue(i)
		}
		if b, ok := new(big.Int).SetString(s, 10); ok {
			return st.vm.ToValue(b)
		}
	}
	f, _ := strconv.ParseFloat(s, 64) // out of range gives +-Inf, which encodes as null
	return st.vm.ToValue(f)
}

// encode stringifies v with the captured JSON.stringify under a byte cap. BigInts come out as exact
// integer literals. A non-nil Error has Kind output.
func (st *runState) encode(v goja.Value, limit int) (string, *Error) {
	st.encoding = true
	st.overflow = false
	defer func() { st.encoding = false }()
	counted := 0
	replacer := st.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		counted += countValue(call.Argument(0), call.Argument(1))
		if counted > limit {
			st.overflow = true
			st.throwError("RangeError", "output limit")
		}
		return call.Argument(1)
	})
	res, err := st.origStringify(goja.Undefined(), v, replacer)
	if err != nil {
		if st.overflow {
			return "", newError(KindOutput, "result exceeds %d bytes", limit)
		}
		var exc *goja.Exception
		if errors.As(err, &exc) {
			return "", newError(KindOutput, "result cannot be encoded as JSON: %s", describeThrown(exc.Value(), st.safeString))
		}
		return "", newError(KindOutput, "result cannot be encoded as JSON")
	}
	if goja.IsUndefined(res) {
		return "null", nil
	}
	text := res.String()
	if len(text) > limit {
		return "", newError(KindOutput, "result exceeds %d bytes", limit)
	}
	return st.rewrite(text), nil
}

// countValue estimates the output bytes one key and value add; the exact size is checked after.
func countValue(key, v goja.Value) int {
	n := 4
	if s, ok := key.(goja.String); ok {
		n += s.Length()
	}
	if s, ok := v.(goja.String); ok {
		return n + s.Length() + 2
	}
	return n + 6
}

// rewrite turns marker strings back into bare integer literals and replaces lone surrogate
// escapes with U+FFFD, in one pass over the encoder's compact output.
func (st *runState) rewrite(text string) string {
	if !strings.Contains(text, `\ud`) && !strings.Contains(text, `\uD`) && !strings.Contains(text, st.markerEsc) {
		return text
	}
	var b strings.Builder
	b.Grow(len(text))
	for i := 0; i < len(text); {
		c := text[i]
		if c != '"' {
			b.WriteByte(c)
			i++
			continue
		}
		if strings.HasPrefix(text[i:], st.markerEsc) {
			j := i + len(st.markerEsc)
			k := j
			for k < len(text) && (text[k] >= '0' && text[k] <= '9' || k == j && text[k] == '-') {
				k++
			}
			if k > j && k < len(text) && text[k] == '"' {
				b.WriteString(text[j:k])
				i = k + 1
				continue
			}
		}
		i = copyString(&b, text, i)
	}
	return b.String()
}

// copyString copies the JSON string starting at text[i] ('"') and returns the index past it.
func copyString(b *strings.Builder, text string, i int) int {
	b.WriteByte('"')
	i++
	for i < len(text) {
		c := text[i]
		switch {
		case c == '"':
			b.WriteByte('"')
			return i + 1
		case c == '\\' && i+1 < len(text) && text[i+1] == 'u' && i+6 <= len(text):
			h, err := strconv.ParseUint(text[i+2:i+6], 16, 32)
			switch {
			case err != nil || h < 0xD800 || h > 0xDFFF:
				b.WriteString(text[i : i+6])
				i += 6
			case h < 0xDC00 && i+12 <= len(text) && text[i+6] == '\\' && text[i+7] == 'u' && isLowSurrogate(text[i+8:i+12]):
				b.WriteString(text[i : i+12])
				i += 12
			default:
				b.WriteString(`�`)
				i += 6
			}
		case c == '\\' && i+1 < len(text):
			b.WriteString(text[i : i+2])
			i += 2
		default:
			b.WriteByte(c)
			i++
		}
	}
	return i
}

func isLowSurrogate(hex string) bool {
	h, err := strconv.ParseUint(hex, 16, 32)
	return err == nil && h >= 0xDC00 && h <= 0xDFFF
}
