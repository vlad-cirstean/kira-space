package model

import "unicode/utf16"

// UTF16Len counts s in UTF-16 code units, the unit zod's string .max() counts in the renderer and
// the single length rule for names and text caps that mirror a zod schema. An invalid byte counts
// as one unit, as JS counts a lone surrogate.
func UTF16Len(s string) int {
	n := 0
	for _, r := range s {
		if u := utf16.RuneLen(r); u > 0 {
			n += u
		} else {
			n++
		}
	}
	return n
}

// TruncateUTF16 cuts s to at most max UTF-16 units on a rune boundary (a surrogate pair is never
// split) and reports whether it cut.
func TruncateUTF16(s string, max int) (string, bool) {
	n := 0
	for i, r := range s {
		u := utf16.RuneLen(r)
		if u < 1 {
			u = 1
		}
		if n+u > max {
			return s[:i], true
		}
		n += u
	}
	return s, false
}
