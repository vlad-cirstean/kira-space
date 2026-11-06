package queryplan

import "testing"

// Explainable's raw-semicolon guard, the Go twin of the TS test in
// console-explain-embedded-semicolon.spec.ts. A leading SELECT is not enough — the splitter that
// decided this was "one statement" can itself be fooled, and wrapping a merged statement in
// EXPLAIN would still execute whatever follows the `;` (Postgres's simple protocol runs every
// command in one Query message).
func TestExplainableEmbeddedSemicolon(t *testing.T) {
	cases := []struct {
		name string
		sql  string
		want bool
	}{
		{"plain select", "SELECT 1", true},
		{"select with trailing semicolon", "SELECT 1;", true},
		{"select with trailing semicolon and whitespace", "SELECT 1;  \n", true},
		{"merged select and delete", "SELECT 1; DELETE FROM t", false},
		{"merged select and delete, trailing semicolon", "SELECT 1; DELETE FROM t;", false},
		{"leading comment then merged statements", "-- note\nSELECT 1; DELETE FROM t", false},
		{"not a select at all", "DELETE FROM t", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Explainable(tc.sql); got != tc.want {
				t.Errorf("Explainable(%q) = %v, want %v", tc.sql, got, tc.want)
			}
		})
	}
}
