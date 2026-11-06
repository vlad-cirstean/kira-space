package apivars

import (
	"encoding/base64"
	"errors"
	"net/url"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// The Go twin of packages/api-core/src/http/transforms.ts — the six-transform closed
// vocabulary a `{{name | transform}}` pipe may apply, byte-identical to the TS side and pinned by
// the same shared corpus. Exactly six, zero-argument, closed.
//
// transformNames is read as plain text by packages/api-core/test/go-ts-api-parity.spec.ts
// (extractGoStringSet) — its shape (a `map[string]bool` literal, one `"name": true` per line) must
// stay exactly this, or that parity check breaks.
var transformNames = map[string]bool{
	"base64":       true,
	"base64decode": true,
	"upper":        true,
	"lower":        true,
	"urlencode":    true,
	"urldecode":    true,
}

// IsTransformName reports whether name is one of the six closed-vocabulary transforms.
func IsTransformName(name string) bool {
	return transformNames[name]
}

// upperCaser/lowerCaser: golang.org/x/text/cases with the "undetermined language" tag applies
// Unicode's *default* case mapping — the same unconditional special-casing rules (ß -> SS, the
// U+FB01 "fi" ligature -> "FI") ECMAScript's String.prototype.toUpperCase/toLowerCase apply,
// unlike strings.ToUpper/ToLower, which only ever does the simple,
// one-rune-to-one-rune Unicode mapping and leaves ß and ligatures untouched.
var (
	upperCaser = cases.Upper(language.Und)
	lowerCaser = cases.Lower(language.Und)
)

// forgivingBase64Decode mirrors the WHATWG "forgiving-base64" algorithm browsers' own atob()
// implements: ASCII whitespace is stripped first;
// then, ONLY when the remaining length is a multiple of 4, up to 2 trailing '=' characters are
// stripped; a remaining length of 1 mod 4 has no valid padding and is rejected; any '=' still
// present past that point (not a multiple of 4, or more than 2 trailing) is illegal and rejected
// too — unconditionally padding out to a multiple of 4 (the old approach) silently accepted a
// string like "YQ=" (length 3, one stray '=') that atob correctly rejects, so the same
// {{name | base64decode}} pipe disagreed between stage 1 (TS/atob) and stage 2 (Go) whenever the
// input carried non-canonical padding.
func forgivingBase64Decode(s string) ([]byte, error) {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch r {
		case ' ', '\t', '\n', '\f', '\r':
			continue
		}
		b.WriteRune(r)
	}
	stripped := b.String()
	if len(stripped)%4 == 0 {
		trimmed := strings.TrimRight(stripped, "=")
		if len(stripped)-len(trimmed) > 2 {
			return nil, errors.New("apivars: invalid base64 padding")
		}
		stripped = trimmed
	}
	if len(stripped)%4 == 1 {
		return nil, errors.New("apivars: invalid base64 length")
	}
	if strings.ContainsRune(stripped, '=') {
		return nil, errors.New("apivars: invalid base64 padding")
	}
	return base64.RawStdEncoding.DecodeString(stripped)
}

// applyTransform is one step of ApplyPipeline. The table: base64/base64decode round-trip
// standard, padded base64; base64decode additionally requires the decoded bytes be valid UTF-8
// (matching TS's `TextDecoder('utf-8', {fatal: true})`) — invalid base64 *or* invalid UTF-8 is a
// failure. upper/lower never fail. urlencode/urldecode are url.QueryEscape/QueryUnescape
// verbatim — the escaper this repo already uses for a urlencoded body — except urldecode
// additionally requires the decoded bytes be valid UTF-8 (url.QueryUnescape alone happily
// returns invalid UTF-8 for e.g. `%FF`, where TS's decodeURIComponent throws).
func applyTransform(name, s string) (string, bool) {
	switch name {
	case "base64":
		return base64.StdEncoding.EncodeToString([]byte(s)), true
	case "base64decode":
		decoded, err := forgivingBase64Decode(s)
		if err != nil || !utf8.Valid(decoded) {
			return "", false
		}
		return string(decoded), true
	case "upper":
		return upperCaser.String(s), true
	case "lower":
		return lowerCaser.String(s), true
	case "urlencode":
		return url.QueryEscape(s), true
	case "urldecode":
		decoded, err := url.QueryUnescape(s)
		if err != nil || !utf8.ValidString(decoded) {
			return "", false
		}
		return decoded, true
	default:
		return "", false
	}
}

// ApplyPipeline applies each transform in pipeline to value, left to right,
// returning ok=false the instant any step fails (nothing is emitted half-transformed). An
// empty pipeline returns value unchanged. Every name in pipeline is expected to already be a valid
// transform (ParseReference only ever produces one) — an unrecognised name here is itself treated
// as a failure rather than a panic, so a future caller passing an unvalidated pipeline fails safe.
func ApplyPipeline(pipeline []string, value string) (string, bool) {
	out := value
	for _, name := range pipeline {
		applied, ok := applyTransform(name, out)
		if !ok {
			return "", false
		}
		out = applied
	}
	return out, true
}
