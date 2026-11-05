// Package ids is the one place Kira mints ids: UUIDv7 for keys and machine identifiers, prefixed
// nanoids for handles the user touches.
package ids

import (
	"strings"

	"github.com/google/uuid"
	gonanoid "github.com/matoous/go-nanoid/v2"
)

// Alphabet is Crockford base32 lowercase: no i, l, o, u look-alikes.
const Alphabet = "0123456789abcdefghjkmnpqrstvwxyz"

// NewID returns a UUIDv7 string. Use it for every key and machine identifier: it sorts by creation
// time and fits a Postgres uuid column.
func NewID() string { return uuid.Must(uuid.NewV7()).String() }

// IsUUID reports whether s is a canonical UUID of any RFC 9562 version, so legacy v4 ids pass.
func IsUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	_, err := uuid.Parse(s)
	return err == nil
}

// New returns "<prefix>_<nanoid>" with the registered body length.
func New(p Prefix) string {
	return string(p) + "_" + gonanoid.MustGenerate(Alphabet, p.size())
}

// Is reports whether s is exactly one id of prefix p.
func Is(p Prefix, s string) bool {
	body, ok := strings.CutPrefix(s, string(p)+"_")
	return ok && len(body) == p.size() && onlyAlphabet(body)
}

// Body returns the part after the first underscore, or s when there is none.
func Body(s string) string {
	_, body, ok := strings.Cut(s, "_")
	if !ok {
		return s
	}
	return body
}

// MaxClientIDBytes bounds a client-minted opaque id (tab and terminal ids).
const MaxClientIDBytes = 64

// IsClientID reports whether s is a safe client-minted opaque id: 1 to 64 bytes of [A-Za-z0-9_:-].
func IsClientID(s string) bool {
	if s == "" || len(s) > MaxClientIDBytes {
		return false
	}
	for i := range len(s) {
		c := s[i]
		ok := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == ':' || c == '-'
		if !ok {
			return false
		}
	}
	return true
}

// Pattern is regexp source matching any registered nanoid inside text.
var Pattern = buildPattern()

func onlyAlphabet(s string) bool {
	for _, r := range s {
		if !strings.ContainsRune(Alphabet, r) {
			return false
		}
	}
	return true
}

func buildPattern() string {
	parts := make([]string, 0, len(all))
	for _, p := range all {
		parts = append(parts, string(p))
	}
	return `\b(?:` + strings.Join(parts, "|") + `)_[0-9a-hjkmnp-tv-z]{10,21}\b`
}
