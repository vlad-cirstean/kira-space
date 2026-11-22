package keptvars

import "strings"

// RefSpan is one {{plain.<name>}} or {{secret.<name>}} span in a text. From and To are byte offsets of the
// whole span.
type RefSpan struct {
	Name string
	// Secret is true for {{secret.name}}: the kind the request wrote, which must match the temporary value's.
	Secret bool
	// Pipeline is true when the span carries a transform: {{plain.name | x}}.
	Pipeline bool
	From, To int
}

const (
	plainPrefix  = KindPlain + "."
	secretPrefix = KindSecret + "."
)

// IsKeptName reports whether a reference name (as apivars.Names returns it) is in the temporary-variable namespace.
func IsKeptName(name string) bool {
	return strings.HasPrefix(name, plainPrefix) || strings.HasPrefix(name, secretPrefix)
}

// RefSpans lists the temporary-variable references of text with the same grammar as the other {{name}}
// references: from `{{` to the next `}}`, no nesting.
func RefSpans(text string) []RefSpan {
	var out []RefSpan
	i := 0
	for i < len(text) {
		open := strings.Index(text[i:], "{{")
		if open < 0 {
			break
		}
		open += i
		end := strings.Index(text[open+2:], "}}")
		if end < 0 {
			break
		}
		end += open + 2
		i = end + 2
		inner := strings.TrimSpace(text[open+2 : end])
		if !IsKeptName(inner) {
			continue
		}
		secret := strings.HasPrefix(inner, secretPrefix)
		prefix := plainPrefix
		if secret {
			prefix = secretPrefix
		}
		name, _, pipeline := strings.Cut(inner[len(prefix):], "|")
		out = append(out, RefSpan{Name: strings.TrimSpace(name), Secret: secret, Pipeline: pipeline, From: open, To: end + 2})
	}
	return out
}

// ToUTF16 converts a byte offset in text to UTF-16 code units, the unit a browser indexes strings by.
func ToUTF16(text string, byteOffset int) int {
	n := 0
	for _, r := range text[:byteOffset] {
		n++
		if r >= 0x10000 {
			n++
		}
	}
	return n
}
