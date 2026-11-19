package main

import "testing"

func TestStripMarkup(t *testing.T) {
	cases := []struct{ in, want string }{
		{"see [the doc](https://x.y/z) now", "see the doc now"},
		{"a <b>bold</b> tag", "a bold tag"},
		{"**bold** and *star* and _under_ and __dunder__", "bold and star and under and dunder"},
		{"run `go test` and ~~skip~~ it", "run go test and skip it"},
		{"  spaced \n out\t text ", "spaced out text"},
		{"snake_case_name stays, 2*3*4 stays", "snake_case_name stays, 2*3*4 stays"},
		{"**[*mix*](u)** <i>all</i>", "mix all"},
	}
	for _, c := range cases {
		if got := stripMarkup(c.in); got != c.want {
			t.Errorf("stripMarkup(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
