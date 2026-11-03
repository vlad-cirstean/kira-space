package configfs

import (
	"reflect"
	"testing"

	"go.yaml.in/yaml/v3"
)

// The runs migration exports every user's scripts through RenderYAML: a value a YAML reader
// re-types or mangles would lose data.
func TestRenderYAML_roundTrip(t *testing.T) {
	type item struct {
		Name  string   `json:"name"`
		Tags  []string `json:"tags"`
		Count int      `json:"count"`
		On    bool     `json:"on"`
		Cost  float64  `json:"cost"`
	}
	for _, s := range []string{
		"yes", "no", "true", "null", "~", "1.0", "1e3", "0x10", "007", " leading", "trailing ", "# hash", "a: b", "a #b",
		"- dash", "line1\nline2", "line1\nline2\n", "\nstartsnl", "  indented\nsecond", "tab\there", "ünï ✓", `quote"s'`, "{flow}", "", "@at", "*star", "&amp", "!bang", "%pct",
	} {
		in := item{Name: s, Tags: []string{s, "x"}, Count: 3, On: true, Cost: 0.25}
		out, err := RenderYAML(in, "Kira test.\nSecond line.")
		if err != nil {
			t.Fatalf("%q: %v", s, err)
		}
		var back item
		if err := yaml.Unmarshal(out, &back); err != nil {
			t.Fatalf("%q: read back: %v\n%s", s, err, out)
		}
		if !reflect.DeepEqual(in, back) {
			t.Errorf("%q: got %+v\n%s", s, back, out)
		}
	}
}
