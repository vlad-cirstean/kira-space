// Package textloss lists the protected tokens a rewrite dropped: numbers, quoted and backtick spans,
// identifiers, negation counts, and for prose text paths and names. Pure; no I/O.
package textloss

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

var (
	actionRe   = regexp.MustCompile(`\{\{.*?\}\}`)
	numberRe   = regexp.MustCompile(`\d+(?:\.\d+)?`)
	backtickRe = regexp.MustCompile("`[^`]+`")
	quotedRe   = regexp.MustCompile(`"[^"\n]+"`)
	identRe    = regexp.MustCompile(`[A-Za-z0-9]+(?:[_.][A-Za-z0-9]+)+|\b[a-z]+[A-Z][A-Za-z0-9]*\b`)
	negationRe = regexp.MustCompile(`(?i)\b(?:not|never|no|only|except)\b|n't\b`)
	wordRe     = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]*`)
	nameRe     = regexp.MustCompile(`\b[A-Z][A-Za-z0-9]*\b`)
	capsRe     = regexp.MustCompile(`\b[A-Z]{2,}\b`)
)

// Lost lists the protected tokens of old that cur dropped, for prompt text: template actions,
// numbers, backtick and double-quoted spans, identifiers, schema fields and negation counts.
func Lost(old, cur string, fields []string) []string { return lost(old, cur, fields, false) }

// LostText is Lost for prose text: also file paths (tokens holding "/") and names
// (capitalized words other than the first word of old, and all-caps words of 2+ letters).
func LostText(old, cur string) []string { return lost(old, cur, nil, true) }

func lost(old, cur string, fields []string, prose bool) []string {
	var out []string
	oa, na := actionRe.FindAllString(old, -1), actionRe.FindAllString(cur, -1)
	if strings.Join(oa, "\x00") != strings.Join(na, "\x00") {
		out = append(out, "actions "+strings.Join(oa, " ")+" changed to "+strings.Join(na, " "))
	}
	body := func(s string) string { return actionRe.ReplaceAllString(s, "") }
	ob, nb := body(old), body(cur)
	for _, re := range []*regexp.Regexp{numberRe, backtickRe, quotedRe, identRe} {
		have := counts(re.FindAllString(nb, -1))
		for tok, n := range counts(re.FindAllString(ob, -1)) {
			if have[tok] < 1 && n > 0 {
				out = append(out, tok)
			}
		}
	}
	if prose {
		out = append(out, lostSet(paths(ob), paths(nb))...)
		out = append(out, lostSet(names(ob), names(nb))...)
	}
	nw := counts(wordRe.FindAllString(nb, -1))
	ow := counts(wordRe.FindAllString(ob, -1))
	for _, f := range fields {
		if ow[f] > 0 && nw[f] == 0 {
			out = append(out, "field "+f)
		}
	}
	nn, on := counts(lowerAll(negationRe.FindAllString(nb, -1))), counts(lowerAll(negationRe.FindAllString(ob, -1)))
	for w, c := range on {
		if nn[w] < c {
			out = append(out, fmt.Sprintf("negation %q (%d to %d)", w, c, nn[w]))
		}
	}
	slices.Sort(out)
	return out
}

func lostSet(old, cur []string) []string {
	var out []string
	for _, t := range old {
		if !slices.Contains(cur, t) {
			out = append(out, t)
		}
	}
	return out
}

// paths are the unique tokens holding "/", trimmed of surrounding punctuation.
func paths(s string) []string {
	var out []string
	for _, f := range strings.Fields(s) {
		f = strings.Trim(f, ".,;:()[]{}\"'`")
		if strings.Contains(f, "/") && !slices.Contains(out, f) {
			out = append(out, f)
		}
	}
	return out
}

// names are capitalized words after the first word, plus all-caps words anywhere, unique.
func names(s string) []string {
	var out []string
	add := func(w string) {
		if !slices.Contains(out, w) {
			out = append(out, w)
		}
	}
	first := wordRe.FindStringIndex(s)
	for _, m := range nameRe.FindAllStringIndex(s, -1) {
		if first != nil && m[0] == first[0] {
			continue
		}
		add(s[m[0]:m[1]])
	}
	for _, w := range capsRe.FindAllString(s, -1) {
		add(w)
	}
	return out
}

func lowerAll(in []string) []string {
	for i := range in {
		in[i] = strings.ToLower(in[i])
	}
	return in
}

func counts(in []string) map[string]int {
	m := map[string]int{}
	for _, s := range in {
		m[s]++
	}
	return m
}
