package keptvars

import (
	"encoding/base64"
	"encoding/json"
	"net/url"
	"sort"
	"strings"
)

// encodedFormsMin is the value length from which the encoded forms are masked too: a shorter value
// would over-mask ordinary text.
const encodedFormsMin = 8

// SecretText is the text a value shows as on the wire: a string as is, any other JSON as its literal.
func SecretText(e Entry) string {
	if e.Type == "string" {
		var s string
		if json.Unmarshal(e.Value, &s) == nil {
			return s
		}
	}
	return string(e.Value)
}

// forms lists the spellings of text a server may echo back.
func forms(text string) []string {
	if text == "" {
		return nil
	}
	out := []string{text}
	add := func(f string) {
		for _, o := range out {
			if o == f {
				return
			}
		}
		out = append(out, f)
	}
	if b, err := json.Marshal(text); err == nil {
		add(string(b[1 : len(b)-1]))
	}
	if len(text) >= encodedFormsMin {
		add(url.QueryEscape(text))
		add(url.PathEscape(text))
		add(base64.StdEncoding.EncodeToString([]byte(text)))
		add(base64.RawURLEncoding.EncodeToString([]byte(text)))
	}
	return out
}

// SecretPairs is the replacer argument list that maps each secret entry's forms to {{kept.<name>}},
// longest form first so a value that is a prefix of another cannot shadow it.
func SecretPairs(entries []Entry) []string {
	type pair struct{ old, new string }
	var ps []pair
	for _, e := range entries {
		ref := "{{kept." + e.Name + "}}"
		for _, f := range forms(SecretText(e)) {
			ps = append(ps, pair{f, ref})
		}
	}
	sort.SliceStable(ps, func(i, j int) bool { return len(ps[i].old) > len(ps[j].old) })
	out := make([]string, 0, len(ps)*2)
	for _, p := range ps {
		out = append(out, p.old, p.new)
	}
	return out
}

// SecretReplacer maps every form of the entries' values to {{kept.<name>}}; nil without entries.
func SecretReplacer(entries []Entry) *strings.Replacer {
	pairs := SecretPairs(entries)
	if len(pairs) == 0 {
		return nil
	}
	return strings.NewReplacer(pairs...)
}

// SecretRaw lists the raw value texts, for a masker that matches values only.
func SecretRaw(entries []Entry) []string {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		if t := SecretText(e); t != "" {
			out = append(out, t)
		}
	}
	return out
}
