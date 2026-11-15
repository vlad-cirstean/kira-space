package taskfiles

import (
	"strings"
	"testing"
)

const hintHead = "---\nname: P\nsummary: s\nsteps:\n"

// One row per YAML fix-hint class: the problem carries the line in the file and a hint that ends in the fix.
func TestProofMDYamlHints(t *testing.T) {
	cases := []struct {
		name, body string
		line       int
		want       string
	}{
		{"tab", hintHead + "\t- id: a\n---\n", 5, "tab"},
		{"unquoted colon", hintHead + "  - id: a\n    description: Totals: agree\n---\n", 6, "quote"},
		{"bad indent", hintHead + "  - id: a\n   description: x\n---\n", 4, "indent"},
		{"unclosed flow", hintHead + "  - {id: a, description: x\n---\n", 4, "not closed"},
		{"duplicate key", hintHead + "  - id: a\n    id: b\n---\n", 6, "duplicate key \"id\""},
		{"no frontmatter", "name: P\n", 1, "Start the file with ---"},
		{"not a mapping", "---\n- a\n---\n", 2, "must be a mapping"},
	}
	for _, c := range cases {
		_, probs := parseProofMD([]byte(c.body), proofFiles{}, map[string]bool{})
		if len(probs) == 0 {
			t.Errorf("%s: accepted", c.name)
			continue
		}
		p := probs[0]
		if c.line > 0 && p.Line != c.line || !strings.Contains(strings.ToLower(p.Msg), strings.ToLower(c.want)) {
			t.Errorf("%s: problem = %+v, want line %d and %q", c.name, p, c.line, c.want)
		}
	}
}

const stepTable = "  - id: t\n    description: Rows\n    view: table\n    inline: {columns: [{name: a, type: text}], rows: [[x]]}\n"

// The unit is judged whole: a missing, an invalid and an unused side file each fail the unit, and
// the problem names the file that caused it.
func TestProofMDSideFiles(t *testing.T) {
	good := []byte(`{"columns":[{"name":"a","type":"text"}],"rows":[["x"]]}`)
	cases := []struct {
		name  string
		steps string
		files proofFiles
		want  string
	}{
		{"valid", "  - id: d\n    description: Rows\n    view: table\n    data: d.json\n", proofFiles{"d.json": good}, ""},
		{"missing data file", "  - id: d\n    description: Rows\n    view: table\n    data: d.json\n", proofFiles{}, "d.json not found in this folder"},
		{"invalid data file", "  - id: d\n    description: Rows\n    view: table\n    data: d.json\n", proofFiles{"d.json": []byte(`{"columns":[]}`)}, "d.json"},
		{"missing image", "  - id: s\n    description: Shot\n    view: image\n    data: s.png\n", proofFiles{}, "s.png not found in this folder"},
		{"image step with inline", "  - id: s\n    description: Shot\n    view: image\n    inline: x\n", proofFiles{}, "an image step takes a file"},
		{"both inline and data", "  - id: s\n    description: Rows\n    view: table\n    inline: {}\n    data: s.json\n", proofFiles{}, "exactly one of inline"},
		{"duplicate id", stepTable + stepTable, proofFiles{}, "the id is used twice"},
		{"bad id", "  - id: Bad_Id\n    description: Rows\n    view: table\n    inline: {}\n", proofFiles{}, "id must be a-z"},
		{"unknown field", stepTable + "    colour: red\n", proofFiles{}, "unknown step field"},
		{"unknown view", "  - id: t\n    description: Rows\n    view: heatmap\n    inline: {}\n", proofFiles{}, "view: must be one of"},
	}
	for _, c := range cases {
		used := map[string]bool{}
		_, probs := parseProofMD([]byte(hintHead+c.steps+"---\n"), c.files, used)
		if c.want == "" {
			if len(probs) != 0 || !used["d.json"] {
				t.Errorf("%s: problems %+v, used %v", c.name, probs, used)
			}
			continue
		}
		if len(probs) == 0 || !strings.Contains(probs[0].Msg, c.want) {
			t.Errorf("%s: problems = %+v, want %q", c.name, probs, c.want)
		}
	}
}
