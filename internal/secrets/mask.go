package secrets

import (
	"encoding/json"
	"sort"
	"strings"
)

// Mask replaces a secret value wherever app-stored output would show it.
const Mask = "••••"

// Masker replaces secret values in text. A nil Masker masks nothing. It over-masks, never
// under-masks: a 1- or 2-char secret masks those chars everywhere.
type Masker struct{ r *strings.Replacer }

// NewMasker masks each non-empty value, each non-empty line of a multi-line value, and the
// JSON-escaped form of both.
func NewMasker(values ...string) *Masker {
	seen := map[string]bool{}
	var pats []string
	add := func(p string) {
		if p != "" && !seen[p] {
			seen[p] = true
			pats = append(pats, p)
		}
	}
	for _, v := range values {
		forms := []string{v}
		if strings.ContainsAny(v, "\r\n") {
			forms = append(forms, strings.FieldsFunc(v, func(r rune) bool { return r == '\n' || r == '\r' })...)
		}
		for _, f := range forms {
			add(f)
			if b, err := json.Marshal(f); err == nil {
				add(string(b[1 : len(b)-1]))
			}
		}
	}
	if len(pats) == 0 {
		return nil
	}
	// strings.Replacer tries patterns in argument order at each position: longest first.
	sort.SliceStable(pats, func(i, j int) bool { return len(pats[i]) > len(pats[j]) })
	args := make([]string, 0, 2*len(pats))
	for _, p := range pats {
		args = append(args, p, Mask)
	}
	return &Masker{r: strings.NewReplacer(args...)}
}

func (m *Masker) Mask(s string) string {
	if m == nil {
		return s
	}
	return m.r.Replace(s)
}
