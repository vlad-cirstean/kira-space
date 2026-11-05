package apivars

import (
	"net/url"
	"sort"
	"strings"
)

// Replacer builds P9 D6's own strings.Replacer over every distinct secret span actually
// substituted — nil when there is nothing to mask. A strings.Replacer can only over-mask (text
// that happens to occur elsewhere is masked too) and never under-mask a surface that carries the
// registered text verbatim — D6's own stated property, which does not by itself cover a surface
// that carries a *re-encoded* form instead: a urlencoded body's rendered wire text is
// url.QueryEscape's own output (finding 6, round 1), never the plaintext buildURLEncoded started
// from, and a secret used in a URL *path* segment is url.PathEscape's own output instead
// (finding 4, round 2) — PathEscape differs from QueryEscape for several characters (a space
// becomes %20 via PathEscape, + via QueryEscape). Both re-encoded forms are registered as
// additional pairs below, alongside the rendered one itself.
//
// P17 D9/§5: `used` carries UsedSecret.Rendered — the exact text the substitution walk wrote onto
// the wire, which is a secret's plaintext ONLY when its span carried no pipeline. With one
// (`{{token | base64}}`), Rendered is the base64 form: the plaintext never reaches the wire at
// all, so masking the plaintext (as this replacer used to, keyed by name) would leave the actual
// base64-encoded credential in every copyable surface, still under the pane's claim that N secret
// values were masked. Registering Rendered — whatever it is — is what closes that gap. One entry
// per distinct (Name, Placeholder) pair (D9(d)): a request using both `{{token}}` and
// `{{token | base64}}` produces two entries, so both wire forms are masked — a plaintext-keyed
// model could not represent that at all.
//
// D9(f): entries are sorted by len(Rendered) descending before the replacer is built —
// strings.NewReplacer matches at each position by the earliest-listed pattern, so a longer
// rendered form that contains a shorter one (e.g. two secrets where one's rendered text happens to
// be a prefix of another's) must be listed first, or the shorter pattern would shadow it.
func Replacer(used []UsedSecret) *strings.Replacer {
	if len(used) == 0 {
		return nil
	}
	sorted := make([]UsedSecret, len(used))
	copy(sorted, used)
	sort.SliceStable(sorted, func(i, j int) bool { return len(sorted[i].Rendered) > len(sorted[j].Rendered) })

	pairs := make([]string, 0, len(sorted)*6)
	for _, u := range sorted {
		if u.Rendered == "" {
			continue
		}
		pairs = append(pairs, u.Rendered, u.Placeholder)
		// url.QueryEscape rewrites space/+//=&%@: and non-ASCII — most real passwords and any
		// base64 token — so a urlencoded body's rendered wire text (encodeURLEncodedFields' own
		// output, wire.go's renderRequestBody) would otherwise carry the rendered text in
		// cleartext, just percent-encoded, with the pane still claiming "N secret values shown as
		// {{name}}".
		if encoded := url.QueryEscape(u.Rendered); encoded != u.Rendered {
			pairs = append(pairs, encoded, u.Placeholder)
		}
		// A secret used in a URL *path* segment (e.g.
		// https://api.example.com/{{secret}}/orders) is percent-encoded via url.PathEscape /
		// EscapedPath(), which differs from QueryEscape for several characters (a space becomes
		// %20 via PathEscape but + via QueryEscape) — that form isn't covered by the QueryEscape
		// pair above and leaked in FinalURL, Timeline.Hops[].URL and Wire.Request's request line.
		if encoded := url.PathEscape(u.Rendered); encoded != u.Rendered {
			pairs = append(pairs, encoded, u.Placeholder)
		}
	}
	if len(pairs) == 0 {
		return nil
	}
	return strings.NewReplacer(pairs...)
}

// DistinctNames counts distinct Names in used — D9(e): Wire.MaskedSecrets means "how many
// secrets were masked", and two spellings of one secret (plain and piped) are still one secret.
func DistinctNames(used []UsedSecret) int {
	seen := make(map[string]bool, len(used))
	for _, u := range used {
		seen[u.Name] = true
	}
	return len(seen)
}
