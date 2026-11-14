package configfs

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
)

const skipSchema = `{
  "type": "object",
  "additionalProperties": false,
  "required": ["name"],
  "properties": {
    "name": {"type": "string"},
    "items": {"type": "array", "items": {"$ref": "#/$defs/item"}}
  },
  "$defs": {"item": {
    "type": "object",
    "additionalProperties": false,
    "required": ["id", "name"],
    "properties": {
      "id": {"type": "string", "pattern": "^[a-z0-9-]+$"},
      "name": {"type": "string"},
      "items": {"type": "array", "items": {"$ref": "#/$defs/item"}}
    }
  }}
}`

type skItem struct {
	ID    string   `json:"id"`
	Name  string   `json:"name"`
	Items []skItem `json:"items"`
}

type skFile struct {
	Name  string   `json:"name"`
	Items []skItem `json:"items"`
}

func dupRule() Rule[skFile] {
	return Rule[skFile]{Doc: "ids are unique", Check: func(f skFile, _ Locator) *Problem {
		seen := map[string]bool{}
		var walk func(items []skItem, ptr string) *Problem
		walk = func(items []skItem, ptr string) *Problem {
			for i, it := range items {
				p := fmt.Sprintf("%s/%d", ptr, i)
				if seen[it.ID] {
					return &Problem{Path: p + "/id", Message: "duplicate"}
				}
				seen[it.ID] = true
				if q := walk(it.Items, p+"/items"); q != nil {
					return q
				}
			}
			return nil
		}
		return walk(f.Items, "/items")
	}}
}

var skipElem = regexp.MustCompile(`^(?:/items/\d+)+`)

func skipDecoder(t *testing.T, max int) func(string, []byte) (skFile, []Skipped, *Problem) {
	t.Helper()
	d, _, err := YAMLSchemaSkip[skFile]([]byte(skipSchema), Skip{
		Element: func(p string) (string, bool) {
			m := skipElem.FindString(p)
			return m, m != ""
		},
		StandIn: func(_ any, n int) any {
			return map[string]any{"id": fmt.Sprintf("skipped-%d", n), "name": "skipped"}
		},
		Max: max,
	}, dupRule())
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestYAMLSchemaSkip(t *testing.T) {
	cases := []struct {
		name, src string
		max       int
		ptrs      []string
		lines     []int
		whole     string // substring of the whole-file problem; "" for none
		wholeLine int
		check     func(t *testing.T, f skFile)
	}{
		{name: "bad element mid-array", max: 10,
			src:  "name: f\nitems:\n  - id: a\n    name: A\n  - id: BAD ID\n    name: B\n  - id: c\n    name: C\n",
			ptrs: []string{"/items/1"}, lines: []int{5},
			check: func(t *testing.T, f skFile) {
				if len(f.Items) != 3 || f.Items[1].ID != "skipped-0" || f.Items[2].ID != "c" {
					t.Errorf("items %+v", f.Items)
				}
			}},
		{name: "nested element only", max: 10,
			src:  "name: f\nitems:\n  - id: a\n    name: A\n    items:\n      - id: x\n        name: X\n      - id: Y!\n        name: Y\n",
			ptrs: []string{"/items/0/items/1"}, lines: []int{8},
			check: func(t *testing.T, f skFile) {
				if f.Items[0].ID != "a" || f.Items[0].Items[0].ID != "x" {
					t.Errorf("items %+v", f.Items)
				}
			}},
		{name: "parent and child both bad", max: 10,
			src:  "name: f\nitems:\n  - id: a\n    name: A\n    items:\n      - id: X!\n        name: X\n  - id: B!\n    name: B\n",
			ptrs: []string{"/items/0/items/0", "/items/1"}, lines: []int{6, 8}},
		{name: "duplicate id skips later", max: 10,
			src:  "name: f\nitems:\n  - id: a\n    name: A\n  - id: a\n    name: B\n",
			ptrs: []string{"/items/1"}, lines: []int{5}},
		{name: "max reached", max: 2,
			src:  "name: f\nitems:\n  - id: A!\n    name: A\n  - id: B!\n    name: B\n  - id: C!\n    name: C\n",
			ptrs: []string{"/items/0", "/items/1"}, lines: []int{3, 5},
			whole: "invalid format", wholeLine: 7},
		{name: "top-level problem stays whole-file", max: 10,
			src:   "items:\n  - id: A!\n    name: A\n",
			whole: "name is required"},
		{name: "yaml syntax", max: 10, src: "name: f\n  bad: [\n", whole: "syntax", wholeLine: 2},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f, sk, p := skipDecoder(t, c.max)("f.yaml", []byte(c.src))
			if len(sk) != len(c.ptrs) {
				t.Fatalf("skipped %d, want %d", len(sk), len(c.ptrs))
			}
			for i, s := range sk {
				if s.Pointer != c.ptrs[i] || s.Problem.Line != c.lines[i] {
					t.Errorf("skip %d: %s line %d", i, s.Pointer, s.Problem.Line)
				}
			}
			if c.whole == "" {
				if p != nil {
					t.Fatalf("unexpected problem %+v", p)
				}
				if c.check != nil {
					c.check(t, f)
				}
				return
			}
			if p == nil || !strings.Contains(p.Message, c.whole) || (c.wholeLine != 0 && p.Line != c.wholeLine) {
				t.Errorf("whole-file problem %+v", p)
			}
		})
	}
}

func TestYAMLSchemaZeroSkipFailsFirstProblem(t *testing.T) {
	d, _, err := YAMLSchema[skFile]([]byte(skipSchema), dupRule())
	if err != nil {
		t.Fatal(err)
	}
	_, p := d("f.yaml", []byte("name: f\nitems:\n  - id: A!\n    name: A\n"))
	if p == nil || p.Path != "/items/0/id" || p.Line != 3 {
		t.Errorf("problem %+v", p)
	}
}
