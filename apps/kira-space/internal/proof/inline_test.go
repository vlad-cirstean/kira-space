package proof

import (
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

func TestInlineToJSON(t *testing.T) {
	cases := []struct{ name, src, want, err string }{
		{"precision and order", "rows: [[9007199254740993, 1.50, 0x1F, 1_000]]\ncolumns: [{name: b}, {name: a}]\nz: null\n",
			`{"rows":[[9007199254740993,1.50,31,1000]],"columns":[{"name":"b"},{"name":"a"}],"z":null}`, ""},
		{"strings stay strings", "a: \"007\"\nb: yes_text\nc: |\n  line1\n  line2\n", `{"a":"007","b":"yes_text","c":"line1\nline2\n"}`, ""},
		{"alias", "a: &x 1\nb: *x\n", "", "line 1: anchors are not allowed"},
		{"merge", "base: {a: 1}\nb:\n  <<: {c: 2}\n", "", "line 3: merge keys are not allowed"},
		{"int key", "1: a\n", "", "line 1: mapping keys must be strings"},
		{"duplicate", "a: 1\na: 2\n", "", "duplicate key"},
		{"inf", "a: .inf\n", "", "line 1: number is not finite"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var doc yaml.Node
			if err := yaml.Unmarshal([]byte(tc.src), &doc); err != nil {
				if tc.err != "" && strings.Contains(err.Error(), "duplicate") {
					return // yaml.v3 refuses duplicate keys itself
				}
				t.Fatal(err)
			}
			got, err := InlineToJSON(&doc)
			if tc.err != "" {
				if err == nil || !strings.Contains(err.Error(), tc.err) {
					t.Fatalf("err = %v, want %q", err, tc.err)
				}
				return
			}
			if err != nil || string(got) != tc.want {
				t.Fatalf("got %s, %v; want %s", got, err, tc.want)
			}
		})
	}
}
