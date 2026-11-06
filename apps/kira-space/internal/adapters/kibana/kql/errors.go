package kql

import "strings"

// SyntaxError reports where a query stopped parsing. Pos is the 1-based character position.
type SyntaxError struct {
	Pos      int
	Expected []string
	Found    string
}

func (e *SyntaxError) Error() string {
	return "KQL syntax error at position " + itoa(e.Pos) + ": expected " + strings.Join(e.Expected, ", ") + ", found " + e.Found
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for ; n > 0; n /= 10 {
		b = append([]byte{byte('0' + n%10)}, b...)
	}
	return string(b)
}
