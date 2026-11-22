package jsengine

import "testing"

func TestDeclares(t *testing.T) {
	cases := []struct {
		name, src, want string
	}{
		{"member use", `plain.x = 1; return plain.x;`, ""},
		{"member of other", `const o = {plain: 1}; return o.plain;`, ""},
		{"const", `const plain = {};`, "plain"},
		{"var in loop", `for (var secret = 0; secret < 1; secret++) {}`, "secret"},
		{"let destructure", `let {a, b: [c, {plain}]} = x;`, "plain"},
		{"destructure default", `const [a = 1, secret = 2] = x;`, "secret"},
		{"rest", `const {a, ...plain} = x;`, "plain"},
		{"array rest", `const [a, ...secret] = x;`, "secret"},
		{"function name", `function plain() {}`, "plain"},
		{"nested param", `function f() { return function (a, secret) {}; }`, "secret"},
		{"param default", `const f = (a, {plain} = {}) => a;`, "plain"},
		{"arrow param", `const f = secret => secret;`, "secret"},
		{"rest param", `const f = (...plain) => 1;`, "plain"},
		{"catch", `try {} catch (plain) {}`, "plain"},
		{"class", `class secret {}`, "secret"},
		{"for of const", `for (const plain of xs) {}`, "plain"},
		{"unrelated", `const a = 1; function f(b) {}`, ""},
	}
	for _, c := range cases {
		prog, err := parseSource(c.src)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if got := declares(prog, []string{"plain", "secret"}); got != c.want {
			t.Errorf("%s: declares = %q, want %q", c.name, got, c.want)
		}
	}
}
