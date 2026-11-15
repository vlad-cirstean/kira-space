package taskfiles

import (
	"fmt"
	"strings"
	"testing"
)

func summarize(cs []Change) string {
	var out []string
	for _, c := range cs {
		out = append(out, fmt.Sprintf("%s:%s%s%s", c.Op, c.Key, c.Field, map[bool]string{true: "(slot)"}[c.Slot]))
	}
	return strings.Join(out, " ")
}

func TestDiffLinks(t *testing.T) {
	doc := LinkEntry{Kind: "doc", Title: "Design", URL: "https://x.test/design"}
	fix := LinkEntry{Kind: "link", Title: "Fixture", Ref: "testdata/orders.csv"}
	base := Links{PR: "https://x.test/pull/1", Entries: []LinkEntry{doc, fix}}
	cases := []struct {
		name  string
		after Links
		want  string
	}{
		{"same", base, ""},
		{"add by url", Links{PR: base.PR, Entries: []LinkEntry{doc, fix, {Kind: "doc", Title: "Spec", URL: "https://x.test/spec"}}}, "create:https://x.test/spec"},
		{"title edit keeps key", Links{PR: base.PR, Entries: []LinkEntry{{Kind: "doc", Title: "Design v2", URL: doc.URL}, fix}}, "edit:https://x.test/design"},
		{"ref entry key is kind and ref", Links{PR: base.PR, Entries: []LinkEntry{doc, {Kind: "link", Title: "Fixture", Ref: "testdata/other.csv"}}}, "create:link\x00testdata/other.csv delete:link\x00testdata/orders.csv"},
		{"remove", Links{PR: base.PR, Entries: []LinkEntry{doc}}, "delete:link\x00testdata/orders.csv"},
		{"pr replaced", Links{PR: "https://x.test/pull/2", Entries: base.Entries}, "edit:pr(slot)"},
		{"pr cleared", Links{Entries: base.Entries}, "delete:pr(slot)"},
		{"jira filled", Links{PR: base.PR, JiraKey: "ABC-1", JiraURL: "https://j.test/ABC-1", Entries: base.Entries}, "create:jira(slot)"},
	}
	for _, tc := range cases {
		if got := summarize(DiffLinks(base, tc.after)); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
	empty := Links{}
	if got := summarize(DiffLinks(empty, Links{PR: "https://x.test/p"})); got != "create:pr(slot)" {
		t.Errorf("empty PR slot filled: %q", got)
	}
}

func TestDiffProof(t *testing.T) {
	a, b, c := Step{"totals", "h1"}, Step{"flow", "h2"}, Step{"why", "h3"}
	base := ProofHead{Name: "Orders", Summary: "s", Steps: []Step{a, b}}
	cases := []struct {
		name  string
		after ProofHead
		want  string
	}{
		{"same", base, ""},
		{"edit by id", ProofHead{Name: "Orders", Summary: "s", Steps: []Step{{"totals", "h9"}, b}}, "edit:totals"},
		{"add", ProofHead{Name: "Orders", Summary: "s", Steps: []Step{a, b, c}}, "create:why"},
		{"remove", ProofHead{Name: "Orders", Summary: "s", Steps: []Step{a}}, "delete:flow"},
		{"rename", ProofHead{Name: "Orders 2", Summary: "s", Steps: []Step{a, b}}, "edit:name"},
		{"summary", ProofHead{Name: "Orders", Summary: "t", Steps: []Step{a, b}}, "edit:summary"},
		{"reorder", ProofHead{Name: "Orders", Summary: "s", Steps: []Step{b, a}}, "edit:order"},
		{"replace id is remove plus add", ProofHead{Name: "Orders", Summary: "s", Steps: []Step{{"t2", "h1"}, b}}, "create:t2 delete:totals"},
		{"edit and add keep order stable", ProofHead{Name: "Orders", Summary: "s", Steps: []Step{a, {"flow", "hx"}, c}}, "edit:flow create:why"},
	}
	for _, tc := range cases {
		if got := summarize(DiffProof(base, tc.after)); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestTextDiff(t *testing.T) {
	if TextDiff("a.md", "x\n", "x\n") != "" {
		t.Fatal("equal texts have a diff")
	}
	got := TextDiff("a.md", "one\ntwo", "one\nthree")
	if !strings.Contains(got, "-two") || !strings.Contains(got, "+three") {
		t.Fatalf("diff = %q", got)
	}
}
