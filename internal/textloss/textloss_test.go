package textloss

import (
	"slices"
	"testing"
)

func TestLost(t *testing.T) {
	cases := []struct {
		name, old, cur string
		fields         []string
		prose          bool
		want           []string
	}{
		{"nothing lost", "Set `max` to 30 s.", "Set `max` 30 s.", nil, false, nil},
		{"number and backtick", "Timeout is 30 s in `cfg.go`.", "Timeout in file.", nil, false, []string{"30", "`cfg.go`", "cfg.go"}},
		{"negation counted", "Never retry; do not wait.", "Never retry; wait.", nil, false, []string{`negation "not" (1 to 0)`}},
		{"negation count kept", "No retry. Not twice.", "No retry; not twice.", nil, false, nil},
		{"schema field", "Fill reason and fact.", "Fill fact.", []string{"reason", "fact"}, false, []string{"field reason"}},
		{"template action", "Hello {{.Name}}.", "Hello.", nil, false, []string{"actions {{.Name}} changed to "}},
		{"path and name only for prose text", "Reader checks internal/docs/store.go for Postgres.", "Reader checks store.", nil, true,
			[]string{"Postgres", "internal/docs/store.go", "store.go"}},
		{"first word is not a name", "Retries use TLS.", "Retries use TLS.", nil, true, nil},
		{"prose classes off for prompts", "Use Postgres at a/b.", "Use it.", nil, false, nil},
	}
	for _, c := range cases {
		got := lost(c.old, c.cur, c.fields, c.prose)
		if len(got) == 0 {
			got = nil
		}
		if !slices.Equal(got, c.want) {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}
