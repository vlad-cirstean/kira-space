package configfs

import (
	_ "embed"
	"strings"
	"testing"
)

//go:embed testdata/probe.schema.json
var probeSchema []byte

type probe struct {
	Name    string `json:"name"`
	Retries int    `json:"retries"`
	Mode    string `json:"mode"`
	Target  struct {
		URL     string            `json:"url"`
		Headers map[string]string `json:"headers"`
	} `json:"target"`
	Tags []string `json:"tags"`
}

func probeDecode(t *testing.T, rules ...Rule[probe]) DecodeFunc[probe] {
	t.Helper()
	d, _, err := YAMLSchema[probe](probeSchema, rules...)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestSchema_problemLines(t *testing.T) {
	d := probeDecode(t)
	cases := []struct {
		name, src, path string
		line            int
		contains        string
	}{
		{"missing required at parent", "name: a\ntarget:\n  headers: {}\n", "/target/url", 2, "target.url is required"},
		{"nested type", "name: a\ntarget:\n  url: x\n  headers:\n    a: 1\n", "/target/headers/a", 5, "must be of type string"},
		{"sequence item", "name: a\ntarget:\n  url: x\ntags:\n  - ok\n  - BAD\n", "/tags/1", 6, "invalid format"},
		{"additional property key", "name: a\ntarget:\n  url: x\nbogus: 1\n", "/bogus", 4, "not a known option"},
		{"range", "name: a\nretries: 9\ntarget:\n  url: x\n", "/retries", 2, "at most 5"},
		{"enum", "name: a\nmode: wild\ntarget:\n  url: x\n", "/mode", 2, "one of"},
		{"anchor", "name: &n a\ntarget:\n  url: x\n", "", 1, "anchors"},
		{"alias", "name: a\ntarget: &t\n  url: x\ncopy: *t\n", "", 2, "anchors"},
		{"merge key", "name: a\ntarget:\n  <<: {url: x}\n", "", 3, "anchors"},
		{"syntax", "name: a\n  bad: [\n", "", 2, "syntax"},
	}
	for _, c := range cases {
		_, p := d("probe.yaml", []byte(c.src))
		if p == nil {
			t.Errorf("%s: accepted", c.name)
			continue
		}
		if p.Line != c.line || (c.path != "" && p.Path != c.path) || !strings.Contains(p.Message, c.contains) {
			t.Errorf("%s: got line %d path %q msg %q", c.name, p.Line, p.Path, p.Message)
		}
	}
}

func TestSchema_validAndRules(t *testing.T) {
	rule := Rule[probe]{Doc: "name must not be reserved.", Check: func(v probe, _ Locator) *Problem {
		if v.Name == "reserved" {
			return &Problem{Path: "/name", Message: "name is reserved."}
		}
		return nil
	}}
	d := probeDecode(t, rule)
	v, p := d("probe.yaml", []byte("name: a\ntarget:\n  url: x\n"))
	if p != nil || v.Name != "a" {
		t.Fatalf("valid: %+v %+v", v, p)
	}
	_, p = d("probe.yaml", []byte("# c\nname: reserved\ntarget:\n  url: x\n"))
	if p == nil || p.Line != 2 {
		t.Fatalf("rule problem line: %+v", p)
	}
}
