package model

import "strings"

// EncodeURIComponent mirrors JS's encodeURIComponent: every byte except A-Za-z0-9 and
// - _ . ! ~ * ' ( ) is percent-escaped (uppercase hex). Operating byte-wise, not rune-wise,
// reproduces encodeURIComponent's own behaviour on non-ASCII text: each UTF-8 byte of the
// character is escaped individually.
//
// This lives here, not in internal/connections (which needs the same encoding for URI userinfo)
// or internal/tree (which needs it for path segment names): tree.go's DecodePath calling into
// connections would be an import cycle in the wrong direction, so both callers share this one
// implementation instead.
func EncodeURIComponent(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if isURIComponentUnreserved(c) {
			b.WriteByte(c)
		} else {
			b.WriteByte('%')
			b.WriteByte(hexDigit(c >> 4))
			b.WriteByte(hexDigit(c & 0x0F))
		}
	}
	return b.String()
}

func isURIComponentUnreserved(c byte) bool {
	switch {
	case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9':
		return true
	}
	switch c {
	case '-', '_', '.', '!', '~', '*', '\'', '(', ')':
		return true
	}
	return false
}

func hexDigit(n byte) byte {
	if n < 10 {
		return '0' + n
	}
	return 'A' + (n - 10)
}

// DecodeURIComponent percent-decodes s; '+' is a literal plus, matching JS's decodeURIComponent
// (unlike application/x-www-form-urlencoded decoding). A malformed %-sequence is left as literal
// text rather than erroring — every value this ever decodes was produced by EncodeURIComponent
// above (our own writes) or by a real WHATWG URL implementation (an imported URI or a path
// segment this app itself encoded), so a malformed sequence never arises for it.
func DecodeURIComponent(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '%' && i+2 < len(s) {
			if hi, ok1 := hexVal(s[i+1]); ok1 {
				if lo, ok2 := hexVal(s[i+2]); ok2 {
					b.WriteByte(hi<<4 | lo)
					i += 2
					continue
				}
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func hexVal(c byte) (byte, bool) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', true
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, true
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10, true
	}
	return 0, false
}
