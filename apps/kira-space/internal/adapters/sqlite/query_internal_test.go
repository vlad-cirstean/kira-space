// query_internal_test.go covers firstTopLevelSemicolon directly (P94 pass 2, CLAUDE.md §9): its
// quote/comment/backtick states interact (an escaped quote, a comment containing a quote character,
// a semicolon inside each), which assertSingleStatement's own callers exercise only indirectly.
package sqlite

import "testing"

func TestFirstTopLevelSemicolon(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want int
	}{
		{"no semicolon", "SELECT 1", -1},
		{"trailing semicolon", "SELECT 1;", 8},
		{"semicolon then more", "SELECT 1; SELECT 2", 8},
		{"semicolon inside single-quoted string is not top-level", "SELECT ';'", -1},
		{"semicolon inside double-quoted identifier is not top-level", `SELECT "a;b"`, -1},
		{"semicolon inside backtick identifier is not top-level", "SELECT `a;b`", -1},
		{"doubled single quote does not close the string early", "SELECT 'it''s; fine'", -1},
		{"doubled double quote does not close early", `SELECT "a""b;c"`, -1},
		{"doubled backtick does not close early", "SELECT `a``b;c`", -1},
		{"semicolon after a closed string is top-level", "SELECT ''; SELECT 1", 9},
		{"semicolon inside a line comment is not top-level", "SELECT 1 -- ;\n", -1},
		{"real semicolon after a line comment", "SELECT 1 -- note\n;", 17},
		{"semicolon inside a block comment is not top-level", "SELECT 1 /* ; */", -1},
		{"nested block comment hides a semicolon", "SELECT 1 /* /* ; */ x */", -1},
		{"real semicolon after a block comment", "SELECT 1 /* note */;", 19},
		{"unterminated block comment has no top-level semicolon", "SELECT 1 /* ;", -1},
		{"unterminated string has no top-level semicolon", "SELECT '; ", -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := firstTopLevelSemicolon(tt.in); got != tt.want {
				t.Fatalf("firstTopLevelSemicolon(%q) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}
