// P108 F14: forgivingBase64Decode's padding/length arithmetic is exactly the boundary logic
// CLAUDE.md's testing bar calls out — table-driven against the WHATWG forgiving-base64 truth table
// (the same table atob() enforces), not just the shared substitution.json corpus's two cases.
package apivars_test

import (
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/apivars"
)

func TestBase64DecodeTransform_MatchesWHATWGForgivingBase64(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
		ok    bool
	}{
		{"unpadded, valid", "YQ", "a", true},
		{"one stray '=' on a non-multiple-of-4 length is illegal", "YQ=", "", false},
		{"canonical single '=' padding", "aGk=", "hi", true},
		{"single trailing '=' padding on a 4-length input", "YQI=", "a\x02", true},
		{"three trailing '=' exceeds the 2-character cap", "====", "", false},
		{"length mod 4 == 1 has no valid decoding", "Y", "", false},
		{"embedded whitespace is stripped before decoding", "a GVsbG8=", "hello", true},
		{"'=' not in trailing position is illegal", "=YQI", "", false},
		{"length mod 4 == 2, no padding needed", "YWJjZA", "abcd", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := apivars.ApplyPipeline([]string{"base64decode"}, tc.input)
			if ok != tc.ok {
				t.Fatalf("ApplyPipeline(%q) ok = %v, want %v (got %q)", tc.input, ok, tc.ok, got)
			}
			if tc.ok && got != tc.want {
				t.Errorf("ApplyPipeline(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}
